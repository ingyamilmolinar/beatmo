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

// TestTextCommitEnterExactlyOneEvent verifies that pressing Enter in a TextInput
// with LogField set produces EXACTLY ONE EventTextCommitted — not two. This is the
// regression test for the double-emit bug where the Enter-key path emitted once and
// then the blur-transition branch emitted a second time on the following frame.
//
// The test drives the full Update() cycle across three frames:
//  - frame 0: settle focus (focused=true, prevFocused catches up)
//  - frame 1: Enter pressed → focused set to false
//  - frame 2: blur transition fires → exactly one EventTextCommitted
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
