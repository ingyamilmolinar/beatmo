//go:build !js

package audio

import (
	"testing"
)

// TestRecordingMIDIEventsHardCapped drives more events than the configured
// hard cap and asserts that the buffer never exceeds maxMIDIEventsPerSession.
// Without the cap, a runaway MIDI input (or a bug producing high-rate events)
// could grow the buffer to OOM during long sessions.
func TestRecordingMIDIEventsHardCapped(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: "kick", Name: "Kick"}})
	t.Cleanup(func() { pipe.Stop() })

	const N = maxMIDIEventsPerSession + 50_000
	for i := 0; i < N; i++ {
		pipe.AppendMIDIEvent(MIDIEvent{Note: 36, Velocity: 100})
	}

	got := len(pipe.MIDIEvents())
	if got > maxMIDIEventsPerSession {
		t.Errorf("MIDIEvents len=%d exceeds hard cap %d after %d appends",
			got, maxMIDIEventsPerSession, N)
	}
}

// TestRecordingMIDIEventsKeepsRecent appends with monotonically increasing
// Note values and asserts the most recent events survive the drop-oldest
// policy. The last 100 entries must contain the highest 100 Note values
// (modulo MIDI's 127 ceiling — we use Velocity instead since it's also int).
func TestRecordingMIDIEventsKeepsRecent(t *testing.T) {
	pipe := newTestPipeline(t, []InstrumentMeta{{ID: "kick", Name: "Kick"}})
	t.Cleanup(func() { pipe.Stop() })

	const N = maxMIDIEventsPerSession + 10_000
	for i := 0; i < N; i++ {
		pipe.AppendMIDIEvent(MIDIEvent{Note: 36, Velocity: i})
	}

	got := pipe.MIDIEvents()
	if len(got) > maxMIDIEventsPerSession {
		t.Fatalf("MIDIEvents len=%d exceeds cap %d", len(got), maxMIDIEventsPerSession)
	}
	if len(got) == 0 {
		t.Fatalf("MIDIEvents empty after %d appends", N)
	}

	// Last entry's Velocity must equal N-1 (most recent append).
	lastIdx := len(got) - 1
	if got[lastIdx].Velocity != N-1 {
		t.Errorf("most recent event Velocity=%d, want %d (drop-oldest policy violated)",
			got[lastIdx].Velocity, N-1)
	}

	// Earliest retained entry's Velocity must be at least N - cap.
	if got[0].Velocity < N-maxMIDIEventsPerSession {
		t.Errorf("oldest retained Velocity=%d < %d; drop-oldest leaked ancient entries",
			got[0].Velocity, N-maxMIDIEventsPerSession)
	}
}
