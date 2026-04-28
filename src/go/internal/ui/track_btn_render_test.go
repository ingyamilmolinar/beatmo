//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestTrackBtnRendersInDesktopFrame is the regression guard the original
// "we have tests but no button can be seen" bug demanded. Every existing
// track-button test inspected state (rect, icon, style); none observed
// pixels actually emitted to the screen. As a result, two contradictory
// platform gates could collude to suppress the draw on every platform —
// and the suite stayed green.
//
// This test drives a desktop frame, intercepts every drawRect call, and
// asserts at least one filled rect lands inside the track button's bounds
// for both follow=true (tracking) and follow=false (free) states. It also
// asserts the play button emits at least one rect at the same instant —
// the track button's chrome must be rendered through the same path play
// is, since both share ComponentButtonSecondary.
func TestTrackBtnRendersInDesktopFrame(t *testing.T) {
	assertDefaultParityState(t)
	withSmallScreen(t, false)

	dv := newDrumViewForTrackTest(t, false)

	trackR := dv.TrackBtnForTest().Rect()
	if trackR.Empty() {
		t.Fatalf("track button rect is empty on desktop — layout did not assign one")
	}
	playR := dv.playBtn().Rect()
	if playR.Empty() {
		t.Fatalf("play button rect is empty on desktop — fixture state invalid")
	}

	type drawCall struct {
		rect   image.Rectangle
		filled bool
	}

	captureFrame := func(t *testing.T, label string) (trackHits, playHits int) {
		t.Helper()
		var calls []drawCall
		orig := drawRect
		drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
			calls = append(calls, drawCall{rect: r, filled: filled})
			orig(dst, r, c, filled)
		}
		defer func() { drawRect = orig }()

		screen := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
		dv.Draw(screen, nil, 0, nil, 0)

		for _, c := range calls {
			if !c.filled {
				continue
			}
			if rectsOverlap(c.rect, trackR) {
				trackHits++
			}
			if rectsOverlap(c.rect, playR) {
				playHits++
			}
		}
		return
	}

	// Frame 1: follow=true (tracking, the default). Track button should be
	// drawn just like play.
	trackHits, playHits := captureFrame(t, "follow=true")
	if trackHits == 0 {
		t.Fatalf("follow=true: track button rect %v received zero filled rects (button invisible)", trackR)
	}
	if playHits == 0 {
		t.Fatalf("follow=true: play button rect %v received zero filled rects (fixture broken — both buttons invisible)", playR)
	}

	// Frame 2: follow=false (free). State swap must not hide the chrome —
	// only the icon glyph and tint should change.
	dv.SetFollow(false)
	trackHits, playHits = captureFrame(t, "follow=false")
	if trackHits == 0 {
		t.Fatalf("follow=false: track button rect %v received zero filled rects (active-state visual hides chrome)", trackR)
	}
	if playHits == 0 {
		t.Fatalf("follow=false: play button rect %v received zero filled rects", playR)
	}
}

// rectsOverlap reports whether a and b share at least one pixel. We don't
// require strict containment because the button chrome is drawn as several
// nested rects (fill, top-edge highlight, border) and any of them landing
// inside the button bounds is proof the button rendered.
func rectsOverlap(a, b image.Rectangle) bool {
	if a.Empty() || b.Empty() {
		return false
	}
	return a.Min.X < b.Max.X && b.Min.X < a.Max.X &&
		a.Min.Y < b.Max.Y && b.Min.Y < a.Max.Y
}
