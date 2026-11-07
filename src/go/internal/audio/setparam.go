package audio

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The single boundary: non-finite values are rejected (they poison rendering, the channel's biquads included) and
// out-of-range ones clamped, since the bridge and import accept any float64. With no schema only the finite check
// applies.
func sanitizeParamValue(def ParamDef, v float64) (float64, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	if def.Name == "" || def.Min == def.Max {
		// Min == Max is a degenerate def: clamping would turn every input into that point.
		return v, true
	}
	if v < def.Min {
		return def.Min, true
	}
	if v > def.Max {
		return def.Max, true
	}
	return v, true
}

// The zero value when the instrument is unbound or does not declare the name.
func lookupParamDef(instrumentID instrument.ID, paramName string) ParamDef {
	return instrumentParamDef(instrumentID, paramName)
}
