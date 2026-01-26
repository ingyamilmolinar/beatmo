//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestClipScope_NilParent(t *testing.T) {
	scope := NewClipScope(nil, image.Rect(0, 0, 100, 100))
	if scope != nil {
		t.Error("expected nil scope for nil parent")
	}
}

func TestClipScope_EmptyClip(t *testing.T) {
	parent := ebiten.NewImage(640, 480)

	scope := NewClipScope(parent, image.Rectangle{})
	if scope != nil {
		t.Error("expected nil scope for empty clip")
	}
}

func TestClipScope_ClipOutsideBounds(t *testing.T) {
	parent := ebiten.NewImage(640, 480)

	// Clip completely outside parent bounds
	scope := NewClipScope(parent, image.Rect(700, 500, 800, 600))
	if scope != nil {
		t.Error("expected nil scope for clip outside parent bounds")
	}
}

func TestClipScope_ValidClip(t *testing.T) {
	parent := ebiten.NewImage(640, 480)

	clip := image.Rect(100, 100, 300, 300)
	scope := NewClipScope(parent, clip)

	if scope == nil {
		t.Fatal("expected non-nil scope for valid clip")
	}

	if scope.Target() == nil {
		t.Error("expected non-nil target")
	}

	if scope.Offset() != clip.Min {
		t.Errorf("expected offset %v, got %v", clip.Min, scope.Offset())
	}

	if scope.Bounds() != clip {
		t.Errorf("expected bounds %v, got %v", clip, scope.Bounds())
	}
}

func TestClipScope_ClipIntersectsParent(t *testing.T) {
	parent := ebiten.NewImage(640, 480)

	// Clip partially outside parent bounds
	scope := NewClipScope(parent, image.Rect(600, 400, 800, 600))

	if scope == nil {
		t.Fatal("expected non-nil scope for intersecting clip")
	}

	// Bounds should be clamped to parent
	expected := image.Rect(600, 400, 640, 480)
	if scope.Bounds() != expected {
		t.Errorf("expected clamped bounds %v, got %v", expected, scope.Bounds())
	}
}

func TestClipScope_NilReceiver(t *testing.T) {
	var scope *ClipScope

	// These should not panic
	if scope.Target() != nil {
		t.Error("expected nil target from nil scope")
	}

	if scope.Offset() != (image.Point{}) {
		t.Error("expected zero offset from nil scope")
	}

	if !scope.Bounds().Empty() {
		t.Error("expected empty bounds from nil scope")
	}

	// DrawAt should not panic
	scope.DrawAt(nil, 0, 0, nil)
}
