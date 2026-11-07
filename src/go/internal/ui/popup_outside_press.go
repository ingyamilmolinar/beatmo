package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// The ONE grid "tap outside a menu" rule: while a menu is open, a grid press outside it only closes
// it. The open pop-up OWNS the outside press, so the release-time tap never creates a node beneath
// (on mobile this tap is the close). Shift+press and group badges pass through.

// Above every grid-local pop-up, below the top-right cluster so a corner button still switches
// menus; presses inside a pop-up fall through to it.
const popupOutsidePressInputZ = 250

// Empty InputBounds while nothing is open, so it costs nothing.
type popupOutsidePressHandler struct{ g *Game }

func (g *Game) gridLocalPopupOpen() bool {
	return (g.sidebar != nil && g.sidebar.IsOpen()) || (g.groupMenu != nil && g.groupMenu.IsOpen())
}

func (g *Game) anyPopupOwnsInput() bool {
	return g.modalOverlayActive() || g.gridLocalPopupOpen()
}

// closeGridLocalPopups dismisses the grid-anchored pop-ups.
func (g *Game) closeGridLocalPopups() {
	if g.sidebar != nil && g.sidebar.IsOpen() {
		g.sidebar.Close()
	}
	if g.groupMenu != nil && g.groupMenu.IsOpen() {
		g.groupMenu.Close()
	}
}

func (g *Game) insideGridLocalPopup(x, y int) bool {
	p := image.Pt(x, y)
	// Hit, not InputBounds: with a picker open the bounds cover grid that is visibly outside both.
	if g.sidebar != nil && g.sidebar.IsOpen() && g.sidebar.Hit(x, y) {
		return true
	}
	if g.groupMenu != nil && g.groupMenu.IsOpen() && p.In(g.groupMenu.InputBounds()) {
		return true
	}
	return false
}

func (h popupOutsidePressHandler) InputBounds() image.Rectangle {
	g := h.g
	if !g.anyPopupOwnsInput() {
		return image.Rectangle{}
	}
	return g.split.GridRect(g.winW, g.winH)
}

func (h popupOutsidePressHandler) ZIndex() int { return popupOutsidePressInputZ }

func (h popupOutsidePressHandler) HandleInput(x, y int, pressed bool) InputResult {
	g := h.g
	if image.Pt(x, y).In(g.split.InputBounds()) {
		// Pane chrome, not the canvas.
		return InputIgnored
	}
	if g.modalOverlayActive() {
		// The portal closes itself on the press; this only owns it so the grid never sees it.
		return InputConsumed
	}
	if g.insideGridLocalPopup(x, y) || !pressed || !g.inputDispatcher.PressEdge() {
		return InputIgnored
	}
	if isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight) {
		// handleEditor's shift block owns connect/marquee, and the marquee needs the group menu open to
		// discard its provisional group.
		return InputIgnored
	}
	if _, ok := g.groupBadgeAt(x, y); ok {
		return InputIgnored
	}
	g.closeGridLocalPopups()
	return InputConsumed
}

func (h popupOutsidePressHandler) HandleWheel(x, y, steps int) InputResult { return InputIgnored }

func (h popupOutsidePressHandler) Capturing() bool { return false }
