package ui

import (
	"image"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/hooks"
)

func TestTextCommitEmits(t *testing.T) {
	ch := make(chan hooks.TextPayload, 2)
	u := hooks.GlobalBus().Subscribe(hooks.EventTextCommitted, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.TextPayload); ok {
			ch <- p
		}
	})
	defer u()

	emitTextCommitted("project-name", "my song")

	select {
	case p := <-ch:
		if p.Field != "project-name" || p.Value != "my song" {
			t.Fatalf("got %+v", p)
		}
	case <-time.After(time.Second):
		t.Fatal("no text-commit event")
	}

	// Empty field must be a no-op: no event should arrive.
	emitTextCommitted("", "x")
	select {
	case p := <-ch:
		t.Fatalf("expected no event for empty LogField, got %+v", p)
	case <-time.After(100 * time.Millisecond):
		// correct: nothing arrived
	}
}

// The Enter path and the blur transition on the next frame must not both emit.
func TestTextCommitEnterExactlyOneEvent(t *testing.T) {
	ch := make(chan hooks.TextPayload, 4) // buffered to catch spurious extra events
	u := hooks.GlobalBus().Subscribe(hooks.EventTextCommitted, func(e hooks.Event) {
		if p, ok := e.Payload.(hooks.TextPayload); ok {
			ch <- p
		}
	})
	defer u()

	ti := NewTextInput(image.Rect(0, 0, 200, 24), TextInputStyle{})
	ti.LogField = "field-x"
	ti.SetText("hello")

	// Stub mouse to be unpressed so Update() click-handling is skipped.
	oldMouse := isMouseButtonPressed
	isMouseButtonPressed = func(ebiten.MouseButton) bool { return false }
	defer func() { isMouseButtonPressed = oldMouse }()

	// Frame 0: set focus and let prevFocused settle (focused=true → prevFocused=true).
	ti.FocusForTest(true)
	ti.Update()

	// Frame 1: press Enter → Update() sets focused=false and returns.
	restoreEnter := stubKeys(map[ebiten.Key]bool{ebiten.KeyEnter: true}, nil)
	ti.Update()
	restoreEnter()

	// Frame 2: Enter is no longer pressed; blur-transition branch fires once.
	ti.Update()

	// Allow the async bus delivery to complete.
	select {
	case p := <-ch:
		if p.Field != "field-x" || p.Value != "hello" {
			t.Fatalf("wrong payload: got field=%q value=%q", p.Field, p.Value)
		}
	case <-time.After(time.Second):
		t.Fatal("no EventTextCommitted received after Enter commit")
	}

	// Assert no second event arrives (double-emit regression).
	select {
	case p := <-ch:
		t.Fatalf("double-emit detected: got a second EventTextCommitted %+v", p)
	case <-time.After(150 * time.Millisecond):
		// correct: exactly one event
	}
}
