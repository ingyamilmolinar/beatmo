package wavref

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestCatalogue_ParsesAndValidates(t *testing.T) {
	c, err := Catalogue()
	if err != nil {
		t.Fatalf("Catalogue(): %v", err)
	}
	if len(c.Refs) == 0 {
		t.Fatal("catalogue has no refs")
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
}

func TestCatalogue_IDsUnique(t *testing.T) {
	c, err := Catalogue()
	if err != nil {
		t.Fatalf("Catalogue(): %v", err)
	}
	seen := map[string]bool{}
	for _, r := range c.Refs {
		if r.ID == "" {
			t.Fatal("ref with empty id")
		}
		if seen[r.ID] {
			t.Errorf("duplicate ref id %q", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestCatalogue_LookupByIDAndInstrument(t *testing.T) {
	c, err := Catalogue()
	if err != nil {
		t.Fatalf("Catalogue(): %v", err)
	}
	r, ok := c.Ref("violin-A4")
	if !ok {
		t.Fatal(`Ref("violin-A4") not found`)
	}
	if r.Tier != TierInstrument {
		t.Errorf("violin-A4 tier = %q, want %q", r.Tier, TierInstrument)
	}
	if got := c.ForInstrument(roster.Violin); len(got) == 0 {
		t.Error(`ForInstrument("violin") returned nothing`)
	}
}

// A real entry must carry a file and a source; a status entry must carry
// neither. This is the invariant that keeps "covered" honest.
func TestCatalogue_StatusEntriesCarryNoFile(t *testing.T) {
	c, _ := Catalogue()
	for _, r := range c.Refs {
		switch r.Status {
		case "":
			if r.File == "" || r.Source == nil {
				t.Errorf("%s: real ref must have file and source", r.ID)
			}
		case StatusSynthetic, StatusNeedsSource:
			if r.File != "" || r.PCMSHA256 != "" {
				t.Errorf("%s: status ref must not carry file/hash", r.ID)
			}
			if r.Note == "" {
				t.Errorf("%s: status ref must explain itself in note", r.ID)
			}
		default:
			t.Errorf("%s: unknown status %q", r.ID, r.Status)
		}
	}
}
