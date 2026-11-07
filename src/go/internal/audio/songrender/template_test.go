package songrender

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

func TestLoadTemplate_BachToccata(t *testing.T) {
	pt, err := LoadTemplate("bach-brandenburg-3")
	if err != nil {
		t.Fatal(err)
	}
	if pt.BPM <= 0 || pt.Subdiv <= 0 {
		t.Errorf("bad header: bpm=%d subdiv=%d", pt.BPM, pt.Subdiv)
	}
	if len(pt.Instruments) == 0 || len(pt.Nodes) == 0 {
		t.Fatalf("empty: %d instruments, %d nodes", len(pt.Instruments), len(pt.Nodes))
	}
	// Organ is the Bach toccata instrument.
	found := false
	for _, in := range pt.Instruments {
		if in.ID == roster.Violin {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a violin instrument in bach-brandenburg-3")
	}
}

func TestLoadTemplate_UnknownStem(t *testing.T) {
	if _, err := LoadTemplate("does-not-exist"); err == nil {
		t.Error("expected error for unknown stem")
	}
}

// As the app's import does: never to an empty document.
func TestParseTemplateResolvesPackRefs(t *testing.T) {
	pt, err := LoadTemplate(assets.Templates()[0].Genre)
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Docs map[string]struct {
			Ref string `json:"ref"`
		} `json:"instrument_docs"`
	}
	if err := json.Unmarshal(assets.Templates()[0].Bytes, &raw); err != nil {
		t.Fatal(err)
	}
	for _, in := range pt.Instruments {
		ref := raw.Docs[string(in.ID)].Ref
		if ref == "" {
			t.Fatalf("%s: template row is not pinned", in.ID)
		}
		if in.Doc == nil {
			t.Fatalf("%s: no document resolved", in.ID)
		}
		if got := instrument.ContentHash(*in.Doc); got != ref || in.Doc.ID != in.ID {
			t.Fatalf("%s: resolved %s (id %q), want %s", in.ID, got, in.Doc.ID, ref)
		}
	}
	if _, err := ParseTemplate([]byte(`{"bpm":120,"instruments":[{"id":"kick"}],"instrument_docs":{"kick":{"ref":"sha256:ffffffffffffffff"}}}`)); err == nil {
		t.Fatal("an unknown ref must be an error")
	}
}
