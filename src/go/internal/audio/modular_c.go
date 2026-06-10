//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "modular.h"
#include "noise.h"
void oracle_ma_noise_white_fill(float *out, int n, int seed);
*/
import "C"
import (
	"fmt"
	"unsafe"
)

// ModularParams mirrors the C modular_params struct (src/c/modular.h). It is
// the wide param block for the unified modular synth voice — distinct from the
// 7-field SynthParams used by the legacy bespoke renderers. Field order is the
// canonical ABI order mirrored by modularParamSchema in synth_param_schema.go.
//
// Discrete choices (osc type, filter type, fm algorithm, amp curve) are carried
// as integer-valued float64 so the entire block flows through the existing
// float-keyed RecipeParams machinery (clamp, hash, merge, persist) unchanged.
type ModularParams struct {
	OscType   float64 // 0=sine 1=saw 2=square 3=triangle 4=FM 5=noise-white 6=noise-pink
	OscDetune float64 // cents
	OscOctave float64 // integer octave shift

	FMAlgorithm float64 // 0..3 operator routing
	FMOp1Ratio  float64
	FMOp2Ratio  float64
	FMOp3Ratio  float64
	FMOp4Ratio  float64
	FMOp1Depth  float64
	FMOp2Depth  float64
	FMOp3Depth  float64
	FMOp4Depth  float64
	FMOp1Level  float64
	FMOp2Level  float64
	FMOp3Level  float64
	FMOp4Level  float64

	AmpAttack  float64 // seconds
	AmpDecay   float64 // seconds
	AmpSustain float64 // 0..1
	AmpRelease float64 // seconds
	AmpCurve   float64 // 0=linear 1=exponential

	FilterType      float64 // 0=LP 1=HP 2=BP
	FilterCutoff    float64 // Hz
	FilterResonance float64 // Q

	Drive float64 // 0..1
	Pitch float64 // semitones
	Gain  float64 // output trim

	// Per-stage bypass toggles (1=enabled, default). Appended in C struct
	// order. NoiseSeed is engine-internal (set per round-robin variant).
	OscEnabled    float64 // 0 = silence the generator
	FMEnabled     float64 // osc_type==FM only; 0 = carrier-only
	EnvEnabled    float64 // 0 = constant unity gain
	FilterEnabled float64 // 0 = filter bypassed
	DriveEnabled  float64 // 0 = drive bypassed
	NoiseSeed     float64 // deterministic seed for osc_type 5/6

	// ── Phase-1 gen bank (append-only ABI; field-major [12] arrays). ──
	NoiseDraws   float64
	NoisePrelude float64

	GenSource      [12]float64
	GenWave        [12]float64
	GenFreqMode    [12]float64
	GenFreq        [12]float64
	GenGain        [12]float64
	GenEnvFastRate [12]float64
	GenEnvTailRate [12]float64
	GenEnvFastMix  [12]float64
	GenEnvTailMix  [12]float64
	GenFiltType    [12]float64
	GenFiltAlpha   [12]float64
	GenFiltFreq    [12]float64
	GenFiltQ       [12]float64
	GenPhaseMode   [12]float64
	GenPhase       [12]float64
	GenNoiseOffset [12]float64

	// ── Phase-2 (bass family) analytic-voice per-slot fields (source==4). ──
	GenPitchEnvAmt  [12]float64
	GenPitchEnvRate [12]float64
	GenHarmMix      [12]float64
	GenAtkAmt       [12]float64
	GenAtkRate      [12]float64
	GenSatK         [12]float64
	GenOutScale     [12]float64

	// ── Phase-2 globals: voice-freq override + shared POST stage. ──
	VoiceFreqHz      float64
	PostEnabled      float64
	PostPitch        float64
	PostDecay        float64
	PostDecayRate    float64
	PostBody         float64
	PostBrightness   float64
	PostTone         float64
	PostDrive        float64
	PostPitchOn      float64
	PostDecayOn      float64
	PostBodyOn       float64
	PostBrightnessOn float64
	PostToneOn       float64
	PostDriveOn      float64

	// ── Phase-2 Task-2 (Karplus-Strong, source==3) per-slot fields. Appended
	// at the very tail of the ABI (after the Phase-2 globals). ──
	GenKsSustain [12]float64
	GenKsPluck   [12]float64

	// ── Phase-3 (kick family, source==5) harmonic-bank kick voice per-slot
	// fields. Appended at the NEW very tail (after the Phase-2 KS columns). ──
	GenKickVariant [12]float64
	GenKickH2      [12]float64
	GenKickH3      [12]float64
	GenKickH4      [12]float64
	GenKickEnv0    [12]float64
	GenKickEnv1    [12]float64
	GenKickPeAmt   [12]float64
	GenKickPeRate  [12]float64
	GenKickClick   [12]float64
	GenKickNoise   [12]float64

	// ── Phase-3 (kick post-order fix): selects the POST-stage op ORDER. Appended
	// at the NEW very tail (after the kick voice columns) so every prior flat
	// index stays frozen. 0 = shared apply_post_params order; 1 = legacy base
	// kick (render_kick_p) order (drive before body). ──
	PostOrder float64

	// ── Phase-4 (tom family, source==6) 808-style tom voice per-slot fields.
	// Appended at the NEW very tail (after post_order) so every prior flat index
	// stays frozen. Identity 0 each; read only when a slot's source is 6.
	// GenTomVariant is a discriminator (0=tom 1=high 2=low). ──
	GenTomVariant [12]float64
	GenTomSweep   [12]float64
	GenTomRing    [12]float64
	GenTomO1      [12]float64
	GenTomO2      [12]float64
	GenTomStick   [12]float64
	GenTomRoom    [12]float64

	// ── Phase-5 (snare family): snare-ish voice (source==7, 3 variant branches)
	// + clap voice (source==8) per-slot fields. Appended at the NEW very tail
	// (after the Phase-4 tom columns) so every prior flat index stays frozen.
	// Identity 0 each; read only when a slot's source is 7 or 8. GenSnareVariant
	// is a discriminator (0=snare 1=rimshot 2=sidestick); the curated knobs are
	// NaN-driven at the binding so the C kp_get supplies the per-variant literal. ──
	GenSnareVariant [12]float64
	GenSnareTone2   [12]float64
	GenSnareTune    [12]float64
	GenSnareToneD   [12]float64
	GenSnareNoiseD  [12]float64
	GenSnareTailD   [12]float64
	GenSnareToneM   [12]float64
	GenSnareNoiseM  [12]float64
	GenSnareWireM   [12]float64
	GenSnareAttack  [12]float64

	// ── Phase-6 (cymbal family): metallic voice (source==9, 6 variant branches:
	// 0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash) per-slot fields.
	// Appended at the NEW very tail (after the Phase-5 snare columns) so every
	// prior flat index stays frozen. Identity 0 each; read only when a slot's
	// source is 9. GenCymVariant is a discriminator; the curated knobs are
	// NaN-driven at the binding so the C kp_get supplies the per-variant literal. ──
	GenCymVariant [12]float64
	GenCymTune    [12]float64
	GenCymEnvFast [12]float64
	GenCymEnvTail [12]float64
	GenCymToneM   [12]float64
	GenCymNoiseM  [12]float64
	GenCymNoiseD  [12]float64

	// ── Phase-7 (FM family, the LAST legacy family): 4-operator FM preset voice
	// (source==10, 5 variant branches: 0=bass 1=bell 2=lead 3=epiano 4=pluck)
	// per-slot fields. Appended at the NEW very tail (after the Phase-6 cymbal
	// columns) so every prior flat index stays frozen. Identity 0 each; read only
	// when a slot's source is 10. GenFMVariant is a discriminator; the curated
	// knobs are NaN-driven at the binding so the C kp_get supplies the per-variant
	// preset literal (the same overlay the deleted fm_apply_params performed). ──
	GenFMVariant [12]float64
	GenFMBase    [12]float64
	GenFMPeAmt   [12]float64
	GenFMPeDecay [12]float64
	GenFMR1      [12]float64
	GenFMR2      [12]float64
	GenFMR3      [12]float64
	GenFMR4      [12]float64
	GenFMD1      [12]float64
	GenFMD2      [12]float64
	GenFMD3      [12]float64
	GenFMD4      [12]float64
	GenFMDec1    [12]float64
	GenFMDec2    [12]float64
	GenFMDec3    [12]float64
	GenFMDec4    [12]float64

	// ── Phase-8C modulator stages (PITCH ENV / LFO / BURST — the spec-§1 gap
	// closure). Appended at the NEW very tail (after the Phase-7 FM columns) so
	// every prior flat index stays frozen. Identity 0 each: the enables default
	// DISABLED (new stages — default-off is the byte-identity polarity) and a
	// disabled stage never reads its numeric knobs (exact bypass). ──
	PitchEnvEnabled float64 // >=0.5 runs the OSC pitch envelope
	PitchEnvAmt     float64 // semitones of sweep
	PitchEnvDecay   float64 // exp time constant, seconds
	LfoEnabled      float64 // >=0.5 runs the amp-wobble LFO
	LfoRate         float64 // Hz
	LfoDepth        float64 // 0..1
	BurstEnabled    float64 // >=0.5 runs the burst gate
	BurstSharp      float64 // per-burst exp decay rate, s^-1
	Burst1Off       float64 // burst onsets, seconds
	Burst1Amp       float64 // burst peak amplitudes
	Burst2Off       float64
	Burst2Amp       float64
	Burst3Off       float64
	Burst3Amp       float64
	Burst4Off       float64
	Burst4Amp       float64
}

// toCModularParams converts the Go struct to a C modular_params pointer.
func (p ModularParams) toCModularParams() *C.modular_params {
	cp := &C.modular_params{
		osc_type:   C.float(p.OscType),
		osc_detune: C.float(p.OscDetune),
		osc_octave: C.float(p.OscOctave),

		fm_algorithm: C.float(p.FMAlgorithm),
		fm_op1_ratio: C.float(p.FMOp1Ratio),
		fm_op2_ratio: C.float(p.FMOp2Ratio),
		fm_op3_ratio: C.float(p.FMOp3Ratio),
		fm_op4_ratio: C.float(p.FMOp4Ratio),
		fm_op1_depth: C.float(p.FMOp1Depth),
		fm_op2_depth: C.float(p.FMOp2Depth),
		fm_op3_depth: C.float(p.FMOp3Depth),
		fm_op4_depth: C.float(p.FMOp4Depth),
		fm_op1_level: C.float(p.FMOp1Level),
		fm_op2_level: C.float(p.FMOp2Level),
		fm_op3_level: C.float(p.FMOp3Level),
		fm_op4_level: C.float(p.FMOp4Level),

		amp_attack:  C.float(p.AmpAttack),
		amp_decay:   C.float(p.AmpDecay),
		amp_sustain: C.float(p.AmpSustain),
		amp_release: C.float(p.AmpRelease),
		amp_curve:   C.float(p.AmpCurve),

		filter_type:      C.float(p.FilterType),
		filter_cutoff:    C.float(p.FilterCutoff),
		filter_resonance: C.float(p.FilterResonance),

		drive: C.float(p.Drive),
		pitch: C.float(p.Pitch),
		gain:  C.float(p.Gain),

		osc_enabled:    C.float(p.OscEnabled),
		fm_enabled:     C.float(p.FMEnabled),
		env_enabled:    C.float(p.EnvEnabled),
		filter_enabled: C.float(p.FilterEnabled),
		drive_enabled:  C.float(p.DriveEnabled),
		noise_seed:     C.float(p.NoiseSeed),

		noise_draws:   C.float(p.NoiseDraws),
		noise_prelude: C.float(p.NoisePrelude),

		voice_freq_hz:      C.float(p.VoiceFreqHz),
		post_enabled:       C.float(p.PostEnabled),
		post_pitch:         C.float(p.PostPitch),
		post_decay:         C.float(p.PostDecay),
		post_decay_rate:    C.float(p.PostDecayRate),
		post_body:          C.float(p.PostBody),
		post_brightness:    C.float(p.PostBrightness),
		post_tone:          C.float(p.PostTone),
		post_drive:         C.float(p.PostDrive),
		post_pitch_on:      C.float(p.PostPitchOn),
		post_decay_on:      C.float(p.PostDecayOn),
		post_body_on:       C.float(p.PostBodyOn),
		post_brightness_on: C.float(p.PostBrightnessOn),
		post_tone_on:       C.float(p.PostToneOn),
		post_drive_on:      C.float(p.PostDriveOn),
		post_order:         C.float(p.PostOrder),

		pitchenv_enabled: C.float(p.PitchEnvEnabled),
		pitchenv_amt:     C.float(p.PitchEnvAmt),
		pitchenv_decay:   C.float(p.PitchEnvDecay),
		lfo_enabled:      C.float(p.LfoEnabled),
		lfo_rate:         C.float(p.LfoRate),
		lfo_depth:        C.float(p.LfoDepth),
		burst_enabled:    C.float(p.BurstEnabled),
		burst_sharp:      C.float(p.BurstSharp),
		burst1_off:       C.float(p.Burst1Off),
		burst1_amp:       C.float(p.Burst1Amp),
		burst2_off:       C.float(p.Burst2Off),
		burst2_amp:       C.float(p.Burst2Amp),
		burst3_off:       C.float(p.Burst3Off),
		burst3_amp:       C.float(p.Burst3Amp),
		burst4_off:       C.float(p.Burst4Off),
		burst4_amp:       C.float(p.Burst4Amp),
	}
	// Go [12]float64 → C float gen_<field>[12] must be copied element-wise.
	for i := 0; i < 12; i++ {
		cp.gen_source[i] = C.float(p.GenSource[i])
		cp.gen_wave[i] = C.float(p.GenWave[i])
		cp.gen_freq_mode[i] = C.float(p.GenFreqMode[i])
		cp.gen_freq[i] = C.float(p.GenFreq[i])
		cp.gen_gain[i] = C.float(p.GenGain[i])
		cp.gen_env_fast_rate[i] = C.float(p.GenEnvFastRate[i])
		cp.gen_env_tail_rate[i] = C.float(p.GenEnvTailRate[i])
		cp.gen_env_fast_mix[i] = C.float(p.GenEnvFastMix[i])
		cp.gen_env_tail_mix[i] = C.float(p.GenEnvTailMix[i])
		cp.gen_filt_type[i] = C.float(p.GenFiltType[i])
		cp.gen_filt_alpha[i] = C.float(p.GenFiltAlpha[i])
		cp.gen_filt_freq[i] = C.float(p.GenFiltFreq[i])
		cp.gen_filt_q[i] = C.float(p.GenFiltQ[i])
		cp.gen_phase_mode[i] = C.float(p.GenPhaseMode[i])
		cp.gen_phase[i] = C.float(p.GenPhase[i])
		cp.gen_noise_offset[i] = C.float(p.GenNoiseOffset[i])

		cp.gen_pitch_env_amt[i] = C.float(p.GenPitchEnvAmt[i])
		cp.gen_pitch_env_rate[i] = C.float(p.GenPitchEnvRate[i])
		cp.gen_harm_mix[i] = C.float(p.GenHarmMix[i])
		cp.gen_atk_amt[i] = C.float(p.GenAtkAmt[i])
		cp.gen_atk_rate[i] = C.float(p.GenAtkRate[i])
		cp.gen_sat_k[i] = C.float(p.GenSatK[i])
		cp.gen_out_scale[i] = C.float(p.GenOutScale[i])

		cp.gen_ks_sustain[i] = C.float(p.GenKsSustain[i])
		cp.gen_ks_pluck[i] = C.float(p.GenKsPluck[i])

		cp.gen_kick_variant[i] = C.float(p.GenKickVariant[i])
		cp.gen_kick_h2[i] = C.float(p.GenKickH2[i])
		cp.gen_kick_h3[i] = C.float(p.GenKickH3[i])
		cp.gen_kick_h4[i] = C.float(p.GenKickH4[i])
		cp.gen_kick_env0[i] = C.float(p.GenKickEnv0[i])
		cp.gen_kick_env1[i] = C.float(p.GenKickEnv1[i])
		cp.gen_kick_pe_amt[i] = C.float(p.GenKickPeAmt[i])
		cp.gen_kick_pe_rate[i] = C.float(p.GenKickPeRate[i])
		cp.gen_kick_click[i] = C.float(p.GenKickClick[i])
		cp.gen_kick_noise[i] = C.float(p.GenKickNoise[i])

		cp.gen_tom_variant[i] = C.float(p.GenTomVariant[i])
		cp.gen_tom_sweep[i] = C.float(p.GenTomSweep[i])
		cp.gen_tom_ring[i] = C.float(p.GenTomRing[i])
		cp.gen_tom_o1[i] = C.float(p.GenTomO1[i])
		cp.gen_tom_o2[i] = C.float(p.GenTomO2[i])
		cp.gen_tom_stick[i] = C.float(p.GenTomStick[i])
		cp.gen_tom_room[i] = C.float(p.GenTomRoom[i])

		cp.gen_snare_variant[i] = C.float(p.GenSnareVariant[i])
		cp.gen_snare_tone2[i] = C.float(p.GenSnareTone2[i])
		cp.gen_snare_tune[i] = C.float(p.GenSnareTune[i])
		cp.gen_snare_tone_d[i] = C.float(p.GenSnareToneD[i])
		cp.gen_snare_noise_d[i] = C.float(p.GenSnareNoiseD[i])
		cp.gen_snare_tail_d[i] = C.float(p.GenSnareTailD[i])
		cp.gen_snare_tone_m[i] = C.float(p.GenSnareToneM[i])
		cp.gen_snare_noise_m[i] = C.float(p.GenSnareNoiseM[i])
		cp.gen_snare_wire_m[i] = C.float(p.GenSnareWireM[i])
		cp.gen_snare_attack[i] = C.float(p.GenSnareAttack[i])

		cp.gen_cym_variant[i] = C.float(p.GenCymVariant[i])
		cp.gen_cym_tune[i] = C.float(p.GenCymTune[i])
		cp.gen_cym_env_fast[i] = C.float(p.GenCymEnvFast[i])
		cp.gen_cym_env_tail[i] = C.float(p.GenCymEnvTail[i])
		cp.gen_cym_tone_m[i] = C.float(p.GenCymToneM[i])
		cp.gen_cym_noise_m[i] = C.float(p.GenCymNoiseM[i])
		cp.gen_cym_noise_d[i] = C.float(p.GenCymNoiseD[i])

		cp.gen_fm_variant[i] = C.float(p.GenFMVariant[i])
		cp.gen_fm_base[i] = C.float(p.GenFMBase[i])
		cp.gen_fm_pe_amt[i] = C.float(p.GenFMPeAmt[i])
		cp.gen_fm_pe_decay[i] = C.float(p.GenFMPeDecay[i])
		cp.gen_fm_r1[i] = C.float(p.GenFMR1[i])
		cp.gen_fm_r2[i] = C.float(p.GenFMR2[i])
		cp.gen_fm_r3[i] = C.float(p.GenFMR3[i])
		cp.gen_fm_r4[i] = C.float(p.GenFMR4[i])
		cp.gen_fm_d1[i] = C.float(p.GenFMD1[i])
		cp.gen_fm_d2[i] = C.float(p.GenFMD2[i])
		cp.gen_fm_d3[i] = C.float(p.GenFMD3[i])
		cp.gen_fm_d4[i] = C.float(p.GenFMD4[i])
		cp.gen_fm_dec1[i] = C.float(p.GenFMDec1[i])
		cp.gen_fm_dec2[i] = C.float(p.GenFMDec2[i])
		cp.gen_fm_dec3[i] = C.float(p.GenFMDec3[i])
		cp.gen_fm_dec4[i] = C.float(p.GenFMDec4[i])
	}
	return cp
}

// recipeParamsToModular builds a ModularParams from a (merged) RecipeParams
// map, reading each field by its canonical schema name and falling back to the
// built-in identity default for absent keys. This is the wide-block analogue of
// recipeParamsToSynth; the nativeModularRecipe renderer uses it so the modular
// voice flows through the existing voice-cache + persistence machinery.
func recipeParamsToModular(p RecipeParams) ModularParams {
	ident := ModularParamSchemaIdentity()
	get := func(name string) float64 {
		if v, ok := p[name]; ok {
			return v
		}
		return ident[name]
	}
	mp := ModularParams{
		OscType:   get("osc_type"),
		OscDetune: get("osc_detune"),
		OscOctave: get("osc_octave"),

		FMAlgorithm: get("fm_algorithm"),
		FMOp1Ratio:  get("fm_op1_ratio"),
		FMOp2Ratio:  get("fm_op2_ratio"),
		FMOp3Ratio:  get("fm_op3_ratio"),
		FMOp4Ratio:  get("fm_op4_ratio"),
		FMOp1Depth:  get("fm_op1_depth"),
		FMOp2Depth:  get("fm_op2_depth"),
		FMOp3Depth:  get("fm_op3_depth"),
		FMOp4Depth:  get("fm_op4_depth"),
		FMOp1Level:  get("fm_op1_level"),
		FMOp2Level:  get("fm_op2_level"),
		FMOp3Level:  get("fm_op3_level"),
		FMOp4Level:  get("fm_op4_level"),

		AmpAttack:  get("amp_attack"),
		AmpDecay:   get("amp_decay"),
		AmpSustain: get("amp_sustain"),
		AmpRelease: get("amp_release"),
		AmpCurve:   get("amp_curve"),

		FilterType:      get("filter_type"),
		FilterCutoff:    get("filter_cutoff"),
		FilterResonance: get("filter_resonance"),

		Drive: get("drive"),
		Pitch: get("pitch"),
		Gain:  get("gain"),

		OscEnabled:    get("osc_enabled"),
		FMEnabled:     get("fm_enabled"),
		EnvEnabled:    get("env_enabled"),
		FilterEnabled: get("filter_enabled"),
		DriveEnabled:  get("drive_enabled"),
		NoiseSeed:     get("noise_seed"),

		NoiseDraws:   get("noise_draws"),
		NoisePrelude: get("noise_prelude"),

		VoiceFreqHz:      get("voice_freq_hz"),
		PostEnabled:      get("post_enabled"),
		PostPitch:        get("post_pitch"),
		PostDecay:        get("post_decay"),
		PostDecayRate:    get("post_decay_rate"),
		PostBody:         get("post_body"),
		PostBrightness:   get("post_brightness"),
		PostTone:         get("post_tone"),
		PostDrive:        get("post_drive"),
		PostPitchOn:      get("post_pitch_on"),
		PostDecayOn:      get("post_decay_on"),
		PostBodyOn:       get("post_body_on"),
		PostBrightnessOn: get("post_brightness_on"),
		PostToneOn:       get("post_tone_on"),
		PostDriveOn:      get("post_drive_on"),
		PostOrder:        get("post_order"),

		PitchEnvEnabled: get("pitchenv_enabled"),
		PitchEnvAmt:     get("pitchenv_amt"),
		PitchEnvDecay:   get("pitchenv_decay"),
		LfoEnabled:      get("lfo_enabled"),
		LfoRate:         get("lfo_rate"),
		LfoDepth:        get("lfo_depth"),
		BurstEnabled:    get("burst_enabled"),
		BurstSharp:      get("burst_sharp"),
		Burst1Off:       get("burst1_off"),
		Burst1Amp:       get("burst1_amp"),
		Burst2Off:       get("burst2_off"),
		Burst2Amp:       get("burst2_amp"),
		Burst3Off:       get("burst3_off"),
		Burst3Amp:       get("burst3_amp"),
		Burst4Off:       get("burst4_off"),
		Burst4Amp:       get("burst4_amp"),
	}
	for i := 0; i < 12; i++ {
		k := i + 1
		mp.GenSource[i] = get(fmt.Sprintf("gen%d_source", k))
		mp.GenWave[i] = get(fmt.Sprintf("gen%d_wave", k))
		mp.GenFreqMode[i] = get(fmt.Sprintf("gen%d_freq_mode", k))
		mp.GenFreq[i] = get(fmt.Sprintf("gen%d_freq", k))
		mp.GenGain[i] = get(fmt.Sprintf("gen%d_gain", k))
		mp.GenEnvFastRate[i] = get(fmt.Sprintf("gen%d_env_fast_rate", k))
		mp.GenEnvTailRate[i] = get(fmt.Sprintf("gen%d_env_tail_rate", k))
		mp.GenEnvFastMix[i] = get(fmt.Sprintf("gen%d_env_fast_mix", k))
		mp.GenEnvTailMix[i] = get(fmt.Sprintf("gen%d_env_tail_mix", k))
		mp.GenFiltType[i] = get(fmt.Sprintf("gen%d_filt_type", k))
		mp.GenFiltAlpha[i] = get(fmt.Sprintf("gen%d_filt_alpha", k))
		mp.GenFiltFreq[i] = get(fmt.Sprintf("gen%d_filt_freq", k))
		mp.GenFiltQ[i] = get(fmt.Sprintf("gen%d_filt_q", k))
		mp.GenPhaseMode[i] = get(fmt.Sprintf("gen%d_phase_mode", k))
		mp.GenPhase[i] = get(fmt.Sprintf("gen%d_phase", k))
		mp.GenNoiseOffset[i] = get(fmt.Sprintf("gen%d_noise_offset", k))

		mp.GenPitchEnvAmt[i] = get(fmt.Sprintf("gen%d_pitch_env_amt", k))
		mp.GenPitchEnvRate[i] = get(fmt.Sprintf("gen%d_pitch_env_rate", k))
		mp.GenHarmMix[i] = get(fmt.Sprintf("gen%d_harm_mix", k))
		mp.GenAtkAmt[i] = get(fmt.Sprintf("gen%d_atk_amt", k))
		mp.GenAtkRate[i] = get(fmt.Sprintf("gen%d_atk_rate", k))
		mp.GenSatK[i] = get(fmt.Sprintf("gen%d_sat_k", k))
		mp.GenOutScale[i] = get(fmt.Sprintf("gen%d_out_scale", k))

		mp.GenKsSustain[i] = get(fmt.Sprintf("gen%d_ks_sustain", k))
		mp.GenKsPluck[i] = get(fmt.Sprintf("gen%d_ks_pluck", k))

		mp.GenKickVariant[i] = get(fmt.Sprintf("gen%d_kick_variant", k))
		mp.GenKickH2[i] = get(fmt.Sprintf("gen%d_kick_h2", k))
		mp.GenKickH3[i] = get(fmt.Sprintf("gen%d_kick_h3", k))
		mp.GenKickH4[i] = get(fmt.Sprintf("gen%d_kick_h4", k))
		mp.GenKickEnv0[i] = get(fmt.Sprintf("gen%d_kick_env0", k))
		mp.GenKickEnv1[i] = get(fmt.Sprintf("gen%d_kick_env1", k))
		mp.GenKickPeAmt[i] = get(fmt.Sprintf("gen%d_kick_pe_amt", k))
		mp.GenKickPeRate[i] = get(fmt.Sprintf("gen%d_kick_pe_rate", k))
		mp.GenKickClick[i] = get(fmt.Sprintf("gen%d_kick_click", k))
		mp.GenKickNoise[i] = get(fmt.Sprintf("gen%d_kick_noise", k))

		mp.GenTomVariant[i] = get(fmt.Sprintf("gen%d_tom_variant", k))
		mp.GenTomSweep[i] = get(fmt.Sprintf("gen%d_tom_sweep", k))
		mp.GenTomRing[i] = get(fmt.Sprintf("gen%d_tom_ring", k))
		mp.GenTomO1[i] = get(fmt.Sprintf("gen%d_tom_o1", k))
		mp.GenTomO2[i] = get(fmt.Sprintf("gen%d_tom_o2", k))
		mp.GenTomStick[i] = get(fmt.Sprintf("gen%d_tom_stick", k))
		mp.GenTomRoom[i] = get(fmt.Sprintf("gen%d_tom_room", k))

		mp.GenSnareVariant[i] = get(fmt.Sprintf("gen%d_snare_variant", k))
		mp.GenSnareTone2[i] = get(fmt.Sprintf("gen%d_snare_tone2", k))
		mp.GenSnareTune[i] = get(fmt.Sprintf("gen%d_snare_tune", k))
		mp.GenSnareToneD[i] = get(fmt.Sprintf("gen%d_snare_tone_d", k))
		mp.GenSnareNoiseD[i] = get(fmt.Sprintf("gen%d_snare_noise_d", k))
		mp.GenSnareTailD[i] = get(fmt.Sprintf("gen%d_snare_tail_d", k))
		mp.GenSnareToneM[i] = get(fmt.Sprintf("gen%d_snare_tone_m", k))
		mp.GenSnareNoiseM[i] = get(fmt.Sprintf("gen%d_snare_noise_m", k))
		mp.GenSnareWireM[i] = get(fmt.Sprintf("gen%d_snare_wire_m", k))
		mp.GenSnareAttack[i] = get(fmt.Sprintf("gen%d_snare_attack", k))

		mp.GenCymVariant[i] = get(fmt.Sprintf("gen%d_cym_variant", k))
		mp.GenCymTune[i] = get(fmt.Sprintf("gen%d_cym_tune", k))
		mp.GenCymEnvFast[i] = get(fmt.Sprintf("gen%d_cym_env_fast", k))
		mp.GenCymEnvTail[i] = get(fmt.Sprintf("gen%d_cym_env_tail", k))
		mp.GenCymToneM[i] = get(fmt.Sprintf("gen%d_cym_tone_m", k))
		mp.GenCymNoiseM[i] = get(fmt.Sprintf("gen%d_cym_noise_m", k))
		mp.GenCymNoiseD[i] = get(fmt.Sprintf("gen%d_cym_noise_d", k))

		mp.GenFMVariant[i] = get(fmt.Sprintf("gen%d_fm_variant", k))
		mp.GenFMBase[i] = get(fmt.Sprintf("gen%d_fm_base", k))
		mp.GenFMPeAmt[i] = get(fmt.Sprintf("gen%d_fm_pe_amt", k))
		mp.GenFMPeDecay[i] = get(fmt.Sprintf("gen%d_fm_pe_decay", k))
		mp.GenFMR1[i] = get(fmt.Sprintf("gen%d_fm_r1", k))
		mp.GenFMR2[i] = get(fmt.Sprintf("gen%d_fm_r2", k))
		mp.GenFMR3[i] = get(fmt.Sprintf("gen%d_fm_r3", k))
		mp.GenFMR4[i] = get(fmt.Sprintf("gen%d_fm_r4", k))
		mp.GenFMD1[i] = get(fmt.Sprintf("gen%d_fm_d1", k))
		mp.GenFMD2[i] = get(fmt.Sprintf("gen%d_fm_d2", k))
		mp.GenFMD3[i] = get(fmt.Sprintf("gen%d_fm_d3", k))
		mp.GenFMD4[i] = get(fmt.Sprintf("gen%d_fm_d4", k))
		mp.GenFMDec1[i] = get(fmt.Sprintf("gen%d_fm_dec1", k))
		mp.GenFMDec2[i] = get(fmt.Sprintf("gen%d_fm_dec2", k))
		mp.GenFMDec3[i] = get(fmt.Sprintf("gen%d_fm_dec3", k))
		mp.GenFMDec4[i] = get(fmt.Sprintf("gen%d_fm_dec4", k))
	}
	return mp
}

// renderModularP renders the modular voice into buf. Mirrors the cParamRenderer
// shape of the drum/FM wrappers but takes the wide ModularParams block.
func renderModularP(buf []float32, sampleRate, samples int, params ModularParams) {
	if samples > len(buf) || len(buf) == 0 || samples == 0 {
		return
	}
	C.render_modular_p((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples), params.toCModularParams())
}

// renderModular renders the modular voice with its built-in defaults (the
// unparameterized fast path). Used as the instrument-table Render for the
// shipped modular instrument when no per-instrument params are set, so the
// no-edit sound matches the recipe's identity defaults on both platforms.
func renderModular(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	C.render_modular((*C.float)(unsafe.Pointer(&buf[0])), C.int(sampleRate), C.int(samples))
}

// modularPadVoiceParams is the baked ModularParams for the "modular-pad"
// preset's no-edit fast path: the modular schema identity overlaid with
// modularPadSeed. Built once at init. The dispatch fast path (tryRecipeVoice's
// cheap branch, taken when an instrument has no param overlay) renders an
// unedited modular-pad through renderModularPad, so the native no-edit sound
// matches the synth-modular-pad recipe defaults (the recipe/edit path renders
// the same values via recipeParamsToModular).
var modularPadVoiceParams = func() ModularParams {
	p := ModularParamSchemaIdentity()
	for k, v := range modularPadSeed {
		p[k] = v
	}
	return recipeParamsToModular(RecipeParams(p))
}()

// renderModularPad renders the pad preset with its baked defaults (the
// unparameterized fast path equivalent for modular-pad).
func renderModularPad(buf []float32, sampleRate, samples int) {
	if len(buf) == 0 || samples == 0 || samples > len(buf) {
		return
	}
	renderModularP(buf, sampleRate, samples, modularPadVoiceParams)
}

// oracleMaNoiseWhiteFill fills out with the REAL miniaudio ma_noise white
// stream (test oracle only — production code must use noiseMaWhiteFill).
func oracleMaNoiseWhiteFill(out []float32, seed int) {
	if len(out) == 0 {
		return
	}
	C.oracle_ma_noise_white_fill((*C.float)(unsafe.Pointer(&out[0])), C.int(len(out)), C.int(seed))
}

// noiseMaWhiteFill fills out with the transplanted deterministic stream.
func noiseMaWhiteFill(out []float32, seed int) {
	if len(out) == 0 {
		return
	}
	C.noise_ma_white_fill((*C.float)(unsafe.Pointer(&out[0])), C.int(len(out)), C.int(seed))
}
