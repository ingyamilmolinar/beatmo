package audio

import (
	"math"
	"testing"
)

// gainProc is a tiny Processor that multiplies each sample by a constant
// gain — used so the splits are easy to predict numerically without depending
// on biquad coefficients or block-processor optimizations.
type gainProc struct{ g float64 }

func (p *gainProc) ProcessSample(x float64) float64 { return x * p.g }

// TestProcessInsertsBlockLocalAppliesVolumeAndInsertsOnly pins the contract:
// ProcessInsertsBlockLocal multiplies by Volume and runs ONLY the inserts
// chain, leaving the EQ-side processors out. This is what lets the mixer
// tap the post-FX/pre-EQ signal for scope.StageInsertFX.
func TestProcessInsertsBlockLocalAppliesVolumeAndInsertsOnly(t *testing.T) {
	ch := newChannel("split-inserts", nil)
	ch.SetVolume(0.5)
	ch.replaceProcessors(
		[]Processor{&gainProc{g: 2.0}}, // inserts: ×2
		[]Processor{&gainProc{g: 8.0}}, // eq: ×8 (must NOT apply here)
	)

	input := []float64{1, 1, 1, 1}
	output := make([]float64, len(input))
	ch.ProcessInsertsBlockLocal(input, output)

	// Expected: input × Volume × inserts = 1 × 0.5 × 2.0 = 1.0
	for i, got := range output {
		if math.Abs(got-1.0) > 1e-12 {
			t.Errorf("output[%d]: got %v want 1.0 (EQ ×8 must NOT have run)", i, got)
		}
	}
}

// TestProcessEQBlockLocalAppliesEQOnly pins the contract: ProcessEQBlockLocal
// runs ONLY the eqProcs chain and does NOT re-apply volume — that's the
// upstream caller's (ProcessInsertsBlockLocal) responsibility.
func TestProcessEQBlockLocalAppliesEQOnly(t *testing.T) {
	ch := newChannel("split-eq", nil)
	ch.SetVolume(0.5) // must NOT be applied here
	ch.replaceProcessors(
		[]Processor{&gainProc{g: 2.0}}, // inserts: ×2 (must NOT apply here)
		[]Processor{&gainProc{g: 3.0}}, // eq: ×3
	)

	input := []float64{1, 1, 1, 1}
	output := make([]float64, len(input))
	ch.ProcessEQBlockLocal(input, output)

	// Expected: input × eq = 1 × 3.0 = 3.0 (volume not re-applied; inserts skipped)
	for i, got := range output {
		if math.Abs(got-3.0) > 1e-12 {
			t.Errorf("output[%d]: got %v want 3.0 (volume must NOT re-apply, inserts must NOT run)", i, got)
		}
	}
}

// TestProcessEQBlockLocalEmptyChainIsIdentity verifies that when eqProcs is
// empty, the EQ pass is a copy-accumulate of the input — important so the
// mixer can call the split methods even when an instrument has no EQ bands
// configured.
func TestProcessEQBlockLocalEmptyChainIsIdentity(t *testing.T) {
	ch := newChannel("split-empty-eq", nil)
	ch.SetVolume(1.0)
	ch.replaceProcessors([]Processor{&gainProc{g: 2.0}}, nil)

	input := []float64{0.25, -0.5, 0.75}
	output := make([]float64, len(input))
	ch.ProcessEQBlockLocal(input, output)

	for i, got := range output {
		if math.Abs(got-input[i]) > 1e-12 {
			t.Errorf("output[%d]: got %v want %v (identity copy-accumulate)", i, got, input[i])
		}
	}
}

// TestProcessInsertsAndEQComposeEqualsProcessBlockLocal verifies that calling
// ProcessInsertsBlockLocal then ProcessEQBlockLocal back-to-back produces
// the same output as the legacy ProcessBlockLocal — i.e. the split does not
// change the channel's audible behavior, only exposes a tap point.
func TestProcessInsertsAndEQComposeEqualsProcessBlockLocal(t *testing.T) {
	// Use two channels with identical config so internal block-cache state
	// is not shared.
	mkChan := func(name string) *Channel {
		ch := newChannel(name, nil)
		ch.SetVolume(0.6)
		ch.replaceProcessors(
			[]Processor{&gainProc{g: 1.5}, &gainProc{g: 0.8}},
			[]Processor{&gainProc{g: 1.25}},
		)
		return ch
	}

	input := []float64{0.1, -0.2, 0.3, -0.4, 0.5}

	chA := mkChan("compose-legacy")
	legacy := make([]float64, len(input))
	chA.ProcessBlockLocal(input, legacy)

	chB := mkChan("compose-split")
	mid := make([]float64, len(input))
	split := make([]float64, len(input))
	chB.ProcessInsertsBlockLocal(input, mid)
	chB.ProcessEQBlockLocal(mid, split)

	for i := range input {
		if math.Abs(legacy[i]-split[i]) > 1e-10 {
			t.Errorf("sample %d: legacy=%v split-compose=%v (must match within 1e-10)",
				i, legacy[i], split[i])
		}
	}
}
