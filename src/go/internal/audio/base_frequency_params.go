package audio

// The params that DECLARE the Hz a voice sounds at node pitch 0, in precedence order. Only FM declares one
// (gen1_fm_base, identity 0, so it counts only where a seed sets it); other families' base is derived and
// measured.
var baseFrequencyParamNames = []string{
	"gen1_fm_base",
}

func BaseFrequencyParamNames() []string {
	return append([]string(nil), baseFrequencyParamNames...)
}

func declaredBaseFrequency(p RecipeParams) (float64, string) {
	for _, k := range baseFrequencyParamNames {
		if v, ok := p[k]; ok && v > 0 {
			return v, k
		}
	}
	return 0, ""
}
