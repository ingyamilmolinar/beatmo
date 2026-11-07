//go:build test || js

package audio

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument/roster"
)

// "builtin" means rendered in-binary with no asset lookup; scope-aware filters rely on it.
func TestCatalogScope_BuiltinsAreBuiltin(t *testing.T) {
	withDefaultAudio(t)
	ResetCatalogForTest(nil)
	if err := InitCatalogFromDir(t.TempDir()); err != nil {
		t.Fatalf("init catalog: %v", err)
	}
	cat := Catalog()
	if len(cat) == 0 {
		t.Fatalf("expected synth placeholders even with empty assets dir")
	}
	for _, m := range cat {
		if m.Source == SourceSynth && !roster.Has(m.ID) {
			t.Errorf("synth catalog entry %q is not a built-in", m.ID)
		}
	}
}

func TestCatalogScope_OnDiskShippedFolderIsShipped(t *testing.T) {
	withDefaultAudio(t)
	ResetCatalogForTest(nil)
	root := t.TempDir()
	catDir := filepath.Join(root, "kick")
	if err := os.MkdirAll(catDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := writeTinyWAV(filepath.Join(catDir, "boom.wav")); err != nil {
		t.Fatalf("write wav: %v", err)
	}
	if err := InitCatalogFromDir(root); err != nil {
		t.Fatalf("init catalog: %v", err)
	}
	found := false
	for _, m := range Catalog() {
		if m.Source != SourceWAV {
			continue
		}
		found = true
	}
	if !found {
		t.Fatalf("expected at least one on-disk wav entry")
	}
}

// A stale local assets/Saved/ must never leak into the catalog.
func TestCatalogScope_NoSavedCategory(t *testing.T) {
	withDefaultAudio(t)
	ResetCatalogForTest(nil)
	root := t.TempDir()
	savedDir := filepath.Join(root, "Saved")
	if err := os.MkdirAll(savedDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := writeTinyWAV(filepath.Join(savedDir, "leftover.wav")); err != nil {
		t.Fatalf("write wav: %v", err)
	}
	if err := InitCatalogFromDir(root); err != nil {
		t.Fatalf("init catalog: %v", err)
	}
	for _, m := range Catalog() {
		if m.Source != SourceWAV {
			continue
		}
		t.Errorf("Saved/-rooted wav leaked into catalog: %+v", m)
	}
	for _, c := range CatalogGroups() {
		if c == "Saved (WAV)" {
			t.Errorf("Saved (WAV) category should not appear; got: %v", CatalogGroups())
		}
	}
}
