//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Exactly one report on the first block a (row, abs) voice renders, through the real mixer.
func TestVoiceEmissionPublishesReport(t *testing.T) {
	resetClockProbeForTest()
	m := newTestMixer() // constructs a mixer without a live Oto player

	// Schedule a voice to start after 128 samples, tagged (row=3, abs=9).
	m.pos = 0
	m.Schedule(channel.Of(roster.Kick), newSineVoice(440, sampleRate, 4096), 128, 3, 9)

	// Drive Read for enough samples to cross the start boundary.
	buf := make([]byte, 2*512)
	_, _ = m.Read(buf)

	dst := make([]EmitReport, 8)
	n := DrainEmitReports(dst)
	found := false
	for _, r := range dst[:n] {
		if r.Row == 3 && r.Abs == 9 {
			found = true
		}
	}
	if !found {
		t.Fatalf("no emit report for (3,9); got %+v", dst[:n])
	}
}

// The publish-on-first-render path adds zero allocations. The voiceState is built by hand, since Schedule's side
// paths run on the caller's goroutine, not in processBlock.
func TestProcessBlockEmitPathAllocFree(t *testing.T) {
	resetClockProbeForTest()
	m := newTestMixer()
	buf := make([]byte, blockSize*2)

	// Lazy init warmed first, so only steady-state code is measured.
	slot := m.instrumentSlot(channel.Of(roster.Kick))
	ch := channelForInstrument(channel.Of(roster.Kick))
	// Long enough to survive every measured iteration without exhausting.
	voice := newSineVoice(440, sampleRate, blockSize*1000)
	vs := &voiceState{start: 0, id: channel.Of(roster.Kick), slot: slot, v: voice, ch: ch, row: 3, abs: 9}
	m.mu.Lock()
	m.voices = append(m.voices, vs)
	m.mu.Unlock()
	m.processBlock(0, blockSize, buf) // warm-up call (not measured)

	avg := testing.AllocsPerRun(20, func() {
		// So the publish branch fires on every measured call.
		vs.emitted = false
		m.mu.Lock()
		m.processBlock(0, blockSize, buf)
		m.mu.Unlock()
	})

	if avg != 0 {
		t.Fatalf("processBlock emit-publish path allocated %.2f allocs/op, want 0", avg)
	}
}
