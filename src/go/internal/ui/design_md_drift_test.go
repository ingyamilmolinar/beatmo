package ui

import (
	"image/color"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestDesignMDDrift pins the YAML front matter in DESIGN.md to the runtime Go
// constants. If a color/spacing/radius value changes in DESIGN.md without a
// matching update in theme.go / touch_sizes.go (or vice versa), this test
// fails. This is the bridge contract: DESIGN.md is the design source of
// truth; Go is its runtime expression.
//
// To extend: add an entry to wantColors / wantSpacing / wantRounded mapping
// the YAML token name to the Go constant value.
func TestDesignMDDrift(t *testing.T) {
	path := findDesignMD(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read DESIGN.md: %v", err)
	}
	front := frontMatter(string(body))
	if front == "" {
		t.Fatalf("DESIGN.md has no YAML front matter; this test (and the Stitch lint) require it")
	}

	gotColors := extractScalarBlock(front, "colors")
	gotSpacing := extractScalarBlock(front, "spacing")
	gotRounded := extractScalarBlock(front, "rounded")

	// Every color in DESIGN.md's `colors:` block must map to a Go constant
	// here. RGB components are compared bit-exact; alpha is verified by
	// the constants whose Go form is RGBA (alpha 255 implicit). Tokens
	// whose Go form is NRGBA with intentional alpha (e.g. colEQBg, colEQCurveFill,
	// border tokens) are RGB-compared via their .R/.G/.B fields below.
	wantColors := map[string]color.RGBA{
		"primary":             colAccent,
		"primary-bright":      colAccentBright,
		"primary-dim":         colAccentDim,
		"background":          colBGTop,
		"surface-1":           colSurface1,
		"surface-2":           colSurface2,
		"surface-3":           colSurface3,
		"on-surface":          colTextPrimary,
		"on-surface-muted":    colTextSecondary,
		"on-surface-disabled": colTextDisabled,
		"on-surface-accent":   colTextAccent,
		"success":             colPlayGreen,
		"error":               colStopRed,
		"mute":                colMuteRed,
		"destructive":         colDeleteFill,
		"destructive-border":  colDeleteBorder,
		"viz-bar":             colEQBar,
		"viz-bar-peak":        colEQBarPeak,
		"viz-wave-a":          colWaveTrace,
		// Spectrum frequency-group tinting tokens (Phase 5 audio-panel polish).
		"viz-bass":   colVizBass,
		"viz-mids":   colVizMids,
		"viz-treble": colVizTreble,
		// ── Phase 3 PR1: graph pane / timeline / drum-mute / EQ overlay ──
		// Each pinned to the runtime alias declared in theme.go. RGB-only
		// comparison; tokens that pick up a non-255 alpha at runtime are
		// listed under rgbOnly below.
		"grid-line":               colGridLine,
		"grid-half":               colGridHalf,
		"grid-quarter":            colGridQuarter,
		"grid-eighth":             colGridEighth,
		"grid-sixteenth":          colGridSixteenth,
		"grid-thirty-second":      colGridThirtySecond,
		"node-fill":               genColorNodeFill,
		"node-border":             genColorNodeBorder,
		"timeline-total-bg":       colTimelineTotal,
		"timeline-view-hi":        colTimelineViewHi,
		"timeline-cursor":         colTimelineCursor,
		"timeline-beat":           colTimelineBeat,
		"drum-mute-cell":          colMuteCell,
		"incdec-icon-hi":          colIncDecIconHi,
	}
	for key, want := range wantColors {
		raw, ok := gotColors[key]
		if !ok {
			t.Errorf("DESIGN.md colors.%s missing", key)
			continue
		}
		got, err := parseHex(raw)
		if err != nil {
			t.Errorf("DESIGN.md colors.%s = %q: %v", key, raw, err)
			continue
		}
		if got.R != want.R || got.G != want.G || got.B != want.B {
			t.Errorf("DESIGN.md colors.%s = #%02X%02X%02X but Go has {%d,%d,%d}",
				key, got.R, got.G, got.B, want.R, want.G, want.B)
		}
	}

	// Tokens whose Go form carries a non-255 alpha (panels, viz canvas,
	// borders, EQ curves, wave-mid) only compare RGB. Alpha is applied
	// in code, not encoded as part of the color token.
	type rgbBytes struct{ R, G, B uint8 }
	rgbOnly := map[string]rgbBytes{
		"surface-overlay": {colPanelBG.R, colPanelBG.G, colPanelBG.B},
		"viz-bg":          {colEQBg.R, colEQBg.G, colEQBg.B},
		"viz-curve":       {colEQCurve.R, colEQCurve.G, colEQCurve.B},
		"viz-wave-b":      {colWaveMid.R, colWaveMid.G, colWaveMid.B},
		// Record state colors are NRGBA in Go (always opaque, but the
		// type forces .R/.G/.B access).
		"record-idle":   {colRecordIdle.R, colRecordIdle.G, colRecordIdle.B},
		"record-active": {colRecordActive.R, colRecordActive.G, colRecordActive.B},
		// Border base — Go encodes per-strength alphas; the RGB is white
		// across all three border tokens. Verify against colBorderSubtle.
		"border": {colBorderSubtle.R, colBorderSubtle.G, colBorderSubtle.B},
		// ── Phase 3 PR1: tokens whose runtime form picks up a non-255 alpha ──
		"edge-color":               {EdgeUI.Color.(color.NRGBA).R, EdgeUI.Color.(color.NRGBA).G, EdgeUI.Color.(color.NRGBA).B},
		"splitter-handle":          {colSplitterHandle.R, colSplitterHandle.G, colSplitterHandle.B},
		"splitter-handle-mobile":   {colSplitterHandleMobile.R, colSplitterHandleMobile.G, colSplitterHandleMobile.B},
		"splitter-grip-line":       {colSplitterGripLine.R, colSplitterGripLine.G, colSplitterGripLine.B},
		"splitter-grip-line-hover": {colSplitterGripLineHover.R, colSplitterGripLineHover.G, colSplitterGripLineHover.B},
		"timeline-view":            {colTimelineView.R, colTimelineView.G, colTimelineView.B},
		"drum-mute-highlight":      {colMuteHighlight.R, colMuteHighlight.G, colMuteHighlight.B},
		"wave-trace-dry":           {colWaveTraceDry.R, colWaveTraceDry.G, colWaveTraceDry.B},
		"eq-zero-line":             {colEQZeroLine.R, colEQZeroLine.G, colEQZeroLine.B},
		"menu-delete-tint":         {colMenuGroupDeleteBG.R, colMenuGroupDeleteBG.G, colMenuGroupDeleteBG.B},
	}
	for key, want := range rgbOnly {
		raw, ok := gotColors[key]
		if !ok {
			t.Errorf("DESIGN.md colors.%s missing", key)
			continue
		}
		got, err := parseHex(raw)
		if err != nil {
			t.Errorf("DESIGN.md colors.%s = %q: %v", key, raw, err)
			continue
		}
		if got.R != want.R || got.G != want.G || got.B != want.B {
			t.Errorf("DESIGN.md colors.%s RGB = #%02X%02X%02X but Go has {%d,%d,%d}",
				key, got.R, got.G, got.B, want.R, want.G, want.B)
		}
	}

	wantSpacing := map[string]int{
		"xs":        SpaceXS,
		"sm":        SpaceSM,
		"md":        SpaceMD,
		"lg":        SpaceLG,
		"xl":        SpaceXL,
		"xxl":       SpaceXXL,
		"btn-sm":    BtnHeightSM,
		"btn-md":    BtnHeightMD,
		"btn-lg":    BtnHeightLG,
		"touch-min": BtnHeightLG, // alias
		"icon-sm":   IconSizeSM,
		"icon-md":   IconSizeMD,
		"icon-lg":   IconSizeLG,
	}
	for key, want := range wantSpacing {
		raw, ok := gotSpacing[key]
		if !ok {
			t.Errorf("DESIGN.md spacing.%s missing", key)
			continue
		}
		got, err := parsePx(raw)
		if err != nil {
			t.Errorf("DESIGN.md spacing.%s = %q: %v", key, raw, err)
			continue
		}
		if got != want {
			t.Errorf("DESIGN.md spacing.%s = %dpx but Go has %d", key, got, want)
		}
	}

	wantRounded := map[string]int{
		"sm": RadiusSM,
		"md": RadiusMD,
		"lg": RadiusLG,
		"xl": RadiusXL,
	}
	for key, want := range wantRounded {
		raw, ok := gotRounded[key]
		if !ok {
			t.Errorf("DESIGN.md rounded.%s missing", key)
			continue
		}
		got, err := parsePx(raw)
		if err != nil {
			t.Errorf("DESIGN.md rounded.%s = %q: %v", key, raw, err)
			continue
		}
		if got != want {
			t.Errorf("DESIGN.md rounded.%s = %dpx but Go has %d", key, got, want)
		}
	}

	gotIcon := extractScalarBlock(front, "icon")
	wantIconInts := map[string]int{
		"grid":    IconGrid,
		"padding": IconPadding,
		"radius":  IconCornerRadius,
	}
	for key, want := range wantIconInts {
		raw, ok := gotIcon[key]
		if !ok {
			t.Errorf("DESIGN.md icon.%s missing", key)
			continue
		}
		got, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			t.Errorf("DESIGN.md icon.%s = %q: %v", key, raw, err)
			continue
		}
		if got != want {
			t.Errorf("DESIGN.md icon.%s = %d but Go has %d", key, got, want)
		}
	}
	if raw, ok := gotIcon["stroke"]; !ok {
		t.Errorf("DESIGN.md icon.stroke missing")
	} else {
		gotF, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			t.Errorf("DESIGN.md icon.stroke = %q: %v", raw, err)
		} else if math.Abs(gotF-float64(IconStrokeWeight)) > 0.001 {
			t.Errorf("DESIGN.md icon.stroke = %.4f but Go has %.4f", gotF, IconStrokeWeight)
		}
	}

	// ── Phase 3 PR2: instrumentDefaults / instrumentFallbackPalette drift ──
	// The generator emits genInstrumentDefaults (map) +
	// genInstrumentFallbackPalette (slice) from DESIGN.md. Both shapes
	// flow through theme.go's buildInstColors / buildCustomPalette
	// helpers into instColors / customPalette. Verify the runtime values
	// match what's in DESIGN.md by parsing the YAML blocks here via a
	// regex-only path (independent of the generator's yaml.v3 codepath).
	defaults := parseInstrumentDefaultsBlock(front)
	if len(defaults) == 0 {
		t.Errorf("DESIGN.md instrumentDefaults: missing or empty")
	}
	for id, hex := range defaults {
		want, err := parseHex(hex)
		if err != nil {
			t.Errorf("DESIGN.md instrumentDefaults.%s = %q: %v", id, hex, err)
			continue
		}
		got, ok := genInstrumentDefaults[id]
		if !ok {
			t.Errorf("DESIGN.md instrumentDefaults.%s present in YAML but missing from genInstrumentDefaults", id)
			continue
		}
		if got.R != want.R || got.G != want.G || got.B != want.B {
			t.Errorf("instrumentDefaults.%s: YAML #%02X%02X%02X but generated {%d,%d,%d}",
				id, want.R, want.G, want.B, got.R, got.G, got.B)
		}
	}
	fallbacks := parseInstrumentFallbackBlock(front)
	if len(fallbacks) == 0 {
		t.Errorf("DESIGN.md instrumentFallbackPalette: missing or empty")
	}
	if len(fallbacks) != len(genInstrumentFallbackPalette) {
		t.Errorf("instrumentFallbackPalette length: YAML=%d but generated=%d",
			len(fallbacks), len(genInstrumentFallbackPalette))
	}
	for i, hex := range fallbacks {
		if i >= len(genInstrumentFallbackPalette) {
			break
		}
		want, err := parseHex(hex)
		if err != nil {
			t.Errorf("DESIGN.md instrumentFallbackPalette[%d] = %q: %v", i, hex, err)
			continue
		}
		got := genInstrumentFallbackPalette[i]
		if got.R != want.R || got.G != want.G || got.B != want.B {
			t.Errorf("instrumentFallbackPalette[%d]: YAML #%02X%02X%02X but generated {%d,%d,%d}",
				i, want.R, want.G, want.B, got.R, got.G, got.B)
		}
	}

	// ── Phase 3 PR3: animations drift ──
	// Each animation entry in DESIGN.md must match the corresponding
	// genAnim* runtime value. exp-decay → ExpDecayAnim struct;
	// sin-pulse → SinPulseAnim struct; fade → FadeFactor.
	anims := parseAnimationsBlock(front)
	wantExpDecay := map[string]ExpDecayAnim{
		"highlight-decay":    genAnimHighlightDecay,
		"button-press-decay": genAnimButtonPressDecay,
	}
	for name, want := range wantExpDecay {
		fields, ok := anims[name]
		if !ok {
			t.Errorf("DESIGN.md animations.%s missing", name)
			continue
		}
		if fields["kind"] != "exp-decay" {
			t.Errorf("DESIGN.md animations.%s kind=%q, want exp-decay", name, fields["kind"])
		}
		if got, _ := strconv.ParseFloat(fields["rate"], 64); got != want.Rate {
			t.Errorf("animations.%s.rate = %v but Go has %v", name, got, want.Rate)
		}
		if got, _ := strconv.ParseFloat(fields["threshold"], 64); got != want.Threshold {
			t.Errorf("animations.%s.threshold = %v but Go has %v", name, got, want.Threshold)
		}
	}
	wantSinPulse := map[string]SinPulseAnim{
		"playhead-pulse":      genAnimPlayheadPulse,
		"button-toggle-pulse": genAnimButtonTogglePulse,
	}
	for name, want := range wantSinPulse {
		fields, ok := anims[name]
		if !ok {
			t.Errorf("DESIGN.md animations.%s missing", name)
			continue
		}
		if got, _ := strconv.ParseFloat(fields["frame-step"], 64); got != want.FrameStep {
			t.Errorf("animations.%s.frame-step = %v but Go has %v", name, got, want.FrameStep)
		}
		if got, _ := strconv.ParseFloat(fields["amplitude"], 64); got != want.Amplitude {
			t.Errorf("animations.%s.amplitude = %v but Go has %v", name, got, want.Amplitude)
		}
		if got, _ := strconv.ParseFloat(fields["base"], 64); got != want.Base {
			t.Errorf("animations.%s.base = %v but Go has %v", name, got, want.Base)
		}
		if got, _ := strconv.Atoi(fields["alpha-scale"]); uint8(got) != want.AlphaScale {
			t.Errorf("animations.%s.alpha-scale = %v but Go has %v", name, got, want.AlphaScale)
		}
	}
	wantFade := map[string]FadeFactor{
		"signal-glow-outer":    genAnimSignalGlowOuter,
		"node-trigger-glow":    genAnimNodeTriggerGlow,
		"playhead-column-fade": genAnimPlayheadColumnFade,
		"edge-faded":           genAnimEdgeFaded,
		"highlight-faded":      genAnimHighlightFaded,
		"button-glow-rest":     genAnimButtonGlowRest,
	}
	for name, want := range wantFade {
		fields, ok := anims[name]
		if !ok {
			t.Errorf("DESIGN.md animations.%s missing", name)
			continue
		}
		if fields["kind"] != "fade" {
			t.Errorf("DESIGN.md animations.%s kind=%q, want fade", name, fields["kind"])
		}
		if got, _ := strconv.ParseFloat(fields["factor"], 64); FadeFactor(got) != want {
			t.Errorf("animations.%s.factor = %v but Go has %v", name, got, want)
		}
	}

	// ── Phase 3 PR4: geometry drift ──
	// Each entry under DESIGN.md `geometry:` must equal the
	// corresponding genGeom* runtime constant. extractScalarBlock
	// yields scalar key→string-value pairs; convert to float64 for
	// comparison.
	gotGeometry := extractScalarBlock(front, "geometry")
	wantGeometry := map[string]float64{
		"signal-glow-radius-multiplier": genGeomSignalGlowRadiusMultiplier,
		"edge-arrow-step-fraction":      genGeomEdgeArrowStepFraction,
		"node-border-thickness":         genGeomNodeBorderThickness,
		"highlight-border-thickness":    genGeomHighlightBorderThickness,
		"button-glow-radius-px":         genGeomButtonGlowRadiusPx,
		"button-inner-shadow-px":        genGeomButtonInnerShadowPx,
		"button-press-scale":            genGeomButtonPressScale,
		"button-release-overshoot":      genGeomButtonReleaseOvershoot,
	}
	for key, want := range wantGeometry {
		raw, ok := gotGeometry[key]
		if !ok {
			t.Errorf("DESIGN.md geometry.%s missing", key)
			continue
		}
		got, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
		if err != nil {
			t.Errorf("DESIGN.md geometry.%s = %q: %v", key, raw, err)
			continue
		}
		if got != want {
			t.Errorf("DESIGN.md geometry.%s = %v but Go has %v", key, got, want)
		}
	}

	gotAlpha := extractScalarBlock(front, "alpha")
	wantAlpha := map[string]uint8{
		"faint":   AlphaFaint,
		"subtle":  AlphaSubtle,
		"medium":  AlphaMedium,
		"strong":  AlphaStrong,
		"overlay": AlphaOverlay,
	}
	for key, want := range wantAlpha {
		raw, ok := gotAlpha[key]
		if !ok {
			t.Errorf("DESIGN.md alpha.%s missing", key)
			continue
		}
		got, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			t.Errorf("DESIGN.md alpha.%s = %q: %v", key, raw, err)
			continue
		}
		if uint8(got) != want {
			t.Errorf("DESIGN.md alpha.%s = %d but Go has %d", key, got, want)
		}
	}
}

// TestDesignMDComponentDrift extends the primitive drift test to cover
// the components: block. For every entry in design_components.gen.go,
// it independently parses the corresponding DESIGN.md YAML stanza
// (regex-based, via parseComponentsBlock from design_md_contrast_test.go,
// distinct from the generator's yaml.v3 codepath) and asserts the
// resolved backgroundColor → spec.Fill mapping matches.
//
// This catches the class of generator regression that the freshness
// test cannot: a generator bug that emits the wrong (but consistent)
// value will produce a green freshness diff and a green
// components_drift_test (which compares Go-vs-Go), but a red drift
// here (which compares YAML-vs-Go via an independent code path).
//
// We intentionally check only `Fill` per component: validating every
// field would require duplicating the generator's resolver, which
// defeats the "independent path" goal. Fill is the most load-bearing
// surface — a wrong Fill is the screenshot regression most likely to
// slip through review. Adding more fields here is welcome but must
// stay regex-based so the two parsers do not converge.
func TestDesignMDComponentDrift(t *testing.T) {
	path := findDesignMD(t)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read DESIGN.md: %v", err)
	}
	front := frontMatter(string(body))
	if front == "" {
		t.Fatalf("DESIGN.md has no YAML front matter")
	}

	colors := extractScalarBlock(front, "colors")
	components := parseComponentsBlock(front)

	// Each entry: kebab-case YAML name → Go ComponentID. Only specs whose
	// generated Fill is byte-equal to a DESIGN.md backgroundColor are
	// listed. Specs with default-zero Fill (e.g. ComponentPanel using
	// {colors.surface-overlay} which carries non-255 alpha) or specs
	// computed from variant deltas are out of scope here — the contrast
	// test (which has its own RGB-only path) covers those.
	cases := []struct {
		yamlName string
		id       ComponentID
	}{
		{"button-secondary", ComponentButtonSecondary},
		{"button-row-control", ComponentButtonRowControl},
		{"button-row-control-mute-active", ComponentButtonRowControlMuteActive},
		{"button-row-control-solo-active", ComponentButtonRowControlSoloActive},
		{"button-row-control-fx-active", ComponentButtonRowControlFxActive},
		{"button-destructive", ComponentButtonDestructive},
		{"button-destructive-confirm", ComponentButtonDestructiveConfirm},
		{"button-disabled", ComponentButtonDisabled},
		{"button-eq-mute", ComponentButtonEqMute},
		{"button-eq-mute-active", ComponentButtonEqMuteActive},
		{"button-eq-filter-active", ComponentButtonEqFilterActive},
		{"button-missing-instrument", ComponentButtonMissingInstrument},
		{"input-field", ComponentInputField},
	}

	for _, tc := range cases {
		fields, ok := components[tc.yamlName]
		if !ok {
			// Component absent from DESIGN.md is a separate failure
			// mode (the generator should have rejected the build).
			t.Errorf("component %q not present in DESIGN.md components: block", tc.yamlName)
			continue
		}
		ref, ok := fields["backgroundColor"]
		if !ok {
			// Components without a backgroundColor (synthetic / panel
			// references) skip the Fill comparison.
			continue
		}
		hex, ok := resolveColorRef(ref, colors)
		if !ok {
			t.Errorf("component %q: cannot resolve backgroundColor %q", tc.yamlName, ref)
			continue
		}
		want, err := parseHex(hex)
		if err != nil {
			t.Errorf("component %q: backgroundColor %q: %v", tc.yamlName, hex, err)
			continue
		}
		spec := Spec(tc.id)
		// Compare RGB only; alpha is fixed at 255 for resting fills and
		// per-component alpha overrides are encoded elsewhere (variant
		// rebindings, BorderRef.Alpha) which would be tested separately.
		if spec.Fill.R != want.R || spec.Fill.G != want.G || spec.Fill.B != want.B {
			t.Errorf("DESIGN.md components.%s.backgroundColor = #%02X%02X%02X but generated Spec(%v).Fill = {%d,%d,%d}",
				tc.yamlName, want.R, want.G, want.B, tc.id, spec.Fill.R, spec.Fill.G, spec.Fill.B)
		}
	}
}

func findDesignMD(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "DESIGN.md")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("DESIGN.md not found walking up from %s", cwd)
	return ""
}

var frontMatterRE = regexp.MustCompile(`(?s)^---\n(.*?)\n---`)

func frontMatter(s string) string {
	m := frontMatterRE.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	return m[1]
}

// extractScalarBlock returns the immediate scalar children of a top-level
// YAML key. Indented sub-keys (like typography role objects) are ignored.
func extractScalarBlock(front, key string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(front, "\n")
	in := false
	headerRE := regexp.MustCompile(`^` + regexp.QuoteMeta(key) + `:\s*$`)
	scalarRE := regexp.MustCompile(`^\s\s([a-zA-Z][a-zA-Z0-9-]*):\s*"?([^"\n]*?)"?\s*$`)
	for _, ln := range lines {
		if headerRE.MatchString(ln) {
			in = true
			continue
		}
		if in {
			if len(ln) > 0 && ln[0] != ' ' {
				break
			}
			if m := scalarRE.FindStringSubmatch(ln); m != nil {
				out[m[1]] = m[2]
			}
		}
	}
	return out
}

func parseHex(raw string) (color.RGBA, error) {
	s := strings.TrimSpace(strings.TrimPrefix(raw, "#"))
	if len(s) != 6 {
		return color.RGBA{}, &parseErr{raw: raw, msg: "expected #RRGGBB"}
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, err
	}
	return color.RGBA{
		R: uint8(v >> 16 & 0xFF),
		G: uint8(v >> 8 & 0xFF),
		B: uint8(v & 0xFF),
		A: 255,
	}, nil
}

func parsePx(raw string) (int, error) {
	s := strings.TrimSpace(strings.TrimSuffix(raw, "px"))
	return strconv.Atoi(s)
}

type parseErr struct{ raw, msg string }

func (e *parseErr) Error() string { return e.msg + ": " + e.raw }

// parseInstrumentDefaultsBlock reads the `instrumentDefaults:` sequence
// from DESIGN.md's front matter and returns id→hex. Regex-based by
// design — kept independent of the generator's yaml.v3 codepath so a
// generator regression that emits the wrong (but consistent) value
// fails a YAML-vs-Go drift check rather than silently slipping through
// the freshness diff. Returns nil when the block is missing.
func parseInstrumentDefaultsBlock(front string) map[string]string {
	out := map[string]string{}
	lines := strings.Split(front, "\n")
	headerRE := regexp.MustCompile(`^instrumentDefaults:\s*$`)
	idRE := regexp.MustCompile(`^\s+-\s+id:\s+([a-z][a-z0-9-]*)\s*$`)
	colorRE := regexp.MustCompile(`^\s+color:\s*"?(#[0-9A-Fa-f]+)"?\s*$`)
	in := false
	pendingID := ""
	for _, ln := range lines {
		if headerRE.MatchString(ln) {
			in = true
			continue
		}
		if !in {
			continue
		}
		// Stop at the next non-indented non-blank line.
		if len(ln) > 0 && ln[0] != ' ' {
			break
		}
		if m := idRE.FindStringSubmatch(ln); m != nil {
			pendingID = m[1]
			continue
		}
		if m := colorRE.FindStringSubmatch(ln); m != nil && pendingID != "" {
			out[pendingID] = m[1]
			pendingID = ""
		}
	}
	return out
}

// parseAnimationsBlock reads the `animations:` mapping from DESIGN.md
// and returns name → field-map. Each animation entry is a sub-mapping
// of scalar fields (kind, rate, threshold, frame-step, amplitude,
// base, alpha-scale, factor) — the parser is regex-only, independent
// of the generator's yaml.v3 codepath, so a generator regression that
// emits a wrong-but-consistent value fails this drift check.
func parseAnimationsBlock(front string) map[string]map[string]string {
	out := map[string]map[string]string{}
	lines := strings.Split(front, "\n")
	headerRE := regexp.MustCompile(`^animations:\s*$`)
	entryRE := regexp.MustCompile(`^\s\s([a-z][a-z0-9-]*):\s*$`)
	fieldRE := regexp.MustCompile(`^\s\s\s\s([a-z][a-z0-9-]*):\s*([^\s].*?)\s*$`)
	in := false
	current := ""
	for _, ln := range lines {
		if headerRE.MatchString(ln) {
			in = true
			continue
		}
		if !in {
			continue
		}
		if len(ln) > 0 && ln[0] != ' ' {
			break
		}
		if m := entryRE.FindStringSubmatch(ln); m != nil {
			current = m[1]
			out[current] = map[string]string{}
			continue
		}
		if current == "" {
			continue
		}
		if m := fieldRE.FindStringSubmatch(ln); m != nil {
			out[current][m[1]] = m[2]
		}
	}
	return out
}

// parseInstrumentFallbackBlock reads the `instrumentFallbackPalette:`
// sequence from DESIGN.md and returns the source-ordered hex slice.
// Same regex-only contract as parseInstrumentDefaultsBlock.
func parseInstrumentFallbackBlock(front string) []string {
	var out []string
	lines := strings.Split(front, "\n")
	headerRE := regexp.MustCompile(`^instrumentFallbackPalette:\s*$`)
	colorRE := regexp.MustCompile(`^\s+-\s+color:\s*"?(#[0-9A-Fa-f]+)"?\s*$`)
	in := false
	for _, ln := range lines {
		if headerRE.MatchString(ln) {
			in = true
			continue
		}
		if !in {
			continue
		}
		if len(ln) > 0 && ln[0] != ' ' {
			break
		}
		if m := colorRE.FindStringSubmatch(ln); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}
