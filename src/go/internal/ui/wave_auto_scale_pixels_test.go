//go:build test

package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/analyzer"
)

// maxWaveTraceRectHeight returns the tallest single colWaveTrace column rect
// intersecting `inside`. drawWaveTrace emits one such rect per pixel column,
// its height proportional to the displayed amplitude — so a louder *displayed*
// signal (higher gain) yields taller columns.
func maxWaveTraceRectHeight(rects []drawnRect, inside image.Rectangle) int {
	want := color.RGBAModel.Convert(colWaveTrace).(color.RGBA)
	maxH := 0
	for _, r := range rects {
		if r.Color != want {
			continue
		}
		if r.Rect.Intersect(inside).Empty() {
			continue
		}
		if h := r.Rect.Dy(); h > maxH {
			maxH = h
		}
	}
	return maxH
}

// TestWaveAutoGainMakesQuietSignalTaller proves the adaptive Y-scale actually
// magnifies a quiet signal: the same 0.1-amplitude sine renders a strictly
// taller trace at auto-gain (peak 0.1 -> ~9x) than at unity gain.
func TestWaveAutoGainMakesQuietSignalTaller(t *testing.T) {
	t.Cleanup(SetWaveTraceCacheForTest(false))

	const n = 256
	wave := make([]float64, n)
	for i := range wave {
		wave[i] = 0.1 * math.Sin(2*math.Pi*float64(i)/32.0) // peak 0.1
	}
	ch := &analyzer.ChannelMetrics{Active: true, Waveform: wave}

	rect := image.Rect(0, 0, 200, 120)

	flatImg := ebiten.NewImage(200, 120)
	flatRects := collectFilledRects(t, func() {
		drawAnalyzerWaveform(flatImg, rect, ch, nil, nil, 1.0, false, false)
	})

	gain := autoGainForPeak(0.1) // 0.9/0.1 = 9.0
	autoImg := ebiten.NewImage(200, 120)
	autoRects := collectFilledRects(t, func() {
		drawAnalyzerWaveform(autoImg, rect, ch, nil, nil, gain, true, false)
	})

	flatH := maxWaveTraceRectHeight(flatRects, rect)
	autoH := maxWaveTraceRectHeight(autoRects, rect)

	if flatH <= 0 {
		t.Fatalf("flat trace produced no colWaveTrace rects (flatH=%d)", flatH)
	}
	if autoH <= flatH {
		t.Fatalf("auto-gain trace not taller than flat: autoH=%d flatH=%d (gain=%v)", autoH, flatH, gain)
	}
}
