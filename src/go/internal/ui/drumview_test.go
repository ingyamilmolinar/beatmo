package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func setupDV() *DrumView {
	dv := NewDrumView(image.Rect(0, 0, 200, 100))
	dv.Rows = []*DrumRow{{Name: "H", Steps: make([]bool, 4)}, {Name: "S", Steps: make([]bool, 4)}}
	dv.recalcButtons()
	return dv
}

func TestPlayStopButtons(t *testing.T) {
	dv := setupDV()
	mx := dv.playBtn.Min.X + 1
	my := dv.playBtn.Min.Y + 1
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	dv.Update()
	if !dv.playing {
		t.Fatal("expected playing after clicking play")
	}

	mx = dv.stopBtn.Min.X + 1
	my = dv.stopBtn.Min.Y + 1
	dv.Update()
	if dv.playing {
		t.Fatal("expected stopped after clicking stop")
	}
}

func TestRowHeightFillsPane(t *testing.T) {
	dv := setupDV()
	dv.Update()
	want := dv.Bounds.Dy() / len(dv.Rows)
	if dv.rowHeight() != want {
		t.Fatalf("expected row height %d, got %d", want, dv.rowHeight())
	}
}

func TestRowHeightSplit(t *testing.T) {
	dv := setupDV()
	dv.Update()
	h := dv.rowHeight()
	expected := dv.Bounds.Dy() / len(dv.Rows)
	if h != expected {
		t.Fatalf("expected row height %d, got %d", expected, h)
	}
}

func TestDrawAfterInit(t *testing.T) {
	dv := setupDV()
	dv.Update()
	img := ebiten.NewImage(200, 100)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Draw panicked: %v", r)
		}
	}()
	dv.Draw(img)
}

func TestSetBoundsRebuilds(t *testing.T) {
	dv := setupDV()
	dv.Update()
	h1 := dv.rowHeight()
	dv.SetBounds(image.Rect(0, 0, 200, 150))
	dv.Update()
	h2 := dv.rowHeight()
	if h2 <= h1 {
		t.Fatalf("expected height to increase from %d to %d", h1, h2)
	}
}
func TestBackgroundWidthMatchesBounds(t *testing.T) {
	dv := setupDV()
	dv.Update()
	for idx, img := range dv.bgCache {
		if img.Bounds().Dx() != dv.Bounds.Dx() {
			t.Fatalf("row %d width=%d want %d", idx, img.Bounds().Dx(), dv.Bounds.Dx())
		}
	}
	dv.resizeSteps(+1)
	dv.Update()
	for idx, img := range dv.bgCache {
		if img.Bounds().Dx() != dv.Bounds.Dx() {
			t.Fatalf("after resize row %d width=%d want %d", idx, img.Bounds().Dx(), dv.Bounds.Dx())
		}
	}
}

func TestRowHeightUnchangedAfterNode(t *testing.T) {
	g := New()
	g.Layout(200, 120)
	h1 := g.drum.rowHeight()
	g.tryAddNode(0, 0)
	g.Update()
	if g.drum.rowHeight() != h1 {
		t.Fatalf("row height changed from %d to %d", h1, g.drum.rowHeight())
	}
}
