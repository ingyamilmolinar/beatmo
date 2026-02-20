package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
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

func TestExportImportHPFLPF(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	// Enable master HPF and LPF.
	g.drum.hpfEnabled = true
	g.drum.hpfCutoffHz = 150
	g.drum.lpfEnabled = true
	g.drum.lpfCutoffHz = 12000

	// Set per-row HPF.
	g.drum.Rows[0].HPFEnabled = true
	g.drum.Rows[0].HPFCutoffHz = 80
	g.drum.Rows[0].LPFEnabled = false
	g.drum.Rows[0].LPFCutoffHz = 20000

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Parse and verify master EQ.
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	if f.EQ == nil {
		t.Fatal("expected master EQ exported")
	}
	if !f.EQ.HPFEnabled || f.EQ.HPFCutoffHz != 150 {
		t.Errorf("master HPF: enabled=%v cutoff=%.0f", f.EQ.HPFEnabled, f.EQ.HPFCutoffHz)
	}
	if !f.EQ.LPFEnabled || f.EQ.LPFCutoffHz != 12000 {
		t.Errorf("master LPF: enabled=%v cutoff=%.0f", f.EQ.LPFEnabled, f.EQ.LPFCutoffHz)
	}

	// Verify per-instrument EQ.
	if len(f.Instruments) == 0 || f.Instruments[0].EQ == nil {
		t.Fatal("expected per-instrument EQ exported")
	}
	instEQ := f.Instruments[0].EQ
	if !instEQ.HPFEnabled || instEQ.HPFCutoffHz != 80 {
		t.Errorf("inst HPF: enabled=%v cutoff=%.0f", instEQ.HPFEnabled, instEQ.HPFCutoffHz)
	}
	if instEQ.LPFEnabled {
		t.Error("expected inst LPF disabled")
	}

	// Import into a new game and verify state restored.
	g2 := New(logger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(640, 480)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Master HPF/LPF.
	if !g2.drum.hpfEnabled || g2.drum.hpfCutoffHz != 150 {
		t.Errorf("imported master HPF: enabled=%v cutoff=%.0f", g2.drum.hpfEnabled, g2.drum.hpfCutoffHz)
	}
	if !g2.drum.lpfEnabled || g2.drum.lpfCutoffHz != 12000 {
		t.Errorf("imported master LPF: enabled=%v cutoff=%.0f", g2.drum.lpfEnabled, g2.drum.lpfCutoffHz)
	}

	// Per-row HPF/LPF.
	if len(g2.drum.Rows) == 0 {
		t.Fatal("no rows after import")
	}
	row := g2.drum.Rows[0]
	if !row.HPFEnabled || row.HPFCutoffHz != 80 {
		t.Errorf("imported row HPF: enabled=%v cutoff=%.0f", row.HPFEnabled, row.HPFCutoffHz)
	}
	if row.LPFEnabled {
		t.Error("expected imported row LPF disabled")
	}
}

func TestImportLegacyNoFilters(t *testing.T) {
	assertDefaultParityState(t)
	// Old file without HPF/LPF fields should load correctly with filters disabled.
	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{{
			Name: "Row", ID: "snare", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
		EQ: &exportEQ{
			GainsDB: []float64{3, 0, 0, 0, 0, 0, 0, 0, 0, 0},
		},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// HPF/LPF should default to disabled.
	if g.drum.hpfEnabled {
		t.Error("expected HPF disabled for legacy file")
	}
	if g.drum.lpfEnabled {
		t.Error("expected LPF disabled for legacy file")
	}
	if g.drum.hpfCutoffHz != 20 {
		t.Errorf("expected HPF cutoff 20, got %.0f", g.drum.hpfCutoffHz)
	}
	if g.drum.lpfCutoffHz != 20000 {
		t.Errorf("expected LPF cutoff 20000, got %.0f", g.drum.lpfCutoffHz)
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
