//go:build !test

package ui

import (
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
// to be fully rendered.
func (g *Game) screenshotReady() bool {
	return g.screenshotPath != "" && g.screenshotDraws >= 90
}

// captureScreen reads pixels from the Ebiten screen and saves as PNG.
func (g *Game) captureScreen(screen *ebiten.Image) error {
	b := screen.Bounds()
	w, h := b.Dx(), b.Dy()
	pix := make([]byte, 4*w*h)
	screen.ReadPixels(pix)

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, pix)

	f, err := os.Create(g.screenshotPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
