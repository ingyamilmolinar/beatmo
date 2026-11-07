package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Callers must use the returned binHz: MagnitudeSpectrum rounds FrameFFTSize up to a power of two, and recomputing it
// mis-scales band and chroma indices.
func stftHops(w wave.Wave, cfg AnalysisConfig) (frames [][]float64, frameHz float64, binHz float64) {
	sr := w.SampleRate
	if sr == 0 {
		return nil, 0, 0
	}
	hop := int(cfg.HopSec * float64(sr))
	if hop < 1 {
		hop = 1
	}
	frameLen := cfg.FrameFFTSize
	for start := 0; start+frameLen <= len(w.Samples); start += hop {
		seg := wave.Wave{Samples: w.Samples[start : start+frameLen], SampleRate: sr}
		mag, bh := wave.MagnitudeSpectrum(seg, frameLen, wave.WindowHann)
		binHz = bh // identical every iteration; last write is fine
		frames = append(frames, mag)
	}
	return frames, float64(sr) / float64(hop), binHz
}

func OnsetEnvelope(w wave.Wave, cfg AnalysisConfig) ([]float64, float64) {
	frames, fhz, _ := stftHops(w, cfg)
	if len(frames) < 2 {
		return nil, fhz
	}
	env := make([]float64, len(frames))
	for i := 1; i < len(frames); i++ {
		flux := 0.0
		prev, cur := frames[i-1], frames[i]
		for k := range cur {
			d := cur[k] - prev[k]
			if d > 0 {
				flux += d
			}
		}
		env[i] = flux
	}
	return env, fhz
}

// OnsetTimes returns the times (sec) of envelope peaks above mean+sensitivity·std.
func OnsetTimes(env []float64, frameHz float64, cfg AnalysisConfig) []float64 {
	if len(env) == 0 || frameHz <= 0 {
		return nil
	}
	mean, m2 := 0.0, 0.0
	for _, v := range env {
		mean += v
	}
	mean /= float64(len(env))
	for _, v := range env {
		m2 += (v - mean) * (v - mean)
	}
	std := math.Sqrt(m2 / float64(len(env)))
	thresh := mean + cfg.OnsetSensitivity*std
	var times []float64
	for i := 1; i < len(env)-1; i++ {
		if env[i] > thresh && env[i] >= env[i-1] && env[i] >= env[i+1] {
			times = append(times, float64(i)/frameHz)
		}
	}
	return times
}

// Confidence = peak autocorrelation over zero-lag energy.
func DetectTempo(env []float64, frameHz float64, cfg AnalysisConfig) (float64, float64) {
	n := len(env)
	if n < 4 || frameHz <= 0 {
		return 0, 0
	}
	// Lag range (in frames) from BPM range: bpm = 60*frameHz/lag.
	lagMin := int(60.0 * frameHz / cfg.TempoMaxBPM)
	lagMax := int(60.0 * frameHz / cfg.TempoMinBPM)
	if lagMin < 1 {
		lagMin = 1
	}
	if lagMax >= n {
		lagMax = n - 1
	}
	zero := 0.0
	for _, v := range env {
		zero += v * v
	}
	if zero <= 0 {
		return 0, 0
	}
	bestLag, bestAC := 0, 0.0
	for lag := lagMin; lag <= lagMax; lag++ {
		ac := 0.0
		for i := lag; i < n; i++ {
			ac += env[i] * env[i-lag]
		}
		if ac > bestAC {
			bestAC = ac
			bestLag = lag
		}
	}
	if bestLag == 0 {
		return 0, 0
	}
	bpm := 60.0 * frameHz / float64(bestLag)
	// The autocorrelation often locks onto the bar or harmonic-rhythm period instead of the tactus.
	for bpm > 0 && bpm < tempoFoldFloorBPM {
		bpm *= 2
	}
	return bpm, bestAC / zero
}

const tempoFoldFloorBPM = 55.0
