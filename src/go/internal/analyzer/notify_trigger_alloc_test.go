package analyzer

import (
	"testing"
)

// TestNotifyTrigger_AllocBudget pins the per-call allocation count for
// NotifyTrigger to a tight budget. Before the OOM-prevention fix this
// function allocated a fresh []float64 of len(rawBuf) on every call,
// converting float32→float64 on the audio thread; the synth-tab profile
// attributed 252 MB of cumulative allocations to that path over 60 s.
//
// After the fix the audio thread captures rawBuf by reference (shared,
// immutable cached recipe buffer) and conversion is deferred to
// processTick, so NotifyTrigger should allocate at most one small
// pendingCap struct per call. A regression that re-adds the per-trigger
// []float64 conversion would push allocs/call from ~1 to 2 and fail this
// test.
//
// This is the canonical regression guard for the per-trigger allocation
// budget — a fix that "looks right" must keep this number pinned.
func TestNotifyTrigger_AllocBudget(t *testing.T) {
	svc := NewService(Config{
		FFTSize:        1024,
		WindowSize:     2048,
		MaxInstruments: 32,
		SampleRate:     44100,
	})

	// Mirror the audio thread's contract: rawBuf is the cached recipe
	// buffer from globalVoiceCache. A realistic snare at 140 BPM is
	// roughly 20 K samples; pick a similarly-sized buffer.
	rawBuf := make([]float32, 22050)

	// Warm up — first call may pay one-time init costs (atomic.Pointer
	// type assertion table, etc.).
	for i := 0; i < 5; i++ {
		svc.NotifyTrigger("snare", rawBuf)
	}

	const budget = 1.0 // exactly the pendingCap struct alloc
	allocs := testing.AllocsPerRun(1000, func() {
		svc.NotifyTrigger("snare", rawBuf)
	})
	if allocs > budget {
		t.Fatalf("NotifyTrigger allocs/call = %.1f exceeds budget %.0f — "+
			"a per-trigger []float64 conversion has re-entered the hot path. "+
			"The audio thread MUST capture rawBuf by reference; conversion "+
			"belongs in processTick where it is bounded by tick rate, not "+
			"trigger rate. See internal/analyzer/service.go NotifyTrigger.",
			allocs, budget)
	}
	t.Logf("NotifyTrigger allocs/call = %.1f (budget %.0f, "+
		"rawBuf len=%d) — pre-fix this was ~2 allocs/call: pendingCap "+
		"struct + a fresh []float64 of len(rawBuf) (~80 KB for snare).",
		allocs, budget, len(rawBuf))
}

// TestNotifyTrigger_DroppedTriggerNoExtraAlloc verifies that overwriting
// a pending capture (the "dropped trigger" case — common when trigger
// rate exceeds processTick rate) does not allocate a buffer that then
// gets thrown away. Before the fix every dropped trigger wasted ~80 KB.
func TestNotifyTrigger_DroppedTriggerNoExtraAlloc(t *testing.T) {
	svc := NewService(Config{
		FFTSize:        1024,
		WindowSize:     2048,
		MaxInstruments: 32,
		SampleRate:     44100,
	})
	rawBuf := make([]float32, 22050)

	// Warm up.
	for i := 0; i < 5; i++ {
		svc.NotifyTrigger("snare", rawBuf)
	}

	// Drive 4 consecutive triggers without ever consuming via processTick.
	// Every call after the first overwrites a pending. None should
	// allocate a buffer beyond the pendingCap struct.
	const budget = 1.0
	allocs := testing.AllocsPerRun(1000, func() {
		svc.NotifyTrigger("snare", rawBuf)
		svc.NotifyTrigger("kick", rawBuf)
		svc.NotifyTrigger("hihat", rawBuf)
		svc.NotifyTrigger("clap", rawBuf)
	})
	perCall := allocs / 4
	if perCall > budget {
		t.Fatalf("dropped-trigger allocs/call = %.2f exceeds budget %.0f — "+
			"overwriting pending should not allocate a discarded buffer",
			perCall, budget)
	}
	t.Logf("dropped-trigger allocs/call = %.2f (budget %.0f)", perCall, budget)
}
