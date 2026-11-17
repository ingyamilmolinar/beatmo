// Package clone turns a recording of a struck / modal instrument into a
// MEASURED model — damped modes, a strike, a decay law, residual noise bands
// — that later stages invert into an instrument document of synth
// parameters (spec docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md).
// Nothing here copies audio into a document: the output is model numbers.
//
// Stages are pure functions, each testable on generated audio:
// Prepare → EstimateModes → EstimateStrike / FitDecayLaw / MeasureResidual →
// Analyze. Analysis runs at the file's own sample rate; time is seconds from
// the detected onset.
//
// Dev-only: nothing reachable from a shipped binary imports this package
// (TestCloneIsDevOnly).
package clone

import "math"

// Mode is one damped sinusoid of the modal model, t in seconds from the onset:
//
//	x(t) = Amp · e^(−Alpha·t) · cos(2π·Freq·t + Phase)
type Mode struct {
	Freq  float64 `json:"freq_hz"`
	Alpha float64 `json:"alpha_np_per_s"`
	Amp   float64 `json:"amp"`
	Phase float64 `json:"phase_rad"`
	Band  int     `json:"band"`
	SNRdB float64 `json:"snr_db"`
}

// Energy is the mode's total energy ∫x² ≈ Amp²/(4·Alpha) — the ranking key.
func (m Mode) Energy() float64 { return m.Amp * m.Amp / (4 * m.Alpha) }

// T60 is the time for the mode to fall 60 dB.
func (m Mode) T60() float64 { return 3 * math.Ln10 / m.Alpha }

// dB converts a linear amplitude to dB, clamped at −200 so JSON never sees
// −Inf.
func dB(x float64) float64 {
	if x <= 1e-10 {
		return -200
	}
	return 20 * math.Log10(x)
}
