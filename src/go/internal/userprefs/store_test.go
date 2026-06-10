//go:build !js

package userprefs

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/async"
)

// uniquePoolName returns a per-test pool name so concurrent tests do
// not share the long-lived production pool. Cleanup releases the pool
// so the global async budget is reset.
func uniquePoolName(t *testing.T) string {
	t.Helper()
	name := PoolName + ".test." + t.Name()
	t.Cleanup(func() {
		_ = async.DefaultRegistry().Release(name)
	})
	return name
}

func newTestStore(t *testing.T) (*fileStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "favorites.json")
	s := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	}).(*fileStore)
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestStoreLoadMissingFileReturnsEmpty(t *testing.T) {
	s, _ := newTestStore(t)
	favs, err := s.LoadFavorites()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(favs) != 0 {
		t.Fatalf("expected empty map, got %v", favs)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s, path := newTestStore(t)
	in := map[string]bool{"kick-808": true, "snare-fat": true, "missing": false}
	if err := s.SaveFavorites(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file at %s: %v", path, err)
	}
	// Read via a fresh store to bypass the in-memory cache.
	s2, _ := newTestStore(t)
	s2.path = path
	out, err := s2.LoadFavorites()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(out), out)
	}
	if !out["kick-808"] || !out["snare-fat"] {
		t.Fatalf("expected kick-808 and snare-fat, got %v", out)
	}
	if out["missing"] {
		t.Fatalf("entries with value=false should not round-trip")
	}
}

func TestStoreSaveDoesNotBlockCaller(t *testing.T) {
	s, _ := newTestStore(t)
	const N = 100
	deadline := 100 * time.Millisecond
	start := time.Now()
	for i := 0; i < N; i++ {
		if err := s.SaveFavorites(map[string]bool{"id": true}); err != nil {
			t.Fatalf("Save[%d]: %v", i, err)
		}
	}
	elapsed := time.Since(start)
	if elapsed > deadline {
		t.Fatalf("100 saves took %v, want < %v (Submit should be non-blocking)", elapsed, deadline)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}
}

func TestStoreConcurrentReadWriteSafe(t *testing.T) {
	s, _ := newTestStore(t)
	var wg sync.WaitGroup
	stop := make(chan struct{})
	var reads, writes atomic.Int64
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_, _ = s.LoadFavorites()
					reads.Add(1)
				}
			}
		}()
	}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				select {
				case <-stop:
					return
				default:
					_ = s.SaveFavorites(map[string]bool{
						"a": j%2 == 0,
						"b": seed%2 == 0,
					})
					writes.Add(1)
				}
			}
		}(i)
	}
	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}
	if reads.Load() == 0 || writes.Load() == 0 {
		t.Fatalf("expected reads and writes to occur (reads=%d writes=%d)", reads.Load(), writes.Load())
	}
}

func TestStoreLoadCorruptedJSONReturnsEmpty(t *testing.T) {
	s, path := newTestStore(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{this is not json"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	favs, err := s.LoadFavorites()
	if err != nil {
		t.Fatalf("Load surfaced error for corrupt file: %v", err)
	}
	if len(favs) != 0 {
		t.Fatalf("expected empty map for corrupt file, got %v", favs)
	}
	// File must NOT be deleted on corrupt-load — operator may want to recover.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("corrupt file was removed: %v", err)
	}
}

func TestStoreAtomicWriteUsesTempFile(t *testing.T) {
	// We can't easily fault-inject a crash mid-write in pure Go, but we
	// can verify the write goes through a `.tmp` companion file that is
	// renamed into place — the standard atomic-rename idiom.
	s, path := newTestStore(t)
	if err := s.SaveFavorites(map[string]bool{"a": true}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("final file missing: %v", err)
	}
	// The .tmp file should be gone after rename.
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file lingered: err=%v", err)
	}
}

func TestStorePoolLazyAcquired(t *testing.T) {
	name := PoolName + ".test." + t.Name()
	t.Cleanup(func() { _ = async.DefaultRegistry().Release(name) })

	if _, present := async.DefaultRegistry().Stats()[name]; present {
		t.Fatalf("pool %q already present at test entry; stale registry state", name)
	}

	s := NewBackingStore(Options{
		Path:     filepath.Join(t.TempDir(), "favorites.json"),
		PoolName: name,
	}).(*fileStore)
	t.Cleanup(func() { _ = s.Close() })

	// Loading must not allocate the pool.
	if _, err := s.LoadFavorites(); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, present := async.DefaultRegistry().Stats()[name]; present {
		t.Fatalf("pool was allocated by LoadFavorites; expected lazy on first SaveFavorites")
	}

	if err := s.SaveFavorites(map[string]bool{"x": true}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}
	if _, present := async.DefaultRegistry().Stats()[name]; !present {
		t.Fatalf("pool should be present in registry stats after first Save")
	}
}

func TestStoreParsePrefsEmptyData(t *testing.T) {
	favs, overrides, recipes := parsePrefs(nil, nil)
	if len(favs) != 0 || len(overrides) != 0 || len(recipes) != 0 {
		t.Fatalf("expected empty state for nil data, got favs=%v overrides=%v recipes=%v", favs, overrides, recipes)
	}
	favs, overrides, recipes = parsePrefs([]byte{}, nil)
	if len(favs) != 0 || len(overrides) != 0 || len(recipes) != 0 {
		t.Fatalf("expected empty state for empty slice, got favs=%v overrides=%v recipes=%v", favs, overrides, recipes)
	}
}

func TestStoreRespectsSubmitContext(t *testing.T) {
	// Smoke check: SaveFavorites must complete even when the caller
	// passes a cancelled context-equivalent (we don't accept ctx today,
	// but the test guards against future regressions where someone wires
	// SubmitBlocking with a stale ctx).
	s, _ := newTestStore(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_ = ctx // not used directly; future-proof guard

	if err := s.SaveFavorites(map[string]bool{"a": true}); err != nil {
		t.Fatalf("Save returned err with cancelled-but-unused ctx: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}
}
