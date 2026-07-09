//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// visibleSamplerKnobRects returns the laid-out (non-empty) knob dial rects keyed
// by their original knob index.
func visibleSamplerKnobRects(dv *DrumView) map[int]image.Rectangle {
	out := map[int]image.Rectangle{}
	for i, k := range dv.sampler.knobs {
		if k == nil || k.Rect().Empty() {
			continue
		}
		out[i] = k.Rect()
	}
	return out
}

// TestSamplerKnobsMultiColumnMobile pins the mobile Sampler tab to a
// width-adaptive multi-column knob grid (2–4 columns, ControlGrid's native
// behaviour capped at 4) so the panel's horizontal space is used instead of
// stacking one pill per row. On a REAL 390×844 mobile layout at Spacious
// density the grid adapts to 4 columns, so the whole first row of knobs is
// visible at once (the second row scrolls, exactly like the Synth tab).
func TestSamplerKnobsMultiColumnMobile(t *testing.T) {
	g := newMobileSamplerTabGameForTestSize(t, 390, 844)

	grid := g.drum.sampler.knobGrid
	if grid == nil {
		t.Fatal("sampler knob grid not created on mobile")
	}
	cols := grid.Cols()
	if cols < 2 || cols > 4 {
		t.Errorf("mobile sampler grid Cols()=%d, want 2..4 (adaptive, capped at 4)", cols)
	}
	vis := visibleSamplerKnobRects(g.drum)
	// The whole first ROW must be visible — one knob per column.
	if len(vis) < cols {
		t.Errorf("only %d knobs visible; the full first row of %d columns must fit", len(vis), cols)
	}
	xs := map[int]bool{}
	for _, r := range vis {
		xs[r.Min.X] = true
	}
	if len(xs) < 2 {
		t.Errorf("mobile sampler knobs must span multiple columns; all share one X (rects=%v)", vis)
	}
	// Knobs 0 and 1 sit in the same (first) row → same top edge.
	if vis[0].Min.Y != vis[1].Min.Y {
		t.Errorf("knobs 0 and 1 should share the first row: Y %d vs %d", vis[0].Min.Y, vis[1].Min.Y)
	}
}

// TestSamplerKnobsSingleRowDesktop guards that the desktop layout is untouched:
// all five knobs in one side-by-side row, no scrolling.
func TestSamplerKnobsSingleRowDesktop(t *testing.T) {
	g := samplerLayoutGame(t)
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	g.drum.sampler.captureFromSynth("kick")
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 180), "kick")

	vis := visibleSamplerKnobRects(g.drum)
	if len(vis) != samplerKnobCount {
		t.Errorf("desktop must show all %d knobs, got %d", samplerKnobCount, len(vis))
	}
	ys := map[int]bool{}
	for _, r := range vis {
		ys[r.Min.Y] = true
	}
	if len(ys) != 1 {
		t.Errorf("desktop sampler knobs must be a single row, got %d distinct Y", len(ys))
	}
}

// samplerScrollGame builds a mobile Sampler tab on a panel short enough that the
// single-column knob list overflows and must scroll. Returns the game and the
// content rect used so callers can re-layout after a scroll.
func samplerScrollGame(t *testing.T) (*Game, image.Rectangle) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(390, 720)
	g.drum.eqPanelZone.SetActiveTab(TabSampler)
	withSmallScreen(t, true)
	restore := SetDensityForTest(DensitySpacious)
	t.Cleanup(restore)
	g.drum.sampler.captureFromSynth("kick")
	// A deliberately short content rect so the five single-column knob cells
	// cannot all fit — the overflow is what the scrollbar exists for.
	contentR := image.Rect(0, 0, 390, 210)
	g.drum.buildSamplerTab(contentR, "kick")
	return g, contentR
}

// TestSamplerKnobsScrollOverflowMobile proves that when the single-column knob
// list overflows the mobile panel, the grid reports scroll, some knobs are
// hidden (off-window empty rects), at least one stays visible, and the scroll
// hit areas are published so the user can actually scroll.
func TestSamplerKnobsScrollOverflowMobile(t *testing.T) {
	g, _ := samplerScrollGame(t)
	grid := g.drum.sampler.knobGrid
	if grid == nil {
		t.Fatal("sampler knob grid not created on mobile")
	}
	if !grid.HasScroll() {
		t.Fatalf("expected sampler knob grid to overflow/scroll on a short mobile panel")
	}
	vis := visibleSamplerKnobRects(g.drum)
	if len(vis) == 0 {
		t.Error("no knob visible while scrolling")
	}
	if len(vis) >= samplerKnobCount {
		t.Errorf("expected some knobs scrolled off-window, but all %d are visible", samplerKnobCount)
	}

	var hasScrollArea bool
	for _, a := range g.drum.samplerTabHitAreas() {
		if strings.HasPrefix(a.Tag, "sampler-scroll") {
			hasScrollArea = true
		}
	}
	if !hasScrollArea {
		t.Error("no sampler scroll hit area published while the knob list overflows")
	}
}

// TestSamplerScrollRevealsLaterKnobs drives the real scroll input (wheel notch +
// per-frame Tick to clear the cooldown) and asserts the last knob (Gain), which
// starts off-window, becomes visible after scrolling to the bottom while the
// first knob (Start) scrolls off.
func TestSamplerScrollRevealsLaterKnobs(t *testing.T) {
	g, contentR := samplerScrollGame(t)
	dv := g.drum
	grid := dv.sampler.knobGrid
	if grid == nil || !grid.HasScroll() {
		t.Fatal("precondition: sampler knob grid must scroll")
	}

	gainVisible := func() bool { return !dv.sampler.knobs[samplerKnobGain].Rect().Empty() }
	startVisible := func() bool { return !dv.sampler.knobs[samplerKnobStart].Rect().Empty() }

	if gainVisible() {
		t.Fatal("precondition: Gain (last knob) should start off-window on a short panel")
	}
	if !startVisible() {
		t.Fatal("precondition: Start (first knob) should be visible before scrolling")
	}

	// Scroll to the bottom: a wheel notch (negative = scroll DOWN, toward later
	// rows) advances one row, then is throttled by the cooldown; Tick clears it so
	// the next notch lands. Re-layout after every successful step so the visible
	// window updates.
	for i := 0; i < 500 && !gainVisible(); i++ {
		if grid.WheelStep(-1) {
			dv.buildSamplerTab(contentR, "kick")
		} else {
			grid.Tick()
		}
	}
	if !gainVisible() {
		t.Error("Gain knob never became visible after scrolling to the bottom")
	}
	if startVisible() {
		t.Error("Start knob should scroll off-window once the list is scrolled to the bottom")
	}
}

// TestSamplerKnobCellsNoOverlapMobile guards that the per-knob cells (which
// carry the group label above the dial) never overlap on the scrolling mobile
// layout — the label of one knob must never collide with the caption/badge of
// the knob above it.
func TestSamplerKnobCellsNoOverlapMobile(t *testing.T) {
	g, _ := samplerScrollGame(t)
	s := &g.drum.sampler
	type named struct {
		idx int
		r   image.Rectangle
	}
	var cells []named
	for i := range s.knobs {
		if s.knobs[i] == nil || s.knobs[i].Rect().Empty() {
			continue
		}
		c := s.knobCells[i]
		if c.Empty() {
			continue
		}
		cells = append(cells, named{i, c})
	}
	for i := 0; i < len(cells); i++ {
		for j := i + 1; j < len(cells); j++ {
			if !cells[i].r.Intersect(cells[j].r).Empty() {
				t.Errorf("sampler knob cells %d %v and %d %v overlap on mobile",
					cells[i].idx, cells[i].r, cells[j].idx, cells[j].r)
			}
		}
	}
}

// TestSamplerKnobCellReservesGroupLabelBandMobile asserts every mobile knob cell
// reserves a band above the dial for the group label, so the label (drawn above
// the dial, clamped into the cell) never overlaps the dial itself.
func TestSamplerKnobCellReservesGroupLabelBandMobile(t *testing.T) {
	g, _ := samplerScrollGame(t)
	s := &g.drum.sampler
	band := TextHeight()
	for i := range s.knobs {
		if s.knobs[i] == nil || s.knobs[i].Rect().Empty() {
			continue
		}
		cell := s.knobCells[i]
		dialTop := s.knobs[i].Rect().Min.Y
		if dialTop-cell.Min.Y < band {
			t.Errorf("knob %d cell top %d leaves only %dpx above the dial top %d; need >= %d for the group label",
				i, cell.Min.Y, dialTop-cell.Min.Y, dialTop, band)
		}
	}
}
