//go:build test

package ui

import (
	"testing"
	"time"
)

// fEq32 compares a float64 (widened from a float32 sample) against an expected
// value with a tolerance large enough to absorb float32 rounding.
func fEq32(a, b float64) bool { return a-b < 1e-6 && b-a < 1e-6 }

// sampler_reverse_test.go — the Reverse control is a stateful TOGGLE: one click
// reverses the working signal in place AND latches the button ON (so it reads
// like every other toggle), a second click un-reverses and un-latches. The
// playback highlight must ALWAYS sweep left→right (forward in time), never
// right→left. Latched-state assertions live in sampler_toggle_state_test.go.

// ── Reverse latches its state and reverses in place ──────────────────────────

func TestSamplerReverseLatchesAcrossRebuild(t *testing.T) {
	assertDefaultParityState(t)
	// A Layout rebuild recreates the buttons; the reverse toggle must re-derive
	// its latched state from the persistent samplerState.reverse parity, so the
	// engaged keycap survives the rebuild.
	dv := &DrumView{}
	dv.sampler.raw = []float32{1, 2, 3, 4}
	dv.sampler.rawSampleRate = 48000
	dv.buildSamplerButtons(true)
	rev := dv.samplerButtonByTag("sampler-reverse")
	if rev == nil {
		t.Fatal("no reverse button")
	}
	if rev.Toggled() {
		t.Fatal("reverse must not start latched")
	}
	rev.OnClick()
	dv.buildSamplerButtons(true) // rebuild as a Layout would
	rev = dv.samplerButtonByTag("sampler-reverse")
	if !rev.Toggled() {
		t.Error("reverse must stay latched ON across a Layout rebuild while the buffer is reversed")
	}
}

// ── A click reverses the working signal in place ─────────────────────────────

func TestSamplerReverseClickReversesBuffer(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	dv.sampler.raw = []float32{1, 2, 3, 4}
	dv.sampler.rawSampleRate = 48000
	dv.buildSamplerButtons(true)

	dv.samplerButtonByTag("sampler-reverse").OnClick()

	want := []float32{4, 3, 2, 1}
	for i := range want {
		if dv.sampler.raw[i] != want[i] {
			t.Fatalf("after reverse raw=%v, want %v", dv.sampler.raw, want)
		}
	}
}

func TestSamplerReverseTwiceRestoresOriginal(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	orig := []float32{1, 2, 3, 4, 5}
	dv.sampler.raw = append([]float32(nil), orig...)
	dv.sampler.rawSampleRate = 48000
	dv.buildSamplerButtons(true)

	rev := dv.samplerButtonByTag("sampler-reverse")
	rev.OnClick()
	rev.OnClick()

	for i := range orig {
		if dv.sampler.raw[i] != orig[i] {
			t.Fatalf("two reverses should restore: raw=%v, want %v", dv.sampler.raw, orig)
		}
	}
}

func TestSamplerReverseFlipsDisplayedWaveform(t *testing.T) {
	assertDefaultParityState(t)
	// The displayed waveform reads s.raw via waveFloat64(); reversing must change
	// what is drawn (the visible signal flips), not just the audio.
	s := &samplerState{raw: []float32{0.1, 0.2, 0.9}, rawSampleRate: 48000}
	s.reverseBuffer()
	wave := s.waveFloat64()
	// float32→float64 widening loses precision, so compare with tolerance.
	if !fEq32(wave[0], 0.9) || !fEq32(wave[2], 0.1) {
		t.Fatalf("displayed waveform not reversed: %v", wave)
	}
}

func TestSamplerReverseEmptyBufferIsNoop(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{}
	s.reverseBuffer() // must not panic on an empty buffer
	if len(s.raw) != 0 {
		t.Fatalf("empty reverse produced %d samples", len(s.raw))
	}
}

// ── Reverse moves the trim window to keep the SAME audio, reversed ────────────
//
// Regression: clicking Reverse flipped the working buffer while leaving the trim
// handles at fixed fractions, so the kept [start,end] region slid onto the
// mirror-image part of the sound. Reverse must MIRROR the trim window so the
// audible chop stays the same audio, only reversed.

// Dyadic trim fractions (0.25, 0.5) map to exact sample indices on an 8-sample
// buffer AND survive the 1-x mirror without float drift, so these tests exercise
// the region-preservation contract without brittleness from int(frac*n) rounding.
// The window is asymmetric about 0.5 so a missing mirror is detectable.

func TestSamplerReverseKeepsTrimmedRegionReversed(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	// Unique ramp so every kept sample is identifiable by value.
	raw := make([]float32, 8)
	for i := range raw {
		raw[i] = float32(i + 1) // 1..8
	}
	dv.sampler.raw = append([]float32(nil), raw...)
	dv.sampler.rawSampleRate = 48000
	dv.sampler.reset()
	dv.sampler.startFrac, dv.sampler.endFrac = 0.25, 0.5 // keep samples {3,4}
	before, _ := dv.sampler.bake()

	dv.buildSamplerButtons(true)
	dv.samplerButtonByTag("sampler-reverse").OnClick()

	after, _ := dv.sampler.bake()
	if len(after) != len(before) {
		t.Fatalf("reverse changed the kept region length: before %d, after %d", len(before), len(after))
	}
	for i := range before {
		want := float64(before[len(before)-1-i])
		if !fEq32(float64(after[i]), want) {
			t.Fatalf("reverse must keep the SAME audio reversed:\n before=%v\n after=%v", before, after)
		}
	}
}

func TestSamplerReverseTwiceRestoresTrimmedRegion(t *testing.T) {
	assertDefaultParityState(t)
	dv := &DrumView{}
	raw := make([]float32, 8)
	for i := range raw {
		raw[i] = float32(i + 1)
	}
	dv.sampler.raw = append([]float32(nil), raw...)
	dv.sampler.rawSampleRate = 48000
	dv.sampler.reset()
	dv.sampler.startFrac, dv.sampler.endFrac = 0.25, 0.5
	before, _ := dv.sampler.bake()

	dv.buildSamplerButtons(true)
	rev := dv.samplerButtonByTag("sampler-reverse")
	rev.OnClick()
	rev.OnClick()

	after, _ := dv.sampler.bake()
	if len(after) != len(before) {
		t.Fatalf("two reverses changed the kept length: before %d, after %d", len(before), len(after))
	}
	for i := range before {
		if !fEq32(float64(after[i]), float64(before[i])) {
			t.Fatalf("two reverses must restore the original trimmed region:\n before=%v\n after=%v", before, after)
		}
	}
}

// ── Reverse is baked into the buffer, not re-applied at bake time ─────────────

func TestSamplerEditNeverRequestsReverse(t *testing.T) {
	assertDefaultParityState(t)
	// Because reverse mutates the working buffer, the bake edit must NOT also set
	// Reverse — otherwise Save/Preview would double-reverse (cancelling it).
	s := &samplerState{raw: []float32{1, 2, 3, 4}, rawSampleRate: 48000}
	s.reset()
	s.reverseBuffer()
	if s.edit().Reverse {
		t.Fatal("edit().Reverse must stay false — reversal lives in the buffer, not the edit")
	}
}

func TestSamplerBakeAfterReverseMatchesDisplayedBuffer(t *testing.T) {
	assertDefaultParityState(t)
	// WYSIWYG: with the full range kept and no pitch/gain, the baked output equals
	// the (reversed) displayed buffer — proving no hidden second reverse.
	s := &samplerState{raw: []float32{1, 2, 3, 4}, rawSampleRate: 48000}
	s.reset()
	s.reverseBuffer()
	baked, _ := s.bake()
	if len(baked) != len(s.raw) {
		t.Fatalf("baked len %d != raw len %d", len(baked), len(s.raw))
	}
	for i := range baked {
		if baked[i] != s.raw[i] {
			t.Fatalf("baked %v != displayed %v (double reverse?)", baked, s.raw)
		}
	}
}

// ── editDescriptor: declarative reverse for the non-destructive synth path ───

func TestSamplerEditDescriptorTracksReverseFlag(t *testing.T) {
	assertDefaultParityState(t)
	// The non-destructive synth path applies the edit to a FORWARD-rendered
	// recipe buffer at trigger time, so unlike edit() (whose source buffer is
	// already flipped), editDescriptor() must carry Reverse declaratively.
	s := &samplerState{raw: []float32{1, 2, 3, 4}, rawSampleRate: 48000}
	s.reset()
	if s.editDescriptor().Reverse {
		t.Fatal("fresh state: editDescriptor().Reverse must be false")
	}
	s.reverseBuffer()
	if !s.editDescriptor().Reverse {
		t.Fatal("after one reverse: editDescriptor().Reverse must be true")
	}
	s.reverseBuffer()
	if s.editDescriptor().Reverse {
		t.Fatal("after two reverses: editDescriptor().Reverse must be false again")
	}
	// edit() must stay false regardless — its source buffer carries the flip.
	if s.edit().Reverse {
		t.Fatal("edit().Reverse must stay false — reversal lives in the buffer")
	}
}

func TestSamplerEditDescriptorMatchesEditOtherFields(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{raw: []float32{1, 2, 3, 4}, rawSampleRate: 48000}
	s.reset()
	s.startFrac, s.endFrac = 0.25, 0.75
	s.transposeSemis, s.detuneCents = 3, -20
	s.gainDB = -6
	s.normalize = true
	s.fadeOn = true
	d, e := s.editDescriptor(), s.edit()
	d.Reverse = e.Reverse // the only intentional divergence
	if d != e {
		t.Fatalf("editDescriptor diverged beyond Reverse: %+v vs %+v", d, e)
	}
}

func TestSamplerResetClearsReverseFlag(t *testing.T) {
	assertDefaultParityState(t)
	s := &samplerState{raw: []float32{1, 2, 3, 4}, rawSampleRate: 48000}
	s.reset()
	s.reverseBuffer()
	s.reset()
	if s.editDescriptor().Reverse {
		t.Fatal("reset() must clear the declarative reverse flag")
	}
}

// ── Playhead always sweeps forward (left→right) ──────────────────────────────

func TestSamplerPlayheadFracAlwaysForward(t *testing.T) {
	assertDefaultParityState(t)
	cases := []struct {
		name     string
		elapsed  float64
		wantFrac float64
	}{
		{"start", 0.0, 0.2},
		{"mid", 0.5, 0.5},
		{"end", 1.0, 0.8},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			frac, vis := samplerPlayheadFrac(c.elapsed, 1.0, 0.2, 0.8)
			if !vis {
				t.Fatalf("elapsed %v: want visible", c.elapsed)
			}
			if !fEq(frac, c.wantFrac) {
				t.Errorf("elapsed %v: frac=%v, want %v (must move left→right)", c.elapsed, frac, c.wantFrac)
			}
		})
	}
}

func TestSamplerPlayheadFracMonotonicNonDecreasing(t *testing.T) {
	assertDefaultParityState(t)
	prev := -1.0
	for i := 0; i <= 100; i++ {
		elapsed := float64(i) / 100.0
		frac, vis := samplerPlayheadFrac(elapsed, 1.0, 0.1, 0.9)
		if !vis {
			t.Fatalf("elapsed %v should be visible", elapsed)
		}
		if frac+1e-9 < prev {
			t.Fatalf("frac went backward at elapsed %v: %v < %v (right→left forbidden)", elapsed, frac, prev)
		}
		prev = frac
	}
}

func TestSamplerActivePlayheadSweepsForwardAfterReverse(t *testing.T) {
	assertDefaultParityState(t)
	// Integration: even after the buffer has been reversed, the in-flight line's
	// x-fraction must increase over time (forward), never decrease.
	var tr samplerPlayheadTracker
	s := &samplerState{raw: []float32{1, 2, 3, 4}, rawSampleRate: 48000}
	s.reset()
	s.reverseBuffer()

	base := int64(1e9)
	tr.observe(base, map[string]int64{"a": base}, 2.0)

	frac := func(elapsedMs int) float64 {
		v := tr.active(base+int64(elapsedMs)*int64(time.Millisecond), 2.0, 0.0, 1.0)
		if len(v) != 1 {
			t.Fatalf("elapsed %dms: got %d views, want 1", elapsedMs, len(v))
		}
		return v[0].frac
	}
	early := frac(200)
	late := frac(1500)
	if late <= early {
		t.Fatalf("after reverse the playhead swept backward: early=%v late=%v (must go forward)", early, late)
	}
}
