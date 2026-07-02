package ui

import (
	"fmt"
	"image/color"

	"github.com/ingyamilmolinar/beatmo/internal/hooks"
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

// instrumentPaletteColors returns the ordered curated palette that every
// circuit/row color must come from. It is the suggested instrument swatches
// (the 35 Vice City ramp members), extended (deduped) with the builtin
// instrument defaults and the
// fallback palette so the set is as wide as possible while staying entirely
// on-palette. The free HSV hue-wheel was removed; colors are now restricted
// to this known set per the "circuit colors are palette-only" requirement.
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

// ensureUniqueColor maps base onto a curated palette color while preferring
// uniqueness across rows. Every result is guaranteed to be a member of the
// instrument palette (no brighten/darken adjustments, no pseudo-random
// generation): circuit colors are palette-only.
//
//   - If base is already a free palette color (unused by another row), keep it.
//   - Otherwise return the first UNUSED palette color, scanning the curated
//     palette in order.
//   - If every palette color is already in use (≥ palette-size distinct rows),
//     reuse is allowed — fall back to base so the result still stays on-palette.
func (dv *DrumView) ensureUniqueColor(base color.Color, idx int) color.Color {
	if !dv.isColorUsed(base, idx) {
		return base
	}
	for _, c := range instrumentPaletteColors() {
		if !dv.isColorUsed(c, idx) {
			return c
		}
	}
	// Palette exhausted (more rows than distinct palette colors): reuse base
	// rather than drift off-palette.
	return base
}

// instrumentSeriesColors returns the canonical, ordered, finite instrument
// color series — THE single predictable list every demo-circuit, template, and
// new-row color is derived from by index. Sourced from DESIGN.md
// `instrumentSequence:` via the generated genInstrumentSequence (the same series
// internal/templates bakes as templates.InstrumentSequence). Falls back to the
// curated palette if the sequence block is somehow empty so colors are never
// off-palette.
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

// seriesColorAt returns the instrument color for a given row index, walking the
// canonical series and wrapping: row N → series[N % len]. Pure sequential by
// row index, independent of instrument identity — this is how the demo circuit
// and newly added rows pick their (and therefore their nodes' + edges') color.
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

// ResequenceRowColors re-derives every row's color from the canonical series by
// row index (row N → seriesColorAt(N)), overwriting any prior colors. Used to
// normalise a freshly imported demo circuit (whose baked JSON colors may be
// stale/off-palette) onto the predictable sequential series.
func (dv *DrumView) ResequenceRowColors() {
	for i := range dv.Rows {
		dv.Rows[i].Color = seriesColorAt(i)
	}
	dv.markAllRowsDirty()
	// The row-rack controls cache (volume icon, accent stripe, label tint) is a
	// separate cache from the grid step cells — markAllRowsDirty doesn't touch it.
	dv.markRowControlsDirty()
}

// rowColorAt returns the live color of row idx, or nil when the index is out of
// range or the row has no color. Used to seed menus (e.g. the color picker's
// CurrentColor selected-ring) from the same value the grid node is drawn with.
func (dv *DrumView) rowColorAt(idx int) color.Color {
	if idx < 0 || idx >= len(dv.Rows) || dv.Rows[idx] == nil {
		return nil
	}
	return dv.Rows[idx].Color
}

// instrumentRowColor returns the live display color of the row currently bound
// to instrument id — the same color the grid node is drawn with — or nil when
// no row uses that instrument. The instrument menu threads this into each
// InstrumentOption.Color so the picker swatch / active stripe / accent match the
// node exactly (instead of the registered instColor default). Re-read on every
// menu (re)build, so a node recolor propagates the next time the menu opens.
func (dv *DrumView) instrumentRowColor(id string) color.Color {
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
	// The volume icon, accent stripe, and label tint are cached in the row-rack
	// controls cache (distinct from the grid step cells above), so invalidate it
	// too or the in-row volume control keeps painting the old color.
	dv.markRowControlsDirty()
	emitRowColorChanged(idx, packRGBA(dv.Rows[idx].Color))
	// An open instrument menu derives its per-instrument swatch/accent from the
	// live row colors (instrumentRowColor → InstrumentOption.Color); refresh it
	// so a recolor is reflected immediately even while the menu is open. No-op
	// when the menu is closed.
	dv.refreshInstMenuComponent()
	// Color picks are a single commit per gesture (the picker latches picked
	// after the first pick), so record one undo step here beside the emit.
	dv.recordUndoStep(hooks.EventRowColorChanged)
}

// SetRowColorManual applies an explicit user-picked color WITHOUT the
// uniqueness substitution that SetRowColor performs. A deliberate pick from
// the color picker wins even if it duplicates another row's color; auto-assigned
// new rows still go through SetRowColor/ensureUniqueColor for distinctness.
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
	// Invalidate the row-rack controls cache too (volume icon / accent stripe /
	// label tint) — markAllRowsDirty/rowDirty only cover the grid step cells.
	dv.markRowControlsDirty()
	emitRowColorChanged(idx, packRGBA(dv.Rows[idx].Color))
	// Keep an open instrument menu's derived colors in sync with the live row
	// color (see SetRowColor). No-op when the menu is closed.
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
