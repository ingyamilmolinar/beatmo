package ui

import "image"

// gridFileInputZ is the z-index of the folder-button input handler in the grid
// pane's inputDispatcher. It sits just below the settings gear (gridHelpInputZ)
// but still above every content handler (drum=100, splitter=150, sidebar=200).
// The gear and folder rects are disjoint, so their relative order never decides
// a hit — the ordering only matters versus lower content handlers.
const gridFileInputZ = gridHelpInputZ - 1

// gridFileInputHandler routes pointer input for the folder button through the
// grid pane's z-ordered inputDispatcher, mirroring gridHelpInputHandler for the
// settings gear. One path serves desktop mouse, mobile touch, and mobile
// pointer/click uniformly.
type gridFileInputHandler struct{ g *Game }

func (h gridFileInputHandler) InputBounds() image.Rectangle {
	return h.g.gridFileButtonRect()
}

func (h gridFileInputHandler) ZIndex() int { return gridFileInputZ }

func (h gridFileInputHandler) HandleInput(x, y int, pressed bool) InputResult {
	g := h.g
	if g.gridFileBtn == nil {
		return InputIgnored
	}
	g.gridFileBtn.SetRect(g.gridFileButtonRect())
	res := g.gridFileBtn.HandleInputResult(x, y, pressed)
	if res == InputConsumed {
		// Defer the DrumViewTree for this frame so its click-outside logic
		// doesn't immediately close the menu the folder button just opened
		// (the folder rect lies outside every tree hit area). Reuses the same
		// flag as the gear.
		g.gridHelpCapturing = true
	}
	return res
}

func (h gridFileInputHandler) HandleWheel(x, y, steps int) InputResult { return InputIgnored }

func (h gridFileInputHandler) Capturing() bool { return false }
