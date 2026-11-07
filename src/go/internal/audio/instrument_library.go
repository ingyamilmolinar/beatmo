package audio

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/ingyamilmolinar/beatmo/internal/assets/instrumentpack"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
	"github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/objects"
)

// The shipped documents from the embedded pack plus the user's saved documents.

var (
	packOnce sync.Once
	packData instrumentpack.Pack
)

func shippedPack() *instrumentpack.Pack {
	packOnce.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(instrumentpack.Embedded()))
		if err != nil {
			panic(fmt.Errorf("instrument pack: %w", err))
		}
		raw, err := io.ReadAll(zr)
		if err != nil {
			panic(fmt.Errorf("instrument pack: %w", err))
		}
		if err := json.Unmarshal(raw, &packData); err != nil {
			panic(fmt.Errorf("instrument pack: %w", err))
		}
		if err := packData.Envelope.Validate("instrument pack"); err != nil {
			panic(err)
		}
		// Skipped and logged, never a crash at package init, which would also lock out the tools that regenerate the pack.
		reg := BuiltinModuleRegistry()
		for h, d := range packData.Docs {
			if err := instrument.Validate(d, reg); err != nil {
				packInvalid = append(packInvalid, fmt.Sprintf("%s: %v", h, err))
				log.Warnf("[instruments] skipped shipped document %s: %v", h, err)
				delete(packData.Docs, h)
			}
		}
		for id, h := range packData.Shipped {
			if _, ok := packData.Docs[h]; !ok {
				delete(packData.Shipped, id)
			}
		}
		kept := packData.Order[:0]
		for _, id := range packData.Order {
			if _, ok := packData.Shipped[id]; ok {
				kept = append(kept, id)
			}
		}
		packData.Order = kept
	})
	return &packData
}

// packInvalid lists shipped documents the build skipped (TestShippedPackValidates).
var packInvalid []string

// ShippedDoc returns a copy of the shipped document for id.
func ShippedDoc(id instrument.ID) (instrument.Doc, bool) {
	p := shippedPack()
	h, ok := p.Shipped[id]
	if !ok {
		return instrument.Doc{}, false
	}
	d := p.Docs[h].Clone()
	d.ID = id
	return d, true
}

// PackDoc returns a copy of the pack document with content hash h.
func PackDoc(h string) (instrument.Doc, bool) {
	d, ok := shippedPack().Docs[h]
	if !ok {
		return instrument.Doc{}, false
	}
	return d.Clone(), true
}

// ── The user's library: saved documents, shadowing shipped ids ──

// The versioned store as one blob, plus (read once, while no store exists) the documents saved before versioning.
type InstrumentLibrarySource interface {
	LoadLibrary() ([]byte, error)
	LoadInstrumentDocs() (map[instrument.ID][]byte, error)
}

// InstrumentLibrarySink persists the versioned store.
type InstrumentLibrarySink interface {
	SaveLibrary(blob []byte) error
}

// ErrNoUserLibraryDoc is returned when deleting an id the user never saved.
var ErrNoUserLibraryDoc = errors.New("no saved instrument with that id")

// libraryRev bumps on every change of the user library (dirty-dot cache).
var libraryRev atomic.Uint64

var (
	userLibMu sync.RWMutex
	// userLib is derived from library: each identity's current document.
	library     = objects.NewStore()
	userLib     = map[instrument.ID]instrument.Doc{}
	librarySink InstrumentLibrarySink
)

// libraryWarnf reports a skipped stored entry (boot-store rule: WARN + skip).
var libraryWarnf = log.Warnf

func setLibraryWarnfForTest(f func(string, ...any)) (restore func()) {
	prev := libraryWarnf
	libraryWarnf = f
	return func() { libraryWarnf = prev }
}

// nil = memory only.
func SetInstrumentLibrarySink(s InstrumentLibrarySink) {
	userLibMu.Lock()
	librarySink = s
	userLibMu.Unlock()
}

// With no versioned store yet, documents saved before versioning become version 1 (a Sampler edit saved for one joins
// its bundle). A bad entry is skipped with one WARN: a boot store never fails startup.
func LoadUserInstrumentLibrary(src InstrumentLibrarySource, oldEdits SampleEditSource) error {
	blob, err := src.LoadLibrary()
	if err != nil {
		return err
	}
	next := objects.NewStore()
	if len(blob) > 0 {
		if err := json.Unmarshal(blob, next); err != nil {
			libraryWarnf("[instruments] unreadable library, starting empty: %v", err)
			next = objects.NewStore()
		}
	}
	userLibMu.Lock()
	defer userLibMu.Unlock()
	library = next
	if len(blob) == 0 {
		if err := readInLegacyLibraryLocked(src, oldEdits); err != nil {
			return err
		}
	}
	rebuildUserLibLocked()
	libraryRev.Add(1)
	return nil
}

func readInLegacyLibraryLocked(src InstrumentLibrarySource, oldEdits SampleEditSource) error {
	raw, err := src.LoadInstrumentDocs()
	if err != nil || len(raw) == 0 {
		return err
	}
	var edits map[instrument.ID]map[string]float64
	if oldEdits != nil {
		edits, _ = oldEdits.LoadSampleEdits()
	}
	reg := BuiltinModuleRegistry()
	for _, id := range slices.Sorted(maps.Keys(raw)) {
		d, err := parseLibraryDoc(id, raw[id], reg)
		if err != nil {
			libraryWarnf("[instruments] skipped %s: %v", id, err)
			continue
		}
		var b AudioBundle
		if f, ok := edits[id]; ok && len(f) > 0 {
			e := SampleEditFromFields(f)
			b.SampleEdit = &e
		}
		library.Save(string(id), putVersionLocked(versionOf(d, ""), b))
	}
	if err := persistLibraryLocked(); err != nil {
		libraryWarnf("[instruments] could not store the library: %v", err)
	}
	return nil
}

func parseLibraryDoc(id instrument.ID, b []byte, reg *instrument.Registry) (instrument.Doc, error) {
	d, err := instrument.ParseFile(b)
	if err != nil {
		return instrument.Doc{}, err
	}
	if d.ID != id {
		return instrument.Doc{}, fmt.Errorf("entry %q holds id %q", id, d.ID)
	}
	if d, err = instrument.Migrate(d, reg); err != nil {
		return instrument.Doc{}, err
	}
	if err := instrument.Validate(d, reg); err != nil {
		return instrument.Doc{}, err
	}
	return d, nil
}

// UserLibraryDoc returns a copy of the user's saved document for id.
func UserLibraryDoc(id instrument.ID) (instrument.Doc, bool) {
	userLibMu.RLock()
	defer userLibMu.RUnlock()
	d, ok := userLib[id]
	if !ok {
		return instrument.Doc{}, false
	}
	return d.Clone(), true
}

// The user's saved one when it exists, else the shipped one.
func LibraryDoc(id instrument.ID) (instrument.Doc, bool) {
	if d, ok := UserLibraryDoc(id); ok {
		return d, true
	}
	return ShippedDoc(id)
}

// Shipped ids in shipped order, then user-only ids sorted.
func LibraryIDs() []instrument.ID {
	out := slices.Clone(roster.All)
	shipped := map[instrument.ID]bool{}
	for _, id := range out {
		shipped[id] = true
	}
	userLibMu.RLock()
	var extra []instrument.ID
	for id := range userLib {
		if !shipped[id] {
			extra = append(extra, id)
		}
	}
	userLibMu.RUnlock()
	slices.Sort(extra)
	return append(out, extra...)
}

// SaveLibraryDoc saves d as a new version of d.ID with no audio bundle.
func SaveLibraryDoc(d instrument.Doc) error {
	return SaveInstrumentVersion(d.ID, d, AudioBundle{})
}

// Keeps every saved version.
func DeleteUserLibraryDoc(id instrument.ID) error { return MoveLibraryToShipped(id) }

// userOnlyLibraryIDs are the library ids that are not shipped, sorted.
func userOnlyLibraryIDs() []instrument.ID {
	return LibraryIDs()[len(roster.All):]
}

// After a Save As or bundle import adds a user instrument mid-session.
func RegisterLibraryInstrument(id instrument.ID) {
	if _, ok := EnsureInstrumentDoc(id); ok {
		registerDocInstrument(id)
	}
}

// "user.<slug(name)>.<6 hex of the content hash>": content addressed, because a project file must be a pure function
// of the project, never of the clock.
func NewUserInstrumentID(name string, d instrument.Doc) instrument.ID {
	d.Name = name
	h := instrument.ContentHash(d)
	return instrument.UserID(name, h[len("sha256:"):len("sha256:")+6])
}
