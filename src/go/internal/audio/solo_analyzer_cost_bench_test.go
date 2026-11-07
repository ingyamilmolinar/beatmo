//go:build !test && !js

package audio

import (
	"fmt"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Soloing a row re-focuses its channel, which re-enables its analyzer; if that appends, every toggle leaves another
// FFT analyzer running on the audio thread forever. The organ is the worst case: sole signal, sustaining voices.

const soloBenchSampleRate = 44100

func eqBlockCost(ch *Channel, blockSize, blocks int) time.Duration {
	in := make([]float64, blockSize)
	out := make([]float64, blockSize)
	for i := range in {
		// A non-trivial signal so the FFT does real work.
		in[i] = float64(i%64)/32.0 - 1.0
	}
	// Warm up (fills analyzer windows, primes pools).
	for i := 0; i < 8; i++ {
		ch.ProcessEQBlockLocal(in, out)
	}
	start := time.Now()
	for i := 0; i < blocks; i++ {
		ch.ProcessEQBlockLocal(in, out)
	}
	return time.Since(start) / time.Duration(blocks)
}

// stackAnalyzers simulates n solo toggles' worth of EnableChannelAnalyzer calls.
func stackAnalyzers(id string, n int) {
	for i := 0; i < n; i++ {
		_ = EnableChannelAnalyzer(channel.Of(instrument.ID(id)), 512)
	}
}

// Flat with an idempotent enable; stacked analyzers grow linearly past the real-time budget.
func TestSoloAnalyzerStackCostTable(t *testing.T) {
	const blockSize = 512
	const blocks = 400

	// A block must process in less than its own duration.
	budgetSec := float64(blockSize) / float64(soloBenchSampleRate)
	budget := time.Duration(budgetSec * float64(time.Second))

	fmt.Printf("\n=== solo→analyzer audio-thread cost (block=%d @ %dHz, realtime budget=%.3fms) ===\n",
		blockSize, soloBenchSampleRate, ms(budget))
	fmt.Printf("%-14s %-12s %-12s %-10s\n", "soloToggles", "analyzers", "perBlock", "%realtime")

	for _, toggles := range []int{0, 1, 2, 4, 8, 16} {
		resetChains(t)
		id := fmt.Sprintf("solo-cost-organ-%d", toggles)
		_ = InstrumentChannel(channel.Of(instrument.ID(id)))

		// Baseline focus (what boot does), then `toggles` more re-focuses.
		stackAnalyzers(id, 1+toggles)

		n := ChannelAnalyzerCount(channel.Of(instrument.ID(id)))
		cost := eqBlockCost(chanMgr.ensureChannel(channel.Of(instrument.ID(id))), blockSize, blocks)
		pct := float64(cost) / float64(budget) * 100

		fmt.Printf("%-14d %-12d %-12.4f %-10.1f\n", toggles, n, ms(cost), pct)

		// The COUNT is the deterministic defect; wall-clock is evidence only and would flake under parallel load.
		if n != 1 {
			t.Errorf("after %d solo re-focuses the channel carries %d analyzers (want exactly 1); "+
				"each runs a 512-pt FFT on the audio thread every 512 samples and "+
				"SetAnalyzerEnabled can never switch the orphans off", toggles, n)
		}
	}
}

// BenchmarkChannelEQBlockSingleAnalyzer is the healthy baseline: one analyzer.
func BenchmarkChannelEQBlockSingleAnalyzer(b *testing.B) {
	benchEQBlock(b, 1)
}

func BenchmarkChannelEQBlockAfterEightSolos(b *testing.B) {
	benchEQBlock(b, 9)
}

func benchEQBlock(b *testing.B, enables int) {
	b.Helper()
	const blockSize = 512
	id := fmt.Sprintf("solo-bench-organ-%d", enables)
	_ = InstrumentChannel(channel.Of(instrument.ID(id)))
	stackAnalyzers(id, enables)
	ch := chanMgr.ensureChannel(channel.Of(instrument.ID(id)))

	in := make([]float64, blockSize)
	out := make([]float64, blockSize)
	for i := range in {
		in[i] = float64(i%64)/32.0 - 1.0
	}
	b.ReportMetric(float64(ChannelAnalyzerCount(channel.Of(instrument.ID(id)))), "analyzers")
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ch.ProcessEQBlockLocal(in, out)
	}
}
