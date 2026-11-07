package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Every surface formats a knob value through these, so the same value never reads differently in two places.

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

func formatKnobDisplay(def audio.ParamDef, norm float64) string {
	return formatKnobDisplayStep(def, norm, math.Inf(1))
}

// An infinite step reproduces formatKnobDisplay.
func formatKnobDisplayStep(def audio.ParamDef, norm, step float64) string {
	if len(def.Enum) > 0 {
		return def.Enum[enumIndex(norm, len(def.Enum))]
	}
	return formatSynthParamValueStep(def, def.Min+norm*(def.Max-def.Min), step)
}

// Falls back to span/200 when StepMul is unset, matching the wheel, so the pill and the wheel centre agree.
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

// At the active step resolution, so a fine resolution shows the decimals the user dialed in.
func knobPillLabel(k *Knob, def audio.ParamDef) string {
	if k == nil {
		return formatKnobDisplay(def, 0)
	}
	return formatKnobDisplayStep(def, k.Value, knobStepResolution(k, def))
}
