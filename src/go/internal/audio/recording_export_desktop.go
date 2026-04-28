//go:build !js && !test

package audio

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// SaveRecording finalizes a recording.
//
// In streaming mode (the production desktop path), the per-channel WAV
// files are already on disk in result.SessionDir — SaveRecording just
// writes session.json next to them.
//
// In legacy/in-memory mode (Data is populated, SessionDir is empty —
// e.g., test fixtures), SaveRecording writes each channel's Data into a
// timestamped subdirectory. Either way, the returned path is the
// directory containing the recording.
func SaveRecording(result *RecordingResult) (string, error) {
	if result == nil || len(result.Channels) == 0 {
		return "", fmt.Errorf("no channels to save")
	}

	dir := result.SessionDir
	if dir == "" {
		base := recordingsBaseDir()
		dir = filepath.Join(base, result.Metadata.Timestamp)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	for _, ch := range result.Channels {
		// Streaming mode: file already at ch.Path; nothing to write.
		if ch.Path != "" {
			continue
		}
		// Legacy mode: write encoded bytes.
		path := filepath.Join(dir, ch.Filename)
		if err := os.WriteFile(path, ch.Data, 0o644); err != nil {
			return "", fmt.Errorf("failed to write %s: %w", path, err)
		}
		log.Printf("[RECORDING] Saved %s (%d bytes)", path, len(ch.Data))
	}

	metaJSON, err := result.MetadataJSON()
	if err != nil {
		log.Printf("[RECORDING] Warning: failed to marshal metadata: %v", err)
	} else {
		metaPath := filepath.Join(dir, "session.json")
		if err := os.WriteFile(metaPath, metaJSON, 0o644); err != nil {
			log.Printf("[RECORDING] Warning: failed to write session.json: %v", err)
		}
	}

	log.Printf("[RECORDING] Saved %d channels to %s", len(result.Channels), dir)
	return dir, nil
}
