// Package wavref locates local reference audio for the analysis tools: LOCAL DEV TOOLING, never committed or shipped (so
// no licence), under Root() ($BEATMO_WAV_REFS, default ~/.beatmo/wav-refs). No test reads it; test audio that must exist
// everywhere is generated (internal/wavgen).
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
