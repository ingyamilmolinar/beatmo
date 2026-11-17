//go:build test

package ui

import "testing"

// simulateScreenshotCapture drives g through the real Ebiten frame ordering —
// Update() (which terminates via screenshotReady) runs before Draw() (which
// increments screenshotDraws and may capture) within each frame. It returns the
// draw count at which the capture fired (0 = never) and the frame at which the
// loop terminated (-1 = never). It uses only the production decision methods so
// it pins the actual capture/termination coupling.
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

// TestScreenshotThreshold pins the settle-frame resolution: an explicit
// SettleFrames override wins; 0 falls back to the 90-frame default.
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

// TestScreenshotCapturesAtSettleThreshold is the core regression guard for the
// hardcoded-frame-90 bug. The capture must fire exactly at the scene's settle
// threshold (never the old hardcoded 90), must actually fire (the SettleFrames<90
// "no PNG" bug), and must precede termination (the loop must not exit before the
// capture frame).
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

// TestScreenshotReadyOnlyAfterCapture verifies termination is gated on the
// capture having happened, not merely on the draw count — so the two predicates
// can never diverge into "terminate before capture".
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

// TestScreenshotNoCaptureWithoutPath verifies screenshot mode is inert when no
// path is configured (normal interactive / bench runs must never capture or
// self-terminate).
func TestScreenshotNoCaptureWithoutPath(t *testing.T) {
	g := &Game{screenshotSettleFrames: 60, screenshotDraws: 500}
	if g.shouldCaptureScreenshot() {
		t.Fatal("shouldCaptureScreenshot true without a path")
	}
	if g.screenshotReady() {
		t.Fatal("screenshotReady true without a path")
	}
}
