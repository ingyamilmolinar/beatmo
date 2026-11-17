package audio

// baseFrequencyParamNames lists, in precedence order, the params that DECLARE
// an instrument's base frequency — the Hz its voice sounds at node pitch 0.
//
// This is the single source of truth for that question (cmd/gen-fundamentals
// and the pitch scanner both read it). Only the FM family declares one: its
// "Base Pitch" knob `gen1_fm_base`. Its schema identity is 0, so it only reads
// as declared on the instruments whose seed actually sets it. The other
// families' base frequency (voice_freq_hz) is DERIVED from the voice, so the
// generator measures them instead.
var baseFrequencyParamNames = []string{
	"gen1_fm_base",
}

// BaseFrequencyParamNames returns the declared-base-frequency params in
// precedence order, for tools outside this package (cmd/gen-fundamentals).
func BaseFrequencyParamNames() []string {
	return append([]string(nil), baseFrequencyParamNames...)
}

// declaredBaseFrequency returns the first positive declared base frequency in
// p, and the param it came from.
func declaredBaseFrequency(p RecipeParams) (float64, string) {
	for _, k := range baseFrequencyParamNames {
		if v, ok := p[k]; ok && v > 0 {
			return v, k
		}
	}
	return 0, ""
}
