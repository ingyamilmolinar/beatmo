//go:build test

package ui

import (
	"strings"
	"testing"
)

// scripts/bench-desktop.sh greps "[bench_json] "; a key that silently vanishes breaks the tracking with no failure.
func TestBenchJSONCarriesWarmupMetrics(t *testing.T) {
	for _, key := range []string{
		"renderStalePlays", "warmSetSize", "warmSkippedCached", "warmMS",
	} {
		if !strings.Contains(benchJSONFormat, `"`+key+`":`) {
			t.Errorf("bench_json format string missing key %q", key)
		}
	}
}
