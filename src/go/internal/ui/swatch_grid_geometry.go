package ui

import "image"

// Shared geometry for anchored "labelled cell grid" pickers.
//
// Two pickers present the same shape: a popup panel anchored to the control
// that opened it, containing a header band, a left gutter of row labels, and a
// fixed cols × rows grid of square-ish cells. The colour picker
// (ColorWheelComponent) lays 5 shade columns × 7 hue-family rows with family
// names in the gutter; the note-matrix picker (NoteMatrixComponent) lays
// octave columns × 12 pitch-class rows with note names in the gutter.
//
// The fiddly parts are identical and were worth having exactly once: solving a
// cell edge from the available width, flooring it at the touch minimum on
// mobile, clamping the panel inside its container, and then RE-DERIVING the
// cell size from the final rects so a clamped panel yields smaller cells that
// still tile the body rather than collapsing into the header.

// swatchGridSpec is the input to solveSwatchGrid.
type swatchGridSpec struct {
	Anchor image.Rectangle // control that opened the picker
	Bounds image.Rectangle // container the panel must stay inside

	Cols, Rows int
	Gutter     int // left label-gutter width (0 for no labels)
	Gap        int // gap between cells
	Pad        int // panel inner padding
	Header     int // header band height (title + close)
	CellMin    int // desktop minimum cell edge; mobile floors at TouchMinTarget

	// DesktopMaxW caps the panel width on desktop so it does not span the
	// whole container. 0 leaves it uncapped.
	DesktopMaxW int

	// BandH reserves a caption band off the TOP of the cell region (e.g. the
	// colour picker's "Light … Deep" hint). 0 for none.
	BandH int
}

// swatchGridLayout is the solved geometry. Cells are laid out and hit-tested
// over Cells (NOT Grid) so the label gutter is never a hit area; producers and
// hit-testers must both use Cells.
type swatchGridLayout struct {
	Panel image.Rectangle
	Title image.Rectangle
	Grid  image.Rectangle // body below the header, INCLUDING the label gutter
	Cells image.Rectangle // Grid minus the gutter (and minus Band)
	Band  image.Rectangle // empty when BandH is 0 or the panel is too short

	CellW, CellH int
}

// solveSwatchGrid computes the panel and grid geometry for an anchored picker.
func solveSwatchGrid(s swatchGridSpec) swatchGridLayout {
	var out swatchGridLayout
	if s.Anchor.Empty() || s.Bounds.Empty() || s.Cols <= 0 || s.Rows <= 0 {
		return out
	}
	bounds := s.Bounds

	// Cell edge: floor at the touch minimum on mobile so every cell is a real
	// tap target, at CellMin on desktop. Grow to use available width when the
	// container is roomy.
	cellMin := s.CellMin
	if tm := TouchMinTarget(); Profile().IsMobile() && tm > cellMin {
		cellMin = tm
	}

	availW := bounds.Dx()
	if !Profile().UseBottomSheet && s.DesktopMaxW > 0 && availW > s.DesktopMaxW {
		availW = s.DesktopMaxW
	}

	// availW = 2*Pad + Gutter + Cols*cell + (Cols-1)*Gap
	cellFromW := (availW - 2*s.Pad - s.Gutter - (s.Cols-1)*s.Gap) / s.Cols
	cell := max(cellMin, cellFromW)

	panelW := 2*s.Pad + s.Gutter + s.Cols*cell + (s.Cols-1)*s.Gap
	gridH := s.Rows*cell + (s.Rows-1)*s.Gap
	panelH := s.Header + s.Pad + s.BandH + gridH + s.Pad

	if panelW > bounds.Dx() {
		panelW = bounds.Dx()
	}
	if panelH > bounds.Dy() {
		panelH = bounds.Dy()
	}

	// Position near the anchor, preferring whichever vertical direction has
	// more room, then clamp fully inside bounds.
	rect := image.Rect(0, 0, panelW, panelH)
	if Profile().UseBottomSheet {
		cx := bounds.Min.X + (bounds.Dx()-panelW)/2
		rect = rect.Add(image.Pt(cx, bounds.Max.Y-panelH))
	} else {
		spaceDown := bounds.Max.Y - s.Anchor.Max.Y
		spaceUp := s.Anchor.Min.Y - bounds.Min.Y
		originY := s.Anchor.Max.Y
		if spaceDown < panelH && spaceUp > spaceDown {
			originY = s.Anchor.Min.Y - panelH // open upward
		}
		rect = rect.Add(image.Pt(s.Anchor.Min.X, originY))
	}
	if rect.Min.X < bounds.Min.X {
		rect = rect.Add(image.Pt(bounds.Min.X-rect.Min.X, 0))
	}
	if rect.Min.Y < bounds.Min.Y {
		rect = rect.Add(image.Pt(0, bounds.Min.Y-rect.Min.Y))
	}
	if rect.Max.X > bounds.Max.X {
		rect = rect.Add(image.Pt(bounds.Max.X-rect.Max.X, 0))
	}
	if rect.Max.Y > bounds.Max.Y {
		rect = rect.Add(image.Pt(0, bounds.Max.Y-rect.Max.Y))
	}
	out.Panel = rect

	// Header and padding adapt down when the panel was clamped short, so the
	// grid body keeps a usable height instead of collapsing into the header.
	header := s.Header
	if maxHeader := rect.Dy() / 3; header > maxHeader {
		header = maxHeader
	}
	pad := s.Pad
	if maxPad := (rect.Dy()/2 - header) / 2; maxPad >= 0 && pad > maxPad {
		pad = maxPad
	}
	if pad < 0 {
		pad = 0
	}

	out.Title = image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+header)
	out.Grid = image.Rect(
		rect.Min.X+pad,
		rect.Min.Y+header+pad,
		rect.Max.X-pad,
		rect.Max.Y-pad,
	)

	labelW := s.Gutter
	if maxLabel := out.Grid.Dx() / 2; labelW > maxLabel {
		labelW = maxLabel
	}
	if labelW < 0 {
		labelW = 0
	}
	out.Cells = image.Rect(out.Grid.Min.X+labelW, out.Grid.Min.Y, out.Grid.Max.X, out.Grid.Max.Y)

	// Carve the optional caption band off the top of the CELL region only, so
	// it never collides with the gutter labels. Suppressed when reserving it
	// would starve the cells.
	if s.BandH > 0 && out.Cells.Dy()-s.BandH >= s.Rows*20 {
		out.Band = image.Rect(out.Cells.Min.X, out.Cells.Min.Y, out.Cells.Max.X, out.Cells.Min.Y+s.BandH)
		out.Cells.Min.Y += s.BandH
	}

	// Re-derive cell size from the FINAL cell region so cells always fit.
	cw := (out.Cells.Dx() - (s.Cols-1)*s.Gap) / s.Cols
	ch := (out.Cells.Dy() - (s.Rows-1)*s.Gap) / s.Rows
	if cw < 1 {
		cw = 1
	}
	if ch < 1 {
		ch = 1
	}
	out.CellW, out.CellH = cw, ch
	return out
}

// CellRect returns the rect of the (col, row) cell.
func (l swatchGridLayout) CellRect(col, row, gap int) image.Rectangle {
	x0 := l.Cells.Min.X + col*(l.CellW+gap)
	y0 := l.Cells.Min.Y + row*(l.CellH+gap)
	return image.Rect(x0, y0, x0+l.CellW, y0+l.CellH)
}

// CellAtPoint maps a point to a (col, row), tolerating the inter-cell gaps so
// gap pixels still resolve to a neighbouring cell rather than a dead gutter.
// ok is false when the point is outside the cell region entirely.
func (l swatchGridLayout) CellAtPoint(x, y, cols, rows, gap int) (col, row int, ok bool) {
	if !image.Pt(x, y).In(l.Cells) {
		return 0, 0, false
	}
	strideX := l.CellW + gap
	strideY := l.CellH + gap
	if strideX < 1 {
		strideX = 1
	}
	if strideY < 1 {
		strideY = 1
	}
	col = clamp((x-l.Cells.Min.X)/strideX, 0, cols-1)
	row = clamp((y-l.Cells.Min.Y)/strideY, 0, rows-1)
	return col, row, true
}
