//go:build !test && !js

package audio

/*
#cgo CFLAGS: -I${SRCDIR}/../../../c
#cgo LDFLAGS: -L${SRCDIR}/../../../../build -ldrums -lm
#include "drums.h"
#include <stdlib.h>
*/
import "C"

// SynthParams mirrors the C synth_params struct for per-node instrument
// customization. All zero values produce default instrument behavior.
//
// Synth-tab redesign (2026-05-16) dropped the Attack and Color fields
// because no C renderer reads them — they were silent no-ops in every
// shipped recipe. See src/c/synth_params.h for the matching change.
type SynthParams struct {
	Pitch       float64 // Semitone offset from default (0 = default)
	Decay       float64 // Decay multiplier (1.0 = default, 0.5 = half, 2.0 = double)
	Tone        float64 // Tone/brightness: -1 = dark, 0 = default, 1 = bright
	Drive       float64 // Saturation amount (0 = none, 1 = heavy)
	Body        float64 // Body/resonance emphasis (0 = default, 1 = max)
	Brightness  float64 // High-frequency content: 0 = default, 1 = max shimmer
	Fundamental float64 // Recipe-specific core-synthesis override (Hz).
	// 0 = use the recipe's built-in default. Phase-3 addition;
	// only drum-kick reads it today, but the schema slot
	// stays open so future per-recipe extras can land
	// without a struct shape change.
}

// IsDefault returns true if all params are at their default values.
func (p SynthParams) IsDefault() bool {
	return p.Pitch == 0 && p.Decay == 0 && p.Tone == 0 &&
		p.Drive == 0 && p.Body == 0 && p.Brightness == 0 && p.Fundamental == 0
}

// ToRecipeParams converts the wired-knob struct to a generic RecipeParams
// map. Only non-zero fields are emitted; the recipe registry treats absent
// keys the same as defaults, so this keeps the voice cache hash stable for
// the "everything zero" case.
func (p SynthParams) ToRecipeParams() RecipeParams {
	out := RecipeParams{}
	if p.Pitch != 0 {
		out["pitch"] = p.Pitch
	}
	if p.Decay != 0 {
		out["decay"] = p.Decay
	}
	if p.Tone != 0 {
		out["tone"] = p.Tone
	}
	if p.Drive != 0 {
		out["drive"] = p.Drive
	}
	if p.Body != 0 {
		out["body"] = p.Body
	}
	if p.Brightness != 0 {
		out["brightness"] = p.Brightness
	}
	if p.Fundamental != 0 {
		out["fundamental"] = p.Fundamental
	}
	return out
}

// toCParams converts Go SynthParams to a C synth_params pointer.
// Returns nil if params are default (so C functions use their hardcoded defaults).
func (p SynthParams) toCParams() *C.synth_params {
	if p.IsDefault() {
		return nil
	}
	cp := &C.synth_params{
		pitch:       C.float(p.Pitch),
		decay:       C.float(p.Decay),
		tone:        C.float(p.Tone),
		drive:       C.float(p.Drive),
		body:        C.float(p.Body),
		brightness:  C.float(p.Brightness),
		fundamental: C.float(p.Fundamental),
	}
	return cp
}

// cParamRenderer is a render function that accepts synth_params.
type cParamRenderer func(buf []float32, sampleRate, samples int, params SynthParams)

// ── Kick family param block (DELETED) ────────────────────────────────────────
//
// KickParams / kickParamsFromSynth / recipeParamsToKick / isUnset / toC /
// renderKickFam (+ deep/punchy/lofi/tight) are deleted: the kick family migrated
// to the unified modular engine (Phase-3). Its render binding is
// kickRecipeToModular (kick_modular_binding.go), and the deleted toC()==nil
// POST-skip rule now lives, tag-neutral, in kickLegacyNilElision
// (kick_modular_push.go).

// ── Snare family param block (DELETED) ───────────────────────────────────────
//
// SnareParams / snareParamsFromSynth / recipeParamsToSnare / isUnset / toC /
// renderSnareFam (+ rimshot/sidestick/clap) are deleted: the snare family
// migrated to the modular engine (Phase-5). The recipe/edit path renders through
// snareRecipeToModular (snare_modular_binding.go) onto the source==7 snare-ish
// voice (snare/rimshot/sidestick) and the source==8 clap voice, and the C
// render_snare* / render_clap paths are deleted. The deleted toC()==nil POST-skip
// rule now lives, tag-neutral, in snareLegacyNilElision (snare_modular_push.go).

// ── Cymbal family param block (DELETED) ──────────────────────────────────────
//
// CymbalParams / cymbalParamsFromSynth / recipeParamsToCymbal / isUnset / toC /
// renderHiHatFam (+ open-hihat/cowbell/shaker/ride/crash) are deleted: the cymbal
// family migrated to the modular engine (Phase-6). The recipe/edit path renders
// through cymbalRecipeToModular (cymbal_modular_binding.go) onto the source==9
// metallic voice (variant 0=hihat 1=open-hihat 2=cowbell 3=shaker 4=ride 5=crash),
// and the C render_hihat* / render_open_hihat* / render_cowbell* / render_shaker* /
// render_ride* / render_crash* paths are deleted. The deleted toC()==nil POST-skip
// rule now lives, tag-neutral, in cymbalLegacyNilElision (cymbal_modular_push.go).

// ── Bass family param block (DELETED) ────────────────────────────────────────
//
// BassParams / bassParamsFromSynth / recipeParamsToBass / isUnset / toC /
// renderBassGuitarFam / renderSubBassFam are deleted: the bass family migrated
// to the unified modular engine (Phase-2). Its render binding is
// bassRecipeToModular (bass_modular_binding.go), and
// the deleted toC()==nil POST-skip rule now lives, tag-neutral, in
// bassLegacyNilElision (bass_modular_push.go).

// ── Tom family param block (DELETED) ─────────────────────────────────────────
//
// TomParams / tomParamsFromSynth / recipeParamsToTom / TomParams.toC /
// renderTomFam / renderTomHighFam / renderTomLowFam deleted with the tom-family
// migration (Phase-4): the three tom recipes render through the modular binding
// (tomRecipeToModular, tom_modular_binding.go) onto the source==6 808-style tom
// voice, and the C render_tom* paths are deleted. The deleted toC()==nil
// POST-skip rule now lives, tag-neutral, in tomLegacyNilElision
// (tom_modular_push.go).

// Family-aware kick render wrappers deleted with the kick-family migration
// (Phase-3). The recipe/edit path renders kick through the modular binding
// (kickRecipeToModular); the no-edit native fast path is renderKickVoice /
// renderKickDeepVoice / renderKickPunchyVoice / renderKickLofiVoice /
// renderKickTightVoice (kick_modular_native.go).

// Parameterized render wrappers.

// renderSnareP / renderClapP deleted — snare/clap render via the modular binding
// (Phase-5). The recipe/edit path renders through snareRecipeToModular.

// renderKickP deleted — kick renders via the modular binding (Phase-3).

// renderHiHatP / renderOpenHihatP / renderCowbellP / renderShakerP / renderRideP
// / renderCrashP deleted with the cymbal family migration (Phase-6). The
// recipe/edit path renders cymbals through the modular binding
// (cymbalRecipeToModular).

// renderTomP / renderTomHighP / renderTomLowP deleted with the tom family
// migration (Phase-4). The recipe/edit path renders tom through the modular
// binding (tomRecipeToModular).

// renderBassGuitarP / renderSubBassP deleted with the bass family migration
// (Phase-2). The recipe/edit path renders bass through the modular binding.

// renderSnareRimshotP / renderSnareSidestickP deleted with the snare family
// migration (Phase-5). The recipe/edit path renders through snareRecipeToModular.

// renderKickDeepP / renderKickPunchyP / renderKickLofiP / renderKickTightP
// deleted with the kick-family migration (Phase-3). The recipe/edit path renders
// these through the modular binding (kickRecipeToModular).

// ParamRenderFuncs maps instrument base names to their _p() C render wrappers.
// EMPTY after Phase-7: EVERY legacy family (bass/kick/tom/snare/cymbal/FM) now
// renders through the modular binding (builtinFamilyRenderers), so no _p() C
// wrapper remains. Kept as a (now-empty) map so callers that range over it stay
// valid; it documents that the legacy bespoke-renderer path is fully retired.
var ParamRenderFuncs = map[string]cParamRenderer{
	// snare / clap (+ snare-rimshot / snare-sidestick) render via the modular
	// binding (Phase-5) — no renderSnareP / renderClapP / renderSnare*P entry.
	// kick (+ deep/punchy/lofi/tight) render via the modular binding (Phase-3) —
	// no renderKickP / renderKick*P entry.
	// hihat / open-hihat / cowbell / shaker / ride / crash render via the modular
	// binding (Phase-6) — no renderHiHatP / … entry.
	// tom (+ high/low) render via the modular binding (Phase-4) — no renderTomP /
	// renderTom*P entry.
	// bass-guitar / sub-bass render via the modular binding (Phase-2) — no
	// renderBassGuitarP / renderSubBassP entry.
	// fm-bass / fm-bell / fm-lead / fm-epiano / fm-pluck render via the modular
	// binding (Phase-7, the LAST family) — no renderFM*P entry.
}
