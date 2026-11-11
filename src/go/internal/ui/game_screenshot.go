//go:build !test

package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

// SetScreenshot configures the game to capture a screenshot after the UI
// has settled (90 draw frames ≈ 1.5s at 60fps) and save it to path.
// The game exits cleanly via ebiten.Termination after saving.
func (g *Game) SetScreenshot(path string) {
	g.screenshotPath = path
	g.demoScheduled = true
}

// screenshotReady reports whether the game should terminate after a screenshot.
// It is gated on the capture having actually fired (screenshotCaptured, set by
// Draw at screenshotThreshold) rather than on the draw count directly, so the
// game can never exit before the capture frame — the root cause of the
// SettleFrames<90 "no PNG" bug.
func (g *Game) screenshotReady() bool {
	return g.screenshotPath != "" && g.screenshotCaptured
}

// SetScreenshotSettleFrames overrides the default 90-frame wait before
// captureScreen fires. Used by scenes whose overlays/caches need extra time.
func (g *Game) SetScreenshotSettleFrames(n int) { g.screenshotSettleFrames = n }

// SetScreenshotSubject configures captureScreen to crop the PNG to the
// requested subject's on-screen bounds. Pass SubjectFullScreen (the empty
// Subject) to disable cropping. Resolved via (*Game).SubjectRect.
func (g *Game) SetScreenshotSubject(s Subject) { g.screenshotSubject = s }

// captureScreen reads pixels from the Ebiten screen and saves as PNG.
// When a non-empty screenshotSubject is set, the encoded image is cropped
// to that subject's bounds (resolved via SubjectRect). If the subject is
// not currently visible, captureScreen returns an error so the harness
// fails fast instead of writing a misleading full-screen PNG.
func (g *Game) captureScreen(screen *ebiten.Image) error {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	pix := make([]byte, 4*w*h)
	screen.ReadPixels(pix)

	full := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(full.Pix, pix)

	out := image.Image(full)
	if g.screenshotSubject != SubjectFullScreen {
		rect, ok := g.SubjectRect(g.screenshotSubject)
		if !ok || rect.Empty() {
			return fmt.Errorf("screenshot subject %q not visible — check Setup", g.screenshotSubject)
		}
		clipped := rect.Intersect(image.Rect(0, 0, w, h))
		if clipped.Empty() {
			return fmt.Errorf("screenshot subject %q rect %v outside framebuffer", g.screenshotSubject, rect)
		}
		out = full.SubImage(clipped)
	}

	f, err := os.Create(g.screenshotPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, out)
}
