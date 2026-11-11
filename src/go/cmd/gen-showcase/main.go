package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
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
	Inst instrument.ID
	Hits []Hit

	// Layer (0 melody, 1 groove) and Kit (a percussion row) record where the
	// row came from. Generator metadata for the fidelity tests; not exported.
	Layer int
	Kit   bool
}
type InstSpec struct {
	// Colour is NOT a field: build() assigns every row its colour from
	// templates.InstrumentSequence by row index, so a per-spec colour would be
	// silently overridden.
	ID      instrument.ID
	Name    string
	Volume  float64
	Effects []EffectSpec // insert effects (filters) applied at import
}

// EffectSpec is a per-instrument insert filter. Mode 0 = lowpass, 1 = highpass.
type EffectSpec struct {
	Mode   int
	Cutoff float64
	Q      float64
}

type Showcase struct {
	Stem        string
	BPM, Subdiv int // Subdiv = steps per four quarter notes
	// TotalSteps is the loop length: the excerpt's NOTATED length in steps,
	// so a circuit loops in meter (a 15-bar 2/4 excerpt is 30 quarters, not
	// rounded up to whole 4/4 bars with a hole at the end).
	TotalSteps int
	Insts      []InstSpec
	Rows       []RowSpec // row i uses Insts[i] (1:1, same order)
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
	Name    string        `json:"name"`
	ID      instrument.ID `json:"id"`
	Kind    string        `json:"kind"`
	Volume  float64       `json:"volume"`
	Origin  int           `json:"origin"`
	Color   string        `json:"color"`
	Effects []effectJSON  `json:"effects,omitempty"`
}

type effectJSON struct {
	Type    string             `json:"type"`
	Enabled bool               `json:"enabled"`
	Params  map[string]float64 `json:"params"`
}
type doc struct {
	schema.Envelope
	Subdiv      int    `json:"subdiv"`
	BPM         int    `json:"bpm"`
	Instruments []inst `json:"instruments"`
	Nodes       []node `json:"nodes"`
	// InstrumentDocs pins every row's instrument to the pack document it
	// plays, by content hash (instance ids "x--N" pin x's document), so a
	// template sounds the same whatever the user has saved over its
	// instruments.
	InstrumentDocs map[instrument.ID]docRef `json:"instrument_docs"`
}

// docRef is a project instrument_docs value in its reference form.
type docRef struct {
	Ref string `json:"ref"`
}

// pinInstrument returns the pack reference for a template row's instrument id:
// the content hash of its base id's SHIPPED document.
func pinInstrument(instID instrument.ID) (docRef, error) {
	base := instID.Base()
	d, ok := audio.ShippedDoc(base)
	if !ok {
		return docRef{}, fmt.Errorf("template row %q: %q is not a shipped instrument", instID, base)
	}
	return docRef{Ref: instrument.ContentHash(d)}, nil
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
	totalSteps := s.TotalSteps
	if totalSteps <= 0 || totalSteps%4 != 0 {
		return nil, fmt.Errorf("showcase %q: %d steps — the loop is laid out as a square, so it must be a positive multiple of 4", s.Stem, totalSteps)
	}
	// ONE grid unit per step, and the exported "subdiv" (units per BEAT to the
	// engine) is therefore steps per quarter.
	//
	// The engine's groove window — the span a node's delay/rush percentage is a
	// fraction of — is one grid unit (nodeNudgeSecondsAt). Templates used to
	// lay four units per step, so a 100% delay was a quarter of a sixteenth and
	// swing (an offbeat eighth sits two thirds of a step late at 2:1) could not
	// be written at all. With one unit per step the window is the step itself.
	// On screen nothing moves: a unit is Step/MaxDiv, so a step is still a
	// quarter of a beat wide at 16 steps per bar.
	// Guard: TestGrooveWindowIsOneStep.
	const unitsPerStep = 1
	stepsPerQuarter := s.Subdiv / 4
	perimeter := totalSteps * unitsPerStep
	side := perimeter / 4
	// Rows sit half a beat apart, as they always have.
	rowGap := stepsPerQuarter * unitsPerStep / 2
	var d doc
	d.Envelope, d.Subdiv, d.BPM = schema.Current(), stepsPerQuarter*unitsPerStep, s.BPM
	idBase := 0
	idSeen := map[instrument.ID]int{}
	for ri, row := range s.Rows {
		baseJ := ri * (side + rowGap)
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
			var err error
			if instID, err = instrument.Instance(ins.ID, n); err != nil {
				return nil, fmt.Errorf("showcase %q: %w", s.Stem, err)
			}
		}
		// Auto-assign color from canonical InstrumentSequence (row index wraps),
		// overriding any spec-level color. This ensures every generated template
		// satisfies TestTemplateColorsDeriveFromSequence without per-spec hex literals.
		seq := templates.InstrumentSequence
		color := seq[ri%len(seq)]
		var fx []effectJSON
		for _, e := range ins.Effects {
			fx = append(fx, effectJSON{
				Type:    "filter",
				Enabled: true,
				Params:  map[string]float64{"mode": float64(e.Mode), "cutoff": e.Cutoff, "q": e.Q, "mix": 1},
			})
		}
		ref, err := pinInstrument(instID)
		if err != nil {
			return nil, err
		}
		if d.InstrumentDocs == nil {
			d.InstrumentDocs = map[instrument.ID]docRef{}
		}
		d.InstrumentDocs[instID] = ref
		d.Instruments = append(d.Instruments, inst{
			Name: ins.Name, ID: instID, Kind: "builtin", Volume: ins.Volume,
			Origin: rowNodes[0].ID, Color: color,
			Effects: fx,
		})
		d.Nodes = append(d.Nodes, rowNodes...)
		idBase += nn
	}
	return encodeDoc(d)
}

// encodeDoc writes a template as compact JSON with one instrument or node per
// line. The templates are embedded verbatim in every shipped binary, and
// indented JSON cost 0.77 MiB of raw WASM once the groove templates landed —
// enough to break TestWasmAssetSizeBudget's raw budget on its own. One object
// per line keeps a regenerated template's diff readable node by node.
func encodeDoc(d doc) ([]byte, error) {
	head, err := json.Marshal(struct {
		schema.Envelope
		Subdiv int `json:"subdiv"`
		BPM    int `json:"bpm"`
	}{d.Envelope, d.Subdiv, d.BPM})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	b.Write(head[:len(head)-1])
	list := func(key string, n int, at func(int) any) error {
		fmt.Fprintf(&b, ",%q:[\n", key)
		for i := 0; i < n; i++ {
			e, err := json.Marshal(at(i))
			if err != nil {
				return err
			}
			b.Write(e)
			if i < n-1 {
				b.WriteByte(',')
			}
			b.WriteByte('\n')
		}
		b.WriteByte(']')
		return nil
	}
	if err := list("instruments", len(d.Instruments), func(i int) any { return d.Instruments[i] }); err != nil {
		return nil, err
	}
	if err := list("nodes", len(d.Nodes), func(i int) any { return d.Nodes[i] }); err != nil {
		return nil, err
	}
	b.WriteString(",\"instrument_docs\":{\n")
	ids := slices.Sorted(maps.Keys(d.InstrumentDocs))
	for i, id := range ids {
		e, err := json.Marshal(d.InstrumentDocs[id])
		if err != nil {
			return nil, err
		}
		fmt.Fprintf(&b, "%q:%s", id, e)
		if i < len(ids)-1 {
			b.WriteByte(',')
		}
		b.WriteByte('\n')
	}
	b.WriteString("}}")
	return b.Bytes(), nil
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
