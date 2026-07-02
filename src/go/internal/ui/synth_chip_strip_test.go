//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Synth tab redesign — pipeline chip strip + expand-one detail pane.
//
// Collapsed stages render as compact chips laid out in audio-pipeline order
// (VOICE·OSC·FM·PITCH·LFO·BURST·ENVELOPE·FILTER·FILTER ENV·POST); exactly one stage is
// expanded at a time into a full-width detail pane below the strip. Disabled
// stages stay visible as dimmed ghost chips. Selection is per-session,
// per-instrument state on DrumView (no userprefs).

// Shared helpers (layoutSynthTab, newModularSynthTabGame, chipByID) live in
// synth_chip_strip_helpers_test.go — UNTAGGED, because untagged test files
// (synth_modular_toggle_test.go etc.) also use them and must keep compiling
// in the real-Ebiten (no `test` tag) build.

// TestSynthChips_StripShowsEveryStageInPipelineOrder pins the chip strip
// contract on desktop: one chip per surviving section, same order as
// synthSectionOrderForSchema, every chip rect visible (non-empty), laid out
// left→right in a single row above the detail pane.
func TestSynthChips_StripShowsEveryStageInPipelineOrder(t *testing.T) {
	g := newSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	sections := dv.SynthTabSections()
	chips := dv.instEditorChips
	if len(chips) != len(sections) {
		t.Fatalf("got %d chips, want one per section (%d)", len(chips), len(sections))
	}
	prevMaxX := -1 << 30
	for i, c := range chips {
		if c.id != sections[i].id {
			t.Errorf("chip[%d] id=%v, want section order %v", i, c.id, sections[i].id)
		}
		if c.rect.Empty() {
			t.Errorf("chip[%d] (%v) has empty rect — every stage must stay visible", i, c.id)
			continue
		}
		if c.rect.Min.X < prevMaxX {
			t.Errorf("chip[%d] (%v) min.X=%d overlaps previous chip maxX=%d — desktop strip must be a single left→right row", i, c.id, c.rect.Min.X, prevMaxX)
		}
		prevMaxX = c.rect.Min.X
		if !dv.instEditorDetailR.Empty() && c.rect.Max.Y > dv.instEditorDetailR.Min.Y {
			t.Errorf("chip[%d] (%v) rect %v dips below detail pane top %d", i, c.id, c.rect, dv.instEditorDetailR.Min.Y)
		}
	}
	if dv.instEditorDetailR.Empty() {
		t.Fatalf("detail pane rect is empty — the selected stage needs a knob area")
	}
}

// TestSynthChips_DefaultSelectionFirstEnabledWithKnobs pins the default
// selection rule: the first section in pipeline order that is enabled AND has
// at least one knob. Computed generically from the section model so the test
// holds for every recipe.
func TestSynthChips_DefaultSelectionFirstEnabledWithKnobs(t *testing.T) {
	for _, mk := range []struct {
		name string
		game func(*testing.T) *Game
		inst string
	}{
		{"drum-snare", newSynthTabGame, "snare"},
		{"synth-modular", newModularSynthTabGame, "ut-chip-modular"},
	} {
		t.Run(mk.name, func(t *testing.T) {
			g := mk.game(t)
			layoutSynthTab(t, g)
			dv := g.drum

			want := synthSectionID(-1)
			var fallback synthSectionID = -1
			for _, s := range dv.SynthTabSections() {
				if len(s.knobIdxs) == 0 {
					continue
				}
				if fallback == -1 {
					fallback = s.id
				}
				if s.enableParam == "" || synthStageEnabled(mk.inst, s.enableParam) {
					want = s.id
					break
				}
			}
			if want == -1 {
				want = fallback
			}
			if want == -1 {
				t.Skipf("recipe has no knobbed sections")
			}

			selCount := 0
			for _, c := range dv.instEditorChips {
				if c.selected {
					selCount++
					if c.id != want {
						t.Errorf("default selected chip = %v, want %v (first enabled section with knobs)", c.id, want)
					}
				}
			}
			if selCount != 1 {
				t.Errorf("got %d selected chips, want exactly 1", selCount)
			}
		})
	}
}

// TestSynthChips_OnlySelectedStageKnobsHaveRects pins the accordion core
// invariant: knobs belonging to the selected stage are laid out (at least the
// first is visible); every knob of every collapsed stage has an empty rect so
// it is neither drawn nor hit-testable.
func TestSynthChips_OnlySelectedStageKnobsHaveRects(t *testing.T) {
	g := newSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	var selected *synthSection
	for i := range dv.instEditorSections {
		s := &dv.instEditorSections[i]
		if chipByID(t, dv, s.id).selected {
			selected = s
		}
	}
	if selected == nil {
		t.Fatalf("no selected section")
	}
	if len(selected.knobIdxs) == 0 {
		t.Fatalf("selected section %v has no knobs — default rule must pick a knobbed section", selected.id)
	}
	if dv.instEditorKnobs[selected.knobIdxs[0]].Rect().Empty() {
		t.Errorf("selected section %v first knob rect is empty — detail pane must lay out its knobs", selected.id)
	}
	for _, s := range dv.instEditorSections {
		if s.id == selected.id {
			continue
		}
		for _, kIdx := range s.knobIdxs {
			if !dv.instEditorKnobs[kIdx].Rect().Empty() {
				t.Errorf("collapsed section %v knob %d has non-empty rect %v — collapsed stages must not lay out knobs", s.id, kIdx, dv.instEditorKnobs[kIdx].Rect())
			}
		}
	}
}

// TestSynthChips_SelectionSurvivesRelayout — selection is durable DrumView
// state re-derived every Layout, not sampled at build time.
func TestSynthChips_SelectionSurvivesRelayout(t *testing.T) {
	g := newSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	dv.setSelectedSynthSection("snare", synthSectionFilter)
	layoutSynthTab(t, g)
	layoutSynthTab(t, g)

	c := chipByID(t, dv, synthSectionFilter)
	if !c.selected {
		t.Errorf("FILTER selection did not survive re-layout")
	}
	for i := range dv.instEditorSections {
		s := dv.instEditorSections[i]
		if s.id == synthSectionFilter && len(s.knobIdxs) > 0 {
			if dv.instEditorKnobs[s.knobIdxs[0]].Rect().Empty() {
				t.Errorf("selected FILTER knobs not laid out after re-layout")
			}
		}
	}
}

// TestSynthChips_SelectionSurvivesTabSwitch — the open stage is DURABLE session
// state, not a transient opener: switching the audio panel away from the Synth
// tab and back (which runs resetTransientTabState) must NOT reset the selection.
// This is the positive counterpart to TestResetTransientTabState_TearsDownEvery
// Opener deliberately EXCLUDING the selection map.
func TestSynthChips_SelectionSurvivesTabSwitch(t *testing.T) {
	g := newSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	dv.setSelectedSynthSection("snare", synthSectionFilter)
	layoutSynthTab(t, g)
	if !chipByID(t, dv, synthSectionFilter).selected {
		t.Fatalf("precondition: FILTER not selected after initial layout")
	}

	// Leave the Synth tab (EQ) and come back — the reset chokepoint runs on the
	// way out. Selection must persist.
	dv.eqPanelZone.SetActiveTab(TabEQ)
	dv.resetTransientTabState()
	layoutSynthTab(t, g)

	if !chipByID(t, dv, synthSectionFilter).selected {
		t.Errorf("FILTER selection lost across a Synth→EQ→Synth tab round-trip — selection map was wrongly treated as transient")
	}
}

// TestSynthChips_SelectionPerInstrument — two instruments keep independent
// open stages.
func TestSynthChips_SelectionPerInstrument(t *testing.T) {
	g := newSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	dv.setSelectedSynthSection("snare", synthSectionFilter)

	// Switch the row to a second instrument and pick a different stage.
	dv.Rows[0].Instrument = "ut-chip-modular"
	audio.BindInstrumentToRecipe("ut-chip-modular", "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams("ut-chip-modular") })
	layoutSynthTab(t, g)
	dv.setSelectedSynthSection("ut-chip-modular", synthSectionEnvelope)
	layoutSynthTab(t, g)
	if !chipByID(t, dv, synthSectionEnvelope).selected {
		t.Fatalf("modular instrument should have ENVELOPE selected")
	}

	// Back to the first instrument: FILTER selection must be remembered.
	dv.Rows[0].Instrument = "snare"
	layoutSynthTab(t, g)
	if !chipByID(t, dv, synthSectionFilter).selected {
		t.Errorf("snare's FILTER selection lost after instrument round-trip")
	}
}

// TestSynthChips_DisabledStageIsGhostChip — a stage toggled off renders as a
// ghost chip (enabled=false) but stays in the strip; enabled stages report
// enabled=true.
func TestSynthChips_DisabledStageIsGhostChip(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	audio.SetInstrumentParam("ut-chip-modular", "filter_enabled", 0)
	layoutSynthTab(t, g)

	fc := chipByID(t, dv, synthSectionFilter)
	if fc.enabled {
		t.Errorf("FILTER chip should be ghost (enabled=false) after filter_enabled=0")
	}
	if fc.rect.Empty() {
		t.Errorf("ghost FILTER chip must stay visible in the strip")
	}
	oc := chipByID(t, dv, synthSectionOsc)
	if !oc.enabled {
		t.Errorf("OSC chip should report enabled=true at modular defaults")
	}
}

// TestSynthDetailPane_EnablePillForSelectedStage — the enable pill lives in
// the detail-pane header for the selected stage (non-empty rect when the
// stage carries an enable toggle; empty for VOICE which has none).
func TestSynthDetailPane_EnablePillForSelectedStage(t *testing.T) {
	g := newModularSynthTabGame(t)
	// Production-like panel height: the 640×480 harness leaves the audio
	// panel only 80 px tall, which legitimately drops the pill (degenerate
	// short pane). Real desktop panels are ~240 px. The panel claims its
	// expanded height on the Layout AFTER the tab activates, so activate
	// first, then resize.
	layoutSynthTab(t, g)
	g.Layout(1280, 720)
	layoutSynthTab(t, g)
	dv := g.drum

	dv.setSelectedSynthSection("ut-chip-modular", synthSectionFilter)
	layoutSynthTab(t, g)
	pill := dv.synthDetailEnablePillRect()
	if pill.Empty() {
		t.Fatalf("selected FILTER stage must show an enable pill in the detail header")
	}
	if !pill.In(dv.instEditorDetailR) {
		t.Errorf("enable pill %v must sit inside the detail pane %v", pill, dv.instEditorDetailR)
	}

	// VOICE never carries an enable pill. drum-snare's first section is VOICE.
	g2 := newSynthTabGame(t)
	layoutSynthTab(t, g2)
	g2.Layout(1280, 720)
	layoutSynthTab(t, g2)
	dv2 := g2.drum
	dv2.setSelectedSynthSection("snare", synthSectionVoice)
	layoutSynthTab(t, g2)
	if hasVoice := func() bool {
		for _, s := range dv2.SynthTabSections() {
			if s.id == synthSectionVoice {
				return true
			}
		}
		return false
	}(); hasVoice {
		if !dv2.synthDetailEnablePillRect().Empty() {
			t.Errorf("VOICE detail header must not show an enable pill")
		}
	}
}

// TestSynthChips_MobileWrapAllStagesVisible — at 360×800 portrait every chip
// stays visible (wrapping rows, no horizontal scroll/clipping) and the body
// below the strip splits into a left knob column + a right wave column that
// together span the content width (Task 1 mobile side-by-side layout).
func TestSynthChips_MobileWrapAllStagesVisible(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)

	g.drum.Rows[0].Instrument = "ut-chip-mobile"
	audio.BindInstrumentToRecipe("ut-chip-mobile", "drum-kick-punchy")
	t.Cleanup(func() { audio.ResetInstrumentParams("ut-chip-mobile") })

	g.drum.setViewMode(viewModeSynth)
	g.Update()
	g.Update()
	layoutSynthTab(t, g)
	dv := g.drum

	panel := dv.eqPanelZone.PanelRect()
	chips := dv.instEditorChips
	if len(chips) == 0 {
		t.Fatalf("no chips laid out on mobile")
	}
	rows := map[int]bool{}
	for _, c := range chips {
		if c.rect.Empty() {
			t.Errorf("chip %v empty on mobile — all stages must stay visible", c.id)
			continue
		}
		if c.rect.Min.X < panel.Min.X || c.rect.Max.X > panel.Max.X {
			t.Errorf("chip %v rect %v overflows panel width %v — mobile must wrap, not clip", c.id, c.rect, panel)
		}
		rows[c.rect.Min.Y] = true
	}
	if len(chips) >= 6 && len(rows) < 2 {
		t.Errorf("%d chips share one row at 360px — expected wrapping rows", len(chips))
	}
	if dv.instEditorDetailR.Empty() {
		t.Fatalf("mobile detail pane is empty")
	}
	// Task 1 split the mobile synth body into a LEFT knob column
	// (instEditorDetailR) and a RIGHT wave column (synthMobileFocusRect). The
	// detail pane is therefore no longer full-content-width; assert instead that
	// it owns a substantial left slice (>= ~45% of content) and that the two
	// columns together span the content width with the knob column on the left.
	detail := dv.instEditorDetailR
	focus := dv.synthMobileFocusRect()
	content := dv.eqPanelZone.ContentRect()
	if got, want := detail.Dx(), content.Dx()*45/100; got < want {
		t.Errorf("mobile knob column width %d too narrow (want >= %d) — knob column must own a substantial left slice", got, want)
	}
	if !focus.Empty() {
		if detail.Min.X > focus.Min.X {
			t.Errorf("mobile knob column %v must be left of the wave column %v", detail, focus)
		}
		if span := focus.Max.X - detail.Min.X; span < content.Dx()*3/4 {
			t.Errorf("knob+wave columns span %d too narrow (want >= %d) — columns must together span content width", span, content.Dx()*3/4)
		}
	}
}

// TestSynthChips_NoPerFrameChipAllocs — chips slice must be reused across
// Layout passes (Layout runs every frame).
func TestSynthChips_NoPerFrameChipAllocs(t *testing.T) {
	g := newSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum
	if cap(dv.instEditorChips) == 0 {
		t.Skip("no chips")
	}
	before := &dv.instEditorChips[:1][0]
	layoutSynthTab(t, g)
	after := &dv.instEditorChips[:1][0]
	if before != after {
		t.Errorf("instEditorChips backing array reallocated across Layout — reuse [:0]")
	}
	_ = image.Rectangle{}
}

// ---- Phase 2: chip input wiring ----------------------------------------

// findChipHitArea returns the published hit area for a stage's chip.
func findChipHitArea(t *testing.T, dv *DrumView, id synthSectionID) HitArea {
	t.Helper()
	wantTag := "synth-chip-" + sectionLabel(id)
	for _, ha := range dv.synthTabHitAreas() {
		if ha.Tag == wantTag {
			return ha
		}
	}
	t.Fatalf("no hit area tagged %q published", wantTag)
	return HitArea{}
}

// TestSynthChips_TapOpensStage — pressing an enabled chip selects its stage;
// the next layout expands it into the detail pane.
func TestSynthChips_TapOpensStage(t *testing.T) {
	g := newSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	target := synthSectionEnvelope
	if chipByID(t, dv, target).selected {
		t.Fatalf("test invariant: ENVELOPE must not be the default selection")
	}
	ha := findChipHitArea(t, dv, target)
	cx, cy := (ha.Rect.Min.X+ha.Rect.Max.X)/2, (ha.Rect.Min.Y+ha.Rect.Max.Y)/2
	if res := ha.Handler.OnPress(cx, cy); res == InputIgnored {
		t.Fatalf("chip press was ignored")
	}
	layoutSynthTab(t, g)
	if !chipByID(t, dv, target).selected {
		t.Errorf("ENVELOPE chip not selected after tap")
	}
}

// TestSynthChips_GhostChipTapOpensButDoesNotEnable — pressing a disabled
// (ghost) chip only OPENS (selects) the stage so the user can inspect it; it
// must NOT enable the stage. Enabling/disabling is an explicit action via the
// detail-pane pill only — a chip tap is visualization, not mutation.
func TestSynthChips_GhostChipTapOpensButDoesNotEnable(t *testing.T) {
	g := newModularSynthTabGame(t)
	auditioned := false
	restore := SwapSynthAuditionFnForTest(func(string) { auditioned = true })
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	audio.SetInstrumentParam("ut-chip-modular", "filter_enabled", 0)
	layoutSynthTab(t, g)
	dv := g.drum
	if chipByID(t, dv, synthSectionFilter).enabled {
		t.Fatalf("test invariant: FILTER chip should be ghost")
	}

	ha := findChipHitArea(t, dv, synthSectionFilter)
	cx, cy := (ha.Rect.Min.X+ha.Rect.Max.X)/2, (ha.Rect.Min.Y+ha.Rect.Max.Y)/2
	ha.Handler.OnPress(cx, cy)
	layoutSynthTab(t, g)

	if synthStageEnabled("ut-chip-modular", "filter_enabled") {
		t.Errorf("ghost chip tap must NOT enable the stage — only the pill enables")
	}
	if auditioned {
		t.Errorf("ghost chip tap must not audition — it makes no audible change")
	}
	c := chipByID(t, dv, synthSectionFilter)
	if !c.selected {
		t.Errorf("ghost chip tap must still OPEN (select) the stage for inspection")
	}
	if c.enabled {
		t.Errorf("chip must still report disabled after a tap (tap does not enable)")
	}
}

// TestSynthChips_HandleInput_GhostChipTapDoesNotEnable — same contract through
// the legacy direct input path (handleSynthTabInput).
func TestSynthChips_HandleInput_GhostChipTapDoesNotEnable(t *testing.T) {
	g := newModularSynthTabGame(t)
	restore := SwapSynthAuditionFnForTest(func(string) {})
	t.Cleanup(func() { SwapSynthAuditionFnForTest(restore) })

	audio.SetInstrumentParam("ut-chip-modular", "filter_enabled", 0)
	layoutSynthTab(t, g)
	dv := g.drum

	c := chipByID(t, dv, synthSectionFilter)
	cx, cy := (c.rect.Min.X+c.rect.Max.X)/2, (c.rect.Min.Y+c.rect.Max.Y)/2
	if !dv.handleSynthTabInput(cx, cy, true, "ut-chip-modular") {
		t.Fatalf("ghost chip tap not handled by handleSynthTabInput")
	}
	layoutSynthTab(t, g)

	if synthStageEnabled("ut-chip-modular", "filter_enabled") {
		t.Errorf("ghost chip tap via handleSynthTabInput must NOT enable the stage")
	}
	if !chipByID(t, dv, synthSectionFilter).selected {
		t.Errorf("ghost chip tap via handleSynthTabInput must still select the stage")
	}
}

// TestSynthChips_TapIgnoredWhileKnobCapturing — chip taps during a captured
// knob drag must not change the selection (a selection swap would zero the
// dragged knob's rect mid-gesture).
func TestSynthChips_TapIgnoredWhileKnobCapturing(t *testing.T) {
	g := newSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	var sel *synthSection
	for i := range dv.instEditorSections {
		if chipByID(t, dv, dv.instEditorSections[i].id).selected {
			sel = &dv.instEditorSections[i]
		}
	}
	if sel == nil || len(sel.knobIdxs) == 0 {
		t.Fatalf("no selected knobbed section")
	}
	k := dv.instEditorKnobs[sel.knobIdxs[0]]
	if k.Rect().Empty() {
		t.Fatalf("selected stage's first knob has no rect")
	}
	kc := k.Rect().Min.Add(k.Rect().Max).Div(2)
	k.HandleInputResult(kc.X, kc.Y, true) // press = capture
	if !k.Capturing() {
		t.Fatalf("knob did not capture the press")
	}
	t.Cleanup(func() { k.HandleInputResult(kc.X, kc.Y, false) })

	target := synthSectionEnvelope
	ha := findChipHitArea(t, dv, target)
	cx, cy := (ha.Rect.Min.X+ha.Rect.Max.X)/2, (ha.Rect.Min.Y+ha.Rect.Max.Y)/2
	ha.Handler.OnPress(cx, cy)
	layoutSynthTab(t, g)
	if chipByID(t, dv, target).selected {
		t.Errorf("chip tap during a captured knob drag must be ignored")
	}
}

// TestSynthChips_DetailPillHitAreaPublished — the enable pill publishes one
// hit area for the SELECTED stage only; collapsed stages publish neither
// pills nor scroll-body catch-alls (the image.Rect normalization trap).
func TestSynthChips_DetailPillHitAreaPublished(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	g.Layout(1280, 720)
	layoutSynthTab(t, g)
	dv := g.drum
	dv.setSelectedSynthSection("ut-chip-modular", synthSectionFilter)
	layoutSynthTab(t, g)

	pillTags := 0
	for _, ha := range dv.synthTabHitAreas() {
		if ha.Tag == "synth-toggle-filter_enabled" {
			pillTags++
			if !ha.Rect.In(dv.instEditorDetailR) {
				t.Errorf("pill hit area %v outside detail pane %v", ha.Rect, dv.instEditorDetailR)
			}
		} else if len(ha.Tag) > 12 && ha.Tag[:12] == "synth-toggle" {
			t.Errorf("collapsed stage published pill hit area %q", ha.Tag)
		}
		if len(ha.Tag) > 16 && ha.Tag[:16] == "synth-scrollbody" {
			if !ha.Rect.In(dv.instEditorDetailR) {
				t.Errorf("scroll-body hit area %q rect %v outside the detail pane %v (collapsed sections must publish none)", ha.Tag, ha.Rect, dv.instEditorDetailR)
			}
		}
	}
	if pillTags != 1 {
		t.Errorf("got %d pill hit areas for the selected stage, want exactly 1", pillTags)
	}
}

// TestSynthChips_HandleSynthTabInputSelects — the legacy direct input path
// (used by JS bridge fallbacks and tests) also routes chip taps.
func TestSynthChips_HandleSynthTabInputSelects(t *testing.T) {
	g := newSynthTabGame(t)
	layoutSynthTab(t, g)
	dv := g.drum

	target := synthSectionEnvelope
	c := chipByID(t, dv, target)
	cx, cy := (c.rect.Min.X+c.rect.Max.X)/2, (c.rect.Min.Y+c.rect.Max.Y)/2
	if !dv.handleSynthTabInput(cx, cy, true, "snare") {
		t.Fatalf("chip tap not handled by handleSynthTabInput")
	}
	layoutSynthTab(t, g)
	if !chipByID(t, dv, target).selected {
		t.Errorf("ENVELOPE not selected via handleSynthTabInput")
	}
}

// TestSynthChips_MobileChipTouchTargetMeets44px — on mobile, the visible chip
// is small (SynthChipH ~32px tall) but a tap anywhere within the 44px HIG
// minimum must still hit it. The redesign meets this with HitArea.Touch +
// a ClipRect skirt (NOT by enlarging the visible Rect, which would leak into
// the detail pane). Assert every published chip hit area is Touch-flagged and
// its ClipRect is at least touchMinTargetPx in both dimensions, so the
// effective (expand ∩ clip) touch region can never fall below 44px.
func TestSynthChips_MobileChipTouchTargetMeets44px(t *testing.T) {
	assertDefaultParityState(t)
	restore := SetForceSmallScreen(t, true)
	defer restore()

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 800)

	g.drum.Rows[0].Instrument = "ut-chip-touch"
	audio.BindInstrumentToRecipe("ut-chip-touch", "synth-modular")
	t.Cleanup(func() { audio.ResetInstrumentParams("ut-chip-touch") })

	g.drum.setViewMode(viewModeSynth)
	g.Update()
	g.Update()
	layoutSynthTab(t, g)
	dv := g.drum

	if TouchMinTarget() <= 0 {
		t.Fatalf("mobile profile reports no touch-min target (%d) — test cannot validate 44px", TouchMinTarget())
	}

	chipHits := 0
	for _, ha := range dv.synthTabHitAreas() {
		if len(ha.Tag) < 11 || ha.Tag[:11] != "synth-chip-" {
			continue
		}
		chipHits++
		if !ha.Touch {
			t.Errorf("chip hit area %q is not Touch-flagged — touch-min skirt inactive", ha.Tag)
		}
		if ha.ClipRect.Empty() {
			t.Errorf("chip hit area %q has empty ClipRect — touch expansion is unbounded (leaks into siblings)", ha.Tag)
			continue
		}
		if w := ha.ClipRect.Dx(); w < touchMinTargetPx {
			t.Errorf("chip %q ClipRect width %d < %d (touch-min)", ha.Tag, w, touchMinTargetPx)
		}
		if h := ha.ClipRect.Dy(); h < touchMinTargetPx {
			t.Errorf("chip %q ClipRect height %d < %d (touch-min)", ha.Tag, h, touchMinTargetPx)
		}
	}
	if chipHits == 0 {
		t.Fatalf("no chip hit areas published on mobile")
	}
}

// TestSynthChips_ChipWinsZOrderOverCatchAll — input-isolation z-order. Each
// chip hit area sits at the zone nominal-z + 1, so a tap at a chip's centre
// must resolve to that chip, never falling through to the EQ-panel catch-all
// (registered at nominal-z to keep taps off lower zones). Builds a HitIndex
// from the FULL set the panel publishes (catch-all + chips + knobs) and asserts
// At(chipCentre) returns the chip first.
func TestSynthChips_ChipWinsZOrderOverCatchAll(t *testing.T) {
	g := newModularSynthTabGame(t)
	layoutSynthTab(t, g)
	g.Layout(1280, 720)
	layoutSynthTab(t, g)
	dv := g.drum

	idx := &HitIndex{}
	idx.Update(dv.eqPanelZone.ID(), dv.eqPanelZone.HitAreas())

	for _, c := range dv.instEditorChips {
		if c.rect.Empty() {
			continue
		}
		cx := (c.rect.Min.X + c.rect.Max.X) / 2
		cy := (c.rect.Min.Y + c.rect.Max.Y) / 2
		hits := idx.At(cx, cy)
		if len(hits) == 0 {
			t.Errorf("chip %v centre (%d,%d) resolves to no hit area", c.id, cx, cy)
			continue
		}
		wantTag := "synth-chip-" + sectionLabel(c.id)
		if hits[0].Tag != wantTag {
			t.Errorf("chip %v centre resolves to %q (z=%d), want the chip %q on top — catch-all stole the tap",
				c.id, hits[0].Tag, hits[0].ZIndex, wantTag)
		}
	}
}
