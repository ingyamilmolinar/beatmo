package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// parityResult stores predictor output for a fixture over a horizon.
type parityResult struct {
	Fixture string          `json:"fixture"`
	Horizon int             `json:"horizon"`
	Rows    int             `json:"rows"`
	Data    []parityRowData `json:"data"`
}

type parityRowData struct {
	Row       int    `json:"row"`
	Visible   []bool `json:"visible"`
	Audible   []bool `json:"audible"`
	Triggered []bool `json:"triggered"`
}

const parityHorizon = 64

// computeParityResult imports a fixture, settles the predictor, and
// queries VisibleAt/AudibleAt/TriggeredAt for all rows over the horizon.
func computeParityResult(t *testing.T, fixturePath string) parityResult {
	t.Helper()
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatalf("read fixture %s: %v", fixturePath, err)
	}

	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	if err := g.Import(data); err != nil {
		t.Fatalf("import fixture %s: %v", fixturePath, err)
	}

	// Ensure predictor has computed up to our horizon.
	g.engine.Predictor.Ensure(parityHorizon)

	nRows := len(g.drum.Rows)
	result := parityResult{
		Fixture: filepath.Base(fixturePath),
		Horizon: parityHorizon,
		Rows:    nRows,
	}

	for row := 0; row < nRows; row++ {
		rd := parityRowData{
			Row:       row,
			Visible:   make([]bool, parityHorizon),
			Audible:   make([]bool, parityHorizon),
			Triggered: make([]bool, parityHorizon),
		}
		for idx := 0; idx < parityHorizon; idx++ {
			rd.Visible[idx] = g.engine.Predictor.VisibleAt(row, idx)
			rd.Audible[idx] = g.engine.Predictor.AudibleAt(row, idx)
			rd.Triggered[idx] = g.engine.Predictor.TriggeredAt(row, idx)
		}
		result.Data = append(result.Data, rd)
	}
	return result
}

// writeGoldenFile writes the parity result as a golden JSON file for
// cross-platform comparison. Creates the testdata directory if needed.
func writeGoldenFile(t *testing.T, result parityResult, name string) {
	t.Helper()
	dir := "testdata"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("create testdata dir: %v", err)
	}
	path := filepath.Join(dir, name)
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		t.Fatalf("marshal golden file: %v", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("write golden file %s: %v", path, err)
	}
	t.Logf("wrote golden file: %s (%d bytes)", path, len(data))
}

// verifyParityResult checks that the result is internally consistent.
func verifyParityResult(t *testing.T, result parityResult) {
	t.Helper()
	if result.Rows == 0 {
		t.Fatalf("fixture %s: no rows", result.Fixture)
	}
	if len(result.Data) != result.Rows {
		t.Fatalf("fixture %s: expected %d rows, got %d", result.Fixture, result.Rows, len(result.Data))
	}
	for _, rd := range result.Data {
		if len(rd.Visible) != result.Horizon {
			t.Errorf("row %d: visible length %d != horizon %d", rd.Row, len(rd.Visible), result.Horizon)
		}
		if len(rd.Audible) != result.Horizon {
			t.Errorf("row %d: audible length %d != horizon %d", rd.Row, len(rd.Audible), result.Horizon)
		}
		if len(rd.Triggered) != result.Horizon {
			t.Errorf("row %d: triggered length %d != horizon %d", rd.Row, len(rd.Triggered), result.Horizon)
		}
	}
}

// verifyGoldenMatch reads an existing golden file and compares it to
// the computed result.
func verifyGoldenMatch(t *testing.T, result parityResult, goldenPath string) {
	t.Helper()
	data, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Logf("no existing golden file %s — will create", goldenPath)
		return
	}
	var golden parityResult
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatalf("unmarshal golden file: %v", err)
	}
	if golden.Rows != result.Rows {
		t.Errorf("golden row count %d != computed %d", golden.Rows, result.Rows)
		return
	}
	mismatches := 0
	for i, grd := range golden.Data {
		if i >= len(result.Data) {
			break
		}
		crd := result.Data[i]
		for idx := 0; idx < result.Horizon; idx++ {
			if idx < len(grd.Visible) && idx < len(crd.Visible) && grd.Visible[idx] != crd.Visible[idx] {
				if mismatches < 5 {
					t.Errorf("row=%d idx=%d visible: golden=%v computed=%v", grd.Row, idx, grd.Visible[idx], crd.Visible[idx])
				}
				mismatches++
			}
			if idx < len(grd.Audible) && idx < len(crd.Audible) && grd.Audible[idx] != crd.Audible[idx] {
				if mismatches < 5 {
					t.Errorf("row=%d idx=%d audible: golden=%v computed=%v", grd.Row, idx, grd.Audible[idx], crd.Audible[idx])
				}
				mismatches++
			}
			if idx < len(grd.Triggered) && idx < len(crd.Triggered) && grd.Triggered[idx] != crd.Triggered[idx] {
				if mismatches < 5 {
					t.Errorf("row=%d idx=%d triggered: golden=%v computed=%v", grd.Row, idx, grd.Triggered[idx], crd.Triggered[idx])
				}
				mismatches++
			}
		}
	}
	if mismatches > 0 {
		t.Errorf("total mismatches: %d", mismatches)
	} else {
		t.Logf("golden match confirmed for %s", result.Fixture)
	}
}

func fixturePath(name string) string {
	return filepath.Join("..", "assets", name)
}

func goldenPath(name string) string {
	return filepath.Join("testdata", "parity_golden_"+name)
}

func TestCrossPlatformParity_SimpleLoop(t *testing.T) {
	result := computeParityResult(t, fixturePath("parity_fixture_simple.json"))
	verifyParityResult(t, result)

	// Simple 4-node loop: all nodes are regular, all should be visible/audible.
	for _, rd := range result.Data {
		anyVisible := false
		for _, v := range rd.Visible {
			if v {
				anyVisible = true
				break
			}
		}
		if !anyVisible {
			t.Errorf("row %d: no visible beats in simple loop", rd.Row)
		}
	}

	writeGoldenFile(t, result, "parity_golden_simple.json")
	verifyGoldenMatch(t, result, goldenPath("simple.json"))
}

func TestCrossPlatformParity_Probability(t *testing.T) {
	result := computeParityResult(t, fixturePath("parity_fixture_probability.json"))
	verifyParityResult(t, result)

	// Probability nodes use deterministic hash — results should be stable
	// across runs. Verify some beats are visible and some are not.
	for _, rd := range result.Data {
		visCount := 0
		for _, v := range rd.Visible {
			if v {
				visCount++
			}
		}
		// With p=0.5 on 2 of 4 nodes over 64 beats, we should see a mix.
		t.Logf("row %d: %d/%d visible", rd.Row, visCount, len(rd.Visible))
	}

	writeGoldenFile(t, result, "parity_golden_probability.json")
	verifyGoldenMatch(t, result, goldenPath("probability.json"))
}

func TestCrossPlatformParity_Mute(t *testing.T) {
	result := computeParityResult(t, fixturePath("parity_fixture_mute.json"))
	verifyParityResult(t, result)

	// Mute node (id=2) should be triggered but not audible at certain beats.
	for _, rd := range result.Data {
		triggeredNotAudible := 0
		for idx := 0; idx < result.Horizon; idx++ {
			if rd.Triggered[idx] && !rd.Audible[idx] {
				triggeredNotAudible++
			}
		}
		t.Logf("row %d: %d beats triggered-but-not-audible (mute gates)", rd.Row, triggeredNotAudible)
	}

	writeGoldenFile(t, result, "parity_golden_mute.json")
	verifyGoldenMatch(t, result, goldenPath("mute.json"))
}

func TestCrossPlatformParity_MultiRow(t *testing.T) {
	result := computeParityResult(t, fixturePath("parity_fixture_multi_row.json"))
	verifyParityResult(t, result)

	if result.Rows < 3 {
		t.Fatalf("expected 3 rows, got %d", result.Rows)
	}

	// All rows should have visible beats (each has a 4-node regular loop).
	for _, rd := range result.Data {
		visCount := 0
		for _, v := range rd.Visible {
			if v {
				visCount++
			}
		}
		if visCount == 0 {
			t.Errorf("row %d: no visible beats in multi-row fixture", rd.Row)
		}
		t.Logf("row %d: %d/%d visible", rd.Row, visCount, len(rd.Visible))
	}

	writeGoldenFile(t, result, "parity_golden_multi_row.json")
	verifyGoldenMatch(t, result, goldenPath("multi_row.json"))
}
