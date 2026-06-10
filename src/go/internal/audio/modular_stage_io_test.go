//go:build !test && !js

package audio

import (
	"math"
	"testing"
)

// Per-stage input/output contract for the modular voice pipeline
// (osc → [FM] → env → filter → drive/gain, src/c/modular.c).
//
// The per-stage enable bits double as TAPS: rendering with stage k disabled is
// the stage's INPUT, with it enabled its OUTPUT — same upstream samples, same
// C code path. Each test below measures the signal before/after exactly one
// stage (spectral content via Goertzel, envelope via windowed RMS, exact
// identity via SHA256) for deterministic simple waves of every generator type.
//
// Spectral tests disable the envelope so the wave is steady-state (no leakage
// from amplitude shaping); biquad tests skip the filter's transient head.

// renderMP is the common render harness: 0.25s at 48 kHz.
func renderMP(t *testing.T, p ModularParams) []float32 {
	t.Helper()
	const sr = 48000
	n := sr / 4
	buf := make([]float32, n)
	renderModularP(buf, sr, n, p)
	assertFinite(t, buf)
	return buf
}

const stageIOSampleRate = 48000

// steadyWave returns params for a steady-state oscillator of the given type:
// env/filter/drive all disabled so the generator output reaches the buffer
// untouched. Base pitch is the engine's 220 Hz.
func steadyWave(oscType float64) ModularParams {
	p := identityModularParams()
	p.OscType = oscType
	p.EnvEnabled = 0
	p.FilterEnabled = 0
	p.DriveEnabled = 0
	return p
}

func f32to64(buf []float32) []float64 {
	out := make([]float64, len(buf))
	for i, v := range buf {
		out[i] = float64(v)
	}
	return out
}

// mag measures the Goertzel magnitude at freq over the steady tail of buf
// (the first 4000 samples are dropped: filter/oscillator transients).
func mag(buf []float32, freq float64) float64 {
	tail := buf
	if len(buf) > 4000 {
		tail = buf[4000:]
	}
	return goertzelMagnitude(f32to64(tail), freq, stageIOSampleRate)
}

// windowRMS computes RMS over buf[lo:hi].
func windowRMS(buf []float32, lo, hi int) float64 {
	if lo < 0 {
		lo = 0
	}
	if hi > len(buf) {
		hi = len(buf)
	}
	if hi <= lo {
		return 0
	}
	var e float64
	for _, v := range buf[lo:hi] {
		e += float64(v) * float64(v)
	}
	return math.Sqrt(e / float64(hi-lo))
}

// ── Oscillator/generator stage: spectral signature per wave type ──────────

// TestModularStageIO_WaveSignatures locks each generator's spectral identity
// at the oscillator tap (everything downstream bypassed). Fundamental 220 Hz;
// harmonic structure distinguishes the four band-limited shapes; the two
// noise colors are distinguished by their low/high spectral tilt.
func TestModularStageIO_WaveSignatures(t *testing.T) {
	const fund = 220.0

	harmonicProfile := func(buf []float32) (f, h2, h3 float64) {
		return mag(buf, fund), mag(buf, 2*fund), mag(buf, 3*fund)
	}

	t.Run("sine", func(t *testing.T) {
		f, h2, h3 := harmonicProfile(renderMP(t, steadyWave(0)))
		if f <= 0 {
			t.Fatalf("sine fundamental missing (mag=%v)", f)
		}
		if h2 > 0.02*f || h3 > 0.02*f {
			t.Errorf("sine has harmonics: fund=%v h2=%v h3=%v (want pure tone)", f, h2, h3)
		}
	})
	t.Run("saw", func(t *testing.T) {
		f, h2, h3 := harmonicProfile(renderMP(t, steadyWave(1)))
		// Ideal saw: h2 = fund/2, h3 = fund/3.
		if h2 < 0.3*f || h3 < 0.2*f {
			t.Errorf("saw harmonic series wrong: fund=%v h2=%v h3=%v (want h2≈f/2, h3≈f/3)", f, h2, h3)
		}
	})
	t.Run("square", func(t *testing.T) {
		f, h2, h3 := harmonicProfile(renderMP(t, steadyWave(2)))
		// Ideal square: odd harmonics only, h3 = fund/3.
		if h3 < 0.2*f {
			t.Errorf("square 3rd harmonic missing: fund=%v h3=%v (want ≈f/3)", f, h3)
		}
		if h2 > 0.05*f {
			t.Errorf("square has even harmonic: fund=%v h2=%v (want ≈0)", f, h2)
		}
	})
	t.Run("triangle", func(t *testing.T) {
		f, h2, h3 := harmonicProfile(renderMP(t, steadyWave(3)))
		// Ideal triangle: odd harmonics, h3 = fund/9 — present but much weaker
		// than a square's f/3.
		if h3 < 0.05*f || h3 > 0.2*f {
			t.Errorf("triangle 3rd harmonic out of range: fund=%v h3=%v (want ≈f/9)", f, h3)
		}
		if h2 > 0.05*f {
			t.Errorf("triangle has even harmonic: fund=%v h2=%v (want ≈0)", f, h2)
		}
	})
	t.Run("noise-tilt", func(t *testing.T) {
		// Spectral tilt: average band magnitude low (80–140 Hz) vs high
		// (7–8.5 kHz). White is flat (ratio ≈ 1); pink rises toward DC.
		band := func(buf []float32, freqs []float64) float64 {
			var s float64
			for _, fr := range freqs {
				s += mag(buf, fr)
			}
			return s / float64(len(freqs))
		}
		low := []float64{80, 100, 120, 140}
		high := []float64{7000, 7500, 8000, 8500}
		white := renderMP(t, steadyWave(5))
		pink := renderMP(t, steadyWave(6))
		whiteTilt := band(white, low) / math.Max(band(white, high), 1e-12)
		pinkTilt := band(pink, low) / math.Max(band(pink, high), 1e-12)
		if !(pinkTilt > 2*whiteTilt) {
			t.Errorf("pink noise is not low-tilted vs white: pinkTilt=%v whiteTilt=%v", pinkTilt, whiteTilt)
		}
		if peakAbs(white) <= 0 || peakAbs(pink) <= 0 {
			t.Fatalf("noise generators silent: white=%v pink=%v", peakAbs(white), peakAbs(pink))
		}
	})
}

// ── Envelope stage: amplitude shaping before/after ────────────────────────

// TestModularStageIO_EnvelopeStage compares the saw generator's raw output
// (env bypassed = stage input) with the enveloped output (= stage output):
// attack ramps the head up from silence, sustain holds ≈ sustain×input level,
// and the bypassed signal stays flat across the note.
func TestModularStageIO_EnvelopeStage(t *testing.T) {
	in := steadyWave(1) // saw input
	out := in
	out.EnvEnabled = 1
	out.AmpAttack = 0.05 // 2400 samples — long enough to measure the ramp
	out.AmpDecay = 0.05
	out.AmpSustain = 0.5
	out.AmpRelease = 0.05
	out.AmpCurve = 0 // linear, so sustain level is exact

	inBuf := renderMP(t, in)
	outBuf := renderMP(t, out)
	n := len(inBuf)

	// Input (bypassed): flat amplitude — head and tail windows within 10%.
	inHead := windowRMS(inBuf, 0, n/8)
	inTail := windowRMS(inBuf, 7*n/8, n)
	if math.Abs(inHead-inTail) > 0.1*inHead {
		t.Errorf("bypassed envelope input is not flat: head=%v tail=%v", inHead, inTail)
	}

	// Output: the first 5ms sit inside the attack ramp → much quieter than the
	// same window of the input.
	atkWin := stageIOSampleRate / 200 // 5 ms
	outAtk := windowRMS(outBuf, 0, atkWin)
	inAtk := windowRMS(inBuf, 0, atkWin)
	if !(outAtk < 0.5*inAtk) {
		t.Errorf("attack did not shape the head: out=%v in=%v", outAtk, inAtk)
	}

	// Sustain plateau (after attack+decay, before release): ≈ sustain × input.
	susLo := int(0.15 * float64(stageIOSampleRate)) // 150 ms — well into sustain
	susHi := n / 2                                  // release starts at n - rel (capped n/2)
	outSus := windowRMS(outBuf, susLo, susHi)
	inSus := windowRMS(inBuf, susLo, susHi)
	ratio := outSus / inSus
	if ratio < 0.4 || ratio > 0.6 {
		t.Errorf("sustain level wrong: out/in RMS = %v, want ≈0.5 (linear sustain=0.5)", ratio)
	}
}

// ── Filter stage: spectral shaping before/after ───────────────────────────

// TestModularStageIO_FilterStage measures the saw's harmonics before (filter
// bypassed) and after each filter type, asserting the pass/stop behavior of
// LP, HP, and BP at the band edges.
func TestModularStageIO_FilterStage(t *testing.T) {
	const fund = 220.0
	input := renderMP(t, steadyWave(1)) // saw, filter bypassed
	inFund := mag(input, fund)
	inH5 := mag(input, 5*fund) // 1100 Hz

	filtered := func(ft, cutoff, q float64) []float32 {
		p := steadyWave(1)
		p.FilterEnabled = 1
		p.FilterType = ft
		p.FilterCutoff = cutoff
		p.FilterResonance = q
		return renderMP(t, p)
	}

	t.Run("lowpass", func(t *testing.T) {
		out := filtered(0, 500, 0.707)
		if g := mag(out, fund) / inFund; g < 0.5 {
			t.Errorf("LP 500: fundamental attenuated too much (gain=%v)", g)
		}
		// 2nd-order (12 dB/oct) LP at 500 Hz ⇒ ≈ -13.7 dB at 1100 Hz (≈0.21).
		if g := mag(out, 5*fund) / inH5; g > 0.3 {
			t.Errorf("LP 500: 1100 Hz not attenuated (gain=%v, want < 0.3 for a 12 dB/oct slope)", g)
		}
	})
	t.Run("highpass", func(t *testing.T) {
		out := filtered(1, 2000, 0.707)
		if g := mag(out, fund) / inFund; g > 0.1 {
			t.Errorf("HP 2000: fundamental not attenuated (gain=%v, want < 0.1)", g)
		}
		inH10 := mag(input, 10*fund) // 2200 Hz — just above cutoff
		if g := mag(out, 10*fund) / inH10; g < 0.4 {
			t.Errorf("HP 2000: 2200 Hz attenuated too much (gain=%v)", g)
		}
	})
	t.Run("bandpass", func(t *testing.T) {
		out := filtered(2, 3*fund, 2)
		gFund := mag(out, fund) / inFund
		inH3 := mag(input, 3*fund)
		gH3 := mag(out, 3*fund) / inH3
		if !(gH3 > 3*gFund) {
			t.Errorf("BP 660: band not selective (gain@660=%v gain@220=%v)", gH3, gFund)
		}
	})
}

// ── Drive stage: harmonic generation before/after ─────────────────────────

// TestModularStageIO_DriveStage feeds a pure sine into the drive stage: the
// bypassed input has no 3rd harmonic; tanh saturation must create one. The
// output must stay bounded.
func TestModularStageIO_DriveStage(t *testing.T) {
	const fund = 220.0
	in := steadyWave(0) // pure sine input
	out := in
	out.DriveEnabled = 1
	out.Drive = 0.8

	inBuf := renderMP(t, in)
	outBuf := renderMP(t, out)

	inH3 := mag(inBuf, 3*fund)
	inF := mag(inBuf, fund)
	outH3 := mag(outBuf, 3*fund)
	outF := mag(outBuf, fund)
	if inH3 > 0.02*inF {
		t.Fatalf("sine input already has h3 (%v of fund) — bad input tap", inH3/inF)
	}
	if outH3 < 0.05*outF {
		t.Errorf("drive 0.8 created no 3rd harmonic: h3=%v fund=%v", outH3, outF)
	}
	if pk := peakAbs(outBuf); pk > 1.01 {
		t.Errorf("drive output exceeds unity: peak=%v", pk)
	}
}

// ── Gain stage: exact scaling ──────────────────────────────────────────────

func TestModularStageIO_GainStage(t *testing.T) {
	unity := steadyWave(0)
	half := unity
	half.Gain = 0.5
	u := renderMP(t, unity)
	h := renderMP(t, half)
	for i := range u {
		want := u[i] * 0.5
		if d := math.Abs(float64(h[i] - want)); d > 1e-6 {
			t.Fatalf("gain 0.5 is not an exact 0.5× scale at sample %d: got %v want %v", i, h[i], want)
		}
	}
}

// ── Bypass contract: a disabled stage's params have NO authority ──────────

// TestModularStageIO_BypassRemovesParamAuthority renders each stage disabled
// with two contrasting settings of that stage's params — the outputs must be
// BIT-IDENTICAL. This is the property that makes the enable pills trustworthy
// taps (and is exactly what a stale ABI would break: a half-written enable
// flag silently re-couples the params).
func TestModularStageIO_BypassRemovesParamAuthority(t *testing.T) {
	cases := []struct {
		name    string
		mutateA func(*ModularParams)
		mutateB func(*ModularParams)
	}{
		{
			name: "env",
			mutateA: func(p *ModularParams) {
				p.EnvEnabled = 0
				p.AmpAttack, p.AmpDecay, p.AmpSustain, p.AmpRelease = 0.001, 0.01, 0.1, 0.01
			},
			mutateB: func(p *ModularParams) {
				p.EnvEnabled = 0
				p.AmpAttack, p.AmpDecay, p.AmpSustain, p.AmpRelease = 1, 1, 1, 1
			},
		},
		{
			name: "filter",
			mutateA: func(p *ModularParams) {
				p.FilterEnabled = 0
				p.FilterType, p.FilterCutoff, p.FilterResonance = 0, 300, 12
			},
			mutateB: func(p *ModularParams) {
				p.FilterEnabled = 0
				p.FilterType, p.FilterCutoff, p.FilterResonance = 1, 18000, 0.5
			},
		},
		{
			name: "drive",
			mutateA: func(p *ModularParams) {
				p.DriveEnabled = 0
				p.Drive = 0.2
			},
			mutateB: func(p *ModularParams) {
				p.DriveEnabled = 0
				p.Drive = 1
			},
		},
		{
			name: "fm-depths",
			mutateA: func(p *ModularParams) {
				p.OscType = 4
				p.FMEnabled = 0
				p.FMOp1Depth, p.FMOp2Depth, p.FMOp3Depth, p.FMOp4Depth = 0, 0, 0, 0
			},
			mutateB: func(p *ModularParams) {
				p.OscType = 4
				p.FMEnabled = 0
				p.FMOp1Depth, p.FMOp2Depth, p.FMOp3Depth, p.FMOp4Depth = 8, 8, 8, 8
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			a := identityModularParams()
			tc.mutateA(&a)
			b := identityModularParams()
			tc.mutateB(&b)
			ha := hashFloat32(renderMP(t, a))
			hb := hashFloat32(renderMP(t, b))
			if ha != hb {
				t.Errorf("disabled %s stage still grants its params authority:\n  A: %s\n  B: %s", tc.name, ha, hb)
			}
		})
	}

	// drive_enabled=0 with drive set must equal drive=0 with the stage enabled —
	// the two ways of "no drive" are the same code path.
	t.Run("drive-off-equals-zero-drive", func(t *testing.T) {
		off := identityModularParams()
		off.DriveEnabled = 0
		off.Drive = 0.9
		zero := identityModularParams()
		zero.Drive = 0
		if hashFloat32(renderMP(t, off)) != hashFloat32(renderMP(t, zero)) {
			t.Errorf("drive bypassed (drive=0.9) != drive=0 enabled — bypass is not a clean no-op")
		}
	})
}

// ── Combination matrix: every enable combo is finite + deterministic ──────

// TestModularStageIO_StageComboMatrix renders every 2³ combination of
// env/filter/drive enables (osc always on — disabled osc is the documented
// silence case) for a harmonically rich wave and a deterministic noise wave.
// Each combo must be finite, non-silent, and render-stable (two renders are
// bit-identical), so the pills can be toggled in any combination without
// nondeterminism or NaN poisoning.
func TestModularStageIO_StageComboMatrix(t *testing.T) {
	for _, osc := range []float64{1, 5} { // saw, white noise
		for combo := range 8 {
			env := combo&1 != 0
			filt := combo&2 != 0
			drive := combo&4 != 0
			p := identityModularParams()
			p.OscType = osc
			p.Drive = 0.6 // give the drive stage authority when enabled
			if !env {
				p.EnvEnabled = 0
			}
			if !filt {
				p.FilterEnabled = 0
			}
			if !drive {
				p.DriveEnabled = 0
			}
			name := map[bool]string{true: "on", false: "off"}
			t.Run(
				"osc"+map[float64]string{1: "Saw", 5: "Noise"}[osc]+
					"_env-"+name[env]+"_filt-"+name[filt]+"_drive-"+name[drive],
				func(t *testing.T) {
					a := renderMP(t, p)
					b := renderMP(t, p)
					if peakAbs(a) <= 0 {
						t.Fatalf("combo rendered silence")
					}
					if hashFloat32(a) != hashFloat32(b) {
						t.Errorf("combo render is nondeterministic")
					}
				},
			)
		}
	}
}
