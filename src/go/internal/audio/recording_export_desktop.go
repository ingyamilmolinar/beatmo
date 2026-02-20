//go:build !js && !test

package audio

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// recordingsBaseDir returns the base directory for recording output.
// Returns an absolute path: uses BEATMO_RECORDINGS_DIR if set, otherwise
// resolves "recordings" relative to the current working directory.
func recordingsBaseDir() string {
	if dir := os.Getenv("BEATMO_RECORDINGS_DIR"); dir != "" {
		return dir
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "recordings" // fallback to relative if Getwd fails
	}
	return filepath.Join(cwd, "recordings")
}

// SaveRecording writes all encoded channels to a timestamped subdirectory.
// Returns the output directory path.
func SaveRecording(result *RecordingResult) (string, error) {
	if result == nil || len(result.Channels) == 0 {
		return "", fmt.Errorf("no channels to save")
	}

	base := recordingsBaseDir()
	dir := filepath.Join(base, result.Metadata.Timestamp)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Write each channel file
	for _, ch := range result.Channels {
		path := filepath.Join(dir, ch.Filename)
		if err := os.WriteFile(path, ch.Data, 0644); err != nil {
			return "", fmt.Errorf("failed to write %s: %w", path, err)
		}
		log.Printf("[RECORDING] Saved %s (%d bytes)", path, len(ch.Data))
	}

	// Write session metadata
	metaJSON, err := result.MetadataJSON()
	if err != nil {
		log.Printf("[RECORDING] Warning: failed to marshal metadata: %v", err)
	} else {
		metaPath := filepath.Join(dir, "session.json")
		if err := os.WriteFile(metaPath, metaJSON, 0644); err != nil {
			log.Printf("[RECORDING] Warning: failed to write session.json: %v", err)
		}
	}

	log.Printf("[RECORDING] Saved %d channels to %s", len(result.Channels), dir)
	return dir, nil
}
