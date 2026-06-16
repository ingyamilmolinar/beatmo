//go:build js

package userprefs

import (
	"sync"
	"syscall/js"
)

// localStorageStore is the WASM backend. Browser localStorage is
// synchronous, single-threaded, and small (typically 5MB cap), so the
// implementation is intentionally minimal — no async pool, no
// double-buffering, no atomic-rename. Reads and writes go straight to
// localStorage.getItem / setItem.
type localStorageStore struct {
	key  string
	logf func(format string, args ...any)

	mu            sync.RWMutex
	favorites     map[string]bool
	overrides     map[string]map[string]float64
	userRecipes   map[string][]byte
	audioPanel    AudioPanelStateDoc
	sampleEdits   map[string]map[string]float64
	notifications []NotificationRecord
	knobSteps     map[string]float64
	language      string
	loaded        bool
}

func newBackingStore(opts Options) Store {
	return &localStorageStore{
		key:  LocalStorageKey,
		logf: opts.Logger,
	}
}

// localStorageOrNil returns localStorage if reachable. In some embedded
// browsers (or under privacy modes) localStorage can be missing — we
// degrade to in-memory rather than crash.
func localStorageOrNil() js.Value {
	defer func() { _ = recover() }()
	g := js.Global()
	if g.IsUndefined() || g.IsNull() {
		return js.Undefined()
	}
	ls := g.Get("localStorage")
	if ls.IsUndefined() || ls.IsNull() {
		return js.Undefined()
	}
	return ls
}

// ensureLoadedLocked populates the cache exactly once. Caller must hold
// the write lock.
func (s *localStorageStore) ensureLoadedLocked() {
	if s.loaded {
		return
	}
	ls := localStorageOrNil()
	if ls.IsUndefined() {
		s.favorites = map[string]bool{}
		s.overrides = map[string]map[string]float64{}
		s.userRecipes = map[string][]byte{}
		s.sampleEdits = map[string]map[string]float64{}
		s.knobSteps = map[string]float64{}
		s.loaded = true
		return
	}
	if val := ls.Call("getItem", s.key); !val.IsNull() && !val.IsUndefined() {
		data := []byte(val.String())
		s.favorites, s.overrides, s.userRecipes, s.audioPanel = parsePrefsV3(data, s.logf)
		s.sampleEdits = parseSampleEdits(data)
		s.notifications = parseNotifications(data)
		s.knobSteps, _ = parseKnobStepsFromPrefs(data)
		s.language = parseLanguageFromPrefs(data)
		s.loaded = true
		return
	}
	s.favorites = map[string]bool{}
	s.overrides = map[string]map[string]float64{}
	s.userRecipes = map[string][]byte{}
	s.sampleEdits = map[string]map[string]float64{}
	s.knobSteps = map[string]float64{}
	s.loaded = true
}

func (s *localStorageStore) LoadFavorites() (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make(map[string]bool, len(s.favorites))
	for k, v := range s.favorites {
		if v {
			out[k] = true
		}
	}
	return out, nil
}

func (s *localStorageStore) SaveFavorites(favs map[string]bool) error {
	snap := make(map[string]bool, len(favs))
	for k, v := range favs {
		if v {
			snap[k] = true
		}
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	s.favorites = snap
	return s.persistLocked()
}

func (s *localStorageStore) SaveRecipeOverride(recipeID string, params map[string]float64) error {
	if recipeID == "" {
		return nil
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	if len(params) == 0 {
		delete(s.overrides, recipeID)
	} else {
		cp := make(map[string]float64, len(params))
		for k, v := range params {
			if !isFiniteFloat(v) {
				continue
			}
			cp[k] = v
		}
		if len(cp) == 0 {
			delete(s.overrides, recipeID)
		} else {
			s.overrides[recipeID] = cp
		}
	}
	return s.persistLocked()
}

func (s *localStorageStore) DeleteRecipeOverride(recipeID string) error {
	if recipeID == "" {
		return nil
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	if _, ok := s.overrides[recipeID]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.overrides, recipeID)
	return s.persistLocked()
}

func (s *localStorageStore) LoadRecipeOverrides() (map[string]map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make(map[string]map[string]float64, len(s.overrides))
	for id, params := range s.overrides {
		cp := make(map[string]float64, len(params))
		for k, v := range params {
			cp[k] = v
		}
		out[id] = cp
	}
	return out, nil
}

// SaveSampleEdit upserts the non-destructive sample-edit descriptor fields
// for an instrument. Empty fields delete (parity with SaveRecipeOverride).
func (s *localStorageStore) SaveSampleEdit(instID string, fields map[string]float64) error {
	if instID == "" {
		return nil
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	if len(fields) == 0 {
		delete(s.sampleEdits, instID)
	} else {
		cp := make(map[string]float64, len(fields))
		for k, v := range fields {
			if !isFiniteFloat(v) {
				continue
			}
			cp[k] = v
		}
		if len(cp) == 0 {
			delete(s.sampleEdits, instID)
		} else {
			s.sampleEdits[instID] = cp
		}
	}
	return s.persistLocked()
}

func (s *localStorageStore) DeleteSampleEdit(instID string) error {
	if instID == "" {
		return nil
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	if _, ok := s.sampleEdits[instID]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.sampleEdits, instID)
	return s.persistLocked()
}

func (s *localStorageStore) LoadSampleEdits() (map[string]map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make(map[string]map[string]float64, len(s.sampleEdits))
	for id, fields := range s.sampleEdits {
		cp := make(map[string]float64, len(fields))
		for k, v := range fields {
			cp[k] = v
		}
		out[id] = cp
	}
	return out, nil
}

func (s *localStorageStore) SaveUserRecipe(recipeID string, doc []byte) error {
	if recipeID == "" || len(doc) == 0 {
		return nil
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	cp := make([]byte, len(doc))
	copy(cp, doc)
	s.userRecipes[recipeID] = cp
	return s.persistLocked()
}

func (s *localStorageStore) DeleteUserRecipe(recipeID string) error {
	if recipeID == "" {
		return nil
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	if _, ok := s.userRecipes[recipeID]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.userRecipes, recipeID)
	return s.persistLocked()
}

func (s *localStorageStore) LoadUserRecipes() (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make(map[string][]byte, len(s.userRecipes))
	for id, raw := range s.userRecipes {
		cp := make([]byte, len(raw))
		copy(cp, raw)
		out[id] = cp
	}
	return out, nil
}

// LoadAudioPanelState returns the persisted Phase-5 audio-panel state.
func (s *localStorageStore) LoadAudioPanelState() (AudioPanelStateDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return s.audioPanel, nil
}

// SaveAudioPanelState upserts the audio-panel section.
func (s *localStorageStore) SaveAudioPanelState(state AudioPanelStateDoc) error {
	s.mu.Lock()
	s.ensureLoadedLocked()
	s.audioPanel = state
	return s.persistLocked()
}

// LoadNotifications returns the persisted notification history (oldest..newest).
func (s *localStorageStore) LoadNotifications() ([]NotificationRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make([]NotificationRecord, len(s.notifications))
	copy(out, s.notifications)
	return out, nil
}

// SaveNotifications upserts the whole notification history, bounded to the
// newest notifHistoryMax entries.
func (s *localStorageStore) SaveNotifications(recs []NotificationRecord) error {
	s.mu.Lock()
	s.ensureLoadedLocked()
	s.notifications = boundNotifications(recs)
	return s.persistLocked()
}

// LoadKnobSteps returns the persisted per-param step-multiplier rungs.
func (s *localStorageStore) LoadKnobSteps() map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make(map[string]float64, len(s.knobSteps))
	for k, v := range s.knobSteps {
		out[k] = v
	}
	return out
}

// SaveKnobStep upserts the chosen step-multiplier for the named param.
func (s *localStorageStore) SaveKnobStep(name string, step float64) error {
	if name == "" || !isFiniteFloat(step) {
		return nil
	}
	s.mu.Lock()
	s.ensureLoadedLocked()
	if s.knobSteps == nil {
		s.knobSteps = map[string]float64{}
	}
	s.knobSteps[name] = step
	return s.persistLocked()
}

// LoadLanguage returns the persisted UI locale ("" = default English).
func (s *localStorageStore) LoadLanguage() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return s.language, nil
}

// SaveLanguage upserts the chosen UI locale.
func (s *localStorageStore) SaveLanguage(lang string) error {
	s.mu.Lock()
	s.ensureLoadedLocked()
	s.language = lang
	return s.persistLocked()
}

// persistLocked marshals the current cache to localStorage. Caller must
// hold s.mu; the lock is released before the JS call so any recover'd
// panic from setItem doesn't strand the mutex.
func (s *localStorageStore) persistLocked() error {
	data, err := marshalPrefsV7(s.favorites, s.overrides, s.userRecipes, s.audioPanel, s.sampleEdits, s.notifications, s.knobSteps, s.language)
	s.mu.Unlock()
	if err != nil {
		return err
	}
	ls := localStorageOrNil()
	if ls.IsUndefined() {
		if s.logf != nil {
			s.logf("[USERPREFS] prefs: localStorage unavailable, in-memory only")
		}
		return nil
	}
	defer func() {
		if r := recover(); r != nil && s.logf != nil {
			s.logf("[USERPREFS] prefs: setItem failed: %v", r)
		}
	}()
	ls.Call("setItem", s.key, string(data))
	return nil
}

// WaitFlushed is a no-op on WASM: localStorage writes are synchronous.
func (s *localStorageStore) WaitFlushed() error { return nil }

func (s *localStorageStore) Close() error { return nil }
