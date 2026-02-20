package ui

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestImportMalformedJSON verifies that garbage bytes produce an error.
func TestImportMalformedJSON(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	if err := g.Import([]byte(`{not valid json!!!`)); err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

// TestImportEmptyNodes verifies that a valid file with no nodes/instruments succeeds.
func TestImportEmptyNodes(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	f := tFile{
		Version:     1,
		Subdiv:      32,
		BPM:         100,
		Nodes:       []tNode{},
		Instruments: []tInst{},
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("import should succeed with empty nodes: %v", err)
	}
	if len(g.drum.Rows) != 0 {
		t.Fatalf("expected 0 rows after empty import, got %d", len(g.drum.Rows))
	}
	if len(g.nodes) != 0 {
		t.Fatalf("expected 0 nodes after empty import, got %d", len(g.nodes))
	}
}

// TestImportTooManyNodes verifies that exceeding the node limit returns an error.
func TestImportTooManyNodes(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	nodes := make([]tNode, maxImportNodes+1)
	for i := range nodes {
		nodes[i] = tNode{ID: i, I: i, J: 0, Type: "regular"}
	}
	f := tFile{
		Version:     1,
		Subdiv:      32,
		BPM:         120,
		Nodes:       nodes,
		Instruments: []tInst{},
	}
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := g.Import(data); err == nil {
		t.Fatal("expected error for too many nodes, got nil")
	}
}

// TestImportInvalidSubdiv verifies that an invalid subdivision (not 4/8/16/32) normalizes to 32.
func TestImportInvalidSubdiv(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	f := tFile{
		Version:     1,
		Subdiv:      7, // invalid
		BPM:         100,
		Nodes:       []tNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
		Instruments: []tInst{{Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#C87850FF"}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if got := g.grid.MaxDiv(); got != 32 {
		t.Fatalf("expected subdiv normalized to 32, got %d", got)
	}
}

// TestImportMissingSubdiv verifies that subdiv=0 defaults to 32.
func TestImportMissingSubdiv(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	f := tFile{
		Version:     1,
		Subdiv:      0, // missing/zero
		BPM:         100,
		Nodes:       []tNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
		Instruments: []tInst{{Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#C87850FF"}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if got := g.grid.MaxDiv(); got != 32 {
		t.Fatalf("expected subdiv defaulted to 32, got %d", got)
	}
}

// TestImportInstrumentVolumePreserved verifies that per-instrument volume survives import.
func TestImportInstrumentVolumePreserved(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	f := tFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Nodes:   []tNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
		Instruments: []tInst{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 0.7, Origin: 1, Color: "#C87850FF",
		}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if len(g.drum.Rows) == 0 {
		t.Fatal("expected at least one row")
	}
	if math.Abs(g.drum.Rows[0].Volume-0.7) > 1e-9 {
		t.Fatalf("expected row volume 0.7, got %f", g.drum.Rows[0].Volume)
	}
}

// TestImportMasterVolumeApplied verifies that master_volume is applied on import.
func TestImportMasterVolumeApplied(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build a payload that includes master_volume using a custom struct that
	// extends tFile with the field.
	type tFileMV struct {
		Version      int     `json:"version"`
		Subdiv       int     `json:"subdiv"`
		BPM          int     `json:"bpm"`
		MasterVolume float64 `json:"master_volume,omitempty"`
		Instruments  []tInst `json:"instruments"`
		Nodes        []tNode `json:"nodes"`
	}
	f := tFileMV{
		Version:      1,
		Subdiv:       32,
		BPM:          120,
		MasterVolume: 0.5,
		Nodes:        []tNode{{ID: 1, I: 0, J: 0, Type: "regular"}},
		Instruments:  []tInst{{Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF"}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	got := audio.MainVolume()
	if math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("expected master volume 0.5, got %f", got)
	}
	// Also check slider state if available.
	if g.drum.mainVolSlider != nil {
		if math.Abs(g.drum.mainVolSlider.Value-0.5) > 1e-9 {
			t.Fatalf("expected mainVolSlider.Value 0.5, got %f", g.drum.mainVolSlider.Value)
		}
	}
}

// TestImportEQFieldParsing verifies that master EQ gains survive import.
func TestImportEQFieldParsing(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	type tEQ struct {
		GainsDB []float64 `json:"gains_db"`
	}
	type tFileEQ struct {
		Version     int     `json:"version"`
		Subdiv      int     `json:"subdiv"`
		BPM         int     `json:"bpm"`
		Instruments []tInst `json:"instruments"`
		Nodes       []tNode `json:"nodes"`
		EQ          *tEQ    `json:"eq,omitempty"`
	}

	// 10 bands matching eqBandDefs length
	gains := []float64{-6, -3, 0, 3, 6, 9, 12, -12, -9, -6}
	f := tFileEQ{
		Version:     1,
		Subdiv:      32,
		BPM:         100,
		Nodes:       []tNode{},
		Instruments: []tInst{},
		EQ:          &tEQ{GainsDB: gains},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if len(g.drum.eqBandGainsDB) != len(eqBandDefs) {
		t.Fatalf("expected %d EQ bands, got %d", len(eqBandDefs), len(g.drum.eqBandGainsDB))
	}
	for i, want := range gains {
		if i >= len(g.drum.eqBandGainsDB) {
			break
		}
		if math.Abs(g.drum.eqBandGainsDB[i]-want) > 1e-9 {
			t.Errorf("EQ band %d: want %.1f, got %.1f", i, want, g.drum.eqBandGainsDB[i])
		}
	}
}

// TestImportNodeParamsRoundTrip verifies that per-node Volume, Pitch, and Duration survive import.
func TestImportNodeParamsRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	f := tFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Nodes: []tNode{{
			ID: 1, I: 0, J: 0, Type: "regular",
			Volume: 0.8, Pitch: 3.0, Duration: 0.5,
		}},
		Instruments: []tInst{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}
	// Find the imported node in the graph.
	var found bool
	for _, n := range g.nodes {
		mn, ok := g.graph.GetNodeByID(n.ID)
		if !ok {
			continue
		}
		found = true
		if math.Abs(mn.Params.Volume-0.8) > 1e-9 {
			t.Errorf("expected volume 0.8, got %f", mn.Params.Volume)
		}
		if math.Abs(mn.Params.Pitch-3.0) > 1e-9 {
			t.Errorf("expected pitch 3.0, got %f", mn.Params.Pitch)
		}
		if math.Abs(mn.Params.Duration-0.5) > 1e-9 {
			t.Errorf("expected duration 0.5, got %f", mn.Params.Duration)
		}
		break
	}
	if !found {
		t.Fatal("imported node not found in graph")
	}
}

// TestImportLogicKindAliases verifies that legacy logic kind names are mapped to canonical forms.
func TestImportLogicKindAliases(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	f := tFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Nodes: []tNode{
			{ID: 1, I: 0, J: 0, Type: "regular", LogicKind: "prev_fired", LogicN: 2},
			{ID: 2, I: 4, J: 0, Type: "regular", LogicKind: "every_n_loops", LogicN: 3},
		},
		Instruments: []tInst{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF",
		}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// Collect all imported nodes and check their logic kinds.
	type want struct {
		logicKind string
	}
	// We cannot rely on the file IDs mapping 1:1 to graph IDs, so scan all nodes.
	wantKinds := map[string]bool{
		"trigger_if_prev_triggered": false,
		"every_n_triggers":          false,
	}
	for _, n := range g.nodes {
		mn, ok := g.graph.GetNodeByID(n.ID)
		if !ok {
			continue
		}
		if mn.Params.LogicKind != "" {
			wantKinds[mn.Params.LogicKind] = true
		}
	}
	if !wantKinds["trigger_if_prev_triggered"] {
		t.Error("expected prev_fired to map to trigger_if_prev_triggered")
	}
	if !wantKinds["every_n_triggers"] {
		t.Error("expected every_n_loops to map to every_n_triggers")
	}
}

// TestResolveSamplePathEmptyBase verifies that trimming "sample-" to empty yields empty string.
func TestResolveSamplePathEmptyBase(t *testing.T) {
	got := resolveSamplePath("sample-")
	if got != "" {
		t.Fatalf("expected empty string for 'sample-', got %q", got)
	}
}

// TestResolveSamplePathCatalogDirect verifies that a nonexistent catalog ID returns empty string.
func TestResolveSamplePathCatalogDirect(t *testing.T) {
	got := resolveSamplePath("nonexistent-instrument-xyz")
	if got != "" {
		t.Fatalf("expected empty string for nonexistent catalog entry, got %q", got)
	}
}

// TestResolveSamplePathNoMatch verifies that a sample- prefixed ID with no filesystem or catalog match returns empty.
func TestResolveSamplePathNoMatch(t *testing.T) {
	got := resolveSamplePath("sample-totally-fake-xyz-123")
	if got != "" {
		t.Fatalf("expected empty string for non-matching sample path, got %q", got)
	}
}

// TestImportNodeTypeRoundTrip verifies that mute and silent node types survive import.
func TestImportNodeTypeRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	f := tFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Nodes: []tNode{
			{ID: 1, I: 0, J: 0, Type: "mute"},
			{ID: 2, I: 4, J: 0, Type: "silent"},
			{ID: 3, I: 8, J: 0, Type: "regular"},
		},
		Instruments: []tInst{{
			Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#C87850FF",
		}},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	foundMute := false
	foundSilent := false
	foundRegular := false
	for _, n := range g.nodes {
		mn, ok := g.graph.GetNodeByID(n.ID)
		if !ok {
			continue
		}
		switch mn.Type {
		case model.NodeTypeMute:
			foundMute = true
		case model.NodeTypeSilent:
			foundSilent = true
		case model.NodeTypeRegular:
			foundRegular = true
		}
	}
	if !foundMute {
		t.Error("expected a mute node after import")
	}
	if !foundSilent {
		t.Error("expected a silent node after import")
	}
	if !foundRegular {
		t.Error("expected a regular node after import")
	}
}

// TestImportGrooveParams verifies that groove_kind and groove_pct survive import.
func TestImportGrooveParams(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Use raw JSON map because tNode does not have groove fields.
	raw := map[string]interface{}{
		"version": 1, "subdiv": 32, "bpm": 120,
		"nodes": []map[string]interface{}{{
			"id": 1, "i": 0, "j": 0, "type": "regular",
			"groove_kind": "delay", "groove_pct": 0.5,
		}},
		"instruments": []map[string]interface{}{{
			"name": "Kick", "id": "kick", "kind": "builtin",
			"volume": 1, "origin": 1, "color": "#C87850FF",
		}},
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	var found bool
	for _, n := range g.nodes {
		mn, ok := g.graph.GetNodeByID(n.ID)
		if !ok {
			continue
		}
		found = true
		if mn.Params.GrooveKind != "delay" {
			t.Errorf("expected groove_kind 'delay', got %q", mn.Params.GrooveKind)
		}
		if math.Abs(mn.Params.GroovePct-0.5) > 1e-9 {
			t.Errorf("expected groove_pct 0.5, got %f", mn.Params.GroovePct)
		}
		break
	}
	if !found {
		t.Fatal("imported node not found in graph")
	}
}
