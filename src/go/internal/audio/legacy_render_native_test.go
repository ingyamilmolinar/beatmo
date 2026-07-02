//go:build !test && !js

package audio

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

// Guard-branch and error-path coverage for the legacy (unparameterized)
// CGo render wrappers in drums_c.go / fmsynth_c.go and the loadAudio
// decode helper. The golden tests cover the happy render path; these cover
// the empty-buffer early returns, the oversized-samples panic, the FM
// envelope's past-end region, and load_audio's error reporting through
// result_description.
//
// Known-unreachable C lines (documented, deliberately not chased):
//   - drums.c ma_noise_init failure guards (6 sites + zero-fill fallback):
//     ma_noise_init only fails on NULL config/output, which the call sites
//     never pass.
//   - drums.c load_audio mid-decode failures (length query, OOM, short
//     read): need a corrupted-but-openable stream; the init-failure path is
//     covered below.
//   - insert_fx.c `!buf || buf_len <= 0` passthrough guards (delay, chorus,
//     flanger, tape): the Go constructors always allocate (sr<=0 falls back
//     to 44100), so the guards protect only direct C misuse / the JS-side
//     allocation path.

var legacyRenderers = map[string]cRenderer{
	// snare / clap (+ rimshot / sidestick) migrated to the modular engine
	// (Phase-5): their legacy renderSnare* / renderClap wrappers were deleted, so
	// they drop out of this legacy guard-branch table. The modular fast path
	// (renderSnareVoice etc.) guards empty buffers by returning (not panicking) and
	// is exercised in snare_native_fastpath_test.go.
	// kick (+ deep/punchy/lofi/tight) migrated to the modular engine (Phase-3):
	// their legacy renderKick* wrappers were deleted, so they drop out of this
	// legacy guard-branch table. The modular fast path (renderKickVoice etc.)
	// guards empty buffers by returning (not panicking) and is exercised in
	// kick_native_fastpath_test.go.
	// hihat / open-hihat / cowbell / shaker / ride / crash migrated to the modular
	// engine (Phase-6): their legacy renderHiHat* / renderCowbell / renderShaker /
	// renderRide / renderCrash wrappers were deleted, so they drop out of this
	// legacy guard-branch table. The modular fast path (renderHiHatVoice etc.)
	// guards empty buffers by returning (not panicking) and is exercised in
	// cymbal_native_fastpath_test.go.
	// tom (+ high/low) migrated to the modular engine (Phase-4): their legacy
	// renderTom* wrappers were deleted, so they drop out of this legacy
	// guard-branch table. The modular fast path (renderTomVoice etc.) guards empty
	// buffers by returning (not panicking) and is exercised in
	// tom_native_fastpath_test.go.
	// bass-guitar / sub-bass migrated to the modular engine (Phase-2): their
	// legacy renderBassGuitar / renderSubBass wrappers were deleted, so they
	// drop out of this legacy guard-branch table. The modular fast path
	// (renderSubBassVoice) guards empty buffers by
	// returning (not panicking) and is exercised in bass_native_fastpath_test.go.
	// fm-bass / fm-bell / fm-lead / fm-epiano / fm-pluck migrated to the modular
	// engine (Phase-7, the LAST legacy family): their legacy render_fm_* wrappers
	// were deleted, so they drop out of this legacy guard-branch table. The modular
	// fast path (renderFMBassVoice etc.) guards empty buffers by returning (not
	// panicking) and is exercised in fm_native_fastpath_test.go.
	//
	// EVERY legacy family has now migrated — this legacyRenderers table is EMPTY.
	// The two guard tests below iterate it (now a no-op), documenting that the
	// legacy unparameterized render-wrapper path is fully retired.
}

func TestLegacyRenderEmptyBufferIsNoOp(t *testing.T) {
	for name, render := range legacyRenderers {
		render(nil, 44100, 0)                // empty buffer + zero samples
		render([]float32{}, 44100, 0)        // empty non-nil
		render(make([]float32, 8), 44100, 0) // zero samples, real buffer
		_ = name
	}
}

func TestLegacyRenderOversizedSamplesPanics(t *testing.T) {
	for name, render := range legacyRenderers {
		name, render := name, render
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatalf("%s: samples > len(buf) must panic", name)
				}
			}()
			render(make([]float32, 4), 44100, 8)
		})
	}
}

// The FM presets normalize their envelope to the render length
// (total_dur = samples*dt in fm_render), so the release always completes
// exactly at the buffer end — the envelope's terminal `return 0.0f` branch
// is unreachable per-render (documented defensive line). What we CAN pin:
// long renders stay finite and the release has fully tapered by the final
// sample. Post-Phase-7 the FM presets render through the modular engine
// (renderFM*Voice → source==10 voice → the SAME fm_render core), with the
// POST stage a no-op at recipe defaults — so this exercises the identical
// fm_render long-buffer behavior the deleted render_fm_* wrappers did.
func TestFMRenderLongBufferTapersToSilence(t *testing.T) {
	const sr = 44100
	for _, tc := range []struct {
		name   string
		render cRenderer
	}{
		{"fm-bass", renderFMBassVoice},
		{"fm-bell", renderFMBellVoice},
		{"fm-lead", renderFMLeadVoice},
		{"fm-epiano", renderFMEPianoVoice},
		{"fm-pluck", renderFMPluckVoice},
	} {
		t.Run(tc.name, func(t *testing.T) {
			buf := make([]float32, 4*sr) // 4 s — far longer than the goldens
			tc.render(buf, sr, len(buf))
			var energy float64
			for i, v := range buf {
				if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
					t.Fatalf("buf[%d] not finite: %v", i, v)
				}
				energy += float64(v) * float64(v)
			}
			if energy == 0 {
				t.Fatal("long render is silent")
			}
			// Release completes at buffer end: last sample ~0.
			if last := buf[len(buf)-1]; math.Abs(float64(last)) > 0.02 {
				t.Fatalf("final sample = %v, want ~0 (release complete)", last)
			}
		})
	}
}

func TestLoadAudioErrorPaths(t *testing.T) {
	// Nonexistent file: C load_audio returns a negative ma_result; the Go
	// wrapper renders it via result_description.
	if _, _, err := loadAudio(filepath.Join(t.TempDir(), "missing.wav")); err == nil {
		t.Fatal("loadAudio(missing) should fail")
	}

	// Garbage bytes: decoder init rejects them.
	bad := filepath.Join(t.TempDir(), "garbage.wav")
	if err := os.WriteFile(bad, []byte("not an audio file at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadAudio(bad); err == nil {
		t.Fatal("loadAudio(garbage) should fail")
	}
}
