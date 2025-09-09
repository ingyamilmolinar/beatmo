package ui

import (
	"encoding/json"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"testing"
)

func TestExportJSONContentAndSaveCalled(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)

	// Build simple chain A->B->C and one stray node D
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	c := g.tryAddNode(2, 0, model.NodeTypeRegular)
	d := g.tryAddNode(0, 1, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, c)

	// Tweak drum rows
	g.drum.Rows[0].Name = "Snare"
	g.drum.Rows[0].Instrument = "snare"
	g.drum.Rows[0].Volume = 0.7
	g.drum.Rows[0].Origin = a.ID
	g.drum.SetBPM(123)

	var gotName string
	var gotData []byte
	restore := SetSaveJSONForTest(func(name string, data []byte) error {
		gotName = name
		gotData = append([]byte(nil), data...)
		return nil
	})
	defer restore()

	if err := g.drum.Export(); err != nil {
		t.Fatalf("export failed: %v", err)
	}
	if gotName == "" || len(gotData) == 0 {
		t.Fatalf("save not called")
	}

	// Parse JSON and validate required fields
	var f struct {
		Version     int `json:"version"`
		BPM         int `json:"bpm"`
		Instruments []struct {
			Name, ID, Kind, Color string
			Volume                float64
			Origin                int
		} `json:"instruments"`
		Nodes []struct {
			ID, I, J        int
			Type            string
			Inputs, Outputs []int
		} `json:"nodes"`
	}
	if err := json.Unmarshal(gotData, &f); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if f.Version != 1 {
		t.Fatalf("version=%d", f.Version)
	}
	if f.BPM != 123 {
		t.Fatalf("bpm=%d", f.BPM)
	}
	if len(f.Instruments) != 1 {
		t.Fatalf("instruments=%d", len(f.Instruments))
	}
	inst := f.Instruments[0]
	if inst.ID != "snare" || inst.Kind != "builtin" || inst.Origin != int(a.ID) {
		t.Fatalf("instrument mismatch: %+v", inst)
	}
	if inst.Volume < 0.69 || inst.Volume > 0.71 {
		t.Fatalf("vol=%.3f", inst.Volume)
	}

	// Nodes includes all and edges are directional
	ids := map[int]bool{}
	for _, n := range f.Nodes {
		ids[n.ID] = true
	}
	if len(ids) < 4 {
		t.Fatalf("expected at least 4 nodes, got %d", len(ids))
	}

	// Check that outputs map matches A->B->C and stray D has no edges
	get := func(id int) *struct {
		ID, I, J        int
		Type            string
		Inputs, Outputs []int
	} {
		for i := range f.Nodes {
			if f.Nodes[i].ID == id {
				return &f.Nodes[i]
			}
		}
		return nil
	}
	na := get(int(a.ID))
	nb := get(int(b.ID))
	nc := get(int(c.ID))
	nd := get(int(d.ID))
	if na == nil || nb == nil || nc == nil || nd == nil {
		t.Fatalf("missing nodes in export")
	}
	if len(na.Outputs) != 1 || na.Outputs[0] != int(b.ID) {
		t.Fatalf("A outputs: %v", na.Outputs)
	}
	if len(nb.Outputs) != 1 || nb.Outputs[0] != int(c.ID) {
		t.Fatalf("B outputs: %v", nb.Outputs)
	}
	if len(nc.Outputs) != 0 {
		t.Fatalf("C outputs: %v", nc.Outputs)
	}
	if len(nd.Outputs) != 0 || len(nd.Inputs) != 0 {
		t.Fatalf("D edges: in=%v out=%v", nd.Inputs, nd.Outputs)
	}
}
