//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

type roundedDraw struct {
	Rect   image.Rectangle
	Color  color.Color
	Radius int
	Filled bool
}

func captureRoundedRectCalls(t *testing.T, fn func()) []roundedDraw {
	t.Helper()
	var calls []roundedDraw
	orig := drawRoundedRect
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		calls = append(calls, roundedDraw{
			Rect:   r,
			Color:  c,
			Radius: radius,
			Filled: filled,
		})
		orig(dst, r, c, radius, filled)
	}
	t.Cleanup(func() { drawRoundedRect = orig })
	fn()
	return calls
}

func newBeatCounterTestZone(rect image.Rectangle) *TimelineZone {
	return NewTimelineZone(TimelineCallbacks{
		BeatCounterRect:      func() image.Rectangle { return rect },
		TimelineBeats:        func() int { return 8 },
		TimelineUnitsPerBeat: func() int { return 1 },
		Length:               func() int { return 8 },
		Offset:               func() int { return 0 },
		IsPlaying:            func() bool { return false },
		SecPerBeat:           func() float64 { return 0.5 },
		BeatLength:           func() int { return 8 },
	})
}

// The cap face drawn inside beatRect with the secondary keycap fill.
func beatCounterKeycapCap(t *testing.T, z *TimelineZone, dst *ebiten.Image, beatRect image.Rectangle) (image.Rectangle, bool) {
	t.Helper()
	restFill := color.RGBAModel.Convert(TransportMiscStyle.Fill).(color.RGBA)
	hoverFill := color.RGBAModel.Convert(adjustColor(TransportMiscStyle.Fill, 12)).(color.RGBA)
	// The flat path paints the body through drawRoundedRect(filled), so that is the primitive intercepted.
	var caps []image.Rectangle
	orig := drawRoundedRect
	drawRoundedRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		if f, ok := color.RGBAModel.Convert(c).(color.RGBA); ok && filled {
			if (f == restFill || f == hoverFill) && r.In(beatRect) && !r.Empty() {
				caps = append(caps, r)
			}
		}
		orig(d, r, c, radius, filled)
	}
	t.Cleanup(func() { drawRoundedRect = orig })
	z.drawBeatCounter(dst, 0)
	if len(caps) == 0 {
		return image.Rectangle{}, false
	}
	// The body is the widest such filled rounded rect.
	best := caps[0]
	for _, c := range caps[1:] {
		if c.Dx() > best.Dx() {
			best = c
		}
	}
	return best, true
}

// So it reads as a pressable control, like the track/lock keycap beside it.
func TestBeatCounterKeycap_DrawsSecondaryFill(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 240, 28)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	if _, ok := beatCounterKeycapCap(t, z, dst, beatRect); !ok {
		t.Fatalf("no secondary-fill keycap cap face inside beatCounterRect %v", beatRect)
	}
}

// A left-anchored slot next to the track/lock keycap.
func TestBeatCounterKeycap_LeftAligned(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 240, 28)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	cap, ok := beatCounterKeycapCap(t, z, dst, beatRect)
	if !ok {
		t.Fatal("no keycap cap face captured")
	}
	gap := cap.Min.X - beatRect.Min.X
	if gap < 0 || gap > 1 {
		t.Fatalf("expected keycap flush-left within 1 px of beatCounterRect.Min.X=%d; cap=%v gap=%d",
			beatRect.Min.X, cap, gap)
	}
}
