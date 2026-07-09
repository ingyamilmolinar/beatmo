//go:build test

package ui

import (
	"image"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/hajimehoshi/ebiten/v2"
)

// These tests pin the mobile-only behavior: a tap on a sampler knob opens the
// shared MobileWheelPopup (the same control the Synth tab uses), while desktop
// keeps the in-place rotary drag. Desktop must NOT open the popup.

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
	g.drum.sampler.captureFromSynth(instID)
	if !g.drum.sampler.hasBuffer() || g.drum.sampler.captureID == "" {
		t.Skipf("could not capture a sampler buffer for %q", instID)
	}
	g.drum.buildSamplerTab(image.Rect(0, 0, 390, 520), instID)
	return g, instID
}

// TestSamplerMobileKnobRendersButtonNotDial pins the VISUAL contract: on mobile
// every visible sampler knob renders the tap-to-open value-pill BUTTON (a
// non-empty rect inside the knob cell), not the rotary dial. This guards the
// reported regression where the sampler tab still showed dials on mobile.
func TestSamplerMobileKnobRendersButtonNotDial(t *testing.T) {
	g, instID := newMobileSamplerWheelGame(t)
	dv := g.drum
	// A real draw populates the dial rects the button geometry is derived from.
	img := newTrackedImage("test.samplermobilebutton", 390, 720)
	defer releaseImage(img)
	g.Draw(img)

	visible := 0
	for i, k := range dv.sampler.knobs {
		if k == nil || k.Rect().Empty() {
			continue
		}
		visible++
		br := dv.samplerMobileKnobButtonRect(i)
		if br.Empty() {
			t.Errorf("knob %d: mobile button rect empty (still a dial?)", i)
			continue
		}
		// The button must sit within the knob cell so it never bleeds into a
		// neighbouring row.
		cell := dv.sampler.knobCells[i]
		if cell.Empty() {
			cell = k.Rect()
		}
		if !br.In(cell) {
			t.Errorf("knob %d: button %v escapes its cell %v", i, br, cell)
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

	if res != InputConsumed {
		t.Fatalf("mobile knob press must return InputConsumed (got %v) so the tap opens the popup, not a rotary drag", res)
	}
	if !dv.samplerWheelPopup.IsOpen() {
		t.Fatal("mobile knob press did not open the sampler wheel popup")
	}
	if dv.tree == nil || !dv.portal().Has("sampler-wheel-popup") {
		t.Fatal("sampler wheel popup did not open as a portal entry")
	}
}

// TestSamplerKnobPressDesktopNoPopup guards that desktop is untouched: a knob
// press starts the in-place rotary drag (InputCaptured) and never opens the
// popup. This pins the IsMobile() gate.
func TestSamplerKnobPressDesktopNoPopup(t *testing.T) {
	g := samplerLayoutGame(t) // desktop 1280x720, comfortable density
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	g.drum.sampler.captureFromSynth("kick")
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 180), "kick")

	img := newTrackedImage("test.samplerwheeldesktop", 1280, 720)
	defer releaseImage(img)
	g.Draw(img) // populate knob dial rects

	dv := g.drum
	idx := samplerKnobGain
	k := dv.sampler.knobs[idx]
	if k == nil || k.Rect().Empty() {
		t.Fatalf("desktop sampler knob %d rect empty after draw", idx)
	}
	r := k.Rect()
	// Press the DIAL square (top knobD×knobD), where the rotary drag begins.
	cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dx()/2

	h := &samplerKnobHitAdapter{dv: dv, idx: idx}
	res := h.OnPress(cx, cy)

	if dv.samplerWheelPopup != nil && dv.samplerWheelPopup.IsOpen() {
		t.Fatal("desktop knob press must NOT open the wheel popup")
	}
	if res != InputCaptured {
		t.Fatalf("desktop knob press must capture the rotary drag (got %v)", res)
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
func TestSamplerWheelPopupCenterTapOpensEditor(t *testing.T) {
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

	if dv.paramEditor == nil || !dv.paramEditor.Active() {
		t.Fatal("center-box tap did not open the sampler numeric editor")
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
