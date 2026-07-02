package fingerprint

import (
	"encoding/binary"
	"math"
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wave"
)

func TestDetectF0_PureTone(t *testing.T) {
	got := DetectF0(wave.Sine(220, 0.9, 0.5, 44100))
	if math.Abs(got-220) > 220*0.02 {
		t.Errorf("F0=%.1f want ~220", got)
	}
}

func TestDetectF0_MissingFundamental(t *testing.T) {
	// 440+660+880 (harmonics 2,3,4 of 220) — naive lowest-peak picks 440.
	sr := 44100
	mk := func(f float64) []float64 { return wave.Sine(f, 0.3, 0.5, sr).Samples }
	a, b, c := mk(440), mk(660), mk(880)
	sum := make([]float64, len(a))
	for i := range sum {
		sum[i] = a[i] + b[i] + c[i]
	}
	got := DetectF0(wave.Wave{Samples: sum, SampleRate: sr})
	if math.Abs(got-220) > 220*0.05 {
		t.Errorf("F0=%.1f want ~220 (missing fundamental)", got)
	}
}

func TestDetectF0_DominantSecondHarmonic(t *testing.T) {
	// Weak fundamental (220), dominant 2nd harmonic (440), plus 3rd (660).
	// True pitch is 220 Hz; a spectral-only octave guard is tempted by 440.
	sr := 44100
	n := int(0.6 * float64(sr))
	s := make([]float64, n)
	for i := range s {
		tsec := float64(i) / float64(sr)
		s[i] = 0.25*math.Sin(2*math.Pi*220*tsec) +
			0.9*math.Sin(2*math.Pi*440*tsec) +
			0.3*math.Sin(2*math.Pi*660*tsec)
	}
	got := DetectF0(wave.Wave{Samples: s, SampleRate: sr})
	if math.Abs(got-220) > 220*0.05 {
		t.Errorf("F0=%.1f want ~220 (dominant 2nd harmonic must not cause octave-up)", got)
	}
}

func TestDetectF0_Vibrato(t *testing.T) {
	sr := 44100
	// mk generates a vibrato waveform using proper phase accumulation (matching
	// the synth engine's wavetable-oscillator model: per-sample frequency update
	// + phase advance, not the closed-form sin(f*t) which creates a pathological
	// non-periodic signal). With phase accumulation each sample advances the
	// phase by the current instantaneous frequency, producing a true vibrato whose
	// pitch detection via short-frame median yields the center frequency f0.
	mk := func(f0, vibHz, depth, dur float64) wave.Wave {
		n := int(dur * float64(sr))
		s := make([]float64, n)
		var phase1, phase2, phase3 float64
		for i := range s {
			ts := float64(i) / float64(sr)
			f := f0 * (1 + depth*math.Sin(2*math.Pi*vibHz*ts))
			inc := 2 * math.Pi * f / float64(sr)
			phase1 += inc
			phase2 += 2 * inc
			phase3 += 3 * inc
			s[i] = 0.6*math.Sin(phase1) + 0.3*math.Sin(phase2) + 0.15*math.Sin(phase3)
		}
		return wave.Wave{Samples: s, SampleRate: sr}
	}
	cases := []struct{ vibHz, depth float64 }{{5.2, 0.07}, {5.2, 0.02}, {6.0, 0.05}}
	for _, c := range cases {
		got := DetectF0(mk(220, c.vibHz, c.depth, 1.2))
		if math.Abs(got-220) > 220*0.03 {
			t.Errorf("vib %.1fHz depth %.2f: F0=%.1f want ~220 (±3%%)", c.vibHz, c.depth, got)
		}
	}
}

// loadRefWaveForTest reads the 24-bit mono violin reference into a wave.Wave.
// fingerprint must not import the audio package, so this is a local minimal reader.
func loadRefWaveForTest(t *testing.T) (wave.Wave, bool) {
	t.Helper()
	data, err := os.ReadFile("../../../../../356181__mtg__violin-d5.wav")
	if err != nil {
		return wave.Wave{}, false
	}
	// Walk RIFF chunks to find fmt and data.
	if len(data) < 44 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return wave.Wave{}, false
	}
	var sr, ch, bits int
	var pcm []float64
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		sz := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := data[pos+8:]
		if id == "fmt " {
			ch = int(binary.LittleEndian.Uint16(body[2:4]))
			sr = int(binary.LittleEndian.Uint32(body[4:8]))
			bits = int(binary.LittleEndian.Uint16(body[14:16]))
		} else if id == "data" {
			bytesPerSample := bits / 8
			frame := ch * bytesPerSample
			n := sz / frame
			pcm = make([]float64, n)
			for i := 0; i < n; i++ {
				off := i * frame
				b0, b1, b2 := body[off], body[off+1], body[off+2]
				raw := int32(b0) | int32(b1)<<8 | int32(b2)<<16
				if raw&0x800000 != 0 {
					raw -= 0x1000000
				}
				pcm[i] = float64(raw) / 8388608.0
			}
			break
		}
		pos += 8 + sz
		if sz%2 == 1 {
			pos++ // chunks are word-aligned
		}
	}
	if sr == 0 || len(pcm) == 0 {
		return wave.Wave{}, false
	}
	return wave.Wave{Samples: pcm, SampleRate: sr}, true
}

func TestDetectF0_NoSubharmonicOctaveError(t *testing.T) {
	w, ok := loadRefWaveForTest(t)
	if !ok {
		t.Skip("reference WAV unavailable")
	}
	seg := AutoSegment(w, 1.5)
	f0 := DetectF0(seg)
	if f0 < 575 || f0 > 605 {
		t.Fatalf("DetectF0 = %.1f Hz, want ~591 (D5); sub-harmonic bug if ~197", f0)
	}
}

// TestDetectF0_ThirdSubharmonicGuard tests that the octave guard considers f/3
// and 3f candidates, not just {f/2, f, 2f}.
//
// The signal contains a 591 Hz tone (D5) with its harmonics 1182 Hz and 1773 Hz,
// plus a weaker 197 Hz component (≈ f0/3). This causes every YIN frame to lock
// onto the 197 Hz period because the combined signal has period 1/197 s.
// Per-frame median YIN also returns 197 Hz.
//
// Both the 197 Hz and 591 Hz candidates cover 3 harmonics (count tie). The old
// octave guard {f/2, f, 2f} tested candidates {98, 197, 394} — the 591 Hz
// candidate was NOT in the list. The new guard {f/3, f/2, f, 2f, 3f} adds
// 3f = 591 Hz as a candidate. On the count tie, the k=1 tiebreak (prefer the
// candidate with more energy at its own fundamental bin) selects 591 Hz because
// mag(591) >> mag(197).
func TestDetectF0_ThirdSubharmonicGuard(t *testing.T) {
	sr := 44100
	trueF0 := 591.0     // D5
	subHz := trueF0 / 3 // ~197 Hz
	n := int(0.5 * float64(sr))
	buf := make([]float64, n)
	for i := range buf {
		tsec := float64(i) / float64(sr)
		// Main signal: 591 Hz with harmonics 1182 and 1773.
		buf[i] = 0.8*math.Sin(2*math.Pi*trueF0*tsec) +
			0.5*math.Sin(2*math.Pi*2*trueF0*tsec) +
			0.25*math.Sin(2*math.Pi*3*trueF0*tsec) +
			// Weaker 197 Hz component causes YIN to lock onto the 197 Hz period.
			0.35*math.Sin(2*math.Pi*subHz*tsec)
	}
	w := wave.Wave{Samples: buf, SampleRate: sr}
	got := DetectF0(w)
	if got < 575 || got > 620 {
		t.Fatalf("DetectF0 = %.1f Hz, want ~591 (D5); old guard {f/2,f,2f} would miss the 3f=591 candidate and stay at ~197", got)
	}
}
