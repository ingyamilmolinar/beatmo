//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Every mutation during Import fired the undo tap, which serialised the whole growing document: O(N²) work,
// a multi-second freeze on single-threaded WASM, for history Import drops anyway. Counting serialisations is
// machine-independent.
func TestImportLargeTemplateNoUndoCaptureStorm(t *testing.T) {
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()

	// Picked by SIZE, not name: a perf guard must not depend on a fixture whose size could itself be a bug.
	var tpl assets.Template
	for _, tp := range assets.Templates() {
		if len(tp.Bytes) > len(tpl.Bytes) {
			tpl = tp
		}
	}
	if len(tpl.Bytes) == 0 {
		t.Fatal("no built-in templates found")
	}

	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.SetForceMobileProfile(true)
	g.Layout(390, 720)

	captures := 0
	prev := undoCaptureHook
	undoCaptureHook = func() { captures++ }
	t.Cleanup(func() { undoCaptureHook = prev })

	if err := g.Import(tpl.Bytes); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Ideally zero, never proportional to the node count.
	nodeCount := len(g.graph.Nodes)
	if nodeCount < 100 {
		t.Fatalf("test fixture too small (%d nodes) to distinguish O(N) from O(1)", nodeCount)
	}
	const maxCaptures = 2
	if captures > maxCaptures {
		t.Fatalf("Import serialized the whole document %d times for %d nodes "+
			"(want <= %d) — per-mutation undo-capture storm makes template "+
			"loading O(N^2) and freezes WASM mobile",
			captures, nodeCount, maxCaptures)
	}
}
