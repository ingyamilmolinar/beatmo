//go:build test

package audio

// InstrumentConfig defines rendering parameters for an instrument.
// This is the single source of truth used by both desktop and WASM.
type InstrumentConfig struct {
	ID          string
	DurationSec float64
	Amplitude   float32
}

// InstrumentConfigs maps instrument IDs to their rendering configuration.
// These values must match the RENDER_INFO in src/js/audio.js for cross-platform parity.
var InstrumentConfigs = map[string]InstrumentConfig{
	"snare":       {ID: "snare", DurationSec: 1.0, Amplitude: 0.8},
	"kick":        {ID: "kick", DurationSec: 0.5, Amplitude: 0.8},
	"hihat":       {ID: "hihat", DurationSec: 0.25, Amplitude: 0.8},
	"tom":         {ID: "tom", DurationSec: 0.5, Amplitude: 0.8},
	"clap":        {ID: "clap", DurationSec: 0.5, Amplitude: 0.8},
	"cowbell":     {ID: "cowbell", DurationSec: 0.4, Amplitude: 0.8},
	"rimshot":     {ID: "rimshot", DurationSec: 0.3, Amplitude: 0.8},
	"sidestick":   {ID: "sidestick", DurationSec: 0.25, Amplitude: 0.8},
	"kick-deep":   {ID: "kick-deep", DurationSec: 0.8, Amplitude: 0.8},
	"shaker":      {ID: "shaker", DurationSec: 0.3, Amplitude: 0.8},
	"ride":        {ID: "ride", DurationSec: 1.0, Amplitude: 0.8},
	"crash":       {ID: "crash", DurationSec: 1.5, Amplitude: 0.8},
	"fm-bass":     {ID: "fm-bass", DurationSec: 1.5, Amplitude: 0.8},
	"fm-bell":     {ID: "fm-bell", DurationSec: 2.0, Amplitude: 0.8},
	"fm-lead":     {ID: "fm-lead", DurationSec: 1.0, Amplitude: 0.8},
	"fm-epiano":   {ID: "fm-epiano", DurationSec: 2.0, Amplitude: 0.8},
	"fm-pluck":    {ID: "fm-pluck", DurationSec: 0.5, Amplitude: 0.8},
	"modular":     {ID: "modular", DurationSec: 1.0, Amplitude: 0.8},
	"modular-pad": {ID: "modular-pad", DurationSec: 1.0, Amplitude: 0.8},
}

// DefaultDurationSec is the fallback duration for unknown instruments.
const DefaultDurationSec = 0.5

// DefaultAmplitude is the fallback amplitude for unknown instruments.
const DefaultAmplitude float32 = 0.8

// MixHeadroom is the gain reduction applied during mixing to prevent clipping.
// Value of 0.25 equals -12dB, allowing ~4 voices at full amplitude.
const MixHeadroom = 0.25

// ConfigForInstrument returns the config for the given instrument ID.
// Returns a default config if the instrument is not found.
func ConfigForInstrument(id string) InstrumentConfig {
	cfg, ok := InstrumentConfigs[id]
	if !ok {
		cfg = InstrumentConfig{
			ID:          id,
			DurationSec: DefaultDurationSec,
			Amplitude:   DefaultAmplitude,
		}
	}
	if amp, ok := loudnessAmp(id); ok {
		cfg.Amplitude = amp
	}
	return cfg
}
