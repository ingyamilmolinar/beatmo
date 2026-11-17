//go:build !test && !js

// export_audio is a tool that renders desktop audio samples and exports them
// to JSON for cross-platform comparison with WASM output.
//
// Usage (the xplat browser tests run the first form themselves on every run via
// loadDesktopAudioReference — the output is a reference rendered from the
// current tree, never a snapshot to commit):
//
//	cd src/go && go run ./cmd/export_audio
//	cd src/go && go run ./cmd/export_audio --mixed > ../js/desktop_mixed_audio.json
//	cd src/go && go run ./cmd/export_audio --voices 48000
//	cd src/go && go run ./cmd/export_audio --stages   (xplat_audio_stages)
package main

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
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
	// Block is the instrument's flattened param block — the exact input of
	// the C render above, so the browser renders the same voice.
	Block []float64 `json:"block"`
}

// ParamCaseSample is one parameterized instrument render: the desktop
// reference for an (instrument, field edits) pair, captured RAW from the C
// render of the edited document's flattened block — no normalization or
// amplitude scaling — so the browser can render the same block (Params, every
// ABI field) and compare sample-by-sample. Covers knob edits, per-stage enable
// toggles, family knobs, and every modular generator type.
type ParamCaseSample struct {
	Name       string             `json:"name"`
	Instrument instrument.ID      `json:"instrument"`
	Params     map[string]float64 `json:"params"`
	// ParamBlock names the param block the browser fills ("modular" —
	// render_modular_p, the only render entry point).
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
	for i, arg := range os.Args[1:] {
		if arg == "--stages" {
			exportStages()
			return
		}
		if arg == "--voices" && i+2 < len(os.Args) {
			sr, err := strconv.Atoi(os.Args[i+2])
			if err != nil || sr <= 0 {
				fmt.Fprintln(os.Stderr, "--voices needs a sample rate")
				os.Exit(2)
			}
			exportVoices(sr)
			return
		}
	}

	// Default: export audio samples
	exportAudioSamples()
}

func exportAudioSamples() {
	// Initialize audio system
	audio.Reset()
	audio.ResetInstruments()

	data := ExportData{
		Version:     1,
		Description: "Desktop Go audio samples for cross-platform comparison",
	}

	// Define instruments to export. We don't specify duration here: the
	// voice is its document's render.seconds long on both platforms.
	// EVERY legacy family migrated to the modular engine: kick (Phase-3), tom
	// (Phase-4), snare/clap (Phase-5), cymbal (Phase-6), bass (Phase-2), and FM
	// (Phase-7, the LAST) — no bespoke C renderer remains for the raw-render
	// harness. Their parity is covered by the kick-* / tom-* / snare-* / hihat-* /
	// fm-* … paramCases (paramBlock:'modular'). Only the unified modular voice's
	// base preset renders raw here.
	instruments := []instrument.ID{
		// Unified modular voice (base preset). Its no-edit render is the
		// identity block through render_modular_p on both platforms, compared
		// identity-vs-identity in xplat_audio_compare. (The pad preset
		// renders through a param block; its parity is covered end-to-end by
		// modular_synth_audible.browser.test.js, not this raw-render harness.)
		roster.Modular,
	}

	const sampleRate = 48000 // Match the new desktop sample rate

	for _, instID := range instruments {
		// Get a voice for this instrument which includes full processing pipeline
		// (C rendering + normalization + amplitude scaling)
		voice := audio.ExportVoice(instID, sampleRate)
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
			Name:       string(instID),
			SampleRate: sampleRate,
			Frames:     len(samples),
			Amplitude:  audio.AmplitudeForInstrumentExport(instID),
			Duration:   duration,
			Samples:    samples,
			Peak:       peak,
			RMS:        rms,
			DCOffset:   dc,
			Block:      instrumentBlock(instID),
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
// Semantics mirror the production path exactly: each case's fields are set
// through the instrument's document (instrument.SetField — what a Synth-tab
// edit does) and the flattened block is rendered RAW through the C engine. The
// browser side (xplat_audio_compare.browser.test.js) rebuilds the same block
// from the emitted Params and renders it through render_modular_p, so a stale
// ABI, a dropped field or a differing C build fails the comparison.
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
		inst   instrument.ID
		params map[string]float64
	}{
		// POST stage (the generic pitch/decay/tone/drive knobs every recipe
		// carries), forced on so the stage is exercised.
		{"snare-post-decay-4", roster.Snare, map[string]float64{"post_decay": 4}},
		{"snare-post-pitch-tone-drive", roster.Snare, map[string]float64{"post_pitch": 12, "post_darken": 0.5, "post_drive": 0.5}},
		{"kick-post-decay-short-body", roster.Kick, map[string]float64{"post_decay": 0.25, "post_body": 1}},
		// Modular voice: every generator type (33-field modular_params block).
		{"modular-identity", roster.Modular, map[string]float64{}},
		{"modular-saw", roster.Modular, map[string]float64{"osc_type": 1}},
		{"modular-square", roster.Modular, map[string]float64{"osc_type": 2}},
		{"modular-triangle", roster.Modular, map[string]float64{"osc_type": 3}},
		{"modular-fm-stack", roster.Modular, map[string]float64{
			"osc_type": 4, "fm_algorithm": 3,
			"fm_op1_depth": 3, "fm_op2_depth": 3, "fm_op3_depth": 3, "fm_op4_depth": 3,
			"fm_op1_level": 0.8, "fm_op2_level": 0.8, "fm_op3_level": 0.8, "fm_op4_level": 0.8,
		}},
		{"modular-noise-white", roster.Modular, map[string]float64{"osc_type": 5}},
		{"modular-noise-pink", roster.Modular, map[string]float64{"osc_type": 6}},
		// Per-stage enable toggles (the Synth-tab pills).
		{"modular-env-off", roster.Modular, map[string]float64{"osc_type": 1, "env_enabled": 0}},
		{"modular-filter-off-no-authority", roster.Modular, map[string]float64{"osc_type": 1, "filter_enabled": 0, "filter_cutoff": 300}},
		{"modular-drive-on", roster.Modular, map[string]float64{"drive": 0.8}},
		{"modular-osc-off-silence", roster.Modular, map[string]float64{"osc_enabled": 0}},
		{"modular-combo-env-off-drive-on", roster.Modular, map[string]float64{
			"osc_type": 1, "env_enabled": 0, "drive": 0.6, "filter_cutoff": 1200,
		}},
		// Generator-slot family knobs (gen1_<family>_*): every shipped instrument
		// is a config-first modular recipe, so these sweep the same family
		// voices the legacy recipes used to, through the modular block only.
		// APPENDED after the existing cases — replay order is part of the
		// cross-platform contract (see ORDER MATTERS above).
		{"fm-bass-identity", roster.FmBass, map[string]float64{}},
		{"fm-bass-op2-depth", roster.FmBass, map[string]float64{"gen1_fm_d2": 5}},
		{"fm-bass-base-freq", roster.FmBass, map[string]float64{"gen1_fm_base": 110}},
		{"fm-bell-ratio-decay", roster.FmBell, map[string]float64{"gen1_fm_r2": 2, "gen1_fm_dec1": 0.4}},
		{"fm-lead-pitch-env", roster.FmLead, map[string]float64{"gen1_fm_pe_amt": 10, "gen1_fm_pe_decay": 0.2}},
		{"fm-epiano-op-decays", roster.FmEpiano, map[string]float64{"gen1_fm_dec1": 0.2, "gen1_fm_dec3": 0.1}},
		{"fm-pluck-combo", roster.FmPluck, map[string]float64{"gen1_fm_d2": 8, "gen1_fm_base": 392, "amp_decay": 0.5}},
		{"kick-identity", roster.Kick, map[string]float64{}},
		{"kick-harmonics", roster.Kick, map[string]float64{"gen1_kick_h2": 0.9, "gen1_kick_h4": 0.5}},
		{"kick-pitch-sweep", roster.Kick, map[string]float64{"gen1_kick_pe_amt": 0.4, "gen1_kick_pe_rate": 80}},
		{"kick-deep-thud", roster.KickDeep, map[string]float64{"gen1_kick_noise": 0.6, "gen1_kick_env0": 9}},
		{"kick-snap-click", roster.KickSnap, map[string]float64{"gen1_kick_click": 1.0, "gen1_freq": 90}},
		{"kick-lofi-combo", roster.KickLofi, map[string]float64{"gen1_kick_h3": 0.7, "amp_decay": 0.5}},
		{"kick-tight-fund", roster.KickTight, map[string]float64{"gen1_freq": 85, "gen1_kick_env1": 25}},
		{"snare-identity", roster.Snare, map[string]float64{}},
		{"snare-wires-tune", roster.Snare, map[string]float64{"gen1_snare_wire_m": 0.2, "gen1_snare_tune": 1.8}},
		{"snare-body", roster.Snare, map[string]float64{"gen1_freq": 320, "gen1_snare_tone_m": 1.0, "gen1_snare_tone_d": 70}},
		{"rimshot-partials", roster.Rimshot, map[string]float64{"gen1_snare_tone2": 1500, "gen1_snare_attack": 0.2}},
		{"sidestick-wood", roster.Sidestick, map[string]float64{"gen1_snare_tone_m": 1.0, "gen1_snare_noise_d": 50}},
		{"clap-burst", roster.Clap, map[string]float64{"gen1_snare_attack": 45, "gen1_snare_noise_m": 0.5}},
		{"hihat-identity", roster.Hihat, map[string]float64{}},
		{"hihat-tuned-tail", roster.Hihat, map[string]float64{"gen1_cym_tune": 0.7, "gen1_cym_env_tail": 10}},
		{"openhat-sizzle", roster.HihatOpenBright, map[string]float64{"gen1_cym_noise_m": 0.2, "gen1_cym_env_fast": 40}},
		{"ride-wash", roster.Ride, map[string]float64{"gen1_cym_noise_m": 0.9, "gen1_cym_tone_m": 0.3}},
		{"crash-short", roster.Crash, map[string]float64{"gen1_cym_env_tail": 9, "gen1_cym_noise_d": 25}},
		{"cowbell-tune-ring", roster.Cowbell, map[string]float64{"gen1_cym_tune": 0.7, "gen1_cym_env_tail": 25}},
		{"shaker-dark", roster.Shaker, map[string]float64{"gen1_cym_tune": 0.7, "gen1_cym_tone_m": 0.1}},
		{"subbass-overtone", roster.SubBass, map[string]float64{"gen1_harm_mix": 0.5, "voice_freq_hz": 80}},
		{"tom-identity", roster.Tom, map[string]float64{}},
		{"tom-tuned-ring", roster.Tom, map[string]float64{"gen1_freq": 220, "gen1_tom_ring": 8}},
		{"tom-high-stick", roster.TomHighDrive, map[string]float64{"gen1_tom_stick": 0.9, "gen1_tom_o1": 1.0}},
		{"tom-low-room", roster.TomLofi, map[string]float64{"gen1_tom_room": 0.5, "gen1_tom_sweep": 45}},
		// Generator waveform (gen1_wave, read by every non-modal C voice family;
		// the modal cymbal plate has no waveform).
		{"kick-wave-square", roster.Kick, map[string]float64{"gen1_wave": 2}},
		{"kick-wave-explicit-sine", roster.Kick, map[string]float64{"gen1_wave": 0, "gen1_kick_click": 0.9}},
		{"tom-wave-saw", roster.Tom, map[string]float64{"gen1_wave": 1}},
		{"snare-wave-triangle", roster.Snare, map[string]float64{"gen1_wave": 3}},
		{"subbass-wave-triangle", roster.SubBass, map[string]float64{"gen1_wave": 3}},
		{"fm-bass-wave-saw", roster.FmBass, map[string]float64{"gen1_wave": 1}},
		{"fm-bell-wave-square", roster.FmBell, map[string]float64{"gen1_wave": 2}},
		// Modular FM operator routing: algorithms 1 (parallel carriers) and
		// 2 (3-op chain) were desktop-covered (modular_fm_algorithms_native_
		// test.go) but had no cross-platform case — only the default stack
		// (0) and the 4-op stack (3) above. APPENDED (order contract).
		{"modular-fm-parallel", roster.Modular, map[string]float64{
			"osc_type": 4, "fm_algorithm": 1,
			"fm_op1_level": 1, "fm_op2_level": 0.6, "fm_op3_level": 0.5, "fm_op4_level": 0.4,
			"fm_op2_depth": 2, "fm_op3_depth": 1.5,
		}},
		{"modular-fm-chain3", roster.Modular, map[string]float64{
			"osc_type": 4, "fm_algorithm": 2,
			"fm_op2_depth": 2, "fm_op3_depth": 1.5,
		}},
		// Pad preset cross-platform lock: explicit values set through the
		// modular-pad document.
		{"modular-pad-seeded", roster.ModularPad, map[string]float64{
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
			inst   instrument.ID
			params map[string]float64
		}{"matrix-" + pc.Name, pc.InstrumentID, params})
	}

	out := make([]ParamCaseSample, 0, len(cases))
	for _, c := range cases {
		d, err := audio.InstrumentDocWithFields(c.inst, c.params)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: param case %q: %v\n", c.name, err)
			continue
		}
		buf, err := audio.RenderInstrumentWithFields(c.inst, sampleRate, frames, c.params)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: param case %q: %v\n", c.name, err)
			continue
		}
		samples := make([]float64, frames)
		for i, v := range buf {
			samples[i] = float64(v)
		}
		// Emit the case's COMPLETE flattened block (every ABI field, by name):
		// the browser rebuilds the block from this map alone and renders it
		// through render_modular_p, so both platforms render the same block.
		emitParams := audio.FlatFieldMap(d)
		paramBlock := "modular"
		out = append(out, ParamCaseSample{
			Name:       c.name,
			Instrument: c.inst,
			Params:     emitParams,
			ParamBlock: paramBlock,
			SampleRate: sampleRate,
			Frames:     frames,
			Samples:    samples,
			Peak:       computePeak(samples),
			RMS:        computeRMS(samples),
		})
	}
	// Document cases (tables cannot be set field by field): APPENDED last —
	// replay order is part of the cross-platform contract.
	for _, dc := range docCases() {
		buf := audio.RenderDocRaw(dc.doc, sampleRate, frames)
		samples := make([]float64, frames)
		for i, v := range buf {
			samples[i] = float64(v)
		}
		out = append(out, ParamCaseSample{
			Name: dc.name, Instrument: dc.doc.ID, Params: audio.FlatFieldMap(dc.doc), ParamBlock: "modular",
			SampleRate: sampleRate, Frames: frames, Samples: samples, Peak: computePeak(samples), RMS: computeRMS(samples),
		})
	}
	return out
}

type docCase struct {
	name string
	doc  instrument.Doc
}

// docCases are documents built here because their TABLE rows have no field
// path: the WAV-clone MODAL (ratio rows, a stretch and tune), STRIKE (contact,
// hardness, click — seeded noise) and NOISEBAND (two rows, seeded noise)
// stages on the bare modular voice.
func docCases() []docCase {
	reg := audio.BuiltinModuleRegistry()
	d, ok := audio.ShippedDoc(roster.Modular)
	if !ok {
		fmt.Fprintln(os.Stderr, "Warning: no shipped modular document; clone-stage case skipped")
		return nil
	}
	d = d.Clone()
	d.ID = "clone-stages"
	var err error
	for _, typ := range []string{"modal", "strike", "noiseband"} {
		if _, has := d.Stages[typ]; !has {
			if d, err = instrument.AddStage(d, reg, typ, true); err != nil {
				fmt.Fprintf(os.Stderr, "Warning: clone-stage case: %v\n", err)
				return nil
			}
		}
	}
	if d.Source != nil {
		src := *d.Source
		src.On = false // the modal bank alone carries the tone
		d.Source = &src
	}
	mv := d.Stages["modal"].Values
	mv["modal_tune"], mv["modal_stretch"], mv["modal_law_a"], mv["modal_law_p"] = 7, 0.01, 1.5, 0.8
	sv := d.Stages["strike"].Values
	sv["strike_tc"], sv["strike_hardness"], sv["strike_click"] = 0.0008, -2, 0.3
	mt := instrument.Table{Cols: []string{"freq", "decay", "amp", "phase", "attack"}, Rows: [][]float64{
		{1, 1, 0.4, 0.3, 0}, {2.76, 1.2, 0.2, -1.1, 0.002}, {5.4, 0.9, 0.12, 2, 0}, {8.93, 1.1, 0.07, 0.7, 0.001},
	}}
	nt := instrument.Table{Cols: []string{"center", "q", "level", "attack", "fast_rate", "fast_mix", "tail_rate"}, Rows: [][]float64{
		{2000, 4.3, -30, 0.002, 80, 0.7, 9}, {6300, 4.3, -36, 0.001, 200, 0.9, 30},
	}}
	if d, err = instrument.SetTable(d, reg, "modal", mt); err == nil {
		d, err = instrument.SetTable(d, reg, "noiseband", nt)
	}
	if err == nil {
		err = instrument.Validate(d, reg)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: clone-stage case: %v\n", err)
		return nil
	}
	return []docCase{{"clone-modal-strike-noiseband", d}}
}

// instrumentBlock is instID's document flattened to the engine's param block.
func instrumentBlock(instID instrument.ID) []float64 {
	d, ok := audio.EnsureInstrumentDoc(instID)
	if !ok {
		return nil
	}
	m := audio.FlatFieldMap(d)
	names := audio.ModularParamNames()
	b := make([]float64, len(names))
	for i, n := range names {
		b[i] = m[n]
	}
	return b
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

// PlayedVoice is one shipped instrument's voice exactly as a trigger at pitch
// 0 plays it on desktop (audio.RenderAsShipped: C render, voice FX, gate tail,
// normalise × amplitude, end fade), with the document's flattened block and
// render settings — what voice_tempo_independence.browser.test.js renders in
// the browser and compares against it.
type PlayedVoice struct {
	ID       instrument.ID    `json:"id"`
	Block    []float64        `json:"block"`
	Seconds  float64          `json:"seconds"`
	Amp      float64          `json:"amp"`
	GateTail float64          `json:"gateTail"`
	FX       []map[string]any `json:"fx"`
	PCM      string           `json:"pcm"` // base64, float32 little-endian
}

// exportVoices prints every shipped instrument's played voice at sr.
func exportVoices(sr int) {
	audio.Reset()
	out := struct {
		SampleRate int           `json:"sampleRate"`
		Voices     []PlayedVoice `json:"voices"`
	}{SampleRate: sr}
	for _, id := range roster.All {
		d, ok := audio.ShippedDoc(id)
		if !ok {
			fmt.Fprintf(os.Stderr, "%s: no shipped document\n", id)
			os.Exit(1)
		}
		pcm := audio.RenderAsShipped(d, sr, 0)
		raw := make([]byte, 4*len(pcm))
		for i, x := range pcm {
			binary.LittleEndian.PutUint32(raw[4*i:], math.Float32bits(float32(x)))
		}
		fx := []map[string]any{}
		for _, c := range audio.VoiceFXCalls(d) {
			fx = append(fx, map[string]any{"type": c.Type, "params": c.Params})
		}
		out.Voices = append(out.Voices, PlayedVoice{
			ID: id, Block: instrument.Flatten(d, audio.BuiltinModuleRegistry()),
			Seconds: d.Render.Seconds, Amp: d.Render.Amplitude, GateTail: d.Render.GateTail,
			FX: fx, PCM: base64.StdEncoding.EncodeToString(raw),
		})
	}
	if err := json.NewEncoder(os.Stdout).Encode(out); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
