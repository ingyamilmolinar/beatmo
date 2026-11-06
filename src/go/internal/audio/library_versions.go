package audio

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/objects"
)

// library_versions.go — the user library as versioned objects (spec
// docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md
// §2.2–2.4). Every Save As appends an instrument version (its document and the
// version of its audio bundle) to that instrument's history and makes it
// current; nothing is overwritten. The library plays each instrument's current
// version; a history whose pointer is on its base plays the shipped document.

// InstrumentVersion is one saved version of an instrument: its document
// (identity cleared — the same content saved under two names is one version)
// or, for a sample instrument, its PCM's SHA-256, plus its bundle's version.
type InstrumentVersion struct {
	Doc    *instrument.Doc `json:"doc,omitempty"`
	Sample string          `json:"sample,omitempty"`
	Bundle string          `json:"bundle,omitempty"`
}

// LibraryVersionLimit is how many versions of each instrument the library
// keeps (the current one always stays).
var LibraryVersionLimit = 20

func versionOf(d instrument.Doc, bundleDigest string) InstrumentVersion {
	d = d.Clone()
	d.ID, d.Origin = "", ""
	return InstrumentVersion{Doc: &d, Bundle: bundleDigest}
}

// InstrumentVersionDigest is the version id of (d, b).
func InstrumentVersionDigest(d instrument.Doc, b AudioBundle) string {
	bd := ""
	if !b.IsNeutral() {
		bd = b.Digest()
	}
	return objects.Digest(versionOf(d, bd))
}

// putVersionLocked stores version v (its bundle part set from b) and b as
// objects and returns the version digest.
func putVersionLocked(v InstrumentVersion, b AudioBundle) string {
	if b = b.Normalized(); b.IsNeutral() {
		return library.Put(objects.KindInstrument, v)
	}
	v.Bundle = library.Put(objects.KindBundle, b)
	return library.Put(objects.KindInstrument, v, v.Bundle)
}

// SaveInstrumentVersion appends (d, b) to id's history, makes it current,
// prunes past LibraryVersionLimit and persists. A save that cannot persist
// changes nothing and returns the error.
func SaveInstrumentVersion(id instrument.ID, d instrument.Doc, b AudioBundle) error {
	d = d.Clone()
	d.ID, d.Origin = id, ""
	if err := instrument.Validate(d, BuiltinModuleRegistry()); err != nil {
		return err
	}
	return saveVersion(id, versionOf(d, ""), b)
}

// SaveSampleVersion appends a version of the sample instrument id — its PCM by
// SHA-256 (the PCM itself stays in the sample store, persisted here) and b.
func SaveSampleVersion(id instrument.ID, b AudioBundle) error {
	rec, ok := UserSamplePCM(id)
	if !ok {
		return fmt.Errorf("%s is not a sample instrument", id)
	}
	PersistUserSample(id)
	return saveVersion(id, InstrumentVersion{Sample: "sha256:" + HashSamplePCM(rec.PCM, rec.SampleRate)}, b)
}

func saveVersion(id instrument.ID, v InstrumentVersion, b AudioBundle) error {
	userLibMu.Lock()
	defer userLibMu.Unlock()
	before, err := json.Marshal(library)
	if err != nil {
		return err
	}
	library.Save(string(id), putVersionLocked(v, b))
	library.Prune(LibraryVersionLimit)
	if err := persistLibraryLocked(); err != nil {
		restore := objects.NewStore()
		_ = json.Unmarshal(before, restore)
		library = restore
		return err
	}
	rebuildUserLibLocked()
	libraryRev.Add(1)
	return nil
}

// MoveLibraryToShipped points id's history at its base (the shipped document,
// or none), keeping every saved version.
func MoveLibraryToShipped(id instrument.ID) error {
	userLibMu.Lock()
	defer userLibMu.Unlock()
	if _, ok := library.Current(string(id)); !ok {
		return fmt.Errorf("%s: %w", id, ErrNoUserLibraryDoc)
	}
	library.MoveToBase(string(id))
	if err := persistLibraryLocked(); err != nil {
		return err
	}
	rebuildUserLibLocked()
	libraryRev.Add(1)
	return nil
}

// LibraryVersions lists id's saved versions, oldest first.
func LibraryVersions(id instrument.ID) []string {
	userLibMu.RLock()
	defer userLibMu.RUnlock()
	if h := library.Histories[string(id)]; h != nil {
		return slices.Clone(h.Versions)
	}
	return nil
}

// LibraryBundle is the audio bundle of id's current saved version.
func LibraryBundle(id instrument.ID) (AudioBundle, bool) {
	userLibMu.RLock()
	defer userLibMu.RUnlock()
	v, ok := currentVersionLocked(id)
	if !ok {
		return AudioBundle{}, false
	}
	var b AudioBundle
	if v.Bundle != "" {
		if err := library.Body(v.Bundle, &b); err != nil {
			return AudioBundle{}, false
		}
	}
	return b, true
}

func currentVersionLocked(id instrument.ID) (InstrumentVersion, bool) {
	d, ok := library.Current(string(id))
	if !ok {
		return InstrumentVersion{}, false
	}
	var v InstrumentVersion
	if err := library.Body(d, &v); err != nil {
		return InstrumentVersion{}, false
	}
	return v, true
}

// rebuildUserLibLocked derives userLib (each identity's current document)
// from the store; an entry that no longer validates is skipped with a WARN.
func rebuildUserLibLocked() {
	reg := BuiltinModuleRegistry()
	next := map[instrument.ID]instrument.Doc{}
	for identity := range library.Histories {
		id := instrument.ID(identity)
		v, ok := currentVersionLocked(id)
		if !ok || v.Doc == nil {
			continue
		}
		d := v.Doc.Clone()
		d.ID = id
		m, err := instrument.Migrate(d, reg)
		if err == nil {
			err = instrument.Validate(m, reg)
		}
		if err != nil {
			libraryWarnf("[instruments] skipped %s: %v", id, err)
			continue
		}
		next[id] = m
	}
	userLib = next
}

func persistLibraryLocked() error {
	if librarySink == nil {
		return nil
	}
	b, err := json.Marshal(library)
	if err != nil {
		return err
	}
	return librarySink.SaveLibrary(b)
}
