//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestTopEdgeNoGarbage asserts that the gap between the top edge of the
// drum pane (dv.Bounds.Min.Y) and the top edge of the transport surface
// (transportRect.Min.Y) contains ONLY the canonical background pixels —
// no stray 1-px ticks from layout guides, no row-color bleed, no
// debug chrome.
//
// Reproduces the user-visible bug from screenshot.png: thin vertical
// tick marks across the top edge of the drum pane, caused by
// drawLayoutGuides being called directly from (*DrumView).Draw and
// enabled by default on desktop. With LayoutGuidesLayer hidden by
// default (BEATMO_DEBUG_LAYOUT=0) and rendering routed through the
// tree, the gap must read back the bg color and nothing else.
func TestTopEdgeNoGarbage(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	// Explicitly clear the debug override so this test exercises the
	// production default. (UpdateProfile on cleanup restores whatever
	// the test runner had set.)
	t.Setenv("BEATMO_DEBUG_LAYOUT", "")
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	dv := g.drum
	if dv == nil {
		t.Fatal("DrumView not initialised")
	}
	transportRect := dv.widgetRects[WidgetTransport]
	if transportRect.Empty() {
		t.Fatal("transport widget rect is empty")
	}

	// The "gap" is the strip between the drum-pane top and the transport
	// surface top. It is normally a few pixels of bg fill — the legacy
	// drawLayoutGuides used to paint 1-px ticks here. Sample a row of
	// pixels at y = (dv.Bounds.Min.Y + transportRect.Min.Y) / 2, which
	// sits in the middle of the gap.
	gapY := (dv.Bounds.Min.Y + transportRect.Min.Y) / 2
	if gapY <= dv.Bounds.Min.Y || gapY >= transportRect.Min.Y {
		// Degenerate gap (zero-height) — nothing to verify; this profile
		// has the transport surface flush with the pane top.
		t.Skipf("no gap above transport (dv.Min.Y=%d transport.Min.Y=%d)",
			dv.Bounds.Min.Y, transportRect.Min.Y)
	}

	screen := ebiten.NewImage(1280, 720)
	g.Draw(screen)

	bg := readPx(screen, dv.Bounds.Min.X+10, gapY)
	for x := dv.Bounds.Min.X; x < dv.Bounds.Max.X; x++ {
		got := readPx(screen, x, gapY)
		if got != bg {
			t.Fatalf("garbage pixel at (%d,%d): got %v, expected bg %v — "+
				"a non-background draw landed in the gap above the transport surface",
				x, gapY, got, bg)
		}
	}
}

// readPx returns the RGBA color at (x,y) clamped via the test ebiten stub.
func readPx(img *ebiten.Image, x, y int) color.RGBA {
	c := img.At(x, y)
	r, g, b, a := c.RGBA()
	return color.RGBA{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8), uint8(a >> 8)}
}

// TestTransportBottomNoBleed asserts that the last 2 px of the transport
// widget do not contain any row colors, FX-orange, or kick-brown — the
// brown stripe at #6f4a42 visible in screenshot.png at y=57-58. With
// strict per-zone clipping enforced by DrumViewTree.Draw, no row content
// can paint into the transport zone's vertical band.
func TestTransportBottomNoBleed(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	t.Setenv("BEATMO_DEBUG_LAYOUT", "")
	UpdateProfile()
	t.Cleanup(UpdateProfile)

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	dv := g.drum
	transportRect := dv.widgetRects[WidgetTransport]
	if transportRect.Empty() {
		t.Fatal("transport widget rect is empty")
	}

	screen := ebiten.NewImage(1280, 720)
	g.Draw(screen)

	// Bleeds we want to catch — saturated row colors that would never
	// legitimately appear in the transport surface.
	const badRedThreshold = 100   // R channel above this AND
	const badRedDominance = 30    // R - G > this AND R - B > this
	bottomBand := image.Rect(transportRect.Min.X, transportRect.Max.Y-2, transportRect.Max.X, transportRect.Max.Y)
	for y := bottomBand.Min.Y; y < bottomBand.Max.Y; y++ {
		for x := bottomBand.Min.X; x < bottomBand.Max.X; x++ {
			c := readPx(screen, x, y)
			r, gC, b := int(c.R), int(c.G), int(c.B)
			if r > badRedThreshold && r-gC > badRedDominance && r-b > badRedDominance {
				t.Fatalf("row-color bleed at (%d,%d) inside transport bottom band: got rgb(%d,%d,%d)",
					x, y, r, gC, b)
			}
		}
	}
}
