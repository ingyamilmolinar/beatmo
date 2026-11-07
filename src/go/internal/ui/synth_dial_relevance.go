package ui

// A dial shows only when its 0→100 sweep is audible (soundview.CompareNotes) - no exception list. Judged on the
// circuit's ANCHOR sound so edits never re-hide a dial; a verdict comes from the generated table or a background measure.

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

// The end values are the same on every instrument.
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

// Relevant = audible AND some view shows it: a change only in quiet cells no picture shows helps no one.
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

// Sorted keys of the dials to hide.
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

type dialRelevanceFile struct {
	schema.Envelope
	Instruments []dialRelevanceEntry `json:"instruments"`
}

type dialRelevanceEntry struct {
	ID        instrument.ID `json:"id"`
	Signature string        `json:"signature"` // audio.SoundSignature, hex
	Hidden    []string      `json:"hidden,omitempty"`
	// "demo" marks a cache of startup-demo sounds (hand-edited; an edited sound is measured at runtime).
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

type dialRelevanceState struct {
	gen     uint64
	anchors map[instrument.ID]uint64
}

// nil while no verdict is known. probes is called only on first sight, on that very sound.
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

// The cached groups are never mutated.
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
