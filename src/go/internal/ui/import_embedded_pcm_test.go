//go:build test

package ui

import (
	"os"
	"runtime/debug"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// A real exported file with embedded gzipped PCM must import into a brand-new game.
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

	// A panic in Import freezes the WASM Update goroutine with no error shown, so it is caught and reported with its stack.
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

	for _, id := range []instrument.ID{roster.KickSnap, roster.Snare, roster.Hihat, roster.FmEpianoDrive} {
		if !audio.IsUserSample(id) {
			t.Errorf("instrument %q: embedded PCM not registered (IsUserSample=false)", id)
			continue
		}
		rec, ok := audio.UserSamplePCM(id)
		if !ok || len(rec.PCM) == 0 {
			t.Errorf("instrument %q: UserSamplePCM empty (ok=%v len=%d)", id, ok, len(rec.PCM))
		}
	}

	// Import leaves an origin unset on a miss and only logs a warning.
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

	// Skipped under coverage: counters inflate the CPU-heavy gunzip far past the budget.
	if testing.CoverMode() == "" {
		const budget = 250 * time.Millisecond
		if elapsed > budget {
			t.Errorf("import too slow: %v > %v budget", elapsed, budget)
		}
	}
}
