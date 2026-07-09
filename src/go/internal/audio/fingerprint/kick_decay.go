package fingerprint

import (
	"math"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// kick_decay.go — PER-BAND DECAY SPECTROGRAM (per-band T60).
//
// Motivation: a real kick does NOT decay uniformly across frequency. Its sub /
// fundamental rings for hundreds of milliseconds while the beater click and the
// upper body modes die in a handful of milliseconds. The whole-signal envelope
// (EnvTrace) and the reverb-tail scalars collapse that frequency-dependent decay
// into one number, so a synth kick whose sub decays too fast (or whose click
// rings too long) can still match the aggregate envelope. BandT60 exposes the
// decay of EACH frequency region independently, which is exactly what the
// env0/env1 pitch-envelope, per-mode decay, and reverb knobs steer.
//
// For every KickBand it builds the band's short-time RMS-amplitude envelope, then
// measures the decay time as the classic T60: the time for the level to fall by
// 60 dB of the band's peak.
//
// Method — RING-OUT (last-crossing) rather than a slope fit. A real kick band is
// NOT a clean exponential: the peak is the broadband attack-click window, after
// which the tonal body rings and only then decays, so the per-window level is
// non-monotonic (click → dip → body → decay). A least-squares slope over that
// shape is meaningless, and a FIRST −60 dB crossing would trip on the transient
// dip right after the click. Instead T60 is the time from the band's peak to the
// LAST window whose level still exceeds −60 dB of the peak — the ring-out time.
// This is dip-robust (a momentary dip then recovery correctly extends the ring)
// and is EXACT for a clean exponential: e^{-t/τ} = 10^(−60/20) at t = ln(1000)·τ.
// When the band never falls below −60 dB before the signal ends (it does not
// decay within the capture), T60 is clamped to the remaining length and flagged
// in BandT60Floored.

const (
	// Decay envelopes use a finer hop than the band trace so a fast high-band decay
	// (a few tens of ms) still yields enough points to fit a slope; the 20 ms window
	// keeps low-frequency intra-cycle RMS ripple suppressed.
	kickDecayWindowSec = 0.020
	kickDecayHopSec    = 0.005

	// A band is analyzable only if its own envelope peak reaches at least this
	// fraction of the loudest band's peak (≈ −40 dB); quieter bands carry no
	// meaningful decay (FFT leakage / noise) and get T60 = 0 (skipped in distance).
	kickDecayBandFloorFrac = 0.01

	// Decay distance is the mean per-band |log ratio| of T60; this small epsilon
	// (seconds) guards the ratio for near-zero decay times.
	kickDecayEps = 1e-4
)

// kickDecayDropFrac is the amplitude ratio 60 dB below peak (10^(−60/20)).
const kickDecayDropFrac = 0.001

// computeDecay fills fp.BandT60 (+ fp.BandT60Floored) with the per-band T60 decay
// times. Mirrors the computeX pattern in kick_metrics.go: one pass of per-window
// spectra, band energies accumulated into per-band amplitude envelopes, then a
// per-band decay-slope fit.
func (fp *KickFingerprint) computeDecay(samples []float64, sr int) {
	fp.BandT60 = make([]float64, KickBandCount)
	fp.BandT60Floored = make([]bool, KickBandCount)
	fp.DecayHopSec = kickDecayHopSec

	win := secToSamples(kickDecayWindowSec, sr)
	hop := secToSamples(kickDecayHopSec, sr)
	fp.DecayHopSec = float64(hop) / float64(sr)
	bands := KickBands()

	// Per-band amplitude envelope over time (sqrt of band energy per window). FULL
	// windows only: a ragged final short window would have its few real samples
	// land at the start of the zero-padded FFT buffer, where the Hann coefficient
	// is ~0 — reading near-zero energy and fabricating a spurious "decay to zero"
	// at the tail (same discipline as computeHF).
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

// bandT60 returns the −60 dB ring-out time: the interval from the band's peak to
// the LAST window whose level still exceeds 10^(−60/20)·peakVal. Returns
// (remaining, true) — clamped and flagged — when that last window is the final
// window (the band never decays 60 dB within the captured signal).
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
