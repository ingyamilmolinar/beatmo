package ui

// chain_density.go — small helpers bridging the generated Chain density
// tokens (design_density.gen.go) to the float scale that
// DrawTextColorAtScale expects.
//
// The Chain text-scale tokens (ChainLabelScale / ChainReadoutScale /
// ChainPillScale / ChainBadgeScale) are authored in DESIGN.md as
// permille-of-Body integers (e.g. 850 → 0.85× the 14px body font) so the
// generator can keep emitting plain ints. chainScale converts one back to
// the multiplier. Reading these per-density is what makes Chain text grow
// on mobile (Spacious) instead of being frozen at the old hardcoded
// FontSizeCaption/FontSizeBody (~0.71×).

// chainScale converts a permille-of-Body density token to the float
// multiplier consumed by DrawTextColorAtScale.
func chainScale(permille int) float64 { return float64(permille) / 1000.0 }

// chainLabelScale / chainReadoutScale / chainPillScale / chainBadgeScale
// read the active-density token and return the draw-ready multiplier.
// Centralising the DensityValues() lookups keeps the per-text-role intent
// legible at every call site.
func chainLabelScale() float64   { return chainScale(Profile().DensityValues().ChainLabelScale) }
func chainReadoutScale() float64 { return chainScale(Profile().DensityValues().ChainReadoutScale) }
func chainPillScale() float64    { return chainScale(Profile().DensityValues().ChainPillScale) }
func chainBadgeScale() float64   { return chainScale(Profile().DensityValues().ChainBadgeScale) }
