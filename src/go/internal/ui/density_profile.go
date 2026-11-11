package ui

// density_profile.go — Phase 0 of the audio-panel design-system
// compliance pass.
//
// **The problem this solves.** Today's LayoutProfile conflates two
// orthogonal questions into a single binary switch (ScreenMobile vs
// ScreenDesktop):
//
//   1. *Layout intent* — where do things go? (one-column stack vs
//      side-by-side, hide tab pills, panel orientation)
//   2. *Control sizing* — how big are interactive elements? (button
//      heights, knob diameters, font scales, touch targets)
//
// These need to evolve independently. A desktop user might want denser
// chrome. A tablet might want desktop layout (side-by-side panels) but
// mobile-sized touch targets. The Density enum is the third orthogonal
// axis that makes those decisions composable.
//
// **Three axes.** After Phase 0 the UI is parameterised on:
//
//   - LayoutProfile.Class (ScreenSizeClass) — layout-arrangement only
//     (panel orientation, hidden controls, bottom-sheet vs floating)
//   - Profile().Density() (Density) — control sizing (heights, radii,
//     touch targets, knob diameters)
//   - RuntimeProf() (RuntimeProfile) — browser/desktop runtime divergence
//     (predictor window cap, GC tick, etc.)
//
// Default coupling: ScreenDesktop ⇒ DensityComfortable (pixel-perfect
// parity with today's desktopProfile()); ScreenMobile ⇒ DensitySpacious
// (today's mobileProfile() values). Tests and future Settings UI can
// override the coupling.

// Density is the control-sizing tier. Three steps: Compact for dense
// desktop power-user screens, Comfortable for today's desktop baseline
// (pixel-perfect parity), Spacious for finger-friendly touch surfaces.
// Names match the Material 3 / GitHub Primer convention so future
// readers can search for prior art.
type Density int

const (
	// DensityCompact is the smallest tier — desktop power users, data-
	// dense surfaces. ~85% of Comfortable. NOT touch-min compliant.
	DensityCompact Density = iota
	// DensityComfortable is today's desktop baseline. Pixel-perfect
	// parity with the pre-density profileOverrides.desktop block.
	DensityComfortable
	// DensitySpacious is today's mobile baseline. Touch-min compliant
	// (every interactive field ≥ 44 px in at least one dimension).
	DensitySpacious
)

// String returns the lowercase YAML name of the density. Used by the
// scene catalog metadata and the screenshot baseline filename.
func (d Density) String() string {
	switch d {
	case DensityCompact:
		return "compact"
	case DensityComfortable:
		return "comfortable"
	case DensitySpacious:
		return "spacious"
	default:
		return "unknown"
	}
}

// densityOverride is the SetDensityForTest hook: when non-nil it
// overrides the screen-class-default density. Reset to nil by
// SetDensityForTest's restore. Production code never sets it.
//
// Density itself is derived from screen-class at every Profile()
// rebuild via chooseDensity(); we deliberately don't persist a
// global "activeDensity" so a class flip can never leak a stale
// density into the next test's profile (Phase 0 of the audio-panel
// design-system pass had a latent bug here — see the test-leak
// note in SetDensityForTest's doc-comment).
var densityOverride *Density

// activeDensity returns the effective density for the current
// screen-class. Reads the override hook when set, otherwise the
// canonical default for the class.
//
// Public callers should prefer Profile().Density() — this is the
// underlying single-source-of-truth that desktopProfile() /
// mobileProfile() consult to populate density-tied fields.
func activeDensity() Density {
	if densityOverride != nil {
		return *densityOverride
	}
	if activeProfile != nil {
		return defaultDensityFor(activeProfile.Class)
	}
	return DensityComfortable
}

// defaultDensityFor returns the canonical density for a screen-size
// class. The coupling is deliberately defaultable — tests override
// via SetDensityForTest and the future Settings UI overrides via a
// userprefs path.
func defaultDensityFor(c ScreenSizeClass) Density {
	if c == ScreenMobile {
		return DensitySpacious
	}
	return DensityComfortable
}

// densityValuesFor returns the per-density field set for d. Generated
// in design_density.gen.go from the DESIGN.md `densities:` block.
// Unknown densities fall back to Comfortable so a bad enum value
// can't crash a render path.
func densityValuesFor(d Density) densityValues {
	switch d {
	case DensityCompact:
		return genCompactDensity
	case DensitySpacious:
		return genSpaciousDensity
	default:
		return genComfortableDensity
	}
}

// densityForClass picks the right density for the supplied screen-
// size class, honoring densityOverride when set. Used by
// desktopProfile() / mobileProfile() to populate density-tied fields
// without persisting state in a global var. The override-then-
// default split keeps SetDensityForTest local to the test scope —
// no class flip in a sibling test can leak a stale density.
func densityForClass(c ScreenSizeClass) Density {
	if densityOverride != nil {
		return *densityOverride
	}
	return defaultDensityFor(c)
}

// Density returns the effective density for the current profile's
// class. Honors SetDensityForTest's override when set, falls back to
// the class default otherwise.
func (p *LayoutProfile) Density() Density { return densityForClass(p.Class) }

// DensityValues returns the density-tied field set for the active
// density. Call sites that need a single field read it through this
// accessor, e.g. `Profile().DensityValues().EqHandleRadius`. The
// existing `Profile().EqHandleRadius` (struct field) keeps working
// during the migration period — the builders in layout_profile.go
// populate it from the same source.
func (p *LayoutProfile) DensityValues() densityValues {
	return densityValuesFor(densityForClass(p.Class))
}

// SetDensityForTest overrides activeDensity for the duration of a
// test. Returns a restore function that reverts to the prior value.
// Mirrors SetRuntimeProfileForTest so the test-override pattern stays
// uniform across the three axes.
//
// The next Profile() call rebuilds the active LayoutProfile from the
// new density values; callers that hold a stale Profile() pointer
// must re-fetch.
func SetDensityForTest(d Density) (restore func()) {
	prev := densityOverride
	dCopy := d
	densityOverride = &dCopy
	UpdateProfile()
	return func() {
		densityOverride = prev
		UpdateProfile()
	}
}
