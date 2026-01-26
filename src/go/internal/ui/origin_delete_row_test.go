package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// Build a simple 2x2 loop, set a secondary row's origin, delete that origin
// node and then delete the row; ensure no panic occurs on subsequent node
// deletions and the row clears its origin state.
func TestDeleteOriginNodeThenDeleteRow_NoPanicAndClearsRow(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build a rectangle loop at (0,0)->(1,0)->(1,1)->(0,1)->(0,0)
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 1, model.NodeTypeRegular)
	n3 := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n0)
	g.pendingStartRow = -1
	g.updateBeatInfos()

	// Add second row and set its origin to n1
	g.drum.AddRow()
	g.drum.Rows[1].Origin = n1.ID
	g.drum.Rows[1].Node = n1
	g.updateBeatInfos()
	if g.drum.Rows[1].Origin != n1.ID {
		t.Fatalf("expected second row origin set")
	}

	// Delete the origin node (n1)
	g.deleteNode(n1)
	// Row referencing origin should be removed automatically.
	if len(g.drum.Rows) != 1 {
		t.Fatalf("expected origin row to be deleted automatically; rows=%d", len(g.drum.Rows))
	}

	// Delete another node in the loop and ensure no panic occurs.
	// Run a draw to exercise paths; should not panic.
	g.deleteNode(n2)
	// Exercise Update/Draw path without a window system; stubs suffice.
	_ = g.Update()
	g.Draw(ebiten.NewImage(800, 600))
}
