package ui

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

// Repro for: after changing subdivisions following an earlier playback,
// the timeline counter and global playhead should advance normally.
func TestTimelineAdvancesAfterSubdivChangeAndEdit(t *testing.T) {
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	g.Layout(800, 600)
	// Freeze input so UI noise doesn't interfere.
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	// Build a simple 1x1-beat rectangle on row 0 at default 32 subdivisions.
	const beat = 32
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(beat, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(beat, beat, model.NodeTypeRegular)
	n3 := g.tryAddNode(0, beat, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n0)
	g.pendingStartRow = -1
	g.updateBeatInfos()

	// Start, let it run briefly, then stop.
	g.drum.playPressed = true
	until := time.Now().Add(250 * time.Millisecond)
	for time.Now().Before(until) {
		_ = g.Update()
		time.Sleep(10 * time.Millisecond)
	}
	g.drum.stopPressed = true
	_ = g.Update()

	// Edit: add a new node connected to the existing circuit.
	// Connect from n1 to a new node further to the right.
	g.pendingStartRow = 0
	nx := g.tryAddNode(beat+beat/2, 0, model.NodeTypeRegular)
	g.addEdge(n1, nx)
	g.pendingStartRow = -1
	g.updateBeatInfos()

	// Change subdivisions to 8.
	if err := g.SetSubdivisions(8); err != nil {
		t.Fatalf("SetSubdivisions error: %v", err)
	}

	// Fresh playback should make the timeline advance from the new base.
	startBeat := g.displayBeat()
	g.drum.playPressed = true
	_ = g.Update()
	// Allow a short window for time to advance.
	until = time.Now().Add(300 * time.Millisecond)
	var moved bool
	var last float64
	for time.Now().Before(until) {
		_ = g.Update()
		b := g.displayBeat()
		if b > startBeat+0.01 { // at least ~1/100 beat progression
			moved = true
			last = b
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !moved {
		t.Fatalf("timeline did not advance after subdiv change: start=%.3f last=%.3f", startBeat, last)
	}
}
