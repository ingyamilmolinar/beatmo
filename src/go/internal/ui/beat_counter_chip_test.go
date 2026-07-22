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

// captureRoundedRectCalls intercepts drawRoundedRect during fn and returns
// the captured calls. Used by the chip styling tests below.
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

// beatCounterKeycapCap returns the keycap CAP-FACE rect drawn inside beatRect by
// drawBeatCounter — the rounded button painted with the secondary keycap fill
// (TransportMiscStyle.Fill, or its hover-brightened variant). The counter now
// renders as a matte keycap via drawKeycapPanelButton (drawRoundedButton), not
// a flat drawRect, matching every other Button (DESIGN.md "Keycap elevation").
func beatCounterKeycapCap(t *testing.T, z *TimelineZone, dst *ebiten.Image, beatRect image.Rectangle) (image.Rectangle, bool) {
	t.Helper()
	restFill := color.RGBAModel.Convert(TransportMiscStyle.Fill).(color.RGBA)
	hoverFill := color.RGBAModel.Convert(adjustColor(TransportMiscStyle.Fill, 12)).(color.RGBA)
	var caps []image.Rectangle
	orig := drawRoundedButton
	drawRoundedButton = func(d *ebiten.Image, r image.Rectangle, fill, border color.Color, radius int, pressed bool) {
		if f, ok := color.RGBAModel.Convert(fill).(color.RGBA); ok {
			if (f == restFill || f == hoverFill) && r.In(beatRect) && !r.Empty() {
				caps = append(caps, r)
			}
		}
		orig(d, r, fill, border, radius, pressed)
	}
	t.Cleanup(func() { drawRoundedButton = orig })
	z.drawBeatCounter(dst, 0)
	if len(caps) == 0 {
		return image.Rectangle{}, false
	}
	// The cap face is the widest such rounded button (the shell uses a darker
	// derived shade, so it won't match the fill).
	best := caps[0]
	for _, c := range caps[1:] {
		if c.Dx() > best.Dx() {
			best = c
		}
	}
	return best, true
}

// TestBeatCounterKeycap_DrawsSecondaryFill verifies the counter renders a matte
// keycap cap face with the shared secondary fill, so it reads as a pressable
// control consistent with the neighbouring track/lock keycap.
func TestBeatCounterKeycap_DrawsSecondaryFill(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 240, 28)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	if _, ok := beatCounterKeycapCap(t, z, dst, beatRect); !ok {
		t.Fatalf("no secondary-fill keycap cap face inside beatCounterRect %v", beatRect)
	}
}

// TestBeatCounterKeycap_LeftAligned verifies the keycap's left edge sits near
// the allotted rect's left edge — the counter is a left-anchored slot next to
// the track/lock keycap.
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
