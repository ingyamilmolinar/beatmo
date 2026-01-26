package ui

import "testing"

func TestTextSpriteCaching(t *testing.T) {
	assertDefaultParityState(t)
	ClearTextCacheForTest()
	a := TextSprite("Play")
	b := TextSprite("Play")
	c := TextSprite("Stop")
	if a == nil || b == nil || c == nil {
		t.Fatalf("expected non-nil sprites")
	}
	if a != b {
		t.Fatalf("expected sprite reuse for identical text")
	}
	if a == c {
		t.Fatalf("expected different sprites for different text")
	}
}
