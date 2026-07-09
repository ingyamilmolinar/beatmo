package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/templates"
)

type Hit struct {
	Step  int
	Pitch float64
	Vol   float64 // 0 => omit (use instrument default)
	Dur   float64 // 0 => omit
	// Groove shifts this hit off the grid: "delay" (behind the beat) or
	// "rush" (ahead of it), by GroovePct (0..1) of the groove window.
	// Empty = straight. Matches node JSON groove_kind/groove_pct
	// (importer: internal/ui/import.go).
	Groove    string
	GroovePct float64
}
type RowSpec struct {
	Inst string // instrument id
	Hits []Hit
}
type InstSpec struct {
	ID, Name, Color string
	Volume, Pan     float64
	ReverbSend      float64
	SynthParams     map[string]float64
	Effects         []EffectSpec // insert effects (filters) applied at import
}

// EffectSpec is a per-instrument insert filter. Mode 0 = lowpass, 1 = highpass.
type EffectSpec struct {
	Mode   int
	Cutoff float64
	Q      float64
}

// seededModularInstruments lists the renderModular instruments whose no-edit
// (legacy) render is the GENERIC identity modular tone — NOT their seed. To make
// a template instrument actually sound like itself (organ/violin/sax/…), the
// emitted JSON must carry a non-empty synth_params so the trigger takes the
// recipe path (which applies the seed via MergeRecipeDefaults). A single
// {"pitch":0} entry suffices; the per-node pitch overrides it at play time.
// Drums (bespoke renders) and fm-* (FM renders) sound correct on the legacy path
// and are intentionally excluded.
var seededModularInstruments = map[string]bool{
	"organ": true, "sax": true,
	"bass-guitar": true, "bass-acid": true, "bass-reese": true, "bass-fm": true, "bass-808": true,
	"piano-grand": true, "piano-felt": true,
	"violin": true, "violin-ensemble": true, "cello": true, "cello-warm": true,
	"guitar-nylon": true, "guitar-nylon-bright": true, "guitar-steel": true, "guitar-steel-warm": true,
	"guitar-electric": true, "guitar-electric-neck": true,
	"flute": true, "flute-breathy": true, "oboe": true, "oboe-full": true,
	"trumpet": true, "trumpet-mellow": true, "french-horn": true, "french-horn-loud": true,
	"conga": true, "conga-open": true, "conga-tumba": true,
	// Remaining modularInstrumentDefs rows (audio/modular_instruments.go) — same
	// renderModular identity-tone hazard as the rest of the table.
	"modular-pad": true, "organ-church": true, "scifi-lead": true, "harp": true,
	"ensemble-lead": true, "ensemble-lead-dark": true, "voice-soprano": true,
	"ghost-bass": true, "viola-pad": true, "voice-whisper": true, "modular": true,
}

type Showcase struct {
	Stem        string
	BPM, Subdiv int
	Bars        int
	Insts       []InstSpec
	Rows        []RowSpec // row i uses Insts[i] (1:1, same order)
}

type node struct {
	ID       int     `json:"id"`
	I        int     `json:"i"`
	J        int     `json:"j"`
	Type     string  `json:"type"`
	Outputs  []int   `json:"outputs,omitempty"`
	Pitch    float64 `json:"pitch,omitempty"`
	Volume   float64 `json:"volume,omitempty"`
	Duration float64 `json:"duration,omitempty"`

	GrooveKind string  `json:"groove_kind,omitempty"`
	GroovePct  float64 `json:"groove_pct,omitempty"`
}
type inst struct {
	Name        string             `json:"name"`
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Volume      float64            `json:"volume"`
	Origin      int                `json:"origin"`
	Color       string             `json:"color"`
	Pan         float64            `json:"pan,omitempty"`
	ReverbSend  float64            `json:"reverb_send,omitempty"`
	SynthParams map[string]float64 `json:"synth_params,omitempty"`
	Effects     []effectJSON       `json:"effects,omitempty"`
}

type effectJSON struct {
	Type    string             `json:"type"`
	Enabled bool               `json:"enabled"`
	Params  map[string]float64 `json:"params"`
}
type doc struct {
	Version     int    `json:"version"`
	Subdiv      int    `json:"subdiv"`
	BPM         int    `json:"bpm"`
	Instruments []inst `json:"instruments"`
	Nodes       []node `json:"nodes"`
}

func perimeterPoint(p, side int) (int, int) {
	switch {
	case p < side:
		return p, 0
	case p < 2*side:
		return side, p - side
	case p < 3*side:
		return 3*side - p, side
	default:
		return 0, 4*side - p
	}
}

// build returns the JSON bytes for one showcase circuit.
func build(s Showcase) ([]byte, error) {
	totalSteps := s.Bars * 16
	unitsPerStep := s.Subdiv / 4
	perimeter := totalSteps * unitsPerStep
	side := perimeter / 4
	var d doc
	d.Version, d.Subdiv, d.BPM = 1, s.Subdiv, s.BPM
	idBase := 0
	idSeen := map[string]int{}
	for ri, row := range s.Rows {
		baseJ := ri * (side + 8)
		hitByStep := map[int]Hit{}
		for _, h := range row.Hits {
			if _, dup := hitByStep[h.Step]; dup {
				panic(fmt.Sprintf("showcase %q row %d (inst %q): duplicate hit at step %d — the model is one node per step per row; voice chords as consecutive-step spreads (chordAt) or separate rows", s.Stem, ri, row.Inst, h.Step))
			}
			hitByStep[h.Step] = h
		}
		stepSet := map[int]bool{0: true, totalSteps / 4: true, totalSteps / 2: true, 3 * totalSteps / 4: true}
		for st := range hitByStep {
			stepSet[st] = true
		}
		steps := make([]int, 0, len(stepSet))
		for st := range stepSet {
			steps = append(steps, st)
		}
		sort.Ints(steps)
		rowNodes := make([]node, 0, len(steps))
		for k, st := range steps {
			i, j := perimeterPoint(st*unitsPerStep, side)
			n := node{ID: idBase + k, I: i, J: baseJ + j, Type: "silent"}
			if h, ok := hitByStep[st]; ok {
				n.Type = "regular"
				n.Pitch = h.Pitch
				n.Volume = h.Vol
				n.Duration = h.Dur
				if h.Groove != "" {
					if h.Groove != "delay" && h.Groove != "rush" {
						return nil, fmt.Errorf("showcase %q row %d (inst %q) step %d: unknown groove kind %q (want delay|rush)", s.Stem, ri, row.Inst, st, h.Groove)
					}
					if h.GroovePct < 0 || h.GroovePct > 1 {
						return nil, fmt.Errorf("showcase %q row %d (inst %q) step %d: groove_pct %v out of range 0..1", s.Stem, ri, row.Inst, st, h.GroovePct)
					}
					n.GrooveKind = h.Groove
					n.GroovePct = h.GroovePct
				}
			}
			rowNodes = append(rowNodes, n)
		}
		nn := len(rowNodes)
		for k := range rowNodes {
			rowNodes[k].Outputs = []int{rowNodes[(k+1)%nn].ID}
		}
		ins := s.Insts[ri]
		instID := ins.ID
		idSeen[ins.ID]++
		if n := idSeen[ins.ID]; n > 1 {
			instID = fmt.Sprintf("%s-%d", ins.ID, n)
		}
		// Auto-assign color from canonical InstrumentSequence (row index wraps),
		// overriding any spec-level color. This ensures every generated template
		// satisfies TestTemplateColorsDeriveFromSequence without per-spec hex literals.
		seq := templates.InstrumentSequence
		color := seq[ri%len(seq)]
		// Seed trigger: seeded modular instruments render the generic identity tone
		// on the no-edit path, so emit a non-empty synth_params to take the recipe
		// (seed) path. The per-node pitch overrides this 0 at trigger time.
		sp := ins.SynthParams
		if sp == nil && seededModularInstruments[ins.ID] {
			sp = map[string]float64{"pitch": 0}
		}
		var fx []effectJSON
		for _, e := range ins.Effects {
			fx = append(fx, effectJSON{
				Type:    "filter",
				Enabled: true,
				Params:  map[string]float64{"mode": float64(e.Mode), "cutoff": e.Cutoff, "q": e.Q, "mix": 1},
			})
		}
		d.Instruments = append(d.Instruments, inst{
			Name: ins.Name, ID: instID, Kind: "builtin", Volume: ins.Volume,
			Origin: rowNodes[0].ID, Color: color, Pan: ins.Pan,
			ReverbSend: ins.ReverbSend, SynthParams: sp, Effects: fx,
		})
		d.Nodes = append(d.Nodes, rowNodes...)
		idBase += nn
	}
	return json.MarshalIndent(d, "", "  ")
}

func main() {
	outDir := filepath.Join("internal", "assets", "templates")
	for _, s := range showcases() {
		b, err := build(s)
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(outDir, s.Stem+".json"), append(b, '\n'), 0o644); err != nil {
			panic(err)
		}
	}
}
