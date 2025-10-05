package audio

// BatchParam represents a single scheduled audio event for batch dispatch.
// ID is the instrument identifier; Vol is [0,1]. Pitch is in semitones.
// Dur is a duration multiplier (>0). When is the AudioContext time in seconds
// and is used only when HasWhen is true.
type BatchParam struct {
	ID      string
	Vol     float64
	Pitch   float64
	Dur     float64
	When    float64
	HasWhen bool
}
