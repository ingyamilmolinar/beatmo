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
// localStorage.getItem / setItem under the LocalStorageKey.
type localStorageStore struct {
	key  string
	logf func(format string, args ...any)

	mu     sync.RWMutex
	cache  map[string]bool
	loaded bool
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

func (s *localStorageStore) LoadFavorites() (map[string]bool, error) {
	s.mu.RLock()
	if s.loaded {
		out := make(map[string]bool, len(s.cache))
		for k, v := range s.cache {
			out[k] = v
		}
		s.mu.RUnlock()
		return out, nil
	}
	s.mu.RUnlock()

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded {
		out := make(map[string]bool, len(s.cache))
		for k, v := range s.cache {
			out[k] = v
		}
		return out, nil
	}
	ls := localStorageOrNil()
	if ls.IsUndefined() {
		s.cache = map[string]bool{}
		s.loaded = true
		return map[string]bool{}, nil
	}
	val := ls.Call("getItem", s.key)
	var data []byte
	if !val.IsNull() && !val.IsUndefined() {
		data = []byte(val.String())
	}
	s.cache = parseFavorites(data, s.logf)
	s.loaded = true
	out := make(map[string]bool, len(s.cache))
	for k, v := range s.cache {
		out[k] = v
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
	s.cache = snap
	s.loaded = true
	s.mu.Unlock()

	data, err := marshalFavorites(snap)
	if err != nil {
		return err
	}
	ls := localStorageOrNil()
	if ls.IsUndefined() {
		// In-memory only; no error. The session keeps working.
		if s.logf != nil {
			s.logf("[USERPREFS] favorites: localStorage unavailable, in-memory only")
		}
		return nil
	}
	defer func() {
		if r := recover(); r != nil && s.logf != nil {
			s.logf("[USERPREFS] favorites: setItem failed: %v", r)
		}
	}()
	ls.Call("setItem", s.key, string(data))
	return nil
}

// WaitFlushed is a no-op on WASM: localStorage writes are synchronous.
func (s *localStorageStore) WaitFlushed() error { return nil }

func (s *localStorageStore) Close() error { return nil }
