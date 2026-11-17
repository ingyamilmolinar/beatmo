package ui

import (
	"image"
	"image/color"
)

const (
	// popupGripSize is the drag-grip glyph box. The visible affordance stays
	// this size on every platform; only the HIT rect grows on touch (HitRect),
	// mirroring the HitArea.Touch/ClipRect pattern used by the zone tree —
	// enlarging the visible box instead would push the card title around.
	popupGripSize = 16
	// popupGripInset is the grip's offset from the card's top-left corner. It
	// matches sidebarPad so the glyph lines up with the header band both menus
	// already inset by that amount.
	popupGripInset = sidebarPad
	// popupGripDragDeadzone is how far the pointer must travel before a press
	// on the grip counts as a drag. Below it the gesture is a tap and the card
	// stays in auto mode — otherwise a stray tap (or a mobile touch wobble)
	// would silently pin a card the user never meant to move.
	popupGripDragDeadzone = 3
)

// PopupDragHandle is the shared drag state for a grid-pane menu card whose
// position is otherwise derived every frame (node pop-up, group menu). The
// zero value is "auto": Place passes the caller's auto-anchored rect through
// untouched, so embedding the handle changes nothing until the user drags it.
//
// Once dragged, the handle is PINNED: it holds an absolute grid-pane origin
// and stops tracking the node/group/camera. Reset() (called from each menu's
// Open/Close) returns it to auto, so a pin never survives a reopen.
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

// HitRect is the pressable area: the glyph on desktop, grown around the same
// center to the platform touch minimum on mobile (ExpandHitArea), then
// translated (never shrunk) back inside panel.
//
// Translate, not clip: callers gate input dispatch on point-in-panel before
// doing any internal hit-testing (input_dispatch.go's Dispatch only calls
// HandleInput for points inside InputBounds, which is the bare panel rect
// for these menus — game_group_menu.go), so a hit rect that pokes outside
// panel is dead space the user can never actually press. popupGripInset (10,
// sidebarPad) sits closer to the card's corner than half the touch-min
// growth (14 at (44-16)/2 in DensitySpacious), so the grown box always
// overhangs the top-left corner before this adjustment. Sliding it back
// inside panel keeps the full touch target reachable AND fully contained —
// e.g. a 44px target anchored at panel's top-left corner, still containing
// the 16px glyph. Only shrinks (via the final Intersect) in the degenerate
// case where panel itself is smaller than the touch minimum on an axis —
// there translating can't fully fit it either.
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

// Dragging reports whether a grip drag is in flight (feeds Capturing() and the
// camera-pan gate).
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

// Begin starts a drag from a press at pt inside panel. It records the grab
// offset so the card does not jump to put its corner under the cursor. It does
// NOT pin — see MoveTo's deadzone.
func (h *PopupDragHandle) Begin(pt image.Point, panel image.Rectangle) {
	h.dragging = true
	h.grab = pt.Sub(panel.Min)
	h.pressPt = pt
}

// MoveTo advances an in-flight drag to pt. Past the deadzone it pins the card
// at pt−grab, clamped by clampPopupOrigin. size is the card's CURRENT size.
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

// Hold pins the card at its current top-left, as if the user had dragged it
// there: a control that changes the card's height (the node type cycle) must
// not make the card re-anchor and slide out from under the pointer.
func (h *PopupDragHandle) Hold(origin image.Point) {
	h.pinned = true
	h.origin = origin
}

// Place is the chokepoint every consumer calls at the end of its layout: auto
// is the rect the menu's own anchoring produced this frame. In auto mode it is
// returned verbatim (zero behavior change); pinned, the stored origin is
// re-clamped against the CURRENT size — so a card that grew a section still
// ends up fully inside the pane.
func (h *PopupDragHandle) Place(auto, pane, reserved image.Rectangle) image.Rectangle {
	if !h.pinned {
		return auto
	}
	size := image.Pt(auto.Dx(), auto.Dy())
	o := clampPopupOrigin(h.origin, size, pane, reserved)
	h.origin = o
	return image.Rect(o.X, o.Y, o.X+size.X, o.Y+size.Y)
}

// clampPopupOrigin enforces the two placement rules from the design:
//  1. a size-sized card at the returned origin sits fully inside pane;
//  2. its grip hit rect does not overlap reserved (the gear/file/logs cluster,
//     which sits at a higher z and would swallow presses) — the escape
//     guarantee that keeps a parked card draggable.
//
// Rule 2 resolves by the SMALLEST axis-aligned push that clears the cluster
// and still satisfies rule 1, so the card slides the short way out rather than
// teleporting across the pane.
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
		// Pane too small for any escape (tiny window): keep rule 1 and accept
		// the overlap — Esc and tap-away still dismiss the card.
		return o
	}
	return best
}

// popupGripTint is the grip's three-state tint: resting (dim, same treatment
// as the bottom-sheet drag handle), hovered, and dragging.
func popupGripTint(dragging, hovered bool) color.Color {
	switch {
	case dragging:
		return colAccent
	case hovered:
		return colTextSecondary
	default:
		// AlphaStrong at rest, NOT the bottom-sheet handle's alpha-40: this grip
		// is a DISCOVERY affordance — it is the only cue that the card can be
		// moved at all — where the sheet handle merely decorates a gesture the
		// user already knows. Screenshot-verified at alpha 40 the dots read as
		// panel noise on both platforms, so the affordance failed silently.
		return WithAlpha(genColorBorder, AlphaStrong)
	}
}

// clampOriginInPane slides a size-sized card so it lies inside pane. A card
// larger than the pane on an axis is pinned to the pane's min on that axis
// (the menus already cap their own size to the pane, so this is a guard).
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
