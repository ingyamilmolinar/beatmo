//go:build test

package ui

import "github.com/hajimehoshi/ebiten/v2"

// logLineFinal is linked into the package during -tags test builds so that
// widgets.go can call it even when the non-test file is excluded.
func logLineFinal(m ebiten.GeoM, thick float64) {}
