package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// These tests are about a melodic row's role (longest sustain, one voice per pitch), not this id.
const testMelodicInst = roster.PianoFelt

// Independent of the shipped startup demo. Row 1, a kick loop, gives solo a row to silence.
func newMelodicCircuitGame(t *testing.T, pitches ...float64) *Game {
	t.Helper()
	if !audio.IsMelodicInstrument(testMelodicInst) {
		t.Fatalf("%s is not melodic; pick another shipped melodic instrument", testMelodicInst)
	}
	return newPitchedRowCircuitGame(t, string(testMelodicInst), pitches...)
}

// newPitchedRowCircuitGame is newMelodicCircuitGame with inst on row 0.
func newPitchedRowCircuitGame(t *testing.T, inst string, pitches ...float64) *Game {
	t.Helper()
	if len(pitches) == 0 {
		t.Fatal("newPitchedRowCircuitGame: no pitches")
	}
	g := buildTestGame(t)
	for len(g.drum.Rows) < 2 {
		g.drum.AddRow()
	}

	var mel []*uiNode
	for i, p := range pitches {
		if i == 0 {
			g.pendingStartRow = 0
		}
		n := g.tryAddNode(2*i, 0, model.NodeTypeRegular)
		if n == nil {
			t.Fatalf("melodic node %d not created", i)
		}
		g.graph.SetNodeParams(n.ID, model.NodeParams{Volume: 1, Pitch: p, Duration: 1})
		mel = append(mel, n)
	}
	for i := range mel {
		g.addEdge(mel[i], mel[(i+1)%len(mel)])
	}

	g.pendingStartRow = 1
	k0 := g.tryAddNode(0, 4, model.NodeTypeRegular)
	k1 := g.tryAddNode(4, 4, model.NodeTypeRegular)
	g.addEdge(k0, k1)
	g.addEdge(k1, k0)

	for row, id := range []string{inst, "kick"} {
		g.drum.selRow = row
		g.drum.SetInstrument(instrument.ID(id))
	}
	g.drum.selRow = 0
	t.Cleanup(func() { audio.ResetInstrumentParams(instrument.ID(inst)) })
	g.updateBeatInfos()
	return g
}

// melodicRow returns the index of the testMelodicInst row.
func melodicRow(t *testing.T, g *Game) int {
	t.Helper()
	for i, r := range g.drum.Rows {
		if r.Instrument == testMelodicInst {
			return i
		}
	}
	t.Fatalf("circuit has no %s row; rows=%d", testMelodicInst, len(g.drum.Rows))
	return -1
}
