//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// rectGetter is the minimal interface satisfied by Button / Slider in
// the row-rack zone — both expose `Rect() image.Rectangle`.
type rectGetter interface {
	Rect() image.Rectangle
}

// TestRowZoomChipsDoNotOverlapAnyRow asserts that the mobile row-zoom +/-
// chips do not overlap any row's interactive controls. The legacy layout
// pinned the chips to the top-right of WidgetRack, which placed them
// directly on top of the FX cells of the topmost rows — visible in
// screenshot2.png 2026-05-09: rows Kick-1 and Snare had their FX buttons
// hidden under "+" / "-" chips. The fix relocates the chips to a
// dedicated strip alongside the addRow "+" button at the bottom of the
// rack column, where they cannot collide with any per-row control rect.
func TestRowZoomChipsDoNotOverlapAnyRow(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	dv := g.drum
	if dv == nil {
		t.Fatal("DrumView nil")
	}
	if dv.rowZoomIncBtn == nil || dv.rowZoomDecBtn == nil {
		t.Fatal("row-zoom buttons not constructed")
	}
	incRect := dv.rowZoomIncBtn.Rect()
	decRect := dv.rowZoomDecBtn.Rect()
	if incRect.Empty() && decRect.Empty() {
		t.Fatal("row-zoom chips have empty rects on mobile (must be visible above the bottom action bar)")
	}

	for i, entry := range dv.rowRackZone.entries {
		controls := []struct {
			name string
			btn  rectGetter
		}{
			{"label", entry.label},
			{"vol", entry.volSlider},
			{"mute", entry.muteBtn},
			{"solo", entry.soloBtn},
			{"fx", entry.fxBtn},
			{"menu", entry.menuBtn},
		}
		for _, c := range controls {
			r := c.btn.Rect()
			if r.Empty() {
				continue
			}
			if !incRect.Empty() && r.Overlaps(incRect) {
				t.Errorf("row %d %s rect %v overlaps zoomInc chip %v", i, c.name, r, incRect)
			}
			if !decRect.Empty() && r.Overlaps(decRect) {
				t.Errorf("row %d %s rect %v overlaps zoomDec chip %v", i, c.name, r, decRect)
			}
		}
	}

	// AddRow button shares the strip but its right margin must be
	// reserved so it cannot overlap the chips.
	if addRect := dv.rowRackZone.AddRowBtnRect(); !addRect.Empty() {
		if !incRect.Empty() && addRect.Overlaps(incRect) {
			t.Errorf("addRow rect %v overlaps zoomInc chip %v", addRect, incRect)
		}
		if !decRect.Empty() && addRect.Overlaps(decRect) {
			t.Errorf("addRow rect %v overlaps zoomDec chip %v", addRect, decRect)
		}
	}
}

// TestRowZoomChipsDoNotOverlapTimelineChrome asserts the chip strip does
// not draw over the trackBtn kebab chip, beat counter pill, or timeline
// ruler dots. The chip block in drumview_layout.go shrinks beatCounter
// and ruler Max.X to clear the chip column — this is the regression
// guard for that invariant.
func TestRowZoomChipsDoNotOverlapTimelineChrome(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 844)

	dv := g.drum
	chip := dv.rowZoomChipRect
	if chip.Empty() {
		t.Fatal("rowZoomChipRect empty on default mobile layout")
	}
	chrome := []struct {
		name string
		r    image.Rectangle
	}{
		{"trackBtn", dv.trackBtn().Rect()},
		{"beatCounter", dv.beatCounterRect},
		{"timelineRuler", dv.timelineRect},
	}
	for _, c := range chrome {
		if c.r.Empty() {
			continue
		}
		if c.r.Overlaps(chip) {
			t.Errorf("%s rect %v overlaps row-zoom chip strip %v", c.name, c.r, chip)
		}
	}
}

// TestRowZoomChipsHiddenOnDesktop asserts the chips are not rendered on
// desktop layouts — the lenInc/lenDec pair next to the timeline already
// covers that role on desktop.
func TestRowZoomChipsHiddenOnDesktop(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	dv := g.drum
	if dv.rowZoomIncBtn != nil && !dv.rowZoomIncBtn.Rect().Empty() {
		t.Errorf("desktop: zoomInc chip should be hidden; got %v", dv.rowZoomIncBtn.Rect())
	}
	if dv.rowZoomDecBtn != nil && !dv.rowZoomDecBtn.Rect().Empty() {
		t.Errorf("desktop: zoomDec chip should be hidden; got %v", dv.rowZoomDecBtn.Rect())
	}
	if !dv.rowZoomChipRect.Empty() {
		t.Errorf("desktop: rowZoomChipRect should be empty; got %v", dv.rowZoomChipRect)
	}
}

// TestRowZoomChipsHidePastBottomActionBar asserts that when the bottom
// action bar collapses (ultra-short viewports), the row-zoom chips also
// hide so they don't squat over content with no semantic anchor.
func TestRowZoomChipsHidePastBottomActionBar(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	// 140-px tall is below the threshold where bottomActionBar fits.
	g.Layout(360, 140)

	dv := g.drum
	if !dv.bottomActionBarRect.Empty() {
		t.Skip("test geometry didn't trigger bottom action bar collapse; skipping")
	}
	if dv.rowZoomIncBtn != nil && !dv.rowZoomIncBtn.Rect().Empty() {
		t.Errorf("expected zoomInc chip empty when bottom bar collapses; got %v", dv.rowZoomIncBtn.Rect())
	}
	if dv.rowZoomDecBtn != nil && !dv.rowZoomDecBtn.Rect().Empty() {
		t.Errorf("expected zoomDec chip empty when bottom bar collapses; got %v", dv.rowZoomDecBtn.Rect())
	}
}

// TestRowZoomChipsInTimelineHeader asserts the chip strip lives in the
// timeline header band (next to the ruler) — strictly above the addRow
// "+" button at the bottom of the rack column. Replaces the legacy
// "inline with addRow" placement.
func TestRowZoomChipsInTimelineHeader(t *testing.T) {
	setupMobileTest(t, true)
	logger := log.New(testLogOutput(), log.LevelInfo)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(360, 700)

	dv := g.drum
	addRect := dv.rowRackZone.AddRowBtnRect()
	incRect := dv.rowZoomIncBtn.Rect()
	decRect := dv.rowZoomDecBtn.Rect()
	if addRect.Empty() || incRect.Empty() || decRect.Empty() {
		t.Skipf("addRow=%v zoomInc=%v zoomDec=%v — preconditions not met",
			addRect, incRect, decRect)
	}
	// Chip strip sits strictly above addRow.
	if incRect.Max.Y > addRect.Min.Y {
		t.Errorf("zoomInc chip Y range [%d,%d] not strictly above addRow Y range [%d,%d]",
			incRect.Min.Y, incRect.Max.Y, addRect.Min.Y, addRect.Max.Y)
	}
	// Chip strip (inc ∪ dec) overlaps the timeline ruler band. The
	// vertical pair spans both the beat counter row and the ruler row,
	// so the union must straddle the ruler band even if one chip alone
	// sits above it.
	tl := dv.timelineRect
	if tl.Empty() {
		t.Fatal("timelineRect empty; preconditions not met")
	}
	stripY := incRect.Union(decRect)
	if stripY.Min.Y > tl.Max.Y || stripY.Max.Y < tl.Min.Y {
		t.Errorf("chip strip Y range [%d,%d] does not overlap timeline ruler Y range [%d,%d]",
			stripY.Min.Y, stripY.Max.Y, tl.Min.Y, tl.Max.Y)
	}
	// Vertical stack: inc on top, dec below — non-overlapping.
	if incRect.Max.Y > decRect.Min.Y+1 {
		t.Errorf("expected vertical stack (inc above dec); got inc=%v dec=%v", incRect, decRect)
	}
	// Same X column.
	if incRect.Min.X != decRect.Min.X || incRect.Max.X != decRect.Max.X {
		t.Errorf("expected inc/dec chips in same X column; got inc=%v dec=%v", incRect, decRect)
	}
	// Strip is to the RIGHT of the timeline ruler (no overlap with ruler dots).
	if incRect.Min.X < tl.Max.X {
		t.Errorf("zoomInc chip Min.X=%d < timeline ruler Max.X=%d — chip overlaps the ruler dots",
			incRect.Min.X, tl.Max.X)
	}
}
