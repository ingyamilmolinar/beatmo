package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestSeekShortcutNodeRegistered(t *testing.T) {
	g := newGameWithStart(t)
	r := newKeyboardShortcutRouter(g)
	keys := r.keysOf("seek")
	if len(keys) == 0 {
		t.Fatal("seek shortcut node not registered")
	}
	want := map[ebiten.Key]bool{ebiten.KeyPeriod: true, ebiten.KeyComma: true, ebiten.KeyHome: true}
	for _, k := range keys {
		if !want[k] {
			t.Fatalf("unexpected seek key %v", k)
		}
	}
}

func TestSeekKeysDoNotCollide(t *testing.T) {
	g := newGameWithStart(t)
	r := newKeyboardShortcutRouter(g)
	seen := map[ebiten.Key]string{}
	for _, name := range r.nodeNames() {
		for _, k := range r.keysOf(name) {
			if other, dup := seen[k]; dup && other != name {
				t.Fatalf("key %v claimed by both %q and %q", k, other, name)
			}
			seen[k] = name
		}
	}
}
