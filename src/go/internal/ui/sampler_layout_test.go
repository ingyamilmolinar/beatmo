//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// samplerLayoutGame builds a Game with the sampler tab active for direct
// buildSamplerTab geometry assertions.
func samplerLayoutGame(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	g.drum.eqPanelZone.SetActiveTab(TabSampler)
	return g
}

// rectInside reports whether inner sits fully within outer (inclusive edges).
func rectInside(inner, outer image.Rectangle) bool {
	if inner.Empty() {
		return true // nothing drawn → can't overflow
	}
	return inner.Min.X >= outer.Min.X && inner.Min.Y >= outer.Min.Y &&
		inner.Max.X <= outer.Max.X && inner.Max.Y <= outer.Max.Y
}

// assertSamplerNoOverflow checks every laid-out sampler element is inside
// contentR — the regression guard for the desktop "Rev/Norm/Fade/Preview
// drawn below the panel" bug.
func assertSamplerNoOverflow(t *testing.T, dv *DrumView, contentR image.Rectangle) {
	t.Helper()
	s := &dv.sampler
	if !rectInside(s.waveformRect, contentR) {
		t.Errorf("waveformRect %v overflows contentR %v", s.waveformRect, contentR)
	}
	for i, k := range s.knobs {
		if k == nil {
			continue
		}
		if !rectInside(k.Rect(), contentR) {
			t.Errorf("knob[%d] %v overflows contentR %v", i, k.Rect(), contentR)
		}
	}
	for _, b := range dv.samplerButtons {
		if b.btn == nil {
			continue
		}
		if !rectInside(b.btn.Rect(), contentR) {
			t.Errorf("button %q %v overflows contentR %v", b.tag, b.btn.Rect(), contentR)
		}
	}
}

func TestSamplerLayoutNoOverflowDesktop(t *testing.T) {
	g := samplerLayoutGame(t)
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	g.drum.sampler.captureFromSynth(roster.Kick)
	contentR := image.Rect(0, 0, 1280, 180) // realistic desktop EQ-panel content
	g.drum.buildSamplerTab(contentR, roster.Kick)
	assertSamplerNoOverflow(t, g.drum, contentR)
}

func TestSamplerLayoutNoOverflowMobile(t *testing.T) {
	g := samplerLayoutGame(t)
	withSmallScreen(t, true)
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	g.drum.sampler.captureFromSynth(roster.Kick)
	contentR := image.Rect(0, 0, 390, 270)
	g.drum.buildSamplerTab(contentR, roster.Kick)
	assertSamplerNoOverflow(t, g.drum, contentR)
}

func TestSamplerHeaderTitleClearsButtons(t *testing.T) {
	g := samplerLayoutGame(t)
	g.drum.sampler.captureFromSynth(roster.Kick)
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 180), roster.Kick)
	// The header's right-aligned action buttons must never overlap the SAMPLER
	// title on the left.
	titleEnd := g.drum.sampler.headerRect.Min.X + samplerTitlePad + TextWidth("SAMPLER")
	for _, tag := range []string{"sampler-save-as"} {
		b := g.drum.samplerButtonByTag(tag)
		if b == nil {
			t.Fatalf("missing button %q", tag)
		}
		if b.Rect().Min.X < titleEnd {
			t.Errorf("button %q starts at x=%d, overlaps SAMPLER title ending at x=%d",
				tag, b.Rect().Min.X, titleEnd)
		}
	}
}

func TestSamplerHeaderButtonsFitLabels(t *testing.T) {
	g := samplerLayoutGame(t)
	g.drum.sampler.captureFromSynth(roster.Kick)
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 180), roster.Kick)
	for _, tc := range []struct{ tag, label string }{
		{"sampler-save-as", "Save As"},
	} {
		b := g.drum.samplerButtonByTag(tc.tag)
		if b == nil {
			t.Errorf("missing button %q", tc.tag)
			continue
		}
		if b.Rect().Dx() < TextWidth(tc.label) {
			t.Errorf("button %q width %d < label %q width %d (would truncate)",
				tc.tag, b.Rect().Dx(), tc.label, TextWidth(tc.label))
		}
	}
}



func TestSamplerResetMobileRelocatedToStickyBar(t *testing.T) {
	g := samplerLayoutGame(t)
	withSmallScreen(t, true)
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	g.drum.sampler.captureFromSynth(roster.Kick)
	g.drum.buildSamplerTab(image.Rect(0, 0, 390, 270), roster.Kick)

	// Approach B: on mobile the Preview/Save As actions relocate to the sticky
	// bar, so the in-panel Save As (and Preview) rects are empty —
	// only the Rev/Norm/Fade transforms row stays in-panel.
	for _, tag := range []string{"sampler-save-as", "sampler-preview"} {
		if b := g.drum.samplerButtonByTag(tag); b != nil && !b.Rect().Empty() {
			t.Errorf("%s still laid out in-panel on mobile (should be on the sticky bar)", tag)
		}
	}
	// The transforms row is still present in-panel.
	if b := g.drum.samplerButtonByTag("sampler-reverse"); b == nil || b.Rect().Empty() {
		t.Errorf("Rev toggle must stay in the in-panel transforms row on mobile")
	}
}

func TestSamplerEmptyStateDisablesActions(t *testing.T) {
	g := samplerLayoutGame(t)
	// No instrument selected (instID == "") → nothing to auto-load → empty
	// buffer. (A selected instrument always auto-loads, so it is never empty.)
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 180), "")
	dv := g.drum

	// Save / Save As / Preview / toggles disabled.
	for _, tag := range []string{"sampler-save-as", "sampler-preview", "sampler-reverse", "sampler-normalize", "sampler-fade"} {
		b := dv.samplerButtonByTag(tag)
		if b == nil {
			t.Errorf("missing button %q", tag)
			continue
		}
		if b.SpecID != ComponentButtonDisabled {
			t.Errorf("button %q SpecID=%v, want ComponentButtonDisabled in empty state", tag, b.SpecID)
		}
	}
	// Knobs not laid out.
	for i, k := range dv.sampler.knobs {
		if k != nil && !k.Rect().Empty() {
			t.Errorf("knob[%d] has rect %v in empty state; want empty (no buffer)", i, k.Rect())
		}
	}
	// Hit areas exclude knobs/handles/save in empty state.
	for _, a := range dv.samplerTabHitAreas() {
		switch {
		case a.Tag == "sampler-save-as", a.Tag == "sampler-preview":
			t.Errorf("hit area %q present in empty state; should be inert", a.Tag)
		case len(a.Tag) >= 13 && a.Tag[:13] == "sampler-knob-":
			t.Errorf("knob hit area %q present in empty state", a.Tag)
		}
	}
}

func TestSamplerLoadedStateEnablesActions(t *testing.T) {
	g := samplerLayoutGame(t)
	g.drum.sampler.captureFromSynth(roster.Kick)
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 180), roster.Kick)
	dv := g.drum

	save := dv.samplerButtonByTag("sampler-save-as")
	if save == nil || save.SpecID == ComponentButtonDisabled {
		t.Error("Save As must be enabled when a buffer is loaded")
	}
	for i, k := range dv.sampler.knobs {
		if k == nil || k.Rect().Empty() {
			t.Errorf("knob[%d] empty rect when buffer loaded", i)
		}
	}
}

func TestSamplerKnobCardSizeFromDensity(t *testing.T) {
	g := samplerLayoutGame(t)
	g.drum.sampler.captureFromSynth(roster.Kick)
	bigR := image.Rect(0, 0, 1280, 320) // ample room so density, not space, governs

	// The card HEIGHT is the density-driven ParamCardH token; its width clamps to
	// the (narrow) control-column cell, so height is the language-invariant proof
	// that sizing follows density.
	measure := func(d Density) int {
		restore := SetDensityForTest(d)
		defer restore()
		g.drum.buildSamplerTab(bigR, roster.Kick)
		return g.drum.sampler.knobs[samplerKnobStart].Rect().Dy()
	}
	compact := measure(DensityCompact)
	spacious := measure(DensitySpacious)
	if compact <= 0 || spacious <= 0 {
		t.Fatalf("card heights not set (compact=%d spacious=%d)", compact, spacious)
	}
	if compact >= spacious {
		t.Errorf("card height not density-driven: compact=%d should be < spacious=%d", compact, spacious)
	}
}

func TestSamplerWaveformScratchReused(t *testing.T) {
	s := &samplerState{}
	s.captureFromSynth(roster.Kick)
	a := s.waveFloat64()
	capA := cap(a)
	b := s.waveFloat64()
	if cap(b) != capA {
		t.Errorf("waveFloat64 reallocated: cap %d → %d (want stable scratch)", capA, cap(b))
	}
	if len(b) != len(s.raw) {
		t.Errorf("waveFloat64 len=%d, want raw len %d", len(b), len(s.raw))
	}
}

func TestSamplerCaptureEmptyReportsStatus(t *testing.T) {
	restore := SwapSamplerCaptureFnForTest(func(id instrument.ID) ([]float32, int) { return nil, 0 })
	defer SwapSamplerCaptureFnForTest(restore)

	s := &samplerState{}
	s.captureFromSynth("anything")
	if s.hasBuffer() {
		t.Fatal("expected no buffer when capture returns empty")
	}
	if s.status == "" {
		t.Error("empty capture must set a status message (silent no-op is the bug)")
	}
}

// TestSamplerButtonsDoNotOverlapMobile guards the mobile header crowding bug
// (title + Save + Save As don't all fit one 390-px row, so labels truncated
// and overlapped). Save / Save As move to the action row on mobile; no two
// laid-out buttons may intersect.
func TestSamplerButtonsDoNotOverlapMobile(t *testing.T) {
	g := samplerLayoutGame(t)
	withSmallScreen(t, true)
	restore := SetDensityForTest(DensitySpacious)
	defer restore()
	g.drum.sampler.captureFromSynth(roster.Kick)
	g.drum.buildSamplerTab(image.Rect(0, 0, 390, 270), roster.Kick)

	type named struct {
		tag string
		r   image.Rectangle
	}
	var rects []named
	for _, b := range g.drum.samplerButtons {
		if b.btn == nil || b.btn.Rect().Empty() {
			continue
		}
		rects = append(rects, named{b.tag, b.btn.Rect()})
	}
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if !rects[i].r.Intersect(rects[j].r).Empty() {
				t.Errorf("buttons %q %v and %q %v overlap on mobile",
					rects[i].tag, rects[i].r, rects[j].tag, rects[j].r)
			}
		}
	}
}

func TestSamplerKnobsHavePlainEnglish(t *testing.T) {
	for i := 0; i < samplerKnobCount; i++ {
		if samplerKnobPlainEnglish(i) == "" {
			t.Errorf("knob %d has no plain-English gloss", i)
		}
	}
}

// newMobileSamplerTabGameForTestSize builds a *Game on the mobile Sampler tab
// with the adaptive mobile split applied (forceAutoSize), so the sampler control
// column is sized by Task 1's audio-panel content floor — the real layout a user
// sees, not a synthetic buildSamplerTab rect.
func newMobileSamplerTabGameForTestSize(t *testing.T, w, h int) *Game {
	t.Helper()
	restoreProfile := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restoreProfile)
	if !forceAutoSize {
		forceAutoSize = true
		t.Cleanup(func() { forceAutoSize = false })
	}

	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(w, h)
	mobileSamplerTabSetup(true)(g) // mobile profile + EQ mode + sampler tab + captured buffer
	// Re-run Layout now that the Sampler tab is active so the adaptive split takes
	// the audio-tab branch and applies mobileAudioPanelMinContentH.
	g.Layout(w, h)
	g.Update()
	return g
}

// TestSamplerControlsClearKnobRowMobile proves the sampler control-button row
// (Rev/Norm/Fade/Preview) sits at or below the knob row on a real mobile Sampler
// tab — i.e. Task 1's content floor gives the control column enough height that
// the buttons never overlap the knob labels.
func TestSamplerControlsClearKnobRowMobile(t *testing.T) {
	g := newMobileSamplerTabGameForTestSize(t, 390, 844)
	dv := g.drum
	if !dv.sampler.hasBuffer() {
		t.Skip("no sampler buffer captured")
	}
	lowestBtnTop := 1 << 30
	for _, tag := range []string{"sampler-reverse", "sampler-normalize", "sampler-fade", "sampler-preview"} {
		b := dv.samplerButtonByTag(tag)
		if b == nil || b.Rect().Empty() {
			continue
		}
		if b.Rect().Min.Y < lowestBtnTop {
			lowestBtnTop = b.Rect().Min.Y
		}
	}
	if lowestBtnTop == 1<<30 {
		t.Skip("no sampler control buttons laid out")
	}
	// knobBottom = the lowest laid-out sampler knob cell's Max.Y (the knob rect
	// spans dial + caption + step-badge band, so its Max.Y is the row bottom).
	knobBottom := 0
	for _, k := range dv.sampler.knobs {
		if k == nil || k.Rect().Empty() {
			continue
		}
		if k.Rect().Max.Y > knobBottom {
			knobBottom = k.Rect().Max.Y
		}
	}
	if knobBottom == 0 {
		t.Skip("no sampler knobs laid out")
	}
	if lowestBtnTop < knobBottom {
		t.Fatalf("sampler buttons (top %d) overlap the knob row (bottom %d)", lowestBtnTop, knobBottom)
	}
}
