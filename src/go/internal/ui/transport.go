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

	bpmBox   *TextInput
	playRect image.Rectangle
	stopRect image.Rectangle

        bpmErrorAnim float64
        bpmPrev      int
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
	_ = w
	r := image.Rect(50, 8, 120, 30)
	ti := NewTextInput(r, BPMBoxStyle)
	ti.SetText("120")
	return &Transport{
		BPM:      120,
		bpmBox:   ti,
		playRect: image.Rect(140, 8, 170, 30),
		stopRect: image.Rect(180, 8, 210, 30),
	}
}

func (t *Transport) Update() {
	x, y := cursorPosition()
        prev := t.bpmBox.Focused()
        t.bpmBox.Update()

        if isMouseButtonPressed(ebiten.MouseButtonLeft) {
                if pt(x, y, t.playRect) {
                        t.Playing = true
                }
                if pt(x, y, t.stopRect) {
                        t.Playing = false
                }
        }

        if !prev && t.bpmBox.Focused() {
                t.bpmPrev = t.BPM
                t.bpmBox.SetText("")
        }

        if t.bpmBox.Focused() {
                if txt := t.bpmBox.Value(); txt != "" {
                        if v, err := strconv.Atoi(txt); err != nil || v < 1 || v > maxBPM {
                                t.bpmErrorAnim = 1
                        }
                }
        } else if prev {
                txt := t.bpmBox.Value()
                if txt == "" {
                        t.SetBPM(t.bpmPrev)
                } else if v, err := strconv.Atoi(txt); err == nil && v >= 1 && v <= maxBPM {
                        t.SetBPM(v)
                } else {
                        t.bpmErrorAnim = 1
                        t.SetBPM(t.bpmPrev)
                }
                t.bpmBox.SetText(fmt.Sprintf("%d", t.BPM))
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

	t.bpmBox.Draw(dst)
	if t.bpmErrorAnim > 0 {
		drawRect(dst, t.bpmBox.Rect, fadeColor(colError, t.bpmErrorAnim), false)
	}

	// play / stop squares
	drawRect(dst, t.playRect, color.White, t.Playing)
	ebitenutil.DebugPrintAt(dst, "▶", t.playRect.Min.X+6, t.playRect.Min.Y+3)

	drawRect(dst, t.stopRect, color.White, !t.Playing)
	ebitenutil.DebugPrintAt(dst, "■", t.stopRect.Min.X+6, t.stopRect.Min.Y+3)
}
