package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestWheelDrawRendersCenterAndTicks(t *testing.T) {
	rec := installDrawRectInterceptor(t)

	def := audio.ParamDef{Name: "cutoff", Min: 0, Max: 200, Unit: "Hz"}
	k := &Knob{Endless: true, StepMul: 1.0}
	k.Scale = KnobScale{Min: def.Min, Max: def.Max, Unit: def.Unit}
	k.Value = 0.5
	b := WheelBinding{Knob: k, Def: def, OnChange: func() {}, OnCommit: func() {}, Title: func() string { return "Cutoff" }}
	w := NewMobileWheelPopup()
	w.Open(b, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 400, 800), 0)

	dst := ebiten.NewImage(400, 800)
	w.Draw(dst)

	rects := rec.snapshot()
	if !anyRectInside(rects, w.centerH) {
		t.Fatalf("center value box not drawn inside %v; got %d rects", w.centerH, len(rects))
	}
	// panel + center box + several tick separators
	inside := 0
	for _, r := range rects {
		if r.Overlaps(w.valRect) {
			inside++
		}
	}
	if inside < 4 {
		t.Fatalf("expected panel/center/>=several tick rects in valRect, got %d (total %d)", inside, len(rects))
	}
}

func TestWheelDrawNoopWhenClosed(t *testing.T) {
	rec := installDrawRectInterceptor(t)
	w := NewMobileWheelPopup()
	w.Draw(ebiten.NewImage(10, 10)) // not open
	if len(rec.snapshot()) != 0 {
		t.Fatalf("closed wheel must draw nothing, got %d rects", len(rec.snapshot()))
	}
}
