//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestRecordPulse_NotDrawnWhenIdle verifies the record-armed halo does NOT
// render when recording is off.
func TestRecordPulse_NotDrawnWhenIdle(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.SetRecording(false)
	if g.drum.transportZone == nil {
		t.Skip("transport zone not built; record button unreachable")
	}
	rr := g.drum.transportZone.recordBtn.Rect()
	if rr.Empty() {
		t.Fatalf("record button rect empty after layout")
	}

	rects := captureOutlinesNear(t, rr, isRecordActiveColor, func() {
		g.drum.Draw(ebiten.NewImage(640, 480), nil, 0, nil, 0)
	})
	if got := len(rects); got != 0 {
		t.Fatalf("expected 0 record-armed halo passes when idle, got %d", got)
	}
}

// TestRecordPulse_DrawsMultiPassWhenArmed verifies that when isRecording is
// true the halo draws as a multi-pass falloff (≥2 passes) extending ≥3 px
// from the record button edge.
func TestRecordPulse_DrawsMultiPassWhenArmed(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	if g.drum.transportZone == nil {
		t.Skip("transport zone not built; record button unreachable")
	}
	g.drum.SetRecording(true)
	rr := g.drum.transportZone.recordBtn.Rect()
	if rr.Empty() {
		t.Fatalf("record button rect empty after layout")
	}

	rects := captureOutlinesNear(t, rr, isRecordActiveColor, func() {
		g.drum.Draw(ebiten.NewImage(640, 480), nil, 0, nil, 0)
	})
	if len(rects) < 2 {
		t.Fatalf("expected ≥2 halo passes when armed, got %d (record btn rect %v)", len(rects), rr)
	}
	maxOutset := 0
	for _, r := range rects {
		out := rr.Min.X - r.Rect.Min.X
		if out > maxOutset {
			maxOutset = out
		}
	}
	if maxOutset < 3 {
		t.Fatalf("expected halo to extend ≥3 px from button edge; max outset = %d px", maxOutset)
	}
}

// captureOutlinesNear is the colour-agnostic equivalent of the pulse-halo
// helper: it intercepts unfilled drawRect calls whose rect brackets
// btnRect and whose colour matches the predicate.
func captureOutlinesNear(t *testing.T, btnRect image.Rectangle, match func(color.Color) bool, fn func()) []drawnRect {
	t.Helper()
	var rects []drawnRect
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if !filled && match(c) &&
			r.Min.X <= btnRect.Min.X && r.Min.Y <= btnRect.Min.Y &&
			r.Max.X >= btnRect.Max.X && r.Max.Y >= btnRect.Max.Y {
			rects = append(rects, drawnRect{
				Rect:  r,
				Color: color.RGBAModel.Convert(c).(color.RGBA),
			})
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()
	fn()
	return rects
}

func isRecordActiveColor(c color.Color) bool {
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return n.R == genColorRecordActive.R && n.G == genColorRecordActive.G && n.B == genColorRecordActive.B
}
