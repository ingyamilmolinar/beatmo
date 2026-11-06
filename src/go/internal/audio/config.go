package audio

import (
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// InstrumentConfig defines rendering parameters for an instrument.
type InstrumentConfig struct {
	ID          instrument.ID
	DurationSec float64
	Amplitude   float32
}

// DefaultDurationSec is the duration of an instrument without a document
// (a user WAV sample).
const DefaultDurationSec = 0.5

// DefaultAmplitude is the amplitude of an instrument without a document or a
// recorded runtime loudness.
const DefaultAmplitude float32 = 0.8

// ConfigForInstrument returns the config for the given instrument ID.
// Returns a default config if the instrument is not found.
func ConfigForInstrument(id instrument.ID) InstrumentConfig {
	// A synth instrument's render metadata lives in its document (loudness
	// already folded into Amplitude), never in a table.
	if d, ok := InstrumentDoc(id); ok {
		return InstrumentConfig{ID: id, DurationSec: d.Render.Seconds, Amplitude: float32(d.Render.Amplitude)}
	}
	cfg := InstrumentConfig{ID: id, DurationSec: DefaultDurationSec, Amplitude: DefaultAmplitude}
	if amp, ok := loudnessAmp(id); ok {
		cfg.Amplitude = amp
	}
	return cfg
}
