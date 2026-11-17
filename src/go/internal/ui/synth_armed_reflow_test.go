package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// stubCellAt: uniform 10x10 cells, no gaps, origin 0.
func stubCellAt(row, col int) image.Rectangle {
	return image.Rect(col*10, row*10, col*10+10, row*10+10)
}

func TestReflowArmed_BlockIs2x2_NothingHidden_NoOverlap(t *testing.T) {
	order := []int{10, 11, 12, 13, 14, 15} // 6 controls
	cols, spanCols, spanRows := 3, 2, 2
	armedPos := 0
	rects, rowMax := reflowArmed(order, armedPos, cols, spanCols, spanRows, stubCellAt)

	if got := rects[armedPos]; got.Dx() != 20 || got.Dy() != 20 {
		t.Fatalf("armed block want 20x20 got %v", got)
	}
	for i, r := range rects {
		if r.Empty() {
			t.Fatalf("control at order[%d] has empty rect (hidden)", i)
		}
	}
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].Overlaps(rects[j]) {
				t.Fatalf("rects %d and %d overlap: %v %v", i, j, rects[i], rects[j])
			}
		}
	}
	if rowMax < 1 {
		t.Fatalf("a 2x2 block spans >=2 rows; rowMax=%d", rowMax)
	}
}

func TestReflowArmed_ClampsSpanToCols(t *testing.T) {
	order := []int{0, 1, 2}
	rects, _ := reflowArmed(order, 0, 1, 2, 2, stubCellAt) // 1-col grid
	if rects[0].Dx() != 10 {
		t.Fatalf("1-col block width must clamp to one cell (10), got %d", rects[0].Dx())
	}
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].Overlaps(rects[j]) {
				t.Fatalf("overlap %d %d", i, j)
			}
		}
	}
}

// Arming a middle control still hides nothing and keeps the block 2x2.
func TestReflowArmed_MiddleArmed(t *testing.T) {
	order := []int{0, 1, 2, 3, 4, 5, 6}
	rects, _ := reflowArmed(order, 3, 4, 2, 2, stubCellAt)
	if rects[3].Dx() != 20 || rects[3].Dy() != 20 {
		t.Fatalf("armed block want 20x20 got %v", rects[3])
	}
	for i, r := range rects {
		if r.Empty() {
			t.Fatalf("order[%d] hidden", i)
		}
	}
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if rects[i].Overlaps(rects[j]) {
				t.Fatalf("overlap %d %d: %v %v", i, j, rects[i], rects[j])
			}
		}
	}
}

// In-place anchor: the armed 2×2 block must always COVER the clicked control's
// resting cell, and its top row must equal the clicked cell's row (never drop to
// the row below). Checked for every position in a 3-col grid, including the
// right-edge cells that used to bump the block down a row.
func TestReflowArmed_BlockContainsClickedCell(t *testing.T) {
	order := []int{0, 1, 2, 3, 4, 5}
	cols, spanCols, spanRows := 3, 2, 2
	for armedPos := range order {
		rects, _ := reflowArmed(order, armedPos, cols, spanCols, spanRows, stubCellAt)
		block := rects[armedPos]
		clicked := stubCellAt(armedPos/cols, armedPos%cols)
		if block.Union(clicked) != block {
			t.Errorf("armedPos=%d: block %v does not contain clicked cell %v", armedPos, block, clicked)
		}
		if block.Min.Y != clicked.Min.Y {
			t.Errorf("armedPos=%d: block top row Y=%d must equal clicked row Y=%d (must not drop below)", armedPos, block.Min.Y, clicked.Min.Y)
		}
	}
}

// Right-edge arm (last control in a row): the block clamps left so the clicked
// cell becomes the block's RIGHT column — its right edge aligns with the clicked
// cell's right edge and stays in the clicked row.
func TestReflowArmed_RightEdgeKeepsClickedCellAsRightColumn(t *testing.T) {
	order := []int{0, 1, 2, 3, 4, 5}
	cols, spanCols, spanRows := 3, 2, 2
	armedPos := 2 // row 0, last column
	rects, _ := reflowArmed(order, armedPos, cols, spanCols, spanRows, stubCellAt)
	block := rects[armedPos]
	clicked := stubCellAt(0, 2)
	if block.Max.X != clicked.Max.X {
		t.Errorf("block right edge %d must align with clicked cell right edge %d", block.Max.X, clicked.Max.X)
	}
	if block.Min.Y != 0 {
		t.Errorf("block must stay in the clicked row (Y=0), got Y=%d", block.Min.Y)
	}
}

// Right-edge arm pushes the immediate LEFT neighbour to a strictly lower row
// (never hides it) so the clicked control keeps its slot.
func TestReflowArmed_LeftNeighbourPushedBelowOnRightEdgeArm(t *testing.T) {
	order := []int{0, 1, 2, 3, 4, 5}
	cols, spanCols, spanRows := 3, 2, 2
	armedPos := 2 // clicked; its left neighbour is order index 1 (resting cell (0,1))
	rects, _ := reflowArmed(order, armedPos, cols, spanCols, spanRows, stubCellAt)
	neighbour := rects[1]
	if neighbour.Empty() {
		t.Fatal("left neighbour must reflow (not be hidden)")
	}
	if neighbour.Min.Y <= stubCellAt(0, 1).Min.Y {
		t.Errorf("left neighbour must be pushed below its resting row; resting Y=%d got Y=%d", stubCellAt(0, 1).Min.Y, neighbour.Min.Y)
	}
}

// Functional: arming a drilled macro control reflows (nothing hidden) and the
// armed control grows to a 2×2-ish block after the animation settles.
func TestArmedGrow_ReflowNothingHiddenAfterSettle(t *testing.T) {
	g := newViolinMacroGame(t)
	dv := g.drum
	_, kIdx := drillMacro(t, g, "brightness")
	order := dv.macroConceptKnobIdxs("brightness")
	rest := map[int]bool{}
	for _, k := range order {
		if !dv.instEditorControls[k].Rect().Empty() {
			rest[k] = true
		}
	}
	dv.armSynthControl(dv.synthTabActiveInstrument(), kIdx)
	for i := 0; i < 50 && dv.advanceArmGrow(); i++ {
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	}
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	for k := range rest {
		if dv.instEditorControls[k].Rect().Empty() {
			t.Errorf("control %d hidden after arming (should reflow, not hide)", k)
		}
	}
	armedR := dv.instEditorControls[kIdx].Rect()
	if a := armedR.Dx() * armedR.Dy(); a < 2*grid0Area(dv, order, kIdx) {
		t.Errorf("armed control did not grow to a 2×2-ish block: %v (area %d)", armedR, a)
	}
}

// Bottom-edge regression: when the armed control sits on the LAST visible row,
// the 2×2 block's bottom cells are off the scroll window (cellAt returns an empty
// rect). reflowArmed must NOT normalize that into a giant rect anchored at the
// origin (image.Rect(tl, {0,0}) → Rect(0,0,tl…)) — the "Voice Tone2 Freq expands
// the control above and much larger" bug. The block must stay in the content area
// and a sane size (it shifts UP so the clicked control is the block's bottom row).
func TestReflowArmed_BottomEdgeDoesNotProduceGiantRect(t *testing.T) {
	const visRows = 2 // window shows rows [0,2); row 2+ is off-window
	cellAt := func(row, col int) image.Rectangle {
		if row < 0 || row >= visRows { // mirrors ControlGrid.CellAt off-window
			return image.Rectangle{}
		}
		return image.Rect(100+col*10, 100+row*10, 100+col*10+10, 100+row*10+10)
	}
	order := []int{0, 1, 2, 3} // cols=2 → rows 0 and 1
	armedPos := 2              // row 1 = the last visible row
	rects, _ := reflowArmed(order, armedPos, 2, 2, 2, cellAt)
	block := rects[armedPos]
	if block.Min.X < 100 || block.Min.Y < 100 {
		t.Fatalf("armed block escaped the content area (giant-rect bug): %v", block)
	}
	if block.Dx() > 40 || block.Dy() > 40 {
		t.Fatalf("armed block absurdly large from an off-window corner: %v", block)
	}
	// The clicked control's cell (row 1) must still be covered by the block.
	clicked := cellAt(1, 0)
	if block.Union(clicked) != block {
		t.Fatalf("block %v must still cover the clicked cell %v", block, clicked)
	}
	// No control may land on the giant/empty origin either.
	for i, r := range rects {
		if !r.Empty() && (r.Min.X < 100 || r.Min.Y < 100) {
			t.Fatalf("control order[%d] rect %v escaped content area", i, r)
		}
	}
}

// Down-only growth: arming a control on the LAST visible row must anchor the
// block at the CLICKED row (grow DOWN) — never shift the block UP into the row
// above, which would move the previous control upward "into nothing". The card's
// top edge stays where the user clicked.
func TestReflowArmed_GrowsDownNotUp(t *testing.T) {
	const visRows = 2 // rows 0,1 on-window; row 2+ off-window
	cellAt := func(row, col int) image.Rectangle {
		if row < 0 || row >= visRows {
			return image.Rectangle{}
		}
		return image.Rect(100+col*10, 100+row*10, 100+col*10+10, 100+row*10+10)
	}
	order := []int{0, 1, 2, 3} // cols=2 → rows 0 and 1
	armedPos := 2              // row 1 = the last visible row
	rects, _ := reflowArmed(order, armedPos, 2, 2, 2, cellAt)
	block := rects[armedPos]
	clicked := cellAt(1, 0)
	if block.Min.Y != clicked.Min.Y {
		t.Fatalf("block top must stay at the clicked row (grow DOWN), not shift up: block.Min.Y=%d clicked.Min.Y=%d", block.Min.Y, clicked.Min.Y)
	}
	// No control may move UP from its resting row — every displaced control reflows
	// down or stays put; nothing rises "into nothing" above where it was.
	for i, r := range rects {
		resting := cellAt(i/2, i%2) // cols==2
		if !r.Empty() && r.Min.Y < resting.Min.Y {
			t.Fatalf("control order[%d] moved UP: rect %v, resting %v", i, r, resting)
		}
	}
}

// One-visible-row regression ("controls do not expand, just show in-place"):
// when only the top row is on-window (visRows=1), the armed block's bottom cells
// are off-window. The card must still grow to its full ON-WINDOW width (the two
// top-row cells) rather than collapsing to a single cell — otherwise the two
// barrel lanes have no room and the control looks un-expanded.
func TestReflowArmed_OneVisibleRowStillGrowsWidth(t *testing.T) {
	cellAt := func(row, col int) image.Rectangle {
		if row != 0 { // only row 0 is on-window
			return image.Rectangle{}
		}
		return image.Rect(100+col*10, 100, 100+col*10+10, 110)
	}
	order := []int{0, 1, 2, 3, 4, 5} // cols=6, single row
	rects, _ := reflowArmed(order, 2, 6, 2, 2, cellAt)
	block := rects[2]
	if block.Dx() < 20 {
		t.Fatalf("armed block must grow to >=2 cells wide even with 1 visible row; got %v (Dx=%d)", block, block.Dx())
	}
	if block.Min.Y < 100 {
		t.Fatalf("block escaped the content area: %v", block)
	}
}

// Functional wiring guard: arming the RIGHTMOST control in the top row must keep
// it in that row (its top Y unchanged) instead of dropping the grown block to the
// row below — the reported bad-UX behaviour.
func TestArmedGrow_LastInRowStaysInItsRow(t *testing.T) {
	g := newViolinMacroGame(t)
	dv := g.drum
	drillMacro(t, g, "brightness")
	order := dv.macroConceptKnobIdxs("brightness")

	minY := 1 << 30
	for _, k := range order {
		if r := dv.instEditorControls[k].Rect(); !r.Empty() && r.Min.Y < minY {
			minY = r.Min.Y
		}
	}
	lastK, lastX := -1, -(1 << 30)
	for _, k := range order {
		r := dv.instEditorControls[k].Rect()
		if !r.Empty() && r.Min.Y == minY && r.Min.X > lastX {
			lastX, lastK = r.Min.X, k
		}
	}
	if lastK < 0 {
		t.Skip("no visible top-row control to arm")
	}
	restY := dv.instEditorControls[lastK].Rect().Min.Y
	dv.armSynthControl(dv.synthTabActiveInstrument(), lastK)
	settleArmGrow(dv)
	if gotY := dv.instEditorControls[lastK].Rect().Min.Y; gotY != restY {
		t.Errorf("armed rightmost top-row control dropped rows: resting Y=%d, armed Y=%d", restY, gotY)
	}
}

// Functional regression for the "Voice Tone2 Freq expands the control above and
// much larger" bug: arming the BOTTOM-most visible control of a SCROLLING section
// (its 2×2 block's lower row is off-window) must not produce a giant rect that
// escapes above the grid content. Drives the real growArmedCard path.
func TestArmedGrow_BottomVisibleControlNoGiantRect(t *testing.T) {
	g := newModularSynthTabGame(t)
	audio.SetInstrumentParam("ut-chip-modular", "osc_type", 4) // FM operators exist only on the FM oscillator
	expandSynthPanelForTest(t, g)
	dv := g.drum
	// A narrow layout wraps the FM section to many rows so it overflows into a
	// scroll window (same setup as TestGridWhitespaceWheelScrollsNotNudges).
	idx := synthIdxByParam(t, g, "fm_op1_ratio")
	selectSectionForKnobIdx(t, g, "ut-chip-modular", idx)
	g.Layout(420, 720)
	g.Update()
	img := newTrackedImage("test.armed.bottomedge", 420, 720)
	defer releaseImage(img)
	g.Draw(img)

	grid := dv.sectionGrid(synthSectionFM)
	if grid == nil || !grid.HasScroll() {
		t.Skip("FM section not scrollable at this layout — bottom-edge arm is vacuous")
	}

	var fmSec *synthSection
	for i := range dv.instEditorSections {
		if dv.instEditorSections[i].id == synthSectionFM {
			fmSec = &dv.instEditorSections[i]
			break
		}
	}
	if fmSec == nil {
		t.Skip("no FM section")
	}
	// Bottom-most ON-window control (largest Min.Y) — its 2×2 block's lower row is
	// off-window, the exact condition that produced the giant rect.
	bottomK, bottomY := -1, -(1 << 30)
	for _, k := range fmSec.knobIdxs {
		if k < 0 || k >= len(dv.instEditorControls) {
			continue
		}
		if r := dv.instEditorControls[k].Rect(); !r.Empty() && r.Min.Y > bottomY {
			bottomY, bottomK = r.Min.Y, k
		}
	}
	if bottomK < 0 {
		t.Skip("no on-window FM control")
	}
	content := grid.ContentRect()
	dv.armSynthControl(dv.synthTabActiveInstrument(), bottomK)
	settleArmGrow(dv)

	r := dv.instEditorControls[bottomK].Rect()
	if r.Min.Y < content.Min.Y-1 {
		t.Fatalf("armed block escaped ABOVE grid content (giant-rect bug): block=%v content=%v", r, content)
	}
	if r.Min.X < content.Min.X-1 {
		t.Fatalf("armed block escaped LEFT of grid content (giant-rect bug): block=%v content=%v", r, content)
	}
	if r.Dx()*r.Dy() > content.Dx()*content.Dy() {
		t.Fatalf("armed block larger than the whole grid content (giant-rect bug): block=%v content=%v", r, content)
	}
	// It must actually EXPAND (not just show the lanes in a single cell): the card
	// grows to at least ~2 columns wide.
	if r.Dx() <= grid.CellW()+grid.HGap() {
		t.Fatalf("armed card did not expand — still one cell wide (Dx=%d, cellW=%d): %v", r.Dx(), grid.CellW(), r)
	}
}

// Functional down-only guard: arming the lowest visible control must not lift any
// control above its resting row — the card grows down, neighbours reflow down.
func TestArmedGrow_NeighboursNeverMoveUp(t *testing.T) {
	g := newViolinMacroGame(t)
	dv := g.drum
	drillMacro(t, g, "brightness")
	order := dv.macroConceptKnobIdxs("brightness")
	restY := map[int]int{}
	for _, k := range order {
		if r := dv.instEditorControls[k].Rect(); !r.Empty() {
			restY[k] = r.Min.Y
		}
	}
	bottomK, maxY := -1, -(1 << 30)
	for k, y := range restY {
		if y > maxY {
			maxY, bottomK = y, k
		}
	}
	if bottomK < 0 {
		t.Skip("no visible control to arm")
	}
	dv.armSynthControl(dv.synthTabActiveInstrument(), bottomK)
	settleArmGrow(dv)
	for _, k := range order {
		r := dv.instEditorControls[k].Rect()
		if r.Empty() {
			continue
		}
		if ry, ok := restY[k]; ok && r.Min.Y < ry-2 {
			t.Errorf("control %d moved UP after arming the lowest control: resting Y=%d, now Y=%d", k, ry, r.Min.Y)
		}
	}
}

// Functional regression for the "controls fly UPWARDS into nothing" bug: when the
// reflow pushes a neighbour onto a lattice row that lies BELOW the visible scroll
// window, ControlGrid.CellAt returns the zero rect (image.Rectangle{}) — an empty
// rect anchored at the SCREEN ORIGIN, not a "no target" sentinel. growArmedCard
// discards CellAt's ok flag and lerps the neighbour's resting rect toward that
// zero rect, so for the whole animation the control travels up-and-left toward
// (0,0) while shrinking, instead of sliding DOWN into the next (scrollable) row.
//
// TestArmedGrow_NeighboursNeverMoveUp cannot catch this: it only inspects the
// SETTLED frame, where lerpRect(t>=1) returns the empty target exactly and the
// control is skipped as empty. The flight is only visible mid-animation, which is
// exactly what the user sees.
//
// Invariant asserted every frame of the grow: a neighbour that is still being
// drawn (non-empty rect) never rises above its resting row, and never escapes
// above or left of the grid's content rect.
func TestArmedGrow_OffWindowNeighbourNeverFliesToOrigin(t *testing.T) {
	g := newViolinMacroGame(t)
	dv := g.drum
	// A short/narrow panel wraps the drilled concept to more rows than fit, so the
	// 2×2 armed block pushes at least one neighbour onto an OFF-WINDOW row.
	g.Layout(700, 900)
	drillMacro(t, g, "brightness")
	g.Layout(700, 900)
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())

	order := dv.macroConceptKnobIdxs("brightness")
	grid := dv.instEditorMacroGrid
	if grid == nil || grid.Cols() < 1 {
		t.Skip("no drilled macro grid at this layout")
	}
	if !grid.HasScroll() {
		t.Skip("drilled grid fits without scrolling — off-window reflow is vacuous")
	}
	content := grid.ContentRect()

	rest := map[int]image.Rectangle{}
	onWin := []int{}
	for _, k := range order {
		r := dv.instEditorControls[k].Rect()
		rest[k] = r
		if !r.Empty() {
			onWin = append(onWin, k)
		}
	}
	if len(onWin) < 2 {
		t.Skip("need >=2 on-window controls for a neighbour to be pushed off-window")
	}

	// Arm the FIRST on-window control: its 2×2 block consumes the row's remaining
	// cells, so every later neighbour reflows onto a row below the window.
	dv.armSynthControl(dv.synthTabActiveInstrument(), onWin[0])
	for f := 0; f < 50 && dv.advanceArmGrow(); f++ {
		dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
		for _, k := range onWin {
			r := dv.instEditorControls[k].Rect()
			if r.Empty() {
				continue // legitimately hidden — nothing is drawn, nothing flies
			}
			if r.Min.Y < rest[k].Min.Y-2 {
				t.Fatalf("frame %d (t=%.2f): control %d moved UP while the armed card grew: resting=%v now=%v",
					f, dv.armGrowT, k, rest[k], r)
			}
			if r.Min.Y < content.Min.Y-2 || r.Min.X < content.Min.X-2 {
				t.Fatalf("frame %d (t=%.2f): control %d escaped the grid content toward the origin: content=%v now=%v",
					f, dv.armGrowT, k, content, r)
			}
		}
	}
}

func grid0Area(dv *DrumView, order []int, except int) int {
	for _, k := range order {
		if k != except {
			r := dv.instEditorControls[k].Rect()
			if !r.Empty() {
				return r.Dx() * r.Dy()
			}
		}
	}
	return 1
}

// The grow is a brief ANIMATION, not an instant jump: one tick after arming, the
// armed control's area sits strictly between its resting and fully-grown size.
func TestArmGrow_MidAnimationRectBetween(t *testing.T) {
	g := newViolinMacroGame(t)
	dv := g.drum
	_, kIdx := drillMacro(t, g, "brightness")
	area := func() int {
		r := dv.instEditorControls[kIdx].Rect()
		return r.Dx() * r.Dy()
	}
	restA := area()
	dv.armSynthControl(dv.synthTabActiveInstrument(), kIdx)
	dv.advanceArmGrow() // one frame → partway grown
	dv.eqPanelZone.Layout(dv.eqPanelZone.PanelRect())
	midA := area()
	settleArmGrow(dv)
	grownA := area()
	if midA <= restA || midA >= grownA {
		t.Fatalf("mid-animation area must be between resting(%d) and grown(%d); got %d", restA, grownA, midA)
	}
}
