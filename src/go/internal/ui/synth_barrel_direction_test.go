//go:build test

package ui

import (
	"image"
	"strconv"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// labeledBarrelBinding labels each detent offset distinctly ("L-1", "L0", "L1", …)
// so a render test can tell exactly which row landed where. It never clamps, so
// every VisibleRows slot draws.
type labeledBarrelBinding struct{}

func (labeledBarrelBinding) labelAt(offset int) string { return "L" + strconv.Itoa(offset) }
func (labeledBarrelBinding) stepBy(int) bool           { return true }
func (labeledBarrelBinding) canStep(int) bool          { return true }

// drawFakeLaneLabelYs renders one lane of labeledBarrelBinding and returns the draw
// Y of every label keyed by its text.
func drawFakeLaneLabelYs(t *testing.T, w *BarrelWheel, rect image.Rectangle) map[string]int {
	t.Helper()
	ys := map[string]int{}
	orig := drawTextStyledPosHook
	drawTextStyledPosHook = func(s string, _, y int) { ys[s] = y }
	t.Cleanup(func() { drawTextStyledPosHook = orig })

	dst := ebiten.NewImage(rect.Dx(), rect.Dy())
	drawBarrelLane(dst, rect, w, labeledBarrelBinding{}, barrelLaneStyle{
		RowH: 26, VisibleRows: 5, Accent: colConceptStroke, Emphasis: true,
	})
	return ys
}

// TestBarrelLaneDrawsLargerValuesAbove pins the wheel's orientation: labelAt(+1)
// is the LARGER value (and the coarser resolution rung), and it must render
// ABOVE the center label, with labelAt(-1) below it. Fails if drawBarrelLane
// goes back to placing row i at centerY + i*rowH.
func TestBarrelLaneDrawsLargerValuesAbove(t *testing.T) {
	w := NewBarrelWheel()
	w.Bind(labeledBarrelBinding{}, 26)
	ys := drawFakeLaneLabelYs(t, w, image.Rect(0, 0, 120, 200))

	for _, k := range []string{"L-1", "L0", "L1"} {
		if _, ok := ys[k]; !ok {
			t.Fatalf("label %q was not drawn; drawn=%v", k, ys)
		}
	}
	if ys["L1"] >= ys["L0"] {
		t.Errorf("larger value must draw ABOVE center: y(L1)=%d, y(L0)=%d", ys["L1"], ys["L0"])
	}
	if ys["L0"] >= ys["L-1"] {
		t.Errorf("smaller value must draw BELOW center: y(L0)=%d, y(L-1)=%d", ys["L0"], ys["L-1"])
	}
}

// TestBarrelLaneStackFollowsPointer pins the gesture contract: a downward drag
// accumulates a POSITIVE offsetPx (BarrelWheel.DragTo adds the raw pointer dy),
// and the label stack must translate DOWN by that same amount — the wheel moves
// with the finger. Fails if drawBarrelLane subtracts the offset.
func TestBarrelLaneStackFollowsPointer(t *testing.T) {
	rect := image.Rect(0, 0, 120, 200)

	w := NewBarrelWheel()
	w.Bind(labeledBarrelBinding{}, 26)
	before := drawFakeLaneLabelYs(t, w, rect)

	const dragDownPx = 10
	w.BeginDrag(0)
	w.DragTo(dragDownPx) // finger moves down by 10px, no detent crossed yet
	if got := w.OffsetPx(); got != dragDownPx {
		t.Fatalf("precondition: a %dpx downward drag should give offsetPx=%d, got %v",
			dragDownPx, dragDownPx, got)
	}
	after := drawFakeLaneLabelYs(t, w, rect)

	for label, y0 := range before {
		y1, ok := after[label]
		if !ok {
			t.Fatalf("label %q disappeared after the drag", label)
		}
		if y1-y0 != dragDownPx {
			t.Errorf("label %q must move DOWN with the finger: y %d -> %d (want +%d)",
				label, y0, y1, dragDownPx)
		}
	}
}
