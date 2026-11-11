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

// GridAnchorOpts configures anchoredPopupRect / (*Game).gridAnchoredPopupRect.
type GridAnchorOpts struct {
	// Prefer is the placement side relative to the anchor; the zero value
	// (PopupBelow) drops the popup under its trigger.
	Prefer PopupSide
	// Avoid lists exclusion zones the popup must clear (e.g. the grid-pane
	// gear/folder button cluster). Each is applied via avoidReservedRect.
	Avoid []image.Rectangle
}

// anchoredPopupRect positions a popup of size (w,h) adjacent to anchor inside
// winBounds, then nudges it clear of each Avoid zone. It reuses AnchorPopupRect
// for the flip/clamp and avoidReservedRect for exclusion, so a popup anchored to
// a grid-pane trigger drops next to that trigger instead of being pinned into
// the bottom drum pane (the dv.Bounds-clamp bug). winBounds MUST have origin
// (0,0) — it is the whole window; avoidReservedRect clamps against its extent.
func anchoredPopupRect(anchor image.Rectangle, w, h int, winBounds image.Rectangle, opts GridAnchorOpts) image.Rectangle {
	r := AnchorPopupRect(winBounds, anchor, w, h, opts.Prefer)
	for _, zone := range opts.Avoid {
		r = avoidReservedRect(r, zone, winBounds.Dx(), winBounds.Dy())
	}
	return r
}

// floatOffPaneEdges nudges a popup inward from any pane edge it welds to, by
// `gutter` px, but only on an axis that has at least `gutter` of slack (so a
// popup that fills the pane on an axis is never resized or shoved off the
// opposite edge). This is what makes the grid-pane button menus read as
// floating cards instead of chrome bolted to the frame. Pure geometry —
// unit-tested in overlay_anchor_gutter_test.go.
func floatOffPaneEdges(r, pane image.Rectangle, gutter int) image.Rectangle {
	if gutter <= 0 {
		return r
	}
	if pane.Dx()-r.Dx() >= gutter { // horizontal slack
		if r.Max.X > pane.Max.X-gutter {
			r = r.Add(image.Pt(pane.Max.X-gutter-r.Max.X, 0)) // pull left off the right edge
		}
		if r.Min.X < pane.Min.X+gutter {
			r = r.Add(image.Pt(pane.Min.X+gutter-r.Min.X, 0)) // push right off the left edge
		}
	}
	if pane.Dy()-r.Dy() >= gutter { // vertical slack
		if r.Max.Y > pane.Max.Y-gutter {
			r = r.Add(image.Pt(0, pane.Max.Y-gutter-r.Max.Y)) // pull up off the bottom edge
		}
		if r.Min.Y < pane.Min.Y+gutter {
			r = r.Add(image.Pt(0, pane.Min.Y+gutter-r.Min.Y)) // push down off the top edge
		}
	}
	return r
}

// gridAnchoredPopupRect positions a popup anchored to a grid-pane trigger rect,
// CLAMPED TO THE GRID PANE (not the whole window) so the popup drops beside its
// trigger and never spills past the split line into the bottom drum-view panel —
// matching the node pop-up. It always dodges the reserved gear/folder cluster.
// This is the single positioning path for the grid-pane top-right button menus
// (Settings, Logs). A popup taller than the grid pane is capped to it; callers
// whose content can exceed the pane clip/scroll it to the returned rect.
func (g *Game) gridAnchoredPopupRect(anchor image.Rectangle, w, h int, opts GridAnchorOpts) image.Rectangle {
	// Avoid the WHOLE top-right button cluster (gear+file+logs) so the popup's own
	// controls (e.g. its close button) never land under a higher-z grid button.
	if z := g.gridTopButtonsReservedRect(); !z.Empty() {
		opts.Avoid = append(opts.Avoid, z)
	}
	gridPane := image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	// Cap the requested size so it never fills the pane edge-to-edge: this
	// guarantees floatOffPaneEdges finds at least a gutter of slack on both
	// axes, so even a panel taller than the short grid pane floats off the top
	// and bottom (its content scrolls) instead of welding to the frame.
	if maxW := gridPane.Dx() - 2*SpaceMD; maxW > 0 && w > maxW {
		w = maxW
	}
	if maxH := gridPane.Dy() - 2*SpaceMD; maxH > 0 && h > maxH {
		h = maxH
	}
	r := anchoredPopupRect(anchor, w, h, gridPane, opts)
	// Float the panel off the pane frame so it reads as a card, not chrome
	// welded to the top/right/bottom edge.
	return floatOffPaneEdges(r, gridPane, SpaceMD)
}
