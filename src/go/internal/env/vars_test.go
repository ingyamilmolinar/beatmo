package env

import "testing"

// Pins the registry size against an accidental deletion or copy-paste duplicate; update it deliberately. A count too
// HIGH usually means a scratch registration in env_test.go that leaked (no scratch(t, ...), or one under t.Parallel()),
// not a new production variable.
func TestTableSize(t *testing.T) {
	const want = 94
	if got := len(All()); got != want {
		t.Errorf("len(All())=%d, want %d — update this constant deliberately when adding or removing a variable", got, want)
	}
}

// TestRemovedNamesAreGone guards the six removals in the approved spec.
// A reintroduction would resurrect a name the spec deliberately retired.
func TestRemovedNamesAreGone(t *testing.T) {
	removed := []string{
		"DRAW_FRAMEBUFFER",
		"PREVIEW_DEBUG",
		"BEATMO_CONFIG",
		"SEQ_TICK_MS",
		"PERF_FAST_PATH",
		"BEATMO_HEAP_PROBE",
		"BEATMO_HEAP_WATCH",
	}
	live := map[string]bool{}
	for _, v := range All() {
		live[v.Name()] = true
	}
	for _, name := range removed {
		if live[name] {
			t.Errorf("%q is registered but the spec removed it", name)
		}
	}
}

// TestRenamedNamesUseNewSpelling guards the twelve renames.
func TestRenamedNamesUseNewSpelling(t *testing.T) {
	oldToNew := map[string]string{
		"SWEEP":                  "BEATMO_KICK_SWEEP",
		"WRITE_WAV":              "BEATMO_WRITE_WAV",
		"TEST_TONE":              "BEATMO_TEST_TONE",
		"TEST_VOICE":             "BEATMO_TEST_VOICE",
		"TEST_RAW_VOICE":         "BEATMO_TEST_RAW_VOICE",
		"TEST_RAW_VOICE_FULL":    "BEATMO_TEST_RAW_VOICE_FULL",
		"TEST_RAW_VOICE_NOSCALE": "BEATMO_TEST_RAW_VOICE_NOSCALE",
		"SINGLE_VOICE":           "BEATMO_SINGLE_VOICE",
		"AUDIO_SAMPLE_RATE":      "BEATMO_AUDIO_SAMPLE_RATE",
		"PPROF":                  "BEATMO_PPROF",
		"UPDATE_GOLDEN":          "BEATMO_UPDATE_GOLDEN",
		"RUN_SOAK_REAL":          "BEATMO_RUN_SOAK_REAL",
	}
	live := map[string]bool{}
	for _, v := range All() {
		live[v.Name()] = true
	}
	for old, new := range oldToNew {
		if live[old] {
			t.Errorf("%q is still registered; the spec renamed it to %q", old, new)
		}
		if !live[new] {
			t.Errorf("%q is not registered; the spec renamed %q to it", new, old)
		}
	}
}

// TestHeapDiagLevels pins the three-state replacement for the two retired
// heap booleans.
func TestHeapDiagLevels(t *testing.T) {
	for _, tc := range []struct{ set string; wantWatch, wantProbe bool }{
		{"", false, false},
		{"watch", true, false},
		{"probe", true, true},
		{"garbage", false, false},
	} {
		t.Setenv("BEATMO_HEAP_DIAG", tc.set)
		Reload()
		if got := HeapWatchEnabled(); got != tc.wantWatch {
			t.Errorf("BEATMO_HEAP_DIAG=%q HeapWatchEnabled()=%v, want %v", tc.set, got, tc.wantWatch)
		}
		if got := HeapProbeEnabled(); got != tc.wantProbe {
			t.Errorf("BEATMO_HEAP_DIAG=%q HeapProbeEnabled()=%v, want %v", tc.set, got, tc.wantProbe)
		}
	}
}
