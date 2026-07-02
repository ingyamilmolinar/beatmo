//go:build test

package ui

import (
	"encoding/json"
	"testing"
)

// Test that clicking Import triggers onImport via the async path when data arrives.
func TestImportButtonFlowCallsOnImport(t *testing.T) {
	assertDefaultParityState(t)
	// Override the async picker to immediately return a tiny JSON
	old := selectJSONAsyncFn
	defer func() { selectJSONAsyncFn = old }()
	selectJSONAsyncFn = func(cb func([]byte, string, error)) {
		cb([]byte(`{"version":1,"subdiv":32,"bpm":120,"instruments":[],"nodes":[]}`), "", nil)
	}

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() {
		closeImportForTest(t, g)
	})
	g.Layout(640, 480)
	called := false
	g.drum.onImport = func(b []byte, _ string) error {
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		called = true
		return nil
	}
	// Click import
	g.drum.importBtn().OnClick()
	_ = g.Update()
	if !called {
		t.Fatalf("onImport never called after import click")
	}
}

// Test that canceling import (no callback) eventually releases the guard.
func TestImportCancelUnblocksAfterTimeout(t *testing.T) {
	assertDefaultParityState(t)
	// Override the picker to do nothing (simulate cancel with no change event)
	old := selectJSONAsyncFn
	defer func() { selectJSONAsyncFn = old }()
	selectJSONAsyncFn = func(cb func([]byte, string, error)) {}

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() {
		closeImportForTest(t, g)
	})
	g.Layout(640, 480)
	// Click import
	g.drum.importBtn().OnClick()
	if !g.drum.importing {
		t.Fatalf("expected importing=true after click")
	}
	// Advance frames to exceed timeout threshold
	for i := 0; i < 620; i++ {
		_ = g.Update()
	}
	if g.drum.importing {
		t.Fatalf("importing guard not released after timeout")
	}
	// Next click should be allowed again
	tries := 0
	selectJSONAsyncFn = func(cb func([]byte, string, error)) { tries++ }
	g.drum.importBtn().OnClick()
	if tries == 0 {
		t.Fatalf("import click did not invoke picker after timeout release")
	}
}

func TestImportButtonIgnoredWhileImporting(t *testing.T) {
	assertDefaultParityState(t)
	tries := 0
	old := selectJSONAsyncFn
	defer func() { selectJSONAsyncFn = old }()
	selectJSONAsyncFn = func(cb func([]byte, string, error)) { tries++ }

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() { closeImportForTest(t, g) })
	g.Layout(640, 480)

	g.drum.importBtn().OnClick()
	if tries != 1 {
		t.Fatalf("picker calls=%d want=1", tries)
	}
	if !g.drum.importing {
		t.Fatalf("expected importing=true after click")
	}

	g.drum.importBtn().OnClick()
	if tries != 1 {
		t.Fatalf("import click should be ignored while importing; calls=%d", tries)
	}
}

func TestImportButtonIgnoredWhileNaming(t *testing.T) {
	assertDefaultParityState(t)
	tries := 0
	old := selectJSONAsyncFn
	defer func() { selectJSONAsyncFn = old }()
	selectJSONAsyncFn = func(cb func([]byte, string, error)) { tries++ }

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	startUploadForTest(t, g.drum)
	waitForUploadNaming(t, g)

	g.drum.importBtn().OnClick()
	if tries != 0 {
		t.Fatalf("import picker should not open during naming")
	}
	if g.drum.importing {
		t.Fatalf("importing started unexpectedly during naming")
	}
}

func TestImportButtonIgnoredWhileUploading(t *testing.T) {
	assertDefaultParityState(t)
	tries := 0
	old := selectJSONAsyncFn
	defer func() { selectJSONAsyncFn = old }()
	selectJSONAsyncFn = func(cb func([]byte, string, error)) { tries++ }

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	startUploadForTest(t, g.drum)
	g.drum.importBtn().OnClick()
	if tries != 0 {
		t.Fatalf("import picker should not open during upload")
	}
	if g.drum.importing {
		t.Fatalf("importing started unexpectedly during upload")
	}
}
