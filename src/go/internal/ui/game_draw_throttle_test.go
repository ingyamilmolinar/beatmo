//go:build test

package ui

import (
	"image/color"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestDrawThrottleKeepsLastFrame verifies that when draw throttling skips a
// frame we still blit the previous frame to avoid flashing the canvas.
func TestDrawThrottleKeepsLastFrame(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drawMinInterval = time.Hour

	screen1 := ebitenImage(640, 480)
	g.Draw(screen1)
	if g.frameBuffer == nil {
		t.Fatalf("expected frame buffer initialized after first draw")
	}
	g.frameBuffer.Fill(color.RGBA{255, 0, 0, 255})
	g.drawThrottleCopies = 0

	screen2 := ebitenImage(640, 480)
	g.Draw(screen2)
	if g.drawThrottleCopies != 1 {
		t.Fatalf("expected second draw to reuse cached frame once; copies=%d", g.drawThrottleCopies)
	}
}

// TestDrawThrottleWASMNoFlicker is a regression test for the WASM flickering bug
// where throttled draws produced blank frames because frame buffer was disabled.
// The fix ensures useFrameBuf is always true when drawMinInterval > 0.
func TestDrawThrottleWASMNoFlicker(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1600, 900)

	// Simulate WASM config: throttling enabled. Use a large interval to guarantee
	// the second draw falls within the throttle window regardless of test timing.
	g.drawMinInterval = time.Hour

	// First draw initializes frame buffer
	screen1 := ebitenImage(1600, 900)
	g.Draw(screen1)
	if g.frameBuffer == nil {
		t.Fatalf("frame buffer must be initialized when throttling is enabled")
	}

	// Mark the frame buffer with a known color
	g.frameBuffer.Fill(color.RGBA{255, 0, 0, 255})
	g.drawThrottleCopies = 0

	// Second draw within throttle interval should copy from frame buffer
	screen2 := ebitenImage(1600, 900)
	g.Draw(screen2)
	if g.drawThrottleCopies != 1 {
		t.Fatalf("throttled draw should copy frame buffer; copies=%d", g.drawThrottleCopies)
	}
}

// TestDrawThrottleResizeForcesDraw verifies that when a resize invalidates the
// frame buffer (sets it to nil), the next Draw call falls through to a full draw
// even within the throttle window, instead of returning blank.
func TestDrawThrottleResizeForcesDraw(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drawMinInterval = time.Hour

	// First draw initializes frame buffer
	screen1 := ebitenImage(640, 480)
	g.Draw(screen1)
	if g.frameBuffer == nil {
		t.Fatalf("expected frame buffer initialized after first draw")
	}

	// Resize invalidates frame buffer (simulating orientation change)
	g.Layout(800, 600)
	if g.frameBuffer != nil {
		t.Fatalf("expected frame buffer nil after resize")
	}

	// Draw again within throttle window — should do a full draw, not blank
	screen2 := ebitenImage(800, 600)
	g.Draw(screen2)
	if g.frameBuffer == nil {
		t.Fatalf("expected frame buffer re-created after draw with invalidated buffer")
	}
	if g.frameBufferW != 800 || g.frameBufferH != 600 {
		t.Fatalf("frame buffer dimensions wrong: got %dx%d, want 800x600", g.frameBufferW, g.frameBufferH)
	}
}

// ebitenImage is a tiny shim that creates a stubbed ebiten.Image for testing.
func ebitenImage(w, h int) *ebiten.Image {
	img := ebiten.NewImage(w, h)
	img.Fill(color.RGBA{0, 0, 0, 0})
	return img
}
