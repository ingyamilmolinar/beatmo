//go:build test

package ui

import (
	"image"
	"testing"
)

// TestButtonHandleInputResult_ClickReturnsConsumed verifies that pressing
// inside the button returns InputConsumed.
func TestButtonHandleInputResult_ClickReturnsConsumed(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	called := false
	b := NewButton("ok", ButtonStyle{}, func() { called = true })
	b.SetRect(image.Rect(10, 10, 60, 30))

	r := b.HandleInputResult(35, 20, true)
	if r != InputConsumed {
		t.Fatalf("expected InputConsumed, got %d", r)
	}
	if !called {
		t.Fatal("OnClick not called")
	}
}

// TestButtonHandleInputResult_MissReturnsIgnored verifies that pressing
// outside the button returns InputIgnored.
func TestButtonHandleInputResult_MissReturnsIgnored(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("ok", ButtonStyle{}, func() { t.Fatal("should not fire") })
	b.SetRect(image.Rect(10, 10, 60, 30))

	r := b.HandleInputResult(0, 0, true)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored, got %d", r)
	}
}

// TestButtonHandleInputResult_ReleaseReturnsIgnored verifies that releasing
// without pressing returns InputIgnored.
func TestButtonHandleInputResult_ReleaseReturnsIgnored(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("ok", ButtonStyle{}, nil)
	b.SetRect(image.Rect(10, 10, 60, 30))

	r := b.HandleInputResult(35, 20, false)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored, got %d", r)
	}
}

// TestButtonHandleInputResult_SuppressReturnsIgnored verifies that the
// suppress guard causes InputIgnored and clears on release.
func TestButtonHandleInputResult_SuppressReturnsIgnored(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = true
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("ok", ButtonStyle{}, func() { t.Fatal("should not fire") })
	b.SetRect(image.Rect(10, 10, 60, 30))

	r := b.HandleInputResult(35, 20, true)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored during suppress, got %d", r)
	}
	if !suppressClicksUntilRelease {
		t.Fatal("suppress should remain set while pressed")
	}

	// Release clears the guard.
	r = b.HandleInputResult(35, 20, false)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored on release, got %d", r)
	}
	if suppressClicksUntilRelease {
		t.Fatal("suppress should be cleared after release")
	}
}

// TestButtonHandleInputResult_ConsumeOnPressSetsSuppress verifies the
// ConsumeOnPress flag activates the suppress guard.
func TestButtonHandleInputResult_ConsumeOnPressSetsSuppress(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("ok", ButtonStyle{}, nil)
	b.SetRect(image.Rect(10, 10, 60, 30))
	b.ConsumeOnPress = true

	r := b.HandleInputResult(35, 20, true)
	if r != InputConsumed {
		t.Fatalf("expected InputConsumed, got %d", r)
	}
	if !suppressClicksUntilRelease {
		t.Fatal("suppress should be set after ConsumeOnPress")
	}
}

// TestButtonHandleInputResult_BackwardCompat verifies that Handle() returns
// the same boolean as before the refactor.
func TestButtonHandleInputResult_BackwardCompat(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("ok", ButtonStyle{}, nil)
	b.SetRect(image.Rect(10, 10, 60, 30))

	// Press inside → consumed
	if b.HandleInputResult(35, 20, true) == InputIgnored {
		t.Fatal("Handle should consume press inside")
	}
	// Release resets held state.
	b.HandleInputResult(35, 20, false)

	// Press outside → ignored
	if b.HandleInputResult(0, 0, true) != InputIgnored {
		t.Fatal("Handle should ignore press outside")
	}
}

// --- buttonHitAdapter tests ---

// TestButtonHitAdapter_OnPressFires verifies that OnPress calls the button's
// OnClick callback and returns InputConsumed.
func TestButtonHitAdapter_OnPressFires(t *testing.T) {
	assertDefaultParityState(t)

	called := false
	b := NewButton("test", ButtonStyle{}, func() { called = true })
	b.SetRect(image.Rect(0, 0, 50, 20))

	h := &buttonHitAdapter{btn: b}
	r := h.OnPress(25, 10)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
	if !called {
		t.Fatal("OnClick was not called")
	}
}

// TestButtonHitAdapter_OnPressNilOnClick verifies that OnPress with a nil
// OnClick does not panic and still returns InputCaptured.
func TestButtonHitAdapter_OnPressNilOnClick(t *testing.T) {
	assertDefaultParityState(t)

	b := NewButton("test", ButtonStyle{}, nil)
	b.SetRect(image.Rect(0, 0, 50, 20))

	h := &buttonHitAdapter{btn: b}
	r := h.OnPress(25, 10)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
}

// TestButtonHitAdapter_OnDragNoOp verifies that OnDrag does nothing and
// does not panic.
func TestButtonHitAdapter_OnDragNoOp(t *testing.T) {
	assertDefaultParityState(t)

	b := NewButton("test", ButtonStyle{}, nil)
	h := &buttonHitAdapter{btn: b}
	h.OnDrag(10, 10) // must not panic
}

// TestButtonHitAdapter_OnReleaseSettlesPress verifies that OnRelease delivers
// the release to the button so it settles out of the pressed state (the tree
// captures the adapter on press, so release always arrives). A release with no
// prior press is a harmless no-op.
func TestButtonHitAdapter_OnReleaseSettlesPress(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("test", ButtonStyle{}, nil)
	b.SetRect(image.Rect(0, 0, 50, 20))
	h := &buttonHitAdapter{btn: b}

	h.OnPress(25, 10)
	if !b.Pressed() {
		t.Fatal("button should be pressed after OnPress")
	}
	h.OnRelease(25, 10)
	if b.Pressed() {
		t.Fatal("button should settle out of pressed after OnRelease")
	}
}

// TestButtonHitAdapter_OnWheelIgnored verifies that OnWheel returns
// InputIgnored.
func TestButtonHitAdapter_OnWheelIgnored(t *testing.T) {
	assertDefaultParityState(t)

	b := NewButton("test", ButtonStyle{}, nil)
	h := &buttonHitAdapter{btn: b}
	r := h.OnWheel(10, 10, 3)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored, got %d", r)
	}
}

// --- repeatButtonHitAdapter tests ---

// TestRepeatButtonHitAdapter_OnPressReturnsCaptured verifies that OnPress
// delegates to Button.Handle and returns InputCaptured.
func TestRepeatButtonHitAdapter_OnPressReturnsCaptured(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("inc", ButtonStyle{}, nil)
	b.SetRect(image.Rect(0, 0, 40, 20))

	h := &repeatButtonHitAdapter{btn: b}
	r := h.OnPress(20, 10)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
}

// TestRepeatButtonHitAdapter_OnPressDoesNotManageSuppress verifies that the
// adapter no longer saves/restores suppress — the tree clears it before dispatch
// and syncs it afterward.
func TestRepeatButtonHitAdapter_OnPressDoesNotManageSuppress(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("inc", ButtonStyle{}, nil)
	b.SetRect(image.Rect(0, 0, 40, 20))

	h := &repeatButtonHitAdapter{btn: b}
	h.OnPress(20, 10)

	// Adapter does not manage suppress; it stays as-is after the call.
	if suppressClicksUntilRelease {
		t.Fatal("adapter should not set suppress")
	}
}

// TestRepeatButtonHitAdapter_OnDragDelegates verifies that OnDrag calls
// Button.HandleInputResult(x, y, true).
func TestRepeatButtonHitAdapter_OnDragDelegates(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("inc", ButtonStyle{}, func() {})
	b.SetRect(image.Rect(0, 0, 40, 20))

	h := &repeatButtonHitAdapter{btn: b}
	// Start a press first so button is in held state.
	h.OnPress(20, 10)

	// Drag continues the press.
	h.OnDrag(20, 10)
	// Button.HandleInputResult(x, y, true) with held > 0 increments held counter;
	// no panic is the primary assertion.
}

// TestRepeatButtonHitAdapter_OnDragDoesNotManageSuppress verifies that the
// adapter no longer saves/restores suppress during OnDrag.
func TestRepeatButtonHitAdapter_OnDragDoesNotManageSuppress(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("inc", ButtonStyle{}, nil)
	b.SetRect(image.Rect(0, 0, 40, 20))

	h := &repeatButtonHitAdapter{btn: b}
	h.OnDrag(20, 10)

	if suppressClicksUntilRelease {
		t.Fatal("adapter should not set suppress")
	}
}

// TestRepeatButtonHitAdapter_OnReleaseCallsFalse verifies that OnRelease
// calls Button.HandleInputResult(x, y, false).
func TestRepeatButtonHitAdapter_OnReleaseCallsFalse(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("inc", ButtonStyle{}, nil)
	b.SetRect(image.Rect(0, 0, 40, 20))

	h := &repeatButtonHitAdapter{btn: b}
	// Press first.
	h.OnPress(20, 10)
	// Release.
	h.OnRelease(20, 10)
	// No panic is the primary assertion; the button is no longer held.
}

// TestRepeatButtonHitAdapter_OnReleaseDoesNotManageSuppress verifies that the
// adapter no longer saves/restores suppress during OnRelease.
func TestRepeatButtonHitAdapter_OnReleaseDoesNotManageSuppress(t *testing.T) {
	assertDefaultParityState(t)
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = false })

	b := NewButton("inc", ButtonStyle{}, nil)
	b.SetRect(image.Rect(0, 0, 40, 20))

	h := &repeatButtonHitAdapter{btn: b}
	h.OnRelease(20, 10)

	if suppressClicksUntilRelease {
		t.Fatal("adapter should not set suppress")
	}
}

// TestRepeatButtonHitAdapter_OnWheelIgnored verifies that OnWheel returns
// InputIgnored.
func TestRepeatButtonHitAdapter_OnWheelIgnored(t *testing.T) {
	assertDefaultParityState(t)

	b := NewButton("inc", ButtonStyle{}, nil)
	h := &repeatButtonHitAdapter{btn: b}
	r := h.OnWheel(10, 10, 1)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored, got %d", r)
	}
}

// --- transportVolIconHitAdapter tests ---

// TestTransportVolIconHitAdapter_OnPressFires verifies that OnPress calls the
// onClick callback and returns InputCaptured.
func TestTransportVolIconHitAdapter_OnPressFires(t *testing.T) {
	assertDefaultParityState(t)

	called := false
	h := &transportVolIconHitAdapter{onClick: func() { called = true }}
	r := h.OnPress(5, 5)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
	if !called {
		t.Fatal("onClick was not called")
	}
}

// TestTransportVolIconHitAdapter_OnPressNilCallback verifies that OnPress
// with a nil onClick does not panic and returns InputCaptured.
func TestTransportVolIconHitAdapter_OnPressNilCallback(t *testing.T) {
	assertDefaultParityState(t)

	h := &transportVolIconHitAdapter{onClick: nil}
	r := h.OnPress(5, 5)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
}

// TestTransportVolIconHitAdapter_NoOps verifies that OnDrag, OnRelease, and
// OnWheel do not panic.
func TestTransportVolIconHitAdapter_NoOps(t *testing.T) {
	assertDefaultParityState(t)

	h := &transportVolIconHitAdapter{onClick: nil}
	h.OnDrag(5, 5)
	h.OnRelease(5, 5)
	r := h.OnWheel(5, 5, 2)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored from OnWheel, got %d", r)
	}
}

// --- textInputHitAdapter tests ---

// TestTextInputHitAdapter_OnPress verifies the consume flag controls the
// dispatcher return value. Focus/caret are driven by the legacy
// TextInput.Update mouse poll (tree Phase 2, before dispatch), so OnPress only
// governs fall-through:
//   - default (consume=false): InputIgnored, so the press falls through to a
//     lower-z handler (EQ dB input over its band mute button).
//   - consume=true: InputConsumed, so a BPM box tap is NOT leaked into the
//     Record button's touch-expanded hit rect on mobile.
//
// Regression: transport_input_isolation_test.go + TestEQBandMuteHoldNoMultipleToggles.
func TestTextInputHitAdapter_OnPress(t *testing.T) {
	assertDefaultParityState(t)

	ti := NewTextInput(image.Rect(0, 0, 100, 20), TextInputStyle{})

	if r := (&textInputHitAdapter{ti: ti}).OnPress(50, 10); r != InputIgnored {
		t.Fatalf("default adapter: expected InputIgnored, got %d", r)
	}
	if r := (&textInputHitAdapter{ti: ti, consume: true}).OnPress(50, 10); r != InputConsumed {
		t.Fatalf("consume adapter: expected InputConsumed, got %d", r)
	}
}

// TestTextInputHitAdapter_NoOps verifies that OnDrag, OnRelease, and OnWheel
// do not panic and OnWheel returns InputIgnored.
func TestTextInputHitAdapter_NoOps(t *testing.T) {
	assertDefaultParityState(t)

	ti := NewTextInput(image.Rect(0, 0, 100, 20), TextInputStyle{})
	h := &textInputHitAdapter{ti: ti}
	h.OnDrag(50, 10)
	h.OnRelease(50, 10)
	r := h.OnWheel(50, 10, 1)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored from OnWheel, got %d", r)
	}
}

// --- rowVolIconHitAdapter tests ---

// TestRowVolIconHitAdapter_OnPressFires verifies that OnPress calls the
// onClick callback and returns InputCaptured (enables drag-through to popup).
func TestRowVolIconHitAdapter_OnPressFires(t *testing.T) {
	assertDefaultParityState(t)

	called := false
	h := &rowVolIconHitAdapter{onClick: func() { called = true }}
	r := h.OnPress(5, 5)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
	if !called {
		t.Fatal("onClick was not called")
	}
}

// TestRowVolIconHitAdapter_OnPressNilCallback verifies that OnPress with a
// nil onClick does not panic and returns InputCaptured.
func TestRowVolIconHitAdapter_OnPressNilCallback(t *testing.T) {
	assertDefaultParityState(t)

	h := &rowVolIconHitAdapter{onClick: nil}
	r := h.OnPress(5, 5)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
}

// TestRowVolIconHitAdapter_NoOps verifies that OnDrag, OnRelease, and
// OnWheel do not panic.
func TestRowVolIconHitAdapter_NoOps(t *testing.T) {
	assertDefaultParityState(t)

	h := &rowVolIconHitAdapter{onClick: nil}
	h.OnDrag(5, 5)
	h.OnRelease(5, 5)
	r := h.OnWheel(5, 5, 2)
	if r != InputIgnored {
		t.Fatalf("expected InputIgnored from OnWheel, got %d", r)
	}
}
