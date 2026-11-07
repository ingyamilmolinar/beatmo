//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// With no sample: the layout leaves no buffer, the tab draws its placeholder card without panicking, and the
// placeholder degrades across widths and shows s.status. Through the real methods.
func TestSamplerEmptyStateDrawsPlaceholder(t *testing.T) {
	g := newSamplerTabGame(t)
	dv := g.drum

	// Empty-instrument layout => no working buffer => placeholder path.
	dv.buildSamplerTab(image.Rect(0, 0, 1280, 180), "")
	if dv.sampler.hasBuffer() {
		t.Fatal("precondition: empty-instrument layout must leave no sampler buffer")
	}

	screen := ebiten.NewImage(1280, 180)
	dv.drawSamplerTab(screen, image.Rect(0, 0, 1280, 180))

	// The wide, narrow and status-line branches, so a refactor cannot drop a fallback.
	dv.drawSamplerPlaceholder(screen, image.Rect(0, 0, 1000, 180)) // wide: full prompt
	dv.drawSamplerPlaceholder(screen, image.Rect(0, 0, 90, 180))   // narrow: shortest msg
	dv.sampler.status = "capture failed"
	dv.drawSamplerPlaceholder(screen, image.Rect(0, 0, 400, 180)) // status line shown
}
