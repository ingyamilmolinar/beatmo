//go:build !test && !js

package audio

import (
	"fmt"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/channel"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// solo_analyzer_cost_bench_test.go — quantifies the AUDIO-THREAD cost that the
// "solo the organ" flow adds per solo toggle.
//
// Scenario (from the field report): play the startup demo, solo the organ, and
// audio degrades and thrashes — even though soloing strictly REMOVES scheduled
// voices. The cost is not in the scheduler; it lands here. Soloing a row makes
// it the sole audible row, which auto-focuses its audio channel
// (reconcileChannelWithAudibleSet → selectAudioChannel → setEQActiveChannel),
// and setEQActiveChannel unconditionally re-issues EnableChannelAnalyzer. When
// that call APPENDS instead of replacing, every toggle leaves another live
// 512-point-FFT Analyzer in the channel's eqProcs, running on the audio
// rendering thread forever.
//
// The organ is the worst case for two compounding reasons: once soloed it is the
// only channel with signal, and its voices sustain ~1s, so its whole accumulated
// analyzer stack runs at ~100% duty cycle instead of drum-transient bursts.
//
// Run:
//   xvfb-run -a go test ./internal/audio/ -run TestSoloAnalyzerStackCostTable -v
//   xvfb-run -a go test ./internal/audio/ -run=^$ -bench=BenchmarkChannelEQBlock -benchmem

const soloBenchSampleRate = 44100

// eqBlockCost times ProcessEQBlockLocal over enough blocks to cover wallSeconds
// of audio, returning the average per-block cost.
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

// TestSoloAnalyzerStackCostTable is the evidence table: audio-thread cost per
// block as a function of how many times the channel was "re-focused" (i.e. how
// many solo toggles the user performed). With an idempotent
// EnableChannelAnalyzer the cost must stay flat; if analyzers stack, cost grows
// linearly and eventually exceeds the realtime budget → dropouts.
func TestSoloAnalyzerStackCostTable(t *testing.T) {
	const blockSize = 512
	const blocks = 400

	// Realtime budget: a blockSize block must be processed in less than its own
	// duration or the audio thread cannot keep up.
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

		// Assert on the analyzer COUNT, not the measured time. Count is the
		// deterministic invariant and the actual defect; wall-clock here is
		// reported as evidence but would flake under parallel test load.
		// Pre-fix this read 1+toggles (17 at the bottom row) and the per-block
		// cost grew ~23x with it.
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

// BenchmarkChannelEQBlockAfterEightSolos is the same channel after eight solo
// toggles. With an idempotent EnableChannelAnalyzer it must match the baseline.
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
