// Package synthdial models the Synth tab's macro dials: METADATA, terms over existing params, each an ABSOLUTE span
// (value at dial 0 and 100), so a dial's Reading is a pure function of the document and nothing is ever stored. Turn
// moves every term proportionally, keeping each one's offset. New dials are data (synth_dials.json); a new Space needs Go.
package synthdial

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// Space selects how a term's value is laid out along its span.
type Space string

const (
	// SpaceLinear lays the raw value out evenly (the default when omitted).
	SpaceLinear Space = "linear"
	// SpaceLog lays the value out in ln space — for Hz- and time-like params
	// whose musical travel is logarithmic. Both span ends must be > 0.
	SpaceLog Space = "log"
	// |v| with the value's own sign kept on write (a −5 st drop gets MORE negative as the dial rises); zero takes +.
	SpaceMagnitude Space = "magnitude"
)

// MaxDialsPerCard caps the dials a card shows for one instrument.
const MaxDialsPerCard = 6

// Term is one component of a dial: a param and the absolute span it travels
// as the dial goes from 0 to 100.
type Term struct {
	Param string `json:"param"`
	// From / To are the param's values at dial 0 and dial 100 (omitted = the
	// param's range ends; magnitude: 0 and the larger edge). To < From means
	// the param FALLS as the dial rises.
	From *float64 `json:"from,omitempty"`
	To   *float64 `json:"to,omitempty"`
	// Space lays the value out along the span (linear when omitted).
	Space Space `json:"space,omitempty"`
	// Weight is the term's influence on the dial's reading (> 0; omitted = 1).
	Weight float64 `json:"weight,omitempty"`
	// A param that must be NON-ZERO for this term to be audible: a selector whose 0 is Off, or a level gating its own
	// shaping.
	Requires string `json:"requires,omitempty"`
	// When names an engine condition (registered by the caller) the term is
	// audible under — for liveness that no single param expresses, such as
	// "the unison path runs". The caller's liveness check reads both.
	When string `json:"when,omitempty"`
	// Make Requires a threshold (an LFO's speed is inaudible until its depth is); with neither it means non-zero.
	RequiresMin   float64 `json:"requires_min,omitempty"`
	RequiresBelow float64 `json:"requires_below,omitempty"`
}

// W is the term's reading weight (1 when omitted).
func (t Term) W() float64 {
	if t.Weight == 0 {
		return 1
	}
	return t.Weight
}

// RequirementMet reports whether the term's Requires gate holds for the
// required param's current value v (always true when the term has none).
func (t Term) RequirementMet(v float64) bool {
	if t.Requires == "" {
		return true
	}
	if t.RequiresMin == 0 && t.RequiresBelow == 0 {
		return v != 0
	}
	if t.RequiresMin != 0 && v < t.RequiresMin {
		return false
	}
	if t.RequiresBelow != 0 && v >= t.RequiresBelow {
		return false
	}
	return true
}

// space resolves the default.
func (t Term) space() Space {
	if t.Space == "" {
		return SpaceLinear
	}
	return t.Space
}

// Dial is one macro dial: presentation (i18n keys), the cards it sits on, the
// descriptor it must move (for the real-render direction test, "" = only an
// audible change is required), and its terms.
type Dial struct {
	ID     string   `json:"id"`
	Cards  []string `json:"cards"`
	Label  string   `json:"label"`
	Gloss  string   `json:"gloss"`
	Metric string   `json:"metric"`
	// MetricFalls declares that Metric DROPS as the dial rises (Snap shortens
	// the attack time); otherwise it rises.
	MetricFalls bool `json:"metric_falls,omitempty"`
	// "start": the dial's 0 is a natural OFF, so it may read 0 on most instruments, and its span starts at the neutral value.
	Rest string `json:"rest,omitempty"`
	// One of Views; "lfo-target" resolves against the live document (Amp → loudness, Pitch → pitch, Cutoff → colour-time).
	View string `json:"view"`
	// Lo / Hi are i18n keys for the words the graph labels the dial's two
	// ends with ("dull" / "bright").
	Lo    string `json:"lo"`
	Hi    string `json:"hi"`
	Terms []Term `json:"terms"`
}

// Views are the graph views a dial may name.
var Views = []string{"loudness", "attack", "pitch", "colour", "harmonics", "colour-time", "noise", "wave", "lfo-target"}

// OnCard reports whether the dial sits on the named card.
func (d Dial) OnCard(card string) bool {
	return slices.Contains(d.Cards, card)
}

// File is the decoded dial document. Dial order is display order.
type File struct {
	schema.Envelope
	Dials []Dial `json:"dials"`
}

// Unknown fields are rejected: a typo in hand-edited metadata must not silently drop a term.
func Load(b []byte) (File, error) {
	var f File
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("synth dials: %w", err)
	}
	if err := f.Envelope.Validate("synth dials"); err != nil {
		return File{}, err
	}
	if err := f.Validate(); err != nil {
		return File{}, err
	}
	return f, nil
}

// Validate checks the structural rules of a decoded file.
func (f File) Validate() error {
	seen := map[string]bool{}
	for i, d := range f.Dials {
		if d.ID == "" {
			return fmt.Errorf("synth dials: dial #%d has no id", i)
		}
		if seen[d.ID] {
			return fmt.Errorf("synth dials: duplicate dial id %q", d.ID)
		}
		seen[d.ID] = true
		if len(d.Cards) == 0 {
			return fmt.Errorf("synth dials: dial %q sits on no card", d.ID)
		}
		if d.Label == "" || d.Gloss == "" {
			return fmt.Errorf("synth dials: dial %q needs a label and a gloss key", d.ID)
		}
		if len(d.Terms) == 0 {
			return fmt.Errorf("synth dials: dial %q has no terms", d.ID)
		}
		if d.Rest != "" && d.Rest != "start" {
			return fmt.Errorf("synth dials: dial %q has unknown rest %q", d.ID, d.Rest)
		}
		if !slices.Contains(Views, d.View) {
			return fmt.Errorf("synth dials: dial %q names unknown view %q (want one of %v)", d.ID, d.View, Views)
		}
		if d.Lo == "" || d.Hi == "" {
			return fmt.Errorf("synth dials: dial %q needs an end word key for each end (lo, hi)", d.ID)
		}
		params := map[string]bool{}
		for _, t := range d.Terms {
			if t.Param == "" {
				return fmt.Errorf("synth dials: dial %q has a term with no param", d.ID)
			}
			if params[t.Param] {
				return fmt.Errorf("synth dials: dial %q lists %q twice", d.ID, t.Param)
			}
			params[t.Param] = true
			if t.Weight < 0 {
				return fmt.Errorf("synth dials: dial %q term %q has a negative weight %v", d.ID, t.Param, t.Weight)
			}
			if (t.RequiresMin != 0 || t.RequiresBelow != 0) && t.Requires == "" {
				return fmt.Errorf("synth dials: dial %q term %q sets requires_min/requires_below without requires", d.ID, t.Param)
			}
			if t.From != nil && t.To != nil && *t.From == *t.To {
				return fmt.Errorf("synth dials: dial %q term %q has an empty span (from = to = %v)", d.ID, t.Param, *t.From)
			}
			switch t.space() {
			case SpaceLog:
				if t.From == nil || t.To == nil || *t.From <= 0 || *t.To <= 0 {
					return fmt.Errorf("synth dials: dial %q term %q: a log term needs an explicit span with both ends > 0", d.ID, t.Param)
				}
			case SpaceMagnitude:
				if (t.From != nil && *t.From < 0) || (t.To != nil && *t.To < 0) {
					return fmt.Errorf("synth dials: dial %q term %q: a magnitude span is in |value| (ends ≥ 0)", d.ID, t.Param)
				}
			}
			switch t.space() {
			case SpaceLinear, SpaceLog, SpaceMagnitude:
			default:
				return fmt.Errorf("synth dials: dial %q term %q has unknown space %q", d.ID, t.Param, t.Space)
			}
		}
	}
	return nil
}
