//go:build test

package ui

import (
	"image"
	"testing"
)

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

// Ignored while suppressed; the guard clears on release.
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

// No panic, still InputCaptured.
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

func TestButtonHitAdapter_OnDragNoOp(t *testing.T) {
	assertDefaultParityState(t)

	b := NewButton("test", ButtonStyle{}, nil)
	h := &buttonHitAdapter{btn: b}
	h.OnDrag(10, 10) // must not panic
}

// The tree captures on press, so the release always arrives and settles the button; one with no press is a no-op.
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

// The tree clears suppress before dispatch and syncs it after.
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
	// No panic is the assertion.
}

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

// No panic, still InputCaptured.
func TestTransportVolIconHitAdapter_OnPressNilCallback(t *testing.T) {
	assertDefaultParityState(t)

	h := &transportVolIconHitAdapter{onClick: nil}
	r := h.OnPress(5, 5)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
}

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

// TextInput.Update owns focus and caret, so OnPress only decides fall-through: by default the press falls through
// (the EQ dB input over its mute), and consume=true stops it (a BPM tap must not reach Record's expanded rect).
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

// InputCaptured, so a drag continues into the popup.
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

// No panic, still InputCaptured.
func TestRowVolIconHitAdapter_OnPressNilCallback(t *testing.T) {
	assertDefaultParityState(t)

	h := &rowVolIconHitAdapter{onClick: nil}
	r := h.OnPress(5, 5)
	if r != InputCaptured {
		t.Fatalf("expected InputCaptured, got %d", r)
	}
}

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
