//go:build !test && !js

package audio

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// demoInstruments is a launch-circuit-sized instrument set — the kind whose
// cold render can stall the audioLoop dispatch goroutine and trip the overdue
// timing gate.
var demoInstruments = []instrument.ID{
	roster.DnbKick, roster.Snare, roster.HihatOpenBright, roster.Clap, roster.Shaker, roster.OrganChurch,
}

// TestSynthRenderCostTable measures the cold (cache-miss) render cost of each
// demo instrument through the SAME path dispatch uses (newRecipeAwareVoicePitched
// for melodic, newRecipeAwareVoice otherwise), at the instrument's natural render
// window. This is the "how expensive is one voice render on the dispatch
// goroutine" evidence table. Run:
//
//	xvfb-run -a go test ./internal/audio/ -run TestSynthRenderCostTable -v
func TestSynthRenderCostTable(t *testing.T) {
	const sr = 44100
	type row struct {
		id       string
		pitch    float64
		samples  int
		coldNS   int64
		warmNS   int64
		pitchAwr bool
	}
	var rows []row
	pitches := []float64{0, 5, -2, 7, 12}

	for _, id := range demoInstruments {
		pa := IsMelodicInstrument(id)
		d, _ := InstrumentDoc(id)
		samples := VoiceFrames(d.Render.Seconds, sr)

		globalVoiceCache.Clear()
		ResetRenderLatencyMetrics()

		// Cold render (cache miss) — the cost paid on the dispatch goroutine.
		coldStart := time.Now()
		if pa {
			newRecipeAwareVoicePitched(id, sr, 5)
		} else {
			newRecipeAwareVoice(id, sr)
		}
		cold := time.Since(coldStart)

		// Warm render (cache hit) — the cost when pre-warmed.
		warmStart := time.Now()
		if pa {
			newRecipeAwareVoicePitched(id, sr, 5)
		} else {
			newRecipeAwareVoice(id, sr)
		}
		warm := time.Since(warmStart)

		rows = append(rows, row{string(id), 5, samples, cold.Nanoseconds(), warm.Nanoseconds(), pa})
		_ = pitches
	}

	// The beat period at BPM 120, subdiv 8, is 60/120/8 = 62.5 ms. Any cold
	// render exceeding this while another beat is queued causes overdue.
	const beatPeriodMS = 62.5
	sort.Slice(rows, func(i, j int) bool { return rows[i].coldNS > rows[j].coldNS })

	fmt.Printf("\n%-14s %-8s %8s %12s %12s %10s  %s\n",
		"instrument", "pitchAw", "samples", "cold(ms)", "warm(us)", "renderSec", "vs 62.5ms beat")
	for _, r := range rows {
		coldMS := float64(r.coldNS) / 1e6
		warmUS := float64(r.warmNS) / 1e3
		renderSec := float64(r.samples) / sr
		flag := ""
		if coldMS > beatPeriodMS {
			flag = "*** EXCEEDS BEAT PERIOD -> overdue risk"
		}
		fmt.Printf("%-14s %-8v %8d %12.2f %12.2f %10.3f  %s\n",
			r.id, r.pitchAwr, r.samples, coldMS, warmUS, renderSec, flag)
	}
	fmt.Printf("(beat period @ bpm120 subdiv8 = %.1f ms; a cold render longer than this stalls the next queued beat)\n\n", beatPeriodMS)
}

// BenchmarkOrganChurchColdRender isolates the single most expensive demo voice
// (church organ) render cost + allocations. Run with a CPU profile to see where
// the time goes inside the C render:
//
//	xvfb-run -a go test ./internal/audio/ -run=^$ -bench=BenchmarkOrganChurchColdRender -benchmem -cpuprofile=/tmp/organ.prof
func BenchmarkOrganChurchColdRender(b *testing.B) {
	const (
		sr = 44100
		id = roster.OrganChurch
	)
	shipped, _ := InstrumentDoc(id)
	samples := VoiceFrames(shipped.Render.Seconds, sr)
	d, err := InstrumentDocWithFields(id, map[string]float64{"pitch": 5})
	if err != nil {
		b.Fatal(err)
	}
	block := modularBlock(instrument.Flatten(d, BuiltinModuleRegistry()))
	buf := make([]float32, samples)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderModularBlock(buf, sr, samples, block)
	}
	b.StopTimer()
	b.ReportMetric(float64(samples)/sr*1000, "renderedAudioMS/op")
}
