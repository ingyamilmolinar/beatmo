//go:build test

package ui

import (
	"os"
	"runtime/debug"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestImportEmbeddedPCM reproduces the reported failure: a freshly-exported
// project file with embedded gzipped PCM (the Sampler-tab "sound travels inside
// the file" feature) fails to import into a brand-new game instance. The fixture
// is the real exported beatmo.json (6 instruments, 4 with embedded PCM, 53 nodes,
// subdiv=8). This test pins the exact failure mode (panic vs. error vs. wrong
// state vs. slow) before any fix is applied.
func TestImportEmbeddedPCM(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	data, err := os.ReadFile("testdata/import_embedded_pcm.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	t.Logf("fixture size: %d bytes", len(data))

	// Catch a panic in Import and surface it with the stack — the browser symptom
	// ("nothing happens", no error toast) is consistent with a panic freezing the
	// WASM Update() goroutine.
	var importErr error
	start := time.Now()
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("g.Import PANICKED: %v\n%s", r, debug.Stack())
			}
		}()
		importErr = g.Import(data)
	}()
	elapsed := time.Since(start)
	t.Logf("g.Import elapsed: %v", elapsed)

	if importErr != nil {
		t.Fatalf("g.Import returned error: %v", importErr)
	}

	// Post-import state.
	if got := len(g.drum.Rows); got != 6 {
		t.Errorf("rows: got %d, want 6", got)
	}
	if got := len(g.nodes); got != 53 {
		t.Errorf("nodes: got %d, want 53", got)
	}
	if g.graph.StartNodeID == model.InvalidNodeID {
		t.Errorf("StartNodeID is InvalidNodeID after import")
	}

	// Each embedded-PCM instrument must be registered as a user sample.
	for _, id := range []string{"kick-1", "snare", "hihat", "fm-epiano-1"} {
		if !audio.IsUserSample(id) {
			t.Errorf("instrument %q: embedded PCM not registered (IsUserSample=false)", id)
			continue
		}
		rec, ok := audio.UserSamplePCM(id)
		if !ok || len(rec.PCM) == 0 {
			t.Errorf("instrument %q: UserSamplePCM empty (ok=%v len=%d)", id, ok, len(rec.PCM))
		}
	}

	// Every row's origin must resolve to a real node (import.go:423/431 leaves it
	// unset on a miss and only logs a warning).
	for i, r := range g.drum.Rows {
		if r.Origin == model.InvalidNodeID {
			t.Errorf("row %d (%q): origin unresolved after import", i, r.Name)
		}
	}

	// Playback must be startable without panicking.
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("post-import playback PANICKED: %v\n%s", rec, debug.Stack())
			}
		}()
		g.SetPlaying(true)
		_ = g.Update()
		g.SetPlaying(false)
	}()

	// Performance budget: this file must load quickly on the fast path.
	// Skip under coverage instrumentation (`make coverage-go`): the embedded-PCM
	// gunzip is CPU-heavy, and per-statement coverage counters inflate wall-clock
	// time well past the budget even though the real fast-path cost is fine.
	// Timing assertions are meaningless when the binary is instrumented.
	if testing.CoverMode() == "" {
		const budget = 250 * time.Millisecond
		if elapsed > budget {
			t.Errorf("import too slow: %v > %v budget", elapsed, budget)
		}
	}
}
