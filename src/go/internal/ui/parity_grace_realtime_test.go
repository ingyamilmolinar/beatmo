package ui

import (
	"testing"
	"time"
)

// Re-forced before each scan so both calls see the same on-screen mismatch rather than trusting nothing fixed it.
func injectParityMismatchAndScan(t *testing.T, g *Game) {
	t.Helper()
	g.drum.Rows[0].Steps[0] = false
	g.parityScan("test-grace-realtime")
}

// Grace is forced off under go test for determinism, leaving this branch otherwise unreached: a real mismatch is
// log-only inside the window and panics after it.
func TestParityGraceDemotesMismatchThenRefiresAfterExpiry(t *testing.T) {
	restore := SetParityGraceForceForTest(true)
	defer restore()

	g := buildParityMismatchGame(t)
	g.parityWatch = parityWatchPanic
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
