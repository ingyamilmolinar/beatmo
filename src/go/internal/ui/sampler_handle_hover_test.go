//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Handles at distinct interior fractions, so their rects do not overlap.
func newHoverSampler(t *testing.T) *DrumView {
	t.Helper()
	dv := &DrumView{}
	loadTestSampler(dv)
	dv.sampler.waveformRect = image.Rect(0, 0, 200, 40)
	dv.sampler.startFrac = 0.25
	dv.sampler.endFrac = 0.75
	return dv
}

func samplerHandleCenter(dv *DrumView, frac float64) (int, int) {
	r := dv.samplerHandleRect(frac)
	return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2
}

// -1 none, 0 start, 1 end, like the EQ handles; idle on mobile.
func TestSamplerHandleHoverStateTracksCursor(t *testing.T) {
	t.Run("start handle hovered", func(t *testing.T) {
		dv := newHoverSampler(t)
		cx, cy := samplerHandleCenter(dv, dv.sampler.startFrac)
		restore := setEQCursorForTest(cx, cy)
		defer restore()
		dv.sp().updateSamplerHandleHover()
		if dv.sampler.hoverHandle != 0 {
			t.Fatalf("cursor over start handle: hoverHandle=%d, want 0 (start)", dv.sampler.hoverHandle)
		}
	})

	t.Run("end handle hovered", func(t *testing.T) {
		dv := newHoverSampler(t)
		cx, cy := samplerHandleCenter(dv, dv.sampler.endFrac)
		restore := setEQCursorForTest(cx, cy)
		defer restore()
		dv.sp().updateSamplerHandleHover()
		if dv.sampler.hoverHandle != 1 {
			t.Fatalf("cursor over end handle: hoverHandle=%d, want 1 (end)", dv.sampler.hoverHandle)
		}
	})

	t.Run("cursor off both handles", func(t *testing.T) {
		dv := newHoverSampler(t)
		// Midpoint between the two handles — clear of both.
		mid, cy := samplerHandleCenter(dv, 0.5)
		restore := setEQCursorForTest(mid, cy)
		defer restore()
		dv.sp().updateSamplerHandleHover()
		if dv.sampler.hoverHandle != -1 {
			t.Fatalf("cursor off handles: hoverHandle=%d, want -1 (none)", dv.sampler.hoverHandle)
		}
	})

	t.Run("mobile has no hover", func(t *testing.T) {
		withSmallScreen(t, true)
		dv := newHoverSampler(t)
		cx, cy := samplerHandleCenter(dv, dv.sampler.startFrac)
		restore := setEQCursorForTest(cx, cy)
		defer restore()
		dv.sp().updateSamplerHandleHover()
		if dv.sampler.hoverHandle != -1 {
			t.Fatalf("mobile: hoverHandle=%d, want -1 (no pointer hover on touch)", dv.sampler.hoverHandle)
		}
	})
}

// Straight RGB, alpha ignored.
func countSamplerHandleOwnColorRects(t *testing.T, dv *DrumView, want color.Color, cx, cy, tol int) int {
	t.Helper()
	n := 0
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		mx := (r.Min.X + r.Max.X) / 2
		my := (r.Min.Y + r.Max.Y) / 2
		if abs(mx-cx) <= tol && abs(my-cy) <= tol && colorMatchesRow(c, want) {
			n++
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()
	dv.drawSamplerWaveform(ebiten.NewImage(200, 40))
	return n
}

// colAccentBright through drawRectHoverGlow, and nothing when not hovered.
func TestSamplerHandleHoverDrawsOwnColorHighlight(t *testing.T) {
	dv := newHoverSampler(t)
	r := dv.samplerHandleRect(dv.sampler.startFrac)
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	dv.sampler.hoverHandle = -1
	base := countSamplerHandleOwnColorRects(t, dv, colAccentBright, cx, cy, 16)

	dv.sampler.hoverHandle = 0 // start hovered
	hover := countSamplerHandleOwnColorRects(t, dv, colAccentBright, cx, cy, 16)

	if hover <= base {
		t.Fatalf("hovering the start trim handle added no own-color highlight rects (baseline=%d hover=%d)", base, hover)
	}
}

// The waveform draws to one target, so stub pixel reads model real Ebiten here.
func TestSamplerHandleHoverIsVisiblyHighlighted(t *testing.T) {
	dv := newHoverSampler(t)
	r := dv.samplerHandleRect(dv.sampler.startFrac)
	cx := (r.Min.X + r.Max.X) / 2
	cy := (r.Min.Y + r.Max.Y) / 2

	read := func() map[[2]int]color.RGBA {
		screen := ebiten.NewImage(200, 40)
		dv.drawSamplerWaveform(screen)
		m := map[[2]int]color.RGBA{}
		for dy := -18; dy <= 18; dy++ {
			for dx := -14; dx <= 14; dx++ {
				x, y := cx+dx, cy+dy
				if x < 0 || y < 0 || x >= 200 || y >= 40 {
					continue
				}
				m[[2]int{x, y}] = color.RGBAModel.Convert(screen.At(x, y)).(color.RGBA)
			}
		}
		return m
	}

	dv.sampler.hoverHandle = -1
	base := read()
	dv.sampler.hoverHandle = 0 // start hovered
	hover := read()

	md := 0
	for p, cb := range base {
		ch := hover[p]
		for _, d := range []int{iabsHover(int(cb.R) - int(ch.R)), iabsHover(int(cb.G) - int(ch.G)), iabsHover(int(cb.B) - int(ch.B))} {
			if d > md {
				md = d
			}
		}
	}
	const minVisible = 60
	t.Logf("sampler hover max channel delta = %d (min visible = %d)", md, minVisible)
	if md < minVisible {
		t.Fatalf("hovering the trim handle changed pixels by only %d/255 — not visibly perceptible (want >= %d)", md, minVisible)
	}
}

// On a tall waveform the handle is a thin tall line, and the glow covers it top to bottom at a roughly even
// width, never a centred blob.
func TestSamplerHandleHoverConformsToComponentShape(t *testing.T) {
	dv := newHoverSampler(t)
	dv.sampler.waveformRect = image.Rect(0, 0, 200, 160) // tall → thin tall handle
	r := dv.samplerHandleRect(dv.sampler.startFrac)
	cx := (r.Min.X + r.Max.X) / 2
	topY := r.Min.Y + r.Dy()/10
	midY := (r.Min.Y + r.Max.Y) / 2
	botY := r.Max.Y - 1 - r.Dy()/10

	const span = 34 // sample ±34px around the handle centre X
	grid := func() map[[2]int]color.RGBA {
		screen := ebiten.NewImage(200, 160)
		dv.drawSamplerWaveform(screen)
		m := map[[2]int]color.RGBA{}
		for _, y := range []int{topY, midY, botY} {
			for dx := -span; dx <= span; dx++ {
				x := cx + dx
				if x < 0 || x >= 200 {
					continue
				}
				m[[2]int{x, y}] = color.RGBAModel.Convert(screen.At(x, y)).(color.RGBA)
			}
		}
		return m
	}

	dv.sampler.hoverHandle = -1
	base := grid()
	dv.sampler.hoverHandle = 0
	hover := grid()

	// Width of the hover-changed band at row y.
	width := func(y int) int {
		n := 0
		for dx := -span; dx <= span; dx++ {
			x := cx + dx
			if x < 0 || x >= 200 {
				continue
			}
			k := [2]int{x, y}
			if base[k] != hover[k] {
				n++
			}
		}
		return n
	}
	wTop, wMid, wBot := width(topY), width(midY), width(botY)
	t.Logf("hover-changed width: top=%d mid=%d bot=%d", wTop, wMid, wBot)

	if wTop == 0 || wBot == 0 {
		t.Fatalf("hover highlight does not cover the top/bottom of the component (wTop=%d wBot=%d) — it doesn't conform to the tall thin line", wTop, wBot)
	}
	// A centred blob makes the middle far wider than the ends.
	if wMid > wTop+6 || wMid > wBot+6 {
		t.Fatalf("hover highlight bulges at the middle (top=%d mid=%d bot=%d) — a centred blob, not conforming to the component's shape", wTop, wMid, wBot)
	}
}

// The footprint tracks the given rect's aspect (tall, wide, square), grown only by the glow spread.
func TestDrawRectHoverGlowConformsToAnyShape(t *testing.T) {
	const glowSpread = 4 // must match drawRectHoverGlow's spread
	cases := []struct {
		name string
		r    image.Rectangle
	}{
		{"tall thin", image.Rect(60, 10, 70, 110)}, // 10 x 100
		{"wide flat", image.Rect(10, 60, 110, 70)}, // 100 x 10
		{"square", image.Rect(50, 50, 70, 70)},     // 20 x 20
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			img := ebiten.NewImage(140, 140)
			drawRectHoverGlow(img, tc.r, colAccent, 1.0)

			minX, minY, maxX, maxY := 140, 140, -1, -1
			for y := 0; y < 140; y++ {
				for x := 0; x < 140; x++ {
					if _, _, _, a := img.At(x, y).RGBA(); a > 0 {
						if x < minX {
							minX = x
						}
						if y < minY {
							minY = y
						}
						if x > maxX {
							maxX = x
						}
						if y > maxY {
							maxY = y
						}
					}
				}
			}
			if maxX < 0 {
				t.Fatalf("%s: glow drew nothing", tc.name)
			}
			want := tc.r.Inset(-glowSpread) // rect grown by the glow, same aspect
			// Allow ±2px slack for the outermost faint ring.
			off := func(a, b int) bool { return a < b-2 || a > b+2 }
			if off(minX, want.Min.X) || off(minY, want.Min.Y) || off(maxX, want.Max.X-1) || off(maxY, want.Max.Y-1) {
				t.Fatalf("%s: glow bbox [%d,%d,%d,%d] does not conform to rect+glow %v — the highlight assumed a shape instead of tracking the rect",
					tc.name, minX, minY, maxX, maxY, want)
			}
		})
	}
}
