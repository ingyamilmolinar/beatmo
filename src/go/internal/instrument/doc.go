// Package instrument holds the instrument document (ONE per instrument, every value its voice depends on) and pure
// functions over it; a *Registry of module types is passed in, nothing touches audio or globals.
package instrument

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// FileFormat names this document type in version errors.
const FileFormat = "instrument"

// Values maps a module's value names (ABI field names; slot-relative for
// layers) to numbers.
type Values map[string]float64

func (v Values) clone() Values { return maps.Clone(v) }

// Stage is a pipeline stage. Its type is its key in Doc.Stages.
type Stage struct {
	V      int    `json:"v"`
	On     bool   `json:"on"`
	Values Values `json:"values"`
	Table  *Table `json:"table,omitempty"` // only for a type that declares one (TableDecl)
	Notes  string `json:"notes,omitempty"`
}

// Module is a source, layer or voice-FX module.
type Module struct {
	Type   string `json:"type"`
	V      int    `json:"v"`
	On     bool   `json:"on"`
	Values Values `json:"values"`
	Notes  string `json:"notes,omitempty"`
}

// Seconds is a fixed time, never beats: a note sounds the same at every tempo and on every platform. Amplitude
// includes loudness; GateTail 0 = no gate.
type Render struct {
	Seconds   float64 `json:"seconds"`
	Amplitude float64 `json:"amplitude"`
	GateTail  float64 `json:"gate_tail"`
	Melodic   bool    `json:"melodic"`
}

// Doc is one instrument. Layers[i] is engine generator slot i+1; nil = empty.
type Doc struct {
	ID       ID               `json:"id"`
	Origin   ID               `json:"origin,omitempty"`
	Name     string           `json:"name"`
	Category string           `json:"category"`
	Notes    string           `json:"notes,omitempty"`
	Render   Render           `json:"render"`
	Stages   map[string]Stage `json:"stages"`
	Source   *Module          `json:"source,omitempty"`
	Layers   []*Module        `json:"layers,omitempty"`
	FX       []Module         `json:"fx,omitempty"`
}

// Clone returns a deep copy: no map or module is shared with d.
func (d Doc) Clone() Doc {
	out := d
	out.Stages = make(map[string]Stage, len(d.Stages))
	for k, s := range d.Stages {
		s.Values = s.Values.clone()
		s.Table = s.Table.clone()
		out.Stages[k] = s
	}
	if d.Source != nil {
		m := *d.Source
		m.Values = m.Values.clone()
		out.Source = &m
	}
	if d.Layers != nil {
		out.Layers = make([]*Module, len(d.Layers))
		for i, l := range d.Layers {
			if l != nil {
				m := *l
				m.Values = m.Values.clone()
				out.Layers[i] = &m
			}
		}
	}
	if d.FX != nil {
		out.FX = make([]Module, len(d.FX))
		for i, m := range d.FX {
			m.Values = m.Values.clone()
			out.FX[i] = m
		}
	}
	return out
}

// File is a standalone document on disk: the schema envelope plus the doc.
type File struct {
	schema.Envelope
	Doc
}

// Strict (unknown fields are an error) with the schema version checked; not Validated: callers Migrate then Validate.
func ParseFile(data []byte) (Doc, error) {
	var f File
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return Doc{}, fmt.Errorf("instrument: %w", err)
	}
	if err := f.Envelope.Validate(FileFormat); err != nil {
		return Doc{}, err
	}
	return f.Doc, nil
}

// No HTML escaping, so notes stay readable ("->", not ">").
func MarshalFile(d Doc) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(File{Envelope: schema.Current(), Doc: d}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
