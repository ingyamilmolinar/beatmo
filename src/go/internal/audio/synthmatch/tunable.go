package synthmatch

import (
	"fmt"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Param describes a single tunable parameter in the search space.
type Param struct {
	Key      string
	Min, Max float64
	Step     float64
	Discrete []float64 // if non-empty, candidate is snapped to one of these (overrides Min/Max/Step)
	Int      bool      // round to nearest integer
}

// TunableSpec describes the tunable search space for an instrument family.
type TunableSpec struct {
	Family string
	Params []Param
}

// So a typo, or a knob on a module the instrument lacks, cannot silently no-op.
func (s TunableSpec) ValidateKeys(instID instrument.ID) error {
	if _, ok := audio.EnsureInstrumentDoc(instID); !ok {
		return fmt.Errorf("synthmatch: instrument %q has no document", instID)
	}
	merged := audio.MergedInstrumentParamsRO(instID)
	var missing []string
	for _, p := range s.Params {
		if _, ok := merged[p.Key]; !ok {
			missing = append(missing, p.Key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("keys not found in the instrument document of %q: %s", instID, strings.Join(missing, ", "))
	}
	return nil
}

// Params is a defensive copy.
func SpecForInstrument(instID instrument.ID) (TunableSpec, bool) {
	spec, ok := specMap[instID]
	if !ok {
		return TunableSpec{}, false
	}
	spec.Params = append([]Param(nil), spec.Params...)
	return spec, true
}

// --- Family definitions ---

// Keys present in every member's document.
var subtractiveReedParams = []Param{
	{Key: "filter_cutoff", Min: 200, Max: 8000, Step: 100},
	{Key: "filter_resonance", Min: 0.5, Max: 3.0, Step: 0.1},
	// The box must contain every member's own values, or a self-rendered reference can never be matched and the optimizer
	// pays the residual down by mis-tuning other params.
	{Key: "amp_attack", Min: 0.001, Max: 0.6, Step: 0.005},
	{Key: "amp_decay", Min: 0.02, Max: 3.0, Step: 0.02},
	{Key: "amp_sustain", Min: 0, Max: 1, Step: 0.05},
	{Key: "amp_release", Min: 0.02, Max: 0.5, Step: 0.02},
	{Key: "filtenv_amt", Min: 0, Max: 6, Step: 0.2},
	{Key: "filtenv_decay", Min: 0.02, Max: 0.4, Step: 0.02},
	{Key: "lfo_rate", Min: 3, Max: 8, Step: 0.2},
	{Key: "lfo_depth", Min: 0, Max: 0.2, Step: 0.01},
	{Key: "gen1_gain", Min: 0, Max: 1, Step: 0.05},
	{Key: "gen1_filt_freq", Min: 400, Max: 4000, Step: 100},
}

// additive-organ family: organ.
var additiveOrganParams = []Param{
	{Key: "gen1_gain", Min: 0, Max: 1, Step: 0.05},
	{Key: "gen2_gain", Min: 0, Max: 1, Step: 0.05},
	{Key: "gen3_gain", Min: 0, Max: 1, Step: 0.05},
	{Key: "gen4_gain", Min: 0, Max: 1, Step: 0.05},
	{Key: "gen5_gain", Min: 0, Max: 1, Step: 0.05},
	{Key: "gen6_gain", Min: 0, Max: 1, Step: 0.05},
	{Key: "gen7_gain", Min: 0, Max: 1, Step: 0.05},
	{Key: "lfo_rate", Min: 3, Max: 8, Step: 0.2},
	{Key: "lfo_depth", Min: 0, Max: 0.2, Step: 0.01},
	{Key: "amp_attack", Min: 0.001, Max: 0.1, Step: 0.005},
	{Key: "amp_release", Min: 0.02, Max: 0.5, Step: 0.02},
	{Key: "filter_cutoff", Min: 200, Max: 8000, Step: 100},
}

// ks-pluck family: guitar-electric, guitar-nylon, guitar-steel.
var ksPluckParams = []Param{
	{Key: "gen1_ks_sustain", Min: 0.9, Max: 0.999, Step: 0.005},
	{Key: "gen1_ks_pluck", Min: 0.3, Max: 1.0, Step: 0.05},
	{Key: "filter_cutoff", Min: 200, Max: 8000, Step: 100},
	{Key: "filter_resonance", Min: 0.5, Max: 3.0, Step: 0.1},
	{Key: "amp_decay", Min: 0.02, Max: 1.0, Step: 0.02},
	{Key: "amp_release", Min: 0.02, Max: 0.5, Step: 0.02},
}

// Brightness, the amp decay/release, the upper-partial level and the sub-octave body.
var struckKeysParams = []Param{
	{Key: "filter_cutoff", Min: 2000, Max: 9000, Step: 250},
	{Key: "amp_attack", Min: 0.001, Max: 0.02, Step: 0.001},
	{Key: "amp_decay", Min: 0.5, Max: 3.5, Step: 0.1},
	{Key: "amp_release", Min: 0.1, Max: 0.6, Step: 0.05},
	{Key: "gen1_gain", Min: 0, Max: 0.8, Step: 0.05},
	{Key: "gen4_gain", Min: 0, Max: 0.9, Step: 0.05},
	{Key: "gain", Min: 0.5, Max: 1.0, Step: 0.05},
}

// specMap maps instrument IDs to their TunableSpec.
var specMap = map[instrument.ID]TunableSpec{
	// struck-keys family.
	roster.PianoGrand: {Family: "struck-keys", Params: struckKeysParams},

	// subtractive-reed family.
	roster.Sax:        {Family: "subtractive-reed", Params: subtractiveReedParams},
	roster.Oboe:       {Family: "subtractive-reed", Params: subtractiveReedParams},
	roster.Trumpet:    {Family: "subtractive-reed", Params: subtractiveReedParams},
	roster.FrenchHorn: {Family: "subtractive-reed", Params: subtractiveReedParams},
	roster.Violin:     {Family: "subtractive-reed", Params: subtractiveReedParams},
	roster.Cello:      {Family: "subtractive-reed", Params: subtractiveReedParams},
	roster.Flute:      {Family: "subtractive-reed", Params: subtractiveReedParams},

	// additive-organ family.
	roster.Organ: {Family: "additive-organ", Params: additiveOrganParams},

	// ks-pluck family.
	roster.GuitarElectric: {Family: "ks-pluck", Params: ksPluckParams},
	roster.GuitarNylon:    {Family: "ks-pluck", Params: ksPluckParams},
	roster.GuitarSteel:    {Family: "ks-pluck", Params: ksPluckParams},
}
