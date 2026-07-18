//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestFastForwardJumpsByDefaultN drives the FF transport button's OnClick
// callback directly and asserts it queues a +jumpBeats (default 4) relative
// seek — the new configurable-jump behavior replacing the old fixed ±1 beat.
func TestFastForwardJumpsByDefaultN(t *testing.T) {
	g := newGameWithStart(t)
	if got := g.drum.jumpBeats; got != 4 {
		t.Fatalf("default jumpBeats=%d, want 4", got)
	}
	div := g.grid.MaxDiv()
	g.SeekSteps(8*div, seekOriginJS) // start at beat 8
	g.drum.transportZone.callbacks.OnFastForward()
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 12*div {
		t.Fatalf("playhead=%d want %d (+4 beats)", got, 12*div)
	}
}

// TestRewindJumpsByDefaultN mirrors the above for the RW button.
func TestRewindJumpsByDefaultN(t *testing.T) {
	g := newGameWithStart(t)
	div := g.grid.MaxDiv()
	g.SeekSteps(8*div, seekOriginJS)
	g.drum.transportZone.callbacks.OnRewind()
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 4*div {
		t.Fatalf("playhead=%d want %d (-4 beats)", got, 4*div)
	}
}

// TestJumpSizeConfigurable asserts that changing dv.jumpBeats (as the ×N
// dropdown's OnSelect would) changes how far FF jumps.
func TestJumpSizeConfigurable(t *testing.T) {
	g := newGameWithStart(t)
	g.drum.jumpBeats = 8
	g.drum.transportZone.SetJumpBeats(8)
	div := g.grid.MaxDiv()
	g.SeekSteps(8*div, seekOriginJS)
	g.drum.transportZone.callbacks.OnFastForward()
	g.drainSeekRequest()
	if got := g.playheadAbsSubdiv(); got != 16*div {
		t.Fatalf("playhead=%d want %d (+8 beats)", got, 16*div)
	}
	if got := g.drum.transportZone.jumpBtn.Text; got != "×8" {
		t.Fatalf("jumpBtn.Text=%q, want %q", got, "×8")
	}
}

// TestFFRWJumpButtonsVisibleOnDesktop is the regression guard for the
// invisibility bug: rwBtn/ffBtn (and now jumpBtn) got real, non-empty rects
// from layoutDesktop, but the toolbar's Draw path never called their
// Draw() methods, so nothing was ever painted. A rect-only assertion would
// NOT have caught that bug, so this test renders the zone into a real
// *ebiten.Image and samples pixels inside each button's rect, asserting
// some ink landed there (not just that the rect is non-empty).
func TestFFRWJumpButtonsVisibleOnDesktop(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, false) // desktop profile
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 3)
	if Profile().IsMobile() {
		t.Fatal("expected desktop profile")
	}

	z := g.drum.transportZone
	buttons := []struct {
		name string
		btn  *Button
	}{
		{"rw", z.rwBtn},
		{"ff", z.ffBtn},
		{"jump", z.jumpBtn},
	}
	for _, b := range buttons {
		if b.btn == nil {
			t.Fatalf("%s button is nil", b.name)
		}
		if b.btn.Rect().Empty() {
			t.Fatalf("%s button rect is empty on desktop", b.name)
		}
	}

	dst := ebiten.NewImage(1280, 720)
	z.Draw(dst)

	painted := func(r image.Rectangle) bool {
		cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
		for dy := -3; dy <= 3; dy++ {
			for dx := -3; dx <= 3; dx++ {
				x, y := cx+dx, cy+dy
				if x < r.Min.X || x >= r.Max.X || y < r.Min.Y || y >= r.Max.Y {
					continue
				}
				if _, _, _, a := dst.At(x, y).RGBA(); a > 0 {
					return true
				}
			}
		}
		return false
	}

	for _, b := range buttons {
		if r := b.btn.Rect(); !painted(r) {
			t.Errorf("%s button rect %v has no painted pixels — Draw() call missing from the toolbar render path", b.name, r)
		}
	}
}
