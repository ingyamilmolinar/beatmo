package audio

import (
	"errors"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

type memVersionedLibrary struct {
	blob    []byte
	oldDocs map[instrument.ID][]byte
	fail    error
	saves   int
}

func (m *memVersionedLibrary) LoadLibrary() ([]byte, error) { return m.blob, nil }
func (m *memVersionedLibrary) LoadInstrumentDocs() (map[instrument.ID][]byte, error) {
	return m.oldDocs, nil
}
func (m *memVersionedLibrary) SaveLibrary(b []byte) error {
	if m.fail != nil {
		return m.fail
	}
	m.saves++
	m.blob = append([]byte(nil), b...)
	return nil
}

type memOldEdits map[instrument.ID]map[string]float64

func (m memOldEdits) LoadSampleEdits() (map[instrument.ID]map[string]float64, error) { return m, nil }

func freshLibrary(t *testing.T) *memVersionedLibrary {
	t.Helper()
	lib := &memVersionedLibrary{}
	SetInstrumentLibrarySink(lib)
	if err := LoadUserInstrumentLibrary(lib, nil); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		SetInstrumentLibrarySink(nil)
		_ = LoadUserInstrumentLibrary(&memVersionedLibrary{}, nil)
	})
	return lib
}

func shippedWith(t *testing.T, id instrument.ID, gain float64) instrument.Doc {
	t.Helper()
	d, ok := ShippedDoc(id)
	if !ok {
		t.Fatalf("%s is not shipped", id)
	}
	d.Render.Amplitude = gain
	return d
}

func TestSaveAppendsVersionAndKeepsOld(t *testing.T) {
	lib := freshLibrary(t)
	id := roster.Snare
	v1 := shippedWith(t, id, 0.5)
	v2 := shippedWith(t, id, 0.25)
	if err := SaveInstrumentVersion(id, v1, AudioBundle{}); err != nil {
		t.Fatal(err)
	}
	if err := SaveInstrumentVersion(id, v2, AudioBundle{EQ: eqWithGain(2)}); err != nil {
		t.Fatal(err)
	}
	got, ok := UserLibraryDoc(id)
	if !ok || got.Render.Amplitude != 0.25 {
		t.Fatalf("the library plays %v (saved: %v), want the newest version", got.Render.Amplitude, ok)
	}
	if b, ok := LibraryBundle(id); !ok || b.EQ == nil || b.EQ.GainsDB[3] != 2 {
		t.Fatalf("the newest version's bundle is not the library's: %+v", b)
	}
	if n := len(LibraryVersions(id)); n != 2 {
		t.Fatalf("history holds %d versions, want both", n)
	}
	// The store survives a reload from its persisted blob.
	if err := LoadUserInstrumentLibrary(lib, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := UserLibraryDoc(id); got.Render.Amplitude != 0.25 || len(LibraryVersions(id)) != 2 {
		t.Fatalf("reload lost history or the current pointer: amp %v, %d versions", got.Render.Amplitude, len(LibraryVersions(id)))
	}
}

func TestPointerOnShippedPlaysTheShippedDocument(t *testing.T) {
	freshLibrary(t)
	id := roster.Snare
	if err := SaveInstrumentVersion(id, shippedWith(t, id, 0.5), AudioBundle{}); err != nil {
		t.Fatal(err)
	}
	if err := MoveLibraryToShipped(id); err != nil {
		t.Fatal(err)
	}
	if _, ok := UserLibraryDoc(id); ok {
		t.Fatal("with the pointer on the shipped version the library still reports a user version")
	}
	if len(LibraryVersions(id)) != 1 {
		t.Fatal("moving to the shipped version dropped the saved one")
	}
}

func TestRetentionKeepsNewestAndCurrent(t *testing.T) {
	freshLibrary(t)
	prev := LibraryVersionLimit
	LibraryVersionLimit = 3
	t.Cleanup(func() { LibraryVersionLimit = prev })
	id := roster.Snare
	for i := 1; i <= 6; i++ {
		if err := SaveInstrumentVersion(id, shippedWith(t, id, float64(i)/10), AudioBundle{}); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(LibraryVersions(id)); n != 3 {
		t.Fatalf("kept %d versions, want the limit of 3", n)
	}
	if got, _ := UserLibraryDoc(id); got.Render.Amplitude != 0.6 {
		t.Fatalf("pruning lost the current version: amp %v", got.Render.Amplitude)
	}
}

func TestSaveFailureKeepsTheLibrary(t *testing.T) {
	lib := freshLibrary(t)
	id := roster.Snare
	if err := SaveInstrumentVersion(id, shippedWith(t, id, 0.5), AudioBundle{}); err != nil {
		t.Fatal(err)
	}
	lib.fail = errors.New("quota exceeded")
	if err := SaveInstrumentVersion(id, shippedWith(t, id, 0.25), AudioBundle{}); err == nil {
		t.Fatal("a save that could not persist reported success")
	}
	if got, _ := UserLibraryDoc(id); got.Render.Amplitude != 0.5 || len(LibraryVersions(id)) != 1 {
		t.Fatalf("a failed save changed the library: amp %v, %d versions", got.Render.Amplitude, len(LibraryVersions(id)))
	}
}

func TestReadInKeepsEverySavedInstrument(t *testing.T) {
	d := shippedWith(t, roster.Snare, 0.4)
	raw, err := instrument.MarshalFile(d)
	if err != nil {
		t.Fatal(err)
	}
	user := shippedWith(t, roster.Kick, 0.3)
	user.ID = instrument.UserID("My Kick", "abcdef")
	rawUser, _ := instrument.MarshalFile(user)
	lib := &memVersionedLibrary{oldDocs: map[instrument.ID][]byte{roster.Snare: raw, user.ID: rawUser}}
	SetInstrumentLibrarySink(lib)
	t.Cleanup(func() { SetInstrumentLibrarySink(nil); _ = LoadUserInstrumentLibrary(&memVersionedLibrary{}, nil) })
	edits := memOldEdits{
		roster.Snare: {"start_frac": 0.25, "end_frac": 1},
		roster.Clap:  {"start_frac": 0.5, "end_frac": 1}, // no saved document: creates no version
	}
	if err := LoadUserInstrumentLibrary(lib, edits); err != nil {
		t.Fatal(err)
	}
	if got, ok := UserLibraryDoc(roster.Snare); !ok || got.Render.Amplitude != 0.4 {
		t.Fatalf("the saved snare was lost in the read-in (%v)", ok)
	}
	if _, ok := UserLibraryDoc(user.ID); !ok {
		t.Fatal("the saved user instrument was lost in the read-in")
	}
	if b, ok := LibraryBundle(roster.Snare); !ok || b.SampleEdit == nil || b.SampleEdit.StartFrac != 0.25 {
		t.Fatalf("the snare's saved Sampler edit did not join its version: %+v", b)
	}
	if _, ok := UserLibraryDoc(roster.Clap); ok {
		t.Fatal("a Sampler edit with no saved document created a version")
	}
	if lib.saves == 0 {
		t.Fatal("the read-in was not persisted in the new store")
	}
}

func TestInstrumentVersionDigestIgnoresIdentity(t *testing.T) {
	a := shippedWith(t, roster.Snare, 0.5)
	b := a.Clone()
	b.ID, b.Origin = "user.other.123456", roster.Snare
	if InstrumentVersionDigest(a, AudioBundle{}) != InstrumentVersionDigest(b, AudioBundle{}) {
		t.Fatal("the same content under another identity hashed differently")
	}
	c := a.Clone()
	c.Render.Amplitude = 0.5000001
	if InstrumentVersionDigest(a, AudioBundle{}) == InstrumentVersionDigest(c, AudioBundle{}) {
		t.Fatal("a changed value kept its version id")
	}
}
