package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func tremoloTone(sr int, dur, hz, rate, depth float64) wave.Wave {
	n := int(dur * float64(sr))
	s := make([]float64, n)
	for i := range s {
		t := float64(i) / float64(sr)
		s[i] = 0.4 * (1 + depth*math.Sin(2*math.Pi*rate*t)) * math.Sin(2*math.Pi*hz*t)
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// modBandsSum is the shared "Bands is an energy-share partition" assertion:
// gesture + vibrato + warble must sum to (approximately) 1, since every bin
// in 0.5-30 Hz is bucketed into exactly one of the three bands. A regression
// here (e.g. a bin below 0.5 Hz leaking into total/peak while being excluded
// from bucketing) shows up as a sum well under 1 rather than as a wrong band
// weighting, which is why it needs its own check.
func modBandsSum(t *testing.T, ms ModSpectrum) float64 {
	t.Helper()
	sum := ms.Bands[0] + ms.Bands[1] + ms.Bands[2]
	if math.Abs(sum-1) > 0.01 {
		t.Errorf("Bands must sum to 1 +/- 0.01 (every bin in 0.5-30Hz belongs to exactly one band), got %v (sum=%.4f)", ms.Bands, sum)
	}
	return sum
}

func TestModulationSpectrum_FiveHzTremoloPeaksAtFive(t *testing.T) {
	b := NewBundle(tremoloTone(44100, 3.0, 440, 5, 0.5), "trem")
	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	if !ms.OK || math.Abs(ms.RateHz-5) > 0.6 {
		t.Fatalf("want 5 Hz, got %.2f ok=%v", ms.RateHz, ms.OK)
	}
	if ms.Bands[1] < 0.6 {
		t.Fatalf("energy should sit in the vibrato band, got %v", ms.Bands)
	}
	if ms.Flatness > 0.5 {
		t.Fatalf("a pure tremolo is peaky, flatness %.2f", ms.Flatness)
	}
	modBandsSum(t, ms)
}

// TestModulationSpectrum_DecayOnlyStaysAboveHalfHz is the reviewer's
// regression case for the lower-bin-truncation bug: a tone whose amplitude
// ONLY decays exponentially (no periodic modulation at all) must never let a
// sub-0.5Hz artifact bin (residual curvature the OLS detrend cannot fully
// remove from an exponential, not a linear, decay) leak into RateHz or into
// total/peak while being excluded from Bands' bucketing. Before the fix,
// `lo := int(modLoHz/binHz)` admitted exactly that bin: Bands summed to far
// under 1 and RateHz read below the analysis floor of 0.5 Hz.
func TestModulationSpectrum_DecayOnlyStaysAboveHalfHz(t *testing.T) {
	const sr = 44100
	const dur = 3.0
	const hz = 440.0
	const dbPerSec = -12.0
	n := int(dur * sr)
	s := make([]float64, n)
	for i := range s {
		sec := float64(i) / float64(sr)
		amp := math.Pow(10, dbPerSec*sec/20)
		s[i] = amp * math.Sin(2*math.Pi*hz*sec)
	}
	b := NewBundle(wave.Wave{Samples: s, SampleRate: sr}, "decay-only")
	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	if !ms.OK {
		t.Fatalf("expected a measurable modulation spectrum, got !OK")
	}
	if ms.RateHz < modLoHz {
		t.Fatalf("RateHz must never read below the analysis floor of %.1f Hz, got %.4f", modLoHz, ms.RateHz)
	}
	modBandsSum(t, ms)
	// Depth is OBSERVED, not asserted against a hard bound: the detrend is a
	// LINEAR (OLS) fit, so a genuinely exponential decay's curvature is only
	// partly removed, and what remains concentrates in the lowest surviving
	// bin near the 0.5 Hz floor -- Depth measures "how much of the modulation
	// power sits at ONE rate", which a leftover curvature lump can also
	// satisfy despite carrying no periodic modulation at all. Recorded here
	// as ground truth for future tuning rather than turned into a pass/fail
	// threshold this test does not have grounds to assert.
	t.Logf("decay-only (no periodic modulation): RateHz=%.4f Depth=%.4f Bands=%v Flatness=%.4f",
		ms.RateHz, ms.Depth, ms.Bands, ms.Flatness)
}

func TestModulationSpectrum_FlatToneHasNoDepth(t *testing.T) {
	b := NewBundle(wave.Sine(440, 0.4, 3.0, 44100), "flat")
	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	if !ms.OK || ms.Depth > 0.02 {
		t.Fatalf("steady sine depth want ~0, got %.3f", ms.Depth)
	}
}

func TestModulationSpectrum_NoiseIsFlat(t *testing.T) {
	b := NewBundle(wave.Noise(3, 0.3, 3.0, 44100), "noise")
	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	if !ms.OK || ms.Flatness < 0.4 {
		t.Fatalf("noise envelope should be a flat modulation spectrum, got %.2f", ms.Flatness)
	}
}

func TestModulationSpectrum_TooShort(t *testing.T) {
	if ms := ModulationSpectrum(make([]float64, 10), 0.0116); ms.OK {
		t.Fatal("fewer than modMinFrames must be !OK")
	}
}

// TestModAMRate_DecayOnlyIsNotAMeasurableRate is I4's regression: the axis
// must report an honest N/A, not the analysis floor dressed up as a rate.
//
// The previous test above records what the SPECTRUM does on a decay-only
// tone -- its peak lands on the lowest usable bin, because the OLS detrend
// removes only a straight line and an exponential decay is not one. This one
// asserts what the DESCRIPTOR does with that: a peak at the floor bin is an
// artifact of the analysis window, so mod.am.rate_hz is unavailable, while
// the sibling rows that describe the spectrum's SHAPE stay measurable.
func TestModAMRate_DecayOnlyIsNotAMeasurableRate(t *testing.T) {
	const sr = 44100
	const dur = 3.0
	n := int(dur * sr)
	s := make([]float64, n)
	for i := range s {
		sec := float64(i) / float64(sr)
		s[i] = math.Pow(10, -12.0*sec/20) * math.Sin(2*math.Pi*440*sec)
	}
	b := NewBundle(wave.Wave{Samples: s, SampleRate: sr}, "decay-only")

	tr := b.Tracks()
	ms := ModulationSpectrum(tr.RMS, tr.HopSec)
	if !ms.OK {
		t.Fatal("modulation spectrum unexpectedly unavailable")
	}
	if !ms.rateAtFloorBin {
		t.Fatalf("premise broken: a decay-only tone's peak should sit on the floor bin (RateHz=%.3f)", ms.RateHz)
	}

	d, ok := DefaultRegistry().ByID(IDModAMRate)
	if !ok {
		t.Fatal("mod.am.rate_hz not registered")
	}
	m := d.Extract(b)
	if m.OK {
		t.Fatalf("mod.am.rate_hz reported %.3f Hz for a tone with no modulation at all", m.Primary())
	}
	if m.NA != naModRateFloor {
		t.Errorf("NA reason = %q, want %q", m.NA, naModRateFloor)
	}

	// The shape rows describe the whole spectrum and stay available.
	for _, id := range []string{IDModAMDepth, IDModAMBands, IDModAMFlatness} {
		sd, ok := DefaultRegistry().ByID(id)
		if !ok {
			t.Fatalf("%s not registered", id)
		}
		if got := sd.Extract(b); !got.OK {
			t.Errorf("%s became unavailable: %s", id, got.NA)
		}
	}
}

// A REAL modulation must still be measured: the floor-bin rule may not
// swallow a genuine tremolo whose rate happens to be low.
func TestModAMRate_GenuineTremoloStillMeasured(t *testing.T) {
	const sr = 44100
	const dur = 4.0
	const rate = 5.0
	n := int(dur * sr)
	s := make([]float64, n)
	for i := range s {
		sec := float64(i) / float64(sr)
		amp := 0.4 * (1 + 0.5*math.Sin(2*math.Pi*rate*sec))
		s[i] = amp * math.Sin(2*math.Pi*440*sec)
	}
	b := NewBundle(wave.Wave{Samples: s, SampleRate: sr}, "tremolo")

	d, _ := DefaultRegistry().ByID(IDModAMRate)
	m := d.Extract(b)
	if !m.OK {
		t.Fatalf("a 5 Hz tremolo must be measurable, got N/A: %s", m.NA)
	}
	if got := m.Primary(); math.Abs(got-rate) > 1.0 {
		t.Errorf("rate = %.2f Hz, want ~%.1f Hz", got, rate)
	}
}
