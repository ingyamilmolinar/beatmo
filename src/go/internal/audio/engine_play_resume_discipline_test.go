package audio

import (
	"os"
	"regexp"
	"testing"
)

// TestNoPerTriggerResumeOnDispatchPath pins the fix for the 4th
// overdue/absmax timing-parity crash (2026-07-13): oto's ALSA driver holds the
// context lock across the BLOCKING snd_pcm_writei/snd_pcm_recover device write
// (driver_unix.go readAndWrite), and Context.Resume takes that same lock. A
// per-trigger ctx.Resume() therefore serializes the sequencer's audioLoop
// dispatch behind the device writer — ~one ALSA period (≈13 ms measured) on
// every trigger and 30–100 ms during xrun recovery, which dispatched beats
// overdue and emitted audio late enough to trip the fatal timing gate.
//
// Resume-on-start belongs to the transport path (audio.Resume in
// engine_state.go, called from game_update.go); the per-trigger play paths
// (Play / PlayVol / playParamsIdentity / PlayBatch) must never touch the oto
// context lock. Execution-trace evidence: 7.3 s of Resume mutex-wait in a 20 s
// trace before the fix; see memory project_overdue_crash4_playbatch_stall.
func TestNoPerTriggerResumeOnDispatchPath(t *testing.T) {
	re := regexp.MustCompile(`ctx\.Resume\(`)
	for _, f := range []string{"engine_play.go", "batch_desktop.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			// batch_desktop.go exists on all builds (build-tagged content);
			// a missing file here means the layout changed — fail loudly so
			// the guard is re-pointed, not silently skipped.
			t.Fatalf("read %s: %v", f, err)
		}
		if loc := re.FindIndex(src); loc != nil {
			t.Errorf("%s calls ctx.Resume() on a per-trigger dispatch path (offset %d); this re-introduces the oto device-lock stall that crashed the timing parity gate. Resume belongs in audio.Resume() only.", f, loc[0])
		}
	}
}
