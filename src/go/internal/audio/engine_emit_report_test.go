//go:build !test && !js

package audio

import "testing"

// TestVoiceEmissionPublishesReport exercises the Task 3.2 wiring: a voice
// scheduled with a parity identity (row, abs) publishes exactly one
// EmitReport the first block it actually renders. Drives processBlock
// directly via the real desktop mixer (no Oto device), per newTestMixer.
func TestVoiceEmissionPublishesReport(t *testing.T) {
	resetClockProbeForTest()
	m := newTestMixer() // constructs a mixer without a live Oto player

	// Schedule a voice to start after 128 samples, tagged (row=3, abs=9).
	m.pos = 0
	m.Schedule("kick", newSineVoice(440, sampleRate, 4096), 128, 3, 9)

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

// TestProcessBlockEmitPathAllocFree pins the hot-path alloc-discipline rule
// from the parity-timing plan (Task 3.2): a processBlock iteration that
// exercises the publish-on-first-render path (a voice with row>=0 rendering
// for the first time) must add zero allocations. publishEmitReport writes
// into a fixed-size ring array and uses only atomics, so this is expected
// to be exactly 0 allocs/op; updateDeviceClock is likewise atomics-only.
//
// The voiceState is built by hand (bypassing Schedule's antiPop-wrap /
// analyzer-notify / scope-push side paths, which run on the caller's
// goroutine at trigger time, not inside processBlock) so only processBlock's
// own allocations are measured.
func TestProcessBlockEmitPathAllocFree(t *testing.T) {
	resetClockProbeForTest()
	m := newTestMixer()
	buf := make([]byte, blockSize*2)

	// Warm up slot/channel/instBuf lazy-init paths before measuring so the
	// measured loop only exercises the steady-state per-block code.
	slot := m.instrumentSlot("kick")
	ch := channelForInstrument("kick")
	// Long enough to survive every measured iteration without exhausting.
	voice := newSineVoice(440, sampleRate, blockSize*1000)
	vs := &voiceState{start: 0, id: "kick", slot: slot, v: voice, ch: ch, row: 3, abs: 9}
	m.mu.Lock()
	m.voices = append(m.voices, vs)
	m.mu.Unlock()
	m.processBlock(0, blockSize, buf) // warm-up call (not measured)

	avg := testing.AllocsPerRun(20, func() {
		// Reset the guard so the publish branch fires on every measured
		// call, not just the first — a plain field write, not an alloc.
		vs.emitted = false
		m.mu.Lock()
		m.processBlock(0, blockSize, buf)
		m.mu.Unlock()
	})

	if avg != 0 {
		t.Fatalf("processBlock emit-publish path allocated %.2f allocs/op, want 0", avg)
	}
}
