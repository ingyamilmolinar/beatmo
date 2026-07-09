//go:build !test && !js

package audio

import (
	"fmt"
	"math"
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// kick_refs_tune_test.go is the native tuning harness for the kick-808 and
// kick-acoustic re-tunes (toward the house + dnb reference samples). Same
// apples-to-apples loop as zgump_kick_tune_test.go: render the seed through the
// real C DSP, decode the reference, compare via the fingerprint kick metrics,
// sweep knob overrides. Run with -v; SWEEP=1 to explore, WRITE_WAV=1 for audio.

func decodeRefAt(t *testing.T, path string, sr int) []float64 {
	pcm, fsr, err := DecodeWAVToPCM(path)
	if err != nil {
		t.Skipf("reference %q unavailable: %v", path, err)
	}
	out := make([]float64, len(pcm))
	for i, v := range pcm {
		out[i] = float64(v)
	}
	if fsr != sr {
		out = resampleLinear(out, fsr, sr)
	}
	return peakNorm(out)
}

func renderSeed(seed RecipeParams, sr, samples int) []float32 {
	buf := make([]float32, samples)
	renderModularP(buf, sr, samples, recipeParamsToModular(seed))
	return buf
}

// tuneKickToRef renders base+overrides candidates against refFP and returns the
// best (lowest fingerprint distance) override set + its distance.
func tuneKickToRef(refFP fingerprint.KickFingerprint, base RecipeParams, sr, samples int, cands map[string]RecipeParams) (string, float64) {
	best, bestD := "", 1e9
	for name, over := range cands {
		seed := cloneSeed(base, over)
		syn := peakNorm(f32toF64(renderSeed(seed, sr, samples)))
		d := fingerprint.KickDistance(refFP, fingerprint.KickAnalyze(wave.Wave{Samples: syn, SampleRate: sr})).Total
		if d < bestD {
			bestD, best = d, name
		}
	}
	return best, bestD
}

func reportKick(tag string, buf []float32, sr int) {
	x := peakNorm(f32toF64(buf))
	analyzeKick(x, sr).print(tag)
	reportFPTimbre(tag, fingerprint.KickAnalyze(wave.Wave{Samples: x, SampleRate: sr}))
}

// TestKickSegmentReport demonstrates the segment framework: it prints the
// per-segment (attack/body/tail) metric grid for the shipped kick-acoustic vs
// its reference + the ranked "top divergences" — the actionable form of "which
// change at which time". Run: go test -run TestKickSegmentReport -v ./internal/audio/
func TestKickSegmentReport(t *testing.T) {
	const sr = 44100
	ref := decodeRefAt(t, "../../../../36010__sandyrb__dnb-kick-003.wav", sr)
	buf, _ := RenderInstrumentOneShotRaw("kick-acoustic")
	syn := peakNorm(f32toF64(buf))

	rs := fingerprint.KickSegmentAnalyze(wave.Wave{Samples: ref, SampleRate: sr})
	ss := fingerprint.KickSegmentAnalyze(wave.Wave{Samples: syn, SampleRate: sr})
	for _, name := range []string{fingerprint.SegAttack, fingerprint.SegBody, fingerprint.SegTail} {
		r, s := rs.Segment(name), ss.Segment(name)
		if r == nil || s == nil {
			continue
		}
		fmt.Printf("── %-6s  energyFrac ref=%.2f syn=%.2f | centroid ref=%.0f syn=%.0f | flat ref=%.3f syn=%.3f | lowMid ref=%.2f syn=%.2f\n",
			name, r.EnergyFrac, s.EnergyFrac, r.Centroid, s.Centroid, r.Flatness, s.Flatness, r.LowMidRatio, s.LowMidRatio)
	}
	d := fingerprint.KickSegmentDistance(rs, ss)
	fmt.Printf("per-segment distance: attack=%.2f body=%.2f tail=%.2f  total=%.2f\n",
		d.PerSegment[fingerprint.SegAttack], d.PerSegment[fingerprint.SegBody], d.PerSegment[fingerprint.SegTail], d.Total)
	fmt.Println("TOP DIVERGENCES (target these):")
	for i, dd := range d.Deltas {
		if i >= 6 {
			break
		}
		fmt.Printf("  %d. %-6s %-10s ref=%.3f synth=%.3f  Δ=%+.3f\n", i+1, dd.Segment, dd.Metric, dd.Ref, dd.Synth, dd.NormDelta)
	}
}

func reportFPTimbre(tag string, fp fingerprint.KickFingerprint) {
	fpScalars(tag, fp)
	cmean := 0.0
	for _, c := range fp.CentroidTrace {
		cmean += c
	}
	if len(fp.CentroidTrace) > 0 {
		cmean /= float64(len(fp.CentroidTrace))
	}
	fmt.Printf("   %s TIMBRE: centroidMean=%.0fHz attackCentroid=%.0fHz flatness=%.4f mfcc1..4=%.0f,%.0f,%.0f,%.0f\n",
		tag, cmean, fp.AttackCentroid, fp.FlatnessAvg, fp.MFCC[1], fp.MFCC[2], fp.MFCC[3], fp.MFCC[4])
	fmt.Printf("   %s BODY: attackLowMid=%.2f (kick>1,tom<1)  tailRatio=%.4f tailDur=%.0fms tailFlat=%.3f\n",
		tag, fp.AttackLowMidRatio, fp.TailRatio, fp.TailDurationSec*1000, fp.TailFlatness)
}

func TestKick808Tune(t *testing.T) {
	const sr = 44100
	ref := decodeRefAt(t, "../../../../tmp/house_kick_ref.wav", sr)
	samples := len(ref)
	refFP := fingerprint.KickAnalyze(wave.Wave{Samples: ref, SampleRate: sr})
	analyzeKick(ref, sr).print("REF house-kick")
	fpScalars("REF ", refFP)
	reportKick("808 current", renderSeed(kick808Seed, sr, samples), sr)
	cur := fingerprint.KickDistance(refFP, fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(f32toF64(renderSeed(kick808Seed, sr, samples))), SampleRate: sr}))
	fmt.Printf("   808 FP-DIST=%.3f\n", cur.Total)

	if os.Getenv("SWEEP") != "" {
		cands := map[string]RecipeParams{}
		for _, vf := range []float64{44, 50, 56} {
			for _, peA := range []float64{2.4, 3.0} {
				for _, peR := range []float64{25, 40} {
					for _, h2 := range []float64{0.20, 0.35, 0.50} {
						for _, env0 := range []float64{1.0, 1.6, 2.4} {
							for _, click := range []float64{0.05, 0.15, 0.30} {
								cands[fmt.Sprintf("vf=%.0f peA=%.1f peR=%.0f h2=%.2f e0=%.1f clk=%.2f", vf, peA, peR, h2, env0, click)] = RecipeParams{
									"voice_freq_hz": vf, "gen1_kick_pe_amt": peA, "gen1_kick_pe_rate": peR,
									"gen1_kick_h2": h2, "gen1_kick_env0": env0, "gen1_kick_click": click, "gen1_kick_fade": 1.3,
								}
							}
						}
					}
				}
			}
		}
		best, d := tuneKickToRef(refFP, kick808Seed, sr, samples, cands)
		fmt.Printf("808 BEST: %s  FP-DIST=%.3f\n", best, d)
		reportKick("808 BEST", renderSeed(cloneSeed(kick808Seed, cands[best]), sr, samples), sr)
	}
	if os.Getenv("WRITE_WAV") != "" {
		writeHits(ref, "../../../../tmp/kick808_ref.wav", sr)
		writeHits(peakNorm(f32toF64(renderSeed(kick808Seed, sr, samples))), "../../../../tmp/kick808_synth.wav", sr)
	}
}

func TestKickAcousticTune(t *testing.T) {
	const sr = 44100
	ref := decodeRefAt(t, "../../../../36010__sandyrb__dnb-kick-003.wav", sr)
	samples := len(ref)
	refFP := fingerprint.KickAnalyze(wave.Wave{Samples: ref, SampleRate: sr})
	analyzeKick(ref, sr).print("REF sandyrb-dnb (acoustic target)")
	reportFPTimbre("REF ", refFP)
	reportKick("acoustic current", renderSeed(acousticKickSeed, sr, samples), sr)
	cur := fingerprint.KickDistance(refFP, fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(f32toF64(renderSeed(acousticKickSeed, sr, samples))), SampleRate: sr}))
	fmt.Printf("   acoustic FP-DIST=%.3f\n", cur.Total)

	if os.Getenv("SWEEP") != "" {
		cands := map[string]RecipeParams{}
		// variant-7 kick: balance fundamental-dominance (kick-vs-tom, via h2/mode_gain
		// + voice_freq) against MFCC spectral shape, plus crest (click/sat), and the
		// reverb tail. Objective now includes LowMid + Reverb + Timbre terms.
		for _, vf := range []float64{58, 68, 78} {
			for _, h2 := range []float64{0.25, 0.40, 0.55} {
				for _, sat := range []float64{0.4, 0.8, 1.4} {
					for _, click := range []float64{0.5, 0.8} {
						for _, rev := range []float64{0.3, 0.5, 0.7} {
							cands[fmt.Sprintf("vf=%.0f h2=%.2f sat=%.1f clk=%.1f rev=%.1f", vf, h2, sat, click, rev)] = RecipeParams{
								"voice_freq_hz": vf, "gen1_kick_h2": h2, "gen1_kick_sat": sat,
								"gen1_kick_click": click, "gen1_kick_reverb": rev,
								"gen1_kick_h3": 0.14, "gen1_kick_mode_gain": 0.6,
							}
						}
					}
				}
			}
		}
		best, d := tuneKickToRef(refFP, acousticKickSeed, sr, samples, cands)
		fmt.Printf("acoustic BEST: %s  FP-DIST=%.3f\n", best, d)
		reportKick("acoustic BEST", renderSeed(cloneSeed(acousticKickSeed, cands[best]), sr, samples), sr)
	}
	if os.Getenv("WRITE_WAV") != "" {
		writeHits(ref, "../../../../tmp/kickacoustic_ref.wav", sr)
		writeHits(peakNorm(f32toF64(renderSeed(acousticKickSeed, sr, samples))), "../../../../tmp/kickacoustic_synth.wav", sr)
	}
}

// TestKickShippedCheck renders the SHIPPED kick-808 / kick-acoustic through the
// real dispatch (at their table Beats) and prints their metrics, so the buffer-
// normalized fade doesn't silently diverge the shipped sound from the tuning.
func TestKickShippedCheck(t *testing.T) {
	for _, id := range []string{"kick-808", "kick-acoustic"} {
		buf, sr := RenderInstrumentOneShotRaw(id)
		if len(buf) == 0 {
			t.Fatalf("%s: no samples", id)
		}
		fmt.Printf("dispatched %s: %.3fs\n", id, float64(len(buf))/float64(sr))
		reportKick(id+" SHIPPED", buf, sr)
	}
}

func writeHits(x []float64, path string, sr int) {
	gap := make([]float64, sr*2/5)
	var seq []float64
	for i := 0; i < 4; i++ {
		for _, v := range x {
			seq = append(seq, v*0.9)
		}
		seq = append(seq, gap...)
	}
	_ = ExportCaptureToWAV(seq, path)
	_ = math.Abs
}
