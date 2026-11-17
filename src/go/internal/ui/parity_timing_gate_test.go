package ui

import "testing"

func TestEvalTimingStage(t *testing.T) {
	b := timingStageBounds{MeanMax: 0.003, P99Max: 0.010, StdDevMax: 0.004, AbsMax: 0.040}

	// Under all bounds with enough samples → no breach.
	if br := evalTimingStage("emitdrift", b, 100, 0.001, 0.005, 0.002, 0.02, 64); br != nil {
		t.Fatalf("unexpected breach: %+v", br)
	}
	// Not enough samples → never breach even if huge (absMax kept within
	// bounds here since the absmax check is deliberately MinSamples-
	// independent — see the dedicated absmax-dropout case below).
	if br := evalTimingStage("emitdrift", b, 10, 1.0, 1.0, 1.0, 0.02, 64); br != nil {
		t.Fatalf("breach below MinSamples: %+v", br)
	}
	// Absmax dropout fires even at low sample count? No — MinSamples still
	// governs statistical stages; absmax uses its own single-sample rule:
	if br := evalTimingStage("emitdrift", b, 1, 0, 0, 0, 0.5, 64); br == nil || br.Stat != "absmax" {
		t.Fatalf("absmax dropout should fire alone, got %+v", br)
	}
	// Mean breach.
	if br := evalTimingStage("emitdrift", b, 100, 0.004, 0.005, 0.002, 0.02, 64); br == nil || br.Stat != "mean" {
		t.Fatalf("expected mean breach, got %+v", br)
	}
	// P99 tail breach.
	if br := evalTimingStage("emitdrift", b, 100, 0.001, 0.011, 0.002, 0.02, 64); br == nil || br.Stat != "p99" {
		t.Fatalf("expected p99 breach, got %+v", br)
	}
	// A bound of 0 disables that statistic.
	b2 := timingStageBounds{MeanMax: 0, P99Max: 0, StdDevMax: 0, AbsMax: 0}
	if br := evalTimingStage("bridge", b2, 1000, 5, 5, 5, 5, 1); br != nil {
		t.Fatalf("zero bounds must disable gating, got %+v", br)
	}
}
