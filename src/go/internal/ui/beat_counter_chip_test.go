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

// beatCounterChipFill returns the first filled colSurface1 rect drawn inside
// beatRect by drawBeatCounter — the square chip background. The chip is now a
// plain drawRect (square corners), so this intercepts drawRect, not
// drawRoundedRect. Squareness itself is pinned by TestBeatCounterChipIsSquare.
func beatCounterChipFill(t *testing.T, z *TimelineZone, dst *ebiten.Image, beatRect image.Rectangle) (image.Rectangle, bool) {
	t.Helper()
	wantFill := color.RGBAModel.Convert(colSurface1).(color.RGBA)
	fills := collectFilledRects(t, func() { z.drawBeatCounter(dst, 0) })
	for _, f := range fills {
		if f.Color == wantFill && f.Rect.In(beatRect) && !f.Rect.Empty() {
			return f.Rect, true
		}
	}
	return image.Rectangle{}, false
}

// TestBeatCounterChip_DrawsSurface1Fill verifies the chip is filled with
// `colSurface1` so it reads as a real container in the toolbar surface ladder.
func TestBeatCounterChip_DrawsSurface1Fill(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 200, 24)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	if _, ok := beatCounterChipFill(t, z, dst, beatRect); !ok {
		t.Fatalf("no filled colSurface1 rect inside beatCounterRect %v", beatRect)
	}
}

// TestBeatCounterChip_LeftAligned verifies the chip's left edge sits near
// the allotted rect's left edge (within 1 px). The notification redesign
// moved the counter to a left-anchored slot (next to the track/lock chip)
// so the dedicated notification area can occupy the space to its right.
func TestBeatCounterChip_LeftAligned(t *testing.T) {
	assertDefaultParityState(t)
	beatRect := image.Rect(0, 0, 200, 24)
	z := newBeatCounterTestZone(beatRect)
	dst := ebiten.NewImage(beatRect.Max.X+16, beatRect.Max.Y+16)

	chip, ok := beatCounterChipFill(t, z, dst, beatRect)
	if !ok {
		t.Fatal("no chip rect captured")
	}
	gap := chip.Min.X - beatRect.Min.X
	if gap < 0 || gap > 1 {
		t.Fatalf("expected chip flush-left within 1 px of beatCounterRect.Min.X=%d; chip=%v gap=%d",
			beatRect.Min.X, chip, gap)
	}
}
