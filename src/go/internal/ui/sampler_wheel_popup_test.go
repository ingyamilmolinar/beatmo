//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// These tests pin the universal behavior: a tap on a sampler knob CARD opens the
// shared MobileWheelPopup (the same control the Synth tab uses) on EVERY platform
// — desktop and mobile alike (the old desktop in-place rotary drag is retired).

// newMobileSamplerWheelGame builds a Game with the Sampler tab active under a
// forced MOBILE profile, captures row-0's instrument as a synth-source buffer
// (so knobs lay out and commitSamplerEdit is non-vacuous), and lays out the tab
// on a mobile panel tall enough to show several knobs. Returns the game and the
// captured instrument id.
func newMobileSamplerWheelGame(t *testing.T) (*Game, string) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 720)
	g.drum.eqPanelZone.SetActiveTab(TabSampler)
	withSmallScreen(t, true)
	restore := SetDensityForTest(DensitySpacious)
	t.Cleanup(restore)
	instID := g.drum.Rows[0].Instrument
	// A test here may Accept a wheel edit, committing a sample-edit descriptor to
	// the internal/audio package-global store. Clear it on cleanup so a later
	// test capturing the same instrument (e.g. undo_sampler_test) sees a clean
	// baseline — the store is process-wide, not per-Game.
	t.Cleanup(func() { audio.ClearSampleEdit(instID) })
	g.drum.sampler.captureFromSynth(instID)
	if !g.drum.sampler.hasBuffer() || g.drum.sampler.captureID == "" {
		t.Skipf("could not capture a sampler buffer for %q", instID)
	}
	g.drum.buildSamplerTab(image.Rect(0, 0, 390, 520), instID)
	return g, string(instID)
}

// TestSamplerMobileKnobCardWithinCell pins the VISUAL contract: on mobile every
// visible sampler knob renders as a ParamCard whose rect sits inside the knob
// cell (below the group-label band), so it never bleeds into a neighbouring row.
func TestSamplerMobileKnobCardWithinCell(t *testing.T) {
	g, instID := newMobileSamplerWheelGame(t)
	dv := g.drum
	// A real draw populates the card rects.
	img := newTrackedImage("test.samplermobilecard", 390, 720)
	defer releaseImage(img)
	g.Draw(img)

	visible := 0
	for i, k := range dv.sampler.knobs {
		if k == nil || k.Rect().Empty() {
			continue
		}
		visible++
		card := k.Rect()
		cell := dv.sampler.knobCells[i]
		if cell.Empty() {
			cell = card
		}
		if !card.In(cell) {
			t.Errorf("knob %d: card %v escapes its cell %v", i, card, cell)
		}
	}
	if visible == 0 {
		t.Skipf("no sampler knobs visible for %q (layout produced none)", instID)
	}
}

// TestSamplerKnobPressOpensWheelOnMobile drives the production knob handler
// (samplerKnobHitAdapter.OnPress) under a mobile profile and asserts a press
// opens the wheel popup as a modal portal and consumes the press.
func TestSamplerKnobPressOpensWheelOnMobile(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	if dv.samplerWheelPopup == nil {
		t.Fatal("samplerWheelPopup not initialised")
	}
	if dv.samplerWheelPopup.IsOpen() {
		t.Fatal("precondition: popup must start closed")
	}

	h := &samplerKnobHitAdapter{dv: dv, idx: samplerKnobGain}
	res := h.OnPress(0, 0)

	if res != InputCaptured {
		t.Fatalf("mobile control press must CAPTURE the gesture (got %v) so a sweep can scroll instead of opening the popup", res)
	}
	if dv.samplerWheelPopup.IsOpen() {
		t.Fatal("mobile control press must not open the popup on the down frame; it opens on release")
	}
	h.OnRelease(0, 0)
	if !dv.samplerWheelPopup.IsOpen() {
		t.Fatal("mobile control tap did not open the sampler wheel popup on release")
	}
	if dv.tree == nil || !dv.portal().Has("sampler-wheel-popup") {
		t.Fatal("sampler wheel popup did not open as a portal entry")
	}
}

// TestSamplerKnobPressOpensWheelOnDesktop pins the universal contract: a desktop
// card press now OPENS the wheel popup (was: in-place rotary drag) and consumes
// the press. Inverts the retired "desktop must NOT open the popup" gate.
func TestSamplerKnobPressOpensWheelOnDesktop(t *testing.T) {
	g := samplerLayoutGame(t) // desktop 1280x720, comfortable density
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	g.drum.sampler.captureFromSynth(roster.Kick)
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 260), roster.Kick)

	img := newTrackedImage("test.samplerwheeldesktop", 1280, 720)
	defer releaseImage(img)
	g.Draw(img) // populate card rects

	dv := g.drum
	idx := samplerKnobGain
	k := dv.sampler.knobs[idx]
	if k == nil || k.Rect().Empty() {
		t.Fatalf("desktop sampler knob %d rect empty after draw", idx)
	}
	r := k.Rect()
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2

	h := &samplerKnobHitAdapter{dv: dv, idx: idx}
	res := h.OnPress(cx, cy)

	if res != InputCaptured {
		t.Fatalf("desktop card press must CAPTURE the gesture (got %v) so a drag can scroll; the popup opens on release", res)
	}
	if dv.samplerWheelPopup != nil && dv.samplerWheelPopup.IsOpen() {
		t.Fatal("desktop card press must not open the popup on the down frame; it opens on release")
	}
	h.OnRelease(cx, cy)
	if dv.samplerWheelPopup == nil || !dv.samplerWheelPopup.IsOpen() {
		t.Fatal("desktop card tap must open the wheel popup on release")
	}
}

// TestSamplerWheelPopupDragChangesValueAndCommitsOnAccept opens the popup for
// the gain knob, drags the barrel, and asserts the knob value changed (OnChange
// → applySamplerKnob) live but that the single undo step is recorded on Accept
// (OnCommit → commitSamplerEdit), NOT on drag release — the transactional model
// (Enter / tap-away persists; Esc reverts). One undo step per accepted edit.
func TestSamplerWheelPopupDragChangesValueAndCommitsOnAccept(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	idx := samplerKnobGain

	dv.openSamplerKnobWheelPopup(idx)
	w := dv.samplerWheelPopup
	if w == nil || !w.IsOpen() {
		t.Fatal("openSamplerKnobWheelPopup did not open the popup")
	}

	k := dv.sampler.knobs[idx]
	valBefore := k.Value

	g.undoManager.OnExternalLoad()
	depth0 := len(g.undoManager.undo)

	// Press in the barrel ABOVE the center value box (a press inside centerH
	// would open the numeric editor instead of starting a value drag).
	cx := (w.valRect.Min.X + w.valRect.Max.X) / 2
	py := w.valRect.Min.Y + 4
	if image.Pt(cx, py).In(w.centerH) {
		t.Fatalf("test press point (%d,%d) lands in the center editor box %v — pick a point above it", cx, py, w.centerH)
	}
	gap := Profile().DensityValues().MobileWheelTickGap
	w.HandleInput(cx, py, true)         // press
	w.HandleInput(cx, py+gap*20, true)  // drag down = increase (live preview)
	w.HandleInput(cx, py+gap*20, false) // release → NO commit (deferred)

	if k.Value <= valBefore {
		t.Fatalf("wheel drag down did not increase the knob value: %.4f -> %.4f", valBefore, k.Value)
	}
	if steps := len(g.undoManager.undo) - depth0; steps != 0 {
		t.Fatalf("drag release recorded %d undo steps, want 0 (commit is deferred to Accept)", steps)
	}

	w.Accept() // Enter / tap-away persists → exactly one undo step
	if steps := len(g.undoManager.undo) - depth0; steps != 1 {
		t.Fatalf("Accept recorded %d undo steps, want exactly 1", steps)
	}
}

// TestSamplerWheelPopupCenterTapOpensEditor asserts a tap on the popup's center
// value box opens the shared numeric editor (OpenEditor → openSamplerParamEditor).
func TestSamplerWheelPopupCenterTapConfirms(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	idx := samplerKnobGain

	dv.openSamplerKnobWheelPopup(idx)
	w := dv.samplerWheelPopup
	if w == nil || !w.IsOpen() {
		t.Fatal("openSamplerKnobWheelPopup did not open the popup")
	}
	if w.centerH.Empty() {
		t.Fatal("popup center value box rect empty")
	}

	cx := (w.centerH.Min.X + w.centerH.Max.X) / 2
	cy := (w.centerH.Min.Y + w.centerH.Max.Y) / 2
	w.HandleInput(cx, cy, true)
	w.HandleInput(cx, cy, false)

	// Typed entry was retired: a centre click CONFIRMS (Accept + close), the
	// uniform "click the middle to confirm" contract.
	if w.IsOpen() {
		t.Fatal("a centre click must confirm and close the sampler wheel")
	}
}

// TestSamplerWheelPopupBlocksGridTap pins input isolation: while the popup is
// open (a modal portal), a tap in the grid pane must NOT create a node beneath
// it. Mirrors the synth wheel popup isolation contract.
func TestSamplerWheelPopupBlocksGridTap(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	dv.openSamplerKnobWheelPopup(samplerKnobGain)
	if !dv.samplerWheelPopup.IsOpen() {
		t.Fatal("popup did not open")
	}

	gx, gy := 12, gridTopOffset()+8
	if !g.split.InGridPane(gx, gy) {
		t.Skipf("grid-pane test point (%d,%d) unavailable", gx, gy)
	}
	if pr := dv.samplerWheelPopup.Rect(); pr.Min.Y <= gy {
		t.Skipf("grid point y=%d overlaps popup %v", gy, pr)
	}

	nodesBefore := len(g.nodes)
	g.handleTapInGrid(gx, gy)
	if len(g.nodes) != nodesBefore {
		t.Fatalf("grid tap leaked through the open sampler wheel popup: nodes %d -> %d", nodesBefore, len(g.nodes))
	}
}

// TestSamplerWheelPopupClickOutsideCloses verifies a click outside the popup
// closes it (and drops the portal entry) via the tree's click-outside path.
func TestSamplerWheelPopupClickOutsideCloses(t *testing.T) {
	g, _ := newMobileSamplerWheelGame(t)
	dv := g.drum
	dv.openSamplerKnobWheelPopup(samplerKnobGain)
	if !dv.samplerWheelPopup.IsOpen() {
		t.Fatal("popup did not open")
	}

	// A point above the (bottom-anchored) popup rect.
	px, py := 4, 4
	if image.Pt(px, py).In(dv.samplerWheelPopup.Rect()) {
		t.Skipf("outside-point (%d,%d) is inside popup %v", px, py, dv.samplerWheelPopup.Rect())
	}
	press := func(down bool) {
		restore := SetInputForTest(
			func() (int, int) { return px, py },
			func(b ebiten.MouseButton) bool { return down && b == ebiten.MouseButtonLeft },
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { return 0, 0 },
			func() (int, int) { return g.winW, g.winH },
		)
		g.drum.Update()
		restore()
	}
	press(true)
	press(false)

	if dv.samplerWheelPopup.IsOpen() {
		t.Fatal("click outside should close the sampler wheel popup")
	}
	if dv.portal().Has("sampler-wheel-popup") {
		t.Fatal("portal should drop the sampler-wheel-popup entry after click-outside")
	}
}
