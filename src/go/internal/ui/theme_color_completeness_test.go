package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The drift test only sees colours already in DESIGN.md; this inverts it so every theme.go colour is claimed. A new
// one is token-backed (YAML entry, drift mapping, tokenBackedColorNames) or, only for runtime composites and
// variants with no component, allowlisted with a reason.
func TestThemeColorsTokenized(t *testing.T) {
	uiDir := findUIDir(t)
	themePath := filepath.Join(uiDir, "theme.go")

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, themePath, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse theme.go: %v", err)
	}

	var declared []string
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.VAR {
			continue
		}
		for _, spec := range gd.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range vs.Names {
				if i >= len(vs.Values) {
					continue
				}
				if !isColorLiteral(vs.Values[i]) {
					continue
				}
				declared = append(declared, name.Name)
			}
		}
	}

	// The initialisers also live inside rebuildThemeAliases(), re-run after ApplyTheme.
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != "rebuildThemeAliases" || fd.Body == nil {
			continue
		}
		for _, st := range fd.Body.List {
			as, ok := st.(*ast.AssignStmt)
			if !ok || len(as.Lhs) != 1 || len(as.Rhs) != 1 {
				continue
			}
			id, ok := as.Lhs[0].(*ast.Ident)
			if !ok || !isColorLiteral(as.Rhs[0]) {
				continue
			}
			declared = append(declared, id.Name)
		}
	}

	if len(declared) == 0 {
		t.Fatalf("AST walk found no color.RGBA/NRGBA declarations in theme.go — parser regression?")
	}

	var unclaimed []string
	for _, name := range declared {
		if tokenBackedColorNames[name] {
			continue
		}
		if _, ok := untokenizedAllowlist[name]; ok {
			continue
		}
		unclaimed = append(unclaimed, name)
	}
	if len(unclaimed) > 0 {
		sort.Strings(unclaimed)
		t.Errorf("theme.go declares color constant(s) not claimed by DESIGN.md or the untokenized allowlist:\n  %s\n"+
			"Add a YAML entry to DESIGN.md + drift-test mapping (preferred) "+
			"or add the name to untokenizedAllowlist in theme_color_completeness_test.go with a justification.",
			strings.Join(unclaimed, "\n  "))
	}

	// Entries without a declaration fail, forcing cleanup when a constant goes.
	declaredSet := map[string]bool{}
	for _, n := range declared {
		declaredSet[n] = true
	}
	var staleTokenBacked, staleAllowlist []string
	for n := range tokenBackedColorNames {
		if !declaredSet[n] {
			staleTokenBacked = append(staleTokenBacked, n)
		}
	}
	for n := range untokenizedAllowlist {
		if !declaredSet[n] {
			staleAllowlist = append(staleAllowlist, n)
		}
	}
	if len(staleTokenBacked) > 0 {
		sort.Strings(staleTokenBacked)
		t.Errorf("tokenBackedColorNames lists name(s) no longer declared in theme.go:\n  %s\n"+
			"Remove from this list and from design_md_drift_test.go.",
			strings.Join(staleTokenBacked, "\n  "))
	}
	if len(staleAllowlist) > 0 {
		sort.Strings(staleAllowlist)
		t.Errorf("untokenizedAllowlist lists name(s) no longer declared in theme.go:\n  %s\n"+
			"Remove the entry — the constant has been deleted or renamed.",
			strings.Join(staleAllowlist, "\n  "))
	}
}

// A composite literal, a bare genColor* identifier, or a WithAlpha* call introduces a colour. A bare col* alias
// only retags an existing one and is not counted.
func isColorLiteral(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.CompositeLit:
		sel, ok := v.Type.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		pkg, ok := sel.X.(*ast.Ident)
		if !ok {
			return false
		}
		return pkg.Name == "color" && (sel.Sel.Name == "RGBA" || sel.Sel.Name == "NRGBA")
	case *ast.Ident:
		return strings.HasPrefix(v.Name, "genColor")
	case *ast.CallExpr:
		ident, ok := v.Fun.(*ast.Ident)
		if !ok {
			return false
		}
		switch ident.Name {
		case "WithAlpha", "WithAlphaNRGBA", "WithAlphaFromColor":
			return true
		}
		return false
	}
	return false
}

// Must mirror the drift test's wantColors and rgbOnly.
var tokenBackedColorNames = map[string]bool{
	"colRackSurface":      true, // DESIGN.md rack-bed
	"colTransportSurface": true, // DESIGN.md toolbar-bed
	"colAccent":           true,
	"colAccentBright":     true,
	"colAccentDim":        true,
	"colBGTop":            true,
	"colSurface1":         true,
	"colSurface2":         true,
	"colSurface3":         true,
	"colTextPrimary":      true,
	"colTextSecondary":    true,
	"colTextDisabled":     true,
	"colTextAccent":       true,
	"colPlayGreen":        true,
	"colStopRed":          true,
	"colMuteRed":          true,
	"colDeleteFill":       true,
	"colDeleteBorder":     true,
	"colEQBar":            true,
	"colEQBarPeak":        true,
	"colWaveTrace":        true,
	"colVizBass":          true, // viz-bass
	"colVizMids":          true, // viz-mids
	"colVizTreble":        true, // viz-treble
	"colPanelBG":          true, // surface-overlay
	"colEQBg":             true, // viz-bg
	"colEQCurve":          true, // viz-curve
	"colWaveMid":          true, // viz-wave-b
	"colRecordIdle":       true,
	"colRecordActive":     true,
	"colBorderSubtle":     true, // colors.border (white base)
	"colGridHorizon":      true, // grid-horizon (sunset grid-pane backdrop)
}

// Each entry needs a reason; lift a value into DESIGN.md whenever a real semantic role exists.
var untokenizedAllowlist = map[string]string{
	// ── Folded per-file derivations (theme rebuild) ──
	"colScopeA":               "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colScopeB":               "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colScopeAFill":           "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colScopeBFill":           "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colScopeDiff":            "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colScopeDiffFill":        "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colScopeBg":              "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colScopeGrid":            "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colScopeGridMid":         "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colScopeFrozenBorder":    "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"meterLow":                "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"meterMid":                "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"meterHigh":               "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"meterBg":                 "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"meterClip":               "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colSpectrumBandEmphasis": "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colSpectrumCurveFill":    "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colSpectrumBarTop":       "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colSpectrumBarMid":       "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colSpectrumBarBase":      "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colWaveTraceFill":        "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colConceptFill":          "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colConceptStroke":        "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"colGraphLo":              "Synth-tab graph role colour (synth_view_draw.go); composes a Visualizers token + a named alpha bucket, re-derived on ApplyTheme",
	"colGraphHi":              "Synth-tab graph role colour (synth_view_draw.go); composes a Visualizers token + a named alpha bucket, re-derived on ApplyTheme",
	"colGraphBefore":          "Synth-tab graph role colour (synth_view_draw.go); composes a Visualizers token + a named alpha bucket, re-derived on ApplyTheme",
	"colGraphDiff":            "Synth-tab graph role colour (synth_view_draw.go); composes a Visualizers token + a named alpha bucket, re-derived on ApplyTheme",
	"colGraphGrid":            "Synth-tab graph role colour (synth_view_draw.go); composes a Visualizers token + a named alpha bucket, re-derived on ApplyTheme",
	"colGraphPlotBg":          "Synth-tab graph role colour (synth_view_draw.go); composes a Visualizers token + a named alpha bucket, re-derived on ApplyTheme",
	"colGraphGlow":            "Synth-tab graph role colour (synth_view_draw.go); composes a Visualizers token + a named alpha bucket, re-derived on ApplyTheme",
	"colGraphGlowRing":        "Synth-tab graph role colour (synth_view_draw.go); composes a Visualizers token + a named alpha bucket, re-derived on ApplyTheme",
	"colFuzzyHighlight":       "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"pillCapFillActive":       "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"pillCapBorderActive":     "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",
	"pillTextActive":          "folded into rebuildThemeAliases from its owning render/chrome file (runtime-theme refactor); composes generated tokens + a named alpha bucket, re-derived on ApplyTheme",

	// Surfaces / backgrounds
	"colBGBottom": "alias of colBGTop (gradient endpoint, currently identical) — collapse if gradient remains flat",

	// Six derived shades: six tokens would add noise without the constraint that they lighten monotonically.
	"colGridLine":         "grid base line — derived shade above surface-1",
	"colGridHalf":         "grid half-beat — derived shade above colGridLine",
	"colGridQuarter":      "grid quarter — derived shade",
	"colGridEighth":       "grid eighth — derived shade",
	"colGridSixteenth":    "grid sixteenth — derived shade",
	"colGridThirtySecond": "grid thirty-second — lightest derived shade",

	// Runtime composites of genColorBorder and named alpha buckets.
	"colBorderMedium": "WithAlpha(genColorBorder, AlphaBorderDefault=15) — control borders",
	"colBorderStrong": "WithAlpha(genColorBorder, AlphaBorderEmphasis=25) — focused inputs",
	// colButtonBorder and colSubtleBorder are bare aliases, not detected; they must not be re-added here.

	// Accent / state aliases and runtime-alpha composites
	"colAccentSubtle": "primary at 20/255 alpha for fills — migrate to WithAlpha(TokenAccent(), AlphaFaint)",
	"colPlayButton":   "desktop play-button fill — slightly richer green than success token; pinned for visual continuity",
	"colStopButton":   "desktop stop-button fill — slightly richer red than error token; pinned for visual continuity",
	"colDropdownEdge": "accent at 50/255 alpha for dropdown borders — runtime alpha composite",
	"colError":        "alias of colStopRed for error-text role — collapse",

	// One handle colour shared by both platforms, with no DESIGN.md component slot.
	"colSplitterHandle":      "splitter handle — sunset-gold touch affordance, shared desktop+mobile",
	"colSplitterHandleHover": "splitter handle hover — runtime brightening",

	// Panel/scrim runtime alphas
	"colPanelBorder": "panel border = border base at panel-border alpha (20/255)",
	"colScrim":       "modal backdrop dim — pure-black at alpha 150; Stitch spec rejects alpha-bearing tokens",

	// Context menu group chrome (alpha composites over surface-overlay)
	"colMenuGroupBG":       "context menu group container — white@8 over surface-overlay",
	"colMenuGroupDeleteBG": "context menu destructive group — destructive@15 over surface-overlay",

	// Drum/timeline cell rendering
	"colStepOff":       "drum cell off state — just above background for depth (raw value, no semantic token)",
	"colStepBorder":    "drum cell border at 10/255 alpha — runtime composite",
	"colHighlight":     "playback highlight flash — cool white, runtime-only",
	"colMuteCell":      "mute cell rendering — derived gray",
	"colMuteHighlight": "mute cell highlight — brighter gray for white flash blend",
	"colBeatGroupAlt":  "beat-group alternating tint — white@4 over surface",

	// Timeline strip
	"colTimelineTotal":  "timeline strip background — darker than surface for inset look",
	"colTimelineView":   "timeline view-window fill — primary at 60/255 alpha (runtime composite)",
	"colTimelineViewHi": "timeline view-window border — accent variant",
	"colTimelineCursor": "timeline playhead cursor — bright cyan, distinct from primary",
	"colTimelineBeat":   "timeline beat tick — neutral gray",

	// Wave / EQ derived
	"colWaveTraceDry":         "wave trace dry signal — desaturated companion to viz-wave-a",
	"colEQCurveFill":          "EQ curve fill = viz-curve at 20/255 — runtime composite",
	"colEQZeroLine":           "EQ 0-dB reference line — neutral gray at 80/255",
	"colEQFilterHandle":       "EQ filter handle — teal accent variant",
	"colEQFilterHandleActive": "EQ filter handle active — brighter variant",
	"colEQFilterLine":         "EQ filter band line — handle color at 60/255",

	// Transport pill group
	"colTransportDivider": "transport divider — border base at 20/255",
	"colTransportGroupBG": "transport pill container fill — white@10",

	// Mute / solo active borders
	"colMuteActiveBdr": "mute active button border — sibling of TokenMuteActiveFill",
	"colSoloActive":    "solo active fill — bright cyan (was amber), distinct from primary",
	"colSoloActiveBdr": "solo active border — lighter sunset-gold companion to colSoloActive",
}
