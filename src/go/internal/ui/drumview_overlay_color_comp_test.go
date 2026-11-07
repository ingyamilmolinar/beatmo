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

// 7 families × 5 shades = 35 cells.
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

// Every family label fits inside the gutter, so none spills over the first swatch column.
func TestColorPickerFamilyLabelsFitGutter(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())
	comp.Open()

	gutter := comp.LabelGutterWidth()
	if gutter <= 0 {
		t.Fatalf("label gutter width must be positive, got %d", gutter)
	}

	labels := comp.FamilyDisplayLabels()
	if len(labels) == 0 {
		t.Fatal("expected family display labels")
	}
	for _, label := range labels {
		w := StyledTextWidth(label, RoleCaption)
		if w > gutter {
			t.Errorf("family label %q width %dpx overflows the %dpx left gutter — text collides with the swatches", label, w, gutter)
		}
	}
}

// Short names keep the gutter tight, leaving the width to the swatches.
func TestColorPickerFamilyLabelsAreShort(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())
	comp.Open()

	const maxLabelRunes = 6 // "Orange"/"Purple" are the longest we allow
	for _, label := range comp.FamilyDisplayLabels() {
		if n := len([]rune(label)); n > maxLabelRunes {
			t.Errorf("family label %q is %d runes; keep color names <= %d runes so the gutter stays narrow", label, n, maxLabelRunes)
		}
	}
}

// 35 per-shade names, not the 7 family labels.
func TestCanonicalColorNameForEverySwatch(t *testing.T) {
	seen := map[string]bool{}
	for _, s := range SuggestedInstrumentSwatches {
		n := canonicalColorName(s.Name)
		if len([]rune(n)) == 0 {
			t.Errorf("swatch %q has an empty canonical name", s.Name)
			continue
		}
		if n[0] == '#' {
			t.Errorf("swatch %q canonical name %q looks like hex; want a human color name", s.Name, n)
		}
		seen[n] = true
	}
	if len(seen) < 20 {
		t.Errorf("only %d distinct canonical color names across %d swatches — names must be per-shade specific, not just the family label",
			len(seen), len(SuggestedInstrumentSwatches))
	}
	// A new swatch without an explicit name entry trips this.
	for _, s := range SuggestedInstrumentSwatches {
		if _, ok := canonicalSwatchNames[s.Name]; !ok {
			t.Errorf("palette swatch %q has no canonicalSwatchNames entry — add its human color name", s.Name)
		}
	}
}

func TestColorPickerShadeHintPresent(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())
	comp.Open()

	hint := comp.ShadeHintRect()
	if hint.Empty() {
		t.Fatal("expected a non-empty shade-direction hint band on a roomy panel")
	}
	if !hint.In(comp.WheelRect()) {
		t.Fatalf("hint band %v not inside panel %v", hint, comp.WheelRect())
	}
	// The band must sit ABOVE the first swatch row (it captions the columns).
	cells := comp.SwatchCells()
	if len(cells) == 0 {
		t.Fatal("no swatch cells")
	}
	if hint.Max.Y > cells[0].Min.Y {
		t.Errorf("hint band bottom %d overlaps first cell top %d — band must be above the swatches", hint.Max.Y, cells[0].Min.Y)
	}
}

func TestColorPickerPanelIsUsableSize(t *testing.T) {
	comp := NewColorWheelComponent()
	comp.SetProps(stdColorProps())
	comp.Open()

	r := comp.WheelRect()
	if r.Dx() < 200 || r.Dy() < 80 {
		t.Fatalf("picker panel too small to be usable: %v", r)
	}
}

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

	// A pick applies and leaves the picker open; the gesture owns input until release.
	if !comp.IsOpen() {
		t.Error("expected picker to remain open until release")
	}
	if !comp.Capturing() {
		t.Error("expected Capturing() true after pick-press")
	}
	res = comp.HandleInput(cx, cy, false)
	if res != InputConsumed {
		t.Errorf("expected InputConsumed on release, got %v", res)
	}
	if !comp.IsOpen() {
		t.Error("expected picker to stay open after applying a colour")
	}

	// Re-selecting the applied colour confirms: a deferred close on release.
	comp.HandleInput(cx, cy, true)
	if !comp.IsOpen() {
		t.Error("expected picker open while the confirm press is held")
	}
	comp.HandleInput(cx, cy, false)
	if comp.IsOpen() {
		t.Error("expected picker closed after confirming the applied colour")
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

// RGB compare.
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

// The mobile bottom sheet is the worst-case clamp.
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

	// Positive-area overlap would mean the grid collapsed; edge-touching is fine.
	for a := 0; a < len(cells); a++ {
		for b := a + 1; b < len(cells); b++ {
			x := cells[a].Intersect(cells[b])
			if !x.Empty() && x.Dx() > 0 && x.Dy() > 0 {
				t.Fatalf("mobile cells %d %v and %d %v overlap: %v", a, cells[a], b, cells[b], x)
			}
		}
	}
}
