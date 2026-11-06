package ui

// synth_dial_relevance.go — which dials an instrument shows is decided by the
// DSP (spec docs/superpowers/specs/2026-10-25-synth-dial-relevance-and-cheap-graphs-design.md §3.2).
//
// A dial is RELEVANT for an instrument when turning it from 0 to 100 changes
// the rendered note audibly (soundview.CompareNotes, the one audibility
// oracle). An irrelevant dial is hidden — there is no exception list
// anywhere: the audio change is the source of truth.
//
//   - WHICH SOUND: the instrument as the Synth tab first saw it in the open
//     circuit (the ANCHOR). Edits in the tab never re-hide or re-show a dial
//     (no card jumps mid-session); opening a circuit re-anchors.
//   - WHERE THE VERDICT COMES FROM, keyed by the anchor's sound signature:
//     the generated table of every shipped sound (synth_dial_relevance.gen.json,
//     `make gen-dial-relevance`, kept current by TestDialRelevanceTableIsCurrent),
//     else a measurement run in the background on the graph pipeline's worker.
//     Until a verdict lands the dial shows (config gates still apply).

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
	"github.com/ingyamilmolinar/beatmo/internal/soundview"
)

// dialKey names a dial on its card ("brightness/edge").
func dialKey(card, dial string) string { return card + "/" + dial }

// dialProbe is one dial of a sound: its key, the view it names, and the
// document fields at its two ends (synthdial Turn to 0 and to 100 — exact,
// the same values on every instrument).
type dialProbe struct {
	key    string
	view   synthView
	lo, hi map[string]float64
}

// The reference graph size relevance judges "visible" at.
const relevanceViewW, relevanceViewH = 420, 170

// dialProbes lists every dial the cards resolve, with its ends, in card order.
func dialProbes(macros []macroGroup, merged audio.RecipeParams) []dialProbe {
	var out []dialProbe
	for _, cg := range macros {
		for _, d := range cg.Dials {
			start := make(map[string]float64, len(d.Terms))
			for _, t := range d.Terms {
				if v, ok := merged[t.Param]; ok {
					start[t.Param] = v
				}
			}
			v, ok := synthViewByName(d.Dial.View, merged)
			if !ok {
				v = viewLoudness
			}
			out = append(out, dialProbe{key: dialKey(cg.ID, d.Dial.ID), view: v, lo: d.Turn(start, 0), hi: d.Turn(start, 1)})
		}
	}
	return out
}

// probeRelevant renders doc with the probe's dial at 0 and at 100 and reports
// whether the change is RELEVANT: the two notes sound different (the
// audibility oracle) AND a graph view shows the difference (chooseDialView
// finds one that moves visibly). A change the oracle finds only in quiet
// cells that no picture can show is no use to anyone turning the knob.
func probeRelevant(id instrument.ID, doc instrument.Doc, p dialProbe, render graphRenderFunc, yield func()) (bool, error) {
	at := func(fields map[string]float64) (*graphRender, error) {
		d, _, err := audio.ApplyInstrumentFields(doc, fields)
		if err != nil {
			return nil, err
		}
		pcm, sr, err := render(id, d, 0)
		if err != nil {
			return nil, err
		}
		voice := audio.GraphVoiceSec(d)
		return &graphRender{sr: sr, voiceSec: voice, an: soundview.New(pcm, sr, voice)}, nil
	}
	lo, err := at(p.lo)
	if err != nil {
		return false, err
	}
	hi, err := at(p.hi)
	if err != nil {
		return false, err
	}
	if !soundview.CompareNotesYield(lo.an.PCM(), hi.an.PCM(), lo.sr, yield).Audible() {
		return false, nil
	}
	lo.ident.variant, hi.ident.variant = "lo", "hi"
	need := synthViewNeeds(p.view)
	lo.an.Warm(need, yield)
	hi.an.Warm(need, yield)
	if viewMovePx(p.view, lo, hi, relevanceViewW, relevanceViewH) >= viewMinMovePx {
		return true, nil
	}
	lo.an.Warm(soundview.NeedAll, yield)
	hi.an.Warm(soundview.NeedAll, yield)
	_, px := chooseDialView(p.view, lo, hi, relevanceViewW, relevanceViewH)
	return px >= viewMinMovePx, nil
}

// measureDialRelevance returns the keys of the probes whose sweep is inaudible
// (the dials to hide), sorted.
func measureDialRelevance(id instrument.ID, doc instrument.Doc, probes []dialProbe, render graphRenderFunc) ([]string, error) {
	var hidden []string
	for _, p := range probes {
		ok, err := probeRelevant(id, doc, p, render, nil)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", id, p.key, err)
		}
		if !ok {
			hidden = append(hidden, p.key)
		}
	}
	sort.Strings(hidden)
	return hidden, nil
}

// --- the generated table of shipped sounds ---

// dialRelevanceFile is synth_dial_relevance.gen.json: per shipped instrument,
// its sound signature and the dials the DSP found inert on it.
type dialRelevanceFile struct {
	schema.Envelope
	Instruments []dialRelevanceEntry `json:"instruments"`
}

type dialRelevanceEntry struct {
	ID        instrument.ID `json:"id"`
	Signature string        `json:"signature"` // audio.SoundSignature, hex
	Hidden    []string      `json:"hidden,omitempty"`
	// Source "demo": a startup-demo sound measured ahead of time (a cache —
	// the demo is hand-edited; an edited sound is measured at runtime and
	// nothing goes stale). Empty: a shipped instrument, kept current by
	// TestDialRelevanceTableIsCurrent.
	Source string `json:"source,omitempty"`
}

var (
	dialRelevanceOnce  sync.Once
	dialRelevanceTable map[uint64][]string
)

// shippedDialRelevance returns the generated verdict for a sound signature.
func shippedDialRelevance(sig uint64) ([]string, bool) {
	dialRelevanceOnce.Do(func() {
		dialRelevanceTable = map[uint64][]string{}
		var f dialRelevanceFile
		if err := json.Unmarshal(assets.SynthDialRelevanceJSON, &f); err != nil {
			return // no table: every verdict is measured at runtime
		}
		for _, e := range f.Instruments {
			if s, err := strconv.ParseUint(e.Signature, 16, 64); err == nil {
				dialRelevanceTable[s] = e.Hidden
			}
		}
	})
	h, ok := dialRelevanceTable[sig]
	return h, ok
}

func soundSignatureHex(sig uint64) string { return strconv.FormatUint(sig, 16) }

// --- the anchor, per instrument and circuit ---

// dialRelevanceState is the DrumView's anchors: the sound signature each
// instrument had when the Synth tab first showed it in the open circuit.
type dialRelevanceState struct {
	gen     uint64
	anchors map[instrument.ID]uint64
}

// synthDialHidden returns the dials to hide on inst (nil while no verdict is
// known) and the anchor's signature. At the first sight of inst in this
// circuit it anchors the current sound and, when no generated verdict knows
// it, queues the measurement of the dials probes lists (resolved on that very
// sound — probes is only called then).
func (dv *DrumView) synthDialHidden(inst instrument.ID, probes func() []dialProbe) (hidden []string, sig uint64, known bool) {
	st := &dv.dialRelevance
	if gen := audio.CircuitGeneration(); st.anchors == nil || st.gen != gen {
		st.gen, st.anchors = gen, map[instrument.ID]uint64{}
	}
	sig, anchored := st.anchors[inst]
	var doc instrument.Doc
	if !anchored {
		d, ok := audio.EnsureInstrumentDoc(inst)
		if !ok {
			return nil, 0, false
		}
		doc, sig = d.Clone(), audio.SoundSignature(d)
		st.anchors[inst] = sig
	}
	if h, ok := shippedDialRelevance(sig); ok {
		return h, sig, true
	}
	pipe := dv.synthGraphPipe()
	if h, ok := pipe.relevanceVerdict(sig); ok {
		return h, sig, true
	}
	if !anchored {
		pipe.measureRelevance(sig, inst, doc, probes())
	}
	return nil, sig, false
}

// withoutDials returns macros minus the dials named in hidden (the cached
// groups are never mutated).
func withoutDials(macros []macroGroup, hidden []string) []macroGroup {
	if len(hidden) == 0 {
		return macros
	}
	drop := make(map[string]bool, len(hidden))
	for _, k := range hidden {
		drop[k] = true
	}
	out := make([]macroGroup, len(macros))
	for i, cg := range macros {
		out[i] = cg
		kept := cg.Dials[:0:0]
		for _, d := range cg.Dials {
			if !drop[dialKey(cg.ID, d.Dial.ID)] {
				kept = append(kept, d)
			}
		}
		out[i].Dials = kept
	}
	return out
}
