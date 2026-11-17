package ui

import "github.com/hajimehoshi/ebiten/v2"

// tourDriver is the seam for the scripted showcase tour that records the
// landing page's video (tour.go, desktop only; spec
// docs/superpowers/specs/2026-10-23-landing-page-design.md). tick runs at the
// top of Game.Update and installs that frame's scripted input; it returns true
// when the tour has finished. frame receives every finished frame from Draw.
type tourDriver interface {
	tick(g *Game) bool
	frame(g *Game, screen *ebiten.Image)
}
