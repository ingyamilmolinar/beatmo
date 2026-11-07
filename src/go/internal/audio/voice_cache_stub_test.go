//go:build test

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestLatestVoiceSample_StubReturnsNil(t *testing.T) {
	if got := LatestVoiceSample(roster.Kick); got != nil {
		t.Fatalf("LatestVoiceSample under stub: got %d-sample buffer, want nil", len(got))
	}
}
