//go:build test

package audio

import "testing"

// TestLatestVoiceSample_StubReturnsNil — under -tags test, the voice
// cache is stubbed (no globalVoiceCache); LatestVoiceSample must
// return nil so UI callers fall back to the placeholder render path.
func TestLatestVoiceSample_StubReturnsNil(t *testing.T) {
	if got := LatestVoiceSample("kick-1"); got != nil {
		t.Fatalf("LatestVoiceSample under stub: got %d-sample buffer, want nil", len(got))
	}
}
