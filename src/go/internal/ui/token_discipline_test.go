package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestTokenDiscipline guards against new inline color.RGBA{} / color.NRGBA{}
// literals in internal/ui/ source files.
//
// The literal allowlist is per-file. Each entry pins the maximum number of
// literals that file is allowed to contain. When a literal is replaced with
// a Token* accessor, decrement the entry. A file not in the map is required
// to have zero literals.
//
// Adding a new entry to allowedLiteralBudget is permitted only with review.
// The DESIGN.md §0/§5 rule is: every meaningful color in chrome must come
// from a token; only color math, computed gradients, fully-transparent
// fills, and explicit infrastructure are exempt.
//
// Infrastructure files (theme_tokens.go, drawing.go, icons.go,
// design_tokens.gen.go) and *_test.go are not scanned at all.
//
// theme.go is scanned with a small budget (transparent zero-value
// sentinels only) — every meaningful color literal previously inside
// theme.go now flows through a gen* token, and the budget ratchet
// catches regressions in the file the rest of the chrome inherits from.
func TestTokenDiscipline(t *testing.T) {
	root := findUIDir(t)
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatalf("read internal/ui: %v", err)
	}

	excludedFiles := map[string]bool{
		"theme_tokens.go":          true,
		"drawing.go":               true,
		"drawing_icons.go":         true, // 29 IconID bodies; chrome glyph color comes from caller
		"icon_renderer.go":         true, // vector helpers — color is caller-supplied
		"icons.go":                 true,
		"design_tokens.gen.go":     true,
		"design_components.gen.go": true, // generated; literals come from DESIGN.md
		"token_discipline_test.go": true,
	}

	literalRE := regexp.MustCompile(`color\.(N?)RGBA\{`)

	var unexpectedFiles []string
	overBudget := map[string]int{}
	underBudget := map[string]int{}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if excludedFiles[name] {
			continue
		}
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") {
			continue
		}

		body, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Errorf("read %s: %v", name, err)
			continue
		}
		count := len(literalRE.FindAllIndex(body, -1))
		budget, allowed := allowedLiteralBudget[name]

		if count > 0 && !allowed {
			unexpectedFiles = append(unexpectedFiles, name)
			continue
		}
		if count > budget {
			overBudget[name] = count
		}
		if count < budget {
			underBudget[name] = count
		}
	}

	if len(unexpectedFiles) > 0 {
		sort.Strings(unexpectedFiles)
		t.Errorf("new inline color.RGBA{} / color.NRGBA{} literals in files not on the allowlist:\n  %s\n"+
			"Replace with the appropriate Token* accessor in theme_tokens.go, "+
			"or (with review) add the file to allowedLiteralBudget in token_discipline_test.go.",
			strings.Join(unexpectedFiles, "\n  "))
	}

	if len(overBudget) > 0 {
		var msgs []string
		for name, count := range overBudget {
			msgs = append(msgs, "  "+name+": "+itoa(count)+" literals (budget "+itoa(allowedLiteralBudget[name])+")")
		}
		sort.Strings(msgs)
		t.Errorf("inline color literal budget exceeded:\n%s\n"+
			"Replace the new literals with Token* accessors, or justify and bump the budget in token_discipline_test.go.",
			strings.Join(msgs, "\n"))
	}

	if len(underBudget) > 0 {
		var msgs []string
		for name, count := range underBudget {
			msgs = append(msgs, "  "+name+": "+itoa(count)+" literals (budget "+itoa(allowedLiteralBudget[name])+")")
		}
		sort.Strings(msgs)
		t.Errorf("budget can be tightened — actual literal counts are below allowedLiteralBudget:\n%s\n"+
			"Decrement the budget so future regressions are caught.",
			strings.Join(msgs, "\n"))
	}
}

// allowedLiteralBudget pins the current count of inline color literals
// per file. Decrement when literals are replaced; never increment without
// review. Files not in this map are required to have zero literals.
//
// Total: 119 literals across 30 files (audit baseline 2026-04-26 after
// Phase B sweep — render_meters / render_scope / row_rack_zone migrated to
// AlphaPanelBorder / AlphaRowActive + WithAlphaNRGBA / WithAlphaFromColor).
var allowedLiteralBudget = map[string]int{
	"theme.go":                         2, // ContextMenuItemStyle Fill+Border are transparent zero-value sentinels (color.RGBA{0,0,0,0} / color.NRGBA{0,0,0,0}); every other color in theme.go flows through gen* tokens
	"design_types.go":                  1, // BorderRef.Resolve() constructs an NRGBA value at runtime
	"components.go":                    1, // 2 of 3 migrated; 1 remaining is runtime alpha math (fadeColor)
	"drumview.go":                      0, // migrated to row-rack-color-fallback
	"drumview_cache_row_sprite.go":     0, // migrated to drum-stripe-* tokens
	"drumview_colors.go":               0, // HSV→RGB / hash→RGB wheel code removed; ensureUniqueColor is palette-only
	"drumview_context_menu.go":         0, // migrated to scrollbar-thumb alpha
	"drumview_draw.go":                 0, // migrated to drum-stripe + drum-glow + row-rack-zebra
	"drumview_fx_panel.go":             0, // migrated to slider-track-fill + border (white)
	"drumview_overlay_color_comp.go":   0, // swatch-grid picker uses genInstrumentSwatches palette; HSV literals removed
	"eq_panel_zone.go":                 0, // migrated to eq-readout-bg + primary alpha composite + border
	"game_connect_mode.go":             0,
	"game_draw.go":                     0, // throttle/frameBuffer deleted in Phase G; no literals remain
	"game_draw_grid_pane.go":           0, // all migrated to viz-debug-* / viz-pill-* / viz-confirm-* / viz-glow tokens
	"game_draw_helpers.go":             0, // all migrated to divider-* tokens + border-panel alpha
	"game_longpress_popup.go":          0, // migrated to popup-text-secondary
	"game_node_sidebar.go":             0, // all migrated to sidebar-* tokens
	"import.go":                        5, // JSON deserialization color parsing — runtime input, permanent
	"render_meters.go":                 0, // border composite → AlphaPanelBorder; meterColorAlpha → WithAlpha
	"render_scope.go":                  0, // half-mode fill alpha → WithAlphaNRGBA helper
	"render_spectrum.go":               0, // both migrated
	"render_waveform.go":               0, // migrated to border + border-panel alpha
	"row_instrument_shades.go":         1, // rowToggleBaseRGBA narrows an arbitrary instrument color.Color to RGBA (one constructor); all shades derive via adjustColor / WithAlphaFromColor token helpers
	"row_rack_zone.go":                 0, // per-row tints via WithAlphaFromColor; volume slider uses the row's node color directly
	"scope_panel_zone.go":              0, // migrated to scope-label-dim + alpha bucket
	"scrollbar_style.go":               0, // all migrated to scrollbar-* alpha buckets
	"slider.go":                        0, // all migrated to slider-* tokens
	"slider_popup.go":                  0, // migrated to border + strong alpha
	"timeline_zone.go":                 0, // migrated to focus-ring + dim-black tokens
	"transport.go":                     0, // migrated to transport-bar-bg
	"transport_zone.go":                0, // record-pulse + separator migrated
	"uigrid.go":                        0, // migrated to primary + subtle alpha
}

func findUIDir(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// Most test runs are inside src/go/internal/ui already.
	if filepath.Base(cwd) == "ui" {
		return cwd
	}
	// Walk up looking for src/go/internal/ui as a sibling.
	dir := cwd
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "src", "go", "internal", "ui")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate src/go/internal/ui from %s", cwd)
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
