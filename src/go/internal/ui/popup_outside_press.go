package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// popup_outside_press.go — the ONE "tap outside a menu" rule for the grid pane.
//
// Contract, for every menu and pop-up on every platform: while one is open, a
// press on the grid canvas outside it CLOSES it and does nothing else. The
// press that dismissed the pop-up owns its whole gesture, so the release-time
// GestureTap / long-press (which the grid recognises at the LIFT point, outside
// this dispatcher) never also creates or selects a node beneath it. Mobile has
// no close × (by design), so this outside tap is the mobile close.
//
// Before this existed each pop-up family answered the question on its own and
// they disagreed:
//   - portal menus (theme / file / settings / logs / row context / nav / confirm
//     dialogs) closed on the PRESS via the DrumViewTree's click-outside, and the
//     grid's release-time tap only checked "is a menu open NOW" — by the lift
//     the menu was gone, so a mobile outside tap closed the menu AND created a
//     node (desktop survived because handleEditor saw the menu on the press).
//   - the node sidebar closed on the release through handleTapInGrid (touch)
//     plus a private closedGuard frame counter, but a desktop click outside it
//     created a node.
//   - the group menu had no outside rule at all: the tap fell through to
//     node creation, which closed the menu as a side effect.
//
// Now the open pop-up becomes the OWNER of the outside press in the grid
// InputDispatcher (the same press-edge latch the top-right buttons use —
// GestureOwnedByHandler), so the existing release-time gates do the rest.
//
// Two grid gestures deliberately pass through a grid-local pop-up (they never
// create a node, and both existed before this rule): a Shift+press is a
// connect / marquee gesture (passed through untouched to handleEditor's shift
// block, which resolves the open pop-up itself), and a press on a group BADGE switches to
// that group's menu (like a top-right button switching menus).

// popupOutsidePressInputZ sits above every grid-local pop-up (sidebar=200,
// group menu=210) so it sees the press before the canvas does, and below the
// top-right cluster (gridCornerButtonsInputZ=300) so a corner button still
// toggles / switches menus while one is open. Presses INSIDE a grid-local
// pop-up are declined here and fall through to that pop-up.
const popupOutsidePressInputZ = 250

// popupOutsidePressHandler owns presses on the grid pane outside the open
// menu / pop-up. Registered permanently; its InputBounds is empty while
// nothing is open, so it costs nothing then.
type popupOutsidePressHandler struct{ g *Game }

// gridLocalPopupOpen reports whether a grid-anchored pop-up that lives outside
// the portal (node sidebar, group menu) is open.
func (g *Game) gridLocalPopupOpen() bool {
	return (g.sidebar != nil && g.sidebar.IsOpen()) || (g.groupMenu != nil && g.groupMenu.IsOpen())
}

// anyPopupOwnsInput reports whether ANY menu or pop-up is open that an outside
// press must dismiss: a portal overlay (modalOverlayActive) or a grid-local one.
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

// insideGridLocalPopup reports whether (x, y) lies on an open grid-local
// pop-up's own interactive area.
func (g *Game) insideGridLocalPopup(x, y int) bool {
	p := image.Pt(x, y)
	// Hit, not InputBounds: with a grid picker open the sidebar's InputBounds
	// is the BOUNDING BOX of card ∪ picker, which covers grid the user can see
	// is outside both (on a phone: the strip beside the top-right buttons —
	// the only "outside" a full-width picker leaves).
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
		// The splitter's grab strip reaches into the grid rect; it is pane
		// chrome, not the canvas, and keeps its own behaviour.
		return InputIgnored
	}
	if g.modalOverlayActive() {
		// A portal overlay is drawn above the whole grid pane and closes itself
		// on the press (the DrumViewTree's click-outside, which also honours
		// must-answer dialogs). Here it only has to OWN the press, so the grid
		// beneath — node create/select, long-press delete — never sees it.
		return InputConsumed
	}
	if g.insideGridLocalPopup(x, y) || !pressed || !g.inputDispatcher.PressEdge() {
		return InputIgnored
	}
	if isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight) {
		// Untouched: handleEditor's shift block owns connect / marquee, and the
		// marquee needs the group menu still open to discard its provisional
		// group (TestSecondMarqueeDiscardsPriorProvisionalGroup).
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
