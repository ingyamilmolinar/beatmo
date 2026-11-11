//go:build test

package ui

import "github.com/hajimehoshi/ebiten/v2"

// iconSprite is a no-op stub for test builds. Returning nil makes Button.Draw
// fall through to the original drawRect-based icon switch, preserving drawRect
// override interception in tests.
func iconSprite(_ string, _, _ int) *ebiten.Image { return nil }
