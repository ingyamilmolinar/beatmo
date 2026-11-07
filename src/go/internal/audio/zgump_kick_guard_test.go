//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// The anti-overfit net for the modal kick: structural invariants from its reference (bloom, plateau, glide, smooth
// HF), so a retune cannot collapse it into a plain exponential thump. Bounds carry margin for honest tuning.
func TestZgumpKickIsOrganicModalKick(t *testing.T) {
	buf, sr := kickGuardRender(t, roster.ZgumpKick)
	if len(buf) == 0 {
		t.Fatal("zgump-kick playback produced no samples")
	}
	x := make([]float64, len(buf))
	peak := 0.0
	for i, v := range buf {
		x[i] = float64(v)
		if a := math.Abs(x[i]); a > peak {
			peak = a
		}
	}
	if peak < 0.1 {
		t.Fatalf("zgump-kick too quiet: peak=%.4f", peak)
	}
	for i := range x {
		x[i] /= peak
	}
	fp := fingerprint.KickAnalyze(wave.Wave{Samples: x, SampleRate: sr})

	// 60-120 Hz carries the most energy: not a sub boom or a mid honk.
	bands := fp.BandAvg
	if bands[1] <= bands[0] || bands[1] <= bands[2] {
		t.Fatalf("60-120 Hz is not the dominant band: bands=%v (want band[1] largest — the kick's weight)", bands)
	}
	// The reference's own BandAvg is 0.447.
	if bands[1] < 0.40 {
		t.Fatalf("60-120 Hz band = %.2f, want >= 0.40 (fundamental must dominate; reference 0.447)", bands[1])
	}
	// 2. Almost no HF (no hiss/spit): energy above 1 kHz is tiny.
	if hf := bands[5] + bands[6]; hf > 0.05 {
		t.Fatalf(">1 kHz energy = %.3f, want <= 0.05 (the reference has ~0.02; a modal kick has no air)", hf)
	}
	// 3. Pitch settles in the reference's low fundamental range.
	if fp.PitchSettleHz < 55 || fp.PitchSettleHz > 85 {
		t.Fatalf("pitch settle = %.1f Hz, want [55,85] (reference 67.5)", fp.PitchSettleHz)
	}
	// A real downward glide, the first organic tell: not a static electronic kick.
	if fp.PitchStartHz < fp.PitchSettleHz*1.4 {
		t.Fatalf("pitch glide too shallow: start=%.0f settle=%.0f (want start >= 1.4x settle — the drumhead drop)",
			fp.PitchStartHz, fp.PitchSettleHz)
	}
	// The envelope peaks after t=0 (reference ~25 ms), not an instant click.
	if ms := fp.AttackPeakSec * 1000; ms < 12 || ms > 50 {
		t.Fatalf("attack peak = %.0f ms, want [12,50] (reference 25; a swell, not an instant click)", ms)
	}
	// 6. Crest = a thump, not a spiky click and not a flat drone (reference 3.4).
	if fp.Crest < 2.6 || fp.Crest > 4.3 {
		t.Fatalf("crest = %.2f, want [2.6,4.3] (reference 3.4)", fp.Crest)
	}
	// A plain exponential decay has neither the plateau nor the delayed bloom.
	if fp.PlateauFlatness < 0.35 {
		t.Fatalf("plateau flatness = %.2f, want >= 0.35 (a sustained plateau, not a pure exponential decay)", fp.PlateauFlatness)
	}
	if !fp.TailBloomPresent {
		t.Fatal("no tail bloom detected — the coupled-mode beating (the organic complexity) collapsed to a plain decay")
	}
	// A clean modal kick has small window-to-window HF jumps; a sputtering one does not.
	if fp.HFRatioMaxJump > 0.15 {
		t.Fatalf("HF-ratio max jump = %.3f, want <= 0.15 (no spit/sputter in the tail)", fp.HFRatioMaxJump)
	}
}

// Its 15 ms band windows read a different sub/low split at 48 kHz for the SAME sound, so the guards analyse at 44.1 kHz.
const kickFingerprintRate = 44100

// After proving the played render inaudibly different from it, so the analysis rate cannot hide a change in what is
// heard.
func kickGuardRender(t *testing.T, id instrument.ID) ([]float32, int) {
	t.Helper()
	played, engineSR := RenderInstrumentOneShotRaw(id)
	pinSampleRate(t, kickFingerprintRate)
	buf, sr := RenderInstrumentOneShotRaw(id)
	if engineSR != sr {
		a := ResampleToRate(played, engineSR, sr)
		n := min(len(a), len(buf))
		x, y := make([]float64, n), make([]float64, n)
		for i := range n {
			x[i], y[i] = float64(buf[i]), float64(a[i])
		}
		if ch := soundview.CompareNotes(x, y, sr); ch.Audible() {
			t.Fatalf("%s at %d Hz sounds different from %d Hz (%+v): the guard's analysis rate would hide it", id, engineSR, sr, ch)
		}
	}
	return buf, sr
}

// Peak-normalized.
func kickFP(t *testing.T, id instrument.ID) fingerprint.KickFingerprint {
	t.Helper()
	buf, sr := kickGuardRender(t, id)
	if len(buf) == 0 {
		t.Fatalf("%s produced no samples", id)
	}
	x := make([]float64, len(buf))
	pk := 0.0
	for i, v := range buf {
		x[i] = float64(v)
		if a := math.Abs(x[i]); a > pk {
			pk = a
		}
	}
	if pk < 0.1 {
		t.Fatalf("%s too quiet: peak=%.4f", id, pk)
	}
	for i := range x {
		x[i] /= pk
	}
	return fingerprint.KickAnalyze(wave.Wave{Samples: x, SampleRate: sr})
}

// A deep sub boom with a BIG pitch drop and a low settled fundamental; loose, ear-tunable bounds.
func TestKick808IsDeepSubKick(t *testing.T) {
	fp := kickFP(t, roster.Kick808)
	// Sub-heavy: the two lowest bands carry the bulk; <60 Hz is substantial.
	if fp.BandAvg[0]+fp.BandAvg[1] < 0.70 {
		t.Fatalf("kick-808 low-end (<120 Hz)=%.2f, want >= 0.70 (a sub boom)", fp.BandAvg[0]+fp.BandAvg[1])
	}
	if fp.BandAvg[0] < 0.30 {
		t.Fatalf("kick-808 <60 Hz=%.2f, want >= 0.30 (deep sub present)", fp.BandAvg[0])
	}
	// Low settled fundamental.
	if fp.PitchSettleHz > 65 {
		t.Fatalf("kick-808 settle=%.1f Hz, want <= 65 (deep sub)", fp.PitchSettleHz)
	}
	// A big downward pitch drop (the 808/house "pew").
	if fp.PitchStartHz < fp.PitchSettleHz*1.6 {
		t.Fatalf("kick-808 pitch drop too small: start=%.0f settle=%.0f (want start >= 1.6x settle)", fp.PitchStartHz, fp.PitchSettleHz)
	}
	// No high air.
	if fp.BandAvg[6] > 0.03 {
		t.Fatalf("kick-808 >2 kHz=%.3f, want <= 0.03 (pure sub, no air)", fp.BandAvg[6])
	}
}

// Tight and punchy: 60-120 Hz DOMINANT (not a sub boom) and a sharp beater transient.
func TestKickAcousticIsTightOrganicKick(t *testing.T) {
	fp := kickFP(t, roster.KickAcoustic)
	// Calibrated to the per-window BandAvg, not the whole-signal integral.
	if fp.BandAvg[1] <= fp.BandAvg[0] || fp.BandAvg[1] <= fp.BandAvg[2] || fp.BandAvg[1] < 0.42 {
		t.Fatalf("kick-acoustic 60-120 Hz=%.2f (must be the dominant band, >= 0.42; a tight kick, not a sub)", fp.BandAvg[1])
	}
	// The sub is present but must not dominate: with AttackLowMidRatio, what keeps it a kick rather than a tom.
	if fp.BandAvg[0] > 0.48 {
		t.Fatalf("kick-acoustic <60 Hz=%.2f, want <= 0.48", fp.BandAvg[0])
	}
	// Real-kick fundamental — LOW/HEAVY (well below the earlier tom-like tuning).
	if fp.PitchSettleHz < 45 || fp.PitchSettleHz > 95 {
		t.Fatalf("kick-acoustic settle=%.1f Hz, want [45,95] (deep kick)", fp.PitchSettleHz)
	}
	// The loud onset must be fundamental-dominant, or it reads as a high tom.
	if fp.AttackLowMidRatio < 1.0 {
		t.Fatalf("kick-acoustic attack low/mid=%.2f, want >= 1.0 (fundamental-dominant kick, not a mid-dominant tom)", fp.AttackLowMidRatio)
	}
	// Lower than a dry kick since the room tail adds sustain.
	if fp.Crest < 4.5 {
		t.Fatalf("kick-acoustic crest=%.2f, want >= 4.5 (a punchy hit, not a soft thump)", fp.Crest)
	}
	// Guards against a bone-dry synth kick.
	if fp.TailDurationSec < 0.15 {
		t.Fatalf("kick-acoustic reverb tail=%.0f ms, want >= 150 (a room tail, not bone-dry)", fp.TailDurationSec*1000)
	}
	// Bright inharmonic modes and recorded-air texture, not a dull pure sine.
	cmean := 0.0
	for _, c := range fp.CentroidTrace {
		cmean += c
	}
	if n := len(fp.CentroidTrace); n > 0 {
		cmean /= float64(n)
	}
	if cmean < 1200 {
		t.Fatalf("kick-acoustic centroid mean=%.0f Hz, want >= 1200 (bright modal timbre, not a dull sine)", cmean)
	}
	if fp.FlatnessAvg < 0.10 {
		t.Fatalf("kick-acoustic flatness=%.3f, want >= 0.10 (recorded-air texture, not pure-tonal)", fp.FlatnessAvg)
	}
}

// A rawer, heavier sibling of zgump-kick: grit and a harder transient instead of the bloom, so the bloom is not
// required; still fundamental-dominant, gliding, punchy, no >2 kHz air, and grittier than zgump.
func TestRawKickIsGrittyHeavyKick(t *testing.T) {
	analyze := func(id instrument.ID) fingerprint.KickFingerprint {
		buf, sr := kickGuardRender(t, id)
		if len(buf) == 0 {
			t.Fatalf("%s produced no samples", id)
		}
		x := make([]float64, len(buf))
		pk := 0.0
		for i, v := range buf {
			x[i] = float64(v)
			if a := math.Abs(x[i]); a > pk {
				pk = a
			}
		}
		if pk < 0.1 {
			t.Fatalf("%s too quiet: peak=%.4f", id, pk)
		}
		for i := range x {
			x[i] /= pk
		}
		return fingerprint.KickAnalyze(wave.Wave{Samples: x, SampleRate: sr})
	}
	raw := analyze(roster.RawKick)
	zg := analyze(roster.ZgumpKick)

	// 1. Still a kick: the fundamental band dominates.
	if raw.BandAvg[1] <= raw.BandAvg[0] || raw.BandAvg[1] <= raw.BandAvg[2] {
		t.Fatalf("raw-kick 60-120 Hz not dominant: bands=%v (must stay kick-shaped)", raw.BandAvg)
	}
	if raw.BandAvg[1] < 0.30 {
		t.Fatalf("raw-kick 60-120 Hz = %.2f, want >= 0.30 (still fundamental-led even when raw)", raw.BandAvg[1])
	}
	// 2. Heavier: settles low.
	if raw.PitchSettleHz < 50 || raw.PitchSettleHz > 80 {
		t.Fatalf("raw-kick settle = %.1f Hz, want [50,80] (heavy sub)", raw.PitchSettleHz)
	}
	// 3. A real downward glide (organic tell survives the rawness).
	if raw.PitchStartHz < raw.PitchSettleHz*1.3 {
		t.Fatalf("raw-kick glide too shallow: start=%.0f settle=%.0f", raw.PitchStartHz, raw.PitchSettleHz)
	}
	// Under a fixed peak a louder kick has a lower crest; a drone sits near √2, so 2.2 still separates a hit from a tone.
	if raw.Crest < 2.2 || raw.Crest > 4.8 {
		t.Fatalf("raw-kick crest = %.2f, want [2.2,4.8]", raw.Crest)
	}
	// 5. No high air (a heavy kick, not a bright/tss one).
	if raw.BandAvg[6] > 0.05 {
		t.Fatalf("raw-kick >2 kHz energy = %.3f, want <= 0.05 (heavy, not airy)", raw.BandAvg[6])
	}
	// 6. Measurably grittier than the clean zgump-kick (the whole point).
	rawGrit := raw.BandAvg[4] + raw.BandAvg[5] + raw.BandAvg[6]
	zgGrit := zg.BandAvg[4] + zg.BandAvg[5] + zg.BandAvg[6]
	if rawGrit < zgGrit {
		t.Fatalf("raw-kick grit (>500 Hz energy)=%.3f is not >= zgump's %.3f — raw must be grittier", rawGrit, zgGrit)
	}
}
