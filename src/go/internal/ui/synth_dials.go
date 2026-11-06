package ui

// synth_dials.go is the Synth tab's MACRO DIAL logic: up to three dials on
// each root category card, each a vector over existing synth params
// (internal/synthdial, data in internal/assets/synth_dials.json).
//
// A dial is a LENS. Turning it writes the same instrument document a raw knob
// writes (all of a detent's params in one audio.SetInstrumentParamsLive), so
// the sound persists through the instrument document like any knob edit — and
// nothing about the dial itself (position, definition) ever reaches a
// project, bundle, prefs or library file.
//
// ABSOLUTE: a dial's position is a pure function of the document
// (synthdial.Resolved.Reading over each term's absolute span), so the same
// sound always shows the same knob — after a restart, an import, undo/redo or
// a raw knob edit. The only state is a GESTURE: a drag or an open value wheel
// captures the values it started from (synthDialGesture), every detent is
// Turn(start, target), and the capture is dropped when the gesture ends.
//
// Specs: docs/superpowers/specs/2026-10-09-synth-macro-dials-design.md,
// docs/superpowers/specs/2026-10-13-absolute-synth-dials-design.md.

import (
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// synthDialTuningParams are the params a dial must never drive: they set the
// note an instrument sounds (note names are derived from them), not its
// timbre. Enum and stepped params are excluded by their ParamDef.
var synthDialTuningParams = map[string]bool{
	"pitch": true, "osc_detune": true, "osc_octave": true, "post_pitch": true,
	"voice_freq_hz": true,
}

// synthDialConditions are the engine conditions a dial term can name in its
// `when` field. Each MIRRORS the C source dispatch (modular.c,
// render_modular_p's oscillator stage) — the one place the UI restates
// engine routing, so a dial never shows when the engine would ignore it.
var synthDialConditions = map[string]func(synthDialEnv) bool{
	// The unison/ensemble path (modular_source_unison) runs only for a
	// wavetable oscillator with ≥ 2 voices, and only when neither an active
	// pitch envelope nor LFO vibrato routes the voice through the modulated
	// single-oscillator path first.
	"ensemble": func(e synthDialEnv) bool {
		m := e.p
		if m["osc_enabled"] < 0.5 || !synthWavetableOsc(m["osc_type"]) || m["unison_voices"] < 1.5 {
			return false
		}
		peActive := m["pitchenv_enabled"] >= 0.5 && m["pitchenv_amt"] != 0 && m["pitchenv_decay"] > 0
		vibOn := m["lfo_enabled"] >= 0.5 && int(m["lfo_target"]+0.5) == 1 && m["lfo_depth"] > 0 && m["lfo_rate"] > 0
		return !peActive && !vibOn
	},
	// The amp release only sounds when the envelope is still up at note-off:
	// a held sustain, or a decay long enough to outlast a short note (a conga
	// whose 50 ms decay is silent long before note-off has no audible tail).
	"release": func(e synthDialEnv) bool {
		return e.p["amp_sustain"] > 0 || e.p["amp_decay"] >= 0.25
	},
	// The amp envelope reaches its sustain INSIDE the played voice (a voice is
	// render.seconds long — a 2.5 s guitar decay in a 0.75 s voice never gets
	// there): only then does the sustain level (Hold) sound, and only then
	// does a longer decay (Length) change anything. Unknown voice length (0)
	// counts as settling.
	"settles": func(e synthDialEnv) bool {
		return e.voiceSecs <= 0 || e.p["amp_decay"] < e.voiceSecs
	},
	// The filter envelope reaches the cutoff only on the filter's envelope path:
	// with the filter off there is no cutoff, and an LFO sweeping the cutoff
	// (lfo_target 2) takes the filter's LFO path, which ignores the envelope
	// entirely (modular.c, the filter stage).
	"filtenv": func(e synthDialEnv) bool {
		m := e.p
		lfoSweep := m["lfo_enabled"] >= 0.5 && int(m["lfo_target"]+0.5) == 2 && m["lfo_depth"] > 0 && m["lfo_rate"] > 0
		return m["filter_enabled"] >= 0.5 && !lfoSweep
	},
	// The formant morph LFO sweeps toward morph_to: toward the vowel it is
	// already singing it moves nothing (formant.c, the morph path).
	"morphs": func(e synthDialEnv) bool {
		return int(e.p["formant_morph_to"]+0.5) != int(e.p["formant_vowel"]+0.5)
	},
	// The modular pitch envelope reaches the wavetable and FM oscillators;
	// the noise and physical-model sources never read it.
	"pitchenv": func(e synthDialEnv) bool {
		m := e.p
		if m["osc_enabled"] < 0.5 {
			return false
		}
		t := m["osc_type"]
		return synthWavetableOsc(t) || int(t+0.5) == 4
	},
}

// synthDialConditionReads names the params each condition reads, so the
// discipline test can prove no term is gated by its own dial (a turn would
// then hide the term mid-gesture and the reading would jump).
var synthDialConditionReads = map[string][]string{
	"ensemble": {"osc_enabled", "osc_type", "unison_voices", "pitchenv_enabled", "pitchenv_amt", "pitchenv_decay", "lfo_enabled", "lfo_target", "lfo_depth", "lfo_rate"},
	"release":  {"amp_sustain", "amp_decay"},
	"settles":  {"amp_decay"},
	"pitchenv": {"osc_enabled", "osc_type"},
	"morphs":   {"formant_morph_to", "formant_vowel"},
	"filtenv":  {"filter_enabled", "lfo_enabled", "lfo_target", "lfo_depth", "lfo_rate"},
}

// synthDialEnv is what a dial condition may read: the instrument's current
// values and the length of the voice a trigger plays (seconds; 0 = unknown).
type synthDialEnv struct {
	p         audio.RecipeParams
	voiceSecs float64
}

// synthWavetableOsc reports whether an osc_type is a wavetable oscillator
// (sine, saw, square, triangle, glottal — modular_table_for).
func synthWavetableOsc(t float64) bool {
	switch int(t + 0.5) {
	case 0, 1, 2, 3, 12:
		return true
	}
	return false
}

var (
	synthDialsOnce sync.Once
	synthDialsFile synthdial.File
	synthDialsErr  error
)

// loadSynthDials returns the embedded dial document, decoded once. An invalid
// document yields an empty file (no dials anywhere — the cards behave exactly
// as before) plus the error, which the Synth tab logs once.
func loadSynthDials() (synthdial.File, error) {
	synthDialsOnce.Do(func() {
		synthDialsFile, synthDialsErr = synthdial.Load(assets.SynthDialsJSON)
		if synthDialsErr != nil {
			synthDialsFile = synthdial.File{}
		}
	})
	return synthDialsFile, synthDialsErr
}

// synthDialDrivable reports whether a dial may drive def: a continuous,
// non-tuning param.
func synthDialDrivable(def audio.ParamDef) bool {
	return len(def.Enum) == 0 && def.Step == 0 && !synthDialTuningParams[def.Name] && def.Max > def.Min
}

// synthStageGates maps each pipeline stage to its enable param, read from the
// instrument's schema. A stage with no bypass, or with SEVERAL enable params
// (VOICE carries one per gen-slot family — kick, snare, tom, … — and only the
// active family's matters), is not gated here: a family's knobs are already
// hidden unless that family is the voice.
func synthStageGates(schema []audio.ParamDef) map[synthSectionID]string {
	gates := map[synthSectionID]string{}
	ambiguous := map[synthSectionID]bool{}
	for _, d := range schema {
		if !isSynthEnableParam(d.Name) {
			continue
		}
		sec := sectionForParamDef(d)
		if _, dup := gates[sec]; dup {
			ambiguous[sec] = true
		}
		gates[sec] = d.Name
	}
	for sec := range ambiguous {
		delete(gates, sec)
	}
	return gates
}

// resolveCardDials returns the dials live on one card for one instrument. A
// term is live when the card claims its param on this instrument (claimed =
// the card's visible params), the param is drivable, its pipeline stage is
// ON, and its Requires param (a model selector) is on. A dial whose every term
// sits in a bypassed stage therefore does not show at all — it would do
// nothing audible — and reappears when the user switches the stage on.
func resolveCardDials(card string, claimed []string, present map[string]audio.ParamDef, merged audio.RecipeParams, gates map[synthSectionID]string, voiceSecs float64) []synthdial.Resolved {
	f, _ := loadSynthDials()
	if len(f.Dials) == 0 || len(claimed) == 0 {
		return nil
	}
	own := make(map[string]bool, len(claimed))
	for _, p := range claimed {
		own[p] = true
	}
	return synthdial.Resolve(f, card, func(t synthdial.Term) (synthdial.Range, bool) {
		def, ok := present[t.Param]
		if !ok || !own[t.Param] || !synthDialDrivable(def) {
			return synthdial.Range{}, false
		}
		if en := gates[sectionForParamDef(def)]; en != "" && merged[en] < 0.5 {
			return synthdial.Range{}, false
		}
		if !t.RequirementMet(merged[t.Requires]) {
			return synthdial.Range{}, false
		}
		if t.When != "" {
			if cond := synthDialConditions[t.When]; cond == nil || !cond(synthDialEnv{merged, voiceSecs}) {
				return synthdial.Range{}, false
			}
		}
		return synthdial.Range{Min: def.Min, Max: def.Max}, true
	})
}

// synthVoiceSecs is how long a trigger of inst sounds: its document's
// render.seconds — the same at every tempo (0 when unknown).
func (dv *DrumView) synthVoiceSecs(inst instrument.ID) float64 {
	d, ok := audio.InstrumentDoc(inst)
	if !ok {
		return 0
	}
	return d.Render.Seconds
}

// synthDialReading is a dial's position on inst as the user reads it: 0…100,
// a pure function of the instrument's current values.
func (dv *DrumView) synthDialReading(inst instrument.ID, d synthdial.Resolved) int {
	return synthDialPercent(d.Reading(audio.MergedInstrumentParamsRO(inst)))
}

// synthDialPercent rounds a reading (0..1) to the integer the dial shows.
func synthDialPercent(r float64) int { return int(math.Round(100 * r)) }

// synthDialGesture is the ONLY dial state: one drag or one open value wheel.
// It holds the values the gesture started from, so every detent is a pure
// Turn(start, target) — dragging back to where it started writes start back
// bit for bit — and Esc restores them verbatim. Dropped when the gesture ends.
type synthDialGesture struct {
	inst         instrument.ID
	card, dial   string
	d            synthdial.Resolved
	start        map[string]float64
	startReading int
	last         int
}

// beginSynthDialGesture captures a dial gesture on inst's card/dial. While it
// is live the Synth root keeps its dial set (synthDialActive freezes the macro
// cache): a turn may flip another dial's liveness and the row must not
// reshuffle under the finger.
func (dv *DrumView) beginSynthDialGesture(inst instrument.ID, card, dial string) (*synthDialGesture, bool) {
	d, ok := dv.synthDialByID(card, dial)
	if !ok || inst == "" || len(d.Terms) == 0 {
		return nil, false
	}
	g := newSynthDialGesture(inst, card, d)
	dv.synthDialActive = g
	return g, true
}

func newSynthDialGesture(inst instrument.ID, card string, d synthdial.Resolved) *synthDialGesture {
	cur := audio.MergedInstrumentParamsRO(inst)
	start := make(map[string]float64, len(d.Terms))
	for _, t := range d.Terms {
		if v, ok := cur[t.Param]; ok {
			start[t.Param] = v
		}
	}
	r := synthDialPercent(d.Reading(start))
	return &synthDialGesture{inst: inst, card: card, dial: d.Dial.ID, d: d, start: start, startReading: r, last: r}
}

// turnSynthDial moves the gesture's dial to target (0…100) and applies it
// LIVE: one document write for the whole detent (the engine and the voice
// prewarmer never see a half-turned dial — guard
// TestSynthDialDetentPublishesOnlyItsFinalDocument). Nothing auditions and
// nothing commits; the gesture's end commits once.
func (dv *DrumView) turnSynthDial(g *synthDialGesture, target int) {
	target = max(0, min(100, target))
	if target == g.last {
		return
	}
	g.last = target
	vals := g.start
	if target != g.startReading {
		vals = g.d.Turn(g.start, float64(target)/100)
	}
	audio.SetInstrumentParamsLive(g.inst, vals)
}

// endSynthDialGesture closes a gesture: commit = ONE commitInstrumentParams
// (one undo step, one log line) when the values moved; otherwise the start
// values go back verbatim. A gesture that moved nothing records nothing.
func (dv *DrumView) endSynthDialGesture(g *synthDialGesture, commit bool) {
	if g == nil {
		return
	}
	if dv.synthDialActive == g {
		dv.synthDialActive = nil
	}
	cur := audio.MergedInstrumentParamsRO(g.inst)
	moved := false
	for p, v := range g.start {
		if cur[p] != v {
			moved = true
			break
		}
	}
	if !moved {
		return
	}
	if commit {
		dv.commitInstrumentParams(g.inst)
		return
	}
	audio.SetInstrumentParamsLive(g.inst, g.start)
}

// healSynthDialGesture cancels a drag gesture whose input capture is gone
// without a release — Esc, a mobile view switch, anything that clears the
// tree's capture never reaches OnRelease. Cancel = Esc semantics: the start
// values go back, nothing commits, and the dial row unfreezes. One check here
// instead of a hook at every ClearCapture site.
func (dv *DrumView) healSynthDialGesture() {
	g := dv.synthDialActive
	if g == nil || (dv.audioTree != nil && dv.audioTree.Capturing()) {
		return
	}
	dv.endSynthDialGesture(g, false)
}

// setSynthDialReading puts dial d on inst at target (0…100) through the same
// Turn a gesture uses, live, without committing — for scenes and tests that
// need a dial in a given position.
func (dv *DrumView) setSynthDialReading(inst instrument.ID, d synthdial.Resolved, target int) {
	if inst == "" || len(d.Terms) == 0 {
		return
	}
	dv.turnSynthDial(newSynthDialGesture(inst, "", d), target)
}
