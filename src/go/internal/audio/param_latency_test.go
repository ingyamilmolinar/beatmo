package audio

import (
	"math"
	"testing"
)

// Phase 9 contract: every SetInstrumentParam call bumps the dispatch
// counter, rejected non-finite calls bump the rejected counter, and the
// dispatch average + max reflect actual wall-clock cost. Tests use the
// snare instrument because it has a stable recipe binding under -tags
// test (see synth_recipe.go bindBuiltinInstrumentRecipes init).
func TestParamLatencyMetrics_CountsDispatchedCalls(t *testing.T) {
	ResetParamLatencyMetrics()
	t.Cleanup(ResetParamLatencyMetrics)
	ResetInstrumentParams("snare")
	t.Cleanup(func() { ResetInstrumentParams("snare") })

	const n = 50
	for i := 0; i < n; i++ {
		SetInstrumentParam("snare", "decay", 0.5+float64(i)/100)
	}

	m := GetParamLatencyMetrics()
	if m.Updates != n {
		t.Errorf("Updates = %d, want %d", m.Updates, n)
	}
	if m.Rejected != 0 {
		t.Errorf("Rejected = %d, want 0", m.Rejected)
	}
	if m.DispatchAvgNS <= 0 {
		t.Errorf("DispatchAvgNS = %d, want > 0 (50 SetInstrumentParam calls cannot all take 0 ns)", m.DispatchAvgNS)
	}
	if m.DispatchMaxNS < m.DispatchAvgNS {
		t.Errorf("DispatchMaxNS (%d) < DispatchAvgNS (%d) — impossible", m.DispatchMaxNS, m.DispatchAvgNS)
	}
}

func TestParamLatencyMetrics_CountsRejectedNonFinite(t *testing.T) {
	ResetParamLatencyMetrics()
	t.Cleanup(ResetParamLatencyMetrics)
	ResetInstrumentParams("kick")
	t.Cleanup(func() { ResetInstrumentParams("kick") })

	SetInstrumentParam("kick", "pitch", math.NaN())
	SetInstrumentParam("kick", "decay", math.Inf(+1))
	SetInstrumentParam("kick", "drive", math.Inf(-1))
	// One valid call — should bump Updates, not Rejected.
	SetInstrumentParam("kick", "pitch", 3)

	m := GetParamLatencyMetrics()
	if m.Rejected != 3 {
		t.Errorf("Rejected = %d, want 3", m.Rejected)
	}
	if m.Updates != 1 {
		t.Errorf("Updates = %d, want 1", m.Updates)
	}
}

func TestParamLatencyMetrics_CoalescesNoOpWrites(t *testing.T) {
	// Phase 8 contract: same-value SetInstrumentParam calls must NOT bump
	// Updates (because state didn't change) and MUST NOT trigger cache
	// invalidate / hook publish / platform callback. The Coalesced counter
	// distinguishes "stationary drag burst" from "user actually changing
	// the param".
	ResetParamLatencyMetrics()
	t.Cleanup(ResetParamLatencyMetrics)
	ResetInstrumentParams("snare")
	t.Cleanup(func() { ResetInstrumentParams("snare") })

	// First write establishes the value — counts as Updates=1.
	SetInstrumentParam("snare", "decay", 0.42)
	// Subsequent identical writes are no-ops — count as Coalesced.
	const repeats = 30
	for i := 0; i < repeats; i++ {
		SetInstrumentParam("snare", "decay", 0.42)
	}
	// A different value resumes mutation.
	SetInstrumentParam("snare", "decay", 0.43)
	for i := 0; i < repeats; i++ {
		SetInstrumentParam("snare", "decay", 0.43)
	}

	m := GetParamLatencyMetrics()
	if m.Updates != 2 {
		t.Errorf("Updates = %d, want 2 (initial + change-of-value)", m.Updates)
	}
	if m.Coalesced != 2*repeats {
		t.Errorf("Coalesced = %d, want %d (two %d-burst stationary drags)", m.Coalesced, 2*repeats, repeats)
	}
}

func TestParamLatencyMetrics_ResetClearsCounters(t *testing.T) {
	ResetParamLatencyMetrics()
	ResetInstrumentParams("snare")
	t.Cleanup(func() { ResetInstrumentParams("snare") })

	SetInstrumentParam("snare", "decay", 0.5)
	SetInstrumentParam("snare", "pitch", math.NaN())

	pre := GetParamLatencyMetrics()
	if pre.Updates == 0 || pre.Rejected == 0 {
		t.Fatalf("setup failed: pre=%+v", pre)
	}

	ResetParamLatencyMetrics()
	post := GetParamLatencyMetrics()
	if post.Updates != 0 || post.Rejected != 0 || post.DispatchAvgNS != 0 || post.DispatchMaxNS != 0 {
		t.Errorf("ResetParamLatencyMetrics did not clear: %+v", post)
	}
}
