package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// knob_format.go — formatting a *Knob's normalized value for display.
//
// These helpers are independent of any particular control VIEW (they were
// previously housed in the retired mobile wheel popup's renderer): the synth
// ParamCard, the armed barrel lanes, and the sampler card all format through
// them, so a value shown in one place can never disagree with the same value
// shown in another.

// enumIndex maps a normalized [0,1] value to a clamped enum index.
func enumIndex(norm float64, n int) int {
	if n <= 0 {
		return 0
	}
	idx := int(math.Round(norm * float64(n-1)))
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}

// formatKnobDisplay formats a normalized knob value for display: the enum label
// for discrete params, otherwise the unit-aware numeric label at natural
// precision. Shared by the inline mobile value pill.
func formatKnobDisplay(def audio.ParamDef, norm float64) string {
	return formatKnobDisplayStep(def, norm, math.Inf(1))
}

// formatKnobDisplayStep is the resolution-aware sibling of formatKnobDisplay: it
// renders the numeric value with the decimals the active `step` (real param
// units) requires. An infinite step reproduces formatKnobDisplay.
func formatKnobDisplayStep(def audio.ParamDef, norm, step float64) string {
	if len(def.Enum) > 0 {
		return def.Enum[enumIndex(norm, len(def.Enum))]
	}
	return formatSynthParamValueStep(def, def.Min+norm*(def.Max-def.Min), step)
}

// knobStepResolution returns a knob's active step in real param units — its
// StepMul, falling back to span/200 when unset — matching the wheel's value
// step. Shared so the inline pill and the wheel center render at one resolution.
func knobStepResolution(k *Knob, def audio.ParamDef) float64 {
	if k == nil {
		return 0
	}
	step := k.StepMul
	if step <= 0 {
		step = (def.Max - def.Min) / 200
	}
	return step
}

// knobPillLabel formats a knob's current value for the inline mobile pill/button
// at the knob's active step resolution, so a fine resolution shows the extra
// decimals the user dialed in (matching the wheel center). Enum-aware.
func knobPillLabel(k *Knob, def audio.ParamDef) string {
	if k == nil {
		return formatKnobDisplay(def, 0)
	}
	return formatKnobDisplayStep(def, k.Value, knobStepResolution(k, def))
}
