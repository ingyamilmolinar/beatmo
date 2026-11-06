//go:build test

package ui

import (
	"strings"
	"testing"
)

// TestBenchJSONCarriesWarmupMetrics pins the machine-readable bench line's
// key set. scripts/bench-desktop.sh greps stdout for "[bench_json] "; a key
// that silently disappears breaks the regression tracking without failing
// anything, so the key list is asserted here.
func TestBenchJSONCarriesWarmupMetrics(t *testing.T) {
	for _, key := range []string{
		"renderStalePlays", "warmSetSize", "warmSkippedCached", "warmMS",
	} {
		if !strings.Contains(benchJSONFormat, `"`+key+`":`) {
			t.Errorf("bench_json format string missing key %q", key)
		}
	}
}
