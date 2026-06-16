//go:build !js

package userprefs

import (
	"path/filepath"
	"testing"
)

// TestFileStoreKnobStepRoundTrip verifies that SaveKnobStep persists to disk
// and is reloaded by a fresh store at the same path (the V6 round-trip).
func TestFileStoreKnobStepRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	s := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s.Close() })

	ks, ok := s.(KnobStepStore)
	if !ok {
		t.Fatalf("backing store does not implement KnobStepStore")
	}

	if err := ks.SaveKnobStep("filter_cutoff", 100); err != nil {
		t.Fatalf("SaveKnobStep: %v", err)
	}
	if err := ks.SaveKnobStep("amp_attack", 0.01); err != nil {
		t.Fatalf("SaveKnobStep: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}

	// Fresh store at the same path — must see persisted values.
	s2 := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s2.Close() })
	ks2, ok := s2.(KnobStepStore)
	if !ok {
		t.Fatalf("backing store2 does not implement KnobStepStore")
	}

	got := ks2.LoadKnobSteps()
	if got["filter_cutoff"] != 100 {
		t.Errorf("filter_cutoff: got %v, want 100", got["filter_cutoff"])
	}
	if got["amp_attack"] != 0.01 {
		t.Errorf("amp_attack: got %v, want 0.01", got["amp_attack"])
	}
}

// TestFileStoreKnobStepOtherSectionsUnaffected verifies that saving a knob
// step doesn't disturb the favorites or other prefs sections.
func TestFileStoreKnobStepOtherSectionsUnaffected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	s := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s.Close() })

	// Seed favorites first.
	if err := s.SaveFavorites(map[string]bool{"kick": true, "snare": true}); err != nil {
		t.Fatalf("SaveFavorites: %v", err)
	}

	// Then save a knob step.
	ks := s.(KnobStepStore)
	if err := ks.SaveKnobStep("osc_freq", 50); err != nil {
		t.Fatalf("SaveKnobStep: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}

	// Fresh store — favorites must survive.
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
		t.Errorf("favorites lost after SaveKnobStep: %v", favs)
	}

	got := s2.(KnobStepStore).LoadKnobSteps()
	if got["osc_freq"] != 50 {
		t.Errorf("osc_freq: got %v, want 50", got["osc_freq"])
	}
}
