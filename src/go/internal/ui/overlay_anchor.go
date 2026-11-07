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

// The single positioning primitive for every anchored overlay: hand-rolled flip/clamp math let
// popups run off-screen with their lower controls unreachable. Oversized popups are shrunk to fit.
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

// Shrinks only when r is larger than bounds on an axis.
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
	// The zero value drops the popup under its trigger.
	Prefer PopupSide
	Avoid  []image.Rectangle
}

// A popup anchored to a grid-pane trigger drops next to it instead of being pinned into the drum
// pane. winBounds MUST have origin (0,0).
func anchoredPopupRect(anchor image.Rectangle, w, h int, winBounds image.Rectangle, opts GridAnchorOpts) image.Rectangle {
	r := AnchorPopupRect(winBounds, anchor, w, h, opts.Prefer)
	for _, zone := range opts.Avoid {
		r = avoidReservedRect(r, zone, winBounds.Dx(), winBounds.Dy())
	}
	return r
}

// Only on an axis with at least `gutter` of slack, so a pane-filling popup is never resized or shoved.
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

// Clamped to the GRID PANE, so the menu drops beside its trigger and never spills into the drum
// view; the single path for the top-right button menus. Taller popups are capped and must scroll.
func (g *Game) gridAnchoredPopupRect(anchor image.Rectangle, w, h int, opts GridAnchorOpts) image.Rectangle {
	// The whole cluster, so the popup's own close button never lands under a higher-z grid button.
	if z := g.gridTopButtonsReservedRect(); !z.Empty() {
		opts.Avoid = append(opts.Avoid, z)
	}
	gridPane := image.Rect(0, 0, g.split.GridW(g.winW), g.split.GridH(g.winH))
	// Never edge-to-edge, so floatOffPaneEdges always has slack and a tall panel still floats.
	if maxW := gridPane.Dx() - 2*SpaceMD; maxW > 0 && w > maxW {
		w = maxW
	}
	if maxH := gridPane.Dy() - 2*SpaceMD; maxH > 0 && h > maxH {
		h = maxH
	}
	r := anchoredPopupRect(anchor, w, h, gridPane, opts)
	return floatOffPaneEdges(r, gridPane, SpaceMD)
}
