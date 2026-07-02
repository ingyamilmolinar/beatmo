// Package songrender renders a Beatmo template circuit offline to a master
// waveform + per-instrument stems. The arrangement (template -> note events via
// the engine predictor) is pure Go and testable; rendering is build-tag split
// (render_stub.go for tests, render_prod.go for the faithful audio-engine path).
package songrender

import (
	"encoding/json"
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

type InstrumentSpec struct {
	Name       string             `json:"name"`
	ID         string             `json:"id"`
	Kind       string             `json:"kind"`
	Volume     float64            `json:"volume"`
	Pan        float64            `json:"pan,omitempty"`
	ReverbSend float64            `json:"reverb_send,omitempty"`
	DelaySend  float64            `json:"delay_send,omitempty"`
	Origin     int                `json:"origin"`
	Recipe     string             `json:"recipe,omitempty"`
	SynthParams map[string]float64 `json:"synth_params,omitempty"`
	Effects    []audio.EffectSlot `json:"effects,omitempty"`
}

type NodeSpec struct {
	ID        int     `json:"id"`
	I         int     `json:"i"`
	J         int     `json:"j"`
	Type      string  `json:"type"`
	Inputs    []int   `json:"inputs,omitempty"`
	Outputs   []int   `json:"outputs,omitempty"`
	Volume    float64 `json:"volume,omitempty"`
	Pitch     float64 `json:"pitch,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	LogicKind string  `json:"logic_kind,omitempty"`
	LogicN    int     `json:"logic_n,omitempty"`
	LogicP    float64 `json:"logic_p,omitempty"`
}

type ParsedTemplate struct {
	BPM         int              `json:"bpm"`
	Subdiv      int              `json:"subdiv"`
	Instruments []InstrumentSpec `json:"instruments"`
	Nodes       []NodeSpec       `json:"nodes"`
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
