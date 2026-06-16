package ui

import "image"

// PopupSide names the preferred placement of a popup relative to its anchor.
type PopupSide int

const (
	// PopupBelow places the popup under the anchor, left edges aligned.
	PopupBelow PopupSide = iota
	// PopupAbove places the popup over the anchor, left edges aligned.
	PopupAbove
	// PopupRight places the popup to the right of the anchor, top edges aligned.
	PopupRight
	// PopupLeft places the popup to the left of the anchor, top edges aligned.
	PopupLeft
)

// popupAnchorGap is the breathing room between an anchor and its popup.
const popupAnchorGap = 4

// AnchorPopupRect positions a popup of size (w, h) adjacent to anchor on the
// preferred side, flipping to the opposite side when there is not enough room,
// and always clamping the result fully inside bounds.
//
// This is the single positioning primitive for every anchored overlay
// (context menu, subdiv menu, volume popups, pickers). Overlays must not
// hand-roll their own flip/clamp math: the pre-existing bug class was popups
// anchored downward from a bottom-zone trigger running off the screen with
// their lower controls unreachable.
//
// If w or h exceed the bounds dimensions they are shrunk to fit, so the
// returned rect is always contained in bounds (callers that can scroll their
// content should compare the returned height against their content height).
func AnchorPopupRect(bounds, anchor image.Rectangle, w, h int, prefer PopupSide) image.Rectangle {
	if w > bounds.Dx() {
		w = bounds.Dx()
	}
	if h > bounds.Dy() {
		h = bounds.Dy()
	}

	var x, y int
	switch prefer {
	case PopupAbove, PopupBelow:
		x = anchor.Min.X
		spaceBelow := bounds.Max.Y - anchor.Max.Y - popupAnchorGap
		spaceAbove := anchor.Min.Y - bounds.Min.Y - popupAnchorGap
		below := prefer == PopupBelow
		if below && h > spaceBelow && spaceAbove > spaceBelow {
			below = false // flip up
		} else if !below && h > spaceAbove && spaceBelow > spaceAbove {
			below = true // flip down
		}
		if below {
			y = anchor.Max.Y + popupAnchorGap
		} else {
			y = anchor.Min.Y - popupAnchorGap - h
		}
	case PopupLeft, PopupRight:
		y = anchor.Min.Y
		spaceRight := bounds.Max.X - anchor.Max.X - popupAnchorGap
		spaceLeft := anchor.Min.X - bounds.Min.X - popupAnchorGap
		right := prefer == PopupRight
		if right && w > spaceRight && spaceLeft > spaceRight {
			right = false // flip left
		} else if !right && w > spaceLeft && spaceRight > spaceLeft {
			right = true // flip right
		}
		if right {
			x = anchor.Max.X + popupAnchorGap
		} else {
			x = anchor.Min.X - popupAnchorGap - w
		}
	}

	return ClampRectInto(image.Rect(x, y, x+w, y+h), bounds)
}

// ClampRectInto translates r so it lies fully inside bounds, shrinking it
// only when it is larger than bounds on an axis.
func ClampRectInto(r, bounds image.Rectangle) image.Rectangle {
	if r.Dx() > bounds.Dx() {
		r.Min.X = bounds.Min.X
		r.Max.X = bounds.Max.X
	} else if r.Min.X < bounds.Min.X {
		r = r.Add(image.Pt(bounds.Min.X-r.Min.X, 0))
	} else if r.Max.X > bounds.Max.X {
		r = r.Add(image.Pt(bounds.Max.X-r.Max.X, 0))
	}
	if r.Dy() > bounds.Dy() {
		r.Min.Y = bounds.Min.Y
		r.Max.Y = bounds.Max.Y
	} else if r.Min.Y < bounds.Min.Y {
		r = r.Add(image.Pt(0, bounds.Min.Y-r.Min.Y))
	} else if r.Max.Y > bounds.Max.Y {
		r = r.Add(image.Pt(0, bounds.Max.Y-r.Max.Y))
	}
	return r
}
