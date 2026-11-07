//go:build !js

package audio

import (
	"sync"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestPipelineMIDIEventsCopyOnRead(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: channel.Of(roster.Kick), Name: "Kick"}})
	t.Cleanup(func() { pipe.Stop() })

	pipe.AppendMIDIEvent(MIDIEvent{Note: 36, Velocity: 100})
	pipe.AppendMIDIEvent(MIDIEvent{Note: 38, Velocity: 90})

	first := pipe.MIDIEvents()
	if len(first) != 2 {
		t.Fatalf("MIDIEvents len=%d, want 2", len(first))
	}

	// What makes MIDIEvents safe from the UI thread without more locking.
	first[0].Note = 99
	first[1].Velocity = 1

	second := pipe.MIDIEvents()
	if second[0].Note != 36 {
		t.Errorf("MIDIEvents[0].Note=%d after mutation; copy-on-read broken", second[0].Note)
	}
	if second[1].Velocity != 90 {
		t.Errorf("MIDIEvents[1].Velocity=%d after mutation; copy-on-read broken", second[1].Velocity)
	}
}

func TestPipelineAppendMIDIEventConcurrent(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: channel.Of(roster.Kick), Name: "Kick"}})
	t.Cleanup(func() { pipe.Stop() })

	const goroutines = 4
	const perGoroutine = 250
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				pipe.AppendMIDIEvent(MIDIEvent{Note: 36, Velocity: 100})
			}
		}()
	}
	wg.Wait()

	got := pipe.MIDIEvents()
	if want := goroutines * perGoroutine; len(got) != want {
		t.Errorf("MIDIEvents len=%d after %d concurrent appends, want %d", len(got), want, want)
	}
}

func TestPipelineElapsedAdvances(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: channel.Of(roster.Kick), Name: "Kick"}})
	t.Cleanup(func() { pipe.Stop() })

	first := pipe.Elapsed()
	time.Sleep(2 * time.Millisecond)
	second := pipe.Elapsed()

	if second <= first {
		t.Errorf("Elapsed not monotonic: first=%v second=%v", first, second)
	}
	if second < 0 || second > time.Second {
		t.Errorf("Elapsed=%v out of plausible range [0, 1s] after a short sleep", second)
	}
}

func TestPipelineIsAutoStoppedFalseInitially(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: channel.Of(roster.Kick), Name: "Kick"}})
	t.Cleanup(func() { pipe.Stop() })

	if pipe.IsAutoStopped() {
		t.Errorf("IsAutoStopped=true on a freshly constructed pipeline, want false")
	}
}

func TestPipelineShutdownEarlyDrainsWithoutPanic(t *testing.T) {
	// A pipeline with no instruments still starts a master worker; shutdownEarly must close everything without panicking.
	pipe := newTestPipeline(t, nil)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("shutdownEarly panicked: %v", r)
		}
	}()

	pipe.shutdownEarly()
}
