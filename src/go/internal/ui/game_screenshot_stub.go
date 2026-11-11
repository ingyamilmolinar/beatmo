//go:build test

package ui

import "github.com/hajimehoshi/ebiten/v2"

func (g *Game) SetScreenshot(path string) {
	g.screenshotPath = path
	g.demoScheduled = true
}

func (g *Game) screenshotReady() bool {
	return g.screenshotPath != "" && g.screenshotCaptured
}

func (g *Game) SetScreenshotSettleFrames(n int) { g.screenshotSettleFrames = n }

func (g *Game) SetScreenshotSubject(s Subject) { g.screenshotSubject = s }

func (g *Game) captureScreen(_ *ebiten.Image) error {
	return nil
}
