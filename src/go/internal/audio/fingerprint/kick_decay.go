package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// Per-band T60 exposes what the whole-signal envelope hides (a ringing sub, a dying click). RING-OUT (peak → LAST window
// above −60 dB), not a slope fit: a first crossing trips on the click-body dip. Never reaching −60 dB clamps and flags.

const (
	// A finer hop than the band trace, so a fast high-band decay has points; 20 ms keeps low-frequency ripple down.
	kickDecayWindowSec = 0.020
	kickDecayHopSec    = 0.005

	// About −40 dB of the loudest band; quieter bands are leakage or noise and get T60 = 0 (skipped in distance).
	kickDecayBandFloorFrac = 0.01

	// Guards the log ratio for near-zero decay times.
	kickDecayEps = 1e-4
)

// kickDecayDropFrac is the amplitude ratio 60 dB below peak (10^(−60/20)).
const kickDecayDropFrac = 0.001

func (fp *KickFingerprint) computeDecay(samples []float64, sr int) {
	fp.BandT60 = make([]float64, KickBandCount)
	fp.BandT60Floored = make([]bool, KickBandCount)
	fp.DecayHopSec = kickDecayHopSec

	win := secToSamples(kickDecayWindowSec, sr)
	hop := secToSamples(kickDecayHopSec, sr)
	fp.DecayHopSec = float64(hop) / float64(sr)
	bands := KickBands()

	// FULL windows only: a short final window's samples land under the ~0 Hann edge of the padded FFT and fabricate a
	// decay to zero.
	env := make([][]float64, KickBandCount)
	for start := 0; start+win <= len(samples); start += hop {
		mag, binHz := wave.MagnitudeSpectrum(wave.Wave{Samples: samples[start : start+win], SampleRate: sr}, kickBandFFT, wave.WindowHann)
		for bi, band := range bands {
			e := bandEnergy(mag, binHz, band.LoHz, band.HiHz)
			env[bi] = append(env[bi], math.Sqrt(e))
		}
	}
	if len(env[0]) == 0 {
		return
	}

	// Loudest band peak → the analyzable-band floor.
	maxPeak := 0.0
	for bi := 0; bi < KickBandCount; bi++ {
		if _, v := argmax(env[bi]); v > maxPeak {
			maxPeak = v
		}
	}
	if maxPeak <= 0 {
		return
	}

	for bi := 0; bi < KickBandCount; bi++ {
		peakIdx, peakVal := argmax(env[bi])
		if peakVal < kickDecayBandFloorFrac*maxPeak {
			continue // empty band → T60 stays 0 (skipped by the distance term)
		}
		remaining := float64(len(env[bi])-1-peakIdx) * fp.DecayHopSec
		t60, floored := bandT60(env[bi], peakIdx, peakVal, fp.DecayHopSec, remaining)
		fp.BandT60[bi] = t60
		fp.BandT60Floored[bi] = floored
	}
}

// (remaining, true) when the last such window is the final one: it never decayed 60 dB.
func bandT60(env []float64, peakIdx int, peakVal, hop, remaining float64) (float64, bool) {
	if peakVal <= 0 || hop <= 0 {
		return remaining, true
	}
	threshold := peakVal * kickDecayDropFrac
	lastAbove := peakIdx
	for i := peakIdx; i < len(env); i++ {
		if env[i] > threshold {
			lastAbove = i
		}
	}
	if lastAbove >= len(env)-1 {
		return remaining, true // still ringing at the end of the signal
	}
	return float64(lastAbove-peakIdx) * hop, false
}
