//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// TestThemeDefaultIsIdentity: an empty doc AND a doc that picks every
// category's own anchor both reproduce DESIGN.md byte-for-byte.
func TestThemeDefaultIsIdentity(t *testing.T) {
	got, warns := resolveThemeTokens(theme.Doc{})
	if len(warns) != 0 {
		t.Fatalf("empty doc produced warnings: %v", warns)
	}
	for name, want := range genColorDefaultByName {
		if got[name] != want {
			t.Errorf("empty doc: %s = %v, want default %v", name, got[name], want)
		}
	}
	anchors := theme.Doc{Categories: map[string]string{}}
	for _, c := range themeCategories {
		anchors.Categories[c.ID] = c.Anchor
	}
	got, warns = resolveThemeTokens(anchors)
	if len(warns) != 0 {
		t.Fatalf("anchor doc produced warnings: %v", warns)
	}
	for name, want := range genColorDefaultByName {
		if got[name] != want {
			t.Errorf("anchor doc: %s = %v, want default %v (derivation is not the identity on the anchor)", name, got[name], want)
		}
	}
}

func lightness(c color.RGBA) float64 { _, _, l := rgbToHSL(c); return l }

// TestThemeDarkCategoriesPreserveLadder: with a LIGHT pick, every
// KeepLightness category token keeps its default lightness, so the surface
// ladder ordering survives and the UI stays dark.
func TestThemeDarkCategoriesPreserveLadder(t *testing.T) {
	doc := theme.Doc{Categories: map[string]string{"background": "mint-100", "surfaces": "mint-100", "grid": "cyan-100", "nodes": "tangerine-100", "dividers": "electric-blue-100", "text": "sunset-gold-500"}}
	got, warns := resolveThemeTokens(doc)
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	for _, c := range themeCategories {
		if !c.KeepLightness {
			continue
		}
		for _, tok := range c.Tokens {
			if dl, gl := lightness(genColorDefaultByName[tok]), lightness(got[tok]); dl < gl-0.02 || dl > gl+0.02 {
				t.Errorf("%s/%s lightness %.3f drifted from default %.3f", c.ID, tok, gl, dl)
			}
		}
	}
	if !(lightness(got["background"]) < lightness(got["surface-1"]) && lightness(got["surface-1"]) < lightness(got["surface-2"]) && lightness(got["surface-2"]) < lightness(got["surface-3"])) {
		t.Fatalf("surface ladder broken: bg=%v s1=%v s2=%v s3=%v", got["background"], got["surface-1"], got["surface-2"], got["surface-3"])
	}
	h0, _, _ := rgbToHSL(genColorDefaultByName["surface-2"])
	h1, _, _ := rgbToHSL(got["surface-2"])
	if hueDistance(h0, h1) < 30 {
		t.Fatalf("surface-2 hue did not follow the mint pick: %.0f → %.0f", h0, h1)
	}
}

// TestThemeAccentFollowsPick: a non-dark category adopts the pick's HSL and
// keeps the ladder offsets (bright lighter than primary, dim darker).
func TestThemeAccentFollowsPick(t *testing.T) {
	got, _ := resolveThemeTokens(theme.Doc{Categories: map[string]string{"accent": "orchid-300"}})
	pick, _ := swatchRGBA("orchid-300")
	if got["primary"] != pick {
		t.Fatalf("primary = %v, want the picked swatch %v", got["primary"], pick)
	}
	// The ladder OFFSETS are preserved: bright stays lighter than primary
	// (as in the default), and dim keeps its default hue/lightness offset
	// (in the shipped design primary-dim is a warmer, slightly LIGHTER
	// orange, not a darker gold — the offset, not a fixed direction, is
	// what carries over).
	if lightness(got["primary-bright"]) <= lightness(got["primary"]) {
		t.Fatalf("bright must stay lighter than primary: bright=%v primary=%v", got["primary-bright"], got["primary"])
	}
	dh0, _, dl0 := rgbToHSL(genColorDefaultByName["primary-dim"])
	ph0, _, pl0 := rgbToHSL(genColorDefaultByName["primary"])
	dh1, _, dl1 := rgbToHSL(got["primary-dim"])
	ph1, _, pl1 := rgbToHSL(got["primary"])
	if d := (dl1 - pl1) - (dl0 - pl0); d < -0.02 || d > 0.02 {
		t.Fatalf("dim lightness offset drifted: default %.3f, themed %.3f", dl0-pl0, dl1-pl1)
	}
	if d := hueDistance(dh1-ph1, dh0-ph0); d > 2 {
		t.Fatalf("dim hue offset drifted: default %.1f, themed %.1f", dh0-ph0, dh1-ph1)
	}
	if got["background"] != genColorDefaultByName["background"] {
		t.Fatal("background changed although only accent was picked")
	}
}

func TestThemeAchromaticStaysUnchanged(t *testing.T) {
	got, _ := resolveThemeTokens(theme.Doc{Categories: map[string]string{"dividers": "mint-300"}})
	if got["border"] != genColorDefaultByName["border"] || got["divider-shadow"] != genColorDefaultByName["divider-shadow"] {
		t.Fatalf("achromatic tokens must not be recolored: border=%v shadow=%v", got["border"], got["divider-shadow"])
	}
	if got["divider-highlight"] == genColorDefaultByName["divider-highlight"] {
		t.Fatal("chromatic divider-highlight should follow the pick")
	}
}

func TestThemeTokensOverrideAfterCategories(t *testing.T) {
	got, warns := resolveThemeTokens(theme.Doc{
		Categories: map[string]string{"accent": "orchid-300"},
		Tokens:     map[string]string{"primary": "#112233"},
	})
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	if got["primary"] != (color.RGBA{0x11, 0x22, 0x33, 255}) {
		t.Fatalf("explicit token must win over category derivation: %v", got["primary"])
	}
}

func TestThemeRejectsFixedAndUnknownNames(t *testing.T) {
	_, warns := resolveThemeTokens(theme.Doc{
		Categories: map[string]string{"accent": "no-such-swatch", "nope": "mint-100"},
		Tokens:     map[string]string{"error": "#000000", "not-a-token": "#000000", "primary": "zzz"},
	})
	if len(warns) != 5 {
		t.Fatalf("want 5 warnings (bad swatch, bad category, fixed token, unknown token, bad hex), got %d: %v", len(warns), warns)
	}
}

func TestHSLRoundTrip(t *testing.T) {
	for _, c := range []color.RGBA{{255, 179, 10, 255}, {18, 10, 28, 255}, {0, 0, 0, 255}, {255, 255, 255, 255}, {110, 18, 224, 255}} {
		h, s, l := rgbToHSL(c)
		if got := hslToRGB(h, s, l); got != c {
			t.Errorf("round trip %v → (%.1f,%.3f,%.3f) → %v", c, h, s, l, got)
		}
	}
}
