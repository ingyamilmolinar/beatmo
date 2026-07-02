package synthmatch

import (
	"fmt"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
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

// ValidateKeys returns an error if any Param.Key is absent from the instrument's
// merged recipe defaults (so a typo can't silently no-op).
func (s TunableSpec) ValidateKeys(instID string) error {
	recipeID := audio.RecipeForInstrument(instID)
	if recipeID == "" {
		return fmt.Errorf("synthmatch: instrument %q is not registered", instID)
	}
	merged := audio.MergeRecipeDefaults(recipeID, audio.GetInstrumentParams(instID))
	var missing []string
	for _, p := range s.Params {
		if _, ok := merged[p.Key]; !ok {
			missing = append(missing, p.Key)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("keys not found in merged recipe for %q: %s", instID, strings.Join(missing, ", "))
	}
	return nil
}

// SpecForInstrument returns the tunable search space for instID (by family),
// ok=false if the instrument has no spec. The returned TunableSpec.Params is a
// defensive copy — callers cannot mutate the shared backing slice.
func SpecForInstrument(instID string) (TunableSpec, bool) {
	spec, ok := specMap[instID]
	if !ok {
		return TunableSpec{}, false
	}
	spec.Params = append([]Param(nil), spec.Params...)
	return spec, true
}

// --- Family definitions ---

// subtractive-reed family: sax, oboe, trumpet, french-horn, violin, cello, flute.
// Keys are drawn only from params present in all these instruments' merged recipe defaults.
var subtractiveReedParams = []Param{
	{Key: "filter_cutoff", Min: 200, Max: 8000, Step: 100},
	{Key: "filter_resonance", Min: 0.5, Max: 3.0, Step: 0.1},
	{Key: "amp_attack", Min: 0.001, Max: 0.1, Step: 0.005},
	{Key: "amp_decay", Min: 0.02, Max: 1.0, Step: 0.02},
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

// struck-keys family: piano-grand (struck-string keyboard — tune brightness,
// the amp decay/release envelope, the upper-partial level, and the sub-octave body).
// Keys are limited to those present in the piano recipe's merged defaults.
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
var specMap = map[string]TunableSpec{
	// struck-keys family.
	"piano-grand": {Family: "struck-keys", Params: struckKeysParams},

	// subtractive-reed family.
	"sax":         {Family: "subtractive-reed", Params: subtractiveReedParams},
	"oboe":        {Family: "subtractive-reed", Params: subtractiveReedParams},
	"trumpet":     {Family: "subtractive-reed", Params: subtractiveReedParams},
	"french-horn": {Family: "subtractive-reed", Params: subtractiveReedParams},
	"violin":      {Family: "subtractive-reed", Params: subtractiveReedParams},
	"cello":       {Family: "subtractive-reed", Params: subtractiveReedParams},
	"flute":       {Family: "subtractive-reed", Params: subtractiveReedParams},

	// additive-organ family.
	"organ": {Family: "additive-organ", Params: additiveOrganParams},

	// ks-pluck family.
	"guitar-electric": {Family: "ks-pluck", Params: ksPluckParams},
	"guitar-nylon":    {Family: "ks-pluck", Params: ksPluckParams},
	"guitar-steel":    {Family: "ks-pluck", Params: ksPluckParams},
}
