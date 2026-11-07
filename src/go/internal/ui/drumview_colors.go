package ui

import (
	"fmt"
	"image/color"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

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

// Circuit colors are palette-only: every row color comes from this set.
func instrumentPaletteColors() []color.Color {
	out := make([]color.Color, 0, len(SuggestedInstrumentSwatches)+len(instColors)+len(customPalette))
	seen := map[string]struct{}{}
	add := func(c color.Color) {
		r, g, b, a := c.RGBA()
		key := fmt.Sprintf("%02X%02X%02X%02X", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		out = append(out, c)
	}
	for _, s := range SuggestedInstrumentSwatches {
		add(s.RGBA)
	}
	for _, c := range instColors {
		add(c)
	}
	for _, c := range customPalette {
		add(c)
	}
	return out
}

// Always returns a palette color, preferring one no other row uses.
func (dv *DrumView) ensureUniqueColor(base color.Color, idx int) color.Color {
	if !dv.isColorUsed(base, idx) {
		return base
	}
	for _, c := range instrumentPaletteColors() {
		if !dv.isColorUsed(c, idx) {
			return c
		}
	}
	// Reuse rather than drift off-palette.
	return base
}

func instrumentSeriesColors() []color.Color {
	if len(genInstrumentSequence) > 0 {
		out := make([]color.Color, len(genInstrumentSequence))
		for i, c := range genInstrumentSequence {
			out[i] = c
		}
		return out
	}
	return instrumentPaletteColors()
}

// By row index, independent of the instrument, so a circuit's colors are predictable.
func seriesColorAt(idx int) color.Color {
	series := instrumentSeriesColors()
	if len(series) == 0 {
		return genColorRowRackColorFallback
	}
	if idx < 0 {
		idx = 0
	}
	return series[idx%len(series)]
}

func (dv *DrumView) rowColorAt(idx int) color.Color {
	if idx < 0 || idx >= len(dv.Rows) || dv.Rows[idx] == nil {
		return nil
	}
	return dv.Rows[idx].Color
}

// The live row color, so the menu swatch matches the node exactly; nil when no row uses id.
func (dv *DrumView) instrumentRowColor(id instrument.ID) color.Color {
	if id == "" {
		return nil
	}
	for i := range dv.Rows {
		if dv.Rows[i] != nil && dv.Rows[i].Instrument == id && dv.Rows[i].Color != nil {
			return dv.Rows[i].Color
		}
	}
	return nil
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
	// The row-rack controls cache bakes this color too.
	dv.markRowControlsDirty()
	emitRowColorChanged(idx, packRGBA(dv.Rows[idx].Color))
	// An open instrument menu derives its swatches from the row colors.
	dv.refreshInstMenuComponent()
	// One commit per gesture: the picker latches after the first pick.
	dv.recordUndoStep(hooks.EventRowColorChanged)
}

// A deliberate pick wins even if another row has the color; auto-assigned rows stay distinct.
func (dv *DrumView) SetRowColorManual(idx int, c color.Color) {
	if idx < 0 || idx >= len(dv.Rows) {
		return
	}
	dv.Rows[idx].Color = c
	if idx < len(dv.rowDirty) {
		dv.rowDirty[idx] = true
		if idx < len(dv.rowFullDirty) {
			dv.rowFullDirty[idx] = true
		}
	}
	dv.markRowControlsDirty()
	emitRowColorChanged(idx, packRGBA(dv.Rows[idx].Color))
	dv.refreshInstMenuComponent()
	dv.recordUndoStep(hooks.EventRowColorChanged)
}

// EnsureUniqueRowColors scans all rows and adjusts any duplicates to unique variants.
func (dv *DrumView) EnsureUniqueRowColors() {
	for i := range dv.Rows {
		dv.Rows[i].Color = dv.ensureUniqueColor(dv.Rows[i].Color, i)
	}
	dv.markAllRowsDirty()
	dv.markRowControlsDirty()
}
