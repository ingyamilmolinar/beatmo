package ui

// Tests for the 2026-07-16 hover-coverage pass: every clickable shows a
// hover cue on desktop, driven by shared mechanisms (portalHoverer for
// portal overlays, hoverFade for hand-drawn sidebar surfaces, the eased
// node bloom for grid nodes, glowTarget for tree-registered synth chips).

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// hoverTestInput points the shared input seams at a fixed cursor, unpressed.
func hoverTestInput(t *testing.T, x, y int) {
	t.Helper()
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	t.Cleanup(restore)
}

// TestPortalHoverLiftsFXPanelButtons pins the portalHoverer seam: plain
// cursor-over refreshes a portal overlay's button hovered flags each frame
// (portal input handlers only run on press edges, so without the seam a
// popup's buttons never hover).
func TestPortalHoverLiftsFXPanelButtons(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	dv := g.drum
	dv.OpenFXPanel(0)
	if len(dv.fxPanelBtns) == 0 {
		t.Fatal("FX panel opened with no buttons")
	}
	var target *Button
	for _, b := range dv.fxPanelBtns {
		if b != nil && !b.Rect().Empty() {
			target = b
			break
		}
	}
	if target == nil {
		t.Fatal("no FX button with a rect")
	}
	r := target.Rect()
	hoverTestInput(t, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)

	dv.portal().Update()
	if !target.hovered {
		t.Error("FX button not hovered with cursor over it after portal Update")
	}

	// Cursor away → hover clears on the next frame.
	hoverTestInput(t, 0, 599)
	dv.portal().Update()
	if target.hovered {
		t.Error("FX button still hovered with cursor away")
	}
}

// TestSidebarSectionHeaderHoverLift pins the hoverFade path: hovering an
// expandable section header eases the shared lift in (headers are not
// Buttons, so they need the sidebar's own hover resolution).
func TestSidebarSectionHeaderHoverLift(t *testing.T) {
	assertDefaultParityState(t)
	forceSmallScreenForTest = false
	t.Cleanup(func() { forceSmallScreenForTest = false; UpdateProfile() })
	UpdateProfile()

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	n := g.tryAddNode(3, 3, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add node")
	}
	g.sidebar.Open(n)
	g.sidebar.layout()
	r, ok := g.sidebar.rects["sec-vol"]
	if !ok || r.Empty() {
		t.Fatal("sec-vol header rect missing")
	}
	hoverTestInput(t, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)

	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		g.sidebar.Draw(screen)
	}
	if lift := g.sidebar.hover.Lift("sec-vol"); lift < 0.9 {
		t.Errorf("section-header hover lift = %v after 30 frames, want ~1", lift)
	}

	// Cursor away → the lift eases back out.
	hoverTestInput(t, 790, 590)
	for i := 0; i < 60; i++ {
		g.sidebar.Draw(screen)
	}
	if lift := g.sidebar.hover.Lift("sec-vol"); lift > 0.05 {
		t.Errorf("section-header hover lift = %v after leave, want ~0", lift)
	}
}

// TestNodeHoverGlowEases pins the grid-node hover halo: the eased progress
// rises toward 1 while a node is hovered and decays after the cursor leaves,
// with the halo target retained for the fade-out.
func TestNodeHoverGlowEases(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	n := g.tryAddNode(3, 3, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add node")
	}

	g.hover = n
	for i := 0; i < 30; i++ {
		g.advanceNodeHoverGlow()
	}
	if g.hoverGlowP < 0.9 {
		t.Errorf("node hover glow = %v after 30 frames, want ~1", g.hoverGlowP)
	}
	if g.hoverGlowNode != n {
		t.Error("hoverGlowNode not tracking the hovered node")
	}

	g.hover = nil
	g.advanceNodeHoverGlow()
	if g.hoverGlowNode != n {
		t.Error("hoverGlowNode dropped immediately — fade-out needs the target")
	}
	for i := 0; i < 60; i++ {
		g.advanceNodeHoverGlow()
	}
	if g.hoverGlowP != 0 || g.hoverGlowNode != nil {
		t.Errorf("node hover glow did not settle to zero (p=%v node=%v)", g.hoverGlowP, g.hoverGlowNode)
	}
}

// TestSynthChipAdapterIsGlowTarget pins that pipeline chips participate in
// the tree hover overlay (the compile-time assertion is the contract; the
// nil-dv case returns the hit rect so hover works even before wiring).
func TestSynthChipAdapterIsGlowTarget(t *testing.T) {
	var gt glowTarget = &synthChipHitAdapter{}
	r := image.Rect(10, 10, 60, 40)
	if got := gt.glowRect(0, 0, r); got != r {
		t.Errorf("chip glowRect = %v, want hit rect %v", got, r)
	}
}

// TestSynthKnobCardAdapterIsGlowTarget pins that a Synth-tab param card
// participates in the tree hover overlay: the whole card is the hover target
// (a tap anywhere on it opens the value-wheel popup), so glowRect must
// return the hit rect unchanged.
func TestSynthKnobCardAdapterIsGlowTarget(t *testing.T) {
	var gt glowTarget = &synthKnobHitAdapter{}
	r := image.Rect(5, 5, 80, 50)
	if got := gt.glowRect(0, 0, r); got != r {
		t.Errorf("synth card glowRect = %v, want hit rect %v", got, r)
	}
}

// TestSamplerKnobCardAdapterIsGlowTarget mirrors
// TestSynthKnobCardAdapterIsGlowTarget for the Sampler-tab param cards.
func TestSamplerKnobCardAdapterIsGlowTarget(t *testing.T) {
	var gt glowTarget = &samplerKnobHitAdapter{}
	r := image.Rect(5, 5, 80, 50)
	if got := gt.glowRect(0, 0, r); got != r {
		t.Errorf("sampler card glowRect = %v, want hit rect %v", got, r)
	}
}

// TestMobileWheelPopupPortalOverlayIsPortalHoverer pins the seam this popup
// uses for desktop hover feedback: OverlayPortal.Update() dispatches
// HoverAt(mx, my) to every stacked overlay implementing portalHoverer once
// per frame (portal input handlers only run on press/drag/release edges).
// The portal wrapper must delegate to the popup's own HoverAt so the ladder
// rung / center grip hover tint (see wheel_ladder_test.go) actually lights up
// when the popup is open through the real portal stack, not just via a
// direct unit-level call.
func TestMobileWheelPopupPortalOverlayIsPortalHoverer(t *testing.T) {
	w, _, _ := mkWheelForLadder(t)
	o := &mobileWheelPopupPortalOverlay{popup: w, tag: "test-wheel"}
	var hv portalHoverer = o
	rungs := w.rungRects()
	if len(rungs) == 0 {
		t.Fatal("no rungs to hover")
	}
	p := rungs[0].Min.Add(image.Pt(2, 2))
	hv.HoverAt(p.X, p.Y)
	if w.hoverRung != 0 {
		t.Errorf("portal HoverAt did not reach popup: hoverRung = %d, want 0", w.hoverRung)
	}
}
