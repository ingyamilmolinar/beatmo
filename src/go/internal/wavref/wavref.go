// Package wavref locates local, UNCOMMITTED reference WAV files for tests and
// analysis tools.
//
// Real instrument recordings can't be committed (copyright / mixed freesound
// licenses), and synth-*matching* tests inherently need real timbres — a
// generated sine can't stand in for a violin. So those refs live outside the
// repo: point $BEATMO_WAV_REFS at a directory holding them. Tests skip cleanly
// when it is unset or a file is missing, so a clean checkout / CI stays green.
//
// Committed test audio, by contrast, is always GENERATED (internal/wavgen /
// cmd/wavgen) so nothing copyright-sensitive is versioned.
package wavref

import (
	"os"
	"path/filepath"
)

// EnvVar names the local reference-WAV directory.
const EnvVar = "BEATMO_WAV_REFS"

// Dir returns the configured reference-WAV directory and ok=false when unset.
func Dir() (string, bool) {
	d := os.Getenv(EnvVar)
	if d == "" {
		return "", false
	}
	return d, true
}

// Path resolves name inside Dir(); ok=false when the dir is unset OR the file is
// absent, so callers can skip.
func Path(name string) (string, bool) {
	d, ok := Dir()
	if !ok {
		return "", false
	}
	p := filepath.Join(d, name)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}
