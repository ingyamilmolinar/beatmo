# Theme Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A fourth top-right grid-pane button opens a Theme overlay where the user recolors nine UI categories with the existing swatch-grid picker, live, persisted per user, with an optional per-circuit theme and an override rule.

**Architecture:** Rebuild-in-place. The generated `genColor*` package variables stay the live palette; `ApplyTheme` overwrites the themed ones, re-runs the alias/spec derivation that today happens once at init, and fires `OnThemeChanged` to invalidate color-baking caches. A theme document is `categories` (category id → swatch name) plus optional `tokens` (token name → hex); each category derives its member tokens from one picked swatch by preserving each token's HSL offset from the category's default anchor. Precedence per category: user (when override on) → circuit → user → default.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), Ebiten stub tests (`-tags test -modfile=go.test.mod`), `cmd/gen_design_tokens` generator, `internal/userprefs` (file + localStorage), `internal/hooks` bus, `internal/i18n`.

**Spec:** `docs/superpowers/specs/2026-09-05-theme-editor-design.md`

## Global Constraints

- Go binary: always `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`. Fast tests: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/<pkg>/ -run <Name>`.
- **Never `git add` or `git commit`** in this repo (standing user rule). Every "Commit" step below is replaced by "run `git status` and `git diff --stat` and confirm only the intended files changed". Leave the tree uncommitted.
- Default theme must be the identity: with no theme applied, every existing test, drift test, screenshot, and `design_tokens.gen.go` value is byte-identical to today.
- No new inline `color.RGBA{}`/`color.NRGBA{}` literals in `internal/ui` outside the discipline exemptions (`token_discipline_test.go`). The HSL math file `theme_derive.go` produces colors from math, never from literals.
- No forbidden glyphs (`▶ ▼ ▲ ✕ ≡ ⏸ ■ ↑ ↓ → ←`) in `internal/ui` source. Use `IconID`s.
- Corner radius by role only: `RadiusControl` for chips/pills, `RadiusContainer` for the panel. No pills.
- All log/event tags lowercase. New hooks kinds need: `KindAll` entry, `ActionRegistry` entry, `eventlogger` formatter, `NumNonVerbose` bump.
- Every UI change is verified with a real screenshot that you open and inspect (CLAUDE.md "Visual Verification of UI Work"), desktop and mobile (`-mobile -win-w 390 -win-h 844`).
- Zero per-frame cost added to any Draw path. Theme work happens only on apply.
- Swatch names are the only thing the editor writes; hex appears only in the `tokens` escape hatch.

---

### Task 1: `internal/theme` document package

**Files:**
- Create: `src/go/internal/theme/doc.go`
- Create: `src/go/internal/theme/doc_test.go`

**Interfaces:**
- Produces: `theme.Doc{Categories map[string]string; Tokens map[string]string}`, `(Doc).IsEmpty() bool`, `(Doc).Clone() Doc`, `(Doc).Equal(Doc) bool`, `theme.UserDoc{Doc; OverrideCircuits bool}`.

- [ ] **Step 1: Write the failing test**

```go
// src/go/internal/theme/doc_test.go
package theme

import "testing"

func TestDocIsEmptyAndClone(t *testing.T) {
	var d Doc
	if !d.IsEmpty() {
		t.Fatal("zero Doc must be empty")
	}
	d = Doc{Categories: map[string]string{"accent": "orchid-300"}}
	if d.IsEmpty() {
		t.Fatal("doc with a category is not empty")
	}
	c := d.Clone()
	c.Categories["accent"] = "mint-100"
	if d.Categories["accent"] != "orchid-300" {
		t.Fatal("Clone must deep-copy Categories")
	}
	if !d.Equal(d.Clone()) || d.Equal(c) {
		t.Fatal("Equal must compare map contents")
	}
	e := Doc{Categories: map[string]string{}, Tokens: map[string]string{}}
	if !e.IsEmpty() {
		t.Fatal("doc with only empty maps is empty")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/theme/ -run TestDocIsEmptyAndClone`
Expected: FAIL (package does not exist / `Doc` undefined).

- [ ] **Step 3: Write minimal implementation**

```go
// src/go/internal/theme/doc.go
// Package theme holds the runtime UI color-scheme document. It is a plain
// data package (no Ebiten, no ui import) so userprefs, the project file, and
// the ui package can all carry a Doc without an import cycle.
package theme

// Doc is a config-only UI color scheme.
//
//   - Categories: category id (see ui/theme_categories.go) → swatch NAME from
//     the Vice City grid (e.g. "sunset-gold-300"). Names, never hex.
//   - Tokens: DESIGN.md color token name → "#RRGGBB". Escape hatch for
//     hand-authored config; applied after category derivation.
//
// Both maps are partial and omitempty. An empty Doc is the shipped design.
type Doc struct {
	Categories map[string]string `json:"categories,omitempty"`
	Tokens     map[string]string `json:"tokens,omitempty"`
}

// UserDoc is the per-user profile theme plus the override flag.
type UserDoc struct {
	Doc
	// OverrideCircuits, when true, makes the user's categories win over a
	// circuit's own theme for every category the user has set.
	OverrideCircuits bool `json:"override_circuits,omitempty"`
}

// IsEmpty reports whether the document sets nothing.
func (d Doc) IsEmpty() bool { return len(d.Categories) == 0 && len(d.Tokens) == 0 }

// Clone deep-copies both maps (nil stays nil).
func (d Doc) Clone() Doc {
	var out Doc
	if d.Categories != nil {
		out.Categories = make(map[string]string, len(d.Categories))
		for k, v := range d.Categories {
			out.Categories[k] = v
		}
	}
	if d.Tokens != nil {
		out.Tokens = make(map[string]string, len(d.Tokens))
		for k, v := range d.Tokens {
			out.Tokens[k] = v
		}
	}
	return out
}

// Equal compares map contents (nil and empty are equal).
func (d Doc) Equal(o Doc) bool {
	return eqMap(d.Categories, o.Categories) && eqMap(d.Tokens, o.Tokens)
}

func eqMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/theme/`
Expected: PASS.

- [ ] **Step 5: Check the tree**

Run: `git status --short` — only `src/go/internal/theme/` is new. Do not commit.

---

### Task 2: Generator emits a name map, default snapshot, and a token-referencing component builder

**Files:**
- Modify: `src/go/cmd/gen_design_tokens/main.go` (template at ~line 1099-1105, `emitTokens` at 874)
- Modify: `src/go/cmd/gen_design_tokens/components.go` (`resolveComponent` 253-300, template 405-420)
- Modify: `src/go/cmd/gen_design_tokens/components_test.go`
- Regenerate: `src/go/internal/ui/design_tokens.gen.go`, `src/go/internal/ui/design_components.gen.go`

**Interfaces:**
- Produces (generated, package `ui`): `var genColorByName = map[string]*color.RGBA{...}` (token name → pointer to the live var), `var genColorDefaultByName = map[string]color.RGBA{...}` (immutable defaults), `func buildComponentSpecs() [componentCount]ComponentSpec`, `var componentSpecs = buildComponentSpecs()`, `func rebuildComponentSpecs()`.

- [ ] **Step 1: Write the failing generator test**

Add to `components_test.go`:

```go
func TestComponentEmitReferencesTokenIdents(t *testing.T) {
	r := newTokenResolver([]keyVal{{"primary", "#FF0000"}, {"border", "#FFFFFF"}}, nil, []keyVal{{"control", "4"}}, []keyVal{{"border-thin", "8"}})
	raw := &componentRaw{Category: "button", BackgroundColor: "{colors.primary}", IconColor: "{colors.border}",
		Border: &borderRaw{Color: "{colors.border}", Alpha: "border-thin"}, Rounded: "{rounded.control}"}
	c, err := resolveComponent("button-x", raw, r, 1)
	if err != nil {
		t.Fatal(err)
	}
	if c.FillIdent != "genColorPrimary" || c.BorderIdent != "genColorBorder" || c.IconIdent != "genColorBorder" {
		t.Fatalf("idents: fill=%q border=%q icon=%q", c.FillIdent, c.BorderIdent, c.IconIdent)
	}
}
```

(Use the real struct names from the file: check `borderRaw` — grep `type .*Raw struct` in `components.go` and use the actual name for the border struct.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test ./cmd/gen_design_tokens/ -run TestComponentEmitReferencesTokenIdents`
Expected: FAIL (`FillIdent` undefined).

- [ ] **Step 3: Implement idents in `resolveComponent` and the template**

In `components.go`, add to `type component struct`: `FillIdent, BorderIdent, IconIdent string`. In `resolveComponent`, after each successful `r.color(...)` call, set the ident from the ref key:

```go
	if raw.BackgroundColor != "" {
		rgb, ok := r.color(raw.BackgroundColor)
		if !ok { /* unchanged error */ }
		c.HasFill = true
		c.FillR, c.FillG, c.FillB = rgb[0], rgb[1], rgb[2]
		key, _, _ := parseRef(raw.BackgroundColor)
		c.FillIdent = identForColor(key)
	}
	// same for IconColor → c.IconIdent, Border.Color → c.BorderIdent
```

`identForColor` lives in `main.go` (same package). Then change the template: replace

```
var componentSpecs = [componentCount]ComponentSpec{
```
with
```
// componentSpecs is the generated registry, indexed by ComponentID. It is
// BUILT from the live genColor* token variables so a runtime theme
// (ui/theme_runtime.go ApplyTheme) can rebuild it after overwriting them.
var componentSpecs = buildComponentSpecs()

// rebuildComponentSpecs re-derives the registry from the current token
// variables. Called by ApplyTheme after the genColor* writes.
func rebuildComponentSpecs() { componentSpecs = buildComponentSpecs() }

func buildComponentSpecs() [componentCount]ComponentSpec {
	return [componentCount]ComponentSpec{
```
and the fill/border/icon lines:

```
		Fill:     {{.FillIdent}},
		Border: BorderRef{
			BaseColor: {{.BorderIdent}},
			Alpha:     {{.BorderAlpha}},
		},
		IconColor:    {{.IconIdent}},
```
Close the function with an extra `}` after the array literal's closing brace. Keep `Spec(id)` unchanged (it reads `componentSpecs`).

- [ ] **Step 4: Emit the name map and defaults in `main.go`**

In the tokens template right after the closing `)` of the colors var block (line ~1105) add:

```
// genColorByName maps every DESIGN.md color token name to its LIVE variable so
// the runtime theme engine can overwrite tokens by name. genColorDefaultByName
// is the immutable generation-time value used to restore the default theme.
var genColorByName = map[string]*color.RGBA{
{{- range .Colors}}
	"{{.SrcKey}}": &{{.Ident}},
{{- end}}
}

var genColorDefaultByName = map[string]color.RGBA{
{{- range .Colors}}
	"{{.SrcKey}}": {{"{"}}{{.R}}, {{.G}}, {{.B}}, 255},
{{- end}}
}
```

- [ ] **Step 5: Regenerate and verify identity**

Run: `make gen-design-tokens` then
`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestComponentSpecsDrift|TestDesignMDDrift|TestTokenDiscipline|TestThemeColorsTokenized'`
Expected: PASS. `git diff src/go/internal/ui/design_components.gen.go` shows only ident substitutions and the builder wrapper; `design_tokens.gen.go` gains the two maps.

- [ ] **Step 6: Run the generator tests and the full ui fast suite**

Run: `cd src/go && ../../.tools/go/bin/go test ./cmd/gen_design_tokens/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS.

---

### Task 3: `theme.go` alias block becomes `rebuildThemeAliases()`; fold per-file derived colors

**Files:**
- Modify: `src/go/internal/ui/theme.go:11-271` (the `var (...)` block)
- Modify: `src/go/internal/ui/render_chain.go:18-37`, `render_meters.go:17-21`, `render_waveform.go:19`, `synth_concept_viz.go:145`
- Create: `src/go/internal/ui/theme_rebuild_discipline_test.go`

**Interfaces:**
- Produces: `func rebuildThemeAliases()` (package `ui`), called from `init()` in `theme.go` and by `ApplyTheme` (Task 6).

- [ ] **Step 1: Write the failing AST discipline test**

```go
//go:build test

package ui

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestNoTopLevelDerivedColorsOutsideTheme guards the rebuild-in-place theme
// engine: a package-level var whose initialiser references a genColor* token
// (directly or via WithAlpha/ButtonStyleFromSpec) is evaluated ONCE at init and
// would go stale after ApplyTheme. Every such derivation must live inside
// rebuildThemeAliases() (theme.go) so a theme change re-derives it.
func TestNoTopLevelDerivedColorsOutsideTheme(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	var bad []string
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") || strings.HasSuffix(f, ".gen.go") || f == "theme.go" {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range file.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				for i, v := range vs.Values {
					refs := false
					ast.Inspect(v, func(n ast.Node) bool {
						if id, ok := n.(*ast.Ident); ok && (strings.HasPrefix(id.Name, "genColor") || strings.HasPrefix(id.Name, "col") && len(id.Name) > 3 && id.Name[3] >= 'A' && id.Name[3] <= 'Z') {
							refs = true
						}
						return true
					})
					if refs {
						bad = append(bad, f+": "+vs.Names[i].Name)
					}
				}
			}
		}
	}
	if len(bad) > 0 {
		t.Fatalf("package-level color derivations outside rebuildThemeAliases (move them into theme.go rebuildThemeAliases):\n  %s", strings.Join(bad, "\n  "))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestNoTopLevelDerivedColorsOutsideTheme`
Expected: FAIL listing `render_chain.go: colScopeA`, `render_meters.go: meterLow`, `render_waveform.go: colWaveTraceFill`, `synth_concept_viz.go: colConceptGhost`, etc.

- [ ] **Step 3: Produce the typed declaration list for theme.go**

The body of the `var (...)` block (lines 12-270) is a list of `name = expr` entries, which are valid **assignment statements** inside a function. So the transformation is: (a) declare every name with its type in a `var (...)` block WITHOUT initialisers, (b) move the existing lines verbatim into `func rebuildThemeAliases() { ... }`.

To get the types without guessing, write a throwaway test BEFORE editing:

```go
//go:build test

package ui

import (
	"fmt"
	"testing"
)

func TestPrintThemeVarTypes(t *testing.T) {
	for _, kv := range []struct{ n string; v any }{
		{"colBGTop", colBGTop}, {"colBGBottom", colBGBottom}, {"colGridHorizon", colGridHorizon},
		// ... one entry per name in the block, in file order (copy the names with
		// `grep -oE '^\s+[A-Za-z][A-Za-z0-9]*\s+=' theme.go | awk '{print $1}'`)
	} {
		fmt.Printf("\t%s %T\n", kv.n, kv.v)
	}
}
```

Run it with `-run TestPrintThemeVarTypes -v`, paste the printed lines into the new declaration block, then DELETE the throwaway test. Types will be `color.RGBA`, `color.NRGBA`, `NodeStyle`, `SignalStyle`, `EdgeStyle`, `ButtonStyle`, `TextInputStyle`, `DrumCellStyle`, `map[string]color.Color`, `[]color.Color`, `int`.

- [ ] **Step 4: Restructure theme.go**

```go
// theme.go layout after the change:
var (
	colBGTop        color.RGBA
	colBGBottom     color.RGBA
	// ... every name from Step 3 with its printed type ...
	DrumCellUI      DrumCellStyle

	// Runtime STATE, not theme derivation — stays initialised here and is
	// NEVER touched by rebuildThemeAliases (a theme change must not wipe
	// user-assigned custom instrument colors).
	instColors      = buildInstColors()
	customPalette   = buildCustomPalette()
	customColors    = map[string]color.Color{}
	nextCustomColor int
)

func init() { rebuildThemeAliases() }

// rebuildThemeAliases derives every semantic color alias and every
// spec-backed style from the CURRENT genColor* token variables. Runs once at
// init and again from ApplyTheme (theme_runtime.go) after a theme overwrites
// tokens. Order matters: rebuildComponentSpecs() must have run first, since
// the *StyleFromSpec calls below copy from the spec table.
func rebuildThemeAliases() {
	colBGTop = genColorBackground
	colBGBottom = genColorBackground
	// ... the former initialiser lines, verbatim, comments included ...
	DrumCellUI = DrumCellStyle{On: colStep, Off: colStepOff, Highlight: colHighlight, Border: colStepBorder}

	// ── Folded from render_chain.go / render_meters.go / render_waveform.go /
	// synth_concept_viz.go (Task 3): per-file derivations that used to be
	// package-level initialisers.
	colScopeA = WithAlpha(genColorVizScopeTraceA, 180)
	colScopeB = WithAlpha(genColorVizScopeTraceB, 150)
	colScopeAFill = WithAlpha(genColorVizScopeTraceA, AlphaSubtle)
	colScopeBFill = WithAlpha(genColorVizScopeTraceB, AlphaSubtle)
	colScopeDiff = WithAlpha(genColorVizScopeTraceDiff, 200)
	colScopeDiffFill = WithAlpha(genColorVizScopeTraceDiff, AlphaSubtle)
	colScopeBg = genColorVizScopeBg
	colScopeGrid = WithAlpha(genColorBorder, 20)
	colScopeGridMid = WithAlpha(genColorBorder, 35)
	colScopeTrigger = genColorVizScopeTrigger
	colConceptGhost = WithAlpha(genColorBorder, AlphaSubtle)
	meterLow = genColorVizMeterLow
	meterMid = genColorVizMeterMid
	meterHigh = genColorVizMeterHigh
	meterBg = genColorVizMeterBg
	meterClip = genColorVizMeterClip
	colWaveTraceFill = WithAlpha(colWaveTrace, AlphaSubtle)
}
```

In the four source files, replace each initialised var with a bare typed declaration (e.g. `var colScopeA color.NRGBA` inside their existing `var (...)` blocks; `var colWaveTraceFill color.NRGBA`). Keep the explanatory comments there and add `// derived in rebuildThemeAliases (theme.go)`.

- [ ] **Step 5: Compile, run the discipline test and the whole ui suite**

Run: `cd src/go && ../../.tools/go/bin/go build ./... && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: build OK; `TestNoTopLevelDerivedColorsOutsideTheme` PASS; all other ui tests PASS (identity — nothing changed at runtime). Also run `../../.tools/go/bin/vet`-equivalent: `../../.tools/go/bin/go vet -tags test ./internal/ui/` is optional; the build is the gate.

- [ ] **Step 6: Check the tree**

`git status --short` — theme.go, the four render files, the new test. Do not commit.

---

### Task 4: Category table + completeness guard

**Files:**
- Create: `src/go/internal/ui/theme_categories.go`
- Create: `src/go/internal/ui/theme_categories_test.go`

**Interfaces:**
- Produces: `type ThemeCategory struct{ ID string; LabelKey, GlossKey i18n.Key; Anchor string; KeepLightness bool; Tokens []string }`, `var themeCategories []ThemeCategory` (ordered as in the spec), `func themeCategoryByID(id string) (ThemeCategory, bool)`, `func themeCategoryOfToken(token string) (string, bool)`, `var themeFixedTokens map[string]bool`.
- Consumes: `i18n.Key` constants added in Task 11 — to keep this task compiling on its own, define the i18n keys HERE (Task 11 only adds catalog strings). Add to `src/go/internal/i18n/keys.go` the constants and to `AllKeys()`; catalogs in Task 11.

- [ ] **Step 1: Add the i18n key constants (no catalog strings yet)**

In `src/go/internal/i18n/keys.go` after `KeyLangSpanish`:

```go
	// Theme editor
	KeyThemeTitle        Key = "theme.title"
	KeyThemeScopeMine    Key = "theme.scope.mine"
	KeyThemeScopeCircuit Key = "theme.scope.circuit"
	KeyThemeOverride     Key = "theme.override"
	KeyThemeResetAll     Key = "theme.reset_all"
	KeyThemeReset        Key = "theme.reset"
	KeyThemeCatBackground Key = "theme.cat.background"
	KeyThemeCatSurfaces   Key = "theme.cat.surfaces"
	KeyThemeCatText       Key = "theme.cat.text"
	KeyThemeCatAccent     Key = "theme.cat.accent"
	KeyThemeCatFocus      Key = "theme.cat.focus"
	KeyThemeCatGrid       Key = "theme.cat.grid"
	KeyThemeCatNodes      Key = "theme.cat.nodes"
	KeyThemeCatDividers   Key = "theme.cat.dividers"
	KeyThemeCatViz        Key = "theme.cat.viz"
	KeyThemeGlossBackground Key = "theme.gloss.background"
	KeyThemeGlossSurfaces   Key = "theme.gloss.surfaces"
	KeyThemeGlossText       Key = "theme.gloss.text"
	KeyThemeGlossAccent     Key = "theme.gloss.accent"
	KeyThemeGlossFocus      Key = "theme.gloss.focus"
	KeyThemeGlossGrid       Key = "theme.gloss.grid"
	KeyThemeGlossNodes      Key = "theme.gloss.nodes"
	KeyThemeGlossDividers   Key = "theme.gloss.dividers"
	KeyThemeGlossViz        Key = "theme.gloss.viz"
```

Add all of them to `AllKeys()` (after `KeyLangEnglish, KeyLangSpanish,`). The i18n catalog test will now FAIL until Task 11 adds the strings — run Task 11's catalog step now if you want the i18n package green immediately (it is independent): add the en/es strings from Task 11 Step 3 right away.

- [ ] **Step 2: Write the failing completeness test**

```go
//go:build test

package ui

import (
	"os"
	"regexp"
	"testing"
)

// designMDColorTokenNames parses the `colors:` block of DESIGN.md and returns
// every token key, in order.
func designMDColorTokenNames(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile("../../../../DESIGN.md")
	if err != nil {
		t.Fatal(err)
	}
	src := string(body)
	start := regexp.MustCompile(`(?m)^colors:\s*$`).FindStringIndex(src)
	if start == nil {
		t.Fatal("colors: block not found")
	}
	rest := src[start[1]:]
	end := regexp.MustCompile(`(?m)^[a-zA-Z]`).FindStringIndex(rest)
	if end != nil {
		rest = rest[:end[0]]
	}
	var names []string
	for _, m := range regexp.MustCompile(`(?m)^  ([a-z][a-z0-9-]*):`).FindAllStringSubmatch(rest, -1) {
		names = append(names, m[1])
	}
	if len(names) < 100 {
		t.Fatalf("parsed only %d color tokens; parser broken?", len(names))
	}
	return names
}

func TestThemeCategoryTableComplete(t *testing.T) {
	seen := map[string]string{}
	for _, c := range themeCategories {
		if _, ok := genColorByName[c.Anchor]; !ok && !isSwatchName(c.Anchor) {
			t.Errorf("category %q anchor %q is not a swatch name", c.ID, c.Anchor)
		}
		for _, tok := range c.Tokens {
			if _, ok := genColorByName[tok]; !ok {
				t.Errorf("category %q lists unknown token %q", c.ID, tok)
			}
			if prev, dup := seen[tok]; dup {
				t.Errorf("token %q in both %q and %q", tok, prev, c.ID)
			}
			seen[tok] = c.ID
			if themeFixedTokens[tok] {
				t.Errorf("token %q is both fixed and in category %q", tok, c.ID)
			}
		}
	}
	for _, name := range designMDColorTokenNames(t) {
		if _, ok := seen[name]; ok {
			continue
		}
		if themeFixedTokens[name] || isRawPaletteToken(name) {
			continue
		}
		t.Errorf("DESIGN.md color token %q is neither in a theme category nor in themeFixedTokens — classify it in theme_categories.go", name)
	}
	for tok := range themeFixedTokens {
		if _, ok := genColorByName[tok]; !ok {
			t.Errorf("themeFixedTokens names unknown token %q", tok)
		}
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestThemeCategoryTableComplete`
Expected: FAIL (`themeCategories` undefined).

- [ ] **Step 4: Write the table**

```go
// src/go/internal/ui/theme_categories.go
package ui

import (
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// ThemeCategory groups DESIGN.md color tokens that recolor together from ONE
// picked swatch. This table is the single source of truth for the Theme
// overlay rows, the derivation engine (theme_derive.go), and the guard
// TestThemeCategoryTableComplete (every color token is in exactly one
// category or in themeFixedTokens).
type ThemeCategory struct {
	ID       string
	LabelKey i18n.Key
	GlossKey i18n.Key
	// Anchor is the swatch NAME whose HSL the default token values are
	// measured against. Applying the anchor reproduces DESIGN.md exactly.
	Anchor string
	// Dark categories keep each token's absolute lightness (the shade column
	// only chooses tint strength) so a light pick never yields a light UI.
	Dark   bool
	Tokens []string
}

var themeCategories = []ThemeCategory{
	{ID: "background", LabelKey: i18n.KeyThemeCatBackground, GlossKey: i18n.KeyThemeGlossBackground, Anchor: "orchid-500", KeepLightness: true, Tokens: []string{
		"background", "grid-horizon", "transport-bar-bg", "timeline-total-bg", "viz-bg", "viz-scope-bg", "viz-meter-bg",
		"drum-stripe-even", "drum-stripe-odd", "drum-cell-off", "eq-readout-bg", "sidebar-badge-bg", "viz-pill-fill"}},
	{ID: "surfaces", LabelKey: i18n.KeyThemeCatSurfaces, GlossKey: i18n.KeyThemeGlossSurfaces, Anchor: "orchid-500", KeepLightness: true, Tokens: []string{
		"surface-1", "surface-2", "surface-3", "surface-overlay", "disabled-fill", "fx-active-fill",
		"sidebar-section-bg", "sidebar-chip-fill", "slider-track-fill", "scope-label-dim"}},
	{ID: "text", LabelKey: i18n.KeyThemeCatText, GlossKey: i18n.KeyThemeGlossText, Anchor: "orchid-100", KeepLightness: true, Tokens: []string{
		"on-surface", "on-surface-muted", "on-surface-disabled", "popup-text-secondary", "slider-thumb-fill",
		"sidebar-swatch-fallback", "row-rack-color-fallback", "drum-cell-highlight", "drum-mute-highlight", "divider-base"}},
	{ID: "accent", LabelKey: i18n.KeyThemeCatAccent, GlossKey: i18n.KeyThemeGlossAccent, Anchor: "sunset-gold-300", Tokens: []string{
		"primary", "primary-bright", "primary-dim", "on-surface-accent", "splitter-handle", "drum-glow", "slider-fill",
		"solo-active-border", "eq-filter-active-fill", "eq-filter-active-border", "viz-glow"}},
	{ID: "focus", LabelKey: i18n.KeyThemeCatFocus, GlossKey: i18n.KeyThemeGlossFocus, Anchor: "hot-pink-300", Tokens: []string{"focus-ring"}},
	{ID: "grid", LabelKey: i18n.KeyThemeCatGrid, GlossKey: i18n.KeyThemeGlossGrid, Anchor: "orchid-500", KeepLightness: true, Tokens: []string{
		"grid-line", "grid-half", "grid-quarter", "grid-eighth", "grid-sixteenth", "grid-thirty-second"}},
	{ID: "nodes", LabelKey: i18n.KeyThemeCatNodes, GlossKey: i18n.KeyThemeGlossNodes, Anchor: "orchid-500", KeepLightness: true, Tokens: []string{
		"node-fill", "node-border", "edge-color", "drum-mute-cell", "timeline-beat"}},
	{ID: "dividers", LabelKey: i18n.KeyThemeCatDividers, GlossKey: i18n.KeyThemeGlossDividers, Anchor: "orchid-100", KeepLightness: true, Tokens: []string{
		"border", "divider-shadow", "divider-highlight", "sidebar-chip-border", "viz-pill-border", "eq-zero-line", "wave-trace-dry"}},
	{ID: "viz", LabelKey: i18n.KeyThemeCatViz, GlossKey: i18n.KeyThemeGlossViz, Anchor: "sunset-gold-300", Tokens: []string{
		"viz-bar", "viz-bar-peak", "viz-curve", "viz-wave-a", "viz-wave-b", "viz-scope-trace-a", "viz-scope-trace-b",
		"viz-scope-trace-diff", "viz-scope-trigger", "viz-spectrum-peak-marker", "viz-meter-low", "viz-meter-mid",
		"viz-bass", "viz-mids", "viz-treble", "timeline-view", "timeline-view-hi", "timeline-cursor"}},
}

// themeFixedTokens are semantic-state and debug colors that a theme never
// touches (red must stay "danger", green must stay "go").
var themeFixedTokens = map[string]bool{
	"success": true, "error": true, "error-text": true, "mute": true, "mute-active-border": true,
	"destructive": true, "destructive-border": true, "destructive-confirm-fill": true, "destructive-confirm-border": true,
	"record-idle": true, "record-active": true, "play-desktop-fill": true, "play-desktop-border": true,
	"stop-desktop-fill": true, "stop-desktop-border": true, "eq-mute-active-fill": true, "menu-delete-tint": true,
	"viz-meter-high": true, "viz-meter-clip": true, "viz-debug-edge": true, "viz-debug-node": true, "dim-black": true,
}

// isRawPaletteToken reports whether name is one of the raw chromatic ramp /
// night-neutral entries (the palette itself, not a role). They are the
// SOURCE swatches and are never themed.
func isRawPaletteToken(name string) bool {
	if strings.HasPrefix(name, "night-") {
		return true
	}
	for _, fam := range []string{"sunset-gold-", "tangerine-", "hot-pink-", "orchid-", "electric-blue-", "cyan-", "mint-"} {
		if strings.HasPrefix(name, fam) {
			return true
		}
	}
	return false
}

// isSwatchName reports whether name is one of the curated picker swatches.
func isSwatchName(name string) bool {
	for _, s := range SuggestedInstrumentSwatches {
		if s.Name == name {
			return true
		}
	}
	return false
}

func themeCategoryByID(id string) (ThemeCategory, bool) {
	for _, c := range themeCategories {
		if c.ID == id {
			return c, true
		}
	}
	return ThemeCategory{}, false
}

// themeCategoryOfToken returns the category id owning token ("" , false for
// fixed / raw / unknown tokens).
func themeCategoryOfToken(token string) (string, bool) {
	for _, c := range themeCategories {
		for _, t := range c.Tokens {
			if t == token {
				return c.ID, true
			}
		}
	}
	return "", false
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestThemeCategoryTableComplete`
Expected: PASS. If it lists an unclassified token, add it to the table (the spec's membership list is the guide; anything semantic-red/green goes to fixed).

- [ ] **Step 6: Check the tree**

`git status --short`. Do not commit.

---

### Task 5: Derivation engine (`theme_derive.go`)

**Files:**
- Create: `src/go/internal/ui/theme_derive.go`
- Create: `src/go/internal/ui/theme_derive_test.go`
- Modify: `src/go/internal/ui/token_discipline_test.go` — no literal budget needed (the file has no color literals); if `TestTokenDiscipline` flags it, the implementation used a literal — remove the literal instead.

**Interfaces:**
- Consumes: `themeCategories`, `genColorDefaultByName`, `SuggestedInstrumentSwatches` (`[]GenInstrumentSwatch{Name string; RGBA color.RGBA; Hex string}`), `theme.Doc`.
- Produces: `func resolveThemeTokens(doc theme.Doc) (map[string]color.RGBA, []string)` — full token map (every DESIGN.md color token name → RGBA after defaults + category derivation + explicit tokens) plus human-readable warnings for ignored entries; `func swatchRGBA(name string) (color.RGBA, bool)`; `func deriveCategory(cat ThemeCategory, pick color.RGBA, pickName string, out map[string]color.RGBA)`; `func parseHexRGB6(s string) (color.RGBA, bool)`.

- [ ] **Step 1: Write the failing tests**

```go
//go:build test

package ui

import (
	"image/color"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// TestThemeDefaultIsIdentity: an empty doc AND a doc that picks every
// category's own anchor both reproduce DESIGN.md byte-for-byte.
func TestThemeDefaultIsIdentity(t *testing.T) {
	got, warns := resolveThemeTokens(theme.Doc{})
	if len(warns) != 0 {
		t.Fatalf("empty doc produced warnings: %v", warns)
	}
	for name, want := range genColorDefaultByName {
		if got[name] != want {
			t.Errorf("empty doc: %s = %v, want default %v", name, got[name], want)
		}
	}
	anchors := theme.Doc{Categories: map[string]string{}}
	for _, c := range themeCategories {
		anchors.Categories[c.ID] = c.Anchor
	}
	got, warns = resolveThemeTokens(anchors)
	if len(warns) != 0 {
		t.Fatalf("anchor doc produced warnings: %v", warns)
	}
	for name, want := range genColorDefaultByName {
		if got[name] != want {
			t.Errorf("anchor doc: %s = %v, want default %v (derivation is not the identity on the anchor)", name, got[name], want)
		}
	}
}

func lightness(c color.RGBA) float64 { _, _, l := rgbToHSL(c); return l }

// TestThemeDarkCategoriesPreserveLadder: with a LIGHT pick, every
// KeepLightness category token keeps its default lightness, so the surface
// ladder ordering survives and the UI stays dark.
func TestThemeDarkCategoriesPreserveLadder(t *testing.T) {
	doc := theme.Doc{Categories: map[string]string{"background": "mint-100", "surfaces": "mint-100", "grid": "cyan-100", "nodes": "tangerine-100", "dividers": "electric-blue-100", "text": "sunset-gold-500"}}
	got, warns := resolveThemeTokens(doc)
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	for _, c := range themeCategories {
		if !c.KeepLightness {
			continue
		}
		for _, tok := range c.Tokens {
			if dl, gl := lightness(genColorDefaultByName[tok]), lightness(got[tok]); dl < gl-0.02 || dl > gl+0.02 {
				t.Errorf("%s/%s lightness %.3f drifted from default %.3f", c.ID, tok, gl, dl)
			}
		}
	}
	if !(lightness(got["background"]) < lightness(got["surface-1"]) && lightness(got["surface-1"]) < lightness(got["surface-2"]) && lightness(got["surface-2"]) < lightness(got["surface-3"])) {
		t.Fatalf("surface ladder broken: bg=%v s1=%v s2=%v s3=%v", got["background"], got["surface-1"], got["surface-2"], got["surface-3"])
	}
	// Hue actually moved toward the pick for a chromatic token.
	h0, _, _ := rgbToHSL(genColorDefaultByName["surface-2"])
	h1, _, _ := rgbToHSL(got["surface-2"])
	if hueDistance(h0, h1) < 30 {
		t.Fatalf("surface-2 hue did not follow the mint pick: %.0f → %.0f", h0, h1)
	}
}

// TestThemeAccentFollowsPick: a non-dark category adopts the pick's HSL and
// keeps the ladder offsets (bright lighter than primary, dim darker).
func TestThemeAccentFollowsPick(t *testing.T) {
	got, _ := resolveThemeTokens(theme.Doc{Categories: map[string]string{"accent": "orchid-300"}})
	pick, _ := swatchRGBA("orchid-300")
	if got["primary"] != pick {
		t.Fatalf("primary = %v, want the picked swatch %v", got["primary"], pick)
	}
	if !(lightness(got["primary-bright"]) > lightness(got["primary"]) && lightness(got["primary-dim"]) < lightness(got["primary"])) {
		t.Fatalf("accent ladder offsets lost: bright=%v primary=%v dim=%v", got["primary-bright"], got["primary"], got["primary-dim"])
	}
	// Untouched category stays default.
	if got["background"] != genColorDefaultByName["background"] {
		t.Fatal("background changed although only accent was picked")
	}
}

func TestThemeAchromaticStaysUnchanged(t *testing.T) {
	got, _ := resolveThemeTokens(theme.Doc{Categories: map[string]string{"dividers": "mint-300"}})
	if got["border"] != genColorDefaultByName["border"] || got["divider-shadow"] != genColorDefaultByName["divider-shadow"] {
		t.Fatalf("achromatic tokens must not be recolored: border=%v shadow=%v", got["border"], got["divider-shadow"])
	}
	if got["divider-highlight"] == genColorDefaultByName["divider-highlight"] {
		t.Fatal("chromatic divider-highlight should follow the pick")
	}
}

func TestThemeTokensOverrideAfterCategories(t *testing.T) {
	got, warns := resolveThemeTokens(theme.Doc{
		Categories: map[string]string{"accent": "orchid-300"},
		Tokens:     map[string]string{"primary": "#112233"},
	})
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	if got["primary"] != (color.RGBA{0x11, 0x22, 0x33, 255}) {
		t.Fatalf("explicit token must win over category derivation: %v", got["primary"])
	}
}

func TestThemeRejectsFixedAndUnknownNames(t *testing.T) {
	_, warns := resolveThemeTokens(theme.Doc{
		Categories: map[string]string{"accent": "no-such-swatch", "nope": "mint-100"},
		Tokens:     map[string]string{"error": "#000000", "not-a-token": "#000000", "primary": "zzz"},
	})
	if len(warns) != 5 {
		t.Fatalf("want 5 warnings (bad swatch, bad category, fixed token, unknown token, bad hex), got %d: %v", len(warns), warns)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTheme(DefaultIsIdentity|DarkCategoriesPreserveLadder|AccentFollowsPick|AchromaticStaysUnchanged|TokensOverrideAfterCategories|RejectsFixedAndUnknownNames)'`
Expected: FAIL (`resolveThemeTokens` undefined).

- [ ] **Step 3: Implement**

```go
// src/go/internal/ui/theme_derive.go
package ui

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// rgbToHSL converts to hue [0,360), saturation [0,1], lightness [0,1].
func rgbToHSL(c color.RGBA) (h, s, l float64) {
	r, g, b := float64(c.R)/255, float64(c.G)/255, float64(c.B)/255
	max, min := math.Max(r, math.Max(g, b)), math.Min(r, math.Min(g, b))
	l = (max + min) / 2
	d := max - min
	if d < 1e-9 {
		return 0, 0, l
	}
	if l > 0.5 {
		s = d / (2 - max - min)
	} else {
		s = d / (max + min)
	}
	switch max {
	case r:
		h = (g - b) / d
		if g < b {
			h += 6
		}
	case g:
		h = (b-r)/d + 2
	default:
		h = (r-g)/d + 4
	}
	return h * 60, s, l
}

func hslToRGB(h, s, l float64) color.RGBA {
	h = math.Mod(h, 360)
	if h < 0 {
		h += 360
	}
	s = clamp01(s)
	l = clamp01(l)
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	to8 := func(v float64) uint8 { return uint8(math.Round(clamp01(v+m) * 255)) }
	return color.RGBA{to8(r), to8(g), to8(b), 255}
}

func clamp01(v float64) float64 { return math.Max(0, math.Min(1, v)) }

// hueDistance is the shortest angular distance between two hues.
func hueDistance(a, b float64) float64 {
	d := math.Abs(math.Mod(a-b+540, 360) - 180)
	return d
}

// achromaticSat is the saturation below which a token is treated as grey /
// white / black and left exactly as designed (a hue is meaningless there).
const achromaticSat = 0.02

// swatchRGBA looks a curated swatch up by name.
func swatchRGBA(name string) (color.RGBA, bool) {
	for _, s := range SuggestedInstrumentSwatches {
		if s.Name == name {
			return s.RGBA, true
		}
	}
	return color.RGBA{}, false
}

// swatchShadeStrength maps the shade column (-100..-500) of a swatch name to
// a tint-strength multiplier for KeepLightness categories. The anchor of
// every dark category is a -500 swatch, so 1.0 there keeps the identity.
func swatchShadeStrength(name string) float64 {
	i := strings.LastIndex(name, "-")
	if i < 0 {
		return 1
	}
	n, err := strconv.Atoi(name[i+1:])
	if err != nil {
		return 1
	}
	switch n {
	case 100:
		return 0.35
	case 200:
		return 0.55
	case 300:
		return 0.75
	case 400:
		return 0.9
	default:
		return 1
	}
}

// deriveCategory writes the category's member tokens into out, derived from
// pick. Every member keeps its HSL OFFSET from the category anchor; the
// anchor pick is therefore the identity.
//
//   - chromatic, non-KeepLightness: h = h(pick)+Δh, s = s(pick)+Δs, l = l(pick)+Δl
//   - chromatic, KeepLightness:     h = h(pick)+Δh, l = l(default),
//     s = s(default) · (s(pick)·strength(pick)) / (s(anchor)·strength(anchor))
//   - achromatic (s(default) < achromaticSat): unchanged.
func deriveCategory(cat ThemeCategory, pick color.RGBA, pickName string, out map[string]color.RGBA) {
	anchor, ok := swatchRGBA(cat.Anchor)
	if !ok {
		return
	}
	ah, as, al := rgbToHSL(anchor)
	ph, ps, pl := rgbToHSL(pick)
	for _, tok := range cat.Tokens {
		def := genColorDefaultByName[tok]
		dh, ds, dl := rgbToHSL(def)
		if ds < achromaticSat {
			out[tok] = def
			continue
		}
		h := ph + (dh - ah)
		if cat.KeepLightness {
			s := ds
			if denom := as * swatchShadeStrength(cat.Anchor); denom > 1e-6 {
				s = ds * (ps * swatchShadeStrength(pickName)) / denom
			}
			out[tok] = hslToRGB(h, s, dl)
			continue
		}
		out[tok] = hslToRGB(h, ps+(ds-as), pl+(dl-al))
	}
}

// parseHexRGB6 parses "#RRGGBB" (case-insensitive).
func parseHexRGB6(s string) (color.RGBA, bool) {
	if len(s) != 7 || s[0] != '#' {
		return color.RGBA{}, false
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, true
}

// resolveThemeTokens turns a document into the full token map: defaults,
// then category derivation, then explicit tokens. Entries that cannot be
// applied are skipped and reported in warnings (never fatal — a theme comes
// from a user file).
func resolveThemeTokens(doc theme.Doc) (map[string]color.RGBA, []string) {
	out := make(map[string]color.RGBA, len(genColorDefaultByName))
	for k, v := range genColorDefaultByName {
		out[k] = v
	}
	var warns []string
	for _, cat := range themeCategories { // table order → deterministic
		name, ok := doc.Categories[cat.ID]
		if !ok {
			continue
		}
		pick, ok := swatchRGBA(name)
		if !ok {
			warns = append(warns, fmt.Sprintf("theme: category %q: unknown swatch %q ignored", cat.ID, name))
			continue
		}
		deriveCategory(cat, pick, name, out)
	}
	for id := range doc.Categories {
		if _, ok := themeCategoryByID(id); !ok {
			warns = append(warns, fmt.Sprintf("theme: unknown category %q ignored", id))
		}
	}
	for tok, hex := range doc.Tokens {
		if _, ok := genColorDefaultByName[tok]; !ok {
			warns = append(warns, fmt.Sprintf("theme: unknown token %q ignored", tok))
			continue
		}
		if themeFixedTokens[tok] || isRawPaletteToken(tok) {
			warns = append(warns, fmt.Sprintf("theme: token %q is fixed and cannot be themed", tok))
			continue
		}
		c, ok := parseHexRGB6(hex)
		if !ok {
			warns = append(warns, fmt.Sprintf("theme: token %q: bad hex %q ignored", tok, hex))
			continue
		}
		out[tok] = c
	}
	return out, warns
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run the same command as Step 2. Expected: PASS. If `TestThemeDefaultIsIdentity` fails on a token by ±1 in one channel, the HSL round trip is lossy for that value: make `deriveCategory` short-circuit `if pick == anchor { out[tok] = def; continue }` — this keeps the identity exact and is documented as such in the comment.

- [ ] **Step 5: Run the discipline tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTokenDiscipline|TestThemeColorsTokenized|TestNoPillShapedControls'`
Expected: PASS.

---

### Task 6: Runtime apply (`theme_runtime.go`) + live icon color on spec buttons

**Files:**
- Create: `src/go/internal/ui/theme_runtime.go`
- Create: `src/go/internal/ui/theme_runtime_test.go`
- Modify: `src/go/internal/ui/uigrid.go:162-172` (SetSpec) and `:305-315` (icon color at draw)

**Interfaces:**
- Produces: `func ApplyTheme(doc theme.Doc)`, `func ActiveTheme() theme.Doc`, `func ThemeGeneration() int`, `func OnThemeChange(fn func()) (cancel func())`, `func SetThemeForTest(doc theme.Doc) (restore func())`, `var themeWarnf = func(string, ...any) {}` (Game points it at its logger's Warnf).
- Consumes: `resolveThemeTokens`, `genColorByName`, `rebuildComponentSpecs`, `rebuildThemeAliases`.

- [ ] **Step 1: Write the failing tests**

```go
//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func TestApplyThemeRewritesTokensSpecsAndAliases(t *testing.T) {
	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"accent": "orchid-300"}})
	defer restore()
	pick, _ := swatchRGBA("orchid-300")
	if genColorPrimary != pick {
		t.Fatalf("genColorPrimary = %v, want %v", genColorPrimary, pick)
	}
	if colAccent != pick || SignalUI.Color != pick {
		t.Fatalf("aliases not rebuilt: colAccent=%v SignalUI=%v", colAccent, SignalUI.Color)
	}
	if Spec(ComponentButtonPrimary).Fill != pick {
		t.Fatalf("component specs not rebuilt: %v", Spec(ComponentButtonPrimary).Fill)
	}
	if FABStyle.Fill != pick {
		t.Fatalf("spec-derived style not rebuilt: %v", FABStyle.Fill)
	}
	restore()
	if genColorPrimary != genColorDefaultByName["primary"] || Spec(ComponentButtonPrimary).Fill != genColorDefaultByName["primary"] {
		t.Fatal("restore did not return to defaults")
	}
}

func TestApplyThemeNotifiesListenersAndBumpsGeneration(t *testing.T) {
	calls := 0
	cancel := OnThemeChange(func() { calls++ })
	defer cancel()
	gen := ThemeGeneration()
	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"focus": "mint-300"}})
	defer restore()
	if calls != 1 || ThemeGeneration() != gen+1 {
		t.Fatalf("calls=%d gen %d→%d", calls, gen, ThemeGeneration())
	}
	if !ActiveTheme().Equal(theme.Doc{Categories: map[string]string{"focus": "mint-300"}}) {
		t.Fatalf("ActiveTheme = %+v", ActiveTheme())
	}
}

// A spec button whose icon color came from its spec must follow the theme;
// one whose call site overrode IconColor must keep the override.
func TestSpecButtonIconColorFollowsTheme(t *testing.T) {
	b := NewSpecButton("", ComponentButtonSecondary, nil) // iconColor: on-surface-muted
	b.Icon = string(IconSettings)
	override := NewSpecButton("", ComponentButtonSecondary, nil)
	override.Icon = string(IconSettings)
	override.IconColor = color.RGBA{1, 2, 3, 255}
	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"text": "mint-300"}})
	defer restore()
	want := Spec(ComponentButtonSecondary).IconColor
	if want == genColorDefaultByName["on-surface-muted"] {
		t.Fatal("test precondition: theme did not change on-surface-muted")
	}
	if got := b.effectiveIconColor(); got != want {
		t.Fatalf("spec-seeded icon color = %v, want themed %v", got, want)
	}
	if got := override.effectiveIconColor(); got != (color.RGBA{1, 2, 3, 255}) {
		t.Fatalf("call-site override lost: %v", got)
	}
	_ = ebiten.NewImage(8, 8)
	_ = image.Rect(0, 0, 8, 8)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestApplyTheme|TestSpecButtonIconColorFollowsTheme'`
Expected: FAIL (`SetThemeForTest` undefined).

- [ ] **Step 3: Implement the runtime**

```go
// src/go/internal/ui/theme_runtime.go
package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// Runtime theme engine — rebuild-in-place.
//
// The generated genColor* variables ARE the live palette. ApplyTheme resolves
// a document to a full token map, overwrites the variables by name, re-runs
// the two derivation steps that otherwise happen once at init (component
// specs, then theme.go aliases/styles), and notifies listeners so caches that
// baked colors can invalidate (Game.OnThemeChanged). Draw sites keep reading
// the same variables: a theme costs nothing per frame.
//
// Always call ApplyTheme from the UI goroutine (it mutates package state read
// by Draw).
var (
	themeMu         sync.Mutex
	themeActive     theme.Doc
	themeGen        int
	themeListeners  []*func()
	themeWarnf      = func(string, ...any) {} // Game rebinds to its logger
)

// ApplyTheme makes doc the active theme. Unknown or fixed entries are skipped
// and logged through themeWarnf.
func ApplyTheme(doc theme.Doc) {
	tokens, warns := resolveThemeTokens(doc)
	for _, w := range warns {
		themeWarnf("[theme] %s", w)
	}
	themeMu.Lock()
	for name, c := range tokens {
		if p, ok := genColorByName[name]; ok {
			*p = c
		}
	}
	rebuildComponentSpecs()
	rebuildThemeAliases()
	themeActive = doc.Clone()
	themeGen++
	listeners := append([]*func(){}, themeListeners...)
	themeMu.Unlock()
	for _, fn := range listeners {
		(*fn)()
	}
}

// ActiveTheme returns a copy of the last applied document.
func ActiveTheme() theme.Doc {
	themeMu.Lock()
	defer themeMu.Unlock()
	return themeActive.Clone()
}

// ThemeGeneration increments on every ApplyTheme (cache keys may include it).
func ThemeGeneration() int {
	themeMu.Lock()
	defer themeMu.Unlock()
	return themeGen
}

// OnThemeChange registers fn to run after every ApplyTheme; cancel removes it.
func OnThemeChange(fn func()) (cancel func()) {
	p := &fn
	themeMu.Lock()
	themeListeners = append(themeListeners, p)
	themeMu.Unlock()
	return func() {
		themeMu.Lock()
		defer themeMu.Unlock()
		for i, q := range themeListeners {
			if q == p {
				themeListeners = append(themeListeners[:i], themeListeners[i+1:]...)
				return
			}
		}
	}
}

// SetThemeForTest applies doc and returns a restore func that reapplies the
// previous document. Tests must call restore so the package stays on the
// default palette for the drift/discipline suites.
func SetThemeForTest(doc theme.Doc) (restore func()) {
	prev := ActiveTheme()
	ApplyTheme(doc)
	return func() { ApplyTheme(prev) }
}
```

- [ ] **Step 4: Live icon color on spec buttons**

In `uigrid.go`, add fields to `Button`: `specIconSeed color.Color` (the value SetSpec seeded). In `SetSpec`:

```go
	if spec := Spec(id); spec.HasIconColor {
		b.IconColor = spec.IconColor
		b.specIconSeed = spec.IconColor
	}
```

Add a method and use it in Draw where `col := b.IconColor` currently is:

```go
// effectiveIconColor returns the icon tint: a call-site override wins; an
// icon color seeded from the spec is re-read live so a runtime theme
// (ApplyTheme rebuilds the spec table) recolors existing buttons.
func (b *Button) effectiveIconColor() color.Color {
	if b.hasSpecID && b.specIconSeed != nil && b.IconColor == b.specIconSeed {
		if spec := Spec(b.SpecID); spec.HasIconColor {
			return spec.IconColor
		}
	}
	return b.IconColor
}
```

In Draw: `col := b.effectiveIconColor()` (keep the following nil/Disabled/toggled logic unchanged).

- [ ] **Step 5: Run the tests**

Run: same as Step 2, then the whole ui suite: `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS everywhere (no theme is applied outside these tests).

---

### Task 7: `Game.OnThemeChanged` cache invalidation + repaint proof

**Files:**
- Create: `src/go/internal/ui/game_theme.go`
- Modify: `src/go/internal/ui/game_new.go:142` (register beside `i18n.OnChange`), `src/go/internal/ui/game_struct.go` (add `themeCancel func()`), `CloseForTest` (call cancel — find it with `grep -n 'i18nCancel' *.go`)
- Create: `src/go/internal/ui/theme_repaint_test.go`

**Interfaces:**
- Produces: `func (g *Game) OnThemeChanged()`.
- Consumes: `OnThemeChange`, `(*DrumView).invalidateRowCaches`, `invalidateLabelCaches`, `markRowControlsDirty`, zone `Invalidate()`s, `gridRenderCache` fields.

- [ ] **Step 1: Write the failing repaint test**

```go
//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// TestThemeApplyRepaintsEverySurface proves a theme change reaches the
// screen on the very next frame through every cache: the OLD accent and
// OLD background must vanish from the frame's fills and the DERIVED ones
// must appear (transport buttons, grid tile, drum cells, panels).
func TestThemeApplyRepaintsEverySurface(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1280, 720)
	advanceFrames(g, 3)

	oldSafe := envRenderSafe
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = oldSafe })

	screen := ebiten.NewImage(1280, 720)
	before := collectAllFills(t, func() { g.Draw(screen) })
	oldPrimary, oldBG := genColorDefaultByName["primary"], genColorDefaultByName["background"]
	if rectsWithColorInside(before, screen.Bounds(), oldPrimary) == 0 || rectsWithColorInside(before, screen.Bounds(), oldBG) == 0 {
		t.Fatal("precondition: default accent/background not painted")
	}

	restore := SetThemeForTest(theme.Doc{Categories: map[string]string{"accent": "orchid-300", "background": "electric-blue-500"}})
	t.Cleanup(restore)
	advanceFrames(g, 2)
	after := collectAllFills(t, func() { g.Draw(screen) })

	newPrimary, newBG := genColorPrimary, genColorBackground
	if newPrimary == oldPrimary || newBG == oldBG {
		t.Fatal("precondition: theme did not change the tokens")
	}
	if n := rectsWithColorInside(after, screen.Bounds(), newPrimary); n == 0 {
		t.Error("derived accent never painted after ApplyTheme (spec/alias rebuild or a cache missed)")
	}
	if n := rectsWithColorInside(after, screen.Bounds(), newBG); n == 0 {
		t.Error("derived background never painted after ApplyTheme")
	}
	if n := rectsWithColorInside(after, screen.Bounds(), oldPrimary); n != 0 {
		t.Errorf("stale default accent still painted %d times after ApplyTheme — a cache was not invalidated", n)
	}
	if n := rectsWithColorInside(after, screen.Bounds(), oldBG); n != 0 {
		t.Errorf("stale default background still painted %d times after ApplyTheme", n)
	}
}
```

`collectFilledRects` (render_verify_test.go:42) intercepts only `drawRect`; button fills go through `drawRoundedRect`. Add this sibling to `theme_repaint_test.go` and use it in place of `collectFilledRects` in the test above:

```go
// collectAllFills records every filled drawRect AND drawRoundedRect call
// made while fn runs (button keycaps are rounded rects).
func collectAllFills(t *testing.T, fn func()) []drawnRect {
	t.Helper()
	var rects []drawnRect
	origRect, origRound := drawRect, drawRoundedRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			rects = append(rects, drawnRect{Rect: r, Color: color.RGBAModel.Convert(c).(color.RGBA)})
		}
		origRect(dst, r, c, filled)
	}
	drawRoundedRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, radius int, filled bool) {
		if filled {
			rects = append(rects, drawnRect{Rect: r, Color: color.RGBAModel.Convert(c).(color.RGBA)})
		}
		origRound(dst, r, c, radius, filled)
	}
	defer func() { drawRect, drawRoundedRect = origRect, origRound }()
	fn()
	return rects
}
```
(`drawnRect` is defined in render_verify_test.go. Add `"image"` and `"image/color"` imports.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestThemeApplyRepaintsEverySurface`
Expected: FAIL on "stale default … still painted" (caches) or compile error if `collectAllFills` is missing.

- [ ] **Step 3: Implement the hook**

```go
// src/go/internal/ui/game_theme.go
package ui

// OnThemeChanged is the whole-Game theme chokepoint, mirroring
// DrumView.OnLocaleChanged: every cache that BAKES a color is marked dirty so
// the next Draw rebuilds it from the freshly rebuilt palette. Cheap — flags
// only; the rebuild burst on the following frame is the same as a resize.
// Registered with OnThemeChange in New(); runs on the UI goroutine.
func (g *Game) OnThemeChanged() {
	if g == nil {
		return
	}
	// Grid pane: tile (grid lines), node layer + sprites (node fill/border),
	// edge cache (edge color). Sprites keyed by color simply miss; clearing
	// the map frees the stale textures.
	g.gridTileSubSig = 0
	g.gridTile = nil
	g.nodeLayerDirty = true
	g.nodeLayerGraphSig = 0
	g.nodeSpriteCache = map[spriteKey]*ebiten.Image{}
	g.edgeCacheColorSig = 0
	g.edgesDirty = true
	if dv := g.drum; dv != nil {
		dv.invalidateRowCaches()
		dv.invalidateLabelCaches()
		dv.markRowControlsDirty()
		if dv.eqPanelZone != nil {
			dv.eqPanelZone.Invalidate()
			if cz := dv.eqPanelZone.ChainZone(); cz != nil {
				cz.Invalidate()
			}
		}
		if dv.rowRackZone != nil {
			dv.rowRackZone.Invalidate()
		}
		if dv.transportZone != nil {
			dv.transportZone.Invalidate()
		}
		if dv.timelineZone != nil {
			dv.timelineZone.Invalidate()
			dv.timelineZone.InvalidateRowCaches()
		}
		if dv.synthPanel != nil {
			dv.synthPanel.sprites.releaseAll()
		}
	}
}
```

(Use the real field names for the transport/timeline zones on DrumView — `grep -n 'transportZone\|timelineZone' drumview.go game_ui_types.go`.) Add `"github.com/hajimehoshi/ebiten/v2"` import. In `game_new.go` next to the i18n registration:

```go
	g.themeCancel = OnThemeChange(g.OnThemeChanged)
	themeWarnf = logger.Warnf
```

and in `CloseForTest` (beside `g.i18nCancel()`): `if g.themeCancel != nil { g.themeCancel() }`.

- [ ] **Step 4: Iterate until the repaint test passes**

Run Step 2's command. If a stale color survives, find its draw site (`grep -rn '<colName>' *.go`) and the cache it goes through; add that cache's dirty flag to `OnThemeChanged`. Keep going until PASS. Then run the full ui suite.

---

### Task 8: Precedence + Game theme state

**Files:**
- Create: `src/go/internal/ui/theme_state.go`
- Create: `src/go/internal/ui/theme_state_test.go`
- Modify: `src/go/internal/ui/game_struct.go` (fields)

**Interfaces:**
- Produces: `func effectiveTheme(user theme.UserDoc, circuit theme.Doc) theme.Doc` (pure), `func (g *Game) recomputeEffectiveTheme()`, `func (g *Game) UserTheme() theme.UserDoc`, `func (g *Game) CircuitTheme() theme.Doc`, `func (g *Game) setUserThemeCategory(id, swatch string)`, `func (g *Game) setCircuitThemeCategory(id, swatch string)`, `func (g *Game) clearUserThemeCategory(id string)`, `func (g *Game) clearCircuitThemeCategory(id string)`, `func (g *Game) resetUserTheme()`, `func (g *Game) resetCircuitTheme()`, `func (g *Game) setThemeOverride(on bool)`, `func (g *Game) SetCircuitTheme(theme.Doc)` (import path), `func dropTokensOfCategory(d *theme.Doc, id string)`.
- Game fields: `userTheme theme.UserDoc`, `circuitTheme theme.Doc`.
- Persistence and events are wired in Tasks 9 and 11; this task leaves two hook seams as no-op package funcs: `var persistUserThemeHook = func(theme.UserDoc) {}` is NOT used — instead Task 9 defines `persistUserTheme` and Task 11 defines the emitters; to keep this task compiling, define `func persistUserTheme(theme.UserDoc) {}` and `func emitThemeChanged(scope, category, swatch string)` as stubs HERE and let Tasks 9/11 replace their bodies.

- [ ] **Step 1: Write the failing tests**

```go
//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func TestThemePrecedenceTable(t *testing.T) {
	cases := []struct {
		name          string
		override      bool
		user, circuit string // "" = unset
		want          string
	}{
		{"nothing set", false, "", "", ""},
		{"user only", false, "mint-100", "", "mint-100"},
		{"circuit only", false, "", "cyan-200", "cyan-200"},
		{"both, no override → circuit", false, "mint-100", "cyan-200", "cyan-200"},
		{"override, nothing", true, "", "", ""},
		{"override, user only", true, "mint-100", "", "mint-100"},
		{"override, circuit only → circuit", true, "", "cyan-200", "cyan-200"},
		{"override, both → user", true, "mint-100", "cyan-200", "mint-100"},
	}
	for _, c := range cases {
		u := theme.UserDoc{OverrideCircuits: c.override}
		if c.user != "" {
			u.Categories = map[string]string{"accent": c.user}
		}
		var cd theme.Doc
		if c.circuit != "" {
			cd.Categories = map[string]string{"accent": c.circuit}
		}
		got := effectiveTheme(u, cd).Categories["accent"]
		if got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestEffectiveThemeMergesTokensWithSameRule(t *testing.T) {
	u := theme.UserDoc{Doc: theme.Doc{Tokens: map[string]string{"primary": "#111111"}}}
	c := theme.Doc{Tokens: map[string]string{"primary": "#222222", "focus-ring": "#333333"}}
	got := effectiveTheme(u, c)
	if got.Tokens["primary"] != "#222222" || got.Tokens["focus-ring"] != "#333333" {
		t.Fatalf("no override: %v", got.Tokens)
	}
	u.OverrideCircuits = true
	if got := effectiveTheme(u, c); got.Tokens["primary"] != "#111111" {
		t.Fatalf("override: %v", got.Tokens)
	}
}

func TestEditorPickClearsCategoryTokens(t *testing.T) {
	d := theme.Doc{Tokens: map[string]string{"primary": "#111111", "focus-ring": "#222222"}}
	dropTokensOfCategory(&d, "accent")
	if _, ok := d.Tokens["primary"]; ok {
		t.Fatal("primary belongs to accent and must be dropped")
	}
	if d.Tokens["focus-ring"] != "#222222" {
		t.Fatal("focus-ring belongs to focus and must survive")
	}
}

func TestRecomputeSkipsApplyWhenUnchanged(t *testing.T) {
	g := newGameWithStart(t)
	gen := ThemeGeneration()
	g.recomputeEffectiveTheme() // nothing set → identical to active → no apply
	if ThemeGeneration() != gen {
		t.Fatal("recompute applied an unchanged theme")
	}
	g.setUserThemeCategory("accent", "orchid-300")
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	if ThemeGeneration() != gen+1 || genColorPrimary == genColorDefaultByName["primary"] {
		t.Fatal("user pick did not apply")
	}
	g.setThemeOverride(true) // still the same effective doc → no new generation
	if ThemeGeneration() != gen+1 {
		t.Fatal("override flip with no circuit theme must not re-apply")
	}
	g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"accent": "mint-300"}})
	if ThemeGeneration() != gen+1 { // override on → user still wins → unchanged
		t.Fatal("override on: circuit theme must not change the effective theme")
	}
	g.setThemeOverride(false)
	mint, _ := swatchRGBA("mint-300")
	if genColorPrimary != mint {
		t.Fatalf("override off: circuit should win, primary=%v", genColorPrimary)
	}
	g.resetUserTheme()
	g.resetCircuitTheme()
	if genColorPrimary != genColorDefaultByName["primary"] {
		t.Fatal("resets did not return to default")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestThemePrecedenceTable|TestEffectiveThemeMergesTokensWithSameRule|TestEditorPickClearsCategoryTokens|TestRecomputeSkipsApplyWhenUnchanged'`
Expected: FAIL (undefined symbols).

- [ ] **Step 3: Implement**

Add to `Game` in `game_struct.go`: `userTheme theme.UserDoc`, `circuitTheme theme.Doc`, `themeCancel func()` (if not already added in Task 7).

```go
// src/go/internal/ui/theme_state.go
package ui

import "github.com/ingyamilmolinar/beatmo/internal/theme"

// effectiveTheme merges the user profile theme and the circuit theme, per
// category and per explicit token:
//
//	override && user has it → user
//	else circuit has it     → circuit
//	else user has it        → user
//	else                    → default (absent)
func effectiveTheme(user theme.UserDoc, circuit theme.Doc) theme.Doc {
	pick := func(u, c map[string]string) map[string]string {
		out := map[string]string{}
		for k, v := range u {
			out[k] = v
		}
		for k, v := range c {
			if uv, ok := u[k]; ok && user.OverrideCircuits {
				out[k] = uv
				continue
			}
			out[k] = v
		}
		if len(out) == 0 {
			return nil
		}
		return out
	}
	return theme.Doc{Categories: pick(user.Categories, circuit.Categories), Tokens: pick(user.Tokens, circuit.Tokens)}
}

// dropTokensOfCategory removes every explicit token override owned by the
// category, so an editor pick is never shadowed by a stale hand-authored hex.
func dropTokensOfCategory(d *theme.Doc, id string) {
	for tok := range d.Tokens {
		if owner, ok := themeCategoryOfToken(tok); ok && owner == id {
			delete(d.Tokens, tok)
		}
	}
}

// recomputeEffectiveTheme re-resolves precedence and applies only when the
// document actually changed (an import of an unthemed circuit must not cost
// a cache-rebuild burst).
func (g *Game) recomputeEffectiveTheme() {
	eff := effectiveTheme(g.userTheme, g.circuitTheme)
	if eff.Equal(ActiveTheme()) {
		return
	}
	ApplyTheme(eff)
}

func (g *Game) UserTheme() theme.UserDoc {
	return theme.UserDoc{Doc: g.userTheme.Doc.Clone(), OverrideCircuits: g.userTheme.OverrideCircuits}
}
func (g *Game) CircuitTheme() theme.Doc { return g.circuitTheme.Clone() }

// SetCircuitTheme replaces the circuit's theme (import / undo restore path).
func (g *Game) SetCircuitTheme(d theme.Doc) {
	g.circuitTheme = d.Clone()
	g.recomputeEffectiveTheme()
}

func (g *Game) setUserThemeCategory(id, swatch string) {
	if g.userTheme.Categories == nil {
		g.userTheme.Categories = map[string]string{}
	}
	g.userTheme.Categories[id] = swatch
	dropTokensOfCategory(&g.userTheme.Doc, id)
	g.recomputeEffectiveTheme()
	persistUserTheme(g.userTheme)
	emitThemeChanged("user", id, swatch)
}

func (g *Game) clearUserThemeCategory(id string) {
	delete(g.userTheme.Categories, id)
	dropTokensOfCategory(&g.userTheme.Doc, id)
	g.recomputeEffectiveTheme()
	persistUserTheme(g.userTheme)
	emitThemeChanged("user", id, "")
}

func (g *Game) resetUserTheme() {
	g.userTheme.Doc = theme.Doc{}
	g.recomputeEffectiveTheme()
	persistUserTheme(g.userTheme)
	emitThemeChanged("user", "", "")
}

func (g *Game) setThemeOverride(on bool) {
	g.userTheme.OverrideCircuits = on
	g.recomputeEffectiveTheme()
	persistUserTheme(g.userTheme)
	emitThemeOverrideChanged(on)
}

func (g *Game) setCircuitThemeCategory(id, swatch string) {
	if g.circuitTheme.Categories == nil {
		g.circuitTheme.Categories = map[string]string{}
	}
	g.circuitTheme.Categories[id] = swatch
	dropTokensOfCategory(&g.circuitTheme, id)
	g.recomputeEffectiveTheme()
	emitCircuitThemeChanged(id, swatch)
}

func (g *Game) clearCircuitThemeCategory(id string) {
	delete(g.circuitTheme.Categories, id)
	dropTokensOfCategory(&g.circuitTheme, id)
	g.recomputeEffectiveTheme()
	emitCircuitThemeChanged(id, "")
}

func (g *Game) resetCircuitTheme() {
	g.circuitTheme = theme.Doc{}
	g.recomputeEffectiveTheme()
	emitCircuitThemeChanged("", "")
}
```

Temporary stubs (replaced in Tasks 9 and 11) at the bottom of the file:

```go
// Replaced by theme_sink.go (Task 9) and event_helpers.go (Task 11).
func persistUserTheme(theme.UserDoc)                {}
func emitThemeChanged(scope, category, swatch string) {}
func emitThemeOverrideChanged(on bool)               {}
func emitCircuitThemeChanged(category, swatch string) {}
```

- [ ] **Step 4: Run tests**

Run Step 2's command, then the full ui suite. Expected: PASS.

---

### Task 9: User-profile persistence (`userprefs.ThemeStore` + `theme_sink.go` + bootstraps)

**Files:**
- Modify: `src/go/internal/userprefs/store.go` (prefsDoc, prefsSnapshot, marshalPrefs, parsePrefs, interface)
- Modify: `src/go/internal/userprefs/store_notjs.go` (fileStore field, applySnapshotLocked, snapshotPrefsLocked, Load/Save)
- Modify: `src/go/internal/userprefs/store_js.go` (same for localStorageStore)
- Create: `src/go/internal/userprefs/theme_test.go`
- Create: `src/go/internal/ui/theme_sink.go`, `src/go/internal/ui/theme_sink_test.go`
- Modify: `src/go/internal/ui/theme_state.go` (delete the `persistUserTheme` stub)
- Modify: `src/go/cmd/beatmo.go:239-243`, `src/go/internal/ui/js_bootstrap_wasm.go:71-74`

**Interfaces:**
- Produces: `userprefs.ThemeStore interface{ LoadTheme() (theme.UserDoc, error); SaveTheme(theme.UserDoc) error }`; ui `ThemeSaveSink` (same method set), `SetThemeSink(ThemeSaveSink)`, `ApplyStoredTheme(g *Game)`, `persistUserTheme(theme.UserDoc)`.
- `userprefs` imports `internal/theme` (allowed: theme has no deps).

- [ ] **Step 1: Write the failing userprefs round-trip test**

```go
// src/go/internal/userprefs/theme_test.go
package userprefs

import (
	"path/filepath"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func TestThemeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	s := NewBackingStore(Options{Path: path, PoolName: uniquePoolName(t), Logger: t.Logf})
	t.Cleanup(func() { _ = s.Close() })
	ts, ok := s.(ThemeStore)
	if !ok {
		t.Fatal("backing store does not implement ThemeStore")
	}
	want := theme.UserDoc{Doc: theme.Doc{Categories: map[string]string{"accent": "orchid-300"}, Tokens: map[string]string{"focus-ring": "#FF2D9E"}}, OverrideCircuits: true}
	if err := ts.SaveTheme(want); err != nil {
		t.Fatal(err)
	}
	if err := s.WaitFlushed(); err != nil {
		t.Fatal(err)
	}
	s2 := NewBackingStore(Options{Path: path, PoolName: uniquePoolName(t), Logger: t.Logf})
	t.Cleanup(func() { _ = s2.Close() })
	got, err := s2.(ThemeStore).LoadTheme()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Doc.Equal(want.Doc) || got.OverrideCircuits != want.OverrideCircuits {
		t.Fatalf("got %+v want %+v", got, want)
	}
}

func TestThemeAbsentIsEmpty(t *testing.T) {
	snap, err := parsePrefs([]byte(`{"schema_version":"0.0.1","language":"es"}`), t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	if !snap.theme.Doc.IsEmpty() || snap.theme.OverrideCircuits {
		t.Fatalf("absent theme must load empty: %+v", snap.theme)
	}
	out, _ := marshalPrefs(snap)
	if bytesContains(out, `"theme"`) {
		t.Fatalf("empty theme must be omitted on write: %s", out)
	}
}

func bytesContains(b []byte, s string) bool { return len(b) > 0 && string(b) != "" && indexOf(string(b), s) >= 0 }
func indexOf(h, n string) int {
	for i := 0; i+len(n) <= len(h); i++ {
		if h[i:i+len(n)] == n {
			return i
		}
	}
	return -1
}
```

(Use `strings.Contains` instead of the two helpers if `strings` is already imported in the test package.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/userprefs/ -run 'TestTheme'`
Expected: FAIL (`ThemeStore` undefined).

- [ ] **Step 3: Implement the store section (both backends)**

`store.go`:

```go
// in prefsDoc, after Language:
	// Theme carries the per-user UI color scheme + the circuit-override flag.
	// omitempty so files don't grow until the user opens the Theme editor.
	Theme *theme.UserDoc `json:"theme,omitempty"`

// in prefsSnapshot:
	theme theme.UserDoc

// in marshalPrefs, after the AudioPanel block:
	if !snap.theme.Doc.IsEmpty() || snap.theme.OverrideCircuits {
		cp := theme.UserDoc{Doc: snap.theme.Doc.Clone(), OverrideCircuits: snap.theme.OverrideCircuits}
		doc.Theme = &cp
	}

// in parsePrefs, after the AudioPanel block:
	if doc.Theme != nil {
		snap.theme = theme.UserDoc{Doc: doc.Theme.Doc.Clone(), OverrideCircuits: doc.Theme.OverrideCircuits}
	}

// interface, next to LanguageStore:
// ThemeStore persists the user's UI color scheme. Expose via type assertion
// on the Store returned by NewBackingStore.
type ThemeStore interface {
	LoadTheme() (theme.UserDoc, error)
	SaveTheme(d theme.UserDoc) error
}
```

`store_notjs.go`: add `theme theme.UserDoc` to `fileStore`; copy it in `applySnapshotLocked` (`s.theme = snap.theme`) and `snapshotPrefsLocked` (`theme: s.theme`); add:

```go
// LoadTheme returns the persisted user theme (empty when never set).
func (s *fileStore) LoadTheme() (theme.UserDoc, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.ensureLoadedLocked()
	return theme.UserDoc{Doc: s.theme.Doc.Clone(), OverrideCircuits: s.theme.OverrideCircuits}, nil
}

// SaveTheme upserts the user theme.
func (s *fileStore) SaveTheme(d theme.UserDoc) error {
	s.mu.Lock()
	_ = s.ensureLoadedLocked()
	s.theme = theme.UserDoc{Doc: d.Doc.Clone(), OverrideCircuits: d.OverrideCircuits}
	return s.queueWriteLocked()
}
```

`store_js.go`: identical shape on `localStorageStore` using `s.ensureLoadedLocked()` (no error) and `return s.persistLocked()`.

- [ ] **Step 4: Run userprefs tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/userprefs/` and `GOOS=js GOARCH=wasm ../../.tools/go/bin/go vet ./internal/userprefs/` (compiles the js backend).
Expected: PASS / no vet errors.

- [ ] **Step 5: Write the failing sink test (ui)**

```go
//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

type fakeThemeSink struct {
	stored theme.UserDoc
	saved  []theme.UserDoc
}

func (f *fakeThemeSink) LoadTheme() (theme.UserDoc, error) { return f.stored, nil }
func (f *fakeThemeSink) SaveTheme(d theme.UserDoc) error    { f.saved = append(f.saved, d); return nil }

func TestApplyStoredThemeBeforeFirstFrame(t *testing.T) {
	defer SetThemeSink(nil)
	sink := &fakeThemeSink{stored: theme.UserDoc{Doc: theme.Doc{Categories: map[string]string{"accent": "mint-300"}}}}
	SetThemeSink(sink)
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	ApplyStoredTheme(g)
	mint, _ := swatchRGBA("mint-300")
	if genColorPrimary != mint {
		t.Fatalf("stored theme not applied: primary=%v", genColorPrimary)
	}
	if got := g.UserTheme().Categories["accent"]; got != "mint-300" {
		t.Fatalf("Game.userTheme not seeded: %q", got)
	}
}

func TestUserPickPersistsThroughSink(t *testing.T) {
	defer SetThemeSink(nil)
	sink := &fakeThemeSink{}
	SetThemeSink(sink)
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.setUserThemeCategory("focus", "cyan-300")
	g.setThemeOverride(true)
	if len(sink.saved) != 2 || sink.saved[0].Categories["focus"] != "cyan-300" || !sink.saved[1].OverrideCircuits {
		t.Fatalf("saved = %+v", sink.saved)
	}
}
```

- [ ] **Step 6: Implement the sink and bootstraps**

```go
// src/go/internal/ui/theme_sink.go
package ui

import (
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// ThemeSaveSink is the persistence shim the Theme overlay writes the user's
// theme through and reads at startup. userprefs.ThemeStore satisfies it;
// tests use a stub. Mirrors LanguageSaveSink (language_sink.go).
type ThemeSaveSink interface {
	LoadTheme() (theme.UserDoc, error)
	SaveTheme(d theme.UserDoc) error
}

var (
	themeSinkMu sync.RWMutex
	themeSink   ThemeSaveSink
)

// SetThemeSink registers the global theme persistence sink. Nil-safe.
func SetThemeSink(s ThemeSaveSink) {
	themeSinkMu.Lock()
	themeSink = s
	themeSinkMu.Unlock()
}

func activeThemeSink() ThemeSaveSink {
	themeSinkMu.RLock()
	defer themeSinkMu.RUnlock()
	return themeSink
}

// ApplyStoredTheme seeds g.userTheme from the sink and applies the effective
// theme before the first frame. Called by the production bootstraps right
// after New(); no sink or empty theme ⇒ default palette.
func ApplyStoredTheme(g *Game) {
	s := activeThemeSink()
	if s == nil || g == nil {
		return
	}
	d, err := s.LoadTheme()
	if err != nil {
		themeWarnf("[theme] load user theme: %v", err)
		return
	}
	g.userTheme = d
	g.recomputeEffectiveTheme()
}

// persistUserTheme writes the user theme through the sink (no-op without one).
func persistUserTheme(d theme.UserDoc) {
	if s := activeThemeSink(); s != nil {
		if err := s.SaveTheme(d); err != nil {
			themeWarnf("[theme] save user theme: %v", err)
		}
	}
}
```

Delete the `persistUserTheme` stub from `theme_state.go`. In `cmd/beatmo.go` right AFTER `g := ui.New(logger)`:

```go
	// User theme: seed the profile theme from prefs and apply it before the
	// first Layout so frame 1 already renders in the user's colors.
	if ts, ok := prefsStore.(userprefs.ThemeStore); ok {
		ui.SetThemeSink(ts)
		ui.ApplyStoredTheme(g)
	}
```

In `js_bootstrap_wasm.go` the language sink is set BEFORE the Game exists; add the `SetThemeSink` call there (beside `SetLanguageSink`) and find the spot where the WASM bootstrap constructs the Game (`grep -n 'New(' js_bootstrap_wasm.go js_main*.go`) and call `ApplyStoredTheme(g)` right after it.

- [ ] **Step 7: Run tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestApplyStoredThemeBeforeFirstFrame|TestUserPickPersistsThroughSink' && ../../.tools/go/bin/go build ./... && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -o /dev/null ./cmd/wasm 2>&1 | head` (adjust the wasm main package path: `grep -rn 'GOOS=js' ../../Makefile | head -2`).
Expected: PASS; both builds succeed.

---

### Task 10: Circuit theme in the project file (export / import / undo detail / template guard)

**Files:**
- Modify: `src/go/internal/ui/project_file.go` (field), `export.go:592` (write), `import.go:123-170` (apply), `undo_change_detail.go:53` (detail line)
- Create: `src/go/internal/ui/theme_project_test.go`
- Create: `src/go/internal/assets/templates_theme_test.go`

**Interfaces:**
- `projectFile.Theme *theme.Doc \`json:"theme,omitempty"\``.
- Consumes: `(*Game).SetCircuitTheme`, `(*Game).CircuitTheme`, `(*DrumView).exportBytes`.

- [ ] **Step 1: Write the failing tests**

```go
//go:build test

package ui

import (
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func TestProjectThemeRoundTrip(t *testing.T) {
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"grid": "cyan-400"}, Tokens: map[string]string{"focus-ring": "#3FD67A"}})
	b, err := g.drum.exportBytes()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"theme"`) {
		t.Fatalf("export lacks theme block: %s", b)
	}
	g.SetCircuitTheme(theme.Doc{})
	if err := g.Import(b); err != nil {
		t.Fatal(err)
	}
	got := g.CircuitTheme()
	if got.Categories["grid"] != "cyan-400" || got.Tokens["focus-ring"] != "#3FD67A" {
		t.Fatalf("import did not restore circuit theme: %+v", got)
	}
	cyan, _ := swatchRGBA("cyan-400")
	h0, _, _ := rgbToHSL(cyan)
	h1, _, _ := rgbToHSL(genColorGridQuarter)
	if hueDistance(h0, h1) > 20 {
		t.Fatalf("imported circuit theme not applied to grid lines: %v", genColorGridQuarter)
	}
}

func TestProjectWithoutThemeExportsNoBlockAndClearsOnImport(t *testing.T) {
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	b, _ := g.drum.exportBytes()
	if strings.Contains(string(b), `"theme"`) {
		t.Fatalf("unthemed project must omit theme: %s", b)
	}
	g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"accent": "mint-300"}})
	if err := g.Import(b); err != nil { // replace-not-merge
		t.Fatal(err)
	}
	if !g.CircuitTheme().IsEmpty() || genColorPrimary != genColorDefaultByName["primary"] {
		t.Fatal("import of an unthemed project must clear the circuit theme")
	}
}

func TestUndoRestoresCircuitTheme(t *testing.T) {
	g := newGameWithStart(t)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.setCircuitThemeCategory("accent", "orchid-300")
	orchid, _ := swatchRGBA("orchid-300")
	if genColorPrimary != orchid {
		t.Fatal("circuit pick not applied")
	}
	if g.undoManager.UndoLabel() == "" {
		t.Fatal("circuit theme pick must be undoable (ScopeDocument kind): no undo step recorded")
	}
	g.undoManager.Undo()
	advanceFrames(g, 2)
	if genColorPrimary != genColorDefaultByName["primary"] || !g.CircuitTheme().IsEmpty() {
		t.Fatalf("undo did not restore: primary=%v circuit=%+v", genColorPrimary, g.CircuitTheme())
	}
	g.undoManager.Redo()
	advanceFrames(g, 2)
	if genColorPrimary != orchid {
		t.Fatal("redo did not re-apply the circuit theme")
	}
}

func TestUndoDetailForTheme(t *testing.T) {
	before := []byte(`{"bpm":120,"instruments":[],"nodes":[]}`)
	after := []byte(`{"bpm":120,"instruments":[],"nodes":[],"theme":{"categories":{"accent":"orchid-300"}}}`)
	d, ok := describeUndoChange(before, after)
	if !ok || d.param != "Theme" || d.new != "accent: orchid-300" || d.old != "accent: (default)" {
		t.Fatalf("detail = %+v ok=%v", d, ok)
	}
}
```

(`changeDetail` has positional fields `param, old, new`; `UndoManager.Undo/Redo` return nothing, `UndoLabel()` is empty when the stack is empty.) Template guard:

```go
// src/go/internal/assets/templates_theme_test.go
package assets

import (
	"encoding/json"
	"testing"
)

// Built-in templates ship the default look; a theme block in one would force
// the author's colors on every user who loads it.
func TestTemplatesCarryNoTheme(t *testing.T) {
	for _, tp := range Templates() {
		var doc map[string]json.RawMessage
		if err := json.Unmarshal(tp.Bytes, &doc); err != nil {
			t.Fatalf("%s: %v", tp.Display, err)
		}
		if _, ok := doc["theme"]; ok {
			t.Errorf("template %s carries a theme block", tp.Display)
		}
	}
}
```

(`Template` has `Genre, Display, BPM, Group, Bytes` — no id field, so `Display` names the offender.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestProjectThemeRoundTrip|TestProjectWithoutTheme|TestUndoRestoresCircuitTheme|TestUndoDetailForTheme' ; ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/ -run TestTemplatesCarryNoTheme`
Expected: ui FAIL (field missing / undo not recorded); assets PASS already (guard in place).

- [ ] **Step 3: Implement**

`project_file.go` after `Groups`:

```go
	// Theme is the circuit's own UI color scheme (see internal/theme and
	// ui/theme_categories.go). Optional; the user's profile theme may override
	// it. Written only when the circuit sets something.
	Theme *theme.Doc `json:"theme,omitempty"`
```

`export.go` after `file.Groups = groups`:

```go
	if g := dv.game; g != nil && !g.circuitTheme.IsEmpty() {
		cp := g.circuitTheme.Clone()
		file.Theme = &cp
	}
```

(`dv.game` is the back-pointer used in `openSettingsOverlay`; confirm its name.) `import.go` right after the `SetProjectPins` block:

```go
	// Circuit theme: replace-not-merge. An absent block clears any previous
	// circuit theme so an import never inherits colors from the last project.
	var ct theme.Doc
	if f.Theme != nil {
		ct = f.Theme.Clone()
	}
	g.SetCircuitTheme(ct)
```

`undo_change_detail.go`, in `describeUndoChange` before the `bn := make(...)` line:

```go
	if d, ok := diffTheme(b.Theme, a.Theme); ok {
		return d, true
	}
```

and

```go
// diffTheme reports the first category whose swatch changed between two
// circuit theme blocks ("accent: orchid-300"), or "(none)" when cleared.
func diffTheme(b, a *theme.Doc) (changeDetail, bool) {
	get := func(d *theme.Doc, k string) string {
		if d == nil {
			return ""
		}
		return d.Categories[k]
	}
	for _, c := range themeCategories {
		bv, av := get(b, c.ID), get(a, c.ID)
		if bv == av {
			continue
		}
		fmtv := func(v string) string {
			if v == "" {
				return c.ID + ": (default)"
			}
			return c.ID + ": " + v
		}
		return changeDetail{"Theme", fmtv(bv), fmtv(av)}, true
	}
	return changeDetail{}, false
}
```

Undo recording: in `theme_state.go` `setCircuitThemeCategory` / `clearCircuitThemeCategory` / `resetCircuitTheme` call `recordUndo(hooks.EventCircuitThemeChanged)` AFTER mutating state — this needs the Kind from Task 11; if executing in order, temporarily use `recordUndo(hooks.EventRowColorChanged)` is NOT acceptable (wrong label) — do Task 11 Step 3 (the hooks kinds) first, then come back. The undo snapshot is the export bytes, so the theme block is captured automatically and `undoRestore → Import → SetCircuitTheme` re-applies it.

- [ ] **Step 4: Run tests**

Run Step 2's commands, then the whole ui suite + `./internal/assets/`. Expected: PASS.

---

### Task 11: Hooks kinds, event-log formatters, emit helpers, i18n strings

**Files:**
- Modify: `src/go/internal/hooks/events.go` (kinds ~line 116, `KindAll` ~line 222, `NumNonVerbose` line 258, payload ~line 588)
- Modify: `src/go/internal/hooks/actions.go:116`
- Modify: `src/go/internal/eventlogger/format.go:299`
- Modify: `src/go/internal/ui/event_helpers.go` (emitters; delete stubs from `theme_state.go`)
- Modify: `src/go/internal/i18n/catalog_en.go:15`, `catalog_es.go:15`

- [ ] **Step 1: Run the guards to see them fail after adding the kinds**

Add to `hooks/events.go` after `EventLanguageChanged`:

```go
	// EventThemeChanged fires on a profile-scope theme edit (pick, per-category
	// reset, reset-all, override toggle). ScopeSession: it persists to
	// userprefs, not the project document.
	EventThemeChanged Kind = "theme.changed"
	// EventCircuitThemeChanged fires on a circuit-scope theme edit. It mutates
	// the exported document and is undoable through the snapshot journal.
	EventCircuitThemeChanged Kind = "theme.circuit.changed"
```

Add both to `KindAll` right after `EventLanguageChanged,`. Bump `NumNonVerbose` to `76` and extend its comment (`// + theme.changed + theme.circuit.changed (theme editor) = 76.`). Payload next to `LanguagePayload`:

```go
// ThemePayload describes a theme edit. Category/Swatch are empty for a
// reset-all; Swatch is empty for a per-category reset. Override is non-nil
// only for the override toggle.
type ThemePayload struct {
	Scope    string `json:"scope"` // "user" | "circuit"
	Category string `json:"category,omitempty"`
	Swatch   string `json:"swatch,omitempty"`
	Override *bool  `json:"override,omitempty"`
}
```

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/ ./internal/log/`
Expected: FAIL — `TestEveryKindClassified` (no registry entry) and `TestEventLoggerCoversAllNonVerboseKinds` (no formatter).

- [ ] **Step 2: Registry + formatter**

`hooks/actions.go` after the `EventLanguageChanged` row:

```go
	{EventThemeChanged, "change theme", "theme", ScopeSession, ""},
	{EventCircuitThemeChanged, "change circuit theme", "theme", ScopeDocument, ""},
```

`eventlogger/format.go` after the `EventLanguageChanged` entry:

```go
	hooks.EventThemeChanged:        formatTheme,
	hooks.EventCircuitThemeChanged: formatTheme,
```

and a helper in the same file:

```go
func formatTheme(p any) string {
	t, _ := p.(hooks.ThemePayload)
	where := "my theme"
	if t.Scope == "circuit" {
		where = "this circuit"
	}
	switch {
	case t.Override != nil:
		return fmt.Sprintf("always use my theme = %v", *t.Override)
	case t.Category == "":
		return fmt.Sprintf("reset all (%s)", where)
	case t.Swatch == "":
		return fmt.Sprintf("%s reset (%s)", t.Category, where)
	}
	return fmt.Sprintf("%s = %s (%s)", t.Category, t.Swatch, where)
}
```

Run the three packages again plus `./internal/ui/ -run 'TestHooks_NumNonVerboseUpdated|TestUndoableSetMatchesRegistry'`. Expected: PASS.

- [ ] **Step 3: Emitters and catalog strings**

Replace the stubs in `theme_state.go` with these in `event_helpers.go`:

```go
// emitThemeChanged publishes a profile-scope theme edit. scope is always
// "user" here; category/swatch may be empty (see hooks.ThemePayload).
func emitThemeChanged(scope, category, swatch string) {
	hooks.PublishWithSource(hooks.EventThemeChanged, hooks.ThemePayload{Scope: scope, Category: category, Swatch: swatch}, hooks.CaptureSource(1))
}

func emitThemeOverrideChanged(on bool) {
	hooks.PublishWithSource(hooks.EventThemeChanged, hooks.ThemePayload{Scope: "user", Override: &on}, hooks.CaptureSource(1))
}

// emitCircuitThemeChanged publishes a circuit-scope edit AND records the undo
// step (document scope).
func emitCircuitThemeChanged(category, swatch string) {
	hooks.PublishWithSource(hooks.EventCircuitThemeChanged, hooks.ThemePayload{Scope: "circuit", Category: category, Swatch: swatch}, hooks.CaptureSource(1))
	recordUndo(hooks.EventCircuitThemeChanged)
}
```

Catalog strings — `catalog_en.go` after `KeyLangSpanish`:

```go
	KeyThemeTitle:           "Theme",
	KeyThemeScopeMine:       "My theme",
	KeyThemeScopeCircuit:    "This circuit",
	KeyThemeOverride:        "Always use my theme",
	KeyThemeResetAll:        "Reset all",
	KeyThemeReset:           "Reset",
	KeyThemeCatBackground:   "Background",
	KeyThemeCatSurfaces:     "Surfaces & menus",
	KeyThemeCatText:         "Text",
	KeyThemeCatAccent:       "Accent",
	KeyThemeCatFocus:        "Focus ring",
	KeyThemeCatGrid:         "Grid lines",
	KeyThemeCatNodes:        "Nodes & edges",
	KeyThemeCatDividers:     "Dividers & borders",
	KeyThemeCatViz:          "Visualizers",
	KeyThemeGlossBackground: "the deep canvas behind everything",
	KeyThemeGlossSurfaces:   "panels, popups and buttons",
	KeyThemeGlossText:       "labels and numbers",
	KeyThemeGlossAccent:     "the glow on anything active",
	KeyThemeGlossFocus:      "the ring around what you are typing in",
	KeyThemeGlossGrid:       "the lines of the beat grid",
	KeyThemeGlossNodes:      "the dots and the wires between them",
	KeyThemeGlossDividers:   "hairlines between areas",
	KeyThemeGlossViz:        "bars and waves in the analyzers",
```

`catalog_es.go`:

```go
	KeyThemeTitle:           "Tema",
	KeyThemeScopeMine:       "Mi tema",
	KeyThemeScopeCircuit:    "Este circuito",
	KeyThemeOverride:        "Usar siempre mi tema",
	KeyThemeResetAll:        "Restablecer todo",
	KeyThemeReset:           "Restablecer",
	KeyThemeCatBackground:   "Fondo",
	KeyThemeCatSurfaces:     "Paneles y menús",
	KeyThemeCatText:         "Texto",
	KeyThemeCatAccent:       "Acento",
	KeyThemeCatFocus:        "Anillo de foco",
	KeyThemeCatGrid:         "Líneas de la cuadrícula",
	KeyThemeCatNodes:        "Nodos y conexiones",
	KeyThemeCatDividers:     "Divisores y bordes",
	KeyThemeCatViz:          "Visualizadores",
	KeyThemeGlossBackground: "el lienzo profundo detrás de todo",
	KeyThemeGlossSurfaces:   "paneles, ventanas y botones",
	KeyThemeGlossText:       "etiquetas y números",
	KeyThemeGlossAccent:     "el brillo de lo que está activo",
	KeyThemeGlossFocus:      "el anillo alrededor de donde escribes",
	KeyThemeGlossGrid:       "las líneas de la cuadrícula del ritmo",
	KeyThemeGlossNodes:      "los puntos y los cables entre ellos",
	KeyThemeGlossDividers:   "líneas finas entre áreas",
	KeyThemeGlossViz:        "barras y ondas de los analizadores",
```

- [ ] **Step 4: Run every affected package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/hooks/ ./internal/eventlogger/ ./internal/log/ ./internal/i18n/ ./internal/ui/`
Expected: PASS (including `TestNoUppercaseInfoTags`, catalog completeness, `TestUndoRestoresCircuitTheme` from Task 10 now that the kind exists).

---

### Task 12: `IconPalette` + fourth top-right button + input handler + reserved zone

**Files:**
- Modify: `src/go/internal/ui/icons.go:56` (id) and `:159` (dispatch), `drawing_icons.go` (glyph)
- Modify: `src/go/internal/ui/game_struct.go:26` (`gridThemeBtn *Button`), `game_new.go:159` (construct), `game_layout.go:168`, `grid_pane_draw.go:1407` (draw), `game_input_shortcuts.go:452-500` (rect + toggle + menu ids), `grid_reserved_zone.go:34-46` (reserved union), `game_update.go:430` (register handler)
- Create: `src/go/internal/ui/grid_theme_input_handler.go`
- Create: `src/go/internal/ui/theme_button_test.go`

**Interfaces:**
- Produces: `IconPalette IconID = "palette"`, `(*Game).gridThemeButtonRect() image.Rectangle`, `(*Game).toggleThemeOverlay()`, `const themeOverlayID = "theme"`, `gridThemeInputHandler`, `const gridThemeInputZ = gridLogsInputZ - 1`.
- Consumes (Task 13): `(*DrumView).openThemeOverlay()` — stub it in this task as `func (dv *DrumView) openThemeOverlay() {}` and let Task 13 fill it.

- [ ] **Step 1: Write the failing tests**

```go
//go:build test

package ui

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestIconPaletteDispatch(t *testing.T) {
	dst := ebiten.NewImage(24, 24)
	if !drawIconByID(dst, IconPalette, image.Rect(0, 0, 24, 24), color.NRGBA{255, 255, 255, 255}) {
		t.Fatal("IconPalette not registered in drawIconByID")
	}
}

// The theme button sits one gap left of Logs, same size, inside the
// protected top-right cluster on desktop AND mobile.
func TestThemeButtonPlacementAndReservedZone(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		if mobile {
			setupMobileTest(t, true) // shared helper (layout_responsive_test.go): forces the mobile screen class; call BEFORE New()
		}
		g := newGameWithStart(t)
		if mobile {
			g.Layout(390, 844)
		} else {
			g.Layout(1280, 720)
		}
		advanceFrames(g, 2)
		th, logs := g.gridThemeButtonRect(), g.gridLogsButtonRect()
		if th.Empty() || logs.Empty() {
			t.Fatalf("mobile=%v: empty rects theme=%v logs=%v", mobile, th, logs)
		}
		if th.Dx() != logs.Dx() || th.Dy() != logs.Dy() || th.Min.Y != logs.Min.Y {
			t.Fatalf("mobile=%v: theme %v must match logs %v size/row", mobile, th, logs)
		}
		if th.Max.X+SpaceSM != logs.Min.X {
			t.Fatalf("mobile=%v: theme must sit one SpaceSM left of logs: %v vs %v", mobile, th, logs)
		}
		if th.Min.X < g.split.GridRect(g.winW, g.winH).Min.X {
			t.Fatalf("mobile=%v: cluster overflows the pane: %v", mobile, th)
		}
		cx, cy := (th.Min.X+th.Max.X)/2, (th.Min.Y+th.Max.Y)/2
		if !g.inTopRightButtonZone(cx, cy) || !g.inTopRightButtonZone(th.Max.X+SpaceSM/2, cy) {
			t.Fatalf("mobile=%v: theme button / gap not in the protected zone", mobile)
		}
	}
}

// A real click on the theme button opens the theme overlay through the grid
// inputDispatcher; a second click closes it; opening it closes Settings.
func TestThemeButtonRealClickTogglesOverlay(t *testing.T) {
	g := newGameWithStart(t)
	g.Layout(1280, 720)
	advanceFrames(g, 2)
	g.toggleSettingsOverlay()
	advanceFrames(g, 2)
	r := g.gridThemeButtonRect()
	cx, cy := (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2
	holdTap := makeHoldTap(g, 1280, 720)
	holdTap(cx, cy, 3)
	advanceFrames(g, 2)
	p := g.drum.portal()
	if !p.Has(themeOverlayID) {
		t.Fatal("theme overlay did not open on click")
	}
	if p.Has(settingsOverlayID) {
		t.Fatal("opening the theme overlay must close Settings (mutual exclusion)")
	}
	holdTap(cx, cy, 3)
	advanceFrames(g, 2)
	if p.Has(themeOverlayID) {
		t.Fatal("second click must close the theme overlay")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestIconPaletteDispatch|TestThemeButton'`
Expected: FAIL (undefined).

- [ ] **Step 3: Implement**

`icons.go`: add `IconPalette IconID = "palette"` after `IconLogs`; in `drawIconByID` add `case IconPalette: drawPaletteIcon(dst, r, col)`. `drawing_icons.go` after `drawLogsIcon`:

```go
// Palette: a painter's palette — a rounded disc with a thumb notch and three
// paint dots. The grid-pane Theme button glyph.
var drawPaletteIcon = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
	if r.Empty() {
		return
	}
	c := newIconCanvas(r)
	iconStrokeCircle(dst, c, 12, 12, 8.5, col)            // disc outline
	iconFillCircle(dst, c, 15.5, 12, 2.2, col)             // thumb notch (filled, reads as the hole)
	iconFillCircle(dst, c, 8, 9, 1.6, col)                 // paint dots
	iconFillCircle(dst, c, 11.5, 6.5, 1.6, col)
	iconFillCircle(dst, c, 8.5, 14.5, 1.6, col)
}
```

`game_struct.go`: `gridThemeBtn *Button // theme button left of the logs button (opens the Theme editor; both platforms)`. `game_new.go` after the logs button:

```go
	// Theme button, one gap left of the logs button: opens the Theme editor.
	// Both platforms. Routed through gridThemeInputHandler.
	g.gridThemeBtn = NewSpecButton("", ComponentButtonSecondary, g.toggleThemeOverlay)
	g.gridThemeBtn.Icon = string(IconPalette)
```

`game_layout.go` + `grid_pane_draw.go`: mirror the logs lines (`SetRect(g.gridThemeButtonRect())`, `Draw(dst)`). `game_input_shortcuts.go`:

```go
// gridThemeButtonRect is the screen-space rect of the theme button, one SpaceSM
// gap to the LEFT of the logs button. Same size. Both platforms.
func (g *Game) gridThemeButtonRect() image.Rectangle {
	if g.gridThemeBtn == nil {
		return image.Rectangle{}
	}
	logs := g.gridLogsButtonRect()
	if logs.Empty() {
		return image.Rectangle{}
	}
	w := logs.Dx()
	x1 := logs.Min.X - SpaceSM
	return image.Rect(x1-w, logs.Min.Y, x1, logs.Max.Y)
}

// themeOverlayID identifies the theme editor portal entry.
const themeOverlayID = "theme"

// toggleThemeOverlay opens the Theme editor, or closes it if already open.
func (g *Game) toggleThemeOverlay() {
	if g.drum == nil || g.drum.tree == nil {
		return
	}
	p := g.drum.portal()
	if p == nil {
		return
	}
	if p.Has(themeOverlayID) {
		p.Close(themeOverlayID)
		return
	}
	g.closeOtherGridButtonMenus(themeOverlayID)
	g.drum.openThemeOverlay()
}
```

Add `themeOverlayID` to `gridButtonMenuIDs`. In `grid_reserved_zone.go` `gridTopButtonsReservedRect`, after the logs union add the same for `g.gridThemeButtonRect()`, and update the doc comment ("gear + file + logs + theme"). `grid_theme_input_handler.go`: copy `grid_logs_input_handler.go` verbatim, renaming to `gridThemeInputHandler`, `gridThemeInputZ = gridLogsInputZ - 1`, `g.gridThemeBtn`, `g.gridThemeButtonRect()`. Register in `game_update.go` after the logs registration:

```go
			if g.gridThemeBtn != nil {
				g.inputDispatcher.Register(gridThemeInputHandler{g: g})
			}
```

Stub for Task 13 in a new file `theme_overlay.go`: `func (dv *DrumView) openThemeOverlay() {}` — the toggle test will fail on `p.Has(themeOverlayID)` until Task 13; that is expected. Mark the test `t.Skip("until Task 13")`? No — leave it failing red and finish Task 13 next; the plan executor runs 12 and 13 back-to-back.

- [ ] **Step 4: Run the tests**

Run Step 2's command. Expected: `TestIconPaletteDispatch` and `TestThemeButtonPlacementAndReservedZone` PASS; `TestThemeButtonRealClickTogglesOverlay` FAIL only on "did not open" (stub). Run the ui suite for regressions: the existing corner-button tests (`game_settings_gear_zone_test.go`, `logs_panel_functional_test.go`, `nav_menu_*`) must stay green.

---

### Task 13: `ThemeOverlay` panel (layout, draw, hit areas, scope switch, override toggle, rows, resets)

**Files:**
- Create: `src/go/internal/ui/theme_overlay.go` (replaces the Task 12 stub file)
- Create: `src/go/internal/ui/theme_overlay_test.go`
- Modify: `src/go/internal/ui/screenshot_subjects.go` (`SubjectThemeEditor`)

**Interfaces:**
- Produces: `type ThemeOverlay struct` implementing `PortalOverlay` (+ `Update()` for scroll momentum), `func NewThemeOverlay(g *Game) *ThemeOverlay`, `(*ThemeOverlay).Rect() image.Rectangle`, `(*ThemeOverlay).ScopePillRects() (mine, circuit image.Rectangle)`, `(*ThemeOverlay).OverrideRect() image.Rectangle`, `(*ThemeOverlay).ChipRect(categoryID string) image.Rectangle`, `(*ThemeOverlay).ResetRect(categoryID string) image.Rectangle`, `(*ThemeOverlay).ResetAllRect() image.Rectangle`, `(*ThemeOverlay).Scope() string` ("user"|"circuit"), `(*DrumView).openThemeOverlay()`, `const SubjectThemeEditor Subject = "theme_editor"`.
- Consumes: Task 8 setters/getters, Task 4 `themeCategories`, `swatchRGBA`, Task 12 `themeOverlayID`. The chip → picker flow is Task 14; here a chip press only calls `o.onChip(categoryID)` (a func field, nil-safe).

- [ ] **Step 1: Write the failing tests (real input through Game.Update)**

```go
//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

func openThemeOverlayForTest(t *testing.T, w, h int) (*Game, *ThemeOverlay, func(x, y, hold int)) {
	t.Helper()
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	t.Cleanup(func() { ApplyTheme(theme.Doc{}) })
	g.Layout(w, h)
	advanceFrames(g, 2)
	g.toggleThemeOverlay()
	advanceFrames(g, 2)
	ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay)
	if !ok || ov == nil {
		t.Fatal("theme overlay is not the top portal overlay after toggle")
	}
	return g, ov, makeHoldTap(g, w, h)
}

func center(r image.Rectangle) (int, int) { return (r.Min.X + r.Max.X) / 2, (r.Min.Y + r.Max.Y) / 2 }

func TestThemeOverlayLayoutHasEveryControl(t *testing.T) {
	g, ov, _ := openThemeOverlayForTest(t, 1280, 720)
	if ov.Rect().Empty() || !ov.Rect().In(g.split.GridRect(g.winW, g.winH)) {
		t.Fatalf("panel %v must sit inside the grid pane", ov.Rect())
	}
	mine, circuit := ov.ScopePillRects()
	if mine.Empty() || circuit.Empty() || mine.Min.Y != circuit.Min.Y || mine.Dy() != circuit.Dy() {
		t.Fatalf("scope pills: %v %v", mine, circuit)
	}
	if ov.OverrideRect().Empty() || ov.ResetAllRect().Empty() {
		t.Fatal("override toggle / reset-all missing")
	}
	for _, c := range themeCategories {
		chip, reset := ov.ChipRect(c.ID), ov.ResetRect(c.ID)
		if chip.Empty() || reset.Empty() {
			t.Fatalf("category %s: chip=%v reset=%v", c.ID, chip, reset)
		}
		if chip.Dx() != chip.Dy() {
			t.Fatalf("category %s: chip must be square: %v", c.ID, chip)
		}
		if !chip.In(ov.Rect()) {
			t.Fatalf("category %s: chip %v outside panel %v (rows must scroll, not overflow)", c.ID, chip, ov.Rect())
		}
	}
	if ov.Rect().Overlaps(g.gridTopButtonsReservedRect()) {
		t.Fatalf("panel %v invades the corner-button cluster %v", ov.Rect(), g.gridTopButtonsReservedRect())
	}
}

func TestThemeOverlayScopeSwitchAndOverrideRealClicks(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	if ov.Scope() != "user" {
		t.Fatalf("default scope = %q, want user", ov.Scope())
	}
	_, circuit := ov.ScopePillRects()
	tap(center(circuit))
	advanceFrames(g, 2)
	if ov.Scope() != "circuit" {
		t.Fatalf("click on 'This circuit' did not switch scope: %q", ov.Scope())
	}
	tap(center(ov.OverrideRect()))
	advanceFrames(g, 2)
	if !g.UserTheme().OverrideCircuits {
		t.Fatal("override toggle click did not flip the flag")
	}
	tap(center(ov.OverrideRect()))
	advanceFrames(g, 2)
	if g.UserTheme().OverrideCircuits {
		t.Fatal("second click did not flip it back")
	}
}

func TestThemeOverlayResetButtonsRealClicks(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	g.setUserThemeCategory("accent", "orchid-300")
	g.setUserThemeCategory("focus", "mint-300")
	advanceFrames(g, 2)
	tap(center(ov.ResetRect("accent")))
	advanceFrames(g, 2)
	if _, ok := g.UserTheme().Categories["accent"]; ok || genColorPrimary != genColorDefaultByName["primary"] {
		t.Fatal("per-category reset did not clear accent")
	}
	if g.UserTheme().Categories["focus"] != "mint-300" {
		t.Fatal("per-category reset must not touch other categories")
	}
	tap(center(ov.ResetAllRect())) // first tap arms
	advanceFrames(g, 2)
	if len(g.UserTheme().Categories) != 1 {
		t.Fatal("first Reset-all tap must only ARM (two-tap confirm)")
	}
	tap(center(ov.ResetAllRect())) // second tap fires
	advanceFrames(g, 2)
	if len(g.UserTheme().Categories) != 0 || genColorFocusRing != genColorDefaultByName["focus-ring"] {
		t.Fatal("reset all did not clear the user theme")
	}
}

func TestThemeOverlayChipShowsEffectiveColorAndRing(t *testing.T) {
	g, ov, _ := openThemeOverlayForTest(t, 1280, 720)
	set, inherited := ov.chipState("accent")
	if set {
		t.Fatal("no pick yet → chip must read as inherited/default")
	}
	_ = inherited
	g.setUserThemeCategory("accent", "orchid-300")
	set, _ = ov.chipState("accent")
	if !set {
		t.Fatal("after a user pick in user scope the chip is explicitly set (ring)")
	}
	orchid, _ := swatchRGBA("orchid-300")
	if ov.chipColor("accent") != orchid {
		t.Fatalf("chip color = %v want %v", ov.chipColor("accent"), orchid)
	}
}

func TestThemeOverlayMobileFitsPane(t *testing.T) {
	setupMobileTest(t, true)
	g, ov, _ := openThemeOverlayForTest(t, 390, 844)
	pane := g.split.GridRect(g.winW, g.winH)
	if !ov.Rect().In(pane) {
		t.Fatalf("mobile panel %v must fit the grid pane %v", ov.Rect(), pane)
	}
	for _, c := range themeCategories {
		if ov.ChipRect(c.ID).Dx() < TouchMinTarget() {
			t.Fatalf("mobile chip %s below touch min: %v", c.ID, ov.ChipRect(c.ID))
		}
	}
}
```

(Import `image` in the test file. `TouchMinTarget()` exists in touch_sizes.go.)

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestThemeOverlay'`
Expected: FAIL (`ThemeOverlay` undefined).

- [ ] **Step 3: Implement the overlay**

```go
// src/go/internal/ui/theme_overlay.go
package ui

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/i18n"
)

// Theme editor panel geometry. Width matches the Settings card so the two
// corner-button panels read as siblings.
const (
	themeOverlayW      = settingsOverlayW
	themeRowH          = 44 // one category row: label + gloss stacked, chip, reset
	themeChipSize      = 28 // desktop chip edge; floored at TouchMinTarget() on mobile
	themePillH         = 32
	themePillW         = 138
	themeResetAllH     = 32
	themeResetAllArmMs = 2000
)

// ThemeOverlay is the portal panel for the runtime theme editor. It edits ONE
// scope at a time (the user's profile theme or the open circuit's theme) and
// applies every pick live through Game.recomputeEffectiveTheme.
type ThemeOverlay struct {
	g      *Game
	closed bool
	rect   image.Rectangle
	place  func(w, h int) image.Rectangle
	scope  string // "user" | "circuit"

	mineBtn, circuitBtn *Button
	overrideBtn         *Button
	resetAllBtn         *Button
	resetAllArmed       bool
	resetAllArmedAt     int64 // unix ms; disarms after themeResetAllArmMs
	resetBtns           map[string]*Button
	chipRects           map[string]image.Rectangle
	rowRects            map[string]image.Rectangle

	// onChip is invoked with the category id when its swatch chip is pressed
	// (Task 14 wires it to the swatch picker). Nil-safe.
	onChip func(categoryID string)

	scroll   *MenuScroll
	rowsView image.Rectangle
	headerH  int
	sectionY int
}

func NewThemeOverlay(g *Game) *ThemeOverlay {
	o := &ThemeOverlay{g: g, scope: "user", resetBtns: map[string]*Button{}, chipRects: map[string]image.Rectangle{}, rowRects: map[string]image.Rectangle{}}
	o.mineBtn = NewButtonKey(i18n.KeyThemeScopeMine, ComponentButtonSecondary, func() { o.scope = "user" })
	o.circuitBtn = NewButtonKey(i18n.KeyThemeScopeCircuit, ComponentButtonSecondary, func() { o.scope = "circuit" })
	o.overrideBtn = NewButtonKey(i18n.KeyThemeOverride, ComponentButtonSecondary, func() {
		if o.g != nil {
			o.g.setThemeOverride(!o.g.userTheme.OverrideCircuits)
		}
	})
	o.resetAllBtn = NewButtonKey(i18n.KeyThemeResetAll, ComponentButtonDestructive, o.onResetAll)
	for _, c := range themeCategories {
		id := c.ID
		b := NewSpecButton("", ComponentButtonSecondary, func() { o.resetCategory(id) })
		b.Icon = string(IconUndo)
		o.resetBtns[id] = b
	}
	o.scroll = NewMenuScroll(DropdownScrollbarStyle, themeRowH)
	return o
}

func (o *ThemeOverlay) SetPositioner(place func(w, h int) image.Rectangle) { o.place = place }
func (o *ThemeOverlay) Close()                                             { o.closed = true }
func (o *ThemeOverlay) ShouldClose() bool                                  { return o.closed }
func (o *ThemeOverlay) Rect() image.Rectangle                              { return o.rect }
func (o *ThemeOverlay) Scope() string                                      { return o.scope }
func (o *ThemeOverlay) ScopePillRects() (mine, circuit image.Rectangle)    { return o.mineBtn.Rect(), o.circuitBtn.Rect() }
func (o *ThemeOverlay) OverrideRect() image.Rectangle                      { return o.overrideBtn.Rect() }
func (o *ThemeOverlay) ResetAllRect() image.Rectangle                      { return o.resetAllBtn.Rect() }
func (o *ThemeOverlay) ChipRect(id string) image.Rectangle                 { return o.chipRects[id] }
func (o *ThemeOverlay) ResetRect(id string) image.Rectangle {
	if b := o.resetBtns[id]; b != nil {
		return b.Rect()
	}
	return image.Rectangle{}
}

// scopeDoc returns the document the editor is currently writing.
func (o *ThemeOverlay) scopeDoc() map[string]string {
	if o.g == nil {
		return nil
	}
	if o.scope == "circuit" {
		return o.g.circuitTheme.Categories
	}
	return o.g.userTheme.Categories
}

// chipState reports whether the active scope explicitly sets the category
// (ring) and, if not, whether the shown color is inherited from the other
// source rather than the default.
func (o *ThemeOverlay) chipState(id string) (set, inherited bool) {
	if _, ok := o.scopeDoc()[id]; ok {
		return true, false
	}
	_, inherited = ActiveTheme().Categories[id]
	return false, inherited
}

// chipColor is the EFFECTIVE swatch for the category (what the screen shows).
func (o *ThemeOverlay) chipColor(id string) color.RGBA {
	if name, ok := ActiveTheme().Categories[id]; ok {
		if c, ok := swatchRGBA(name); ok {
			return c
		}
	}
	c, _ := themeCategoryByID(id)
	col, _ := swatchRGBA(c.Anchor)
	return col
}

func (o *ThemeOverlay) resetCategory(id string) {
	if o.g == nil {
		return
	}
	if o.scope == "circuit" {
		o.g.clearCircuitThemeCategory(id)
	} else {
		o.g.clearUserThemeCategory(id)
	}
}

// onResetAll is a two-tap confirm (first tap arms the destructive button,
// second within themeResetAllArmMs fires), mirroring the row Delete button.
func (o *ThemeOverlay) onResetAll() {
	now := nowUnixMs()
	if !o.resetAllArmed || now-o.resetAllArmedAt > themeResetAllArmMs {
		o.resetAllArmed, o.resetAllArmedAt = true, now
		o.resetAllBtn.SetSpec(ComponentButtonDestructiveConfirm)
		return
	}
	o.resetAllArmed = false
	o.resetAllBtn.SetSpec(ComponentButtonDestructive)
	if o.g == nil {
		return
	}
	if o.scope == "circuit" {
		o.g.resetCircuitTheme()
	} else {
		o.g.resetUserTheme()
	}
}

func (o *ThemeOverlay) chipSize() int {
	if Profile().IsMobile() && TouchMinTarget() > themeChipSize {
		return TouchMinTarget()
	}
	return themeChipSize
}

// Layout sizes the panel (header + scope pills + override + N rows + reset
// all), clamps it to the pane, and positions every control. Rows scroll when
// the pane is short.
func (o *ThemeOverlay) Layout(anchor, screenBounds image.Rectangle) {
	b := screenBounds
	w := themeOverlayW
	if maxW := b.Dx() - 2*24; maxW > 0 && w > maxW {
		w = maxW
	}
	titleH := StyledTextHeight(cardTitleRole)
	o.headerH = SpaceMD + titleH + SpaceSM
	rowH := themeRowH
	if cs := o.chipSize(); cs+2*SpaceXS > rowH {
		rowH = cs + 2*SpaceXS
	}
	rows := len(themeCategories)
	h := o.headerH + themePillH + SpaceSM + themePillH + SpaceMD + rows*rowH + SpaceMD + themeResetAllH + SpaceMD
	if maxH := b.Dy() - 2*12; maxH > 0 && h > maxH {
		h = maxH
	}
	if o.place != nil {
		o.rect = o.place(w, h)
	} else {
		cx, cy := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
		o.rect = image.Rect(cx-w/2, cy-h/2, cx+w/2, cy-h/2+h)
	}
	x := o.rect.Min.X + cardContentPadX
	y := o.rect.Min.Y + o.headerH
	pillW := themePillW
	if 2*pillW+SpaceMD > o.rect.Dx()-2*cardContentPadX {
		pillW = (o.rect.Dx() - 2*cardContentPadX - SpaceMD) / 2
	}
	o.mineBtn.SetRect(image.Rect(x, y, x+pillW, y+themePillH))
	o.circuitBtn.SetRect(image.Rect(x+pillW+SpaceMD, y, x+2*pillW+SpaceMD, y+themePillH))
	y += themePillH + SpaceSM
	o.overrideBtn.SetRect(image.Rect(x, y, o.rect.Max.X-cardContentPadX, y+themePillH))
	y += themePillH + SpaceMD
	o.sectionY = y
	// Reset-all pinned to the panel bottom; rows scroll between.
	raY := o.rect.Max.Y - SpaceMD - themeResetAllH
	o.resetAllBtn.SetRect(image.Rect(x, raY, o.rect.Max.X-cardContentPadX, raY+themeResetAllH))
	o.rowsView = image.Rect(o.rect.Min.X, y, o.rect.Max.X, raY-SpaceMD)
	vis := o.rowsView.Dy() / rowH
	if vis < 1 {
		vis = 1
	}
	o.scroll.Configure(o.rowsView, rows, vis)
	o.layoutRows(rowH)
}

func (o *ThemeOverlay) layoutRows(rowH int) {
	x := o.rect.Min.X + cardContentPadX
	right := o.rect.Max.X - cardContentPadX - dropdownScrollbarWidth()
	cs := o.chipSize()
	ry := o.rowsView.Min.Y - o.scroll.OffsetPx()
	for _, c := range themeCategories {
		row := image.Rect(x, ry, right, ry+rowH)
		o.rowRects[c.ID] = row
		resetSz := cs
		reset := image.Rect(right-resetSz, ry+(rowH-resetSz)/2, right, ry+(rowH-resetSz)/2+resetSz)
		o.resetBtns[c.ID].SetRect(reset)
		chipX := reset.Min.X - SpaceSM - cs
		o.chipRects[c.ID] = image.Rect(chipX, ry+(rowH-cs)/2, chipX+cs, ry+(rowH-cs)/2+cs)
		ry += rowH
	}
}

// Update re-lays rows while scroll momentum runs (PortalUpdater).
func (o *ThemeOverlay) Update() {
	if o.scroll.UpdateMomentum() {
		o.layoutRows(o.rowHeight())
	}
	if o.resetAllArmed && nowUnixMs()-o.resetAllArmedAt > themeResetAllArmMs {
		o.resetAllArmed = false
		o.resetAllBtn.SetSpec(ComponentButtonDestructive)
	}
}

func (o *ThemeOverlay) rowHeight() int {
	if r, ok := o.rowRects[themeCategories[0].ID]; ok && r.Dy() > 0 {
		return r.Dy()
	}
	return themeRowH
}

// themePanelHandler is the full-panel catch-all + scroll owner (mirrors
// settingsPanelHandler). Chip presses are resolved here by rect so a chip
// needs no Button of its own.
type themePanelHandler struct{ o *ThemeOverlay }

func (h themePanelHandler) OnPress(x, y int) InputResult {
	o := h.o
	if o.scroll.HandleScrollbarDrag(image.Pt(x, y), true, func() { o.layoutRows(o.rowHeight()) }) {
		return InputCaptured
	}
	pt := image.Pt(x, y)
	if pt.In(o.rowsView) {
		for _, c := range themeCategories {
			if pt.In(o.chipRects[c.ID]) {
				if o.onChip != nil {
					o.onChip(c.ID)
				}
				return InputConsumed
			}
		}
	}
	return InputConsumed
}
func (h themePanelHandler) OnDrag(x, y int) {
	h.o.scroll.HandleScrollbarDrag(image.Pt(x, y), true, func() { h.o.layoutRows(h.o.rowHeight()) })
}
func (h themePanelHandler) OnRelease(x, y int) {
	h.o.scroll.HandleScrollbarDrag(image.Pt(x, y), false, nil)
}
func (h themePanelHandler) OnWheel(x, y, steps int) InputResult {
	if h.o.scroll.HandleWheel(steps) {
		h.o.layoutRows(h.o.rowHeight())
	}
	return InputConsumed
}

// HitAreas: every Button first (they win the equal-z stable sort), the
// panel catch-all LAST. Reset buttons are clipped to the rows viewport so a
// scrolled-away row cannot be pressed through the header.
func (o *ThemeOverlay) HitAreas() []HitArea {
	if o.rect.Empty() {
		return nil
	}
	areas := []HitArea{
		{Rect: o.mineBtn.Rect(), ZIndex: ZOverlayMin, Tag: "theme-scope-mine", Handler: &buttonHitAdapter{btn: o.mineBtn}},
		{Rect: o.circuitBtn.Rect(), ZIndex: ZOverlayMin, Tag: "theme-scope-circuit", Handler: &buttonHitAdapter{btn: o.circuitBtn}},
		{Rect: o.overrideBtn.Rect(), ZIndex: ZOverlayMin, Tag: "theme-override", Handler: &buttonHitAdapter{btn: o.overrideBtn}},
		{Rect: o.resetAllBtn.Rect(), ZIndex: ZOverlayMin, Tag: "theme-reset-all", Handler: &buttonHitAdapter{btn: o.resetAllBtn}},
	}
	for _, c := range themeCategories {
		b := o.resetBtns[c.ID]
		if r := b.Rect().Intersect(o.rowsView); !r.Empty() {
			areas = append(areas, HitArea{Rect: r, ZIndex: ZOverlayMin, Tag: "theme-reset-" + c.ID, Handler: &buttonHitAdapter{btn: b}})
		}
	}
	return append(areas, HitArea{Rect: o.rect, ZIndex: ZOverlayMin, Tag: "theme-panel", Handler: themePanelHandler{o: o}})
}

func (o *ThemeOverlay) Draw(screen *ebiten.Image) {
	if screen == nil || o.rect.Empty() {
		return
	}
	drawRect(screen, screen.Bounds(), colScrim, true)
	drawPanel(screen, o.rect)
	dst := screen.SubImage(o.rect).(*ebiten.Image)
	x := o.rect.Min.X + cardContentPadX
	titleH := StyledTextHeight(cardTitleRole)
	drawCardTitleIn(dst, image.Rect(x, o.rect.Min.Y+SpaceMD, o.rect.Max.X-cardContentPadX, o.rect.Min.Y+SpaceMD+titleH), i18n.T(i18n.KeyThemeTitle))
	o.mineBtn.SetToggled(o.scope == "user")
	o.circuitBtn.SetToggled(o.scope == "circuit")
	o.mineBtn.Draw(dst)
	o.circuitBtn.Draw(dst)
	o.overrideBtn.SetToggled(o.g != nil && o.g.userTheme.OverrideCircuits)
	o.overrideBtn.Draw(dst)
	rows := dst.SubImage(o.rowsView).(*ebiten.Image)
	labelH := StyledTextHeight(RoleBody)
	glossH := StyledTextHeight(RoleCaption)
	for _, c := range themeCategories {
		row := o.rowRects[c.ID]
		if !row.Overlaps(o.rowsView) {
			continue
		}
		ty := row.Min.Y + (row.Dy()-labelH-glossH)/2
		DrawTextStyled(rows, i18n.T(c.LabelKey), row.Min.X, ty, RoleBody, colTextPrimary)
		DrawTextStyled(rows, i18n.T(c.GlossKey), row.Min.X, ty+labelH, RoleCaption, colTextSecondary)
		chip := o.chipRects[c.ID]
		set, inherited := o.chipState(c.ID)
		drawRoundedRect(rows, chip, o.chipColor(c.ID), RadiusControl, true)
		switch {
		case set:
			drawRoundedRect(rows, chip.Inset(-2), colTextPrimary, RadiusControl, false) // explicit-in-scope ring
		case inherited:
			drawRoundedRect(rows, chip, colBorderMedium, RadiusControl, false)
		default:
			drawRoundedRect(rows, chip, colBorderSubtle, RadiusControl, false)
		}
		o.resetBtns[c.ID].Draw(rows)
	}
	o.scroll.Draw(dst)
	o.resetAllBtn.Draw(dst)
}
```

`nowUnixMs` — reuse the repo's existing clock helper (`grep -rn 'func nowUnixMs\|time.Now().UnixMilli' *.go | grep -v _test | head`); if none exists, add `func nowUnixMs() int64 { return time.Now().UnixMilli() }` to `theme_overlay.go`. Confirm `dropdownScrollbarWidth()`, `ZOverlayMin`, `DropdownScrollbarStyle`, `cardContentPadX`, `cardTitleRole` names by grep (they are used by `settings_overlay.go`).

`openThemeOverlay` (replace the Task 12 stub):

```go
// openThemeOverlay opens the theme editor portal (grid gear-cluster sibling of
// Settings). Modal like Settings; the panel is anchored under the theme button.
func (dv *DrumView) openThemeOverlay() {
	if dv.tree == nil {
		return
	}
	p := dv.portal()
	if p == nil || p.Has(themeOverlayID) {
		return
	}
	ov := NewThemeOverlay(dv.game)
	if g := dv.game; g != nil {
		ov.SetPositioner(func(w, h int) image.Rectangle {
			return g.gridAnchoredPopupRect(g.gridThemeButtonRect(), w, h, GridAnchorOpts{Avoid: []image.Rectangle{g.gridTopButtonsReservedRect()}})
		})
		ov.onChip = func(id string) { g.openThemeSwatchPicker(ov, id) } // Task 14
	}
	p.Open(PortalEntry{ID: themeOverlayID, Overlay: ov, Modal: true, Scrim: false})
}
```

Until Task 14 exists, define `func (g *Game) openThemeSwatchPicker(ov *ThemeOverlay, id string) {}` as a stub in `theme_overlay.go`. `screenshot_subjects.go`: add `SubjectThemeEditor Subject = "theme_editor"` and a `SubjectRect` case returning `clamp(ov.Rect())` when the top overlay is a `*ThemeOverlay`, else `(image.Rectangle{}, false)`.

- [ ] **Step 4: Run the tests until green**

Run Step 2's command plus `-run 'TestThemeButtonRealClickTogglesOverlay|TestZoneInvisibleEqualsNoInput|TestSettings'`. Expected: PASS. Typical failures and fixes: chip inside panel (`!chip.In(ov.Rect())`) → the height clamp is fine but rows must be laid out after `scroll.Configure`; Reset-all arming → `SetSpec` on the button changes chrome only (label stays "Reset all"). Then run the full ui suite.

- [ ] **Step 5: First screenshot (desktop + mobile), inspect by eye**

Add scene entries to `scene_catalog.go` now (Task 15 keeps them):

```go
	{Name: "theme_editor", Description: "theme editor overlay open (grid palette button)",
		Setup: func(g *Game) { g.toggleThemeOverlay() }},
```

Build + capture:
```
cd /home/ymolinar/Repos/beatmo && CGO_ENABLED=1 .tools/go/bin/go build -o /tmp/beatmo_screenshot ./src/go/cmd
xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene theme_editor -screenshot /tmp/claude-1000/-home-ymolinar-Repos-beatmo/7b2eefc3-d4fa-4245-9c5b-8a8536e60849/scratchpad/theme_editor.png
xvfb-run -a -s "-screen 0 390x844x24" /tmp/beatmo_screenshot -log ERROR -mobile -win-w 390 -win-h 844 -scene theme_editor -screenshot /tmp/claude-1000/-home-ymolinar-Repos-beatmo/7b2eefc3-d4fa-4245-9c5b-8a8536e60849/scratchpad/theme_editor_mobile.png
```
(Check the actual build path of the desktop main package: `grep -n 'beatmo_screenshot' Makefile`.) Open both PNGs with the Read tool. Checklist: title "Theme" present; two scope pills with "My theme" latched; override pill; nine rows each with label + gloss + colored chip + reset icon; "Reset all" at the bottom; nothing overlapping the corner buttons; on mobile the panel fits and chips are finger-sized. Fix and re-capture until it looks right.

---

### Task 14: Chip → swatch picker → live pick, persisted, undoable (end-to-end input)

**Files:**
- Create: `src/go/internal/ui/theme_picker.go` (replaces the `openThemeSwatchPicker` stub)
- Create: `src/go/internal/ui/theme_picker_e2e_test.go`

**Interfaces:**
- Produces: `(*Game).openThemeSwatchPicker(ov *ThemeOverlay, categoryID string)`, `const themePickerID = "theme-swatch"`.
- Consumes: `dv.colorWheelComp` (`*ColorWheelComponent`: `SetProps`, `Open`, `ClearHold`, `Close`, `IsOpen`, `SwatchCells()`, `UpdateKeyNav`), `compPortalOverlay`, `themeOverlayID`, Task 8 setters.

- [ ] **Step 1: Write the failing end-to-end tests**

```go
//go:build test

package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/internal/theme"
)

// swatchCellNamed returns the on-screen cell of the named swatch in the open picker.
func swatchCellNamed(t *testing.T, g *Game, name string) image.Rectangle {
	t.Helper()
	cells := g.drum.colorWheelComp.SwatchCells()
	for i, s := range SuggestedInstrumentSwatches {
		if s.Name == name && i < len(cells) {
			return cells[i]
		}
	}
	t.Fatalf("swatch %q not in picker cells (%d cells)", name, len(cells))
	return image.Rectangle{}
}

// Full path: palette button → overlay → Accent chip → picker → orchid-300
// released → screen recolors, prefs saved, event logged, picker closed,
// overlay still open with the chip ringed.
func TestThemePickAccentEndToEnd(t *testing.T) {
	defer SetThemeSink(nil)
	sink := &fakeThemeSink{}
	SetThemeSink(sink)
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	p := g.drum.portal()

	tap(center(ov.ChipRect("accent")))
	advanceFrames(g, 2)
	if !p.Has(themePickerID) || !g.drum.colorWheelComp.IsOpen() {
		t.Fatal("chip press did not open the swatch picker")
	}
	if !p.Has(themeOverlayID) {
		t.Fatal("theme overlay must stay open under the picker")
	}
	tap(center(swatchCellNamed(t, g, "orchid-300")))
	advanceFrames(g, 3)

	orchid, _ := swatchRGBA("orchid-300")
	if genColorPrimary != orchid {
		t.Fatalf("pick did not apply: primary=%v", genColorPrimary)
	}
	if got := g.UserTheme().Categories["accent"]; got != "orchid-300" {
		t.Fatalf("user theme not updated: %q", got)
	}
	if len(sink.saved) == 0 || sink.saved[len(sink.saved)-1].Categories["accent"] != "orchid-300" {
		t.Fatalf("pick not persisted: %+v", sink.saved)
	}
	if p.Has(themePickerID) || g.drum.colorWheelComp.IsOpen() {
		t.Fatal("picker must close after a pick")
	}
	if !p.Has(themeOverlayID) {
		t.Fatal("overlay must remain open after a pick")
	}
	if set, _ := ov.chipState("accent"); !set {
		t.Fatal("chip must show the explicit ring after the pick")
	}
	// The frame actually paints the new accent.
	oldSafe := envRenderSafe
	envRenderSafe = true
	t.Cleanup(func() { envRenderSafe = oldSafe })
	screen := ebiten.NewImage(1280, 720)
	rects := collectAllFills(t, func() { g.Draw(screen) })
	if rectsWithColorInside(rects, screen.Bounds(), orchid) == 0 {
		t.Fatal("derived accent not painted on the frame after the pick")
	}
}

// Circuit scope: the pick lands in the circuit theme, is undoable, and the
// user theme is untouched.
func TestThemePickCircuitScopeUndoEndToEnd(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	_, circuit := ov.ScopePillRects()
	tap(center(circuit))
	advanceFrames(g, 2)
	tap(center(ov.ChipRect("grid")))
	advanceFrames(g, 2)
	tap(center(swatchCellNamed(t, g, "cyan-300")))
	advanceFrames(g, 3)
	if g.CircuitTheme().Categories["grid"] != "cyan-300" {
		t.Fatalf("circuit theme: %+v", g.CircuitTheme())
	}
	if len(g.UserTheme().Categories) != 0 {
		t.Fatal("user theme must be untouched by a circuit-scope pick")
	}
	before := genColorGridQuarter
	if g.undoManager.UndoLabel() == "" {
		t.Fatal("circuit pick not undoable")
	}
	g.undoManager.Undo()
	advanceFrames(g, 2)
	if genColorGridQuarter == before || !g.CircuitTheme().IsEmpty() {
		t.Fatal("undo did not revert the circuit pick")
	}
}

// Override rule through the UI: with a circuit theme loaded, toggling
// "Always use my theme" flips which accent is on screen.
func TestThemeOverrideToggleFlipsEffectiveThemeEndToEnd(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"accent": "mint-300"}})
	g.setUserThemeCategory("accent", "orchid-300")
	mint, _ := swatchRGBA("mint-300")
	orchid, _ := swatchRGBA("orchid-300")
	if genColorPrimary != mint {
		t.Fatalf("override off: circuit must win, got %v", genColorPrimary)
	}
	tap(center(ov.OverrideRect()))
	advanceFrames(g, 2)
	if genColorPrimary != orchid {
		t.Fatalf("override on: user must win, got %v", genColorPrimary)
	}
}

// Esc closes the picker first, then the overlay (stacked portals).
func TestThemePickerEscapeOrder(t *testing.T) {
	g, ov, tap := openThemeOverlayForTest(t, 1280, 720)
	p := g.drum.portal()
	tap(center(ov.ChipRect("viz")))
	advanceFrames(g, 2)
	pressEscape := func() { // repo pattern (connect_mode_test.go): stub the key for one frame
		restore := stubKeys(nil, map[ebiten.Key]bool{ebiten.KeyEscape: true})
		advanceFrames(g, 1)
		restore()
		advanceFrames(g, 1)
	}
	pressEscape()
	if p.Has(themePickerID) || !p.Has(themeOverlayID) {
		t.Fatal("first Esc must close only the picker")
	}
	pressEscape()
	if p.Has(themeOverlayID) {
		t.Fatal("second Esc must close the overlay")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestThemePick|TestThemeOverrideToggleFlips|TestThemePickerEscapeOrder'`
Expected: FAIL (`themePickerID` undefined / picker never opens).

- [ ] **Step 3: Implement the picker bridge**

```go
// src/go/internal/ui/theme_picker.go
package ui

import (
	"image"
	"image/color"
)

// themePickerID identifies the swatch-picker portal entry opened from a theme
// chip. It is opened MODAL on top of the (modal) theme overlay: the HitIndex
// modal filter only routes to the topmost modal, so a non-modal picker above
// a modal panel would be unclickable. Closing it restores the overlay as the
// modal (OverlayPortal.updateModalState).
const themePickerID = "theme-swatch"

// openThemeSwatchPicker opens the shared swatch grid for a theme category.
// The pick writes the swatch NAME into the overlay's active scope and applies
// live; the picker closes on release (ColorWheelComponent latches `picked`),
// the overlay stays open.
func (g *Game) openThemeSwatchPicker(ov *ThemeOverlay, categoryID string) {
	dv := g.drum
	if dv == nil || dv.tree == nil || dv.colorWheelComp == nil || ov == nil {
		return
	}
	p := dv.portal()
	if p == nil {
		return
	}
	if p.Has(themePickerID) {
		p.Close(themePickerID)
	}
	anchor := ov.ChipRect(categoryID)
	scope := ov.Scope()
	dv.colorWheelComp.SetProps(ColorWheelProps{
		AnchorRect:   anchor,
		Bounds:       g.split.GridRect(g.winW, g.winH),
		RowHeight:    dv.rowHeight(),
		CurrentColor: ov.chipColor(categoryID),
		OnColorPick: func(c color.Color) {
			name, ok := swatchNameOf(c)
			if !ok {
				return
			}
			if scope == "circuit" {
				g.setCircuitThemeCategory(categoryID, name)
			} else {
				g.setUserThemeCategory(categoryID, name)
			}
		},
		OnClose: func() {},
	})
	dv.colorWheelComp.Open()
	dv.colorWheelComp.ClearHold()
	p.Open(PortalEntry{
		ID: themePickerID,
		Overlay: &compPortalOverlay{comp: dv.colorWheelComp, tag: "theme-swatch", updateFn: dv.colorWheelComp.UpdateKeyNav},
		Modal:  true,
		Scrim:  false,
		Anchor: anchor,
		OnClose: func() {
			if dv.colorWheelComp != nil && dv.colorWheelComp.IsOpen() {
				dv.colorWheelComp.Close()
			}
		},
	})
}

// swatchNameOf maps a picked color back to its curated swatch name (RGB only;
// the picker returns opaque palette colors).
func swatchNameOf(c color.Color) (string, bool) {
	r, gg, b, _ := c.RGBA()
	for _, s := range SuggestedInstrumentSwatches {
		if s.RGBA.R == uint8(r>>8) && s.RGBA.G == uint8(gg>>8) && s.RGBA.B == uint8(b>>8) {
			return s.Name, true
		}
	}
	return "", false
}

var _ = image.Rectangle{}
```

Remove the stub from `theme_overlay.go`. `ColorWheelProps.RowHeight` is only used for sizing; `dv.rowHeight()` exists. Check `PortalEntry` has `Anchor` and `OnClose` fields (it does — see `drumview_portal_open.go:176-195`).

- [ ] **Step 4: Run the tests until green**

Run Step 2's command. Likely issues: (a) the picker's `Bounds` — if the panel is clamped into the drum pane, pass the grid rect as above; (b) the modal filter — verify with `TestThemePickAccentEndToEnd` that the swatch tap reaches the picker; if not, confirm `Modal: true` on the picker entry; (c) after the pick, the chip ring requires `ActiveTheme()`/`scopeDoc()` to reflect the new state — they read Game state live, so no extra plumbing. Then run the whole ui suite.

- [ ] **Step 5: Screenshot with the picker open + a themed full UI, inspect**

Add scenes (Task 15 keeps them):

```go
	{Name: "theme_editor_picker", Description: "theme editor with the Accent swatch picker open",
		Setup: func(g *Game) {
			g.toggleThemeOverlay()
			if ov, ok := g.drum.portal().TopOverlay().(*ThemeOverlay); ok {
				g.openThemeSwatchPicker(ov, "accent")
			}
		}},
	{Name: "theme_applied_orchid", Description: "whole UI recolored: accent orchid-300, background electric-blue-500, viz mint-300 (Synth tab open)",
		Setup: func(g *Game) {
			g.SetCircuitTheme(theme.Doc{Categories: map[string]string{"accent": "orchid-300", "background": "electric-blue-500", "viz": "mint-300"}})
			g.drum.eqPanelZone.SetActiveTab(TabSynth) // reuse the existing synth-tab scene's setup line if it differs
		}},
```

Capture `theme_editor_picker` and `theme_applied_orchid` on desktop and mobile as in Task 13 Step 5 and open the PNGs. Checklist: picker anchored to the Accent chip, current swatch ringed, panel behind still visible; themed scene shows purple accent on transport buttons and knobs, blue-tinted background, mint analyzer traces, red/green semantic buttons UNCHANGED, text legible. Fix and re-capture until right.

---

### Task 15: Scenes as regression surfaces, crop subject, docs, final verification

**Files:**
- Modify: `src/go/internal/ui/scene_catalog.go` (keep the three scenes from Tasks 13/14; add the crop scene)
- Modify: `DESIGN.md` (prose subsection), `CLAUDE.md` (theme section, standing tables)
- Modify: `src/go/internal/ui/zone_input_isolation_discipline_test.go` only if it enumerates overlays (it enumerates Zones; overlays are not zones — verify and leave alone if so).

- [ ] **Step 1: Crop scene + crop test**

```go
	{Name: "crop_theme_editor", Description: "theme editor panel, subject-cropped", Subject: SubjectThemeEditor,
		Setup: func(g *Game) { g.toggleThemeOverlay() }},
```

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSceneCrop|TestScene'`
Expected: PASS (`scene_crop_test.go` exercises every `crop_*` scene; the subject rect must be non-empty and smaller than the screen).

- [ ] **Step 2: Batch screenshot pass, both platforms, inspected**

Run: `make screenshots-all SCENES=theme_editor,theme_editor_picker,theme_applied_orchid,crop_theme_editor` and the mobile pass per `reference_mobile_single_scene_screenshot` (`-mobile -win-w 390 -win-h 844`, portrait). Open every PNG. Confirm: labels present and legible, chips colored, no overlap with the corner buttons, picker anchored, themed scene coherent. Fix and re-capture until right.

- [ ] **Step 3: Docs**

DESIGN.md — after "Chrome accents — three load-bearing invariants" add:

```markdown
### Runtime themes (user / circuit color schemes)

The invariants above describe the DEFAULT theme and the STRUCTURE every theme
keeps. At runtime a theme document (`internal/theme.Doc`: nine category →
swatch-name picks plus an optional token → hex escape hatch) recolors the
palette in place (`ui/theme_runtime.go`): every token keeps its HSL offset from
its category's default anchor, dark categories keep their absolute lightness,
semantic reds/greens are fixed. So "two chrome accents", "panel state never
recolors chrome", and "instrument identity is the only multi-color signal" all
hold under every theme — the accent is simply a different hue. Membership:
`ui/theme_categories.go` (guard: `TestThemeCategoryTableComplete`). The
default theme is the identity (`TestThemeDefaultIsIdentity`), which is why the
drift/discipline tests keep comparing against these YAML values.
```

CLAUDE.md — add a "Runtime theme (Theme editor)" subsection under Architecture: the rebuild-in-place model, the three rebuild steps and their order (specs → aliases), `OnThemeChanged` as the cache chokepoint, the precedence rule, where the user theme and circuit theme persist, the two hooks kinds, and the rule "a new package-level color derivation must live in `rebuildThemeAliases` (guard `TestNoTopLevelDerivedColorsOutsideTheme`)". Update the event-tag table with `theme`. Update `NumNonVerbose` mention (74 → 76). Add the three scenes to the visual-verification examples if a list exists.

- [ ] **Step 4: Full verification**

Run, in order, and paste the outputs into the final report:

```
cd src/go && ../../.tools/go/bin/go build ./... && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build -o /dev/null ./cmd/wasm
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/theme/ ./internal/userprefs/ ./internal/hooks/ ./internal/eventlogger/ ./internal/log/ ./internal/i18n/ ./internal/assets/ ./internal/ui/ ./cmd/gen_design_tokens/
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot|TestNoPillShapedControls|TestNoUppercaseInfoTags|TestComponentSpecsDrift' ./internal/ui/ ./internal/log/
make gen-design-tokens && git diff --stat src/go/internal/ui/*.gen.go   # must be empty (generated files are current)
GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
```

(Adjust the wasm main package path to the one the Makefile's `wasm` target builds.) Expected: everything PASS; `git diff --stat` on the gen files empty. If a browser test needs a theme export in the catalogue, none was added (no new JS export), so the smoke suite is unchanged.

- [ ] **Step 5: Report**

`git status --short` and `git diff --stat`; list every new file; attach the inspected screenshots' paths. Do not commit.
