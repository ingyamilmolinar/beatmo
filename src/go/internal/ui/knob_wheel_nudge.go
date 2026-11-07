package ui

// A wheel spin writes live audio on every notch but records ONE undo step when a pause ends it; opening the
// popup or switching tabs flushes a pending commit first, so the popup snapshots the committed value.

// ≈ 600 ms at 60 fps.
const wheelNudgeCommitFrames = 36

// The zero value is inert.
type wheelNudger struct {
	framesLeft int
	commit     func()
}

// bump (re)arms the debounce and latches the commit to run when it elapses.
func (n *wheelNudger) bump(commit func()) {
	n.framesLeft = wheelNudgeCommitFrames
	n.commit = commit
}

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

func (n *wheelNudger) flush() {
	if n.framesLeft > 0 && n.commit != nil {
		c := n.commit
		n.commit = nil
		n.framesLeft = 0
		c()
	}
}

// fine (Shift) steps one rung finer than the badge WITHOUT persisting the rung change.
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

// -1 at the finest rung or for a nil badge.
func badgeIndexBelow(b *KnobStepBadge) int {
	if b == nil || b.idx <= 0 {
		return -1
	}
	return b.idx - 1
}
