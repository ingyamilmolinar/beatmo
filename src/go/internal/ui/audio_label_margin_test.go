//go:build test

package ui

import "testing"

// TestAudioLabelMarginToken pins the dB/amplitude left-label margin shared by
// the Levels, Spectrum and Wave tabs to ONE density token (was triplicated
// hardcoded 28px), so the analyzer label gutter is re-styleable in one place.
func TestAudioLabelMarginToken(t *testing.T) {
	if got := Profile().DensityValues().AudioLabelMarginW; got <= 0 {
		t.Fatalf("AudioLabelMarginW density token unset (got %d)", got)
	}
}
