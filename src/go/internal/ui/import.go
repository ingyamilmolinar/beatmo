package ui

import (
	"encoding/json"
	"fmt"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	"image/color"
	"math"
	"strings"
)

type importFile struct {
	Version     int                `json:"version"`
	Subdiv      int                `json:"subdiv"`
	BPM         int                `json:"bpm"`
	Instruments []exportInstrument `json:"instruments"`
	Nodes       []exportNode       `json:"nodes"`
}

func parseHexColor(s string) color.Color {
	s = strings.TrimPrefix(s, "#")
	if len(s) == 8 {
		var r, g, b, a uint8
		fmt.Sscanf(s, "%02X%02X%02X%02X", &r, &g, &b, &a)
		return color.RGBA{r, g, b, a}
	}
	return color.RGBA{255, 255, 255, 255}
}

// Import rebuilds the graph and drum rows from exported JSON data. It accepts
// coordinates in the JSON's subdivision units and scales them to the current
// grid's MaxDiv. Missing subdiv defaults to 32.
func (g *Game) Import(data []byte) error {
	g.logger.Infof("[GAME] Import requested (%d bytes)", len(data))
	var f importFile
	if err := json.Unmarshal(data, &f); err != nil {
		return err
	}
	// Normalize and apply imported subdivision before constructing nodes so
	// coordinates in the JSON map 1:1 to the new grid.
	if f.Subdiv == 0 {
		f.Subdiv = 32
	}
	switch f.Subdiv {
	case 4, 8, 16, 32:
	default:
		f.Subdiv = 32
	}

	// Reset graph and UI state without replacing the graph pointer. The engine
	// predictor and other subsystems hold references to g.graph and must remain
	// in sync. Replacing the pointer would desynchronize predictions and break
	// precomputed drum view state.
	g.graph.Nodes = map[model.NodeID]model.Node{}
	g.graph.Edges = map[[2]model.NodeID]struct{}{}
	g.graph.Next = 0
	g.graph.Row = make([]bool, 4)
	g.nodes = nil
	g.nodesByID = make(map[model.NodeID]*uiNode)
	g.edges = nil
	g.drum.Graph = g.graph
	g.start = nil
	g.graph.StartNodeID = model.InvalidNodeID

	// Apply incoming subdivision now that graph/UI state are cleared.
	if err := g.SetSubdivisions(f.Subdiv); err != nil {
		// If change is denied (e.g., playing), keep current grid and scale coords.
		g.logger.Warnf("[GAME] Import: cannot apply subdiv %d now: %v; scaling nodes to current grid", f.Subdiv, err)
	}
	cur := g.grid.MaxDiv()
	scale := float64(cur) / float64(f.Subdiv)

	// Create nodes (regular/silent/mute) and remember mapping id->uiNode.
	idToNode := map[int]*uiNode{}
	for _, n := range f.Nodes {
		if n.Type == "invisible" {
			continue
		}
		i := int(math.Round(float64(n.ID)))
		sx := int(math.Round(float64(n.I) * scale))
		sy := int(math.Round(float64(n.J) * scale))
		nodeType := model.NodeTypeRegular
		switch strings.ToLower(n.Type) {
		case "silent":
			nodeType = model.NodeTypeSilent
		case "mute":
			nodeType = model.NodeTypeMute
		}
		ui := g.tryAddNode(sx, sy, nodeType)
		// Apply parameters if present.
		if mn, ok := g.graph.GetNodeByID(ui.ID); ok {
			p := mn.Params
			if n.Volume != 0 {
				p.Volume = n.Volume
			}
			if n.Pitch != 0 {
				p.Pitch = n.Pitch
			}
			if n.Duration != 0 {
				p.Duration = n.Duration
			}
			// Groove (new)
			if n.GrooveKind != "" {
				p.GrooveKind = n.GrooveKind
			}
			if n.GroovePct != 0 {
				p.GroovePct = n.GroovePct
			}
			// Logic fields (new). If present, take precedence over SkipEvery.
			if n.LogicKind != "" {
				if n.LogicKind == "prev_fired" {
					n.LogicKind = "trigger_if_prev_triggered"
				}
				if n.LogicKind == "every_n_loops" {
					n.LogicKind = "every_n_triggers"
				}
				// Map removed redundant rules to canonical forms.
				if n.LogicKind == "skip_if_prev_skipped" {
					n.LogicKind = "trigger_if_prev_triggered"
				}
				if n.LogicKind == "skip_if_prev_triggered" {
					n.LogicKind = "trigger_if_prev_skipped"
				}
				p.LogicKind = n.LogicKind
				if n.LogicN > 0 {
					p.LogicN = n.LogicN
				}
				if n.LogicP > 0 {
					p.LogicP = n.LogicP
				}
				// Do not copy SkipEvery into params when logic is explicit to avoid double gating.
			} else if n.SkipEvery > 0 {
				// Back-compat import upgrade: map legacy SkipEvery to logic_kind=skip_every_n so
				// the UI reflects the rule and parameter controls work via the logic dropdown.
				p.LogicKind = "skip_every_n"
				p.LogicN = n.SkipEvery
				p.SkipEveryN = 0
			}
			g.graph.SetNodeParams(ui.ID, p)
		}
		idToNode[i] = ui
	}
	// Create edges from all non-invisible nodes. Silent nodes are valid
	// routing points and must retain their connections.
	for _, n := range f.Nodes {
		if strings.ToLower(n.Type) == "invisible" {
			continue
		}
		from := idToNode[int(n.ID)]
		if from == nil {
			continue
		}
		for _, out := range n.Outputs {
			to := idToNode[int(out)]
			if to != nil {
				g.addEdge(from, to)
			}
		}
	}
	// Instruments -> rows
	g.drum.Rows = nil
	for i, inst := range f.Instruments {
		g.drum.AddRow()
		idx := len(g.drum.Rows) - 1
		row := g.drum.Rows[idx]
		row.Name = inst.Name
		row.Instrument = inst.ID
		row.Volume = inst.Volume
		row.Color = parseHexColor(inst.Color)
		// Remember instrument id even if missing so users can switch back or
		// load it later by the same name.
		g.drum.EnsureInstrumentKnown(inst.ID)
		// If a path is provided, try to register immediately so it's available.
		if inst.Path != "" {
			if g.drum.samplePath == nil {
				g.drum.samplePath = map[string]string{}
			}
			g.drum.samplePath[inst.ID] = inst.Path
			// Attempt to register; on stub/wasm this makes it available immediately.
			_ = audio.RegisterWAV(inst.ID, inst.Path)
			g.drum.refreshInstruments()
		}
		if ui := idToNode[int(inst.Origin)]; ui != nil {
			row.Origin = ui.ID
			row.Node = ui
			if idx == 0 {
				g.start = ui
				g.graph.StartNodeID = ui.ID
			}
			g.logger.Debugf("[GAME] Import row %d: name=%q inst=%q origin(json)=%d -> nodeID=%d", i, inst.Name, inst.ID, inst.Origin, ui.ID)
		} else {
			g.logger.Infof("[GAME] Import row %d: name=%q inst=%q origin(json)=%d not found; leaving origin unset", i, inst.Name, inst.ID, inst.Origin)
		}
	}
	// Ensure imported colors are unique across rows.
	g.drum.EnsureUniqueRowColors()
	if f.BPM > 0 {
		g.drum.SetBPM(f.BPM)
	}
	// Clear any pending UI-added rows state so origin selection does not remain
	// armed after an import that already set each row's origin.
	g.drum.added = nil
	g.pendingStartRow = -1
	g.updateBeatInfos()
	g.logger.Infof("[GAME] Import completed: bpm=%d nodes=%d rows=%d subdiv=%d", f.BPM, len(f.Nodes), len(g.drum.Rows), f.Subdiv)
	return nil
}
