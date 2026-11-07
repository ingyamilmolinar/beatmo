//go:build !test

package ui

import (
	"fmt"
	"image"
	"image/png"
	"os"

	"github.com/hajimehoshi/ebiten/v2"
)

// Exits through ebiten.Termination after saving.
func (g *Game) SetScreenshot(path string) {
	g.screenshotPath = path
	g.demoScheduled = true
}

// Gated on the capture having fired, never the draw count, so the game cannot exit before it.
func (g *Game) screenshotReady() bool {
	return g.screenshotPath != "" && g.screenshotCaptured
}

func (g *Game) SetScreenshotSettleFrames(n int) { g.screenshotSettleFrames = n }

// SubjectFullScreen disables cropping.
func (g *Game) SetScreenshotSubject(s Subject) { g.screenshotSubject = s }

// An invisible subject is an error, never a misleading full-screen PNG.
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
