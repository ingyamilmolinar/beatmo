package main

import (
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// wavRoot returns the local reference-WAV directory (with a trailing separator,
// so existing `root + filename` concatenation keeps working), or skips the test
// when $BEATMO_WAV_REFS is unset. These reference recordings are NOT committed
// (copyright); see internal/wavref.
func wavRoot(t *testing.T) string {
	t.Helper()
	d, ok := wavref.Dir()
	if !ok {
		t.Skipf("set %s to a directory of reference WAVs (uncommitted; see internal/wavref)", wavref.EnvVar)
	}
	return d + string(os.PathSeparator)
}
