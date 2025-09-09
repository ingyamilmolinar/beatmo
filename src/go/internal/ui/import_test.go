package ui

import (
	"encoding/json"
	"testing"
)

func TestImportScalesSubdivisionsAndKeepsOrthogonality(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	// Make current grid use 16 subdivisions per beat instead of default 32.
	g.grid.SetSubs([]Subdivision{{Div: 1}, {Div: 2}, {Div: 4}, {Div: 8}, {Div: 16}})

	// Build an export-like JSON with subdiv=32 and a 1x1 beat rectangle.
	type node struct {
		ID      int    `json:"id"`
		I       int    `json:"i"`
		J       int    `json:"j"`
		Type    string `json:"type"`
		Inputs  []int  `json:"inputs"`
		Outputs []int  `json:"outputs"`
	}
	type inst struct {
		Name   string  `json:"name"`
		ID     string  `json:"id"`
		Kind   string  `json:"kind"`
		Volume float64 `json:"volume"`
		Origin int     `json:"origin"`
		Color  string  `json:"color"`
	}
	type file struct {
		Version     int    `json:"version"`
		Subdiv      int    `json:"subdiv"`
		BPM         int    `json:"bpm"`
		Instruments []inst `json:"instruments"`
		Nodes       []node `json:"nodes"`
	}
	f := file{
		Version:     1,
		Subdiv:      32,
		BPM:         110,
		Instruments: []inst{{Name: "Kick", ID: "kick", Kind: "builtin", Volume: 1, Origin: 1, Color: "#FFFFFFFF"}},
		Nodes: []node{
			{ID: 1, I: 0, J: 32, Type: "regular", Inputs: []int{4}, Outputs: []int{2}},
			{ID: 2, I: 32, J: 32, Type: "regular", Inputs: []int{1}, Outputs: []int{3}},
			{ID: 3, I: 32, J: 64, Type: "regular", Inputs: []int{2}, Outputs: []int{4}},
			{ID: 4, I: 0, J: 64, Type: "regular", Inputs: []int{3}, Outputs: []int{1}},
		},
	}
	data, _ := json.Marshal(f)
	if err := g.Import(data); err != nil {
		t.Fatalf("import failed: %v", err)
	}

	// After scaling from 32 -> 16, points should be halved.
	pts := [][2]int{{0, 16}, {16, 16}, {16, 32}, {0, 32}}
	for _, p := range pts {
		if n := g.nodeAt(p[0], p[1]); n == nil {
			t.Fatalf("expected node at (%d,%d)", p[0], p[1])
		}
	}
	// Orthogonality: check the first two nodes share same J (horizontal), next share same I (vertical).
	a := g.nodeAt(0, 16)
	b := g.nodeAt(16, 16)
	c := g.nodeAt(16, 32)
	if a == nil || b == nil || c == nil {
		t.Fatalf("missing nodes after import")
	}
	if a.J != b.J {
		t.Fatalf("not horizontal: a.J=%d b.J=%d", a.J, b.J)
	}
	if b.I != c.I {
		t.Fatalf("not vertical: b.I=%d c.I=%d", b.I, c.I)
	}
	if g.drum.BPM() != 110 {
		t.Fatalf("bpm not imported: %d", g.drum.BPM())
	}
}
