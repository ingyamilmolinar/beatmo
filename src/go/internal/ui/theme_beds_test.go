//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// TestDrumBedsFollowBackgroundCategory: the drum-view BEDS (row-rack column,
// mobile toolbar, add-row strip) share surface-1's hex by design but are
// AREAS, not controls — they must follow the Background category together
// with the cell grid and the canvas, or a background-only theme leaves a
// violet rack next to a recolored grid (the reported mismatch).
func TestDrumBedsFollowBackgroundCategory(t *testing.T) {
	for _, tok := range []string{"rack-bed", "toolbar-bed"} {
		if cat, ok := themeCategoryOfToken(tok); !ok || cat != "background" {
			t.Fatalf("%s must be a Background-category token (got %q, %v)", tok, cat, ok)
		}
	}
	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"background": "mint-500"}})
	defer restore()
	mint, _ := swatchRGBA("mint-500")
	mh, _, _ := rgbToHSL(mint)
	for name, c := range map[string]color.RGBA{"colRackSurface": colRackSurface, "colTransportSurface": colTransportSurface} {
		h, _, _ := rgbToHSL(c)
		if c == genColorDefaultByName["surface-1"] || hueDistance(h, mh) > 20 {
			t.Fatalf("%s = %v did not follow the background pick (hue %.0f vs %.0f); surface-1 (a CONTROL surface) still = %v", name, c, h, mh, colSurface1)
		}
	}
	if colSurface1 != genColorDefaultByName["surface-1"] {
		t.Fatalf("surface-1 is a control surface and must NOT move with the background: %v", colSurface1)
	}

	// Frame-level: the rack column is painted in the themed bed color, and
	// no large fill in that column still carries the default surface-1.
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 3)
	oldSafe := envRenderSafe
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = oldSafe })
	screen := ebiten.NewImage(1280, 720)
	rects := collectAllFills(t, func() { g.Draw(screen) })
	rack := g.drum.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatal("rack widget rect empty")
	}
	themed, stale := 0, 0
	for _, r := range rects {
		if r.Rect.Dx()*r.Rect.Dy() < 2000 || !r.Rect.Overlaps(rack) {
			continue
		}
		if r.Color == colRackSurface {
			themed++
		}
		if r.Color == genColorDefaultByName["surface-1"] && r.Rect.Dx() > rack.Dx()/2 {
			stale++
		}
	}
	if themed == 0 {
		t.Fatalf("rack column never painted with the themed bed color %v", colRackSurface)
	}
	if stale != 0 {
		t.Fatalf("%d large rack-column fill(s) still use the default surface-1 (rack mask / add-row strip not retargeted)", stale)
	}
	_ = image.Rect
}
