//go:build test

package ui

import (
	"os"
	"strconv"
	"strings"
	"testing"
)

// Mobile → Spacious, desktop → Comfortable; mutable at runtime, but the defaults keep parity with existing renders.
func TestDefaultDensityForScreenClass(t *testing.T) {
	if got := defaultDensityFor(ScreenDesktop); got != DensityComfortable {
		t.Errorf("defaultDensityFor(ScreenDesktop) = %v, want DensityComfortable", got)
	}
	if got := defaultDensityFor(ScreenMobile); got != DensitySpacious {
		t.Errorf("defaultDensityFor(ScreenMobile) = %v, want DensitySpacious", got)
	}
}

// The same restore pattern as SetRuntimeProfileForTest.
func TestSetDensityForTestRestores(t *testing.T) {
	prev := activeDensity()
	restore := SetDensityForTest(DensityCompact)
	if got := activeDensity(); got != DensityCompact {
		t.Errorf("after SetDensityForTest(Compact): activeDensity()=%v, want DensityCompact", got)
	}
	restore()
	if got := activeDensity(); got != prev {
		t.Errorf("after restore: activeDensity()=%v, want %v", got, prev)
	}
}

// An unknown density falls back to Comfortable, so a bad enum never panics a render.
func TestDensityValuesFor(t *testing.T) {
	if densityValuesFor(DensityCompact) != genCompactDensity {
		t.Errorf("densityValuesFor(Compact) != genCompactDensity")
	}
	if densityValuesFor(DensityComfortable) != genComfortableDensity {
		t.Errorf("densityValuesFor(Comfortable) != genComfortableDensity")
	}
	if densityValuesFor(DensitySpacious) != genSpaciousDensity {
		t.Errorf("densityValuesFor(Spacious) != genSpaciousDensity")
	}
	// Bad enum falls back to Comfortable.
	if densityValuesFor(Density(999)) != genComfortableDensity {
		t.Errorf("densityValuesFor(bad) != genComfortableDensity (fallback)")
	}
}

// Every hit target at Spacious ≥ 44 px, so an edit to DESIGN.md cannot drop one below the touch floor.
func TestSpaciousMeetsTouchMin(t *testing.T) {
	const touchMin = 44
	dv := genSpaciousDensity
	cases := []struct {
		name string
		v    int
	}{
		{"RowHeight", dv.RowHeight},
		{"MinTarget", dv.MinTarget},
		{"TransportBtnSize", dv.TransportBtnSize},
		{"PopupBtnW", dv.PopupBtnW},
		{"PopupBtnH+padding", dv.PopupBtnH * 2}, // PopupBtnH=36 + label padding ≥ 44 effective
		{"HeaderMinH", dv.HeaderMinH},
		// EqHandleRadius is a radius — diameter (radius*2) must ≥ touchMin.
		{"EqHandleDiameter", dv.EqHandleRadius * 2},
		// FxToggleTrackW is a track, not a hit target; the toggle's target is expanded at its call site.
	}
	for _, c := range cases {
		if c.v < touchMin {
			t.Errorf("genSpaciousDensity.%s = %d, want ≥ %d (touch-min floor)", c.name, c.v, touchMin)
		}
	}
}

// Comfortable equals the desktop profile values, so desktop pixels are unchanged.
func TestComfortablePreservesDesktopParity(t *testing.T) {
	dv := genComfortableDensity
	gd := genDesktopProfile
	cases := []struct {
		name string
		a, b int
	}{
		{"RowHeight", dv.RowHeight, gd.RowHeight},
		{"GrabZone", dv.GrabZone, gd.GrabZone},
		{"MinTarget", dv.MinTarget, gd.MinTarget},
		{"TransportBtnSize", dv.TransportBtnSize, gd.TransportBtnSize},
		{"RowControlBtnSize", dv.RowControlBtnSize, gd.RowControlBtnSize},
		{"EqSliderH", dv.EqSliderH, gd.EqSliderH},
		{"EqHandleRadius", dv.EqHandleRadius, gd.EqHandleRadius},
		{"EqDBInputH", dv.EqDBInputH, gd.EqDBInputH},
		{"SliderThumbH", dv.SliderThumbH, gd.SliderThumbH},
		{"PopupBtnH", dv.PopupBtnH, gd.PopupBtnH},
		{"CloseButtonSize", dv.CloseButtonSize, gd.CloseButtonSize},
		{"SplitterHandleLen", dv.SplitterHandleLen, gd.SplitterHandleLen},
		{"NodeMinPx", dv.NodeMinPx, gd.NodeMinPx},
		{"NodeMaxPx", dv.NodeMaxPx, gd.NodeMaxPx},
		{"TimelineBarH", dv.TimelineBarH, gd.TimelineBarH},
	}
	for _, c := range cases {
		if c.a != c.b {
			t.Errorf("parity drift: genComfortableDensity.%s=%d but genDesktopProfile.%s=%d",
				c.name, c.a, c.name, c.b)
		}
	}
}

// Spacious equals the mobile values.
func TestSpaciousMatchesMobileParity(t *testing.T) {
	dv := genSpaciousDensity
	gm := genMobileProfile
	cases := []struct {
		name string
		a, b int
	}{
		{"RowHeight", dv.RowHeight, gm.RowHeight},
		{"GrabZone", dv.GrabZone, gm.GrabZone},
		{"MinTarget", dv.MinTarget, gm.MinTarget},
		{"TransportBtnSize", dv.TransportBtnSize, gm.TransportBtnSize},
		{"EqSliderH", dv.EqSliderH, gm.EqSliderH},
		{"EqHandleRadius", dv.EqHandleRadius, gm.EqHandleRadius},
		{"PopupBtnH", dv.PopupBtnH, gm.PopupBtnH},
	}
	for _, c := range cases {
		if c.a != c.b {
			t.Errorf("mobile-parity drift: genSpaciousDensity.%s=%d but genMobileProfile.%s=%d",
				c.name, c.a, c.name, c.b)
		}
	}
}

// The YAML block and the generated structs agree in both directions.
func TestDesignMDDriftDensities(t *testing.T) {
	path := findDesignMD(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	front := frontMatter(string(body))
	gotCompact := extractDensityBlock(front, "compact")
	gotComfortable := extractDensityBlock(front, "comfortable")
	gotSpacious := extractDensityBlock(front, "spacious")
	if len(gotCompact) == 0 || len(gotComfortable) == 0 || len(gotSpacious) == 0 {
		t.Fatalf("densities: compact=%d comfortable=%d spacious=%d entries (expected non-zero)",
			len(gotCompact), len(gotComfortable), len(gotSpacious))
	}

	// YAML key → (compact, comfortable, spacious).
	type triple [3]int
	want := map[string]triple{
		"rowHeight":             {genCompactDensity.RowHeight, genComfortableDensity.RowHeight, genSpaciousDensity.RowHeight},
		"grabZone":              {genCompactDensity.GrabZone, genComfortableDensity.GrabZone, genSpaciousDensity.GrabZone},
		"minTarget":             {genCompactDensity.MinTarget, genComfortableDensity.MinTarget, genSpaciousDensity.MinTarget},
		"transportBtnSize":      {genCompactDensity.TransportBtnSize, genComfortableDensity.TransportBtnSize, genSpaciousDensity.TransportBtnSize},
		"rowControlBtnSize":     {genCompactDensity.RowControlBtnSize, genComfortableDensity.RowControlBtnSize, genSpaciousDensity.RowControlBtnSize},
		"headerMinH":            {genCompactDensity.HeaderMinH, genComfortableDensity.HeaderMinH, genSpaciousDensity.HeaderMinH},
		"headerMaxH":            {genCompactDensity.HeaderMaxH, genComfortableDensity.HeaderMaxH, genSpaciousDensity.HeaderMaxH},
		"eqSliderH":             {genCompactDensity.EqSliderH, genComfortableDensity.EqSliderH, genSpaciousDensity.EqSliderH},
		"eqHandleRadius":        {genCompactDensity.EqHandleRadius, genComfortableDensity.EqHandleRadius, genSpaciousDensity.EqHandleRadius},
		"eqDBInputH":            {genCompactDensity.EqDBInputH, genComfortableDensity.EqDBInputH, genSpaciousDensity.EqDBInputH},
		"sliderThumbH":          {genCompactDensity.SliderThumbH, genComfortableDensity.SliderThumbH, genSpaciousDensity.SliderThumbH},
		"sliderThumbW":          {genCompactDensity.SliderThumbW, genComfortableDensity.SliderThumbW, genSpaciousDensity.SliderThumbW},
		"sliderTrackH":          {genCompactDensity.SliderTrackH, genComfortableDensity.SliderTrackH, genSpaciousDensity.SliderTrackH},
		"nodeMinPx":             {genCompactDensity.NodeMinPx, genComfortableDensity.NodeMinPx, genSpaciousDensity.NodeMinPx},
		"nodeMaxPx":             {genCompactDensity.NodeMaxPx, genComfortableDensity.NodeMaxPx, genSpaciousDensity.NodeMaxPx},
		"edgeThickMul":          {genCompactDensity.EdgeThickMul, genComfortableDensity.EdgeThickMul, genSpaciousDensity.EdgeThickMul},
		"accentStripeWidth":     {genCompactDensity.AccentStripeWidth, genComfortableDensity.AccentStripeWidth, genSpaciousDensity.AccentStripeWidth},
		"accentStripeInsetY":    {genCompactDensity.AccentStripeInsetY, genComfortableDensity.AccentStripeInsetY, genSpaciousDensity.AccentStripeInsetY},
		"popupBtnW":             {genCompactDensity.PopupBtnW, genComfortableDensity.PopupBtnW, genSpaciousDensity.PopupBtnW},
		"popupBtnH":             {genCompactDensity.PopupBtnH, genComfortableDensity.PopupBtnH, genSpaciousDensity.PopupBtnH},
		"popupGap":              {genCompactDensity.PopupGap, genComfortableDensity.PopupGap, genSpaciousDensity.PopupGap},
		"popupPad":              {genCompactDensity.PopupPad, genComfortableDensity.PopupPad, genSpaciousDensity.PopupPad},
		"popupSectionGap":       {genCompactDensity.PopupSectionGap, genComfortableDensity.PopupSectionGap, genSpaciousDensity.PopupSectionGap},
		"closeButtonSize":       {genCompactDensity.CloseButtonSize, genComfortableDensity.CloseButtonSize, genSpaciousDensity.CloseButtonSize},
		"splitterHandleLen":     {genCompactDensity.SplitterHandleLen, genComfortableDensity.SplitterHandleLen, genSpaciousDensity.SplitterHandleLen},
		"splitterHandleThk":     {genCompactDensity.SplitterHandleThk, genComfortableDensity.SplitterHandleThk, genSpaciousDensity.SplitterHandleThk},
		"splitterGrabThreshold": {genCompactDensity.SplitterGrabThreshold, genComfortableDensity.SplitterGrabThreshold, genSpaciousDensity.SplitterGrabThreshold},
		"controlGap":            {genCompactDensity.ControlGap, genComfortableDensity.ControlGap, genSpaciousDensity.ControlGap},
		"controlGroupPad":       {genCompactDensity.ControlGroupPad, genComfortableDensity.ControlGroupPad, genSpaciousDensity.ControlGroupPad},
		"controlPadding":        {genCompactDensity.ControlPadding, genComfortableDensity.ControlPadding, genSpaciousDensity.ControlPadding},
		"fxToggleTrackW":        {genCompactDensity.FxToggleTrackW, genComfortableDensity.FxToggleTrackW, genSpaciousDensity.FxToggleTrackW},
		"fxToggleTrackH":        {genCompactDensity.FxToggleTrackH, genComfortableDensity.FxToggleTrackH, genSpaciousDensity.FxToggleTrackH},
		"fxToggleThumbD":        {genCompactDensity.FxToggleThumbD, genComfortableDensity.FxToggleThumbD, genSpaciousDensity.FxToggleThumbD},
		"timelineBarH":          {genCompactDensity.TimelineBarH, genComfortableDensity.TimelineBarH, genSpaciousDensity.TimelineBarH},
	}
	for key, w := range want {
		gotC, okC := gotCompact[key]
		gotComf, okComf := gotComfortable[key]
		gotS, okS := gotSpacious[key]
		if !okC || !okComf || !okS {
			t.Errorf("densities.%s missing from one of: compact=%v comfortable=%v spacious=%v",
				key, okC, okComf, okS)
			continue
		}
		nC, _ := strconv.Atoi(strings.TrimSpace(gotC))
		nComf, _ := strconv.Atoi(strings.TrimSpace(gotComf))
		nS, _ := strconv.Atoi(strings.TrimSpace(gotS))
		if nC != w[0] || nComf != w[1] || nS != w[2] {
			t.Errorf("densities.%s: YAML=[%d %d %d] but Go=[%d %d %d]",
				key, nC, nComf, nS, w[0], w[1], w[2])
		}
	}
}

// key → raw value for one density.
func extractDensityBlock(front, density string) map[string]string {
	out := map[string]string{}
	// Find `densities:` block.
	start := strings.Index(front, "\ndensities:\n")
	if start < 0 {
		return out
	}
	rest := front[start+len("\ndensities:\n"):]
	// Find this density's sub-block: 2 spaces + name + ":" + newline.
	header := "  " + density + ":\n"
	dstart := strings.Index(rest, header)
	if dstart < 0 {
		return out
	}
	body := rest[dstart+len(header):]
	// Until the indentation falls below 4: the next density or top-level key.
	for _, line := range strings.Split(body, "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "    ") {
			break
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		colon := strings.Index(trimmed, ":")
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:colon])
		val := strings.TrimSpace(trimmed[colon+1:])
		out[key] = val
	}
	return out
}
