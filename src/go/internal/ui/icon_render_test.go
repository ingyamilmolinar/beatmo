package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestPlayAndStopIconsRender(t *testing.T) {
	assertDefaultParityState(t)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, testLogger)
	dv.recalcButtons()

	origPlay := drawPlayIcon
	origStop := drawStopIcon
	calledPlay := 0
	calledStop := 0
	drawPlayIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
		calledPlay++
	}
	drawStopIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
		calledStop++
	}
	defer func() {
		drawPlayIcon = origPlay
		drawStopIcon = origStop
	}()

	buttons := []*Button{dv.playBtn(), dv.stopBtn()}
	for _, b := range buttons {
		r := b.Rect()
		img := ebiten.NewImage(r.Dx(), r.Dy())
		old := b.r
		b.r = image.Rect(0, 0, r.Dx(), r.Dy())
		b.Draw(img)
		b.r = old
	}
	if calledPlay == 0 {
		t.Fatalf("play icon draw not invoked")
	}
	if calledStop == 0 {
		t.Fatalf("stop icon draw not invoked")
	}
}

func TestPencilIconRenders(t *testing.T) {
	assertDefaultParityState(t)
	orig := drawPencilIcon
	called := 0
	drawPencilIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
		called++
	}
	defer func() { drawPencilIcon = orig }()

	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.rowEditBtns()) == 0 {
		t.Fatalf("no row edit buttons")
	}
	b := dv.rowEditBtns()[0]
	// Edit button is hidden on desktop (empty rect); give it a test rect
	// so we can verify the pencil icon draws correctly.
	testRect := image.Rect(0, 0, 24, 24)
	img := ebiten.NewImage(testRect.Dx(), testRect.Dy())
	old := b.r
	b.r = testRect
	b.Draw(img)
	b.r = old
	if called == 0 {
		t.Fatalf("pencil icon draw not invoked")
	}
}

func TestSaveIconRenders(t *testing.T) {
	assertDefaultParityState(t)
	orig := drawSaveIcon
	called := 0
	drawSaveIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
		called++
	}
	defer func() { drawSaveIcon = orig }()

	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.rowSaveBtns()) == 0 {
		t.Fatalf("no row save buttons")
	}
	b := dv.rowSaveBtns()[0]
	// Save button is hidden on desktop (empty rect); give it a test rect
	// so we can verify the save icon draws correctly.
	testRect := image.Rect(0, 0, 24, 24)
	img := ebiten.NewImage(testRect.Dx(), testRect.Dy())
	old := b.r
	b.r = testRect
	b.Draw(img)
	b.r = old
	if called == 0 {
		t.Fatalf("save icon draw not invoked")
	}
}
