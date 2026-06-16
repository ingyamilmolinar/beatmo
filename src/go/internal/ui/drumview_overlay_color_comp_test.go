//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"
)

// stdProps returns props with a roomy desktop container for grid tests.
func stdColorProps() ColorWheelProps {
	return ColorWheelProps{
		AnchorRect: image.Rect(100, 200, 130, 220),
		Bounds:     image.Rect(0, 0, 600, 600),
		RowHeight:  24,
	}
}

func TestColorWheelComponent_OpenClose(t *testing.T) {
	comp := NewColorWheelComponent()

	if comp.IsOpen() {
		t.Error("expected picker to be closed initially")
	}

	comp.SetProps(stdColorProps())
	comp.Open()

	if !comp.IsOpen() {
		t.Error("expected picker to be open after Open()")
	}
	if comp.Bounds().Empty() {
		t.Error("expected non-empty bounds after opening")
	}

	comp.Close()
	if comp.IsOpen() {
		t.Error("expected picker to be closed after Close()")
	}
}

func TestColorWheelComponent_NoHoldAfterOpen(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())
	comp.Open()

	if comp.Capturing() {
		t.Error("expected Capturing() to be false after opening — hold is not set")
	}
}

// TestColorSwatchGrid_LaysOutFullPalette verifies the full curated palette
// renders as a grid of cells, each holding a palette color.
func TestColorSwatchGrid_LaysOutFullPalette(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())
	comp.Open()

	cells := comp.SwatchCells()
	if len(cells) != len(SuggestedInstrumentSwatches) {
		t.Fatalf("expected %d swatch cells, got %d", len(SuggestedInstrumentSwatches), len(cells))
	}
	// Each cell must be a sensible touch-comfortable size.
	for i, r := range cells {
		if r.Dx() < 20 || r.Dy() < 20 {
			t.Fatalf("cell %d too small: %v", i, r)
		}
	}
}

// TestColorRampGridLayout verifies the 7-family × 5-shade ramp layout: 35 cells
// total, and the first and last cell rects are non-empty and inside the panel.
func TestColorRampGridLayout(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())
	comp.Open()

	cells := comp.SwatchCells()
	if len(cells) != 35 {
		t.Fatalf("expected 35 swatch cells (7 families × 5 shades), got %d", len(cells))
	}

	panel := comp.WheelRect()
	first := cells[0]
	last := cells[len(cells)-1]
	if first.Empty() {
		t.Fatalf("first cell rect is empty: %v", first)
	}
	if last.Empty() {
		t.Fatalf("last cell rect is empty: %v", last)
	}
	if !first.In(panel) {
		t.Fatalf("first cell %v not inside panel %v", first, panel)
	}
	if !last.In(panel) {
		t.Fatalf("last cell %v not inside panel %v", last, panel)
	}
	// Family labels must derive correctly (suffix stripped, Title-Cased).
	if got := familyLabelFromSwatchName("hot-pink-300"); got != "Hot Pink" {
		t.Errorf("familyLabelFromSwatchName(hot-pink-300) = %q, want Hot Pink", got)
	}
}

// TestColorPickerPanelIsUsableSize verifies the panel is a real, usable size —
// not the ~52px square the broken hue wheel collapsed to on desktop.
func TestColorPickerPanelIsUsableSize(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())
	comp.Open()

	r := comp.WheelRect()
	if r.Dx() < 200 || r.Dy() < 80 {
		t.Fatalf("picker panel too small to be usable: %v", r)
	}
}

// TestColorSwatchClickPicksExactPaletteColor verifies clicking a swatch cell
// invokes OnColorPick with that exact palette color.
func TestColorSwatchClickPicksExactPaletteColor(t *testing.T) {
	comp := NewColorWheelComponent()

	var picked color.Color
	p := stdColorProps()
	p.OnColorPick = func(c color.Color) { picked = c }
	comp.SetProps(p)
	comp.Open()

	cells := comp.SwatchCells()
	if len(cells) == 0 {
		t.Fatal("no swatch cells")
	}
	// Click the 5th swatch (sunset-gold-500) — center of its cell.
	idx := 4
	target := cells[idx]
	cx := target.Min.X + target.Dx()/2
	cy := target.Min.Y + target.Dy()/2

	res := comp.HandleInput(cx, cy, true)
	if res != InputCaptured {
		t.Errorf("expected InputCaptured on swatch press, got %v", res)
	}
	if picked == nil {
		t.Fatal("expected OnColorPick to fire")
	}
	want := SuggestedInstrumentSwatches[idx].RGBA
	gr, gg, gb, _ := picked.RGBA()
	if uint8(gr>>8) != want.R || uint8(gg>>8) != want.G || uint8(gb>>8) != want.B {
		t.Fatalf("picked color %v != palette swatch %v", picked, want)
	}

	// Deferred close: stays open while held, closes on release.
	if !comp.IsOpen() {
		t.Error("expected picker to remain open until release")
	}
	if !comp.Capturing() {
		t.Error("expected Capturing() true after pick-press")
	}
	res = comp.HandleInput(cx, cy, false)
	if res != InputConsumed {
		t.Errorf("expected InputConsumed on deferred close, got %v", res)
	}
	if comp.IsOpen() {
		t.Error("expected picker closed after release")
	}
}

func TestColorWheelComponent_DeferredClose(t *testing.T) {
	comp := NewColorWheelComponent()

	var closeCalled bool
	p := stdColorProps()
	p.OnColorPick = func(c color.Color) {}
	p.OnClose = func() { closeCalled = true }
	comp.SetProps(p)
	comp.Open()

	cells := comp.SwatchCells()
	target := cells[0]
	cx := target.Min.X + target.Dx()/2
	cy := target.Min.Y + target.Dy()/2

	comp.HandleInput(cx, cy, true)
	if !comp.IsOpen() {
		t.Fatal("expected picker to stay open after pick-press")
	}
	if !comp.Capturing() {
		t.Fatal("expected Capturing after pick-press")
	}
	if closeCalled {
		t.Fatal("OnClose should not fire until release")
	}

	if res := comp.HandleInput(cx, cy, true); res != InputCaptured {
		t.Errorf("expected InputCaptured while holding, got %v", res)
	}
	if res := comp.HandleInput(cx, cy, false); res != InputConsumed {
		t.Errorf("expected InputConsumed on release, got %v", res)
	}
	if comp.IsOpen() {
		t.Fatal("expected picker closed after release")
	}
	if !closeCalled {
		t.Fatal("expected OnClose to fire on release")
	}
}

func TestColorWheelComponent_ClickOutsideCloses(t *testing.T) {
	comp := NewColorWheelComponent()

	var closeCalled bool
	p := stdColorProps()
	p.Bounds = image.Rect(100, 100, 500, 500)
	p.AnchorRect = image.Rect(120, 120, 150, 140)
	p.OnClose = func() { closeCalled = true }
	comp.SetProps(p)
	comp.Open()

	// Click far outside the panel.
	res := comp.HandleInput(5, 5, true)
	if res != InputConsumed {
		t.Errorf("expected click outside to be consumed, got %v", res)
	}
	if !closeCalled {
		t.Error("expected OnClose to be called")
	}
	if comp.IsOpen() {
		t.Error("expected picker closed after click outside")
	}
}

// TestColorPickerStaysInsideBounds verifies the panel is clamped inside the
// container on all sides.
func TestColorPickerStaysInsideBounds(t *testing.T) {
	comp := NewColorWheelComponent()
	bounds := image.Rect(50, 100, 600, 500)
	p := stdColorProps()
	p.Bounds = bounds
	p.AnchorRect = image.Rect(560, 470, 590, 490) // near bottom-right corner
	comp.SetProps(p)
	comp.Open()

	r := comp.WheelRect()
	if r.Min.X < bounds.Min.X || r.Min.Y < bounds.Min.Y ||
		r.Max.X > bounds.Max.X || r.Max.Y > bounds.Max.Y {
		t.Fatalf("panel %v exceeds bounds %v", r, bounds)
	}
}

func TestColorWheelComponent_HandleInputWhenClosed(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())

	if res := comp.HandleInput(150, 150, true); res != InputIgnored {
		t.Error("expected input ignored when picker is closed")
	}
}

// TestColorSwatchPickOutsideGridAbsorbed verifies a press inside the panel but
// not on a swatch (e.g. header whitespace) is absorbed without picking.
func TestColorSwatchPickOutsideGridAbsorbed(t *testing.T) {
	comp := NewColorWheelComponent()

	var pickCalled bool
	p := stdColorProps()
	p.OnColorPick = func(c color.Color) { pickCalled = true }
	comp.SetProps(p)
	comp.Open()

	// Press in the header band (between title and close button — no swatch).
	r := comp.WheelRect()
	hx := r.Min.X + r.Dx()/2
	hy := r.Min.Y + 2
	res := comp.HandleInput(hx, hy, true)
	if res != InputConsumed {
		t.Errorf("expected InputConsumed for header press, got %v", res)
	}
	if pickCalled {
		t.Error("OnColorPick should NOT fire for a non-swatch press")
	}
	if !comp.IsOpen() {
		t.Error("picker should remain open after a non-swatch press")
	}
	if comp.Capturing() {
		t.Error("Capturing() should be false — no pick occurred")
	}
}

func TestColorWheelComponent_EmptyProps(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rectangle{}, // empty
		Bounds:     image.Rect(0, 0, 500, 500),
		RowHeight:  24,
	})
	comp.Open()

	if !comp.Bounds().Empty() {
		t.Error("expected empty bounds with empty anchor")
	}
	if len(comp.SwatchCells()) != 0 {
		t.Error("expected no cells with empty anchor")
	}
}

func TestColorWheelComponent_InputBounds(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())

	if !comp.InputBounds().Empty() {
		t.Error("expected empty input bounds before opening")
	}

	comp.Open()

	ib := comp.InputBounds()
	if ib.Empty() {
		t.Error("expected non-empty input bounds after opening")
	}
	if ib != comp.WheelRect() {
		t.Error("expected input bounds to match panel rect")
	}
}

// TestColorSwatchSelectedRing verifies the selected ring renders only when the
// current color matches a palette swatch (RGB compare).
func TestColorSwatchSelectedRing(t *testing.T) {
	// Matching color → colorsEqualRGB true for that swatch.
	want := SuggestedInstrumentSwatches[2].RGBA // coral
	if !colorsEqualRGB(want, want) {
		t.Error("expected exact palette color to match its swatch")
	}
	// Off-palette color → no match for any swatch.
	off := color.RGBA{1, 2, 3, 255}
	for _, s := range SuggestedInstrumentSwatches {
		if colorsEqualRGB(off, s.RGBA) {
			t.Fatalf("off-palette color unexpectedly matched swatch %v", s.RGBA)
		}
	}
	// Alpha is ignored in the compare.
	alphaVariant := color.RGBA{want.R, want.G, want.B, 128}
	if !colorsEqualRGB(alphaVariant, want) {
		t.Error("expected RGB compare to ignore alpha")
	}
	// nil current color → no ring.
	if colorsEqualRGB(nil, want) {
		t.Error("nil current color should not match any swatch")
	}
}

// TestColorPickerMobileTouchTargets verifies swatch cells meet the touch
// minimum on mobile.
func TestColorPickerMobileTouchTargets(t *testing.T) {
	forceSmallScreenForTest = true
	UpdateProfile()
	defer func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	}()

	comp := NewColorWheelComponent()
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(10, 600, 120, 640),
		Bounds:     image.Rect(0, 0, 400, 700),
		RowHeight:  24,
	})
	comp.Open()

	tm := TouchMinTarget()
	for i, r := range comp.SwatchCells() {
		if r.Dx() < tm || r.Dy() < tm {
			t.Fatalf("mobile cell %d below touch min (%d): %v", i, tm, r)
		}
	}
}

// TestColorRampGridLayoutMobile mirrors TestColorRampGridLayout but under the
// mobile bottom-sheet profile (the worst-case clamp): the full 7×5 ramp must
// still lay out as 35 non-empty, non-overlapping cells contained in the panel.
func TestColorRampGridLayoutMobile(t *testing.T) {
	forceSmallScreenForTest = true
	UpdateProfile()
	defer func() {
		forceSmallScreenForTest = false
		UpdateProfile()
	}()

	comp := NewColorWheelComponent()
	// Narrow portrait bottom-sheet container, anchored low like a real picker.
	comp.SetProps(ColorWheelProps{
		AnchorRect: image.Rect(10, 600, 120, 640),
		Bounds:     image.Rect(0, 0, 400, 700),
		RowHeight:  24,
	})
	comp.Open()

	cells := comp.SwatchCells()
	if len(cells) != 35 {
		t.Fatalf("expected 35 swatch cells (7 families × 5 shades) on mobile, got %d", len(cells))
	}

	panel := comp.WheelRect()
	for i, r := range cells {
		if r.Empty() {
			t.Fatalf("mobile cell %d rect is empty: %v", i, r)
		}
		if !r.In(panel) {
			t.Fatalf("mobile cell %d %v not inside panel %v", i, r, panel)
		}
	}

	// Cells must not overlap each other (would mean the grid collapsed under
	// the mobile clamp). Allow edge-touching; flag only positive-area overlap.
	for a := 0; a < len(cells); a++ {
		for b := a + 1; b < len(cells); b++ {
			x := cells[a].Intersect(cells[b])
			if !x.Empty() && x.Dx() > 0 && x.Dy() > 0 {
				t.Fatalf("mobile cells %d %v and %d %v overlap: %v", a, cells[a], b, cells[b], x)
			}
		}
	}
}
