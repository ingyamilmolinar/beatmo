package ui

import (
    "encoding/json"
    "fmt"
    "image/color"
    "sort"
    "strings"
)
import (
    "github.com/ingyamilmolinar/tunkul/core/model"
)

// Export schema
type exportFile struct {
    Version     int                `json:"version"`
    Subdiv      int                `json:"subdiv,omitempty"`
    BPM         int                `json:"bpm"`
    Instruments []exportInstrument `json:"instruments"`
    Nodes       []exportNode       `json:"nodes"`
}

type exportInstrument struct {
    Name    string  `json:"name"`
    ID      string  `json:"id"`
    Kind    string  `json:"kind"` // builtin|sample
    Volume  float64 `json:"volume"`
    Origin  int     `json:"origin"`
    Color   string  `json:"color"` // #RRGGBBAA
}

type exportNode struct {
    ID      int      `json:"id"`
    I       int      `json:"i"`
    J       int      `json:"j"`
    Type    string   `json:"type"`           // regular|invisible
    Inputs  []int    `json:"inputs,omitempty"`
    Outputs []int    `json:"outputs,omitempty"`
}

func kindForID(id string) string {
    if strings.HasPrefix(id, "sample-") { return "sample" }
    return "builtin"
}

func hexColor(c color.Color) string {
    r, g, b, a := c.RGBA()
    // r,g,b,a are 0..65535; convert to 0..255
    return fmt.Sprintf("#%02X%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
}

// Export builds the export JSON and triggers a download/save.
func (dv *DrumView) Export() error {
    data, err := dv.exportBytes()
    if err != nil { return err }
    name := "tunkul-export.json"
    return saveJSON(name, data)
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
    for id := range g.Nodes { ids = append(ids, int(id)) }
    sort.Ints(ids)

    in := map[int][]int{}
    out := map[int][]int{}
    for e := range g.Edges {
        a := int(e[0]); b := int(e[1])
        out[a] = append(out[a], b)
        in[b] = append(in[b], a)
    }
    nodes := make([]exportNode, 0, len(ids))
    for _, id := range ids {
        n := g.Nodes[model.NodeID(id)]
        if n.Type == model.NodeTypeInvisible { continue }
        en := exportNode{ID: id, I: n.I, J: n.J, Type: "regular"}
        if v := in[id]; len(v) > 0 { sort.Ints(v); en.Inputs = v }
        if v := out[id]; len(v) > 0 { sort.Ints(v); en.Outputs = v }
        nodes = append(nodes, en)
    }
    insts := make([]exportInstrument, 0, len(dv.Rows))
    for _, r := range dv.Rows {
        insts = append(insts, exportInstrument{
            Name:   r.Name,
            ID:     r.Instrument,
            Kind:   kindForID(r.Instrument),
            Volume: r.Volume,
            Origin: int(r.Origin),
            Color:  hexColor(r.Color),
        })
    }
    file := exportFile{Version: 1, Subdiv: currentMaxDiv(), BPM: dv.BPM(), Instruments: insts, Nodes: nodes}
    return json.MarshalIndent(file, "", "  ")
}

// Allow tests to override save sink.
func SetSaveJSONForTest(fn func(name string, data []byte) error) (restore func()) {
    prev := saveJSON
    saveJSON = fn
    return func() { saveJSON = prev }
}

// saveJSONDefault is defined in platform-specific files.
