package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// Phase 3 schema tests: the per-instrument / per-node synth fields. Every
// field is additive (`omitempty`); the fixtures carry the current
// schema_version.

// newPhase3Game returns a Game with one row + one node ready for
// export/import round-trip testing. Mirrors the setup pattern from
// eq_export_import_test.go.
func newPhase3Game(t *testing.T) (*Game, *uiNode) {
	t.Helper()
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = roster.Snare
	return g, ui
}

// exportedDoc exports g and returns the instrument document the file carries
// for id, read through its flat field view.
func exportedDocFields(t *testing.T, g *Game, id instrument.ID) (map[string]float64, instrument.Doc) {
	t.Helper()
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	var parsed projectFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	d, ok := parsed.InstrumentDocs[id]
	if !ok {
		t.Fatalf("export carries no instrument document for %q", id)
	}
	return instrument.FieldValues(d.Doc, audio.BuiltinModuleRegistry()), d.Doc
}

func TestExportInstrument_EmitsDocumentWithOrigin(t *testing.T) {
	g, _ := newPhase3Game(t)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })
	audio.ResetInstrumentParams(roster.Snare)
	_, d := exportedDocFields(t, g, roster.Snare)
	if d.Origin != roster.Snare || d.ID != roster.Snare {
		t.Errorf("exported document id=%q origin=%q, want snare/snare", d.ID, d.Origin)
	}
}

func TestExportInstrument_DocumentCarriesEdits(t *testing.T) {
	g, _ := newPhase3Game(t)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.4)
	audio.SetInstrumentParam(roster.Snare, "gen1_snare_tune", 1.3)
	got, _ := exportedDocFields(t, g, roster.Snare)
	if got["post_decay"] != 0.4 || got["gen1_snare_tune"] != 1.3 {
		t.Errorf("exported document post_decay=%v gen1_snare_tune=%v, want 0.4 / 1.3", got["post_decay"], got["gen1_snare_tune"])
	}
}

func TestExportNeverWritesRecipeEraFields(t *testing.T) {
	g, _ := newPhase3Game(t)
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.4)
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	for _, k := range []string{`"synth_params"`, `"recipe"`, `"recipes"`} {
		if strings.Contains(string(data), k) {
			t.Errorf("export wrote the recipe-era key %s", k)
		}
	}
}

// TestExportStampsSchemaVersion pins the write side: every exported project
// carries the current schema_version, and never the retired integer marker.
func TestExportStampsSchemaVersion(t *testing.T) {
	g, _ := newPhase3Game(t)
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	var parsed projectFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.SchemaVersion != schema.Version {
		t.Errorf("schema_version=%q; want %q", parsed.SchemaVersion, schema.Version)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if _, present := raw["version"]; present {
		t.Errorf("exported document still carries a \"version\" key: %s", data)
	}
}

func TestImport_RoundTripPreservesTheDocument(t *testing.T) {
	t.Cleanup(func() { audio.ResetInstrumentParams(roster.Snare) })

	g, _ := newPhase3Game(t)
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.4)
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}

	// Reset and re-import into a fresh Game.
	audio.ResetInstrumentParams(roster.Snare)
	g2, _ := newPhase3Game(t)
	if err := g2.Import(data); err != nil {
		t.Fatalf("re-Import: %v", err)
	}
	if got := audio.GetInstrumentParams(roster.Snare)["post_decay"]; got != 0.4 {
		t.Errorf("round-trip: post_decay=%v want 0.4", got)
	}
}
