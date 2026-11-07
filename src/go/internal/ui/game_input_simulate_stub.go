//go:build test

package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Tests use SetInputForTest instead.

func (g *Game) SimulateClickAt(int, int)                   {}
func (g *Game) SimulateDrag(image.Point, image.Point, int) {}
func (g *Game) SimulateKey(ebiten.Key)                     {}
func (g *Game) SimulateScroll(int, int, float64)           {}
