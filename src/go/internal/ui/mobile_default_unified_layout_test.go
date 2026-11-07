//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// No EQ peek strip between rack and bottom bar: at a flat 0 dB it read as a dead black band, and the "EQ" tab
// already offers the same thing.
func TestMobileDefault_NoEQPeekStripBelowRack(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844) // iPhone-class portrait viewport (matches mobile_default)

	if !g.drum.mobileEQCollapsed {
		t.Fatalf("precondition: expected mobileEQCollapsed=true on default boot")
	}
	if g.drum.bottomActionBarRect.Empty() {
		t.Fatalf("precondition: expected bottomActionBarRect non-empty on default boot")
	}
	if !g.drum.eqPeekRect.Empty() {
		t.Fatalf("eqPeekRect must be empty on default mobile boot — the peek strip is the useless black band; "+
			"users access EQ via the bottom segmented control's EQ tab. Got rect=%v height=%dpx",
			g.drum.eqPeekRect, g.drum.eqPeekRect.Dy())
	}
}

// The rack runs down to the bottom bar with nothing between them.
func TestMobileDefault_RackBottomAbutsActionBar(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	rb := g.drum.rowsBottom()
	barTop := g.drum.bottomActionBarRect.Min.Y
	if rb != barTop {
		t.Fatalf("rowsBottom()=%d must equal bottomActionBarRect.Min.Y=%d (no gap, no peek strip); "+
			"current diff=%dpx", rb, barTop, barTop-rb)
	}
}

// The mobile_default scene's state: the browser loads its rows before the first Layout while this harness
// boots with one, so add them and lay out again to recompute the split.
func fillMobileRackToScreenshotScenario(t *testing.T, g *Game) {
	t.Helper()
	relayout := func() {
		g.Layout(390, 844)
		g.drum.refreshWidgetLayout()
		g.drum.recalcButtons()
		g.drum.calcLayout()
	}
	relayout()
	// Until the visible count stops growing, plus a couple more, so the snap-flush precondition (slack < rowHeight)
	// holds whatever the row height; a fixed count under-fills as rows grow.
	prevVis := -1
	for i := 0; i < 64; i++ {
		vis := g.drum.rackVisibleRows()
		if vis > 0 && vis == prevVis && len(g.drum.Rows) > vis+1 {
			break
		}
		prevVis = vis
		g.drum.AddRow()
		relayout()
	}
}

// Near saturation (< 1 row of slack) the addRow button sits flush below the last row, with no rack gap above
// it.
func TestMobileDefault_AddRowBtnFlushBelowLastRow(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	fillMobileRackToScreenshotScenario(t, g)

	if g.drum.rowRackZone == nil {
		t.Fatalf("precondition: rowRackZone must exist")
	}
	addRect := g.drum.rowRackZone.AddRowBtnRect()
	if addRect.Empty() {
		t.Fatalf("precondition: addRow button rect must be non-empty")
	}
	rows := g.drum.Rows
	vis := g.drum.rackVisibleRows()
	if vis > len(rows) {
		vis = len(rows)
	}
	if vis <= 0 {
		t.Fatalf("precondition: vis=%d but expected ≥1 visible row", vis)
	}
	rackRect := g.drum.widgetRects[WidgetRack]
	if rackRect.Empty() {
		t.Fatalf("precondition: rack widget rect empty")
	}
	rh := g.drum.rowHeight()
	lastRowBottom := rackRect.Min.Y + vis*rh

	// If this fails the helper is not filling the rack: fix the helper, not the assertion.
	rackBottom := g.drum.rowsBottom()
	bottomAnchor := rackBottom - rh
	slackToBottom := bottomAnchor - lastRowBottom
	if slackToBottom >= rh {
		t.Fatalf("precondition: rack not near-saturated (slack=%d ≥ rh=%d) — "+
			"snap-flush branch will not fire; bump fillMobileRackToScreenshotScenario",
			slackToBottom, rh)
	}

	gap := addRect.Min.Y - lastRowBottom
	const maxAllowedGap = SpaceXS + 1
	if gap > maxAllowedGap {
		t.Fatalf("gap above addRow button: addRow.Min.Y=%d, lastRow bottom=%d, gap=%dpx; "+
			"expected ≤ %dpx (no orphan rack-background band between rows and addRow)",
			addRect.Min.Y, lastRowBottom, gap, maxAllowedGap)
	}
}

// Within SpaceXS of the bottom bar. Snapping addRow flush only moves the slack below it, so the rack height
// must equal the content exactly: slack must never live as visible surface.
func TestMobileDefault_NoGapBelowAddRowBtn(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	fillMobileRackToScreenshotScenario(t, g)

	if g.drum.rowRackZone == nil {
		t.Fatalf("precondition: rowRackZone must exist")
	}
	addRect := g.drum.rowRackZone.AddRowBtnRect()
	if addRect.Empty() {
		t.Fatalf("precondition: addRow button rect must be non-empty")
	}
	barTop := g.drum.bottomActionBarRect.Min.Y
	if barTop <= 0 {
		t.Fatalf("precondition: bottomActionBarRect must be non-empty")
	}
	gap := barTop - addRect.Max.Y
	const maxAllowedGap = SpaceXS + 1
	if gap > maxAllowedGap {
		t.Fatalf("gap below addRow button: addRow.Max.Y=%d, bar.Min.Y=%d, gap=%dpx; "+
			"expected ≤ %dpx (no orphan rack-background band between addRow and the bar)",
			addRect.Max.Y, barTop, gap, maxAllowedGap)
	}
}

// In the timeline header, next to the ruler they affect: overlapping the ruler band and strictly above
// addRow, even with the rack full.
func TestMobileDefault_RowZoomChipsAnchoredToTimeline(t *testing.T) {
	setupMobileTest(t, true)

	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)
	fillMobileRackToScreenshotScenario(t, g)

	addRect := g.drum.rowRackZone.AddRowBtnRect()
	if addRect.Empty() {
		t.Fatalf("precondition: addRow button rect must be non-empty")
	}
	chipRect := g.drum.rowZoomChipRect
	if chipRect.Empty() {
		t.Fatalf("precondition: rowZoomChipRect must be non-empty on default mobile")
	}
	tlRect := g.drum.timelineRect
	if tlRect.Empty() {
		t.Fatalf("precondition: timelineRect must be non-empty")
	}
	// Chips overlap the timeline ruler band vertically.
	if chipRect.Min.Y > tlRect.Max.Y || chipRect.Max.Y < tlRect.Min.Y {
		t.Fatalf("row-zoom chip Y band (%d..%d) must overlap timeline ruler Y band (%d..%d)",
			chipRect.Min.Y, chipRect.Max.Y, tlRect.Min.Y, tlRect.Max.Y)
	}
	// Chips sit strictly above the addRow button.
	if chipRect.Max.Y > addRect.Min.Y {
		t.Fatalf("row-zoom chip Y band (%d..%d) must sit strictly above addRow Y band (%d..%d)",
			chipRect.Min.Y, chipRect.Max.Y, addRect.Min.Y, addRect.Max.Y)
	}
	// Chips are right-aligned to (or within) the timeline widget X range.
	tlWidget := g.drum.widgetRects[WidgetTimeline]
	if !tlWidget.Empty() {
		if chipRect.Min.X < tlWidget.Min.X || chipRect.Max.X > tlWidget.Max.X {
			t.Fatalf("row-zoom chip X band (%d..%d) escapes timeline widget X range (%d..%d)",
				chipRect.Min.X, chipRect.Max.X, tlWidget.Min.X, tlWidget.Max.X)
		}
	}
}
