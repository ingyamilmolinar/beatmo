package ui

// synth_macro_flow.go lays out the PRODUCTION root's compact category cards
// (spec 2026-10-24 §4.2) as a flow: each card is as wide as its dials need,
// cards pack left to right and wrap, and every row is stretched to the full
// width. A uniform grid gives a one-dial card the width of a four-dial card,
// which pushed ten instruments' cards past two rows at 1280x720
// (TestProductionCardsFitTwoRows). The rows ride the shared ControlGrid as
// single full-width cells, so scrolling, edge fades and the scroll body work
// exactly as on the developer root.

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// productionCardMinW is the narrowest a compact card may be for n dials and
// its title: the dial cells side by side (capped at the row width by the
// caller), or the title, whichever is wider.
func productionCardMinW(label string, n int) int {
	in := 2 * macroCardInset()
	w := in + StyledTextWidth(label, RoleBody)
	if n > 0 {
		dialsW := in + n*macroDialCellMinW() + (n-1)*SpaceSM
		w = max(w, dialsW)
	}
	return w
}

// packCardRows packs widths greedily into rows no wider than avail (gap
// between neighbours). A card wider than avail gets a row of its own. Returns
// the index lists per row, in order.
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

// justifyRow stretches a row's card widths to fill avail exactly (the extra
// shared in proportion to each card's width; rounding remainder to the last).
// A row already at or over avail is clamped card by card to avail.
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
