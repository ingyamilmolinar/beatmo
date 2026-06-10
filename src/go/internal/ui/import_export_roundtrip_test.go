//go:build test

package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// roleOfInstrument finds the exported Role for instrument id, "" if absent.
func roleOfInstrument(f exportFile, id string) string {
	for _, in := range f.Instruments {
		if in.ID == id {
			return in.Role
		}
	}
	return ""
}

// TestImportExportRoundtripPreservesAllFields is the durable guarantee behind
// "every valid exported JSON must import successfully": build a project that
// exercises the breadth of the export schema, export → (fresh instance) import →
// re-export, and assert the second export reproduces the first. It specifically
// guards the two export-but-not-imported asymmetries closed in this change —
// per-instrument Role and top-level Kits — by dropping the process-global kit
// registry between export and import (a real "brand-new instance"); without the
// import-side fixes the re-export loses the kit and the role.
func TestImportExportRoundtripPreservesAllFields(t *testing.T) {
	withDefaultAudio(t)
	t.Cleanup(audio.ResetKitsForTest)
	audio.ResetKitsForTest()

	// --- Build a rich source project ---
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 2, model.NodeTypeSilent)
	g.addEdge(a, b)
	g.addEdge(b, c)

	// Node params: pitch/duration/volume + logic + groove (all optional fields).
	if mn, ok := g.graph.GetNodeByID(b.ID); ok {
		p := mn.Params
		p.Pitch = 3
		p.Duration = 0.5
		p.Volume = 0.8
		p.LogicKind = "probability"
		p.LogicP = 0.5
		p.GrooveKind = "rush"
		p.GroovePct = 0.4
		g.graph.SetNodeParams(b.ID, p)
	}

	// Row fields: name, instrument, origin, volume, role, pan, sends.
	r0 := g.drum.Rows[0]
	r0.Name = "Kick"
	g.drum.SetInstrument("kick")
	r0.Origin = a.ID
	r0.Node = a
	r0.Volume = 0.9
	r0.Role = "kick"
	r0.Pan = -0.3
	r0.DelaySend = 0.2
	r0.ReverbSend = 0.4
	g.drum.SetBPM(132)

	// A registered kit (top-level Kits field).
	audio.RegisterKit(audio.Kit{
		ID:          "kit.rt",
		DisplayName: "Roundtrip Kit",
		Members:     map[string]string{"kick": "kick"},
	})

	bytes1, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("first export: %v", err)
	}
	var f1 exportFile
	if err := json.Unmarshal(bytes1, &f1); err != nil {
		t.Fatalf("unmarshal export1: %v", err)
	}
	// Preconditions: the source export actually carries the fields under test.
	if len(f1.Kits) != 1 {
		t.Fatalf("precondition: source export Kits=%d, want 1", len(f1.Kits))
	}
	if roleOfInstrument(f1, "kick") != "kick" {
		t.Fatalf("precondition: source export missing kick Role")
	}

	// --- Brand-new instance: drop the global kit registry so re-export can only
	// re-emit the kit if Import re-registered it. ---
	audio.ResetKitsForTest()

	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(800, 600)
	if err := g2.Import(bytes1); err != nil {
		t.Fatalf("import into fresh instance: %v", err)
	}

	bytes2, err := g2.drum.exportBytes()
	if err != nil {
		t.Fatalf("second export: %v", err)
	}
	var f2 exportFile
	if err := json.Unmarshal(bytes2, &f2); err != nil {
		t.Fatalf("unmarshal export2: %v", err)
	}

	// --- Round-trip closure assertions ---
	if f2.BPM != f1.BPM {
		t.Errorf("BPM: export2=%d export1=%d", f2.BPM, f1.BPM)
	}
	if f2.Subdiv != f1.Subdiv {
		t.Errorf("Subdiv: export2=%d export1=%d", f2.Subdiv, f1.Subdiv)
	}
	if len(f2.Nodes) != len(f1.Nodes) {
		t.Errorf("node count: export2=%d export1=%d", len(f2.Nodes), len(f1.Nodes))
	}
	if len(f2.Instruments) != len(f1.Instruments) {
		t.Errorf("instrument count: export2=%d export1=%d", len(f2.Instruments), len(f1.Instruments))
	}
	// Kits asymmetry (would be 0 without the importFile.Kits + RegisterKit fix).
	if len(f2.Kits) != 1 {
		t.Errorf("Kits dropped on import: export2 Kits=%d, want 1", len(f2.Kits))
	} else if f2.Kits[0].ID != "kit.rt" || f2.Kits[0].Members["kick"] != "kick" {
		t.Errorf("Kit corrupted on round-trip: %+v", f2.Kits[0])
	}
	// Role asymmetry (would be "" without the row.Role = inst.Role fix).
	if got := roleOfInstrument(f2, "kick"); got != "kick" {
		t.Errorf("Role dropped on import: export2 kick Role=%q, want \"kick\"", got)
	}

	// Spot-check a node's optional params survived export→import→export.
	var found bool
	for _, n := range f2.Nodes {
		if n.LogicKind == "probability" {
			found = true
			if n.GrooveKind != "rush" {
				t.Errorf("node GrooveKind=%q, want rush", n.GrooveKind)
			}
			if n.Pitch != 3 {
				t.Errorf("node Pitch=%v, want 3", n.Pitch)
			}
		}
	}
	if !found {
		t.Errorf("node with logic_kind=probability not found after round-trip")
	}

	// Per-instrument pan/sends round-trip.
	for _, in := range f2.Instruments {
		if in.ID == "kick" {
			if in.Pan != -0.3 {
				t.Errorf("kick Pan=%v, want -0.3", in.Pan)
			}
			if in.DelaySend != 0.2 {
				t.Errorf("kick DelaySend=%v, want 0.2", in.DelaySend)
			}
			if in.ReverbSend != 0.4 {
				t.Errorf("kick ReverbSend=%v, want 0.4", in.ReverbSend)
			}
		}
	}
}
