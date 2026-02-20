package audio

// synthParamInstruments lists instrument IDs that support parameterized rendering.
var synthParamInstruments = map[string]bool{
	"snare":   true,
	"kick":    true,
	"hihat":   true,
	"clap":    true,
	"tom":     true,
	"cowbell": true,
}

// SynthParamDefs returns the synth parameter definitions for an instrument,
// formatted as EffectParamDefs for compatibility with the FX panel UI.
// Returns nil if the instrument does not support parameterized rendering.
func SynthParamDefs(instrumentID string) []EffectParamDef {
	if !synthParamInstruments[instrumentID] {
		return nil
	}
	return []EffectParamDef{
		{Name: "pitch", Min: -12, Max: 12, Default: 0, Unit: "st"},
		{Name: "decay", Min: 0.25, Max: 4, Default: 0},
		{Name: "tone", Min: -1, Max: 1, Default: 0},
		{Name: "attack", Min: 0.25, Max: 4, Default: 0},
		{Name: "drive", Min: 0, Max: 1, Default: 0},
		{Name: "body", Min: 0, Max: 1, Default: 0},
		{Name: "color", Min: -1, Max: 1, Default: 0},
		{Name: "brightness", Min: 0, Max: 1, Default: 0},
	}
}

// HasSynthParams returns true if the instrument supports parameterized rendering.
func HasSynthParams(instrumentID string) bool {
	return synthParamInstruments[instrumentID]
}
