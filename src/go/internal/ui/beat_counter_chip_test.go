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

// TestBeatCounterChip_DrawsSurface1Fill verifies the chip is filled with
// `colSurface1` (not the prior `colSurface2 @ AlphaMedium` pill) so it
// reads as a real container in the toolbar surface ladder.
func TestBeatCounterChip_DrawsSurface1Fill(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 200, 24)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	calls := captureRoundedRectCalls(t, func() { z.drawBeatCounter(dst, 0) })

	wantFill := color.RGBAModel.Convert(colSurface1).(color.RGBA)
	found := false
	for _, c := range calls {
		if !c.Filled {
			continue
		}
		got := color.RGBAModel.Convert(c.Color).(color.RGBA)
		if got == wantFill && c.Rect.In(beatRect) && !c.Rect.Empty() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("no filled drawRoundedRect with colSurface1 inside beatCounterRect %v; calls=%+v", beatRect, calls)
	}
}

// TestBeatCounterChip_UsesRadiusMD verifies the chip uses RadiusMD (8 px)
// rather than the prior RadiusSM (which read as a pill, not a chip).
func TestBeatCounterChip_UsesRadiusMD(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 200, 24)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	calls := captureRoundedRectCalls(t, func() { z.drawBeatCounter(dst, 0) })

	for _, c := range calls {
		if c.Filled && c.Rect.In(beatRect) && !c.Rect.Empty() {
			if c.Radius != RadiusMD {
				t.Fatalf("beat counter chip radius = %d; want RadiusMD = %d", c.Radius, RadiusMD)
			}
			return
		}
	}
	t.Fatal("beat counter chip filled draw not found")
}

// TestBeatCounterChip_RightAligned verifies the chip's right edge sits
// near the allotted rect's right edge (within 1 px), confirming the
// right-alignment refactor.
func TestBeatCounterChip_RightAligned(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 200, 24)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	calls := captureRoundedRectCalls(t, func() { z.drawBeatCounter(dst, 0) })

	var chip image.Rectangle
	for _, c := range calls {
		if c.Filled && c.Rect.In(beatRect) && !c.Rect.Empty() {
			chip = c.Rect
			break
		}
	}
	if chip.Empty() {
		t.Fatal("no chip rect captured")
	}
	gap := beatRect.Max.X - chip.Max.X
	if gap < 0 || gap > 1 {
		t.Fatalf("expected chip flush-right within 1 px of beatCounterRect.Max.X=%d; chip=%v gap=%d",
			beatRect.Max.X, chip, gap)
	}
}
