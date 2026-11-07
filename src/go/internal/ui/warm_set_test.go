//go:build test

package ui

import (
	"os"
	"path/filepath"
	"testing"

	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// A real multi-row template: the bare New() fixture has one row and one node, too little to exercise dedup, the cap
// or multi-row coverage. Skips rather than fails if the asset is unreadable.
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

// The raw walk count, without deduplication.
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

// Every distinct (instrument, rounded pitch) the circuit can trigger, not only those in the render window or on row
// 0: a pitch outside the window would be discovered mid-playback.
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

	// Three differently instrumented rows, so a walk of row 0 only fails here.
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

// Many beats of the same pair yield ONE item, so a graph edit cannot flood the warm backend; the template collapses
// enough that deleting dedup would show.
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

// A pathological circuit must not make the gate effectively infinite; the cap is lowered below the template's size
// so truncation really happens.
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

	// With the cap above the natural size nothing is truncated.
	maxWarmSetItems = orig
	if full.truncated {
		t.Error("truncated flag set when the cap was not hit")
	}
	if full.Len() > maxWarmSetItems {
		t.Fatalf("warm set size %d exceeds cap %d", full.Len(), maxWarmSetItems)
	}
}

// The warm set follows the whole graph path, not the slice on screen: warming only the viewport would put every
// off-screen pitch back on the cold-render path during playback.
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
