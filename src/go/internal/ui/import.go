package ui

import (
    "encoding/json"
    "fmt"
    "image/color"
    "math"
    "strings"
    "github.com/ingyamilmolinar/tunkul/core/model"
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
    var f importFile
    if err := json.Unmarshal(data, &f); err != nil { return err }
    if f.Subdiv == 0 { f.Subdiv = 32 }
    cur := g.grid.MaxDiv()
    scale := float64(cur) / float64(f.Subdiv)

    // Reset graph and UI state.
    g.graph = model.NewGraph(g.logger)
    g.nodes = nil
    g.edges = nil
    g.drum.Graph = g.graph
    g.start = nil
    g.graph.StartNodeID = model.InvalidNodeID

    // Create nodes (regular only) and remember mapping id->uiNode.
    idToNode := map[int]*uiNode{}
    for _, n := range f.Nodes {
        if n.Type != "regular" { continue }
        i := int(math.Round(float64(n.ID)))
        sx := int(math.Round(float64(n.I) * scale))
        sy := int(math.Round(float64(n.J) * scale))
        ui := g.tryAddNode(sx, sy, model.NodeTypeRegular)
        idToNode[i] = ui
    }
    // Create edges
    for _, n := range f.Nodes {
        if n.Type != "regular" { continue }
        from := idToNode[int(n.ID)]
        if from == nil { continue }
        for _, out := range n.Outputs {
            to := idToNode[int(out)]
            if to != nil {
                g.addEdge(from, to)
            }
        }
    }
    // Instruments -> rows
    g.drum.Rows = nil
    for _, inst := range f.Instruments {
        g.drum.AddRow()
        idx := len(g.drum.Rows)-1
        row := g.drum.Rows[idx]
        row.Name = inst.Name
        row.Instrument = inst.ID
        row.Volume = inst.Volume
        row.Color = parseHexColor(inst.Color)
        if ui := idToNode[int(inst.Origin)]; ui != nil {
            row.Origin = ui.ID
            row.Node = ui
            if idx == 0 {
                g.start = ui
                g.graph.StartNodeID = ui.ID
            }
        }
    }
    if f.BPM > 0 { g.drum.SetBPM(f.BPM) }
    g.updateBeatInfos()
    return nil
}

