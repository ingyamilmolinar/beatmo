//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRackMaskBelowRowRack pins the load-bearing invariant for the regression
// where the rack-column surface fill was registered above ZRowRack and so
// painted opaquely on top of every row label/button on desktop. The rack
// surface MUST render before the row rack zone or its opaque fill erases the
// row controls. This is a Z-order constant check — fast, deterministic, and
// independent of layer registration order.
func TestRackMaskBelowRowRack(t *testing.T) {
	if ZRackMask >= ZRowRack {
		t.Fatalf("ZRackMask (%d) must be below ZRowRack (%d): the rack-column surface fill is opaque (alpha=255) and would erase per-row labels/buttons if painted above the row rack zone (regression that hid every instrument label on desktop screenshots).",
			ZRackMask, ZRowRack)
	}
}

// TestDesktopRowRack_NoOpaqueOverpaintOnRackColumn pins the user-observable
// outcome at a higher level than the Z-constant check: with default desktop
// startup at 1280x720, the live merged-layer order produced by the
// DrumViewTree must place the rack-mask layer BEFORE the row-rack zone. This
// catches mistakes where a layer is registered with a stale Z (e.g., copied
// from an older constant) even when ZRackMask itself is correct.
func TestDesktopRowRack_NoOpaqueOverpaintOnRackColumn(t *testing.T) {
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	for i := 0; i < 32; i++ {
		_ = g.Update()
	}

	if Profile().IsMobile() {
		t.Fatalf("default startup at 1280x720 should be desktop-class profile, got mobile")
	}
	if g.drum == nil || g.drum.tree == nil {
		t.Fatalf("drum or drum.tree is nil after Layout/Update")
	}

	layers := g.drum.tree.LayersForTest()
	rackMaskIdx, rowRackIdx := -1, -1
	for i, l := range layers {
		switch l.ID() {
		case "rack-mask":
			rackMaskIdx = i
		case "row-rack":
			rowRackIdx = i
		}
	}
	if rackMaskIdx < 0 {
		t.Fatalf("rack-mask layer not registered with DrumViewTree")
	}
	if rowRackIdx < 0 {
		t.Fatalf("row-rack zone not registered with DrumViewTree")
	}
	if rackMaskIdx >= rowRackIdx {
		var ids []string
		for _, l := range layers {
			ids = append(ids, l.ID())
		}
		t.Fatalf("rack-mask must render before row-rack (so the rack surface fill sits beneath row controls). got rack-mask at index %d, row-rack at index %d. Order: %v",
			rackMaskIdx, rowRackIdx, ids)
	}

	// Defense-in-depth: verify the rack-mask layer's actual Draw produces an
	// opaque colRackSurface fill (its raison d'être), and that this fill
	// covers the rack column. Catches a future "no-op the layer" regression
	// that would silently leave the rack column visually undifferentiated.
	wantSurface := color.RGBAModel.Convert(colRackSurface).(color.RGBA)
	dst := ebiten.NewImage(1280, 720)
	rects := collectFilledRects(t, func() {
		g.Draw(dst)
	})
	rackRect := g.drum.widgetRects[WidgetRack]
	if rackRect.Empty() {
		t.Fatalf("WidgetRack rect empty after Layout — cannot validate rack-mask fill")
	}
	foundFill := false
	for _, r := range rects {
		if r.Color != wantSurface {
			continue
		}
		if r.Color.A != 0xFF {
			continue
		}
		// Mask covers the rack column horizontally and the rows region
		// vertically (header is excluded by the layer's Y clamp).
		if !rectContains(r.Rect, image.Rect(rackRect.Min.X, rackRect.Min.Y, rackRect.Max.X, rackRect.Min.Y+1)) {
			continue
		}
		foundFill = true
		break
	}
	if !foundFill {
		t.Fatalf("expected an opaque colRackSurface fill spanning the rack column (%v) — rack-mask layer did not draw its surface", rackRect)
	}
}

func rectContains(outer, inner image.Rectangle) bool {
	return outer.Min.X <= inner.Min.X && outer.Min.Y <= inner.Min.Y &&
		outer.Max.X >= inner.Max.X && outer.Max.Y >= inner.Max.Y
}
