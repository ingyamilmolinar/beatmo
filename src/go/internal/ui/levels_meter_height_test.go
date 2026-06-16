//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// countColoredSegments draws a segmented level bar of the given height with a
// loud signal and counts how many lit LED fills (green/yellow/red) land inside
// it. It uses the package's drawRect interceptor (collectFilledRects).
func countColoredSegments(t *testing.T, barDy int, db float64) int {
	t.Helper()
	dst := ebiten.NewImage(40, barDy+4)
	r := image.Rect(2, 2, 38, 2+barDy)
	lit := map[color.RGBA]bool{
		meterGreen:  true,
		meterYellow: true,
		meterRed:    true,
	}
	rects := collectFilledRects(t, func() {
		drawSegmentedLevelBar(dst, r, db, true)
	})
	var n int
	for _, dr := range rects {
		if dr.Rect.In(r) && dr.Rect != r && lit[dr.Color] {
			n++
		}
	}
	return n
}

// A short level bar (38px tall) must still render lit LED segments for a loud
// signal — regression guard for the audio-panel control-header height squeeze.
func TestSegmentedLevelBarRendersWhenShort(t *testing.T) {
	if got := countColoredSegments(t, 38, -3.0); got < 1 {
		t.Fatalf("short (38px) level bar drew %d lit segments, want ≥1", got)
	}
}

// A silent signal (≤ floor) must draw NO lit segments at any height.
func TestSegmentedLevelBarSilentDrawsNothing(t *testing.T) {
	if got := countColoredSegments(t, 38, -120.0); got != 0 {
		t.Fatalf("silent short bar drew %d lit segments, want 0", got)
	}
	if got := countColoredSegments(t, 120, -120.0); got != 0 {
		t.Fatalf("silent tall bar drew %d lit segments, want 0", got)
	}
}
