package ui

import (
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The synth card on every platform: a tap arms it, a desktop wheel over the armed value lane nudges by one step
// and collapses the spin into ONE debounced undo step, and a wheel over whitespace scrolls. Real tree dispatch.

// One entry per committed gesture.
func undoDepth(g *Game) int { return len(g.undoManager.undo) }

// filter_cutoff, the canonical endless badge-backed knob; fails if it has no rect.
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

// One frame pressed, one released.
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

// Then one settle frame with no wheel.
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

// The card-level dispatch reaches the armed path on desktop.
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

// Over the armed control's value lane the wheel nudges and commits ONE undo step after the debounce; a wheel over
// an unarmed card scrolls the row instead.
func TestParamCardWheelNudgesValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	inst := dv.synthTabActiveInstrument()
	idx, _ := firstContinuousSynthControl(t, g)

	// The value lane's rect exists only after the grown card is laid out and drawn once.
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
	// One impulse may not cross a detent at every SynthWheelRowH, so two.
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

	// The section's scroll-body catch-all, at a point over no knob.
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

// Arms the first continuous control and spins its value lane once, leaving the debounced commit PENDING; returns
// the undo depth from before, so the caller can assert one step lands.
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

	// Leaving Synth before the debounce elapses: the onLeave hook must flush the pending commit.
	dv.eqPanelZone.SetActiveTab(TabWave)
	for i := 0; i < 3; i++ {
		g.Update()
	}

	if got := undoDepth(g); got != undoBefore+1 {
		t.Fatalf("tab switch away from Synth must flush the pending wheel-nudge commit exactly once, got undo depth %d->%d", undoBefore, got)
	}
}

// setViewMode sets the tab state directly, bypassing SetActiveTab; PanelTabState's one onLeave hook still flushes
// the pending commit, as on every tab transition.
func TestWheelNudgeFlushesOnViewModeSwitchAwayFromSynth(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	undoBefore := armWheelPendingCommit(t, g)

	// Bypasses EQPanelZone.SetActiveTab.
	dv.setViewMode(viewModeEQ)
	for i := 0; i < 3; i++ {
		g.Update()
	}

	if got := undoDepth(g); got != undoBefore+1 {
		t.Fatalf("setViewMode away from Synth must flush the pending wheel-nudge commit exactly once, got undo depth %d->%d", undoBefore, got)
	}
}

// The armed editor's resolution lane moves the badge rung finer or coarser, and the choice sticks to the knob's
// StepMul.
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
