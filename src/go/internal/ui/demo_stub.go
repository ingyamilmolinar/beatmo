//go:build test

package ui

// buildDemo is a no-op during test builds so unit tests control their own setup
// without automatic demo construction.
func (g *Game) buildDemo() {}

// RunDemo is a no-op stub in test builds.
func (g *Game) RunDemo() {}
