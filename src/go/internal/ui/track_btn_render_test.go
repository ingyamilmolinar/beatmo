//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Asserted on pixels emitted, not state: state-only tests stayed green while two platform gates hid the button
// everywhere. Play shares its spec, so it must render through the same path at the same instant.
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

	trackHits, playHits := captureFrame(t, "follow=true")
	if trackHits == 0 {
		t.Fatalf("follow=true: track button rect %v received zero filled rects (button invisible)", trackR)
	}
	if playHits == 0 {
		t.Fatalf("follow=true: play button rect %v received zero filled rects (fixture broken — both buttons invisible)", playR)
	}

	// Only the icon and tint change with the state, never the chrome.
	dv.SetFollow(false)
	trackHits, playHits = captureFrame(t, "follow=false")
	if trackHits == 0 {
		t.Fatalf("follow=false: track button rect %v received zero filled rects (active-state visual hides chrome)", trackR)
	}
	if playHits == 0 {
		t.Fatalf("follow=false: play button rect %v received zero filled rects", playR)
	}
}

// The chrome is several nested rects; any landing inside proves the button rendered.
func rectsOverlap(a, b image.Rectangle) bool {
	if a.Empty() || b.Empty() {
		return false
	}
	return a.Min.X < b.Max.X && b.Min.X < a.Max.X &&
		a.Min.Y < b.Max.Y && b.Min.Y < a.Max.Y
}
