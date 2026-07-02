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
	"snare":       {ID: "snare", DurationSec: 1.0, Amplitude: 0.8, RenderFunc: "render_snare"},
	"kick":        {ID: "kick", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_kick"},
	"hihat":       {ID: "hihat", DurationSec: 0.25, Amplitude: 0.8, RenderFunc: "render_hihat"},
	"tom":         {ID: "tom", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_tom"},
	"clap":        {ID: "clap", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_clap"},
	"cowbell":     {ID: "cowbell", DurationSec: 0.4, Amplitude: 0.8, RenderFunc: "render_cowbell"},
	"rimshot":     {ID: "rimshot", DurationSec: 0.3, Amplitude: 0.8, RenderFunc: "render_snare_rimshot"},
	"sidestick":   {ID: "sidestick", DurationSec: 0.25, Amplitude: 0.8, RenderFunc: "render_snare_sidestick"},
	"kick-deep":   {ID: "kick-deep", DurationSec: 0.8, Amplitude: 0.8, RenderFunc: "render_kick_deep"},
	"shaker":      {ID: "shaker", DurationSec: 0.3, Amplitude: 0.8, RenderFunc: "render_shaker"},
	"ride":        {ID: "ride", DurationSec: 2.5, Amplitude: 0.8, RenderFunc: "render_ride"},
	"crash":       {ID: "crash", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_crash"},
	"fm-bass":     {ID: "fm-bass", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_fm_bass"},
	"fm-bell":     {ID: "fm-bell", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_fm_bell"},
	"fm-lead":     {ID: "fm-lead", DurationSec: 1.0, Amplitude: 0.8, RenderFunc: "render_fm_lead"},
	"fm-epiano":   {ID: "fm-epiano", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_fm_epiano"},
	"fm-pluck":    {ID: "fm-pluck", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_fm_pluck"},
	"modular":     {ID: "modular", DurationSec: 1.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"modular-pad": {ID: "modular-pad", DurationSec: 1.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	// Bowed strings family.
	"violin":          {ID: "violin", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"violin-ensemble": {ID: "violin-ensemble", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"cello":           {ID: "cello", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"cello-warm":      {ID: "cello-warm", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"organ-church":    {ID: "organ-church", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"scifi-lead":      {ID: "scifi-lead", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	// Plucked strings — guitars (Task 2, 1.5s).
	"guitar-nylon":         {ID: "guitar-nylon", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"guitar-nylon-bright":  {ID: "guitar-nylon-bright", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"guitar-steel":         {ID: "guitar-steel", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"guitar-steel-warm":    {ID: "guitar-steel-warm", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"guitar-electric":      {ID: "guitar-electric", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"harp":                 {ID: "harp", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"guitar-electric-neck": {ID: "guitar-electric-neck", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	// Keys — piano (Task 3, 2.0s).
	"piano-grand": {ID: "piano-grand", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"piano-felt":  {ID: "piano-felt", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	// Woodwind (Task 4, 2.0s).
	"flute":         {ID: "flute", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"flute-breathy": {ID: "flute-breathy", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"oboe":          {ID: "oboe", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"oboe-full":     {ID: "oboe-full", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	// Brass (Task 5, 2.0s).
	"trumpet":          {ID: "trumpet", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"trumpet-mellow":   {ID: "trumpet-mellow", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"french-horn":      {ID: "french-horn", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	"french-horn-loud": {ID: "french-horn-loud", DurationSec: 2.0, Amplitude: 0.8, RenderFunc: "render_modular"},
	// Bass guitar (renamed from synth-bass) + synth bass family (1.5s).
	"bass-guitar": {ID: "bass-guitar", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"bass-acid":   {ID: "bass-acid", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"bass-reese":  {ID: "bass-reese", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"bass-fm":     {ID: "bass-fm", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"bass-808":    {ID: "bass-808", DurationSec: 1.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	// Modal conga (Task 7, 0.5s — short percussion).
	"conga":       {ID: "conga", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"conga-open":  {ID: "conga-open", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"conga-tumba": {ID: "conga-tumba", DurationSec: 0.6, Amplitude: 0.8, RenderFunc: "render_modular"},
	// Masterpiece template set (organ/sax). Organ is fully sustained, so
	// it carries a lower amplitude for headroom under reverb sends.
	"organ": {ID: "organ", DurationSec: 2.0, Amplitude: 0.65, RenderFunc: "render_modular"},
	"sax":   {ID: "sax", DurationSec: 2.0, Amplitude: 0.78, RenderFunc: "render_modular"},
	// Configurable KICK stage family (short one-shot percussion).
	"dnb-kick":      {ID: "dnb-kick", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"kick-electro":  {ID: "kick-electro", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"kick-808":      {ID: "kick-808", DurationSec: 0.75, Amplitude: 0.8, RenderFunc: "render_modular"},
	"kick-acoustic": {ID: "kick-acoustic", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_modular"},
	"kick-punchy":   {ID: "kick-punchy", DurationSec: 0.5, Amplitude: 0.8, RenderFunc: "render_modular"},
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
