//go:build !test && !js

package audio

import (
	"sort"
	"testing"
	"time"
)

// TestP5_RenderPerfAudit is the P5 perf audit: it renders every built-in
// instrument through the unified pipeline (RenderInstrumentOneShotRaw →
// bakedModularRender → render_modular_p) and reports the slowest, so a
// pathologically expensive voice (e.g. a heavy physical model or a deep
// gen-bank) is visible. Run: go test -run TestP5_RenderPerfAudit -v ./internal/audio/
func TestP5_RenderPerfAudit(t *testing.T) {
	Reset()
	ResetInstruments()

	type row struct {
		id      string
		nsPer   int64
		samples int
	}
	var rows []row
	const iters = 8
	for _, id := range BuiltinInstrumentIDs {
		buf, _ := RenderInstrumentOneShotRaw(id) // warm-up (first-render setup excluded)
		start := time.Now()
		for i := 0; i < iters; i++ {
			RenderInstrumentOneShotRaw(id)
		}
		rows = append(rows, row{id, time.Since(start).Nanoseconds() / iters, len(buf)})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].nsPer > rows[j].nsPer })

	var total int64
	for _, r := range rows {
		total += r.nsPer
	}
	t.Logf("P5 PERF AUDIT — %d instruments, mean %.2f ms/render", len(rows), float64(total)/float64(len(rows))/1e6)
	t.Log("P5 PERF AUDIT — 15 slowest:")
	for i := 0; i < 15 && i < len(rows); i++ {
		r := rows[i]
		// Normalize to per-second-of-audio so long voices don't look slow just for
		// being long (44100 samples/s).
		msPerSecAudio := float64(r.nsPer) / 1e6 / (float64(r.samples) / 44100.0)
		t.Logf("  %-22s %6.2f ms/render  (%5d samp, %6.2f ms per audio-sec)", r.id, float64(r.nsPer)/1e6, r.samples, msPerSecAudio)
	}
}
