//go:build test

package ui

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// TestSetPlayFunc_FutureWhenSchedulesNotSpawns verifies the migration:
// scheduling many future-timestamped notes must not grow goroutine count
// proportional to N (the old code did `go func() { time.Sleep(d); fn() }`
// per call, which leaked under sustained playback).
func TestSetPlayFunc_FutureWhenSchedulesNotSpawns(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Use a fixed audio clock so 'when' deltas are deterministic.
	restore := audio.SetNowForTest(func() float64 { return 0 })
	t.Cleanup(restore)

	var fired atomic.Int64
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { fired.Add(1) })

	runtime.GC()
	before := runtime.NumGoroutine()

	const N = 100
	// Schedule N notes spread over 100ms (one per ms) — realistic for a
	// sequencer's lookahead. The old code spawned a goroutine per note;
	// the new code uses one scheduler goroutine + bounded pool.
	for i := range N {
		g.playFn("kick", 1.0, 0.020+float64(i)*0.001)
	}

	// Goroutine count must NOT grow with N. The scheduler owns one goroutine;
	// any growth here would mean we leaked a goroutine per scheduled note.
	peak := runtime.NumGoroutine()
	if delta := peak - before; delta > N/4 {
		t.Fatalf("goroutine count grew by %d for %d scheduled notes (before=%d peak=%d); should not scale with N",
			delta, N, before, peak)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fired.Load() == int64(N) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if got := fired.Load(); got != int64(N) {
		t.Fatalf("fired = %d, want %d", got, N)
	}
}

// TestSetPlayFunc_PausedSkipsAtFireTime verifies that pausing the game
// between schedule and fire causes the scheduled fn to be skipped.
func TestSetPlayFunc_PausedSkipsAtFireTime(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	restore := audio.SetNowForTest(func() float64 { return 0 })
	t.Cleanup(restore)

	var fired atomic.Bool
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { fired.Store(true) })

	// Schedule far enough in the future that we have time to pause first.
	g.playFn("kick", 1.0, 0.080)
	g.SetPausedForTest(true)
	t.Cleanup(func() { g.SetPausedForTest(false) })

	time.Sleep(150 * time.Millisecond)
	if fired.Load() {
		t.Fatal("paused playback fired anyway")
	}
}

// TestSetPlayFunc_ImmediateWhenBypassesScheduler verifies that 'when'
// values within the 100µs epsilon dispatch synchronously (no scheduler
// hop), preserving the behavior the old code relied on for "now"-ish
// audio.
func TestSetPlayFunc_ImmediateWhenBypassesScheduler(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	restore := audio.SetNowForTest(func() float64 { return 0 })
	t.Cleanup(restore)

	var fired atomic.Bool
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { fired.Store(true) })

	// 50µs is well under the 100µs epsilon — must dispatch immediately.
	g.playFn("kick", 1.0, 0.00005)

	if !fired.Load() {
		t.Fatal("immediate dispatch did not fire synchronously")
	}
}
