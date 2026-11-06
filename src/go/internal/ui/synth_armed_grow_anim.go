package ui

// synth_armed_grow_anim.go — the grow/shrink animation state for the armed
// control's 2×2 card. A single eased progress `armGrowT` (advanced once per frame
// in synthTabUpdate, the same loop that ticks the barrel kinetics) drives the
// reflow in growArmedCard: every affected control's rect is the lerp of its
// resting slot and its reflowed slot by smoothstep(armGrowT). The subject is
// retained through the shrink (armGrowTarget 0) so the collapse animates after
// the control is no longer "armed", then cleared once it reaches rest.

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// armGrowStep is the per-frame progress delta (~6–7 frames ≈ 100–120 ms at 60 fps).
// Deterministic so tests can tick a bounded number of frames.
const armGrowStep = 0.16

// smoothstep is the classic 3t²−2t³ ease-in-out on [0,1]. (clamp01 lives in
// param_card.go.)
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

// setArmGrowSubject points the grow animation at (instID, idx) and aims it at
// target (1 = grow to armed, 0 = shrink to rest). A change of subject restarts
// the progress from rest so the new control eases in from scratch.
// scope distinguishes the Synth tab's controls from the Sampler tab's: both
// index into their own widget slice, so the same instID means different things
// in each and an unscoped subject would let one tab drive the other's reflow.
func (dv *DrumView) setArmGrowSubject(scope armedScope, instID instrument.ID, idx int, target float64) {
	if dv.armGrowIdx != idx || dv.armGrowInst != instID || dv.armGrowScope != scope {
		dv.armGrowT = 0
	}
	dv.armGrowIdx = idx
	dv.armGrowInst = instID
	dv.armGrowScope = scope
	dv.armGrowTarget = target
}

// armGrowControl reports the control currently being grown/shrunk for instID and
// whether the animation is live for it. Layout guards on this (not on
// synthControlArmed) so the reflow keeps running through the shrink.
func (dv *DrumView) armGrowControl(scope armedScope, instID instrument.ID) (int, bool) {
	return dv.armGrowIdx, dv.armGrowIdx >= 0 && dv.armGrowInst != "" &&
		dv.armGrowInst == instID && dv.armGrowScope == scope
}

// armGrowProgress is the eased 0..1 grow amount for the current subject.
func (dv *DrumView) armGrowProgress() float64 { return smoothstep(dv.armGrowT) }

// advanceArmGrow steps armGrowT one frame toward armGrowTarget. Returns true while
// still animating (drives a redraw + relayout). Clears the subject once a shrink
// (target 0) settles at rest.
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
