package ui

import (
	"image"
	"testing"
)

func TestSynthFocusGraphTokenPresent(t *testing.T) {
	for _, d := range []Density{DensityCompact, DensityComfortable, DensitySpacious} {
		restore := SetDensityForTest(d)
		got := Profile().DensityValues().SynthFocusGraphH
		restore()
		if got < 24 {
			t.Fatalf("density %v: SynthFocusGraphH=%d, want >=24 (legible focus band)", d, got)
		}
	}
}

func TestSplitSynthRightPane(t *testing.T) {
	r := image.Rect(0, 0, 300, 400)
	top, bot := splitSynthRightPane(r)
	if top.Empty() || bot.Empty() {
		t.Fatalf("both bands must be non-empty: top=%v bot=%v", top, bot)
	}
	if top.Min.Y != r.Min.Y || bot.Max.Y != r.Max.Y {
		t.Fatalf("bands must span the rect: top=%v bot=%v r=%v", top, bot, r)
	}
	if bot.Min.Y <= top.Max.Y {
		t.Fatalf("focus band must sit below the mirror with a gap: top=%v bot=%v", top, bot)
	}
	if bot.Dy() < top.Dy() {
		t.Fatalf("focus band (%d) should be >= mirror band (%d)", bot.Dy(), top.Dy())
	}
}

func TestSplitSynthRightPane_ShortGivesFocusAll(t *testing.T) {
	top, bot := splitSynthRightPane(image.Rect(0, 0, 300, 70))
	if !top.Empty() {
		t.Fatalf("too-short pane: mirror should collapse, got %v", top)
	}
	if bot.Empty() {
		t.Fatalf("too-short pane: focus should take the whole rect, got empty")
	}
}
