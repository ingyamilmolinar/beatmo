package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// BatchParam represents a single scheduled audio event for batch dispatch.
// ID is the instrument identifier; Vol is [0,1]. Pitch is in semitones.
// Dur is a duration multiplier (>0). When is the AudioContext time in seconds
// and is used only when HasWhen is true.
type BatchParam struct {
	ID      instrument.ID
	Vol     float64
	Pitch   float64
	Dur     float64
	When    float64
	HasWhen bool
	Row     int // parity identity (-1 for fire-and-forget)
	Abs     int
	// Ideal is the scheduler's requested start (ctx seconds) BEFORE the
	// dispatch-time floor clamp (audioLoop's MinDispatchLeadSec). When differs
	// from Ideal exactly when the note was already late on dispatch. 0 = unknown.
	Ideal float64
	// Grid is the predictor's trigger time for this beat (ctx seconds): the
	// ideal grid time plus groove, WITHOUT the lookahead cushion. The playback
	// probe (audio_probe_worklet.js) measures audible onsets against it, so
	// lookahead changes mid-play show up as timing jitter. 0 = unknown.
	Grid float64
	// VoiceID identifies an audio-lane voice (> 0) so it can be cancelled or
	// replaced before it starts (CancelVoices, CancelAllLaneFuture). 0 = a
	// preview / UI sound, never touched by lane operations.
	VoiceID uint64
}
