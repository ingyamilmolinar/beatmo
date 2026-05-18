package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// Phase 3 schema tests: the v1 -> v1+synth JSON migration. The "version"
// field stays at 1 because every new field is additive (`omitempty`) so
// old loaders see the file unchanged.

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
	g.drum.Rows[0].Instrument = "snare"
	return g, ui
}

func TestExportInstrument_EmitsRecipeWhenBound(t *testing.T) {
	g, _ := newPhase3Game(t)
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })
	audio.BindInstrumentToRecipe("snare", "drum-snare")

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	var parsed exportFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Instruments) == 0 {
		t.Fatal("no instruments in export")
	}
	if parsed.Instruments[0].Recipe != "drum-snare" {
		t.Errorf("got Recipe=%q, want drum-snare", parsed.Instruments[0].Recipe)
	}
}

func TestExportInstrument_EmitsSynthParamsWhenSet(t *testing.T) {
	g, _ := newPhase3Game(t)
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	audio.BindInstrumentToRecipe("snare", "drum-snare")
	audio.SetInstrumentParam("snare", "decay", 0.4)
	audio.SetInstrumentParam("snare", "drive", 0.6)

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	var parsed exportFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.Instruments) == 0 {
		t.Fatal("no instruments in export")
	}
	got := parsed.Instruments[0].SynthParams
	if got["decay"] != 0.4 {
		t.Errorf("got SynthParams[decay]=%v, want 0.4 (full=%v)", got["decay"], got)
	}
	if got["drive"] != 0.6 {
		t.Errorf("got SynthParams[drive]=%v, want 0.6 (full=%v)", got["drive"], got)
	}
}

func TestExportInstrument_OmitsSynthParamsWhenEmpty(t *testing.T) {
	g, _ := newPhase3Game(t)
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	audio.ResetInstrumentParams("snare")

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	if strings.Contains(string(data), `"synth_params"`) {
		t.Errorf("export emitted synth_params for an instrument with no user params:\n%s", string(data))
	}
}

func TestExportNode_EmitsSynthOverridesMapNotIndividualFields(t *testing.T) {
	g, ui := newPhase3Game(t)
	if n, ok := g.graph.GetNodeByID(ui.ID); ok {
		p := n.Params
		p.SynthDecay = 0.5
		p.SynthDrive = 0.7
		g.graph.SetNodeParams(ui.ID, p)
	}

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	s := string(data)
	if !strings.Contains(s, `"synth_overrides"`) {
		t.Errorf("expected exported JSON to contain synth_overrides map, got:\n%s", s)
	}
	// Old individual fields must NOT be emitted alongside the new map.
	for _, banned := range []string{`"synth_decay"`, `"synth_tone"`, `"synth_attack"`, `"synth_drive"`, `"synth_body"`, `"synth_color"`, `"synth_brightness"`} {
		if strings.Contains(s, banned) {
			t.Errorf("export emitted legacy field %s; should use synth_overrides map only", banned)
		}
	}
}

func TestExportSchemaVersionStaysOne(t *testing.T) {
	g, _ := newPhase3Game(t)
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}
	var parsed exportFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Version != 1 {
		t.Errorf("Version=%d; want 1 (schema is additive)", parsed.Version)
	}
}

func TestImportLegacyNodeSynthBecomesOverrides(t *testing.T) {
	// Hand-crafted v1 legacy fixture with the 7 individual synth_* node
	// fields. Import must populate the same NodeParams.Synth* slots.
	legacy := []byte(`{
		"version": 1,
		"subdiv": 32,
		"bpm": 120,
		"instruments": [{"name":"Snare","id":"snare","kind":"builtin","volume":1,"origin":0,"color":"#FFFFFFFF"}],
		"nodes": [{"id":0,"i":0,"j":0,"type":"regular","synth_decay":0.5,"synth_drive":0.7}]
	}`)
	g, _ := newPhase3Game(t)
	if err := g.Import(legacy); err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Find any imported node and check its params.
	for _, node := range g.graph.Nodes {
		p := node.Params
		if p.SynthDecay == 0.5 && p.SynthDrive == 0.7 {
			return // found the imported node
		}
	}
	t.Error("imported legacy synth_decay/synth_drive did not reach any NodeParams")
}

func TestImportV2PerInstrumentSynthParams_PopulatesAudio(t *testing.T) {
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })
	audio.ResetInstrumentParams("snare")

	v2 := []byte(`{
		"version": 1,
		"subdiv": 32,
		"bpm": 120,
		"instruments": [{
			"name":"Snare","id":"snare","kind":"builtin","volume":1,
			"origin":0,"color":"#FFFFFFFF",
			"recipe":"drum-snare",
			"synth_params":{"decay":0.4,"drive":0.8}
		}],
		"nodes": [{"id":0,"i":0,"j":0,"type":"regular"}]
	}`)
	g, _ := newPhase3Game(t)
	if err := g.Import(v2); err != nil {
		t.Fatalf("Import: %v", err)
	}
	got := audio.GetInstrumentParams("snare")
	if got["decay"] != 0.4 {
		t.Errorf("audio params decay=%v, want 0.4", got["decay"])
	}
	if got["drive"] != 0.8 {
		t.Errorf("audio params drive=%v, want 0.8", got["drive"])
	}
}

func TestImportV2SynthOverrides_PopulatesNodeParams(t *testing.T) {
	v2 := []byte(`{
		"version": 1,
		"subdiv": 32,
		"bpm": 120,
		"instruments": [{"name":"Snare","id":"snare","kind":"builtin","volume":1,"origin":0,"color":"#FFFFFFFF"}],
		"nodes": [{"id":0,"i":0,"j":0,"type":"regular","synth_overrides":{"decay":0.6,"drive":0.9}}]
	}`)
	g, _ := newPhase3Game(t)
	if err := g.Import(v2); err != nil {
		t.Fatalf("Import: %v", err)
	}
	for _, node := range g.graph.Nodes {
		p := node.Params
		if p.SynthDecay == 0.6 && p.SynthDrive == 0.9 {
			return
		}
	}
	t.Error("synth_overrides map did not reach any NodeParams")
}

func TestImportV2_RoundTripPreservesEverything(t *testing.T) {
	t.Cleanup(func() { audio.ResetInstrumentParams("snare") })

	// Build a project with both per-instrument SynthParams and per-node
	// SynthOverrides, export it, re-import it, check both sides.
	g, ui := newPhase3Game(t)
	audio.BindInstrumentToRecipe("snare", "drum-snare")
	audio.SetInstrumentParam("snare", "decay", 0.4)

	if n, ok := g.graph.GetNodeByID(ui.ID); ok {
		p := n.Params
		p.SynthDecay = 0.5
		g.graph.SetNodeParams(ui.ID, p)
	}

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("exportBytes: %v", err)
	}

	// Reset and re-import into a fresh Game.
	audio.ResetInstrumentParams("snare")

	g2, _ := newPhase3Game(t)
	if err := g2.Import(data); err != nil {
		t.Fatalf("re-Import: %v", err)
	}
	if got := audio.GetInstrumentParams("snare")["decay"]; got != 0.4 {
		t.Errorf("round-trip: SynthParams[decay]=%v want 0.4", got)
	}
	foundDecay := false
	for _, node := range g2.graph.Nodes {
		if node.Params.SynthDecay == 0.5 {
			foundDecay = true
			break
		}
	}
	if !foundDecay {
		t.Error("round-trip: SynthDecay=0.5 not preserved on any imported node")
	}
}
