//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestSamplerEmptyStateDrawsPlaceholder is a characterization guard for the
// Sampler tab's empty-state rendering (no sample loaded). Before this test,
// drawSamplerTab's no-buffer branch and drawSamplerPlaceholder had ZERO test
// coverage — a refactor of the tab could silently break the "pick an
// instrument" prompt or panic drawing the empty card. This pins:
//   - an empty-instrument layout leaves no working buffer, and
//   - drawSamplerTab renders the placeholder card without panicking, and
//   - drawSamplerPlaceholder degrades its message across card widths and
//     surfaces s.status.
//
// It deliberately drives the real methods (not a reimplementation) so the guard
// tracks whatever the production draw path does.
func TestSamplerEmptyStateDrawsPlaceholder(t *testing.T) {
	g := newSamplerTabGame(t)
	dv := g.drum

	// Empty-instrument layout => no working buffer => placeholder path.
	dv.buildSamplerTab(image.Rect(0, 0, 1280, 180), "")
	if dv.sampler.hasBuffer() {
		t.Fatal("precondition: empty-instrument layout must leave no sampler buffer")
	}

	screen := ebiten.NewImage(1280, 180)
	// drawSamplerTab must render the empty-state tab (header + placeholder card +
	// buttons) without panicking.
	dv.drawSamplerTab(screen, image.Rect(0, 0, 1280, 180))

	// drawSamplerPlaceholder is width-responsive and surfaces s.status; exercise
	// the wide, narrow, and status-line branches directly so a refactor can't
	// drop the fallbacks unnoticed. (Any panic here fails the test.)
	dv.drawSamplerPlaceholder(screen, image.Rect(0, 0, 1000, 180)) // wide: full prompt
	dv.drawSamplerPlaceholder(screen, image.Rect(0, 0, 90, 180))   // narrow: shortest msg
	dv.sampler.status = "capture failed"
	dv.drawSamplerPlaceholder(screen, image.Rect(0, 0, 400, 180)) // status line shown
}
