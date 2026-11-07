package userprefs

import (
	"encoding/json"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// Instrument data (the library, sample-edit descriptors, the WAV registry) has its OWN store apart from prefs, so
// sharing instruments never touches favorites, language or UI state. Writes are rare (Save As, import), so they are
// synchronous and atomic, not the prefs store's coalescing pool; only raw byte IO is build-tagged.
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
	// One document per id; opaque here, audio parses and validates.
	Instruments map[instrument.ID]json.RawMessage `json:"instruments,omitempty"`
	// Instruments is read once into it and kept as it was.
	Library json.RawMessage `json:"library,omitempty"`
}

// Satisfies audio's library source and sink, so both bootstraps point the library at it, never at prefs.
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

// Path picks the desktop location; wasm always uses InstrumentsLocalStorageKey.
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
	// Loaded automatically at boot, so an unreadable version (absent included) starts empty with a WARN, never a startup
	// failure.
	if err := doc.Validate("instrument store"); err != nil {
		if s.logf != nil {
			s.logf("instrument store: %v; starting from empty", err)
		}
		return
	}
	// Recipe-era sections (user_recipes / recipe_overrides) are not read: one warning, and the next write drops them (no
	// migration before the first release).
	var legacy struct {
		UserRecipes     json.RawMessage `json:"user_recipes"`
		RecipeOverrides json.RawMessage `json:"recipe_overrides"`
	}
	if json.Unmarshal(raw, &legacy) == nil && (len(legacy.UserRecipes) > 0 || len(legacy.RecipeOverrides) > 0) && s.logf != nil {
		s.logf("instrument store: ignoring legacy user_recipes/recipe_overrides (instrument documents replace them; re-save the instruments in this build)")
	}
	s.doc = doc
}

// Writes the whole file; a failure is logged and returned.
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
	if len(fields) == 0 {
		delete(s.doc.SampleEdits, instID)
		s.persistLocked()
		return nil
	}
	if s.doc.SampleEdits == nil {
		s.doc.SampleEdits = map[instrument.ID]map[string]float64{}
	}
	cp := make(map[string]float64, len(fields))
	for k, v := range fields {
		cp[k] = v
	}
	s.doc.SampleEdits[instID] = cp
	s.persistLocked()
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
	cp := make([]byte, len(raw))
	copy(cp, raw)
	s.doc.WavRegistry = cp
	s.persistLocked()
	return nil
}

// Writes are synchronous, so these are no-ops for parity with the prefs Store.
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
