package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// Full flow: import missing instrument -> menu includes id -> switch to available ->
// missing id persists in menu -> register WAV with same id -> becomes available & playable.
func TestMissingInstrumentMenuAndAvailabilityFlow(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	// Ensure baseline instruments are present
	dv.refreshInstruments()
	// Import config referencing missing instrument "myst"
	exp := exportFile{
		Version: 1, Subdiv: 32, BPM: 120,
		Nodes:       []exportNode{{ID: 0, I: 0, J: 0, Type: "regular"}},
		Instruments: []exportInstrument{{Name: "Myst", ID: "myst", Kind: "builtin", Volume: 1, Origin: 0, Color: "#FFFFFFFF"}},
	}
	data, _ := json.Marshal(exp)
	if err := g.Import(data); err != nil {
		t.Fatalf("import: %v", err)
	}
	// Missing instrument should be known but unavailable
	if dv.IsInstrumentAvailable("myst") {
		t.Fatalf("myst unexpectedly available")
	}
	seen := false
	for _, id := range dv.instOptions {
		if id == "myst" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("myst not present in instOptions")
	}
	// Switch to an available instrument
	dv.SetInstrument("snare")
	if !dv.IsInstrumentAvailable("snare") {
		t.Fatalf("snare not available")
	}
	// The missing id should remain in the menu
	seen = false
	for _, id := range dv.instOptions {
		if id == "myst" {
			seen = true
			break
		}
	}
	if !seen {
		t.Fatalf("myst disappeared from instOptions")
	}
	// Register a WAV with the same id to make it available
	if err := audio.RegisterWAV("myst", writeTempWAV(t, "myst.wav")); err != nil {
		t.Fatalf("register wav: %v", err)
	}
	dv.refreshInstruments()
	if !dv.IsInstrumentAvailable("myst") {
		t.Fatalf("myst did not become available after register")
	}
	// Switch back to myst and verify availability
	dv.SetInstrument("myst")
	if !dv.IsInstrumentAvailable(dv.Rows[dv.selRow].Instrument) {
		t.Fatalf("row instrument still marked missing")
	}
	// Playback should no longer be suppressed after registering the instrument.
	played := make(chan struct{}, 1)
	g.SetPlayFunc(func(string, float64, ...float64) { played <- struct{}{} })
	info := g.beatInfoAtRow(0, 0)
	if info.NodeType == 0 {
		g.highlightBeat(0, 0, info, 10)
	}
	waitForChan(t, played, 10000)
}
