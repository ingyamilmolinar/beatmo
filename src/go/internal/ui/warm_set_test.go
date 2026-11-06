//go:build test

package ui

import (
	"os"
	"path/filepath"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// warmSetTestGameWithTemplate loads a real multi-row circuit (3 rows /
// 117 nodes / ~16 distinct (instrument, pitch) pairs) instead of the bare
// New() fixture, which is empirically 1 row / 1 node / 1 beat-info and cannot
// exercise dedup, the cap, or multi-row coverage. Skips (does not fail) the
// test if the asset is unreadable, so a missing fixture never masquerades as
// a real failure.
func warmSetTestGameWithTemplate(t *testing.T) *Game {
	t.Helper()
	assertDefaultParityState(t)
	g := New(game_log.New(nil, game_log.LevelError))
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	data, err := os.ReadFile(filepath.Join("..", "assets", "templates", "beethoven-ode-to-joy.json"))
	if err != nil {
		t.Skipf("template fixture unavailable: %v", err)
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("Import: %v", err)
	}
	g.updateBeatInfos()
	return g
}

// countRegularBeatInfos walks g.beatInfosByRow exactly like buildWarmSet does
// and counts every NodeTypeRegular entry across every row, WITHOUT
// deduplicating. This is the raw walk count the dedup test compares against.
func countRegularBeatInfos(g *Game) int {
	n := 0
	rows := g.beatInfosByRow
	for row := range rows {
		if row >= len(g.drum.Rows) {
			break
		}
		if g.drum.Rows[row].Instrument == "" {
			continue
		}
		for _, info := range rows[row] {
			if info.NodeType == model.NodeTypeRegular {
				n++
			}
		}
	}
	return n
}

// TestBuildWarmSetCoversEveryRowPitch asserts the warm set enumerates every
// distinct (instrument, rounded-pitch) pair the circuit can trigger — NOT just
// the pairs inside the current predictor render window, and NOT just row 0.
// The window horizon was the original bug: pitches outside it were only
// discovered mid-playback. Uses the 3-row Ode to Joy template so a "only
// processes the first row" or "breaks after the first regular node" bug is
// actually reachable.
func TestBuildWarmSetCoversEveryRowPitch(t *testing.T) {
	g := warmSetTestGameWithTemplate(t)

	set := g.buildWarmSet()
	if set.Len() == 0 {
		t.Fatal("warm set empty for the template circuit")
	}

	// Every item must name a row instrument that actually exists.
	known := map[string]bool{}
	for _, r := range g.drum.Rows {
		known[string(r.Instrument)] = true
	}
	distinctInst := map[string]bool{}
	for _, it := range set.items {
		if it.inst == "" || !known[string(it.inst)] {
			t.Errorf("warm item names unknown instrument %q", it.inst)
		}
		if it.pitch != roundWarmPitch(it.pitch) {
			t.Errorf("warm item pitch %v is not rounded to 0.5st", it.pitch)
		}
		distinctInst[string(it.inst)] = true
	}

	// A bug that only walks row 0 (or stops after the first regular node)
	// would still pass a single-instrument check on the New() fixture, but
	// must fail here: this template has 3 differently-instrumented rows.
	if len(distinctInst) < 2 {
		t.Fatalf("warm set spans only %d distinct instrument(s) %v; expected coverage of multiple rows (cello/contrabass/bassoon)", len(distinctInst), distinctInst)
	}

	// Every row with a non-empty instrument must contribute at least one item.
	gotByInst := map[string]bool{}
	for _, it := range set.items {
		gotByInst[string(it.inst)] = true
	}
	for _, r := range g.drum.Rows {
		if r.Instrument == "" {
			continue
		}
		if !gotByInst[string(r.Instrument)] {
			t.Errorf("row instrument %q contributed no warm item", r.Instrument)
		}
	}
}

// TestBuildWarmSetDeduplicates pins that a looping melody producing the same
// (instrument, pitch) on many beats yields ONE item, so a graph edit cannot
// burst-flood the warm backend. Uses the Ode to Joy template (117 nodes
// collapsing to ~16 pairs) so the assertion can actually observe collapsing —
// against the bare 1-node New() fixture this test could pass even if dedup
// were deleted entirely.
func TestBuildWarmSetDeduplicates(t *testing.T) {
	g := warmSetTestGameWithTemplate(t)

	raw := countRegularBeatInfos(g)
	set := g.buildWarmSet()

	seen := map[warmItem]bool{}
	for _, it := range set.items {
		if seen[it] {
			t.Fatalf("duplicate warm item %+v", it)
		}
		seen[it] = true
	}

	if set.Len() >= raw {
		t.Fatalf("warm set size %d did not collapse below raw regular-node count %d; dedup does not appear to be running", set.Len(), raw)
	}
}

// TestBuildWarmSetRespectsCap pins the bound so a pathological circuit cannot
// make the readiness gate effectively infinite. maxWarmSetItems is lowered
// below the template's natural size so truncation is actually exercised
// (against the default 512 cap and a 16-item fixture, this test could pass
// even if the cap check were deleted).
func TestBuildWarmSetRespectsCap(t *testing.T) {
	g := warmSetTestGameWithTemplate(t)

	full := g.buildWarmSet()
	if full.Len() < 6 {
		t.Fatalf("template fixture only yields %d items; need >5 for the cap test to be meaningful", full.Len())
	}

	orig := maxWarmSetItems
	maxWarmSetItems = 5
	t.Cleanup(func() { maxWarmSetItems = orig })

	capped := g.buildWarmSet()
	if capped.Len() != 5 {
		t.Fatalf("warm set size %d, want exactly the cap (5)", capped.Len())
	}
	if !capped.truncated {
		t.Error("truncated flag not set when the cap was hit")
	}

	// Companion: with the cap above the fixture's natural size, truncated
	// must be false and the set must match the untruncated size.
	maxWarmSetItems = orig
	if full.truncated {
		t.Error("truncated flag set when the cap was not hit")
	}
	if full.Len() > maxWarmSetItems {
		t.Fatalf("warm set size %d exceeds cap %d", full.Len(), maxWarmSetItems)
	}
}

// TestWarmSetIgnoresTheVisibleWindow pins that the warm set covers the row's
// whole graph path, not the slice of it currently on screen. beatInfosByRow is
// the full path from graph.CalculateBeatRowFrom (NOT a render window), so
// scrolling or shrinking the timeline must not shrink the set. Guards against a
// future "optimization" that scopes warming to the viewport — which would put
// every off-screen pitch back on the cold-render path during playback. Run
// against the Ode to Joy template so "the set did not shrink" is a statement
// about ~16 items, not the trivial 1-item New() fixture.
func TestWarmSetIgnoresTheVisibleWindow(t *testing.T) {
	g := warmSetTestGameWithTemplate(t)

	full := g.buildWarmSet().Len()
	if full == 0 {
		t.Fatal("warm set is empty for the template circuit")
	}

	g.drum.Length = max1(g.drum.Length / 4)
	g.drum.Offset += 2
	g.updateBeatInfos()

	if got := g.buildWarmSet().Len(); got != full {
		t.Errorf("warm set changed with the visible window: got %d want %d", got, full)
	}
}
