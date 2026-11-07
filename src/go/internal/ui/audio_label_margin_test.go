//go:build test

package ui

import "testing"

// One density token for the Levels, Spectrum and Wave label gutter.
func TestAudioLabelMarginToken(t *testing.T) {
	if got := Profile().DensityValues().AudioLabelMarginW; got <= 0 {
		t.Fatalf("AudioLabelMarginW density token unset (got %d)", got)
	}
}
