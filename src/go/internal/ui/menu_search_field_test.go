//go:build test

package ui

import (
	"image"
	"testing"
)

func TestEnsureMenuSearchBox_CreatesStyledBoxAndSyncsRect(t *testing.T) {
	searchRect := image.Rect(10, 20, 210, 64)
	box := ensureMenuSearchBox(nil, searchRect)
	if box == nil {
		t.Fatal("ensureMenuSearchBox returned nil")
	}
	if box.MaxLen != 40 {
		t.Fatalf("MaxLen = %d, want 40 (parity with instrument search box)", box.MaxLen)
	}
	if got, want := box.Rect, insetRect(searchRect, SpaceSM); got != want {
		t.Fatalf("box.Rect = %v, want insetRect(searchRect, SpaceSM) = %v", got, want)
	}
	// Second call reuses the same box and re-syncs the rect.
	newRect := image.Rect(0, 0, 100, 44)
	box2 := ensureMenuSearchBox(box, newRect)
	if box2 != box {
		t.Fatal("ensureMenuSearchBox allocated a new box instead of reusing the existing one")
	}
	if got, want := box2.Rect, insetRect(newRect, SpaceSM); got != want {
		t.Fatalf("box.Rect not re-synced: got %v want %v", got, want)
	}
}
