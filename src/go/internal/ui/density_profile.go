package ui

// Density is control SIZING, orthogonal to layout (ScreenSizeClass) and runtime (RuntimeProfile): a tablet can
// keep desktop layout with touch-sized targets. Desktop defaults to Comfortable, mobile to Spacious.

// Names follow the Material 3 / Primer convention.
type Density int

const (
	// ~85% of Comfortable. NOT touch-min compliant.
	DensityCompact Density = iota
	DensityComfortable
	// Touch-min compliant: every interactive field ≥ 44 px in at least one dimension.
	DensitySpacious
)

// Also names the screenshot baseline file.
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

// Production never sets it. Density is re-derived from the class on every Profile() rebuild, never stored,
// so a class flip cannot leak a stale density into the next test.
var densityOverride *Density

// Callers use Profile().Density(); the profile builders read this.
func activeDensity() Density {
	if densityOverride != nil {
		return *densityOverride
	}
	if activeProfile != nil {
		return defaultDensityFor(activeProfile.Class)
	}
	return DensityComfortable
}

// Defaultable on purpose: tests override with SetDensityForTest.
func defaultDensityFor(c ScreenSizeClass) Density {
	if c == ScreenMobile {
		return DensitySpacious
	}
	return DensityComfortable
}

// Unknown densities fall back to Comfortable so a bad value cannot crash a render path.
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

// Override first, then the class default, so SetDensityForTest stays scoped to its test.
func densityForClass(c ScreenSizeClass) Density {
	if densityOverride != nil {
		return *densityOverride
	}
	return defaultDensityFor(c)
}

func (p *LayoutProfile) Density() Density { return densityForClass(p.Class) }

func (p *LayoutProfile) DensityValues() densityValues {
	return densityValuesFor(densityForClass(p.Class))
}

// The next Profile() rebuilds from the new density; a held Profile() pointer must be re-fetched.
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
