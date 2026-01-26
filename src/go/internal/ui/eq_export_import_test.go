package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

func TestExportIncludesEQ(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Ensure at least one node references the row origin so export is valid.
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	// Set EQ gains.
	g.drum.eqBandGainsDB = make([]float64, len(eqBandDefs))
	g.drum.eqBandGainsDB[0] = 3
	g.drum.eqBandGainsDB[5] = -2

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	if f.EQ == nil || len(f.EQ.GainsDB) != len(eqBandDefs) {
		t.Fatalf("expected EQ gains exported")
	}
	if f.EQ.GainsDB[0] != 3 || f.EQ.GainsDB[5] != -2 {
		t.Fatalf("unexpected gains %+v", f.EQ.GainsDB)
	}
	if len(f.EQ.BandsHz) != len(eqBandDefs) {
		t.Fatalf("expected band metadata")
	}
}

func TestImportAppliesEQ(t *testing.T) {
	assertDefaultParityState(t)
	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{{
			Name: "Row", ID: "snare", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
		EQ: &exportEQ{
			GainsDB: []float64{6, -3, 0},
		},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(g.drum.eqBandGainsDB) == 0 || g.drum.eqBandGainsDB[0] != 6 {
		t.Fatalf("eq gains not applied: %+v", g.drum.eqBandGainsDB)
	}
	rec := audio.LastSetEQ()
	if rec.ID != "main" {
		t.Fatalf("expected main EQ applied")
	}
	if len(rec.Bands) == 0 || rec.Bands[0].GainDB != 6 {
		t.Fatalf("expected gain applied to audio chain")
	}
}

func TestImportSyncsEQSliders(t *testing.T) {
	assertDefaultParityState(t)
	// Create a file with master EQ gains that map to non-50% slider values
	// Gain of 12 dB should map to slider value 1.0 (formula: value = gain/24 + 0.5)
	// Gain of -12 dB should map to slider value 0.0
	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
			EQ: &exportEQ{
				GainsDB: []float64{-12, 6, 0, 3, -6, 12, 0, 0, 0, 0},
			},
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
		EQ: &exportEQ{
			GainsDB: []float64{12, -6, 0, 3, -3, 6, 0, 0, 0, 0},
		},
	}
	data, _ := json.Marshal(file)

	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 300)

	// Initialize EQ sliders (normally done by draw, but we need them for test)
	if g.drum.eqSliders == nil {
		g.drum.eqSliders = make([]*Slider, len(eqBandDefs))
		for i := range g.drum.eqSliders {
			g.drum.eqSliders[i] = &Slider{Value: 0.5} // default to center
		}
	}

	// Import the file
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Verify master EQ sliders are synced (master is active by default)
	// Gain 12 -> slider (12/24)+0.5 = 1.0
	// Gain -6 -> slider (-6/24)+0.5 = 0.25
	if g.drum.activeEQChannel() != "main" {
		t.Fatalf("expected main channel active, got %s", g.drum.activeEQChannel())
	}
	if len(g.drum.eqSliders) < 2 {
		t.Fatalf("expected sliders initialized")
	}
	// Band 0: gain 12 dB -> slider 1.0
	if g.drum.eqSliders[0].Value != 1.0 {
		t.Errorf("slider[0] expected 1.0 for 12dB, got %f", g.drum.eqSliders[0].Value)
	}
	// Band 1: gain -6 dB -> slider 0.25
	expected1 := (-6.0 / 24.0) + 0.5
	if g.drum.eqSliders[1].Value != expected1 {
		t.Errorf("slider[1] expected %f for -6dB, got %f", expected1, g.drum.eqSliders[1].Value)
	}
	// Band 2: gain 0 dB -> slider 0.5
	if g.drum.eqSliders[2].Value != 0.5 {
		t.Errorf("slider[2] expected 0.5 for 0dB, got %f", g.drum.eqSliders[2].Value)
	}

	// Now switch to the instrument channel and verify those sliders sync
	g.drum.setEQActiveChannel("kick")
	if g.drum.activeEQChannel() != "kick" {
		t.Fatalf("expected kick channel active, got %s", g.drum.activeEQChannel())
	}
	// Band 0: gain -12 dB -> slider 0.0
	if g.drum.eqSliders[0].Value != 0.0 {
		t.Errorf("instrument slider[0] expected 0.0 for -12dB, got %f", g.drum.eqSliders[0].Value)
	}
	// Band 1: gain 6 dB -> slider 0.75
	expected1Inst := (6.0 / 24.0) + 0.5
	if g.drum.eqSliders[1].Value != expected1Inst {
		t.Errorf("instrument slider[1] expected %f for 6dB, got %f", expected1Inst, g.drum.eqSliders[1].Value)
	}
}
