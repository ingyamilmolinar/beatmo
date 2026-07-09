//go:build !test && !js

package audio

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// zgump_kick_guard_test.go is the REGRESSION + ANTI-OVERFIT net for the modal
// (variant 6) zgump-kick. It renders the SHIPPED instrument through the real
// playback dispatch and asserts the organic-character invariants measured from
// the reference 83768__zgump__kick-pack-0708.wav via the fingerprint kick
// metrics. The invariants are deliberately structural (bloom present, plateau
// sustained, glide present, HF smooth) so a future re-tune cannot silently
// collapse the complex organic envelope into a plain exponential thump — that
// would be "watering down" the reference. Bounds carry margin so honest tuning
// tweaks pass; only a character change trips them.
func TestZgumpKickIsOrganicModalKick(t *testing.T) {
	buf, sr := RenderInstrumentOneShotRaw("zgump-kick")
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

	// 1. Fundamental-dominant mid-bass thump: the 60-120 Hz band carries the most
	//    energy (the reference is 71% there); NOT a sub-only boom or a mid honk.
	bands := fp.BandAvg
	if bands[1] <= bands[0] || bands[1] <= bands[2] {
		t.Fatalf("60-120 Hz is not the dominant band: bands=%v (want band[1] largest — the kick's weight)", bands)
	}
	// Calibrated to the reference's own per-window BandAvg (0.447); 0.40 keeps
	// margin while still requiring the fundamental to carry most of the energy.
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
	// 4. A real downward pitch GLIDE (the #1 organic tell) — start clearly above
	//    settle. Guards against a static-pitch (electronic) kick.
	if fp.PitchStartHz < fp.PitchSettleHz*1.4 {
		t.Fatalf("pitch glide too shallow: start=%.0f settle=%.0f (want start >= 1.4x settle — the drumhead drop)",
			fp.PitchStartHz, fp.PitchSettleHz)
	}
	// 5. A SWELL-IN onset: the envelope peaks a bit after t=0 (reference ~25 ms),
	//    not an instant click spike.
	if ms := fp.AttackPeakSec * 1000; ms < 12 || ms > 50 {
		t.Fatalf("attack peak = %.0f ms, want [12,50] (reference 25; a swell, not an instant click)", ms)
	}
	// 6. Crest = a thump, not a spiky click and not a flat drone (reference 3.4).
	if fp.Crest < 2.6 || fp.Crest > 4.3 {
		t.Fatalf("crest = %.2f, want [2.6,4.3] (reference 3.4)", fp.Crest)
	}
	// 7. THE ORGANIC-COMPLEXITY GUARDS (anti-overfit): the coupled-mode envelope
	//    must keep its sustained plateau AND its delayed tail bloom. A plain
	//    exponential decay (the "watered down" failure) has neither.
	if fp.PlateauFlatness < 0.35 {
		t.Fatalf("plateau flatness = %.2f, want >= 0.35 (a sustained plateau, not a pure exponential decay)", fp.PlateauFlatness)
	}
	if !fp.TailBloomPresent {
		t.Fatal("no tail bloom detected — the coupled-mode beating (the organic complexity) collapsed to a plain decay")
	}
	// 8. SMOOTHNESS (anti-spit): the mid-body HF ratio must not oscillate — the
	//    variant-5 "sputter" failure. A clean modal kick has small window-to-window
	//    HF jumps.
	if fp.HFRatioMaxJump > 0.15 {
		t.Fatalf("HF-ratio max jump = %.3f, want <= 0.15 (no spit/sputter in the tail)", fp.HFRatioMaxJump)
	}
}

// kickFP renders a shipped instrument and returns its peak-normalized kick
// fingerprint (shared by the 808 + acoustic guards).
func kickFP(t *testing.T, id string) fingerprint.KickFingerprint {
	t.Helper()
	buf, sr := RenderInstrumentOneShotRaw(id)
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

// TestKick808IsDeepSubKick guards kick-808's identity vs the progressive-house
// reference: a deep, sub-heavy boom with a BIG pitch drop and a low settled
// fundamental. Loose bounds (ear-tunable) but enough to catch a regression back
// to the old barely-dropping ~89%-sub version.
func TestKick808IsDeepSubKick(t *testing.T) {
	fp := kickFP(t, "kick-808")
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

// TestKickAcousticIsTightOrganicKick guards kick-acoustic's identity vs the
// sandyrb reference: a tight, punchy, organic kick — fundamental ~80-90 Hz with
// 60-120 Hz DOMINANT (NOT a sub boom) and a SHARP beater transient (high crest).
// This is the anti-"boomy/un-acoustic" net (the old seed sat ~50% sub, crest ~5.7).
func TestKickAcousticIsTightOrganicKick(t *testing.T) {
	fp := kickFP(t, "kick-acoustic")
	// 60-120 Hz dominates (not a sub kick). Threshold calibrated to the
	// fingerprint's per-window BandAvg (≈0.5 here), not the whole-signal integral.
	if fp.BandAvg[1] <= fp.BandAvg[0] || fp.BandAvg[1] <= fp.BandAvg[2] || fp.BandAvg[1] < 0.42 {
		t.Fatalf("kick-acoustic 60-120 Hz=%.2f (must be the dominant band, >= 0.42; a tight kick, not a sub)", fp.BandAvg[1])
	}
	// The deep sub is present (this is a low/heavy kick) but must not DOMINATE
	// over 60-120 (already required above) — that plus the AttackLowMidRatio check
	// below is what keeps it a kick rather than a boomy/tom-ish sound.
	if fp.BandAvg[0] > 0.48 {
		t.Fatalf("kick-acoustic <60 Hz=%.2f, want <= 0.48", fp.BandAvg[0])
	}
	// Real-kick fundamental — LOW/HEAVY (well below the earlier tom-like tuning).
	if fp.PitchSettleHz < 45 || fp.PitchSettleHz > 95 {
		t.Fatalf("kick-acoustic settle=%.1f Hz, want [45,95] (deep kick)", fp.PitchSettleHz)
	}
	// KICK, not a tom: the LOUD onset must be fundamental-dominant (energy in
	// 40-100 Hz >= 100-500 Hz). The earlier tuning read as a high tom (mid-mode
	// dominant); this is the guard against regressing to that.
	if fp.AttackLowMidRatio < 1.0 {
		t.Fatalf("kick-acoustic attack low/mid=%.2f, want >= 1.0 (fundamental-dominant kick, not a mid-dominant tom)", fp.AttackLowMidRatio)
	}
	// A punchy transient (the aggressive "soul"). Lower than the dry variant since
	// the room reverb tail adds a little sustain, but still a clear hit.
	if fp.Crest < 4.5 {
		t.Fatalf("kick-acoustic crest=%.2f, want >= 4.5 (a punchy hit, not a soft thump)", fp.Crest)
	}
	// A room/reverb TAIL (the recorded kick's space): the tail must ring past the
	// main hit. Guards against a bone-dry synth kick.
	if fp.TailDurationSec < 0.15 {
		t.Fatalf("kick-acoustic reverb tail=%.0f ms, want >= 150 (a room tail, not bone-dry)", fp.TailDurationSec*1000)
	}
	// TIMBRE (the variant-7 modal win): a real-recorded spectral SHAPE — bright
	// inharmonic modal content + recorded-air texture, NOT the dull pure sine of
	// the old variant-4 tuning. Guards against a regression to a lifeless tone.
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

// TestRawKickIsGrittyHeavyKick guards the raw-kick's IDENTITY: a still-kick-
// shaped but rawer/heavier/harder sibling of zgump-kick (the user brief: "not as
// brutal, raw, heavy as the reference"). The guard is deliberately looser than
// zgump's — raw trades the smooth tail bloom for grit + a harder transient, so
// it does NOT require the bloom. It DOES require: fundamental-dominant (still a
// kick, not a mid honk), a real pitch glide, a punchy crest, no >2 kHz air, and
// measurably MORE grit than the clean zgump-kick. Bounds carry margin so ear-
// tuning the raw character doesn't trip it.
func TestRawKickIsGrittyHeavyKick(t *testing.T) {
	analyze := func(id string) fingerprint.KickFingerprint {
		buf, sr := RenderInstrumentOneShotRaw(id)
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
	raw := analyze("raw-kick")
	zg := analyze("zgump-kick")

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
	// 4. Punchy but bounded (a thump/hit, not a pure click or a drone).
	if raw.Crest < 2.8 || raw.Crest > 4.8 {
		t.Fatalf("raw-kick crest = %.2f, want [2.8,4.8]", raw.Crest)
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
