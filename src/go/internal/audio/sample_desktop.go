//go:build !test && !js

package audio

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/log"
)

// Sample represents a preloaded PCM buffer.
type Sample struct{ data []float32 }

// NewVoice returns a voice that plays the sample once.
func (s Sample) NewVoice(sampleRate int) Voice {
	return &cVoice{buf: s.data}
}

// RegisterAudio decodes a WAV/MP3 (or any miniaudio-supported file) and registers it as an instrument.
func RegisterAudio(id instrument.ID, path string) error {
	if path == "" {
		Register(id, Sample{data: make([]float32, sampleRate/10)})
		return nil
	}
	buf, sr, err := loadAudio(path)
	if err != nil {
		// Headless environments may lack the file; a silent placeholder keeps the id available for UI flows.
		log.Warnf("[audio] RegisterAudio decode failed for %q: %v (installing silent placeholder)", path, err)
		Register(id, Sample{data: make([]float32, sampleRate/20)})
		return nil
	}
	// Converted, never replaced by silence; the browser registers the same converted PCM.
	Register(id, Sample{data: pcmAtEngineRate(buf, sr, sampleRate)})
	return nil
}

// RegisterWAV remains for compatibility; it forwards to RegisterAudio.
func RegisterWAV(id instrument.ID, path string) error { return RegisterAudio(id, path) }

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
