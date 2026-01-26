package ui

import (
	"image"
	"image/color"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	"testing"
)

// When a circuit that feeds a particular drum row changes during playback,
// only that row's sprite cache should be invalidated and rebuilt. Other rows
// keep their caches intact.
func TestRowCacheLiveUpdate_PerRowInvalidation(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)

	// Build two disjoint loops and assign them to row 0 and row 1.
	// Row 0: rectangle A->B->C->D->A
	A := g.tryAddNode(0, 0, model.NodeTypeRegular)
	B := g.tryAddNode(2, 0, model.NodeTypeRegular)
	C := g.tryAddNode(2, 2, model.NodeTypeRegular)
	D := g.tryAddNode(0, 2, model.NodeTypeRegular)
	g.addEdge(A, B)
	g.addEdge(B, C)
	g.addEdge(C, D)
	g.addEdge(D, A)
	g.start = A
	g.graph.StartNodeID = A.ID

	// Row 1: triangle G->H->I->G
	G := g.tryAddNode(6, 0, model.NodeTypeRegular)
	H := g.tryAddNode(7, 0, model.NodeTypeRegular)
	I := g.tryAddNode(7, 1, model.NodeTypeRegular)
	g.addEdge(G, H)
	g.addEdge(H, I)
	g.addEdge(I, G)

	// Add second row and point its origin to G.
	g.drum.AddRow()
	g.drum.Rows[1].Origin = G.ID
	g.drum.Rows[1].Node = g.nodeByID(G.ID)

	// Window long enough to show differences.
	g.drum.SetLength(16)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Build initial caches.
	dst := ebiten.NewImage(800, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	if len(g.drum.rowCache) < 2 || g.drum.rowCache[0] == nil || g.drum.rowCache[1] == nil {
		t.Fatalf("expected two built row caches")
	}
	r0 := g.drum.rowCache[0]
	r1 := g.drum.rowCache[1]
	gen0 := g.drum.rowCacheGen[0]
	gen1 := g.drum.rowCacheGen[1]

	// Live edit: silence node B (row 0 circuit) so its steps change.
	if n, ok := g.graph.GetNodeByID(B.ID); ok {
		n.Type = model.NodeTypeSilent
		g.graph.Nodes[B.ID] = n
		g.notifyPredictorNode(B.ID)
	}
	// Recompute paths/predictions like the UI would after an edit.
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Draw again — only row 0 should be rebuilt.
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)

	if g.drum.rowCache[0] == r0 || g.drum.rowCacheGen[0] == gen0 {
		t.Fatalf("row 0 cache not invalidated after circuit change")
	}
	if g.drum.rowCache[1] != r1 || g.drum.rowCacheGen[1] != gen1 {
		t.Fatalf("row 1 cache changed unnecessarily on unrelated edit")
	}
}

// Changing only a cell's type (e.g., regular -> mute) without changing the
// boolean Steps must still invalidate the row sprite cache so rendering stays
// correct (mute cells are tinted differently).
func TestRowCacheLiveUpdate_CellTypeChangeInvalidatesRowCache(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(800, 600)

	// Build two disjoint loops and assign them to row 0 and row 1.
	// Row 0: rectangle A->B->C->D->A
	A := g.tryAddNode(0, 0, model.NodeTypeRegular)
	B := g.tryAddNode(2, 0, model.NodeTypeRegular)
	C := g.tryAddNode(2, 2, model.NodeTypeRegular)
	D := g.tryAddNode(0, 2, model.NodeTypeRegular)
	g.addEdge(A, B)
	g.addEdge(B, C)
	g.addEdge(C, D)
	g.addEdge(D, A)
	g.start = A
	g.graph.StartNodeID = A.ID

	// Row 1: triangle G->H->I->G
	G := g.tryAddNode(6, 0, model.NodeTypeRegular)
	H := g.tryAddNode(7, 0, model.NodeTypeRegular)
	I := g.tryAddNode(7, 1, model.NodeTypeRegular)
	g.addEdge(G, H)
	g.addEdge(H, I)
	g.addEdge(I, G)

	// Add second row and point its origin to G.
	g.drum.AddRow()
	g.drum.Rows[1].Origin = G.ID
	g.drum.Rows[1].Node = g.nodeByID(G.ID)

	// Window long enough to show differences.
	g.drum.SetLength(16)
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Build initial caches.
	dst := ebiten.NewImage(800, 240)
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	if len(g.drum.rowCache) < 2 || g.drum.rowCache[0] == nil || g.drum.rowCache[1] == nil {
		t.Fatalf("expected two built row caches")
	}
	r0 := g.drum.rowCache[0]
	r1 := g.drum.rowCache[1]
	gen0 := g.drum.rowCacheGen[0]
	gen1 := g.drum.rowCacheGen[1]
	beforeSteps := append([]bool(nil), g.drum.Rows[0].Steps...)
	beforeTypes := append([]model.NodeType(nil), g.drum.Rows[0].CellTypes...)

	// Live edit: convert node B to mute. With no hold/logic, this should not
	// change boolean Steps (still "on") but must update CellTypes.
	nodeB, ok := g.graph.GetNodeByID(B.ID)
	if !ok {
		t.Fatalf("node B missing")
	}
	nodeB.Type = model.NodeTypeMute
	g.graph.Nodes[B.ID] = nodeB
	g.notifyPredictorNode(B.ID)

	g.updateBeatInfos()
	g.refreshDrumRow()

	afterSteps := append([]bool(nil), g.drum.Rows[0].Steps...)
	afterTypes := append([]model.NodeType(nil), g.drum.Rows[0].CellTypes...)
	if !slices.Equal(beforeSteps, afterSteps) {
		t.Fatalf("expected steps unchanged after type-only edit; before=%v after=%v", beforeSteps, afterSteps)
	}
	if slices.Equal(beforeTypes, afterTypes) {
		t.Fatalf("expected cell types to change after type-only edit; before=%v after=%v", beforeTypes, afterTypes)
	}

	// Draw again — only row 0 should be rebuilt.
	g.drum.Draw(dst, map[int]int64{}, 0, nil, 0)
	if g.drum.rowCache[0] == r0 || g.drum.rowCacheGen[0] == gen0 {
		t.Fatalf("row 0 cache not invalidated after cell type change")
	}
	if g.drum.rowCache[1] != r1 || g.drum.rowCacheGen[1] != gen1 {
		t.Fatalf("row 1 cache changed unnecessarily on unrelated edit")
	}
}

// When the window offset shifts during playback in the same frame a row's
// contents change, the cache must rebuild fully. Incremental reuse would drag
// the old sprite forward, leaving stale steps on screen.
func TestRowCacheLiveUpdate_ShiftDuringContentChange(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 480, 240), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()

	// Ensure 1px-per-step so a +1 offset is within the incremental pad.
	dv.SetLength(dv.timelineRect.Dx())
	dv.Rows[0].Steps[10] = true
	dv.SetRowColor(0, color.RGBA{255, 0, 0, 255})
	dst := ebiten.NewImage(480, 240)
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if len(dv.rowCacheGen) == 0 {
		t.Fatalf("expected row cache generation tracking")
	}
	gen0 := dv.rowCacheGen[0]

	// Playback advances by one subdivision while the row's visible steps change
	// (e.g., probability edit). The old cache must not be shifted forward.
	dv.Offset = 1
	dv.markRowsShiftDirty()
	dv.Rows[0].Steps = make([]bool, dv.Length)
	dv.markRowDirty(0)
	dv.Draw(dst, map[int]int64{}, 0, nil, 0)
	if dv.rowCacheGen[0] == gen0 {
		t.Fatalf("row cache reused sprite after content change during shift; gen %d", gen0)
	}
}
