package audio

import (
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Both platforms run every effect through the same C host, so this list is a product choice (which effects belong to a
// SOUND rather than a row's FX panel), not a platform limit.
func voiceFXTypes() []EffectType {
	out := []EffectType{EffectDistortion, EffectBitcrusher, EffectFilter}
	slices.Sort(out)
	return out
}

// COMPLETE params (defaults filled); the C host orders them itself.
type VoiceFXCall struct {
	Type   string
	Params map[string]float64
}

// VoiceFXCalls lists d's enabled voice FX in order.
func VoiceFXCalls(d instrument.Doc) []VoiceFXCall {
	var out []VoiceFXCall
	for _, m := range d.FX {
		t := EffectType(m.Type[len("fx."):])
		if !m.On || !slices.Contains(voiceFXTypes(), t) {
			continue
		}
		out = append(out, VoiceFXCall{Type: string(t), Params: mergeDefaults(t, m.Values)})
	}
	return out
}
