package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The predictor schedule for a circuit using every logic kind over a deep horizon; the browser test asserts WASM
// reproduces it byte for byte, probability rolls included. No large-windowStart probe: a static UI predictor
// window cannot slide there, and that path is pure integer logic proven natively.

type nodeRuleRowData struct {
	Row       int    `json:"row"`
	Visible   []bool `json:"visible"`
	Audible   []bool `json:"audible"`
	Triggered []bool `json:"triggered"`
}

type nodeRuleParityGolden struct {
	Fixture string            `json:"fixture"`
	Rows    int               `json:"rows"`
	Horizon int               `json:"horizon"`
	Small   []nodeRuleRowData `json:"small"`
}

const nodeRuleHorizon = 1024 // deep — many loop iterations; within windowCap (4096)

func sampleRows(g *Game, base, n int) []nodeRuleRowData {
	g.engine.Predictor.Ensure(base + n)
	nRows := len(g.drum.Rows)
	out := make([]nodeRuleRowData, 0, nRows)
	for row := 0; row < nRows; row++ {
		rd := nodeRuleRowData{
			Row:       row,
			Visible:   make([]bool, n),
			Audible:   make([]bool, n),
			Triggered: make([]bool, n),
		}
		for k := 0; k < n; k++ {
			idx := base + k
			rd.Visible[k] = g.engine.Predictor.VisibleAt(row, idx)
			rd.Audible[k] = g.engine.Predictor.AudibleAt(row, idx)
			rd.Triggered[k] = g.engine.Predictor.TriggeredAt(row, idx)
		}
		out = append(out, rd)
	}
	return out
}

func TestNodeRuleParityGolden(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "assets", "parity_fixture_node_rules.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if err := g.Import(data); err != nil {
		t.Fatalf("import fixture: %v", err)
	}

	small := sampleRows(g, 0, nodeRuleHorizon)

	golden := nodeRuleParityGolden{
		Fixture: "parity_fixture_node_rules.json",
		Rows:    len(g.drum.Rows),
		Horizon: nodeRuleHorizon,
		Small:   small,
	}

	// Otherwise the parity check is vacuous.
	anyAudible, anyGated := false, false
	for _, rd := range small {
		for k := 0; k < len(rd.Audible); k++ {
			if rd.Audible[k] {
				anyAudible = true
			}
			if rd.Triggered[k] && !rd.Audible[k] {
				anyGated = true
			}
		}
	}
	if !anyAudible {
		t.Fatal("fixture produced no audible hits; parity check would be vacuous")
	}
	_ = anyGated // gating presence is logic-dependent; audible activity is the hard requirement

	out, err := json.MarshalIndent(golden, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatalf("mkdir testdata: %v", err)
	}
	path := filepath.Join("testdata", "parity_golden_node_rules.json")
	if err := os.WriteFile(path, out, 0644); err != nil {
		t.Fatalf("write golden: %v", err)
	}
	t.Logf("wrote %s (%d bytes, rows=%d)", path, len(out), golden.Rows)
}
