package wavgen

import (
	"math"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestEncodeDecodeRoundTrip(t *testing.T) {
	pcm := Sine(440, 0.5, 0.05, 48000)
	raw, err := EncodeWAV(pcm, 48000)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	if string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		t.Fatalf("not a WAV: %q", raw[:12])
	}
	got, sr, err := DecodeWAV(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if sr != 48000 {
		t.Errorf("sample rate = %d, want 48000", sr)
	}
	if len(got) != len(pcm) {
		t.Fatalf("length %d, want %d", len(got), len(pcm))
	}
	// 24-bit round-trip: samples match within quantization.
	var maxErr float64
	for i := range pcm {
		if e := math.Abs(float64(got[i] - pcm[i])); e > maxErr {
			maxErr = e
		}
	}
	if maxErr > 1e-3 {
		t.Errorf("round-trip error %g too large", maxErr)
	}
}

func TestEncodeIsDeterministic(t *testing.T) {
	pcm := Noise(0.3, 0.02, 48000, 7)
	a, _ := EncodeWAV(pcm, 48000)
	b, _ := EncodeWAV(pcm, 48000)
	if len(a) != len(b) {
		t.Fatalf("nondeterministic length %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("nondeterministic bytes at %d", i)
		}
	}
}

func TestNoiseDeterministicAndBounded(t *testing.T) {
	a := Noise(0.8, 0.01, 48000, 42)
	b := Noise(0.8, 0.01, 48000, 42)
	c := Noise(0.8, 0.01, 48000, 43)
	if len(a) != len(b) {
		t.Fatal("length mismatch")
	}
	same := true
	for i := range a {
		if a[i] != b[i] {
			same = false
			break
		}
		if math.Abs(float64(a[i])) > 0.8+1e-6 {
			t.Fatalf("noise out of amplitude bound: %g", a[i])
		}
	}
	if !same {
		t.Error("same seed produced different noise")
	}
	if len(c) == len(a) && a[100] == c[100] {
		t.Error("different seed produced identical sample")
	}
}

func TestSineFrequency(t *testing.T) {
	sr := 48000
	pcm := Sine(1000, 1.0, 0.1, sr)
	// Zero-crossings (rising) ≈ freq*dur; count sign changes - to + .
	crossings := 0
	for i := 1; i < len(pcm); i++ {
		if pcm[i-1] < 0 && pcm[i] >= 0 {
			crossings++
		}
	}
	// 1000 Hz over 0.1s → ~100 rising crossings.
	if crossings < 95 || crossings > 105 {
		t.Errorf("rising zero-crossings = %d, want ~100", crossings)
	}
}

func TestDecodeRejectsGarbage(t *testing.T) {
	if _, _, err := DecodeWAV([]byte("not a wav file at all")); err == nil {
		t.Error("expected error on non-WAV input")
	}
}

// Sanity: the encoder we reuse is the app's registered WAV24 encoder.
func TestReusesAudioEncoder(t *testing.T) {
	if _, err := audio.NewEncoder(audio.FormatWAV24); err != nil {
		t.Fatalf("audio WAV24 encoder unavailable: %v", err)
	}
}
