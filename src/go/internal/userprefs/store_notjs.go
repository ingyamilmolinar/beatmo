//go:build !js

package userprefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/theme"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// Reads synchronously at start (a small file). Writes coalesce latest-write-wins: every Save* dirties one flag, and the
// worker snapshots the whole state under the mutex and writes prefs.json atomically, so a flurry of toggles is at most
// one queued job. The pool is acquired on the first save, so a load-only session pays no goroutine.
type fileStore struct {
	path     string
	poolName string
	logf     func(format string, args ...any)

	mu        sync.Mutex
	favorites map[instrument.ID]bool // mirrors disk + most-recent Save
	// Zero when no override is active, so older files do not grow.
	audioPanel AudioPanelStateDoc
	// sampleEdits mirrors the on-disk non-destructive sample-edit
	// descriptors (instrument id → field → value).
	sampleEdits map[instrument.ID]map[string]float64
	// notifications mirrors the on-disk bounded notification history
	// (oldest..newest).
	notifications []NotificationRecord
	// knobSteps mirrors the on-disk per-param step-multiplier rung choices.
	knobSteps map[string]float64
	// language mirrors the on-disk chosen UI locale ("" = default English).
	language string
	// theme mirrors the on-disk user theme (empty = shipped design).
	theme  theme.UserDoc
	loaded bool

	dirty    bool           // queued write pending (any section modified)
	inflight bool           // true between worker submit and worker exit
	flushed  sync.WaitGroup // tracks any work the worker still owes

	poolOnce sync.Once
	poolErr  error
	pool     *async.Pool
}

// Only the desktop backend uses a pool.
func optsPoolName(o Options) string {
	if o.PoolName != "" {
		return o.PoolName
	}
	return PoolName
}

func newBackingStore(opts Options) Store {
	// Empty → <UserConfigDir>/beatmo/prefs.json, *.json → that exact target, a bare dir → <dir>/prefs.json.
	var path string
	switch {
	case opts.Path == "":
		base, err := os.UserConfigDir()
		if err != nil {
			home, _ := os.UserHomeDir()
			if home == "" {
				home = os.TempDir()
			}
			base = filepath.Join(home, ".config")
		}
		path = filepath.Join(base, "beatmo", FileName)
	case filepath.Ext(opts.Path) == ".json":
		path = opts.Path
	default:
		path = filepath.Join(opts.Path, FileName)
	}
	return &fileStore{
		path:     path,
		poolName: optsPoolName(opts),
		logf:     opts.Logger,
	}
}

// ensurePool acquires the async pool on first need. Idempotent and
// goroutine-safe.
func (s *fileStore) ensurePool() (*async.Pool, error) {
	s.poolOnce.Do(func() {
		p, err := async.DefaultRegistry().Get(s.poolName, async.Options{
			MaxConcurrent: 1,
			QueueSize:     8,
			Name:          s.poolName,
		})
		s.pool = p
		s.poolErr = err
	})
	return s.pool, s.poolErr
}

// Read errors other than a missing file leave the cache empty and return the error. Caller holds s.mu.
func (s *fileStore) ensureLoadedLocked() error {
	if s.loaded {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err == nil {
		// A rejected version yields empty state; the error is dropped on purpose (parsePrefs logged it) because prefs load at
		// boot and must not fail startup.
		snap, _ := parsePrefs(data, s.logf)
		s.applySnapshotLocked(snap)
		return nil
	}
	s.applySnapshotLocked(emptyPrefsSnapshot())
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// applySnapshotLocked copies a parsed snapshot into the live cache.
// Caller must hold s.mu.
func (s *fileStore) applySnapshotLocked(snap prefsSnapshot) {
	s.favorites = snap.favorites
	s.audioPanel = snap.audioPanel
	s.sampleEdits = snap.sampleEdits
	s.notifications = snap.notifications
	s.knobSteps = snap.knobSteps
	s.language = snap.language
	s.theme = snap.theme
	s.loaded = true
}

func (s *fileStore) LoadFavorites() (map[instrument.ID]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return map[instrument.ID]bool{}, err
	}
	return cloneFavMap(s.favorites), nil
}

// Returns once the state is captured; the write runs on the pool, and since the worker snapshots the latest state, many
// toggles mean at most one write in flight plus one queued.
func (s *fileStore) SaveFavorites(favs map[instrument.ID]bool) error {
	snap := snapshotFavs(favs)
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	s.favorites = snap
	return s.queueWriteLocked()
}

// SaveSampleEdit upserts the descriptor fields for an instrument and
// schedules the same write path the other sections use. Empty fields
// delete the entry.
func (s *fileStore) SaveSampleEdit(instID instrument.ID, fields map[string]float64) error {
	if instID == "" {
		return nil
	}
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
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
	return s.queueWriteLocked()
}

func (s *fileStore) DeleteSampleEdit(instID instrument.ID) error {
	if instID == "" {
		return nil
	}
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	if _, ok := s.sampleEdits[instID]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.sampleEdits, instID)
	return s.queueWriteLocked()
}

func (s *fileStore) LoadSampleEdits() (map[instrument.ID]map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return map[instrument.ID]map[string]float64{}, err
	}
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

// The single write chokepoint. Releases s.mu before Submit and never waits; if the pool or Submit fails, a synchronous
// atomic write keeps the state durable.
func (s *fileStore) queueWriteLocked() error {
	pool, err := s.ensurePool()
	if err != nil {
		// Snapshot under the lock for a synchronous write, then release.
		snap := s.snapshotPrefsLocked()
		s.mu.Unlock()
		return s.writeAtomic(snap)
	}
	s.dirty = true
	if s.inflight {
		s.mu.Unlock()
		return nil
	}
	s.inflight = true
	s.flushed.Add(1)
	s.mu.Unlock()

	if submitErr := pool.Submit(s.drainPending); submitErr != nil {
		s.mu.Lock()
		s.dirty = false
		s.inflight = false
		snap := s.snapshotPrefsLocked()
		s.mu.Unlock()
		s.flushed.Done()
		if s.logf != nil {
			s.logf("[USERPREFS] prefs pool unavailable (%v); writing synchronously", submitErr)
		}
		return s.writeAtomic(snap)
	}
	return nil
}

// LoadAudioPanelState returns the persisted Phase-5 audio-panel state.
func (s *fileStore) LoadAudioPanelState() (AudioPanelStateDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return AudioPanelStateDoc{}, err
	}
	return s.audioPanel, nil
}

// SaveAudioPanelState upserts the audio-panel section. Defaults
// (IsDefault() true) clear the on-disk section.
func (s *fileStore) SaveAudioPanelState(state AudioPanelStateDoc) error {
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	s.audioPanel = state
	return s.queueWriteLocked()
}

// LoadNotifications returns the persisted notification history (oldest..newest).
func (s *fileStore) LoadNotifications() ([]NotificationRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return nil, err
	}
	out := make([]NotificationRecord, len(s.notifications))
	copy(out, s.notifications)
	return out, nil
}

// SaveNotifications upserts the whole notification history, bounded to the
// newest notifHistoryMax entries.
func (s *fileStore) SaveNotifications(recs []NotificationRecord) error {
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	s.notifications = boundNotifications(recs)
	return s.queueWriteLocked()
}

// LoadKnobSteps returns the persisted per-param step-multiplier rungs.
func (s *fileStore) LoadKnobSteps() map[string]float64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.ensureLoadedLocked()
	out := make(map[string]float64, len(s.knobSteps))
	for k, v := range s.knobSteps {
		out[k] = v
	}
	return out
}

// SaveKnobStep upserts the chosen step-multiplier for the named param.
func (s *fileStore) SaveKnobStep(name string, step float64) error {
	if name == "" || !isFiniteFloat(step) {
		return nil
	}
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	if s.knobSteps == nil {
		s.knobSteps = map[string]float64{}
	}
	s.knobSteps[name] = step
	return s.queueWriteLocked()
}

// LoadLanguage returns the persisted UI locale ("" = default English).
func (s *fileStore) LoadLanguage() (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.ensureLoadedLocked()
	return s.language, nil
}

// SaveLanguage upserts the chosen UI locale.
func (s *fileStore) SaveLanguage(lang string) error {
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	s.language = lang
	return s.queueWriteLocked()
}

// LoadTheme returns the persisted user theme (empty when never set).
func (s *fileStore) LoadTheme() (theme.UserDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.ensureLoadedLocked()
	return s.theme.Clone(), nil
}

// SaveTheme upserts the user theme.
func (s *fileStore) SaveTheme(d theme.UserDoc) error {
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	s.theme = d.Clone()
	return s.queueWriteLocked()
}

func (s *fileStore) snapshotPrefsLocked() prefsSnapshot {
	favs := make(map[instrument.ID]bool, len(s.favorites))
	for k, v := range s.favorites {
		if v {
			favs[k] = true
		}
	}
	sampleEdits := make(map[instrument.ID]map[string]float64, len(s.sampleEdits))
	for id, fields := range s.sampleEdits {
		cp := make(map[string]float64, len(fields))
		for k, v := range fields {
			cp[k] = v
		}
		sampleEdits[id] = cp
	}
	notifs := make([]NotificationRecord, len(s.notifications))
	copy(notifs, s.notifications)
	knobSteps := make(map[string]float64, len(s.knobSteps))
	for k, v := range s.knobSteps {
		knobSteps[k] = v
	}
	return prefsSnapshot{
		favorites:     favs,
		audioPanel:    s.audioPanel,
		sampleEdits:   sampleEdits,
		notifications: notifs,
		knobSteps:     knobSteps,
		language:      s.language,
		theme:         s.theme.Clone(),
	}
}

// Loops until nothing is pending, snapshotting each time, so a write racing new saves ends on the freshest state.
func (s *fileStore) drainPending(_ context.Context) {
	defer s.flushed.Done()
	for {
		s.mu.Lock()
		if !s.dirty {
			s.inflight = false
			s.mu.Unlock()
			return
		}
		s.dirty = false
		snap := s.snapshotPrefsLocked()
		s.mu.Unlock()
		if err := s.writeAtomic(snap); err != nil && s.logf != nil {
			s.logf("[USERPREFS] prefs write failed: %v", err)
		}
	}
}

// snapshotFavs returns a copy of favs containing only keys whose value
// is true. Caller can safely mutate the original map after we return.
func snapshotFavs(favs map[instrument.ID]bool) map[instrument.ID]bool {
	out := make(map[instrument.ID]bool, len(favs))
	for k, v := range favs {
		if v {
			out[k] = true
		}
	}
	return out
}

func cloneFavMap(in map[instrument.ID]bool) map[instrument.ID]bool {
	out := make(map[instrument.ID]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// Temp + rename: a crash mid-write leaves the old or the new state, never a truncated half.
func (s *fileStore) writeAtomic(snap prefsSnapshot) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := marshalPrefs(snap)
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *fileStore) WaitFlushed() error {
	s.flushed.Wait()
	return nil
}

func (s *fileStore) Close() error {
	s.flushed.Wait()
	return nil
}
