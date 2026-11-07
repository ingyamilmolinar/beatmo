package roster

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// TestAllMatchesTheIndex: All is the shipped index, in order, with no
// duplicates — the instrument menu orders a category by it.
func TestAllMatchesTheIndex(t *testing.T) {
	b, err := os.ReadFile("../../assets/instruments/index.json")
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Instruments []instrument.ID `json:"instruments"`
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatal(err)
	}
	if len(All) != len(idx.Instruments) {
		t.Fatalf("len(All) = %d, index has %d", len(All), len(idx.Instruments))
	}
	seen := map[instrument.ID]bool{}
	for i, id := range All {
		if id != idx.Instruments[i] {
			t.Fatalf("All[%d] = %q, index has %q", i, id, idx.Instruments[i])
		}
		if seen[id] {
			t.Fatalf("All lists %q twice", id)
		}
		seen[id] = true
	}
	if Oboe != "oboe" || Kick != "kick" {
		t.Fatalf("constants spell the wrong ids: Oboe=%q Kick=%q", Oboe, Kick)
	}
}

// So instrument.ID.Base / IsUser need no roster lookup.
func TestNoRosterIDLooksMinted(t *testing.T) {
	for _, id := range All {
		if id.IsInstance() || id.IsUser() || string(id) == instrument.ReservedMasterName {
			t.Errorf("roster id %q has a minted shape", id)
		}
		if _, err := instrument.ParseID(string(id)); err != nil {
			t.Errorf("roster id %q does not parse: %v", id, err)
		}
	}
}
