package ui

import (
	"testing"
	"time"
)

// injectParityMismatchAndScan drives a parity scan against a game whose view
// slate was already forced out of sync with the predictor by
// buildParityMismatchGame (row 0, abs 0: predictor says on, slate says off).
// It re-asserts the forced mismatch before each scan so repeated calls (once
// inside grace, once after expiry) both observe the same on-screen mismatch
// rather than relying on nothing else in the scan path having "fixed" it.
func injectParityMismatchAndScan(t *testing.T, g *Game) {
	t.Helper()
	g.drum.Rows[0].Steps[0] = false
	g.parityScan("test-grace-realtime")
}

// TestParityGraceDemotesMismatchThenRefiresAfterExpiry exercises the
// grace-demotion branch in parityReport/emitParityFatal, which has zero
// coverage under the default `go test` bypass in parityInGrace (grace is
// normally forced off under test for determinism). With
// SetParityGraceForceForTest(true), a short real grace window is armed: a
// genuine mismatch must be swallowed (log-only) while inside the window, and
// must panic once the window has expired.
func TestParityGraceDemotesMismatchThenRefiresAfterExpiry(t *testing.T) {
	restore := SetParityGraceForceForTest(true)
	defer restore()

	g := buildParityMismatchGame(t)
	g.parityWatch.Store(parityWatchPanic)
	prevFatal := parityFatalEnabled.Load()
	SetParityFatal(true)
	t.Cleanup(func() { SetParityFatal(prevFatal) })

	// Arm a short grace window (structural mutation just happened).
	g.parityGraceUntilNS.Store(time.Now().Add(40 * time.Millisecond).UnixNano())

	// Inject a genuine on-screen mismatch. Within grace it must NOT panic.
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("mismatch panicked inside grace window: %v", r)
			}
		}()
		injectParityMismatchAndScan(t, g)
	}()

	// After grace expires, the same mismatch must panic.
	time.Sleep(60 * time.Millisecond)
	paniced := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				paniced = true
			}
		}()
		injectParityMismatchAndScan(t, g)
	}()
	if !paniced {
		t.Fatal("expected panic after grace expiry, got none")
	}
}
