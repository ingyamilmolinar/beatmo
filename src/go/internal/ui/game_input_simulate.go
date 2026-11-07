//go:build !test

package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// Production-only simulators for scene setup (tests use SetInputForTest). Call before
// ebiten.RunGame; each runs its own Update ticks, so never interleave with the live loop.

func (g *Game) SimulateClickAt(x, y int) {
	restore := overrideMouse(x, y, true)
	g.Update()
	restore()
	restore = overrideMouse(x, y, false)
	g.Update()
	restore()
}

func (g *Game) SimulateDrag(from, to image.Point, frames int) {
	if frames < 2 {
		frames = 2
	}
	// Press at start position.
	restore := overrideMouse(from.X, from.Y, true)
	g.Update()
	restore()
	// Drag across intermediate positions.
	for i := 1; i <= frames; i++ {
		t := float64(i) / float64(frames)
		x := from.X + int(float64(to.X-from.X)*t)
		y := from.Y + int(float64(to.Y-from.Y)*t)
		restore = overrideMouse(x, y, true)
		g.Update()
		restore()
	}
	// Release at end.
	restore = overrideMouse(to.X, to.Y, false)
	g.Update()
	restore()
}

func (g *Game) SimulateKey(k ebiten.Key) {
	restore := overrideKey(k, true)
	g.Update()
	restore()
	restore = overrideKey(k, false)
	g.Update()
	restore()
}

// SimulateScroll simulates a mouse-wheel scroll at (x, y) for one Update tick.
func (g *Game) SimulateScroll(x, y int, dy float64) {
	restoreM := overrideMouse(x, y, false)
	restoreW := overrideWheel(0, dy)
	g.Update()
	restoreW()
	restoreM()
}

func overrideMouse(x, y int, pressed bool) func() {
	oldCursor := cursorPosition
	oldMouse := isMouseButtonPressed
	cursorPosition = func() (int, int) { return x, y }
	isMouseButtonPressed = func(b ebiten.MouseButton) bool {
		return pressed && b == ebiten.MouseButtonLeft
	}
	suppressClicksUntilRelease = false
	inputForTestActive = true
	resetTouchOverride()
	return func() {
		cursorPosition = oldCursor
		isMouseButtonPressed = oldMouse
		inputForTestActive = false
		resetTouchOverride()
	}
}

func overrideKey(k ebiten.Key, pressed bool) func() {
	old := isKeyPressed
	isKeyPressed = func(key ebiten.Key) bool { return pressed && key == k }
	return func() { isKeyPressed = old }
}

func overrideWheel(dx, dy float64) func() {
	old := wheel
	wheel = func() (float64, float64) { return dx, dy }
	return func() { wheel = old }
}
