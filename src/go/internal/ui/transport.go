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

	boxAnim      float64
	bpmErrorAnim float64

	bpmInput string // typed digits while editing
	bpmPrev  int    // previous BPM before editing
}

func (t *Transport) SetBPM(b int) {
	if b < 1 {
		t.BPM = 1
		t.bpmErrorAnim = 1
		return
	}
	if b > maxBPM {
		t.BPM = maxBPM
		t.bpmErrorAnim = 1
		return
	}
	t.BPM = b
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
	prevFocus := t.focusBox

	if isMouseButtonPressed(ebiten.MouseButtonLeft) {
		if t.boxRect.Min.X <= x && x <= t.boxRect.Max.X &&
			t.boxRect.Min.Y <= y && y <= t.boxRect.Max.Y {
			t.focusBox = true
			t.boxAnim = 1
			t.bpmPrev = t.BPM
			t.bpmInput = ""
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
			if _, err := strconv.Atoi(string(ch)); err == nil {
				t.bpmInput += string(ch)
			}
		}
		if isKeyPressed(ebiten.KeyBackspace) {
			if l := len(t.bpmInput); l > 0 {
				t.bpmInput = t.bpmInput[:l-1]
			}
		}
		if isKeyPressed(ebiten.KeyEnter) {
			t.focusBox = false
		}
		if t.bpmInput != "" {
			if v, err := strconv.Atoi(t.bpmInput); err == nil {
				t.BPM = v
			}
		} else {
			t.BPM = t.bpmPrev
		}
	}

	if !t.focusBox && prevFocus {
		if t.bpmInput == "" {
			t.BPM = t.bpmPrev
		}
		t.SetBPM(t.BPM)
		t.bpmInput = ""
	}

	// decay animations
	t.boxAnim *= 0.85
	if t.boxAnim < 0.01 {
		t.boxAnim = 0
	}
	t.bpmErrorAnim *= 0.85
	if t.bpmErrorAnim < 0.01 {
		t.bpmErrorAnim = 0
	}
}

func (t *Transport) Draw(dst *ebiten.Image) {
	// background bar
	bar := ebiten.NewImage(dst.Bounds().Dx(), 40)
	bar.Fill(color.RGBA{15, 15, 15, 255})
	dst.DrawImage(bar, nil)

	// BPM label
	ebitenutil.DebugPrintAt(dst, "BPM:", 10, 12)

	BPMBoxStyle.DrawAnimated(dst, t.boxRect, t.focusBox, t.boxAnim)
	if t.bpmErrorAnim > 0 {
		drawRect(dst, t.boxRect, fadeColor(colError, t.bpmErrorAnim), false)
	}
	text := t.bpmInput
	if !t.focusBox {
		text = fmt.Sprintf("%d", t.BPM)
	}
	ebitenutil.DebugPrintAt(dst,
		text,
		t.boxRect.Min.X+4, t.boxRect.Min.Y+4)

	// play / stop squares
	drawRect(dst, t.playRect, color.White, t.Playing)
	ebitenutil.DebugPrintAt(dst, "▶", t.playRect.Min.X+6, t.playRect.Min.Y+3)

	drawRect(dst, t.stopRect, color.White, !t.Playing)
	ebitenutil.DebugPrintAt(dst, "■", t.stopRect.Min.X+6, t.stopRect.Min.Y+3)
}
