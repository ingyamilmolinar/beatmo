//go:build test

package ui

import (
	"image"
	"reflect"
	"strings"
	"testing"
)

// passthroughHandler is a mockHandler that opts out of whole-gesture ownership.
type passthroughHandler struct{ mockHandler }

func (p *passthroughHandler) PassesGesturesThrough() bool { return true }

// TestInputDispatcher_PressEdgeDecidesGestureOwner pins the ownership latch:
// the handler that consumes the press EDGE owns the gesture until the next
// edge, wherever the pointer goes in between (held frames, the release frame,
// frames routed around the dispatcher).
func TestInputDispatcher_PressEdgeDecidesGestureOwner(t *testing.T) {
	d := NewInputDispatcher()
	btn := &mockHandler{bounds: image.Rect(0, 0, 10, 10), zIndex: 300, result: InputConsumed}
	d.Register(btn)
	d.Sort()

	if d.GestureOwnedByHandler() {
		t.Fatal("no press yet: nothing may own a gesture")
	}
	d.Dispatch(5, 5, true) // press edge on the button
	if d.PressOwner() != btn || !d.GestureOwnedByHandler() {
		t.Fatalf("press edge on a consuming handler must make it the owner (owner=%v)", d.PressOwner())
	}
	d.Dispatch(50, 50, true)  // held, drifted off the button
	d.Dispatch(50, 50, false) // released off the button
	if !d.GestureOwnedByHandler() {
		t.Fatal("the owner must survive held/release frames off its bounds — the press edge decides once")
	}

	d.Dispatch(50, 50, true) // a NEW press on empty canvas
	if d.PressOwner() != nil || d.GestureOwnedByHandler() {
		t.Fatal("a new press edge that no handler consumes must clear the previous owner")
	}
}

// TestInputDispatcher_SkipFrameClearsStaleOwner: a press cycle that starts on a
// frame routed around the dispatcher has no dispatcher owner — an earlier
// gesture's owner must not swallow it.
func TestInputDispatcher_SkipFrameClearsStaleOwner(t *testing.T) {
	d := NewInputDispatcher()
	btn := &mockHandler{bounds: image.Rect(0, 0, 10, 10), zIndex: 300, result: InputConsumed}
	d.Register(btn)
	d.Sort()
	d.Dispatch(5, 5, true)
	d.Dispatch(5, 5, false)
	d.SkipFrame(true) // next press edge never reached the dispatcher
	if d.GestureOwnedByHandler() {
		t.Fatal("SkipFrame on a press edge must clear the stale owner")
	}
}

// TestInputDispatcher_PassthroughHandlerDoesNotOwnGesture: swallowing is the
// default; a handler opts into passthrough explicitly.
func TestInputDispatcher_PassthroughHandlerDoesNotOwnGesture(t *testing.T) {
	d := NewInputDispatcher()
	sb := &passthroughHandler{mockHandler{bounds: image.Rect(0, 0, 10, 10), zIndex: 200, result: InputCaptured, captured: true}}
	d.Register(sb)
	d.Sort()
	d.Dispatch(5, 5, true)
	if d.PressOwner() != InputHandler(sb) {
		t.Fatalf("the capturing handler must still be the press owner (owner=%v)", d.PressOwner())
	}
	if d.GestureOwnedByHandler() {
		t.Fatal("a GesturePassthrough handler must not swallow the release-time gesture")
	}
}

// TestEveryGridCornerButtonIsInTheTable: every top-right grid button field on
// Game is a row of gridCornerButtons — the one table layout, draw, input,
// toggle reconcile and menu exclusion iterate. A button added beside the table
// is exactly how the theme button lost its toggle reconcile.
func TestEveryGridCornerButtonIsInTheTable(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	inTable := map[*Button]bool{}
	for _, c := range g.gridCornerButtons() {
		if c.btn == nil || c.rect == nil || c.menuID == "" {
			t.Fatalf("incomplete gridCornerButtons row: %+v", c)
		}
		inTable[c.btn] = true
	}
	gv := reflect.ValueOf(g).Elem()
	gt := gv.Type()
	btnType := reflect.TypeOf((*Button)(nil))
	found := 0
	for i := 0; i < gt.NumField(); i++ {
		f := gt.Field(i)
		if f.Type != btnType || !strings.HasPrefix(f.Name, "grid") || !strings.HasSuffix(f.Name, "Btn") {
			continue
		}
		found++
		b := (*Button)(gv.Field(i).UnsafePointer())
		if b != nil && !inTable[b] {
			t.Errorf("Game.%s is a grid button outside gridCornerButtons — add it to the table", f.Name)
		}
	}
	if found < 4 {
		t.Fatalf("expected the four grid corner button fields, found %d (did the naming change?)", found)
	}
}
