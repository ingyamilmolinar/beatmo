package ui

import (
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
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
// HitIndex → synthKnobHitAdapter), mirroring knob_interaction_functional_test.go.

// undoDepth returns the current undo-stack depth (one entry per committed
// gesture). Mirrors the accessor used by game_marquee_test.go and friends.
func undoDepth(g *Game) int { return len(g.undoManager.undo) }

// firstContinuousSynthKnob selects the section that owns the canonical
// continuous (endless, badge-backed) synth knob — filter_cutoff — lays it out,
// and returns its binding index + on-screen rect. Fails if the knob has no rect.
func firstContinuousSynthKnob(t *testing.T, g *Game) (int, image.Rectangle) {
	t.Helper()
	idx := synthIdxByParam(t, g, "filter_cutoff")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Update()
	img := newTrackedImage("test.paramcardinput", 1280, 720)
	defer releaseImage(img)
	g.Draw(img)
	r := g.drum.SynthTabKnobs()[idx].Rect()
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

// Desktop click on a card opens the value-wheel popup (was mobile-only).
func TestParamCardClickOpensPopupOnDesktop(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	_, r := firstContinuousSynthKnob(t, g)
	pressAndReleaseAt(t, g, r.Min.X+4, r.Min.Y+4)
	if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
		t.Fatal("desktop card tap must open the value-wheel popup")
	}
}

// Desktop wheel over a card nudges the value and commits ONE undo step after the
// debounce elapses — without opening the popup.
func TestParamCardWheelNudgesValue(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	idx, r := firstContinuousSynthKnob(t, g)
	k := dv.instEditorKnobs[idx]
	before := k.Value
	undoBefore := undoDepth(g)

	dispatchWheelAt(t, g, r.Min.X+4, r.Min.Y+4, +1)
	if k.Value == before {
		t.Fatal("wheel notch over card did not change value")
	}
	if dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen() {
		t.Fatal("wheel nudge must not open the popup")
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
			if ki >= 0 && ki < len(dv.instEditorKnobs) {
				fmVals[ki] = dv.instEditorKnobs[ki].Value
				if kr := dv.instEditorKnobs[ki].Rect(); !kr.Empty() {
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
				if r := dv.instEditorKnobs[ki].Rect(); !r.Empty() {
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
		if dv.instEditorKnobs[ki].Value != v {
			t.Fatalf("FM knob %d value changed during whitespace scroll (%.4f -> %.4f)", ki, v, dv.instEditorKnobs[ki].Value)
		}
	}
}

// TestWheelNudgeFlushesOnTabSwitchAway is a Task-5 review-fix regression test:
// spinning the synth-card wheel and then switching the active audio-panel
// tab away from Synth — WITHOUT waiting out the wheelNudgeCommitFrames
// debounce — must still land exactly one undo step. Before the fix, the
// debounce only ticked while SynthTabUpdate ran (TabSynth active), so a tab
// switch mid-gesture silently dropped the latched commit.
func TestWheelNudgeFlushesOnTabSwitchAway(t *testing.T) {
	g := newModularSynthTabGame(t)
	expandSynthPanelForTest(t, g)
	dv := g.drum
	_, r := firstContinuousSynthKnob(t, g)
	undoBefore := undoDepth(g)

	dispatchWheelAt(t, g, r.Min.X+4, r.Min.Y+4, +1)
	if got := undoDepth(g); got != undoBefore {
		t.Fatalf("undo step recorded before the debounce elapsed or the tab switched (got %d, want %d)", got, undoBefore)
	}

	// Switch away from Synth through the real production path (mirrors a
	// user clicking a different sticky-bar tab pill) WITHOUT running the
	// wheelNudgeCommitFrames worth of frames the debounce needs to
	// self-commit.
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
	_, r := firstContinuousSynthKnob(t, g)
	undoBefore := undoDepth(g)

	dispatchWheelAt(t, g, r.Min.X+4, r.Min.Y+4, +1)
	if got := undoDepth(g); got != undoBefore {
		t.Fatalf("undo step recorded before the debounce elapsed or the view mode switched (got %d, want %d)", got, undoBefore)
	}

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

// TestSynthKnobShiftWheelIsFiner proves Shift-held wheel notches nudge a
// continuous knob by a SMALLER amount than an unshifted notch (Task-5's
// "fine" wheel path in nudgeKnobByWheel), and that the temporary finer step
// never persists: the knob's normal StepMul and the step badge's active
// rung are unchanged afterward.
func TestSynthKnobShiftWheelIsFiner(t *testing.T) {
	// Two independent games (rather than one game hit twice) so the second
	// trial isn't contaminated by the first commit's param propagation
	// re-syncing the knob's Value out from under a manual reset.
	setup := func(t *testing.T) (*Game, int, image.Rectangle) {
		t.Helper()
		g := newModularSynthTabGame(t)
		expandSynthPanelForTest(t, g)
		idx, r := firstContinuousSynthKnob(t, g)
		return g, idx, r
	}

	gU, idxU, rU := setup(t)
	kU := gU.drum.instEditorKnobs[idxU]
	if idxU >= len(gU.drum.instEditorStepBadges) || gU.drum.instEditorStepBadges[idxU] == nil {
		t.Fatal("filter_cutoff knob has no step badge")
	}
	badgeU := gU.drum.instEditorStepBadges[idxU]
	if badgeU.idx <= 0 {
		t.Skip("badge already at the finest rung — no finer rung available to test against")
	}
	startU := kU.Value
	dispatchWheelAt(t, gU, rU.Min.X+4, rU.Min.Y+4, +1)
	unshiftedDelta := kU.Value - startU
	if unshiftedDelta == 0 {
		t.Fatal("unshifted wheel notch did not change value")
	}

	gS, idxS, rS := setup(t)
	kS := gS.drum.instEditorKnobs[idxS]
	badgeS := gS.drum.instEditorStepBadges[idxS]
	stepMulBefore := kS.StepMul
	idxBefore := badgeS.idx
	startS := kS.Value
	cx, cy := rS.Min.X+4, rS.Min.Y+4

	// One Shift-held notch, simulated via the SetInputForTest key stub.
	gS.drum.eqPanelZone.Invalidate()
	wheelActive := true
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(b ebiten.MouseButton) bool { return false },
		func(kk ebiten.Key) bool { return kk == ebiten.KeyShiftLeft },
		func() []rune { return nil },
		func() (float64, float64) {
			if wheelActive {
				return 0, 1
			}
			return 0, 0
		},
		func() (int, int) { return 1280, 720 },
	)
	gS.Update()
	wheelActive = false
	gS.Update()
	restore()
	shiftedDelta := kS.Value - startS

	if shiftedDelta == 0 {
		t.Fatal("shifted wheel notch did not change value")
	}
	if math.Abs(shiftedDelta) >= math.Abs(unshiftedDelta) {
		t.Fatalf("shift-held notch must be finer than an unshifted notch: unshifted=%.6f shifted=%.6f", unshiftedDelta, shiftedDelta)
	}
	if kS.StepMul != stepMulBefore {
		t.Fatalf("k.StepMul must be restored after a fine wheel nudge: before=%.6f after=%.6f", stepMulBefore, kS.StepMul)
	}
	if badgeS.idx != idxBefore {
		t.Fatalf("badge rung must not persist a fine wheel nudge: before=%d after=%d", idxBefore, badgeS.idx)
	}
}
