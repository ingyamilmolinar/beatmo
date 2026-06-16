//go:build test

package ui

import (
	"image"
	"testing"
)

// transportSquareButtons returns the transport controls that must render as
// equal-size squares (everything except the BPM text box and the BPM ±
// stepper, which are the intentional exceptions). The master volume control
// is an icon rect (mainVolIconRect), not a Button, so it is included by rect.
func transportSquareRects(z *TransportZone, includeUndoRedo bool) []struct {
	name string
	r    image.Rectangle
} {
	out := []struct {
		name string
		r    image.Rectangle
	}{
		{"play", z.playBtn.Rect()},
		{"stop", z.stopBtn.Rect()},
		{"record", z.recordBtn.Rect()},
		{"subdiv", z.subdivBtn.Rect()},
		{"vol", z.mainVolIconRect},
		{"overflow", z.overflowBtn.Rect()},
	}
	if includeUndoRedo {
		out = append(out,
			struct {
				name string
				r    image.Rectangle
			}{"undo", z.undoBtn.Rect()},
			struct {
				name string
				r    image.Rectangle
			}{"redo", z.redoBtn.Rect()},
		)
	}
	return out
}

// TestDesktopTransportButtonsAreEqualSquares asserts that every transport
// button except the BPM box and BPM ± stepper is a square (width == height)
// and that all such squares share the same side length, across the full
// desktop width range. This is the "make the subdivision button square; make
// all buttons except +/- BPM the same size and shape" requirement.
func TestDesktopTransportButtonsAreEqualSquares(t *testing.T) {
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	for _, w := range []int{1024, 1280, 1366, 1440, 1680, 1920} {
		g := New(testLogger)
		g.Layout(w, 720)
		advanceFrames(g, 3)
		if Profile().IsMobile() {
			g.CloseForTest()
			t.Fatalf("w=%d should be desktop-class", w)
		}
		z := g.drum.transportZone
		squares := transportSquareRects(z, true)

		side := -1
		for _, s := range squares {
			if s.r.Empty() {
				t.Errorf("w=%d: %s square rect is empty", w, s.name)
				continue
			}
			if dw, dh := s.r.Dx(), s.r.Dy(); abs(dw-dh) > 1 {
				t.Errorf("w=%d: %s is not square: %dx%d (%v)", w, s.name, dw, dh, s.r)
			}
			if side < 0 {
				side = s.r.Dy()
			} else if abs(s.r.Dy()-side) > 1 {
				t.Errorf("w=%d: %s side %d differs from common side %d", w, s.name, s.r.Dy(), side)
			}
		}
		g.CloseForTest()
	}
}

// orderedTransportControls returns the desktop transport controls in their
// left-to-right running order, for gap analysis. The BPM ± stepper is
// represented by its inc button (inc/dec share the same X span).
func orderedTransportControls(z *TransportZone) []struct {
	name string
	r    image.Rectangle
} {
	return []struct {
		name string
		r    image.Rectangle
	}{
		{"play", z.playBtn.Rect()},
		{"stop", z.stopBtn.Rect()},
		{"record", z.recordBtn.Rect()},
		{"bpmBox", z.bpmBox.Rect},
		{"bpmStep", z.bpmIncBtn.Rect()},
		{"subdiv", z.subdivBtn.Rect()},
		{"vol", z.mainVolIconRect},
		{"undo", z.undoBtn.Rect()},
		{"redo", z.redoBtn.Rect()},
		{"overflow", z.overflowBtn.Rect()},
	}
}

// TestDesktopTransportNoSpacerGap asserts there is no large empty gap between
// any two adjacent transport controls — in particular none between the
// subdivision button and the master volume control (the old weight-0.6
// "spacer" cell). Gaps between adjacent controls must be uniform and small.
func TestDesktopTransportNoSpacerGap(t *testing.T) {
	assertDefaultParityState(t)
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	for _, w := range []int{1024, 1280, 1366, 1440, 1920} {
		g := New(testLogger)
		g.Layout(w, 720)
		advanceFrames(g, 3)
		if Profile().IsMobile() {
			g.CloseForTest()
			t.Fatalf("w=%d should be desktop-class", w)
		}
		z := g.drum.transportZone
		controls := orderedTransportControls(z)

		// Square side is the reference for "small": gaps must be smaller than
		// a button, so the row reads as one continuous cluster.
		side := z.playBtn.Rect().Dy()

		minGap, maxGap := 1<<30, -(1 << 30)
		var subdivVolGap int
		for i := 0; i+1 < len(controls); i++ {
			a, b := controls[i].r, controls[i+1].r
			if a.Empty() || b.Empty() {
				t.Fatalf("w=%d: %s or %s rect empty", w, controls[i].name, controls[i+1].name)
			}
			if b.Min.X < a.Max.X {
				t.Errorf("w=%d: %s %v overlaps %s %v", w, controls[i].name, a, controls[i+1].name, b)
			}
			gap := b.Min.X - a.Max.X
			if gap < minGap {
				minGap = gap
			}
			if gap > maxGap {
				maxGap = gap
			}
			if controls[i].name == "subdiv" && controls[i+1].name == "vol" {
				subdivVolGap = gap
			}
		}

		// No gap may be as large as a whole button (no spacer void).
		if maxGap >= side {
			t.Errorf("w=%d: largest inter-control gap %d >= button side %d (spacer void present)", w, maxGap, side)
		}
		// Gaps must be uniform — the spread between the widest and narrowest
		// gap is at most a couple of pixels of rounding.
		if maxGap-minGap > 2 {
			t.Errorf("w=%d: inter-control gaps not uniform: min=%d max=%d", w, minGap, maxGap)
		}
		// The subdiv→vol gap must match the uniform gap (no leftover spacer).
		if subdivVolGap > maxGap {
			t.Errorf("w=%d: subdiv→vol gap %d exceeds uniform max gap %d", w, subdivVolGap, maxGap)
		}
	}
}

// TestMobileTransportNoButtonSizedVoid drives the real mobile Game layout and
// asserts the mobile transport row has no button-sized empty gap between
// adjacent controls. Mobile is intentionally NOT converted to equal squares
// (its transport region is only ~188px wide, so seven controls at the 44px
// touch-min cannot be square — see layoutMobile's note). This test instead
// locks the "no weird gaps" half of the request for mobile: no void as wide as
// a full touch target may appear between consecutive controls.
func TestMobileTransportNoButtonSizedVoid(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, true)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	if !Profile().IsMobile() {
		t.Fatal("expected mobile profile")
	}
	z := g.drum.transportZone

	// Every control that occupies a slot in the mobile transport row, by rect.
	rects := []image.Rectangle{
		z.playBtn.Rect(), z.stopBtn.Rect(), z.recordBtn.Rect(),
		z.bpmDecBtn.Rect(), z.bpmBox.Rect, z.bpmIncBtn.Rect(),
		z.subdivBtn.Rect(), z.mainVolIconRect, z.overflowBtn.Rect(),
	}
	// Sort left-to-right by Min.X (simple insertion sort; tiny slice).
	for i := 1; i < len(rects); i++ {
		for j := i; j > 0 && rects[j].Min.X < rects[j-1].Min.X; j-- {
			rects[j], rects[j-1] = rects[j-1], rects[j]
		}
	}

	// No empty gap between consecutive controls may be as wide as a full
	// touch target — that would be the kind of "spacer void" the desktop bar
	// had. (Mobile's actual gaps are 0–15px.)
	maxVoid := TouchMinTarget()
	for i := 0; i+1 < len(rects); i++ {
		a, b := rects[i], rects[i+1]
		if a.Empty() || b.Empty() {
			continue
		}
		if gap := b.Min.X - a.Max.X; gap >= maxVoid {
			t.Errorf("button-sized void between %v and %v: gap=%d >= %d", a, b, gap, maxVoid)
		}
	}
}
