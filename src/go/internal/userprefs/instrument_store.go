package userprefs

import (
	"encoding/json"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// Instrument data (the user's instrument library, sample-edit descriptors, and
// the WAV version registry) lives in its OWN store, physically separate from
// the prefs blob — so sharing/importing instruments never touches favorites,
// language, or UI state, and vice-versa. See
// docs/superpowers/specs/2026-07-19-portable-instrument-format-design.md.
//
// Instruments mutate infrequently (Save As / import), so this store writes
// synchronously with an atomic replace rather than the coalescing async pool the
// high-churn prefs store uses. All doc logic is platform-agnostic; only raw
// byte read/write is behind build tags (instrumentBackend).
const (
	// InstrumentsFileName is the desktop instrument-store file (sibling of prefs.json).
	InstrumentsFileName = "instruments.json"
	// InstrumentsLocalStorageKey is the browser instrument-store key (sibling of beatmo.prefs).
	InstrumentsLocalStorageKey = "beatmo.instruments"
)

// instrumentsDoc is the on-the-wire schema for the instrument store.
type instrumentsDoc struct {
	schema.Envelope
	SampleEdits map[instrument.ID]map[string]float64 `json:"sample_edits,omitempty"`
	WavRegistry json.RawMessage                      `json:"wav_registry,omitempty"`
	// Instruments holds the user's library: one instrument document per id
	// (spec 2026-10-04 §6). Opaque bytes here; audio parses and validates.
	Instruments map[instrument.ID]json.RawMessage `json:"instruments,omitempty"`
	// Library is the versioned instrument library (audio's objects store).
	// Since it exists, Instruments is only ever read (once, into it) and is
	// kept as it was.
	Library json.RawMessage `json:"library,omitempty"`
}

// InstrumentStore is the decoupled persistence surface for instrument data. It
// satisfies audio.InstrumentLibrarySource + InstrumentLibrarySink (the user's
// instrument documents), so bootstraps point audio.LoadUserInstrumentLibrary +
// audio.SetInstrumentLibrarySink at it instead of prefs.
type InstrumentStore interface {
	LoadSampleEdits() (map[instrument.ID]map[string]float64, error)
	SaveSampleEdit(instID instrument.ID, fields map[string]float64) error

	LoadWavRegistry() ([]byte, error)
	SaveWavRegistry(raw []byte) error

	LoadInstrumentDocs() (map[instrument.ID][]byte, error)
	LoadLibrary() ([]byte, error)
	SaveLibrary(blob []byte) error

	WaitFlushed()
	Close()
}

// instrumentBackend abstracts the raw persistence of the marshaled doc. Platform
// files provide newInstrumentBackend(opts).
type instrumentBackend interface {
	// read returns the stored bytes, or (nil, nil) when nothing is stored.
	read() ([]byte, error)
	write([]byte) error
}

type instrumentStore struct {
	mu      sync.Mutex
	doc     instrumentsDoc
	loaded  bool
	backend instrumentBackend
	logf    func(format string, args ...any)
}

// NewInstrumentStore constructs the platform-appropriate decoupled instrument
// store. Reuses userprefs.Options (Path selects the desktop location; ignored on
// wasm, which always uses InstrumentsLocalStorageKey).
func NewInstrumentStore(opts Options) InstrumentStore {
	return &instrumentStore{
		backend: newInstrumentBackend(opts),
		logf:    opts.Logger,
	}
}

func (s *instrumentStore) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	s.loaded = true
	s.doc = instrumentsDoc{}
	raw, err := s.backend.read()
	if err != nil {
		if s.logf != nil {
			s.logf("instrument store: read failed: %v", err)
		}
		return
	}
	if len(raw) == 0 {
		return
	}
	var doc instrumentsDoc
	if err := json.Unmarshal(raw, &doc); err != nil {
		if s.logf != nil {
			s.logf("instrument store: parse failed: %v", err)
		}
		return
	}
	// Same strict rule as prefs, and the same recovery: this store is loaded
	// automatically at boot, so an unreadable version — an absent one
	// included — starts from empty (WARN) rather than failing startup.
	if err := doc.Validate("instrument store"); err != nil {
		if s.logf != nil {
			s.logf("instrument store: %v; starting from empty", err)
		}
		return
	}
	// The recipe-era sections (user_recipes / recipe_overrides) predate
	// instrument documents and are not read; say so once, and the next write
	// drops them (spec 2026-10-04 §6: no migration before the first release).
	var legacy struct {
		UserRecipes     json.RawMessage `json:"user_recipes"`
		RecipeOverrides json.RawMessage `json:"recipe_overrides"`
	}
	if json.Unmarshal(raw, &legacy) == nil && (len(legacy.UserRecipes) > 0 || len(legacy.RecipeOverrides) > 0) && s.logf != nil {
		s.logf("instrument store: ignoring legacy user_recipes/recipe_overrides (instrument documents replace them; re-save the instruments in this build)")
	}
	s.doc = doc
}

// persistLocked marshals the current doc and writes it through the backend.
// persistLocked writes the whole file; a failure is logged and returned.
func (s *instrumentStore) persistLocked() error {
	s.doc.Envelope = schema.Current()
	raw, err := json.MarshalIndent(&s.doc, "", "  ")
	if err == nil {
		err = s.backend.write(raw)
	}
	if err != nil && s.logf != nil {
		s.logf("instrument store: write failed: %v", err)
	}
	return err
}

func (s *instrumentStore) LoadSampleEdits() (map[instrument.ID]map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return copyFloatMap2(s.doc.SampleEdits), nil
}

func (s *instrumentStore) SaveSampleEdit(instID instrument.ID, fields map[string]float64) error {
	if instID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	prev, had := s.doc.SampleEdits[instID]
	if len(fields) == 0 {
		delete(s.doc.SampleEdits, instID)
	} else {
		if s.doc.SampleEdits == nil {
			s.doc.SampleEdits = map[instrument.ID]map[string]float64{}
		}
		cp := make(map[string]float64, len(fields))
		for k, v := range fields {
			cp[k] = v
		}
		s.doc.SampleEdits[instID] = cp
	}
	if err := s.persistLocked(); err != nil {
		if had {
			s.doc.SampleEdits[instID] = prev
		} else {
			delete(s.doc.SampleEdits, instID)
		}
		return err
	}
	return nil
}

func (s *instrumentStore) LoadWavRegistry() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if len(s.doc.WavRegistry) == 0 {
		return nil, nil
	}
	cp := make([]byte, len(s.doc.WavRegistry))
	copy(cp, s.doc.WavRegistry)
	return cp, nil
}

func (s *instrumentStore) SaveWavRegistry(raw []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	prev := s.doc.WavRegistry
	s.doc.WavRegistry = append([]byte(nil), raw...)
	if err := s.persistLocked(); err != nil {
		s.doc.WavRegistry = prev
		return err
	}
	return nil
}

// WaitFlushed / Close: writes are synchronous, so these are no-ops (kept to
// satisfy the interface parity with the prefs Store).
func (s *instrumentStore) WaitFlushed() {}
func (s *instrumentStore) Close()       {}

func copyFloatMap2(m map[instrument.ID]map[string]float64) map[instrument.ID]map[string]float64 {
	out := make(map[instrument.ID]map[string]float64, len(m))
	for k, inner := range m {
		cp := make(map[string]float64, len(inner))
		for ik, iv := range inner {
			cp[ik] = iv
		}
		out[k] = cp
	}
	return out
}

func (s *instrumentStore) LoadInstrumentDocs() (map[instrument.ID][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make(map[instrument.ID][]byte, len(s.doc.Instruments))
	for id, raw := range s.doc.Instruments {
		out[id] = append([]byte(nil), raw...)
	}
	return out, nil
}

// LoadLibrary returns the versioned instrument library (audio's objects
// store, opaque here); nil when none was saved yet.
func (s *instrumentStore) LoadLibrary() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if len(s.doc.Library) == 0 {
		return nil, nil
	}
	return append([]byte(nil), s.doc.Library...), nil
}

// SaveLibrary replaces the versioned library. A write that fails returns its
// error and leaves the stored library as it was.
func (s *instrumentStore) SaveLibrary(blob []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	prev := s.doc.Library
	s.doc.Library = append(json.RawMessage(nil), blob...)
	if err := s.persistLocked(); err != nil {
		s.doc.Library = prev
		return err
	}
	return nil
}
