package ui

// knob_wheel_nudge.go — the desktop scroll-wheel VALUE nudge for synth param
// cards, plus the debounced single-undo commit that collapses a whole wheel
// spin into one undo step.
//
// A wheel spin mutates live audio on every notch (so the sound tracks the
// gesture), but records exactly ONE undo step when the spin ends — the same
// "one event per gesture" contract the old rotary-drag release-commit provided.
// The end of a spin is detected by a pause: wheelNudgeCommitFrames with no new
// notch fires the latched commit. Opening the value-wheel popup (or switching
// tabs) flushes any pending commit first so the popup snapshots the committed
// value.

// wheelNudgeCommitFrames ≈ 600ms at 60fps: a pause this long ends a wheel
// gesture and commits its single undo step.
const wheelNudgeCommitFrames = 36

// wheelNudger debounces a wheel-spin gesture into a single trailing commit.
// The zero value is inert (framesLeft == 0). Owned per-DrumView; ticked once
// per synth-tab Update frame.
type wheelNudger struct {
	framesLeft int
	commit     func()
}

// bump (re)arms the debounce and latches the commit to run when it elapses.
func (n *wheelNudger) bump(commit func()) {
	n.framesLeft = wheelNudgeCommitFrames
	n.commit = commit
}

// tick advances the debounce by one frame and fires the latched commit exactly
// once when it reaches zero. Call once per Update frame.
func (n *wheelNudger) tick() {
	if n.framesLeft <= 0 {
		return
	}
	n.framesLeft--
	if n.framesLeft == 0 && n.commit != nil {
		c := n.commit
		n.commit = nil
		c()
	}
}

// flush commits immediately (popup opening / tab switch mid-gesture) so a
// pending wheel commit lands before the next snapshot is taken.
func (n *wheelNudger) flush() {
	if n.framesLeft > 0 && n.commit != nil {
		c := n.commit
		n.commit = nil
		n.framesLeft = 0
		c()
	}
}

// nudgeKnobByWheel applies one wheel event to the shared knob model: one StepMul
// (endless) or one detent (discrete) per notch. fine=true (Shift held) applies
// at ONE rung finer than the badge's active rung WITHOUT persisting the rung
// change. Returns true if the value changed.
func nudgeKnobByWheel(k *Knob, badge *KnobStepBadge, steps int, fine bool) bool {
	if k == nil || steps == 0 {
		return false
	}
	dir := 1
	if steps < 0 {
		dir = -1
	}
	if k.Discrete {
		return k.StepValueDirect(dir)
	}
	// Continuous / endless: Shift applies a temporarily-finer step (one ladder
	// rung below the active resolution) without moving the persisted badge rung.
	if fine && badge != nil {
		if i := badgeIndexBelow(badge); i >= 0 && i < len(badge.ladder) {
			saved := k.StepMul
			k.StepMul = badge.ladder[i]
			defer func() { k.StepMul = saved }()
		}
	}
	k.NudgeEndless(dir * knobEndlessPxPerNotch)
	return true
}

// badgeIndexBelow returns the ladder index one rung FINER than the active rung,
// or -1 when already at the finest rung (or the badge is nil). The ladder is
// ordered fine→coarse (index 0 is the smallest step), so finer == lower index.
func badgeIndexBelow(b *KnobStepBadge) int {
	if b == nil || b.idx <= 0 {
		return -1
	}
	return b.idx - 1
}
