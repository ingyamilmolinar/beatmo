package ui

// Pure geometry over a cellAt callback, so it is testable without a grid.

import "image"

// The caller feeds rowMax+1 to SetContentRows so the reflow scrolls instead of clipping. No on-window rect is
// empty; none overlap.
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
	// Without a divider the armed control stays in its clicked cell; the divider path keeps earliest-free placement.
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
			// The divider stays a full-width band, so the advanced tier after it is floored below and never packs up
			// beside the armed block.
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

// Every other control moves DOWN only and keeps its column: with column changes each card's straight-line
// path flew diagonally across its siblings. The cost is ragged column ends.
func reflowArmedAnchored(order []int, armedPos, cols, spanCols, spanRows int, cellAt func(row, col int) image.Rectangle) ([]image.Rectangle, int) {
	rects := make([]image.Rectangle, len(order))
	rowMax := 0
	mark := func(r int) {
		if r > rowMax {
			rowMax = r
		}
	}
	// A last-in-row control keeps its cell as the block's RIGHT column.
	armedRow := armedPos / cols
	armedCol := armedPos % cols
	if armedCol+spanCols > cols {
		armedCol = cols - spanCols
	}
	if armedCol < 0 {
		armedCol = 0
	}
	// The block only ever grows down (and sideways), never up: that would pull the card's top off the point the
	// user clicked. An off-window lower row is handled by spanRect and the caller's height stretch.
	mark(armedRow + spanRows - 1)
	nextBelow := make([]int, cols)
	for c := range nextBelow {
		nextBelow[c] = armedRow + spanRows
	}
	inBlockCol := func(c int) bool { return c >= armedCol && c < armedCol+spanCols }
	for i := range order {
		if i == armedPos {
			// All four corners, so an off-window bottom row still yields the block's full width.
			rects[i] = spanRect(
				cellAt(armedRow, armedCol),
				cellAt(armedRow, armedCol+spanCols-1),
				cellAt(armedRow+spanRows-1, armedCol),
				cellAt(armedRow+spanRows-1, armedCol+spanCols-1),
			)
			continue
		}
		// Only controls inside the block's columns at or below its first row are displaced.
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

// Empty corners are dropped: image.Rect would normalize a {0,0} corner into a giant rect at the origin.
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
