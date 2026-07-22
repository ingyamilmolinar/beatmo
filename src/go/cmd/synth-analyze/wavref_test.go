//go:build !js

package main

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// violinRefPath resolves the (uncommitted) violin reference WAV via
// $BEATMO_WAV_REFS, or skips the test when it isn't available. See
// internal/wavref.
func violinRefPath(t *testing.T) string {
	t.Helper()
	p, ok := wavref.Path("356181__mtg__violin-d5.wav")
	if !ok {
		t.Skipf("set %s to a directory containing the reference WAVs (uncommitted)", wavref.EnvVar)
	}
	return p
}
