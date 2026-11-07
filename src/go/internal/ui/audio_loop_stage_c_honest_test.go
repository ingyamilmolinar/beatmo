//go:build test

package ui

import (
	"math"
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// audioLoop floors a late request to audioNow + 12 ms so WebAudio gets a future start, but Stage C must observe
// the REQUESTED time: observing the clamped one let every lateness gate pass by construction.
func TestStageCObservesRequestedTimeNotTheClampedOne(t *testing.T) {
	restoreProfile := SetRuntimeProfileForTest(browserRuntimeProfile())
	t.Cleanup(restoreProfile)
	g := buildTestGame(t)
	g.SetPlayingForTest(true)

	var clockMu sync.Mutex
	clock := 10.0
	restoreNow := audio.SetNowForTest(func() float64 {
		clockMu.Lock()
		defer clockMu.Unlock()
		return clock
	})
	t.Cleanup(restoreNow)
	setClock := func(v float64) {
		clockMu.Lock()
		clock = v
		clockMu.Unlock()
	}

	type dispatched struct{ when float64 }
	got := make(chan dispatched, 8)
	g.playFn = func(id instrument.ID, vol float64, when ...float64) {
		d := dispatched{when: -1}
		if len(when) > 0 {
			d.when = when[0]
		}
		got <- d
	}
	recv := func() dispatched {
		t.Helper()
		select {
		case d := <-got:
			return d
		case <-time.After(3 * time.Second):
			t.Fatal("audioLoop did not dispatch the request")
			return dispatched{}
		}
	}

	// Prime: the first Stage C observation only anchors the warm-up window.
	g.queueSoundAtParams(0, 0, roster.Kick, 1, 0, 1, 10.05, 10.0)
	recv()

	// Past the 0.5 s warm-up: a request 100 ms in the past.
	setClock(11.0)
	g.queueSoundAtParams(0, 1, roster.Kick, 1, 0, 1, 10.9, 10.84)
	d := recv()

	floor := 11.0 + RuntimeProf().MinDispatchLeadSec
	if math.Abs(d.when-floor) > 1e-9 {
		t.Fatalf("dispatch when = %.4f, want the floor-clamped %.4f (the clamp keeps the late note audible)", d.when, floor)
	}
	snap := g.schedMetrics.Snapshot()
	if snap.Overdue < 1 {
		t.Fatalf("Stage C recorded overdue=%d for a note dispatched 100 ms late — it observed the CLAMPED time again (min lead %.4f)", snap.Overdue, snap.MinLead)
	}
	if math.Abs(snap.MaxLag-0.1) > 1e-6 {
		t.Fatalf("Stage C maxLag = %.4f s, want the real lateness 0.100 s", snap.MaxLag)
	}
}
