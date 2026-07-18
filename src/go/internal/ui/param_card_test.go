//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestParamCardGaugeFor_Kinds(t *testing.T) {
	// Continuous → fill with frac == normalized value.
	k := &Knob{Value: 0.25}
	g := paramCardGaugeFor(k, audio.ParamDef{Min: 0, Max: 100})
	if g.kind != gaugeFill || g.frac != 0.25 {
		t.Fatalf("continuous: got kind=%v frac=%v", g.kind, g.frac)
	}
	// Bipolar → gaugeBipolar carrying the knob's zero fraction.
	kb := &Knob{Value: 0.75, Bipolar: true, ZeroFrac: 0.5}
	gb := paramCardGaugeFor(kb, audio.ParamDef{Min: -12, Max: 12})
	if gb.kind != gaugeBipolar || gb.zero != 0.5 || gb.frac != 0.75 {
		t.Fatalf("bipolar: got %+v", gb)
	}
	// Enum (<=8) → dots with the rounded index.
	ke := &Knob{Value: 1} // normalized 1.0 → last index
	def := audio.ParamDef{Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Tri"}}
	ge := paramCardGaugeFor(ke, def)
	if ge.kind != gaugeDots || ge.count != 4 || ge.idx != 3 {
		t.Fatalf("enum: got %+v", ge)
	}
	// Enum >8 → index-proportional fill, not dots.
	big := make([]string, 12)
	for i := range big {
		big[i] = "v"
	}
	gbig := paramCardGaugeFor(&Knob{Value: 0.5}, audio.ParamDef{Min: 0, Max: 11, Enum: big})
	if gbig.kind != gaugeFill {
		t.Fatalf("large enum should fall back to fill, got %+v", gbig)
	}
}

func TestParamCardDraw_RendersInkAndGauge(t *testing.T) {
	dst := ebiten.NewImage(200, 80)
	r := image.Rect(10, 10, 10+148, 10+56)
	drawParamCard(dst, r, "CUTOFF", "8.00 kHz",
		paramCardGauge{kind: gaugeFill, frac: 0.5}, paramCardState{})
	// Some ink must land inside the card rect.
	ink := 0
	for y := r.Min.Y; y < r.Max.Y; y += 2 {
		for x := r.Min.X; x < r.Max.X; x += 2 {
			if _, _, _, a := dst.At(x, y).RGBA(); a > 0 {
				ink++
			}
		}
	}
	if ink < 50 {
		t.Fatalf("card drew almost nothing: ink=%d", ink)
	}
	// Empty rect must be a no-op (no panic).
	drawParamCard(dst, image.Rectangle{}, "X", "1", paramCardGauge{}, paramCardState{})
}
