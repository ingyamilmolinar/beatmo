package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// mkADSRWave synthesizes a mono tone at freq Hz whose amplitude envelope is a
// classic ADSR: linear attack 0->1 over attackSec, linear decay 1->sustainLevel
// over decaySec, a flat sustain at sustainLevel for sustainSec (optionally
// tremolo-modulated by amDepth at amHz), then a linear release sustainLevel->0
// over releaseSec.
func mkADSRWave(sr int, freq, attackSec, decaySec, sustainLevel, sustainSec, releaseSec, amDepth, amHz float64) wave.Wave {
	total := attackSec + decaySec + sustainSec + releaseSec
	n := int(total * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		var env float64
		switch {
		case t < attackSec:
			env = t / attackSec
		case t < attackSec+decaySec:
			x := (t - attackSec) / decaySec
			env = 1 + (sustainLevel-1)*x
		case t < attackSec+decaySec+sustainSec:
			env = sustainLevel
			if amDepth > 0 {
				env *= 1 + amDepth*math.Sin(2*math.Pi*amHz*(t-attackSec-decaySec))
			}
		default:
			x := (t - attackSec - decaySec - sustainSec) / releaseSec
			env = sustainLevel * (1 - x)
		}
		if env < 0 {
			env = 0
		}
		s[i] = env * math.Sin(2*math.Pi*freq*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// mkExpDecayWave synthesizes a tone whose envelope is a pure exponential decay
// from peak (no attack, no plateau): env(t) = exp(-t/tau). durSec is the total
// clip length.
func mkExpDecayWave(sr int, freq, tau, durSec float64) wave.Wave {
	n := int(durSec * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		env := math.Exp(-t / tau)
		s[i] = env * math.Sin(2*math.Pi*freq*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// mkTwoStageDecayWave synthesizes a tone with a FAST initial decay (peak ->
// midLevel over fastSec, linear in dB) followed by a much SLOWER exponential
// decay (tau=slowTau) for the remainder of durSec. Models a struck bar/
// bell's strike-transient-then-hum shape, where AnalyzeEnvelopeSegments's
// plateau detector (envSustainFlatDBPerSec) can trigger as soon as the slow
// phase begins even though the "plateau" it found is itself still decaying.
func mkTwoStageDecayWave(sr int, freq, fastSec, midLevel, slowTau, durSec float64) wave.Wave {
	n := int(durSec * float64(sr))
	s := make([]float64, n)
	logMid := math.Log(midLevel)
	for i := range s {
		t := float64(i) / float64(sr)
		var env float64
		if t < fastSec {
			x := t / fastSec
			env = math.Exp(logMid * x) // linear-in-dB from 1.0 -> midLevel
		} else {
			env = midLevel * math.Exp(-(t-fastSec)/slowTau)
		}
		s[i] = env * math.Sin(2*math.Pi*freq*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

func TestAnalyzeEnvelopeSegments_Silence(t *testing.T) {
	w := wave.Wave{Samples: make([]float64, 44100), SampleRate: 44100}
	seg := AnalyzeEnvelopeSegments(w)
	if !seg.Silent {
		t.Fatalf("expected Silent=true for an all-zero wave, got %+v", seg)
	}
}

func TestAnalyzeEnvelopeSegments_EmptyWave(t *testing.T) {
	w := wave.Wave{Samples: nil, SampleRate: 44100}
	seg := AnalyzeEnvelopeSegments(w)
	if !seg.Silent {
		t.Fatalf("expected Silent=true for an empty wave, got %+v", seg)
	}
}

// A classic ADSR envelope must be recovered with roughly the parameters used
// to build it: attack/decay/sustain/release durations and sustain level, and
// NoSustainPlateau must be false since a real flat plateau exists.
func TestAnalyzeEnvelopeSegments_ADSRShape(t *testing.T) {
	sr := 44100
	const attackSec, decaySec, sustainSec, releaseSec = 0.05, 0.10, 0.50, 0.20
	const sustainLevel = 0.3
	w := mkADSRWave(sr, 440, attackSec, decaySec, sustainLevel, sustainSec, releaseSec, 0, 0)

	seg := AnalyzeEnvelopeSegments(w)
	if seg.Silent {
		t.Fatalf("expected a non-silent ADSR wave, got Silent=true")
	}
	if seg.NoSustainPlateau {
		t.Errorf("expected a detected sustain plateau, got NoSustainPlateau=true (seg=%+v)", seg)
	}

	tol := 0.03 // 30ms tolerance given 10ms envelope hop + slope-window smoothing
	if math.Abs(seg.AttackSec-attackSec) > tol {
		t.Errorf("AttackSec = %.4f, want ~%.4f (+/-%.3f)", seg.AttackSec, attackSec, tol)
	}
	if math.Abs(seg.DecaySec-decaySec) > tol {
		t.Errorf("DecaySec = %.4f, want ~%.4f (+/-%.3f)", seg.DecaySec, decaySec, tol)
	}
	if seg.DecayRateDBPerSec >= 0 {
		t.Errorf("DecayRateDBPerSec = %.3f, want negative (decaying)", seg.DecayRateDBPerSec)
	}
	if math.Abs(seg.SustainLevel-sustainLevel) > 0.1 {
		t.Errorf("SustainLevel = %.3f, want ~%.3f", seg.SustainLevel, sustainLevel)
	}
	// Sustain region is bounded above by the true sustain duration; some of it
	// is consumed detecting the plateau + the release-detection lag.
	if seg.SustainSec <= 0 || seg.SustainSec > sustainSec+tol {
		t.Errorf("SustainSec = %.4f, want in (0, %.4f]", seg.SustainSec, sustainSec+tol)
	}
	if seg.ReleaseSec <= 0 {
		t.Errorf("ReleaseSec = %.4f, want > 0", seg.ReleaseSec)
	}
	if seg.SustainVariation > 0.05 {
		t.Errorf("SustainVariation = %.4f, want ~0 for a flat sustain", seg.SustainVariation)
	}
}

// A percussive hit that decays straight to the noise floor with no flat
// plateau must be flagged NoSustainPlateau=true and report SustainSec==0.
func TestAnalyzeEnvelopeSegments_PercussiveNoSustain(t *testing.T) {
	sr := 44100
	w := mkExpDecayWave(sr, 200, 0.02, 0.3) // tau=20ms decays well below floor by 300ms
	seg := AnalyzeEnvelopeSegments(w)
	if seg.Silent {
		t.Fatalf("expected non-silent, got Silent=true")
	}
	if !seg.NoSustainPlateau {
		t.Errorf("expected NoSustainPlateau=true for a pure percussive decay, got false (seg=%+v)", seg)
	}
	if seg.SustainSec != 0 {
		t.Errorf("SustainSec = %.4f, want 0 (no plateau)", seg.SustainSec)
	}
	if seg.RingT60Extrapolated {
		t.Errorf("expected RingT60 to be measured (not extrapolated) since tau=20ms decays well within a 300ms clip")
	}
	// T60 for exp(-t/tau) is tau*ln(1000) ~= 0.02 * 6.9078 = 0.1382s.
	wantT60 := 0.02 * math.Log(1000)
	if math.Abs(seg.RingT60Sec-wantT60) > 0.03 {
		t.Errorf("RingT60Sec = %.4f, want ~%.4f", seg.RingT60Sec, wantT60)
	}
}

// A slowly-decaying ring that does not fall 60dB before the clip ends must be
// flagged as extrapolated rather than silently reporting a misleadingly short
// number.
func TestAnalyzeEnvelopeSegments_RingT60Extrapolated(t *testing.T) {
	sr := 44100
	w := mkExpDecayWave(sr, 200, 5.0, 0.3) // tau=5s: negligible decay within 300ms
	seg := AnalyzeEnvelopeSegments(w)
	if seg.Silent {
		t.Fatalf("expected non-silent, got Silent=true")
	}
	if !seg.RingT60Extrapolated {
		t.Errorf("expected RingT60Extrapolated=true for a ring that outlasts the clip")
	}
	// RingT60Sec should be (approximately) the remaining clip length after the peak.
	if seg.RingT60Sec < 0.25 {
		t.Errorf("RingT60Sec = %.4f, want close to the ~0.3s clip length (extrapolated lower bound)", seg.RingT60Sec)
	}
}

// OverallDecayRateDBPerSec must recover the true dB/s slope of a pure
// exponential decay even when RingT60 itself is extrapolated (clip ends
// before -60dB) -- this is the field diagnose.go's ring-T60 escalation
// (both-sides-extrapolated case) relies on for a real, unclamped signal.
func TestAnalyzeEnvelopeSegments_OverallDecayRateMatchesExpDecay(t *testing.T) {
	sr := 44100
	const tau = 5.0
	w := mkExpDecayWave(sr, 200, tau, 0.3) // negligible decay in 300ms -> extrapolated
	seg := AnalyzeEnvelopeSegments(w)
	if !seg.RingT60Extrapolated {
		t.Fatalf("expected RingT60Extrapolated=true for this setup (sanity check)")
	}
	wantSlope := -20 / (tau * math.Log(10)) // dB/s for env(t)=exp(-t/tau)
	if math.Abs(seg.OverallDecayRateDBPerSec-wantSlope) > math.Abs(wantSlope)*0.25+0.2 {
		t.Errorf("OverallDecayRateDBPerSec = %.3f dB/s, want close to %.3f dB/s (the true exponential-decay slope)",
			seg.OverallDecayRateDBPerSec, wantSlope)
	}
}

// A fast strike-transient decay followed by a much slower "hum" tail can
// trip the plateau detector right when the slow phase begins (its LOCAL
// slope, over a short window, already reads as "flat" relative to the fast
// phase's threshold) -- so DecayRateDBPerSec (peak -> plateau onset) only
// describes the brief fast phase. OverallDecayRateDBPerSec must instead
// reflect the much gentler slow-tail trend that dominates the clip -- this
// is exactly the case a bell/tube's ring exhibits (see the mapping doc's
// tubular-bells findings).
func TestAnalyzeEnvelopeSegments_OverallDecayRateCapturesSlowTail(t *testing.T) {
	sr := 44100
	const slowTau = 2.0
	w := mkTwoStageDecayWave(sr, 300, 0.05, 0.1, slowTau, 2.0)
	seg := AnalyzeEnvelopeSegments(w)

	if seg.DecaySec > 0.15 {
		t.Fatalf("expected the plateau detector to trigger shortly after the fast phase, DecaySec=%.3f too large", seg.DecaySec)
	}
	if seg.DecayRateDBPerSec > -50 {
		t.Errorf("DecayRateDBPerSec = %.2f dB/s, want a steep (very negative) fast-phase-only rate", seg.DecayRateDBPerSec)
	}

	wantSlow := -20 / (slowTau * math.Log(10)) // ~ -4.34 dB/s
	if math.Abs(seg.OverallDecayRateDBPerSec-wantSlow) > 2.0 {
		t.Errorf("OverallDecayRateDBPerSec = %.2f dB/s, want close to the slow-tail rate %.2f dB/s",
			seg.OverallDecayRateDBPerSec, wantSlow)
	}
	if seg.OverallDecayRateDBPerSec <= seg.DecayRateDBPerSec {
		t.Errorf("OverallDecayRateDBPerSec (%.2f) should be far less steep than DecayRateDBPerSec (%.2f) -- it must capture the slow tail, not just the fast onset",
			seg.OverallDecayRateDBPerSec, seg.DecayRateDBPerSec)
	}
}

// A sustain with tremolo (amplitude modulation) must report a materially
// higher SustainVariation than a flat sustain -- this is the "wavy... prolongs
// and waves in time" tell (tubular-bells) and the "dead-flat sustain" organ
// tell in the opposite direction.
func TestAnalyzeEnvelopeSegments_SustainVariationDetectsModulation(t *testing.T) {
	sr := 44100
	flat := mkADSRWave(sr, 440, 0.02, 0.05, 0.5, 0.6, 0.1, 0, 0)
	wobble := mkADSRWave(sr, 440, 0.02, 0.05, 0.5, 0.6, 0.1, 0.3, 4.0)

	flatSeg := AnalyzeEnvelopeSegments(flat)
	wobbleSeg := AnalyzeEnvelopeSegments(wobble)

	if flatSeg.NoSustainPlateau || wobbleSeg.NoSustainPlateau {
		t.Fatalf("expected both to have a detected sustain plateau: flat=%v wobble=%v",
			flatSeg.NoSustainPlateau, wobbleSeg.NoSustainPlateau)
	}
	if !(wobbleSeg.SustainVariation > flatSeg.SustainVariation*3) {
		t.Errorf("SustainVariation flat=%.4f wobble=%.4f; want wobble much larger than flat",
			flatSeg.SustainVariation, wobbleSeg.SustainVariation)
	}
}

func TestAnalyzeEnvelopeSegments_TotalAudibleSec(t *testing.T) {
	sr := 44100
	w := mkADSRWave(sr, 440, 0.05, 0.10, 0.3, 0.50, 0.20, 0, 0)
	seg := AnalyzeEnvelopeSegments(w)
	total := 0.05 + 0.10 + 0.50 + 0.20
	if seg.TotalAudibleSec <= 0 || seg.TotalAudibleSec > total+0.02 {
		t.Errorf("TotalAudibleSec = %.4f, want in (0, %.4f]", seg.TotalAudibleSec, total+0.02)
	}
}
