package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func mustExport(t *testing.T, g *Game) []byte {
	t.Helper()
	b, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	return b
}

func TestExportCarriesEditedDocument(t *testing.T) {
	g := newDocsGame(t, roster.Kick)
	audio.SetInstrumentParam(roster.Kick, "post_decay", 0.9)
	var f projectFile
	if err := json.Unmarshal(mustExport(t, g), &f); err != nil {
		t.Fatal(err)
	}
	d, ok := f.InstrumentDocs[roster.Kick]
	if !ok {
		t.Fatalf("instrument_docs lacks kick: %v", f.InstrumentDocs)
	}
	if v := instrument.FieldValues(d.Doc, audio.BuiltinModuleRegistry())["post_decay"]; v != 0.9 {
		t.Fatalf("instrument_docs[kick] post_decay = %v, want 0.9", v)
	}
	if strings.Contains(string(mustExport(t, g)), `"synth_params"`) || strings.Contains(string(mustExport(t, g)), `"recipe"`) {
		t.Fatal("the export still writes the recipe-era fields")
	}
}

// An instrument the file does not carry returns to its library entry.
func TestImportReplacesDocuments(t *testing.T) {
	g := newDocsGame(t, roster.Kick, roster.Snare)
	audio.SetInstrumentParam(roster.Kick, "post_decay", 0.9)
	file := mustExport(t, g)

	audio.SetInstrumentParam(roster.Kick, "post_decay", 0.3)
	audio.SetInstrumentParam(roster.Hihat, "post_decay", 0.3) // not in the file
	if err := g.Import(file); err != nil {
		t.Fatal(err)
	}
	if v := docField(t, "kick", "post_decay"); v != 0.9 {
		t.Errorf("kick post_decay = %v after import, want the file's 0.9", v)
	}
	if audio.InstrumentDocModified(roster.Hihat) {
		t.Error("an instrument absent from the file kept the previous session's edit")
	}
}

// Review Focus 4 (project half).
func TestImportRejectsUnknownModuleTypeBeforeReset(t *testing.T) {
	g := newDocsGame(t, roster.Kick)
	before := mustExport(t, g)
	var f map[string]any
	if err := json.Unmarshal(before, &f); err != nil {
		t.Fatal(err)
	}
	docs, _ := f["instrument_docs"].(map[string]any)
	if len(docs) == 0 {
		t.Fatal("export carries no instrument_docs")
	}
	for _, raw := range docs {
		raw.(map[string]any)["fx"] = []any{map[string]any{"type": "fx.warp", "v": 1, "on": true, "values": map[string]any{}}}
		break
	}
	bad, _ := json.Marshal(f)
	err := g.Import(bad)
	if err == nil || !strings.Contains(err.Error(), "instrument_docs.") || !strings.Contains(err.Error(), "fx.warp") {
		t.Fatalf("want a path-named error, got %v", err)
	}
	if string(mustExport(t, g)) != string(before) {
		t.Fatal("a rejected file must leave the circuit untouched")
	}
}

func TestImportRejectsMismatchedDocumentKey(t *testing.T) {
	g := newDocsGame(t, roster.Kick)
	var f map[string]any
	_ = json.Unmarshal(mustExport(t, g), &f)
	docs := f["instrument_docs"].(map[string]any)
	docs["kick"].(map[string]any)["id"] = "snare"
	bad, _ := json.Marshal(f)
	if err := g.Import(bad); err == nil || !strings.Contains(err.Error(), "instrument_docs.kick") {
		t.Fatalf("want a key/id mismatch error, got %v", err)
	}
}

func TestImportRejectsLegacySynthParams(t *testing.T) {
	g := newDocsGame(t, roster.Kick)
	for _, legacy := range []string{`"synth_params":{"pitch":0}`, `"recipe":"synth-modular-kick-base"`} {
		data := `{"schema_version":"0.0.1","bpm":120,"instruments":[{"name":"K","id":"kick","kind":"builtin","volume":1,"origin":0,"color":"#FFFFFFFF",` + legacy + `}],"nodes":[]}`
		if err := g.Import([]byte(data)); err == nil || !strings.Contains(err.Error(), "predates instrument documents") {
			t.Fatalf("%s: want the legacy error, got %v", legacy, err)
		}
	}
}
