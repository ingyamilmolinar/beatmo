package ui

import "testing"

func TestParseBenchStormScenario(t *testing.T) {
	cases := map[string]benchStormScenario{
		"": benchStormOff, "off": benchStormOff,
		"toggle": benchStormToggle,
		"ctrl":   benchStormCtrlSweep, "ctrl-sweep": benchStormCtrlSweep,
		"combined": benchStormCombined,
	}
	for in, want := range cases {
		if got := parseBenchStormScenario(in); got != want {
			t.Fatalf("parseBenchStormScenario(%q)=%v want %v", in, got, want)
		}
	}
}

func TestBenchStormMutations(t *testing.T) {
	if m := benchStormMutationsForTick(benchStormOff, 5); len(m) != 0 {
		t.Fatalf("off scenario must emit no mutations, got %d", len(m))
	}
	// Toggle alternates filter_enabled between 1 and 0 across ticks.
	m0 := benchStormMutationsForTick(benchStormToggle, 0)
	m1 := benchStormMutationsForTick(benchStormToggle, 1)
	if len(m0) != 1 || m0[0].Param != "filter_enabled" {
		t.Fatalf("toggle tick0 = %+v, want single filter_enabled", m0)
	}
	if m0[0].Value == m1[0].Value {
		t.Fatalf("toggle must alternate value across ticks: %v vs %v", m0[0].Value, m1[0].Value)
	}
	// Ctrl sweep drives filter_cutoff within [0,1].
	mc := benchStormMutationsForTick(benchStormCtrlSweep, 3)
	if len(mc) != 1 || mc[0].Param != "filter_cutoff" || mc[0].Value < 0 || mc[0].Value > 1 {
		t.Fatalf("ctrl sweep tick3 = %+v, want filter_cutoff in [0,1]", mc)
	}
	// Combined emits both a toggle (periodically) and a cutoff sweep.
	total := 0
	for tick := 0; tick < 8; tick++ {
		total += len(benchStormMutationsForTick(benchStormCombined, tick))
	}
	if total <= 8 {
		t.Fatalf("combined over 8 ticks should emit >8 mutations (sweep every tick + periodic toggle), got %d", total)
	}
}
