package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// StartRecording begins an audio recording session using sane defaults.
// The hooks event EventRecordStart is published by the audio package.
func (g *Game) StartRecording() error {
	opts := audio.RecordingOptions{
		Format: audio.FormatWAV24,
	}
	if g.drum != nil {
		opts.BPM = g.drum.BPM()
	}
	return audio.StartRecording(opts)
}

// StopRecording finalizes the active recording session and returns the
// result. The hooks event EventRecordStop is published by the audio package.
func (g *Game) StopRecording() (*audio.RecordingResult, error) {
	return audio.StopRecording()
}

// IsRecording reports whether a recording session is currently active.
func (g *Game) IsRecording() bool {
	return audio.IsRecording()
}
