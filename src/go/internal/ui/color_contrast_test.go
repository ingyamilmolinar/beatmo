package ui

import (
	"image/color"
	"math"
	"testing"
)

// wcagRelLum returns WCAG relative luminance of an opaque color.
func wcagRelLum(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v) / 65535.0
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func wcagContrastRatio(a, b color.Color) float64 {
	la, lb := wcagRelLum(a), wcagRelLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func TestColorContrast(t *testing.T) {
	// Text/icon on surfaces must clear 4.5:1.
	surfaces := []color.Color{genColorBackground, genColorSurface1, genColorSurface2, genColorSurface3}
	for _, s := range surfaces {
		if r := wcagContrastRatio(genColorOnSurface, s); r < 4.5 {
			t.Errorf("text-hi on surface %v contrast %.2f < 4.5", s, r)
		}
	}
	// Secondary text is body copy, so it clears 4.5:1 on every surface; the dim hint role is exempt.
	for _, s := range surfaces {
		if r := wcagContrastRatio(genColorOnSurfaceMuted, s); r < 4.5 {
			t.Errorf("text-mid on surface %v contrast %.2f < 4.5", s, r)
		}
	}
	// Dark label (#120A1C) on each neon fill must clear 3:1 (graphical/large).
	dark := genColorBackground
	neons := map[string]color.Color{
		"primary": genColorPrimary,
		"focus":   genColorHotPink300,
		"play":    genColorSuccess,
		"warning": genColorSunsetGold200,
		"error":   genColorError,
	}
	for name, n := range neons {
		if r := wcagContrastRatio(dark, n); r < 3.0 {
			t.Errorf("dark-on-%s fill contrast %.2f < 3.0", name, r)
		}
	}
	// A secondary control is a neon stroke and label on a surface, so each neon clears 3:1 on the lightest, surface-3.
	for name, n := range neons {
		for _, s := range surfaces {
			if r := wcagContrastRatio(n, s); r < 3.0 {
				t.Errorf("ghost-outline %s neon on surface %v contrast %.2f < 3.0", name, s, r)
			}
		}
	}
}
