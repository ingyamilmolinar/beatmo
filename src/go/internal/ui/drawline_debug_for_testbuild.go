//go:build test

package ui

import "github.com/hajimehoshi/ebiten/v2"

// Linked under -tags test so widgets.go compiles without the non-test file.
func logLineFinal(m ebiten.GeoM, thick float64) {}
