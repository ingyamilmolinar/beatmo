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
// The pool is lazy-acquired on the first SaveFavorites call. This keeps
// the 1-worker budget free until the user actually toggles a star, so a
// session that only loads favorites at startup pays no goroutine cost.
type fileStore struct {
	path     string
	poolName string
	logf     func(format string, args ...any)

	mu     sync.Mutex
	cache  map[string]bool // last-known state, mirrors disk + most-recent Save
	loaded bool

	pending  map[string]bool // newer state to be written; nil = caught up
	inflight bool            // true between worker submit and worker exit
	flushed  sync.WaitGroup  // tracks any work the worker still owes

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
	path := opts.Path
	if path == "" {
		dir, err := os.UserConfigDir()
		if err != nil {
			// Fall back to ~/.config/beatmo/favorites.json. A failure
			// here is rare (no $HOME set) — degrade to a temp path so
			// the rest of the API still works for the session.
			home, _ := os.UserHomeDir()
			if home == "" {
				home = os.TempDir()
			}
			dir = filepath.Join(home, ".config")
		}
		path = filepath.Join(dir, "beatmo", "favorites.json")
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

func (s *fileStore) LoadFavorites() (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		return cloneFavMap(s.cache), nil
	}
	data, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		// Read errors other than missing-file are surfaced — caller
		// may want to know the disk is unreadable. Cache stays empty.
		s.cache = map[string]bool{}
		s.loaded = true
		return map[string]bool{}, err
	}
	s.cache = parseFavorites(data, s.logf)
	s.loaded = true
	return cloneFavMap(s.cache), nil
}

// SaveFavorites schedules a write to disk. The call returns once the
// new state is captured; the actual write happens on the userprefs.persist
// pool. Rapid successive saves coalesce: the next worker iteration
// always sees the latest snapshot, so 100 toggles produce at most one
// in-flight write + one queued write at any moment.
func (s *fileStore) SaveFavorites(favs map[string]bool) error {
	snap := snapshotFavs(favs)

	pool, err := s.ensurePool()
	if err != nil {
		// Registry budget exhausted — degrade to synchronous write so
		// the star is at least durable for this session.
		s.mu.Lock()
		s.cache = snap
		s.loaded = true
		s.mu.Unlock()
		return s.writeAtomic(snap)
	}

	s.mu.Lock()
	s.cache = snap
	s.loaded = true
	s.pending = snap
	if s.inflight {
		// A worker is already running; it will pick up s.pending when
		// it finishes the current write. No new job to submit.
		s.mu.Unlock()
		return nil
	}
	s.inflight = true
	s.flushed.Add(1)
	s.mu.Unlock()

	if submitErr := pool.Submit(s.drainPending); submitErr != nil {
		// Couldn't even queue a single job — pool is closed or otherwise
		// broken. Roll back inflight bookkeeping and fall back to sync.
		s.mu.Lock()
		s.pending = nil
		s.inflight = false
		s.mu.Unlock()
		s.flushed.Done()
		if s.logf != nil {
			s.logf("[USERPREFS] favorites pool unavailable (%v); writing synchronously", submitErr)
		}
		return s.writeAtomic(snap)
	}
	return nil
}

// drainPending is the single worker job; it loops until s.pending is
// nil. Each iteration writes the latest snapshot, so a write that races
// with new Save calls always reflects the freshest state on exit.
func (s *fileStore) drainPending(_ context.Context) {
	defer s.flushed.Done()
	for {
		s.mu.Lock()
		next := s.pending
		s.pending = nil
		if next == nil {
			s.inflight = false
			s.mu.Unlock()
			return
		}
		s.mu.Unlock()
		if err := s.writeAtomic(next); err != nil && s.logf != nil {
			s.logf("[USERPREFS] favorites write failed: %v", err)
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

// writeAtomic serializes favs and replaces the target file via temp +
// rename so a crash mid-write leaves either the old or new state, never
// a truncated half.
func (s *fileStore) writeAtomic(favs map[string]bool) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := marshalFavorites(favs)
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
