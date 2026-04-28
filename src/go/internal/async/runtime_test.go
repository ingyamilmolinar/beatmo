package async

import (
	"runtime"
	"testing"
)

func TestEnvInt(t *testing.T) {
	cases := []struct {
		name string
		set  bool
		val  string
		def  int
		want int
	}{
		{"unset returns default", false, "", 7, 7},
		{"empty returns default", true, "", 7, 7},
		{"valid positive", true, "42", 0, 42},
		{"non-numeric returns default", true, "abc", 9, 9},
		{"zero returns default", true, "0", 5, 5},
		{"negative returns default", true, "-3", 5, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const k = "BEATMO_ENVINT_TEST"
			if tc.set {
				t.Setenv(k, tc.val)
			} else {
				// t.Setenv unsets on cleanup, but we want absent to start.
				// Setting to "" then deleting is awkward; rely on default unset.
			}
			got := envInt(k, tc.def)
			if got != tc.want {
				t.Errorf("envInt(%q, %d) = %d, want %d", tc.val, tc.def, got, tc.want)
			}
		})
	}
}

// TestSnapshotReportsRuntimeState verifies Snapshot reads through to the
// real runtime helpers. We cannot assert exact values (NumCPU varies per
// machine) but the basics must be sane.
func TestSnapshotReportsRuntimeState(t *testing.T) {
	s := Snapshot()
	if s.NumCPU != runtime.NumCPU() {
		t.Errorf("NumCPU = %d, want %d", s.NumCPU, runtime.NumCPU())
	}
	if s.GOMAXPROCS < 1 {
		t.Errorf("GOMAXPROCS = %d, want >=1", s.GOMAXPROCS)
	}
	if s.NumGoroutine < 1 {
		t.Errorf("NumGoroutine = %d, want >=1", s.NumGoroutine)
	}
}

// TestConfigureRuntimeIsIdempotent calls ConfigureRuntime twice with
// all-zero opts (so no process-wide mutations happen) and confirms both
// calls return a sensible snapshot. The sync.Once gate means only the
// first invocation runs the apply body; the second must be a cheap no-op
// that still returns Snapshot().
//
// We intentionally avoid asserting against settable knobs (GCPercent,
// GOMAXPROCS) because:
//   - mutating them would affect every other test in the package, and
//   - Snapshot()'s GCPercent read uses a side-effecting helper
//     (debug.SetGCPercent(-2)) that does not behave as a pure getter.
//
// IMPORTANT: this test must remain the only caller of ConfigureRuntime
// in the package — sync.Once is process-wide.
func TestConfigureRuntimeIsIdempotent(t *testing.T) {
	first := ConfigureRuntime(RuntimeOptions{})
	if first.NumCPU < 1 || first.GOMAXPROCS < 1 {
		t.Fatalf("first snapshot looks bogus: %+v", first)
	}
	second := ConfigureRuntime(RuntimeOptions{
		MemoryLimitMB:        999,
		GCPercent:            999,
		GOMAXPROCS:           999,
		BlockProfileRate:     999,
		MutexProfileFraction: 999,
	})
	if second.NumCPU != first.NumCPU {
		t.Errorf("NumCPU drifted between calls: first=%d second=%d", first.NumCPU, second.NumCPU)
	}
	if second.GOMAXPROCS != first.GOMAXPROCS {
		t.Errorf("GOMAXPROCS changed on second call (sync.Once should have skipped apply): first=%d second=%d",
			first.GOMAXPROCS, second.GOMAXPROCS)
	}
}
