//go:build !test

package ui

import "testing"

// Real build only: under -tags test the debug font always gives a space width. Width must come from glyph
// ADVANCE, not ink bounds — a space has no ink, so a leading space would be invisible and the caret would not move.

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
