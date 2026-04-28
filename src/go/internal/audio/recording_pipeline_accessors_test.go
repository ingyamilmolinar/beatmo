//go:build !js

package audio

import (
	"sync"
	"testing"
	"time"
)

// recording_pipeline_accessors_test.go covers the public-ish accessors on
// *pipeline that the rest of the audio + recording subsystem depends on.
// Each test uses newTestPipeline (defined in recording_pipeline_test.go)
// so the construction stays in lockstep with production wiring.

func TestPipelineMIDIEventsCopyOnRead(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: "kick", Name: "Kick"}})
	t.Cleanup(func() { pipe.Stop() })

	pipe.AppendMIDIEvent(MIDIEvent{Note: 36, Velocity: 100})
	pipe.AppendMIDIEvent(MIDIEvent{Note: 38, Velocity: 90})

	first := pipe.MIDIEvents()
	if len(first) != 2 {
		t.Fatalf("MIDIEvents len=%d, want 2", len(first))
	}

	// Mutating the returned slice must NOT change pipeline state. This is
	// the single invariant that makes MIDIEvents safe to call from the UI
	// thread without further locking.
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
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: "kick", Name: "Kick"}})
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
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: "kick", Name: "Kick"}})
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
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: "kick", Name: "Kick"}})
	t.Cleanup(func() { pipe.Stop() })

	if pipe.IsAutoStopped() {
		t.Errorf("IsAutoStopped=true on a freshly constructed pipeline, want false")
	}
}

func TestPipelineShutdownEarlyDrainsWithoutPanic(t *testing.T) {
	// shutdownEarly is what newPipeline calls on partial-init failure.
	// A fresh pipeline with no instruments still starts a master worker;
	// shutdownEarly must close every channel and let the workers exit
	// cleanly. The contract: it returns and does not panic.
	pipe := newTestPipeline(t, nil)

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("shutdownEarly panicked: %v", r)
		}
	}()

	pipe.shutdownEarly()
}
