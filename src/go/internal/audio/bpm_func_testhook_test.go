//go:build test

package audio

import "testing"

// TestSetBPMFuncForTestRoundTrip verifies that SetBPMFuncForTest installs the
// provided callback and that nil resets it to a safe no-op (avoiding nil
// dereferences in callers that always invoke SetBPMFunc).
func TestSetBPMFuncForTestRoundTrip(t *testing.T) {
	prev := SetBPMFunc
	t.Cleanup(func() { SetBPMFunc = prev })

	var got int
	SetBPMFuncForTest(func(bpm int) { got = bpm })
	SetBPMFunc(120)
	if got != 120 {
		t.Errorf("SetBPMFunc(120) recorded %d, want 120", got)
	}

	// Nil swaps in a no-op rather than nil — calling it must not panic.
	SetBPMFuncForTest(nil)
	SetBPMFunc(60) // would panic if SetBPMFunc were left nil
}
