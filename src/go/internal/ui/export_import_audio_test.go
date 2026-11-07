package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// --- Synth Params Tests ---

func TestExportOmitsSynthParamsWhenDefault(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	// Default synth params (all zero)

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	raw := string(data)
	for _, key := range []string{
		`"synth_decay"`, `"synth_tone"`, `"synth_attack"`,
		`"synth_drive"`, `"synth_body"`, `"synth_color"`, `"synth_brightness"`,
		`"synth_overrides"`,
	} {
		if contains(raw, key) {
			t.Errorf("expected %s omitted from JSON when default", key)
		}
	}
}

// --- Insert Effects Tests ---

func TestExportIncludesInsertEffects(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	// Through the audio layer, which export reads; r.Effects is a UI mirror that can lag.
	audio.SetInsertEffects(g.drum.Rows[0].Instrument, []audio.EffectSlot{
		{Type: "distortion", Enabled: true, Params: map[string]float64{"drive": 0.5}},
		{Type: "delay", Enabled: false, Params: map[string]float64{"time": 0.3, "feedback": 0.4}},
	})
	t.Cleanup(audio.ClearAllInsertEffects)

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(f.Instruments) == 0 {
		t.Fatal("no instruments")
	}
	fx := f.Instruments[0].Effects
	if len(fx) != 2 {
		t.Fatalf("Effects: got %d want 2", len(fx))
	}
	if fx[0].Type != "distortion" || !fx[0].Enabled {
		t.Errorf("Effects[0]: %+v", fx[0])
	}
	if fx[1].Type != "delay" || fx[1].Enabled {
		t.Errorf("Effects[1]: %+v", fx[1])
	}
}

func TestImportAppliesInsertEffects(t *testing.T) {
	assertDefaultParityState(t)
	effects := []audio.EffectSlot{
		{Type: "reverb", Enabled: true, Params: map[string]float64{"mix": 0.5}},
	}
	file := projectFile{
		Envelope: schema.Current(),
		Subdiv:   32,
		BPM:      120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: roster.Kick, Kind: kindBuiltin, Volume: 1, Origin: 1, Color: "#FFFFFFFF",
			Effects: effects,
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: model.NodeTypeRegular}},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	row := g.drum.Rows[0]
	if len(audio.GetInsertEffects(row.Instrument)) != 1 {
		t.Fatalf("Effects: got %d want 1", len(audio.GetInsertEffects(row.Instrument)))
	}
	if audio.GetInsertEffects(row.Instrument)[0].Type != "reverb" || !audio.GetInsertEffects(row.Instrument)[0].Enabled {
		t.Errorf("Effects[0]: %+v", audio.GetInsertEffects(row.Instrument)[0])
	}
	// Verify audio.SetInsertEffects was called
	got := audio.GetInsertEffects(roster.Kick)
	if len(got) != 1 || got[0].Type != "reverb" {
		t.Errorf("audio.GetInsertEffects: %+v", got)
	}
}

func TestExportImportInsertEffectsRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	// Install through the audio layer — the single source of truth export reads.
	audio.SetInsertEffects(g.drum.Rows[0].Instrument, []audio.EffectSlot{
		{Type: "chorus", Enabled: true, Params: map[string]float64{"rate": 0.5, "depth": 0.3}},
		{Type: "bitcrusher", Enabled: true, Params: map[string]float64{"bits": 8}},
	})
	t.Cleanup(audio.ClearAllInsertEffects)

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	g2 := New(logger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(640, 480)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	row := g2.drum.Rows[0]
	if len(audio.GetInsertEffects(row.Instrument)) != 2 {
		t.Fatalf("Effects round-trip: got %d want 2", len(audio.GetInsertEffects(row.Instrument)))
	}
	if audio.GetInsertEffects(row.Instrument)[0].Type != "chorus" || audio.GetInsertEffects(row.Instrument)[0].Params["rate"] != 0.5 {
		t.Errorf("Effects[0] round-trip: %+v", audio.GetInsertEffects(row.Instrument)[0])
	}
	if audio.GetInsertEffects(row.Instrument)[1].Type != "bitcrusher" || audio.GetInsertEffects(row.Instrument)[1].Params["bits"] != 8 {
		t.Errorf("Effects[1] round-trip: %+v", audio.GetInsertEffects(row.Instrument)[1])
	}
}

// --- Node Params Round-Trip Tests ---

func TestExportImportNodeParamsRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	g.graph.SetNodeParams(ui.ID, model.NodeParams{
		Volume:     0.7,
		Pitch:      3,
		Duration:   1.5,
		GrooveKind: model.GrooveDelay,
		GroovePct:  0.3,
	})

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	g2 := New(logger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(640, 480)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, n := range g2.graph.Nodes {
		if n.Type == model.NodeTypeRegular {
			if n.Params.Volume != 0.7 {
				t.Errorf("Volume: got %f want 0.7", n.Params.Volume)
			}
			if n.Params.Pitch != 3 {
				t.Errorf("Pitch: got %f want 3", n.Params.Pitch)
			}
			if n.Params.Duration != 1.5 {
				t.Errorf("Duration: got %f want 1.5", n.Params.Duration)
			}
			if n.Params.GrooveKind != model.GrooveDelay {
				t.Errorf("GrooveKind: got %q want delay", n.Params.GrooveKind)
			}
			if n.Params.GroovePct != 0.3 {
				t.Errorf("GroovePct: got %f want 0.3", n.Params.GroovePct)
			}
			return
		}
	}
	t.Fatal("no regular node found")
}

func TestExportImportEffectOverridesRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	overrides := []model.EffectOverride{
		{Type: "distortion", Param: "drive", Value: 0.8},
		{Type: "filter", Param: "cutoff", Value: 2000},
	}
	g.graph.SetNodeParams(ui.ID, model.NodeParams{
		Volume:          1,
		Duration:        1,
		EffectOverrides: overrides,
	})

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	g2 := New(logger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(640, 480)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, n := range g2.graph.Nodes {
		if n.Type == model.NodeTypeRegular {
			if len(n.Params.EffectOverrides) != 2 {
				t.Fatalf("EffectOverrides: got %d want 2", len(n.Params.EffectOverrides))
			}
			if n.Params.EffectOverrides[0].Type != "distortion" || n.Params.EffectOverrides[0].Value != 0.8 {
				t.Errorf("EffectOverrides[0]: %+v", n.Params.EffectOverrides[0])
			}
			if n.Params.EffectOverrides[1].Type != "filter" || n.Params.EffectOverrides[1].Value != 2000 {
				t.Errorf("EffectOverrides[1]: %+v", n.Params.EffectOverrides[1])
			}
			return
		}
	}
	t.Fatal("no regular node found")
}

// --- Master Volume Tests ---

func TestExportIncludesMasterVolume(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	prevVol := audio.MainVolume()
	audio.SetMainVolume(0.65)
	t.Cleanup(func() { audio.SetMainVolume(prevVol) })

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	if f.MasterVolume == nil || *f.MasterVolume != 0.65 {
		t.Errorf("MasterVolume: got %v want 0.65", f.MasterVolume)
	}
}

// Always written: sessions start at 0.5 headroom, so omitting a "default" bakes a stale value into the file.
func TestExportAlwaysWritesMasterVolume(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	prevVol := audio.MainVolume()
	audio.SetMainVolume(1)
	t.Cleanup(func() { audio.SetMainVolume(prevVol) })

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if !contains(string(data), `"master_volume"`) {
		t.Error("expected master_volume ALWAYS present in exported JSON (files must be self-describing)")
	}
}

// A pointer, so a muted master (0) is not dropped by omitempty and re-imported at full volume.
func TestMasterVolumeZeroRoundTrips(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	prevVol := audio.MainVolume()
	audio.SetMainVolume(0)
	t.Cleanup(func() { audio.SetMainVolume(prevVol) })

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	g2 := New(testLogger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(640, 480)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	if got := audio.MainVolume(); got != 0 {
		t.Errorf("muted master round-trip: got %f want 0 (came back at full volume)", got)
	}
}

func TestImportAppliesMasterVolume(t *testing.T) {
	assertDefaultParityState(t)
	prevVol := audio.MainVolume()
	t.Cleanup(func() { audio.SetMainVolume(prevVol) })

	mv := 0.4
	file := projectFile{
		Envelope:     schema.Current(),
		Subdiv:       32,
		BPM:          120,
		MasterVolume: &mv,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: roster.Kick, Kind: kindBuiltin, Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: model.NodeTypeRegular}},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := audio.MainVolume()
	if got != 0.4 {
		t.Errorf("MainVolume: got %f want 0.4", got)
	}
}

func TestExportImportMasterVolumeRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	prevVol := audio.MainVolume()
	audio.SetMainVolume(0.55)
	t.Cleanup(func() { audio.SetMainVolume(prevVol) })

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	// Reset to different value before import
	audio.SetMainVolume(1)

	g2 := New(logger)
	t.Cleanup(g2.CloseForTest)
	g2.Layout(640, 480)
	if err := g2.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	got := audio.MainVolume()
	if got != 0.55 {
		t.Errorf("MasterVolume round-trip: got %f want 0.55", got)
	}
}

// --- Helpers ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
