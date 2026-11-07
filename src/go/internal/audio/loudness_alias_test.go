//go:build !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// No roster id ends in an ordinal, so the renderer never needs to strip one; an instance id renders through its base's
// Instrument and never reaches it either.
func TestNoVariantNameEndsInAnOrdinal(t *testing.T) {
	for _, id := range roster.All {
		if len(id) > 2 && id[len(id)-2] == '-' {
			if c := id[len(id)-1]; c >= '1' && c <= '9' {
				t.Errorf("%s ends in an ordinal suffix; Phase 0 removed those", id)
			}
		}
	}
}
