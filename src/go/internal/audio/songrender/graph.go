package songrender

import (
	"fmt"
	"io"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

type RowPaths struct {
	Graph     *model.Graph
	Paths     [][]model.BeatInfo
	IsLoop    []bool
	LoopStart []int
	Nodes     map[model.NodeID]model.Node
	RowInst   []string
}

func nodeType(s string) model.NodeType {
	switch s {
	case "invisible":
		return model.NodeTypeInvisible
	case "silent":
		return model.NodeTypeSilent
	case "mute":
		return model.NodeTypeMute
	default:
		return model.NodeTypeRegular
	}
}

// BuildGraph constructs a model.Graph from the parsed template and computes one
// traversal path per instrument (from that instrument's Origin node).
func BuildGraph(pt ParsedTemplate) (RowPaths, error) {
	g := model.NewGraph(game_log.New(io.Discard, game_log.LevelError))
	idMap := make(map[int]model.NodeID, len(pt.Nodes))
	for _, n := range pt.Nodes {
		nid := g.AddNode(n.I, n.J, nodeType(n.Type))
		g.SetNodeParams(nid, model.NodeParams{
			Volume:    n.Volume,
			Pitch:     n.Pitch,
			Duration:  n.Duration,
			LogicKind: n.LogicKind,
			LogicN:    n.LogicN,
			LogicP:    n.LogicP,
		})
		idMap[n.ID] = nid
	}
	// Wire edges from each node's Outputs.
	for _, n := range pt.Nodes {
		from, ok := idMap[n.ID]
		if !ok {
			continue
		}
		for _, out := range n.Outputs {
			to, ok := idMap[out]
			if ok {
				g.Edges[[2]model.NodeID{from, to}] = struct{}{}
			}
		}
	}

	rp := RowPaths{Graph: g, Nodes: g.Nodes}
	for _, inst := range pt.Instruments {
		origin, ok := idMap[inst.Origin]
		if !ok {
			// No origin node → empty path for this row (still keep the slot).
			rp.Paths = append(rp.Paths, nil)
			rp.IsLoop = append(rp.IsLoop, false)
			rp.LoopStart = append(rp.LoopStart, 0)
			rp.RowInst = append(rp.RowInst, inst.ID)
			continue
		}
		path, isLoop, loopStart := g.CalculateBeatRowFrom(origin)
		rp.Paths = append(rp.Paths, path)
		rp.IsLoop = append(rp.IsLoop, isLoop)
		rp.LoopStart = append(rp.LoopStart, loopStart)
		rp.RowInst = append(rp.RowInst, inst.ID)
	}
	if len(rp.Paths) == 0 {
		return RowPaths{}, fmt.Errorf("songrender: no instrument rows")
	}
	return rp, nil
}
