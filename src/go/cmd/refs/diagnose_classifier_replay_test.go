//go:build !js

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

// The acceptance test reads frozen verdicts and runs no classifier, so this replays the frozen pairs (real measurements)
// through the registry comparators at the KIND's defaults: it gates the classifier branches and Kind assignments, not
// calibrated thresholds (TestCalibratedComparatorsReproduceTheirGolden covers those).

// Rows whose frozen verdict came from the decay-RATE escalation cannot be reproduced from two scalars (they need the
// extrapolation flags and rates); TestRingT60EscalatesWhenBothSidesExtrapolated covers them.
const replaySkipNote = "T60 NOT OBSERVABLE"

func TestClassifiersReproduceFrozenBaseline(t *testing.T) {
	raw, err := os.ReadFile("testdata/diagnose_baseline_2026-08-22.json")
	if err != nil {
		t.Fatalf("read frozen baseline: %v", err)
	}
	var baseline map[string]instrumentDiagnosis
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatalf("decode frozen baseline: %v", err)
	}
	if len(baseline) == 0 {
		t.Fatal("frozen baseline is empty — the gate would be vacuous")
	}

	// Through the registry, not a hand table, so a changed Kind is picked up and the frozen verdicts catch it.
	byName := make(map[string]fingerprint.Descriptor)
	for _, d := range fingerprint.DefaultRegistry().All() {
		byName[d.Name] = d
	}

	checked, skipped := 0, 0
	for inst, diag := range baseline {
		for _, ax := range diag.Axes {
			if strings.Contains(ax.Note, replaySkipNote) {
				skipped++
				continue
			}
			d, ok := byName[ax.Name]
			if !ok {
				t.Errorf("%s: frozen axis %q has no registry descriptor", inst, ax.Name)
				continue
			}
			got := fingerprint.CompareByKind(d.Kind,
				fingerprint.Scalar(ax.Ref), fingerprint.Scalar(ax.Synth))

			if string(got.Verdict) != string(ax.Verdict) {
				t.Errorf("%s / %s: verdict drifted: registry says %q, frozen baseline says %q (ref=%v synth=%v)",
					inst, ax.Name, got.Verdict, ax.Verdict, ax.Ref, ax.Synth)
			}
			if got.Direction != ax.Direction {
				t.Errorf("%s / %s: direction drifted: registry says %q, frozen baseline says %q (ref=%v synth=%v)",
					inst, ax.Name, got.Direction, ax.Direction, ax.Ref, ax.Synth)
			}
			checked++
		}
	}

	// Every row skipping or every name missing would pass while asserting nothing.
	if checked < 50 {
		t.Fatalf("only %d axis rows were actually checked (%d skipped) — the gate has gone hollow", checked, skipped)
	}
	t.Logf("replayed %d frozen axis rows through the registry comparators (%d ring-T60 escalation rows skipped)", checked, skipped)
}

// TestEveryFrozenAxisNameHasADescriptor is the coverage half of the gate: the
// registry must still define every axis the frozen baseline knows about, so
// the refactor cannot quietly drop one.
func TestEveryFrozenAxisNameHasADescriptor(t *testing.T) {
	raw, err := os.ReadFile("testdata/diagnose_baseline_2026-08-22.json")
	if err != nil {
		t.Fatalf("read frozen baseline: %v", err)
	}
	var baseline map[string]instrumentDiagnosis
	if err := json.Unmarshal(raw, &baseline); err != nil {
		t.Fatalf("decode frozen baseline: %v", err)
	}

	known := make(map[string]bool)
	for _, d := range fingerprint.DefaultRegistry().All() {
		known[d.Name] = true
	}
	missing := make(map[string]bool)
	for _, diag := range baseline {
		for _, ax := range diag.Axes {
			if !known[ax.Name] {
				missing[ax.Name] = true
			}
		}
	}
	for name := range missing {
		t.Errorf("frozen baseline axis %q is no longer defined in the descriptor registry", name)
	}
}
