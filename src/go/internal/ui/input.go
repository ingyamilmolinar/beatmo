package ui

import "github.com/hajimehoshi/ebiten/v2"

var (
	cursorPosition       = ebiten.CursorPosition
	isMouseButtonPressed = ebiten.IsMouseButtonPressed
	isKeyPressed         = ebiten.IsKeyPressed
	inputChars           = ebiten.InputChars
	wheel                = ebiten.Wheel
	screenSize           = ebiten.ScreenSizeInFullscreen
)

// SetInputForTest replaces input functions during tests and returns a function
// to restore the originals.
func SetInputForTest(
	cursor func() (int, int),
	mouse func(ebiten.MouseButton) bool,
	key func(ebiten.Key) bool,
	chars func() []rune,
	wh func() (float64, float64),
	screen func() (int, int),
) func() {
	oldCursor := cursorPosition
	oldMouse := isMouseButtonPressed
	oldKey := isKeyPressed
	oldChars := inputChars
	oldWheel := wheel
	oldScreen := screenSize
	cursorPosition = cursor
	isMouseButtonPressed = mouse
	isKeyPressed = key
	inputChars = chars
	wheel = wh
	screenSize = screen
	return func() {
		cursorPosition = oldCursor
		isMouseButtonPressed = oldMouse
		isKeyPressed = oldKey
		inputChars = oldChars
		wheel = oldWheel
		screenSize = oldScreen
	}
}
