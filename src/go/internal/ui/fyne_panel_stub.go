//go:build !fyne

package ui

// RunFynePanel is a no-op on unsupported platforms.
func RunFynePanel(g *Game) {}
