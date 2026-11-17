//go:build js

package userprefs

import (
	"sync"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
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
	favorites     map[instrument.ID]bool
	audioPanel    AudioPanelStateDoc
	sampleEdits   map[instrument.ID]map[string]float64
	notifications []NotificationRecord
	knobSteps     map[string]float64
	language      string
	theme         theme.UserDoc
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
		s.applySnapshotLocked(emptyPrefsSnapshot())
		return
	}
	if val := ls.Call("getItem", s.key); !val.IsNull() && !val.IsUndefined() {
		// A rejected schema_version yields empty-but-usable state; the
		// error is swallowed deliberately (parsePrefs already logged it)
		// because prefs load at boot and must not fail startup.
		snap, _ := parsePrefs([]byte(val.String()), s.logf)
		s.applySnapshotLocked(snap)
		return
	}
	s.applySnapshotLocked(emptyPrefsSnapshot())
}

// snapshotLocked captures the live cache as a snapshot for marshalling.
// Caller must hold s.mu. The maps are handed over by reference — safe
// because marshalPrefs deep-copies every section through its sanitizers.
func (s *localStorageStore) snapshotLocked() prefsSnapshot {
	return prefsSnapshot{
		favorites:     s.favorites,
		audioPanel:    s.audioPanel,
		sampleEdits:   s.sampleEdits,
		notifications: s.notifications,
		knobSteps:     s.knobSteps,
		language:      s.language,
		theme:         s.theme.Clone(),
	}
}

// applySnapshotLocked copies a parsed snapshot into the live cache.
// Caller must hold s.mu.
func (s *localStorageStore) applySnapshotLocked(snap prefsSnapshot) {
	s.favorites = snap.favorites
	s.audioPanel = snap.audioPanel
	s.sampleEdits = snap.sampleEdits
	s.notifications = snap.notifications
	s.knobSteps = snap.knobSteps
	s.language = snap.language
	s.theme = snap.theme
	s.loaded = true
}

func (s *localStorageStore) LoadFavorites() (map[instrument.ID]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make(map[instrument.ID]bool, len(s.favorites))
	for k, v := range s.favorites {
		if v {
			out[k] = true
		}
	}
	return out, nil
}

func (s *localStorageStore) SaveFavorites(favs map[instrument.ID]bool) error {
	snap := make(map[instrument.ID]bool, len(favs))
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

// SaveSampleEdit upserts the non-destructive sample-edit descriptor fields
// for an instrument. Empty fields delete.
func (s *localStorageStore) SaveSampleEdit(instID instrument.ID, fields map[string]float64) error {
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

func (s *localStorageStore) DeleteSampleEdit(instID instrument.ID) error {
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

func (s *localStorageStore) LoadSampleEdits() (map[instrument.ID]map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	out := make(map[instrument.ID]map[string]float64, len(s.sampleEdits))
	for id, fields := range s.sampleEdits {
		cp := make(map[string]float64, len(fields))
		for k, v := range fields {
			cp[k] = v
		}
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

// LoadTheme returns the persisted user theme (empty when never set).
func (s *localStorageStore) LoadTheme() (theme.UserDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ensureLoadedLocked()
	return s.theme.Clone(), nil
}

// SaveTheme upserts the user theme.
func (s *localStorageStore) SaveTheme(d theme.UserDoc) error {
	s.mu.Lock()
	s.ensureLoadedLocked()
	s.theme = d.Clone()
	return s.persistLocked()
}

// persistLocked marshals the current cache to localStorage. Caller must
// hold s.mu; the lock is released before the JS call so any recover'd
// panic from setItem doesn't strand the mutex.
func (s *localStorageStore) persistLocked() error {
	data, err := marshalPrefs(s.snapshotLocked())
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
