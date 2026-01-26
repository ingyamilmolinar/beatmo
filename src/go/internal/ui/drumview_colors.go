package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
)

// pickColorFromWheel maps a screen coordinate to a color in the wheel rectangle.
// Hue is angle, saturation is radius; value is fixed at 1. Alpha is 255.
func (dv *DrumView) pickColorFromWheel(x, y int) color.Color {
	r := dv.colorWheelRect
	if r.Empty() {
		return color.RGBA{200, 200, 200, 255}
	}
	// Map point to [-1,1] range centered in rect
	cx := float64(r.Min.X + r.Dx()/2)
	cy := float64(r.Min.Y + r.Dy()/2)
	rx := float64(x) - cx
	ry := float64(y) - cy
	radius := float64(imin(r.Dx(), r.Dy())) / 2
	if radius <= 0 {
		return color.RGBA{200, 200, 200, 255}
	}
	// Normalize radius to [0,1]
	rnorm := math.Hypot(rx, ry) / radius
	if rnorm > 1 {
		rnorm = 1
	}
	// Hue from angle in [0,1)
	h := math.Atan2(ry, rx) // [-pi, pi]
	if h < 0 {
		h += 2 * math.Pi
	}
	h /= 2 * math.Pi
	// Two-zone mapping for broader gamut:
	// Inner half: saturated darks (s=1, v in [0..1])
	// Outer half: bright pastels to saturated (v=1, s in [0..1]) with white at the seam
	var s, v float64
	if rnorm < 0.5 {
		s = 1
		v = rnorm / 0.5 // 0..1
	} else {
		s = (rnorm - 0.5) / 0.5 // 0..1
		v = 1
	}
	return hsvToRGBA(h, s, v)
}

func hsvToRGBA(h, s, v float64) color.Color {
	if s <= 0 {
		c := uint8(clamp(int(v*255), 0, 255))
		return color.RGBA{c, c, c, 255}
	}
	h6 := h * 6
	i := int(math.Floor(h6))
	f := h6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - s*f)
	t := v * (1 - s*(1-f))
	var r, g, b float64
	switch i % 6 {
	case 0:
		r, g, b = v, t, p
	case 1:
		r, g, b = q, v, p
	case 2:
		r, g, b = p, v, t
	case 3:
		r, g, b = p, q, v
	case 4:
		r, g, b = t, p, v
	default:
		r, g, b = v, p, q
	}
	return color.RGBA{uint8(clamp(int(r*255), 0, 255)), uint8(clamp(int(g*255), 0, 255)), uint8(clamp(int(b*255), 0, 255)), 255}
}

// rebuildColorWheelImage regenerates the cached wheel image for the current rect.
func (dv *DrumView) rebuildColorWheelImage() {
	r := dv.colorWheelRect
	w, h := r.Dx(), r.Dy()
	if w <= 0 || h <= 0 {
		dv.colorWheelImg = nil
		dv.wheelCacheW, dv.wheelCacheH = 0, 0
		return
	}
	dv.logger.Debugf("[COLOR] rebuild wheel image %dx%d at=(%d,%d)", w, h, r.Min.X, r.Min.Y)
	// Build an RGBA buffer for speed, then upload to ebiten
	buf := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := dv.pickColorFromWheel(r.Min.X+x, r.Min.Y+y)
			rr, gg, bb, aa := color.RGBAModel.Convert(c).(color.RGBA).RGBA()
			buf.SetRGBA(x, y, color.RGBA{uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8), uint8(aa >> 8)})
		}
	}
	dv.colorWheelImg = ebiten.NewImageFromImage(buf)
	dv.wheelCacheW, dv.wheelCacheH = w, h
}

// colorKey returns a canonical string key for a color.
func (dv *DrumView) colorKey(c color.Color) string {
	r, g, b, a := c.RGBA()
	return fmt.Sprintf("%02X%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
}

// isColorUsed reports whether the color is used by any row except excludeIdx.
func (dv *DrumView) isColorUsed(c color.Color, excludeIdx int) bool {
	key := dv.colorKey(c)
	for i, r := range dv.Rows {
		if i == excludeIdx {
			continue
		}
		if dv.colorKey(r.Color) == key {
			return true
		}
	}
	return false
}

// generatedColor derives a pseudo-random but deterministic vivid color from a seed.
func (dv *DrumView) generatedColor(seed int) color.Color {
	h := uint32(seed) * 2654435761
	r := uint8((h >> 16) & 0xFF)
	g := uint8((h >> 8) & 0xFF)
	b := uint8(h & 0xFF)
	// Ensure minimum brightness.
	if int(r)+int(g)+int(b) < 200 {
		r = r/2 + 60
		g = g/2 + 60
		b = b/2 + 60
	}
	return color.RGBA{r, g, b, 255}
}

// ensureUniqueColor adjusts base to avoid conflicts with other rows.
func (dv *DrumView) ensureUniqueColor(base color.Color, idx int) color.Color {
	if !dv.isColorUsed(base, idx) {
		return base
	}
	// Try light/dark adjustments
	for d := 20; d <= 120; d += 20 {
		for _, s := range []int{+1, -1} {
			c := adjustColor(base, s*d)
			if !dv.isColorUsed(c, idx) {
				return c
			}
		}
	}
	// Try palette fallbacks
	for _, c := range instColors {
		if !dv.isColorUsed(c, idx) {
			return c
		}
	}
	for _, c := range customPalette {
		if !dv.isColorUsed(c, idx) {
			return c
		}
	}
	// Generate until unique
	for i := 0; i < 256; i++ {
		c := dv.generatedColor(len(dv.Rows) + 1 + i*7)
		if !dv.isColorUsed(c, idx) {
			return c
		}
	}
	// Fallback to white (unlikely)
	return color.RGBA{255, 255, 255, 255}
}

// parseHexRGB parses #RRGGBB or #RGB and returns the color if valid.
func (dv *DrumView) parseHexRGB(s string) (color.Color, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) == 6 {
		var r, g, b uint8
		if _, err := fmt.Sscanf(s, "%02X%02X%02X", &r, &g, &b); err == nil {
			return color.RGBA{r, g, b, 255}, true
		}
	}
	if len(s) == 3 {
		var r, g, b uint8
		if _, err := fmt.Sscanf(s, "%1X%1X%1X", &r, &g, &b); err == nil {
			// expand 4-bit to 8-bit by duplication (e.g., A -> AA)
			r = r * 17
			g = g * 17
			b = b * 17
			return color.RGBA{r, g, b, 255}, true
		}
	}
	return color.RGBA{255, 255, 255, 255}, false
}

// SetRowColor sets the color for a row ensuring uniqueness across rows.
func (dv *DrumView) SetRowColor(idx int, c color.Color) {
	if idx < 0 || idx >= len(dv.Rows) {
		return
	}
	dv.Rows[idx].Color = dv.ensureUniqueColor(c, idx)
	// Mark the row dirty so cache picks up new color.
	if idx >= 0 && idx < len(dv.rowDirty) {
		dv.rowDirty[idx] = true
		if idx < len(dv.rowFullDirty) {
			dv.rowFullDirty[idx] = true
		}
	}
}

// EnsureUniqueRowColors scans all rows and adjusts any duplicates to unique variants.
func (dv *DrumView) EnsureUniqueRowColors() {
	for i := range dv.Rows {
		dv.Rows[i].Color = dv.ensureUniqueColor(dv.Rows[i].Color, i)
	}
	dv.markAllRowsDirty()
}
