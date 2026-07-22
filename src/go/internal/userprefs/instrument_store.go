package userprefs

import (
	"encoding/json"
	"sync"
)

// Instrument data (user recipes, per-recipe overrides, sample-edit descriptors,
// and the WAV version registry) lives in its OWN store, physically separate from
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
	UserRecipes     map[string]json.RawMessage    `json:"user_recipes,omitempty"`
	RecipeOverrides map[string]map[string]float64 `json:"recipe_overrides,omitempty"`
	SampleEdits     map[string]map[string]float64 `json:"sample_edits,omitempty"`
	WavRegistry     json.RawMessage               `json:"wav_registry,omitempty"`
}

// InstrumentStore is the decoupled persistence surface for instrument data. It
// satisfies the audio package's UserRecipeSource (LoadRecipeOverrides +
// LoadUserRecipes) and the RecipeStore write surface, so bootstraps can point
// audio.ApplyUserRecipeOverrides + ui.SetRecipeSink at it instead of prefs.
type InstrumentStore interface {
	LoadUserRecipes() (map[string][]byte, error)
	SaveUserRecipe(id string, doc []byte) error
	DeleteUserRecipe(id string) error

	LoadRecipeOverrides() (map[string]map[string]float64, error)
	SaveRecipeOverride(recipeID string, params map[string]float64) error
	DeleteRecipeOverride(recipeID string) error

	LoadSampleEdits() (map[string]map[string]float64, error)
	SaveSampleEdit(instID string, fields map[string]float64) error

	LoadWavRegistry() ([]byte, error)
	SaveWavRegistry(raw []byte) error

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
	if err := json.Unmarshal(raw, &s.doc); err != nil && s.logf != nil {
		s.logf("instrument store: parse failed: %v", err)
	}
}

// persistLocked marshals the current doc and writes it through the backend.
func (s *instrumentStore) persistLocked() {
	raw, err := json.MarshalIndent(&s.doc, "", "  ")
	if err != nil {
		if s.logf != nil {
			s.logf("instrument store: marshal failed: %v", err)
		}
		return
	}
	if err := s.backend.write(raw); err != nil && s.logf != nil {
		s.logf("instrument store: write failed: %v", err)
	}
}

func (s *instrumentStore) LoadUserRecipes() (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make(map[string][]byte, len(s.doc.UserRecipes))
	for id, raw := range s.doc.UserRecipes {
		cp := make([]byte, len(raw))
		copy(cp, raw)
		out[id] = cp
	}
	return out, nil
}

func (s *instrumentStore) SaveUserRecipe(id string, doc []byte) error {
	if id == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if s.doc.UserRecipes == nil {
		s.doc.UserRecipes = map[string]json.RawMessage{}
	}
	cp := make([]byte, len(doc))
	copy(cp, doc)
	s.doc.UserRecipes[id] = cp
	s.persistLocked()
	return nil
}

func (s *instrumentStore) DeleteUserRecipe(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if _, ok := s.doc.UserRecipes[id]; !ok {
		return nil
	}
	delete(s.doc.UserRecipes, id)
	s.persistLocked()
	return nil
}

func (s *instrumentStore) LoadRecipeOverrides() (map[string]map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return copyFloatMap2(s.doc.RecipeOverrides), nil
}

func (s *instrumentStore) SaveRecipeOverride(recipeID string, params map[string]float64) error {
	if recipeID == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if len(params) == 0 {
		delete(s.doc.RecipeOverrides, recipeID)
		s.persistLocked()
		return nil
	}
	if s.doc.RecipeOverrides == nil {
		s.doc.RecipeOverrides = map[string]map[string]float64{}
	}
	cp := make(map[string]float64, len(params))
	for k, v := range params {
		cp[k] = v
	}
	s.doc.RecipeOverrides[recipeID] = cp
	s.persistLocked()
	return nil
}

func (s *instrumentStore) DeleteRecipeOverride(recipeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	if _, ok := s.doc.RecipeOverrides[recipeID]; !ok {
		return nil
	}
	delete(s.doc.RecipeOverrides, recipeID)
	s.persistLocked()
	return nil
}

func (s *instrumentStore) LoadSampleEdits() (map[string]map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return copyFloatMap2(s.doc.SampleEdits), nil
}

func (s *instrumentStore) SaveSampleEdit(instID string, fields map[string]float64) error {
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
		s.doc.SampleEdits = map[string]map[string]float64{}
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

// WaitFlushed / Close: writes are synchronous, so these are no-ops (kept to
// satisfy the interface parity with the prefs Store).
func (s *instrumentStore) WaitFlushed() {}
func (s *instrumentStore) Close()       {}

func copyFloatMap2(m map[string]map[string]float64) map[string]map[string]float64 {
	out := make(map[string]map[string]float64, len(m))
	for k, inner := range m {
		cp := make(map[string]float64, len(inner))
		for ik, iv := range inner {
			cp[ik] = iv
		}
		out[k] = cp
	}
	return out
}
