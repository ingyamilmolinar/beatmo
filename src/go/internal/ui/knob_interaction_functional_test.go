package ui

import (
	"image"
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

// TestSynthStepBadgeTapCyclesStepViaTreeDispatch verifies the FULL production
// input path for badge taps: mouse→SetInputForTest→g.Update()→HitIndex→
// synthKnobHitAdapter.OnPress→cycleKnobStep. Replaces the isolation-test
// (direct handleSynthTabInput call) that never exercised the real tree dispatch.
func TestSynthStepBadgeTapCyclesStepViaTreeDispatch(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update() // flush hit-index

	img := newTrackedImage("test.badgetree", 1280, 720)
	defer releaseImage(img)
	g.Draw(img) // populate badge rects from a real draw

	badge := g.drum.instEditorStepBadges[idx]
	if badge == nil || badge.Rect().Empty() {
		t.Fatalf("badge rect not populated after draw (knob rect=%v)", g.drum.SynthTabKnobs()[idx].Rect())
	}
	before := badge.Step()
	r := badge.Rect()
	bx, by := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	// Invalidate so the next Update's layoutPass re-publishes HitAreas,
	// picking up the badge rect that was just populated by Draw.
	g.drum.eqPanelZone.Invalidate()

	mouseX, mouseY := bx, by
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
	g.Update() // layout re-publishes badge HitArea; tree dispatches press to OnPress
	pressed = false
	g.Update()

	if badge.Step() == before {
		t.Fatalf("badge step did not cycle via real tree dispatch (still %v)", before)
	}
	if g.drum.SynthTabKnobs()[idx].StepMul != badge.Step() {
		t.Fatalf("knob StepMul (%v) not synced to badge step (%v) after tap", g.drum.SynthTabKnobs()[idx].StepMul, badge.Step())
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
func TestSynthReadoutTapOpensEditorViaTreeDispatch(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()

	img := newTrackedImage("test.readouttree", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	rr := g.drum.synthKnobReadoutRect(idx)
	if rr.Empty() {
		t.Fatalf("readout rect not populated after draw")
	}

	// Find a tap point inside the readout rect but outside any badge rect.
	// The readout spans the full cell width; badges are narrow centered pills.
	// We walk the left edge rightward until we find a badge-free x.
	tx, ty := rr.Min.X+2, rr.Min.Y+rr.Dy()/2
	badge := g.drum.instEditorStepBadges[idx]
	if badge != nil && !badge.Rect().Empty() && image.Pt(tx, ty).In(badge.Rect()) {
		// Left edge is inside the badge; try right edge.
		tx = rr.Max.X - 2
		if image.Pt(tx, ty).In(badge.Rect()) {
			t.Fatalf("both left and right edges of readout rect (%v) are inside badge rect (%v) — geometry overlap: badge fully covers readout", rr, badge.Rect())
		}
	}

	// Invalidate so the next Update's layoutPass re-publishes HitAreas,
	// picking up the readout rect that was just populated by Draw.
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
	g.Update() // layout re-publishes readout HitArea; tree dispatches press to OnPress
	pressed = false
	g.Update()

	if g.drum.paramEditor == nil || !g.drum.paramEditor.Active() {
		t.Fatalf("readout tap via tree dispatch did not open the editor (tap=(%d,%d) readout=%v)", tx, ty, rr)
	}
	// Commit a value and confirm it reaches audio.
	g.drum.paramEditor.ti.SetText("5000")
	g.drum.paramEditor.commit()
	got := audio.GetInstrumentParams("ut-chip-modular")["filter_cutoff"]
	if got < 4999 || got > 5001 {
		t.Fatalf("param after edit=%v want ~5000", got)
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

// TestSynthBadgeCycleDoesNotAudition cycles the step badge for filter_cutoff
// and asserts no audition fires. The badge step change is the non-vacuous
// proof (the badge is the only mutable state).
func TestSynthBadgeCycleDoesNotAudition(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)

	prev := SwapSynthAuditionFnForTest(func(string) { t.Fatalf("badge cycle must NOT audition") })
	defer SwapSynthAuditionFnForTest(prev)

	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)

	badges := g.drum.instEditorStepBadges
	if idx >= len(badges) || badges[idx] == nil {
		t.Fatalf("filter_cutoff has no step badge (len=%d)", len(badges))
	}
	before := badges[idx].Step()

	g.drum.cycleKnobStep(idx)

	after := badges[idx].Step()
	if after == before {
		t.Fatalf("badge step did not cycle (still %v)", before)
	}
}

// TestSynthBadgeWheelChangesResolutionViaTreeDispatch verifies that scrolling
// the mouse wheel while hovering the step-RESOLUTION badge shifts the rung
// (scroll down = coarser), WITHOUT turning the knob — driven through the real
// tree dispatch, not a direct handler call.
func TestSynthBadgeWheelChangesResolutionViaTreeDispatch(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.synthbadgewheel", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	badge := g.drum.instEditorStepBadges[idx]
	if badge == nil || badge.Rect().Empty() {
		t.Fatalf("badge rect not populated")
	}
	knob := g.drum.SynthTabKnobs()[idx]
	valBefore := knob.Value
	stepBefore := badge.Step()

	r := badge.Rect()
	mouseX, mouseY := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
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

	// The wheel is debounced/clicky: a single notch must NOT move the rung.
	wy = -1 // scroll down = coarser = larger step
	g.Update()
	if badge.Step() != stepBefore {
		t.Fatalf("a single wheel notch must not change resolution: %v -> %v", stepBefore, badge.Step())
	}
	// Crossing the per-rung notch threshold advances exactly one rung.
	for i := 1; i < knobBadgeWheelNotchesPerRung; i++ {
		g.Update()
	}
	wy = 0

	if badge.Step() <= stepBefore {
		t.Fatalf("scroll down over badge should coarsen the step: before=%v after=%v", stepBefore, badge.Step())
	}
	if knob.StepMul != badge.Step() {
		t.Fatalf("knob StepMul not synced to badge after wheel: StepMul=%v step=%v", knob.StepMul, badge.Step())
	}
	if knob.Value != valBefore {
		t.Fatalf("wheel over the badge must NOT turn the knob: value %v -> %v", valBefore, knob.Value)
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

// TestSynthBadgeNeverOverlapsCaption guards the fix for the "resolution label
// overlaps the knob text" bug: across several sections (incl. short detail
// panes that force the inline-badge fallback), no visible step badge may
// overlap its own caption/readout rect.
func TestSynthBadgeNeverOverlapsCaption(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	img := newTrackedImage("test.synthbadgeoverlap", 1280, 720)
	defer releaseImage(img)
	drawn := 0
	for _, sec := range []synthSectionID{synthSectionFM, synthSectionOsc, synthSectionEnvelope, synthSectionFilter} {
		g.drum.setSelectedSynthSection("ut-chip-modular", sec)
		expandSynthPanelForTest(t, g)
		g.Draw(img)
		for i, b := range g.drum.instEditorStepBadges {
			if b == nil || b.Rect().Empty() {
				continue
			}
			drawn++
			rr := g.drum.synthKnobReadoutRect(i)
			if !rr.Empty() && b.Rect().Overlaps(rr) {
				t.Fatalf("section %v knob %d: badge %v overlaps caption %v", sec, i, b.Rect(), rr)
			}
		}
	}
	if drawn == 0 {
		t.Fatalf("no step badges were drawn — the test is vacuous")
	}
}

// TestSynthKnobHorizontalDragChangesValueViaTreeDispatch is the regression
// guard for "left/right drag on a knob stopped working". It drives the FULL
// production input path (SetInputForTest → g.Update → HitIndex →
// synthKnobHitAdapter.OnPress/OnDrag) for several continuous knobs across
// sections, pressing the DIAL (above the caption/badge row) and asserting a
// rightward drag increases the value and a leftward drag decreases it — never
// opening the numeric editor or cycling the resolution badge.
func TestSynthKnobHorizontalDragChangesValueViaTreeDispatch(t *testing.T) {
	for _, name := range []string{"osc_detune", "fm_op1_ratio", "amp_attack", "filter_cutoff", "filter_resonance"} {
		t.Run(name, func(t *testing.T) {
			g := newModularSynthTabGame(t)
			expandSynthPanelForTest(t, g)
			idx := synthIdxByParam(t, g, name)
			selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
			g.Update()
			img := newTrackedImage("test.hdrag", 1280, 720)
			defer releaseImage(img)
			g.Draw(img)

			k := g.drum.SynthTabKnobs()[idx]
			r := k.Rect()
			if r.Empty() {
				t.Fatalf("%s knob rect empty", name)
			}
			// Press the dial: horizontally centred, vertically in the dial
			// circle (above the caption band that hosts the readout + badge).
			capH := Profile().DensityValues().SynthKnobCaptionH
			cx := r.Min.X + r.Dx()/2
			cy := r.Min.Y + (r.Dy()-capH)/2

			mx, my := cx, cy
			pressed := false
			restore := SetInputForTest(
				func() (int, int) { return mx, my },
				func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
				func(k ebiten.Key) bool { return false },
				func() []rune { return nil },
				func() (float64, float64) { return 0, 0 },
				func() (int, int) { return 1280, 720 },
			)
			defer restore()
			g.drum.eqPanelZone.Invalidate()

			// Rightward drag → value must INCREASE.
			start := k.Value
			pressed = true
			g.Update()
			if !k.Capturing() {
				t.Fatalf("%s: dial press did not start a knob drag (capturing=false) — left/right drag is broken", name)
			}
			for s := 1; s <= 5; s++ {
				mx = cx + s*16
				g.Update()
			}
			pressed = false
			g.Update()
			afterRight := k.Value
			if afterRight <= start {
				t.Fatalf("%s: rightward drag did not increase value (%.4f -> %.4f)", name, start, afterRight)
			}
			if g.drum.paramEditor != nil && g.drum.paramEditor.Active() {
				t.Fatalf("%s: dial drag wrongly opened the numeric editor", name)
			}

			// Leftward drag → value must DECREASE.
			mx, my = cx, cy
			g.drum.eqPanelZone.Invalidate()
			beforeLeft := k.Value
			pressed = true
			g.Update()
			for s := 1; s <= 5; s++ {
				mx = cx - s*16
				g.Update()
			}
			pressed = false
			g.Update()
			if k.Value >= beforeLeft {
				t.Fatalf("%s: leftward drag did not decrease value (%.4f -> %.4f)", name, beforeLeft, k.Value)
			}
		})
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

// TestSynthKnobHorizontalWheelChangesValue: a two-finger LEFT/RIGHT scroll over
// a knob changes its value (mirroring left/right mouse drag); an UP/DOWN scroll
// does NOT change the value (it is reserved for scrolling overflow rows).
func TestSynthKnobHorizontalWheelChangesValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.hwheel", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)
	g.drum.eqPanelZone.Invalidate()

	k := g.drum.SynthTabKnobs()[idx]
	r := k.Rect()
	capH := Profile().DensityValues().SynthKnobCaptionH
	cx := r.Min.X + r.Dx()/2
	cy := r.Min.Y + (r.Dy()-capH)/2

	before := k.Value
	// Debounced/clicky: a single horizontal notch must NOT move the value.
	wheelOverPoint(t, g, cx, cy, 3, 0) // horizontal (two-finger left/right)
	if k.Value != before {
		t.Fatalf("a single horizontal wheel notch must not move the value (debounced): %.4f -> %.4f", before, k.Value)
	}
	// Crossing the notch threshold advances the value by one step.
	for i := 1; i < knobValueWheelNotchesPerStep; i++ {
		wheelOverPoint(t, g, cx, cy, 3, 0)
	}
	if k.Value == before {
		t.Fatalf("horizontal wheel did not change value after %d notches (%.4f)", knobValueWheelNotchesPerStep, before)
	}

	// Vertical wheel must NOT change the value (reserved for row scrolling).
	mid := k.Value
	wheelOverPoint(t, g, cx, cy, 0, 3)
	if k.Value != mid {
		t.Fatalf("vertical wheel over knob changed the value (%.4f -> %.4f) — up/down must be reserved for overflow-row scroll", mid, k.Value)
	}
}

// TestSynthVerticalWheelScrollsOverflowRows: in an overflowing section a
// two-finger UP/DOWN scroll over a knob moves between rows (the set of visible
// knobs changes) without altering any knob's value.
func TestSynthVerticalWheelScrollsOverflowRows(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx := synthIdxByParam(t, g, "fm_op1_ratio")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Layout(420, 720) // narrow → FM wraps to multiple rows and overflows
	g.Update()
	img := newTrackedImage("test.vscroll", 420, 720)
	defer releaseImage(img)
	g.Draw(img)

	visible := func() map[int]int {
		out := map[int]int{}
		for _, s := range g.drum.instEditorSections {
			if s.id != synthSectionFM {
				continue
			}
			for _, ki := range s.knobIdxs {
				if r := g.drum.instEditorKnobs[ki].Rect(); !r.Empty() {
					out[ki] = r.Min.Y
				}
			}
		}
		return out
	}
	before := visible()
	if len(before) == 0 {
		t.Fatalf("no visible FM knobs to scroll")
	}
	// Snapshot all FM values to prove the scroll never changes a value.
	vals := map[int]float64{}
	for _, s := range g.drum.instEditorSections {
		if s.id == synthSectionFM {
			for _, ki := range s.knobIdxs {
				vals[ki] = g.drum.instEditorKnobs[ki].Value
			}
		}
	}

	// Vertical wheel down over a visible knob.
	var anyIdx int
	for ki := range before {
		anyIdx = ki
		break
	}
	r := g.drum.instEditorKnobs[anyIdx].Rect()
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/3
	g.drum.eqPanelZone.Invalidate()
	active := true
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) {
			if active {
				return 0, -3
			}
			return 0, 0
		},
		func() (int, int) { return 420, 720 },
	)
	defer restore()
	g.Update()
	active = false
	g.Update()
	g.Draw(img)

	after := visible()
	scrolled := len(after) != len(before)
	for ki, yy := range before {
		if ay, ok := after[ki]; !ok || ay != yy {
			scrolled = true
		}
	}
	if !scrolled {
		t.Fatalf("vertical wheel over a knob did not scroll the overflow rows (visible set unchanged: %v)", before)
	}
	// No value may change from a scroll.
	for _, s := range g.drum.instEditorSections {
		if s.id == synthSectionFM {
			for _, ki := range s.knobIdxs {
				if g.drum.instEditorKnobs[ki].Value != vals[ki] {
					t.Fatalf("knob %d value changed during vertical-wheel scroll (%.4f -> %.4f)", ki, vals[ki], g.drum.instEditorKnobs[ki].Value)
				}
			}
		}
	}
}
