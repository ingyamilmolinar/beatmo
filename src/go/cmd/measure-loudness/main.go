//go:build !test && !js

// Command measure-loudness sets each instrument's render.amplitude (with -write) so every voice lands at one flat-RMS
// loudness target, from its crest factor through the real C DSP. Never -tags test: that build's synth is a pure-Go
// approximation.
package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/clone"
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

const sampleRate = 48000

type row struct {
	id         instrument.ID
	rmsNorm    float64 // rms/peak over loudest window (== 1/crest)
	currentAmp float64
	newAmp     float64
}

// measureRMSNorm returns rms/peak over the loudest window (ok=false when silent or unrenderable). It is scale-invariant,
// so a normalised voice gives the same value as the raw render.
func measureRMSNorm(id instrument.ID, durSec float64) (float64, bool) {
	if rn, ok := measureFromWave(renderRecipe(id, durSec)); ok {
		return rn, true
	}
	return measureFromWave(renderVoice(id, durSec))
}

func renderRecipe(id instrument.ID, durSec float64) (wave.Wave, bool) {
	w, err := synthmatch.RenderWithParams(id, 0, sampleRate, durSec, nil)
	if err != nil || len(w.Samples) == 0 {
		return wave.Wave{}, false
	}
	return w, true
}

func renderVoice(id instrument.ID, durSec float64) (wave.Wave, bool) {
	v := audio.ExportVoice(id, sampleRate)
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
	return wave.Wave{Samples: samples, SampleRate: sampleRate, Label: string(id)}, true
}

func measureFromWave(w wave.Wave, ok bool) (float64, bool) {
	if !ok {
		return 0, false
	}
	// The one crest rule, shared with WAV clones.
	return clone.LoudnessNorm(w.Samples, w.SampleRate)
}

func main() {
	peakCap := flag.Float64("peakcap", 1.0, "maximum post-normalization amplitude")
	write := flag.Bool("write", false, "write each amplitude into its shipped instrument document")
	flag.Parse()

	ids := roster.All
	slices.Sort(ids)

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
	fmt.Fprintf(os.Stderr, "# measured %d / %d instruments\n", len(rows), len(ids))
	if *write {
		writeDocs(rows)
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
			capped = append(capped, string(r.id))
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

// Only documents whose value changes are rewritten, in the generator's own format, so the rest stay byte-identical.
func writeDocs(rows []row) {
	changed := 0
	for _, r := range rows {
		path := filepath.Join("internal", "assets", "instruments", string(r.id)+".json")
		raw, err := os.ReadFile(path)
		must(err)
		d, err := instrument.ParseFile(raw)
		must(err)
		amp, err := strconv.ParseFloat(formatAmp(r.newAmp), 64)
		must(err)
		if d.Render.Amplitude == amp {
			continue
		}
		d.Render.Amplitude = amp
		out, err := instrument.MarshalFile(d)
		must(err)
		must(os.WriteFile(path, out, 0o644))
		changed++
	}
	fmt.Fprintf(os.Stderr, "# wrote %d changed document amplitudes\n", changed)
}

// formatAmp rounds an amplitude to the 4-decimal float32 value the documents
// store, so a re-measurement that moves nothing writes nothing.
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
