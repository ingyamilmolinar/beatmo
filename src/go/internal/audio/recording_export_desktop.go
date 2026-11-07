//go:build !js && !test

package audio

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Streaming mode (production) already has the WAVs on disk, so this only writes session.json; in-memory mode (test
// fixtures) writes each channel's Data. Returns the recording's directory either way.
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
		log.Debugf("[record] saved %s (%d bytes)", path, len(ch.Data))
	}

	metaJSON, err := result.MetadataJSON()
	if err != nil {
		log.Warnf("[record] failed to marshal metadata: %v", err)
	} else {
		metaPath := filepath.Join(dir, "session.json")
		if err := os.WriteFile(metaPath, metaJSON, 0o644); err != nil {
			log.Warnf("[record] failed to write session.json: %v", err)
		}
	}

	log.Debugf("[record] saved %d channels to %s", len(result.Channels), dir)
	return dir, nil
}
