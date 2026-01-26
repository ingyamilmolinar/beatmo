//go:build test

package ui

import (
	"encoding/json"
	"testing"
	"time"
)

// TestImportViaUIFlowNoDeadlock tests the real import flow:
// Game.Update() holds seqMu -> drum.Update() -> importCh -> onImport callback
// This catches recursive lock issues that direct g.Import() calls miss.
func TestImportViaUIFlowNoDeadlock(t *testing.T) {
	assertDefaultParityState(t)

	// Override the async picker to return test data
	old := selectJSONAsyncFn
	defer func() { selectJSONAsyncFn = old }()

	testData := buildSimpleImportJSON(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Simulate clicking the import button - this sets dv.importing=true
	// and calls selectJSONAsyncFn
	selectJSONAsyncFn = func(cb func([]byte, error)) {
		// Simulate async file picker returning data
		cb(testData, nil)
	}
	startImportForTest(t, g.drum)

	// Now call Game.Update() which will:
	// 1. Acquire seqMu
	// 2. Call drum.Update() which reads from importCh and calls onImport
	// 3. onImport should queue data (not call Import directly)
	// 4. After seqMu.Unlock(), process pendingImportData
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Multiple Update() calls to ensure the import is fully processed
		for i := 0; i < 5; i++ {
			g.Update()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		// Success - no deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("Game.Update() deadlocked during import - seqMu recursive lock issue")
	}

	// Verify import actually happened
	if len(g.nodes) != 2 {
		t.Errorf("expected 2 nodes after import, got %d", len(g.nodes))
	}
}

// TestImportViaUIFlowWhilePlaying tests import during playback.
// The sequencer goroutine is disabled in tests, but we simulate the
// playing state to exercise the safety code paths.
func TestImportViaUIFlowWhilePlaying(t *testing.T) {
	assertDefaultParityState(t)

	old := selectJSONAsyncFn
	defer func() { selectJSONAsyncFn = old }()

	testData := buildSimpleImportJSON(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// First import to set up initial state
	if err := g.Import(testData); err != nil {
		t.Fatalf("initial import failed: %v", err)
	}

	// Start "playback" (sequencer goroutine disabled in tests, but state is set)
	g.SetPlayingForTest(true)
	g.SetAppliedBPMForTest(120)
	g.SetPlayStartForTest(time.Now())

	// Verify we're "playing"
	if !g.Playing() {
		t.Fatal("expected playing=true")
	}

	// Now trigger import via UI flow while "playing"
	selectJSONAsyncFn = func(cb func([]byte, error)) {
		cb(testData, nil)
	}
	startImportForTest(t, g.drum)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 5; i++ {
			g.Update()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Game.Update() deadlocked during import while playing")
	}
}

// TestImportViaUIFlowWithSeqMuHeld directly tests the scenario where
// seqMu is held when the import callback fires.
func TestImportViaUIFlowWithSeqMuHeld(t *testing.T) {
	assertDefaultParityState(t)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	testData := buildSimpleImportJSON(t)

	// Simulate the exact deadlock scenario:
	// 1. Hold seqMu (as Game.Update does)
	// 2. Call onImport (as drum.Update does)
	// 3. onImport should NOT try to acquire seqMu (would deadlock)

	done := make(chan struct{})
	go func() {
		defer close(done)
		g.seqMu.Lock()
		defer g.seqMu.Unlock()

		// This simulates what happens inside drum.Update() when import data arrives
		if g.drum.onImport != nil {
			err := g.drum.onImport(testData)
			if err != nil {
				t.Errorf("onImport returned error: %v", err)
			}
		}
	}()

	select {
	case <-done:
		// onImport returned without deadlock - it queued data correctly
	case <-time.After(2 * time.Second):
		t.Fatal("onImport deadlocked - it's trying to acquire seqMu recursively")
	}

	// Now process the queued import outside the lock
	if g.pendingImportData != nil {
		if err := g.Import(g.pendingImportData); err != nil {
			t.Errorf("deferred import failed: %v", err)
		}
		g.pendingImportData = nil
	}

	if len(g.nodes) != 2 {
		t.Errorf("expected 2 nodes after deferred import, got %d", len(g.nodes))
	}
}

func buildSimpleImportJSON(t *testing.T) []byte {
	t.Helper()
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
		Instruments: []inst{{Name: "Kick", ID: "kick", Volume: 1, Origin: 1, Color: "#FF0000FF"}},
		Nodes: []node{
			{ID: 1, I: 0, J: 0, Type: "regular", Outputs: []int{2}},
			{ID: 2, I: 32, J: 0, Type: "regular", Outputs: []int{1}},
		},
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("failed to marshal test JSON: %v", err)
	}
	return data
}
