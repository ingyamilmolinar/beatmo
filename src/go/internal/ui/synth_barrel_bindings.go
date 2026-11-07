package ui

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Resolves the knob on every call so a re-layout is safe.
type synthValueBinding struct {
	dv     *DrumView
	idx    int
	instID instrument.ID
}

func (b synthValueBinding) knob() *Knob {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.instEditorControls) {
		return nil
	}
	return b.dv.instEditorControls[b.idx]
}

func (b synthValueBinding) def() (audio.ParamDef, bool) {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.instEditorBindings) {
		return audio.ParamDef{}, false
	}
	return b.dv.instEditorBindings[b.idx].def, true
}

// Without mutating the knob.
func (b synthValueBinding) realAt(offset int) (val, cur, step float64, def audio.ParamDef, ok bool) {
	k := b.knob()
	def, ok = b.def()
	if k == nil || !ok {
		return 0, 0, 0, def, false
	}
	span := def.Max - def.Min
	step = knobStepResolution(k, def)
	cur = def.Min + k.Value*span
	return cur + float64(offset)*step, cur, step, def, true
}

func (b synthValueBinding) labelAt(offset int) string {
	real, cur, step, def, ok := b.realAt(offset)
	if !ok {
		return ""
	}
	// Enum params show labels; the offset walks the enum index directly.
	if len(def.Enum) > 0 {
		k := b.knob()
		span := float64(len(def.Enum) - 1)
		idx := int(math.Round(k.Value*span)) + offset
		if idx < 0 || idx >= len(def.Enum) {
			return ""
		}
		return def.Enum[idx]
	}
	// canStep() refuses to pass Min/Max, so an out-of-range row would advertise a value the control can never take.
	if eps := math.Abs(step) * 1e-6; real < def.Min-eps || real > def.Max+eps {
		return ""
	}
	return formatSynthParamValueStep(def, real, labelStepFor(cur, def.Min, step))
}

// An off-grid value would let two rows a detent apart round to the same string and the wheel look frozen;
// step/10 buys one more decimal, so adjacent rows always differ. On-grid values keep the step's precision.
func labelStepFor(cur, min, step float64) float64 {
	if step <= 0 || math.IsInf(step, 0) || math.IsNaN(step) {
		return step
	}
	off := math.Mod(cur-min, step)
	if off < 0 {
		off += step
	}
	if off > step*1e-6 && off < step*(1-1e-6) {
		return step / 10
	}
	return step
}

func (b synthValueBinding) stepBy(detents int) bool {
	k := b.knob()
	if k == nil || detents == 0 {
		return false
	}
	dir := sign(detents)
	changed := false
	for i := 0; i < abs(detents); i++ {
		if k.StepValueDirect(dir) {
			changed = true
		}
	}
	return changed
}

func (b synthValueBinding) canStep(dir int) bool {
	k := b.knob()
	if k == nil {
		return false
	}
	if dir > 0 {
		return k.Value < 1.0
	}
	return k.Value > 0.0
}

// +1 detent = coarser. A nil badge (discrete param) means no lane.
type synthResolutionBinding struct {
	dv     *DrumView
	idx    int
	instID instrument.ID
}

func (b synthResolutionBinding) badge() *KnobStepBadge {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.instEditorStepBadges) {
		return nil
	}
	return b.dv.instEditorStepBadges[b.idx]
}

func (b synthResolutionBinding) available() bool { return b.badge() != nil }

func (b synthResolutionBinding) def() audio.ParamDef {
	if b.dv == nil || b.idx < 0 || b.idx >= len(b.dv.instEditorBindings) {
		return audio.ParamDef{}
	}
	return b.dv.instEditorBindings[b.idx].def
}

func (b synthResolutionBinding) labelAt(offset int) string {
	bg := b.badge()
	if bg == nil {
		return ""
	}
	i := bg.Index() + offset
	rungs := bg.Rungs()
	if i < 0 || i >= len(rungs) {
		return ""
	}
	return formatStepValue(b.def(), rungs[i])
}

func (b synthResolutionBinding) stepBy(detents int) bool {
	bg := b.badge()
	if bg == nil || detents == 0 {
		return false
	}
	// AdjustResolution(+1) is FINER, so coarser needs a negative delta.
	moved := bg.AdjustResolution(-detents)
	if moved {
		if k := (synthValueBinding{dv: b.dv, idx: b.idx, instID: b.instID}).knob(); k != nil {
			k.StepMul = bg.Step()
		}
		b.dv.persistKnobStep(bg)
	}
	return moved
}

func (b synthResolutionBinding) canStep(dir int) bool {
	bg := b.badge()
	if bg == nil {
		return false
	}
	i := bg.Index()
	// dir>0 = coarser = higher index; guard against the ladder ends.
	if dir > 0 {
		return i < bg.LadderLen()-1
	}
	return i > 0
}
