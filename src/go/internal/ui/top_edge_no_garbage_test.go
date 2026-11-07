//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// The strip between the drum-pane top and the transport surface shows only the background: no layout-guide ticks,
// no row-colour bleed, no debug chrome.
func TestTopEdgeNoGarbage(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)

	// The production default; UpdateProfile on cleanup restores the runner's setting.
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

	// The middle of the gap.
	gapY := (dv.Bounds.Min.Y + transportRect.Min.Y) / 2
	if gapY <= dv.Bounds.Min.Y || gapY >= transportRect.Min.Y {
		// This profile puts the transport flush with the pane top.
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

// Per-zone clipping keeps row content out of the transport band.
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

	// Saturated row colours never belong in the transport surface.
	const badRedThreshold = 100 // R channel above this AND
	const badRedDominance = 30  // R - G > this AND R - B > this
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
