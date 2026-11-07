package ui

import "image"

// ONE component: one table, one handler, one hit geometry. Separate handlers and lists drifted
// (a latched theme button; presses routed by the drawn rect, not the touch target, created nodes).

// Above every content handler, so a press in the cluster always wins.
const gridCornerButtonsInputZ = 300

// gridCornerButton is one button of the top-right cluster.
type gridCornerButton struct {
	btn *Button
	// rect is the button's drawn rect (empty when the button does not exist).
	rect func(*Game) image.Rectangle
	// Its open state drives the button's toggled look.
	menuID string
}

// THE table, right to left; every per-button concern iterates it.
func (g *Game) gridCornerButtons() []gridCornerButton {
	return []gridCornerButton{
		{g.gridHelpBtn, (*Game).gridHelpButtonRect, settingsOverlayID},
		{g.gridFileBtn, (*Game).gridFileButtonRect, overflowMenuID},
		{g.gridLogsBtn, (*Game).gridLogsButtonRect, logsPanelID},
		{g.gridThemeBtn, (*Game).gridThemeButtonRect, themeOverlayID},
	}
}

func (g *Game) syncGridCornerButtonRects() {
	for _, c := range g.gridCornerButtons() {
		if c.btn != nil {
			c.btn.SetRect(c.rect(g))
		}
	}
}

// Both the cluster's InputBounds and the grid's no-create zone, so the two can never disagree.
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

// OPAQUE: a press anywhere in the cluster is consumed (inert in gaps), so the cluster owns the
// gesture and the release-time grid tap never reaches the canvas, wherever the finger lifts.
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
		// Not the target: a neighbour whose touch target overlaps this point must not fire.
		c.btn.applyPress(false, pressed)
	}
	if res == InputConsumed {
		// Defer the tree this frame so its click-outside does not close the menu just opened.
		g.gridHelpCapturing = true
	}
	if pressed {
		// Opaque to z: a press anywhere in the cluster belongs to it.
		return InputConsumed
	}
	return res
}

// Touch targets overlap, so the point goes to the button whose drawn rect is nearest; nil in a gap.
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
