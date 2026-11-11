package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

func TestImportClampsRowAndNodeParams(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// One instrument with excessive volume; one node with invalid params.
	// Local node with groove fields for this test
	type node2 struct {
		ID         int     `json:"id"`
		I          int     `json:"i"`
		J          int     `json:"j"`
		Type       string  `json:"type"`
		Volume     float64 `json:"volume,omitempty"`
		Duration   float64 `json:"duration,omitempty"`
		GrooveKind string  `json:"groove_kind,omitempty"`
		GroovePct  float64 `json:"groove_pct,omitempty"`
	}
	f := struct {
		SchemaVersion string  `json:"schema_version"`
		Subdiv        int     `json:"subdiv"`
		BPM           int     `json:"bpm"`
		Nodes         []node2 `json:"nodes"`
		Insts         []tInst `json:"instruments"`
	}{
		SchemaVersion: schema.Version,
		Subdiv:        32,
		BPM:           120,
		Nodes:         []node2{{ID: 1, I: 0, J: 0, Type: "regular", Volume: -5, Duration: 0, GrooveKind: "delay", GroovePct: 2}},
		Insts:         []tInst{{Name: "Row", ID: roster.Snare, Kind: "builtin", Volume: 9.9, Origin: 1, Color: "#FFFFFFFF"}},
	}
	// Recompose with correct field names for instruments
	type file struct {
		SchemaVersion string  `json:"schema_version"`
		Subdiv        int     `json:"subdiv"`
		BPM           int     `json:"bpm"`
		Nodes         []node2 `json:"nodes"`
		Instruments   []tInst `json:"instruments"`
	}
	data, _ := json.Marshal(file{SchemaVersion: f.SchemaVersion, Subdiv: f.Subdiv, BPM: f.BPM, Nodes: f.Nodes, Instruments: f.Insts})
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if len(g.drum.Rows) == 0 {
		t.Fatalf("no rows after import")
	}
	if !(g.drum.Rows[0].Volume >= 0 && g.drum.Rows[0].Volume <= 4) {
		t.Fatalf("row volume not clamped: %v", g.drum.Rows[0].Volume)
	}
	// Find node param
	var any *uiNode
	for _, n := range g.nodes {
		any = n
		break
	}
	if any == nil {
		t.Fatalf("missing node")
	}
	if mn, ok := g.graph.GetNodeByID(any.ID); ok {
		// Node volume is floored at 0 (the imported -5 becomes 0) but has no
		// upper clamp — per-node gain is intentionally allowed to go arbitrarily
		// high. (Row/instrument volume, checked above, is still clamped to [0,4].)
		if mn.Params.Volume < 0 {
			t.Fatalf("node volume not floored at 0: %v", mn.Params.Volume)
		}
		if mn.Params.Duration <= 0 {
			// Duration should not be <=0 after import; default remains 1 (set by model.New node params default)
			t.Fatalf("node duration invalid after import: %v", mn.Params.Duration)
		}
		if mn.Params.GrooveKind != model.GrooveDelay || mn.Params.GroovePct != 1 {
			t.Fatalf("groove = %v %v, want delay clamped to 1", mn.Params.GrooveKind, mn.Params.GroovePct)
		}
	} else {
		t.Fatalf("node not found in graph")
	}
}

// TestImportRejectsUnknownGroove: a groove has one spelling; anything else
// rejects the file before anything resets (it used to be silently dropped).
func TestImportRejectsUnknownGroove(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	data := []byte(`{"schema_version":"` + schema.Version + `","subdiv":32,"bpm":120,
		"instruments":[{"name":"Row","id":"snare","kind":"builtin","volume":1,"origin":1,"color":"#FFFFFFFF"}],
		"nodes":[{"id":1,"i":0,"j":0,"type":"regular","groove_kind":"weird","groove_pct":0.5}]}`)
	if err := g.Import(data); err == nil {
		t.Fatal("import accepted groove_kind \"weird\"")
	}
}

// TestImportNeverReplacesABuiltinWithAWAV: a row naming a shipped instrument
// with a WAV path would have registered that file under the built-in's id and
// replaced it for the session. A built-in is a sample only through embedded
// PCM (the Sampler's edit of it), so such a file is rejected — and so is a row
// that carries a path but claims to be the synth.
func TestImportNeverReplacesABuiltinWithAWAV(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	for _, inst := range []string{
		`{"name":"Kick","id":"kick","kind":"sample","path":"/tmp/other.wav","volume":1,"origin":1,"color":"#FFFFFFFF"}`,
		`{"name":"Kick","id":"kick--2","kind":"sample","path":"/tmp/other.wav","volume":1,"origin":1,"color":"#FFFFFFFF"}`,
		`{"name":"Kick","id":"kick","kind":"sample","volume":1,"origin":1,"color":"#FFFFFFFF"}`,
		`{"name":"Clap","id":"my-clap","kind":"builtin","path":"/tmp/clap.wav","volume":1,"origin":1,"color":"#FFFFFFFF"}`,
		`{"name":"Clap","id":"my clap","kind":"builtin","volume":1,"origin":1,"color":"#FFFFFFFF"}`,
	} {
		data := []byte(`{"schema_version":"` + schema.Version + `","subdiv":32,"bpm":120,"instruments":[` + inst + `],
			"nodes":[{"id":1,"i":0,"j":0,"type":"regular"}]}`)
		if err := g.Import(data); err == nil {
			t.Errorf("import accepted %s", inst)
		}
	}
}
