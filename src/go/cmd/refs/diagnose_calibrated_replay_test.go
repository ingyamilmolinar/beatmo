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

// diagnose_calibrated_replay_test.go — the gate on what CALIBRATED thresholds
// do to the frozen verdicts.
//
// THE HOLE THIS FILLS
// -------------------
// TestClassifiersReproduceFrozenBaseline replays the 2026-08-22 baseline
// through CompareByKind(d.Kind, …) — the KIND's defaults. It never touches
// Descriptor.Drift/Mismatch, so every descriptor could be recalibrated to
// nonsense and it would stay green. The acceptance tests are no help either:
// they read `verdict` straight out of the frozen JSON and run no classifier
// at all. Between them, nothing in the tree noticed a threshold change.
//
// So this file replays the SAME frozen (ref, synth) pairs — real measurements
// against real audio, taken before any 2026-08-22 tuning — through
// d.CompareWith, which DOES apply the calibrated thresholds, and pins the
// result against its own golden. Two independent assertions:
//
//  1. every replayed row matches the golden exactly, so a threshold cannot
//     move without a reviewed regeneration of this file;
//  2. every axis the LISTENER named in diagnose_acceptance_test.go's tables
//     still reads non-ok under the calibrated thresholds. This is the
//     falsifiable version of "a calibration that stops the tool reproducing
//     the ear is wrong by definition" — the acceptance gate itself cannot
//     check it, because it reads pre-calibration verdicts out of a file.
//
// Regeneration protocol (deliberate threshold changes only):
//
//	cd src/go && CALIBRATED_REPLAY_UPDATE=1 ../../.tools/go/bin/go test \
//	  -tags test -modfile=go.test.mod ./cmd/refs/ -run TestCalibratedComparators -v
//
// then re-run without the env var, confirm assertion (2) still passes, and
// commit the JSON in the same change.

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

// replayCalibrated pushes every frozen pair through the descriptor's own
// comparator (calibrated thresholds included) and returns the rows sorted so
// the golden is stable.
//
// Rows the frozen baseline cannot express are skipped for the same reason
// the Kind-level replay skips them: the ring-T60 escalation needs the
// extrapolation flags and decay rates, which the baseline JSON does not
// carry, so a scalar replay of those rows would compare a quantity the real
// comparator never looked at.
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

// TestCalibratedThresholdsStillReproduceTheListener is the assertion that
// makes calibration falsifiable: every axis the listener named must still
// speak under the calibrated thresholds.
//
// It re-derives the verdict rather than reading it, which is exactly what
// diagnose_acceptance_test.go cannot do (it reads pre-calibration verdicts
// out of the frozen JSON). A threshold that silences a listener complaint
// fails here even though the acceptance gate would stay green.
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
			// Skipped above (compound comparator / escalation row). Those
			// axes are covered by the Kind-level replay and the acceptance
			// gate; nothing to assert here.
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
