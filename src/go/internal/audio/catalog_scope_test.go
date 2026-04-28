//go:build test || js

package audio

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCatalogScope_BuiltinsAreBuiltin pins the contract that synth ids carry
// Scope == "builtin" — the marker that signals "rendered in-binary, no asset
// lookup needed". The favorites view and any future Scope-aware filter rely
// on this.
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
		if m.Source != "synth" {
			continue
		}
		if m.Scope != "builtin" {
			t.Errorf("synth %q: Scope=%q, want %q", m.ID, m.Scope, "builtin")
		}
	}
}

// TestCatalogScope_OnDiskShippedFolderIsShipped pins that on-disk WAVs in any
// non-Saved folder are tagged Scope == "shipped". This is the bundled-asset
// scope; future "remote" packs will use a different value.
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
		if m.Source != "wav" {
			continue
		}
		found = true
		if m.Scope != "shipped" {
			t.Errorf("on-disk wav %q (%s): Scope=%q, want %q", m.ID, m.RelPath, m.Scope, "shipped")
		}
	}
	if !found {
		t.Fatalf("expected at least one on-disk wav entry")
	}
}

// TestCatalogScope_NoSavedCategory pins that the legacy assets/Saved/ folder
// is filtered out of the catalog scan even when a developer accidentally
// re-creates it. Once the directory is removed from the repo, the only way
// for entries to leak in is via a stale local copy — this test guards against
// that. It also implicitly proves wavCategoryStub no longer maps "saved" to
// "Saved (WAV)".
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
		if m.Source != "wav" {
			continue
		}
		t.Errorf("Saved/-rooted wav leaked into catalog: %+v", m)
	}
	for _, c := range CatalogCategories() {
		if c == "Saved (WAV)" {
			t.Errorf("Saved (WAV) category should not appear; got: %v", CatalogCategories())
		}
	}
}
