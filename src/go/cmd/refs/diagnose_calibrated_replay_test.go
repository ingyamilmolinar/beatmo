//go:build !js

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

// The Kind-level replay never applies Descriptor.Drift/Mismatch and the acceptance tests read frozen verdicts, so
// this replays the frozen pairs through d.CompareWith: rows must match the golden, and every listener-named axis must
// still read non-ok. Regenerate: CALIBRATED_REPLAY_UPDATE=1 go test -tags test ./cmd/refs/ -run TestCalibratedComparators

const calibratedReplayGolden = "testdata/diagnose_calibrated_verdicts_2026-09-04.json"

// calibratedRow is one replayed (instrument, axis) verdict.
type calibratedRow struct {
	Instrument string  `json:"instrument"`
	AxisID     string  `json:"id"`
	Axis       string  `json:"axis"`
	Verdict    string  `json:"verdict"`
	Direction  string  `json:"direction"`
	Drift      float64 `json:"drift"`
	Mismatch   float64 `json:"mismatch"`
}

// Ring-T60 escalation rows are skipped: they need extrapolation flags and decay rates the baseline does not carry.
func replayCalibrated(t *testing.T) []calibratedRow {
	t.Helper()
	raw, err := os.ReadFile("testdata/diagnose_baseline_2026-08-22.json")
	if err != nil {
		t.Fatalf("read frozen baseline: %v", err)
	}
	var baseline map[string]instrumentDiagnosis
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatalf("decode frozen baseline: %v", err)
	}
	byName := make(map[string]fingerprint.Descriptor)
	for _, d := range fingerprint.DefaultRegistry().All() {
		byName[d.Name] = d
	}

	var out []calibratedRow
	for inst, diag := range baseline {
		for _, ax := range diag.Axes {
			if strings.Contains(ax.Note, replaySkipNote) {
				continue
			}
			d, ok := byName[ax.Name]
			if !ok {
				t.Errorf("%s: frozen axis %q has no registry descriptor", inst, ax.Name)
				continue
			}
			if d.Compare != nil {
				// A compound measurement's comparator reads components the
				// baseline stores only the headline of.
				continue
			}
			drift, mismatch := d.Thresholds()
			got := d.CompareWith(fingerprint.Scalar(ax.Ref), fingerprint.Scalar(ax.Synth))
			out = append(out, calibratedRow{
				Instrument: inst, AxisID: d.ID, Axis: ax.Name,
				Verdict: string(got.Verdict), Direction: got.Direction,
				Drift: drift, Mismatch: mismatch,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Instrument != out[j].Instrument {
			return out[i].Instrument < out[j].Instrument
		}
		return out[i].AxisID < out[j].AxisID
	})
	return out
}

func TestCalibratedComparatorsReproduceTheirGolden(t *testing.T) {
	got := replayCalibrated(t)
	if len(got) < 50 {
		t.Fatalf("only %d rows replayed — the gate has gone hollow", len(got))
	}

	if os.Getenv("CALIBRATED_REPLAY_UPDATE") == "1" {
		raw, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(calibratedReplayGolden, append(raw, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s with %d rows", calibratedReplayGolden, len(got))
		return
	}

	raw, err := os.ReadFile(calibratedReplayGolden)
	if err != nil {
		t.Fatalf("read %s: %v (regenerate with CALIBRATED_REPLAY_UPDATE=1)", calibratedReplayGolden, err)
	}
	var want []calibratedRow
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("decode %s: %v", calibratedReplayGolden, err)
	}
	if len(want) != len(got) {
		t.Fatalf("%s has %d rows, replay produced %d — regenerate with CALIBRATED_REPLAY_UPDATE=1",
			calibratedReplayGolden, len(want), len(got))
	}
	var drifted []string
	for i := range got {
		g, w := got[i], want[i]
		if g.Instrument != w.Instrument || g.AxisID != w.AxisID {
			t.Fatalf("row %d: replay is %s/%s, golden is %s/%s — regenerate",
				i, g.Instrument, g.AxisID, w.Instrument, w.AxisID)
		}
		if g.Verdict != w.Verdict || g.Direction != w.Direction ||
			g.Drift != w.Drift || g.Mismatch != w.Mismatch {
			drifted = append(drifted, fmt.Sprintf(
				"%s/%s: got %s/%s (%.6g,%.6g) want %s/%s (%.6g,%.6g)",
				g.Instrument, g.AxisID, g.Verdict, g.Direction, g.Drift, g.Mismatch,
				w.Verdict, w.Direction, w.Drift, w.Mismatch))
		}
	}
	if len(drifted) > 0 {
		t.Fatalf("calibrated verdicts drifted from %s (regenerate with CALIBRATED_REPLAY_UPDATE=1):\n  %s",
			calibratedReplayGolden, strings.Join(drifted, "\n  "))
	}
	t.Logf("replayed %d frozen pairs through the CALIBRATED descriptor comparators", len(got))
}

// Re-derived, not read: a threshold that silences a listener complaint fails here though the acceptance gate stays green.
func TestCalibratedThresholdsStillReproduceTheListener(t *testing.T) {
	rows := replayCalibrated(t)
	byKey := make(map[string]calibratedRow, len(rows))
	for _, r := range rows {
		byKey[r.Instrument+"/"+r.Axis] = r
	}

	checked := 0
	for _, c := range append(append([]verdictCase{}, primaryVerdicts...), bonusVerdicts...) {
		r, ok := byKey[string(c.instrument)+"/"+c.axis]
		if !ok {
			// Covered by the Kind-level replay and the acceptance gate.
			continue
		}
		checked++
		if r.Verdict == string(verdictOK) || r.Verdict == "" {
			t.Errorf("%s / %s reads %q under the CALIBRATED thresholds (drift=%.6g mismatch=%.6g) "+
				"but the listener said: %s — a calibration that silences a complaint the ear made "+
				"is wrong whatever the corpus says; pin this descriptor with source \"hand\"",
				c.instrument, c.axis, r.Verdict, r.Drift, r.Mismatch, c.verdict)
			continue
		}
		if r.Direction != c.wantDir {
			t.Errorf("%s / %s points %q under the CALIBRATED thresholds, want %q — listener said: %s",
				c.instrument, c.axis, r.Direction, c.wantDir, c.verdict)
		}
	}
	if checked < 5 {
		t.Fatalf("only %d listener-named axes were re-derived — the gate has gone hollow", checked)
	}
	t.Logf("%d listener-named axes still speak under the calibrated thresholds", checked)
}
