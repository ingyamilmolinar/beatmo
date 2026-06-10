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

// screenshotReady returns true once enough draws have occurred for the UI
// to be fully rendered. Default threshold is 90 frames (~1.5s @ 60fps);
// scenes with slow-settling caches override via SetScreenshotSettleFrames.
func (g *Game) screenshotReady() bool {
	threshold := g.screenshotSettleFrames
	if threshold <= 0 {
		threshold = 90
	}
	return g.screenshotPath != "" && g.screenshotDraws >= threshold
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
