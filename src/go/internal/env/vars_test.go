package env

import "testing"

// TestTableSize pins the registry size so an accidental deletion or a
// duplicate-by-copy-paste is caught. Update deliberately when adding a var.
//
// This depends on env_test.go cleaning up every scratch variable it
// registers (via the scratch() helper) before this test runs — the registry
// is process-global and Go runs a package's test files in filename order, so
// env_test.go's tests execute first. If this reports a count HIGHER than
// expected, suspect either (a) a leaked test-scratch registration in
// env_test.go (a Bool/Int/Str/Float/Dur call missing scratch(t, ...)), or (b)
// a scratch(t, ...) call in a test that also calls t.Parallel() — scratch's
// cleanup does not run until the parallel-resume phase, strictly after this
// test has already read All(), so a parallelized scratch registration leaks
// into the count exactly like a missing scratch(t, ...) would (see scratch's
// doc comment) — before suspecting a new production variable.
// 2026-07-27: bumped 91 -> 92 deliberately (Task 4 of the env-var
// centralization plan). runtime_profile_js.go's applyJSProfileOverride could
// set 14 RuntimeProfile fields, but the ported applyEnvProfileOverride only
// covered 13 — the odd one out, MinDispatchLeadSec, was reachable ONLY via
// window.__beatmoProfileOverride's "minDispatchLeadSec" key. Task 4 retires
// that reader in favor of the shared env path, so a 14th variable
// (env.MinDispatchLeadSec / BEATMO_MIN_DISPATCH_LEAD_SEC) was added to avoid
// silently losing the ability to sweep that field. This is the one
// documented case where bumping this constant reflects an intentional
// addition, not a reconciliation to match drifted code.
func TestTableSize(t *testing.T) {
	const want = 94 // +BEATMO_AUDIO_LANE, +BEATMO_AUDIO_HORIZON_SEC (audio lane, 2026-10-02)
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
	for _, tc := range []struct {
		set                  string
		wantWatch, wantProbe bool
	}{
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
