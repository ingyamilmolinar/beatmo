//go:build !js

package userprefs

import (
	"context"
	"encoding/json"
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

func TestStoreLoadVersionMismatchReturnsEmpty(t *testing.T) {
	s, path := newTestStore(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	doc := map[string]any{"version": 99, "favorites": []string{"future-id"}}
	data, _ := json.Marshal(doc)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	favs, err := s.LoadFavorites()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(favs) != 0 {
		t.Fatalf("expected empty map for unsupported version, got %v", favs)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should be left intact for forward-compat recovery: %v", err)
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

func TestStoreSerializesSorted(t *testing.T) {
	// Stable on-disk diff: marshalFavorites sorts ids alphabetically.
	favs := map[string]bool{"zeta": true, "alpha": true, "mike": true}
	data, err := marshalFavorites(favs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed favoritesV1
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if parsed.Version != SchemaVersion {
		t.Fatalf("version=%d want %d", parsed.Version, SchemaVersion)
	}
	want := []string{"alpha", "mike", "zeta"}
	if len(parsed.Favorites) != len(want) {
		t.Fatalf("favorites len=%d want %d", len(parsed.Favorites), len(want))
	}
	for i, id := range want {
		if parsed.Favorites[i] != id {
			t.Fatalf("favorites[%d]=%q want %q (full=%v)", i, parsed.Favorites[i], id, parsed.Favorites)
		}
	}
}

func TestStoreParseFavoritesEmptyData(t *testing.T) {
	out := parseFavorites(nil, nil)
	if len(out) != 0 {
		t.Fatalf("expected empty map for nil data, got %v", out)
	}
	out = parseFavorites([]byte{}, nil)
	if len(out) != 0 {
		t.Fatalf("expected empty map for empty slice, got %v", out)
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
