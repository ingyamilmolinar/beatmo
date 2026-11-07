//go:build test

package ui

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Many future-timestamped notes must not grow the goroutine count: one scheduler goroutine serves them all.
func TestSetPlayFunc_FutureWhenSchedulesNotSpawns(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	// Use a fixed audio clock so 'when' deltas are deterministic.
	restore := audio.SetNowForTest(func() float64 { return 0 })
	t.Cleanup(restore)

	var fired atomic.Int64
	g.SetPlayFunc(func(id instrument.ID, vol float64, when ...float64) { fired.Add(1) })

	runtime.GC()
	before := runtime.NumGoroutine()

	const N = 100
	// One per ms over 100 ms, a realistic sequencer lookahead.
	for i := range N {
		g.playFn(roster.Kick, 1.0, 0.020+float64(i)*0.001)
	}

	// Any growth with N means a goroutine per scheduled note.
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

// Paused between schedule and fire, the fn is skipped.
func TestSetPlayFunc_PausedSkipsAtFireTime(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	restore := audio.SetNowForTest(func() float64 { return 0 })
	t.Cleanup(restore)

	var fired atomic.Bool
	g.SetPlayFunc(func(id instrument.ID, vol float64, when ...float64) { fired.Store(true) })

	// Schedule far enough in the future that we have time to pause first.
	g.playFn(roster.Kick, 1.0, 0.080)
	g.SetPausedForTest(true)
	t.Cleanup(func() { g.SetPausedForTest(false) })

	time.Sleep(150 * time.Millisecond)
	if fired.Load() {
		t.Fatal("paused playback fired anyway")
	}
}

// A when within 100 µs dispatches synchronously, with no scheduler hop.
func TestSetPlayFunc_ImmediateWhenBypassesScheduler(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)

	restore := audio.SetNowForTest(func() float64 { return 0 })
	t.Cleanup(restore)

	var fired atomic.Bool
	g.SetPlayFunc(func(id instrument.ID, vol float64, when ...float64) { fired.Store(true) })

	// 50µs is well under the 100µs epsilon — must dispatch immediately.
	g.playFn(roster.Kick, 1.0, 0.00005)

	if !fired.Load() {
		t.Fatal("immediate dispatch did not fire synchronously")
	}
}
