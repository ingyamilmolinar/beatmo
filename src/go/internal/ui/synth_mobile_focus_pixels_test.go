//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Stub only: real Ebiten's At() panics before the game loop starts, which once took the whole package down under
// `make test-real`.

// Final composited pixels, so the sprite blit counts; the stub's FaithfulCompositing is needed, or draws into the
// clipped sub-image are discarded.
func TestMobileSynthTab_GraphPaintsInBand(t *testing.T) {
	withFaithfulCompositing(t)
	g := newMobileSynthTabGameForTestSize(t, 390, 844)
	dv := g.drum
	drillFirstStageMobile(t, g)
	band := dv.synthMobileFocusRect()
	if band.Empty() {
		t.Fatal("no graph band")
	}
	// The rendered note lands a frame or two later; settle it as a scene capture does.
	screen := ebiten.NewImage(390, 844)
	g.Draw(screen)
	dv.settleSynthGraphNow()
	g.Draw(screen)
	// Anything but the measured band fill: the drilled card bed differs from the panel token.
	probe := image.Rect(band.Min.X+4, band.Min.Y+4, band.Max.X-4, band.Max.Y-4)
	counts := map[color.Color]int{}
	for y := probe.Min.Y; y < probe.Max.Y; y += 2 {
		for x := probe.Min.X; x < probe.Max.X; x += 2 {
			counts[screen.At(x, y)]++
		}
	}
	bg, most := color.Color(nil), 0
	for c, n := range counts {
		if n > most {
			bg, most = c, n
		}
	}
	inked := 0
	for c, n := range counts {
		if c != bg {
			inked += n
		}
	}
	if inked < 50 {
		t.Fatalf("graph band %v painted only %d sampled non-background pixels", band, inked)
	}
}
