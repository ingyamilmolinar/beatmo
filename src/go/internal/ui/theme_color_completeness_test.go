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

// TestThemeColorsTokenized AST-parses theme.go and asserts every top-level
// color.RGBA / color.NRGBA constant is *either* token-backed (i.e. mapped
// to a DESIGN.md YAML entry by design_md_drift_test.go) *or* listed in
// the untokenized allowlist below with a justification.
//
// Why: design_md_drift_test.go only checks tokens that appear in DESIGN.md.
// New colors added on the Go side that are not in the YAML are invisible
// to the drift test. This guard inverts the relationship — every theme.go
// color must be claimed somewhere, ratchet-style.
//
// Adding a new color to theme.go: pick one.
//   1. Token-backed (preferred) — add the YAML entry to DESIGN.md and the
//      mapping to design_md_drift_test.go's wantColors / rgbOnly. Then
//      add the name to tokenBackedColorNames below.
//   2. Untokenized (escape hatch) — add the name to untokenizedAllowlist
//      with a one-line justification. Use this only when the value is a
//      runtime-derived alpha composite, a platform-specific variant, or
//      otherwise has no place in a Stitch-modeled component.
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

	// Ratchet: catch token-backed / untokenized entries that no longer
	// correspond to a real declaration. Forces cleanup when a constant is
	// removed.
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

// isColorLiteral reports whether expr declares a *new* color value. After
// Phase 1 of the design-system refactor, color constants in theme.go come
// in three shapes that introduce a color; all count:
//   1. A `color.RGBA{...}` / `color.NRGBA{...}` composite literal — the
//      original form, preserved for runtime alpha composites whose
//      generated form would lose information.
//   2. A bare identifier whose name starts with `genColor` — the Phase 1
//      alias form, where a hand-named constant points directly at a
//      generated token (e.g. `colAccent = genColorPrimary`).
//   3. A call to one of the alpha-rebind helpers — `WithAlpha(c, a)`,
//      `WithAlphaNRGBA(c, a)`, `WithAlphaFromColor(c, a)`. Phase B/C of
//      the refactor migrated many `color.NRGBA{R, G, B, A}` literals to
//      these helpers so the alpha picker is centralised in theme_tokens.go.
//
// Bare-`col*` aliases (e.g. `colBPMBox = colSurface2`) are intentionally
// NOT detected: they don't introduce a new color, they retag an existing
// one. The pre-Phase-1 test ignored them, and Phase 1 preserves that.
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

// tokenBackedColorNames mirrors the union of design_md_drift_test.go's
// wantColors and rgbOnly maps: every Go constant claimed by a DESIGN.md
// YAML entry. Updates to this set must mirror updates to the drift test
// — the discipline is "three places to update" by design.
var tokenBackedColorNames = map[string]bool{
	"colAccent":        true,
	"colAccentBright":  true,
	"colAccentDim":     true,
	"colBGTop":         true,
	"colSurface1":      true,
	"colSurface2":      true,
	"colSurface3":      true,
	"colTextPrimary":   true,
	"colTextSecondary": true,
	"colTextDisabled":  true,
	"colTextAccent":    true,
	"colPlayGreen":     true,
	"colStopRed":       true,
	"colMuteRed":       true,
	"colDeleteFill":    true,
	"colDeleteBorder":  true,
	"colEQBar":         true,
	"colEQBarPeak":     true,
	"colWaveTrace":     true,
	"colVizBass":       true, // viz-bass (Phase 5 audio-panel spectrum tinting)
	"colVizMids":       true, // viz-mids
	"colVizTreble":     true, // viz-treble
	"colPanelBG":       true, // surface-overlay
	"colEQBg":          true, // viz-bg
	"colEQCurve":       true, // viz-curve
	"colWaveMid":       true, // viz-wave-b
	"colRecordIdle":    true,
	"colRecordActive":  true,
	"colBorderSubtle":  true, // colors.border (white base)
}

// untokenizedAllowlist names colors that are intentionally not in DESIGN.md's
// YAML. Each entry must carry a one-line justification — runtime-derived
// alphas, platform variants, aliases, or chrome that has no Stitch component
// to anchor against. Adding to this list is a ratchet failure mode: the
// preference is always to lift values into DESIGN.md when a real semantic
// role exists.
var untokenizedAllowlist = map[string]string{
	// Surfaces / backgrounds
	"colBGBottom": "alias of colBGTop (gradient endpoint, currently identical) — collapse if gradient remains flat",

	// Grid hierarchy — six levels of subdivision lines, derived shades. A
	// single token would obscure the level relationship; lifting six tokens
	// into DESIGN.md adds noise without capturing the constraint that they
	// must monotonically lighten.
	"colGridLine":         "grid base line — derived shade above surface-1",
	"colGridHalf":         "grid half-beat — derived shade above colGridLine",
	"colGridQuarter":      "grid quarter — derived shade",
	"colGridEighth":       "grid eighth — derived shade",
	"colGridSixteenth":    "grid sixteenth — derived shade",
	"colGridThirtySecond": "grid thirty-second — lightest derived shade",

	// Border ladder — runtime composites of genColorBorder + named alpha
	// buckets. Each is a function-call expression, not a YAML token, so they
	// stay on the allowlist with the alpha bucket spelled out.
	"colBorderMedium": "WithAlpha(genColorBorder, AlphaBorderDefault=15) — control borders",
	"colBorderStrong": "WithAlpha(genColorBorder, AlphaBorderEmphasis=25) — focused inputs",
	// colButtonBorder and colSubtleBorder are bare aliases of the above
	// (colBorderMedium / colBorderSubtle) and are intentionally not detected
	// by isColorLiteral. They MUST NOT be added back to this allowlist.

	// Accent / state aliases and runtime-alpha composites
	"colAccentSubtle":  "primary at 20/255 alpha for fills — migrate to WithAlpha(TokenAccent(), AlphaFaint)",
	"colPlayButton":    "desktop play-button fill — slightly richer green than success token; pinned for visual continuity",
	"colStopButton":    "desktop stop-button fill — slightly richer red than error token; pinned for visual continuity",
	"colDropdownEdge":  "accent at 50/255 alpha for dropdown borders — runtime alpha composite",
	"colError":         "alias of colStopRed for error-text role — collapse",

	// Splitter chrome — desktop/mobile differ intentionally; mobile uses
	// primary-dim cyan which is already tokenized as colors.primary-dim
	// indirectly. Splitter has no DESIGN.md component slot.
	"colSplitterHandle":        "splitter handle desktop — neutral pill, no Stitch component slot",
	"colSplitterHandleMobile":  "splitter handle mobile — primary-dim variant, runtime-only",
	"colSplitterHandleHover":   "splitter handle hover — runtime brightening",
	"colSplitterGripLine":      "splitter grip tick mark — desktop chrome",
	"colSplitterGripLineHover": "splitter grip tick hover — desktop chrome",

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

	// Increment/decrement steppers (BPM ±, etc.)
	"colIncDec":       "BPM stepper fill — dark navy, distinct from surface ladder",
	"colIncDecBorder": "BPM stepper border — accent blue",
	"colIncDecIconHi": "BPM stepper icon tint — desktop blue, distinct from accent",

	// Transport pill group
	"colTransportDivider":     "transport divider — border base at 20/255",
	"colTransportGroupBG":     "transport pill container fill — white@10",
	"colTransportGroupBorder": "transport pill container border — white@15",

	// Mute / solo active borders
	"colMuteActiveBdr": "mute active button border — sibling of TokenMuteActiveFill",
	"colSoloActive":    "solo active fill — bright cyan (was amber), distinct from primary",
	"colSoloActiveBdr": "solo active border — lighter cyan companion to colSoloActive",
}
