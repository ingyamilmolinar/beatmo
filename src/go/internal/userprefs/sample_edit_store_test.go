//go:build !js

package userprefs

import (
	"math"
	"path/filepath"
	"testing"
)

// Sample-edit store tests: the SampleEditStore surface persists the
// non-destructive Sampler-edit descriptors (tiny flat float maps) inside
// prefs.json, exactly like RecipeOverrides — NOT the PCM SampleStore.

func newTestSampleEditStore(t *testing.T) (Store, SampleEditStore, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	s := NewBackingStore(Options{
		Path:     path,
		PoolName: uniquePoolName(t),
		Logger:   t.Logf,
	})
	t.Cleanup(func() { _ = s.Close() })
	se, ok := s.(SampleEditStore)
	if !ok {
		t.Fatalf("backing store does not implement SampleEditStore")
	}
	return s, se, path
}

func TestPrefs_SampleEditRoundTrip(t *testing.T) {
	s, se, path := newTestSampleEditStore(t)

	fields := map[string]float64{"start_frac": 0.25, "end_frac": 0.75, "gain_db": -6, "reverse": 1}
	if err := se.SaveSampleEdit("kick", fields); err != nil {
		t.Fatalf("SaveSampleEdit: %v", err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatalf("WaitFlushed: %v", err)
	}

	// Fresh store at the same path — survives restart.
	s2 := NewBackingStore(Options{Path: path, PoolName: uniquePoolName(t), Logger: t.Logf})
	t.Cleanup(func() { _ = s2.Close() })
	se2 := s2.(SampleEditStore)
	got, err := se2.LoadSampleEdits()
	if err != nil {
		t.Fatalf("LoadSampleEdits: %v", err)
	}
	kick, ok := got["kick"]
	if !ok {
		t.Fatalf("persisted edits missing kick: %v", got)
	}
	for k, v := range fields {
		if kick[k] != v {
			t.Errorf("field %q = %v, want %v", k, kick[k], v)
		}
	}
}

func TestPrefs_SampleEditDeleteAndEmptySave(t *testing.T) {
	s, se, _ := newTestSampleEditStore(t)

	if err := se.SaveSampleEdit("kick", map[string]float64{"start_frac": 0.1}); err != nil {
		t.Fatalf("SaveSampleEdit: %v", err)
	}
	if err := se.DeleteSampleEdit("kick"); err != nil {
		t.Fatalf("DeleteSampleEdit: %v", err)
	}
	got, _ := se.LoadSampleEdits()
	if _, ok := got["kick"]; ok {
		t.Error("DeleteSampleEdit left the entry behind")
	}

	// Empty fields map deletes (parity with SaveRecipeOverride).
	_ = se.SaveSampleEdit("snare", map[string]float64{"gain_db": -3})
	_ = se.SaveSampleEdit("snare", nil)
	got, _ = se.LoadSampleEdits()
	if _, ok := got["snare"]; ok {
		t.Error("empty SaveSampleEdit must delete the entry")
	}
	_ = s.WaitFlushed()
}

func TestPrefs_SampleEditDropsNonFinite(t *testing.T) {
	_, se, _ := newTestSampleEditStore(t)
	if err := se.SaveSampleEdit("hat", map[string]float64{"gain_db": math.NaN(), "start_frac": math.Inf(1), "end_frac": 0.9}); err != nil {
		t.Fatalf("SaveSampleEdit: %v", err)
	}
	got, _ := se.LoadSampleEdits()
	hat := got["hat"]
	if len(hat) != 1 || hat["end_frac"] != 0.9 {
		t.Errorf("non-finite values must be dropped at the boundary, got %v", hat)
	}
}
