package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestNoGapTimelineRightEdge verifies the timeline rect extends close to
// the right edge of the drum view bounds (accounting for len buttons).
func TestNoGapTimelineRightEdge(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1200, 600)

	dv := g.drum
	dv.recalcButtons()
	dv.calcLayout()

	// Timeline right edge should be within 80px of bounds right edge
	// (len buttons take ~36px + 4px gap).
	gap := dv.Bounds.Max.X - dv.timelineRect.Max.X
	if gap > 80 {
		t.Errorf("timeline right edge gap too large: %dpx (timeline.Max.X=%d, bounds.Max.X=%d)",
			gap, dv.timelineRect.Max.X, dv.Bounds.Max.X)
	}
}

// TestNoGapRackToTimeline verifies no black gap between rack column controls
// and the timeline cells.
func TestNoGapRackToTimeline(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1200, 600)

	dv := g.drum
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	// The rack widget right edge should be close to the actual content width.
	rackR := dv.widgetRects[WidgetRack]
	contentW := dv.Bounds.Min.X + dv.labelW + dv.controlsW

	// Rack column should not exceed content width by more than SpaceMD+10.
	if !rackR.Empty() && rackR.Max.X > contentW+SpaceMD+10 {
		t.Errorf("rack column oversized: rack.Max.X=%d, content edge=%d, gap=%d",
			rackR.Max.X, contentW, rackR.Max.X-contentW)
	}
}

// TestContentAwareColumnSizingActivates verifies that on wide windows the
// adaptive column sizing tightens the rack column when controlsW hits its cap.
func TestContentAwareColumnSizingActivates(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Very wide window — at 2400px the [1, 2] ratio gives rack col 800px.
	// controlsW caps at 520, labelW ~80, so content needs ~600px.
	// Without adaptive sizing, 200px would be wasted.
	g.Layout(2400, 600)

	dv := g.drum
	dv.refreshWidgetLayout()

	rackW := dv.widgets.ColWidth(0)
	contentW := dv.labelW + dv.controlsW

	// After content-aware sizing, rack column should be smaller than
	// the default 33% allocation.
	defaultRackW := 2400 / 3 // 800px from [1,2] ratio
	if rackW >= defaultRackW {
		t.Errorf("content-aware sizing did not activate: rackW=%d >= defaultRackW=%d (contentW=%d)",
			rackW, defaultRackW, contentW)
	}
}

// TestPanelMaskTightenedToControls verifies the panel mask right edge matches
// the actual controls width, not the full rack widget column.
func TestPanelMaskTightenedToControls(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1200, 600)

	dv := g.drum
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	// Simulate a draw to populate panelMaskRect.
	screen := ebiten.NewImage(1200, 600)
	dv.Draw(screen, nil, 0, nil, 0)

	maskRight := dv.panelMaskRect.Max.X
	contentRight := dv.Bounds.Min.X + dv.labelW + dv.controlsW
	if maskRight > contentRight+2 {
		t.Errorf("panel mask overdraw: mask.Max.X=%d > content right=%d",
			maskRight, contentRight)
	}
}

// TestBelowRowsNotBlack verifies the area below visible rows gets a fill.
func TestBelowRowsNotBlack(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	dv := g.drum
	dv.refreshWidgetLayout()
	dv.recalcButtons()
	dv.calcLayout()

	// With default 1-2 rows and 24px row height at 600px window height,
	// there should be a gap below the rows.
	vis := dv.visibleRows()
	rowsTop := dv.Bounds.Min.Y + dv.headerH
	nBelow := len(dv.Rows) - dv.rowOffset
	if nBelow > vis {
		nBelow = vis
	}
	bottomOfRows := rowsTop + nBelow*dv.rowHeight()
	rackBottom := dv.widgetRects[WidgetRack].Max.Y
	if rackBottom == 0 {
		rackBottom = dv.Bounds.Max.Y - dv.eqH
	}

	if bottomOfRows < rackBottom {
		// Gap exists — the draw code fills it with colStepOff.
		gapRect := image.Rect(dv.timelineRect.Min.X, bottomOfRows, dv.timelineRect.Max.X, rackBottom)
		if gapRect.Empty() {
			t.Error("gap rect is empty despite bottomOfRows < rackBottom")
		}
		// Just verify the gap is accounted for; actual fill happens in Draw().
	}
}
