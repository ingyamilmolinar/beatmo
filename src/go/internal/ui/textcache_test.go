package ui

import (
	"fmt"
	"testing"
)

// The sprite cache must stay bounded: a text input caches one sprite per DISTINCT
// string, so typing/searching generates thousands of transient strings over a
// session. Without a cap the cache grows without bound (a slow memory leak).
func TestTextSpriteCacheBounded(t *testing.T) {
	assertDefaultParityState(t)
	ClearTextCacheForTest()
	for i := 0; i < textCacheMaxEntries+2000; i++ {
		TextSprite(fmt.Sprintf("transient-%d", i))
	}
	if n := textCacheLen(); n > textCacheMaxEntries {
		t.Fatalf("text sprite cache grew to %d entries, cap is %d — unbounded growth", n, textCacheMaxEntries)
	}
}

func TestStyledTextSpriteCacheBounded(t *testing.T) {
	assertDefaultParityState(t)
	ClearTextCacheForTest()
	for i := 0; i < textCacheMaxEntries+2000; i++ {
		StyledTextSprite(fmt.Sprintf("styled-%d", i), RoleBody)
	}
	if n := styledCacheLen(); n > textCacheMaxEntries {
		t.Fatalf("styled sprite cache grew to %d entries, cap is %d — unbounded growth", n, textCacheMaxEntries)
	}
}

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
