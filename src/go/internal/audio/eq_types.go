package audio

// EQKind enumerates supported biquad shapes.
type EQKind int

const (
	EQPeaking EQKind = iota
	EQLowShelf
	EQHighShelf
	EQLowpass  // Butterworth lowpass for crossover filters
	EQHighpass // Butterworth highpass for crossover filters
)

// EQBand describes one filter stage.
type EQBand struct {
	Kind   EQKind
	Freq   float64
	Q      float64
	GainDB float64
	Muted  bool // When true, this band produces complete silence
}
