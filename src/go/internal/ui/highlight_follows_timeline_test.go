package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestHighlightAlignsWithRibbonCursor checks that the yellow per-beat
// highlight on a drum row is rendered at the SAME horizontal screen X as the
// timeline ribbon's playhead cursor (both live inside `dv.timelineRect`).
//
// Bug under test: TrackBeat recenters the drum view so the playhead column
// sits at `length / 2` (50% of the visible window), while the ribbon pins its
// playhead cursor at `RibbonPlayheadFrac` (=0.70 → 70% of the bar width).
// During the first ~60 beats the ribbon cursor floats from the left edge so
// the two playheads coincidentally agree; once the ribbon window starts
// sliding (`winStart > 0`), the cursor pins at 70% while the cell highlight
// remains at 50% — the timeline drum view and cell highlight visibly desync.
func TestHighlightAlignsWithRibbonCursor(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.Layout(1024, 720)
	g.drum.SetFollow(true)
	g.drum.SetLength(16)
	g.drum.SetBPM(120)

	// Simple 4-node loop so beats keep firing as we fast-forward playback.
	nodes := make([]*uiNode, 4)
	for i := 0; i < len(nodes); i++ {
		nodes[i] = g.tryAddNode(i, 0, model.NodeTypeRegular)
		if i > 0 {
			g.addEdge(nodes[i-1], nodes[i])
		}
	}
	g.addEdge(nodes[len(nodes)-1], nodes[0])
	g.start = nodes[0]
	g.graph.StartNodeID = nodes[0].ID
	g.drum.Rows[0].Origin = nodes[0].ID
	g.drum.Rows[0].Node = nodes[0]

	g.updateBeatInfos()
	g.refreshDrumRow()

	dst := ebiten.NewImage(1024, 720)
	g.drum.Draw(dst, nil, 0, nil, 0)
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update at play: %v", err)
	}

	// Advance well past the ribbon's "pin" threshold so the ribbon cursor is
	// no longer growing from the left edge but pinned at RibbonPlayheadFrac.
	// At default RibbonBeatsPerPixel=0.25 and barWidth ~800 px that threshold
	// is ~60 beats; we drive 120 beats to be safely past it.
	const beats = 120
	advancePlaybackByAbs(g, beats*g.grid.MaxDiv())
	g.drum.Draw(dst, nil, 0, nil, 0)

	// Snapshot the row-0 highlight (the "current cell" the user is looking
	// at) and the ribbon cursor position used by the timeline bar.
	hl := -1
	g.highlightMu.RLock()
	for key := range g.highlightedBeats {
		row, idx := splitBeatKey(key)
		if row == 0 {
			hl = idx
			break
		}
	}
	g.highlightMu.RUnlock()
	if hl < 0 {
		t.Fatalf("no row-0 highlight after %d beats of playback", beats)
	}
	if hl < g.drum.Offset || hl >= g.drum.Offset+g.drum.Length {
		t.Fatalf("highlight idx=%d outside drum-view window [%d, %d)",
			hl, g.drum.Offset, g.drum.Offset+g.drum.Length)
	}

	// Convert the highlight cell to a screen-space center X using the same
	// math drawHighlights uses (`x0 = startX + j*totalW/n`).
	z := g.drum.timelineZone
	if z == nil {
		t.Fatalf("timeline zone not initialized")
	}
	barRect := z.timelineBarRect
	if barRect.Empty() {
		t.Fatalf("timeline bar rect empty; layout not run")
	}
	n := g.drum.Length
	j := hl - g.drum.Offset
	x0 := barRect.Min.X + (j*barRect.Dx())/n
	x1 := barRect.Min.X + ((j+1)*barRect.Dx())/n
	hlCenterX := (x0 + x1) / 2

	// Compute the ribbon cursor X using the same logic as drawTimelineBar.
	// Use g.displayBeat() — the exact value game_draw_helpers.go passes
	// into DrumView.Draw → TimelineZone.SetDrawParams. Earlier this test
	// read g.elapsedBeats (int subdivisions / div) which masked the source-
	// of-truth divergence that screenshot.png surfaced: TrackBeat ran off
	// elapsedBeats while the cursor ran off displayBeat(). After the
	// playheadAbsSubdiv() unification, both derive from displayBeat() and
	// the alignment holds within sub-cell quantisation.
	elapsedBeats := g.displayBeat()
	winStart, _, pxPerBeat := ribbonWindowBeats(barRect.Dx(), elapsedBeats)
	frac := RuntimeProf().RibbonPlayheadFrac
	if frac <= 0 || frac >= 1 {
		frac = 0.70
	}
	cursorX := barRect.Min.X + int(math.Round(frac*float64(barRect.Dx())))
	// Mirror drawTimelineBar: the playhead floats at its true position while
	// the window is pinned at zero, switching to the frac pin only once the
	// true position reaches it (elapsedBeats >= frac*barWidth/pxPerBeat).
	if winStart <= 0 && elapsedBeats < frac*float64(barRect.Dx())/pxPerBeat {
		cursorX = barRect.Min.X + int(math.Round((elapsedBeats-winStart)*pxPerBeat))
	}

	// The yellow highlight cell and the ribbon playhead share the same
	// horizontal extent (timelineBarRect == timelineRect). After the ribbon
	// pins to RibbonPlayheadFrac, the cell highlight must live in the same
	// region the cursor is hovering over — otherwise the user sees the cell
	// flash several cells to the LEFT of where the ribbon says playback is.
	// TrackBeat's ±1-cell dead-zone is the only intentional slack here, so
	// allow at most ~1.5 cell widths of separation between the two playheads.
	cellW := barRect.Dx() / n
	tolerance := cellW + cellW/2
	if absInt(hlCenterX-cursorX) > tolerance {
		t.Fatalf("highlight desync with ribbon: highlight center x=%d (col %d), ribbon cursor x=%d, bar=[%d,%d), tol=%d (cellW=%d)",
			hlCenterX, j, cursorX, barRect.Min.X, barRect.Max.X, tolerance, cellW)
	}
}
