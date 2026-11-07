package ui

// Every view is drawn from the rendered note, never a model. A raw knob resolves through an explicit table with
// NO fallback.

import (
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

type synthView int

const (
	viewOverview synthView = iota
	viewLoudness
	viewAttack
	viewPitch
	viewColour
	viewHarmonics
	viewColourTime
	viewNoise
	viewWave
	synthViewCount
)

// synthViewTitle is the view's plain-language name.
func synthViewTitle(v synthView) string {
	switch v {
	case viewOverview:
		return i18n.T(i18n.KeyGraphViewOverview)
	case viewLoudness:
		return i18n.T(i18n.KeyGraphViewLoudness)
	case viewAttack:
		return i18n.T(i18n.KeyGraphViewAttack)
	case viewPitch:
		return i18n.T(i18n.KeyGraphViewPitch)
	case viewColour:
		return i18n.T(i18n.KeyGraphViewColour)
	case viewHarmonics:
		return i18n.T(i18n.KeyGraphViewHarmonics)
	case viewColourTime:
		return i18n.T(i18n.KeyGraphViewColourTime)
	case viewNoise:
		return i18n.T(i18n.KeyGraphViewNoise)
	case viewWave:
		return i18n.T(i18n.KeyGraphViewWave)
	}
	return ""
}

// "lfo-target" follows the LFO's routing: Amp → loudness, Pitch → pitch, Cutoff → colour over time.
func synthViewByName(name string, merged audio.RecipeParams) (synthView, bool) {
	switch name {
	case "loudness":
		return viewLoudness, true
	case "attack":
		return viewAttack, true
	case "pitch":
		return viewPitch, true
	case "colour":
		return viewColour, true
	case "harmonics":
		return viewHarmonics, true
	case "colour-time":
		return viewColourTime, true
	case "noise":
		return viewNoise, true
	case "wave":
		return viewWave, true
	case "lfo-target":
		return synthLFOTargetView(merged), true
	}
	return 0, false
}

func synthLFOTargetView(merged audio.RecipeParams) synthView {
	switch int(merged["lfo_target"] + 0.5) {
	case 1:
		return viewPitch
	case 2:
		return viewColourTime
	}
	return viewLoudness
}

// Every visible group must be here: an unknown group is a test failure, not a fallback.
var synthParamViewGroups = map[string]synthView{
	"osc":                viewHarmonics,
	"fm":                 viewHarmonics,
	"filter":             viewColour,
	"filtenv":            viewColourTime,
	"env":                viewLoudness,
	"lfo":                viewLoudness, // resolved by lfo_target below
	"pitchenv":           viewPitch,
	"voice":              viewPitch,
	"burst":              viewAttack,
	"unison":             viewHarmonics,
	"formant":            viewColour,
	"resonator":          viewColour,
	"post":               viewWave,
	"finish":             viewColour,
	audio.AnyFamilyGroup: viewWave,
	"kick":               viewColour,
	"snare":              viewColour,
	"tom":                viewColour,
	"cymbal":             viewColour,
	"bass":               viewHarmonics,
	"modal":              viewColour,
	"noiseband":          viewColour,
	"strike":             viewAttack,
	"core":               viewLoudness,
	"generic":            viewLoudness,
	"":                   viewLoudness,
}

// synthParamViewNames overrides a knob's group view by its exact name.
var synthParamViewNames = map[string]synthView{
	"osc_type": viewWave, "osc_octave": viewPitch, "osc_detune": viewPitch,
	"gen1_fm_pe_amt": viewPitch, "gen1_fm_pe_decay": viewPitch, "gen1_fm_base": viewPitch,
	"amp_attack":        viewAttack,
	"unison_drift_rate": viewPitch, "unison_drift_depth": viewPitch, "ens_scatter": viewPitch,
	"ens_humanize": viewPitch, "ens_jitter": viewPitch, "ens_vib_rate": viewPitch, "ens_vib_depth": viewPitch,
	"formant_morph_rate": viewColourTime, "formant_morph_to": viewColourTime, "formant_breath": viewNoise,
	"bow_dynamics": viewColourTime,
	"gain":         viewLoudness, "pitch": viewPitch,
	"post_pitch": viewPitch, "post_decay": viewLoudness, "post_decay_rate": viewLoudness,
	"post_drive": viewWave, "out_clamp": viewWave,
	"gen1_kick_attack": viewAttack, "gen1_kick_click": viewAttack,
	"gen1_kick_env0": viewLoudness, "gen1_kick_env1": viewLoudness, "gen1_kick_fade": viewLoudness,
	"gen1_kick_hold": viewLoudness, "gen1_kick_mode_decay": viewLoudness, "gen1_kick_reverb": viewLoudness,
	"gen1_kick_noise": viewNoise, "gen1_kick_hiss": viewNoise,
	"gen1_kick_pe_amt": viewPitch, "gen1_kick_pe_rate": viewPitch,
	"gen1_kick_sat":       viewWave,
	"gen1_kick_mode_gain": viewColourTime, "gen1_kick_mode_detune": viewColourTime,
	"gen1_kick_variant": viewOverview, "gen1_snare_variant": viewOverview, "gen1_tom_variant": viewOverview,
	"gen1_cym_variant": viewOverview, "gen1_fm_variant": viewHarmonics,
	"gen1_snare_attack": viewAttack, "gen1_snare_noise_m": viewNoise, "gen1_snare_wire_m": viewNoise,
	"gen1_tom_stick": viewAttack, "gen1_tom_ring": viewLoudness, "gen1_tom_room": viewLoudness, "gen1_tom_sweep": viewPitch,
	"gen1_cym_attack": viewAttack, "gen1_cym_spark": viewAttack, "gen1_cym_spark_f": viewAttack, "gen1_cym_spark_w": viewAttack,
	"gen1_cym_decay_tilt": viewLoudness, "gen1_cym_env_fast": viewLoudness, "gen1_cym_env_tail": viewLoudness,
	"gen1_cym_noise_d": viewLoudness, "gen1_cym_rattle_decay": viewLoudness, "gen1_cym_rattle_hold": viewLoudness,
	"gen1_cym_beat": viewColourTime, "gen1_cym_rattle_rate": viewColourTime, "gen1_cym_drive_delay": viewColourTime,
	"gen1_cym_noise_m": viewNoise, "gen1_cym_wash": viewNoise, "gen1_cym_rattle": viewNoise,
	"gen1_atk_amt": viewAttack, "gen1_atk_rate": viewAttack,
	"gen1_env_fast_rate": viewLoudness, "gen1_out_scale": viewLoudness,
	"gen1_pitch_env_amt": viewPitch, "gen1_pitch_env_rate": viewPitch, "gen1_sat_k": viewWave,
	"noiseband_level": viewNoise,
}

var synthParamViewSuffixes = []struct {
	suffix string
	view   synthView
}{
	{"_noise_d", viewLoudness}, {"_tail_d", viewLoudness}, {"_tone_d", viewLoudness},
	{"_decay_x", viewColourTime}, {"_law_a", viewColourTime}, {"_law_p", viewColourTime}, {"_law_b", viewColourTime},
	{"_dec1", viewColourTime}, {"_dec2", viewColourTime}, {"_dec3", viewColourTime}, {"_dec4", viewColourTime},
}

// ok=false only for an unknown group, never silently absorbed.
func synthParamView(def audio.ParamDef, merged audio.RecipeParams) (synthView, bool) {
	if v, ok := synthParamViewNames[def.Name]; ok {
		return v, true
	}
	for _, s := range synthParamViewSuffixes {
		if strings.HasSuffix(def.Name, s.suffix) {
			return s.view, true
		}
	}
	if def.Group == "lfo" {
		return synthLFOTargetView(merged), true
	}
	v, ok := synthParamViewGroups[def.Group]
	return v, ok
}

// Only these are computed, on the worker, so a loudness view never pays for a spectrogram on the browser's
// single thread.
func synthViewNeeds(v synthView) soundview.Need {
	n := soundview.NeedEnvelope
	switch v {
	case viewOverview, viewColour, viewColourTime, viewNoise:
		n |= soundview.NeedOnsets | soundview.NeedSTFT
	case viewAttack:
		n |= soundview.NeedOnsets
	case viewPitch:
		n |= soundview.NeedPitch
	case viewHarmonics:
		// Harmonics falls back to the tone colour on an unpitched note.
		n |= soundview.NeedOnsets | soundview.NeedPitch | soundview.NeedHarmonics | soundview.NeedSTFT
	case viewWave:
		n |= soundview.NeedOnsets | soundview.NeedPitch
	}
	return n
}

// An unpitched note's close-up falls back to colour, so that is warmed once its pitch is known absent.
func synthRenderNeeds(v synthView, r *graphRender) soundview.Need {
	n := synthViewNeeds(v)
	if v == viewWave && r != nil && r.an != nil && r.an.Has(soundview.NeedPitch) && !r.an.Pitched() {
		n |= soundview.NeedSTFT
	}
	return n
}
