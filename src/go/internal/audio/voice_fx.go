package audio

import (
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// voiceFXTypes is the voice FX a document may carry, in a stable order.
// Both platforms run every insert effect through the same C host (fx_host.c:
// desktop via cgo, the browser render worker via voice_post.js), so the list
// is a product choice — which effects belong to a SOUND rather than to a row's
// FX panel — not a platform limit.
func voiceFXTypes() []EffectType {
	out := []EffectType{EffectDistortion, EffectBitcrusher, EffectFilter}
	slices.Sort(out)
	return out
}

// VoiceFXCall is one enabled voice FX module as both platforms apply it to a
// rendered voice: the effect type and its COMPLETE parameter set (defaults
// filled), which the C host orders itself.
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
