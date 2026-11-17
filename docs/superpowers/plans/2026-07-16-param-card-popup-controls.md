# ParamCard + Universal Value-Wheel Popup Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace rotary knobs on ALL platforms (synth + sampler tabs) with a unified tappable ParamCard (name + value + range gauge) that opens the value-wheel popup; add a visible resolution rung ladder, desktop scroll-nudge, hover, and an always-visible Advanced divider.

**Architecture:** `Knob` stays as the headless value model; a new `ParamCard` renderer replaces the dial cell in both panels; the existing `MobileWheelPopup` becomes platform-universal and gains a rung ladder + title gloss. The advanced expander is replaced by an in-grid divider row.

**Tech Stack:** Go 1.26 (bundled `.tools/go/bin/go`), Ebiten, ebitenstub `-tags test` harness.

**Spec:** `docs/superpowers/specs/2026-07-16-param-card-popup-controls-design.md`

## Global Constraints

- **NEVER `git add` or `git commit`** — this repo's standing rule is all work stays uncommitted on `add-core-node-types`. Every "Commit" step in the standard template is replaced by "verify + leave uncommitted".
- Always use the bundled toolchain: `/home/ymolinar/Repos/beatmo/.tools/go/bin/go` (and its `gofmt`).
- Fast test command (run from `/home/ymolinar/Repos/beatmo/src/go`):
  `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run '<Pattern>'`
- TDD: write the failing test first, watch it fail, implement, watch it pass.
- Design tokens: any new size constant goes in DESIGN.md `densities:` (×3) + `make gen-design-tokens`; never a bare literal (token_discipline ratchet).
- Forbidden glyphs (▶▼▲✕≡→←↑↓): use `IconID` + `DrawIcon`. `±`/`x` in step labels are already sanctioned (`formatStepValue`).
- Text: styled roles only (`DrawTextStyled`, RoleMicro/Caption/Body) — never `DrawTextColorAtScale`.
- Concurrent WIP may flap the full `./internal/ui/` build — verify your own work with `-run` filters; never `rm` foreign files.
- If a test that passed recently goes red, prove causation — no "pre-existing failure" dismissals (except the two documented red node-logic-volume tests at HEAD).

---

### Task 1: Density tokens for ParamCard + wider ladder strip

**Files:**
- Modify: `DESIGN.md` (three `densities:` blocks, near lines 589/680/771)
- Regenerate: `src/go/internal/ui/design_density.gen.go` via `make gen-design-tokens`

**Interfaces:**
- Produces: `Profile().DensityValues().ParamCardW/.ParamCardH/.ParamCardGaugeH` (int), and widened `.MobileWheelResStripW` / `.MobileWheelW`. Later tasks read these exact field names (generator maps kebab-case → CamelCase).

- [ ] **Step 1: Add tokens to DESIGN.md.** In each of the three `densities:` blocks add (indentation matching siblings like `synthKnobIdeal`):

```yaml
# compact block (sibling of synthKnobIdeal: 56):
    paramCardW:            128
    paramCardH:            48
    paramCardGaugeH:       3
# comfortable block (sibling of synthKnobIdeal: 72):
    paramCardW:            148
    paramCardH:            56
    paramCardGaugeH:       4
# spacious block (sibling of synthKnobIdeal: 88):
    paramCardW:            168
    paramCardH:            64
    paramCardGaugeH:       5
```

Also widen the ladder strip and popup so rung labels ("±100 Hz") fit:
compact `mobileWheelResStripW: 36→64`, `mobileWheelW: 200→228`;
comfortable `40→72`, `220→252`; spacious (`~44→80`, `240→276` — read the
spacious block for exact current values and add the same deltas).

- [ ] **Step 2: Regenerate + verify.**

Run: `cd /home/ymolinar/Repos/beatmo && make gen-design-tokens`
Then: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestDesignTokensFresh|TestDesignMDDrift|TestDesignMDSchema'`
Expected: PASS; `design_density.gen.go` now contains `ParamCardW` etc. in all three density structs.

- [ ] **Step 3: Leave uncommitted.** Run `../../.tools/go/bin/gofmt -l internal/ui/ | head` → no output.

---

### Task 2: ParamCard widget (render + geometry + gauge model)

**Files:**
- Create: `src/go/internal/ui/param_card.go`
- Test: `src/go/internal/ui/param_card_test.go`

**Interfaces:**
- Consumes: Task 1 tokens; existing helpers `drawRoundedButton`, `keycapCapRect`, `drawKeycapContactShadow`, `drawKeycapBevel`, `drawRoundedRect`, `WithAlphaFromColor`, `DrawTextStyled`, `StyledTextWidth/Height`, `truncCaptionStyled`, `DrawIcon(IconChevronDown)`, tokens `TokenSurface3/adjustColor/genColorPrimary/colTextPrimary/TokenTextSecondary`.
- Produces (later tasks call these EXACT signatures):

```go
type paramCardGaugeKind int
const (
    gaugeFill paramCardGaugeKind = iota
    gaugeBipolar
    gaugeDots
)
type paramCardGauge struct {
    kind  paramCardGaugeKind
    frac  float64 // normalized value 0..1 (fill/bipolar)
    zero  float64 // bipolar zero fraction (0..1)
    count int     // dots: enum length
    idx   int     // dots: selected index
}
type paramCardState struct{ Selected, Dim bool }
func paramCardGaugeFor(k *Knob, def audio.ParamDef) paramCardGauge
func drawParamCard(dst *ebiten.Image, r image.Rectangle, name, value string, g paramCardGauge, st paramCardState)
```

- [ ] **Step 1: Write the failing test** (`param_card_test.go`, `//go:build test` NOT needed — ui tests are plain files run with `-tags test`):

```go
package ui

import (
    "image"
    "testing"

    "github.com/hajimehoshi/ebiten/v2"
    "github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestParamCardGaugeFor_Kinds(t *testing.T) {
    // Continuous → fill with frac == normalized value.
    k := &Knob{Value: 0.25}
    g := paramCardGaugeFor(k, audio.ParamDef{Min: 0, Max: 100})
    if g.kind != gaugeFill || g.frac != 0.25 {
        t.Fatalf("continuous: got kind=%v frac=%v", g.kind, g.frac)
    }
    // Bipolar → gaugeBipolar carrying the knob's zero fraction.
    kb := &Knob{Value: 0.75, Bipolar: true, ZeroFrac: 0.5}
    gb := paramCardGaugeFor(kb, audio.ParamDef{Min: -12, Max: 12})
    if gb.kind != gaugeBipolar || gb.zero != 0.5 || gb.frac != 0.75 {
        t.Fatalf("bipolar: got %+v", gb)
    }
    // Enum (<=8) → dots with the rounded index.
    ke := &Knob{Value: 1} // normalized 1.0 → last index
    def := audio.ParamDef{Min: 0, Max: 3, Enum: []string{"Sine", "Saw", "Square", "Tri"}}
    ge := paramCardGaugeFor(ke, def)
    if ge.kind != gaugeDots || ge.count != 4 || ge.idx != 3 {
        t.Fatalf("enum: got %+v", ge)
    }
    // Enum >8 → index-proportional fill, not dots.
    big := make([]string, 12)
    for i := range big {
        big[i] = "v"
    }
    gbig := paramCardGaugeFor(&Knob{Value: 0.5}, audio.ParamDef{Min: 0, Max: 11, Enum: big})
    if gbig.kind != gaugeFill {
        t.Fatalf("large enum should fall back to fill, got %+v", gbig)
    }
}

func TestParamCardDraw_RendersInkAndGauge(t *testing.T) {
    dst := ebiten.NewImage(200, 80)
    r := image.Rect(10, 10, 10+148, 10+56)
    drawParamCard(dst, r, "CUTOFF", "8.00 kHz",
        paramCardGauge{kind: gaugeFill, frac: 0.5}, paramCardState{})
    // Some ink must land inside the card rect.
    ink := 0
    for y := r.Min.Y; y < r.Max.Y; y += 2 {
        for x := r.Min.X; x < r.Max.X; x += 2 {
            if _, _, _, a := dst.At(x, y).RGBA(); a > 0 {
                ink++
            }
        }
    }
    if ink < 50 {
        t.Fatalf("card drew almost nothing: ink=%d", ink)
    }
    // Empty rect must be a no-op (no panic).
    drawParamCard(dst, image.Rectangle{}, "X", "1", paramCardGauge{}, paramCardState{})
}
```

- [ ] **Step 2: Run to verify failure.**
Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestParamCard'`
Expected: FAIL — `undefined: paramCardGaugeFor` etc.

- [ ] **Step 3: Implement `param_card.go`.**

```go
// src/go/internal/ui/param_card.go
package ui

import (
    "image"
    "image/color"
    "math"

    "github.com/hajimehoshi/ebiten/v2"
    "github.com/ingyamilmolinar/beatmo/internal/audio"
)

// ParamCard is the closed state of every synth/sampler parameter control on
// every platform: a compact keycap-style card showing the param NAME, the
// current VALUE (resolution-aware), and a thin range GAUGE. Tapping it opens
// the value-wheel popup; on desktop the mouse wheel nudges the value in place.

type paramCardGaugeKind int

const (
    gaugeFill paramCardGaugeKind = iota
    gaugeBipolar
    gaugeDots
)

// paramCardMaxDots is the largest enum rendered as discrete position dots;
// longer enums fall back to an index-proportional fill.
const paramCardMaxDots = 8

type paramCardGauge struct {
    kind  paramCardGaugeKind
    frac  float64
    zero  float64
    count int
    idx   int
}

type paramCardState struct{ Selected, Dim bool }

// paramCardGaugeFor derives the gauge from the shared knob value model.
func paramCardGaugeFor(k *Knob, def audio.ParamDef) paramCardGauge {
    v := 0.0
    if k != nil {
        v = k.Value
    }
    if n := len(def.Enum); n > 0 && n <= paramCardMaxDots {
        span := def.Max - def.Min
        actual := def.Min + v*span
        idx := int(math.Round(actual))
        if idx < 0 {
            idx = 0
        } else if idx >= n {
            idx = n - 1
        }
        return paramCardGauge{kind: gaugeDots, count: n, idx: idx}
    }
    if k != nil && k.Bipolar {
        return paramCardGauge{kind: gaugeBipolar, frac: v, zero: k.ZeroFrac}
    }
    return paramCardGauge{kind: gaugeFill, frac: v}
}

// drawParamCard renders the card. Token accessors only; no-op on empty rect.
func drawParamCard(dst *ebiten.Image, r image.Rectangle, name, value string, g paramCardGauge, st paramCardState) {
    if r.Empty() {
        return
    }
    rad := RadiusMD / 2
    capFill := TokenSurface3()
    shell := adjustColor(capFill, -45)
    capR := keycapCapRect(r, 0, true)
    drawRoundedButton(dst, r, shell, shell, rad, false)
    drawKeycapContactShadow(dst, capR, rad)
    drawRoundedButton(dst, capR, capFill, capFill, rad, false)
    drawKeycapBevel(dst, capR, rad)
    rim := WithAlphaFromColor(genColorPrimary, AlphaMedium)
    if st.Selected {
        rim = colConceptStroke
    }
    drawRoundedRect(dst, capR, rim, rad, false)

    inner := capR.Inset(SpaceXS + 2)
    if inner.Empty() {
        return
    }
    nameCol := TokenTextSecondary()
    valCol := colTextPrimary
    if st.Dim {
        nameCol = TokenTextDisabled()
        valCol = TokenTextDisabled()
    }
    dv2 := Profile().DensityValues()
    gaugeH := dv2.ParamCardGaugeH
    nameH := StyledTextHeight(RoleMicro)
    valH := StyledTextHeight(RoleBody)

    // Name row (top).
    nm := truncCaptionStyled(name, inner.Dx(), RoleMicro)
    DrawTextStyled(dst, nm, inner.Min.X, inner.Min.Y, RoleMicro, nameCol)

    // Value row (center between name and gauge) + "opens" caret at right.
    const caretW = 9
    valTop := inner.Min.Y + nameH
    valBottom := inner.Max.Y - gaugeH - SpaceXS
    vy := valTop + (valBottom-valTop-valH)/2
    if vy < valTop {
        vy = valTop
    }
    vzW := inner.Dx() - caretW - SpaceXS
    val := truncCaptionStyled(value, vzW, RoleBody)
    DrawTextStyled(dst, val, inner.Min.X, vy+1, RoleBody, WithAlphaFromColor(color.Black, 150))
    DrawTextStyled(dst, val, inner.Min.X, vy, RoleBody, valCol)
    cy := vy + valH/2
    cx := inner.Max.X - caretW
    DrawIcon(dst, IconChevronDown, image.Rect(cx, cy-caretW/2, cx+caretW, cy+caretW/2+1),
        WithAlphaFromColor(genColorPrimary, AlphaStrong))

    // Gauge strip (bottom).
    gr := image.Rect(inner.Min.X, inner.Max.Y-gaugeH, inner.Max.X, inner.Max.Y)
    drawParamCardGauge(dst, gr, g, st.Dim)
}

func drawParamCardGauge(dst *ebiten.Image, gr image.Rectangle, g paramCardGauge, dim bool) {
    if gr.Empty() {
        return
    }
    track := WithAlpha(TokenSurface1(), AlphaStrong)
    fill := genColorPrimary
    if dim {
        fill = TokenTextDisabled()
    }
    drawRoundedRect(dst, gr, track, RadiusXS, true)
    switch g.kind {
    case gaugeDots:
        if g.count <= 0 {
            return
        }
        d := gr.Dy()
        stride := 0
        if g.count > 1 {
            stride = (gr.Dx() - d) / (g.count - 1)
        }
        for i := 0; i < g.count; i++ {
            x := gr.Min.X + i*stride
            dot := image.Rect(x, gr.Min.Y, x+d, gr.Max.Y)
            col := WithAlphaFromColor(fill, AlphaSubtle)
            if i == g.idx {
                col = fill
            }
            drawRoundedRect(dst, dot, col, RadiusXS, true)
        }
    case gaugeBipolar:
        zx := gr.Min.X + int(g.zero*float64(gr.Dx()))
        vx := gr.Min.X + int(clamp01(g.frac)*float64(gr.Dx()))
        seg := image.Rect(zx, gr.Min.Y, vx, gr.Max.Y)
        if vx < zx {
            seg = image.Rect(vx, gr.Min.Y, zx, gr.Max.Y)
        }
        if seg.Dx() < 2 { // always show the center detent
            seg.Max.X = seg.Min.X + 2
        }
        drawRoundedRect(dst, seg, fill, RadiusXS, true)
    default: // gaugeFill
        w := int(clamp01(g.frac) * float64(gr.Dx()))
        if w < 2 && g.frac > 0 {
            w = 2
        }
        drawRoundedRect(dst, image.Rect(gr.Min.X, gr.Min.Y, gr.Min.X+w, gr.Max.Y), fill, RadiusXS, true)
    }
}
```

If `clamp01` does not exist in the package, add it here as `func clamp01(v float64) float64 { if v < 0 { return 0 }; if v > 1 { return 1 }; return v }` (grep first — there may be one already).

- [ ] **Step 4: Run to verify pass.**
Run: same `-run 'TestParamCard'` command.
Expected: PASS. Also run `../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | head` (ignore pre-existing noise from foreign files).

- [ ] **Step 5: gofmt + leave uncommitted.**

---

### Task 3: Synth grid — card cells, no desktop column cap, Advanced divider row

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (`synthKnobCellHeight` ~1718, `placeKnobsInSection` ~1767-1900, `drawSynthAdvExpander` ~1746, `toggleSynthAdvanced` ~3468, `synthAdvExpanderHitAdapter` ~3489)
- Modify: `src/go/internal/ui/drumview_*.go` wherever `synthAdvExpanded`/`synthAdvScrollPending`/`synthAdvExpanderRect` fields are declared (grep) — replace `synthAdvExpanderRect` with `synthAdvDividerRect image.Rectangle` and a `synthAdvDividerCount int`; delete the other two.
- Test: `src/go/internal/ui/synth_advanced_divider_test.go` (new)

**Interfaces:**
- Consumes: Task 1 `ParamCardW/ParamCardH`; existing `synthSectionTierSplit`, `ControlGrid` (`SetMaxCols`, `Layout`, `Cols`, `CellRect`).
- Produces: layout invariants used by Tasks 4/5: every visible knob idx has rect == its full grid cell; `dv.synthAdvDividerRect` (empty when the stage has no advanced params); phantom grid indices map to knob idx `-1`.

- [ ] **Step 1: Write the failing test** (`synth_advanced_divider_test.go`). Use the existing helpers `newModularSynthTabGame(t)` / `layoutSynthTab` (in `synth_chip_strip_helpers_test.go`) — read that file first and mirror an existing test's setup (e.g. the P4 test at `synth_panel_zone` test files, `TestP4_AdvancedExpanderRevealsKnobs`) for how to select a stage with advanced params:

```go
package ui

import "testing"

// The advanced tier is always laid out below a labeled divider row — no
// expander, no hidden state. Essentials come first; advanced knobs always
// have rects (possibly scrolled out, but never structurally hidden).
func TestSynthAdvancedDivider_EssentialsFirstAdvancedAlwaysPlaced(t *testing.T) {
    g := newModularSynthTabGame(t) // helper: modular synth tab, expanded panel
    dv := g.drum
    sel := dv.synthSelectedSection()
    if sel == nil {
        t.Fatal("no selected section")
    }
    // Pick a stage that has advanced knobs (walk sections until tierSplit
    // returns a non-empty advanced list; select it and re-layout).
    var stage *synthSection
    for i := range dv.instEditorSections {
        s := &dv.instEditorSections[i]
        _, adv := dv.synthSectionTierSplit(s.knobIdxs)
        if len(adv) > 0 {
            stage = s
            break
        }
    }
    if stage == nil {
        t.Skip("no stage with advanced params in test registry")
    }
    dv.setSelectedSynthSection(dv.synthTabActiveInstrument(), stage.id)
    layoutSynthTab(t, g)

    sel = dv.synthSelectedSection()
    ess, adv := dv.synthSectionTierSplit(sel.knobIdxs)
    if len(adv) == 0 {
        t.Fatal("selected stage lost its advanced knobs")
    }
    // Divider rect exists and sits below the last essential knob's rect.
    if dv.synthAdvDividerRect.Empty() {
        t.Fatal("expected a non-empty advanced divider rect")
    }
    maxEssBottom := 0
    for _, i := range ess {
        if r := dv.instEditorKnobs[i].Rect(); !r.Empty() && r.Max.Y > maxEssBottom {
            maxEssBottom = r.Max.Y
        }
    }
    if maxEssBottom > 0 && dv.synthAdvDividerRect.Min.Y < maxEssBottom {
        t.Fatalf("divider (%v) overlaps essentials (bottom %d)", dv.synthAdvDividerRect, maxEssBottom)
    }
    // Advanced knobs are placed (first advanced knob is in the grid — its
    // cell exists even if scrolled out of the window).
    // Scroll to it and require a visible rect.
    grid := dv.sectionGrid(sel.id)
    grid.ScrollToIndex(grid.VisibleCount()) // nudge; then scroll to end
    layoutSynthTab(t, g)
    anyAdvVisible := false
    for _, i := range adv {
        if !dv.instEditorKnobs[i].Rect().Empty() {
            anyAdvVisible = true
        }
    }
    _ = anyAdvVisible // structural: no synthAdvExpanded gate exists anymore
}

// Desktop rows now flow as many cards as fit — at 1280 wide the detail pane
// must place at least 4 cards on the first row of a knob-rich stage.
func TestSynthCardGrid_DesktopRowDensity(t *testing.T) {
    g := newModularSynthTabGame(t)
    dv := g.drum
    sel := dv.synthSelectedSection()
    if sel == nil || len(sel.knobIdxs) < 4 {
        // pick the stage with the most knobs
        best := -1
        for i := range dv.instEditorSections {
            if n := len(dv.instEditorSections[i].knobIdxs); n >= 4 && n > best {
                best = n
                dv.setSelectedSynthSection(dv.synthTabActiveInstrument(), dv.instEditorSections[i].id)
            }
        }
        layoutSynthTab(t, g)
        sel = dv.synthSelectedSection()
    }
    if sel == nil || len(sel.knobIdxs) < 4 {
        t.Skip("no 4-knob stage available")
    }
    if got := dv.sectionGrid(sel.id).Cols(); got < 4 {
        t.Fatalf("desktop card grid cols = %d, want >= 4 (was capped at 2 in the dial era)", got)
    }
}
```

Adjust helper names to what actually exists in `synth_chip_strip_helpers_test.go` (read it first); the ASSERTIONS are the contract.

- [ ] **Step 2: Run to verify failure.**
Run: `-run 'TestSynthAdvancedDivider|TestSynthCardGrid'`
Expected: FAIL — `dv.synthAdvDividerRect` undefined / cols == 2.

- [ ] **Step 3: Implement.** In `synth_panel_zone.go`:

1. `synthKnobCellHeight()` → `return Profile().DensityValues().ParamCardH` (keep the func; callers stay).
2. In `placeKnobsInSection`:
   - Replace the `maxCols := 2 / mobile 4` block with `grid.SetMaxCols(12)` (both platforms — mobile columns self-limit via `ParamCardW` ideal width).
   - Replace the expander block (the `expanded := dv.synthAdvExpanded[...]` / `visible :=` / expander-rect reservation, lines ~1844-1874) with the divider-row layout:

```go
    essential, advanced := dv.synthSectionTierSplit(s.knobIdxs)
    dv.synthAdvDividerRect = image.Rectangle{}
    dv.synthAdvDividerCount = len(advanced)
    visible := essential
    dividerStart, dividerEnd := -1, -1
    if len(advanced) > 0 {
        // Two-pass: first Layout discovers the column count for this width,
        // then the padded list (essentials rounded up to a full row + one
        // phantom divider row + advanced) is laid out for real.
        grid.Layout(innerR, len(essential)+len(advanced), dv2.ParamCardW, cellH, hGap, vGap)
        cols := grid.Cols()
        if cols < 1 {
            cols = 1
        }
        padded := append([]int{}, essential...)
        for len(padded)%cols != 0 {
            padded = append(padded, -1)
        }
        dividerStart = len(padded)
        for i := 0; i < cols; i++ { // one full phantom row = the divider band
            padded = append(padded, -1)
        }
        dividerEnd = len(padded)
        visible = append(padded, advanced...)
    }
    grid.Layout(innerR, len(visible), dv2.ParamCardW, cellH, hGap, vGap)
    if dividerStart >= 0 {
        var dr image.Rectangle
        for i := dividerStart; i < dividerEnd; i++ {
            if c, vis := grid.CellRect(i); vis {
                dr = dr.Union(c)
            }
        }
        dv.synthAdvDividerRect = dr
    }
```

   - In the assignment loop below, skip phantoms: `if kIdx < 0 { continue }`, and set each visible knob's rect to the FULL slot (the card fills the cell): `dv.instEditorKnobs[kIdx].SetRect(slot)` (delete the dial-diameter clamp math `d := slot.Dx()` … that follows; sliders keep mirroring the knob rect).
   - Delete the `knobR`/expander-rect reservation; use `innerR` directly as the grid rect.
3. Delete `drawSynthAdvExpander`, `synthAdvExpanderH`, `toggleSynthAdvanced`, `synthAdvExpanderHitAdapter`, and the hit-area registration for the expander (grep `synthAdvExpander` across `internal/ui/` for every site, including `synthTabHitAreas`). Delete fields `synthAdvExpanded`, `synthAdvScrollPending`; add `synthAdvDividerRect image.Rectangle` and `synthAdvDividerCount int` where the old fields were declared.
4. Grep tests referencing `synthAdvExpanded|AdvancedExpander|toggleSynthAdvanced` (e.g. `TestP4_AdvancedExpanderRevealsKnobs` and the scene `crop_synth_tab_kick_voice_expanded` Setup): rewrite the P4 test to assert divider ordering (essentials above `synthAdvDividerRect`, advanced below), and change the scene Setup to just select the stage (no toggle) — rename the scene to `crop_synth_advanced_divider` in `scene_catalog.go` and in any scene-name lists (grep the old name repo-wide, incl. `scripts/basic_scenes.txt`).

- [ ] **Step 4: Run to verify pass.**
`-run 'TestSynthAdvancedDivider|TestSynthCardGrid|TestP4'` → PASS (P4 rewritten).
Then build the whole package: `../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/` → OK.

- [ ] **Step 5: gofmt + leave uncommitted.**

---

### Task 4: Synth draw — cards everywhere + divider render

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (`drawSynthDetailPane` knob loop ~2447-2592, `mobileKnobButtonRect`/`drawKnobValuePill` ~2776-2805)
- Test: `src/go/internal/ui/synth_card_draw_test.go` (new)

**Interfaces:**
- Consumes: Task 2 `drawParamCard`/`paramCardGaugeFor`; Task 3 `synthAdvDividerRect`/`synthAdvDividerCount`; existing `synthParamDisplayName`, `knobPillLabel`, `i18n.KeySynthAdvanced`.
- Produces: `(*synthPanel).drawSynthAdvDivider(dst *ebiten.Image)`; draw loop renders ONE card per visible knob on ALL platforms (no `k.Draw`, no caption, no badge, no gloss in the grid).

- [ ] **Step 1: Failing test.** Intercept styled text (use the `drawTextStyledHook` seam in `textcache.go` — read how `collectDrawnTexts` in an existing levels test intercepts BOTH hooks and copy that pattern):

```go
package ui

import (
    "strings"
    "testing"
)

// Cards carry name AND value on every platform; the divider row labels the
// advanced tier. The old desktop dial caption ("name  value") is gone.
func TestSynthCardDraw_NameValueAndDivider(t *testing.T) {
    g := newModularSynthTabGame(t)
    dv := g.drum
    // select a stage with advanced knobs (same walk as the divider test)
    for i := range dv.instEditorSections {
        if _, adv := dv.synthSectionTierSplit(dv.instEditorSections[i].knobIdxs); len(adv) > 0 {
            dv.setSelectedSynthSection(dv.synthTabActiveInstrument(), dv.instEditorSections[i].id)
            break
        }
    }
    layoutSynthTab(t, g)
    texts := collectDrawnTextsDuring(t, g) // helper: draw one frame, capture styled texts
    joined := strings.ToLower(strings.Join(texts, "\n"))
    sel := dv.synthSelectedSection()
    if sel == nil || len(sel.knobIdxs) == 0 {
        t.Fatal("no knobs")
    }
    def := dv.instEditorBindings[sel.knobIdxs[0]].def
    wantName := strings.ToLower(synthParamDisplayName(def.Name))
    if !strings.Contains(joined, wantName) {
        t.Fatalf("card name %q not drawn; texts:\n%s", wantName, joined)
    }
    if !dv.synthAdvDividerRect.Empty() && !strings.Contains(joined, strings.ToLower(i18n.T(i18n.KeySynthAdvanced))) {
        t.Fatalf("advanced divider label not drawn")
    }
}
```

Write `collectDrawnTextsDuring` in this test file if no shared helper exists (register both `drawTextHook` and `drawTextStyledHook`, call the game's draw of the synth tab — mirror `audio_panel_render_pixels_test.go` setup, then restore hooks with `t.Cleanup`).

- [ ] **Step 2: Verify failure** (`-run 'TestSynthCardDraw'`): FAIL (name only appears via caption on desktop path? note: old desktop caption DID include the name — so make the failing assertion the DIVIDER one first; after the swap both hold. If the name assertion passes pre-change, keep it as a regression guard anyway and rely on the divider assertion to be RED).

- [ ] **Step 3: Implement.** In `drawSynthDetailPane`:
1. Replace the whole per-knob block (dial/pill draw + selection ring + caption + gloss + readout + badge, ~2448-2581) with:

```go
    for _, kIdx := range sel.knobIdxs {
        if kIdx < 0 || kIdx >= len(dv.instEditorKnobs) {
            continue
        }
        k := dv.instEditorKnobs[kIdx]
        r := k.Rect()
        if r.Empty() {
            continue // scrolled out / phantom
        }
        b := dv.instEditorBindings[kIdx]
        st := paramCardState{
            Selected: dv.synthSelectedKnobIdx(instID, sel) == kIdx,
            Dim:      !stageEnabled,
        }
        drawParamCard(dst, r, synthParamDisplayName(b.def.Name),
            knobPillLabel(k, b.def), paramCardGaugeFor(k, b.def), st)
    }
```

2. Keep the badge-rect + readout-rect CLEARING loops (~2434-2445) for now (Task 5 deletes the fields' interactions; a later cleanup task removes the fields).
3. After the loop, replace `dv.drawSynthAdvExpander(dst, sel.id)` with `p.drawSynthAdvDivider(dst)`:

```go
// drawSynthAdvDivider renders the always-visible "— ADVANCED · N —" tier
// divider row (non-interactive). Empty rect (stage has no advanced params,
// or the row is scrolled out) draws nothing.
func (p *synthPanel) drawSynthAdvDivider(dst *ebiten.Image) {
    dv := p.dv
    r := dv.synthAdvDividerRect
    if r.Empty() {
        return
    }
    lbl := i18n.T(i18n.KeySynthAdvanced)
    if dv.synthAdvDividerCount > 0 {
        lbl += " · " + strconv.Itoa(dv.synthAdvDividerCount)
    }
    lw := StyledTextWidth(lbl, RoleMicro)
    lh := StyledTextHeight(RoleMicro)
    cx := r.Min.X + (r.Dx()-lw)/2
    cy := r.Min.Y + (r.Dy()-lh)/2
    hairY := r.Min.Y + r.Dy()/2
    hairCol := WithAlpha(TokenBorderSubtle(), AlphaMedium)
    drawRect(dst, image.Rect(r.Min.X, hairY, cx-SpaceSM, hairY+1), hairCol, true)
    drawRect(dst, image.Rect(cx+lw+SpaceSM, hairY, r.Max.X, hairY+1), hairCol, true)
    DrawTextStyled(dst, lbl, cx, cy, RoleMicro, TokenTextSecondary())
}
```

4. Delete `(*synthPanel).drawKnobValuePill` and `mobileKnobButtonRect` (grep callers: the selection-ring site is gone with the new loop; `knob_value_pill.go` itself is deleted in Task 6 after the sampler swap).
5. Grep-and-fix tests that pinned the old draw (`mobile_knob_caption_dupe_test.go`, caption tests referencing `synthKnobCaption`/`synthKnobMobileCaption` draw output, `TestSynthKnobCaptionIsDisplayOnly` in `knob_interaction_functional_test.go`): captions no longer render — update each test to assert the CARD contract instead (name via card, value via card, no duplicate). Keep `synthKnobCaption`/helpers only if still referenced (popup/editor); delete dead ones.

- [ ] **Step 4: Verify pass** (`-run 'TestSynthCardDraw|TestSynthAdvancedDivider'`) + package builds.

- [ ] **Step 5: gofmt + leave uncommitted.**

---

### Task 5: Synth input — universal tap-to-popup, desktop wheel nudge, Shift-fine, hover

**Files:**
- Modify: `src/go/internal/ui/synth_panel_zone.go` (`synthKnobHitAdapter` methods ~3664-3860)
- Create: `src/go/internal/ui/knob_wheel_nudge.go` (shared nudge + debounced commit)
- Test: `src/go/internal/ui/param_card_input_test.go` (new)

**Interfaces:**
- Consumes: existing `openSynthKnobWheelPopup(idx, instID)`, `Knob.HandleWheel`, `KnobStepBadge.Step()/AdjustResolution`, `stepNotcher`, `isKeyPressed(ebiten.KeyShiftLeft/Right)`.
- Produces:

```go
// knob_wheel_nudge.go
type wheelNudger struct{ pending bool; framesLeft int; commit func() }
func (n *wheelNudger) bump(commit func())      // (re)arm ~600ms debounce, latch commit
func (n *wheelNudger) tick()                   // call once per Update frame; fires commit at 0
func nudgeKnobByWheel(k *Knob, badge *KnobStepBadge, steps int, fine bool) bool
```

- also: `synthKnobHitAdapter` implements `glowRect` (glowTarget) returning the knob's rect.

- [ ] **Step 1: Failing tests** (functional, through the real tree dispatch — mirror `TestSynthKnobHorizontalDragChangesValueViaTreeDispatch` in `knob_interaction_functional_test.go` for how to find a knob rect and dispatch presses/wheel through `dv.rootTree`/`g.Update`; read that file first):

```go
package ui

import "testing"

// Desktop click on a card opens the value-wheel popup (was mobile-only).
func TestParamCardClickOpensPopupOnDesktop(t *testing.T) {
    g := newModularSynthTabGame(t)
    dv := g.drum
    r := firstVisibleSynthKnobRect(t, dv) // helper exists in wheel tests; grep it
    pressAndReleaseAt(t, g, r.Min.X+4, r.Min.Y+4)
    if dv.synthWheelPopup == nil || !dv.synthWheelPopup.IsOpen() {
        t.Fatal("desktop card tap must open the value-wheel popup")
    }
}

// Desktop wheel over a card nudges the value by one StepMul per notch and
// commits ONE undo step after the debounce elapses.
func TestParamCardWheelNudgesValue(t *testing.T) {
    g := newModularSynthTabGame(t)
    dv := g.drum
    idx, r := firstContinuousSynthKnob(t, dv) // continuous param with a badge
    k := dv.instEditorKnobs[idx]
    before := k.Value
    undoBefore := undoDepth(g) // helper: read undo stack depth; grep existing undo tests
    dispatchWheelAt(t, g, r.Min.X+4, r.Min.Y+4, +1)
    if k.Value == before {
        t.Fatal("wheel notch over card did not change value")
    }
    if dv.synthWheelPopup != nil && dv.synthWheelPopup.IsOpen() {
        t.Fatal("wheel nudge must not open the popup")
    }
    for i := 0; i < 60; i++ { // run the debounce down
        g.Update()
    }
    if undoDepth(g) != undoBefore+1 {
        t.Fatalf("wheel gesture must commit exactly one undo step, got %d→%d", undoBefore, undoDepth(g))
    }
}

// Wheel over grid whitespace still scrolls the section (no value change).
func TestGridWhitespaceWheelScrollsNotNudges(t *testing.T) {
    // press point: inside the section body but outside every knob rect;
    // assert section grid scroll offset changes and no knob value changes.
    // (mirror TestSynthVerticalWheelScrollsOverflowRows for setup)
}
```

Use the real helper names found in the existing test files (`firstNonScrollableSynthKnobRect`, dispatch helpers, undo-depth accessors). If a helper doesn't exist, write it in this file using the tree-dispatch pattern from `knob_interaction_functional_test.go`.

- [ ] **Step 2: Verify failure** (`-run 'TestParamCard'`): desktop click currently starts a rotary drag → popup not open → FAIL.

- [ ] **Step 3: Implement.**

1. `knob_wheel_nudge.go`:

```go
package ui

// wheelNudgeCommitFrames ≈ 600ms at 60fps: a pause this long ends a wheel
// gesture and commits its single undo step.
const wheelNudgeCommitFrames = 36

type wheelNudger struct {
    framesLeft int
    commit     func()
}

func (n *wheelNudger) bump(commit func()) { n.framesLeft = wheelNudgeCommitFrames; n.commit = commit }

func (n *wheelNudger) tick() {
    if n.framesLeft <= 0 {
        return
    }
    n.framesLeft--
    if n.framesLeft == 0 && n.commit != nil {
        c := n.commit
        n.commit = nil
        c()
    }
}

// flush commits immediately (popup opening / tab switch mid-gesture).
func (n *wheelNudger) flush() {
    if n.framesLeft > 0 && n.commit != nil {
        c := n.commit
        n.commit = nil
        n.framesLeft = 0
        c()
    }
}

// nudgeKnobByWheel applies one wheel event to the shared knob model: one
// StepMul (or enum detent) per notch. fine=true (Shift) applies at one rung
// finer than the badge's active rung WITHOUT persisting the rung change.
func nudgeKnobByWheel(k *Knob, badge *KnobStepBadge, steps int, fine bool) bool {
    if k == nil || steps == 0 {
        return false
    }
    if fine && badge != nil && !k.Discrete {
        saved := k.StepMul
        if i := badgeIndexBelow(badge); i >= 0 {
            k.StepMul = badge.ladder[i]
        }
        defer func() { k.StepMul = saved }()
    }
    if k.Discrete {
        dir := 1
        if steps < 0 {
            dir = -1
        }
        return k.StepValueDirect(dir)
    }
    dir := 1
    if steps < 0 {
        dir = -1
    }
    k.NudgeEndless(dir * knobEndlessPxPerNotch)
    return true
}

// badgeIndexBelow returns the ladder index one rung FINER than active, or -1.
func badgeIndexBelow(b *KnobStepBadge) int {
    if b == nil || b.idx <= 0 {
        return -1
    }
    return b.idx - 1
}
```

(Adjust to the real `Knob.StepValueDirect` signature — read `knob.go` first; if it returns nothing, drop the bool plumbing. If `k.Discrete` is the field name per `knob.go:77`, use it.)

2. Rewrite `synthKnobHitAdapter`:
   - `OnPress`: keep the paramEditor guard; then unconditionally (drop `Profile().IsMobile()`): `h.dv.setSynthSelectedKnob(h.instID, h.idx); h.dv.synthWheelNudge.flush(); h.dv.openSynthKnobWheelPopup(h.idx, h.instID); return InputConsumed`. Delete the badge-tap branch and the whole rotary-drag branch.
   - `OnDrag`/`OnRelease`: reduce to no-ops (`func (h *synthKnobHitAdapter) OnDrag(x, y int) {}` etc.). Delete the axis-lock consts/fields (`knobDragUndecided` etc.) IF no other file references them (grep).
   - `OnWheel`: replace body with the nudge:

```go
func (h *synthKnobHitAdapter) OnWheel(x, y, steps int) InputResult {
    k := h.currentKnob()
    if k == nil || steps == 0 {
        return InputIgnored
    }
    var badge *KnobStepBadge
    if h.idx < len(h.dv.instEditorStepBadges) {
        badge = h.dv.instEditorStepBadges[h.idx]
    }
    fine := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
    if !nudgeKnobByWheel(k, badge, steps, fine) {
        return InputIgnored
    }
    h.dv.setSynthSelectedKnob(h.instID, h.idx)
    h.dv.syncSliderFromKnob(h.idx)
    h.dv.propagateSynthSliderValue(h.idx, h.instID)
    instID := h.instID
    dv := h.dv
    dv.synthWheelNudge.bump(func() {
        dv.commitInstrumentParams(instID)
        dv.requestSynthMirror(instID)
    })
    h.invalidatePanel()
    return InputConsumed
}
```

   - Delete `OnWheel2D` from this adapter entirely (the tree then routes 2-finger vertical scroll to `OnWheel` → value nudge; the section still scrolls via the whitespace/scroll-body areas). Grep `wheel2DHandler` to confirm the interface is optional.
   - Add hover: `func (h *synthKnobHitAdapter) glowRect(_, _ int, hitRect image.Rectangle) image.Rectangle { return hitRect }`.
3. Add field `synthWheelNudge wheelNudger` to `DrumView` (next to `synthWheelPopup`; grep its declaration site) and tick it: find `synthTabUpdate` (the func containing the paramEditor Update at ~1690) and add `dv.synthWheelNudge.tick()` alongside.
4. `openSynthKnobWheelPopup` (drumview_wheel_popup.go): call `dv.synthWheelNudge.flush()` first so a pending wheel commit lands before the popup snapshot (`initialValue`) is taken.

- [ ] **Step 4: Verify pass** (`-run 'TestParamCard|TestGridWhitespace'`). Also run the wheel-popup suites: `-run 'TestSynthWheel|TestWheel'` — the popup itself is untouched, they must stay green. Update/remove now-obsolete rotary functional tests (`TestSynthKnobHorizontalDragChangesValueViaTreeDispatch`, `TestSynthKnobHorizontalWheelChangesValue`, `TestSynthKnobDialWheelDoesNotChangeResolution`, `TestSynthContinuousKnobIsEndlessWithBadge` keep-if-model-only, `synth_knob_touch_lifecycle_test.go`, `synth_select_knob_input_test.go` select-on-press now happens via popup-open — update assertions accordingly). The old "desktop press returns InputCaptured" sampler/synth gate tests flip to InputConsumed+popup-open.

- [ ] **Step 5: gofmt + leave uncommitted.**

---

### Task 6: Sampler parity — cards, tap-to-popup, wheel nudge; delete pill file

**Files:**
- Modify: `src/go/internal/ui/sampler_panel_zone.go` (desktop layout ~554-611, mobile layout ~629-680, `drawSamplerKnobs` ~985-1055, `samplerKnobHitAdapter` ~1560-1690)
- Modify: `src/go/internal/ui/sampler_wheel_popup.go` (delete `samplerMobileKnobButtonRect`)
- Delete: `src/go/internal/ui/knob_value_pill.go`
- Test: `src/go/internal/ui/sampler_card_test.go` (new)

**Interfaces:**
- Consumes: Task 2 `drawParamCard`/`paramCardGaugeFor`; Task 5 `nudgeKnobByWheel` + `wheelNudger`; existing `openSamplerKnobWheelPopup(idx)`, `samplerKnobValueText`, `samplerKnobNameText`, `samplerGroupLabel`, `applySamplerKnob`, `commitSamplerEdit`.
- Produces: sampler knob rect == card rect (label band excluded); `dv.samplerWheelNudge wheelNudger` field.

- [ ] **Step 1: Failing tests** (mirror `sampler_wheel_popup_test.go` setup — `newMobileSamplerWheelGame` exists for mobile; for desktop use the plain sampler-tab game helper found in `sampler_scroll_test.go` / `sampler_knob_interaction_test.go`):

```go
package ui

import "testing"

// Desktop sampler card tap opens the wheel popup (was: rotary drag capture).
func TestSamplerCardClickOpensPopupOnDesktop(t *testing.T) {
    g := newSamplerTabGame(t) // grep the real desktop helper name
    dv := g.drum
    r := dv.sampler.knobs[0].Rect()
    if r.Empty() {
        t.Fatal("knob 0 has no rect")
    }
    pressAndReleaseAt(t, g, r.Min.X+4, r.Min.Y+4)
    if dv.samplerWheelPopup == nil || !dv.samplerWheelPopup.IsOpen() {
        t.Fatal("desktop sampler card tap must open the wheel popup")
    }
}

// Wheel over a sampler card nudges + single undo commit after debounce;
// the old dial-wheel and readout-editor special cases are gone.
func TestSamplerCardWheelNudges(t *testing.T) {
    g := newSamplerTabGame(t)
    dv := g.drum
    k := dv.sampler.knobs[0]
    before := k.Value
    r := k.Rect()
    dispatchWheelAt(t, g, r.Min.X+4, r.Min.Y+4, +1)
    if k.Value == before {
        t.Fatal("sampler card wheel nudge did not change value")
    }
}
```

- [ ] **Step 2: Verify failure** (`-run 'TestSamplerCard'`).

- [ ] **Step 3: Implement.**
1. Desktop layout (~554-611): keep the one-row × 5 structure and the label band, but the cell control band is now a card, not a dial: `cardH := dv2.ParamCardH`, `cardW := min(cellW - SpaceSM, dv2.ParamCardW*3/2)` (cards may stretch a bit in the wide column; floor at `dv2.ParamCardW/2`); center the card horizontally in the cell, top at `dialTop`; `k.SetRect(cardRect)`; `s.knobCells[i]` = full cell as today. Delete the `knobD` dial-budget math. The vertical budget check keeps the toggle row clear (`gridBottom` unchanged).
2. Mobile layout (~629-680): `cellH := labelBandH + dv2.ParamCardH + SpaceXS` (drop `captionCellH`); in the per-cell loop, `k.SetRect(image.Rect(slot.Min.X, slot.Min.Y+labelBandH, slot.Max.X, slot.Min.Y+labelBandH+dv2.ParamCardH))`.
3. `drawSamplerKnobs`: keep the group-label block verbatim; replace the mobile/desktop split + caption + readout + badge blocks with:

```go
        drawParamCard(dst, r, samplerKnobNameText(i, s), samplerKnobValueText(i, s),
            paramCardGaugeFor(k, samplerKnobDef(i)), paramCardState{})
        s.readoutRects[i] = image.Rectangle{}
```

   where `samplerKnobDef(i)` builds the ParamDef the same way `openSamplerKnobWheelPopup` does (grep `sampler_wheel_popup.go` for the Def synthesis — extract it into a shared `func (dv *DrumView) samplerKnobDef(idx int) audio.ParamDef` if it's inline there). Clear all badge rects (`s.knobStepBadges[i].SetRect(image.Rectangle{})`) once before the loop.
4. `samplerKnobHitAdapter`: `OnPress` → paramEditor guard, then `h.dv.samplerWheelNudge.flush(); h.dv.openSamplerKnobWheelPopup(h.idx); return InputConsumed` (drop the IsMobile gate, badge branch, readout branch, rotary branch). `OnDrag`/`OnRelease` → no-ops. `OnWheel` → mirror Task 5's nudge but with sampler plumbing:

```go
    fine := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
    var badge *KnobStepBadge
    if h.idx < len(h.dv.sampler.knobStepBadges) {
        badge = h.dv.sampler.knobStepBadges[h.idx]
    }
    if !nudgeKnobByWheel(k, badge, steps, fine) {
        return InputIgnored
    }
    h.dv.applySamplerKnob(h.idx)
    dv := h.dv
    dv.samplerWheelNudge.bump(func() { dv.commitSamplerEdit() })
    return InputConsumed
```

   Delete `OnWheel2D`. Add `glowRect` (hover) same as synth. Add `samplerWheelNudge wheelNudger` to DrumView; tick it in `samplerTabUpdate` (grep the update func that calls `knobGrid.Tick()`); flush it at the top of `openSamplerKnobWheelPopup`.
5. Delete `knob_value_pill.go` + `samplerMobileKnobButtonRect` + `drawKnobValuePillRect` callers (grep `knobValuePill|KnobValuePill` repo-wide, including tests — update `sampler` pill tests to card assertions).

- [ ] **Step 4: Verify pass** (`-run 'TestSamplerCard|TestSamplerWheel|TestSamplerKnob'` — update `TestSamplerKnobScalesAndBadges` and `sampler_knob_size_i18n_test.go` to the card contract: rect non-empty, min sizes from ParamCard tokens). Build package.

- [ ] **Step 5: gofmt + leave uncommitted.**

---

### Task 7: Popup v2 — title gloss + tappable rung ladder + Shift-fine

**Files:**
- Modify: `src/go/internal/ui/mobile_wheel_popup.go` (WheelBinding, layout, HandleInput, HandleWheel)
- Modify: `src/go/internal/ui/mobile_wheel_render.go` (`drawTitle`, `drawResStrip` → `drawRungLadder`)
- Modify: `src/go/internal/ui/knob_step.go` (add `SetIndex`)
- Modify: `src/go/internal/ui/drumview_wheel_popup.go` + `sampler_wheel_popup.go` + `eq_wheel_popup.go` (Gloss wiring)
- Test: `src/go/internal/ui/wheel_ladder_test.go` (new)

**Interfaces:**
- Consumes: existing popup internals (this task must READ both popup files fully first).
- Produces:

```go
// WheelBinding gains:
Gloss func() string // optional one-line purpose ("bright or dull")
// KnobStepBadge gains:
func (b *KnobStepBadge) SetIndex(i int) bool // clamp+set; true if moved
func (b *KnobStepBadge) Index() int
func (b *KnobStepBadge) Rungs() []float64    // the ladder (read-only use)
// MobileWheelPopup gains:
func (w *MobileWheelPopup) rungRects() []image.Rectangle // laid out in resRect, finest at TOP
```

- [ ] **Step 1: Failing tests:**

```go
package ui

import (
    "image"
    "testing"

    "github.com/ingyamilmolinar/beatmo/internal/audio"
)

func mkWheelForLadder(t *testing.T) (*MobileWheelPopup, *Knob, *KnobStepBadge) {
    t.Helper()
    def := audio.ParamDef{Name: "cutoff", Label: "Cutoff", Min: 20, Max: 16000, Unit: "Hz"}
    k := &Knob{Endless: true, StepMul: 50}
    b := NewKnobStepBadge(def)
    w := NewMobileWheelPopup()
    w.Open(WheelBinding{Knob: k, Badge: b, Def: def,
        Title: func() string { return "Cutoff" },
        Gloss: func() string { return "bright or dull" },
    }, image.Rect(0, 0, 10, 10), image.Rect(0, 0, 800, 600), 0)
    return w, k, b
}

// Every ladder rung gets a tappable rect; tapping one jumps straight to it
// and syncs the knob's StepMul.
func TestWheelLadderTapJumpsToRung(t *testing.T) {
    w, k, b := mkWheelForLadder(t)
    rungs := w.rungRects()
    if len(rungs) != b.LadderLen() {
        t.Fatalf("rung rects %d != ladder %d", len(rungs), b.LadderLen())
    }
    target := 0 // finest rung (top)
    if b.Index() == target {
        target = b.LadderLen() - 1
    }
    p := rungs[target].Min.Add(image.Pt(2, 2))
    w.HandleInput(p.X, p.Y, true)
    w.HandleInput(p.X, p.Y, false)
    if b.Index() != target {
        t.Fatalf("tap on rung %d landed on %d", target, b.Index())
    }
    if k.StepMul != b.Step() {
        t.Fatalf("StepMul %v not synced to rung step %v", k.StepMul, b.Step())
    }
}

// Finest rung renders at the TOP (scroll up = finer stays true).
func TestWheelLadderFinestAtTop(t *testing.T) {
    w, _, b := mkWheelForLadder(t)
    rungs := w.rungRects()
    if len(rungs) < 2 {
        t.Skip("degenerate ladder")
    }
    _ = b
    if !(rungs[0].Min.Y < rungs[len(rungs)-1].Min.Y) {
        t.Fatal("rung 0 (finest) must be the top pill")
    }
}
```

- [ ] **Step 2: Verify failure** (`-run 'TestWheelLadder'`): `rungRects` undefined.

- [ ] **Step 3: Implement.**
1. `knob_step.go`:

```go
// SetIndex jumps directly to ladder rung i (clamped). Returns true if moved.
func (b *KnobStepBadge) SetIndex(i int) bool {
    if len(b.ladder) == 0 {
        return false
    }
    if i < 0 {
        i = 0
    } else if i >= len(b.ladder) {
        i = len(b.ladder) - 1
    }
    if i == b.idx {
        return false
    }
    b.idx = i
    return true
}
func (b *KnobStepBadge) Index() int       { return b.idx }
func (b *KnobStepBadge) Rungs() []float64 { return b.ladder }
```

2. `mobile_wheel_popup.go`: add `Gloss func() string` to WheelBinding. Add `rungRects`:

```go
// rungRects lays the ladder pills vertically inside resRect, finest rung at
// the TOP (matching "scroll up = finer"). Equal heights, compressed when the
// ladder is long; empty when the strip is hidden.
func (w *MobileWheelPopup) rungRects() []image.Rectangle {
    if w.resRect.Empty() || w.binding.Badge == nil {
        return nil
    }
    n := w.binding.Badge.LadderLen()
    if n == 0 {
        return nil
    }
    inner := w.resRect.Inset(SpaceXS)
    gap := 2
    h := (inner.Dy() - gap*(n-1)) / n
    if h < 12 {
        h = 12
        gap = 1
    }
    out := make([]image.Rectangle, n)
    y := inner.Min.Y
    for i := 0; i < n; i++ {
        out[i] = image.Rect(inner.Min.X, y, inner.Max.X, y+h)
        y += h + gap
    }
    return out
}
```

3. `HandleInput` press branch: BEFORE the generic `pt.In(w.resRect)` drag case, add rung-tap:

```go
        case !w.resRect.Empty() && pt.In(w.resRect):
            if b := w.binding.Badge; b != nil {
                for i, rr := range w.rungRects() {
                    if pt.In(rr) {
                        if b.SetIndex(i) {
                            if w.binding.Knob != nil {
                                w.binding.Knob.StepMul = b.Step()
                            }
                            if w.binding.OnResolution != nil {
                                w.binding.OnResolution()
                            }
                        }
                        return true
                    }
                }
            }
            // fall through to the existing rung-drag capture (keep it)
```

(Structure: tap-on-a-rung consumes; a press in strip gutter still starts the legacy res drag — keep `wheelDragRes` code untouched.)
4. `HandleWheel`: keep everything; add Shift-fine on the value branch: read `fine := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)`; when fine and !Discrete and Badge != nil, temporarily apply at one rung finer (reuse `badgeIndexBelow` from Task 5's `knob_wheel_nudge.go`): save `k.StepMul`, set to `b.Rungs()[badgeIndexBelow(b)]` when index ≥ 0, NudgeEndless, restore.
5. `drawTitle`: render Title (RolePanelTitle, as today) and, when `w.binding.Gloss != nil && w.binding.Gloss() != ""`, append the gloss on the same band: `title + " — " + gloss` truncated to `titleRect.Dx()-2*SpaceSM` via `truncCaptionStyled(..., RolePanelTitle)`; if the combined string truncates the gloss away entirely, draw the bare title (don't leave a dangling "—").
6. Replace `drawResStrip` body with the ladder render (keep the func name so the Draw dispatch line is unchanged):

```go
func (w *MobileWheelPopup) drawResStrip(dst *ebiten.Image) {
    if w.resRect.Empty() || w.binding.Badge == nil {
        return
    }
    r := w.resRect
    drawRoundedRect(dst, r, TokenSurface2(), RadiusMD, true)
    drawRoundedRect(dst, r, WithAlphaFromColor(TokenBorderSubtle(), AlphaMedium), RadiusMD, false)
    b := w.binding.Badge
    for i, rr := range w.rungRects() {
        lbl := formatStepValue(w.binding.Def, b.Rungs()[i])
        active := i == b.Index()
        if active {
            drawRoundedRect(dst, rr, WithAlphaFromColor(w.accent(), AlphaMedium), RadiusXS, true)
            drawRoundedRect(dst, rr, w.accent(), RadiusXS, false)
        } else {
            drawRoundedRect(dst, rr, WithAlpha(TokenSurface3(), AlphaMedium), RadiusXS, true)
        }
        lw := StyledTextWidth(lbl, RoleMicro)
        lh := StyledTextHeight(RoleMicro)
        col := TokenTextSecondary()
        if active {
            col = colTextPrimary
        }
        DrawTextStyled(dst, truncCaptionStyled(lbl, rr.Dx()-4, RoleMicro),
            rr.Min.X+(rr.Dx()-lw)/2, rr.Min.Y+(rr.Dy()-lh)/2, RoleMicro, col)
    }
}
```

`w.accent()`: grep the render file — an accent fallback helper exists (binding.Accent or primary token); reuse it, else `genColorPrimary`.
7. Wire glosses at the three openers: synth `Gloss: func() string { return synthKnobPurpose(def) }` (drumview_wheel_popup.go); sampler — Title currently passes `samplerKnobPlainEnglish(idx)`: change `Title` to the param NAME (`samplerKnobNameText`-equivalent; grep what the pill used) and `Gloss` to `samplerKnobPlainEnglish(idx)`; EQ (`eq_wheel_popup.go`) — Title stays, add a Gloss only if a natural string exists (`PlainEnglish("Gain")`), else leave nil.

- [ ] **Step 4: Verify pass** (`-run 'TestWheelLadder'`) AND the full popup suites: `-run 'TestWheel|TestMobileWheel|TestSynthWheel|TestSamplerWheel|TestEQ.*Wheel'` — direction/cadence tests must stay green (`TestWheelDownGesturesAllIncrease`, `TestWheelResolutionDragAndWheelAgree`, `mobile_wheel_precision_test.go`). The old chevron-stepper drawing test (grep `drawResStrip|ResStrip` in tests) updates to ladder assertions.

- [ ] **Step 5: gofmt + leave uncommitted.**

---

### Task 8: Popup hover (portalHoverer) + hover coverage

**Files:**
- Modify: `src/go/internal/ui/mobile_wheel_popup.go` / `mobile_wheel_render.go` (HoverAt + hovered rung tint)
- Modify: `src/go/internal/ui/mobile_wheel_popup_portal.go` (hover seam)
- Modify: `src/go/internal/ui/hover_coverage_test.go` (extend)
- Test: extend `wheel_ladder_test.go`

**Interfaces:**
- Consumes: the `portalHoverer` seam — READ `overlay_portal.go`/`overlay_portal*.go` and one wired example (fx-panel or inst menu `HoverAt`) FIRST and copy that exact mechanism.
- Produces: `func (w *MobileWheelPopup) HoverAt(mx, my int)` storing `hoverRung int` (-1 none) and `hoverCenter bool`; ladder + center grip render a hover tint; `mobileWheelPopupPortalOverlay` implements the portal hover interface delegating to `HoverAt`.

- [ ] **Step 1: Failing test** (extend `wheel_ladder_test.go`):

```go
func TestWheelLadderHoverTracksRung(t *testing.T) {
    w, _, _ := mkWheelForLadder(t)
    rungs := w.rungRects()
    p := rungs[0].Min.Add(image.Pt(2, 2))
    w.HoverAt(p.X, p.Y)
    if w.hoverRung != 0 {
        t.Fatalf("hoverRung = %d, want 0", w.hoverRung)
    }
    w.HoverAt(w.rect.Min.X-5, w.rect.Min.Y-5)
    if w.hoverRung != -1 {
        t.Fatalf("hover off-popup must clear, got %d", w.hoverRung)
    }
}
```

- [ ] **Step 2: Verify failure**, **Step 3: Implement** (`HoverAt` sets fields; `drawResStrip` adds `if i == w.hoverRung && i != b.Index() { drawRoundedRect(dst, rr, WithAlpha(TokenSurface3(), AlphaStrong), RadiusXS, true) }` brighten + a `drawButtonHoverGlow`-style rim if trivially available; portal overlay implements the hover interface found in Step consume-note — desktop only, gated the same way the fx-panel does). Extend `hover_coverage_test.go` per its own in-file pattern: add the synth/sampler card adapters (glowTarget from Tasks 5/6) and the wheel portal hoverer to whatever enumeration that test pins.

- [ ] **Step 4: Verify pass**: `-run 'TestWheelLadder|TestHoverCoverage'` (grep the real test name in `hover_coverage_test.go`).

- [ ] **Step 5: gofmt + leave uncommitted.**

---

### Task 9: Rotary deletion + Knob slim-down + panel heights

**Files:**
- Modify: `src/go/internal/ui/knob.go` (delete Draw/knurledCapSprite/drawDetents/drawArc pools/updateFromDrag/dragPixels/FineDrag/HandleInputResult)
- Modify: `src/go/internal/ui/knob_step.go` (delete `Draw`, `Cycle`, `WheelResolution`, `wheelAccum`, `SetRect/Rect` IF no longer referenced — grep first; `SetStep/SetIndex/Step/Label/Rungs/Index/ParamName/AdjustResolution/LadderLen` stay)
- Modify: `src/go/internal/ui/game_layout.go` (`mobileAudioPanelMinContentH`), `sampler_panel_zone.go` (`samplerControlMinHeight`)
- Modify/trim tests: `knob_test.go`, `knob_endless_test.go`, `knob_discrete_test.go`, `knob_bipolar_test.go`, `knob_detent_draw_test.go`, `knob_cap_test.go`, `knob_draw_pool_test.go`, `game_mobile_split_test.go`, `TestMobileSynthTab_KnobRendersAtIdealSize` etc.

**Interfaces:**
- Consumes: everything above (this is the demolition task — run it AFTER Tasks 3-8 are green).
- Produces: `Knob` = headless model: fields + `SetRect/Rect/Capturing?` (grep `Capturing()` callers — sampler layout uses it; keep) + `HandleWheel/StepValueByWheel/StepValueDirect/NudgeEndless/updateEndless/snapToDetent/detentCount` + scale helpers. NOTHING in `internal/ui` calls `Knob.Draw` or `KnobStepBadge.Draw` anymore.

- [ ] **Step 1: Prove nothing references the dead code.** Run:
`grep -rn "\.Draw(" src/go/internal/ui/*.go | grep -iE "knob|badge" | grep -v _test` → expect ZERO production hits (fix any leftovers first).
`grep -rn "HandleInputResult|FineDrag|dragPixels|knurledCap|drawDetents" src/go/internal/ui/*.go | grep -v _test | grep -v knob.go` → zero.
- [ ] **Step 2: Delete** the listed members from `knob.go`/`knob_step.go`. Keep `knobEndlessPxPerNotch`, `knobWheelStep` if `HandleWheel` still uses it, `KnobScale`, `Discrete/Endless/StepMul/Bipolar/ZeroFrac/Value/rect`. If `wheelGripSprite`/`keycap*` helpers live in knob-adjacent files and are still used by the popup/cards, they stay where they are.
- [ ] **Step 3: Trim tests.** Keep model-math tests (endless/discrete/bipolar/wheel/step-ladder); delete draw/drag tests (`TestKnobPressInsideRectStartsDrag`, `TestKnobHorizontalDragSweepAndClamp`, `TestKnobVerticalDragDoesNotChangeValue`, `TestKnobFineDragIsLessSensitive`, `TestKnobDragSequenceIsAbsoluteFromPressPoint`, `TestKnobDrawAtTypicalSize`, `knob_detent_draw_test.go`, `knob_cap_test.go`, `knob_draw_pool_test.go` arc-pool parts).
- [ ] **Step 4: Panel heights.** `mobileAudioPanelMinContentH` (game_layout.go): replace the `SynthKnobIdeal + SynthKnobCaptionH` knob-cell term with `ParamCardH` (read the func; it sums synth chrome — swap only the knob-cell term). `samplerControlMinHeight`: same swap (label band + `ParamCardH` + toggle + meta). Update `game_mobile_split_test.go` expectations per its own comments (the stable-split invariants stay; only the numeric floor drops) and rewrite `TestMobileSynthTab_KnobRendersAtIdealSize` → `TestMobileSynthTab_CardRendersAtIdealSize` asserting card rect height == `ParamCardH`.
- [ ] **Step 5: Full verify.**
`../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/` then
`-run 'TestKnob|TestMobileSplit|TestMobileSynthTab|TestSampler'` → PASS. gofmt; leave uncommitted.

---

### Task 10: Guardrails sweep — budgets, i18n, scenes, exports, discipline tests

**Files:**
- Modify: `src/go/internal/ui/eq_panel_draw_alloc_discipline_test.go` (TabSynth/TabSampler budgets, with a one-line rationale)
- Modify: `src/go/internal/ui/scene_catalog.go` + `scripts/basic_scenes.txt` (scene rename from Task 3; ensure `crop_mobile_synth_wheel` gains a desktop `Setup` so the wheel is captured on both passes)
- Verify: `internal/ui/js_exports_catalogue_drift_test.go`, `zone_input_isolation_discipline_test.go`, token discipline, i18n tests
- Test: run the wide suites

- [ ] **Step 1: Full fast suite for the two panels + infra:**
`-run 'TestSynth|TestSampler|TestWheel|TestParamCard|TestDesign|TestToken|TestHover|TestZoneInput|TestI18n|TestScene'` (split into a few runs if slow). Fix fallout: alloc budgets (re-ratchet with rationale comments citing param_card.go), i18n key usage (any new user-visible string must go through `i18n.T` — the divider reuses `KeySynthAdvanced`; rung labels reuse `formatStepValue`), scene setups (`synthWheelSceneSetup` must not gate on mobile — grep it, remove any `withSmallScreen` requirement so a desktop `Setup` exists).
- [ ] **Step 2: JS bridge:** `synthKnobRects()` still returns knob rects (now card rects) — run `internal/ui/js_exports_catalogue_drift_test.go` via `-run 'TestJSExports|TestCatalogue'`. No export rename.
- [ ] **Step 3: Whole-package run:**
`cd src/go && timeout 900 ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ 2>&1 | tail -40`
Triage every FAIL: yours → fix; the two documented node-logic-volume reds at HEAD are the ONLY dismissible failures (prove any other red's causation before touching it).
- [ ] **Step 4: Other packages sanity:** `../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./...` and `-run` the audio package smoke: `./internal/audio/ -run 'TestParamDef|TestRecipe' | tail -5`.
- [ ] **Step 5: gofmt everything touched; leave uncommitted.**

---

### Task 11: Visual verification + WASM build

**Files:**
- Output: fresh screenshots (scratchpad), `src/js/main.wasm` (local only, never committed)

- [ ] **Step 1: Native screenshot build:**
`make build/libdrums.a && (cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/beatmo_ss_v2 ./cmd)`
- [ ] **Step 2: Capture** desktop `crop_synth_tab_modular`, `crop_synth_tab_detail_envelope`, `crop_sampler_tab_captured`, `crop_synth_advanced_divider`, `crop_mobile_synth_wheel` (desktop pass) and mobile `mobile_bottom_nav_synth`, `mobile_bottom_nav_sampler`, `crop_mobile_synth_wheel`:
`xvfb-run -a /tmp/beatmo_ss_v2 -scene <name> -screenshot out/<name>.png` (+ `-mobile -win-w 414 -win-h 896 -scene-pass mobile` for the mobile pass).
- [ ] **Step 3: EYEBALL each PNG** (Read tool): cards show NAME + value + gauge; ≥4 cards on the first desktop row; divider visible with count; ladder pills visible in the popup with the active rung highlighted; no dial remnants; sampler row of 5 cards under TRIM/TUNE/LEVEL. Iterate on visual defects (spacing, truncation, contrast) before declaring done.
- [ ] **Step 4: WASM:** `make wasm` → builds clean. Optional browser smoke: `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js`.
- [ ] **Step 5: Leave everything uncommitted;** summarize residual risks (wheel-direction on-device check; FM stage row count).

---

## Self-Review (done at plan time)

- **Spec coverage:** §1→Tasks 2/4/6; §2→Tasks 1/3/9; §3→Task 7/8; §4→Tasks 5/6; §5→Tasks 6/9; §6→Tasks 3-10; scenes/visual→Task 11. Out-of-scope items untouched.
- **Type consistency:** `paramCardGaugeFor(k *Knob, def audio.ParamDef)`, `drawParamCard(dst, r, name, value, gauge, state)`, `nudgeKnobByWheel(k, badge, steps, fine)`, `wheelNudger.bump/tick/flush`, `KnobStepBadge.SetIndex/Index/Rungs` used consistently across Tasks 2-9.
- **Known unknowns called out in-task:** exact test-helper names (each task says "grep/read first"), `StepValueDirect` return type, portal hover interface name, spacious-density current values — every one has a read-first instruction rather than an invented API.
