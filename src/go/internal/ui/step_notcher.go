// src/go/internal/ui/step_notcher.go
package ui

// stepNotcher converts continuous 1-D travel (drag pixels or wheel deltas) into
// evenly-paced discrete "clicks". Every pxPerNotch units of accumulated travel
// emit exactly one signed step; sub-notch travel is retained, not applied, so a
// tiny drag does not fly through every value. This is the single shared cadence
// primitive for clicky, step-by-step controls — the mobile scroll-wheel's value
// drag, its resolution strip, and any future detented control — so they all feel
// identical (one detent per notch) instead of each re-deriving an accumulator.
type stepNotcher struct {
	acc        int
	pxPerNotch int
}

// newStepNotcher returns a notcher that emits one step per pxPerNotch of travel.
func newStepNotcher(pxPerNotch int) stepNotcher {
	if pxPerNotch < 1 {
		pxPerNotch = 1
	}
	return stepNotcher{pxPerNotch: pxPerNotch}
}

// add feeds delta px of travel and returns the net whole steps to apply now
// (signed); the remainder is retained for the next add so the cadence stays even
// across many small moves.
func (s *stepNotcher) add(delta int) int {
	if s.pxPerNotch < 1 {
		s.pxPerNotch = 1
	}
	s.acc += delta
	steps := 0
	for s.acc >= s.pxPerNotch {
		steps++
		s.acc -= s.pxPerNotch
	}
	for s.acc <= -s.pxPerNotch {
		steps--
		s.acc += s.pxPerNotch
	}
	return steps
}

// reset clears partial accumulation; call at the start of a new gesture so a
// fresh drag begins from a clean detent boundary.
func (s *stepNotcher) reset() { s.acc = 0 }

// setPxPerNotch updates the cadence (e.g. when density changes), preserving any
// in-flight partial accumulation.
func (s *stepNotcher) setPxPerNotch(px int) {
	if px < 1 {
		px = 1
	}
	s.pxPerNotch = px
}

// applyKnobSteps advances a knob by n discrete detents (n may be negative),
// reusing the knob's own one-step primitive. Shared by every clicky caller so
// the per-detent semantics live in exactly one place.
func applyKnobSteps(k *Knob, n int) {
	if k == nil || n == 0 {
		return
	}
	dir := 1
	if n < 0 {
		dir, n = -1, -n
	}
	for i := 0; i < n; i++ {
		k.StepValueDirect(dir)
	}
}
