//go:build !test && !js

package audio

import (
	"fmt"
	"strings"

	"github.com/ncruces/zenity"
	"github.com/sqweek/dialog"
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

// RegisterWAVDialog opens a file selector, asks for a short name, and registers the chosen WAV.
func RegisterWAVDialog() (string, error) {
	path, err := dialog.File().Filter("WAV files", "wav").Load()
	if err != nil {
		return "", err
	}
	if !strings.HasSuffix(strings.ToLower(path), ".wav") {
		return "", fmt.Errorf("selected file is not a .wav: %s", path)
	}
	name, err := zenity.Entry("Instrument name?", zenity.Title("Instrument name"))
	if err != nil {
		return "", err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("instrument name is required")
	}
	if err := RegisterWAV(name, path); err != nil {
		return "", err
	}
	return name, nil
}
