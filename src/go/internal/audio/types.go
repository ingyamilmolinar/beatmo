package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// Vol in [0,1], Pitch in semitones, Dur a multiplier (> 0). When (ctx seconds) is used only with HasWhen.
type BatchParam struct {
	ID      instrument.ID
	Vol     float64
	Pitch   float64
	Dur     float64
	When    float64
	HasWhen bool
	Row     int // parity identity (-1 for fire-and-forget)
	Abs     int
	// Before the dispatch floor clamp: When differs from Ideal exactly when the note was already late. 0 = unknown.
	Ideal float64
	// Without the lookahead, so the playback probe sees lookahead changes as timing jitter. 0 = unknown.
	Grid float64
	// > 0 so the lane can cancel or replace it before it starts; 0 = a preview, never touched by lane operations.
	VoiceID uint64
}
