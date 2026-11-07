package ui

import (
	"image"
	"image/color"
)

const (
	// Only the HIT rect grows on touch; growing the visible box would push the title around.
	popupGripSize = 16
	// Matches sidebarPad, so the glyph lines up with the header band.
	popupGripInset = sidebarPad
	// Below this a press is a tap and the card stays in auto mode, so a stray touch never pins it.
	popupGripDragDeadzone = 3
)

// Zero value = auto: Place passes the anchored rect through. A drag PINS an absolute origin that
// stops tracking the node and camera; Reset() on Open/Close returns to auto.
type PopupDragHandle struct {
	pinned   bool
	origin   image.Point // pinned top-left, grid-pane space
	dragging bool
	grab     image.Point // cursor − panel.Min at press: keeps the grip under the finger
	pressPt  image.Point // press origin, for the deadzone test
}

// GripRect is the glyph box in the card's top-left corner.
func (h *PopupDragHandle) GripRect(panel image.Rectangle) image.Rectangle {
	if panel.Empty() {
		return image.Rectangle{}
	}
	x := panel.Min.X + popupGripInset
	y := panel.Min.Y + popupGripInset
	return image.Rect(x, y, x+popupGripSize, y+popupGripSize)
}

// Grown to the touch minimum on mobile, then translated (never shrunk) back inside the panel:
// dispatch only reaches points inside the panel, so any overhang would be dead space.
func (h *PopupDragHandle) HitRect(panel image.Rectangle) image.Rectangle {
	grip := h.GripRect(panel)
	if grip.Empty() {
		return image.Rectangle{}
	}
	want := ExpandHitArea(popupGripSize)
	grow := (want - popupGripSize) / 2
	if grow <= 0 {
		return grip
	}
	hit := grip.Inset(-grow)

	var d image.Point
	if hit.Min.X < panel.Min.X {
		d.X = panel.Min.X - hit.Min.X
	} else if hit.Max.X > panel.Max.X {
		d.X = panel.Max.X - hit.Max.X
	}
	if hit.Min.Y < panel.Min.Y {
		d.Y = panel.Min.Y - hit.Min.Y
	} else if hit.Max.Y > panel.Max.Y {
		d.Y = panel.Max.Y - hit.Max.Y
	}
	hit = hit.Add(d)

	return hit.Intersect(panel)
}

func (h *PopupDragHandle) Dragging() bool { return h.dragging }

// Pinned reports whether the user has moved this card off its auto position.
func (h *PopupDragHandle) Pinned() bool { return h.pinned }

// Reset returns the handle to auto mode.
func (h *PopupDragHandle) Reset() {
	h.pinned = false
	h.dragging = false
	h.origin = image.Point{}
	h.grab = image.Point{}
	h.pressPt = image.Point{}
}

// Records the grab offset so the card does not jump; does NOT pin (MoveTo's deadzone does).
func (h *PopupDragHandle) Begin(pt image.Point, panel image.Rectangle) {
	h.dragging = true
	h.grab = pt.Sub(panel.Min)
	h.pressPt = pt
}

// size is the card's CURRENT size.
func (h *PopupDragHandle) MoveTo(pt image.Point, pane, reserved image.Rectangle, size image.Point) {
	if !h.dragging {
		return
	}
	d := pt.Sub(h.pressPt)
	if !h.pinned && abs(d.X) < popupGripDragDeadzone && abs(d.Y) < popupGripDragDeadzone {
		return
	}
	h.pinned = true
	h.origin = clampPopupOrigin(pt.Sub(h.grab), size, pane, reserved)
}

// End finishes the drag, keeping the pin.
func (h *PopupDragHandle) End() { h.dragging = false }

// A control that changes the card's height must not make it re-anchor out from under the pointer.
func (h *PopupDragHandle) Hold(origin image.Point) {
	h.pinned = true
	h.origin = origin
}

// Auto: returned verbatim. Pinned: re-clamped to the current size, so a card that grew still fits.
func (h *PopupDragHandle) Place(auto, pane, reserved image.Rectangle) image.Rectangle {
	if !h.pinned {
		return auto
	}
	size := image.Pt(auto.Dx(), auto.Dy())
	o := clampPopupOrigin(h.origin, size, pane, reserved)
	h.origin = o
	return image.Rect(o.X, o.Y, o.X+size.X, o.Y+size.Y)
}

// The card stays inside the pane, and its grip never overlaps the higher-z button cluster (so a
// parked card stays draggable), by the SMALLEST axis-aligned push.
func clampPopupOrigin(origin, size image.Point, pane, reserved image.Rectangle) image.Point {
	o := clampOriginInPane(origin, size, pane)
	if reserved.Empty() {
		return o
	}
	var h PopupDragHandle
	panelAt := func(p image.Point) image.Rectangle {
		return image.Rect(p.X, p.Y, p.X+size.X, p.Y+size.Y)
	}
	hit := h.HitRect(panelAt(o))
	if hit.Empty() || !hit.Overlaps(reserved) {
		return o
	}
	// Four candidate pushes: out the left, right, top, bottom of the cluster.
	deltas := []image.Point{
		{X: reserved.Min.X - hit.Max.X},
		{X: reserved.Max.X - hit.Min.X},
		{Y: reserved.Min.Y - hit.Max.Y},
		{Y: reserved.Max.Y - hit.Min.Y},
	}
	best, haveBest := o, false
	bestCost := 0
	for _, d := range deltas {
		cand := clampOriginInPane(o.Add(d), size, pane)
		if ch := h.HitRect(panelAt(cand)); ch.Empty() || ch.Overlaps(reserved) {
			continue // clamping undid the push — this direction has no room
		}
		cost := abs(cand.X-o.X) + abs(cand.Y-o.Y)
		if !haveBest || cost < bestCost {
			best, bestCost, haveBest = cand, cost, true
		}
	}
	if !haveBest {
		// No escape fits a tiny window: keep it in the pane; Esc and tap-away still dismiss.
		return o
	}
	return best
}

func popupGripTint(dragging, hovered bool) color.Color {
	switch {
	case dragging:
		return colAccent
	case hovered:
		return colTextSecondary
	default:
		// Stronger than the sheet handle: this grip is the only cue the card can move, and at the sheet's
		// alpha it read as panel noise.
		return WithAlpha(genColorBorder, AlphaStrong)
	}
}

// A guard: the menus already cap their size to the pane.
func clampOriginInPane(origin, size image.Point, pane image.Rectangle) image.Point {
	o := origin
	if max := pane.Max.X - size.X; o.X > max {
		o.X = max
	}
	if o.X < pane.Min.X {
		o.X = pane.Min.X
	}
	if max := pane.Max.Y - size.Y; o.Y > max {
		o.Y = max
	}
	if o.Y < pane.Min.Y {
		o.Y = pane.Min.Y
	}
	return o
}
