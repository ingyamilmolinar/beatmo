package ui

import (
	"fmt"
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
)

type Transport struct {
	BPM     int
	Playing bool

	bpmBox   *TextInput
	playRect image.Rectangle
	stopRect image.Rectangle

	bpmErrorAnim float64
	bpmPrev      int

	// cached top bar background to avoid per-frame allocations
	barCache *ebiten.Image
	barW     int
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
			if _, ok := parseBPM(txt); !ok {
				t.bpmErrorAnim = 1
			}
		}
	} else if prev {
		txt := t.bpmBox.Value()
		if txt == "" {
			prevVal := t.bpmPrev
			if prevVal < 1 {
				prevVal = t.BPM
			}
			t.SetBPM(prevVal)
		} else if v, ok := parseBPM(txt); ok {
			t.SetBPM(v)
		} else {
			t.bpmErrorAnim = 1
			prevVal := t.bpmPrev
			if prevVal < 1 {
				prevVal = t.BPM
			}
			t.SetBPM(prevVal)
		}
		t.bpmBox.SetText(fmt.Sprintf("%d", t.BPM))
	}

	t.bpmErrorAnim *= 0.85
	if t.bpmErrorAnim < 0.01 {
		t.bpmErrorAnim = 0
	}
}

func (t *Transport) Draw(dst *ebiten.Image) {
	// background bar (cached)
	w := dst.Bounds().Dx()
	const h = 40
	if t.barCache == nil || t.barW != w {
		releaseImage(t.barCache)
		t.barCache = newTrackedImage("transport.barCache", w, h)
		t.barCache.Fill(genColorTransportBarBg)
		t.barW = w
	}
	dst.DrawImage(t.barCache, nil)

	// BPM label
	DrawTextAt(dst, "BPM:", 10, 12)

	t.bpmBox.Draw(dst)
	if t.bpmErrorAnim > 0 {
		drawRect(dst, t.bpmBox.Rect, fadeColor(colError, t.bpmErrorAnim), false)
	}

	// play / stop squares — DESIGN.md §0/§5: never raw Unicode; use IconID.
	drawRect(dst, t.playRect, color.White, t.Playing)
	DrawIcon(dst, IconPlay, t.playRect, colTextPrimary)

	drawRect(dst, t.stopRect, color.White, !t.Playing)
	DrawIcon(dst, IconStop, t.stopRect, colTextPrimary)
}
