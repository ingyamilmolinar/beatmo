//go:build !test && !js

// export_audio is a tool that renders desktop audio samples and exports them
// to JSON for cross-platform comparison with WASM output.
//
// Usage:
//
//	cd src/go && go run ./cmd/export_audio > ../js/desktop_audio.json
//	cd src/go && go run ./cmd/export_audio --config > ../js/audio_config.generated.js
//	cd src/go && go run ./cmd/export_audio --mixed > ../js/desktop_mixed_audio.json
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// InstrumentSample contains rendered audio data for one instrument.
type InstrumentSample struct {
	Name       string    `json:"name"`
	SampleRate int       `json:"sampleRate"`
	Frames     int       `json:"frames"`
	Amplitude  float64   `json:"amplitude"`
	Duration   float64   `json:"duration"`
	Samples    []float64 `json:"samples"`
	Peak       float64   `json:"peak"`
	RMS        float64   `json:"rms"`
	DCOffset   float64   `json:"dcOffset"`
}

// ParamCaseSample is one parameterized recipe render: the desktop reference
// for a (recipe, params) pair, captured RAW from SynthRecipe.Render — no
// normalization or amplitude scaling — so the browser can compare its own
// schema-keyed param-block construction + C render against ours sample-by-
// sample. Covers knob edits, per-stage enable toggles, family knobs,
// and every modular generator type.
type ParamCaseSample struct {
	Name   string             `json:"name"`
	Recipe string             `json:"recipe"`
	Params map[string]float64 `json:"params"`
	// ParamBlock tells the browser which param block to fill + which renderer to
	// dispatch, overriding its recipe-prefix inference. Empty = infer as before.
	// Migrated families (bass) set "modular" so the browser fills the modular
	// block (Params already translated to modular-named values) and calls
	// render_modular_p — the desktop reference above already rendered through the
	// modular binding, so the two platforms exercise the same engine.
	ParamBlock string    `json:"paramBlock,omitempty"`
	SampleRate int       `json:"sampleRate"`
	Frames     int       `json:"frames"`
	Samples    []float64 `json:"samples"`
	Peak       float64   `json:"peak"`
	RMS        float64   `json:"rms"`
}

// ExportData contains all exported audio samples.
type ExportData struct {
	Version     int                `json:"version"`
	Description string             `json:"description"`
	Instruments []InstrumentSample `json:"instruments"`
	ParamCases  []ParamCaseSample  `json:"paramCases,omitempty"`
}

func main() {
	// Check for --config flag
	for _, arg := range os.Args[1:] {
		if arg == "--config" {
			exportJSConfig()
			return
		}
	}

	// Default: export audio samples
	exportAudioSamples()
}

func exportJSConfig() {
	fmt.Println("// AUTO-GENERATED from Go - do not edit")
	fmt.Println("// Run: cd src/go && go run ./cmd/export_audio --config > ../js/audio_config.generated.js")
	fmt.Println("")
	fmt.Println("export const RENDER_INFO = {")

	// Export all instrument configs (durations/amps for the JS INSTRUMENT_CONFIG).
	// Every shipped instrument keeps its config entry regardless of which C engine
	// renders it — kick (Phase-3) and tom (Phase-4) render via the modular engine
	// but still need their duration/amp config here.
	instruments := []string{"snare", "hihat", "tom", "clap", "cowbell"}
	for i, id := range instruments {
		cfg := audio.ConfigForInstrument(id)
		comma := ","
		if i == len(instruments)-1 {
			comma = ""
		}
		fmt.Printf("  %s: { seconds: %.2f, amp: %.1f }%s\n", id, cfg.DurationSec, cfg.Amplitude, comma)
	}
	fmt.Println("};")
	fmt.Println("")
	fmt.Printf("export const MIX_HEADROOM = %.2f;\n", audio.MixHeadroom)
}

func exportAudioSamples() {
	// Initialize audio system
	audio.Reset()
	audio.ResetInstruments()

	data := ExportData{
		Version:     1,
		Description: "Desktop Go audio samples for cross-platform comparison",
	}

	// Define instruments to export. We don't specify duration here because
	// the Voice generator determines duration based on BPM.
	// At BPM=120, the durations are: snare=0.5s, hihat=0.125s,
	// tom=0.25s, clap=0.125s, cowbell=0.25s
	// Note: WASM uses different (longer) fixed durations which causes
	// waveform divergence since the C synth envelopes are proportional to buffer length.
	// EVERY legacy family migrated to the modular engine: kick (Phase-3), tom
	// (Phase-4), snare/clap (Phase-5), cymbal (Phase-6), bass (Phase-2), and FM
	// (Phase-7, the LAST) — no bespoke C renderer remains for the raw-render
	// harness. Their parity is covered by the kick-* / tom-* / snare-* / hihat-* /
	// fm-* … paramCases (paramBlock:'modular'). Only the unified modular voice's
	// base preset renders raw here.
	instruments := []string{
		// Unified modular voice (base preset). Its no-edit render uses the C
		// built-in defaults, so native render_modular and WASM render_modular
		// compare defaults-vs-defaults in xplat_audio_compare. (The pad preset
		// renders through a param block; its parity is covered end-to-end by
		// modular_synth_audible.browser.test.js, not this raw-render harness.)
		"modular",
	}

	const sampleRate = 48000 // Match the new desktop sample rate
	const bpm = 120          // Default BPM for voice generation

	for _, instID := range instruments {
		// Get a voice for this instrument which includes full processing pipeline
		// (C rendering + normalization + amplitude scaling)
		voice := audio.ExportVoice(instID, bpm, sampleRate)
		if voice == nil {
			fmt.Fprintf(os.Stderr, "Warning: could not get voice for %s\n", instID)
			continue
		}

		// Extract all samples from voice (don't specify frames - let voice determine length)
		samples := make([]float64, 0, sampleRate) // Pre-allocate for up to 1 second
		for {
			s, done := voice.Sample()
			if done {
				break
			}
			samples = append(samples, s)
		}

		// Compute metrics
		peak := computePeak(samples)
		rms := computeRMS(samples)
		dc := computeDCOffset(samples)

		// Calculate actual duration from sample count
		duration := float64(len(samples)) / float64(sampleRate)

		data.Instruments = append(data.Instruments, InstrumentSample{
			Name:       instID,
			SampleRate: sampleRate,
			Frames:     len(samples),
			Amplitude:  audio.AmplitudeForInstrumentExport(instID),
			Duration:   duration,
			Samples:    samples,
			Peak:       peak,
			RMS:        rms,
			DCOffset:   dc,
		})
	}

	data.ParamCases = renderParamCases()

	// Output as JSON
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(data); err != nil {
		fmt.Fprintf(os.Stderr, "Error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

// renderParamCases renders the parameterized cross-platform reference set.
//
// Semantics mirror the desktop production path exactly: params are merged
// over the recipe defaults (MergeRecipeDefaults — what tryRecipeVoice does)
// and rendered through SynthRecipe.Render with variant 0. The browser side
// (xplat_audio_compare.browser.test.js) constructs the same cases through
// ITS production semantics: the user-overlay params written into the
// schema-keyed param block with ABI-identity fallback, then render_X_p /
// render_modular_p. A platform that constructs the block differently —
// stale ABI, missing identity, dropped enable bit, unseeded recipe
// default — fails the comparison.
//
// ORDER MATTERS for the bespoke drum cases: the C drum renderers share
// advancing ma_noise state, so the browser must replay the full render
// sequence (instruments above, then these cases, in JSON order) on a single
// module instance to stay aligned.
func renderParamCases() []ParamCaseSample {
	const sampleRate = 48000
	const frames = sampleRate / 2 // fixed 0.5s — BPM-independent

	cases := []struct {
		name   string
		recipe string
		params map[string]float64
	}{
		// Bespoke drum knobs (7-field synth_params block).
		{"snare-decay-4", "drum-snare", map[string]float64{"decay": 4}},
		{"snare-pitch-tone-drive", "drum-snare", map[string]float64{"pitch": 12, "tone": 1, "drive": 0.5}},
		{"kick-decay-short-body", "drum-kick", map[string]float64{"decay": 0.25, "body": 1}},
		// Modular voice: every generator type (33-field modular_params block).
		{"modular-identity", "synth-modular", map[string]float64{}},
		{"modular-saw", "synth-modular", map[string]float64{"osc_type": 1}},
		{"modular-square", "synth-modular", map[string]float64{"osc_type": 2}},
		{"modular-triangle", "synth-modular", map[string]float64{"osc_type": 3}},
		{"modular-fm-stack", "synth-modular", map[string]float64{
			"osc_type": 4, "fm_algorithm": 3,
			"fm_op1_depth": 3, "fm_op2_depth": 3, "fm_op3_depth": 3, "fm_op4_depth": 3,
			"fm_op1_level": 0.8, "fm_op2_level": 0.8, "fm_op3_level": 0.8, "fm_op4_level": 0.8,
		}},
		{"modular-noise-white", "synth-modular", map[string]float64{"osc_type": 5}},
		{"modular-noise-pink", "synth-modular", map[string]float64{"osc_type": 6}},
		// Per-stage enable toggles (the Synth-tab pills).
		{"modular-env-off", "synth-modular", map[string]float64{"osc_type": 1, "env_enabled": 0}},
		{"modular-filter-off-no-authority", "synth-modular", map[string]float64{"osc_type": 1, "filter_enabled": 0, "filter_cutoff": 300}},
		{"modular-drive-on", "synth-modular", map[string]float64{"drive": 0.8}},
		{"modular-osc-off-silence", "synth-modular", map[string]float64{"osc_enabled": 0}},
		{"modular-combo-env-off-drive-on", "synth-modular", map[string]float64{
			"osc_type": 1, "env_enabled": 0, "drive": 0.6, "filter_cutoff": 1200,
		}},
		// FM family knobs. Cases stay LEGACY-named (fm_base_freq, fm_op2_depth,
		// fm_wave, …); the FM family migrated to the modular engine (Phase-7, the
		// LAST family), so the export translates them to the modular block
		// (paramBlock:'modular') via audio.ModularPushParams for the browser, while
		// the Go render goes through the modular binding (fmRecipeToModular). Same
		// param sweeps as before the migration — coverage preserved. APPENDED after
		// the existing cases — replay order is part of the cross-platform contract
		// (see ORDER MATTERS above).
		{"fm-bass-identity", "fm-bass", map[string]float64{}},
		{"fm-bass-op2-depth", "fm-bass", map[string]float64{"fm_op2_depth": 5}},
		{"fm-bass-base-freq", "fm-bass", map[string]float64{"fm_base_freq": 110}},
		{"fm-bell-ratio-decay", "fm-bell", map[string]float64{"fm_op2_ratio": 2, "fm_op1_decay": 0.4}},
		{"fm-lead-pitch-env", "fm-lead", map[string]float64{"fm_pitch_env_amount": 10, "fm_pitch_env_decay": 0.2}},
		{"fm-epiano-op-decays", "fm-epiano", map[string]float64{"fm_op1_decay": 0.2, "fm_op3_decay": 0.1}},
		{"fm-pluck-combo", "fm-pluck", map[string]float64{"fm_op2_depth": 8, "fm_base_freq": 392, "decay": 0.5}},
		// Kick family knobs. Cases stay LEGACY-named (kick_h2_gain, kick_wave,
		// fundamental, …); the kick family migrated to the modular engine
		// (Phase-3), so the export translates them to the modular block
		// (paramBlock:'modular') via audio.ModularPushParams for the browser,
		// while the Go render goes through the modular binding (kickRecipeToModular).
		{"kick-identity", "drum-kick", map[string]float64{}},
		{"kick-harmonics", "drum-kick", map[string]float64{"kick_h2_gain": 0.9, "kick_h4_gain": 0.5}},
		{"kick-pitch-sweep", "drum-kick", map[string]float64{"kick_pitch_env_amount": 0.4, "kick_pitch_env_rate": 80}},
		{"kick-deep-thud", "drum-kick-deep", map[string]float64{"kick_noise": 0.6, "kick_env0_rate": 9}},
		{"kick-punchy-click", "drum-kick-punchy", map[string]float64{"kick_click": 1.0, "fundamental": 90}},
		{"kick-lofi-combo", "drum-kick-lofi", map[string]float64{"kick_h3_gain": 0.7, "decay": 0.5}},
		{"kick-tight-fund", "drum-kick-tight", map[string]float64{"fundamental": 85, "kick_env1_rate": 25}},
		// Snare family knobs (snare_params block).
		{"snare-identity", "drum-snare", map[string]float64{}},
		{"snare-wires-tune", "drum-snare", map[string]float64{"snare_wire_mix": 0.2, "snare_noise_tune": 1.8}},
		{"snare-body", "drum-snare", map[string]float64{"fundamental": 320, "snare_tone_mix": 1.0, "snare_tone_decay": 70}},
		{"rimshot-partials", "drum-snare-rimshot", map[string]float64{"snare_tone2_freq": 1500, "snare_attack": 0.2}},
		{"sidestick-wood", "drum-snare-sidestick", map[string]float64{"snare_tone_mix": 1.0, "snare_noise_decay": 50}},
		{"clap-burst", "drum-clap", map[string]float64{"snare_attack": 45, "snare_noise_mix": 0.5}},
		// Cymbal family knobs. Cases stay LEGACY-named (cym_tune, cym_env_tail,
		// cym_wave, …); the cymbal family migrated to the modular engine (Phase-6),
		// so the export translates them to the modular block (paramBlock:'modular')
		// via audio.ModularPushParams for the browser, while the Go render goes
		// through the modular binding (cymbalRecipeToModular). Same param sweeps as
		// before the migration — coverage preserved.
		{"hihat-identity", "drum-hihat", map[string]float64{}},
		{"hihat-tuned-tail", "drum-hihat", map[string]float64{"cym_tune": 0.7, "cym_env_tail": 10}},
		{"openhat-sizzle", "drum-open-hihat", map[string]float64{"cym_noise_mix": 0.2, "cym_env_fast": 40}},
		{"ride-wash", "drum-ride", map[string]float64{"cym_noise_mix": 0.9, "cym_tone_mix": 0.3}},
		{"crash-short", "drum-crash", map[string]float64{"cym_env_tail": 9, "cym_noise_decay": 25}},
		{"cowbell-tune-ring", "drum-cowbell", map[string]float64{"cym_tune": 0.7, "cym_env_tail": 25}},
		{"shaker-dark", "drum-shaker", map[string]float64{"cym_tune": 0.7, "cym_tone_mix": 0.1}},
		// Bass family knobs. Cases stay LEGACY-named (bass_sustain, bass_wave,
		// fundamental, …); renderParamCases translates them to the modular block
		// (paramBlock:'modular') via audio.ModularPushParams for the browser,
		// while the desktop reference renders through the modular binding. Same
		// param sweeps as before the Phase-2 migration — coverage preserved.
		{"bassgtr-identity", "drum-bass-guitar", map[string]float64{}},
		{"bassgtr-sustain-pluck", "drum-bass-guitar", map[string]float64{"bass_sustain": 0.95, "bass_pluck": 0.8}},
		{"subbass-overtone", "drum-sub-bass", map[string]float64{"bass_harmonic": 0.5, "fundamental": 80}},
		// Tom family knobs. Cases stay LEGACY-named (tom_sweep_rate, tom_ring_rate,
		// tom_wave, fundamental, …); renderParamCases translates them to the modular
		// block (paramBlock:'modular') via audio.ModularPushParams for the browser,
		// while the desktop reference renders through the modular binding. Same param
		// sweeps as before the Phase-4 migration — coverage preserved.
		{"tom-identity", "drum-tom", map[string]float64{}},
		{"tom-tuned-ring", "drum-tom", map[string]float64{"fundamental": 220, "tom_ring_rate": 8}},
		{"tom-high-stick", "drum-tom-high", map[string]float64{"tom_stick": 0.9, "tom_o1_gain": 1.0}},
		{"tom-low-room", "drum-tom-low", map[string]float64{"tom_room": 0.5, "tom_sweep_rate": 45}},
		// Generator-waveform knobs (<family>_wave; default elided to NaN,
		// explicit values reach the osc_wave / fm wavetable paths).
		{"kick-wave-square", "drum-kick", map[string]float64{"kick_wave": 2}},
		{"kick-wave-explicit-sine", "drum-kick", map[string]float64{"kick_wave": 0, "kick_click": 0.9}},
		{"tom-wave-saw", "drum-tom", map[string]float64{"tom_wave": 1}},
		{"snare-wave-triangle", "drum-snare", map[string]float64{"snare_wave": 3}},
		{"hihat-wave-sine", "drum-hihat", map[string]float64{"cym_wave": 0}},
		{"ride-wave-square", "drum-ride", map[string]float64{"cym_wave": 2}},
		{"subbass-wave-triangle", "drum-sub-bass", map[string]float64{"bass_wave": 3}},
		{"fm-bass-wave-saw", "fm-bass", map[string]float64{"fm_wave": 1}},
		{"fm-bell-wave-square", "fm-bell", map[string]float64{"fm_wave": 2}},
		// Modular FM operator routing: algorithms 1 (parallel carriers) and
		// 2 (3-op chain) were desktop-covered (modular_fm_algorithms_native_
		// test.go) but had no cross-platform case — only the default stack
		// (0) and the 4-op stack (3) above. APPENDED (order contract).
		{"modular-fm-parallel", "synth-modular", map[string]float64{
			"osc_type": 4, "fm_algorithm": 1,
			"fm_op1_level": 1, "fm_op2_level": 0.6, "fm_op3_level": 0.5, "fm_op4_level": 0.4,
			"fm_op2_depth": 2, "fm_op3_depth": 1.5,
		}},
		{"modular-fm-chain3", "synth-modular", map[string]float64{
			"osc_type": 4, "fm_algorithm": 2,
			"fm_op2_depth": 2, "fm_op3_depth": 1.5,
		}},
		// Pad preset cross-platform lock. The params spell out the
		// modularPadSeed values explicitly: the Go side merges the recipe's
		// seeded defaults (MergeRecipeDefaults), the JS side overlays params
		// on the BASE ABI identity — explicit values are the only shape both
		// sides agree on, and any Go-side seed drift shows up as a mismatch.
		{"modular-pad-seeded", "synth-modular-pad", map[string]float64{
			"filter_cutoff": 1200, "filter_resonance": 1.4,
			"amp_attack": 0.12, "amp_decay": 0.8, "amp_sustain": 0.85, "amp_release": 1.4,
			"gain": 0.9,
		}},
	}

	// Append the SHARED cross-platform parity matrix (audio.ParityMatrix): one
	// flip per modular synth stage + a combined case. This is the same matrix
	// the Go export/import byte-parity suite uses, so both suites exercise
	// identical edits. APPENDED after the existing cases — replay order is part
	// of the cross-platform contract (see ORDER MATTERS above). Notably this is
	// the only cross-platform coverage of the Phase-8C stages (PITCH_ENV / LFO /
	// BURST), which the hand-written cases above never exercised.
	for _, pc := range audio.ParityMatrix() {
		params := make(map[string]float64, len(pc.Tweak))
		for k, v := range pc.Tweak {
			params[k] = v
		}
		cases = append(cases, struct {
			name   string
			recipe string
			params map[string]float64
		}{"matrix-" + pc.Name, pc.Recipe, params})
	}

	out := make([]ParamCaseSample, 0, len(cases))
	for _, c := range cases {
		r := audio.NewRecipe(c.recipe)
		if r == nil {
			fmt.Fprintf(os.Stderr, "Warning: no recipe %q for param case %q\n", c.recipe, c.name)
			continue
		}
		merged := audio.MergeRecipeDefaults(c.recipe, audio.RecipeParams(c.params))
		buf := make([]float32, frames)
		r.Render(buf, sampleRate, frames, 0, merged)
		samples := make([]float64, frames)
		for i, v := range buf {
			samples[i] = float64(v)
		}
		// Browser-side params + block. Migrated bass families render via the
		// modular engine on BOTH platforms; the browser fills the modular block,
		// so emit the binding-translated modular-named params (the same block the
		// production WASM push sends) and flag paramBlock:'modular'.
		emitParams := c.params
		paramBlock := ""
		if pushed, ok := audio.ModularPushParams(c.recipe, merged); ok {
			emitParams = pushed
			paramBlock = "modular"
		} else if strings.HasPrefix(c.recipe, "synth-modular") {
			// Native modular recipes (synth-modular / synth-modular-pad) render
			// through the modular block directly. Emit the full MERGED effective
			// params (recipe defaults ⊕ overlay), not just the overlay: the
			// browser's real playback path merges the overlay over the recipe's
			// SEEDED defaults (instrumentDefaultParams), so to compare effective-
			// vs-effective the reference must carry the recipe-default values too.
			// Without this, default-OFF stages with audible defaults (PITCH_ENV /
			// LFO / BURST, whose recipe default ≠ schema identity) diverge: desktop
			// uses the audible default (e.g. pitchenv_decay=0.08) while the browser
			// would fall back to the schema identity. (That divergence IS the
			// modulator-stage parity bug the matrix cases surface.)
			emitParams = map[string]float64(merged)
			paramBlock = "modular"
		}
		out = append(out, ParamCaseSample{
			Name:       c.name,
			Recipe:     c.recipe,
			Params:     emitParams,
			ParamBlock: paramBlock,
			SampleRate: sampleRate,
			Frames:     frames,
			Samples:    samples,
			Peak:       computePeak(samples),
			RMS:        computeRMS(samples),
		})
	}
	return out
}

func computePeak(data []float64) float64 {
	var peak float64
	for _, v := range data {
		a := v
		if a < 0 {
			a = -a
		}
		if a > peak {
			peak = a
		}
	}
	return peak
}

func computeRMS(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	var sum float64
	for _, v := range data {
		sum += v * v
	}
	return sqrt(sum / float64(len(data)))
}

func computeDCOffset(data []float64) float64 {
	if len(data) == 0 {
		return 0
	}
	var sum float64
	for _, v := range data {
		sum += v
	}
	return sum / float64(len(data))
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	// Newton-Raphson
	z := x / 2
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}
