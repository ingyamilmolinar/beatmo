package ui

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// The audio package publishes EventRecordStart.
func (g *Game) StartRecording() error {
	opts := audio.RecordingOptions{
		Format: audio.FormatWAV24,
	}
	if g.drum != nil {
		opts.BPM = g.drum.BPM()
	}
	return audio.StartRecording(opts)
}

// The audio package publishes EventRecordStop.
func (g *Game) StopRecording() (*audio.RecordingResult, error) {
	return audio.StopRecording()
}

// IsRecording reports whether a recording session is currently active.
func (g *Game) IsRecording() bool {
	return audio.IsRecording()
}
