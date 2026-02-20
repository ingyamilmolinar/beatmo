package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

// --- Pan & Send Tests ---

func TestExportIncludesPanAndSends(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	g.drum.Rows[0].Pan = -0.5
	g.drum.Rows[0].DelaySend = 0.3
	g.drum.Rows[0].ReverbSend = 0.7

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(f.Instruments) == 0 {
		t.Fatal("no instruments")
	}
	inst := f.Instruments[0]
	if inst.Pan != -0.5 {
		t.Errorf("Pan: got %f want -0.5", inst.Pan)
	}
	if inst.DelaySend != 0.3 {
		t.Errorf("DelaySend: got %f want 0.3", inst.DelaySend)
	}
	if inst.ReverbSend != 0.7 {
		t.Errorf("ReverbSend: got %f want 0.7", inst.ReverbSend)
	}
}

func TestExportOmitsPanAndSendsWhenDefault(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	// Leave pan/sends at zero defaults

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	// Check raw JSON doesn't contain pan/send keys
	raw := string(data)
	for _, key := range []string{`"pan"`, `"delay_send"`, `"reverb_send"`} {
		if contains(raw, key) {
			t.Errorf("expected %s omitted from JSON when default, got: %s", key, raw)
		}
	}
}

func TestImportAppliesPanAndSends(t *testing.T) {
	assertDefaultParityState(t)
	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
			Pan: 0.8, DelaySend: 0.4, ReverbSend: 0.6,
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	if len(g.drum.Rows) == 0 {
		t.Fatal("no rows")
	}
	row := g.drum.Rows[0]
	if row.Pan != 0.8 {
		t.Errorf("Pan: got %f want 0.8", row.Pan)
	}
	if row.DelaySend != 0.4 {
		t.Errorf("DelaySend: got %f want 0.4", row.DelaySend)
	}
	if row.ReverbSend != 0.6 {
		t.Errorf("ReverbSend: got %f want 0.6", row.ReverbSend)
	}
	// Verify audio API was called
	if audio.ChannelPan("kick") != 0.8 {
		t.Errorf("audio.ChannelPan: got %f want 0.8", audio.ChannelPan("kick"))
	}
	if audio.DelaySend("kick") != 0.4 {
		t.Errorf("audio.DelaySend: got %f want 0.4", audio.DelaySend("kick"))
	}
	if audio.ReverbSend("kick") != 0.6 {
		t.Errorf("audio.ReverbSend: got %f want 0.6", audio.ReverbSend("kick"))
	}
}

func TestImportClampsPanAndSends(t *testing.T) {
	assertDefaultParityState(t)
	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
			Pan: -5, DelaySend: 2.0, ReverbSend: -1,
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	row := g.drum.Rows[0]
	if row.Pan != -1 {
		t.Errorf("Pan clamped: got %f want -1", row.Pan)
	}
	if row.DelaySend != 1 {
		t.Errorf("DelaySend clamped: got %f want 1", row.DelaySend)
	}
	if row.ReverbSend != 0 {
		t.Errorf("ReverbSend clamped: got %f want 0", row.ReverbSend)
	}
}

func TestImportLegacyNoPanSends(t *testing.T) {
	assertDefaultParityState(t)
	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
			// No pan/send fields
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	row := g.drum.Rows[0]
	if row.Pan != 0 {
		t.Errorf("Pan: got %f want 0", row.Pan)
	}
	if row.DelaySend != 0 {
		t.Errorf("DelaySend: got %f want 0", row.DelaySend)
	}
	if row.ReverbSend != 0 {
		t.Errorf("ReverbSend: got %f want 0", row.ReverbSend)
	}
}

func TestExportImportPanSendsRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui
	g.drum.Rows[0].Pan = -0.75
	g.drum.Rows[0].DelaySend = 0.5
	g.drum.Rows[0].ReverbSend = 0.25

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
	if row.Pan != -0.75 {
		t.Errorf("Pan round-trip: got %f want -0.75", row.Pan)
	}
	if row.DelaySend != 0.5 {
		t.Errorf("DelaySend round-trip: got %f want 0.5", row.DelaySend)
	}
	if row.ReverbSend != 0.25 {
		t.Errorf("ReverbSend round-trip: got %f want 0.25", row.ReverbSend)
	}
}

// --- Synth Params Tests ---

func TestExportIncludesSynthParams(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(nil, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	ui := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.drum.Rows[0].Origin = ui.ID
	g.drum.Rows[0].Node = ui

	g.graph.SetNodeParams(ui.ID, model.NodeParams{
		Volume:          1,
		Duration:        1,
		SynthDecay:      0.5,
		SynthTone:       -0.3,
		SynthAttack:     0.1,
		SynthDrive:      0.6,
		SynthBody:       0.4,
		SynthColor:      -0.5,
		SynthBrightness: 0.9,
	})

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	if len(f.Nodes) == 0 {
		t.Fatal("no nodes")
	}
	n := f.Nodes[0]
	if n.SynthDecay != 0.5 {
		t.Errorf("SynthDecay: got %f want 0.5", n.SynthDecay)
	}
	if n.SynthTone != -0.3 {
		t.Errorf("SynthTone: got %f want -0.3", n.SynthTone)
	}
	if n.SynthDrive != 0.6 {
		t.Errorf("SynthDrive: got %f want 0.6", n.SynthDrive)
	}
	if n.SynthBrightness != 0.9 {
		t.Errorf("SynthBrightness: got %f want 0.9", n.SynthBrightness)
	}
}

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
	} {
		if contains(raw, key) {
			t.Errorf("expected %s omitted from JSON when default", key)
		}
	}
}

func TestImportAppliesSynthParams(t *testing.T) {
	assertDefaultParityState(t)
	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{
			ID: 1, I: 0, J: 0, Type: "regular",
			SynthDecay: 0.5, SynthTone: -0.3, SynthAttack: 0.1,
			SynthDrive: 0.6, SynthBody: 0.4, SynthColor: -0.5,
			SynthBrightness: 0.9,
		}},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Find the imported node
	var found bool
	for _, n := range g.graph.Nodes {
		if n.Type == model.NodeTypeRegular {
			if n.Params.SynthDecay != 0.5 {
				t.Errorf("SynthDecay: got %f want 0.5", n.Params.SynthDecay)
			}
			if n.Params.SynthTone != -0.3 {
				t.Errorf("SynthTone: got %f want -0.3", n.Params.SynthTone)
			}
			if n.Params.SynthDrive != 0.6 {
				t.Errorf("SynthDrive: got %f want 0.6", n.Params.SynthDrive)
			}
			if n.Params.SynthBrightness != 0.9 {
				t.Errorf("SynthBrightness: got %f want 0.9", n.Params.SynthBrightness)
			}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("imported node not found")
	}
}

func TestImportClampsSynthParams(t *testing.T) {
	assertDefaultParityState(t)
	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{
			ID: 1, I: 0, J: 0, Type: "regular",
			SynthTone: 99, SynthDrive: -5, SynthBody: 10,
			SynthColor: -99, SynthBrightness: 50,
		}},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	for _, n := range g.graph.Nodes {
		if n.Type == model.NodeTypeRegular {
			if n.Params.SynthTone != 10 {
				t.Errorf("SynthTone clamped: got %f want 10", n.Params.SynthTone)
			}
			if n.Params.SynthDrive != 0 {
				t.Errorf("SynthDrive clamped: got %f want 0", n.Params.SynthDrive)
			}
			if n.Params.SynthBody != 1 {
				t.Errorf("SynthBody clamped: got %f want 1", n.Params.SynthBody)
			}
			if n.Params.SynthColor != -1 {
				t.Errorf("SynthColor clamped: got %f want -1", n.Params.SynthColor)
			}
			if n.Params.SynthBrightness != 1 {
				t.Errorf("SynthBrightness clamped: got %f want 1", n.Params.SynthBrightness)
			}
			return
		}
	}
	t.Fatal("no regular node found")
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
	g.drum.Rows[0].Effects = []audio.EffectSlot{
		{Type: "distortion", Enabled: true, Params: map[string]float64{"drive": 0.5}},
		{Type: "delay", Enabled: false, Params: map[string]float64{"time": 0.3, "feedback": 0.4}},
	}

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f exportFile
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
	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
			Effects: effects,
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	row := g.drum.Rows[0]
	if len(row.Effects) != 1 {
		t.Fatalf("Effects: got %d want 1", len(row.Effects))
	}
	if row.Effects[0].Type != "reverb" || !row.Effects[0].Enabled {
		t.Errorf("Effects[0]: %+v", row.Effects[0])
	}
	// Verify audio.SetInsertEffects was called
	got := audio.GetInsertEffects("kick")
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
	g.drum.Rows[0].Effects = []audio.EffectSlot{
		{Type: "chorus", Enabled: true, Params: map[string]float64{"rate": 0.5, "depth": 0.3}},
		{Type: "bitcrusher", Enabled: true, Params: map[string]float64{"bits": 8}},
	}

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
	if len(row.Effects) != 2 {
		t.Fatalf("Effects round-trip: got %d want 2", len(row.Effects))
	}
	if row.Effects[0].Type != "chorus" || row.Effects[0].Params["rate"] != 0.5 {
		t.Errorf("Effects[0] round-trip: %+v", row.Effects[0])
	}
	if row.Effects[1].Type != "bitcrusher" || row.Effects[1].Params["bits"] != 8 {
		t.Errorf("Effects[1] round-trip: %+v", row.Effects[1])
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
		GrooveKind: "delay",
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
			if n.Params.GrooveKind != "delay" {
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
	var f exportFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("json: %v", err)
	}
	if f.MasterVolume != 0.65 {
		t.Errorf("MasterVolume: got %f want 0.65", f.MasterVolume)
	}
}

func TestExportOmitsMasterVolumeWhenDefault(t *testing.T) {
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
	if contains(string(data), `"master_volume"`) {
		t.Error("expected master_volume omitted from JSON when default (1.0)")
	}
}

func TestImportAppliesMasterVolume(t *testing.T) {
	assertDefaultParityState(t)
	prevVol := audio.MainVolume()
	t.Cleanup(func() { audio.SetMainVolume(prevVol) })

	file := exportFile{
		Version:      1,
		Subdiv:       32,
		BPM:          120,
		MasterVolume: 0.4,
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
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

func TestImportLegacyNoMasterVolume(t *testing.T) {
	assertDefaultParityState(t)
	prevVol := audio.MainVolume()
	audio.SetMainVolume(0.3) // set non-default before import
	t.Cleanup(func() { audio.SetMainVolume(prevVol) })

	file := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		// No MasterVolume field
		Instruments: []exportInstrument{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
		Nodes: []exportNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
	}
	data, _ := json.Marshal(file)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Legacy files without master_volume should default to 1.0
	got := audio.MainVolume()
	if got != 1 {
		t.Errorf("MainVolume: got %f want 1 (default for legacy)", got)
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
