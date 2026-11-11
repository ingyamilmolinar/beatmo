//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// TestImportLargeTemplateNoUndoCaptureStorm reproduces the WASM-mobile
// crash/hang when loading a large built-in template.
//
// Root cause: every tryAddNode / addEdge / AddRow during Import fires the undo
// tap (endUndoGroup -> commitNow -> undoCapture -> DrumView.exportBytes), which
// serializes the ENTIRE growing document to indented JSON. For an N-node
// template that is O(N^2) full-document serializations — 7s on native for the
// the largest beethoven-quartet-op18 template, a multi-second single-frame freeze on
// single-threaded WASM mobile (the "hang"). The undo history is dropped at the
// end of Import (OnExternalLoad) anyway, so every one of those captures is
// wasted work.
//
// This is a machine-independent guard: it counts document serializations during
// Import and requires them to be bounded (not proportional to node count).
// Before the fix the count equals roughly the node+edge+row count (thousands);
// after the fix Import performs no per-mutation captures.
func TestImportLargeTemplateNoUndoCaptureStorm(t *testing.T) {
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()

	// The largest shipped template is the worst case a user can trigger from
	// the template menu on mobile. Pick it by SIZE rather than by name: this
	// named beethoven-quartet-op18, which was the biggest only because a
	// measure-range defect was splicing all four movements of the quartet into
	// one circuit. Fixing that shrank it to 83 nodes and left this guard
	// unable to tell O(N) from O(1) — a perf guard must not depend on a
	// fixture whose size was itself a bug.
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

	// A correct import performs at most a small constant number of document
	// serializations (ideally zero — history is cleared at the end regardless).
	// It must NOT scale with the node count.
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
