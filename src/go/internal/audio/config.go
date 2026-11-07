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

const DefaultDurationSec = 0.5

const DefaultAmplitude float32 = 0.8

// A default config when not found.
func ConfigForInstrument(id instrument.ID) InstrumentConfig {
	// A synth's render metadata lives in its document (loudness already in Amplitude), never in a table.
	if d, ok := InstrumentDoc(id); ok {
		return InstrumentConfig{ID: id, DurationSec: d.Render.Seconds, Amplitude: float32(d.Render.Amplitude)}
	}
	cfg := InstrumentConfig{ID: id, DurationSec: DefaultDurationSec, Amplitude: DefaultAmplitude}
	if amp, ok := loudnessAmp(id); ok {
		cfg.Amplitude = amp
	}
	return cfg
}
