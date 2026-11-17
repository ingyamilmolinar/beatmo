# Mechanical Keycap Buttons, Knurled Knobs & Analogue Motion — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give every interactive control (buttons, knobs, slider thumbs) a voluminous mechanical-keycap look with analogue press/release motion, a lit-&-raised active state, and a restrained hover rim — replacing the current cyan bloom — while staying allocation-free and WASM-audio-safe.

**Architecture:** One shared `Button` type feeds every button, so the work lands in `uigrid.go` (state + Draw), `button_cushion.go` (render helpers), `hover_glow_overlay.go` (hover paint), `drawing.go` (cached chrome), `knob.go`, and `slider_neon.go`. The binding rule: **static volume bakes into the existing sprite caches; all motion is expressed as cheap blit-time destination offsets + alpha overlays — never per-frame re-rasterization.** A control at rest or steadily-on does zero per-frame work.

**Tech Stack:** Go 1.23, Ebiten v2, the DESIGN.md → `make gen-design-tokens` → `design_tokens.gen.go` token pipeline, `-tags test -modfile=go.test.mod` fast test path.

---

## Conventions for this repo (READ FIRST)

- **Bundled Go:** always `../../.tools/go/bin/go` from `src/go`. Fast tests:
  `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run <Name> ./internal/ui/`
- **Verify with explicit package paths** (`./internal/ui/`), never `./...` — this branch has a phantom duplicate-package build error under `./...`. Trust `go vet -tags test ./internal/ui/` over gopls "undefined" flapping.
- **Do NOT `git add`/commit/stash/restore/`rm`.** Standing user rule: leave all changes in the working tree; the user owns commits. Each task ends with a **Checkpoint** (run tests, leave tree dirty) — there are no `git commit` steps.
- **Branch carries large foreign WIP.** If a `zz_*.go` orphan or another in-flight file breaks the build, `mv` it aside to `/tmp` (never `rm`) and/or use `-vet=off`; verify your own work with `-run` filters. The full `internal/ui` suite has pre-existing unrelated failures.
- **Token discipline:** do not introduce `color.RGBA{}`/`color.NRGBA{}` literals in `button_cushion.go`, `knob.go`, `slider_neon.go`, `hover_glow_overlay.go` (budget 0). Derive shades via `adjustColor(...)`, `WithAlpha(...)`, `WithAlphaFromColor(...)`, or `Token*()` accessors — none of which are literals.

## File structure / responsibilities

| File | Responsibility after this plan |
|---|---|
| `DESIGN.md` | New geometry + animation tokens; updated Elevation & Depth prose |
| `design_tokens.gen.go` | Regenerated `genGeom*`/`genAnim*` constants (never hand-edit) |
| `design_md_drift_test.go` | want-maps extended for new tokens (+ close the hover-glow-spread gap) |
| `button_cushion.go` | Keycap shell/cap/wall geometry + draw, engage-flash, hover-rim paint |
| `uigrid.go` | `Button` press-depth travel model, engage flash, lit-&-raised in Draw |
| `hover_glow_overlay.go` | Unchanged routing; calls the new rim paint |
| `drawing.go` | Cached cap chrome (unchanged signature; fills the cap rect) |
| `knob.go` | Cached knurled cap sprite + dynamic arc/pointer |
| `slider_neon.go` | Sculpted-cap slider thumb |
| `button_keycap_test.go` (new) | Geometry-helper + travel-model unit tests |
| `button_perf_guard_test.go` (new) | Alloc-rate + settle-to-silence guards |

---

## Task 1: Design tokens (geometry + animation) + regenerate + drift test

**Files:**
- Modify: `DESIGN.md` (geometry block ~line 999–1012; animations block ~1040–1075)
- Modify (generated): `src/go/internal/ui/design_tokens.gen.go` (via `make gen-design-tokens`)
- Modify: `src/go/internal/ui/design_md_drift_test.go` (`wantGeometry` ~line 358; `wantExpDecay` ~line 287)

- [ ] **Step 1: Add geometry tokens to DESIGN.md.** In the `geometry:` block, immediately after the `button-hover-glow-spread: 3` entry, add:

```yaml
  # Mechanical-keycap geometry (see "Elevation & Depth" prose). The keycap
  # affordance extrudes a cap above a darker side-wall/socket; pressing
  # travels the cap straight down until it bottoms out.
  # keycap-wall-depth: rest side-wall depth in px == max press travel.
  keycap-wall-depth: 4
  # keycap-active-raise: px a latched/active key sits raised above rest.
  keycap-active-raise: 1
  # keycap-release-overshoot-frac: fraction of wall-depth the cap springs
  #   UP past rest on release before settling (analogue overshoot).
  keycap-release-overshoot-frac: 0.18
```

- [ ] **Step 2: Add the engage-flash animation to DESIGN.md.** In the `animations:` block, after `button-hover-fade`, add:

```yaml
  # One-shot brightness flash fired when a key latches ON (off→on). Decays
  # to silence so a steadily-on key does zero per-frame work. Drives
  # Button.engageAnim via DecayStep in button_cushion.go.
  button-engage-flash:
    kind: exp-decay
    rate: 0.82
    threshold: 0.02
```

- [ ] **Step 3: Regenerate tokens.**

Run: `make gen-design-tokens`
Expected: `src/go/internal/ui/design_tokens.gen.go` now contains
`genGeomKeycapWallDepth = 4`, `genGeomKeycapActiveRaise = 1`,
`genGeomKeycapReleaseOvershootFrac = 0.18`, and
`genAnimButtonEngageFlash = ExpDecayAnim{Rate: 0.82, Threshold: 0.02}`.
Verify: `grep -n 'Keycap\|EngageFlash' src/go/internal/ui/design_tokens.gen.go`

- [ ] **Step 4: Extend the drift-test want-maps.** In `design_md_drift_test.go`, add the new keys to `wantGeometry` (and close the pre-existing `button-hover-glow-spread` gap in the same edit):

```go
	"button-press-scale":            genGeomButtonPressScale,
	"button-release-overshoot":      genGeomButtonReleaseOvershoot,
	"button-hover-glow-spread":      genGeomButtonHoverGlowSpread, // was missing
	"keycap-wall-depth":             genGeomKeycapWallDepth,
	"keycap-active-raise":           genGeomKeycapActiveRaise,
	"keycap-release-overshoot-frac": genGeomKeycapReleaseOvershootFrac,
```

And add to `wantExpDecay`:

```go
	"highlight-decay":     genAnimHighlightDecay,
	"button-press-decay":  genAnimButtonPressDecay,
	"button-hover-fade":   genAnimButtonHoverFade, // was only in code, pin it
	"button-engage-flash": genAnimButtonEngageFlash,
```

- [ ] **Step 5: Run the drift test → PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDesignMDDrift ./internal/ui/`
Expected: PASS. (If `button-hover-fade` was already present in the want-map, keep a single entry — duplicate map keys won't compile.)

- [ ] **Step 6: Checkpoint.** Build the package: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` → no new errors. Leave the working tree dirty (no commit).

---

## Task 2: Keycap geometry + shell/cap/wall render helpers (pure, TDD)

**Files:**
- Create: `src/go/internal/ui/button_keycap_test.go`
- Modify: `src/go/internal/ui/button_cushion.go`

- [ ] **Step 1: Write failing geometry tests.** Create `button_keycap_test.go`:

```go
package ui

import (
	"image"
	"testing"
)

func TestKeycapCapRect_RestPressedRaised(t *testing.T) {
	r := image.Rect(10, 20, 60, 50) // 50x30
	wall := genGeomKeycapWallDepth   // 4
	// Rest: cap occupies the top (height - wall), wall band below.
	rest := keycapCapRect(r, 0, false)
	if rest.Min != image.Pt(10, 20) {
		t.Fatalf("rest cap min = %v, want (10,20)", rest.Min)
	}
	if rest.Dy() != r.Dy()-wall {
		t.Fatalf("rest cap height = %d, want %d", rest.Dy(), r.Dy()-wall)
	}
	if rest.Dx() != r.Dx() {
		t.Fatalf("rest cap width = %d, want %d", rest.Dx(), r.Dx())
	}
	// Fully pressed (travel == wall): cap bottom meets rect bottom (bottom-out).
	pressed := keycapCapRect(r, wall, false)
	if pressed.Max.Y != r.Max.Y {
		t.Fatalf("pressed cap max.Y = %d, want %d (bottom-out)", pressed.Max.Y, r.Max.Y)
	}
	if pressed.Dy() != rest.Dy() {
		t.Fatalf("cap height must be travel-invariant: %d vs %d", pressed.Dy(), rest.Dy())
	}
	// Active raised: cap sits keycap-active-raise px above rest.
	raised := keycapCapRect(r, 0, true)
	if raised.Min.Y != r.Min.Y-genGeomKeycapActiveRaise {
		t.Fatalf("raised cap min.Y = %d, want %d", raised.Min.Y, r.Min.Y-genGeomKeycapActiveRaise)
	}
}
```

- [ ] **Step 2: Run → FAIL** (`keycapCapRect` undefined).

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestKeycapCapRect ./internal/ui/`
Expected: FAIL / build error: `undefined: keycapCapRect`.

- [ ] **Step 3: Implement the geometry + draw helpers** in `button_cushion.go` (append near the other cushion helpers):

```go
// keycapCapRect returns the rect the cap face occupies inside the button
// rect r. The cap height is constant (r.Dy - wall) so its sprite cache is
// invariant across press travel — only its Y position moves. travelPx ∈
// [0, wall] descends the cap toward bottom-out; raised lifts a latched/
// active cap by keycap-active-raise px above rest.
func keycapCapRect(r image.Rectangle, travelPx int, raised bool) image.Rectangle {
	wall := genGeomKeycapWallDepth
	capH := r.Dy() - wall
	if capH < 1 {
		capH = r.Dy() // too short for a wall; degrade to a flat cap
		wall = 0
	}
	top := r.Min.Y + travelPx
	if raised {
		top -= genGeomKeycapActiveRaise
	}
	return image.Rect(r.Min.X, top, r.Max.X, top+capH)
}

// drawKeycapShell paints the dark socket/side-wall that the cap sits in. The
// cap is blitted on top; whatever shell shows below (rest) or above+below
// (pressed) reads as the extruded wall / sunken socket. shellCol is derived
// from the cap fill (no literal) so it stays in-family on any button color.
func drawKeycapShell(dst *ebiten.Image, r image.Rectangle, rad int, fill color.Color) {
	if r.Empty() || genGeomKeycapWallDepth == 0 {
		return
	}
	drawRoundedRect(dst, r, adjustColor(fill, -45), rad, true)
}

// drawKeycapContactShadow paints a soft 1-px contact shadow just under the
// resting key so it reads as raised above the panel. Skipped when bottomed-out.
func drawKeycapContactShadow(dst *ebiten.Image, capRect image.Rectangle, rad int) {
	if capRect.Empty() {
		return
	}
	sh := image.Rect(capRect.Min.X, capRect.Max.Y, capRect.Max.X, capRect.Max.Y+1)
	drawRoundedRect(dst, sh, WithAlphaFromColor(color.Black, 64), rad, true)
}

// drawKeycapEngageFlash paints a one-shot cyan brightness flash over a key
// that just latched on. progress (0..1) decays each frame to silence.
func drawKeycapEngageFlash(dst *ebiten.Image, capRect image.Rectangle, rad int, progress float64) {
	if progress <= 0 || capRect.Empty() {
		return
	}
	if progress > 1 {
		progress = 1
	}
	a := uint8(float64(genAlphaStrong) * progress)
	drawRoundedRect(dst, capRect, WithAlpha(TokenAccentBright(), a), rad, true)
}
```

- [ ] **Step 4: Run → PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestKeycapCapRect ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Checkpoint.** `go vet -tags test ./internal/ui/` clean. Working tree dirty.

---

## Task 3: Button press-depth travel model (replaces press-scale)

**Files:**
- Modify: `src/go/internal/ui/uigrid.go` (`Button` struct ~80; `Draw` ~187; `HandleInputResult` ~355–377; `PressScale`/`AdvancePressAnim` ~386–406)
- Modify: `src/go/internal/ui/button_anim_test.go` (`TestButtonPressSpring`)
- Modify: `src/go/internal/ui/button_keycap_test.go`

- [ ] **Step 1: Write the failing travel-model test.** Add to `button_keycap_test.go`:

```go
func TestButtonPressDepthTravel(t *testing.T) {
	b := NewButton(image.Rect(0, 0, 60, 30), "Play", nil)
	// Press edge targets full depth (1.0).
	b.HandleInputResult(10, 10, true)
	if b.pressTarget != 1 {
		t.Fatalf("press target = %v, want 1", b.pressTarget)
	}
	// Several ticks ease the depth up toward 1 (analogue, not instant).
	for i := 0; i < 12; i++ {
		b.AdvancePressAnim()
	}
	if b.pressDepth < 0.9 {
		t.Fatalf("held depth eased to %v, want ~1", b.pressDepth)
	}
	if px := b.pressTravelPx(); px < genGeomKeycapWallDepth-1 {
		t.Fatalf("held travel = %dpx, want ~%d", px, genGeomKeycapWallDepth)
	}
	// Release: overshoot UP (negative depth) then settle to 0.
	b.HandleInputResult(10, 10, false)
	if b.pressTarget != 0 {
		t.Fatalf("release target = %v, want 0", b.pressTarget)
	}
	if b.pressDepth >= 0 {
		t.Fatalf("release should kick depth negative (overshoot up), got %v", b.pressDepth)
	}
	for i := 0; i < 60; i++ {
		b.AdvancePressAnim()
	}
	if b.pressDepth != 0 {
		t.Fatalf("settled depth = %v, want exactly 0 (silence)", b.pressDepth)
	}
}
```

- [ ] **Step 2: Run → FAIL** (`pressTarget`/`pressDepth`/`pressTravelPx` undefined).

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestButtonPressDepthTravel ./internal/ui/`
Expected: FAIL: `b.pressTarget undefined`.

- [ ] **Step 3: Replace the scale fields with depth fields** in the `Button` struct (`uigrid.go`). Remove the `pressAnim float64` field + its doc comment and add:

```go
	// pressDepth is the springy press travel: 0 = rest, 1 = fully bottomed
	// out (cap descended by genGeomKeycapWallDepth px), negative = raised
	// (release overshoot). Eased toward pressTarget each frame by
	// AdvancePressAnim via genAnimButtonPressDecay. Stored on the struct so
	// the animation allocates nothing per frame and settles exactly to 0.
	pressDepth  float64
	pressTarget float64
	// engageAnim is a one-shot 1→0 brightness flash fired when the button
	// latches ON (off→on via SetToggled). Decays via genAnimButtonEngageFlash.
	engageAnim float64
```

- [ ] **Step 4: Rewrite the press/release edges** in `HandleInputResult`. Replace the press-edge line `b.pressAnim = genGeomButtonPressScale` with:

```go
				// Press edge: ease the cap down toward full travel.
				b.pressTarget = 1
```

Replace the release-edge block (`if b.pressed { b.pressAnim = genGeomButtonReleaseOvershoot }`) with:

```go
	if b.pressed {
		// Release edge: settle target to rest, kick the cap UP for the
		// analogue overshoot before it springs back down to 0.
		b.pressTarget = 0
		if b.pressDepth > 0 {
			b.pressDepth = -genGeomKeycapReleaseOvershootFrac
		}
	}
```

- [ ] **Step 5: Rewrite `PressScale`/`AdvancePressAnim` and add `pressTravelPx`.** Replace both methods with:

```go
// AdvancePressAnim eases the press depth toward its target (analogue settle,
// both directions) and decays the one-shot engage flash. Call once per frame
// from Draw. Allocation-free; a no-op once depth==target and flash==0.
func (b *Button) AdvancePressAnim() {
	if b.pressDepth != b.pressTarget {
		r := genAnimButtonPressDecay.Rate
		b.pressDepth = b.pressTarget + (b.pressDepth-b.pressTarget)*r
		if math.Abs(b.pressDepth-b.pressTarget) < genAnimButtonPressDecay.Threshold {
			b.pressDepth = b.pressTarget
		}
	}
	if b.engageAnim > 0 {
		if v, alive := DecayStep(b.engageAnim, genAnimButtonEngageFlash); alive {
			b.engageAnim = v
		} else {
			b.engageAnim = 0
		}
	}
}

// pressTravelPx is the current downward cap travel in px (clamped ≥ 0; the
// negative overshoot region renders as a raise, handled by keycapCapRect).
func (b *Button) pressTravelPx() int {
	d := b.pressDepth
	if d < 0 {
		d = 0
	}
	return int(d*float64(genGeomKeycapWallDepth) + 0.5)
}
```

- [ ] **Step 6: Rewrite `Button.Draw`'s framing** (`uigrid.go` ~187–212). Replace the block from `sr := scaleRectAboutCenter(...)` through the `if b.pressed { drawButtonInnerShadow(...) }` with:

```go
	rad := RadiusSM
	raised := b.toggled // latched/active keys sit raised + lit
	cap := keycapCapRect(b.r, b.pressTravelPx(), raised)
	// Static volume: dark socket/wall behind the cap, contact shadow when
	// the key is up. Both derive from the cap fill — no literals.
	fill := b.fillColor()
	drawKeycapShell(dst, b.r, rad, fill)
	if b.pressDepth <= 0 {
		drawKeycapContactShadow(dst, cap, rad)
	}
	// Cap chrome (cached): spec path or legacy style, drawn INTO the cap rect.
	sr := cap
	switch {
	case b.hasSpecID:
		Render(dst, sr, Spec(b.SpecID), ComponentState{Pressed: b.pressed, Hovered: b.hovered, Toggled: b.toggled})
	case b.Style != nil:
		b.Style.Draw(dst, sr, b.pressed, b.hovered)
	}
	// One-shot engage flash over the freshly-latched cap.
	drawKeycapEngageFlash(dst, cap, rad, b.engageAnim)
```

Then change the text/icon centering below to use `cap` instead of `b.r` (replace `b.r.Min.X`/`b.r.Min.Y`/`b.r.Dx()`/`b.r.Dy()` in the text and icon blocks with `cap.Min.X`/`cap.Min.Y`/`cap.Dx()`/`cap.Dy()`).

Add a `fillColor()` helper (used above) near `Button` methods:

```go
// fillColor resolves the cap's base fill for the shell/wall derivation:
// the spec's background when spec-driven, else a neutral surface fallback.
func (b *Button) fillColor() color.Color {
	if b.hasSpecID {
		if c := Spec(b.SpecID).BackgroundColor; c != nil {
			return c
		}
	}
	return TokenSurface2()
}
```

> Note for the implementer: confirm `ComponentSpec` exposes `BackgroundColor` (it does per `design_components.gen.go`) and that `ComponentState` has a `Toggled` field; if not, add `Toggled bool` to `ComponentState` in `render.go` and have `Render` pick the lit-cyan fill when set (Task 4 wires the lit fill — for now `Toggled` may be ignored by Render).

- [ ] **Step 7: Retarget `TestButtonPressSpring`** in `button_anim_test.go` to the depth model: replace assertions on `pressAnim == genGeomButtonPressScale` / `== genGeomButtonReleaseOvershoot` with the `pressTarget`/`pressDepth` expectations from Step 1 (press→target 1, release→negative then settle to 0). Delete references to `PressScale()` and `scaleRectAboutCenter` (the latter is now unused — remove it from `button_cushion.go` and any test that referenced it).

- [ ] **Step 8: Run the button anim + keycap tests → PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestButtonPressDepthTravel|TestButtonPressSpring|TestKeycap' ./internal/ui/`
Expected: PASS.

- [ ] **Step 9: Checkpoint.** `go vet -tags test ./internal/ui/` clean (fix any remaining `pressAnim`/`PressScale`/`scaleRectAboutCenter` references the compiler flags). Working tree dirty.

---

## Task 4: Keycap pills + lit-cyan-raised active + engage flash (`drawPillTabAt` chokepoint)

**Why this replaces the original "active state in Render" task:** investigation of the active-state architecture showed (a) all audio-tab pills — tab pills, freeze, AG, spectrum/levels/chain mode pills — are custom-drawn by `drawPillTabAt(dst, btn, active)`, which **bypasses `Button.Draw`**, so Task 3's keycap framing never reaches them; and (b) every `Button.Draw`-path toggle is *semantic-colored* (mute=darkened-instrument, solo=brightened, FX=instrument hue, record=red, follow=coral) — none is a generic toggle that should turn cyan. So the lit-cyan+raised active state belongs in `drawPillTabAt`, not in `Render`. Semantic-colored toggles keep their own colors (and already gained keycap *volume* via Task 3, since their fill flows through `Button.Draw`). Synth/FX *enable switches* (`drawFXTogglePill`/`drawFXTogglePillCol`) are a switch idiom, intentionally out of scope.

**Files:**
- Modify: `src/go/internal/ui/uigrid.go` (add a `wasActive bool` field to `Button`)
- Modify: `src/go/internal/ui/audio_sticky_bar.go` (`drawPillTabAt` ~line 311)
- Test: `src/go/internal/ui/audio_sticky_bar_pill_keycap_test.go` (new)

- [ ] **Step 1: Add the `wasActive` field to `Button`** (uigrid.go), near `engageAnim`:

```go
	// wasActive tracks whether this button was drawn in its active/selected
	// state last frame, so a pill renderer can fire the one-shot engage flash
	// exactly on an off→on transition. Set by drawPillTabAt.
	wasActive bool
```

- [ ] **Step 2: Write the failing pill state-machine test.** Create `audio_sticky_bar_pill_keycap_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestPillKeycapEngageAndTravel(t *testing.T) {
	b := NewButton("EQ", InstButtonStyle, nil)
	b.SetRect(image.Rect(0, 0, 40, 22))
	dst := ebiten.NewImage(40, 22)

	// Inactive: no engage flash.
	drawPillTabAt(dst, b, false)
	if b.engageAnim != 0 {
		t.Fatalf("inactive pill engageAnim = %v, want 0", b.engageAnim)
	}
	// off→on: fires the one-shot engage flash.
	drawPillTabAt(dst, b, true)
	if b.engageAnim <= 0 {
		t.Fatalf("off→on did not fire engage flash (got %v)", b.engageAnim)
	}
	// Staying active does NOT re-fire — it decays each frame.
	prev := b.engageAnim
	drawPillTabAt(dst, b, true)
	if b.engageAnim >= prev {
		t.Fatalf("engage flash re-fired/stuck while staying active (%v → %v)", prev, b.engageAnim)
	}
	// Decays to silence.
	for i := 0; i < 80; i++ {
		drawPillTabAt(dst, b, true)
	}
	if b.engageAnim != 0 {
		t.Fatalf("engage flash did not settle to 0 (got %v)", b.engageAnim)
	}

	// Press travels the pill cap down (drawPillTabAt ticks AdvancePressAnim).
	b.HandleInputResult(20, 11, true) // press at center
	for i := 0; i < 12; i++ {
		drawPillTabAt(dst, b, true)
	}
	if px := b.pressTravelPx(); px < genGeomKeycapWallDepth-1 {
		t.Fatalf("held pill travel = %dpx, want ~%d", px, genGeomKeycapWallDepth)
	}
}
```

Run → FAIL (compile error: `wasActive` referenced indirectly, or behavior not yet implemented):
`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestPillKeycapEngageAndTravel ./internal/ui/`

- [ ] **Step 3: Rewrite `drawPillTabAt`** (audio_sticky_bar.go) to render a voluminous keycap pill with a lit-cyan-raised active state, an engage flash, and press travel. Replace the body (keeping the `nil`/empty guards and the icon-vs-text branch) with:

```go
func drawPillTabAt(dst *ebiten.Image, btn *Button, active bool) {
	if btn == nil {
		return
	}
	r := btn.Rect()
	if r.Empty() {
		return
	}
	// Pills bypass Button.Draw, so tick the press/engage springs here.
	btn.AdvancePressAnim()
	if active && !btn.wasActive {
		btn.engageAnim = 1 // one-shot flash on selection
	}
	btn.wasActive = active

	pillRadius := RadiusMD / 2 // 4px corners
	capR := keycapCapRect(r, btn.pressTravelPx(), active)

	// Active = lit cyan face (raised); inactive = neutral surface cap.
	capFill := colSurface2
	borderCol := colButtonBorder
	var textCol color.Color = colTextSecondary
	if active {
		capFill = colAccent
		borderCol = colAccent
		textCol = colOnAccentText // dark legend on the cyan face
	}
	drawKeycapShell(dst, r, pillRadius, capFill)
	if btn.pressDepth <= 0 {
		drawKeycapContactShadow(dst, capR, pillRadius)
	}
	drawRoundedRect(dst, capR, capFill, pillRadius, true)
	drawRoundedRect(dst, capR, borderCol, pillRadius, false)
	drawKeycapEngageFlash(dst, capR, pillRadius, btn.engageAnim)

	if btn.Icon != "" {
		iconCol := btn.IconColor
		if iconCol == nil {
			iconCol = textCol
		}
		DrawIcon(dst, IconID(btn.Icon), capR, iconCol)
		return
	}
	if btn.TextColor != nil {
		textCol = btn.TextColor
	}
	tw := TextWidth(btn.Text)
	th := TextHeight()
	tx := capR.Min.X + (capR.Dx()-tw)/2
	ty := capR.Min.Y + (capR.Dy()-th)/2
	DrawTextColorAt(dst, btn.Text, tx, ty, textCol)
}
```

`colOnAccentText` is the dark text color used on an accent fill — the same color `button-primary` uses for its label (DESIGN.md `textColor: {colors.background}`). Find the exact existing token (likely `colBackground` / `TokenBackground()` / `genColorBackground`) and use it; do NOT introduce a new color literal. If no single token exists, reuse whatever `button-primary`'s spec resolves its text color to.

- [ ] **Step 4: Run → PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestPillKeycapEngageAndTravel ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Run the existing sticky-bar tests to catch regressions.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestAudioStickyBar|TestPill|TestTab' ./internal/ui/`
Expected: PASS. If an existing test asserted the OLD active look (surface2 fill + cyan *border* + cyan *text*), retarget it to the new contract (cyan *fill* + dark text, raised cap) — but verify the retarget reflects the real new behavior, don't weaken it.

- [ ] **Step 6: Checkpoint.** `go vet -tags test ./internal/ui/` clean; `GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/` succeeds. Working tree dirty.

> Note on `Button.Draw`-path active: no separate change is needed. Mute/Solo/FX/record/follow keep their existing semantic active fills (via their style-swap / per-frame icon-color paths) and already render with keycap volume from Task 3. `SetToggled`/`toggled` remain dormant as before — do NOT wire generic cyan into `Render`.

---

## Task 5: Hover — restrained rim (drop the bloom)

**Files:**
- Modify: `src/go/internal/ui/button_cushion.go` (`drawButtonHoverGlow`)
- Modify: `src/go/internal/ui/hover_glow_overlay_test.go`

- [ ] **Step 1: Retarget the hover test to the rim.** In `hover_glow_overlay_test.go`, `TestHoverGlowOverlayDrawsRingOnButtonHover` currently expects a bold multi-layer bloom. Change it to assert (a) a hover ring/top-face is drawn within `genGeomButtonHoverGlowSpread+1` of the cap edge while hovered, and (b) its peak alpha is **lower** than the old bloom (a hairline, not a neon lift). Concretely, lower the expected-alpha threshold and keep the `brightRingNear(rings(), btnRect, genGeomButtonHoverGlowSpread+1)` locator. (Keep `TestHoverGlowOverlayCoversTextInput` and `...RowVolumesAreIndividual` — only the alpha magnitude changes.)

- [ ] **Step 2: Run → FAIL** (old code still paints the bold bloom / bevel).

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestHoverGlowOverlayDrawsRing ./internal/ui/`
Expected: FAIL on the new (lower) alpha / single-layer expectation.

- [ ] **Step 3: Rewrite `drawButtonHoverGlow`** to a restrained rim (replace the whole body):

```go
// drawButtonHoverGlow paints the restrained desktop hover affordance around a
// keycap: a subtle top-face warm highlight + a hairline neutral rim, each
// scaled by the fade progress p (0..1). Deliberately NOT a neon bloom — hover
// is a quiet "this is reachable" cue (consistent with the neutral chrome pass);
// cyan is reserved for the active (lit) state. Desktop-only (caller-enforced).
func drawButtonHoverGlow(dst *ebiten.Image, r image.Rectangle, rad int, p float64) {
	if p <= 0 || r.Empty() {
		return
	}
	if p > 1 {
		p = 1
	}
	sa := func(base uint8) uint8 { return uint8(float64(base) * p) }
	// Hairline neutral rim hugging the cap edge.
	drawRoundedRect(dst, r, WithAlpha(colTextPrimary, sa(genAlphaSubtle)), rad, false)
	// Top-face warm highlight (specular catch), brightening the cap top.
	if r.Dx() > 6 && r.Dy() > 6 {
		top := image.Rect(r.Min.X+2, r.Min.Y+1, r.Max.X-2, r.Min.Y+1+genGeomButtonInnerShadowPx)
		drawRect(dst, top, WithAlpha(colTextPrimary, sa(genAlphaMedium)), true)
	}
}
```

- [ ] **Step 4: Run → PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestHoverGlowOverlay ./internal/ui/`
Expected: PASS (all three hover-overlay tests).

- [ ] **Step 5: Checkpoint.** `go vet -tags test ./internal/ui/` clean. Working tree dirty.

---

## Task 6: Knurled volumetric knob (static cached cap + dynamic arc/pointer)

**Files:**
- Modify: `src/go/internal/ui/knob.go` (`Draw` ~436)
- Create: `src/go/internal/ui/knob_cap_test.go`

- [ ] **Step 1: Write the failing cache test.** Create `knob_cap_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestKnurledCapSpriteIsCachedByDiameter(t *testing.T) {
	knurledCapCache = map[knurledCapKey]*ebiten.Image{} // reset
	a := knurledCapSprite(48, true)
	b := knurledCapSprite(48, true)
	if a != b {
		t.Fatalf("same (diameter,enabled) must return the same cached sprite")
	}
	c := knurledCapSprite(40, true)
	if c == a {
		t.Fatalf("different diameter must produce a different sprite")
	}
	if got := len(knurledCapCache); got != 2 {
		t.Fatalf("cache size = %d, want 2 (48,40)", got)
	}
}
```

- [ ] **Step 2: Run → FAIL** (`knurledCapSprite`/`knurledCapKey`/`knurledCapCache` undefined).

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestKnurledCap ./internal/ui/`
Expected: FAIL (undefined).

- [ ] **Step 3: Add the cached knurled cap** in `knob.go` (above `Draw`):

```go
// knurledCapKey identifies a cached static knob-cap sprite. The cap (dome +
// grip ridges + side wall + contact shadow) is travel/value-invariant, so it
// is rasterized once per (diameter, enabled) and blitted each frame — only the
// value arc + pointer are drawn live on top. This keeps per-frame knob cost a
// single blit + the existing two drawArc calls, critical on the synth tab's
// many small knobs under the WASM single audio thread.
type knurledCapKey struct {
	d       int
	enabled bool
}

var knurledCapCache = map[knurledCapKey]*ebiten.Image{}

const knurledRidges = 18

// knurledCapSprite returns the cached static cap sprite of the given diameter.
func knurledCapSprite(diameter int, enabled bool) *ebiten.Image {
	if diameter < 2 {
		diameter = 2
	}
	k := knurledCapKey{d: diameter, enabled: enabled}
	if spr, ok := knurledCapCache[k]; ok {
		return spr
	}
	spr := ebiten.NewImage(diameter, diameter)
	cx, cy := float64(diameter)/2, float64(diameter)/2
	rad := float32(diameter) / 2
	wall := TokenSurface1()
	body := TokenSurface2()
	dome := TokenSurface3()
	if !enabled {
		body = adjustColor(body, -15)
		dome = adjustColor(dome, -15)
	}
	// Side wall + contact shadow (offset down 1px under the body).
	vector.DrawFilledCircle(spr, float32(cx), float32(cy)+1, rad, WithAlphaFromColor(color.Black, 64), true)
	vector.DrawFilledCircle(spr, float32(cx), float32(cy), rad, wall, true)
	// Grip ridges around the rim (alternating light/dark wedges via short
	// radial strokes — the "knurl").
	for i := 0; i < knurledRidges; i++ {
		a := float64(i) / float64(knurledRidges) * 2 * math.Pi
		col := dome
		if i%2 == 0 {
			col = adjustColor(dome, 18)
		}
		ix := cx + math.Cos(a)*float64(rad)*0.74
		iy := cy + math.Sin(a)*float64(rad)*0.74
		ox := cx + math.Cos(a)*float64(rad)*0.96
		oy := cy + math.Sin(a)*float64(rad)*0.96
		vector.StrokeLine(spr, float32(ix), float32(iy), float32(ox), float32(oy), 2, col, true)
	}
	// Raised center dome with a top specular highlight (light from above).
	vector.DrawFilledCircle(spr, float32(cx), float32(cy), rad*0.72, body, true)
	vector.DrawFilledCircle(spr, float32(cx), float32(cy)-rad*0.12, rad*0.5, adjustColor(body, 22), true)
	vector.DrawFilledCircle(spr, float32(cx), float32(cy), rad*0.5, dome, true)
	knurledCapCache[k] = spr
	return spr
}
```

(Ensure `knob.go` imports `image/color`; `vector` and `math` are already imported.)

- [ ] **Step 4: Blit the cached cap in `Knob.Draw`** — after the value-arc/pointer setup, replace the `Centre disk` block (the final `vector.DrawFilledCircle(..., trackCol, ...)`) with a blit of the cap *behind* the indicator. Concretely, near the top of `Draw` (after `cx, cy, radius` resolved, before drawing the track ring), blit the cap so arc + pointer render on top:

```go
	// Static volumetric cap (cached), blitted centered under the live arc.
	d := int(radius * 2)
	if spr := knurledCapSprite(d, !k.Disabled()); spr != nil {
		var op ebiten.DrawImageOptions
		op.GeoM.Translate(cx-float64(d)/2, cy-float64(d)/2)
		dst.DrawImage(spr, &op)
	}
```

Then delete the old standalone `Centre disk` filled-circle (the cap provides it). Keep the background ring, value arc, detents, and indicator line drawing on top.

> Implementer note: confirm the knob exposes a `Disabled()` predicate; if not, pass `true` (always enabled) for now and add disabled styling as a follow-up. The cache key already separates the two.

- [ ] **Step 5: Run knob tests → PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestKnurledCap|TestKnob|TestDrawArc' ./internal/ui/`
Expected: PASS (cache test + existing knob/draw-pool tests still green).

- [ ] **Step 6: Checkpoint.** `go vet -tags test ./internal/ui/` clean. Working tree dirty.

---

## Task 7: Sculpted-cap slider thumb

**Files:**
- Modify: `src/go/internal/ui/slider_neon.go` (`drawSliderThumb` ~57)
- Modify: `src/go/internal/ui/slider_neon_test.go`

- [ ] **Step 1: Write/adjust the thumb test.** In `slider_neon_test.go`, add an assertion that an active thumb draws a darker contact-shadow pixel just below its center and a lit body above (i.e. the thumb has top/bottom tonal asymmetry = volume), and that it no longer paints the wide cyan bloom (`r.Inset(-spread)` ring) at rest. (Model the pixel reads on existing slider tests in the file.)

- [ ] **Step 2: Run → FAIL.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSliderThumb ./internal/ui/`
Expected: FAIL (current thumb is a flat disc with a bloom).

- [ ] **Step 3: Give the thumb cap volume** — replace `drawSliderThumb` body:

```go
func drawSliderThumb(dst *ebiten.Image, center image.Point, diameter int, active, calm bool) {
	if diameter < 2 {
		return
	}
	rad := diameter / 2
	r := image.Rect(center.X-rad, center.Y-rad, center.X+rad, center.Y+rad)
	// Contact shadow (1px below) → reads as a raised cap.
	drawRoundedRect(dst, image.Rect(r.Min.X, r.Min.Y+1, r.Max.X, r.Max.Y+1),
		WithAlphaFromColor(color.Black, 64), rad, true)
	// Body + top specular highlight (light from above).
	body := genColorOnSurface
	drawRoundedRect(dst, r, body, rad, true)
	drawRoundedRect(dst, image.Rect(r.Min.X+1, r.Min.Y+1, r.Max.X-1, r.Min.Y+1+genGeomButtonInnerShadowPx),
		WithAlpha(colTextPrimary, genAlphaMedium), rad, true)
	// Edge rim: neutral at rest, lit cyan when active (matches the keycap on-state).
	rim := buttonGlowRingColor(active)
	ringAlpha := genAlphaMedium
	if active {
		ringAlpha = genAlphaStrong
	}
	if calm {
		ringAlpha /= 2
	}
	drawRoundedRect(dst, r, WithAlpha(rim, ringAlpha), rad, false)
}
```

- [ ] **Step 4: Run → PASS.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestSlider ./internal/ui/`
Expected: PASS.

- [ ] **Step 5: Prune now-dead symbols.** If `buttonGlowAlphaAnimated`/`drawButtonGlowRing`/`buttonGlowRest`-derived `restGlow` are no longer referenced anywhere (`grep -rn 'buttonGlowAlphaAnimated\|drawButtonGlowRing' src/go/internal/ui/ | grep -v _test`), delete them from `button_cushion.go` and remove the dead `genAnimButtonGlowRest`/`button-glow-rest` usage. If still referenced, leave them. Re-run vet.

- [ ] **Step 6: Checkpoint.** `go vet -tags test ./internal/ui/` clean. Working tree dirty.

---

## Task 8: Performance guards — alloc-rate + settle-to-silence (WASM-safety)

**Files:**
- Create: `src/go/internal/ui/button_perf_guard_test.go`

- [ ] **Step 1: Write the settle-to-silence + alloc-rate guards.** Create `button_perf_guard_test.go`:

```go
package ui

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// A button/knob at rest or steadily-on must do ZERO per-frame state work and
// allocate nothing — the single WASM audio thread cannot afford chrome churn.
func TestButtonSettlesToSilence(t *testing.T) {
	b := NewButton(image.Rect(0, 0, 60, 30), "Play", nil)
	b.HandleInputResult(10, 10, true)
	b.HandleInputResult(10, 10, false)
	settled := -1
	for i := 0; i < 200; i++ {
		b.AdvancePressAnim()
		if b.pressDepth == b.pressTarget && b.engageAnim == 0 {
			settled = i
			break
		}
	}
	if settled < 0 {
		t.Fatalf("button never settled within 200 frames")
	}
	// Once settled, AdvancePressAnim must be a pure no-op (idempotent state).
	before := b.pressDepth
	for i := 0; i < 100; i++ {
		b.AdvancePressAnim()
	}
	if b.pressDepth != before || b.engageAnim != 0 {
		t.Fatalf("settled button mutated state on tick (depth %v→%v)", before, b.pressDepth)
	}
}

func TestButtonAnimationAllocFree(t *testing.T) {
	b := NewButton(image.Rect(0, 0, 60, 30), "Rec", nil)
	b.SetToggled(true)
	b.HandleInputResult(10, 10, true)
	b.HandleInputResult(10, 10, false)
	allocs := testing.AllocsPerRun(500, func() {
		b.AdvancePressAnim()
	})
	if allocs != 0 {
		t.Fatalf("AdvancePressAnim allocates %v/run, want 0", allocs)
	}
}

func TestKnurledCapDrawDoesNotReallocPerFrame(t *testing.T) {
	knurledCapCache = map[knurledCapKey]*ebiten.Image{}
	k := NewKnob(image.Rect(0, 0, 48, 48), 0.5)
	dst := ebiten.NewImage(48, 48)
	k.Draw(dst) // warm the cap cache
	n := len(knurledCapCache)
	for i := 0; i < 30; i++ {
		k.Value = float64(i) / 30 // value changes every frame
		k.Draw(dst)
	}
	if len(knurledCapCache) != n {
		t.Fatalf("knob cap cache grew from %d to %d across frames (per-frame realloc)", n, len(knurledCapCache))
	}
}
```

(Adjust `NewKnob` signature to match `knob.go`; if it takes more args, supply them.)

- [ ] **Step 2: Run → expect PASS** (the model was built allocation-free + cache-stable in Tasks 3/6). If a guard fails, fix the implementation (not the test): a non-zero alloc means a closure/slice escaped a per-frame path; cache growth means the cap key leaked a per-frame value.

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestButtonSettles|TestButtonAnimationAllocFree|TestKnurledCapDraw' ./internal/ui/`
Expected: PASS.

- [ ] **Step 3: Checkpoint.** Working tree dirty.

---

## Task 9: DESIGN.md Elevation & Depth prose + lint snapshot

**Files:**
- Modify: `DESIGN.md` (Elevation & Depth section ~1928–1997)
- Maybe modify: `src/go/internal/ui/testdata/design_md_lint.expected.json`

- [ ] **Step 1: Rewrite the "Cushioned elevation" prose** (~1947–1997) to describe the new model: keycap shell/cap/wall extrusion, press-depth travel & bottom-out, lit-&-raised active state with the one-shot engage flash (replacing the toggle-pulse), the restrained hover rim (no bloom), and the knurled cached knob cap. State the WASM rule explicitly: *static volume bakes into sprite caches; motion is blit-offset + alpha overlay; controls settle to zero per-frame work.* Update the token references (`keycap-wall-depth`, `keycap-active-raise`, `keycap-release-overshoot-frac`, `button-engage-flash`).

- [ ] **Step 2: Run the lint snapshot test.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestDesignMDLintSnapshot ./internal/ui/`
Expected: PASS, or a diff in the warning set. If the prose edit changes the lint warnings, update `testdata/design_md_lint.expected.json` to match (the test prints the new expected JSON). Skips cleanly if `npx` is unavailable — that's fine.

- [ ] **Step 3: Checkpoint.** Working tree dirty.

---

## Task 10: Full retarget sweep + cross-platform verification

**Files:**
- Modify: any remaining `internal/ui` tests the compiler/suite flags
- Verify: browser/WASM perf unaffected

- [ ] **Step 1: Build the whole UI package and the WASM target.**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/`
Run: `cd src/go && GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./internal/ui/`
Expected: both succeed. Fix any leftover references to removed symbols (`pressAnim`, `PressScale`, `scaleRectAboutCenter`, the deleted glow-ring call).

- [ ] **Step 2: Run the full design-system + button/knob/slider/hover test set.**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'TestDesignMDDrift|TestTokenDiscipline|TestDesignMDLintSnapshot|TestButton|TestKeycap|TestSetToggled|TestHoverGlowOverlay|TestKnurledCap|TestKnob|TestSlider' \
  ./internal/ui/
```
Expected: PASS. Investigate any failure with a narrow `-run` (the full suite has pre-existing unrelated failures — confirm a failure is yours by reverting your edit to that test mentally, not via git).

- [ ] **Step 3: Token-discipline guard.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestTokenDiscipline ./internal/ui/`
Expected: PASS. If it fails, you introduced a `color.RGBA{}`/`color.NRGBA{}` literal in a budget-0 file — replace it with `adjustColor`/`WithAlpha`/`Token*`.

- [ ] **Step 4: WASM/browser perf sanity (the user's binding requirement).** Build WASM and run a perf-sensitive browser suite that toggles buttons + drags under playback, confirming no draw/audio regression:

Run:
```bash
make wasm
GO=$(pwd)/.tools/go/bin/go node src/js/webaudio_perf_e2e.browser.test.js
```
Expected: FPS / draw-ms / audio queue-latency within the existing thresholds (no spike vs a pre-change baseline). If `drawMax` or `qlat` regresses, the cause is per-frame chrome work — re-check that the press/engage/hover paths blit cached sprites and never re-rasterize (revisit Tasks 3/5/6). This is the same metric that caught the chain-tab starvation.

- [ ] **Step 5: Visual spot-check (optional but recommended).** Capture screenshots to eyeball the keycaps/knobs across tabs:

Run: `make screenshots-all SCENES=$(./tmp/beatmo_screenshot -list-scenes | grep '^crop_' | paste -sd ,)` (per CLAUDE.md) and inspect `screenshots/` for transport, synth, EQ, sidebar.

- [ ] **Step 6: Final checkpoint.** All targeted tests green; WASM builds; perf within thresholds. Leave everything in the working tree for the user to review and commit. Summarize what changed and the verification evidence.

---

## Self-review notes (coverage map)

- Spec "Sculpted Keycap" → Tasks 2, 3 (shell/cap/wall + travel).
- Spec "Knurled Grip knob" → Task 6.
- Spec "Lit & Raised + engage flash, remove toggle-pulse" → Task 4.
- Spec "Cushioned Analogue motion" → Task 3 (eased depth both ways + overshoot).
- Spec "restrained hover rim, drop bloom" → Task 5.
- Spec "slider thumbs get modest cap volume" → Task 7.
- Spec "WASM-safe: static cache + blit-offset + settle-to-silence" → Tasks 3/6 design + Task 8 guards + Task 10 Step 4.
- Spec "token-driven + tests in same change" → Task 1 + Task 9 + per-task test retargets.
- Spec "menus/list rows stay flat" → untouched by design (no task modifies `menu_chrome.go`).

**Deferred (fast-follow, not in this plan):** knob *pointer*-spring settle on discrete nudge — omitted to avoid drag jitter and extra per-frame work; the cap volume is the knob win. Disabled-knob styling beyond the dim cap variant. Both are noted so they aren't mistaken for gaps.
