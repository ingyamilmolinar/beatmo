//go:build !test

package audio

import "testing"

// TestUnwrapCVoiceDirect verifies that unwrapCVoice returns a bare cVoice.
func TestUnwrapCVoiceDirect(t *testing.T) {
	cv := &cVoice{buf: []float32{0.1, 0.2}}
	got := unwrapCVoice(cv)
	if got != cv {
		t.Fatal("expected direct cVoice to be returned")
	}
}

// TestUnwrapCVoiceWrapped verifies traversal through all wrapper layers.
func TestUnwrapCVoiceWrapped(t *testing.T) {
	cv := &cVoice{buf: []float32{0.5}}
	wrapped := Voice(&antiPopVoice{
		inner: &resampleVoice{
			src: &scaledVoice{v: cv},
		},
	})
	got := unwrapCVoice(wrapped)
	if got != cv {
		t.Fatalf("expected inner cVoice, got %T", got)
	}
}

// TestUnwrapCVoiceNilForUnknown verifies nil return for unrecognized voice types.
func TestUnwrapCVoiceNilForUnknown(t *testing.T) {
	v := &testVoice{buf: []float64{0.1}}
	got := unwrapCVoice(v)
	if got != nil {
		t.Fatalf("expected nil for testVoice, got %T", got)
	}
}

// TestMixHeadroomValue asserts the expected headroom constant to prevent
// accidental regressions. The value 0.18 provides -14.9dB per-voice
// headroom for dense mixes (7+ instruments).
func TestMixHeadroomValue(t *testing.T) {
	if mixHeadroom != 0.18 {
		t.Fatalf("mixHeadroom = %v, want 0.18", mixHeadroom)
	}
}
