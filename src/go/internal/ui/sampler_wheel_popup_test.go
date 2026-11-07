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

// A tap on a sampler card opens the shared MobileWheelPopup on every platform.

// Forced mobile, row 0 captured as a synth source so the knobs lay out and the commit is real, on a panel tall
// enough for several knobs.
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
	// An accepted edit lands in the process-wide sample-edit store; clear it so a later capture of this instrument
	// starts clean.
	t.Cleanup(func() { audio.ClearSampleEdit(instID) })
	g.drum.sampler.captureFromSynth(instID)
	if !g.drum.sampler.hasBuffer() || g.drum.sampler.captureID == "" {
		t.Skipf("could not capture a sampler buffer for %q", instID)
	}
	g.drum.buildSamplerTab(image.Rect(0, 0, 390, 520), instID)
	return g, string(instID)
}

// Each visible card sits inside its cell, below the group-label band, never in a neighbouring row.
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

// Opens as a modal portal and consumes the press.
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

// Opens and consumes on desktop too.
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

// The value moves live through OnChange, and the one undo step lands on Accept, not on drag release.
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

	// Above the centre box, which confirms.
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

	// A centre click confirms: Accept and close.
	if w.IsOpen() {
		t.Fatal("a centre click must confirm and close the sampler wheel")
	}
}

// A modal portal, so a grid tap beneath creates no node.
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

// Through the tree's click-outside path, dropping the portal entry.
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
