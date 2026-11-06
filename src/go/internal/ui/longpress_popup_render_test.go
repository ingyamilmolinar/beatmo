//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestDrawRoundedRect_CornerShape verifies that drawRoundedRect produces a
// convex corner arc: fill widths must increase monotonically from tip (row 0)
// to base (row radius-1). A concave/inverted arc would have widths decrease.
func TestDrawRoundedRect_CornerShape(t *testing.T) {
	assertDefaultParityState(t)

	for _, radius := range []int{4, 8, 12, 16} {
		t.Run("", func(t *testing.T) {
			widths := cornerFillWidths(t, radius)

			// Row 0 (tip of the arc) should have minimal fill.
			if widths[0] >= radius {
				t.Errorf("radius=%d: row 0 width=%d, want < %d (tip should be narrow)",
					radius, widths[0], radius)
			}

			// Last row (base, adjacent to body) should have near-full fill.
			last := widths[radius-1]
			if last < radius/2 {
				t.Errorf("radius=%d: last row width=%d, want >= %d (base should be wide)",
					radius, last, radius/2)
			}

			// Widths must be monotonically non-decreasing (convex property).
			assertCornerConvex(t, widths)
		})
	}
}

// TestDrawRoundedRect_StrokedCornerShape verifies that the stroked variant
// traces a convex arc path: stroke insets must decrease monotonically from
// large (row 0) to 0 (row radius-1).
func TestDrawRoundedRect_StrokedCornerShape(t *testing.T) {
	assertDefaultParityState(t)

	for _, radius := range []int{4, 8, 12} {
		t.Run("", func(t *testing.T) {
			insets := strokeCornerInsets(t, radius)

			// Row 0 (tip) may have no stroke pixel within the corner region
			// because the correct convex arc places it at x=radius. Skip it.
			// Last row should have inset near 0 (base of arc near the edge).
			last := insets[radius-1]
			if last < 0 {
				t.Fatalf("radius=%d: no stroke pixel at last row", radius)
			}
			if last > 1 {
				t.Errorf("radius=%d: last row inset=%d, want <= 1 (base should be at edge)",
					radius, last)
			}

			// Insets must be monotonically non-increasing (convex arc).
			for i := 1; i < len(insets); i++ {
				if insets[i] < 0 || insets[i-1] < 0 {
					continue
				}
				if insets[i] > insets[i-1] {
					t.Errorf("radius=%d: stroke inset increases at row %d (%d > %d), not convex (insets=%v)",
						radius, i, insets[i], insets[i-1], insets)
					break
				}
			}
		})
	}
}

// TestLongPressPopupButtonsRounded verifies that the long-press popup uses
// drawRoundedButton (not square drawButton) for all three action buttons.
func TestLongPressPopupButtonsRounded(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	centerCameraOn(g, 2, 2)

	// Add a node and trigger the popup.
	g.tryAddNode(2, 2, model.NodeTypeRegular)
	g.updateBeatInfos()
	node := g.nodeAt(2, 2)
	if node == nil {
		t.Fatalf("node at (2,2) not found")
	}

	cx, cy := nodeCenter(g, node)
	g.showLongPressPopup(node, cx, cy)
	if !g.longPressPopup {
		t.Fatalf("popup not shown")
	}

	// Record draw calls during popup rendering.
	var rec drawCallRecorder
	screen := ebiten.NewImage(640, 480)
	rec.record(t, func() {
		g.drawLongPressPopup(screen)
	})

	// Each button rect should have a drawRoundedButton call, not drawButton.
	for _, btn := range []struct {
		name string
		rect image.Rectangle
	}{
		{"move", g.longPressPopupMove},
		{"copy", g.longPressPopupCopy},
		{"connect", g.longPressPopupConn},
		{"delete", g.longPressPopupDel},
	} {
		if rec.hasSquareButton(btn.rect) {
			t.Errorf("%s button uses square drawButton at %v, want a rounded body", btn.name, btn.rect)
		}
		// Flat-tonal paints the body via drawFlatButtonBody -> drawRoundedRect
		// (filled), not the drawRoundedButton fill+border composite.
		var rounded bool
		for _, c := range rec.calls {
			if c.Kind == drawCallRoundedRect && c.Filled && c.Rect == btn.rect && c.Radius > 0 {
				rounded = true
				break
			}
		}
		if !rounded {
			t.Errorf("%s button missing filled rounded body at %v", btn.name, btn.rect)
		}
	}

	// No square buttons should exist inside the popup panel.
	rec.assertNoSquareButtons(t, g.longPressPopupRect)
}

// TestLongPressPopupButtonColors verifies that Move/Connect buttons use
// colDropdown fill and Delete uses colDeleteFill on the cap face.
//
// After the keycap restyle each button renders as a raised keycap:
//   - drawKeycapShell paints the socket at the full hit rect (shell = adjustColor(fill,-45))
//   - drawRoundedButton paints the cap face at keycapCapRect(hitRect, 0, true)
//     with the semantic fill color
//
// We therefore check the cap-face rect, not the hit rect.
func TestLongPressPopupButtonColors(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	centerCameraOn(g, 2, 2)

	g.tryAddNode(2, 2, model.NodeTypeRegular)
	g.updateBeatInfos()
	node := g.nodeAt(2, 2)
	if node == nil {
		t.Fatalf("node at (2,2) not found")
	}

	cx, cy := nodeCenter(g, node)
	g.showLongPressPopup(node, cx, cy)
	if !g.longPressPopup {
		t.Fatalf("popup not shown")
	}

	var rec drawCallRecorder
	screen := ebiten.NewImage(640, 480)
	rec.record(t, func() {
		g.drawLongPressPopup(screen)
	})

	expectDropdown := color.RGBAModel.Convert(colDropdown).(color.RGBA)
	expectDelete := color.RGBAModel.Convert(colDeleteFill).(color.RGBA)

	// Flat-tonal: the body IS the hit rect (no inset cap face), painted by
	// drawFlatPanelButton -> drawFlatButtonBody -> drawRoundedRect. We look for
	// a filled rounded rect at the button rect whose fill is the semantic color.
	for _, btn := range []struct {
		name      string
		hitRect   image.Rectangle
		wantColor color.RGBA
	}{
		{"move", g.longPressPopupMove, expectDropdown},
		{"copy", g.longPressPopupCopy, expectDropdown},
		{"connect", g.longPressPopupConn, expectDropdown},
		{"delete", g.longPressPopupDel, expectDelete},
	} {
		capRect := btn.hitRect
		found := false
		for _, c := range rec.calls {
			if c.Kind == drawCallRoundedRect && c.Rect == capRect && c.Color == btn.wantColor {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s button: no filled rounded rect at %v with fill %v", btn.name, capRect, btn.wantColor)
		}
	}
}

// TestLongPressPopup_KeycapsHaveIconRoom verifies each quick-action keycap is
// tall enough to hold a leading/top icon (IconSizeMD) stacked above a
// RoleBody label, per the icon+label keycap draw upgrade.
func TestLongPressPopup_KeycapsHaveIconRoom(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	centerCameraOn(g, 2, 2)

	g.tryAddNode(2, 2, model.NodeTypeRegular)
	g.updateBeatInfos()
	node := g.nodeAt(2, 2)
	if node == nil {
		t.Fatalf("node at (2,2) not found")
	}

	cx, cy := nodeCenter(g, node)
	g.showLongPressPopup(node, cx, cy)
	if !g.longPressPopup {
		t.Fatalf("popup not shown")
	}

	minH := IconSizeMD + StyledTextHeight(RoleBody)
	for _, btn := range []struct {
		name string
		rect image.Rectangle
	}{
		{"move", g.longPressPopupMove},
		{"copy", g.longPressPopupCopy},
		{"connect", g.longPressPopupConn},
		{"delete", g.longPressPopupDel},
	} {
		if btn.rect.Dy() < minH {
			t.Errorf("%s keycap too short for icon+label: got %d, want >= %d", btn.name, btn.rect.Dy(), minH)
		}
	}
}
