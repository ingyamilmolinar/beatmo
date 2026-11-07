//go:build !test && !js

package audio

import (
	"sort"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Reports the slowest voices, so a pathologically expensive one (a heavy physical model, a deep gen bank) is visible.
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
	for _, id := range roster.All {
		buf, _ := RenderInstrumentOneShotRaw(id) // warm-up (first-render setup excluded)
		start := time.Now()
		for i := 0; i < iters; i++ {
			RenderInstrumentOneShotRaw(id)
		}
		rows = append(rows, row{string(id), time.Since(start).Nanoseconds() / iters, len(buf)})
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
		// Per second of audio, so a long voice does not look slow for being long.
		msPerSecAudio := float64(r.nsPer) / 1e6 / (float64(r.samples) / 44100.0)
		t.Logf("  %-22s %6.2f ms/render  (%5d samp, %6.2f ms per audio-sec)", r.id, float64(r.nsPer)/1e6, r.samples, msPerSecAudio)
	}
}
