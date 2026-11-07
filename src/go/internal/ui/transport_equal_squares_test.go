//go:build test

package ui

import (
	"image"
	"testing"
)

// All but the BPM box and its ± stepper; the master volume is an icon rect, not a Button.
func transportSquareRects(z *TransportZone, includeUndoRedo bool) []struct {
	name string
	r    image.Rectangle
} {
	out := []struct {
		name string
		r    image.Rectangle
	}{
		// Nav sits after subdiv and shares the square side.
		{"play", z.playBtn.Rect()},
		{"stop", z.stopBtn.Rect()},
		{"record", z.recordBtn.Rect()},
		{"subdiv", z.subdivBtn.Rect()},
		{"vol", z.mainVolIconRect},
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

// One square side for all of them across the desktop width range.
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

// Left to right; the ± stepper is represented by inc (they share an X span).
func orderedTransportControls(z *TransportZone) []struct {
	name string
	r    image.Rectangle
} {
	return []struct {
		name string
		r    image.Rectangle
	}{
		// Every control, so no measured gap spans an unlisted button.
		{"play", z.playBtn.Rect()},
		{"stop", z.stopBtn.Rect()},
		{"record", z.recordBtn.Rect()},
		{"bpmBox", z.bpmBox.Rect},
		{"bpmStep", z.bpmIncBtn.Rect()},
		{"subdiv", z.subdivBtn.Rect()},
		{"vol", z.mainVolIconRect},
		{"undo", z.undoBtn.Rect()},
		{"redo", z.redoBtn.Rect()},
	}
}

// Gaps take two values: the in-group gap and spacing.cluster at the three group boundaries, never a spacer
// cell.
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

		// Smaller than a button, so the row reads as one cluster.
		side := z.playBtn.Rect().Dy()

		groupBoundary := map[string]bool{"record": true, "subdiv": true, "vol": true}
		minGap, maxGap := 1<<30, -(1 << 30)
		for i := 0; i+1 < len(controls); i++ {
			a, b := controls[i].r, controls[i+1].r
			if a.Empty() || b.Empty() {
				t.Fatalf("w=%d: %s or %s rect empty", w, controls[i].name, controls[i+1].name)
			}
			if b.Min.X < a.Max.X {
				t.Errorf("w=%d: %s %v overlaps %s %v", w, controls[i].name, a, controls[i+1].name, b)
			}
			gap := b.Min.X - a.Max.X
			if groupBoundary[controls[i].name] {
				if gap != SpaceCluster {
					t.Errorf("w=%d: %s→%s group gap %d, want spacing.cluster %d", w, controls[i].name, controls[i+1].name, gap, SpaceCluster)
				}
				continue
			}
			if gap < minGap {
				minGap = gap
			}
			if gap > maxGap {
				maxGap = gap
			}
		}

		// No gap may be as large as a whole button (no spacer void).
		if maxGap >= side {
			t.Errorf("w=%d: largest inter-control gap %d >= button side %d (spacer void present)", w, maxGap, side)
		}
		// Within a couple of pixels of rounding.
		if maxGap-minGap > 2 {
			t.Errorf("w=%d: in-group gaps not uniform: min=%d max=%d", w, minGap, maxGap)
		}
	}
}

// Mobile cannot be squares (seven 44 px controls in ~188 px), so it only guarantees no button-sized void.
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

	// Navigation opens from the beat counter, file options from the grid folder button.
	rects := []image.Rectangle{
		z.playBtn.Rect(), z.stopBtn.Rect(), z.recordBtn.Rect(),
		z.bpmDecBtn.Rect(), z.bpmBox.Rect, z.bpmIncBtn.Rect(),
		z.subdivBtn.Rect(), z.mainVolIconRect,
	}
	// Sort left-to-right by Min.X (simple insertion sort; tiny slice).
	for i := 1; i < len(rects); i++ {
		for j := i; j > 0 && rects[j].Min.X < rects[j-1].Min.X; j-- {
			rects[j], rects[j-1] = rects[j-1], rects[j]
		}
	}

	// Real gaps are 0–15 px.
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
