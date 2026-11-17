//go:build test

package ui

import (
	"image"
	"math"
	"testing"
)

// TestIconStrokeWeightWithinSpec asserts the proportional stroke calc
// returns the spec-defined logical-unit weight scaled by render size for
// every standard icon dimension. This replaces the pre-vector tests that
// intercepted drawRect — the new renderer uses ebiten/v2/vector and
// drawRect is no longer in the icon path.
func TestIconStrokeWeightWithinSpec(t *testing.T) {
	for _, dim := range []int{16, 20, 24, 32, 44} {
		got := iconStrokeWidth(image.Rect(0, 0, dim, dim))
		want := IconStrokeWeight * float32(dim) / float32(IconGrid)
		if want < 1 {
			want = 1
		}
		if math.Abs(float64(got-want)) > 0.01 {
			t.Errorf("stroke at %dx%d = %.3f, want %.3f", dim, dim, got, want)
		}
	}
}

// TestIconStrokeMinimumFloor pins the floor: at 8px (well below the
// 16-px minimum) the proportional weight is 0.583, but the floor must
// keep stroke ≥ 1 px so sub-pixel renders don't vanish.
func TestIconStrokeMinimumFloor(t *testing.T) {
	for _, dim := range []int{4, 8, 12} {
		got := iconStrokeWidth(image.Rect(0, 0, dim, dim))
		if got < 1.0 {
			t.Errorf("stroke at %dx%d = %.3f, want ≥ 1.0 (sub-pixel floor)", dim, dim, got)
		}
	}
}
