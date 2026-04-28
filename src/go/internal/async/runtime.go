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
	// MemoryLimitMB sets a soft heap cap via debug.SetMemoryLimit.
	// 0 = leave default (math.MaxInt64). For audio-friendly behavior
	// on a 4 GB machine, a value around 256 MB triggers GC earlier
	// and avoids long mark phases during recording.
	MemoryLimitMB int

	// GCPercent overrides the GC trigger via debug.SetGCPercent.
	// 0 = leave default (100). Lower (e.g., 50) = more frequent,
	// shorter pauses; higher = fewer, longer pauses. Real-time apps
	// usually want lower.
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

// ConfigureRuntime applies opts to the live process. Idempotent — second
// call returns the current snapshot without re-applying. Returns the
// snapshot AFTER applying.
//
// Reads matching env vars when an opts field is zero so callers can
// configure via either surface:
//
//	BEATMO_MEMORY_LIMIT_MB     → MemoryLimitMB
//	BEATMO_GC_PERCENT          → GCPercent
//	BEATMO_GOMAXPROCS          → GOMAXPROCS
//	BEATMO_BLOCK_PROFILE       → BlockProfileRate
//	BEATMO_MUTEX_PROFILE       → MutexProfileFraction
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
		GCPercent:            debug.SetGCPercent(-2), // -2 returns current without changing
		MemoryLimit:          debug.SetMemoryLimit(-1),
		BlockProfileRate:     0, // runtime does not expose a getter
		MutexProfileFraction: runtime.SetMutexProfileFraction(-1),
		NumGoroutine:         runtime.NumGoroutine(),
	}
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
