package ui

// A dial is a LENS: turning it writes the instrument document like a raw knob, and nothing about the dial reaches
// any file. Its reading is a pure function of the document; the only state is the gesture's captured start values.

import (
	"math"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/synthdial"
)

// They set the note (note names derive from them), not the timbre.
var synthDialTuningParams = map[string]bool{
	"pitch": true, "osc_detune": true, "osc_octave": true, "post_pitch": true,
	"voice_freq_hz": true,
}

// Each MIRRORS the C dispatch in modular.c - the one place the UI restates engine routing - so a dial never shows
// where the engine would ignore it.
var synthDialConditions = map[string]func(synthDialEnv) bool{
	// Unison runs only for a wavetable oscillator with ≥ 2 voices and no pitch envelope or vibrato routing it first.
	"ensemble": func(e synthDialEnv) bool {
		m := e.p
		if m["osc_enabled"] < 0.5 || !synthWavetableOsc(m["osc_type"]) || m["unison_voices"] < 1.5 {
			return false
		}
		peActive := m["pitchenv_enabled"] >= 0.5 && m["pitchenv_amt"] != 0 && m["pitchenv_decay"] > 0
		vibOn := m["lfo_enabled"] >= 0.5 && int(m["lfo_target"]+0.5) == 1 && m["lfo_depth"] > 0 && m["lfo_rate"] > 0
		return !peActive && !vibOn
	},
	// Release sounds only if the envelope is still up at note-off (a 50 ms conga decay has no audible tail).
	"release": func(e synthDialEnv) bool {
		return e.p["amp_sustain"] > 0 || e.p["amp_decay"] >= 0.25
	},
	// Sustain (Hold) and a longer decay (Length) matter only if the envelope settles INSIDE the voice's
	// render.seconds; an unknown length (0) counts as settling.
	"settles": func(e synthDialEnv) bool {
		return e.voiceSecs <= 0 || e.p["amp_decay"] < e.voiceSecs
	},
	// The LFO cutoff path (lfo_target 2) ignores the filter envelope, and with the filter off there is no cutoff.
	"filtenv": func(e synthDialEnv) bool {
		m := e.p
		lfoSweep := m["lfo_enabled"] >= 0.5 && int(m["lfo_target"]+0.5) == 2 && m["lfo_depth"] > 0 && m["lfo_rate"] > 0
		return m["filter_enabled"] >= 0.5 && !lfoSweep
	},
	// Morphing toward the vowel already sung moves nothing.
	"morphs": func(e synthDialEnv) bool {
		return int(e.p["formant_morph_to"]+0.5) != int(e.p["formant_vowel"]+0.5)
	},
	// The noise and physical-model sources never read the pitch envelope.
	"pitchenv": func(e synthDialEnv) bool {
		m := e.p
		if m["osc_enabled"] < 0.5 {
			return false
		}
		t := m["osc_type"]
		return synthWavetableOsc(t) || int(t+0.5) == 4
	},
}

// Lets the discipline test prove no term is gated by its own dial, which would hide it mid-gesture.
var synthDialConditionReads = map[string][]string{
	"ensemble": {"osc_enabled", "osc_type", "unison_voices", "pitchenv_enabled", "pitchenv_amt", "pitchenv_decay", "lfo_enabled", "lfo_target", "lfo_depth", "lfo_rate"},
	"release":  {"amp_sustain", "amp_decay"},
	"settles":  {"amp_decay"},
	"pitchenv": {"osc_enabled", "osc_type"},
	"morphs":   {"formant_morph_to", "formant_vowel"},
	"filtenv":  {"filter_enabled", "lfo_enabled", "lfo_target", "lfo_depth", "lfo_rate"},
}

// Voice length in seconds, 0 = unknown.
type synthDialEnv struct {
	p         audio.RecipeParams
	voiceSecs float64
}

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

// An invalid document yields no dials anywhere plus the error, which the Synth tab logs once.
func loadSynthDials() (synthdial.File, error) {
	synthDialsOnce.Do(func() {
		synthDialsFile, synthDialsErr = synthdial.Load(assets.SynthDialsJSON)
		if synthDialsErr != nil {
			synthDialsFile = synthdial.File{}
		}
	})
	return synthDialsFile, synthDialsErr
}

func synthDialDrivable(def audio.ParamDef) bool {
	return len(def.Enum) == 0 && def.Step == 0 && !synthDialTuningParams[def.Name] && def.Max > def.Min
}

// Stages with several enable params (VOICE has one per family) are not gated here: a family's knobs are already
// hidden unless it is the voice.
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

// A dial whose every term sits in a bypassed stage does not show (it would do nothing) and reappears when the
// stage is switched on.
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

// The same at every tempo; 0 when unknown.
func (dv *DrumView) synthVoiceSecs(inst instrument.ID) float64 {
	d, ok := audio.InstrumentDoc(inst)
	if !ok {
		return 0
	}
	return d.Render.Seconds
}

func (dv *DrumView) synthDialReading(inst instrument.ID, d synthdial.Resolved) int {
	return synthDialPercent(d.Reading(audio.MergedInstrumentParamsRO(inst)))
}

// synthDialPercent rounds a reading (0..1) to the integer the dial shows.
func synthDialPercent(r float64) int { return int(math.Round(100 * r)) }

// The ONLY dial state: dragging back to the start writes the start values back bit for bit, and Esc restores
// them verbatim.
type synthDialGesture struct {
	inst         instrument.ID
	card, dial   string
	d            synthdial.Resolved
	start        map[string]float64
	startReading int
	last         int
}

// The dial set freezes while live: a turn may flip another dial's liveness and the row must not reshuffle
// under the finger.
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

// One document write per detent, so the engine and prewarmer never see a half-turned dial. Nothing
// auditions or commits here.
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

// A gesture that moved nothing records nothing.
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

// Anything that clears the tree's capture (Esc, a view switch) never reaches OnRelease; one check here
// instead of a hook at every ClearCapture site.
func (dv *DrumView) healSynthDialGesture() {
	g := dv.synthDialActive
	if g == nil || (dv.audioTree != nil && dv.audioTree.Capturing()) {
		return
	}
	dv.endSynthDialGesture(g, false)
}

// For scenes and tests.
func (dv *DrumView) setSynthDialReading(inst instrument.ID, d synthdial.Resolved, target int) {
	if inst == "" || len(d.Terms) == 0 {
		return
	}
	dv.turnSynthDial(newSynthDialGesture(inst, "", d), target)
}
