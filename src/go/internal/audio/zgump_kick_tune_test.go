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

// fpDist is the PRINCIPLED objective: the validated, trace-based kick distance
// from internal/audio/fingerprint (the same metric the guard tests use), so
// tuning targets exactly what the regression net will check. Both signals are
// peak-normalized into a wave.Wave first.
func fpDist(refFP fingerprint.KickFingerprint, x []float64, sr int) (float64, fingerprint.KickDistanceBreakdown) {
	w := wave.Wave{Samples: peakNorm(x), SampleRate: sr}
	b := fingerprint.KickDistance(refFP, fingerprint.KickAnalyze(w))
	return b.Total, b
}

func peakNorm(x []float64) []float64 {
	pk := 0.0
	for _, v := range x {
		if a := math.Abs(v); a > pk {
			pk = a
		}
	}
	out := make([]float64, len(x))
	if pk == 0 {
		return out
	}
	for i, v := range x {
		out[i] = v / pk
	}
	return out
}

// zgump_kick_tune_test.go is the native tuning harness for the modal (variant 6)
// zgump-kick. It renders the seed (and ad-hoc override candidates) through the
// real C DSP, decodes the reference WAV, and prints a TIME-RESOLVED comparison
// (envelope landmarks, per-window bands, pitch glide, crest) so the seed can be
// A/B'd apples-to-apples against 83768__zgump__kick-pack-0708.wav. It is an
// iteration tool, not a regression gate (the guards live in the *_guard_test).
// Run: go test -run TestZgumpKickTune -v ./internal/audio/  (env WRITE_WAV=1 to
// emit audition WAVs under ../../tmp/).

const zgumpRefPath = "../../../../83768__zgump__kick-pack-0708.wav"

func renderKickOverrides(over RecipeParams, sr, samples int) []float32 {
	seed := kickSeedWith(RecipeParams{})
	// start from the zgump seed, then apply overrides
	for k, v := range zgumpKickSeed {
		seed[k] = v
	}
	for k, v := range over {
		seed[k] = v
	}
	mp := recipeParamsToModular(seed)
	buf := make([]float32, samples)
	renderModularP(buf, sr, samples, mp)
	return buf
}

// --- lightweight time-resolved analysis (inline; the permanent metrics live in
// internal/audio/fingerprint/kick_metrics.go) ---

type kickShape struct {
	peak, rms, crest float64
	envPeakMs        float64
	env              []float64 // 5ms-window RMS, normalized to peak
	bands            []float64 // fractions: <60,60-120,120-250,250-500,500-1k,1k-2k,>2k
	pitch            []float64 // 15ms-window dominant 20-400Hz
}

func analyzeKick(x []float64, sr int) kickShape {
	var ks kickShape
	n := len(x)
	for _, v := range x {
		if a := math.Abs(v); a > ks.peak {
			ks.peak = a
		}
		ks.rms += v * v
	}
	ks.rms = math.Sqrt(ks.rms / float64(n))
	if ks.rms > 0 {
		ks.crest = ks.peak / ks.rms
	}
	// envelope, 5ms windows
	win := sr / 200
	mx := 0.0
	for s := 0; s+win <= n; s += win {
		e := 0.0
		for i := s; i < s+win; i++ {
			e += x[i] * x[i]
		}
		e = math.Sqrt(e / float64(win))
		ks.env = append(ks.env, e)
		if e > mx {
			mx = e
			ks.envPeakMs = float64(s) / float64(sr) * 1000
		}
	}
	if mx > 0 {
		for i := range ks.env {
			ks.env[i] /= mx
		}
	}
	// bands over whole signal (Goertzel on a 5Hz grid to 2500Hz)
	edges := []float64{60, 120, 250, 500, 1000, 2000, 1e9}
	ks.bands = make([]float64, len(edges))
	tot := 0.0
	for f := 20.0; f <= 2500; f += 5 {
		m := goertzelMagnitude(x, f, sr)
		e := m * m
		for bi, hi := range edges {
			if f < hi {
				ks.bands[bi] += e
				break
			}
		}
		tot += e
	}
	if tot > 0 {
		for i := range ks.bands {
			ks.bands[i] = ks.bands[i] / tot * 100
		}
	}
	// pitch trace, 15ms windows
	pwin := sr * 15 / 1000
	for s := 0; s+pwin <= n && s < sr*180/1000; s += pwin {
		seg := x[s : s+pwin]
		best, bestMag := 0.0, -1.0
		for f := 30.0; f <= 300; f += 5 {
			if m := goertzelMagnitude(seg, f, sr); m > bestMag {
				bestMag, best = m, f
			}
		}
		ks.pitch = append(ks.pitch, best)
	}
	return ks
}

func (ks kickShape) print(label string) {
	fmt.Printf("── %s\n", label)
	fmt.Printf("   peak=%.3f rms=%.4f crest=%.2f envPeak=%.0fms\n", ks.peak, ks.rms, ks.crest, ks.envPeakMs)
	fmt.Printf("   bands%% <60=%.1f 60-120=%.1f 120-250=%.1f 250-500=%.1f 500-1k=%.1f 1-2k=%.1f >2k=%.1f\n",
		ks.bands[0], ks.bands[1], ks.bands[2], ks.bands[3], ks.bands[4], ks.bands[5], ks.bands[6])
	fmt.Printf("   pitch:")
	for i, p := range ks.pitch {
		fmt.Printf(" %d:%.0f", i*15, p)
	}
	fmt.Printf("\n   env :")
	for i := 0; i < len(ks.env); i += 2 { // 10ms steps
		fmt.Printf(" %.2f", ks.env[i])
	}
	fmt.Printf("\n")
}

func decodeRefMono(t *testing.T, sr int) []float64 {
	pcm, fsr, err := DecodeWAVToPCM(zgumpRefPath)
	if err != nil {
		t.Skipf("reference WAV unavailable: %v", err)
	}
	out := make([]float64, len(pcm))
	for i, v := range pcm {
		out[i] = float64(v)
	}
	if fsr != sr {
		out = resampleLinear(out, fsr, sr)
	}
	// peak-normalize (the synth render is ~full-scale)
	pk := 0.0
	for _, v := range out {
		if a := math.Abs(v); a > pk {
			pk = a
		}
	}
	if pk > 0 {
		for i := range out {
			out[i] /= pk
		}
	}
	return out
}

func resampleLinear(x []float64, from, to int) []float64 {
	if from == to {
		return x
	}
	ratio := float64(to) / float64(from)
	n := int(float64(len(x)) * ratio)
	out := make([]float64, n)
	for i := range out {
		src := float64(i) / ratio
		j := int(src)
		f := src - float64(j)
		if j+1 < len(x) {
			out[i] = x[j]*(1-f) + x[j+1]*f
		} else if j < len(x) {
			out[i] = x[j]
		}
	}
	return out
}

// TestRawKickTune renders the SHIPPED raw-kick (the rawer zgump clone) next to
// zgump and the reference so its grittier/heavier character can be A/B'd by ear.
// Run: go test -run TestRawKickTune -v ./internal/audio/  (WRITE_WAV=1 for WAVs).
func TestRawKickTune(t *testing.T) {
	const sr = 44100
	ref := decodeRefMono(t, sr)
	refFP := fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(ref), SampleRate: sr})
	fpScalars("REF  ", refFP)

	zg, _ := RenderInstrumentOneShotRaw("zgump-kick")
	raw, rsr := RenderInstrumentOneShotRaw("raw-kick")
	zgKS := analyzeKick(peakNorm(f32toF64(zg)), sr)
	rawKS := analyzeKick(peakNorm(f32toF64(raw)), rsr)
	zgKS.print("ZGUMP-KICK")
	fpScalars("ZGUMP", fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(f32toF64(zg)), SampleRate: sr}))
	rawKS.print("RAW-KICK")
	fpScalars("RAW  ", fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(f32toF64(raw)), SampleRate: rsr}))

	// Raw should be gritter (more HF/noise energy) and no brighter-than-kick.
	rawHF := rawKS.bands[4] + rawKS.bands[5] + rawKS.bands[6] // >500 Hz
	zgHF := zgKS.bands[4] + zgKS.bands[5] + zgKS.bands[6]
	fmt.Printf("HF energy >500Hz: zgump=%.2f%% raw=%.2f%% (raw should be >= zgump — grittier)\n", zgHF, rawHF)

	if os.Getenv("WRITE_WAV") != "" {
		gap := make([]float64, sr*2/5)
		var seq []float64
		for i := 0; i < 4; i++ {
			for _, v := range peakNorm(f32toF64(raw)) {
				seq = append(seq, v*0.9)
			}
			seq = append(seq, gap...)
		}
		_ = ExportCaptureToWAV(seq, "../../../../tmp/rawkick_synth.wav")
		fmt.Println("wrote ../../../../tmp/rawkick_synth.wav (4 hits)")
	}
}

func fpScalars(tag string, fp fingerprint.KickFingerprint) {
	fmt.Printf("   %sfp: pitchStart=%.0f settle=%.1f glide=%.0fms attkPeak=%.0fms rise=%.0fms plateau=%.2f bloom=%v@%.0fms(%.2f) crest=%.2f\n",
		tag, fp.PitchStartHz, fp.PitchSettleHz, fp.GlideSec*1000, fp.AttackPeakSec*1000, fp.AttackRiseSec*1000,
		fp.PlateauFlatness, fp.TailBloomPresent, fp.TailBloomSec*1000, fp.TailBloomRatio, fp.Crest)
	fmt.Printf("   %sfp pitchTrace:", tag)
	for i, p := range fp.PitchTrace {
		if i > 12 {
			break
		}
		fmt.Printf(" %.0f", p)
	}
	fmt.Printf("\n")
}

func TestZgumpKickTune(t *testing.T) {
	const sr = 44100
	ref := decodeRefMono(t, sr)
	samples := len(ref)
	refKS := analyzeKick(ref, sr)
	fmt.Printf("reference %d samples (%.3fs)\n", samples, float64(samples)/sr)
	refKS.print("REFERENCE zgump")
	refFP := fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(ref), SampleRate: sr})
	fpScalars("REF ", refFP)

	dbuf, dsr := RenderInstrumentOneShotRaw("zgump-kick")
	fmt.Printf("dispatched zgump-kick: %d samples @ %dHz (%.3fs)\n", len(dbuf), dsr, float64(len(dbuf))/float64(dsr))
	fpScalars("DISP ", fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(f32toF64(dbuf)), SampleRate: dsr}))

	baseSyn := f32toF64(renderKickOverrides(nil, sr, samples))
	analyzeKick(baseSyn, sr).print("SYNTH zgumpKickSeed (current)")
	fpScalars("SYN ", fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(baseSyn), SampleRate: sr}))
	d0, b0 := fpDist(refFP, baseSyn, sr)
	fmt.Printf("   FP-DIST=%.4f (env=%.3f pitch=%.3f bands=%.3f land=%.3f crest=%.3f smooth=%.3f)\n",
		d0, b0.EnvShape, b0.Pitch, b0.Bands, b0.Landmarks, b0.Crest, b0.Smoothness)

	// Candidate sweep: knob overrides tried against the reference; the harness
	// prints each candidate's distance so tuning converges without a rebuild.
	candidates := map[string]RecipeParams{}
	if os.Getenv("SWEEP") != "" {
		for _, vf := range []float64{74, 76, 78} {
			for _, peA := range []float64{1.9, 2.1} {
				for _, peR := range []float64{40, 50} {
					for _, h2 := range []float64{0.18, 0.21} {
						for _, click := range []float64{0.12, 0.18, 0.24} {
							for _, det := range []float64{0.10, 0.12, 0.15} {
								name := fmt.Sprintf("vf=%.0f peA=%.1f peR=%.0f h2=%.2f clk=%.2f det=%.2f", vf, peA, peR, h2, click, det)
								candidates[name] = RecipeParams{
									"voice_freq_hz": vf, "gen1_kick_pe_amt": peA, "gen1_kick_pe_rate": peR,
									"gen1_kick_h2": h2, "gen1_kick_click": click, "gen1_kick_noise": 0.04, "gen1_kick_fade": 4.0,
									"gen1_kick_mode_detune": det, "gen1_kick_mode_gain": 1.3, "gen1_kick_mode_decay": 3.0,
								}
							}
						}
					}
				}
			}
		}
	}
	best, bestD := "", 1e9
	bestBand, bestBandD := "", 1e9 // best subject to bands 60-120 in [65,74]% (perceptual weight)
	for name, over := range candidates {
		syn := f32toF64(renderKickOverrides(over, sr, samples))
		d, _ := fpDist(refFP, syn, sr)
		if d < bestD {
			bestD, best = d, name
		}
		ks := analyzeKick(syn, sr)
		if ks.bands[1] >= 65 && ks.bands[1] <= 74 && d < bestBandD {
			bestBandD, bestBand = d, name
		}
	}
	if bestBand != "" {
		bsyn := f32toF64(renderKickOverrides(candidates[bestBand], sr, samples))
		_, bb := fpDist(refFP, bsyn, sr)
		fmt.Printf("BAND-ACCURATE best: %s  FP-DIST=%.4f (env=%.3f pitch=%.3f bands=%.3f land=%.3f crest=%.3f smooth=%.3f)\n",
			bestBand, bestBandD, bb.EnvShape, bb.Pitch, bb.Bands, bb.Landmarks, bb.Crest, bb.Smoothness)
		analyzeKick(bsyn, sr).print("BAND-ACCURATE " + bestBand)
		fpScalars("BANDBEST ", fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(bsyn), SampleRate: sr}))
	}
	if best != "" {
		bsyn := f32toF64(renderKickOverrides(candidates[best], sr, samples))
		_, bb := fpDist(refFP, bsyn, sr)
		fmt.Printf("BEST candidate: %s  FP-DIST=%.4f (env=%.3f pitch=%.3f bands=%.3f land=%.3f crest=%.3f smooth=%.3f)\n",
			best, bestD, bb.EnvShape, bb.Pitch, bb.Bands, bb.Landmarks, bb.Crest, bb.Smoothness)
		analyzeKick(bsyn, sr).print("BEST " + best)
		fpScalars("BEST ", fingerprint.KickAnalyze(wave.Wave{Samples: peakNorm(bsyn), SampleRate: sr}))
	}

	if os.Getenv("WRITE_WAV") != "" {
		// four hits, 0.4s apart, peak-normalized to ~0.9 for a fair loudness A/B.
		syn := peakNorm(f32toF64(renderKickOverrides(nil, sr, samples)))
		refN := peakNorm(ref)
		gap := make([]float64, sr*2/5)
		var refSeq, synSeq []float64
		for i := 0; i < 4; i++ {
			for _, v := range refN {
				refSeq = append(refSeq, v*0.9)
			}
			refSeq = append(refSeq, gap...)
			for _, v := range syn {
				synSeq = append(synSeq, v*0.9)
			}
			synSeq = append(synSeq, gap...)
		}
		_ = ExportCaptureToWAV(refSeq, "../../../../tmp/zgump_ref.wav")
		_ = ExportCaptureToWAV(synSeq, "../../../../tmp/zgump_synth.wav")
		fmt.Println("wrote ../../../../tmp/zgump_ref.wav and ../../../../tmp/zgump_synth.wav (4 hits each, normalized)")
	}
}
