package audio

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// sanitizeParamValue is the single boundary that decides what to do with a
// param value the caller wants to write. Two failure modes are recognized:
//
//   - non-finite (NaN, +Inf, -Inf): rejected outright (ok=false). These
//     poison voice rendering — the decay=0 → NaN incident that motivated
//     this helper traveled through the channel's biquad EQ and required
//     the per-recipe clamp at drums.c:1736-1745 to recover.
//   - out-of-range: clamped to [def.Min, def.Max]. The UI slider already
//     enforces these bounds, but the JS bridge, JSON import, and the API
//     itself accept arbitrary float64. A single clamp here means every
//     entry point inherits the same contract.
//
// When def is the zero value (no schema available, e.g. an unknown
// recipe-name pair), only the finite check applies — out-of-range cannot
// be defined without a schema. Callers that have a ParamDef should always
// pass it; callers that don't (e.g. transient test recipes) get the
// permissive path.
func sanitizeParamValue(def ParamDef, v float64) (float64, bool) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	if def.Name == "" || def.Min == def.Max {
		// No schema bound to this name — accept the value verbatim. Min==Max
		// covers the degenerate ParamDef (test fixtures occasionally produce
		// these); clamping to a single point would change every input to
		// that point, which is rarely what the caller wants.
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

// lookupParamDef returns the ParamDef for (instrumentID, paramName) or the
// zero value when either the instrument is unbound or the recipe doesn't
// declare that name. Cheap enough to inline at every SetInstrumentParam
// call — both lookups (binding map, params slice) are O(1)/O(N) where N
// is small (≤ 8 params per recipe today).
func lookupParamDef(instrumentID instrument.ID, paramName string) ParamDef {
	return instrumentParamDef(instrumentID, paramName)
}
