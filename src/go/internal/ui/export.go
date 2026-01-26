package ui

import (
	"encoding/json"
	"fmt"
	"image/color"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

// Export schema
type exportFile struct {
	Version     int                `json:"version"`
	Subdiv      int                `json:"subdiv,omitempty"`
	BPM         int                `json:"bpm"`
	Instruments []exportInstrument `json:"instruments"`
	Nodes       []exportNode       `json:"nodes"`
	EQ          *exportEQ          `json:"eq,omitempty"`
}

type exportInstrument struct {
	Name   string    `json:"name"`
	ID     string    `json:"id"`
	Kind   string    `json:"kind"` // builtin|sample
	Volume float64   `json:"volume"`
	Origin int       `json:"origin"`
	Color  string    `json:"color"`          // #RRGGBBAA
	Path   string    `json:"path,omitempty"` // local cache path or object URL for custom samples
	EQ     *exportEQ `json:"eq,omitempty"`   // per-instrument EQ settings
}

type exportNode struct {
	ID      int    `json:"id"`
	I       int    `json:"i"`
	J       int    `json:"j"`
	Type    string `json:"type"` // regular|invisible|silent|mute
	Inputs  []int  `json:"inputs,omitempty"`
	Outputs []int  `json:"outputs,omitempty"`
	// Optional per-node parameters. Omitted when at defaults.
	Volume    float64 `json:"volume,omitempty"`
	Pitch     float64 `json:"pitch,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	SkipEvery int     `json:"skip_every,omitempty"`
	// Optional node logic fields (new). Backwards compatible: older files
	// will ignore these and rely on SkipEvery when applicable.
	LogicKind string  `json:"logic_kind,omitempty"`
	LogicN    int     `json:"logic_n,omitempty"`
	LogicP    float64 `json:"logic_p,omitempty"`
	// Groove: per-node rule (none|delay|rush) and percentage (0..1)
	GrooveKind string  `json:"groove_kind,omitempty"`
	GroovePct  float64 `json:"groove_pct,omitempty"`
}

// exportEQ encodes the master EQ settings. Optional; absent for legacy files.
type exportEQ struct {
	GainsDB   []float64    `json:"gains_db,omitempty"`   // per band gain in dB
	BandsHz   [][2]float64 `json:"bands_hz,omitempty"`
	BandMuted []bool       `json:"band_muted,omitempty"` // per band mute state
}

// kindForID determines the instrument kind by checking catalog metadata.
// WAV-based and embedded samples return "sample", synthesized instruments return "builtin".
func kindForID(id string) string {
	// Legacy "sample-" prefix check for backwards compatibility
	if strings.HasPrefix(id, "sample-") {
		return "sample"
	}
	// Look up in catalog to check Source field
	if meta, ok := audio.CatalogLookup(id); ok {
		switch meta.Source {
		case "wav", "embedded":
			return "sample"
		}
	}
	return "builtin"
}

func hexColor(c color.Color) string {
	r, g, b, a := c.RGBA()
	// r,g,b,a are 0..65535; convert to 0..255
	return fmt.Sprintf("#%02X%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
}

// Export builds the export JSON and triggers a download/save.
func (dv *DrumView) Export() error {
	dv.logger.Infof("[DRUMVIEW] Export requested")
	data, err := dv.exportBytes()
	if err != nil {
		return err
	}
	name := "tunkul-export.json"
	if err := saveJSON(name, data); err != nil {
		dv.logger.Infof("[DRUMVIEW] Export failed: %v", err)
		return err
	}
	dv.logger.Infof("[DRUMVIEW] Export completed: %s (%d bytes)", name, len(data))
	return nil
}

// Save function indirection for platform and tests.
var saveJSON = saveJSONDefault

// currentMaxDiv returns the smallest subdivision per beat for export. Game sets
// this to the live grid's MaxDiv; default to 32.
var currentMaxDiv = func() int { return 32 }

// exportBytes builds the export JSON without saving to disk.
func (dv *DrumView) exportBytes() ([]byte, error) {
	if dv == nil || dv.Graph == nil {
		return nil, fmt.Errorf("no graph")
	}
	g := dv.Graph
	ids := make([]int, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, int(id))
	}
	sort.Ints(ids)

	in := map[int][]int{}
	out := map[int][]int{}
	for e := range g.Edges {
		a := int(e[0])
		b := int(e[1])
		out[a] = append(out[a], b)
		in[b] = append(in[b], a)
	}
	nodes := make([]exportNode, 0, len(ids))
	for _, id := range ids {
		n := g.Nodes[model.NodeID(id)]
		if n.Type == model.NodeTypeInvisible {
			continue
		}
		typ := "regular"
		switch n.Type {
		case model.NodeTypeSilent:
			typ = "silent"
		case model.NodeTypeMute:
			typ = "mute"
		}
		en := exportNode{ID: id, I: n.I, J: n.J, Type: typ}
		if v := in[id]; len(v) > 0 {
			sort.Ints(v)
			en.Inputs = v
		}
		if v := out[id]; len(v) > 0 {
			sort.Ints(v)
			en.Outputs = v
		}
		// Include params only when not defaults to keep JSON compact.
		if p := n.Params; true {
			if p.Volume != 0 && p.Volume != 1 {
				en.Volume = p.Volume
			}
			if p.Pitch != 0 {
				en.Pitch = p.Pitch
			}
			if p.Duration != 0 && p.Duration != 1 {
				en.Duration = p.Duration
			}
			// New logic fields
			if p.LogicKind != "" {
				en.LogicKind = p.LogicKind
				if p.LogicN > 0 {
					en.LogicN = p.LogicN
				}
				if p.LogicP > 0 {
					en.LogicP = p.LogicP
				}
			}
			if p.GrooveKind != "" {
				en.GrooveKind = p.GrooveKind
			}
			if p.GroovePct != 0 {
				en.GroovePct = p.GroovePct
			}
		}
		nodes = append(nodes, en)
	}
	insts := make([]exportInstrument, 0, len(dv.Rows))
	for _, r := range dv.Rows {
		kind := kindForID(r.Instrument)
		ei := exportInstrument{
			Name:   r.Name,
			ID:     r.Instrument,
			Kind:   kind,
			Volume: r.Volume,
			Origin: int(r.Origin),
			Color:  hexColor(r.Color),
		}
		// Check samplePath for custom uploaded instruments
		if dv.samplePath != nil {
			if p, ok := dv.samplePath[r.Instrument]; ok && p != "" {
				ei.Path = p
				// Custom uploads are samples even if kindForID says builtin
				ei.Kind = "sample"
			}
		}
		// For catalog-based samples without a custom path, try catalog lookup
		if ei.Path == "" && (kind == "sample" || ei.Kind == "sample") {
			if meta, ok := audio.CatalogLookup(r.Instrument); ok && meta.Path != "" {
				ei.Path = meta.Path
			}
		}
		// Export per-instrument EQ if any band is non-zero or muted
		if len(r.EQGainsDB) > 0 || len(r.EQBandMuted) > 0 {
			hasNonZeroGain := false
			for _, g := range r.EQGainsDB {
				if g != 0 {
					hasNonZeroGain = true
					break
				}
			}
			hasMutedBand := false
			for _, m := range r.EQBandMuted {
				if m {
					hasMutedBand = true
					break
				}
			}
			if hasNonZeroGain || hasMutedBand {
				eq := exportEQ{}
				eq.GainsDB = append(eq.GainsDB, r.EQGainsDB...)
				for _, b := range eqBandDefs {
					eq.BandsHz = append(eq.BandsHz, [2]float64{b.loHz, b.hiHz})
				}
				if hasMutedBand {
					eq.BandMuted = append(eq.BandMuted, r.EQBandMuted...)
				}
				ei.EQ = &eq
			}
		}
		insts = append(insts, ei)
	}
	file := exportFile{Version: 1, Subdiv: currentMaxDiv(), BPM: dv.BPM(), Instruments: insts, Nodes: nodes}
	// Export master EQ if any band has non-zero gain or is muted.
	hasNonZeroGain := false
	for _, g := range dv.eqBandGainsDB {
		if g != 0 {
			hasNonZeroGain = true
			break
		}
	}
	hasMutedBand := false
	for _, m := range dv.eqBandMuted {
		if m {
			hasMutedBand = true
			break
		}
	}
	if hasNonZeroGain || hasMutedBand {
		eq := exportEQ{}
		eq.GainsDB = append(eq.GainsDB, dv.eqBandGainsDB...)
		for _, b := range eqBandDefs {
			eq.BandsHz = append(eq.BandsHz, [2]float64{b.loHz, b.hiHz})
		}
		if hasMutedBand {
			eq.BandMuted = append(eq.BandMuted, dv.eqBandMuted...)
		}
		file.EQ = &eq
	}
	return json.MarshalIndent(file, "", "  ")
}

// Allow tests to override save sink.
func SetSaveJSONForTest(fn func(name string, data []byte) error) (restore func()) {
	prev := saveJSON
	saveJSON = fn
	return func() { saveJSON = prev }
}

// saveJSONDefault is defined in platform-specific files.
