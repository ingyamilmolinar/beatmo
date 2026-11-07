//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func lumOf(c color.Color) int {
	r, g, b, _ := c.RGBA()
	return int(r>>8) + int(g>>8) + int(b>>8)
}

// One filled rounded rect at the full control rect: no stroke, wall inset or shadow.
func TestFlatButtonBodyIsSingleFilledRect(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	r := image.Rect(10, 10, 90, 40)
	calls := captureRoundedRectCalls(t, func() {
		drawFlatButtonBody(dst, r, colSurface2, RadiusSM)
	})
	if len(calls) != 1 {
		t.Fatalf("flat body emitted %d rounded-rect calls, want exactly 1 (no border stroke)", len(calls))
	}
	if !calls[0].Filled {
		t.Fatal("flat body must be filled, not stroked")
	}
	if calls[0].Rect != r {
		t.Fatalf("flat body rect = %v, want %v (no keycap wall inset)", calls[0].Rect, r)
	}
}

// Which makes button-quiet free at rest.
func TestFlatButtonBodyTransparentPaintsNothing(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	calls := captureRoundedRectCalls(t, func() {
		drawFlatButtonBody(dst, image.Rect(10, 10, 90, 40), color.Transparent, RadiusSM)
	})
	if len(calls) != 0 {
		t.Fatalf("transparent fill emitted %d calls, want 0", len(calls))
	}
}

// flatSelectionBarPx tall, flush with the bottom edge.
func TestFlatSelectionBarBottomEdge(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	r := image.Rect(10, 10, 110, 40)
	rects, _ := captureDrawRects(t, func() {
		drawFlatSelectionBar(dst, r, colAccent, selectionEdgeBottom, 1.0)
	})
	if len(rects) != 1 {
		t.Fatalf("selection bar emitted %d rects, want 1", len(rects))
	}
	got := rects[0]
	if got.Dy() != flatSelectionBarPx {
		t.Fatalf("bar height = %d, want %d", got.Dy(), flatSelectionBarPx)
	}
	if got.Max.Y != r.Max.Y {
		t.Fatalf("bar bottom = %d, want %d (flush with control bottom)", got.Max.Y, r.Max.Y)
	}
	if got.Dx() != r.Dx() {
		t.Fatalf("bar width at frac=1 = %d, want full %d", got.Dx(), r.Dx())
	}
}

// A partial bar grows from the centre, so a hover fade reads as the indicator emerging.
func TestFlatSelectionBarFractionalWidthCentered(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	r := image.Rect(10, 10, 110, 40) // 100 wide
	rects, _ := captureDrawRects(t, func() {
		drawFlatSelectionBar(dst, r, colAccent, selectionEdgeBottom, 0.5)
	})
	if len(rects) != 1 {
		t.Fatalf("emitted %d rects, want 1", len(rects))
	}
	got := rects[0]
	if got.Dx() != 50 {
		t.Fatalf("bar width at frac=0.5 = %d, want 50", got.Dx())
	}
	if cx, want := (got.Min.X+got.Max.X)/2, (r.Min.X+r.Max.X)/2; cx != want {
		t.Fatalf("bar center = %d, want %d (must grow from center)", cx, want)
	}
}

// The left-edge variant is the vertical-list stripe: fixed width, full height.
func TestFlatSelectionBarLeftEdge(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	r := image.Rect(10, 10, 110, 40)
	rects, _ := captureDrawRects(t, func() {
		drawFlatSelectionBar(dst, r, colAccent, selectionEdgeLeft, 1.0)
	})
	if len(rects) != 1 {
		t.Fatalf("emitted %d rects, want 1", len(rects))
	}
	got := rects[0]
	if got.Dx() != flatSelectionBarPx {
		t.Fatalf("left bar width = %d, want %d", got.Dx(), flatSelectionBarPx)
	}
	if got.Min.X != r.Min.X {
		t.Fatalf("left bar x = %d, want %d", got.Min.X, r.Min.X)
	}
	if got.Dy() != r.Dy() {
		t.Fatalf("left bar height = %d, want full %d", got.Dy(), r.Dy())
	}
}

// Zero-idle-work contract: an unselected control's bar costs no draw call.
func TestFlatSelectionBarZeroFracDrawsNothing(t *testing.T) {
	dst := ebiten.NewImage(200, 200)
	rects, _ := captureDrawRects(t, func() {
		drawFlatSelectionBar(dst, image.Rect(10, 10, 110, 40), colAccent, selectionEdgeBottom, 0)
	})
	if len(rects) != 0 {
		t.Fatalf("frac=0 emitted %d rects, want 0 (zero idle work)", len(rects))
	}
}

// Rest unchanged, hover lifts, press dims, disabled collapses to the container surface.
func TestFlatStateFillDeltas(t *testing.T) {
	base := colSurface2
	if got := flatStateFill(base, ComponentState{}); lumOf(got) != lumOf(base) {
		t.Fatalf("rest fill = %v, want base %v unchanged", got, base)
	}
	hover := flatStateFill(base, ComponentState{Hovered: true})
	if lumOf(hover) <= lumOf(base) {
		t.Fatalf("hover fill must be lighter than rest (%v vs %v)", hover, base)
	}
	press := flatStateFill(base, ComponentState{Pressed: true})
	if lumOf(press) >= lumOf(base) {
		t.Fatalf("press fill must be darker than rest (%v vs %v)", press, base)
	}
	dis := flatStateFill(base, ComponentState{Disabled: true})
	if lumOf(dis) != lumOf(colSurface1) {
		t.Fatalf("disabled fill = %v, want surface-1 %v", dis, colSurface1)
	}
}
