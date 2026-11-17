package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// Templates pin every row's instrument to a pack document by content hash, so
// a template sounds the same whatever the user has saved over its instruments.

func TestEveryTemplateRefResolves(t *testing.T) {
	for _, tp := range assets.Templates() {
		var f projectFile
		if err := json.Unmarshal(tp.Bytes, &f); err != nil {
			t.Fatalf("%s: %v", tp.Display, err)
		}
		if len(f.InstrumentDocs) == 0 {
			t.Fatalf("%s: a template must pin its instruments", tp.Display)
		}
		for id, pd := range f.InstrumentDocs {
			if pd.Ref == "" {
				t.Fatalf("%s/%s: templates reference the pack", tp.Display, id)
			}
			if _, ok := audio.PackDoc(pd.Ref); !ok {
				t.Fatalf("%s/%s: ref %s not in the pack", tp.Display, id, pd.Ref)
			}
		}
	}
}

func TestTemplateImportExpandsToFullCopies(t *testing.T) {
	g := newDocsGame(t)
	tp := assets.Templates()[0]
	if err := g.Import(tp.Bytes); err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, r := range g.drum.Rows {
		d, ok := audio.InstrumentDoc(r.Instrument)
		if !ok {
			continue // sample rows
		}
		checked++
		if err := instrument.Validate(d, audio.BuiltinModuleRegistry()); err != nil || d.ID != r.Instrument {
			t.Fatalf("%s: id=%q %v", r.Instrument, d.ID, err)
		}
		if want := r.Instrument.Base(); d.Origin != want {
			t.Errorf("%s: origin %q, want %q", r.Instrument, d.Origin, want)
		}
	}
	if checked == 0 {
		t.Fatal("no synth row checked")
	}
}

// A template is a circuit like any other: when the user has SAVED their own
// version of one of its instruments, that version plays (the user's Save
// trumps every circuit's document for the same instrument), and the template's
// PINNED document is what Reset returns to.
func TestUserSaveTrumpsTemplateInstruments(t *testing.T) {
	g := newDocsGame(t)
	tp := assets.Templates()[0]
	var f projectFile
	if err := json.Unmarshal(tp.Bytes, &f); err != nil {
		t.Fatal(err)
	}
	var id, ref string
	for k, pd := range f.InstrumentDocs {
		if k.Base() == k {
			id, ref = string(k), pd.Ref
			break
		}
	}
	if id == "" {
		t.Fatal("template has no base-id row")
	}
	lib, _ := audio.LibraryDoc(instrument.ID(id))
	lib, err := instrument.SetField(lib, audio.BuiltinModuleRegistry(), "gain", 0.123)
	if err != nil {
		t.Fatal(err)
	}
	if err := audio.SaveLibraryDoc(lib); err != nil {
		t.Fatal(err)
	}
	if err := g.Import(tp.Bytes); err != nil {
		t.Fatal(err)
	}
	got, _ := audio.InstrumentDoc(instrument.ID(id))
	if d := instrument.Diff(got, lib); len(d) != 0 {
		t.Errorf("%s: the template's pinned document won over the user's Save: %v", id, d)
	}
	if got.Origin != instrument.ID(id) || audio.InstrumentDocModified(instrument.ID(id)) {
		t.Errorf("%s: origin %q modified=%v, want the saved entry, clean", id, got.Origin, audio.InstrumentDocModified(instrument.ID(id)))
	}
	pinned, ok := audio.CircuitInstrumentDoc(instrument.ID(id))
	if !ok || instrument.ContentHash(pinned) != ref {
		t.Fatalf("%s: Reset's target is not the template's pinned %s (ok=%v)", id, ref, ok)
	}
	audio.ResetInstrumentToCircuit(instrument.ID(id))
	if got, _ := audio.InstrumentDoc(instrument.ID(id)); instrument.ContentHash(got) != ref {
		t.Errorf("%s: Reset gave %s, want the pinned %s", id, instrument.ContentHash(got), ref)
	}
}

func TestProjectDocJSONShapes(t *testing.T) {
	var pd projectDoc
	if err := json.Unmarshal([]byte(`{"ref":"sha256:0123456789abcdef"}`), &pd); err != nil || pd.Ref != "sha256:0123456789abcdef" {
		t.Fatalf("ref form: %+v %v", pd, err)
	}
	full, _ := audio.ShippedDoc(roster.Kick)
	b, err := json.Marshal(projectDoc{Doc: full})
	if err != nil || strings.Contains(string(b), `"ref"`) {
		t.Fatalf("a full document must not carry a ref: %s %v", b, err)
	}
	var back projectDoc
	if err := json.Unmarshal(b, &back); err != nil || back.Ref != "" || len(instrument.Diff(back.Doc, full)) != 0 {
		t.Fatalf("full form round trip: %v", err)
	}
	if err := json.Unmarshal([]byte(`{"ref":"sha256:0123456789abcdef","name":"x"}`), &pd); err == nil {
		t.Fatal("a ref carries nothing else")
	}
}

func TestImportRejectsAnUnknownRef(t *testing.T) {
	g := newDocsGame(t)
	raw := `{"schema_version":"0.0.1","subdiv":8,"bpm":120,` +
		`"instruments":[{"name":"Kick","id":"kick","kind":"builtin","volume":1,"origin":0}],` +
		`"nodes":[{"id":0,"i":0,"j":0,"type":"regular","inputs":[],"outputs":[],"volume":1,"duration":1}],` +
		`"instrument_docs":{"kick":{"ref":"sha256:ffffffffffffffff"}}}`
	err := g.Import([]byte(raw))
	if err == nil || !strings.Contains(err.Error(), "sha256:ffffffffffffffff") {
		t.Fatalf("unknown ref: err=%v, want an error naming the ref", err)
	}
}
