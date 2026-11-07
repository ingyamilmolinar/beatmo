package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// Function variables, never direct Ebiten calls: a single touch maps onto the mouse so every
// mouse handler works on touch. updateTouchOverride runs once per frame, after globalTouchState.Update
// and before any cursor read; call resetTouchOverride alongside SetInputForTest.
var (
	_ebCursorPosition       = ebiten.CursorPosition
	_ebIsMouseButtonPressed = ebiten.IsMouseButtonPressed
)

var setCursorShape = ebiten.SetCursorShape

var (
	cursorPosition = func() (int, int) {
		if touchOverrideActive {
			return touchOverrideX, touchOverrideY
		}
		return _ebCursorPosition()
	}
	isMouseButtonPressed = func(b ebiten.MouseButton) bool {
		if touchOverrideActive && b == ebiten.MouseButtonLeft && touchOverrideLeft {
			return true
		}
		return _ebIsMouseButtonPressed(b)
	}
	isKeyPressed       = ebiten.IsKeyPressed
	isKeyJustPressed   = inpututil.IsKeyJustPressed
	inputChars         = ebiten.InputChars //nolint:staticcheck // deprecated Ebiten API, migration tracked separately
	wheel              = ebiten.Wheel
	screenSize         = ebiten.ScreenSizeInFullscreen //nolint:staticcheck // deprecated Ebiten API, migration tracked separately
	touchIDs           = ebiten.TouchIDs               //nolint:staticcheck // deprecated Ebiten API, migration tracked separately
	touchPosition      = ebiten.TouchPosition
	inputForTestActive bool
)

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
	suppressClicksUntilRelease = false
	inputForTestActive = true
	resetTouchOverride()
	return func() {
		cursorPosition = oldCursor
		isMouseButtonPressed = oldMouse
		isKeyPressed = oldKey
		inputChars = oldChars
		wheel = oldWheel
		screenSize = oldScreen
		suppressClicksUntilRelease = false
		inputForTestActive = false
		resetTouchOverride()
	}
}

// For tests that read through the real wrappers: a prior test whose stub restore never ran must
// not leak a stale mock into them.
func resetInputForTest() {
	_ebCursorPosition = ebiten.CursorPosition
	_ebIsMouseButtonPressed = ebiten.IsMouseButtonPressed
	cursorPosition = func() (int, int) {
		if touchOverrideActive {
			return touchOverrideX, touchOverrideY
		}
		return _ebCursorPosition()
	}
	isMouseButtonPressed = func(b ebiten.MouseButton) bool {
		if touchOverrideActive && b == ebiten.MouseButtonLeft && touchOverrideLeft {
			return true
		}
		return _ebIsMouseButtonPressed(b)
	}
	isKeyPressed = ebiten.IsKeyPressed
	isKeyJustPressed = inpututil.IsKeyJustPressed
	inputChars = ebiten.InputChars //nolint:staticcheck // deprecated Ebiten API, migration tracked separately
	wheel = ebiten.Wheel
	screenSize = ebiten.ScreenSizeInFullscreen //nolint:staticcheck // deprecated Ebiten API, migration tracked separately
	// SetTouchForTest owns the touch mocks with its own restore, so call order does not matter.
	inputForTestActive = false
}

// SetTouchForTest replaces touch input functions during tests.
func SetTouchForTest(
	ids func() []ebiten.TouchID,
	pos func(ebiten.TouchID) (int, int),
) func() {
	oldIDs := touchIDs
	oldPos := touchPosition
	touchIDs = ids
	touchPosition = pos
	return func() {
		touchIDs = oldIDs
		touchPosition = oldPos
	}
}

// Wraps the current readers, so it composes with SetInputForTest. The single place a key is
// masked: never add an inline "is a popup open?" check at a shortcut site.
func suppressKeys(blocked map[ebiten.Key]bool) func() {
	if len(blocked) == 0 {
		return func() {}
	}
	prevPressed, prevJust := isKeyPressed, isKeyJustPressed
	isKeyPressed = func(k ebiten.Key) bool { return !blocked[k] && prevPressed(k) }
	isKeyJustPressed = func(k ebiten.Key) bool { return !blocked[k] && prevJust(k) }
	return func() { isKeyPressed, isKeyJustPressed = prevPressed, prevJust }
}
