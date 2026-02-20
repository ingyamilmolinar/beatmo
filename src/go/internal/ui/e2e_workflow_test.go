package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// TestE2EFullLifecycle exercises the complete user workflow:
// build circuit → play → live edit → change BPM → stop → export → import → verify → resume.
func TestE2EFullLifecycle(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Step 1: Build a 4-node loop circuit.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	c := g.tryAddNode(8, 8, model.NodeTypeRegular)
	d := g.tryAddNode(0, 8, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	g.updateBeatInfos()
	g.refreshDrumRow()

	// Verify predictor has data.
	g.engine.Predictor.Ensure(32)
	visCount := 0
	for i := 0; i < 32; i++ {
		if g.engine.Predictor.VisibleAt(0, i) {
			visCount++
		}
	}
	if visCount == 0 {
		t.Fatal("no visible beats after build")
	}

	// Step 2: Start playback.
	var plays int
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })
	g.SetPlaying(true)
	advanceFrames(g, 30)
	if !g.Playing() {
		t.Fatal("expected playing after start")
	}

	// Step 3: Live edit — add a new node and edge while playing.
	e := g.tryAddNode(0, -8, model.NodeTypeRegular)
	g.addEdge(a, e)
	g.updateBeatInfos()

	// Verify still playing after edit.
	advanceFrames(g, 10)
	if !g.Playing() {
		t.Fatal("expected playing after live edit")
	}

	// Step 4: Change BPM.
	oldBPM := g.drum.BPM()
	g.drum.SetBPM(oldBPM + 10)
	advanceFrames(g, 5)
	newBPM := g.drum.BPM()
	if newBPM != oldBPM+10 {
		t.Fatalf("BPM change failed: %d -> %d (expected %d)", oldBPM, newBPM, oldBPM+10)
	}

	// Step 5: Stop playback.
	stopPlaybackForTest(g)
	if g.Playing() {
		t.Fatal("expected stopped after stop")
	}

	// Step 6: Export JSON.
	exported, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(exported) < 10 {
		t.Fatalf("export too short: %d bytes", len(exported))
	}
	var parsedExport exportFile
	if err := json.Unmarshal(exported, &parsedExport); err != nil {
		t.Fatalf("parse export: %v", err)
	}
	exportedNodeCount := len(parsedExport.Nodes)
	if exportedNodeCount < 4 {
		t.Fatalf("expected at least 4 nodes in export, got %d", exportedNodeCount)
	}

	// Step 7: Import the exported JSON into a fresh game (round-trip).
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(exported); err != nil {
		t.Fatalf("import round-trip: %v", err)
	}

	// Step 8: Verify circuit restored.
	reExported, err := g2.drum.exportBytes()
	if err != nil {
		t.Fatalf("re-export: %v", err)
	}
	var parsedReExport exportFile
	if err := json.Unmarshal(reExported, &parsedReExport); err != nil {
		t.Fatalf("parse re-export: %v", err)
	}
	if len(parsedReExport.Nodes) != exportedNodeCount {
		t.Fatalf("node count mismatch after round-trip: %d -> %d",
			exportedNodeCount, len(parsedReExport.Nodes))
	}
	if parsedReExport.BPM != parsedExport.BPM {
		t.Fatalf("BPM mismatch after round-trip: %d -> %d",
			parsedExport.BPM, parsedReExport.BPM)
	}

	// Step 9: Verify predictor parity between original and imported game.
	g2.engine.Predictor.Ensure(32)
	g.engine.Predictor.Ensure(32)
	for i := 0; i < 32; i++ {
		v1 := g.engine.Predictor.VisibleAt(0, i)
		v2 := g2.engine.Predictor.VisibleAt(0, i)
		if v1 != v2 {
			t.Fatalf("predictor mismatch at idx %d: original=%v imported=%v", i, v1, v2)
		}
	}

	// Step 10: Resume playback on imported game.
	g2.SetPlaying(true)
	advanceFrames(g2, 20)
	if !g2.Playing() {
		t.Fatal("expected playing after resume on imported game")
	}
	stopPlaybackForTest(g2)
}

// TestE2ERapidBPMStress verifies the engine survives rapid BPM changes
// during playback without crashes or predictor corruption.
func TestE2ERapidBPMStress(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build a simple 4-node loop.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	c := g.tryAddNode(8, 8, model.NodeTypeRegular)
	d := g.tryAddNode(0, 8, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()
	g.refreshDrumRow()

	// Start playback.
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.SetPlaying(true)
	advanceFrames(g, 10)

	// Rapid BPM changes: increment 20 times.
	startBPM := g.drum.BPM()
	for i := 0; i < 20; i++ {
		g.drum.SetBPM(g.drum.BPM() + 1)
		advanceFrames(g, 2)
	}
	endBPM := g.drum.BPM()

	if endBPM != startBPM+20 {
		t.Fatalf("BPM stress: expected %d, got %d", startBPM+20, endBPM)
	}

	// Verify still playing.
	if !g.Playing() {
		t.Fatal("playback should survive rapid BPM changes")
	}

	// Verify predictor is still healthy.
	g.engine.Predictor.Ensure(64)
	visCount := 0
	for i := 0; i < 64; i++ {
		if g.engine.Predictor.VisibleAt(0, i) {
			visCount++
		}
	}
	if visCount == 0 {
		t.Fatal("predictor returned no visible beats after BPM stress")
	}

	stopPlaybackForTest(g)
}

// TestE2ERowAddDuringPlayback verifies that adding a row during
// playback does not crash and playback continues.
func TestE2ERowAddDuringPlayback(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build two 4-node loops for kick and snare.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	c := g.tryAddNode(8, 8, model.NodeTypeRegular)
	d := g.tryAddNode(0, 8, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a)

	e := g.tryAddNode(-16, 0, model.NodeTypeRegular)
	f := g.tryAddNode(-8, 0, model.NodeTypeRegular)
	h := g.tryAddNode(-8, 8, model.NodeTypeRegular)
	i := g.tryAddNode(-16, 8, model.NodeTypeRegular)
	g.addEdge(e, f)
	g.addEdge(f, h)
	g.addEdge(h, i)
	g.addEdge(i, e)

	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a

	// Add second row.
	g.drum.AddRow()
	g.drum.Rows[1].Origin = e.ID
	g.drum.Rows[1].Node = e

	g.updateBeatInfos()
	g.refreshDrumRow()

	rowsBefore := len(g.drum.Rows)
	if rowsBefore != 2 {
		t.Fatalf("expected 2 rows, got %d", rowsBefore)
	}

	// Start playback.
	g.SetPlayFunc(func(string, float64, ...float64) {})
	g.SetPlaying(true)
	advanceFrames(g, 20)

	if !g.Playing() {
		t.Fatal("expected playing")
	}

	// Add a new row during playback.
	g.drum.AddRow()
	advanceFrames(g, 5)

	rowsAfter := len(g.drum.Rows)
	if rowsAfter != rowsBefore+1 {
		t.Fatalf("row not added: %d -> %d", rowsBefore, rowsAfter)
	}

	// Verify still playing.
	advanceFrames(g, 20)
	if !g.Playing() {
		t.Fatal("playback should survive row add")
	}

	// Verify predictor still works for original rows.
	g.engine.Predictor.Ensure(32)
	for row := 0; row < 2; row++ {
		vis := 0
		for idx := 0; idx < 32; idx++ {
			if g.engine.Predictor.VisibleAt(row, idx) {
				vis++
			}
		}
		if vis == 0 {
			t.Fatalf("row %d has no visible beats after row add", row)
		}
	}

	stopPlaybackForTest(g)
}
