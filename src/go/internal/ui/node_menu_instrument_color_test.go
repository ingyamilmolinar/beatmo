//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestNodeSidebarTintsToOwningInstrumentColor verifies the desktop node pop-up
// (sidebar) draws its header underline and active section stripes in the
// owning row's INSTRUMENT color, not the fixed azure accent.
func TestNodeSidebarTintsToOwningInstrumentColor(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n := g.tryAddNode(3, 3, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add node")
	}
	// Bind the node to row 0 and give that row a distinctive warm color.
	g.nodeRows[n.ID] = 0
	want := color.RGBA{190, 120, 60, 255} // warm orange
	g.drum.Rows[0].Color = want

	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.layout()

	dst := ebiten.NewImage(640, 480)
	rects := collectFilledRects(t, func() { g.sidebar.Draw(dst) })

	// An expanded section header draws a solid left accent stripe in the
	// instrument color (thin, tall).
	stripe := false
	warmUnderline := false
	for _, dr := range rects {
		if dr.Color == want && dr.Rect.Dx() <= accentStripeW()+1 && dr.Rect.Dy() >= 12 {
			stripe = true
		}
		// Header underline: alpha-blended tint of the warm color (R >= B).
		if dr.Rect.Dy() == 1 && dr.Color.A > 0 && dr.Color.R >= dr.Color.B && dr.Color.R > 0 {
			warmUnderline = true
		}
	}
	if !stripe {
		t.Errorf("expanded section header should draw a solid instrument-color (%v) stripe", want)
	}
	if !warmUnderline {
		t.Errorf("node sidebar header underline should tint to the warm instrument color")
	}
}

// TestLongPressPopupConnectHoverTintsToInstrument verifies the mobile node
// long-press popup's Connect hover accent uses the node's instrument color.
func TestLongPressPopupConnectHoverTintsToInstrument(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n := g.tryAddNode(3, 3, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add node")
	}
	g.nodeRows[n.ID] = 0
	want := color.RGBA{190, 120, 60, 255}
	g.drum.Rows[0].Color = want

	g.longPressPopupNode = n
	g.longPressPopup = true
	g.longPressPopupRect = image.Rect(100, 100, 360, 180)
	g.longPressPopupMove = image.Rect(110, 130, 190, 170)
	g.longPressPopupConn = image.Rect(200, 130, 280, 170)
	g.longPressPopupDel = image.Rect(290, 130, 350, 170)
	g.longPressPopupHover = "connect"

	got := g.longPressPopupAccent()
	if !colorsEqual(got, want) {
		t.Errorf("long-press popup accent = %v, want node instrument color %v", got, want)
	}
}

// TestNodeSidebarButtonBordersUseInstrumentColor verifies that the node
// sidebar's interactive buttons (the ± steppers and the logic selector) carry
// borders in the node's instrument color, not the fixed light-blue stepper
// border / white secondary border.
func TestNodeSidebarButtonBordersUseInstrumentColor(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 1000)

	n := g.tryAddNode(3, 3, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add node")
	}
	g.nodeRows[n.ID] = 0
	want := color.RGBA{190, 120, 60, 255}
	g.drum.Rows[0].Color = want

	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.layout()

	volRect, ok := g.sidebar.rects["vol+"]
	if !ok || volRect.Empty() || !g.sidebar.inViewport(volRect) {
		t.Skip("vol+ not laid out / not in viewport")
	}

	// Intercept border (unfilled) drawRect calls — collectFilledRects only
	// records filled rects.
	var borders []drawnRect
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if !filled {
			borders = append(borders, drawnRect{Rect: r, Color: color.RGBAModel.Convert(c).(color.RGBA)})
		}
		orig(dst, r, c, filled)
	}
	g.sidebar.Draw(ebiten.NewImage(640, 1000))
	drawRect = orig

	// The vol+ stepper button must be outlined in the instrument color.
	stepperTinted := false
	for _, b := range borders {
		if b.Color == want && b.Rect == volRect {
			stepperTinted = true
		}
	}
	if !stepperTinted {
		t.Errorf("vol+ stepper border should be the instrument color %v", want)
	}

	// The logic selector button's style border must also be the instrument color.
	if lb := g.sidebar.btns["logic"]; lb != nil {
		if bs, ok := lb.Style.(ButtonStyle); ok {
			got := color.RGBAModel.Convert(bs.Border).(color.RGBA)
			if got != want {
				t.Errorf("logic selector border = %v, want instrument color %v", got, want)
			}
		} else {
			t.Errorf("logic button style is %T, want ButtonStyle", lb.Style)
		}
	}
}

// TestNodeSidebarSelectedLogicOptionTintsToInstrument verifies that the
// currently-selected logic dropdown option carries an instrument-color active
// highlight, isolated from the section-header stripe by its Y position.
func TestNodeSidebarSelectedLogicOptionTintsToInstrument(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Tall window so the expanded logic dropdown items sit inside the sidebar
	// viewport (off-screen items are correctly not drawn).
	g.Layout(640, 1000)

	n := g.tryAddNode(3, 3, model.NodeTypeRegular)
	if n == nil {
		t.Fatal("could not add node")
	}
	g.nodeRows[n.ID] = 0
	want := color.RGBA{190, 120, 60, 255}
	g.drum.Rows[0].Color = want

	g.sidebar.Open(n)
	g.sidebar.ExpandAllSections()
	g.sidebar.sectionOpen["logic"] = true
	g.sidebar.logicDropdownOpen = true
	g.sidebar.layout()

	// The selected logic option for a fresh regular node is "none" → id "logic:".
	selRect, ok := g.sidebar.rects["logic:"]
	if !ok || selRect.Empty() || !g.sidebar.inViewport(selRect) {
		t.Skip("selected logic option rect not laid out / not in viewport")
	}

	dst := ebiten.NewImage(640, 1000)
	rects := collectFilledRects(t, func() { g.sidebar.Draw(dst) })

	found := false
	for _, dr := range rects {
		if dr.Color == want && dr.Rect.Dx() <= accentStripeW()+1 && dr.Rect.Dy() >= 8 &&
			dr.Rect.Min.Y >= selRect.Min.Y-2 && dr.Rect.Max.Y <= selRect.Max.Y+2 {
			found = true
		}
	}
	if !found {
		t.Errorf("selected logic option should draw an instrument-color (%v) stripe within its row %v", want, selRect)
	}
}
