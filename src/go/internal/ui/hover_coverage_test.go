package ui

// Every clickable shows a hover cue on desktop through a shared mechanism: portalHoverer, hoverFade, the eased node
// bloom, or glowTarget.

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

// Portal input handlers run only on press edges, so without the portalHoverer seam a popup's buttons never hover.
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

// Section headers are not Buttons, so they need the sidebar's own hover resolution.
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
	r, ok := g.sidebar.rects[sbSectionHeader(secVol)]
	if !ok || r.Empty() {
		t.Fatal("sec-vol header rect missing")
	}
	hoverTestInput(t, r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2)

	screen := ebiten.NewImage(800, 600)
	for i := 0; i < 30; i++ {
		g.sidebar.Draw(screen)
	}
	if lift := g.sidebar.hover.Lift(sbSectionHeader(secVol).String()); lift < 0.9 {
		t.Errorf("section-header hover lift = %v after 30 frames, want ~1", lift)
	}

	// Cursor away → the lift eases back out.
	hoverTestInput(t, 790, 590)
	for i := 0; i < 60; i++ {
		g.sidebar.Draw(screen)
	}
	if lift := g.sidebar.hover.Lift(sbSectionHeader(secVol).String()); lift > 0.05 {
		t.Errorf("section-header hover lift = %v after leave, want ~0", lift)
	}
}

// The halo target is retained so the glow fades out after the cursor leaves.
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

// The whole card is the target (a tap anywhere opens the wheel), so glowRect returns the hit rect unchanged.
func TestSynthControlCardAdapterIsGlowTarget(t *testing.T) {
	var gt glowTarget = &synthControlHitAdapter{}
	r := image.Rect(5, 5, 80, 50)
	if got := gt.glowRect(0, 0, r); got != r {
		t.Errorf("synth card glowRect = %v, want hit rect %v", got, r)
	}
}

func TestSamplerKnobCardAdapterIsGlowTarget(t *testing.T) {
	var gt glowTarget = &samplerKnobHitAdapter{}
	r := image.Rect(5, 5, 80, 50)
	if got := gt.glowRect(0, 0, r); got != r {
		t.Errorf("sampler card glowRect = %v, want hit rect %v", got, r)
	}
}

// The portal wrapper must delegate HoverAt to the popup, or its ladder hover tint never lights through the real
// portal stack.
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
