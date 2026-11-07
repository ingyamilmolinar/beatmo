package ui

import "github.com/hajimehoshi/ebiten/v2"

// tick installs the frame's scripted input at the top of Update and reports when the tour is done; frame gets each
// finished frame from Draw.
type tourDriver interface {
	tick(g *Game) bool
	frame(g *Game, screen *ebiten.Image)
}
