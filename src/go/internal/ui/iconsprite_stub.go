//go:build test

package ui

import "github.com/hajimehoshi/ebiten/v2"

// nil makes Button.Draw use the drawRect icon path, which tests intercept.
func iconSprite(_ string, _, _ int) *ebiten.Image { return nil }
