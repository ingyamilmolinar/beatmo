//go:build !js

// analyze.go — testable core of synth-analyze. Split from main.go so that
// analyzeFile, analyzeOpts, and printFP are all available under -tags test
// (main.go is excluded by its !test guard).
package main

import (
	"fmt"
	"io"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// f32ToF64 converts a []float32 slice to []float64.
func f32ToF64(in []float32) []float64 {
	out := make([]float64, len(in))
	for i, v := range in {
		out[i] = float64(v)
	}
	return out
}

// analyzeOpts controls how analyzeFile segments and labels the audio.
type analyzeOpts struct {
	OffsetSec float64
	LengthSec float64 // <=0 means auto-segment (up to 1.5 s of sustained material)
	Label     string
}

// analyzeFile decodes the audio at path at its NATIVE sample rate (no
// resampling) and returns the computed Fingerprint. Under -tags test,
// audio.DecodeWAVToPCM is the pure-Go stdlib decoder; under production builds
// it is the miniaudio-backed decoder. Either way no resampling is applied
// before fingerprint.FromWave.
//
// If opts.Label is empty the file path is used as the fingerprint label.
func analyzeFile(path string, opts analyzeOpts) (fingerprint.Fingerprint, error) {
	pcm, sr, err := audio.DecodeWAVToPCM(path)
	if err != nil {
		return fingerprint.Fingerprint{}, fmt.Errorf("analyzeFile: decode %q: %w", path, err)
	}

	lbl := opts.Label
	if lbl == "" {
		lbl = path
	}

	w := wave.Wave{Samples: f32ToF64(pcm), SampleRate: sr}

	// Apply offset.
	if opts.OffsetSec > 0 {
		skip := int(opts.OffsetSec * float64(sr))
		if skip >= len(w.Samples) {
			return fingerprint.Fingerprint{}, fmt.Errorf("analyzeFile: offset %.2f s exceeds file length", opts.OffsetSec)
		}
		w.Samples = w.Samples[skip:]
	}

	// Retain the full (post-offset, pre-segment) wave to compute whole-note
	// macro coupling after segmentation, so fp.Coupling.MacroCentroidRMSCorr
	// reflects the true attack+sustain+decay coupling, not a window-local value.
	fullW := w

	// Crop or auto-segment.
	if opts.LengthSec > 0 {
		durSamples := int(opts.LengthSec * float64(sr))
		if durSamples < len(w.Samples) {
			w.Samples = w.Samples[:durSamples]
		}
	} else {
		// Auto-segment: find up to 1.5 s of sustained material.
		w = fingerprint.AutoSegment(w, 1.5)
	}

	fp := fingerprint.FromWave(w, lbl)

	// Overwrite MacroCentroidRMSCorr with the whole-note value so the printed
	// and returned fingerprint reflects the full-note expressive coupling rather
	// than the window-local value computed by FromWave over the segmented clip.
	if fp.Coupling != nil {
		fp.Coupling.MacroCentroidRMSCorr = fingerprint.WholeSignalCentroidRMSCorr(fullW, fingerprint.DefaultAnalysisConfig())
	}

	return fp, nil
}

// printFP writes the human-readable fingerprint summary to w. Callers pass
// os.Stdout for normal output and os.Stderr in -json mode (so the summary does
// not corrupt the machine-readable JSON on stdout — e.g. `... -json > fp.json`).
func printFP(w io.Writer, fp *fingerprint.Fingerprint) {
	fmt.Fprintf(w, "\n=== Fingerprint: %q ===\n", fp.Label)
	fmt.Fprintf(w, "  Sample rate:       %d Hz\n", fp.SampleRate)
	fmt.Fprintf(w, "  F0:                %.2f Hz\n", fp.F0Hz)
	fmt.Fprintf(w, "  SpectralCentroid:  %.2f Hz\n", fp.SpectralCentroid)
	fmt.Fprintf(w, "  SpectralRolloff:   %.2f Hz\n", fp.SpectralRolloff)
	fmt.Fprintf(w, "  SpectralCrest:     %.4f\n", fp.SpectralCrest)
	fmt.Fprintf(w, "  SpectralSkewness:  %.4f\n", fp.SpectralSkewness)
	fmt.Fprintf(w, "  SpectralKurtosis:  %.4f\n", fp.SpectralKurtosis)
	fmt.Fprintf(w, "  Tristimulus:       [%.4f, %.4f, %.4f]\n", fp.Tristimulus[0], fp.Tristimulus[1], fp.Tristimulus[2])
	fmt.Fprintf(w, "  EvenOddRatio:      %.4f\n", fp.EvenOddRatio)
	fmt.Fprintf(w, "  Inharmonicity:     %.6f\n", fp.Inharmonicity)
	fmt.Fprintf(w, "  AttackTime:        %.4f sec\n", fp.AttackTimeSec)
	fmt.Fprintf(w, "  DecaySlope:        %.4f dB/sec\n", fp.DecaySlope)
	fmt.Fprintf(w, "  ReleaseTime:       %.4f sec\n", fp.ReleaseTimeSec)
	fmt.Fprintf(w, "  HNR:               %.4f dB\n", fp.HNR)
	fmt.Fprintf(w, "  NoiseRatio:        %.4f\n", fp.NoiseRatio)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Partials (normalized, F0=%.1f Hz):\n", fp.F0Hz)
	for k, p := range fp.Partials {
		bar := ""
		stars := int(p * 20)
		if stars > 40 {
			stars = 40
		}
		for i := 0; i < stars; i++ {
			bar += "█"
		}
		fmt.Fprintf(w, "    P%-2d (%6.1f Hz): %6.4f  %s\n",
			k+1, fp.F0Hz*float64(k+1), p, bar)
	}
	fmt.Fprintln(w)

	if t := fp.Temporal; t != nil {
		fmt.Fprintf(w, "  --- Temporal (STFT) ---\n")
		fmt.Fprintf(w, "  Frames:            %d\n", t.NumFrames)
		fmt.Fprintf(w, "  CentroidMean:      %.2f Hz\n", t.CentroidMean)
		fmt.Fprintf(w, "  CentroidSlope:     %.2f Hz/sec (positive=brightening, negative=darkening)\n", t.CentroidSlope)
		fmt.Fprintf(w, "  CentroidRange:     %.2f Hz (organic > ~200Hz; static synth < ~50Hz)\n", t.CentroidRange)
		fmt.Fprintf(w, "  HarmonicFluxMean:  %.6f (organic > 0.01; static synth ≈ 0)\n", t.HarmonicFluxMean)
		fmt.Fprintf(w, "  SpectralVariance:  %.6f\n", t.SpectralVariance)
		if t.VibratoDetected {
			fmt.Fprintf(w, "  VibratoDetected:   true [%.2f Hz, %.2f cents]\n", t.VibratoRateHz, t.VibratoDepthCents)
		} else {
			fmt.Fprintf(w, "  VibratoDetected:   false [depth=%.2f cents]\n", t.VibratoDepthCents)
		}
		fmt.Fprintf(w, "  PartialDecays (dB/sec): P1=%.2f P2=%.2f P3=%.2f P4=%.2f P5=%.2f P6=%.2f\n",
			t.PartialDecaySlopes[0], t.PartialDecaySlopes[1], t.PartialDecaySlopes[2],
			t.PartialDecaySlopes[3], t.PartialDecaySlopes[4], t.PartialDecaySlopes[5])
		fmt.Fprintln(w)
	}

	// ---- Vibrato ----------------------------------------------------------------
	if vib := fp.Vibrato; vib != nil {
		fmt.Fprintf(w, "  --- Vibrato ---\n")
		fmt.Fprintf(w, "  Detected:          %v\n", vib.Detected)
		fmt.Fprintf(w, "  RateHz:            %.2f Hz\n", vib.RateHz)
		fmt.Fprintf(w, "  ExtentCents:       %.2f cents\n", vib.ExtentCents)
		fmt.Fprintf(w, "  Jitter:            %.4f (std/mean of cycle periods)\n", vib.Jitter)
		fmt.Fprintf(w, "  AMDepth:           %.4f (std/mean of frame RMS)\n", vib.AMDepth)
		fmt.Fprintf(w, "  OnsetSec:          %.4f sec\n", vib.OnsetSec)
		fmt.Fprintln(w)
	}

	// ---- Spectral Envelope (Bridge Hill + Formants) -----------------------------
	if env := fp.SpectralEnv; env != nil {
		fmt.Fprintf(w, "  --- Spectral Envelope ---\n")
		if env.BridgeHillHz > 0 {
			fmt.Fprintf(w, "  Bridge Hill:       %.1f Hz  (%.2f dB)\n", env.BridgeHillHz, env.BridgeHillGainDB)
		} else {
			fmt.Fprintf(w, "  Bridge Hill:       (absent)\n")
		}
		maxFormants := 5
		if len(env.Formants) < maxFormants {
			maxFormants = len(env.Formants)
		}
		if maxFormants > 0 {
			fmt.Fprintf(w, "  Top formants (%d):\n", maxFormants)
			for i := 0; i < maxFormants; i++ {
				f := env.Formants[i]
				fmt.Fprintf(w, "    F%d: %.1f Hz  BW=%.1f Hz  %.2f dB\n",
					i+1, f.Hz, f.BandwidthHz, f.GainDB)
			}
		}
		fmt.Fprintln(w)
	}

	// ---- Dynamic Coupling -------------------------------------------------------
	if cpl := fp.Coupling; cpl != nil {
		fmt.Fprintf(w, "  --- Coupling ---\n")
		fmt.Fprintf(w, "  CentroidRMSCorr:      %.4f (micro: brightness~loudness in sustain; bowed strings > 0.5 or < -0.5)\n", cpl.CentroidRMSCorr)
		fmt.Fprintf(w, "  MacroCentroidRMSCorr: %.4f (macro: whole-note swell coupling; expressive crescendo > 0)\n", cpl.MacroCentroidRMSCorr)
		fmt.Fprintf(w, "  BrightnessModDepth:   %.4f (std/mean centroid; organic > 0.05)\n", cpl.BrightnessModDepth)
		fmt.Fprintf(w, "  BrightnessModRateHz:  %.2f Hz\n", cpl.BrightnessModRateHz)
		fmt.Fprintln(w)
	}

	// ---- Harmonic Trajectory ----------------------------------------------------
	if h := fp.Harmonics; h != nil {
		fmt.Fprintf(w, "  --- Harmonics (N=%d) ---\n", h.N)
		n := h.N
		if n > 4 {
			n = 4 // print up to 4 harmonics as a representative sample
		}
		for k := 0; k < n; k++ {
			fmt.Fprintf(w, "  H%-2d: AttackRate=%.2f dB/s  PeakAt=%.3f s  SustainLvl=%.4f  DecayRate=%.2f dB/s\n",
				k+1, h.AttackRate[k], h.PeakTimeSec[k], h.SustainLevel[k], h.DecayRate[k])
		}
		if h.N > 4 {
			fmt.Fprintf(w, "  ... (%d more harmonics tracked)\n", h.N-4)
		}
		fmt.Fprintln(w)
	}

	// ---- Noise Profile ----------------------------------------------------------
	if nz := fp.NoiseProf; nz != nil {
		fmt.Fprintf(w, "  --- Noise Profile ---\n")
		fmt.Fprintf(w, "  ResidualRatio:     %.4f (fraction of energy in non-harmonic bins)\n", nz.ResidualRatio)
		fmt.Fprintf(w, "  ResidualCentroidHz: %.1f Hz (centroid of residual spectrum)\n", nz.ResidualCentroidHz)
		fmt.Fprintln(w)
	}
}
