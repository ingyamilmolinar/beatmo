//go:build test

package ui

import (
	"image"
	"testing"
)

// TestEQCurveSpanXInsetFromEdges pins the fix for the EQ curve/hatch running
// flush to x=0 / x=max (amputated by the panel edge): eqCurveSpanX insets the
// curve span by SpaceXS on each side for a normal-width plot, and degrades
// gracefully to the full span for a degenerate (too-narrow) rect.
func TestEQCurveSpanXInsetFromEdges(t *testing.T) {
	z := &EQPanelZone{}

	wide := image.Rect(100, 50, 700, 300)
	minX, maxX := z.eqCurveSpanX(wide)
	if minX != wide.Min.X+SpaceXS {
		t.Errorf("minX = %d, want inset %d", minX, wide.Min.X+SpaceXS)
	}
	if maxX != wide.Max.X-SpaceXS {
		t.Errorf("maxX = %d, want inset %d", maxX, wide.Max.X-SpaceXS)
	}
	if minX <= wide.Min.X || maxX >= wide.Max.X {
		t.Errorf("curve span [%d,%d] is flush with panel edges [%d,%d]", minX, maxX, wide.Min.X, wide.Max.X)
	}

	// Degenerate: narrower than two insets → fall back to the full span
	// rather than inverting.
	narrow := image.Rect(10, 0, 10+SpaceXS, 20)
	nMin, nMax := z.eqCurveSpanX(narrow)
	if nMin != narrow.Min.X || nMax != narrow.Max.X {
		t.Errorf("degenerate span = [%d,%d], want full [%d,%d]", nMin, nMax, narrow.Min.X, narrow.Max.X)
	}
}
