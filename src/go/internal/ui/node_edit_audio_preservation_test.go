package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// These tests pin the invariant that editing the graph during playback —
// adding, moving, or removing a node — must NOT tear down the still-ringing
// audio voices of an instrument (nor of any other channel / the master mix).
// The only audible effect of removing a node should be that that node's own
// future trigger disappears; every voice already sounding must keep ringing.
//
// The bug they reproduce: (*Game).updateBeatInfos, when a row's path signature
// changes during playback, calls audio.Stop(instrument) for every changed row
// (see game_graph_update_beat_infos.go). Deleting a node changes the circuit's
// path signature, so the whole instrument's live voices are killed — not just
// the removed node's trigger. In-flight *scheduled* audio is already handled
// separately by g.audioGen.Add(1), so this blanket per-instrument voice-kill is
// redundant and wrong. audio.Stop fires the SetStopHook, so these tests observe
// the kill directly via that hook.

// buildPlayingLoop wires a 3-node regular loop (start -> mid -> tail -> start)
// onto row 0's instrument, fills every step, refreshes beat infos, and starts
// playback. It returns the three nodes so a test can edit the circuit mid-play.
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
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

	inst := g.drum.Rows[0].Instrument
	instStops := 0
	anyStops := 0
	audio.SetStopHook(func(id string) {
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

	// Remove a node from the circuit mid-playback. Dropping the node's own
	// future trigger is expected; muting/clearing the instrument's already
	// ringing voices is the bug.
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
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })

	inst := g.drum.Rows[0].Instrument
	instStops := 0
	anyStops := 0
	audio.SetStopHook(func(id string) {
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

	// Insert a new node onto the circuit's traversal path mid-playback by
	// rerouting start -> mid through it: start -> extra -> mid. This changes
	// the row's path signature (the added node is genuinely on the path, not a
	// dead-end branch). Adding a node must not clear the instrument's already
	// ringing voices.
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
