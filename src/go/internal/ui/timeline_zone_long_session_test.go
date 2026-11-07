//go:build test

package ui

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

// At thousands of beats the whole history squeezed into the bar's pixels became a wall of ticks; density
// and the cursor must stay stable at any session length.

// One shared high-water counter behind SetTimelineBeats/TimelineBeats, as the live drum view wires it.
func ribbonZoneForTest(t *testing.T, elapsedBeats float64) (*TimelineZone, image.Rectangle) {
	t.Helper()
	const unitsPerBeat = 4
	const patternLength = 8 // pattern length in units (so 2 beats @ unitsPerBeat=4)
	const offset = 0
	totalBeats := 16
	cb := TimelineCallbacks{
		Rows:                 func() []*DrumRow { return nil },
		IsPlaying:            func() bool { return true },
		Follow:               func() bool { return true },
		BPM:                  func() int { return 120 },
		SecPerBeat:           func() float64 { return 0.5 },
		TimelineUnitsPerBeat: func() int { return unitsPerBeat },
		Length:               func() int { return patternLength },
		Offset:               func() int { return offset },
		RowOffset:            func() int { return 0 },
		VisibleRows:          func() int { return 4 },
		RowHeight:            func() int { return 24 },
		Cell:                 func() int { return 20 },
		TimelineBeats:        func() int { return totalBeats },
		Frame:                func() int64 { return 0 },
		BeatLength:           func() int { return patternLength },
		OnOffsetChange:       func(int) {},
		OnSeekToStep:         func(int, hooks.SeekOrigin) {},
		SetTimelineBeats:     func(n int) { totalBeats = n },
	}
	z := NewTimelineZone(cb)
	// 1024-px-wide ribbon to mirror desktop layouts.
	barRect := image.Rect(0, 0, 1024, 24)
	z.timelineBarH = barRect.Dy()
	z.Layout(image.Rect(0, 0, barRect.Dx(), barRect.Dy()+40))
	z.timelineBarRect = barRect
	z.SetDrawParams(elapsedBeats, nil)
	return z, barRect
}

// At most barRect.Dx()/4 ticks (≥ 4 px apart) at every long-session checkpoint.
func TestTimelineRibbon_DoesNotClutterAtHighAbs(t *testing.T) {
	// In beats, not subdivision units.
	checkpoints := []float64{
		8552,  // the screenshot symptom
		20000, // deeper into long-session territory
	}
	for _, elapsedBeats := range checkpoints {
		t.Run("", func(t *testing.T) {
			z, barRect := ribbonZoneForTest(t, elapsedBeats)
			screen := ebiten.NewImage(barRect.Dx(), barRect.Dy()+40)

			rects := collectFilledRects(t, func() {
				z.computeTimelineBeats(elapsedBeats)
				totalBeats := z.callbacks.TimelineBeats()
				z.drawTimelineBar(screen, elapsedBeats, totalBeats)
			})

			// The ticks land on TlCache, but collectFilledRects intercepts the function itself.
			beatRGBA := color.RGBAModel.Convert(colTimelineBeat).(color.RGBA)
			ticks := 0
			for _, r := range rects {
				if r.Color != beatRGBA {
					continue
				}
				if r.Rect.Dx() <= 0 || r.Rect.Dy() <= 0 {
					continue
				}
				if r.Rect.Dx() > 4 {
					// Ticks are 1 px wide; wider rects are chrome sharing the colour.
					continue
				}
				ticks++
			}

			maxTicks := barRect.Dx() / 4 // ≥4 px between adjacent ticks
			if ticks > maxTicks {
				t.Fatalf("ribbon clutter: %d ticks rendered in %dpx-wide bar at elapsedBeats=%v (max allowed %d)",
					ticks, barRect.Dx(), elapsedBeats, maxTicks)
			}
		})
	}
}

// The ribbon spans the whole song, so the cursor sits at elapsedBeats / TimelineBeats() of the bar at any
// length.
func TestTimelineRibbon_CursorAtWholeSongFraction(t *testing.T) {
	const tolerancePx = 2
	for _, elapsedBeats := range []float64{100, 1000, 8552, 20000} {
		t.Run("", func(t *testing.T) {
			z, barRect := ribbonZoneForTest(t, elapsedBeats)
			screen := ebiten.NewImage(barRect.Dx(), barRect.Dy()+40)

			rects := collectFilledRects(t, func() {
				z.computeTimelineBeats(elapsedBeats)
				totalBeats := z.callbacks.TimelineBeats()
				z.drawTimelineBar(screen, elapsedBeats, totalBeats)
			})

			// A filled rect 2·cursorThick wide centred on cursorX (colAccentBright on mobile).
			cursorRGBA := color.RGBAModel.Convert(colTimelineCursor).(color.RGBA)
			accentRGBA := color.RGBAModel.Convert(colAccentBright).(color.RGBA)
			var cursorRect image.Rectangle
			found := false
			for _, r := range rects {
				if (r.Color == cursorRGBA || r.Color == accentRGBA) && r.Rect.Dx() <= 5 && r.Rect.Dy() == barRect.Dy() {
					cursorRect = r.Rect
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("playback cursor rect not located at elapsedBeats=%v", elapsedBeats)
			}
			cursorMid := (cursorRect.Min.X + cursorRect.Max.X) / 2

			totalBeats := z.callbacks.TimelineBeats()
			frac := elapsedBeats / float64(max1(totalBeats))
			expectedMid := barRect.Min.X + int(math.Round(frac*float64(barRect.Dx())))
			diff := cursorMid - expectedMid
			if diff < 0 {
				diff = -diff
			}
			if diff > tolerancePx {
				t.Fatalf("playhead misplaced: cursorMid=%d, expectedMid=%d (diff %d > %d) at elapsedBeats=%v (totalBeats=%d)",
					cursorMid, expectedMid, diff, tolerancePx, elapsedBeats, totalBeats)
			}
		})
	}
}
