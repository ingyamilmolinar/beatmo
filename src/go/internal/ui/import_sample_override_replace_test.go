//go:build test

package ui

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Replace-not-merge covers a BUILT-IN a previous project turned into a sample: otherwise B's snare plays A's
// chop and B exports A's PCM. Undo/redo rides the same Import.
func TestImportDropsASamplePreviousProjectPutOnABuiltin(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	withPCM, err := os.ReadFile("testdata/import_embedded_pcm.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	if err := g.Import(withPCM); err != nil {
		t.Fatalf("import A: %v", err)
	}
	if !audio.IsUserSample(roster.Snare) {
		t.Fatalf("precondition: project A must register snare as a sample")
	}

	// Built from A, not the startup demo, whose roster is free to change.
	if err := g.Import(withSnareAsSynth(t, withPCM)); err != nil {
		t.Fatalf("import B: %v", err)
	}
	if audio.IsUserSample(roster.Snare) {
		t.Errorf("snare is still project A's sample after importing a project where it is the synth")
	}
	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	var f projectFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, inst := range f.Instruments {
		if inst.ID == roster.Snare && (inst.Kind != kindBuiltin || inst.PCM != nil) {
			t.Errorf("re-export writes snare as kind=%q with PCM=%v, want the builtin synth", inst.Kind, inst.PCM != nil)
		}
	}
}

func withSnareAsSynth(t *testing.T, doc []byte) []byte {
	t.Helper()
	var f map[string]any
	if err := json.Unmarshal(doc, &f); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	insts, _ := f["instruments"].([]any)
	found := false
	for _, it := range insts {
		inst, _ := it.(map[string]any)
		if inst["id"] != "snare" {
			continue
		}
		inst["kind"] = "builtin"
		delete(inst, "pcm")
		found = true
	}
	if !found {
		t.Fatal("fixture has no snare instrument")
	}
	out, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal project B: %v", err)
	}
	return out
}
