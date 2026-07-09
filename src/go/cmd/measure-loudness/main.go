//go:build !test && !js

// Command measure-loudness renders every instrument through the real C DSP
// engine, measures its crest factor (scale-invariant RMS-to-peak), and computes
// a per-instrument playback amplitude that lands each instrument at a common
// perceived-loudness (flat-RMS) target. It prints a before/after dB report and
// (with -write) emits the two generated amplitude files.
//
// MUST be built as a production binary (NOT -tags test): the test build swaps
// in a pure-Go approximation synth instead of the real engine.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

const sampleRate = 48000

type row struct {
	id         string
	rmsNorm    float64 // rms/peak over loudest window (== 1/crest)
	currentAmp float64
	newAmp     float64
}

// measureRMSNorm renders one instrument and returns rms/peak over its loudest
// window. Returns ok=false when the instrument cannot be rendered or is silent.
//
// It first tries the synthmatch recipe path (real C DSP). Legacy bespoke drums
// (snare/kick/hihat/...) that have no recipe binding fall back to the registered
// Voice via audio.ExportVoice. rms/peak is scale-invariant, so an
// already-normalized voice output yields the same rmsNorm as the raw render.
func measureRMSNorm(id string, durSec float64) (float64, bool) {
	if rn, ok := measureFromWave(renderRecipe(id, durSec)); ok {
		return rn, true
	}
	return measureFromWave(renderVoice(id, durSec))
}

func renderRecipe(id string, durSec float64) (wave.Wave, bool) {
	w, err := synthmatch.RenderWithParams(id, 0, sampleRate, durSec, nil)
	if err != nil || len(w.Samples) == 0 {
		return wave.Wave{}, false
	}
	return w, true
}

func renderVoice(id string, durSec float64) (wave.Wave, bool) {
	v := audio.ExportVoice(id, 120, sampleRate)
	if v == nil {
		return wave.Wave{}, false
	}
	n := int(float64(sampleRate) * durSec)
	if n < 1 {
		return wave.Wave{}, false
	}
	samples := make([]float64, 0, n)
	for i := 0; i < n; i++ {
		s, done := v.Sample()
		samples = append(samples, s)
		if done {
			break
		}
	}
	if len(samples) == 0 {
		return wave.Wave{}, false
	}
	return wave.Wave{Samples: samples, SampleRate: sampleRate, Label: id}, true
}

func measureFromWave(w wave.Wave, ok bool) (float64, bool) {
	if !ok || len(w.Samples) == 0 {
		return 0, false
	}
	seg := fingerprint.AutoSegment(w, 0.3) // loudest 300ms window
	obs := wave.NewPeakRMSObserver().Observe(seg)
	if obs.Peak <= 1e-9 || obs.RMS <= 1e-12 {
		return 0, false
	}
	return obs.RMS / obs.Peak, true
}

func main() {
	peakCap := flag.Float64("peakcap", 1.0, "maximum post-normalization amplitude")
	write := flag.Bool("write", false, "write the generated Go + JS files")
	flag.Parse()

	ids := make([]string, 0, len(audio.InstrumentConfigs))
	for id := range audio.InstrumentConfigs {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	var rows []row
	for _, id := range ids {
		cfg := audio.ConfigForInstrument(id)
		rn, ok := measureRMSNorm(id, cfg.DurationSec)
		if !ok {
			fmt.Fprintf(os.Stderr, "skip %s: unrenderable/silent\n", id)
			continue
		}
		rows = append(rows, row{id: id, rmsNorm: rn, currentAmp: float64(cfg.Amplitude)})
	}

	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "no instruments measured")
		os.Exit(1)
	}

	// Auto-calibrate L so the MEDIAN instrument keeps amplitude ~0.8.
	norms := make([]float64, len(rows))
	for i, r := range rows {
		norms[i] = r.rmsNorm
	}
	sort.Float64s(norms)
	medianNorm := norms[len(norms)/2]
	L := medianNorm * 0.8

	for i := range rows {
		amp := L / rows[i].rmsNorm
		if amp > *peakCap {
			amp = *peakCap
		}
		if amp < 0 {
			amp = 0
		}
		rows[i].newAmp = amp
	}

	printReport(rows, L, *peakCap)
	fmt.Fprintf(os.Stderr, "# measured %d / %d instruments\n", len(rows), len(audio.InstrumentConfigs))
	if *write {
		writeGo(rows)
		writeJS(rows)
	}
}

func printReport(rows []row, L, peakCap float64) {
	fmt.Printf("# loudness measurement  L=%.4f  peakCap=%.2f  n=%d\n", L, peakCap, len(rows))
	fmt.Printf("%-22s %10s %10s %10s %10s\n", "instrument", "curRMSdB", "newRMSdB", "curAmp", "newAmp")
	var before, after []float64
	var capped []string
	for _, r := range rows {
		curDB := db(r.currentAmp * r.rmsNorm)
		newDB := db(r.newAmp * r.rmsNorm)
		before = append(before, curDB)
		after = append(after, newDB)
		flag := ""
		if math.Abs(r.newAmp-peakCap) < 1e-9 {
			flag = "  <-cap"
			capped = append(capped, r.id)
		}
		fmt.Printf("%-22s %10.2f %10.2f %10.3f %10.3f%s\n", r.id, curDB, newDB, r.currentAmp, r.newAmp, flag)
	}
	fmt.Printf("# spread (max-min RMS dB): before=%.2f  after=%.2f\n", spread(before), spread(after))
	if len(capped) > 0 {
		fmt.Printf("# pinned at peakCap (%d): %v\n", len(capped), capped)
	}
	// After-spread among the non-capped (typical) instruments only.
	var typical []float64
	for _, r := range rows {
		if math.Abs(r.newAmp-peakCap) >= 1e-9 {
			typical = append(typical, db(r.newAmp*r.rmsNorm))
		}
	}
	if len(typical) > 0 {
		fmt.Printf("# after-spread (non-capped, n=%d): %.2f\n", len(typical), spread(typical))
	}
}

func spread(v []float64) float64 {
	lo, hi := v[0], v[0]
	for _, x := range v {
		if x < lo {
			lo = x
		}
		if x > hi {
			hi = x
		}
	}
	return hi - lo
}

func writeGo(rows []row) {
	var b []byte
	b = append(b, []byte("// Code generated by cmd/measure-loudness. DO NOT EDIT.\n"+
		"// Regenerate: cd src/go && ../../.tools/go/bin/go run ./cmd/measure-loudness -write\n"+
		"// Per-instrument playback amplitude, loudness-normalized (flat-RMS) so every\n"+
		"// instrument sits at a common perceived loudness. See\n"+
		"// docs/superpowers/specs/2026-07-03-instrument-loudness-normalization-design.md\n\n"+
		"package audio\n\n"+
		"var instrumentLoudnessAmp = map[string]float32{\n")...)
	for _, r := range rows {
		b = append(b, []byte(fmt.Sprintf("\t%q: %s,\n", r.id, formatAmp(r.newAmp)))...)
	}
	b = append(b, []byte("}\n")...)
	must(os.WriteFile("internal/audio/instrument_loudness_gen.go", b, 0o644))
}

func writeJS(rows []row) {
	var b []byte
	b = append(b, []byte("// Code generated by cmd/measure-loudness. DO NOT EDIT.\n"+
		"// Regenerate: cd src/go && ../../.tools/go/bin/go run ./cmd/measure-loudness -write\n"+
		"export const INSTRUMENT_LOUDNESS_AMP = Object.freeze({\n")...)
	for _, r := range rows {
		b = append(b, []byte(fmt.Sprintf("  %q: %s,\n", r.id, formatAmp(r.newAmp)))...)
	}
	b = append(b, []byte("});\n")...)
	must(os.WriteFile("../js/instrument_loudness.gen.js", b, 0o644))
}

// formatAmp emits the float32 value both files share, identically formatted, so
// the Go<->JS parity guard is a clean numeric match.
func formatAmp(v float64) string {
	return fmt.Sprintf("%.4f", float64(float32(v)))
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func db(x float64) float64 {
	if x <= 1e-9 {
		return -120
	}
	return 20 * math.Log10(x)
}
