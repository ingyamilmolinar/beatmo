//go:build test

package ui

import (
	"os"
	"testing"
	"time"
)

func TestImportRealTunkulJSON(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)

	// Load the actual tunkul.json (try multiple paths to handle different working directories)
	paths := []string{
		"../../tunkul.json",
		"../../../../tunkul.json",
	}
	var data []byte
	var err error
	for _, p := range paths {
		data, err = os.ReadFile(p)
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Skipf("tunkul.json not found: %v", err)
	}

	// First import
	if err := g.Import(data); err != nil {
		t.Fatalf("first import failed: %v", err)
	}
	t.Logf("First import: %d nodes, %d rows", len(g.nodes), len(g.drum.Rows))

	// Start playback
	g.SetPlayingForTest(true)
	g.SetAppliedBPMForTest(120)
	g.SetPlayStartForTest(time.Now())

	g.Update()

	// Second import should complete without hanging
	done := make(chan error, 1)
	go func() {
		done <- g.Import(data)
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("second import failed: %v", err)
		}
		t.Logf("Second import: %d nodes, %d rows", len(g.nodes), len(g.drum.Rows))
	case <-time.After(10 * time.Second):
		t.Fatal("Import hung - seqMu deadlock detected")
	}
}
