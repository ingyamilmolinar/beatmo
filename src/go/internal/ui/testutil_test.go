//go:build test

package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
)

// click simulates a mouse click at (x,y) and releases it on the next frame.
func click(g *Game, x, y int) {
	restore := SetInputForTest(
		func() (int, int) { return x, y },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(k ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	g.drum.Update()
	restore()
	g.drum.Update()
}
