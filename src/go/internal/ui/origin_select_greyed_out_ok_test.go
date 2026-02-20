package ui

import (
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Ensure that when playback is active but a different circuit is greyed out (muted or
// excluded by solo), we allow selecting a node from that circuit as a new origin.
func TestOriginSelectAllowsMutedOtherCircuit(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Circuit A (row 0): a0
	a0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	// Circuit B (row 1): b0
	g.drum.AddRow()
	_ = g.tryAddNode(0, 2, model.NodeTypeRegular)
	g.updateBeatInfos()

	// Arm origin selection for row 1
	g.drum.recalcButtons()
	g.drum.calcLayout()
	ob := g.drum.rowOriginBtns[1]
	or := ob.Rect()
	_ = ob.Handle((or.Min.X+or.Max.X)/2, (or.Min.Y+or.Max.Y)/2, true)
	_ = ob.Handle((or.Min.X+or.Max.X)/2, (or.Min.Y+or.Max.Y)/2, false)
	_ = g.Update()
	if g.pendingStartRow != 1 {
		t.Fatalf("pendingStartRow=%d want 1", g.pendingStartRow)
	}

	// Start playback and mute row 0 to grey it out
	g.SetPlaying(true)
	setRowMuted(t, g.drum, 0, true)

	// Click node a0 (belongs to muted row 0). This should be allowed.
	x1, y1, x2, y2 := g.nodeScreenRect(a0)
	sx := int(math.Round((x1 + x2) / 2))
	sy := int(math.Round((y1 + y2) / 2))
	// press
	restore := SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	t.Cleanup(restore)
	_ = g.Update()
	restore()
	// release
	restore = SetInputForTest(
		func() (int, int) { return sx, sy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	t.Cleanup(restore)
	_ = g.Update()
	restore()

	if g.pendingStartRow != -1 {
		t.Fatalf("pendingStartRow not cleared after selecting muted other")
	}
	if g.drum.Rows[1].Origin != a0.ID {
		t.Fatalf("row1 origin not switched to a0; got %d", g.drum.Rows[1].Origin)
	}
	if g.sidebar.IsOpen() {
		t.Fatalf("node popup opened but origin selection should take precedence")
	}
}
