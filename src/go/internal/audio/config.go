//go:build !test && !js

package audio

// InstrumentConfig defines rendering parameters for an instrument.
// This is the single source of truth used by both desktop and WASM.
type InstrumentConfig struct {
	ID          string
	DurationSec float64
	Amplitude   float32
	RenderFunc  string // C function name
}

// InstrumentConfigs maps instrument IDs to their rendering configuration.
// These values must match the RENDER_INFO in src/js/audio.js for cross-platform parity.
var InstrumentConfigs = map[string]InstrumentConfig{
	"snare":     {ID: "snare", DurationSec: 1.0, Amplitude: 0.8, RenderFunc: "render_snare"},
	"kick":      {ID: "kick", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_kick"},
	"hihat":     {ID: "hihat", DurationSec: 0.25, Amplitude: 0.8, RenderFunc: "render_hihat"},
	"tom":       {ID: "tom", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_tom"},
	"clap":      {ID: "clap", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_clap"},
	"cowbell":   {ID: "cowbell", DurationSec: 0.4, Amplitude: 0.8, RenderFunc: "render_cowbell"},
	"rimshot":   {ID: "rimshot", DurationSec: 0.3, Amplitude: 0.8, RenderFunc: "render_snare_rimshot"},
	"sidestick": {ID: "sidestick", DurationSec: 0.25, Amplitude: 0.8, RenderFunc: "render_snare_sidestick"},
	"kick-deep": {ID: "kick-deep", DurationSec: 0.8, Amplitude: 0.8, RenderFunc: "render_kick_deep"},
	"shaker":    {ID: "shaker", DurationSec: 0.3, Amplitude: 0.8, RenderFunc: "render_shaker"},
	"ride":      {ID: "ride", DurationSec: 1.0, Amplitude: 0.8, RenderFunc: "render_ride"},
	"crash":     {ID: "crash", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_crash"},
	"fm-bass":   {ID: "fm-bass", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_fm_bass"},
	"fm-bell":   {ID: "fm-bell", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_fm_bell"},
	"fm-lead":   {ID: "fm-lead", DurationSec: 1.0, Amplitude: 0.8, RenderFunc: "render_fm_lead"},
	"fm-epiano": {ID: "fm-epiano", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_fm_epiano"},
	"fm-pluck":  {ID: "fm-pluck", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_fm_pluck"},
	"modular":     {ID: "modular", DurationSec: 1.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"modular-pad": {ID: "modular-pad", DurationSec: 1.0, Amplitude: 0.8, RenderFunc: "render_modular"},
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
	if cfg, ok := InstrumentConfigs[id]; ok {
		return cfg
	}
	return InstrumentConfig{
		ID:          id,
		DurationSec: DefaultDurationSec,
		Amplitude:   DefaultAmplitude,
	}
}
