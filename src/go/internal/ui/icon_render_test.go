package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestPlayAndStopIconsRender(t *testing.T) {
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

	buttons := []*Button{dv.playBtn, dv.stopBtn}
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
	orig := drawPencilIcon
	called := 0
	drawPencilIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
		called++
	}
	defer func() { drawPencilIcon = orig }()

	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, testLogger)
	dv.recalcButtons()
	dv.calcLayout()
	if len(dv.rowEditBtns) == 0 {
		t.Fatalf("no row edit buttons")
	}
	b := dv.rowEditBtns[0]
	r := b.Rect()
	img := ebiten.NewImage(r.Dx(), r.Dy())
	old := b.r
	b.r = image.Rect(0, 0, r.Dx(), r.Dy())
	b.Draw(img)
	b.r = old
	if called == 0 {
		t.Fatalf("pencil icon draw not invoked")
	}
}
