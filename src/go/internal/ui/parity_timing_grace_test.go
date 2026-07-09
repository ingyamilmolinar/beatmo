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
	// Stage C (overdue) dropout: the fatal vehicle post-2026-07 calibration
	// (bridge/emitdrift are WarnOnly and would only log). Grace suppression is
	// stage-agnostic, so this still exercises the same suppress→release path.
	feedOverdueDropoutForTest(g)

	// Inside grace: no panic.
	g.parityTimingCheck()

	// After grace: panics on the same (still-present) breach.
	time.Sleep(70 * time.Millisecond)
	assertPanics(t, "overdue", func() { g.parityTimingCheck() })
}
