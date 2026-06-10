//go:build test

package ui

import "github.com/hajimehoshi/ebiten/v2"

func (g *Game) SetScreenshot(path string) {
	g.screenshotPath = path
	g.demoScheduled = true
}

func (g *Game) screenshotReady() bool {
	threshold := g.screenshotSettleFrames
	if threshold <= 0 {
		threshold = 90
	}
	return g.screenshotPath != "" && g.screenshotDraws >= threshold
}

func (g *Game) SetScreenshotSettleFrames(n int) { g.screenshotSettleFrames = n }

func (g *Game) SetScreenshotSubject(s Subject) { g.screenshotSubject = s }

func (g *Game) captureScreen(_ *ebiten.Image) error {
	return nil
}
