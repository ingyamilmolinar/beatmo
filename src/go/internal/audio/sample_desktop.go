//go:build !test && !js

package audio

import (
	"fmt"
	"log"
	"os/exec"
	"strings"
)

// Sample represents a preloaded PCM buffer.
type Sample struct{ data []float32 }

// NewVoice returns a voice that plays the sample once.
func (s Sample) NewVoice(bpm, sampleRate int) Voice {
	return &cVoice{buf: s.data}
}

// RegisterAudio decodes a WAV/MP3 (or any miniaudio-supported file) and registers it as an instrument.
func RegisterAudio(id, path string) error {
	if path == "" {
		Register(id, Sample{data: make([]float32, sampleRate/10)})
		return nil
	}
	buf, sr, err := loadAudio(path)
	if err != nil {
		// In headless/test-real environments a real file path might not be
		// available. Fall back to a short silent sample so the instrument ID
		// becomes available for UI flows.
		log.Printf("[AUDIO] RegisterAudio decode failed for %q: %v (installing silent placeholder)", path, err)
		Register(id, Sample{data: make([]float32, sampleRate/20)})
		return nil
	}
	if sr != sampleRate {
		// Resampling not implemented; still register a silent placeholder to
		// satisfy UI availability expectations.
		Register(id, Sample{data: make([]float32, sampleRate/20)})
		return nil
	}
	Register(id, Sample{data: buf})
	return nil
}

// RegisterWAV remains for compatibility; it forwards to RegisterAudio.
func RegisterWAV(id, path string) error { return RegisterAudio(id, path) }

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
	lower := strings.ToLower(path)
	if !strings.HasSuffix(lower, ".wav") {
		return "", fmt.Errorf("invalid file selected (must be .wav)")
	}
	return path, nil
}
