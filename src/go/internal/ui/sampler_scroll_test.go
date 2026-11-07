//go:build test

package ui

import (
	"image"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Keyed by knob index.
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

// 2–4 columns (ControlGrid capped at 4), so the width is used; at 390×844 Spacious the first row of four shows at
// once and the rest scrolls.
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

// All five knobs in one row, no scrolling.
func TestSamplerKnobsSingleRowDesktop(t *testing.T) {
	g := samplerLayoutGame(t)
	restore := SetDensityForTest(DensityComfortable)
	defer restore()
	g.drum.sampler.captureFromSynth(roster.Kick)
	g.drum.buildSamplerTab(image.Rect(0, 0, 1280, 180), roster.Kick)

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

// Short enough that the single-column list overflows.
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
	g.drum.sampler.captureFromSynth(roster.Kick)
	contentR := image.Rect(0, 0, 390, 210)
	g.drum.buildSamplerTab(contentR, roster.Kick)
	return g, contentR
}

// The grid scrolls, some knobs are off-window, one stays visible, and the scroll hit areas exist.
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

// A wheel notch plus a Tick per frame to clear the cooldown: Gain scrolls in, Start scrolls out.
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

	// Negative scrolls down; re-lay-out after each step.
	for i := 0; i < 500 && !gainVisible(); i++ {
		if grid.WheelStep(-1) {
			dv.buildSamplerTab(contentR, roster.Kick)
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

// One knob's group label never collides with the caption above it.
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

// A band above the dial for the group label.
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
