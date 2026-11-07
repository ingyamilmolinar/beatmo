package audio

import (
	"os"
	"regexp"
	"testing"
)

// oto's ALSA driver holds the context lock across the blocking device write and Context.Resume takes the same lock,
// so a per-trigger Resume serialises dispatch behind the writer and beats go out overdue. Resume belongs to the
// transport path; Play / PlayVol / playParamsIdentity / PlayBatch never touch the lock.
func TestNoPerTriggerResumeOnDispatchPath(t *testing.T) {
	re := regexp.MustCompile(`ctx\.Resume\(`)
	for _, f := range []string{"engine_play.go", "batch_desktop.go"} {
		src, err := os.ReadFile(f)
		if err != nil {
			// A missing file means the layout changed: fail loudly so the guard is re-pointed, not silently skipped.
			t.Fatalf("read %s: %v", f, err)
		}
		if loc := re.FindIndex(src); loc != nil {
			t.Errorf("%s calls ctx.Resume() on a per-trigger dispatch path (offset %d); this re-introduces the oto device-lock stall that crashed the timing parity gate. Resume belongs in audio.Resume() only.", f, loc[0])
		}
	}
}
