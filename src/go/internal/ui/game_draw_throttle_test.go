package ui

import (
	"image/color"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
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
	g.Layout(640, 480)

	// Simulate WASM config: throttling enabled
	g.drawMinInterval = 40 * time.Millisecond

	// First draw initializes frame buffer
	screen1 := ebitenImage(640, 480)
	g.Draw(screen1)
	if g.frameBuffer == nil {
		t.Fatalf("frame buffer must be initialized when throttling is enabled")
	}

	// Mark the frame buffer with a known color
	g.frameBuffer.Fill(color.RGBA{255, 0, 0, 255})
	g.drawThrottleCopies = 0

	// Second draw within throttle interval should copy from frame buffer
	screen2 := ebitenImage(640, 480)
	g.Draw(screen2)
	if g.drawThrottleCopies != 1 {
		t.Fatalf("throttled draw should copy frame buffer; copies=%d", g.drawThrottleCopies)
	}
}

// ebitenImage is a tiny shim so this test stays build-tag agnostic.
func ebitenImage(w, h int) *ebiten.Image {
	img := ebiten.NewImage(w, h)
	img.Fill(color.RGBA{0, 0, 0, 0})
	return img
}
