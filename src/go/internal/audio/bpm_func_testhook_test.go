//go:build test

package audio

import "testing"

// TestSetBPMFuncForTestRoundTrip verifies that SetBPMFuncForTest installs the
// provided callback and that nil resets it to a safe no-op (avoiding nil
// dereferences in callers that always invoke the hook via SetBPM).
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
