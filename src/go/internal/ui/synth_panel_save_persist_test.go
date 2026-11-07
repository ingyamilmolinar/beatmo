package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Save leaves the project copy unchanged, so it pushes nothing to the platform: on WASM the pushed block is all a
// render reads, and a reverted push lost the liked tone.
func TestSynthSave_KeepsLikedParamsForBridge(t *testing.T) {
	newDocsGame(t, roster.Snare)
	audio.SetInstrumentParam(roster.Snare, "post_decay", 0.5)
	before, _ := audio.InstrumentDoc(roster.Snare)
	rev := audio.InstrumentDocRev(roster.Snare)

	if got := saveRecipeForInstrument(roster.Snare); got != roster.Snare {
		t.Fatalf("SaveActiveRecipe = %q, want snare", got)
	}
	if audio.InstrumentDocRev(roster.Snare) != rev {
		t.Error("Save rewrote the project copy (a platform re-push would follow)")
	}
	after, _ := audio.InstrumentDoc(roster.Snare)
	if d := instrument.Diff(before, after); len(d) != 0 {
		t.Errorf("Save changed the liked sound: %v", d)
	}
	if audio.InstrumentDocModified(roster.Snare) {
		t.Error("instrument still reports dirty after Save (Save button would stay highlighted)")
	}
}
