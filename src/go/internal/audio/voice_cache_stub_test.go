//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// TestLatestVoiceSample_StubReturnsNil — under -tags test, the voice
// cache is stubbed (no globalVoiceCache); LatestVoiceSample must
// return nil so UI callers fall back to the placeholder render path.
func TestLatestVoiceSample_StubReturnsNil(t *testing.T) {
	if got := LatestVoiceSample(roster.Kick); got != nil {
		t.Fatalf("LatestVoiceSample under stub: got %d-sample buffer, want nil", len(got))
	}
}
