package fingerprint

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

// With the synchronous amplitude modulation a bowed string produces.
func vibratoTone(sr int, f0, rateHz, extentCents, sec float64) wave.Wave {
	n := int(float64(sr) * sec)
	s := make([]float64, n)
	depth := math.Pow(2, extentCents/1200) - 1
	phase := 0.0
	for i := range s {
		t := float64(i) / float64(sr)
		lfo := math.Sin(2 * math.Pi * rateHz * t)
		f := f0 * (1 + depth*lfo)
		phase += 2 * math.Pi * f / float64(sr)
		am := 1 + 0.12*lfo
		s[i] = am * (0.5*math.Sin(phase) + 0.2*math.Sin(2*phase))
	}
	return wave.Wave{Samples: s, SampleRate: sr}
}

// A descriptor that can never produce a measurement is worse than none: nobody can tell signal from plumbing.
func TestVibratoDescriptorsFireOnRealVibrato(t *testing.T) {
	b := NewBundle(vibratoTone(44100, 440, 5.5, 40, 2.5), "vibrato")
	if !RequiresPitched(b) {
		t.Fatalf("the probe must be pitched; F0=%.2f confidence=%.3f",
			b.Fingerprint().F0Hz, b.Fingerprint().F0Confidence)
	}

	for _, id := range []string{IDVibratoRate, IDVibratoExtent, IDVibratoJitter, IDVibratoAMDepth} {
		d, ok := DefaultRegistry().ByID(id)
		if !ok {
			t.Fatalf("%s is not registered", id)
		}
		m := d.Extract(b)
		if !m.OK {
			t.Errorf("%s: unavailable on a signal with genuine 5.5 Hz / 40 cent vibrato (%s)", id, m.NA)
			continue
		}
		t.Logf("%-24s %.4g", id, m.Primary())
	}

	// The rate must land near the truth, or the descriptor is reporting noise.
	d, _ := DefaultRegistry().ByID(IDVibratoRate)
	if m := d.Extract(b); m.OK {
		if got := m.Primary(); got < 4.0 || got > 7.0 {
			t.Errorf("vibrato rate read %.2f Hz for a 5.5 Hz vibrato -- outside any plausible tolerance", got)
		}
	}
}

// Never a fabricated zero.
func TestVibratoDescriptorsStaySilentOnASteadyTone(t *testing.T) {
	b := NewBundle(bundleTone(44100, 440, 2), "steady")
	for _, id := range []string{IDVibratoRate, IDVibratoExtent} {
		d, _ := DefaultRegistry().ByID(id)
		if m := d.Extract(b); m.OK {
			t.Errorf("%s reported %.4g on a dead-steady tone -- a zero here reads as "+
				"'measured, and it is zero' rather than 'there is no vibrato to measure'",
				id, m.Primary())
		}
	}
}

func TestFormantAndMFCCDescriptorsAreReachable(t *testing.T) {
	b := NewBundle(scaleNoisyTone(44100, 300, 2), "noisy")
	d, ok := DefaultRegistry().ByID(IDMFCCProfile)
	if !ok {
		t.Fatal("cepstral.mfcc is not registered")
	}
	m := d.Extract(b)
	if !m.OK {
		t.Fatalf("cepstral.mfcc unavailable on an ordinary signal: %s", m.NA)
	}
	if m.Form != FormVector || len(m.Values) != 13 {
		t.Errorf("cepstral.mfcc must be a 13-component vector, got form=%s len=%d", m.Form, len(m.Values))
	}
	// The cepstral domain must actually be represented.
	var cepstral, modulation int
	for _, dd := range DefaultRegistry().All() {
		switch dd.Repr {
		case ReprCepstral:
			cepstral++
		case ReprModulation:
			modulation++
		}
	}
	if cepstral == 0 {
		t.Error("no descriptor reads ReprCepstral, but Repr declares the domain")
	}
	if modulation == 0 {
		t.Error("no descriptor reads ReprModulation, but Repr declares the domain")
	}
}
