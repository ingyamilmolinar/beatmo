package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Fix D: the just-scheduled-beat grace (scheduled && !slate && idx ==
// seqNextIdxs[row]-1) is a one-frame pipeline-latency artifact — the sequencer
// scheduled idx and the DrumView slate will reflect it on the very next
// refresh. The grace must hold in EVERY mode, including the fatal/panic mode.
// Previously `fatalNow ||` short-circuited the grace exactly in the mode that
// panics, turning a benign one-frame lag into a crash.
func TestParityJustScheduledBeatGracedUnderFatal(t *testing.T) {
	assertDefaultParityState(t)
	prevFatal := parityFatalEnabled.Load()
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(prevFatal) })

	g := buildTestGame(t)
	g.parityWatch = parityWatchPanic // fatal mode
	t.Cleanup(g.CloseForTest)

	root := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.graph.StartNodeID = root.ID
	g.start = root
	g.drum.Rows[0].Origin = root.ID
	g.drum.Rows[0].Node = root
	g.addEdge(root, n1)
	g.addEdge(n1, root)
	g.updateBeatInfos()
	g.drum.SetLength(8)
	g.drum.Offset = 0
	g.refreshDrumRow()

	// Slate has NOT yet rendered the just-scheduled beat at idx=4 (still false),
	// while the scheduler decided it visible. This is the one-frame lag.
	g.drum.Rows[0].Steps = []bool{true, false, true, false, false, false, false, false}

	// idx=4 is the just-scheduled beat: seqNextIdxs[0]-1 == 4.
	g.nextBeatIdxs = []int{5}
	g.seqNextIdxs = []int{5}

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("just-scheduled-beat lag must be graced even under fatal parity, but it panicked: %v", r)
		}
	}()
	// scheduled=true, slate=false, idx==seqNextIdxs[0]-1 → graced (no panic).
	g.parityCheck(0, 4, g.beatInfoAtRow(0, 4), true, "test-just-scheduled", false)
}
