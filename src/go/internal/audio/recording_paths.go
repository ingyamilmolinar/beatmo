//go:build !js

package audio

import (
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/env"
)

func recordingsBaseDir() string {
	if dir := env.RecordingsDir.Get(); dir != "" {
		return dir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "recordings"
	}
	return filepath.Join(cwd, "recordings")
}
