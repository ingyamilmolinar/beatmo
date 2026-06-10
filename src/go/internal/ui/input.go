package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// ─── Input System Architecture ──────────────────────────────────────────────
//
// cursorPosition and isMouseButtonPressed are FUNCTION VARIABLES, not direct
// Ebiten calls. They wrap Ebiten with touch-override logic so that a single
// active touch transparently maps to mouse input, allowing all 50+ existing
// mouse-based handlers (DrumView, Splitter, Camera, Editor, etc.) to work
// with touch without modification.
//
// Platform behavior:
//
//	Desktop:                cursorPosition/isMouseButtonPressed → Ebiten directly
//	Mobile WASM (1 touch):  → touch override (primary touch X,Y; left=true)
//	Mobile WASM (2+ touch): → Ebiten (falls through; multi-touch handled as gestures)
//	Mobile WASM (tap):      → 2-frame injection cycle (see touch.go injectTouchTap)
//
// The touch override is FRAME-SCOPED: updateTouchOverride() must run once per
// frame, after globalTouchState.Update() and before any cursorPosition() reads.
// Reordering input processing in game_update.go MUST preserve this sequence.
//
// When inputForTestActive is set (via SetInputForTest), the override is suppressed
// so test mocks control input exclusively. Always call resetTouchOverride()
// alongside SetInputForTest.
//
// Multi-touch gestures (pinch, two-finger pan, long-press) bypass the override
// entirely — they are handled as direct gesture events in game_update.go.
//
// IMPORTANT: Never call ebiten.CursorPosition() directly in UI code. Always use
// the package-level cursorPosition(). Raw Ebiten functions are stored as
// _ebCursorPosition / _ebIsMouseButtonPressed.
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
	isKeyPressed     = ebiten.IsKeyPressed
	isKeyJustPressed = inpututil.IsKeyJustPressed
	inputChars       = ebiten.InputChars //nolint:staticcheck // deprecated Ebiten API, migration tracked separately
	wheel              = ebiten.Wheel
	screenSize         = ebiten.ScreenSizeInFullscreen //nolint:staticcheck // deprecated Ebiten API, migration tracked separately
	touchIDs           = ebiten.TouchIDs               //nolint:staticcheck // deprecated Ebiten API, migration tracked separately
	touchPosition      = ebiten.TouchPosition
	inputForTestActive bool
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
