package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Editing the graph during playback never tears down a still-ringing voice: the only audible effect is the
// edited node's own future trigger. audio.Stop fires the stop hook, so a kill is observed directly.

// Every step filled, playback started.
func buildPlayingLoop(t *testing.T, g *Game) (start, mid, tail *uiNode) {
	t.Helper()
	start = g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	mid = g.tryAddNode(1, 0, model.NodeTypeRegular)
	tail = g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(start, mid)
	g.addEdge(mid, tail)
	g.addEdge(tail, start)

	g.drum.Rows[0].Origin = start.ID
	g.drum.Rows[0].Node = start
	for i := range g.drum.Rows[0].Steps {
		g.drum.Rows[0].Steps[i] = true
	}
	g.updateBeatInfos()
	if len(g.beatInfosByRow) == 0 || len(g.beatInfosByRow[0]) == 0 {
		t.Fatalf("beat infos not generated: %+v", g.beatInfosByRow)
	}

	g.SetPlaying(true)
	return start, mid, tail
}

func TestDeleteNodeDuringPlaybackDoesNotStopInstrumentAudio(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	_, mid, _ := buildPlayingLoop(t, g)

	plays := 0
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) { plays++ })

	inst := g.drum.Rows[0].Instrument
	instStops := 0
	anyStops := 0
	audio.SetStopHook(func(id instrument.ID) {
		anyStops++
		if id == inst {
			instStops++
		}
	})
	defer audio.SetStopHook(nil)

	// Produce real playback audio so the instrument has live voices.
	scheduleAbsForMuteTest(g, 0)
	scheduleAbsForMuteTest(g, 1)
	if plays == 0 {
		t.Fatalf("expected playback to produce audio before the edit: plays=%d", plays)
	}
	if instStops != 0 || anyStops != 0 {
		t.Fatalf("unexpected stop before the edit: instStops=%d anyStops=%d", instStops, anyStops)
	}

	// The node's own future trigger may go; the instrument's ringing voices must not.
	g.deleteNode(mid)

	if instStops != 0 {
		t.Fatalf("deleting a node during playback stopped the instrument's audio: instStops=%d (want 0)", instStops)
	}
	if anyStops != 0 {
		t.Fatalf("deleting a node during playback stopped a channel's audio: anyStops=%d (want 0)", anyStops)
	}

	// Playback must continue after the edit (voices alive, path still valid).
	playsBefore := plays
	scheduleAbsForMuteTest(g, 2)
	scheduleAbsForMuteTest(g, 3)
	if plays <= playsBefore {
		t.Fatalf("playback did not continue after node delete: plays=%d before=%d", plays, playsBefore)
	}
	if instStops != 0 || anyStops != 0 {
		t.Fatalf("stop fired after node delete: instStops=%d anyStops=%d", instStops, anyStops)
	}
}

func TestAddNodeDuringPlaybackDoesNotStopInstrumentAudio(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start, mid, _ := buildPlayingLoop(t, g)

	plays := 0
	g.SetPlayFunc(func(instrument.ID, float64, ...float64) { plays++ })

	inst := g.drum.Rows[0].Instrument
	instStops := 0
	anyStops := 0
	audio.SetStopHook(func(id instrument.ID) {
		anyStops++
		if id == inst {
			instStops++
		}
	})
	defer audio.SetStopHook(nil)

	scheduleAbsForMuteTest(g, 0)
	scheduleAbsForMuteTest(g, 1)
	if plays == 0 {
		t.Fatalf("expected playback to produce audio before the edit: plays=%d", plays)
	}
	if instStops != 0 || anyStops != 0 {
		t.Fatalf("unexpected stop before the edit: instStops=%d anyStops=%d", instStops, anyStops)
	}

	// On the traversal path (start → extra → mid), so the path signature changes.
	extra := g.tryAddNode(1, 1, model.NodeTypeRegular)
	g.deleteEdgeNoRefresh(start, mid)
	g.addEdgeNoRefresh(start, extra)
	g.addEdgeNoRefresh(extra, mid)
	g.updateBeatInfos()

	if instStops != 0 {
		t.Fatalf("adding a node during playback stopped the instrument's audio: instStops=%d (want 0)", instStops)
	}
	if anyStops != 0 {
		t.Fatalf("adding a node during playback stopped a channel's audio: anyStops=%d (want 0)", anyStops)
	}
}
