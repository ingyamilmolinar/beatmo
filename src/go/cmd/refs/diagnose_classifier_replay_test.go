//go:build !js

package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
)

// diagnose_classifier_replay_test.go — THE regression gate for the registry
// refactor.
//
// WHY THIS EXISTS SEPARATELY FROM diagnose_acceptance_test.go
// -----------------------------------------------------------
// The acceptance test deliberately runs against a FROZEN snapshot of
// `refs diagnose -json` and only calls instrumentDiagnosis.Axis(), a pure
// lookup over deserialized JSON. That is exactly right for its purpose (it
// must not start failing when an instrument is deliberately retuned), but it
// means it exercises NO classification code at all: it would pass unchanged
// even if every classifier in the codebase were deleted and replaced with a
// constant.
//
// So it cannot serve as the gate for moving the classifiers out of this
// package and into internal/audio/fingerprint. This test supplies that gate.
// It takes the same frozen baseline — whose `reference` and `synth` numbers
// are REAL measurements taken against REAL audio at freeze time — and pushes
// each pair back through the NEW registry comparators, asserting that the
// verdict and direction come out byte-identical to the frozen ones.
//
// WHAT IT DOES AND DOES NOT COVER (corrected 2026-09-04). It replays through
// CompareByKind(d.Kind, …), i.e. through the KIND's default thresholds. It
// therefore does NOT read Descriptor.Drift/Mismatch and does NOT notice
// threshold calibration at all — the earlier claim here that "if a threshold
// … drifts during the refactor, this fails" was true only of Kind-level
// thresholds. What it does gate is the classifier BRANCHES and the Kind
// assignments, which is what the refactor could have broken; that is why the
// 2026-08-22 baseline still replays byte-identically after calibration.
//
// The descriptor-threshold half is covered by
// TestCalibratedComparatorsReproduceTheirGolden below, against its own
// golden, plus internal/audio/fingerprint's TestEmbeddedThresholdsAreWellFormed
// and TestDeltaMagReproducesTheVerdict.

// replaySkipNote marks the ring-T60 rows whose frozen verdict came from the
// decay-RATE escalation path rather than the plain T60 ratio. Those rows are
// not reproducible from the two stored scalars alone (the escalation needs
// the extrapolation flags and the decay rates, which the baseline JSON does
// not carry), so they are skipped here and covered instead by
// TestRingT60EscalatesWhenBothSidesExtrapolated in the fingerprint package.
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

	// Map the human-facing axis name to the registry descriptor that now owns
	// it. Going through the registry (rather than a hand-written name->Kind
	// table here) is deliberate: if a descriptor's Kind is changed, this test
	// picks the change up and the frozen verdicts catch it.
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

	// Guard against the gate silently going hollow: if a future change makes
	// every row skip or every name miss, the loop above would pass while
	// asserting nothing.
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
