package async

import (
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"sync"
)

// RuntimeOptions consolidates GC, scheduler, and diagnostic-profile
// configuration so a single ConfigureRuntime call at process startup
// owns all of them. Each field is "0 means leave default."
type RuntimeOptions struct {
	// Soft heap cap (debug.SetMemoryLimit). Lower triggers GC earlier, avoiding long mark phases during recording.
	MemoryLimitMB int

	// debug.SetGCPercent: lower = more frequent, shorter pauses, which real-time work usually wants.
	GCPercent int

	// GOMAXPROCS overrides runtime.GOMAXPROCS. 0 = leave default
	// (NumCPU). Almost always leave 0; the Go scheduler is good.
	GOMAXPROCS int

	// BlockProfileRate enables runtime.SetBlockProfileRate.
	// 0 = off. Use 1 for "every event" (heavy), 100k for sampling.
	BlockProfileRate int

	// MutexProfileFraction enables runtime.SetMutexProfileFraction.
	// 0 = off. 1 = every contention event.
	MutexProfileFraction int
}

// RuntimeSnapshot captures the live values of the runtime knobs we own,
// suitable for logging at startup so each run is self-documenting.
type RuntimeSnapshot struct {
	NumCPU               int
	GOMAXPROCS           int
	GCPercent            int
	MemoryLimit          int64 // bytes; -1 if unbounded
	BlockProfileRate     int
	MutexProfileFraction int
	NumGoroutine         int
}

var configuredOnce sync.Once

// Idempotent: a second call returns the current snapshot. A zero field falls back to its env var
// (BEATMO_MEMORY_LIMIT_MB, _GC_PERCENT, _GOMAXPROCS, _BLOCK_PROFILE, _MUTEX_PROFILE).
func ConfigureRuntime(opts RuntimeOptions) RuntimeSnapshot {
	configuredOnce.Do(func() {
		if opts.MemoryLimitMB == 0 {
			opts.MemoryLimitMB = envInt("BEATMO_MEMORY_LIMIT_MB", 0)
		}
		if opts.GCPercent == 0 {
			opts.GCPercent = envInt("BEATMO_GC_PERCENT", 0)
		}
		if opts.GOMAXPROCS == 0 {
			opts.GOMAXPROCS = envInt("BEATMO_GOMAXPROCS", 0)
		}
		if opts.BlockProfileRate == 0 {
			opts.BlockProfileRate = envInt("BEATMO_BLOCK_PROFILE", 0)
		}
		if opts.MutexProfileFraction == 0 {
			opts.MutexProfileFraction = envInt("BEATMO_MUTEX_PROFILE", 0)
		}

		if opts.MemoryLimitMB > 0 {
			debug.SetMemoryLimit(int64(opts.MemoryLimitMB) * 1024 * 1024)
		}
		if opts.GCPercent > 0 {
			debug.SetGCPercent(opts.GCPercent)
		}
		if opts.GOMAXPROCS > 0 {
			runtime.GOMAXPROCS(opts.GOMAXPROCS)
		}
		if opts.BlockProfileRate > 0 {
			runtime.SetBlockProfileRate(opts.BlockProfileRate)
		}
		if opts.MutexProfileFraction > 0 {
			runtime.SetMutexProfileFraction(opts.MutexProfileFraction)
		}
	})
	return Snapshot()
}

// Snapshot reports the current runtime knob settings without modifying
// anything. Cheap; safe to call from anywhere.
func Snapshot() RuntimeSnapshot {
	return RuntimeSnapshot{
		NumCPU:               runtime.NumCPU(),
		GOMAXPROCS:           runtime.GOMAXPROCS(0),
		GCPercent:            readGCPercent(),
		MemoryLimit:          debug.SetMemoryLimit(-1), // negative genuinely reads-without-setting for SetMemoryLimit
		BlockProfileRate:     0,                        // runtime does not expose a getter
		MutexProfileFraction: runtime.SetMutexProfileFraction(-1),
		NumGoroutine:         runtime.NumGoroutine(),
	}
}

// A NEGATIVE argument to debug.SetGCPercent does not read: the runtime clamps it to -1 and DISABLES GC, so a "read"
// through it turned GC off process-wide and the heap climbed unbounded. Round-trip through the setter and restore.
func readGCPercent() int {
	prev := debug.SetGCPercent(-1) // disables GC, returns the previous value
	debug.SetGCPercent(prev)       // restore it — net no-op (re-enables if it was on)
	return prev
}

func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
