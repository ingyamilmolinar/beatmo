//go:build !js

package userprefs

import (
	"path/filepath"
	"testing"
)

// TestLanguageRoundTrip verifies that SaveLanguage persists to disk and is
// reloaded by a fresh store at the same path (the V7 round-trip). Mirrors
// TestFileStoreKnobStepRoundTrip.
func TestLanguageRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	s := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s.Close() })

	ls, ok := s.(LanguageStore)
	if !ok {
		t.Fatalf("backing store does not implement LanguageStore")
	}

	if err := ls.SaveLanguage("es"); err != nil {
		t.Fatalf("SaveLanguage: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}

	// Fresh store at the same path — must see the persisted language.
	s2 := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s2.Close() })
	ls2, ok := s2.(LanguageStore)
	if !ok {
		t.Fatalf("backing store2 does not implement LanguageStore")
	}

	got, err := ls2.LoadLanguage()
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if got != "es" {
		t.Errorf("language: got %q, want %q", got, "es")
	}
}

// TestLanguageDefaultsEmptyWhenAbsent verifies a fresh store with no saved
// language returns "" (default → English upstream).
func TestLanguageDefaultsEmptyWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	s := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s.Close() })

	ls, ok := s.(LanguageStore)
	if !ok {
		t.Fatalf("backing store does not implement LanguageStore")
	}

	got, err := ls.LoadLanguage()
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if got != "" {
		t.Errorf("language: got %q, want empty", got)
	}
}

// TestLanguageOtherSectionsUnaffected verifies that saving a language doesn't
// disturb favorites or other prefs sections (V7 extends V6 without loss).
func TestLanguageOtherSectionsUnaffected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	s := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s.Close() })

	if err := s.SaveFavorites(map[string]bool{"kick": true, "snare": true}); err != nil {
		t.Fatalf("SaveFavorites: %v", err)
	}
	if err := s.(LanguageStore).SaveLanguage("fr"); err != nil {
		t.Fatalf("SaveLanguage: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}

	s2 := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s2.Close() })

	favs, err := s2.LoadFavorites()
	if err != nil {
		t.Fatalf("LoadFavorites: %v", err)
	}
	if !favs["kick"] || !favs["snare"] {
		t.Errorf("favorites lost after SaveLanguage: %v", favs)
	}
	got, err := s2.(LanguageStore).LoadLanguage()
	if err != nil {
		t.Fatalf("LoadLanguage: %v", err)
	}
	if got != "fr" {
		t.Errorf("language: got %q, want %q", got, "fr")
	}
}
