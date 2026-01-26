package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

// After registering a WAV matching a missing instrument ID, the row label
// style should switch from MissingInstStyle to InstButtonStyle.
func TestMissingInstrumentStyleResetsOnRegister(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	dv.refreshInstruments()
	// Import with missing instrument "myst"
	exp := exportFile{
		Version: 1, Subdiv: 32, BPM: 120,
		Nodes:       []exportNode{{ID: 0, I: 0, J: 0, Type: "regular"}},
		Instruments: []exportInstrument{{Name: "Myst", ID: "myst", Kind: "builtin", Volume: 1, Origin: 0, Color: "#FFFFFFFF"}},
	}
	data, _ := json.Marshal(exp)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	dv.recalcButtons()
	if len(dv.rowLabels) == 0 {
		t.Fatalf("no row labels")
	}
	if dv.rowLabels[0].Style != MissingInstStyle {
		t.Fatalf("expected missing style; got %+v", dv.rowLabels[0].Style)
	}
	// Register matching WAV and refresh instruments
	if err := audio.RegisterWAV("myst", writeTempWAV(t, "myst.wav")); err != nil {
		t.Fatalf("register: %v", err)
	}
	dv.refreshInstruments()
	if dv.rowLabels[0].Style != InstButtonStyle {
		t.Fatalf("expected normal style after register; got %+v", dv.rowLabels[0].Style)
	}
}
