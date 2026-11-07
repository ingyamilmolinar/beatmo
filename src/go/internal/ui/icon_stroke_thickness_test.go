//go:build test

package ui

import (
	"image"
	"math"
	"testing"
)

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

// The floor keeps sub-pixel renders from vanishing.
func TestIconStrokeMinimumFloor(t *testing.T) {
	for _, dim := range []int{4, 8, 12} {
		got := iconStrokeWidth(image.Rect(0, 0, dim, dim))
		if got < 1.0 {
			t.Errorf("stroke at %dx%d = %.3f, want ≥ 1.0 (sub-pixel floor)", dim, dim, got)
		}
	}
}
