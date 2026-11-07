package ui

import (
	"testing"
	"time"
)

func TestParityTimingSuppressedDuringGrace(t *testing.T) {
	restore := SetParityGraceForceForTest(true)
	defer restore()
	g := newTestGameForParity(t)
	enableParityTimingForTest(t, g)
	g.SetPlaying(true)

	g.parityGraceUntilNS.Store(time.Now().Add(50 * time.Millisecond).UnixNano())
	// Stage C is the fatal stage; grace suppression is stage-agnostic, so it exercises the same path.
	feedOverdueDropoutForTest(g)

	// Inside grace: no panic.
	g.parityTimingCheck()

	// After grace: panics on the same (still-present) breach.
	time.Sleep(70 * time.Millisecond)
	assertPanics(t, "overdue", func() { g.parityTimingCheck() })
}
