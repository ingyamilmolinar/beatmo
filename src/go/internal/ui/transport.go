package ui

import (
	"fmt"
	"image"
	"image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

type Transport struct {
	BPM     int
	Playing bool

	boxRect  image.Rectangle // bpm input box coords
	playRect image.Rectangle
	stopRect image.Rectangle
	focusBox bool
}

func NewTransport(w int) *Transport {
	return &Transport{
		BPM:      120,
		boxRect:  image.Rect(50, 8, 120, 30),
		playRect: image.Rect(140, 8, 170, 30),
		stopRect: image.Rect(180, 8, 210, 30),
	}
}

func (t *Transport) Update() {
	x, y := cursorPosition()

	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		if t.boxRect.Min.X <= x && x <= t.boxRect.Max.X &&
			t.boxRect.Min.Y <= y && y <= t.boxRect.Max.Y {
			t.focusBox = true
		} else {
			t.focusBox = false
		}
		if pt(x, y, t.playRect) {
			t.Playing = true
		}
		if pt(x, y, t.stopRect) {
			t.Playing = false
		}
	}

	if t.focusBox {
		if ch := inputChars(); len(ch) > 0 {
			// simplistic numeric entry
			if d, err := strconv.Atoi(string(ch)); err == nil {
				newBpm := t.BPM*10 + d
				if newBpm <= 300 {
					t.BPM = newBpm
				}
			}
		}
		if isKeyPressed(ebiten.KeyBackspace) {
			t.BPM /= 10
		}
	}
}

func (t *Transport) Draw(dst *ebiten.Image) {
	// background bar
	bar := ebiten.NewImage(dst.Bounds().Dx(), 40)
	bar.Fill(color.RGBA{15, 15, 15, 255})
	dst.DrawImage(bar, nil)

	// BPM label
	ebitenutil.DebugPrintAt(dst, "BPM:", 10, 12)

	// box outline
	drawRect(dst, t.boxRect, color.White, false)
	ebitenutil.DebugPrintAt(dst,
		fmt.Sprintf("%d", t.BPM),
		t.boxRect.Min.X+4, t.boxRect.Min.Y+4)

	// play / stop squares
	drawRect(dst, t.playRect, color.White, t.Playing)
	ebitenutil.DebugPrintAt(dst, "▶", t.playRect.Min.X+6, t.playRect.Min.Y+3)

	drawRect(dst, t.stopRect, color.White, !t.Playing)
	ebitenutil.DebugPrintAt(dst, "■", t.stopRect.Min.X+6, t.stopRect.Min.Y+3)
}
