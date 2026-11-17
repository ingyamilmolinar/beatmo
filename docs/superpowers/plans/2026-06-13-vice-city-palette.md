# Vice City Palette Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

> **⚠️ REPO POLICY — DO NOT COMMIT.** This repository has a standing instruction: never `git add`/`git commit`/`git stash`/`git checkout`/`git restore`/`git reset`. Leave every change in the working tree; the user owns all commits. The "Checkpoint" steps below therefore **run verification only** — they never commit. To revert a change, re-edit the lines back manually with the Edit tool; never use git to undo.

**Goal:** Make a single authentic GTA-Vice-City color palette the only source of color in the UI (repaint all 111 DESIGN.md color tokens + a build-failing membership guard), and add a grouped gradient color picker to the instrument-row ellipsis menu.

**Architecture:** DESIGN.md YAML front matter is the single source of color; `make gen-design-tokens` emits `genColor*` constants; `theme.go` aliases (`col*`) and every call site already read named tokens (the `TestTokenDiscipline` ratchet guarantees no stray literals). So the work is: (1) declare a 35-color Vice City ramp set + neon-night neutrals + semantic aliases in DESIGN.md; (2) re-point all existing tokens onto palette members; (3) add a `TestPaletteMembership` guard that rejects any off-palette token; (4) re-expose + upgrade the existing scene-only `ColorWheelComponent` as a grouped gradient picker. Colors-only — no geometry/layout/typography/spacing changes.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), Ebiten, `cmd/gen_design_tokens` codegen, `-tags test -modfile=go.test.mod` fast test path.

**Spec:** `docs/superpowers/specs/2026-06-13-vice-city-palette-design.md`

**Conventions used throughout:**
- Fast test run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`
- Regenerate tokens after any DESIGN.md change: `make gen-design-tokens` (from repo root)
- Bundled gofmt: `.tools/go/bin/gofmt -w <file>`

---

## File Structure

**Created:**
- `src/go/internal/ui/palette_membership_test.go` — the off-palette guard (the lock).
- `src/go/internal/ui/color_contrast_test.go` — WCAG contrast guard for semantic pairings.
- `src/go/internal/ui/color_picker_menu_test.go` — functional test: kebab → Color → pick → recolor + undo.

**Modified:**
- `DESIGN.md` — YAML front matter: add 35 ramp tokens + neon-night neutrals; repoint semantic tokens + all 111 color values; swap `instrumentSwatches`/`instrumentSequence`/`instrumentDefaults`/`instrumentFallbackPalette`; add "Color usage" prose.
- `src/go/internal/ui/design_md_drift_test.go` — want-maps updated to repainted values.
- `src/go/internal/ui/theme.go` — only where a `col*` alias names a semantic token that must point at a different `gen*` member (e.g. `colSplitterHandle` no longer needs the exception comment). Most aliases unchanged.
- `src/go/internal/ui/drumview_context_menu.go` — add "Color" item to `contextMenuItems`.
- `src/go/internal/ui/drumview_colors.go` — add `SetRowColorManual` (duplicate-allowed manual path).
- `src/go/internal/ui/drumview_overlay_color_comp.go` — grouped 7×5 gradient layout + ramp source + family labels.
- `src/go/internal/ui/drumview_portal_open.go` — anchor the picker to the kebab/label when opened from the menu (already opens via `openColorWheelPortal`; wire `colorMenuRow`).
- `src/go/internal/ui/color_features_test.go`, `vice_city_palette_compliance_test.go`, `instrument_palette_test.go` — re-point expectations to the new set + manual-pick-wins.
- `src/go/internal/assets/startup_demo.json` + `assets/templates/*.json` — re-sequenced on-palette colors (via `ResequenceRowColors` flow; only if drift tests demand).
- `src/go/internal/ui/testdata/design_md_lint.expected.json` — regenerated snapshot.

---

## PHASE 1 — Palette foundation (no visual change yet)

### Task 1: Declare the 35 Vice City ramps + neon-night neutrals in DESIGN.md

**Files:**
- Modify: `DESIGN.md` (YAML `colors:` block, starts line 10)

- [ ] **Step 1: Add the ramp + neutral tokens to the `colors:` block**

Insert these keys at the top of `colors:` (right after `colors:` on line 10, before `primary:`). Keep all existing keys for now — this task only *adds* the new vocabulary; Task 4 repaints the old keys.

```yaml
  # ── Vice City chromatic ramps (35) — the single source of all hue.
  # Seven hue families × five shades, light→deep (-100..-500). Family order
  # is itself a continuous neon spectrum. hot-pink-400 (#EE00DD) is the
  # GTA Vice City signature. See docs/superpowers/specs/2026-06-13-vice-city-palette-design.md.
  sunset-gold-100: "#FFE98A"
  sunset-gold-200: "#FFD319"
  sunset-gold-300: "#FFB30A"
  sunset-gold-400: "#FF9E1F"
  sunset-gold-500: "#F58A12"
  tangerine-100: "#FFC299"
  tangerine-200: "#FF9E54"
  tangerine-300: "#FF7A3D"
  tangerine-400: "#FF5C2A"
  tangerine-500: "#E84A1F"
  hot-pink-100: "#FFA8E0"
  hot-pink-200: "#FF6AD5"
  hot-pink-300: "#FF2D9E"
  hot-pink-400: "#EE00DD"
  hot-pink-500: "#C400B5"
  orchid-100: "#E0A8FF"
  orchid-200: "#C24FFF"
  orchid-300: "#A030FF"
  orchid-400: "#8C1EFF"
  orchid-500: "#6E12E0"
  electric-blue-100: "#A8C8FF"
  electric-blue-200: "#5E8AFF"
  electric-blue-300: "#3A6AF0"
  electric-blue-400: "#2A5FD6"
  electric-blue-500: "#1E47B5"
  cyan-100: "#9AF0FF"
  cyan-200: "#3FE0E8"
  cyan-300: "#00C8E0"
  cyan-400: "#00A8C9"
  cyan-500: "#0E7391"
  mint-100: "#B3F5C2"
  mint-200: "#6BE89A"
  mint-300: "#3FD67A"
  mint-400: "#2BC85C"
  mint-500: "#1FA84A"
  # ── Neon-night neutrals (violet-tinted; the structural ramp).
  night-bg: "#120A1C"
  night-surface-1: "#1C1230"
  night-surface-2: "#281A40"
  night-surface-3: "#382354"
  night-surface-overlay: "#1F1438"
  night-text-hi: "#FDF2FF"
  night-text-mid: "#C9B8D6"
  night-text-dim: "#8A7A9C"
```

(The achromatic anchors `#FFFFFF`/`#000000` are already present as `border`/`dim-black`; no new key needed.)

- [ ] **Step 2: Regenerate tokens**

Run: `make gen-design-tokens`
Expected: succeeds; `src/go/internal/ui/design_tokens.gen.go` now contains `genColorSunsetGold100` … `genColorMint500`, `genColorNightBg`, etc. The schema (`scripts/gen_design_tokens/schema.json`) uses open `patternProperties` for color keys, so **no schema edit is required**.

- [ ] **Step 3: Verify it compiles**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/`
Expected: builds clean (TestDesignMDDrift will fail until Task 2 — that's expected and handled there).

- [ ] **Step 4: Checkpoint (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | head`
Leave changes in the working tree.

### Task 2: Extend the drift want-map for the new tokens

**Files:**
- Modify: `src/go/internal/ui/design_md_drift_test.go` (the `wantColors` map)

- [ ] **Step 1: Run the drift test to see exactly what it wants**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDesignMDDrift ./internal/ui/ 2>&1 | head -40`
Expected: FAIL listing the new tokens present in DESIGN.md but missing from the Go want-map (the test compares YAML ↔ generated constants).

- [ ] **Step 2: Add the 43 new tokens to the want-map**

In `design_md_drift_test.go`, find the color want-map (a `map[string]color.RGBA` keyed by token name) and add an entry per new token, e.g.:

```go
"sunset-gold-100":      {0xFF, 0xE9, 0x8A, 0xFF},
"sunset-gold-200":      {0xFF, 0xD3, 0x19, 0xFF},
// … all 35 ramp members …
"mint-500":             {0x1F, 0xA8, 0x4A, 0xFF},
"night-bg":             {0x12, 0x0A, 0x1C, 0xFF},
"night-surface-1":      {0x1C, 0x12, 0x30, 0xFF},
"night-surface-2":      {0x28, 0x1A, 0x40, 0xFF},
"night-surface-3":      {0x38, 0x23, 0x54, 0xFF},
"night-surface-overlay":{0x1F, 0x14, 0x38, 0xFF},
"night-text-hi":        {0xFD, 0xF2, 0xFF, 0xFF},
"night-text-mid":       {0xC9, 0xB8, 0xD6, 0xFF},
"night-text-dim":       {0x8A, 0x7A, 0x9C, 0xFF},
```

(Copy the hex values directly from Task 1 Step 1.)

- [ ] **Step 3: Run the drift test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDesignMDDrift ./internal/ui/`
Expected: PASS.

- [ ] **Step 4: Checkpoint (no commit)** — leave changes in the working tree.

### Task 3: Add `TestPaletteMembership` — the off-palette lock

**Files:**
- Create: `src/go/internal/ui/palette_membership_test.go`

- [ ] **Step 1: Write the failing test**

This test parses DESIGN.md's `colors:` block and asserts every value is in the allowed set. The allowed set is the single canonical list of legal colors.

```go
package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// allowedPaletteHex is the COMPLETE set of colors any DESIGN.md `colors:` token
// may take: the 35 Vice City ramp members, the neon-night neutrals, and the two
// achromatic anchors (white/black, used only via alpha). Adding a color here is
// a deliberate palette decision — do NOT widen it to make a rogue token pass.
var allowedPaletteHex = map[string]bool{
	// sunset-gold
	"FFE98A": true, "FFD319": true, "FFB30A": true, "FF9E1F": true, "F58A12": true,
	// tangerine
	"FFC299": true, "FF9E54": true, "FF7A3D": true, "FF5C2A": true, "E84A1F": true,
	// hot-pink
	"FFA8E0": true, "FF6AD5": true, "FF2D9E": true, "EE00DD": true, "C400B5": true,
	// orchid
	"E0A8FF": true, "C24FFF": true, "A030FF": true, "8C1EFF": true, "6E12E0": true,
	// electric-blue
	"A8C8FF": true, "5E8AFF": true, "3A6AF0": true, "2A5FD6": true, "1E47B5": true,
	// cyan
	"9AF0FF": true, "3FE0E8": true, "00C8E0": true, "00A8C9": true, "0E7391": true,
	// mint
	"B3F5C2": true, "6BE89A": true, "3FD67A": true, "2BC85C": true, "1FA84A": true,
	// neon-night neutrals
	"120A1C": true, "1C1230": true, "281A40": true, "382354": true, "1F1438": true,
	"FDF2FF": true, "C9B8D6": true, "8A7A9C": true,
	// achromatic anchors (alpha-only)
	"FFFFFF": true, "000000": true,
}

var colorsLineRe = regexp.MustCompile(`^\s*([a-z][a-z0-9-]*):\s*"#([0-9A-Fa-f]{6})([0-9A-Fa-f]{2})?"`)

// designMDPath walks up from the test's cwd to the repo root DESIGN.md.
func designMDPath(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		p := filepath.Join(dir, "DESIGN.md")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("DESIGN.md not found walking up from cwd")
	return ""
}

func TestPaletteMembership(t *testing.T) {
	raw, err := os.ReadFile(designMDPath(t))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(raw), "\n")
	// Only scan the `colors:` block: from the line `colors:` to the next
	// top-level key (a line starting at column 0 that is not a comment).
	inColors := false
	checked := 0
	for _, ln := range lines {
		if strings.HasPrefix(ln, "colors:") {
			inColors = true
			continue
		}
		if inColors && len(ln) > 0 && ln[0] != ' ' && ln[0] != '#' && ln[0] != '\t' {
			break // left the colors: block
		}
		if !inColors {
			continue
		}
		m := colorsLineRe.FindStringSubmatch(ln)
		if m == nil {
			continue
		}
		name, hex := m[1], strings.ToUpper(m[2])
		checked++
		if !allowedPaletteHex[hex] {
			t.Errorf("color token %q = #%s is OFF-PALETTE. Every colors: token must be a Vice City palette member (see allowedPaletteHex / the design spec). Repoint it to the nearest legal member.", name, hex)
		}
	}
	if checked < 100 {
		t.Fatalf("only scanned %d color tokens; expected the full colors: block (~111). Parser/anchor drift?", checked)
	}
}
```

- [ ] **Step 2: Run it — expect FAIL (old tokens still off-palette)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestPaletteMembership ./internal/ui/ 2>&1 | head -30`
Expected: FAIL — dozens of off-palette tokens listed (`primary = #4FB4FF is OFF-PALETTE`, etc.). This is correct: the lock works and Phase 1 hasn't repainted yet. **Do not "fix" by widening `allowedPaletteHex`.** Task 4 repaints the tokens.

- [ ] **Step 3: Checkpoint (no commit)** — leave the failing test in place; it goes green at end of Task 4.

### Task 4: Repaint all existing color tokens onto the palette

**Files:**
- Modify: `DESIGN.md` (every existing `colors:` value)
- Modify: `src/go/internal/ui/design_md_drift_test.go` (want-map for repainted tokens)
- Possibly: `src/go/internal/ui/theme.go` (only comments / the splitter "exception" note)

This is a mechanical value-swap. Token *names* and call sites stay; only hex values change. Use the mapping table in the spec (§3–§4). Concrete assignments:

- [ ] **Step 1: Repaint semantic + surface + text + border tokens**

Apply (Edit each value in `DESIGN.md` `colors:`):

```yaml
  primary:              "#00C8E0"   # cyan-300
  primary-bright:       "#3FE0E8"   # cyan-200
  primary-dim:          "#00A8C9"   # cyan-400
  background:           "#120A1C"   # night-bg
  surface-1:            "#1C1230"
  surface-2:            "#281A40"
  surface-3:            "#382354"
  surface-overlay:      "#1F1438"
  on-surface:           "#FDF2FF"   # night-text-hi
  on-surface-muted:     "#C9B8D6"   # night-text-mid
  on-surface-disabled:  "#8A7A9C"   # night-text-dim
  on-surface-accent:    "#00C8E0"   # cyan-300 (matches primary)
  border:               "#FFFFFF"   # unchanged anchor
  success:              "#3FD67A"   # mint-300
  error:                "#FF5C2A"   # tangerine-400
  mute:                 "#C400B5"   # hot-pink-500 (was rust)
  destructive:          "#E84A1F"   # tangerine-500
  destructive-border:   "#FF5C2A"   # tangerine-400
  record-idle:          "#E84A1F"
  record-active:        "#FF5C2A"
```

- [ ] **Step 2: Repaint viz/data tokens (drives every audio-panel tab)**

```yaml
  viz-bg:               "#120A1C"
  viz-bar:              "#00C8E0"   # cyan-300
  viz-bar-peak:         "#9AF0FF"   # cyan-100
  viz-curve:            "#00C8E0"
  viz-wave-a:           "#3FE0E8"   # cyan-200
  viz-wave-b:           "#8A7A9C"   # text-dim
  viz-meter-green:      "#3FD67A"   # mint-300
  viz-meter-yellow:     "#FFD319"   # sunset-gold-200
  viz-meter-red:        "#FF5C2A"   # tangerine-400
  viz-meter-bg:         "#1C1230"
  viz-meter-clip:       "#E84A1F"   # tangerine-500
  viz-bass:             "#FF7A3D"   # tangerine-300
  viz-mids:             "#C9B8D6"   # text-mid
  viz-treble:           "#3A6AF0"   # electric-blue-300
  viz-glow:             "#9AF0FF"   # cyan-100
  viz-scope-trace-a:    "#00C8E0"
  viz-scope-trace-b:    "#3FE0E8"
  viz-scope-trace-diff: "#3FD67A"   # mint-300
  viz-scope-bg:         "#120A1C"
  viz-scope-trigger:    "#FFD319"   # sunset-gold-200
  viz-spectrum-peak-marker: "#FDF2FF"
```

- [ ] **Step 3: Repaint the long tail (remaining ~60 tokens)**

For every remaining token in `colors:`, repoint to the nearest legal member. Guidance:
- Any blue/azure chrome → `cyan-300 #00C8E0` (or `cyan-400`/`cyan-200` for dim/bright).
- Any amber/timeline → sunset-gold/tangerine ramp (`timeline-view #FF9E1F`, `timeline-view-hi #FFB30A`, `timeline-cursor #FFD319`, `timeline-beat #8A7A9C`).
- Any gray/slate fill (`slider-track-fill`, `disabled-fill`, `stepper-fill`, `sidebar-*`, `node-fill`, `grid-*`) → nearest night surface (`#1C1230`/`#281A40`/`#382354`). The `grid-line/half/quarter/eighth/sixteenth/thirty-second` gradation collapses onto `night-surface-1→3` (subtle stepping survives via the existing per-line draw alpha; colors-only, acceptable).
- `splitter-handle` → `#00C8E0` (cyan-300; now on-palette). In `theme.go` delete the "exception to single-chrome-accent" comment on `colSplitterHandle` if present.
- Reds: `stop-desktop-fill`/`eq-mute-active-fill`/`destructive-confirm-*`/`menu-delete-tint`/`viz-cancel-red`/`viz-debug-node` → `#E84A1F` or `#FF5C2A`.
- Greens: `play-desktop-*`/`viz-confirm-green` → `#3FD67A`/`#2BC85C`.
- `drum-stripe-even/odd`, `drum-cell-off`, `drum-cell-highlight` → night surfaces + `#FDF2FF` highlight; `drum-glow` → `#00C8E0`.
- `focus-ring`/`slider-thumb-fill` → `#FDF2FF`; `slider-thumb-shadow`/`divider-shadow`/`dim-black` → `#000000`.
- `node-border`/`edge-color` → `#8A7A9C`.

Iterate: run the membership test (Step 5) — it names every remaining off-palette token and you repoint until clean.

- [ ] **Step 4: Update the drift want-map for every repainted token**

Run the drift test; it prints exact `got vs want` per token. Update each changed entry in `wantColors` to the new hex (mechanical, mirror the values from Steps 1–3).

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDesignMDDrift ./internal/ui/ 2>&1 | head -60`
Repeat edit→run until PASS.

- [ ] **Step 5: Run the membership lock — expect PASS**

Run: `make gen-design-tokens && cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestPaletteMembership|TestDesignMDDrift' ./internal/ui/`
Expected: PASS — no token is off-palette.

- [ ] **Step 6: Checkpoint (no commit)** — leave changes in the working tree.

### Task 5: Add `TestColorContrast` (WCAG guard)

**Files:**
- Create: `src/go/internal/ui/color_contrast_test.go`

- [ ] **Step 1: Write the test**

```go
package ui

import (
	"image/color"
	"math"
	"testing"
)

// relLum returns WCAG relative luminance of an opaque color.
func relLum(c color.Color) float64 {
	r, g, b, _ := c.RGBA()
	lin := func(v uint32) float64 {
		s := float64(v) / 65535.0
		if s <= 0.03928 {
			return s / 12.92
		}
		return math.Pow((s+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b)
}

func contrastRatio(a, b color.Color) float64 {
	la, lb := relLum(a), relLum(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func TestColorContrast(t *testing.T) {
	// Text/icon on surfaces must clear 4.5:1.
	surfaces := []color.Color{genColorBackground, genColorSurface1, genColorSurface2, genColorSurface3}
	for _, s := range surfaces {
		if r := contrastRatio(genColorOnSurface, s); r < 4.5 {
			t.Errorf("text-hi on surface %v contrast %.2f < 4.5", s, r)
		}
	}
	// Dark label (#120A1C) on each neon fill must clear 3:1 (graphical/large).
	dark := genColorBackground
	neons := map[string]color.Color{
		"primary": genColorPrimary, "focus": genColorHotPink300,
		"play": genColorSuccess, "warning": genColorWarning,
		"error": genColorError,
	}
	for name, n := range neons {
		if r := contrastRatio(dark, n); r < 3.0 {
			t.Errorf("dark-on-%s fill contrast %.2f < 3.0", name, r)
		}
	}
}
```

Note: if `genColorWarning`/`genColorHotPink300` aliases don't exist yet, reference the concrete token (`genColorSunsetGold200`, `genColorHotPink300`). Adjust names to whatever `make gen-design-tokens` emitted (check `design_tokens.gen.go`).

- [ ] **Step 2: Run it**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestColorContrast ./internal/ui/`
Expected: PASS. If a pairing fails, the chosen member is too mid-toned for that role — pick a lighter/darker ramp shade for that semantic token in DESIGN.md and re-run Task 4 Step 4.

- [ ] **Step 3: Checkpoint (no commit).**

### Task 6: Add the "Color usage" normative prose to DESIGN.md

**Files:**
- Modify: `DESIGN.md` (prose section after the YAML, near the existing "Semantic invariants" block)

- [ ] **Step 1: Add a "Color usage (Vice City)" subsection** documenting the §7 contrast rules verbatim from the spec (two safe patterns; ≥4.5:1 text / ≥3:1 graphical; controls dark-on-neon or ghost-outline; pale swatches get a 1px border; glow is decoration not contrast; cyan+hot-pink two-accent replaces the old single-azure invariant; the three reds → vermillion/ember/magenta).

- [ ] **Step 2: Regenerate the lint snapshot**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDesignMDLintSnapshot ./internal/ui/ 2>&1 | head`
If it fails on changed warnings, update `src/go/internal/ui/testdata/design_md_lint.expected.json` to the new snapshot the test prints, then re-run to PASS. (Skips if `npx` unavailable — acceptable.)

- [ ] **Step 3: Checkpoint (no commit).**

### Task 7: Phase-1 full-suite verification

- [ ] **Step 1: Run the UI + design guards**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestDesignMDDrift|TestTokenDiscipline|TestPaletteMembership|TestColorContrast|TestDesignMDLintSnapshot' ./internal/ui/`
Expected: PASS.

- [ ] **Step 2: Build the whole module**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./...`
Expected: clean.

- [ ] **Step 3: Checkpoint (no commit).**

---

## PHASE 2 — Instrument swatches + sequence (the data colors)

### Task 8: Swap instrument swatches/sequence/defaults to the 35

**Files:**
- Modify: `DESIGN.md` (`instrumentSwatches:` L1384, `instrumentSequence:` L1426, `instrumentDefaults:` L1341, `instrumentFallbackPalette:` L1359)
- Modify: `src/go/internal/ui/instrument_palette_test.go`, `vice_city_palette_compliance_test.go`, `color_features_test.go` (expectations)

- [ ] **Step 1: Replace `instrumentSwatches:` with the 35 ramp swatches**

```yaml
instrumentSwatches:
  - {name: sunset-gold-100, hex: "#FFE98A"}
  - {name: sunset-gold-200, hex: "#FFD319"}
  # … all 35, names matching the colors: ramp keys, hexes identical …
  - {name: mint-500, hex: "#1FA84A"}
```

- [ ] **Step 2: Replace `instrumentSequence:` with a family-alternating walk**

```yaml
instrumentSequence:
  - hot-pink-400
  - cyan-300
  - sunset-gold-200
  - orchid-300
  - mint-300
  - electric-blue-300
  - tangerine-300
  - hot-pink-200
```

Invariant: consecutive entries are from different families (keeps adjacent rows distinct). Each name must exist in `instrumentSwatches:`.

- [ ] **Step 3: Repoint `instrumentDefaults:` + `instrumentFallbackPalette:` to on-palette hexes**

```yaml
instrumentDefaults:
  - {id: kick,  color: "#FF7A3D"}   # tangerine-300
  - {id: snare, color: "#EE00DD"}   # hot-pink-400
  - {id: hihat, color: "#FFD319"}   # sunset-gold-200
  - {id: tom,   color: "#8C1EFF"}   # orchid-400
  - {id: clap,  color: "#00C8E0"}   # cyan-300
instrumentFallbackPalette:
  - {color: "#3FD67A"}   # mint-300
  - {color: "#FF6AD5"}   # hot-pink-200
  - {color: "#2A5FD6"}   # electric-blue-400
  - {color: "#C24FFF"}   # orchid-200
  - {color: "#FFB30A"}   # sunset-gold-300
```

- [ ] **Step 4: Regenerate + run the membership + drift guards**

Run: `make gen-design-tokens && cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestPaletteMembership|TestDesignMDDrift' ./internal/ui/`
Expected: PASS (all swatch/default/fallback hexes are palette members).

- [ ] **Step 5: Fix compliance test expectations**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestInstrument|TestViceCity|TestTemplate|TestStartupDemo' ./internal/ui/ 2>&1 | head -40`
Update each failing expectation to the new swatches/sequence (e.g. `TestInstrumentSwatchesParity` compares `SuggestedInstrumentSwatches` to the YAML — it auto-follows; failures are in tests that hard-code old hexes). Re-run to PASS.

- [ ] **Step 6: Checkpoint (no commit).**

### Task 9: Re-sequence startup demo + templates if drift demands

**Files:**
- Modify (only if a test fails): `src/go/internal/assets/startup_demo.json`, `assets/templates/*.json`

- [ ] **Step 1: Run the template/demo color tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestStartupDemoColorsAreSequential|TestTemplateColors' ./internal/ui/ ./internal/templates/ 2>&1 | head -40`
Expected: may FAIL because baked JSON colors are old swatches.

- [ ] **Step 2: Regenerate the baked colors**

The colors derive from `instrumentSequence` via `ResequenceRowColors`/`templates.InstrumentSequence`. If the JSON stores raw hexes, update each instrument's `color` field to `instrumentSequence[N % len]`'s new hex (row order). For `startup_demo.json`, the buildDemo path calls `ResequenceRowColors` at load, so the test asserting on-palette sequential should pass after Task 8; only hand-edit JSON if a test explicitly compares stored bytes.

- [ ] **Step 3: Re-run to PASS, Checkpoint (no commit).**

### Task 10: Phase-2 verification + visual smoke

- [ ] **Step 1: Full UI fast suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -20`
Expected: PASS (modulo the pre-existing failures documented in MEMORY for this branch — compare against a baseline run on HEAD before blaming this diff).

- [ ] **Step 2: Capture a screenshot to eyeball the repaint**

Run: `make screenshot` (output → `screenshots/`)
Expected: surfaces are violet-black; accents cyan/pink; instrument rows neon. Audio panel tabs recolored. No layout shift.

- [ ] **Step 3: Checkpoint (no commit).**

---

## PHASE 3 — Audio-panel token audit (colors-only confirmation)

### Task 11: Confirm every audio tab is token-sourced and unchanged in geometry

**Files:**
- Read/verify: `render_waveform.go`, `render_spectrum.go`, `render_meters.go`, `render_scope.go`, `render_eq_curve.go`, `eq_panel_zone.go`, `chain_panel_zone.go`, `synth_panel_zone.go`

- [ ] **Step 1: Grep each renderer for non-token color usage**

Run: `cd src/go && grep -nE 'color\.(RGBA|NRGBA)\{' internal/ui/render_*.go internal/ui/eq_panel_zone.go internal/ui/chain_panel_zone.go internal/ui/synth_panel_zone.go`
Expected: no matches (all are budget-0 in `token_discipline_test.go`). If any match appears, route it through a `gen*`/`col*` token and decrement the file's budget in `token_discipline_test.go`.

- [ ] **Step 2: Run the audio-panel pixel tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'AudioPanelRender|Spectrum|Meters|Scope|Waveform' ./internal/ui/`
Expected: PASS. These assert data-trace pixels render (color counts) — geometry unchanged, only hues differ. If a test pins an exact old hex, update it to the repainted token value.

- [ ] **Step 3: Checkpoint (no commit).**

---

## PHASE 4 — The color picker feature

### Task 12: Add the manual (duplicate-allowed) color setter

**Files:**
- Modify: `src/go/internal/ui/drumview_colors.go`
- Test: `src/go/internal/ui/color_features_test.go`

- [ ] **Step 1: Write the failing test**

```go
func TestSetRowColorManualAllowsDuplicate(t *testing.T) {
	dv := newTestDrumViewWithRows(t, 2) // helper already used in color_features_test.go
	want := dv.Rows[0].Color
	dv.SetRowColorManual(1, want) // deliberately duplicate row 0's color
	if dv.colorKey(dv.Rows[1].Color) != dv.colorKey(want) {
		t.Fatalf("manual pick should win even when duplicate: got %v want %v", dv.Rows[1].Color, want)
	}
}
```

(If `newTestDrumViewWithRows` doesn't exist, use the existing constructor pattern in `color_features_test.go` — mirror `TestUniqueColorsAcrossRows`'s setup.)

- [ ] **Step 2: Run — expect FAIL (undefined SetRowColorManual)**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSetRowColorManualAllowsDuplicate ./internal/ui/`
Expected: FAIL (compile: SetRowColorManual undefined).

- [ ] **Step 3: Implement `SetRowColorManual`**

Add to `drumview_colors.go`:

```go
// SetRowColorManual applies an explicit user-picked color WITHOUT the
// uniqueness substitution that SetRowColor performs. A deliberate pick from
// the color picker wins even if it duplicates another row's color; auto-assigned
// new rows still go through SetRowColor/ensureUniqueColor for distinctness.
func (dv *DrumView) SetRowColorManual(idx int, c color.Color) {
	if idx < 0 || idx >= len(dv.Rows) {
		return
	}
	dv.Rows[idx].Color = c
	if idx < len(dv.rowDirty) {
		dv.rowDirty[idx] = true
		if idx < len(dv.rowFullDirty) {
			dv.rowFullDirty[idx] = true
		}
	}
	emitRowColorChanged(idx, packRGBA(dv.Rows[idx].Color))
	dv.recordUndoStep(hooks.EventRowColorChanged)
}
```

- [ ] **Step 4: Run — expect PASS**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSetRowColorManualAllowsDuplicate ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Checkpoint (no commit).**

### Task 13: Point the picker callback at the manual setter

**Files:**
- Modify: `src/go/internal/ui/drumview_portal_open.go` (the `OnColorPick` wiring) — find where `ColorWheelProps.OnColorPick` is set and change `dv.SetRowColor(row, c)` → `dv.SetRowColorManual(row, c)`.

- [ ] **Step 1: Locate the callback**

Run: `cd src/go && grep -rn 'OnColorPick' internal/ui/`

- [ ] **Step 2: Edit the callback** to call `SetRowColorManual` with the picker's `colorMenuRow`.

- [ ] **Step 3: Build** — `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/` → clean.

- [ ] **Step 4: Checkpoint (no commit).**

### Task 14: Re-expose "Color" on the ellipsis menu

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go` (`contextMenuItems`, L246-285)

- [ ] **Step 1: Add the menu item** (after Rename's group-0 block, before the Origin divider):

```go
	// Group 1: Appearance — open the grouped Vice City color picker.
	items = append(items,
		contextMenuItem{label: "Color", icon: "color", style: itemStyle, group: 1, onClick: func() {
			dv.closeContextMenuPortal()
			dv.OpenColorMenu(rowIdx) // existing entry point that sets colorMenuRow + opens the portal
		}},
		contextMenuItem{divider: true},
	)
```

Verify `OpenColorMenu(rowIdx)` exists (grep) — it's the programmatic entry the scenes already use; if its name differs, use the actual one (e.g. `openColorWheelFor(rowIdx)`). Also update the stale comment at L239-245 (remove "interactive color editing was dropped").

- [ ] **Step 2: Build** → clean.

- [ ] **Step 3: Checkpoint (no commit).**

### Task 15: Upgrade the picker to grouped gradient ramps (7×5 + labels)

**Files:**
- Modify: `src/go/internal/ui/drumview_overlay_color_comp.go`

- [ ] **Step 1: Change the swatch source to grouped ramps**

Replace `swatches()` (L117-120) so it returns the 35 in family order (it already does if `SuggestedInstrumentSwatches` is the 35 from Task 8). Add a ramp grouping helper:

```go
// colorRamps groups the palette into the 7 hue families (5 shades each), in
// the canonical family order, for the grouped picker layout. Each inner slice
// is one gradient row rendered left→right.
func (c *ColorWheelComponent) colorRamps() [][]GenInstrumentSwatch {
	sw := c.swatches() // 35, family-ordered
	const perRow = 5
	var rows [][]GenInstrumentSwatch
	for i := 0; i < len(sw); i += perRow {
		end := i + perRow
		if end > len(sw) {
			end = len(sw)
		}
		rows = append(rows, sw[i:end])
	}
	return rows
}
```

- [ ] **Step 2: Re-layout `rebuildWheel`** from fixed 6×2 to `len(ramps)`×5 with a family-label gutter. Change the grid constants:

```go
const (
	colorSwatchCols    = 5  // shades per ramp
	colorSwatchGap     = 8
	colorSwatchPad     = 12
	colorSwatchHeader  = 32
	colorSwatchCellMin = 40
	colorRampLabelW    = 64 // left gutter for the family name
	colorRampRowGap    = 10 // vertical gap between ramps
)
```

In `rebuildWheel`, iterate `colorRamps()`; for ramp index `ri`, lay the 5 cells starting at `gridRect.Min.X + colorRampLabelW`, row top `= gridRect.Min.Y + ri*(cellH+colorRampRowGap)`. Store each cell in `c.cells` exactly as today (so hit-testing in `HandleInput` is unchanged). Panel height becomes `header + 7*cellH + 6*rowGap + 2*pad`. Reuse the existing clamp-to-`Bounds` logic and `AnchorPopupRect` — **do not change positioning philosophy.**

- [ ] **Step 3: Draw family labels + bordered pale swatches** in `Draw()`: for each ramp, draw the family name (derive from the first swatch's name prefix, e.g. `"hot-pink"`) in `colTextSecondary` at the left gutter using the existing text helper (`DrawTextStyled`/`drawText` — match what the component already uses for its title). For each cell, after the existing fill, draw a 1px `colBorderSubtle` outline (satisfies §7.4 — pale shades don't vanish). Keep the existing selected-ring logic for `CurrentColor`, recolored to `genColorHotPink300` (focus).

- [ ] **Step 4: Run the existing picker tests**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'ColorWheel|ColorButton|ColorMenu' ./internal/ui/ 2>&1 | head -30`
Expected: PASS, after updating any test that asserts exactly 12 cells / 6×2 to the new 35 cells / 7×5. (`TestColorWheelClickSetsColor` should still pass — it clicks a cell and asserts the row recolors.)

- [ ] **Step 5: Checkpoint (no commit).**

### Task 16: Functional test — kebab → Color → pick recolors + records undo

**Files:**
- Create: `src/go/internal/ui/color_picker_menu_test.go`

- [ ] **Step 1: Write the functional test** driving the real input loop (per the functional-over-isolation preference). Mirror the setup in existing kebab/menu functional tests (`drumview_context_menu` tests / `mobile_instrument_button_test.go`):

```go
package ui

import "testing"

func TestEllipsisColorOpensPickerAndRecolors(t *testing.T) {
	g := newGameForTest(t)        // existing harness used by other functional UI tests
	dv := g.drum                  // or the accessor those tests use
	row := 0
	before := dv.Rows[row].Color

	// 1. Open the kebab context menu for the row.
	openRowKebabForTest(t, g, row)            // helper pattern from existing menu tests
	if !dv.IsContextMenuOpen() {
		t.Fatal("context menu did not open")
	}

	// 2. Click the "Color" item.
	clickContextMenuItemByLabel(t, g, "Color") // helper pattern from existing menu tests
	if !dv.colorWheelComp.IsOpen() {
		t.Fatal("Color item did not open the picker")
	}

	// 3. Pick a swatch different from the current color.
	target := genColorHotPink400
	dv.colorWheelComp.props.OnColorPick(target) // drive the wired callback
	if dv.colorKey(dv.Rows[row].Color) == dv.colorKey(before) {
		t.Fatalf("row color did not change after pick")
	}
	if dv.colorKey(dv.Rows[row].Color) != dv.colorKey(target) {
		t.Fatalf("manual pick should apply exactly: got %v want %v", dv.Rows[row].Color, target)
	}

	// 4. Undo restores the previous color.
	g.undo()                                   // existing undo entry point
	if dv.colorKey(dv.Rows[row].Color) != dv.colorKey(before) {
		t.Fatalf("undo did not restore color: got %v want %v", dv.Rows[row].Color, before)
	}
}
```

Replace `newGameForTest`/`openRowKebabForTest`/`clickContextMenuItemByLabel`/`g.undo` with the actual helper names found in the existing functional menu tests (grep `func.*ContextMenu` and `func.*Kebab` in `internal/ui/*_test.go`). Do not invent helpers — reuse what those tests use.

- [ ] **Step 2: Run — expect PASS**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestEllipsisColorOpensPickerAndRecolors ./internal/ui/`
Expected: PASS.

- [ ] **Step 3: Checkpoint (no commit).**

### Task 17: Full verification

- [ ] **Step 1: UI fast suite + guards**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/templates/ 2>&1 | tail -30`
Expected: PASS modulo documented pre-existing branch failures (diff against a baseline HEAD run before attributing any failure to this work).

- [ ] **Step 2: Cross-platform parity smoke** (instrument colors are baked into exported JSON):

Run: `make test-parity` and `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`
Expected: PASS.

- [ ] **Step 3: Screenshot the picker + app**

Run: `make screenshots-all SCENES=$(./tmp/beatmo_screenshot -list-scenes | grep -i color | paste -sd ,)` (or `make screenshot`) and eyeball the grouped gradient picker + Vice City repaint.

- [ ] **Step 4: Final checkpoint (no commit).** Leave everything in the working tree; the user owns the commit.

---

## Self-review notes (gaps & risks)

- **Drift want-map is the biggest mechanical edit.** It must be updated in lockstep (Tasks 2, 4, 8) or the build breaks — each task pairs the DESIGN.md edit with its want-map edit.
- **Helper names in functional tests (Task 16) are placeholders for real ones** — the task explicitly says to grep existing menu tests and reuse their harness rather than invent helpers.
- **`OpenColorMenu` / `OnColorPick` exact names** are verified by grep in Tasks 13–14 before editing.
- **Pre-existing branch failures**: per MEMORY, `add-core-node-types` carries unrelated failing tests; always baseline against HEAD before attributing a red test to this diff.
- **Audio tabs** are covered by Phase 3's grep + pixel tests proving colors-only/token-sourced, satisfying the user's explicit "EQ and all other tabs from these tokens" requirement.
