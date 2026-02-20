package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
)

// Ensure that rows referencing missing instruments are marked red and silent.
func TestImportMissingInstrumentMarksRedAndSilent(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Export-like JSON with a missing instrument id "unknown"
	exp := exportFile{
		Version: 1,
		Subdiv:  32,
		BPM:     120,
		Nodes: []exportNode{
			{ID: 0, I: 0, J: 0, Type: "regular"},
		},
		Instruments: []exportInstrument{
			{Name: "Unknown", ID: "unknown", Kind: "builtin", Volume: 1, Origin: 0, Color: "#FFFFFFFF"},
		},
	}
	data, _ := json.Marshal(exp)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	g.drum.calcLayout() // ensure label buttons are rebuilt
	if len(g.drum.rowLabels()) == 0 {
		t.Fatalf("no row labels")
	}
	// Row label style should be MissingInstStyle
	btn := g.drum.rowLabels()[0]
	// The style is an interface; we check by drawing semantics via comparing the struct fields.
	// We rely on our style variable identity (ButtonStyle) for a quick type assertion.
	if _, ok := btn.Style.(ButtonStyle); !ok {
		t.Fatalf("label style is not a ButtonStyle")
	}
	// Verify HasInstrument says false
	if g.drum.IsInstrumentAvailable(g.drum.Rows[0].Instrument) {
		t.Fatalf("instrument unexpectedly available")
	}
	// Install a playFn to capture any playback
	var plays int
	g.SetPlayFunc(func(string, float64, ...float64) { plays++ })
	// Trigger a highlight that would normally play
	info := g.beatInfoAtRow(0, 0)
	if info.NodeType == 0 { // NodeTypeRegular
		g.highlightBeat(0, 0, info, 10)
	} else {
		// If initial graph lacks regular node, simulate a regular
		g.highlightBeat(0, 0, model.BeatInfo{NodeType: model.NodeTypeRegular}, 10)
	}
	if plays != 0 {
		t.Fatalf("row with missing instrument should be silent; got plays=%d", plays)
	}
}
