//go:build test

package ui

import "testing"

// Update (which ends the run via screenshotReady) precedes Draw (which counts and captures) in each frame, as
// in Ebiten; only the production decision methods are used. Returns the capture draw (0 = never) and the end frame (-1 = never).
func simulateScreenshotCapture(g *Game) (captureFrame, terminateFrame int) {
	g.screenshotCaptured = false
	g.screenshotDraws = 0
	for frame := 1; frame <= 100000; frame++ {
		if g.screenshotReady() { // Update phase
			return captureFrame, frame
		}
		g.screenshotDraws++ // Draw phase
		if g.shouldCaptureScreenshot() {
			captureFrame = g.screenshotDraws
			g.screenshotCaptured = true
		}
	}
	return captureFrame, -1
}

// An explicit SettleFrames wins; 0 falls back to 90.
func TestScreenshotThreshold(t *testing.T) {
	g := &Game{}
	if got := g.screenshotThreshold(); got != 90 {
		t.Fatalf("default threshold = %d, want 90", got)
	}
	g.screenshotSettleFrames = 60
	if got := g.screenshotThreshold(); got != 60 {
		t.Fatalf("override threshold = %d, want 60", got)
	}
	g.screenshotSettleFrames = 150
	if got := g.screenshotThreshold(); got != 150 {
		t.Fatalf("override threshold = %d, want 150", got)
	}
}

// The capture fires exactly at the scene's settle threshold, always fires, and precedes termination.
func TestScreenshotCapturesAtSettleThreshold(t *testing.T) {
	for _, settle := range []int{0, 60, 90, 120, 150} {
		want := settle
		if want == 0 {
			want = 90
		}
		g := &Game{screenshotPath: "out.png", screenshotSettleFrames: settle}
		cf, tf := simulateScreenshotCapture(g)
		if cf == 0 {
			t.Errorf("settle=%d: capture never fired (no PNG would be written)", settle)
			continue
		}
		if cf != want {
			t.Errorf("settle=%d: captured at frame %d, want %d", settle, cf, want)
		}
		if tf <= cf {
			t.Errorf("settle=%d: terminated at frame %d, at/before capture frame %d", settle, tf, cf)
		}
	}
}

// Termination waits for the capture, not merely the draw count, so the two can never diverge.
func TestScreenshotReadyOnlyAfterCapture(t *testing.T) {
	g := &Game{screenshotPath: "out.png", screenshotSettleFrames: 60}
	g.screenshotDraws = 500 // far past the threshold
	if g.screenshotReady() {
		t.Fatal("screenshotReady true before capture")
	}
	g.screenshotCaptured = true
	if !g.screenshotReady() {
		t.Fatal("screenshotReady false after capture")
	}
}

// Interactive and bench runs must never capture or self-terminate.
func TestScreenshotNoCaptureWithoutPath(t *testing.T) {
	g := &Game{screenshotSettleFrames: 60, screenshotDraws: 500}
	if g.shouldCaptureScreenshot() {
		t.Fatal("shouldCaptureScreenshot true without a path")
	}
	if g.screenshotReady() {
		t.Fatal("screenshotReady true without a path")
	}
}
