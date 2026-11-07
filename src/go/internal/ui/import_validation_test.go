package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// minimal JSON structs for composing import payloads
type tNode struct {
	ID        int     `json:"id"`
	I         int     `json:"i"`
	J         int     `json:"j"`
	Type      string  `json:"type"`
	Outputs   []int   `json:"outputs,omitempty"`
	Volume    float64 `json:"volume,omitempty"`
	Pitch     float64 `json:"pitch,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	SkipEvery int     `json:"skip_every,omitempty"`
	LogicKind string  `json:"logic_kind,omitempty"`
	LogicN    int     `json:"logic_n,omitempty"`
	LogicP    float64 `json:"logic_p,omitempty"`
}

type tInst struct {
	Name   string        `json:"name"`
	ID     instrument.ID `json:"id"`
	Kind   string        `json:"kind"`
	Volume float64       `json:"volume"`
	Origin int           `json:"origin"`
	Color  string        `json:"color"`
}

type tFile struct {
	SchemaVersion string  `json:"schema_version"`
	Subdiv        int     `json:"subdiv"`
	BPM           int     `json:"bpm"`
	Instruments   []tInst `json:"instruments"`
	Nodes         []tNode `json:"nodes"`
}

func TestImportClampsColorLogicAndCoords(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// Use default 32 subdiv grid.

	// Construct payload with invalid color and out-of-range logic and coords.
	huge := 1_000_000_000
	f := tFile{
		SchemaVersion: schema.Version,
		Subdiv:        32,
		BPM:           100,
		Instruments: []tInst{{
			Name: "X", ID: roster.Kick, Kind: "builtin", Volume: 1, Origin: 1, Color: "#nothex",
		}},
		Nodes: []tNode{{
			ID: 1, I: huge, J: -huge, Type: "regular",
			LogicKind: "probability", LogicP: 1.7, // should clamp to 1.0
		}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Color should fall back to opaque white due to invalid hex.
	if len(g.drum.Rows) == 0 {
		t.Fatalf("expected at least one row after import")
	}
	r, gcol, b, a := g.drum.Rows[0].Color.RGBA()
	if r != 0xFFFF || gcol != 0xFFFF || b != 0xFFFF || a != 0xFFFF {
		t.Fatalf("invalid color was not clamped to white, got %#v", g.drum.Rows[0].Color)
	}

	// Node should be present and coordinates clamped within ±MaxDiv*factor.
	bound := g.grid.MaxDiv() * importCoordLimitFactor
	// Find model node by scanning graph.
	var found *uiNode
	for _, n := range g.nodes {
		if n.ID == 0 || n.ID == 1 {
			// origin id maps from file id 1; match by closest I/J
			found = n
			break
		}
	}
	if found == nil {
		// fallback to find any node (import created exactly one regular node)
		for _, n := range g.nodes {
			found = n
			break
		}
	}
	if found == nil {
		t.Fatalf("expected a node to be imported")
	}
	if intAbs2(found.I) > bound || intAbs2(found.J) > bound {
		t.Fatalf("coordinates not clamped: (%d,%d) > %d", found.I, found.J, bound)
	}

	// Check logic clamp applied in graph params (LogicP <= 1.0)
	if mn, ok := g.graph.GetNodeByID(found.ID); ok {
		if mn.Params.LogicKind != model.LogicProbability {
			t.Fatalf("logic kind not set: %q", mn.Params.LogicKind)
		}
		if !(mn.Params.LogicP >= 0 && mn.Params.LogicP <= 1) {
			t.Fatalf("logic P out of range: %f", mn.Params.LogicP)
		}
	} else {
		t.Fatalf("node not found in graph")
	}
}

func TestImportRejectsTooManyInstruments(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)
	// Build file with instruments > maxImportInstruments
	insts := make([]tInst, maxImportInstruments+1)
	for i := range insts {
		insts[i] = tInst{Name: "I", ID: "id", Kind: "builtin", Volume: 1, Origin: 0, Color: "#FFFFFFFF"}
	}
	f := tFile{SchemaVersion: schema.Version, Subdiv: 32, BPM: 120, Instruments: insts}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err == nil {
		t.Fatalf("expected error on too many instruments")
	}
}

// local abs (int) to avoid importing math for the test file
func intAbs2(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// Import is replace-not-merge, so a JSON doc with no version (an empty {} or another app's file) must be
// rejected before it touches the project, or it replaces everything with an empty one.
func TestImportRejectsVersionlessDoc(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	rowsBefore := len(g.drum.Rows)
	for _, doc := range []string{
		`{}`,
		`{"subdiv": 8, "bpm": 120}`,
		`{"version": 0, "instruments": [], "nodes": []}`,
		`{"version": -3}`,
	} {
		if err := g.Import([]byte(doc)); err == nil {
			t.Errorf("Import(%s) succeeded; want version-guard error", doc)
		}
		if got := len(g.drum.Rows); got != rowsBefore {
			t.Fatalf("Import(%s) mutated rows: %d -> %d (project wiped on rejected import)", doc, rowsBefore, got)
		}
	}
}
