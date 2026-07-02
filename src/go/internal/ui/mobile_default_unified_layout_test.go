//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestMobileDefault_NoEQPeekStripBelowRack asserts the unified-layout
// invariant: on the default mobile boot (mobile_default scene) there
// must NOT be a redundant EQ peek strip between the row rack and the
// bottom segmented control.
//
// Background (2026-05-10 screenshot review): the 24-px EQ peek sparkline
// renders as a solid black band on default boot because every band sits
// at 0 dB (flat curve = invisible polyline). The bottom segmented
// control already exposes an "EQ" tab providing the same expand-EQ
// affordance, making the peek a dead, useless surface that the user
// flagged as a "weird black section". Unifying the bottom-of-pane
// stack to {rack → bottom action bar} removes the wasted band.
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

// TestMobileDefault_RackBottomAbutsActionBar asserts that the row-rack
// area extends all the way down to the bottom action bar with no
// intervening surface (peek strip, gap, or anything else). This is the
// load-bearing invariant for the unified bottom-of-pane layout: the
// addRow button anchors flush with the bottom action bar so there is
// no orphan surface between them.
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

// fillMobileRackToScreenshotScenario brings the layout into the same
// state as the mobile_default browser scene used in the 2026-05-10
// screenshot review: 4 data rows AND a drum-pane height auto-sized to
// fit those rows + the addRow + the bar tightly. The browser scene
// loads 4 rows from the WASM default circuit before its first Layout;
// the Go-side test harness boots with 1 row, so we add 3 then re-run
// `g.Layout` so `adaptiveMobilePortraitSplitY` recomputes the split Y
// for the new row count.
func fillMobileRackToScreenshotScenario(t *testing.T, g *Game) {
	t.Helper()
	relayout := func() {
		g.Layout(390, 844)
		g.drum.refreshWidgetLayout()
		g.drum.recalcButtons()
		g.drum.calcLayout()
	}
	relayout()
	// Add rows until the rack is saturated: keep going until the visible-row
	// count stops growing (rows now overflow the rack) plus a couple extra so
	// the snap-flush precondition (slack < rowHeight) holds regardless of the
	// exact row-height / rack geometry. A fixed count (was 4) under-fills once
	// the mobile row height grows, leaving the snap-flush branch dormant.
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

// TestMobileDefault_AddRowBtnFlushBelowLastRow asserts that the addRow
// button sits immediately below the last visible drum row when the
// rack is near-saturated (< 1 row of slack remaining) — no orphan
// rack-background gap parked between the rows and the addRow band.
//
// Background (2026-05-10 screenshot review): the rack is taller than
// the content (4 rows × rh + 1 addRow × rh ≈ 220 px) by the rack-height
// modulo, leaving ~40 px of slack. The historical "addRow always
// pinned to rack bottom" anchor parked that slack above the addRow,
// producing a visible black band between the last drum row and the
// "+" button. Tight-layout fix: snap addRow flush below the last row
// when the slack-to-rack-bottom is < rh.
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

	// Sanity: this scenario must be near-saturated so the snap-flush
	// branch fires. If this fails, the helper above isn't filling the
	// rack enough — fix the helper, not the assertion.
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

// TestMobileDefault_NoGapBelowAddRowBtn asserts the addRow button's
// bottom edge sits within one inset (SpaceXS) of the bottom action bar
// — no orphan rack-background band between the button and the bar.
//
// Background (2026-05-10 follow-up to the snap-flush fix): when
// `positionAddRowBtn` snaps addRow flush below the last row, any
// rack-height modulo slack moves from above the button to below it.
// On iPhone-class portraits the slack is ~42 px, which renders as a
// solid rack-color strip between the addRow buttons and the bar — the
// same visual problem the snap-flush was meant to solve, just relocated.
//
// Eliminating this requires a *structural* fix: the rack height must
// equal `rendered_rows*rh + addRow*rh` exactly. Either rh expands to
// fill the rack, the rack contracts to fit the content, or the bar
// extends upward — but slack must not live as visible surface.
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

// TestMobileDefault_RowZoomChipsAnchoredToTimeline asserts the row-zoom
// +/− chips are anchored to the timeline header band (NOT inline with
// the addRow button) so they sit next to the timeline ruler they affect.
// The chip strip's Y band must overlap the timeline ruler bar and sit
// strictly above the addRow button — even with the rack full and
// addRow snapped flush below the last row.
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

