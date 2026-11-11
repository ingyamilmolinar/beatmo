// Package wavref locates local reference audio for tests and analysis tools.
//
// Reference recordings are LOCAL DEV TOOLING: they are never committed and
// never shipped, so there is no licence to clear -- any recording you have
// may serve as ground truth. Synth-*matching* tests inherently need real
// timbres (a generated sine can't stand in for a violin), so the audio lives
// outside the repo under one root (Root(): $BEATMO_WAV_REFS, default
// ~/.beatmo/wav-refs). Tests skip cleanly when a file is missing, so a clean
// checkout / CI stays green.
//
// No audio file is ever versioned: test audio that must exist everywhere is
// GENERATED (internal/wavgen / cmd/wavgen). TestNoAudioFilesInRepository
// (internal/deploy), the .gitignore and the pre-commit hook
// (scripts/precommit/no-audio-files.sh) enforce it.
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

// Path resolves name inside Root(); ok=false when the file is absent, so
// callers can skip.
func Path(name string) (string, bool) {
	p := filepath.Join(Root(), name)
	if _, err := os.Stat(p); err != nil {
		return "", false
	}
	return p, true
}
