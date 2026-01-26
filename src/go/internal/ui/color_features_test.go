package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestUniqueColorsAcrossRows(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testDiscard{}, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, timelineHeight+4*24), graph, logger)

	// Ensure at least two rows with the same instrument ID
	dv.AddRow()
	dv.AddRow()
	if len(dv.Rows) < 3 {
		t.Fatalf("expected >=3 rows, got %d", len(dv.Rows))
	}
	id := dv.Rows[0].Instrument
	if len(dv.rowLabels) < 3 {
		t.Fatalf("expected row labels for selection")
	}
	dv.rowLabels[1].OnClick()
	dv.SetInstrument(id)
	dv.rowLabels[2].OnClick()
	dv.SetInstrument(id)

	// Colors must be unique across rows
	keys := map[string]bool{}
	for i, r := range dv.Rows {
		k := dv.colorKey(r.Color)
		if keys[k] {
			t.Fatalf("duplicate color across rows at %d: %s", i, k)
		}
		keys[k] = true
	}
}

func TestColorButtonLayoutAndMenu(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 500, timelineHeight+3*24), graph, testLogger)
	dv.calcLayout()

	if len(dv.rowColorBtns) == 0 {
		t.Fatalf("missing rowColorBtns")
	}
	// Button order: edit < save < color < slider
	er := dv.rowEditBtns[0].Rect()
	sv := dv.rowSaveBtns[0].Rect()
	cr := dv.rowColorBtns[0].Rect()
	sr := dv.rowVolSliders[0].Rect()
	if !(er.Max.X <= sv.Min.X && sv.Max.X <= cr.Min.X && cr.Max.X <= sr.Min.X) {
		t.Fatalf("button order invalid: edit=%v save=%v color=%v slider=%v", er, sv, cr, sr)
	}

	// Open color menu
	dv.rowColorBtns[0].OnClick()
	dv.Update()
	if !dv.colorMenuOpen {
		t.Fatalf("color menu not open")
	}
	// Color wheel rectangle should be inside drum view bounds
	w := dv.colorWheelRect
	if w.Empty() || w.Min.Y < dv.Bounds.Min.Y || w.Max.Y > dv.Bounds.Max.Y {
		t.Fatalf("color wheel rect out of bounds: %v vs %v", w, dv.Bounds)
	}
}

func TestSetRowColorRejectsDuplicate(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, timelineHeight+3*24), graph, testLogger)
	dv.AddRow()
	if len(dv.Rows) < 2 {
		t.Fatalf("need at least two rows")
	}
	// Force row 0 to try picking row 1's color; it should be remapped.
	dup := dv.Rows[1].Color
	dv.SetRowColor(0, dup)
	if dv.colorKey(dv.Rows[0].Color) == dv.colorKey(dup) {
		t.Fatalf("duplicate color allowed for different rows")
	}
}

func TestRowColorSwatchPerRowAndUpdate(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 500, timelineHeight+4*24), graph, testLogger)
	dv.AddRow()
	dv.AddRow()
	dv.calcLayout()
	// Assign distinct colors and verify swatches draw them.
	cols := []color.Color{
		color.RGBA{50, 10, 10, 255},
		color.RGBA{10, 50, 10, 255},
		color.RGBA{10, 10, 50, 255},
	}
	for i := range dv.Rows {
		dv.SetRowColor(i, cols[i])
	}
	// Capture fill colors used by swatch buttons on draw.
	img := ebiten.NewImage(10, 10)
	type call struct{ fill color.Color }
	var fills []call
	orig := drawButton
	drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
		fills = append(fills, call{fill: fill})
	}
	defer func() { drawButton = orig }()
	// Draw each swatch directly and verify its color matches the row color.
	for i := range dv.Rows {
		fills = nil
		dv.rowColorBtns[i].Draw(img)
		if len(fills) == 0 {
			t.Fatalf("no draw captured for row %d swatch", i)
		}
		got := color.RGBAModel.Convert(fills[len(fills)-1].fill).(color.RGBA)
		want := color.RGBAModel.Convert(dv.Rows[i].Color).(color.RGBA)
		if got != want {
			t.Fatalf("row %d swatch color=%v want %v", i, got, want)
		}
	}
	// Change row 1 color and ensure swatch updates accordingly.
	newC := color.RGBA{200, 30, 40, 255}
	dv.SetRowColor(1, newC)
	fills = nil
	dv.rowColorBtns[1].Draw(img)
	if len(fills) == 0 {
		t.Fatalf("no draw captured after update")
	}
	got := color.RGBAModel.Convert(fills[len(fills)-1].fill).(color.RGBA)
	want := color.RGBAModel.Convert(dv.Rows[1].Color).(color.RGBA)
	if got != want {
		t.Fatalf("updated swatch color=%v want %v", got, want)
	}
}

func TestColorWheelClickSetsColor(t *testing.T) {
	assertDefaultParityState(t)
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 500, timelineHeight+3*24), graph, testLogger)
	dv.calcLayout()
	before := dv.colorKey(dv.Rows[0].Color)
	dv.rowColorBtns[0].OnClick()
	if !dv.colorMenuOpen {
		t.Fatalf("menu not open")
	}
	// Click near right edge of wheel
	r := dv.colorWheelRect
	x := (r.Min.X+r.Max.X)/2 + r.Dx()/3
	y := (r.Min.Y + r.Max.Y) / 2
	// Release to clear hold, then press to pick
	restoreUp := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 500, timelineHeight + 3*24 },
	)
	t.Cleanup(restoreUp)
	dv.Update()
	restoreUp()
	restoreDown := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 500, timelineHeight + 3*24 },
	)
	t.Cleanup(restoreDown)
	dv.Update()
	restoreDown()
	after := dv.colorKey(dv.Rows[0].Color)
	if after == before {
		t.Fatalf("wheel click did not change color: %s", after)
	}
}

// testDiscard implements io.Writer to silence logs in tests that don't import io.
type testDiscard struct{}

func (testDiscard) Write(p []byte) (int, error) { return len(p), nil }

// Ensure ebiten package is referenced so stubs link.
var _ = ebiten.KeyEnter
