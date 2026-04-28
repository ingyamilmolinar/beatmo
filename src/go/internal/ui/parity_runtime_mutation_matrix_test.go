package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// runtimeMutationCase exercises a single runtime mutation while playback
// is active and asserts no parity panic + clean ring afterwards. Every
// case in the matrix below MUST route through the mutation gate
// (game_mutation_gate.go) — this test pins the contract.
type runtimeMutationCase struct {
	name   string
	mutate func(t *testing.T, g *Game)
}

func runRuntimeMutationCase(t *testing.T, c runtimeMutationCase) {
	t.Helper()
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 720)
	g.parityWatch = parityWatchPanic

	// Two-row playable circuit.
	a0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	a1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	a2 := g.tryAddNode(1, 1, model.NodeTypeRegular)
	a3 := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a0, a1)
	g.addEdge(a1, a2)
	g.addEdge(a2, a3)
	g.addEdge(a3, a0)
	g.start = a0
	g.graph.StartNodeID = a0.ID

	b0 := g.tryAddNode(5, 0, model.NodeTypeRegular)
	b1 := g.tryAddNode(6, 0, model.NodeTypeRegular)
	b2 := g.tryAddNode(6, 1, model.NodeTypeRegular)
	g.addEdge(b0, b1)
	g.addEdge(b1, b2)
	g.addEdge(b2, b0)

	g.drum.AddRow()
	g.drum.SetInstrument("snare")
	g.drum.Rows[1].Origin = b0.ID
	g.drum.Rows[1].Node = g.nodeByID(b0.ID)

	g.drum.SetLength(32)
	g.updateBeatInfos()
	g.drum.Offset = 0
	g.refreshDrumRow()
	g.ClearParityMismatches()

	g.SetPlaying(true)
	for abs := 0; abs < 16; abs++ {
		scheduleAbsForMuteTest(g, abs)
	}
	g.refreshDrumRow()
	g.parityScan("pre-mutation")
	if got := len(g.ParityMismatchSnapshot()); got != 0 {
		t.Fatalf("[%s] unexpected pre-mutation mismatches: %d %+v", c.name, got, g.ParityMismatchSnapshot())
	}

	// The mutation under test.
	c.mutate(t, g)

	for abs := 16; abs < 64; abs++ {
		scheduleAbsForMuteTest(g, abs)
		if abs%4 == 0 {
			g.refreshDrumRow()
			g.parityScan("post-mutation")
		}
	}
	g.refreshDrumRow()
	g.parityScan("final")

	if got := g.ParityMismatchSnapshot(); len(got) != 0 {
		t.Fatalf("[%s] mutation produced %d parity mismatches:\n%+v", c.name, len(got), got)
	}
}

func TestParityRuntimeMutationMatrix(t *testing.T) {
	cases := []runtimeMutationCase{
		{
			name: "instrument-change-row1",
			mutate: func(t *testing.T, g *Game) {
				g.drum.selRow = 1
				g.drum.SetInstrument("hihat")
			},
		},
		{
			name: "instrument-change-row0",
			mutate: func(t *testing.T, g *Game) {
				g.drum.selRow = 0
				g.drum.SetInstrument("clap")
			},
		},
		{
			name: "bpm-change-up",
			mutate: func(t *testing.T, g *Game) {
				g.drum.SetBPM(180)
			},
		},
		{
			name: "bpm-change-down",
			mutate: func(t *testing.T, g *Game) {
				g.drum.SetBPM(60)
			},
		},
		{
			name: "row-add",
			mutate: func(t *testing.T, g *Game) {
				g.drum.AddRow()
			},
		},
		{
			name: "row-mute-toggle",
			mutate: func(t *testing.T, g *Game) {
				if len(g.drum.Rows) > 1 {
					g.drum.toggleMute(1)
				}
			},
		},
		{
			name: "row-solo-toggle",
			mutate: func(t *testing.T, g *Game) {
				if len(g.drum.Rows) > 0 {
					g.drum.toggleSolo(0)
				}
			},
		},
		{
			name: "rapid-instrument-cycling",
			mutate: func(t *testing.T, g *Game) {
				g.drum.selRow = 1
				g.drum.SetInstrument("hihat")
				g.drum.SetInstrument("clap")
				g.drum.SetInstrument("tom")
				g.drum.SetInstrument("cowbell")
			},
		},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			runRuntimeMutationCase(t, c)
		})
	}
}
