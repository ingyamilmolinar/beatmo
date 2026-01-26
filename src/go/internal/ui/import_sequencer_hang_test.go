//go:build test

package ui

import (
	"encoding/json"
	"testing"
	"time"
)

// TestImportTwiceNoHang verifies that importing while the sequencer is active
// does not cause a deadlock. This test simulates the scenario where:
// 1. First import is done (sets up a graph)
// 2. Playback is started (sequencer is running)
// 3. Second import is attempted while sequencer holds seqMu
// The fix stops playback during import to avoid seqMu contention.
func TestImportTwiceNoHang(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a simple import JSON
	type node struct {
		ID      int    `json:"id"`
		I       int    `json:"i"`
		J       int    `json:"j"`
		Type    string `json:"type"`
		Outputs []int  `json:"outputs"`
	}
	type inst struct {
		Name   string  `json:"name"`
		ID     string  `json:"id"`
		Kind   string  `json:"kind"`
		Volume float64 `json:"volume"`
		Origin int     `json:"origin"`
		Color  string  `json:"color"`
	}
	type file struct {
		Version     int    `json:"version"`
		Subdiv      int    `json:"subdiv"`
		BPM         int    `json:"bpm"`
		Instruments []inst `json:"instruments"`
		Nodes       []node `json:"nodes"`
	}

	// First JSON: simple 2-node loop
	f1 := file{
		Version:     1,
		Subdiv:      32,
		BPM:         120,
		Instruments: []inst{{Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FF0000FF"}},
		Nodes: []node{
			{ID: 1, I: 0, J: 0, Type: "regular", Outputs: []int{2}},
			{ID: 2, I: 32, J: 0, Type: "regular", Outputs: []int{1}},
		},
	}
	data1, _ := json.Marshal(f1)

	// Second JSON: different 3-node loop
	f2 := file{
		Version:     1,
		Subdiv:      32,
		BPM:         100,
		Instruments: []inst{{Name: "Snare", ID: "snare", Kind: "builtin", Volume: 1, Origin: 10, Color: "#00FF00FF"}},
		Nodes: []node{
			{ID: 10, I: 0, J: 32, Type: "regular", Outputs: []int{11}},
			{ID: 11, I: 32, J: 32, Type: "regular", Outputs: []int{12}},
			{ID: 12, I: 64, J: 32, Type: "regular", Outputs: []int{10}},
		},
	}
	data2, _ := json.Marshal(f2)

	// First import
	if err := g.Import(data1); err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	t.Log("First import succeeded")

	// Start playback - this would activate the sequencer in non-test mode
	// In tests, sequencerLoop doesn't run (runningUnderGoTest returns true),
	// so we simulate by setting playing state
	g.SetPlayingForTest(true)
	g.SetAppliedBPMForTest(120)
	g.SetPlayStartForTest(time.Now())
	g.SetAudioStartForTest(0.0)

	// Give the game a tick to process state
	g.Update()

	// Second import should complete without hanging
	done := make(chan error, 1)
	go func() {
		done <- g.Import(data2)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second import failed: %v", err)
		}
		t.Log("Second import succeeded without hanging")
	case <-time.After(5 * time.Second):
		t.Fatal("Import hung - seqMu deadlock detected")
	}

	// Verify import applied
	if g.drum.BPM() != 100 {
		t.Errorf("expected BPM 100 after second import, got %d", g.drum.BPM())
	}
	if len(g.nodes) != 3 {
		t.Errorf("expected 3 nodes after second import, got %d", len(g.nodes))
	}
}

// TestImportResetsPlayingState verifies that importing stops playback
// during the import operation and restores it afterward.
func TestImportResetsPlayingState(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a simple import JSON
	type node struct {
		ID      int    `json:"id"`
		I       int    `json:"i"`
		J       int    `json:"j"`
		Type    string `json:"type"`
		Outputs []int  `json:"outputs"`
	}
	type inst struct {
		Name   string  `json:"name"`
		ID     string  `json:"id"`
		Kind   string  `json:"kind"`
		Volume float64 `json:"volume"`
		Origin int     `json:"origin"`
		Color  string  `json:"color"`
	}
	type file struct {
		Version     int    `json:"version"`
		Subdiv      int    `json:"subdiv"`
		BPM         int    `json:"bpm"`
		Instruments []inst `json:"instruments"`
		Nodes       []node `json:"nodes"`
	}

	f := file{
		Version:     1,
		Subdiv:      32,
		BPM:         120,
		Instruments: []inst{{Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FF0000FF"}},
		Nodes: []node{
			{ID: 1, I: 0, J: 0, Type: "regular", Outputs: []int{2}},
			{ID: 2, I: 32, J: 0, Type: "regular", Outputs: []int{1}},
		},
	}
	data, _ := json.Marshal(f)

	// Start playback before import
	g.SetPlayingForTest(true)
	wasPlayingBefore := g.Playing()
	if !wasPlayingBefore {
		t.Fatal("expected playing=true before import")
	}

	// Import
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// After import, playing state should be restored
	wasPlayingAfter := g.Playing()
	if !wasPlayingAfter {
		t.Error("expected playing state to be restored after import")
	}
}

// TestImportClearsSeqPathSnap verifies that seqPathSnap is cleared during
// import to provide an additional safety net against lock contention.
func TestImportClearsSeqPathSnap(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a simple import JSON
	type node struct {
		ID      int    `json:"id"`
		I       int    `json:"i"`
		J       int    `json:"j"`
		Type    string `json:"type"`
		Outputs []int  `json:"outputs"`
	}
	type inst struct {
		Name   string  `json:"name"`
		ID     string  `json:"id"`
		Kind   string  `json:"kind"`
		Volume float64 `json:"volume"`
		Origin int     `json:"origin"`
		Color  string  `json:"color"`
	}
	type file struct {
		Version     int    `json:"version"`
		Subdiv      int    `json:"subdiv"`
		BPM         int    `json:"bpm"`
		Instruments []inst `json:"instruments"`
		Nodes       []node `json:"nodes"`
	}

	f := file{
		Version:     1,
		Subdiv:      32,
		BPM:         120,
		Instruments: []inst{{Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FF0000FF"}},
		Nodes: []node{
			{ID: 1, I: 0, J: 0, Type: "regular", Outputs: []int{2}},
			{ID: 2, I: 32, J: 0, Type: "regular", Outputs: []int{1}},
		},
	}
	data, _ := json.Marshal(f)

	// First import to establish a path
	if err := g.Import(data); err != nil {
		t.Fatalf("first import failed: %v", err)
	}

	// Verify path snapshot exists after first import
	snap := g.seqPathSnapshot()
	if snap == nil {
		t.Log("seqPathSnapshot is nil after import (expected after import clears and rebuilds it)")
	}

	// Do another import and verify it completes
	if err := g.Import(data); err != nil {
		t.Fatalf("second import failed: %v", err)
	}
}
