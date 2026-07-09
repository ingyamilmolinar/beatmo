//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// newMobileSynthTabGameForTest builds a *Game whose Synth tab is laid out under
// the MOBILE profile (browser runtime + forced small-screen + mobile EQ mode),
// so synthPreviewWidth returns 0 and the stacked mobile focus band should be
// reserved above the knob grid. Mirrors setupSynthFooterEnv
// (synth_panel_footer_hit_test.go), which is the proven mobile-synth-tab setup.
func newMobileSynthTabGameForTest(t *testing.T) *Game {
	t.Helper()
	// Typical mobile viewport (short iPhone-class portrait). Under the OLD
	// surplus gate the focus band was EMPTY here (sections height ~185px <
	// the old minGridH=280 floor) — only ~1280px-tall panels got a band. The
	// always-on compact band must be non-empty even at this realistic size.
	return newMobileSynthTabGameForTestSize(t, 390, 720)
}

// newMobileSynthTabGameForTestSize is newMobileSynthTabGameForTest with an
// explicit mobile viewport, used to assert knob-column height at a taller panel.
func newMobileSynthTabGameForTestSize(t *testing.T, w, h int) *Game {
	t.Helper()
	restoreProfile := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restoreProfile)
	// Drive the adaptive mobile split so the audio-panel content floor
	// (mobileAudioPanelMinContentH) actually sizes the drum/synth pane —
	// otherwise Layout keeps the default 0.5 split under `go test` and the
	// knob column never sees Task 1's floor. Idempotent: callers may invoke
	// this helper in a loop within one test (e.g. mobileSplitGame).
	if !forceAutoSize {
		forceAutoSize = true
		t.Cleanup(func() { forceAutoSize = false })
	}

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(w, h)

	uiNode := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = uiNode.ID
	g.drum.Rows[0].Node = uiNode
	g.drum.Rows[0].Name = "Modular"
	g.drum.Rows[0].Instrument = "modular"
	audio.BindInstrumentToRecipe("modular", "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams("modular") })

	g.drum.SetMobileEQMode(true)
	g.drum.eqPanelZone.SetActiveTab(TabSynth)
	// Re-run Layout now that we are in mobile EQ mode so the adaptive split takes
	// the audio-tab branch (and applies mobileAudioPanelMinContentH). The first
	// Layout above ran before SetMobileEQMode, so it sized the split for Pads.
	g.Layout(w, h)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	g.Update() // flush layout/hit index via tree
	return g
}

func TestMobileSynthTab_WavesRightOfKnobs(t *testing.T) {
	g := newMobileSynthTabGameForTest(t)
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		t.Skip("no synth instrument")
	}
	waves := dv.synthMobileFocusRect()
	knobs := dv.instEditorDetailR
	if waves.Empty() {
		t.Fatalf("mobile wave column is empty — focus graph unreachable on mobile")
	}
	if knobs.Empty() {
		t.Fatalf("mobile knob/detail column is empty")
	}
	if waves.Min.X < knobs.Max.X {
		t.Fatalf("wave column (%v) must be right of the knob column (%v)", waves, knobs)
	}
	if waves.Dy() < knobs.Dy()/2 {
		t.Fatalf("wave column height %d too short vs knob column %d", waves.Dy(), knobs.Dy())
	}
}

// TestMobileSynthTab_KnobColumnHasUsableHeight guards Task 1's payoff: putting
// the waves in the RIGHT column gives the knob grid the full panel body height.
// The knob column body (detail rect minus its header) must fit at least one good
// knob row, and the first knob's dial must be at least the density minimum.
func TestMobileSynthTab_KnobColumnHasUsableHeight(t *testing.T) {
	g := newMobileSynthTabGameForTest(t)
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		t.Skip("no synth instrument")
	}
	d := Profile().DensityValues()
	rowUnit := d.SynthKnobMin + d.SynthKnobCaptionH
	body := dv.instEditorDetailR.Dy() - dv.instEditorDetailHeaderH
	// At the realistic 390x720 mobile viewport the knob column must fit at least
	// one full knob row (the Task-1 goal: knobs are no longer cramped to a sliver).
	// The short 390x720 panel only affords ~1 row of body height, so 2 rows is
	// asserted separately at a taller (but still real iPhone-class) viewport.
	if body < rowUnit {
		t.Fatalf("knob column body height %d < 1 knob row (%d) — knobs still cramped", body, rowUnit)
	}
	// At a taller realistic mobile viewport (390x896 — iPhone 11 Pro Max class)
	// the column must comfortably fit at least two knob rows.
	gTall := newMobileSynthTabGameForTestSize(t, 390, 896)
	dvTall := gTall.drum
	if dvTall.resolveSynthInstrument(dvTall.synthTabActiveInstrument()) != "" {
		dTall := Profile().DensityValues()
		rowUnitTall := dTall.SynthKnobMin + dTall.SynthKnobCaptionH
		bodyTall := dvTall.instEditorDetailR.Dy() - dvTall.instEditorDetailHeaderH
		if bodyTall < 2*rowUnitTall {
			t.Fatalf("tall knob column body height %d < 2 knob rows (%d) — knobs still cramped at 390x896", bodyTall, 2*rowUnitTall)
		}
	}
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Skip("no section knobs")
	}
	k := dv.instEditorKnobs[sec.knobIdxs[0]]
	if k.Rect().Empty() || k.Rect().Dx() < d.SynthKnobMin {
		t.Fatalf("first knob rect %v too small (min dia %d)", k.Rect(), d.SynthKnobMin)
	}
}

// TestMobileSynthTab_FocusGraphRespondsToKnobSelect drives the real draw path:
// on a typical mobile panel, after selecting a knob, drawSynthTab must paint ink
// INSIDE the mobile focus band — proving the focus graph actually renders on
// mobile (not just that a rect was reserved).
func TestMobileSynthTab_FocusGraphRespondsToKnobSelect(t *testing.T) {
	g := newMobileSynthTabGameForTest(t)
	dv := g.drum
	inst := dv.resolveSynthInstrument(dv.synthTabActiveInstrument())
	if inst == "" {
		t.Skip("no synth instrument")
	}
	band := dv.synthMobileFocusRect()
	if band.Empty() {
		t.Fatalf("mobile focus band empty — cannot render focus graph")
	}
	// Select a knob from the currently open section so the focus graph has a
	// concrete domain to draw.
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Skip("no open section with knobs on mobile")
	}
	dv.setSynthSelectedKnob(inst, sec.knobIdxs[0])

	contentR := dv.eqPanelZone.ContentRect()
	rects := collectFilledRects(t, func() {
		dv.drawSynthTab(ebiten.NewImage(g.winW, g.winH), contentR, inst)
	})
	// Re-read the band: drawSynthTab calls buildSynthTab (via layout) but in this
	// path the band was already laid out; read after draw to be safe.
	band = dv.synthMobileFocusRect()
	ink := 0
	for _, r := range rects {
		if r.Rect.Overlaps(band) {
			ink++
		}
	}
	if ink == 0 {
		t.Fatalf("no ink painted inside the mobile focus band %v — focus graph not rendering on mobile", band)
	}
}

// TestMobileSynthTab_KnobRendersAtIdealSize proves Task 1's content floor gives
// the synth knob column enough height for a FULL-size dial at a tall mobile
// viewport. k.Rect().Dx() is the dial diameter (placeKnobsInSection sets the
// rect width to the clamped diameter d; height adds the caption row).
func TestMobileSynthTab_KnobRendersAtIdealSize(t *testing.T) {
	g := newMobileSynthTabGameForTestSize(t, 390, 844)
	dv := g.drum
	d := Profile().DensityValues()
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) == 0 {
		t.Skip("no section knobs")
	}
	k := dv.instEditorKnobs[sec.knobIdxs[0]]
	if k.Rect().Empty() {
		t.Fatalf("first knob not laid out")
	}
	if k.Rect().Dx() < d.SynthKnobIdeal {
		t.Fatalf("knob diameter %d < ideal %d (still shrinking)", k.Rect().Dx(), d.SynthKnobIdeal)
	}
}

// TestMobileSynthSectionGridMultiColumn pins the mobile Synth stage-card knob
// grid to ControlGrid's width-adaptive column count capped at 4 (was a hard
// single column), matching the Sampler tab, so stage cards use the panel's
// width instead of one knob per row.
func TestMobileSynthSectionGridMultiColumn(t *testing.T) {
	g := newMobileSynthTabGameForTestSize(t, 390, 844)
	dv := g.drum
	sec := dv.synthSelectedSection()
	if sec == nil || len(sec.knobIdxs) < 2 {
		t.Skip("no open section with >=2 knobs on mobile")
	}
	grid := dv.sectionGrid(sec.id)
	if grid == nil {
		t.Fatal("selected synth section has no grid")
	}
	if c := grid.Cols(); c < 2 || c > 4 {
		t.Errorf("mobile synth section grid Cols()=%d, want 2..4 (adaptive, capped at 4)", c)
	}
}
