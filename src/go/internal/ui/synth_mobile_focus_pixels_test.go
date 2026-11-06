//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// synth_mobile_focus_pixels_test.go — the one pixel-reading assertion split out
// of synth_mobile_focus_test.go.
//
// WHY THE BUILD TAG. ebiten.Image.At() routes to ReadPixels, which panics
// "ui: ReadPixels cannot be called before the game starts" under REAL Ebiten —
// there is no running game loop in a test binary. Only the ebitenstub backend
// (-tags test) implements At() in pure Go. Left untagged, this test compiled
// into `make test-real` and panicked the whole internal/ui package, taking the
// rest of the suite's results with it.
//
// This is the convention audio_panel_render_pixels_test.go already follows for
// the same reason; the sibling file keeps its layout-only tests untagged so
// they still run on both builds.

// The graph actually paints ink inside the mobile band (final composited
// pixels, so the sprite-cache blit counts).
//
// Final composited pixels need the stub's FaithfulCompositing: the tree draws
// the EQ panel through a zone-clipped SubImage, which the legacy stub returns
// DETACHED — every draw into it is discarded and the band reads as pure
// background (0 inked pixels) however well the graph renders.
func TestMobileSynthTab_GraphPaintsInBand(t *testing.T) {
	withFaithfulCompositing(t)
	g := newMobileSynthTabGameForTestSize(t, 390, 844)
	dv := g.drum
	drillFirstStageMobile(t, g)
	band := dv.synthMobileFocusRect()
	if band.Empty() {
		t.Fatal("no graph band")
	}
	// The graph draws the RENDERED note, which lands off the UI goroutine a
	// frame or two after the request; settle it as a scene capture does.
	screen := ebiten.NewImage(390, 844)
	g.Draw(screen)
	dv.settleSynthGraphNow()
	g.Draw(screen)
	// Ink = anything but the band's own fill. The fill is measured (the modal
	// color), not assumed: the drilled view paints its card bed over the panel
	// surface, so comparing against one token would count the bed as ink.
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
