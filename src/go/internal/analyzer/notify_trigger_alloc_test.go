package analyzer

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// NotifyTrigger runs on the audio thread and must allocate at most the small pendingCap: rawBuf is held by reference
// (the shared, immutable cached voice) and converted only in processTick. A per-trigger float64 copy doubles allocs/call.
func TestNotifyTrigger_AllocBudget(t *testing.T) {
	svc := NewService(Config{
		FFTSize:        1024,
		WindowSize:     2048,
		MaxInstruments: 32,
		SampleRate:     44100,
	})

	// The cached voice buffer the audio thread holds, sized like a real snare.
	rawBuf := make([]float32, 22050)

	// The first calls may pay one-time init costs.
	for i := 0; i < 5; i++ {
		svc.NotifyTrigger(channel.Of(roster.Snare), rawBuf)
	}

	const budget = 1.0 // exactly the pendingCap struct alloc
	allocs := testing.AllocsPerRun(1000, func() {
		svc.NotifyTrigger(channel.Of(roster.Snare), rawBuf)
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

// Overwriting a pending capture (triggers outpacing processTick, which is common) must not allocate a buffer that is
// then thrown away.
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
		svc.NotifyTrigger(channel.Of(roster.Snare), rawBuf)
	}

	// Every call after the first overwrites a pending; none may allocate beyond the pendingCap.
	const budget = 1.0
	allocs := testing.AllocsPerRun(1000, func() {
		svc.NotifyTrigger(channel.Of(roster.Snare), rawBuf)
		svc.NotifyTrigger(channel.Of(roster.Kick), rawBuf)
		svc.NotifyTrigger(channel.Of(roster.Hihat), rawBuf)
		svc.NotifyTrigger(channel.Of(roster.Clap), rawBuf)
	})
	perCall := allocs / 4
	if perCall > budget {
		t.Fatalf("dropped-trigger allocs/call = %.2f exceeds budget %.0f — "+
			"overwriting pending should not allocate a discarded buffer",
			perCall, budget)
	}
	t.Logf("dropped-trigger allocs/call = %.2f (budget %.0f)", perCall, budget)
}
