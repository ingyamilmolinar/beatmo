package ui

// SuggestedInstrumentSwatches is the curated suggestion palette consumed by
// the per-row color picker (drumview_color_menu.go) to render a strip of
// suggested swatches above the free hue wheel.
//
// Source of truth: the `instrumentSwatches:` block in DESIGN.md, mirrored
// by `genInstrumentSwatches` in design_tokens.gen.go. This file simply
// re-exports the generated slice under a public name so call sites read
// from a stable identifier; the runtime never mutates the slice.
//
// Per the "Instrument identity is the only multi-color signal" invariant
// in DESIGN.md, this set is suggestion-only — users may still pick
// arbitrary hex via the wheel. The chrome runtime ignores this palette
// entirely.
var SuggestedInstrumentSwatches = genInstrumentSwatches
