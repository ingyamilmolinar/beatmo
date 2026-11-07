package ui

// One eased progress drives the reflow: each rect lerps from rest to reflowed by smoothstep(armGrowT). The
// subject is retained through the shrink so the collapse animates after the control is unarmed.

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// ~100–120 ms at 60 fps; deterministic so tests tick a bounded number of frames.
const armGrowStep = 0.16

func smoothstep(t float64) float64 {
	t = clamp01(t)
	return t * t * (3 - 2*t)
}

func lerpInt(a, b int, t float64) int {
	return a + int(float64(b-a)*t+0.5)
}

func lerpRect(a, b image.Rectangle, t float64) image.Rectangle {
	if t <= 0 {
		return a
	}
	if t >= 1 {
		return b
	}
	return image.Rect(
		lerpInt(a.Min.X, b.Min.X, t), lerpInt(a.Min.Y, b.Min.Y, t),
		lerpInt(a.Max.X, b.Max.X, t), lerpInt(a.Max.Y, b.Max.Y, t),
	)
}

// A new subject restarts from rest. scope separates the Synth and Sampler widgets: both index their own slice,
// so an unscoped subject would let one tab drive the other's reflow.
func (dv *DrumView) setArmGrowSubject(scope armedScope, instID instrument.ID, idx int, target float64) {
	if dv.armGrowIdx != idx || dv.armGrowInst != instID || dv.armGrowScope != scope {
		dv.armGrowT = 0
	}
	dv.armGrowIdx = idx
	dv.armGrowInst = instID
	dv.armGrowScope = scope
	dv.armGrowTarget = target
}

// Layout guards on this, not synthControlArmed, so the reflow keeps running through the shrink.
func (dv *DrumView) armGrowControl(scope armedScope, instID instrument.ID) (int, bool) {
	return dv.armGrowIdx, dv.armGrowIdx >= 0 && dv.armGrowInst != "" &&
		dv.armGrowInst == instID && dv.armGrowScope == scope
}

// armGrowProgress is the eased 0..1 grow amount for the current subject.
func (dv *DrumView) armGrowProgress() float64 { return smoothstep(dv.armGrowT) }

// Clears the subject once a shrink settles.
func (dv *DrumView) advanceArmGrow() bool {
	if dv.armGrowIdx < 0 {
		return false
	}
	if dv.armGrowT == dv.armGrowTarget {
		if dv.armGrowTarget == 0 {
			dv.armGrowIdx = -1
			dv.armGrowInst = ""
		}
		return false
	}
	if dv.armGrowT < dv.armGrowTarget {
		dv.armGrowT = clamp01(dv.armGrowT + armGrowStep)
	} else {
		dv.armGrowT = clamp01(dv.armGrowT - armGrowStep)
	}
	if dv.armGrowT == dv.armGrowTarget && dv.armGrowTarget == 0 {
		dv.armGrowIdx = -1
		dv.armGrowInst = ""
	}
	return true
}
