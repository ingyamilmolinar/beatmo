package audio

import (
	"math"
	"testing"
)

func approxF32(t *testing.T, got, want, tol float32, msg string) {
	t.Helper()
	d := got - want
	if d < 0 {
		d = -d
	}
	if d > tol {
		t.Fatalf("%s: got %v want %v (tol %v)", msg, got, want, tol)
	}
}

func ramp(n int) []float32 {
	b := make([]float32, n)
	for i := range b {
		b[i] = float32(i)
	}
	return b
}

func TestTrimSampleSelectsRegion(t *testing.T) {
	src := ramp(100)
	out := TrimSample(src, 0.25, 0.75)
	if len(out) != 50 {
		t.Fatalf("len = %d, want 50", len(out))
	}
	if out[0] != 25 || out[len(out)-1] != 74 {
		t.Fatalf("region = [%v..%v], want [25..74]", out[0], out[len(out)-1])
	}
}

func TestTrimSampleClampsFractions(t *testing.T) {
	src := ramp(100)
	// Out-of-range fractions clamp to [0,1].
	full := TrimSample(src, -0.5, 1.5)
	if len(full) != 100 {
		t.Fatalf("clamped len = %d, want 100", len(full))
	}
	// start >= end yields an empty region rather than negative length.
	empty := TrimSample(src, 0.8, 0.2)
	if len(empty) != 0 {
		t.Fatalf("inverted region len = %d, want 0", len(empty))
	}
}

func TestReverseSampleSymmetry(t *testing.T) {
	src := []float32{1, 2, 3, 4}
	rev := ReverseSample(append([]float32(nil), src...))
	want := []float32{4, 3, 2, 1}
	for i := range want {
		if rev[i] != want[i] {
			t.Fatalf("rev[%d] = %v, want %v", i, rev[i], want[i])
		}
	}
	// Reversing twice restores the original.
	back := ReverseSample(append([]float32(nil), rev...))
	for i := range src {
		if back[i] != src[i] {
			t.Fatalf("double-reverse[%d] = %v, want %v", i, back[i], src[i])
		}
	}
}

func TestResampleSemitonesUpHalvesLength(t *testing.T) {
	src := ramp(100)
	out := ResampleSemitones(src, 12, 0)
	if len(out) != 50 {
		t.Fatalf("len = %d, want 50 (one octave up halves length)", len(out))
	}
}

func TestResampleSemitonesZeroIsIdentity(t *testing.T) {
	src := ramp(64)
	out := ResampleSemitones(src, 0, 0)
	if len(out) != len(src) {
		t.Fatalf("len = %d, want %d", len(out), len(src))
	}
	for i := range src {
		approxF32(t, out[i], src[i], 1e-4, "identity resample")
	}
}

func TestResampleSemitonesDetuneEquivalence(t *testing.T) {
	src := ramp(100)
	// 1200 cents == 12 semitones, so detune-only should also halve length.
	out := ResampleSemitones(src, 0, 1200)
	if len(out) != 50 {
		t.Fatalf("len = %d, want 50 (1200 cents == octave)", len(out))
	}
}

func TestApplyGainDBLinear(t *testing.T) {
	src := []float32{0.5, -0.5, 0.25}
	// +6.0206 dB ~= x2.0
	out := ApplyGainDB(append([]float32(nil), src...), 20*float32(math.Log10(2)))
	approxF32(t, out[0], 1.0, 1e-3, "gain +6dB sample0")
	approxF32(t, out[1], -1.0, 1e-3, "gain +6dB sample1")
	// 0 dB leaves the signal untouched.
	id := ApplyGainDB(append([]float32(nil), src...), 0)
	for i := range src {
		approxF32(t, id[i], src[i], 1e-6, "gain 0dB identity")
	}
}

func TestNormalizePeakReaches0dBFS(t *testing.T) {
	src := []float32{0.0, 0.25, -0.5, 0.1}
	out := NormalizePeak(append([]float32(nil), src...))
	var peak float32
	for _, v := range out {
		a := v
		if a < 0 {
			a = -a
		}
		if a > peak {
			peak = a
		}
	}
	approxF32(t, peak, 1.0, 1e-4, "normalized peak")
	// Silence stays silent (no divide-by-zero).
	sil := NormalizePeak([]float32{0, 0, 0})
	for i, v := range sil {
		if v != 0 {
			t.Fatalf("silence normalize[%d] = %v, want 0", i, v)
		}
	}
}

func TestApplyFadesEndpointsZero(t *testing.T) {
	n := 100
	buf := make([]float32, n)
	for i := range buf {
		buf[i] = 1.0
	}
	sr := 1000 // 10ms == 10 samples
	out := ApplyFades(buf, sr, 10, 10)
	approxF32(t, out[0], 0.0, 1e-3, "fade-in start")
	approxF32(t, out[n-1], 0.0, 1e-3, "fade-out end")
	approxF32(t, out[n/2], 1.0, 1e-3, "fade middle untouched")
}

func TestBakeSamplePipelineProducesTrimmedReversedBuffer(t *testing.T) {
	src := ramp(100)
	e := SampleEdit{
		StartFrac: 0.0, EndFrac: 0.5, // keep first 50 samples (values 0..49)
		Reverse: true,
		GainDB:  0,
	}
	out := BakeSample(src, 1000, e)
	if len(out) != 50 {
		t.Fatalf("baked len = %d, want 50", len(out))
	}
	// Reversed: first baked sample comes from the trimmed region's end (value 49).
	if out[0] != 49 {
		t.Fatalf("baked[0] = %v, want 49 (reversed trim)", out[0])
	}
}

func TestBakeSampleNormalizeThenGainOrder(t *testing.T) {
	// A quiet buffer normalized to 0 dBFS then attenuated -6 dB should peak ~0.5.
	src := []float32{0, 0.1, -0.2, 0.05}
	e := SampleEdit{StartFrac: 0, EndFrac: 1, Normalize: true, GainDB: -20 * float32(math.Log10(2))}
	out := BakeSample(src, 1000, e)
	var peak float32
	for _, v := range out {
		a := v
		if a < 0 {
			a = -a
		}
		if a > peak {
			peak = a
		}
	}
	approxF32(t, peak, 0.5, 1e-3, "normalize then -6dB peak")
}

func TestResampleToRateChangesLengthByRatio(t *testing.T) {
	src := ramp(100)
	// 48k -> 24k halves the length.
	out := ResampleToRate(src, 48000, 24000)
	if len(out) != 50 {
		t.Errorf("downsample 48k->24k len = %d, want 50", len(out))
	}
	// Same rate is identity.
	id := ResampleToRate(src, 44100, 44100)
	if len(id) != len(src) {
		t.Errorf("same-rate resample len = %d, want %d", len(id), len(src))
	}
	// 0/invalid rates pass through unchanged.
	pass := ResampleToRate(src, 0, 44100)
	if len(pass) != len(src) {
		t.Errorf("invalid-rate resample len = %d, want %d", len(pass), len(src))
	}
}

// ── Multi-octave / extreme-value resampling (gap-closure batch) ─────────────

func TestResampleSemitonesMultiOctave(t *testing.T) {
	src := ramp(9600)
	for _, tc := range []struct {
		semitones float64
		wantLen   int
	}{
		{12, 4800},   // +1 octave halves
		{24, 2400},   // +2 octaves quarters
		{36, 1200},   // +3 octaves eighths
		{-12, 19200}, // -1 octave doubles
		{-24, 38400}, // -2 octaves quadruples
		{-36, 76800}, // -3 octaves octuples
	} {
		out := ResampleSemitones(src, tc.semitones, 0)
		// Linear interp walks floor((n-1)/step) steps, so the length lands
		// within 0.1% of the ideal ratio (a few samples at 8x stretch).
		tol := tc.wantLen/1000 + 1
		if d := out_len_diff(len(out), tc.wantLen); d > tol {
			t.Fatalf("%+.0f st: len = %d, want %d±%d", tc.semitones, len(out), tc.wantLen, tol)
		}
		for i, v := range out {
			f := float64(v)
			if math.IsNaN(f) || math.IsInf(f, 0) {
				t.Fatalf("%+.0f st: out[%d] not finite", tc.semitones, i)
			}
		}
	}
}

func out_len_diff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

func TestResampleSemitonesExtremeCents(t *testing.T) {
	src := ramp(1000)
	for _, cents := range []float64{-1200, -750, 750, 1200} {
		out := ResampleSemitones(src, 0, cents)
		if len(out) == 0 {
			t.Fatalf("cents %v produced empty output", cents)
		}
		for i, v := range out {
			f := float64(v)
			if math.IsNaN(f) || math.IsInf(f, 0) {
				t.Fatalf("cents %v: out[%d] not finite", cents, i)
			}
		}
	}
}

func TestResampleEdgeBuffers(t *testing.T) {
	// Empty source: stays empty for any shift.
	if out := ResampleSemitones(nil, 12, 0); len(out) != 0 {
		t.Fatalf("nil src produced %d samples", len(out))
	}
	if out := ResampleSemitones([]float32{}, -24, 0); len(out) != 0 {
		t.Fatalf("empty src produced %d samples", len(out))
	}

	// Single sample: must survive both directions without panicking.
	one := []float32{0.5}
	if out := ResampleSemitones(one, 12, 0); len(out) > 1 {
		t.Fatalf("1-sample src upshift produced %d samples", len(out))
	}
	down := ResampleSemitones(one, -12, 0)
	if len(down) == 0 {
		t.Fatal("1-sample src downshift produced nothing")
	}
	for i, v := range down {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			t.Fatalf("down[%d] not finite", i)
		}
	}
}

func TestResampleToRateEdgeCases(t *testing.T) {
	src := ramp(441)
	// Invalid rates pass through as a copy.
	for _, tc := range [][2]int{{0, 48000}, {44100, 0}, {-1, -1}, {44100, 44100}} {
		out := ResampleToRate(src, tc[0], tc[1])
		if len(out) != len(src) {
			t.Fatalf("rates %v: len = %d, want passthrough %d", tc, len(out), len(src))
		}
	}
	// Genuine conversions land near the ratio.
	up := ResampleToRate(src, 44100, 48000)
	if d := out_len_diff(len(up), 480); d > 1 {
		t.Fatalf("44.1k→48k: len = %d, want ~480", len(up))
	}
	dn := ResampleToRate(src, 48000, 44100)
	if d := out_len_diff(len(dn), 405); d > 1 {
		t.Fatalf("48k→44.1k: len = %d, want ~405", len(dn))
	}
}

func TestBakeSampleExtremePipeline(t *testing.T) {
	src := ramp(4096)
	edit := SampleEdit{
		StartFrac:      0.1,
		EndFrac:        0.9,
		TransposeSemis: -24,
		DetuneCents:    -100,
		GainDB:         12,
		FadeInMs:       5,
		FadeOutMs:      5,
		Reverse:        true,
		Normalize:      true,
	}
	out := BakeSample(src, 44100, edit)
	if len(out) == 0 {
		t.Fatal("extreme bake produced nothing")
	}
	var peak float64
	for i, v := range out {
		f := math.Abs(float64(v))
		if math.IsNaN(f) || math.IsInf(f, 0) {
			t.Fatalf("out[%d] not finite", i)
		}
		if f > peak {
			peak = f
		}
	}
	// Normalize→gain order: +12 dB after normalize-to-1 ⇒ peak ~3.98,
	// proving gain applies post-normalize (locked by BakeSample's contract).
	if peak < 3.5 || peak > 4.5 {
		t.Fatalf("peak = %v, want ~3.98 (normalize then +12 dB)", peak)
	}
}
