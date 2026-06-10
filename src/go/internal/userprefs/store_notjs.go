//go:build !js

package userprefs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// fileStore is the desktop / native backend. Reads happen synchronously
// at process start (small file, blocks for a few ms at most). Writes
// follow a coalescing latest-write-wins pattern so a flurry of toggles
// collapses into at most one queued worker job. The pool always has
// queue depth ≤1 — there is no backpressure path under normal load.
//
// The pool is lazy-acquired on the first SaveFavorites / SaveRecipeOverride
// / SaveUserRecipe call. This keeps the 1-worker budget free until the
// user actually mutates state, so a session that only loads at startup
// pays no goroutine cost.
//
// All Save* methods funnel through queueWriteLocked which dirties a single
// flag — the worker always snapshots the entire prefs state under the
// mutex and writes prefs.json atomically.
type fileStore struct {
	path     string
	poolName string
	logf     func(format string, args ...any)

	mu          sync.Mutex
	favorites   map[string]bool               // mirrors disk + most-recent Save
	overrides   map[string]map[string]float64 // recipe id → param name → value
	userRecipes map[string][]byte             // recipe id → opaque RecipeDoc bytes
	// audioPanel mirrors the on-disk Phase-5 audio-panel state.
	// Zero-value AudioPanelStateDoc when no override is active so
	// older prefs files don't grow without need.
	audioPanel AudioPanelStateDoc
	// sampleEdits mirrors the on-disk non-destructive sample-edit
	// descriptors (instrument id → field → value).
	sampleEdits map[string]map[string]float64
	loaded      bool

	dirty    bool           // queued write pending (any section modified)
	inflight bool           // true between worker submit and worker exit
	flushed  sync.WaitGroup // tracks any work the worker still owes

	poolOnce sync.Once
	poolErr  error
	pool     *async.Pool
}

// poolName returns the effective async-pool name. Lives here because
// only the desktop backend uses a pool.
func optsPoolName(o Options) string {
	if o.PoolName != "" {
		return o.PoolName
	}
	return PoolName
}

func newBackingStore(opts Options) Store {
	// Options.Path semantics:
	//   - empty: production default — <UserConfigDir>/beatmo/prefs.json.
	//   - ends in .json: caller picks the exact write target.
	//   - bare directory: write <dir>/prefs.json.
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

// ensureLoadedLocked loads disk state into the cache exactly once. Caller
// must hold s.mu. Read errors (other than missing-file) leave the cache
// empty + return the error to the caller.
func (s *fileStore) ensureLoadedLocked() error {
	if s.loaded {
		return nil
	}
	data, err := os.ReadFile(s.path)
	if err == nil {
		s.favorites, s.overrides, s.userRecipes, s.audioPanel = parsePrefsV3(data, s.logf)
		s.sampleEdits = parseSampleEdits(data)
		s.loaded = true
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		s.favorites = map[string]bool{}
		s.overrides = map[string]map[string]float64{}
		s.userRecipes = map[string][]byte{}
		s.sampleEdits = map[string]map[string]float64{}
		s.loaded = true
		return err
	}
	s.favorites = map[string]bool{}
	s.overrides = map[string]map[string]float64{}
	s.userRecipes = map[string][]byte{}
	s.sampleEdits = map[string]map[string]float64{}
	s.loaded = true
	return nil
}

func (s *fileStore) LoadFavorites() (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return map[string]bool{}, err
	}
	return cloneFavMap(s.favorites), nil
}

// SaveFavorites schedules a write to disk. The call returns once the
// new state is captured; the actual write happens on the userprefs.persist
// pool. Rapid successive saves coalesce: the worker always snapshots the
// latest cache state under the mutex, so 100 toggles produce at most one
// in-flight write + one queued write at any moment.
func (s *fileStore) SaveFavorites(favs map[string]bool) error {
	snap := snapshotFavs(favs)
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	s.favorites = snap
	return s.queueWriteLocked()
}

// SaveRecipeOverride upserts the recipe-id → param-name → value map and
// schedules the same write path SaveFavorites uses. Empty params removes
// the override (parity with DeleteRecipeOverride).
func (s *fileStore) SaveRecipeOverride(recipeID string, params map[string]float64) error {
	if recipeID == "" {
		return nil
	}
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
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
	return s.queueWriteLocked()
}

func (s *fileStore) DeleteRecipeOverride(recipeID string) error {
	if recipeID == "" {
		return nil
	}
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	if _, ok := s.overrides[recipeID]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.overrides, recipeID)
	return s.queueWriteLocked()
}

func (s *fileStore) LoadRecipeOverrides() (map[string]map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return map[string]map[string]float64{}, err
	}
	out := make(map[string]map[string]float64, len(s.overrides))
	for recipeID, params := range s.overrides {
		cp := make(map[string]float64, len(params))
		for k, v := range params {
			cp[k] = v
		}
		out[recipeID] = cp
	}
	return out, nil
}

func (s *fileStore) SaveUserRecipe(recipeID string, doc []byte) error {
	if recipeID == "" || len(doc) == 0 {
		return nil
	}
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	cp := make([]byte, len(doc))
	copy(cp, doc)
	s.userRecipes[recipeID] = cp
	return s.queueWriteLocked()
}

func (s *fileStore) DeleteUserRecipe(recipeID string) error {
	if recipeID == "" {
		return nil
	}
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	if _, ok := s.userRecipes[recipeID]; !ok {
		s.mu.Unlock()
		return nil
	}
	delete(s.userRecipes, recipeID)
	return s.queueWriteLocked()
}

// SaveSampleEdit upserts the descriptor fields for an instrument and
// schedules the same write path the other sections use. Empty fields
// delete the entry (parity with SaveRecipeOverride).
func (s *fileStore) SaveSampleEdit(instID string, fields map[string]float64) error {
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

func (s *fileStore) DeleteSampleEdit(instID string) error {
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

func (s *fileStore) LoadSampleEdits() (map[string]map[string]float64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return map[string]map[string]float64{}, err
	}
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

func (s *fileStore) LoadUserRecipes() (map[string][]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensureLoadedLocked(); err != nil {
		return map[string][]byte{}, err
	}
	out := make(map[string][]byte, len(s.userRecipes))
	for id, raw := range s.userRecipes {
		cp := make([]byte, len(raw))
		copy(cp, raw)
		out[id] = cp
	}
	return out, nil
}

// queueWriteLocked is the single write-scheduling chokepoint. Caller must
// hold s.mu; this method releases the lock before submitting the worker
// (so Submit's bookkeeping doesn't run under our mutex) and returns
// without waiting for the write to land. If the pool can't be acquired
// or Submit fails, we fall back to a synchronous atomic write so the user's
// state is at least durable for this session.
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

// prefsSnapshot is the immutable state the worker writes. Distinct
// from the live cache so the worker can drop the lock before disk I/O.
type prefsSnapshot struct {
	favorites   map[string]bool
	overrides   map[string]map[string]float64
	userRecipes map[string][]byte
	audioPanel  AudioPanelStateDoc
	sampleEdits map[string]map[string]float64
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

func (s *fileStore) snapshotPrefsLocked() prefsSnapshot {
	favs := make(map[string]bool, len(s.favorites))
	for k, v := range s.favorites {
		if v {
			favs[k] = true
		}
	}
	overrides := make(map[string]map[string]float64, len(s.overrides))
	for id, params := range s.overrides {
		cp := make(map[string]float64, len(params))
		for k, v := range params {
			cp[k] = v
		}
		overrides[id] = cp
	}
	userRecipes := make(map[string][]byte, len(s.userRecipes))
	for id, raw := range s.userRecipes {
		cp := make([]byte, len(raw))
		copy(cp, raw)
		userRecipes[id] = cp
	}
	sampleEdits := make(map[string]map[string]float64, len(s.sampleEdits))
	for id, fields := range s.sampleEdits {
		cp := make(map[string]float64, len(fields))
		for k, v := range fields {
			cp[k] = v
		}
		sampleEdits[id] = cp
	}
	return prefsSnapshot{favs, overrides, userRecipes, s.audioPanel, sampleEdits}
}

// drainPending is the single worker job; it loops until no further write
// is pending. Each iteration snapshots the latest state, so a write that
// races with new Save* calls always reflects the freshest state on exit.
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
func snapshotFavs(favs map[string]bool) map[string]bool {
	out := make(map[string]bool, len(favs))
	for k, v := range favs {
		if v {
			out[k] = true
		}
	}
	return out
}

func cloneFavMap(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// writeAtomic serializes the full snapshot and replaces prefs.json via
// temp + rename so a crash mid-write leaves either the old or new state,
// never a truncated half.
func (s *fileStore) writeAtomic(snap prefsSnapshot) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := marshalPrefsV4(snap.favorites, snap.overrides, snap.userRecipes, snap.audioPanel, snap.sampleEdits)
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
