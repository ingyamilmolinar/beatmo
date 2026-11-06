package ui

// synth_armed_reflow.go — masonry placement for the armed-control grow. The armed
// control occupies a spanCols×spanRows block; every OTHER control packs into the
// earliest free cell in reading order, so nothing is hidden and there are no gaps.
// Pure geometry (a cellAt callback yields each lattice cell's pixel rect), so it is
// unit-testable without a real grid.

import "image"

// reflowArmed returns an index-aligned target rect per `order` entry, with
// order[armedPos] a spanCols×spanRows block and every other control in the
// earliest free cell (reading order). rowMax is the highest lattice row used (the
// caller feeds rowMax+1 to ControlGrid.SetContentRows so the reflow scrolls rather
// than clips). No rect is empty (for on-window cells); none overlap.
func reflowArmed(order []int, armedPos, cols, spanCols, spanRows int, cellAt func(row, col int) image.Rectangle) ([]image.Rectangle, int) {
	if cols < 1 {
		cols = 1
	}
	if spanCols > cols {
		spanCols = cols
	}
	if spanCols < 1 {
		spanCols = 1
	}
	if spanRows < 1 {
		spanRows = 1
	}
	// No-divider path (macro drill-down + simple stage sections): keep the armed
	// control in its clicked cell instead of relocating the block to the earliest
	// free slot. The phantom/divider path below keeps earliestFree placement (its
	// full-width band needs the floor logic, and growArmedCard routes those
	// sections to growArmedCardLegacy anyway).
	hasPhantom := false
	for _, k := range order {
		if k < 0 {
			hasPhantom = true
			break
		}
	}
	if !hasPhantom && armedPos >= 0 && armedPos < len(order) {
		return reflowArmedAnchored(order, armedPos, cols, spanCols, spanRows, cellAt)
	}
	rects := make([]image.Rectangle, len(order))
	occ := map[[2]int]bool{}
	rowMax := 0
	floor := 0 // earliest row later controls may use (raised past the divider band)
	mark := func(r, c int) {
		occ[[2]int{r, c}] = true
		if r > rowMax {
			rowMax = r
		}
	}
	fits := func(r, c int) bool {
		if c+spanCols > cols {
			return false
		}
		for dr := 0; dr < spanRows; dr++ {
			for dc := 0; dc < spanCols; dc++ {
				if occ[[2]int{r + dr, c + dc}] {
					return false
				}
			}
		}
		return true
	}
	earliestFree := func(block bool) (int, int) {
		for lin := floor * cols; ; lin++ {
			r, c := lin/cols, lin%cols
			if occ[[2]int{r, c}] {
				continue
			}
			if !block || fits(r, c) {
				return r, c
			}
		}
	}
	// fullyFreeRow returns the first row >= floor with every column free.
	fullyFreeRow := func() int {
		for r := floor; ; r++ {
			free := true
			for c := 0; c < cols; c++ {
				if occ[[2]int{r, c}] {
					free = false
					break
				}
			}
			if free {
				return r
			}
		}
	}
	dividerRow := -1
	for i, k := range order {
		if k < 0 {
			// Advanced-tier divider phantom: it must stay a full-width band, so the
			// run reserves its own fresh row; controls after it (advanced tier) are
			// floored below it so they never pack back up beside the armed block.
			if dividerRow < 0 {
				dividerRow = fullyFreeRow()
			}
			c := 0
			for occ[[2]int{dividerRow, c}] {
				c++
			}
			mark(dividerRow, c)
			rects[i] = cellAt(dividerRow, c)
			if c == cols-1 {
				floor = dividerRow + 1 // divider row filled; advanced tier goes below
			}
			continue
		}
		if i == armedPos {
			r, c := earliestFree(true)
			for dr := 0; dr < spanRows; dr++ {
				for dc := 0; dc < spanCols; dc++ {
					mark(r+dr, c+dc)
				}
			}
			tl := cellAt(r, c)
			br := cellAt(r+spanRows-1, c+spanCols-1)
			rects[i] = image.Rect(tl.Min.X, tl.Min.Y, br.Max.X, br.Max.Y)
			continue
		}
		r, c := earliestFree(false)
		mark(r, c)
		rects[i] = cellAt(r, c)
	}
	return rects, rowMax
}

// reflowArmedAnchored keeps the armed control anchored at its CLICKED cell: the
// spanCols×spanRows block's top-left is the armed control's resting cell (row =
// armedPos/cols, col = armedPos%cols), clamped left so a right-edge control's
// block grows leftward instead of dropping to the row below.
//
// Every other control moves DOWN ONLY, never sideways: it KEEPS ITS COLUMN. A
// control outside the block's columns, or above the block's row, does not move at
// all. The rest re-stack (in their original top-to-bottom order) directly below
// the block in the same column. Nothing is hidden — the caller feeds rowMax+1 to
// SetContentRows so the pushed-down rows are scrollable — and no cells overlap.
//
// This deliberately replaces an "earliest free cell in reading order" masonry.
// That packed tighter, but a control could be reassigned to a different COLUMN,
// and since the grow interpolates each control along a straight line between its
// resting and reflowed rect, cross-column moves flew diagonally across the panel
// and crossed over their stationary siblings mid-flight. Keeping the column makes
// every path purely vertical, so cards never cross each other. The cost is that a
// column can end shorter than its neighbours; that is the intended trade.
//
// rowMax is the highest lattice row used (for SetContentRows).
func reflowArmedAnchored(order []int, armedPos, cols, spanCols, spanRows int, cellAt func(row, col int) image.Rectangle) ([]image.Rectangle, int) {
	rects := make([]image.Rectangle, len(order))
	rowMax := 0
	mark := func(r int) {
		if r > rowMax {
			rowMax = r
		}
	}
	// Anchor at the clicked cell; clamp the column so the block fits within cols
	// (a last-in-row control keeps its cell as the block's RIGHT column).
	armedRow := armedPos / cols
	armedCol := armedPos % cols
	if armedCol+spanCols > cols {
		armedCol = cols - spanCols
	}
	if armedCol < 0 {
		armedCol = 0
	}
	// DOWN-ONLY growth: the block's top-left stays at the clicked cell and the card
	// only ever grows DOWN (and right, or left at the right edge). It never shifts
	// UP into the row above — that would move the previous control upward "into
	// nothing" and pull the card's top edge off the point the user clicked. A block
	// whose lower row is off-window is handled by spanRect (it unions only the
	// on-window corners, so no giant rect) plus the caller's height stretch (grows
	// the card down toward the pane bottom), so the anchor never needs to move up.
	mark(armedRow + spanRows - 1)
	// Per block column, the next free row BELOW the block. Displaced controls stack
	// here in their original order, keeping their column.
	nextBelow := make([]int, cols)
	for c := range nextBelow {
		nextBelow[c] = armedRow + spanRows
	}
	inBlockCol := func(c int) bool { return c >= armedCol && c < armedCol+spanCols }
	for i := range order {
		if i == armedPos {
			// Union the ON-WINDOW cells the block covers. Using all four corners
			// (not just top-left + bottom-right) keeps the block's full width when
			// its bottom row is off-window (visRows==1): the top row's cells define
			// the width even though the bottom corners are empty — so the card grows
			// to the 2-column footprint instead of collapsing to one cell.
			rects[i] = spanRect(
				cellAt(armedRow, armedCol),
				cellAt(armedRow, armedCol+spanCols-1),
				cellAt(armedRow+spanRows-1, armedCol),
				cellAt(armedRow+spanRows-1, armedCol+spanCols-1),
			)
			continue
		}
		// Column is preserved. Only a control inside the block's columns AND at or
		// below the block's first row is displaced; it re-stacks directly below the
		// block. Everything else keeps its resting cell and does not move at all.
		r, c := i/cols, i%cols
		if inBlockCol(c) && r >= armedRow {
			r = nextBelow[c]
			nextBelow[c]++
		}
		mark(r)
		rects[i] = cellAt(r, c)
	}
	return rects, rowMax
}

// spanRect returns the bounding box of a block's corner cells, skipping any that
// are off-window (empty). Empties are DROPPED rather than fed to image.Rect —
// which would normalize a {0,0} corner into a giant rect anchored at the origin.
// Unioning every on-window corner keeps the block's full on-window extent: when
// the bottom row is off-window the two top corners still define the width, so the
// card grows to its 2-column footprint instead of collapsing to a single cell.
// All corners empty → empty.
func spanRect(corners ...image.Rectangle) image.Rectangle {
	var out image.Rectangle
	for _, c := range corners {
		if c.Empty() {
			continue
		}
		if out.Empty() {
			out = c
		} else {
			out = out.Union(c)
		}
	}
	return out
}
