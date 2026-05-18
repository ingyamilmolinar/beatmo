//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestPlayPulseGated_NotDrawnWhenStopped verifies the play-pulse halo does
// NOT render around the play button when playback is stopped. Regression
// for the screenshot bug where an orange rectangular outline appeared
// "behind" the play button.
func TestPlayPulseGated_NotDrawnWhenStopped(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.SetPlaying(false)

	pr := g.drum.playBtn().Rect()
	rects := captureGlowOutlinesNear(t, pr, func() {
		g.drum.Draw(ebiten.NewImage(640, 480), nil, 0, nil, 0)
	})
	if got := len(rects); got != 0 {
		t.Fatalf("expected 0 drum-glow halo passes when stopped, got %d: %+v", got, rects)
	}
}

// TestPlayPulseGated_IconMustBePauseToShowGlow verifies the halo is gated
// on the actual visible icon (Pause) rather than just the isPlaying flag.
// Defends against state drift: if isPlaying is true but the icon was left
// as Play, we must NOT draw the orange ring (the screenshot bug class).
func TestPlayPulseGated_IconMustBePauseToShowGlow(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.drum.isPlaying = true
	pb := g.drum.playBtn()
	if pb == nil {
		t.Fatal("play button missing")
	}
	pb.Icon = string(IconPlay) // wrong icon for the flag

	pr := pb.Rect()
	rects := captureGlowOutlinesNear(t, pr, func() {
		g.drum.Draw(ebiten.NewImage(640, 480), nil, 0, nil, 0)
	})
	if got := len(rects); got != 0 {
		t.Fatalf("expected 0 halo passes when icon is Play (state drift), got %d", got)
	}
}

// TestPlayPulseHalo_MultiPassFalloff verifies that during playback the glow
// is rendered as a soft multi-pass halo, not a single hard outline at
// Inset(-2). The hard 1-pixel outline at exactly Inset(-2) is the
// screenshot bug: the new implementation must use multiple decreasing-
// alpha passes (a halo), and one of them must extend wider than 2px from
// the button edge.
func TestPlayPulseHalo_MultiPassFalloff(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Drive the visible state via the public setter so the icon flips to
	// Pause (the gate the new implementation enforces).
	g.drum.SetPlaying(true)
	pb := g.drum.playBtn()
	if pb == nil {
		t.Fatal("play button missing")
	}
	if pb.Icon != string(IconPause) {
		t.Fatalf("expected Pause icon when playing, got %q", pb.Icon)
	}

	pr := pb.Rect()
	if pr.Empty() {
		t.Fatalf("play button rect is empty after Layout; got %v", pr)
	}
	rects := captureGlowOutlinesNear(t, pr, func() {
		g.drum.Draw(ebiten.NewImage(640, 480), nil, 0, nil, 0)
	})
	if len(rects) < 2 {
		t.Fatalf("expected halo to draw multiple falloff passes (≥2), got %d (play btn rect %v)", len(rects), pr)
	}

	// At least one pass extends ≥3 px from the button edge.
	maxOutset := 0
	for _, r := range rects {
		out := pr.Min.X - r.Rect.Min.X
		if out > maxOutset {
			maxOutset = out
		}
	}
	if maxOutset < 3 {
		t.Fatalf("expected halo to extend ≥3 px from button edge; max outset = %d px", maxOutset)
	}
}

// captureGlowOutlinesNear intercepts unfilled drumGlow-coloured drawRect
// calls whose rectangle contains the play button rect (i.e., halo passes
// drawn around it).
func captureGlowOutlinesNear(t *testing.T, btnRect image.Rectangle, fn func()) []drawnRect {
	t.Helper()
	var rects []drawnRect
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		// Keep only halo passes that bracket the entire play-button rect
		// (i.e., r contains btnRect with some outset).
		if !filled && isDrumGlowColor(c) &&
			r.Min.X <= btnRect.Min.X && r.Min.Y <= btnRect.Min.Y &&
			r.Max.X >= btnRect.Max.X && r.Max.Y >= btnRect.Max.Y {
			rects = append(rects, drawnRect{
				Rect:  r,
				Color: color.RGBAModel.Convert(c).(color.RGBA),
			})
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()
	fn()
	return rects
}

func isDrumGlowColor(c color.Color) bool {
	// drumGlow is published as color.RGBA but threaded through WithAlpha,
	// which returns color.NRGBA. Convert via NRGBAModel so the raw RGB
	// channels survive the round-trip (RGBAModel premultiplies and would
	// distort low-alpha samples).
	n := color.NRGBAModel.Convert(c).(color.NRGBA)
	return n.R == genColorDrumGlow.R && n.G == genColorDrumGlow.G && n.B == genColorDrumGlow.B
}
