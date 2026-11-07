package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Opening a circuit plays each instrument's newest saved DOCUMENT, instances included, while the circuit keeps its own
// EQ, FX and Sampler settings.

func circuitWithKickAndInstance(t *testing.T) (*Game, instrument.ID) {
	t.Helper()
	g := newMelodicCircuitGame(t, 0)
	copyID, err := instrument.Instance(roster.Kick, 2)
	if err != nil {
		t.Fatal(err)
	}
	audio.EnsureInstanceInstrument(copyID)
	g.drum.AddRow()
	g.drum.selRow = len(g.drum.Rows) - 1
	g.drum.SetInstrument(copyID)
	g.drum.selRow = 0
	for _, id := range []instrument.ID{roster.Kick, copyID} {
		t.Cleanup(func() { audio.ApplyAudioBundle(id, audio.AudioBundle{}) })
	}
	return g, copyID
}

func TestOpenUsesNewestDocumentKeepsCircuitBundle(t *testing.T) {
	withPrivateInstrumentLibrary(t)
	g, copyID := circuitWithKickAndInstance(t)

	// The circuit's own settings: different EQs on the kick and its copy.
	audio.EQSettingsOf(roster.Kick).GainsDB[1] = -4
	audio.EQSettingsOf(copyID).GainsDB[7] = 3
	circuitKick := audio.CurrentAudioBundle(roster.Kick)
	circuitCopy := audio.CurrentAudioBundle(copyID)
	file := mustExport(t, g)

	// A newer saved Kick: another document AND its own EQ.
	saved, _ := audio.ShippedDoc(roster.Kick)
	saved.Render.Amplitude *= 0.5
	if err := audio.SaveInstrumentVersion(roster.Kick, saved, audio.AudioBundle{EQ: func() *audio.EQSettings {
		s := audio.NewEQSettings()
		s.GainsDB[9] = 6
		return s
	}()}); err != nil {
		t.Fatal(err)
	}

	if err := g.Import(file); err != nil {
		t.Fatal(err)
	}
	for _, id := range []instrument.ID{roster.Kick, copyID} {
		d, _ := audio.InstrumentDoc(id)
		if d.Render.Amplitude != saved.Render.Amplitude {
			t.Errorf("%s opened with amplitude %v, want the newest saved version's %v", id, d.Render.Amplitude, saved.Render.Amplitude)
		}
	}
	if got := audio.CurrentAudioBundle(roster.Kick); got.Digest() != circuitKick.Digest() {
		t.Errorf("opening replaced the circuit's kick settings:\n got %+v\nwant %+v", got.EQ, circuitKick.EQ)
	}
	if got := audio.CurrentAudioBundle(copyID); got.Digest() != circuitCopy.Digest() {
		t.Errorf("opening replaced the circuit's settings for its kick copy:\n got %+v\nwant %+v", got.EQ, circuitCopy.EQ)
	}
}

func TestOpenWithPointerOnShippedPlaysTheCircuitCopy(t *testing.T) {
	withPrivateInstrumentLibrary(t)
	g, _ := circuitWithKickAndInstance(t)
	file := mustExport(t, g)
	var f projectFile
	if err := json.Unmarshal(file, &f); err != nil {
		t.Fatal(err)
	}
	circuitAmp := f.InstrumentDocs[roster.Kick].Render.Amplitude

	saved, _ := audio.ShippedDoc(roster.Kick)
	saved.Render.Amplitude = circuitAmp * 0.5
	if err := audio.SaveInstrumentVersion(roster.Kick, saved, audio.AudioBundle{}); err != nil {
		t.Fatal(err)
	}
	if err := audio.MoveLibraryToShipped(roster.Kick); err != nil {
		t.Fatal(err)
	}
	if err := g.Import(file); err != nil {
		t.Fatal(err)
	}
	if d, _ := audio.InstrumentDoc(roster.Kick); d.Render.Amplitude != circuitAmp {
		t.Fatalf("with the pointer on the shipped version the circuit's kick should play (%v), got %v", circuitAmp, d.Render.Amplitude)
	}
}

func TestCircuitDigestIsAFunctionOfTheCircuit(t *testing.T) {
	g := newMelodicCircuitGame(t, 0, 2)
	file := mustExport(t, g)
	var a, b projectFile
	if err := json.Unmarshal(file, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mustExport(t, g), &b); err != nil {
		t.Fatal(err)
	}
	if circuitDigest(a) != circuitDigest(b) {
		t.Fatal("the same circuit exported twice has two digests")
	}
	if err := g.Import(file); err != nil {
		t.Fatal(err)
	}
	var c projectFile
	if err := json.Unmarshal(mustExport(t, g), &c); err != nil {
		t.Fatal(err)
	}
	if circuitDigest(a) != circuitDigest(c) {
		t.Fatal("export → import → export moved the circuit digest")
	}
	c.BPM++
	if circuitDigest(a) == circuitDigest(c) {
		t.Fatal("a tempo change kept the circuit digest")
	}
	d := a
	d.InstrumentDocs = map[instrument.ID]projectDoc{}
	for id, pd := range a.InstrumentDocs {
		pd.Doc = pd.Doc.Clone()
		pd.Doc.Render.Amplitude += 0.001
		d.InstrumentDocs[id] = pd
	}
	if circuitDigest(a) == circuitDigest(d) {
		t.Fatal("a changed instrument sound kept the circuit digest")
	}
}
