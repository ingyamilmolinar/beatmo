// Package songrender renders a Beatmo template circuit offline to a master
// waveform + per-instrument stems. The arrangement (template -> note events via
// the engine predictor) is pure Go and testable; rendering is build-tag split
// (render_stub.go for tests, render_prod.go for the faithful audio-engine path).
package songrender

import (
	"encoding/json"
	"fmt"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

type InstrumentSpec struct {
	Name    string             `json:"name"`
	ID      instrument.ID      `json:"id"`
	Kind    string             `json:"kind"`
	Volume  float64            `json:"volume"`
	Origin  int                `json:"origin"`
	Effects []audio.EffectSlot `json:"effects,omitempty"`
	// Doc is the instrument's document from the file's instrument_docs (nil =
	// the instrument plays its library copy). Filled by ParseTemplate.
	Doc *instrument.Doc `json:"-"`
}

type NodeSpec struct {
	ID        int             `json:"id"`
	I         int             `json:"i"`
	J         int             `json:"j"`
	Type      model.NodeType  `json:"type"`
	Inputs    []int           `json:"inputs,omitempty"`
	Outputs   []int           `json:"outputs,omitempty"`
	Volume    float64         `json:"volume,omitempty"`
	Pitch     float64         `json:"pitch,omitempty"`
	Duration  float64         `json:"duration,omitempty"`
	LogicKind model.LogicKind `json:"logic_kind,omitempty"`
	LogicN    int             `json:"logic_n,omitempty"`
	LogicP    float64         `json:"logic_p,omitempty"`

	// GrooveKind/GroovePct move this node off the grid by a fraction of one
	// subdivision ("delay" late, "rush" early) — swing, tuplets and grace
	// notes in the generated templates live here.
	GrooveKind model.GrooveKind `json:"groove_kind,omitempty"`
	GroovePct  float64          `json:"groove_pct,omitempty"`

	// Chord is a node's chord spec (the project file's "chord" block).
	Chord *ChordSpec `json:"chord,omitempty"`
}

// ChordSpec mirrors the project file's node chord block.
type ChordSpec struct {
	Quality   string `json:"quality"`
	Inversion int    `json:"inversion,omitempty"`
	Spread    int    `json:"spread,omitempty"`
}

func (c *ChordSpec) model() model.ChordSpec {
	if c == nil {
		return model.ChordSpec{}
	}
	return model.NormalizeChord(model.ChordSpec{Quality: c.Quality, Inversion: c.Inversion, Spread: c.Spread})
}

type ParsedTemplate struct {
	BPM int `json:"bpm"`
	// Subdiv is grid units per BEAT — the engine's MaxDiv — so one path step
	// lasts 60/(BPM*Subdiv) seconds.
	Subdiv      int              `json:"subdiv"`
	Instruments []InstrumentSpec `json:"instruments"`
	Nodes       []NodeSpec       `json:"nodes"`
	// InstrumentDocs are the file's instrument documents, keyed by id: a full
	// document, or (built-in templates) {"ref":"sha256:…"} resolved against the
	// instrument pack by ParseTemplate.
	InstrumentDocs map[instrument.ID]instrument.Doc  `json:"-"`
	RawDocs        map[instrument.ID]json.RawMessage `json:"instrument_docs,omitempty"`
}

// ParseTemplate unmarshals a v1 template JSON into a ParsedTemplate.
func ParseTemplate(b []byte) (ParsedTemplate, error) {
	var pt ParsedTemplate
	if err := json.Unmarshal(b, &pt); err != nil {
		return ParsedTemplate{}, fmt.Errorf("songrender: parse template: %w", err)
	}
	if pt.Subdiv <= 0 {
		pt.Subdiv = 16 // schema default
	}
	if len(pt.RawDocs) > 0 {
		pt.InstrumentDocs = make(map[instrument.ID]instrument.Doc, len(pt.RawDocs))
	}
	for id, raw := range pt.RawDocs {
		var ref struct {
			Ref string `json:"ref"`
		}
		if err := json.Unmarshal(raw, &ref); err == nil && ref.Ref != "" {
			d, ok := audio.PackDoc(ref.Ref)
			if !ok {
				return ParsedTemplate{}, fmt.Errorf("songrender: instrument_docs.%s: reference %s is not in the instrument pack", id, ref.Ref)
			}
			d.ID, d.Origin = id, id.Base()
			pt.InstrumentDocs[id] = d
			continue
		}
		var d instrument.Doc
		if err := json.Unmarshal(raw, &d); err != nil {
			return ParsedTemplate{}, fmt.Errorf("songrender: instrument_docs.%s: %w", id, err)
		}
		pt.InstrumentDocs[id] = d
	}
	for i := range pt.Instruments {
		if d, ok := pt.InstrumentDocs[pt.Instruments[i].ID]; ok {
			d := d
			pt.Instruments[i].Doc = &d
		}
	}
	return pt, nil
}

// LoadTemplate finds the embedded built-in template whose stem == genre and parses it.
func LoadTemplate(stem string) (ParsedTemplate, error) {
	for _, t := range assets.Templates() {
		if t.Genre == stem {
			return ParseTemplate(t.Bytes)
		}
	}
	return ParsedTemplate{}, fmt.Errorf("songrender: no template with stem %q", stem)
}
