//go:build !test && !js

package audio

import (
	"fmt"
	"os/exec"
	"strings"
)

// Sample represents a preloaded PCM buffer.
type Sample struct{ data []float32 }

// NewVoice returns a voice that plays the sample once.
func (s Sample) NewVoice(bpm, sampleRate int) Voice {
	return &cVoice{buf: s.data}
}

// RegisterWAV decodes a .wav file and registers it as an instrument.
func RegisterWAV(id, path string) error {
	buf, sr, err := loadWav(path)
	if err != nil {
		return fmt.Errorf("load wav %s: %w", path, err)
	}
	if sr != sampleRate {
		return fmt.Errorf("expected %dHz wav, got %d", sampleRate, sr)
	}
	Register(id, Sample{data: buf})
	return nil
}

// SelectWAV opens a file picker and returns the chosen path.
func SelectWAV() (string, error) {
	pathBytes, err := exec.Command("zenity", "--file-selection", "--file-filter=*.wav").Output()
	if err != nil {
		return "", fmt.Errorf("failed to open file dialog: %w", err)
	}
	path := strings.TrimSpace(string(pathBytes))
	if path == "" {
		return "", fmt.Errorf("no file selected")
	}
	return path, nil
}
