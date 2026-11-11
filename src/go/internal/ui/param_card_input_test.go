package ui

import (
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// param_card_input_test.go — Task 5. The synth knob card is now a UNIVERSAL
// tap-to-popup + desktop-wheel-nudge control on every platform:
//   - a click/tap anywhere on the card opens the value-wheel popup (was
//     mobile-only; desktop used to start an in-place rotary drag);
//   - a desktop scroll-wheel notch over the card nudges the value by one
//     StepMul (endless) / one detent (discrete) and collapses the whole spin
//     into ONE debounced undo step;
//   - a wheel over grid whitespace still scrolls the section (never nudges).
//
// These drive the REAL production tree dispatch (SetInputForTest → g.Update →
// HitIndex → synthControlHitAdapter), mirroring knob_interaction_functional_test.go.

// undoDepth returns the current undo-stack depth (one entry per committed
// gesture). Mirrors the accessor used by game_marquee_test.go and friends.
func undoDepth(g *Game) int { return len(g.undoManager.undo) }

// firstContinuousSynthControl selects the section that owns the canonical
// continuous (endless, badge-backed) synth knob — filter_cutoff — lays it out,
// and returns its binding index + on-screen rect. Fails if the knob has no rect.
func firstContinuousSynthControl(t *testing.T, g *Game) (int, image.Rectangle) {
	t.Helper()
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.paramcardinput", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)
	r := g.drum.SynthTabControls()[idx].Rect()
	if r.Empty() {
		t.Fatalf("filter_cutoff knob has empty rect after layout")
	}
	return idx, r
}

// pressAndReleaseAt drives a press+release at (x,y) through the real tree
// dispatcher (one frame pressed, one frame released).
func pressAndReleaseAt(t *testing.T, g *Game, x, y int) {
	t.Helper()
	g.drum.eqPanelZone.Invalidate()
	mx, my := x, y
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
	pressed = true
	g.Update()
	pressed = false
	g.Update()
}

// dispatchWheelAt delivers one vertical wheel notch (steps) at (x,y) through the
// real tree dispatcher, then settles one frame with no wheel.
func dispatchWheelAt(t *testing.T, g *Game, x, y, steps int) {
	t.Helper()
	g.drum.eqPanelZone.Invalidate()
	active := true
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) {
			if active {
				return 0, float64(steps)
			}
			return 0, 0
		},
		func() (int, int) { return 1280, 720 },
	)
	defer restore()
	g.Update()
	active = false
	g.Update()
}

// Desktop tap on a card arms it inline (Task 13: the barrel/wheel popup that
// used to open on tap was retired — Task 9 replaced tap→popup with tap→arm).
// See synth_armed_input_test.go's TestTapArmsThenWheelEditsValue for the full
// armed-model tap contract; this only pins that the card-level dispatch still
// reaches the armed path on desktop.
func TestParamCardClickArmsOnDesktop(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	_, r := firstContinuousSynthControl(t, g)
	pressAndReleaseAt(t, g, r.Min.X+4, r.Min.Y+4)
	if _, armed := dv.synthControlArmed(inst); !armed {
		t.Fatal("desktop card tap must arm the control")
	}
}

// Desktop wheel over the ARMED control's value lane nudges the value and
// commits ONE undo step after the debounce elapses (Task 13: migrated off the
// retired not-armed wheel-nudge — Task 9 made a not-armed wheel over a card
// SCROLL the row instead, see TestNotArmedWheelScrollsRow in
// synth_armed_input_test.go). The gesture is now: arm the control (grows the
// card + builds the ArmedControlEditor), let the layout populate the value
// lane's rect, then wheel over that lane through the same real-tree
// dispatchWheelAt path used everywhere else in this file.
func TestParamCardWheelNudgesValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	idx, _ := firstContinuousSynthControl(t, g)

	// Arm the control, then re-layout/re-draw once so growArmedCard grows the
	// card and drawArmedControlCard calls ArmedControlEditor.SetCardRect with
	// the real grown rect — only then is ValueLaneRect() populated.
	dv.armSynthControl(inst, idx)
	g.Update()
	img := newTrackedImage("test.paramcardinput.armed", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	vr := e.ValueLaneRect()
	if vr.Empty() {
		t.Fatal("value lane rect must be populated after arming + a layout/draw pass")
	}

	k := dv.instEditorControls[idx]
	before := k.Value
	undoBefore := undoDepth(g)

	vx, vy := vr.Min.X+vr.Dx()/2, vr.Min.Y+vr.Dy()/2
	// One notch (ImpulseNotch) may not accumulate enough kinetic offset to
	// cross a detent depending on density's SynthWheelRowH, so drive two —
	// same margin TestTapArmsThenWheelEditsValue uses in
	// synth_armed_input_test.go.
	dispatchWheelAt(t, g, vx, vy, +2)
	for i := 0; i < 240 && e.Tick(); i++ { // resolve the kinetic wheel animation
	}
	if k.Value == before {
		t.Fatal("armed wheel notch over the value lane did not change value")
	}
	for i := 0; i < 60; i++ { // run the debounce down
		g.Update()
	}
	if got := undoDepth(g); got != undoBefore+1 {
		t.Fatalf("wheel gesture must commit exactly one undo step, got %d→%d", undoBefore, got)
	}
}

// Wheel over grid whitespace still scrolls the section (no value change).
func TestGridWhitespaceWheelScrollsNotNudges(t *testing.T) {
	g := newModularSynthTabGame(t)
	audio.SetInstrumentParam("ut-chip-modular", "osc_type", 4) // FM operators exist only on the FM oscillator
	expandSynthPanelForTest(t, g)
	dv := g.drum
	idx := synthIdxByParam(t, g, "fm_op1_ratio")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Layout(420, 720) // narrow → FM wraps to multiple rows and overflows
	g.Update()
	img := newTrackedImage("test.gridwhitespace", 420, 720)
	defer releaseImage(img)
	g.Draw(img)

	grid := dv.sectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Skip("FM section not scrollable at this layout — whitespace-scroll is vacuous")
	}

	// Locate the section's scroll-body catch-all rect (covers the whole body),
	// then find a point inside it that is NOT over any knob's base rect.
	bodyTag := "synth-scrollbody-" + fmt.Sprint(int(synthSectionFM))
	var body image.Rectangle
	for _, h := range dv.synthTabHitAreas() {
		if h.Tag == bodyTag {
			body = h.Rect
			break
		}
	}
	if body.Empty() {
		t.Skip("no FM scroll-body hit area — whitespace-scroll is vacuous")
	}
	var knobRects []image.Rectangle
	fmVals := map[int]float64{}
	for _, s := range dv.instEditorSections {
		if s.id != synthSectionFM {
			continue
		}
		for _, ki := range s.knobIdxs {
			if ki >= 0 && ki < len(dv.instEditorControls) {
				fmVals[ki] = dv.instEditorControls[ki].Value
				if kr := dv.instEditorControls[ki].Rect(); !kr.Empty() {
					knobRects = append(knobRects, kr)
				}
			}
		}
	}
	wx, wy, ok := -1, -1, false
	for y := body.Min.Y + 2; y < body.Max.Y-2 && !ok; y += 4 {
		for x := body.Min.X + 2; x < body.Max.X-2; x += 4 {
			p := image.Pt(x, y)
			hit := false
			for _, kr := range knobRects {
				if p.In(kr) {
					hit = true
					break
				}
			}
			if !hit {
				wx, wy, ok = x, y, true
				break
			}
		}
	}
	if !ok {
		t.Skip("no whitespace point found inside the FM body")
	}

	visible := func() map[int]int {
		out := map[int]int{}
		for _, s := range dv.instEditorSections {
			if s.id != synthSectionFM {
				continue
			}
			for _, ki := range s.knobIdxs {
				if r := dv.instEditorControls[ki].Rect(); !r.Empty() {
					out[ki] = r.Min.Y
				}
			}
		}
		return out
	}
	before := visible()

	// A few whitespace wheel notches (scroll down) — must scroll the section.
	for i := 0; i < 6; i++ {
		g.drum.eqPanelZone.Invalidate()
		active := true
		restore := SetInputForTest(
			func() (int, int) { return wx, wy },
			func(b ebiten.MouseButton) bool { return false },
			func(k ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) {
				if active {
					return 0, -1
				}
				return 0, 0
			},
			func() (int, int) { return 420, 720 },
		)
		g.Update()
		active = false
		g.Update()
		restore()
	}
	g.Draw(img)

	after := visible()
	scrolled := len(after) != len(before)
	for ki, yy := range before {
		if ay, okk := after[ki]; !okk || ay != yy {
			scrolled = true
		}
	}
	if !scrolled {
		t.Fatalf("whitespace wheel did not scroll the section (visible set unchanged: %v)", before)
	}
	// No knob value may change from a whitespace scroll.
	for ki, v := range fmVals {
		if dv.instEditorControls[ki].Value != v {
			t.Fatalf("FM knob %d value changed during whitespace scroll (%.4f -> %.4f)", ki, v, dv.instEditorControls[ki].Value)
		}
	}
}

// TestWheelNudgeFlushesOnTabSwitchAway is a Task-5 review-fix regression test:
// spinning the synth-card wheel and then switching the active audio-panel
// tab away from Synth — WITHOUT waiting out the wheelNudgeCommitFrames
// debounce — must still land exactly one undo step. Before the fix, the
// debounce only ticked while SynthTabUpdate ran (TabSynth active), so a tab
// switch mid-gesture silently dropped the latched commit.
// armWheelPendingCommit arms the first continuous synth control, spins its value
// lane by one gesture through the real dispatcher, resolves the kinetic
// animation, and asserts the debounced wheel-nudge commit is still PENDING (not
// yet an undo step). Armed-model migration of the retired not-armed wheel-nudge:
// a not-armed wheel now scrolls the row, so a pending value commit only exists
// once the control is armed and its value lane spun. Returns the undo depth
// captured before the gesture, so the caller can assert the flush lands exactly
// one step.
func armWheelPendingCommit(t *testing.T, g *Game) int {
	t.Helper()
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	idx, _ := firstContinuousSynthControl(t, g)
	dv.armSynthControl(inst, idx)
	g.Update()
	img := newTrackedImage("test.wheelflush.armed", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)

	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	vr := e.ValueLaneRect()
	if vr.Empty() {
		t.Fatal("value lane rect must be populated after arming + a layout/draw pass")
	}
	k := dv.instEditorControls[idx]
	before := k.Value
	undoBefore := undoDepth(g)

	vx, vy := vr.Min.X+vr.Dx()/2, vr.Min.Y+vr.Dy()/2
	dispatchWheelAt(t, g, vx, vy, +2)
	for i := 0; i < 240 && e.Tick(); i++ { // resolve the kinetic wheel
	}
	if k.Value == before {
		t.Fatal("armed wheel notch over the value lane did not change value")
	}
	if got := undoDepth(g); got != undoBefore {
		t.Fatalf("wheel-nudge commit fired before the debounce elapsed (got %d, want %d)", got, undoBefore)
	}
	return undoBefore
}

func TestWheelNudgeFlushesOnTabSwitchAway(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	undoBefore := armWheelPendingCommit(t, g)

	// Switch away from Synth through the real production path (mirrors a
	// user clicking a different sticky-bar tab pill) WITHOUT running the
	// wheelNudgeCommitFrames worth of frames the debounce needs to
	// self-commit — the onLeave hook must flush the pending commit.
	dv.eqPanelZone.SetActiveTab(TabWave)
	for i := 0; i < 3; i++ {
		g.Update()
	}

	if got := undoDepth(g); got != undoBefore+1 {
		t.Fatalf("tab switch away from Synth must flush the pending wheel-nudge commit exactly once, got undo depth %d->%d", undoBefore, got)
	}
}

// TestWheelNudgeFlushesOnViewModeSwitchAwayFromSynth is a Task-5 review-fix
// regression test for the OTHER tab-transition path: (*DrumView).setViewMode
// (driven by the mobile bottom-nav segmented control / scene catalog / JS
// setViewMode export) sets eqPanelZone.tabState directly rather than going
// through EQPanelZone.SetActiveTab or the sticky-bar onTab closure. It must
// still flush a pending wheel-nudge commit when leaving TabSynth — verified
// here via PanelTabState's single onLeave hook (wired once in
// NewEQPanelZone), which every tab-transition path — including this one —
// funnels through.
func TestWheelNudgeFlushesOnViewModeSwitchAwayFromSynth(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	undoBefore := armWheelPendingCommit(t, g)

	// dv.setViewMode(viewModeEQ) calls eqPanelZone.tabState.SetActiveTab(TabEQ)
	// directly (drumview_context_menu.go), bypassing EQPanelZone.SetActiveTab.
	dv.setViewMode(viewModeEQ)
	for i := 0; i < 3; i++ {
		g.Update()
	}

	if got := undoDepth(g); got != undoBefore+1 {
		t.Fatalf("setViewMode away from Synth must flush the pending wheel-nudge commit exactly once, got undo depth %d->%d", undoBefore, got)
	}
}

// TestSynthControlResolutionLaneIsFiner (Task 9 armed-model rewrite) proves the
// RESOLUTION lane of the armed editor moves the badge's active rung finer or
// coarser, and that the choice PERSISTS onto the knob's StepMul (unlike the
// old transient Shift-wheel nudge, resolution is now a deliberate, sticky
// choice made via the left lane of the armed control — see
// synth_barrel_bindings_test.go's badge-index pattern).
func TestSynthControlResolutionLaneIsFiner(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	idx, r := firstContinuousSynthControl(t, g)
	dv := g.drum
	instID := dv.synthTabActiveInstrument()

	if idx >= len(dv.instEditorStepBadges) || dv.instEditorStepBadges[idx] == nil {
		t.Fatal("filter_cutoff knob has no step badge")
	}
	badge := dv.instEditorStepBadges[idx]
	startIdx := badge.Index()
	if startIdx <= 0 {
		t.Skip("badge already at the finest rung — no finer rung available to test against")
	}
	stepMulBefore := dv.instEditorControls[idx].StepMul

	dv.armSynthControl(instID, idx)
	e := dv.instEditorArmedEditor
	if e == nil {
		t.Fatal("arming should create the ArmedControlEditor")
	}
	e.SetCardRect(r)
	if e.ResLaneRect().Empty() {
		t.Fatal("resolution lane should be available for a badged (continuous) control")
	}

	if !e.StepResolution(-1) {
		t.Fatal("StepResolution(-1) should move the rung one step finer")
	}

	if got := badge.Index(); got != startIdx-1 {
		t.Fatalf("resolution rung should move exactly one step finer: start=%d now=%d", startIdx, got)
	}
	if got := dv.instEditorControls[idx].StepMul; got == stepMulBefore {
		t.Fatalf("k.StepMul must reflect the newly chosen finer rung: before=%.6f after=%.6f", stepMulBefore, got)
	}
	if got := badge.Step(); got != badge.Rungs()[startIdx-1] {
		t.Fatalf("badge.Step() should read back the finer rung's step: got=%.6f want=%.6f", got, badge.Rungs()[startIdx-1])
	}
}
