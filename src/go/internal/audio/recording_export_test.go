//go:build !test && !js

package audio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDesktopSaveRecordingCreatesFiles verifies that SaveRecording creates
// the correct directory structure with all channel files + session.json.
func TestDesktopSaveRecordingCreatesFiles(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("BEATMO_RECORDINGS_DIR", tmpDir)

	result := &RecordingResult{
		Channels: []EncodedChannel{
			{ID: "master", Name: "Master", Filename: "master.wav", Data: []byte("master-data")},
			{ID: "kick", Name: "Kick", Filename: "kick.wav", Data: []byte("kick-data")},
			{ID: "snare", Name: "Snare", Filename: "snare.wav", Data: []byte("snare-data")},
		},
		Metadata: SessionMetadata{
			BPM:        120,
			Duration:   5.0,
			SampleRate: 44100,
			Format:     FormatWAV24,
			Timestamp:  "2026-03-22_150000",
			Channels: []ChannelMeta{
				{ID: "master", Name: "Master", Filename: "master.wav", Samples: 220500},
				{ID: "kick", Name: "Kick", Filename: "kick.wav", Samples: 220500},
				{ID: "snare", Name: "Snare", Filename: "snare.wav", Samples: 220500},
			},
		},
	}

	savePath, err := SaveRecording(result)
	if err != nil {
		t.Fatalf("SaveRecording: %v", err)
	}

	// Verify path is absolute
	if !filepath.IsAbs(savePath) {
		t.Errorf("save path should be absolute, got %q", savePath)
	}

	// Verify path is under the configured directory
	expectedDir := filepath.Join(tmpDir, "2026-03-22_150000")
	if savePath != expectedDir {
		t.Errorf("save path = %q, want %q", savePath, expectedDir)
	}

	// Verify all channel files exist with correct content
	for _, ch := range result.Channels {
		filePath := filepath.Join(savePath, ch.Filename)
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Errorf("failed to read %s: %v", ch.Filename, err)
			continue
		}
		if string(data) != string(ch.Data) {
			t.Errorf("%s content mismatch: got %q, want %q", ch.Filename, string(data), string(ch.Data))
		}
	}

	// Verify session.json exists and is valid
	metaPath := filepath.Join(savePath, "session.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("failed to read session.json: %v", err)
	}

	var meta SessionMetadata
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("failed to parse session.json: %v", err)
	}
	if meta.BPM != 120 {
		t.Errorf("session.json BPM = %d, want 120", meta.BPM)
	}
	if len(meta.Channels) != 3 {
		t.Errorf("session.json channels = %d, want 3", len(meta.Channels))
	}
}

// TestDesktopSaveRecordingDefaultPath verifies that without
// BEATMO_RECORDINGS_DIR, recordings go to a "recordings" subdirectory
// in the current working directory, using an absolute path.
func TestDesktopSaveRecordingDefaultPath(t *testing.T) {
	// Ensure env var is not set
	t.Setenv("BEATMO_RECORDINGS_DIR", "")

	base := recordingsBaseDir()
	if !filepath.IsAbs(base) {
		t.Errorf("default recordings path should be absolute, got %q", base)
	}

	// Should end with "recordings"
	if filepath.Base(base) != "recordings" {
		t.Errorf("default path should end with 'recordings', got %q", filepath.Base(base))
	}
}

// TestDesktopSaveRecordingEnvVarPath verifies that BEATMO_RECORDINGS_DIR
// is used when set.
func TestDesktopSaveRecordingEnvVarPath(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("BEATMO_RECORDINGS_DIR", tmpDir)

	base := recordingsBaseDir()
	if base != tmpDir {
		t.Errorf("recordingsBaseDir() = %q, want %q", base, tmpDir)
	}
}
