//go:build !test && !js

package audio

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"math"
	"testing"
)

// TestFMPresetGolden locks the byte output of the five FM presets
// (bass/bell/lead/epiano/pluck) against a known-good hash.
//
// Why this test exists: the modular-voice work de-staticised fm_render in
// fmsynth.c (`static void fm_render` -> `void fm_render`) and moved the
// fm_operator / fm_preset structs into fmsynth.h so modular.c can reuse the
// core renderer. That refactor touches the exact code path every FM
// instrument depends on. The plan flagged it as the single riskiest change.
//
// The golden hashes below were generated from the pre-refactor baseline
// (git HEAD before the de-static change) and re-verified against the
// post-refactor working tree, empirically proving the refactor is
// byte-identical. From here on, any future change that alters fm_render's
// numerical output — intentional or accidental — fails this test.
//
// Phase-7 cutover: the static render_fm_* C wrappers were deleted, so the
// renders now flow through the migrated modular no-edit fast path
// (renderFM*Voice → source==10 FM voice → the SAME fm_render core, with a
// no-op POST stage at recipe defaults). The hashes are UNCHANGED — this is the
// proof the modular FM voice reproduces the legacy static presets BYTE-FOR-BYTE
// (the same invariant the oracle |default cases assert, pinned here at the
// golden-render config).
//
// Render config is fixed (48 kHz, 48000 samples) so the hashes are stable.
func TestFMPresetGolden(t *testing.T) {
	const (
		sr      = 48000
		samples = 48000
	)
	cases := []struct {
		name   string
		render func(buf []float32, sampleRate, samples int)
		want   string
	}{
		{"fm-bass", renderFMBassVoice, "998475b068d793cea7105767e9c066ac6c9f63e17752a5f9b801c6eff449a39f"},
		{"fm-bell", renderFMBellVoice, "5292788a862816663611bf08163160c700387aab0d39dca109e2062c34c0a096"},
		{"fm-lead", renderFMLeadVoice, "0612df6948f77cc95245f313e671d229d7b7d73e83665007be4bc363254dbb4b"},
		{"fm-epiano", renderFMEPianoVoice, "69d406b04c8f3e42293fa68aa1b0678575a0419ab351859dbcd51c71b68c451f"},
		{"fm-pluck", renderFMPluckVoice, "61463214e4b161b265127b59a0a05cb1ab8d1fc92c8df1be510726a26824e98c"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := make([]float32, samples)
			tc.render(buf, sr, samples)
			got := hashFloat32(buf)
			if got != tc.want {
				t.Fatalf("%s render hash drifted\n  got:  %s\n  want: %s\n(if this change to fm_render is intentional, update the golden)", tc.name, got, tc.want)
			}
		})
	}
}

// hashFloat32 returns a stable sha256 hex digest over the little-endian byte
// representation of a float32 buffer.
func hashFloat32(buf []float32) string {
	h := sha256.New()
	var b [4]byte
	for _, v := range buf {
		binary.LittleEndian.PutUint32(b[:], math.Float32bits(v))
		h.Write(b[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}
