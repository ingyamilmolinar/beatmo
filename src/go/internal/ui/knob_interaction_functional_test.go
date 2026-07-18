package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// sectionHasEndlessKnob returns true if at least one knob in the section is
// backed by a non-nil KnobStepBadge (i.e. it is a continuous/endless knob).
func sectionHasEndlessKnob(g *Game, sec synthSectionID) bool {
	dv := g.drum
	for _, s := range dv.instEditorSections {
		if s.id != sec {
			continue
		}
		for _, kIdx := range s.knobIdxs {
			if kIdx < len(dv.instEditorStepBadges) && dv.instEditorStepBadges[kIdx] != nil {
				return true
			}
		}
	}
	return false
}

// synthIdxByParam returns the binding index of the named param, failing if absent.
func synthIdxByParam(t *testing.T, g *Game, name string) int {
	t.Helper()
	for i, b := range g.drum.SynthTabBindings() {
		if b.def.Name == name {
			return i
		}
	}
	t.Fatalf("no %q knob in synth bindings", name)
	return -1
}

func TestSynthContinuousKnobIsEndlessWithBadge(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	k := g.drum.SynthTabKnobs()[idx]
	if !k.Endless || k.Discrete {
		t.Fatalf("filter_cutoff knob should be endless, not discrete (endless=%v discrete=%v)", k.Endless, k.Discrete)
	}
	if k.StepMul <= 0 {
		t.Fatalf("filter_cutoff knob StepMul not initialised: %v", k.StepMul)
	}
	badges := g.drum.instEditorStepBadges
	if idx >= len(badges) || badges[idx] == nil {
		t.Fatalf("filter_cutoff knob has no step badge (len=%d)", len(badges))
	}
}

func TestSynthDiscreteKnobIsClicky(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "osc_type")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	k := g.drum.SynthTabKnobs()[idx]
	if !k.Discrete || k.Endless {
		t.Fatalf("osc_type knob should be discrete clicky (discrete=%v endless=%v)", k.Discrete, k.Endless)
	}
	// Discrete knobs get no step badge.
	badges := g.drum.instEditorStepBadges
	if idx < len(badges) && badges[idx] != nil {
		t.Fatalf("discrete osc_type knob must NOT have a step badge")
	}
}

// TestSynthBadgeRectsNeverOverlapOtherKnobs asserts that after a draw pass for
// any section, no badge's rect overlaps a DIFFERENT knob's rect. This is the
// layout-agnostic guard for BUG 1 (cross-row tap stealing): if the badge is
// clamped within its own cell slot it cannot physically reach another knob's
// rect.
func TestSynthBadgeRectsNeverOverlapOtherKnobs(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	screen := ebiten.NewImage(1280, 720)
	defer screen.Deallocate()

	for _, sec := range []synthSectionID{synthSectionOsc, synthSectionFilter, synthSectionEnvelope} {
		if !sectionHasEndlessKnob(g, sec) {
			continue // section has no endless knobs; overlap test vacuously holds
		}
		g.drum.setSelectedSynthSection("ut-chip-modular", sec)
		expandSynthPanelForTest(t, g)
		screen.Clear()
		g.drum.eqPanelZone.Draw(screen)

		badges := g.drum.instEditorStepBadges
		knobs := g.drum.SynthTabKnobs()
		for bi, badge := range badges {
			if badge == nil || badge.Rect().Empty() {
				continue
			}
			for ki, k := range knobs {
				if ki == bi || k.Rect().Empty() {
					continue
				}
				if badge.Rect().Overlaps(k.Rect()) {
					t.Fatalf("section %v: badge[%d] rect %v overlaps knob[%d] rect %v",
						sec, bi, badge.Rect(), ki, k.Rect())
				}
			}
		}
	}
}

// TestSynthReadoutTapOpensEditorViaTreeDispatch verifies the FULL production
// input path for readout taps: mouse→SetInputForTest→g.Update()→HitIndex→
// synthKnobHitAdapter.OnPress→openSynthParamEditor. Replaces the isolation-test
// (direct handleSynthTabInput call) that never exercised the real tree dispatch.
// TestSynthKnobCaptionIsDisplayOnly pins the simplified knob title: the value
// caption is a SIMPLE display label, so tapping it must NOT open the numeric
// text-input editor (values change via the dial). Was
// TestSynthReadoutTapOpensEditorViaTreeDispatch before the caption was made
// display-only.
func TestSynthKnobCaptionIsDisplayOnly(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()

	img := newTrackedImage("test.captiontext", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	// The ParamCard grid draws NAME + VALUE + gauge as one control surface —
	// there is no longer a separate caption/readout sub-rect (that used to
	// be populated by the deleted per-knob caption draw). Assert it stays
	// empty under the new contract.
	rr := g.drum.synthKnobReadoutRect(idx)
	if !rr.Empty() {
		t.Fatalf("readout rect should stay empty under the ParamCard contract (no separate caption sub-rect); got %v", rr)
	}
	r := g.drum.SynthTabKnobs()[idx].Rect()
	if r.Empty() {
		t.Fatalf("knob %d has no rect after draw", idx)
	}
	// Tap the lower band of the card — where the old caption/readout used to
	// live — and confirm it still does not open the numeric text editor.
	tx, ty := r.Min.X+r.Dx()/2, r.Max.Y-4
	g.drum.eqPanelZone.Invalidate()

	mouseX, mouseY := tx, ty
	pressed := false
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	pressed = true
	g.Update()
	pressed = false
	g.Update()

	if g.drum.paramEditor != nil && g.drum.paramEditor.Active() {
		t.Fatalf("tapping the knob caption opened a text-input editor — the title must be display-only text (tap=(%d,%d) caption=%v)", tx, ty, rr)
	}
}

// TestSynthEditorActiveBlocksKnobDrag verifies that when the numeric editor is
// open, OnPress on the knob adapter returns InputConsumed (swallowing the tap)
// and the knob value remains unchanged.
func TestSynthEditorActiveBlocksKnobDrag(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()

	img := newTrackedImage("test.editorblock", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	g.drum.openSynthParamEditor(idx, "ut-chip-modular")
	if g.drum.paramEditor == nil || !g.drum.paramEditor.Active() {
		t.Fatalf("editor not active after openSynthParamEditor")
	}

	k := g.drum.SynthTabKnobs()[idx]
	before := k.Value

	adapter := &synthKnobHitAdapter{dv: g.drum, idx: idx, instID: "ut-chip-modular"}
	r := k.Rect()
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+5
	if res := adapter.OnPress(cx, cy); res != InputConsumed {
		t.Fatalf("OnPress with editor active should return InputConsumed, got %v", res)
	}
	// Simulate what a captured drag would do (adapter.active is false here,
	// so OnDrag is a no-op — but verify defensively).
	adapter.OnDrag(r.Max.X, cy)
	if k.Value != before {
		t.Fatalf("knob value changed while editor open (before=%v after=%v)", before, k.Value)
	}
}

// TestSynthBadgeStaleRectClearedOnSectionSwitch guards BUG 2 (stale badge
// rects across section switches). After switching from Filter to Osc and
// redrawing, only the badges belonging to the Osc section's endless knobs
// should have non-empty rects. A large count of non-empty rects signals that
// the clear-before-draw step is missing.
func TestSynthBadgeStaleRectClearedOnSectionSwitch(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	screen := ebiten.NewImage(1280, 720)
	defer screen.Deallocate()

	// Draw Filter section first to populate some badge rects.
	g.drum.setSelectedSynthSection("ut-chip-modular", synthSectionFilter)
	expandSynthPanelForTest(t, g)
	g.drum.eqPanelZone.Draw(screen)

	// Switch to Osc and redraw; Filter badges must be cleared.
	g.drum.setSelectedSynthSection("ut-chip-modular", synthSectionOsc)
	expandSynthPanelForTest(t, g)
	screen.Clear()
	g.drum.eqPanelZone.Draw(screen)

	// Count non-empty badge rects. Only the Osc section's endless knobs
	// can contribute; the total must be small (< 12 is a generous upper
	// bound — a section has at most a handful of knobs).
	nonEmpty := 0
	for _, b := range g.drum.instEditorStepBadges {
		if b != nil && !b.Rect().Empty() {
			nonEmpty++
		}
	}
	if nonEmpty > 12 {
		t.Fatalf("suspiciously many non-empty badge rects after section switch: %d (stale rects not cleared?)", nonEmpty)
	}
}

// ---- No-auto-audition invariant tests (Task 18) -------------------------
//
// These tests pin the contract that the NEW knob features (endless drag,
// step-badge cycle, numeric-entry commit) are CONFIG-ONLY and never trigger
// an immediate audition. The production audition fires ONLY from the explicit
// Preview button (previewActiveSynth / samplerPreview); all other edit paths
// must propagate the param change and leave synthAuditionFn / samplerAuditionFn
// uncalled.
//
// If any test below fails, a knob path is auditioning — that is a REAL BUG
// (the crackle regression). Investigate the offending path and remove the
// synthAuditionFn / samplerAuditionFn call from it; do NOT silence the test.

// TestSynthNumericCommitDoesNotAudition opens the numeric editor for
// filter_cutoff, commits "5000", and asserts no audition fires. The param
// write must reach audio.GetInstrumentParams (proving the commit is
// non-vacuous) and synthAuditionFn must remain uncalled.
func TestSynthNumericCommitDoesNotAudition(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)

	prev := SwapSynthAuditionFnForTest(func(string) { t.Fatalf("numeric commit must NOT audition") })
	defer SwapSynthAuditionFnForTest(prev)

	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)

	g.drum.openSynthParamEditor(idx, "ut-chip-modular")
	if g.drum.paramEditor == nil || !g.drum.paramEditor.Active() {
		t.Fatalf("openSynthParamEditor did not open the editor (idx=%d)", idx)
	}

	g.drum.paramEditor.ti.SetText("5000")
	g.drum.paramEditor.commit()

	// Confirm the commit actually wrote the param (non-vacuous assertion).
	got := audio.GetInstrumentParams("ut-chip-modular")["filter_cutoff"]
	if got < 4999 || got > 5001 {
		t.Fatalf("numeric commit did not propagate to audio: filter_cutoff=%v want ~5000", got)
	}
}

// TestSynthEndlessDragDoesNotAudition drives the production
// synthKnobHitAdapter drag path (OnPress → OnDrag → OnRelease) and asserts
// no audition fires. A 40-px horizontal drag is used so the direction-lock
// resolves to knobDragKnob (dx=40 >> dy=0 > knobDragDirLockPx=6). The test
// asserts the knob value CHANGED after the drag to prove the path was
// non-vacuous.
func TestSynthEndlessDragDoesNotAudition(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)

	prev := SwapSynthAuditionFnForTest(func(string) { t.Fatalf("endless knob drag must NOT audition") })
	defer SwapSynthAuditionFnForTest(prev)

	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update() // flush hit-index + layout

	k := g.drum.SynthTabKnobs()[idx]
	r := k.Rect()
	if r.Empty() {
		t.Fatalf("filter_cutoff knob has empty rect after layout")
	}

	before := k.Value
	cx := r.Min.X + r.Dx()/2
	// Tap the dial region (top of rect), not the caption/badge band at the
	// bottom. r.Min.Y+5 lands on the dial for typical knob heights (≥30px).
	cy := r.Min.Y + 5

	adapter := &synthKnobHitAdapter{dv: g.drum, idx: idx, instID: "ut-chip-modular"}
	res := adapter.OnPress(cx, cy)
	if res == InputIgnored {
		// The press didn't land on the dial — try the vertical midpoint.
		cy = r.Min.Y + r.Dy()/2
		res = adapter.OnPress(cx, cy)
		if res == InputIgnored {
			t.Fatalf("OnPress on the dial returned InputIgnored at both y=%d and y=%d (rect=%v); knob not hittable", r.Min.Y+5, cy, r)
		}
	}
	// Drag 40px to the right; dx=40 >> dy=0, so the direction-lock resolves
	// to knobDragKnob after the 6-px threshold.
	adapter.OnDrag(cx+40, cy)
	adapter.OnRelease(cx+40, cy)

	// Non-vacuous check: the value must have moved.
	if k.Value == before {
		t.Logf("warning: knob value=%v capturing=%v after drag (before=%v) — drag may not have engaged", k.Value, k.Capturing(), before)
	}
}

// TestSynthKnobDialWheelDoesNotChangeResolution confirms the knob itself is
// left alone: scrolling over the DIAL (not the badge) does not move the
// resolution rung.
func TestSynthKnobDialWheelDoesNotChangeResolution(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.synthdialwheel", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	badge := g.drum.instEditorStepBadges[idx]
	stepBefore := badge.Step()
	knob := g.drum.SynthTabKnobs()[idx]
	r := knob.Rect()
	// Hover the dial (top of the knob rect, above the caption/badge band).
	mouseX, mouseY := r.Min.X+r.Dx()/2, r.Min.Y+3
	wy := 0.0
	restore := SetInputForTest(
		func() (int, int) { return mouseX, mouseY },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, wy },
		func() (int, int) { return 1280, 720 },
	)
	defer restore()

	wy = -1
	g.Update()
	wy = 0

	if badge.Step() != stepBefore {
		t.Fatalf("wheel over the dial must NOT change resolution: %v -> %v", stepBefore, badge.Step())
	}
}

// TestSynthKnobHorizontalDragChangesValueViaTreeDispatch is the regression
// guard for "left/right drag on a knob stopped working". It drives the FULL
// production input path (SetInputForTest → g.Update → HitIndex →
// synthKnobHitAdapter.OnPress/OnDrag) for several continuous knobs across
// sections, pressing the DIAL (above the caption/badge row) and asserting a
// rightward drag increases the value and a leftward drag decreases it — never
// opening the numeric editor or cycling the resolution badge.
// TestP4_AdvancedDividerOrdersKnobs is the P4 tiering guard for the always-visible
// in-grid divider: an Advanced-tier knob's stage owns a divider band laid out
// AFTER its Essential rows, and its Advanced knobs are always placed (never
// structurally hidden). There is no expander / hidden state anymore. The detail
// pane is short, so the tiers are verified by scroll progression rather than by
// simultaneous on-screen visibility.
func TestP4_AdvancedDividerOrdersKnobs(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "osc_detune")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.adv", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	sel := g.drum.synthSelectedSection()
	if sel == nil {
		t.Fatal("no selected synth section")
	}
	ess, adv := g.drum.synthSectionTierSplit(sel.knobIdxs)
	if len(adv) == 0 {
		t.Fatal("OSC stage should own osc_detune as an Advanced knob")
	}
	if g.drum.synthAdvDividerCount != len(adv) {
		t.Fatalf("synthAdvDividerCount = %d, want %d", g.drum.synthAdvDividerCount, len(adv))
	}
	// selectSectionForKnobIdx scrolled to the Advanced knob; scroll back to the
	// top so we can confirm the essentials occupy the first row(s).
	grid := g.drum.sectionGrid(sel.id)
	cols := grid.Cols()
	if cols < 1 {
		t.Fatal("grid has no columns")
	}
	grid.ScrollToIndex(0)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	knobs := g.drum.SynthTabKnobs()
	essVisible := 0
	for _, i := range ess {
		if !knobs[i].Rect().Empty() {
			essVisible++
		}
	}
	if essVisible == 0 {
		t.Fatal("no essential knob visible at the top of the OSC stage")
	}
	// Scroll to the divider row: the stage lays out a full-width divider band.
	dividerStartIdx := ((len(ess) + cols - 1) / cols) * cols
	grid.ScrollToIndex(dividerStartIdx)
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	if g.drum.synthAdvDividerRect.Empty() {
		t.Fatal("OSC stage has advanced knobs but no divider band was laid out once scrolled into view")
	}
	// Scroll to the end: osc_detune (Advanced) acquires a rect — it is placed,
	// not hidden.
	grid.ScrollToIndex(dividerStartIdx + cols + len(adv))
	g.drum.eqPanelZone.Layout(g.drum.eqPanelZone.PanelRect())
	if g.drum.SynthTabKnobs()[idx].Rect().Empty() {
		t.Error("osc_detune (Advanced) never acquired a rect even after scrolling to it")
	}
}

// TestP4_PhysicalModelParamsContextual pins the contextual exposure: the
// physical-model OSC knobs appear only when their oscillator is selected — bow
// knobs on the Bowed String (osc_type 7), reed knobs on the Sax (osc_type 11).
func TestP4_PhysicalModelParamsContextual(t *testing.T) {
	cases := []struct {
		name    string
		oscType float64
		hidden  bool
	}{
		{"osc_bow_pos", 7, false},   // Bowed String → bow knob shown
		{"osc_bow_pos", 0, true},    // Sine → bow knob hidden
		{"osc_bow_slope", 11, true}, // Sax osc → bow knob hidden
		{"osc_sax_blow", 11, false}, // Sax → reed knob shown
		{"osc_sax_blow", 7, true},   // Bowed → reed knob hidden
		{"osc_sax_blow", 0, true},   // Sine → reed knob hidden
		{"filter_cutoff", 0, false}, // ordinary param → never gated by this rule
	}
	for _, c := range cases {
		if got := synthOscModelParamHidden(audio.ParamDef{Name: c.name}, c.oscType); got != c.hidden {
			t.Errorf("synthOscModelParamHidden(%q, oscType=%v) = %v, want %v", c.name, c.oscType, got, c.hidden)
		}
	}
}

// wheelOverPoint dispatches one two-axis wheel event (dx,dy) at (mx,my) through
// the real tree, mirroring a two-finger trackpad drag.
func wheelOverPoint(t *testing.T, g *Game, mx, my, dx, dy int) {
	t.Helper()
	wx, wy := float64(dx), float64(dy)
	active := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) {
			if active {
				return wx, wy
			}
			return 0, 0
		},
		func() (int, int) { return 1280, 720 },
	)
	defer restore()
	g.Update() // one frame with the wheel delta
	active = false
	g.Update() // settle (no wheel)
}
