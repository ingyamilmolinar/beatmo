package ui

// Each card is as wide as its dials need: a uniform grid gave a one-dial card a four-dial width and pushed
// cards past two rows at 1280x720. Rows ride ControlGrid as full-width cells, so scrolling works as elsewhere.

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// The caller caps it at the row width.
func productionCardMinW(label string, n int) int {
	in := 2 * macroCardInset()
	w := in + StyledTextWidth(label, RoleBody)
	if n > 0 {
		dialsW := in + n*macroDialCellMinW() + (n-1)*SpaceSM
		w = max(w, dialsW)
	}
	return w
}

// A card wider than avail gets a row of its own.
func packCardRows(widths []int, avail, gap int) [][]int {
	var rows [][]int
	var row []int
	used := 0
	for i, w := range widths {
		need := w
		if len(row) > 0 {
			need += gap
		}
		if len(row) > 0 && used+need > avail {
			rows = append(rows, row)
			row, used, need = nil, 0, w
		}
		row = append(row, i)
		used += need
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	return rows
}

// The extra is shared in proportion, the remainder to the last card; a row over avail is clamped card by card.
func justifyRow(widths []int, avail, gap int) []int {
	out := make([]int, len(widths))
	total := 0
	for _, w := range widths {
		total += w
	}
	free := avail - gap*(len(widths)-1)
	if total <= 0 || free <= 0 {
		return out
	}
	given := 0
	for i, w := range widths {
		out[i] = w * free / total
		given += out[i]
	}
	out[len(out)-1] += free - given
	return out
}

// layoutProductionCards lays the compact cards out as flow rows inside cardsR.
func (p *synthPanel) layoutProductionCards(grid *ControlGrid, cardsR image.Rectangle, resolved instrument.ID, cards []macroGroup) {
	dv := p.dv
	widths := make([]int, len(cards))
	rowsFor := func(availW int) (rows [][]int, rowH int) {
		for i, cg := range cards {
			widths[i] = min(productionCardMinW(cg.Label, min(len(cg.Dials), synthdial.MaxDialsPerCard)), availW)
		}
		rows = packCardRows(widths, availW, SpaceSM)
		for _, row := range rows {
			ws := make([]int, len(row))
			for k, i := range row {
				ws[k] = widths[i]
			}
			for k, w := range justifyRow(ws, availW, SpaceSM) {
				n := min(len(cards[row[k]].Dials), synthdial.MaxDialsPerCard)
				rowH = max(rowH, macroRootCardH(macroDialRows(n, w)))
			}
		}
		if rowH == 0 {
			rowH = macroRootCardShortH()
		}
		return rows, rowH
	}
	grid.SetMaxCols(1)
	grid.SetSmoothScroll(true)
	grid.SetPeekBand(false)
	rows, rowH := rowsFor(cardsR.Dx())
	grid.Layout(cardsR, len(rows), cardsR.Dx(), rowH, 0, SpaceSM)
	// A scrollbar narrows the cell: re-pack at the real width.
	if cw := grid.CellW(); cw > 0 && cw != cardsR.Dx() {
		rows, rowH = rowsFor(cw)
		grid.Layout(cardsR, len(rows), cardsR.Dx(), rowH, 0, SpaceSM)
	}
	for r, row := range rows {
		cell, vis := grid.CellRect(r)
		if !vis {
			continue
		}
		ws := make([]int, len(row))
		for k, i := range row {
			ws[k] = widths[i]
		}
		x := cell.Min.X
		for k, w := range justifyRow(ws, cell.Dx(), SpaceSM) {
			cg := cards[row[k]]
			slot := image.Rect(x, cell.Min.Y, x+w, cell.Max.Y)
			x += w + SpaceSM
			card := macroCardSlot{id: cg.ID, spriteID: cg.spriteID, label: cg.Label, gloss: cg.Gloss, r: slot}
			card.nDials = dv.macroCardDials(cg, resolved, &card.dials)
			card.dialRects = macroCardDialRects(slot, card.nDials)
			dv.instEditorMacroCards = append(dv.instEditorMacroCards, card)
		}
	}
}
