// src/go/internal/ui/step_notcher.go
package ui

// Sub-notch travel is retained, not applied, so a tiny drag never flies through values. The one cadence for every
// detented control, so they all feel alike.
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

// The remainder carries over so the cadence stays even across small moves.
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

// At a gesture start, so a drag begins on a clean detent boundary.
func (s *stepNotcher) reset() { s.acc = 0 }

// Keeps in-flight accumulation.
func (s *stepNotcher) setPxPerNotch(px int) {
	if px < 1 {
		px = 1
	}
	s.pxPerNotch = px
}

// The per-detent semantics live here only.
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
