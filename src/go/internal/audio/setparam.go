package audio

import "math"

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
func lookupParamDef(instrumentID, paramName string) ParamDef {
	recipeID := RecipeForInstrument(instrumentID)
	if recipeID == "" {
		return ParamDef{}
	}
	recipeRegMu.RLock()
	reg, ok := recipeRegMap[recipeID]
	recipeRegMu.RUnlock()
	if !ok {
		return ParamDef{}
	}
	for _, d := range reg.Params {
		if d.Name == paramName {
			return d
		}
	}
	return ParamDef{}
}

// sendFXParamDefs is the schema for the global send-FX configuration knobs.
// Pinned in code (not in the recipe registry) because send FX are not per-
// instrument recipes — they are global processors with a fixed shape. The
// ranges match the C clamps in effects.c and the practical bounds the UI
// slider widget shows (and the maxSendDelayMs cap defined in send_effects.go).
var sendFXParamDefs = map[string]ParamDef{
	"delay.time_ms":    {Name: "delay.time_ms", Min: 1, Max: 2000, Default: 300, Unit: "ms"},
	"delay.feedback":   {Name: "delay.feedback", Min: 0, Max: 1, Default: 0.3},
	"delay.damping_hz": {Name: "delay.damping_hz", Min: 100, Max: 20000, Default: 3000, Unit: "Hz"},
	"reverb.room":      {Name: "reverb.room", Min: 0, Max: 1, Default: 0.7},
	"reverb.damping":   {Name: "reverb.damping", Min: 0, Max: 1, Default: 0.4},
	"reverb.wet":       {Name: "reverb.wet", Min: 0, Max: 1, Default: 0.3},
}

// SendFXParamDef returns the ParamDef for a global send-FX knob, or the zero
// value for unknown names. Used by the UI to render sliders without
// hard-coding ranges in two places.
func SendFXParamDef(name string) ParamDef {
	return sendFXParamDefs[name]
}

// SendFXParamNames returns the canonical ordered name list for the send-FX
// schema. Stable across calls so the UI can render a consistent layout.
func SendFXParamNames() []string {
	return []string{
		"delay.time_ms",
		"delay.feedback",
		"delay.damping_hz",
		"reverb.room",
		"reverb.damping",
		"reverb.wet",
	}
}

// SetSendDelayParam updates one knob of the global send delay. value is
// sanitized through the same clamp+finite path as SetInstrumentParam, then
// dispatched to ConfigureSendDelay. Returns the stored value so callers can
// reflect the clamped result back to the UI without re-reading state.
// Unknown names are a no-op so future plan phases can add knobs without
// breaking older callers.
func SetSendDelayParam(name string, value float64) (float64, bool) {
	def := sendFXParamDefs[name]
	stored, ok := sanitizeParamValue(def, value)
	if !ok {
		return 0, false
	}
	t, f, d := SendDelayParams()
	switch name {
	case "delay.time_ms":
		t = stored
	case "delay.feedback":
		f = stored
	case "delay.damping_hz":
		d = stored
	default:
		return 0, false
	}
	ConfigureSendDelay(t, f, d)
	return stored, true
}

// SetSendReverbParam updates one knob of the global send reverb. Same
// sanitize-then-dispatch contract as SetSendDelayParam.
func SetSendReverbParam(name string, value float64) (float64, bool) {
	def := sendFXParamDefs[name]
	stored, ok := sanitizeParamValue(def, value)
	if !ok {
		return 0, false
	}
	r, d, w := SendReverbParams()
	switch name {
	case "reverb.room":
		r = stored
	case "reverb.damping":
		d = stored
	case "reverb.wet":
		w = stored
	default:
		return 0, false
	}
	ConfigureSendReverb(r, d, w)
	return stored, true
}
