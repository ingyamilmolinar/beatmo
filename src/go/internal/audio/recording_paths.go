//go:build !js

package audio

import (
	"os"
	"path/filepath"
)

// recordingsBaseDir returns the base directory for recording output.
// Uses BEATMO_RECORDINGS_DIR when set, otherwise an absolute "recordings"
// subdirectory of the current working directory.
func recordingsBaseDir() string {
	if dir := os.Getenv("BEATMO_RECORDINGS_DIR"); dir != "" {
		return dir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "recordings"
	}
	return filepath.Join(cwd, "recordings")
}
