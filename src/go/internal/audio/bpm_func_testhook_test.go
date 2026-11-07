//go:build test

package audio

import "testing"

// nil restores a safe no-op, since SetBPM always invokes the hook.
func TestSetBPMFuncForTestRoundTrip(t *testing.T) {
	prev := BPMFuncForTest()
	t.Cleanup(func() { SetBPMFuncForTest(prev) })

	var got int
	SetBPMFuncForTest(func(bpm int) { got = bpm })
	SetBPM(120)
	if got != 120 {
		t.Errorf("SetBPM(120) recorded %d, want 120", got)
	}

	// Nil swaps in a no-op rather than nil — calling it must not panic.
	SetBPMFuncForTest(nil)
	SetBPM(60) // would panic if the hook were left nil
}
