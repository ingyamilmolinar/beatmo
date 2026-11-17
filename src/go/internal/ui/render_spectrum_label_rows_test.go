//go:build test

package ui

import (
	"image"
	"testing"
)

// TestSpectrumLabelRowsNonOverlapping pins the spectrum layout chokepoint that
// fixed the bracket-label/Hz-label collision (Bass/Mids/Treble brackets used to
// stamp over the 1k/8k Hz tick labels). spectrumLabelRows must split the panel
// into three vertically-stacked, edge-to-edge, non-overlapping rows:
// barRect (plot) above bracketRow above hzRow, all inside the panel.
func TestSpectrumLabelRowsNonOverlapping(t *testing.T) {
	sizes := []struct{ w, h int }{
		{400, 220}, // mobile-ish
		{900, 360}, // desktop
		{1280, 500},
	}
	for _, sz := range sizes {
		rect := image.Rect(20, 30, 20+sz.w, 30+sz.h)
		for _, bracketH := range []int{10, 16} {
			for _, capH := range []int{8, 12} {
				bar, bracket, hz := spectrumLabelRows(rect, bracketH, capH)

				// Vertical stacking, edge-to-edge (no gap, no overlap).
				if bar.Max.Y != bracket.Min.Y {
					t.Errorf("%v bH=%d cH=%d: barRect.Max.Y=%d != bracketRow.Min.Y=%d", rect, bracketH, capH, bar.Max.Y, bracket.Min.Y)
				}
				if bracket.Max.Y != hz.Min.Y {
					t.Errorf("%v bH=%d cH=%d: bracketRow.Max.Y=%d != hzRow.Min.Y=%d", rect, bracketH, capH, bracket.Max.Y, hz.Min.Y)
				}
				// Pairwise non-overlap.
				for _, p := range []struct {
					name string
					a, b image.Rectangle
				}{{"bar∩bracket", bar, bracket}, {"bar∩hz", bar, hz}, {"bracket∩hz", bracket, hz}} {
					if !p.a.Intersect(p.b).Empty() {
						t.Errorf("%v bH=%d cH=%d: %s overlap %v", rect, bracketH, capH, p.name, p.a.Intersect(p.b))
					}
				}
				// All rows stay inside the panel and have positive height.
				for _, r := range []image.Rectangle{bar, bracket, hz} {
					if r.Min.Y < rect.Min.Y || r.Max.Y > rect.Max.Y {
						t.Errorf("%v bH=%d cH=%d: row %v escapes panel", rect, bracketH, capH, r)
					}
					if r.Dy() <= 0 {
						t.Errorf("%v bH=%d cH=%d: row %v has non-positive height", rect, bracketH, capH, r)
					}
				}
			}
		}
	}
}
