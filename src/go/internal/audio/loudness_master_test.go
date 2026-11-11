package audio

import (
	"math"
	"sync/atomic"
	"testing"
	"time"
)

// resetMasterLUFSForTest fully clears the process-global integrator,
// matching what ResetLoudnessForTest does but also nilling the
// integrator pointer so the "before any Ensure" path of
// MasterLUFSShortTerm is reachable. Tests use this for the
// "no-Ensure" path; otherwise they use ResetLoudnessForTest.
func resetMasterLUFSForTest(t *testing.T) {
	t.Helper()
	masterLUFSMu.Lock()
	masterLUFSCurrent = nil
	masterLUFSBits.Store(0)
	masterLUFSMu.Unlock()
}

// sineBlock returns `n` samples of a sine at freq Hz, sample rate sr,
// amplitude amp. Phase-continuous within a single call (each call
// restarts at zero phase — fine for steady-state tests).
func sineBlock(freq, sr, amp float64, n int) []float64 {
	buf := make([]float64, n)
	step := 2 * math.Pi * freq / sr
	for i := range n {
		buf[i] = amp * math.Sin(step*float64(i))
	}
	return buf
}

func TestEnsureMasterLUFS_IdempotentAtSameRate(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	EnsureMasterLUFS(48000)
	FeedMasterLUFS(sineBlock(1000, 48000, 0.5, 6000))
	first := MasterLUFSShortTerm()
	if first <= -120 {
		t.Fatalf("after one feed, MasterLUFSShortTerm = %.2f; want > -120", first)
	}
	EnsureMasterLUFS(48000)
	second := MasterLUFSShortTerm()
	if math.Abs(second-first) > 0.001 {
		t.Errorf("idempotent Ensure changed published value: before=%.4f after=%.4f", first, second)
	}
}

func TestEnsureMasterLUFS_RateChangeResets(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	EnsureMasterLUFS(48000)
	FeedMasterLUFS(sineBlock(1000, 48000, 0.5, 6000))
	if MasterLUFSShortTerm() <= -120 {
		t.Fatalf("precondition: feed should have lifted LUFS above -120")
	}
	EnsureMasterLUFS(44100)
	got := MasterLUFSShortTerm()
	if got != -120 {
		t.Errorf("after rate change MasterLUFSShortTerm = %.2f; want -120", got)
	}
}

func TestMasterLUFSShortTerm_BeforeAnyEnsureReturnsFloor(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	got := MasterLUFSShortTerm()
	if got != -120 {
		t.Errorf("MasterLUFSShortTerm before Ensure = %.2f; want -120", got)
	}
}

func TestFeedMasterLUFS_KnownTone(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	EnsureMasterLUFS(48000)
	const sr = 48000
	const dur = 3
	tone := sineBlock(997, sr, 0.1, sr*dur)
	FeedMasterLUFS(tone)

	lufs := MasterLUFSShortTerm()
	if lufs < -24 || lufs > -18 {
		t.Errorf("LUFS for 997 Hz @ -20 dBFS = %.2f; want in [-24, -18]", lufs)
	}
}

func TestFeedMasterLUFS_NoIntegratorIsNoOp(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	FeedMasterLUFS(sineBlock(1000, 48000, 0.5, 6000))
	if got := MasterLUFSShortTerm(); got != -120 {
		t.Errorf("after feed without Ensure, MasterLUFSShortTerm = %.2f; want -120", got)
	}
}

func TestMasterLUFSShortTerm_CrossGoroutineRace(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	EnsureMasterLUFS(48000)

	var reads atomic.Int64
	var stop atomic.Bool
	var sawAboveFloor atomic.Bool

	doneR := make(chan struct{})
	go func() {
		defer close(doneR)
		for !stop.Load() {
			v := MasterLUFSShortTerm()
			reads.Add(1)
			if v > -120 {
				sawAboveFloor.Store(true)
			}
		}
	}()

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		FeedMasterLUFS(sineBlock(1000, 48000, 0.5, 4800))
	}
	stop.Store(true)
	<-doneR

	if reads.Load() == 0 {
		t.Fatalf("reader goroutine performed zero reads")
	}
	if !sawAboveFloor.Load() {
		t.Errorf("reader never observed LUFS > -120 across %d reads", reads.Load())
	}
}

func TestResetLoudnessForTest_ZeroesPublishedValue(t *testing.T) {
	resetMasterLUFSForTest(t)
	t.Cleanup(func() { resetMasterLUFSForTest(t) })

	EnsureMasterLUFS(48000)
	FeedMasterLUFS(sineBlock(1000, 48000, 0.5, 6000))
	if MasterLUFSShortTerm() <= -120 {
		t.Fatalf("precondition: feed should have lifted LUFS above -120")
	}
	ResetLoudnessForTest()
	if got := MasterLUFSShortTerm(); got != -120 {
		t.Errorf("after ResetLoudnessForTest, MasterLUFSShortTerm = %.2f; want -120", got)
	}
}
