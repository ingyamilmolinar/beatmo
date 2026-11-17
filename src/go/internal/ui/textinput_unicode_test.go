package ui

import (
	"image"
	"testing"
)

func TestAcceptTextRunePermitsUnicodeRejectsControl(t *testing.T) {
	for _, r := range []rune{'é', 'ñ', '日', 'A', '1', ' ', '-'} {
		if !AcceptTextRune(r) {
			t.Fatalf("AcceptTextRune(%q) = false, want true (printable)", r)
		}
	}
	for _, r := range []rune{'\x00', '\b', '\n', '\t', rune(127)} {
		if AcceptTextRune(r) {
			t.Fatalf("AcceptTextRune(%q) = true, want false (control)", r)
		}
	}
}

// Locks in the rune-index → byte-offset invariant that makes insert/backspace
// rune-correct for multi-byte text.
func TestByteIndexRuneCorrect(t *testing.T) {
	// "áé" is 2 runes, 4 bytes (each is 2 bytes in UTF-8).
	if got := byteIndex("áé", 0); got != 0 {
		t.Fatalf("byteIndex 0 = %d want 0", got)
	}
	if got := byteIndex("áé", 1); got != 2 {
		t.Fatalf("byteIndex 1 = %d want 2", got)
	}
	if got := byteIndex("áé", 2); got != 4 {
		t.Fatalf("byteIndex 2 = %d want 4", got)
	}
}

// newRenameComponentForTest constructs a real RenameComponent the same way
// production does (SetProps + Open), so the wired Accept func is exercised.
func newRenameComponentForTest(t *testing.T) *RenameComponent {
	t.Helper()
	rc := NewRenameComponent()
	rc.SetProps(RenameProps{
		AnchorRect:  image.Rect(100, 50, 250, 75),
		InitialText: "kick",
		MaxLen:      32,
	})
	rc.Open()
	return rc
}

// The rename text box must accept Unicode so international instrument names work.
func TestRenameBoxAcceptsUnicode(t *testing.T) {
	rc := newRenameComponentForTest(t) // see Step 3; use the real constructor
	tb := rc.TextBox()
	if tb == nil {
		t.Fatal("rename component has no text box")
	}
	if tb.Accept == nil {
		t.Fatal("rename box has no Accept func — Unicode names would be rejected by the ASCII default")
	}
	if !tb.Accept('é') || !tb.Accept('日') {
		t.Fatal("rename box Accept rejects Unicode")
	}
}
