package ui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// theme_derive.go — turns a theme.Doc into the full token map.
//
// Every category has a default ANCHOR swatch equal to today's design. Each
// member token is stored implicitly as its HSL offset from that anchor;
// picking a new swatch re-applies the offsets, so the anchor pick is the
// exact identity and the default palette stays byte-identical to DESIGN.md.

// rgbToHSL converts to hue [0,360), saturation [0,1], lightness [0,1].
func rgbToHSL(c color.RGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (max + min) / 2
	d := max - min
	if d < 1e-9 {
		return 0, 0, l
	}
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h * 60, s, l
}

func hslToRGB(h, s, l float64) color.RGBA {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s = clamp01(s)
	l = clamp01(l)
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	to8 := func(v float64) uint8 { return uint8(math.Round(clamp01(v+m) * 255)) }
	return color.RGBA{to8(r), to8(g), to8(b), 255}
}

// clamp01 lives in param_card.go (shared helper).

// hueDistance is the shortest angular distance between two hues.
func hueDistance(a, b float64) float64 {
	return math.Abs(math.Mod(a-b+540, 360) - 180)
}

// achromaticSat is the saturation below which a token is treated as grey /
// white / black and left exactly as designed (a hue is meaningless there).
const achromaticSat = 0.02

// swatchRGBA looks a curated swatch up by name.
func swatchRGBA(name string) (color.RGBA, bool) {
	for _, s := range SuggestedInstrumentSwatches {
		if s.Name == name {
			return s.RGBA, true
		}
	}
	return color.RGBA{}, false
}

// swatchShadeStrength maps the shade column (-100..-500) of a swatch name to
// a tint-strength multiplier for KeepLightness categories. The anchor of the
// dark categories is a -500 swatch and the text/dividers anchor a -100 one;
// the ratio against the anchor's own strength keeps the identity either way.
func swatchShadeStrength(name string) float64 {
	i := strings.LastIndex(name, "-")
	if i < 0 {
		return 1
	}
	n, err := strconv.Atoi(name[i+1:])
	if err != nil {
		return 1
	}
	switch n {
	case 100:
		return 0.35
	case 200:
		return 0.55
	case 300:
		return 0.75
	case 400:
		return 0.9
	default:
		return 1
	}
}

// deriveCategory writes the category's member tokens into out, derived from
// pick. Every member keeps its HSL OFFSET from the category anchor; the
// anchor pick is therefore the identity (short-circuited exactly).
//
//   - chromatic, non-KeepLightness: h = h(pick)+Δh, s = s(pick)+Δs, l = l(pick)+Δl
//   - chromatic, KeepLightness:     h = h(pick)+Δh, l = l(default),
//     s = s(default) · (s(pick)·strength(pick)) / (s(anchor)·strength(anchor))
//   - achromatic (s(default) < achromaticSat): unchanged.
func deriveCategory(cat ThemeCategory, pick color.RGBA, pickName string, out map[string]color.RGBA) {
	anchor, ok := swatchRGBA(cat.Anchor)
	if !ok {
		return
	}
	if pick == anchor {
		for _, tok := range cat.Tokens {
			out[tok] = genColorDefaultByName[tok]
		}
		return
	}
	ah, as, al := rgbToHSL(anchor)
	ph, ps, pl := rgbToHSL(pick)
	for _, tok := range cat.Tokens {
		def := genColorDefaultByName[tok]
		dh, ds, dl := rgbToHSL(def)
		if ds < achromaticSat {
			out[tok] = def
			continue
		}
		h := ph + (dh - ah)
		if cat.KeepLightness {
			s := ds
			if denom := as * swatchShadeStrength(cat.Anchor); denom > 1e-6 {
				s = ds * (ps * swatchShadeStrength(pickName)) / denom
			}
			out[tok] = hslToRGB(h, s, dl)
			continue
		}
		out[tok] = hslToRGB(h, ps+(ds-as), pl+(dl-al))
	}
}

// parseHexRGB6 parses "#RRGGBB" (case-insensitive).
func parseHexRGB6(s string) (color.RGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, true
}

// resolveThemeTokens turns a document into the full token map: defaults,
// then category derivation, then explicit tokens. Entries that cannot be
// applied are skipped and reported in warnings (never fatal — a theme comes
// from a user file).
func resolveThemeTokens(doc theme.Doc) (map[string]color.RGBA, []string) {
	out := make(map[string]color.RGBA, len(genColorDefaultByName))
	for k, v := range genColorDefaultByName {
		out[k] = v
	}
	var warns []string
	// 1. Preset picks (a first-party theme) …
	if doc.Preset != "" {
		if p, ok := themePresetByID(doc.Preset); ok {
			for _, cat := range themeCategories {
				if name, ok := p.Categories[cat.ID]; ok {
					if pick, ok := swatchRGBA(name); ok {
						deriveCategory(cat, pick, name, out)
					}
				}
			}
		} else {
			warns = append(warns, fmt.Sprintf("theme: unknown preset %q ignored", doc.Preset))
		}
	}
	// 2. … then the document's own category picks …
	for _, cat := range themeCategories { // table order → deterministic
		name, ok := doc.Categories[cat.ID]
		if !ok {
			continue
		}
		pick, ok := swatchRGBA(name)
		if !ok {
			warns = append(warns, fmt.Sprintf("theme: category %q: unknown swatch %q ignored", cat.ID, name))
			continue
		}
		deriveCategory(cat, pick, name, out)
	}
	for id := range doc.Categories {
		if _, ok := themeCategoryByID(id); !ok {
			warns = append(warns, fmt.Sprintf("theme: unknown category %q ignored", id))
		}
	}
	for tok, hex := range doc.Tokens {
		if _, ok := genColorDefaultByName[tok]; !ok {
			warns = append(warns, fmt.Sprintf("theme: unknown token %q ignored", tok))
			continue
		}
		if themeFixedTokens[tok] || isRawPaletteToken(tok) {
			warns = append(warns, fmt.Sprintf("theme: token %q is fixed and cannot be themed", tok))
			continue
		}
		c, ok := parseHexRGB6(hex)
		if !ok {
			warns = append(warns, fmt.Sprintf("theme: token %q: bad hex %q ignored", tok, hex))
			continue
		}
		out[tok] = c
	}
	return out, warns
}
