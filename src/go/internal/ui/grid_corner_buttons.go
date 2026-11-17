package ui

import "image"

// The top-right grid-pane button cluster (settings gear, folder, logs, theme)
// is ONE component: one table, one input handler, one hit geometry.
//
// It used to be four copy-pasted input handlers plus four parallel per-button
// lists (layout, draw, toggle-reconcile, reserved zone). They drifted: the
// theme button never made it into the toggle-reconcile list (its press stayed
// latched after opening the editor, so the second tap was swallowed), and every
// handler routed presses by the 30px DRAWN rect while Button hit-tests its 44px
// TOUCH target — a press on a button's lower edge was never delivered, fell
// into the grid, and created a node under the button. Adding a corner button is
// now one row in gridCornerButtons.

// gridCornerButtonsInputZ is the z of the cluster's handler in the grid pane's
// inputDispatcher: above every content handler (drum=100, splitter=150,
// sidebar=200, group menu=210, the tap-outside-a-menu owner=250 —
// popup_outside_press.go), so a press in the cluster always wins.
const gridCornerButtonsInputZ = 300

// gridCornerButton is one button of the top-right cluster.
type gridCornerButton struct {
	btn *Button
	// rect is the button's drawn rect (empty when the button does not exist).
	rect func(*Game) image.Rectangle
	// menuID is the portal entry the button toggles; its open state drives the
	// button's toggled look (reconcileGridButtonToggles).
	menuID string
}

// gridCornerButtons is THE table of top-right grid buttons, right to left.
// Every per-button concern (layout, draw, input, toggle reconcile, the menus'
// mutual exclusion, the reserved zone) iterates it.
func (g *Game) gridCornerButtons() []gridCornerButton {
	return []gridCornerButton{
		{g.gridHelpBtn, (*Game).gridHelpButtonRect, settingsOverlayID},
		{g.gridFileBtn, (*Game).gridFileButtonRect, overflowMenuID},
		{g.gridLogsBtn, (*Game).gridLogsButtonRect, logsPanelID},
		{g.gridThemeBtn, (*Game).gridThemeButtonRect, themeOverlayID},
	}
}

// syncGridCornerButtonRects pushes the live layout into each button so its hit
// test and its draw agree with the current splitter geometry.
func (g *Game) syncGridCornerButtonRects() {
	for _, c := range g.gridCornerButtons() {
		if c.btn != nil {
			c.btn.SetRect(c.rect(g))
		}
	}
}

// gridTopButtonsInputRect is the cluster's whole interactive area: the reserved
// zone (buttons + gaps + margin) grown to cover every button's TOUCH target.
// It is the cluster handler's InputBounds and the no-create zone of the grid
// gesture paths, so "the press hit a button" and "the grid may act here" can
// never disagree. Empty when the buttons are absent.
func (g *Game) gridTopButtonsInputRect() image.Rectangle {
	zone := g.gridTopButtonsReservedRect()
	for _, c := range g.gridCornerButtons() {
		if c.btn == nil {
			continue
		}
		c.btn.SetRect(c.rect(g))
		if hit := c.btn.HitRect(); !hit.Empty() {
			if zone.Empty() {
				zone = hit
			} else {
				zone = zone.Union(hit)
			}
		}
	}
	return zone
}

// gridCornerButtonsHandler routes pointer input for the whole cluster through
// the grid pane's z-ordered inputDispatcher. One path serves desktop mouse,
// mobile touch and mobile pointer/click.
//
// It is OPAQUE: a press anywhere in gridTopButtonsInputRect is consumed —
// on a button it fires that button; in a gap or the margin it is inert — so
// the press edge is owned by the cluster and the dispatcher's gesture latch
// (GestureOwnedByHandler) keeps the release-time grid tap off the canvas
// beneath, wherever the finger lifts.
type gridCornerButtonsHandler struct{ g *Game }

func (h gridCornerButtonsHandler) InputBounds() image.Rectangle {
	return h.g.gridTopButtonsInputRect()
}

func (h gridCornerButtonsHandler) ZIndex() int { return gridCornerButtonsInputZ }

func (h gridCornerButtonsHandler) HandleInput(x, y int, pressed bool) InputResult {
	g := h.g
	g.syncGridCornerButtonRects()
	target := g.gridCornerButtonAt(x, y)
	res := InputIgnored
	for _, c := range g.gridCornerButtons() {
		if c.btn == nil {
			continue
		}
		if c.btn == target {
			res = c.btn.HandleInputResult(x, y, pressed)
			continue
		}
		// Not the target: a pointer-outside update, so a neighbour whose touch
		// target overlaps this point never fires and drops its hover.
		c.btn.applyPress(false, pressed)
	}
	if res == InputConsumed {
		// Defer the DrumViewTree for this frame so its click-outside logic
		// doesn't immediately close the menu the button just opened (the
		// cluster lies outside every tree hit area).
		g.gridHelpCapturing = true
	}
	if pressed {
		// Opaque to z: a press anywhere in the cluster belongs to it.
		return InputConsumed
	}
	return res
}

// gridCornerButtonAt picks the button a point targets. Touch targets of
// neighbouring buttons overlap (44px targets, 30px buttons, 6px gaps), so the
// point goes to the button whose DRAWN rect is nearest — the one the user aimed
// at — among those whose touch target contains it. Nil in a gap / the margin.
func (g *Game) gridCornerButtonAt(x, y int) *Button {
	pt := image.Pt(x, y)
	var best *Button
	bestD := -1
	for _, c := range g.gridCornerButtons() {
		if c.btn == nil || !pt.In(c.btn.HitRect()) {
			continue
		}
		if d := rectDistSq(c.btn.Rect(), pt); best == nil || d < bestD {
			best, bestD = c.btn, d
		}
	}
	return best
}

// rectDistSq is the squared distance from p to the nearest point of r (0 inside).
func rectDistSq(r image.Rectangle, p image.Point) int {
	dx, dy := 0, 0
	if p.X < r.Min.X {
		dx = r.Min.X - p.X
	} else if p.X >= r.Max.X {
		dx = p.X - (r.Max.X - 1)
	}
	if p.Y < r.Min.Y {
		dy = r.Min.Y - p.Y
	} else if p.Y >= r.Max.Y {
		dy = p.Y - (r.Max.Y - 1)
	}
	return dx*dx + dy*dy
}

func (h gridCornerButtonsHandler) HandleWheel(x, y, steps int) InputResult { return InputIgnored }

func (h gridCornerButtonsHandler) Capturing() bool { return false }
