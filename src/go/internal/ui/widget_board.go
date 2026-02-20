package ui

import (
	"image"
	"math"
)

// WidgetKind groups the known bottom-pane widgets.
type WidgetKind string

const (
	WidgetTransport WidgetKind = "transport"
	WidgetRack      WidgetKind = "rack"
	WidgetTimeline  WidgetKind = "timeline"
	WidgetWave      WidgetKind = "wave"
	WidgetCustom    WidgetKind = "custom"
)

// WidgetPlacement describes the location of a widget within a weighted grid.
type WidgetPlacement struct {
	ID       WidgetKind
	Title    string
	Col      int
	Row      int
	ColSpan  int
	RowSpan  int
	MinW     int
	MinH     int
	Visible  bool
	Editable bool // whether the user may move/remove this widget
}

// LayoutSnapshot captures the current grid ratios and widget placements so
// callers (tests/JS) can persist or assert layout.
type LayoutSnapshot struct {
	Cols    []float64
	Rows    []float64
	Widgets []WidgetPlacement
	Bounds  image.Rectangle
}

// WidgetBoard manages a simple weighted grid and widget placements. It is not
// concerned with drawing; DrumView owns rendering.
type WidgetBoard struct {
	bounds     image.Rectangle
	offset     image.Point
	cols       []float64
	rows       []float64
	colPos     []int
	rowPos     []int
	minColW    int
	minRowH    int
	pad        int
	placements map[WidgetKind]WidgetPlacement
	pixelRows  bool
}

// NewWidgetBoard creates a board with the provided column/row weights.
func NewWidgetBoard(b image.Rectangle, cols, rows []float64) *WidgetBoard {
	if len(cols) == 0 {
		cols = []float64{1}
	}
	if len(rows) == 0 {
		rows = []float64{1}
	}
	wb := &WidgetBoard{
		bounds:     image.Rect(0, 0, b.Dx(), b.Dy()),
		offset:     b.Min,
		cols:       append([]float64(nil), cols...),
		rows:       append([]float64(nil), rows...),
		minColW:    220,
		minRowH:    24,
		pad:        0,
		placements: map[WidgetKind]WidgetPlacement{},
	}
	wb.recalc()
	return wb
}

// SetBounds updates the container rectangle and recomputes splits.
func (wb *WidgetBoard) SetBounds(b image.Rectangle) {
	if wb.offset == b.Min && wb.bounds.Dx() == b.Dx() && wb.bounds.Dy() == b.Dy() {
		return
	}
	wb.offset = b.Min
	wb.bounds = image.Rect(0, 0, b.Dx(), b.Dy())
	// When overall bounds change, re-normalize proportions so dividers remain visible.
	wb.pixelRows = false
	wb.recalc()
}

// SetWeights replaces the column/row weights and recalculates positions.
func (wb *WidgetBoard) SetWeights(cols, rows []float64) {
	if len(cols) > 0 {
		wb.cols = append([]float64(nil), cols...)
	}
	if len(rows) > 0 {
		wb.rows = append([]float64(nil), rows...)
	}
	wb.pixelRows = false
	wb.recalc()
}

// recalc recalculates cumulative positions from weights, enforcing minimums.
func (wb *WidgetBoard) recalc() {
	// On narrow panes, cap rack column so the timeline gets at least 60%.
	effectiveMinColW := wb.minColW
	if wb.bounds.Dx() > 0 && wb.bounds.Dx() < wb.minColW*2+60 {
		effectiveMinColW = wb.bounds.Dx() * 40 / 100
		if effectiveMinColW < 100 {
			effectiveMinColW = 100
		}
	}

	if wb.pixelRows {
		// Columns remain proportional to weights.
		sumCols := 0.0
		for _, v := range wb.cols {
			sumCols += v
		}
		if sumCols == 0 {
			sumCols = 1
		}
		wb.colPos = make([]int, len(wb.cols)+1)
		totalW := wb.bounds.Dx()
		cumCol := 0.0
		x := 0
		for i, w := range wb.cols {
			wb.colPos[i] = x
			cumCol += w
			nextX := int(math.Round(float64(totalW) * (cumCol / sumCols)))
			px := nextX - x
			if px < effectiveMinColW {
				px = effectiveMinColW
				nextX = x + px
			}
			if nextX > wb.bounds.Max.X {
				nextX = wb.bounds.Max.X
			}
			x = nextX
		}
		wb.colPos[len(wb.cols)] = wb.bounds.Max.X

		wb.rowPos = make([]int, len(wb.rows)+1)
		y := 0
		for i, h := range wb.rows {
			py := int(math.Round(h))
			if py < wb.minRowH {
				py = wb.minRowH
			}
			if y+py > wb.bounds.Max.Y {
				py = wb.bounds.Max.Y - y
			}
			wb.rowPos[i] = y
			y += py
		}
		wb.rowPos[len(wb.rows)] = wb.bounds.Max.Y
		return
	}

	sumCols := 0.0
	for _, v := range wb.cols {
		sumCols += v
	}
	sumRows := 0.0
	for _, v := range wb.rows {
		sumRows += v
	}
	if sumCols == 0 {
		sumCols = 1
	}
	if sumRows == 0 {
		sumRows = 1
	}
	// Cumulative rounding: compute each boundary as Round(totalW * cumWeight/sumCols)
	// so row/col positions tile exactly without gaps or overlaps.
	wb.colPos = make([]int, len(wb.cols)+1)
	totalW := wb.bounds.Dx()
	cumCol := 0.0
	x := 0
	for i, w := range wb.cols {
		wb.colPos[i] = x
		cumCol += w
		nextX := int(math.Round(float64(totalW) * (cumCol / sumCols)))
		px := nextX - x
		if px < effectiveMinColW {
			px = effectiveMinColW
			nextX = x + px
		}
		if nextX > wb.bounds.Max.X {
			nextX = wb.bounds.Max.X
		}
		x = nextX
	}
	wb.colPos[len(wb.cols)] = wb.bounds.Max.X

	totalH := wb.bounds.Dy()
	cumRow := 0.0
	wb.rowPos = make([]int, len(wb.rows)+1)
	y := 0
	for i, h := range wb.rows {
		wb.rowPos[i] = y
		cumRow += h
		nextY := int(math.Round(float64(totalH) * (cumRow / sumRows)))
		py := nextY - y
		if py < wb.minRowH {
			py = wb.minRowH
			nextY = y + py
		}
		if nextY > wb.bounds.Max.Y {
			nextY = wb.bounds.Max.Y
		}
		y = nextY
	}
	wb.rowPos[len(wb.rows)] = wb.bounds.Max.Y
}

// Snapshot captures the current ratios and placements.
func (wb *WidgetBoard) Snapshot() LayoutSnapshot {
	ws := make([]WidgetPlacement, 0, len(wb.placements))
	for _, p := range wb.placements {
		ws = append(ws, p)
	}
	b := wb.bounds
	b = b.Add(wb.offset)
	return LayoutSnapshot{
		Cols:    append([]float64(nil), wb.cols...),
		Rows:    append([]float64(nil), wb.rows...),
		Widgets: ws,
		Bounds:  b,
	}
}

// Restore overwrites the board weights and placements from a snapshot.
func (wb *WidgetBoard) Restore(s LayoutSnapshot) {
	if len(s.Cols) > 0 {
		wb.cols = append([]float64(nil), s.Cols...)
	}
	if len(s.Rows) > 0 {
		wb.rows = append([]float64(nil), s.Rows...)
	}
	wb.placements = map[WidgetKind]WidgetPlacement{}
	for _, p := range s.Widgets {
		wb.placements[p.ID] = p
	}
	wb.offset = s.Bounds.Min
	wb.bounds = image.Rect(0, 0, s.Bounds.Dx(), s.Bounds.Dy())
	wb.recalc()
}

// Rect returns the padded rectangle for a widget; zero rectangle when missing.
func (wb *WidgetBoard) Rect(id WidgetKind) image.Rectangle {
	p, ok := wb.placements[id]
	if !ok || !p.Visible {
		return image.Rectangle{}
	}
	if p.Col < 0 || p.Col >= len(wb.cols) || p.Row < 0 || p.Row >= len(wb.rows) {
		return image.Rectangle{}
	}
	c0 := wb.colPos[p.Col] + wb.offset.X
	c1 := wb.colPos[minIntClamp(p.Col+p.ColSpan, len(wb.colPos)-1)] + wb.offset.X
	r0 := wb.rowPos[p.Row] + wb.offset.Y
	r1 := wb.rowPos[minIntClamp(p.Row+p.RowSpan, len(wb.rowPos)-1)] + wb.offset.Y
	rect := image.Rect(c0, r0, c1, r1)
	return insetRect(rect, wb.pad)
}

// ColWidth reports the width of a column in pixels.
func (wb *WidgetBoard) ColWidth(col int) int {
	if col < 0 || col >= len(wb.cols) {
		return 0
	}
	return wb.colPos[col+1] - wb.colPos[col]
}

// RowHeight reports the height of a row in pixels.
func (wb *WidgetBoard) RowHeight(row int) int {
	if row < 0 || row >= len(wb.rows) {
		return 0
	}
	return wb.rowPos[row+1] - wb.rowPos[row]
}

// ResizeAxis nudges a split by deltaPx. axis="col" adjusts the split between
// column idx and idx+1; axis="row" adjusts row boundaries. deltaPx may be
// positive (expand left/top) or negative (shrink).
func (wb *WidgetBoard) ResizeAxis(axis string, idx int, deltaPx int) {
	switch axis {
	case "col":
		if idx < 0 || idx >= len(wb.cols)-1 {
			return
		}
		w0 := wb.cols[idx]
		w1 := wb.cols[idx+1]
		px0 := float64(wb.bounds.Dx()) * (w0 / (w0 + w1))
		px1 := float64(wb.bounds.Dx()) * (w1 / (w0 + w1))
		px0 += float64(deltaPx)
		px1 -= float64(deltaPx)
		if px0 < float64(wb.minColW) || px1 < float64(wb.minColW) {
			return
		}
		wb.cols[idx] = px0
		wb.cols[idx+1] = px1
	case "row":
		if idx < 0 || idx >= len(wb.rows)-1 {
			return
		}
		// If we're still using weights, convert all rows to pixel heights once so only
		// the two adjacent rows change while others keep their current pixel sizes.
		if !wb.pixelRows {
			sumRows := 0.0
			for _, v := range wb.rows {
				sumRows += v
			}
			if sumRows == 0 {
				sumRows = 1
			}
			for i, v := range wb.rows {
				h := float64(wb.bounds.Dy()) * (v / sumRows)
				if h < float64(wb.minRowH) {
					h = float64(wb.minRowH)
				}
				wb.rows[i] = h
			}
			wb.pixelRows = true
		}
		h0 := wb.rows[idx]
		h1 := wb.rows[idx+1]
		py0 := h0
		py1 := h1
		py0 += float64(deltaPx)
		py1 -= float64(deltaPx)
		if py0 < float64(wb.minRowH) || py1 < float64(wb.minRowH) {
			return
		}
		wb.rows[idx] = py0
		wb.rows[idx+1] = py1
		wb.pixelRows = true
	}
	wb.recalc()
}

// AddWidget registers a widget; existing IDs are replaced.
func (wb *WidgetBoard) AddWidget(p WidgetPlacement) {
	if p.ColSpan < 1 {
		p.ColSpan = 1
	}
	if p.RowSpan < 1 {
		p.RowSpan = 1
	}
	p.Visible = true
	if p.Title == "" {
		p.Title = string(p.ID)
	}
	wb.placements[p.ID] = p
}

// RemoveWidget hides and deletes a widget by ID.
func (wb *WidgetBoard) RemoveWidget(id WidgetKind) {
	delete(wb.placements, id)
}

// MoveWidget moves an existing widget to a different cell.
func (wb *WidgetBoard) MoveWidget(id WidgetKind, col, row int) {
	p, ok := wb.placements[id]
	if !ok {
		return
	}
	if col >= 0 {
		p.Col = col
	}
	if row >= 0 {
		p.Row = row
	}
	wb.placements[id] = p
}

// SetRowHeight sets an absolute pixel height for a row and enables pixel sizing.
func (wb *WidgetBoard) SetRowHeight(idx int, px int) {
	if idx < 0 || idx >= len(wb.rows) {
		return
	}
	if px < wb.minRowH {
		px = wb.minRowH
	}
	wb.rows[idx] = float64(px)
	wb.pixelRows = true
	wb.recalc()
}

// ToggleWidget sets the visibility of a widget.
func (wb *WidgetBoard) ToggleWidget(id WidgetKind, visible bool) {
	p, ok := wb.placements[id]
	if !ok {
		return
	}
	p.Visible = visible
	wb.placements[id] = p
}

// LayoutGroup creates a LayoutGroup whose bounds match the widget's Rect().
// cols and rows define the grid subdivision within the widget cell.
func (wb *WidgetBoard) LayoutGroup(id WidgetKind, cols, rows []float64) *LayoutGroup {
	return NewLayoutGroup(string(id), wb.Rect(id), cols, rows)
}

func minIntClamp(a, b int) int {
	if a < b {
		return a
	}
	return b
}
