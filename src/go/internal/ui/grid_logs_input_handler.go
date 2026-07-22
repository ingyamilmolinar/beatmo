package ui

import "image"

// gridLogsInputZ is the z-index of the logs-button input handler in the grid
// pane's inputDispatcher. It sits just below the folder button (gridFileInputZ)
// but still above every content handler (drum=100, splitter=150, sidebar=200).
// The gear, folder, and logs rects are disjoint, so their relative order never
// decides a hit — the ordering only matters versus lower content handlers.
const gridLogsInputZ = gridFileInputZ - 1

// gridLogsInputHandler routes pointer input for the logs button through the grid
// pane's z-ordered inputDispatcher, mirroring gridFileInputHandler /
// gridHelpInputHandler. One path serves desktop mouse, mobile touch, and mobile
// pointer/click uniformly.
type gridLogsInputHandler struct{ g *Game }

func (h gridLogsInputHandler) InputBounds() image.Rectangle {
	return h.g.gridLogsButtonRect()
}

func (h gridLogsInputHandler) ZIndex() int { return gridLogsInputZ }

func (h gridLogsInputHandler) HandleInput(x, y int, pressed bool) InputResult {
	g := h.g
	if g.gridLogsBtn == nil {
		return InputIgnored
	}
	g.gridLogsBtn.SetRect(g.gridLogsButtonRect())
	res := g.gridLogsBtn.HandleInputResult(x, y, pressed)
	if res == InputConsumed {
		// Defer the DrumViewTree for this frame so its click-outside logic
		// doesn't immediately close the panel the logs button just opened
		// (the logs rect lies outside every tree hit area). Reuses the same
		// flag as the gear/folder.
		g.gridHelpCapturing = true
	}
	return res
}

func (h gridLogsInputHandler) HandleWheel(x, y, steps int) InputResult { return InputIgnored }

func (h gridLogsInputHandler) Capturing() bool { return false }
