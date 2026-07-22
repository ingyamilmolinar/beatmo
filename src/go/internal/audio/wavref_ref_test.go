//go:build !test && !js

package audio

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// wavRefOrSkip resolves an UNCOMMITTED reference WAV via $BEATMO_WAV_REFS, or
// skips the test when it isn't available (synth-matching tests need real
// instrument timbres that can't be committed — see internal/wavref).
func wavRefOrSkip(t *testing.T, name string) string {
	t.Helper()
	p, ok := wavref.Path(name)
	if !ok {
		t.Skipf("reference WAV %q unavailable; set %s to its directory", name, wavref.EnvVar)
	}
	return p
}
