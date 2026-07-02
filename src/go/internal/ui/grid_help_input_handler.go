package ui

import "image"

// gridHelpInputZ is the z-index of the settings-gear input handler in the grid
// pane's inputDispatcher. It sits ABOVE every other registered handler
// (drum=100, splitter=150, sidebar=200) so a press inside the gear's rect always
// wins on overlap and is never swallowed by a lower handler.
const gridHelpInputZ = 300

// gridHelpInputHandler routes pointer input for the settings-gear button through
// the grid pane's z-ordered inputDispatcher — the same infra every other grid
// control uses — instead of the bespoke, platform-gated path that previously
// lived in Game.Update. Because the dispatcher runs on the touch-to-mouse
// override coordinates, this one path serves desktop mouse, mobile touch, and
// mobile pointer/click uniformly. The gear Button's ConsumeOnPress flag sets
// suppressClicksUntilRelease on the press edge, which the release-time
// GestureTap path (handleTapInGrid) honors to avoid a double-fire.
type gridHelpInputHandler struct{ g *Game }

func (h gridHelpInputHandler) InputBounds() image.Rectangle {
	return h.g.gridHelpButtonRect()
}

func (h gridHelpInputHandler) ZIndex() int { return gridHelpInputZ }

func (h gridHelpInputHandler) HandleInput(x, y int, pressed bool) InputResult {
	g := h.g
	if g.gridHelpBtn == nil {
		return InputIgnored
	}
	// Keep the button's internal rect in sync with the live layout so the
	// press-edge hit test matches InputBounds.
	g.gridHelpBtn.SetRect(g.gridHelpButtonRect())
	res := g.gridHelpBtn.HandleInputResult(x, y, pressed)
	if res == InputConsumed {
		// Defer the DrumViewTree for this frame so its click-outside logic
		// doesn't immediately close the overlay the gear just opened (the gear
		// rect lies outside every tree hit area).
		g.gridHelpCapturing = true
	}
	return res
}

func (h gridHelpInputHandler) HandleWheel(x, y, steps int) InputResult { return InputIgnored }

func (h gridHelpInputHandler) Capturing() bool { return false }
