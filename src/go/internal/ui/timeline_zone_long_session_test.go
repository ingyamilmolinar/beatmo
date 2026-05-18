//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Long-session regression guards for the timeline ribbon.
//
// At beat 8552 (~1:11:15 elapsed), screenshot.png shows the ribbon
// rendering a near-solid wall of vertical ticks compressed across the
// whole bar width. The pre-fix behavior maps the entire [0..elapsedBeats]
// history into barRect.Dx() pixels with sub-pixel clustering. The post-fix
// behavior fixes beats-per-pixel and pins the playhead at ~70% of the bar
// width, so tick density and playhead position stay stable regardless of
// session length.

// ribbonZoneForTest constructs a TimelineZone wired with callbacks that
// emulate live playback at the given elapsed-beat count. The mutable
// totalBeats cell mirrors the high-water-mark behavior the live
// drumview wires up: SetTimelineBeats() and TimelineBeats() share one
// counter.
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
		OnScrubPosition:      func(int) {},
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

// TestTimelineRibbon_DoesNotClutterAtHighAbs asserts the ribbon cannot
// degenerate into a forest of ticks once playback crosses several thousand
// beats. The fix should hold tick density ≤ barRect.Dx()/4 (≥4 px between
// adjacent ticks) at every elapsed-beat checkpoint that exercises a long
// session. Pre-fix the loop emits ~1024 ticks at 1 px spacing.
func TestTimelineRibbon_DoesNotClutterAtHighAbs(t *testing.T) {
	// elapsedBeats is whatever displayBeat() returns — in beats, not
	// subdivision units. screenshot.png shows playback at beat 8552.
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

			// The ribbon caches into TlCache, then blits it onto screen.
			// Tick drawRect calls land on TlCache, not on screen, but
			// collectFilledRects intercepts the function pointer itself,
			// so they show up here regardless of dst.
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
					// Tick marks are 1-px wide; anything wider is background
					// chrome that happens to share the color.
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

// TestTimelineRibbon_PlayheadStaysAt70Percent asserts the cursor stays at
// roughly 70% of the bar width regardless of session length. Pre-fix the
// cursor X is computed as elapsedBeats/totalBeats × width, which compresses
// toward the right edge as the session grows.
func TestTimelineRibbon_PlayheadStaysAt70Percent(t *testing.T) {
	const tolerancePx = 4
	for _, elapsedBeats := range []float64{100, 1000, 8552, 20000} {
		t.Run("", func(t *testing.T) {
			z, barRect := ribbonZoneForTest(t, elapsedBeats)
			screen := ebiten.NewImage(barRect.Dx(), barRect.Dy()+40)

			rects := collectFilledRects(t, func() {
				z.computeTimelineBeats(elapsedBeats)
				totalBeats := z.callbacks.TimelineBeats()
				z.drawTimelineBar(screen, elapsedBeats, totalBeats)
			})

			// The cursor draws as a filled rect colTimelineCursor (or
			// colAccentBright on mobile) with 2*cursorThick width centered
			// on cursorX. We find the only such rect and compare its
			// midpoint to the expected 70%-of-width target.
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
			expectedMid := barRect.Min.X + int(0.70*float64(barRect.Dx()))
			diff := cursorMid - expectedMid
			if diff < 0 {
				diff = -diff
			}
			if diff > tolerancePx {
				t.Fatalf("playhead drifted: cursorMid=%d, expectedMid=%d (diff %d > %d) at elapsedBeats=%v",
					cursorMid, expectedMid, diff, tolerancePx, elapsedBeats)
			}
		})
	}
}
