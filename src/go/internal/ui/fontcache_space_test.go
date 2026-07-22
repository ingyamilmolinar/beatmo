//go:build !test

package ui

import "testing"

// These tests exercise the REAL TrueType measurement path in fontcache.go
// (compiled only WITHOUT the `test` tag — under `-tags test` the debug font is
// used and space always has width, so the fast suite can't catch this). Run via
// `make test-real` / `xvfb-run go test ./internal/ui/`.
//
// Regression: measurement used text.BoundString (INK bounds), so a space — which
// has no ink — measured 0 wide. A LEADING space was then invisible: the caret
// (drawn at x+TextWidth(prefix)) never advanced and the text sprite clipped the
// space, so typing a space at the start of a name looked like "nothing happened"
// on desktop AND WASM. The fix measures/renders by glyph ADVANCE.

func TestSpaceHasWidth(t *testing.T) {
	if fontFaceBold(FontSizeBody) == nil {
		t.Skip("no TrueType face available")
	}
	if w := TextWidth(" "); w <= 0 {
		t.Fatalf("TextWidth(%q) = %d, want > 0 (a space must advance the caret)", " ", w)
	}
}

func TestLeadingSpaceWidensText(t *testing.T) {
	if fontFaceBold(FontSizeBody) == nil {
		t.Skip("no TrueType face available")
	}
	base := TextWidth("token")
	lead := TextWidth(" token")
	if lead <= base {
		t.Fatalf("TextWidth(%q)=%d must exceed TextWidth(%q)=%d — a leading space adds width", " token", lead, "token", base)
	}
}

func TestLeadingSpaceSpritePreserved(t *testing.T) {
	if fontFaceBold(FontSizeBody) == nil {
		t.Skip("no TrueType face available")
	}
	base := renderTextSprite("token", FontSizeBody, true)
	lead := renderTextSprite(" token", FontSizeBody, true)
	if base == nil || lead == nil {
		t.Skip("sprite renderer returned nil")
	}
	bw := base.Bounds().Dx()
	lw := lead.Bounds().Dx()
	if lw <= bw {
		t.Fatalf("sprite width %q=%d must exceed %q=%d — the leading space must not be clipped", " token", lw, "token", bw)
	}
}
