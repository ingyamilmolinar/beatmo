# Audio-Panel Tab Decoupling Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Strip the desktop audio-panel tab-switcher row down to channel · tabs · `?` · expander, and move every per-tab control (freeze, freq-scale, slope, pre, reset-hold, clear-clips, K-20) into self-contained per-tab components that each own their own buttons; delete the dead X close button.

**Architecture:** A new `tabControls` interface (`audio_tab_controls.go`) with three concrete components — `waveControls`, `spectrumControls`, `levelsControls`. Each owns its button widgets, toggle state, layout, draw, and hit areas. The dispatcher (`EQPanelZone`) reserves a control-header strip directly below the tab-switcher row, hands it to the active tab's component, and draws that tab's content into the body region below the header (reading toggle state from the component). `AudioStickyBar` slims to switcher-row-only. The shared `ChainPanelZone`/Synth-header tabs already follow this ownership model and are untouched.

**Tech Stack:** Go 1.23, Ebiten v2, ebitenstub test harness (`-tags test -modfile=go.test.mod`).

---

## Reference: current code (read before starting)

- `internal/ui/audio_sticky_bar.go` — the shared bar. Fields (lines 30–72), constructor `NewAudioStickyBar(parentZIndex int, onChannel, onFreeze, onClose func(), onTab func(PanelTab))` (line 79), `Layout` (188–355), `rebuildHitAreas` (357–404), `Draw` (409–470), accessors (472–566). `stickyBarHeight()`/`stickyBarH=26` (13–23).
- `internal/ui/eq_panel_zone.go` — the dispatcher. `EQCallbacks` (17–120, `OnFreezeToggle` at 62, `OnClose` at 72), `EQPanelZone` fields (125–249), `NewEQPanelZone` (256–276), `initButtons` (278–395), `Layout` (405–423), `HitAreas` (499–521), `Draw` (523–694, content switch 538–650, bar freeze-sync 652–665, sticky draw 681), `layoutButtons` (945–979), `contentRect` (757–759), `rebuildHitAreas` (1128+, const `zIdx=130`, catch-all 1153, sticky-bar hits 1198–1200, mobile scrub 1206–1217).
- `internal/ui/audio_panel_cursor.go` — `updateAudioPanelCursor` (24–48) uses `z.contentRect()`.
- Density tokens (read at `Profile().DensityValues()`): `AudioPillH`, `AudioPillIconW`, `AudioPillGap`, `AudioPillWideW`, `AudioPillNarrowW`, `AudioPillPadX`, `AudioPillSlopeMinW`. Spacing: `SpaceSM`.
- Helpers reused: `NewButton(text, style, onClick)`, `InstButtonStyle`, `buttonHitAdapter{btn}`, `drawPillTabAt(dst, btn, active)`, `formatSlopeLabel(v float64) string`, `SetSpectrumSlope(v float64)`, `audio.ResetClipsWindow()`, `colTextSecondary`, `colAccent`, `IconClose`, `IconChevronDown`.

**Test command (fast path), used throughout:**
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run <TestName> -v
```

---

## Phase 0 — Seam (interface + dispatcher geometry, zero behavior change)

Introduce the interface, constants, and the dispatcher's header/body geometry. With no components built yet, `activeTabControls()` returns nil, header height is 0, and `bodyRect()==contentRect()`, so nothing visibly changes.

### Task 0.1: Create `audio_tab_controls.go` with the interface + shared helpers

**Files:**
- Create: `src/go/internal/ui/audio_tab_controls.go`
- Test: `src/go/internal/ui/audio_tab_controls_test.go`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/audio_tab_controls_test.go`:

```go
package ui

import "testing"

// controlHeaderHeight mirrors stickyBarHeight: 26 desktop, 36 mobile.
func TestControlHeaderHeight(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	r2 := SetDensityForTest(DensityComfortable)
	defer r2()
	if got := controlHeaderHeight(); got != audioControlHeaderH {
		t.Fatalf("desktop control header height = %d, want %d", got, audioControlHeaderH)
	}
}

// newFreezePill builds a "||" pill that flips to ">" / colAccent when the
// freeze callback reports frozen, and back to "||" / colTextSecondary.
func TestNewFreezePillTogglesVisual(t *testing.T) {
	frozen := false
	b := newFreezePill(func() bool { frozen = !frozen; return frozen })
	if b.Text != "||" {
		t.Fatalf("initial freeze text = %q, want ||", b.Text)
	}
	b.OnClick()
	if b.Text != ">" {
		t.Fatalf("after first toggle text = %q, want >", b.Text)
	}
	b.OnClick()
	if b.Text != "||" {
		t.Fatalf("after second toggle text = %q, want ||", b.Text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestControlHeaderHeight|TestNewFreezePillTogglesVisual' -v`
Expected: FAIL — `undefined: controlHeaderHeight`, `undefined: audioControlHeaderH`, `undefined: newFreezePill`.

- [ ] **Step 3: Write the implementation**

Create `src/go/internal/ui/audio_tab_controls.go`:

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// tabControls is the control header owned by a single audio-panel tab. Each
// tab's controls are an independent component — no tab shares a button widget
// or visibility state with another. The dispatcher (EQPanelZone) asks the
// active tab's component for its header height, lays it into the strip just
// below the shared tab-switcher row, draws it, and routes its hit areas. This
// is the per-tab ownership model that ChainPanelZone and the Synth header
// already follow.
type tabControls interface {
	// HeaderH returns the control-header height in pixels (0 = no header).
	HeaderH() int
	// Layout positions the buttons inside header (the strip reserved at the
	// top of the tab's content region, directly below the tab-switcher row).
	Layout(header image.Rectangle)
	// Draw renders the buttons.
	Draw(dst *ebiten.Image)
	// HitAreas returns the buttons' hit areas (z-indexed above the panel body).
	HitAreas() []HitArea
	// SyncFreeze updates the freeze button's glyph/color from the analyzer's
	// global frozen state. No-op for components without a freeze button.
	SyncFreeze(frozen bool)
}

// audioControlHeaderH is the desktop height of a per-tab control header — the
// row of tab-specific pills directly below the shared tab-switcher row. Mirrors
// stickyBarH so the two rows read as a stacked pair.
const audioControlHeaderH = 26

// controlHeaderHeight returns the per-tab control-header height (taller on
// mobile so the freeze pill meets the touch-min, mirroring stickyBarHeight).
func controlHeaderHeight() int {
	if Profile().IsMobile() {
		return 36
	}
	return audioControlHeaderH
}

// newFreezePill builds a freeze/resume toggle pill wired to onFreeze (which
// returns the new frozen state). Each tab owns its own instance — there is no
// shared freeze widget. The "||" (capturing) / ">" (frozen) glyphs are a
// DESIGN.md §5d permitted text-glyph exception.
func newFreezePill(onFreeze func() bool) *Button {
	b := NewButton("||", InstButtonStyle, nil)
	b.TextColor = colTextSecondary
	b.OnClick = func() {
		if onFreeze == nil {
			return
		}
		applyFreezeVisual(b, onFreeze())
	}
	return b
}

// applyFreezeVisual sets the freeze pill's glyph + color from a frozen bool.
func applyFreezeVisual(b *Button, frozen bool) {
	if b == nil {
		return
	}
	if frozen {
		b.Text = ">"
		b.TextColor = colAccent
	} else {
		b.Text = "||"
		b.TextColor = colTextSecondary
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestControlHeaderHeight|TestNewFreezePillTogglesVisual' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/ui/audio_tab_controls.go src/go/internal/ui/audio_tab_controls_test.go
git commit -m "feat(ui): tabControls interface + freeze-pill helper (audio-panel decoupling phase 0)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 0.2: Dispatcher header/body geometry + nil selector

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go` (add fields near line 133; add methods after `contentRect` ~759)
- Test: `src/go/internal/ui/audio_tab_controls_test.go`

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/audio_tab_controls_test.go`:

```go
import "image" // add to existing import block if not present

// With no components built, the active controls are nil on every tab, the
// header height is 0, and bodyRect equals contentRect (zero behavior change).
func TestPhase0NoControlHeader(t *testing.T) {
	z := NewEQPanelZone(EQCallbacks{})
	z.Layout(image.Rect(0, 400, 600, 600))
	for _, tab := range AllPanelTabs() {
		z.SetActiveTab(tab)
		z.Layout(image.Rect(0, 400, 600, 600))
		if z.controlHeaderH() != 0 {
			t.Fatalf("tab %v: controlHeaderH = %d, want 0", tab, z.controlHeaderH())
		}
		if z.bodyRect() != z.contentRect() {
			t.Fatalf("tab %v: bodyRect %v != contentRect %v", tab, z.bodyRect(), z.contentRect())
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestPhase0NoControlHeader -v`
Expected: FAIL — `z.controlHeaderH undefined`, `z.bodyRect undefined`.

- [ ] **Step 3: Add the geometry methods + a nil selector stub**

Phase 0 adds NO struct fields and NO concrete component types (those arrive in their phases). It adds only four methods so the dispatcher compiles and the geometry contract is locked. The selector is a stub returning nil; Phase 1 replaces its body and adds the `waveControls` field, Phase 2 adds `levelsControls`, Phase 3 adds `spectrumControls`.

Add these methods to `src/go/internal/ui/eq_panel_zone.go` immediately after `contentRect()` (after line 759):

```go
// activeTabControls returns the control component owning the active tab's
// chrome, or nil for tabs that own their controls elsewhere (EQ inline, Chain
// via ChainPanelZone, Synth/Sampler via DrumView header). Phase 0 stub — no
// components are built yet, so this always returns nil. Later phases replace
// the body with a per-tab switch.
func (z *EQPanelZone) activeTabControls() tabControls { return nil }

// controlHeaderH is the height the active tab's control header needs (0 none).
func (z *EQPanelZone) controlHeaderH() int {
	if c := z.activeTabControls(); c != nil {
		return c.HeaderH()
	}
	return 0
}

// headerRect is the control-header strip at the top of contentRect().
func (z *EQPanelZone) headerRect() image.Rectangle {
	cr := z.contentRect()
	return image.Rect(cr.Min.X, cr.Min.Y, cr.Max.X, cr.Min.Y+z.controlHeaderH())
}

// bodyRect is the drawable area below the control header — where the active
// tab's content (waveform / spectrum / meters) is rendered. Equals
// contentRect() when the active tab has no control header.
func (z *EQPanelZone) bodyRect() image.Rectangle {
	cr := z.contentRect()
	return image.Rect(cr.Min.X, cr.Min.Y+z.controlHeaderH(), cr.Max.X, cr.Max.Y)
}
```

Do not add struct fields or component types in this phase.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestPhase0NoControlHeader -v`
Expected: PASS.

- [ ] **Step 5: Point Wave/Spectrum/Levels content + cursor at bodyRect (still == contentRect)**

In `eq_panel_zone.go Draw`, change the content rects for the three migrated tabs from `z.contentRect()` to `z.bodyRect()`:
- TabWave case (line 540): `cr := z.bodyRect()`
- TabSpectrum case (line 555): `cr := z.bodyRect()`
- TabMeters case (lines 625 and 628): replace both `z.contentRect()` with `z.bodyRect()`

In `audio_panel_cursor.go` `updateAudioPanelCursor` (line 31): `cr := z.bodyRect()`.

In `eq_panel_zone.go rebuildHitAreas` mobile scrub (line 1209): `if cr := z.bodyRect(); !cr.Empty() {`.

(All are no-ops while header height is 0, but they lock the body-region contract in one place before any header exists.)

- [ ] **Step 6: Run the full ui Wave/Spectrum/Levels render tests to confirm no regression**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestPhase0NoControlHeader|AudioPanelRenderPixels' -v`
Expected: PASS (or the same pre-existing skips as before — diff against `git stash` baseline if unsure).

- [ ] **Step 7: Commit**

```bash
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/ui/audio_tab_controls.go src/go/internal/ui/audio_tab_controls_test.go src/go/internal/ui/eq_panel_zone.go src/go/internal/ui/audio_panel_cursor.go
git commit -m "feat(ui): dispatcher header/body geometry seam (audio-panel decoupling phase 0)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 1 — Wave controls (freeze)

`waveControls` owns its own freeze button; the dispatcher draws its header on TabWave and insets the body; the sticky bar suppresses its freeze on TabWave so freeze never renders twice.

### Task 1.1: Add the `SetMigratedTabs` suppression scaffold to `AudioStickyBar`

**Files:**
- Modify: `src/go/internal/ui/audio_sticky_bar.go` (struct field, accessor, Layout gates)
- Test: `src/go/internal/ui/audio_sticky_bar_migration_test.go`

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/audio_sticky_bar_migration_test.go`:

```go
package ui

import (
	"image"
	"testing"
)

// When a tab is marked migrated, the sticky bar suppresses its freeze pill on
// that tab (the per-tab component owns it instead).
func TestStickyBarSuppressesFreezeForMigratedTab(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	b := NewAudioStickyBar(130, func() {}, func() bool { return false }, func() {}, func(PanelTab) {})
	rect := image.Rect(0, 0, 600, 26)

	b.SetActiveTab(TabWave)
	b.Layout(rect)
	if b.FreezeBtn().Rect().Empty() {
		t.Fatal("pre-migration: freeze should be laid out on Wave")
	}

	b.SetMigratedTabs(TabWave)
	b.Layout(rect)
	if !b.FreezeBtn().Rect().Empty() {
		t.Fatal("post-migration: freeze must be suppressed on Wave")
	}

	// Non-migrated tab still shows freeze.
	b.SetActiveTab(TabSpectrum)
	b.Layout(rect)
	if b.FreezeBtn().Rect().Empty() {
		t.Fatal("freeze should still show on non-migrated Spectrum")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestStickyBarSuppressesFreezeForMigratedTab -v`
Expected: FAIL — `b.SetMigratedTabs undefined`.

- [ ] **Step 3: Add the field, setter, and Layout gates**

In `audio_sticky_bar.go`, add to the `AudioStickyBar` struct (after `parentZIndex int`, line 71):

```go
	// migrated records tabs whose per-tab controls have moved to their own
	// component (audio_tab_controls.go). The bar suppresses its legacy pills
	// for these tabs so a control never renders twice during the phased
	// migration. Temporary scaffold — removed in the slim-bar phase.
	migrated map[PanelTab]bool
```

Add after `SetActiveTab` (after line 487):

```go
// SetMigratedTabs marks tabs whose controls now live in their own per-tab
// component; the bar suppresses its legacy pills for them. Temporary scaffold
// for the phased audio-panel decoupling — deleted with the legacy pills.
func (b *AudioStickyBar) SetMigratedTabs(tabs ...PanelTab) {
	b.migrated = make(map[PanelTab]bool, len(tabs))
	for _, t := range tabs {
		b.migrated[t] = true
	}
}

func (b *AudioStickyBar) isMigrated(t PanelTab) bool {
	return b.migrated != nil && b.migrated[t]
}
```

In `Layout`, gate the per-tab pill blocks on `!isMigrated`:
- Freeze (line 222): change `freezeWantsButton := b.activeTab == TabWave || ...` to:
```go
	freezeWantsButton := (b.activeTab == TabWave || b.activeTab == TabSpectrum ||
		b.activeTab == TabMeters || b.activeTab == TabScope) && !b.isMigrated(b.activeTab)
```
- Freq-scale (line 238): `if b.activeTab == TabSpectrum && !b.isMigrated(TabSpectrum) {`
- Spectrum pills (line 251): `if b.activeTab == TabSpectrum && !Profile().IsMobile() && !b.isMigrated(TabSpectrum) {`
- Levels pills (line 310): `if b.activeTab == TabMeters && !Profile().IsMobile() && !b.isMigrated(TabMeters) {`

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestStickyBarSuppressesFreezeForMigratedTab -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/ui/audio_sticky_bar.go src/go/internal/ui/audio_sticky_bar_migration_test.go
git commit -m "feat(ui): sticky-bar SetMigratedTabs suppression scaffold (decoupling phase 1)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 1.2: Implement `waveControls` + wire it into the dispatcher

**Files:**
- Modify: `src/go/internal/ui/audio_tab_controls.go` (add `waveControls` type)
- Modify: `src/go/internal/ui/eq_panel_zone.go` (field, ctor, selector, Layout, Draw, rebuildHitAreas)
- Test: `src/go/internal/ui/audio_tab_controls_test.go`

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/audio_tab_controls_test.go`:

```go
// On TabWave, the dispatcher publishes the wave freeze hit area in the control
// header (below the switcher row, above the body), and NOT via the sticky bar.
func TestWaveControlsOwnFreeze(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	froze := 0
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { froze++; return froze%2 == 1 }})
	z.SetActiveTab(TabWave)
	z.Layout(image.Rect(0, 400, 600, 600))

	// Header height is non-zero and body sits below it.
	if z.controlHeaderH() != audioControlHeaderH {
		t.Fatalf("wave control header = %d, want %d", z.controlHeaderH(), audioControlHeaderH)
	}
	if z.bodyRect().Min.Y != z.contentRect().Min.Y+audioControlHeaderH {
		t.Fatalf("body top %d, want %d", z.bodyRect().Min.Y, z.contentRect().Min.Y+audioControlHeaderH)
	}

	// The wave-freeze hit area is published, inside the header strip.
	var freezeHit *HitArea
	for i := range z.HitAreas() {
		if z.HitAreas()[i].Tag == "wave-freeze-btn" {
			freezeHit = &z.HitAreas()[i]
		}
	}
	if freezeHit == nil {
		t.Fatal("wave-freeze-btn hit area not published on TabWave")
	}
	if freezeHit.Rect.Min.Y < z.headerRect().Min.Y || freezeHit.Rect.Max.Y > z.headerRect().Max.Y {
		t.Fatalf("freeze rect %v not inside header %v", freezeHit.Rect, z.headerRect())
	}

	// The sticky bar no longer lays out its own freeze on Wave.
	if !z.stickyBar.FreezeBtn().Rect().Empty() {
		t.Fatal("sticky bar freeze must be suppressed on migrated Wave tab")
	}

	// Clicking the wave freeze invokes OnFreezeToggle.
	freezeHit.Handler.OnPress(freezeHit.Rect.Min.X+1, freezeHit.Rect.Min.Y+1)
	freezeHit.Handler.OnRelease(freezeHit.Rect.Min.X+1, freezeHit.Rect.Min.Y+1)
	if froze == 0 {
		t.Fatal("clicking wave freeze did not invoke OnFreezeToggle")
	}
}

// On another tab, the wave freeze hit area is not published.
func TestWaveControlsInactiveNoHitAreas(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	z.SetActiveTab(TabSpectrum)
	z.Layout(image.Rect(0, 400, 600, 600))
	for _, h := range z.HitAreas() {
		if h.Tag == "wave-freeze-btn" {
			t.Fatal("wave-freeze-btn must not publish when Spectrum is active")
		}
	}
}
```

(Confirm `buttonHitAdapter` implements `OnPress`/`OnRelease`; if its press-then-release semantics differ, drive the click via `z.waveControls.freezeBtn.OnClick()` instead — both are acceptable. The hit-area-presence assertions are the load-bearing checks.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveControls' -v`
Expected: FAIL — `controlHeaderH` returns 0 / no `wave-freeze-btn` hit area.

- [ ] **Step 3: Implement `waveControls`**

Append to `src/go/internal/ui/audio_tab_controls.go`:

```go
// waveControls owns the Wave tab's control header: a single freeze pill.
type waveControls struct {
	freezeBtn *Button
	z         int // hit-area z-index (panel zIdx + 1)
	hitAreas  []HitArea
}

func newWaveControls(z int, onFreeze func() bool) *waveControls {
	return &waveControls{freezeBtn: newFreezePill(onFreeze), z: z}
}

func (c *waveControls) HeaderH() int { return controlHeaderHeight() }

func (c *waveControls) Layout(header image.Rectangle) {
	d := Profile().DensityValues()
	btnH := header.Dy() - 8
	if btnH < d.AudioPillH {
		btnH = d.AudioPillH
	}
	y := header.Min.Y + 4
	rightEdge := header.Max.X - SpaceSM
	w := d.AudioPillIconW
	c.freezeBtn.SetRect(image.Rect(rightEdge-w, y, rightEdge, y+btnH))
	c.rebuildHitAreas()
}

func (c *waveControls) rebuildHitAreas() {
	c.hitAreas = c.hitAreas[:0]
	if r := c.freezeBtn.Rect(); !r.Empty() {
		c.hitAreas = append(c.hitAreas, HitArea{
			Rect: r, ZIndex: c.z,
			Handler: &buttonHitAdapter{btn: c.freezeBtn}, Tag: "wave-freeze-btn",
		})
	}
}

func (c *waveControls) Draw(dst *ebiten.Image) {
	drawPillTabAt(dst, c.freezeBtn, c.freezeBtn.Text == ">")
}

func (c *waveControls) HitAreas() []HitArea { return c.hitAreas }

func (c *waveControls) SyncFreeze(frozen bool) { applyFreezeVisual(c.freezeBtn, frozen) }
```

- [ ] **Step 4: Wire it into the dispatcher**

In `eq_panel_zone.go`:

(a) Add the field after `stickyBar` (line 133):
```go
	// Per-tab control components (audio_tab_controls.go) — each owns its own
	// buttons. nil for tabs that own controls elsewhere.
	waveControls *waveControls
```

(b) In `initButtons`, after the sticky bar is created + its legend/expander wired (after line 358), construct the component and mark the migrated tab:
```go
	// Per-tab control components own their buttons (audio-panel decoupling).
	// ctrlZ sits one above the eq-panel catch-all (zIdx=130) so header pills
	// win taps over the panel body — same z the sticky bar chrome uses.
	const ctrlZ = 131
	z.waveControls = newWaveControls(ctrlZ, z.callbacks.OnFreezeToggle)
	z.stickyBar.SetMigratedTabs(TabWave)
```

(c) Replace the Phase-0 stub `activeTabControls` with the real selector:
```go
func (z *EQPanelZone) activeTabControls() tabControls {
	switch z.tabState.ActiveTab() {
	case TabWave:
		if z.waveControls != nil {
			return z.waveControls
		}
	}
	return nil
}
```

(d) In `Layout` (after `z.layoutButtons()` at line 408), lay out the active control header:
```go
	if c := z.activeTabControls(); c != nil {
		c.Layout(z.headerRect())
	}
```

(e) In `Draw`, replace the sticky-bar freeze-sync block (lines 652–665) so the ACTIVE component syncs+draws its own freeze, while the bar block only runs for non-migrated tabs. Insert just before the `// HPF/LPF live inside...` comment (line 667):
```go
	// Per-tab control header: sync its freeze glyph from analyzer state, then
	// draw it on top of the body (below the sticky bar, which is drawn last).
	if c := z.activeTabControls(); c != nil {
		frozen := false
		if state := z.getAnalyzerStateForTab(activeTab); state != nil && state.Capture != nil {
			frozen = state.Capture.Frozen
		}
		c.SyncFreeze(frozen)
		c.Draw(screen)
	}
```
Leave the existing bar freeze-sync block (652–665) in place — it harmlessly syncs the bar's freeze for non-migrated tabs (on migrated tabs the bar freeze has an empty rect, so it draws nothing).

(f) In `rebuildHitAreas`, after the sticky-bar hits append (line 1200), publish the active component's hits:
```go
	if c := z.activeTabControls(); c != nil {
		z.hitAreas = append(z.hitAreas, c.HitAreas()...)
	}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveControls|TestPhase0NoControlHeader' -v`
Expected: PASS. (`TestPhase0NoControlHeader` now expects non-zero header only on Wave — update it: change its loop to skip TabWave, or assert `controlHeaderH()==0` for all tabs EXCEPT TabWave. Apply this edit: in `TestPhase0NoControlHeader`, wrap the assertions in `if tab != TabWave {`.)

- [ ] **Step 6: Run the broader audio-panel suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'AudioPanel|StickyBar|Wave' -v`
Expected: PASS or pre-existing-only failures (diff against baseline).

- [ ] **Step 7: Commit**

```bash
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/ui/audio_tab_controls.go src/go/internal/ui/audio_tab_controls_test.go src/go/internal/ui/eq_panel_zone.go
git commit -m "feat(ui): Wave tab owns its freeze control (audio-panel decoupling phase 1)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 2 — Levels controls (freeze + Clear-Clips + K-20)

`levelsControls` owns freeze, Clear-Clips, and K-20. The dispatcher's Levels content read of K-20 redirects to it; the bar suppresses these on TabMeters.

### Task 2.1: Implement `levelsControls` + wire it

**Files:**
- Modify: `src/go/internal/ui/audio_tab_controls.go` (add `levelsControls`)
- Modify: `src/go/internal/ui/eq_panel_zone.go` (field, ctor, selector, content K-20 read)
- Test: `src/go/internal/ui/audio_tab_controls_test.go`

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/audio_tab_controls_test.go`:

```go
func TestLevelsControlsButtonsAndWiring(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	cleared := 0
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	// Replace the Clear-Clips audio side effect by spying through the latch:
	z.levelsLatches = NewMultiLevelsLatch()
	z.SetActiveTab(TabMeters)
	z.Layout(image.Rect(0, 400, 600, 600))

	tags := map[string]bool{}
	for _, h := range z.HitAreas() {
		tags[h.Tag] = true
	}
	for _, want := range []string{"levels-freeze-btn", "levels-clear-clips-btn", "levels-k20-btn"} {
		if !tags[want] {
			t.Fatalf("levels control %q hit area missing", want)
		}
	}

	// K-20 toggles via the component and is what the renderer reads.
	if z.levelsControls.K20View() {
		t.Fatal("K20 should start false")
	}
	z.levelsControls.k20Btn.OnClick()
	if !z.levelsControls.K20View() {
		t.Fatal("K20 toggle did not flip")
	}

	// Clear-Clips clears the latches (proxy for the wired callback firing).
	z.levelsLatches.Get("main").Update(3, -1, -1)
	z.levelsControls.clearClipsBtn.OnClick()
	_ = cleared
	if z.levelsLatches.Get("main").ClipCount() != 0 { // see note on accessor below
		t.Fatal("Clear-Clips did not clear the latch")
	}

	// Sticky bar suppresses its own clear/k20/freeze on Meters.
	if !z.stickyBar.FreezeBtn().Rect().Empty() || !z.stickyBar.ClearClipsBtn().Rect().Empty() || !z.stickyBar.K20Btn().Rect().Empty() {
		t.Fatal("sticky bar must suppress levels pills on migrated Meters tab")
	}
}
```

Note on the latch assertion: if `MultiLevelsLatch.Get(...)` returns a latch without a `ClipCount()` accessor, replace that assertion block with a direct callback spy: construct the zone normally and instead assert the component's `clearClipsBtn.OnClick` is non-nil and calls through — simplest is to verify `z.levelsControls.clearClipsBtn.OnClick != nil` and that calling it does not panic, plus a separate `audio`-level test. Keep the hit-area + K-20 assertions (the load-bearing ones) regardless.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestLevelsControlsButtonsAndWiring -v`
Expected: FAIL — `z.levelsControls` undefined.

- [ ] **Step 3: Implement `levelsControls`**

Append to `src/go/internal/ui/audio_tab_controls.go`:

```go
// levelsControls owns the Levels tab's control header: freeze, Clear-Clips
// (CLR), and the K-20 reference-scale toggle. Clear-Clips and K-20 are
// desktop-only (mobile hides them, mirroring the legacy bar); freeze shows on
// both platforms.
type levelsControls struct {
	freezeBtn     *Button
	clearClipsBtn *Button
	k20Btn        *Button
	k20View       bool
	z             int
	hitAreas      []HitArea
}

func newLevelsControls(z int, onFreeze func() bool, onClearClips func()) *levelsControls {
	c := &levelsControls{z: z}
	c.freezeBtn = newFreezePill(onFreeze)
	c.clearClipsBtn = NewButton("CLR", InstButtonStyle, onClearClips)
	c.clearClipsBtn.TextColor = colTextSecondary
	c.k20Btn = NewButton("K20", InstButtonStyle, nil)
	c.k20Btn.TextColor = colTextSecondary
	c.k20Btn.OnClick = func() { c.k20View = !c.k20View }
	return c
}

// K20View reports whether the Bob Katz -20 dBFS reference scale is active. The
// Levels renderer reads this each frame.
func (c *levelsControls) K20View() bool { return c.k20View }

func (c *levelsControls) HeaderH() int { return controlHeaderHeight() }

func (c *levelsControls) Layout(header image.Rectangle) {
	d := Profile().DensityValues()
	btnH := header.Dy() - 8
	if btnH < d.AudioPillH {
		btnH = d.AudioPillH
	}
	y := header.Min.Y + 4
	rightEdge := header.Max.X - SpaceSM
	// Freeze — both platforms.
	fw := d.AudioPillIconW
	c.freezeBtn.SetRect(image.Rect(rightEdge-fw, y, rightEdge, y+btnH))
	rightEdge -= fw + d.AudioPillGap
	// Clear-Clips + K-20 — desktop-only (mirrors legacy bar gating).
	if !Profile().IsMobile() {
		cw := d.AudioPillWideW
		c.clearClipsBtn.SetRect(image.Rect(rightEdge-cw, y, rightEdge, y+btnH))
		rightEdge -= cw + d.AudioPillGap
		kw := d.AudioPillWideW
		c.k20Btn.SetRect(image.Rect(rightEdge-kw, y, rightEdge, y+btnH))
		rightEdge -= kw + d.AudioPillGap
	} else {
		c.clearClipsBtn.SetRect(image.Rectangle{})
		c.k20Btn.SetRect(image.Rectangle{})
	}
	c.rebuildHitAreas()
}

func (c *levelsControls) rebuildHitAreas() {
	c.hitAreas = c.hitAreas[:0]
	add := func(b *Button, tag string) {
		if b == nil {
			return
		}
		if r := b.Rect(); !r.Empty() {
			c.hitAreas = append(c.hitAreas, HitArea{
				Rect: r, ZIndex: c.z,
				Handler: &buttonHitAdapter{btn: b}, Tag: tag,
			})
		}
	}
	add(c.freezeBtn, "levels-freeze-btn")
	add(c.clearClipsBtn, "levels-clear-clips-btn")
	add(c.k20Btn, "levels-k20-btn")
}

func (c *levelsControls) Draw(dst *ebiten.Image) {
	drawPillTabAt(dst, c.freezeBtn, c.freezeBtn.Text == ">")
	drawPillTabAt(dst, c.clearClipsBtn, false)
	drawPillTabAt(dst, c.k20Btn, c.k20View)
}

func (c *levelsControls) HitAreas() []HitArea { return c.hitAreas }

func (c *levelsControls) SyncFreeze(frozen bool) { applyFreezeVisual(c.freezeBtn, frozen) }
```

- [ ] **Step 4: Wire into the dispatcher**

In `eq_panel_zone.go`:

(a) Add field after `waveControls` (line ~134):
```go
	levelsControls *levelsControls
```

(b) In `initButtons`, after `z.waveControls = ...`, add (and extend the migrated set):
```go
	z.levelsControls = newLevelsControls(ctrlZ, z.callbacks.OnFreezeToggle, func() {
		if z.levelsLatches != nil {
			z.levelsLatches.Clear()
		}
		audio.ResetClipsWindow()
	})
	z.stickyBar.SetMigratedTabs(TabWave, TabMeters)
```
(Replace the prior `SetMigratedTabs(TabWave)` line so there is a single call listing all migrated tabs.)

(c) Extend `activeTabControls`:
```go
	case TabMeters:
		if z.levelsControls != nil {
			return z.levelsControls
		}
```

(d) Redirect the K-20 read in the Levels content path. The Levels renderer reads K-20 from the sticky bar today — find that read (search `K20View` in `render_meters.go` / `eq_panel_zone.go` Draw TabMeters case) and change it to `z.levelsControls.K20View()`. If `drawLevelsMultiChannel` takes K-20 as a parameter, pass `z.levelsControls.K20View()`. If it reads `z.stickyBar.K20View()` internally, thread the value through instead. Run `grep -rn "K20View" src/go/internal/ui` to find every read and point them at `levelsControls`.

(e) Remove the now-dead Clear-Clips/Reset-Hold wiring in `initButtons` that targeted the sticky bar **only for Clear-Clips** (lines 334–345): the `levelsControls` constructor now owns that callback. Delete the `if clr := z.stickyBar.ClearClipsBtn(); clr != nil { ... }` block. (Leave the Reset-Hold block for now — handled in Phase 3.)

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestLevelsControls|TestWaveControls' -v`
Expected: PASS.

- [ ] **Step 6: Honor the Levels draw-alloc budget**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestEQPanelDrawAlloc -v`
Expected: PASS. If it fails on `TabMeters`, update `perTabAllocBudget[TabMeters]` in `eq_panel_draw_alloc_discipline_test.go` with a one-line rationale: `// +N: levels control header drawn as its own component (decoupling)`.

- [ ] **Step 7: Commit**

```bash
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/ui/audio_tab_controls.go src/go/internal/ui/audio_tab_controls_test.go src/go/internal/ui/eq_panel_zone.go src/go/internal/ui/render_meters.go src/go/internal/ui/eq_panel_draw_alloc_discipline_test.go
git commit -m "feat(ui): Levels tab owns freeze/clear-clips/K20 (audio-panel decoupling phase 2)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 3 — Spectrum controls (freeze + freq-scale + slope + pre + reset)

`spectrumControls` owns all five Spectrum pills. The dispatcher's Spectrum content reads (freq-scale, slope, pre) redirect to it; reset-hold is wired to `spectrumPeaks.ResetMax`; the bar suppresses these on TabSpectrum.

### Task 3.1: Implement `spectrumControls` + wire it

**Files:**
- Modify: `src/go/internal/ui/audio_tab_controls.go` (add `spectrumControls`)
- Modify: `src/go/internal/ui/eq_panel_zone.go` (field, ctor, selector, content reads, remove reset-hold sticky wiring)
- Test: `src/go/internal/ui/audio_tab_controls_test.go`

- [ ] **Step 1: Write the failing test**

Append to `src/go/internal/ui/audio_tab_controls_test.go`:

```go
func TestSpectrumControlsButtonsAndWiring(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	z.SetActiveTab(TabSpectrum)
	z.Layout(image.Rect(0, 400, 600, 600))

	tags := map[string]bool{}
	for _, h := range z.HitAreas() {
		tags[h.Tag] = true
	}
	for _, want := range []string{
		"spectrum-freeze-btn", "spectrum-freqscale-btn",
		"spectrum-slope-btn", "spectrum-pre-btn", "spectrum-reset-hold-btn",
	} {
		if !tags[want] {
			t.Fatalf("spectrum control %q hit area missing", want)
		}
	}

	// Slope cycles 0 -> 3 -> 4.5 -> 0.
	if z.spectrumControls.SlopeDBPerOct() != 0 {
		t.Fatal("slope should start at 0")
	}
	z.spectrumControls.slopeBtn.OnClick()
	if z.spectrumControls.SlopeDBPerOct() != 3 {
		t.Fatalf("slope after 1 click = %v, want 3", z.spectrumControls.SlopeDBPerOct())
	}
	z.spectrumControls.slopeBtn.OnClick()
	if z.spectrumControls.SlopeDBPerOct() != 4.5 {
		t.Fatalf("slope after 2 clicks = %v, want 4.5", z.spectrumControls.SlopeDBPerOct())
	}
	z.spectrumControls.slopeBtn.OnClick()
	if z.spectrumControls.SlopeDBPerOct() != 0 {
		t.Fatalf("slope after 3 clicks = %v, want 0 (wrap)", z.spectrumControls.SlopeDBPerOct())
	}

	// Freq-scale defaults to log and toggles to lin.
	if !z.spectrumControls.FreqScaleLog() {
		t.Fatal("freq scale should default to log")
	}
	z.spectrumControls.freqScaleBtn.OnClick()
	if z.spectrumControls.FreqScaleLog() {
		t.Fatal("freq scale did not toggle to lin")
	}

	// Pre overlay toggles.
	if z.spectrumControls.PreOverlay() {
		t.Fatal("pre overlay should start false")
	}
	z.spectrumControls.preBtn.OnClick()
	if !z.spectrumControls.PreOverlay() {
		t.Fatal("pre overlay did not toggle")
	}

	// Reset-hold clears the peak watermark (does not panic; wired callback).
	z.spectrumControls.resetHoldBtn.OnClick()

	// Sticky bar suppresses its own spectrum pills.
	if !z.stickyBar.FreezeBtn().Rect().Empty() || !z.stickyBar.FreqScaleBtn().Rect().Empty() ||
		!z.stickyBar.SlopeBtn().Rect().Empty() || !z.stickyBar.PreBtn().Rect().Empty() ||
		!z.stickyBar.ResetHoldBtn().Rect().Empty() {
		t.Fatal("sticky bar must suppress spectrum pills on migrated Spectrum tab")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestSpectrumControlsButtonsAndWiring -v`
Expected: FAIL — `z.spectrumControls` undefined.

- [ ] **Step 3: Implement `spectrumControls`**

Append to `src/go/internal/ui/audio_tab_controls.go`:

```go
// spectrumControls owns the Spectrum tab's control header: freeze, freq-scale
// (log/lin), slope tilt (0/3/4.5 dB/oct), Pre overlay toggle, and Reset-Hold.
// Freeze shows on both platforms; the other four are desktop-only (mirroring
// the legacy bar gating).
type spectrumControls struct {
	freezeBtn    *Button
	freqScaleBtn *Button
	freqScaleLog bool
	slopeBtn     *Button
	slopeOptions []float64
	slopeIdx     int
	preBtn       *Button
	preOverlay   bool
	resetHoldBtn *Button
	z            int
	hitAreas     []HitArea
}

func newSpectrumControls(z int, onFreeze func() bool, onResetHold func()) *spectrumControls {
	c := &spectrumControls{z: z, freqScaleLog: true, slopeOptions: []float64{0, 3, 4.5}}
	c.freezeBtn = newFreezePill(onFreeze)

	c.freqScaleBtn = NewButton("log", InstButtonStyle, nil)
	c.freqScaleBtn.TextColor = colTextSecondary
	c.freqScaleBtn.OnClick = func() {
		c.freqScaleLog = !c.freqScaleLog
		if c.freqScaleLog {
			c.freqScaleBtn.Text = "log"
		} else {
			c.freqScaleBtn.Text = "lin"
		}
	}

	c.slopeBtn = NewButton(formatSlopeLabel(0), InstButtonStyle, nil)
	c.slopeBtn.TextColor = colTextSecondary
	c.slopeBtn.OnClick = func() {
		c.slopeIdx = (c.slopeIdx + 1) % len(c.slopeOptions)
		v := c.slopeOptions[c.slopeIdx]
		c.slopeBtn.Text = formatSlopeLabel(v)
		SetSpectrumSlope(v)
	}

	c.preBtn = NewButton("Pre", InstButtonStyle, nil)
	c.preBtn.TextColor = colTextSecondary
	c.preBtn.OnClick = func() { c.preOverlay = !c.preOverlay }

	c.resetHoldBtn = NewButton("R", InstButtonStyle, onResetHold)
	c.resetHoldBtn.TextColor = colTextSecondary
	return c
}

// SlopeDBPerOct / PreOverlay / FreqScaleLog are read by the spectrum renderer.
func (c *spectrumControls) SlopeDBPerOct() float64 { return c.slopeOptions[c.slopeIdx] }
func (c *spectrumControls) PreOverlay() bool       { return c.preOverlay }
func (c *spectrumControls) FreqScaleLog() bool     { return c.freqScaleLog }

func (c *spectrumControls) HeaderH() int { return controlHeaderHeight() }

func (c *spectrumControls) Layout(header image.Rectangle) {
	d := Profile().DensityValues()
	btnH := header.Dy() - 8
	if btnH < d.AudioPillH {
		btnH = d.AudioPillH
	}
	y := header.Min.Y + 4
	rightEdge := header.Max.X - SpaceSM
	// Freeze — both platforms.
	fw := d.AudioPillIconW
	c.freezeBtn.SetRect(image.Rect(rightEdge-fw, y, rightEdge, y+btnH))
	rightEdge -= fw + d.AudioPillGap
	if !Profile().IsMobile() {
		// freq-scale (wide).
		qw := d.AudioPillWideW
		c.freqScaleBtn.SetRect(image.Rect(rightEdge-qw, y, rightEdge, y+btnH))
		rightEdge -= qw + d.AudioPillGap
		// reset-hold (narrow).
		rw := d.AudioPillNarrowW
		c.resetHoldBtn.SetRect(image.Rect(rightEdge-rw, y, rightEdge, y+btnH))
		rightEdge -= rw + d.AudioPillGap
		// pre (wide).
		pw := d.AudioPillWideW
		c.preBtn.SetRect(image.Rect(rightEdge-pw, y, rightEdge, y+btnH))
		rightEdge -= pw + d.AudioPillGap
		// slope (text-sized, min slope width).
		c.slopeBtn.Text = formatSlopeLabel(c.slopeOptions[c.slopeIdx])
		sw := TextWidth(c.slopeBtn.Text) + d.AudioPillPadX
		if sw < d.AudioPillSlopeMinW {
			sw = d.AudioPillSlopeMinW
		}
		c.slopeBtn.SetRect(image.Rect(rightEdge-sw, y, rightEdge, y+btnH))
		rightEdge -= sw + d.AudioPillGap
	} else {
		c.freqScaleBtn.SetRect(image.Rectangle{})
		c.resetHoldBtn.SetRect(image.Rectangle{})
		c.preBtn.SetRect(image.Rectangle{})
		c.slopeBtn.SetRect(image.Rectangle{})
	}
	c.rebuildHitAreas()
}

func (c *spectrumControls) rebuildHitAreas() {
	c.hitAreas = c.hitAreas[:0]
	add := func(b *Button, tag string) {
		if b == nil {
			return
		}
		if r := b.Rect(); !r.Empty() {
			c.hitAreas = append(c.hitAreas, HitArea{
				Rect: r, ZIndex: c.z,
				Handler: &buttonHitAdapter{btn: b}, Tag: tag,
			})
		}
	}
	add(c.freezeBtn, "spectrum-freeze-btn")
	add(c.freqScaleBtn, "spectrum-freqscale-btn")
	add(c.slopeBtn, "spectrum-slope-btn")
	add(c.preBtn, "spectrum-pre-btn")
	add(c.resetHoldBtn, "spectrum-reset-hold-btn")
}

func (c *spectrumControls) Draw(dst *ebiten.Image) {
	drawPillTabAt(dst, c.freezeBtn, c.freezeBtn.Text == ">")
	drawPillTabAt(dst, c.freqScaleBtn, !c.freqScaleLog)
	drawPillTabAt(dst, c.slopeBtn, c.slopeIdx != 0)
	drawPillTabAt(dst, c.preBtn, c.preOverlay)
	drawPillTabAt(dst, c.resetHoldBtn, false)
}

func (c *spectrumControls) HitAreas() []HitArea { return c.hitAreas }

func (c *spectrumControls) SyncFreeze(frozen bool) { applyFreezeVisual(c.freezeBtn, frozen) }
```

- [ ] **Step 4: Wire into the dispatcher**

In `eq_panel_zone.go`:

(a) Add field after `levelsControls`:
```go
	spectrumControls *spectrumControls
```

(b) In `initButtons`, after `z.levelsControls = ...`, add and extend the migrated set:
```go
	z.spectrumControls = newSpectrumControls(ctrlZ, z.callbacks.OnFreezeToggle, func() {
		z.spectrumPeaks.ResetMax()
	})
	z.stickyBar.SetMigratedTabs(TabWave, TabMeters, TabSpectrum)
```
(Replace the prior `SetMigratedTabs(TabWave, TabMeters)` so there is one call.)

(c) Delete the now-dead Reset-Hold sticky-bar wiring block in `initButtons` (lines 329–333, the `if rst := z.stickyBar.ResetHoldBtn(); rst != nil { ... }`).

(d) Extend `activeTabControls`:
```go
	case TabSpectrum:
		if z.spectrumControls != nil {
			return z.spectrumControls
		}
```

(e) Redirect the Spectrum content reads in `Draw` (TabSpectrum case, lines 554–580):
- Line 556–559: change
  ```go
  scale := freqScaleLog
  if z.stickyBar != nil && !z.stickyBar.FreqScaleLog() {
      scale = freqScaleLinear
  }
  ```
  to
  ```go
  scale := freqScaleLog
  if z.spectrumControls != nil && !z.spectrumControls.FreqScaleLog() {
      scale = freqScaleLinear
  }
  ```
- Line 569: change `if z.stickyBar != nil && z.stickyBar.PreOverlay() {` to `if z.spectrumControls != nil && z.spectrumControls.PreOverlay() {`.
- Search for any `z.stickyBar.SlopeDBPerOct()` read (in `render_spectrum.go` or the spectrum draw path) and change it to `z.spectrumControls.SlopeDBPerOct()`. Run `grep -rn "SlopeDBPerOct\|PreOverlay\|FreqScaleLog" src/go/internal/ui` and repoint every dispatcher/render read at `spectrumControls`. (The `AudioStickyBar` accessor methods stay until Phase 4 but are no longer read.)

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestSpectrumControls|TestWaveControls|TestLevelsControls' -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/ui/audio_tab_controls.go src/go/internal/ui/audio_tab_controls_test.go src/go/internal/ui/eq_panel_zone.go src/go/internal/ui/render_spectrum.go
git commit -m "feat(ui): Spectrum tab owns its 5 pills (audio-panel decoupling phase 3)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 4 — Slim the sticky bar + delete the X close button

All per-tab buttons now live in their components. Remove the dead bar pills, the migration scaffold, the close button, and `EQCallbacks.OnClose`.

### Task 4.1: Cross-cutting assertions (write first, drive the removal)

**Files:**
- Test: `src/go/internal/ui/audio_tab_controls_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `src/go/internal/ui/audio_tab_controls_test.go`:

```go
// The main tab-switcher row publishes ONLY channel/tabs/legend/expander hit
// areas — never any per-tab control or the close button — on every tab.
func TestMainTabBarOnlyHasTabsChannelLegendExpander(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	banned := map[string]bool{
		"eq-freeze-btn": true, "eq-freqscale-btn": true, "eq-slope-btn": true,
		"eq-pre-btn": true, "eq-reset-hold-btn": true, "eq-clear-clips-btn": true,
		"eq-k20-btn": true, "eq-close-btn": true,
	}
	for _, tab := range AllPanelTabs() {
		z.SetActiveTab(tab)
		z.Layout(image.Rect(0, 400, 600, 600))
		for _, h := range z.stickyBar.HitAreas() {
			if banned[h.Tag] {
				t.Fatalf("tab %v: sticky bar still publishes banned hit %q", tab, h.Tag)
			}
		}
	}
}

// The X close button is gone everywhere.
func TestCloseButtonGone(t *testing.T) {
	restore := SetRuntimeProfileForTest(desktopRuntimeProfile())
	defer restore()
	z := NewEQPanelZone(EQCallbacks{})
	for _, tab := range AllPanelTabs() {
		z.SetActiveTab(tab)
		z.Layout(image.Rect(0, 400, 600, 600))
		for _, h := range z.HitAreas() {
			if h.Tag == "eq-close-btn" {
				t.Fatalf("tab %v: eq-close-btn hit area still present", tab)
			}
		}
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMainTabBarOnlyHasTabsChannelLegendExpander|TestCloseButtonGone' -v`
Expected: FAIL — bar still lays out `eq-close-btn` (and freeze/etc. on non-migrated tabs like EQ where freeze was never shown, but close is always present).

### Task 4.2: Remove the close button + dead pills from `AudioStickyBar`

**Files:**
- Modify: `src/go/internal/ui/audio_sticky_bar.go`

- [ ] **Step 3: Strip the bar**

In `audio_sticky_bar.go`:

(a) Delete struct fields (lines 36–57): `freezeBtn`, `closeBtn`, `freqScaleBtn`, `freqScaleLog`, `slopeBtn`, `preBtn`, `resetHoldBtn`, `slopeOptions`, `slopeIdx`, `preOverlay`, `clearClipsBtn`, `k20Btn`, `k20View`. Also delete the `migrated` field added in Phase 1. Keep `channelBtn`, `tabBtns`, `legendBtn`, `expanderBtn`, `activeTab`, `hitAreas`, `parentZIndex`, `rect`.

(b) Change the constructor signature to drop `onFreeze`/`onClose`:
```go
func NewAudioStickyBar(parentZIndex int, onChannel func(), onTab func(PanelTab)) *AudioStickyBar {
```
Delete the construction of `freezeBtn`, `freqScaleBtn`, `slopeBtn`, `preBtn`, `resetHoldBtn`, `clearClipsBtn`, `k20Btn`, `closeBtn` (lines 92–148, 161–166). Keep `channelBtn`, `tabBtns`, `legendBtn`, `expanderBtn` construction.

(c) In `Layout`, delete the close block (lines 211–215), the freeze block (217–230), the freq-scale block (232–245), the spectrum-pills block (247–282), and the levels-pills block (309–328). Also delete `SetMigratedTabs`/`isMigrated` (Phase-1 scaffold). Keep channel, legend/expander, and tab-pill blocks.

(d) In `rebuildHitAreas`, delete the `addBtn` calls for `freezeBtn`, `freqScaleBtn`, `slopeBtn`, `preBtn`, `resetHoldBtn`, `clearClipsBtn`, `k20Btn`, `closeBtn` (lines 394–400, 403). Keep channel/tabs/legend/expander.

(e) In `Draw`, delete the freeze (435–436), freq-scale (437–441), spectrum-pills (442–454), levels-pills (455–462), and close (469) draw blocks. Keep channel, tabs, legend, expander.

(f) Delete now-dead accessors: `FreezeBtn`, `CloseBtn`, `FreqScaleBtn`, `FreqScaleLog`, `SlopeDBPerOct`, `PreOverlay`, `SlopeBtn`, `PreBtn`, `ResetHoldBtn`, `ClearClipsBtn`, `K20Btn`, `K20View`. Keep `LegendBtn`, `ExpanderBtn`, `SetActiveTab`, `HitAreas`, `Rect`, `ChannelBtn`, `TabBtn`. Delete `formatSlopeLabel` ONLY if no longer referenced — it is now used by `spectrumControls`, so KEEP it (it lives in `audio_sticky_bar.go`; leave it there or move to `audio_tab_controls.go` — leaving it is fine).

Update the type doc comment (lines 25–29) to: `// AudioStickyBar owns the audio-panel switcher row: channel dropdown trigger, // tab pills, the "?" legend chip, and the panel expander chevron. Per-tab // controls live in their own components (audio_tab_controls.go).`

### Task 4.3: Update the dispatcher for the new constructor + remove OnClose

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go`

- [ ] **Step 4: Update dispatcher**

In `eq_panel_zone.go`:

(a) In `initButtons`, delete the `onFreeze` closure (290–306), the `onClose` closure (307–311), and the now-dead bar freeze-sync block in `Draw` (lines 652–665 — the `if freeze := z.stickyBar.FreezeBtn(); ...`). Change the constructor call (line 325) to:
```go
	z.stickyBar = NewAudioStickyBar(130, onChannel, onTab)
```
Delete the `z.stickyBar.SetMigratedTabs(...)` line (scaffold gone).

(b) Delete the `OnClose func()` field from `EQCallbacks` (lines 68–72).

(c) Remove `OnClose` from any caller. Run `grep -rn "OnClose" src/go/internal/ui/drumview_ctor.go` — the EQ panel's `EQCallbacks{...}` block (lines 199–383) does not set it, so likely no change there; but the Chain `ScopeCallbacks` at line 472 is a DIFFERENT struct (`ChainCallbacks`/scope) — DO NOT touch it. Only remove `OnClose` references that target `EQCallbacks`.

(d) `activeTabControls` now references all three components — confirm the final form:
```go
func (z *EQPanelZone) activeTabControls() tabControls {
	switch z.tabState.ActiveTab() {
	case TabWave:
		if z.waveControls != nil {
			return z.waveControls
		}
	case TabSpectrum:
		if z.spectrumControls != nil {
			return z.spectrumControls
		}
	case TabMeters:
		if z.levelsControls != nil {
			return z.levelsControls
		}
	}
	return nil
}
```

- [ ] **Step 5: Fix compile + the close-button unit test**

Build: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/`
Expected: compile errors point at removed symbols. Resolve each:
- `audio_sticky_bar_test.go:313`+ (`TestAudioStickyBarOnClose` / close-button test) — delete that test function entirely (the button is gone).
- Any test calling `NewAudioStickyBar(z, onChannel, onFreeze, onClose, onTab)` — update to the 3-arg form `NewAudioStickyBar(z, onChannel, onTab)`. Search: `grep -rn "NewAudioStickyBar(" src/go/internal/ui`.
- Any test referencing removed accessors (`FreezeBtn`, `CloseBtn`, `K20View`, etc.) on `AudioStickyBar` — update to the per-tab component (`z.waveControls.freezeBtn`, `z.levelsControls.K20View()`, …) or delete if redundant. Search each removed name.
- `audio_sticky_bar_migration_test.go` (Phase 1) — delete `TestStickyBarSuppressesFreezeForMigratedTab` (scaffold gone).

- [ ] **Step 6: Run the cross-cutting tests + full ui package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestMainTabBarOnlyHasTabsChannelLegendExpander|TestCloseButtonGone|TestWaveControls|TestLevelsControls|TestSpectrumControls' -v`
Expected: PASS.

Run the whole package: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS, or ONLY the pre-existing failures recorded for this branch (diff against the baseline noted in MEMORY: the internal/ui suite has known pre-existing breakage on `add-core-node-types`). Any NEW failure must be fixed.

- [ ] **Step 7: Commit**

```bash
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/ui/
git commit -m "feat(ui): slim sticky bar to switcher row + delete X close (decoupling phase 4)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Phase 5 — Mobile + Chain regression coverage, scene/JS guards

### Task 5.1: Mobile freeze + Chain freeze regression tests

**Files:**
- Test: `src/go/internal/ui/audio_tab_controls_test.go`

- [ ] **Step 1: Write the tests**

Append to `src/go/internal/ui/audio_tab_controls_test.go`:

```go
// On mobile, the desktop-only pills are absent but the freeze button is still
// present on Wave/Spectrum/Levels (no mobile freeze regression).
func TestMobileControlVisibilityPreserved(t *testing.T) {
	restore := SetRuntimeProfileForTest(browserRuntimeProfile())
	defer restore()
	r2 := SetDensityForTest(DensitySpacious)
	defer r2()
	// Force mobile screen-class.
	rp := SetMobileForTest(true) // if a helper exists; otherwise set viewport via the standard mobile test setup
	defer rp()

	z := NewEQPanelZone(EQCallbacks{OnFreezeToggle: func() bool { return false }})
	for _, tab := range []PanelTab{TabWave, TabSpectrum, TabMeters} {
		z.SetActiveTab(tab)
		z.Layout(image.Rect(0, 400, 360, 600))
		tags := map[string]bool{}
		for _, h := range z.HitAreas() {
			tags[h.Tag] = true
		}
		freezeTag := map[PanelTab]string{TabWave: "wave-freeze-btn", TabSpectrum: "spectrum-freeze-btn", TabMeters: "levels-freeze-btn"}[tab]
		if !tags[freezeTag] {
			t.Fatalf("mobile tab %v: freeze %q missing", tab, freezeTag)
		}
		// Desktop-only pills must be absent on mobile.
		for _, banned := range []string{"spectrum-slope-btn", "spectrum-pre-btn", "spectrum-reset-hold-btn", "spectrum-freqscale-btn", "levels-clear-clips-btn", "levels-k20-btn"} {
			if tags[banned] {
				t.Fatalf("mobile tab %v: desktop-only pill %q should be hidden", tab, banned)
			}
		}
	}
}
```

Note: use whatever mobile-forcing helper the codebase provides (search `grep -rn "func SetMobileForTest\|detectSmallScreen\|forceMobile" src/go/internal/ui`). If none exists, lay out with a narrow rect AND set the runtime/layout profile the same way existing mobile tests do (copy the setup from `pads_input_regression_test.go` or `responsive_layout_test.go`). The load-bearing assertions are freeze-present + desktop-pills-absent.

- [ ] **Step 2: Run + verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestMobileControlVisibilityPreserved -v`
Expected: PASS (fix the mobile-forcing setup until it does).

- [ ] **Step 3: Verify Chain freeze still works (its own component, unaffected by bar slim)**

Run the existing Chain test that exercises freeze: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Chain.*Freeze|ChainPanelZone' -v`
Expected: PASS. If no such test asserts the Chain header freeze specifically, add one mirroring `chain_panel_zone_test.go`'s close-button test (`chain_panel_zone_test.go:932`+) but driving `freezeBtn`.

- [ ] **Step 4: Commit**

```bash
cd /home/ymolinar/Repos/beatmo && git add src/go/internal/ui/audio_tab_controls_test.go src/go/internal/ui/chain_panel_zone_test.go
git commit -m "test(ui): mobile freeze + Chain freeze regression coverage (decoupling phase 5)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

### Task 5.2: Scene / pixel / JS-export guards

**Files:**
- Modify (as needed): scene catalog tests, `audio_panel_render_pixels_test.go`, `js_exports_*`, `wasm_bridge_smoke.browser.test.js`

- [ ] **Step 1: Find every reference to the removed sticky-bar surface**

Run:
```bash
cd /home/ymolinar/Repos/beatmo && grep -rn "FreezeBtn\|CloseBtn\|FreqScaleBtn\|ResetHoldBtn\|ClearClipsBtn\|K20Btn\|SlopeBtn\|PreBtn\|eq-close-btn\|eq-freeze-btn\|SubjectAudioStickyBar" src/go/internal/ui src/js | grep -v "_test.go:.*spectrum-\|wave-\|levels-"
```
Expected: a finite list. For each:
- Go production code → repoint at the per-tab component (already done in earlier phases; confirm none remain).
- Go test → update to the component or delete if redundant.
- Scene-subject crop (`SubjectAudioStickyBar` / `screenshot_subjects.go`) — the sticky bar is now shorter content-wise but its `Rect()` is unchanged (still the switcher row), so the crop still works; verify `scene_crop_test.go` passes.
- JS (`wasm_bridge_smoke.browser.test.js`) — only update if a removed Go symbol backed a JS export. Run the Go catalogue drift guard below; it will tell you.

- [ ] **Step 2: Run the catalogue-drift + scene-crop guards**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestJSExportsCatalogueDrift|TestDesignMDDrift|TestTokenDiscipline|SceneCrop' -v
```
Expected: PASS. Fix any drift (e.g. update the export catalogue if a removed accessor was exported — unlikely, these were internal).

- [ ] **Step 3: Run the full fast suite for the two touched packages**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ ./internal/audio/
```
Expected: PASS or pre-existing-only failures (diff against baseline). Any new failure is in-scope.

- [ ] **Step 4: Commit**

```bash
cd /home/ymolinar/Repos/beatmo && git add -A
git commit -m "test(ui): repoint scene/pixel/export guards after sticky-bar slim (decoupling phase 5)

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>"
```

---

## Final verification

- [ ] **Run the whole fast Go suite**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...
```
Expected: PASS, or ONLY the pre-existing failures documented for `add-core-node-types` (see MEMORY: real-mode parity + concurrent chip-strip Synth-tab FAILs). Confirm no NEW failures are attributable to this work by diffing the failure set against a clean baseline (`git stash` + run + compare).

- [ ] **Visual smoke (optional but recommended)**

```bash
make screenshots-all SCENES=$(./tmp/beatmo_screenshot -list-scenes | grep -i 'eq\|spectrum\|levels\|wave' | paste -sd ,)
```
Confirm the desktop audio panel now shows a clean two-row layout: tab-switcher row (channel · tabs · `?` · expander) with each tab's controls in the row directly below, and no X button.

- [ ] **Update MEMORY**

Add a one-line entry to `~/.claude/projects/-home-ymolinar-Repos-beatmo/memory/MEMORY.md` pointing at a new topic file summarizing: audio-panel tabs decoupled into per-tab `tabControls` components; sticky bar slimmed to switcher row; X close deleted; EQ extraction deferred.

---

## Self-review notes (addressed)

- **Spec coverage:** switcher-row slim (Phase 4), per-tab control ownership Wave/Spectrum/Levels (Phases 1–3), X-close deletion (Phase 4), mobile preservation (Phase 5.1), guards (Phase 5.2). EQ extraction explicitly deferred (non-goal). ✓
- **Type consistency:** `tabControls` interface methods (`HeaderH/Layout/Draw/HitAreas/SyncFreeze`) match every component; `newWaveControls/newLevelsControls/newSpectrumControls` signatures match their `eq_panel_zone.go` call sites; accessor names (`K20View`, `SlopeDBPerOct`, `PreOverlay`, `FreqScaleLog`) match between component and redirected reads. ✓
- **No placeholders:** every new file/struct/test has full code; moves cite exact line ranges + the transformation. The only deliberately open items are "search-and-repoint" greps where the count is environment-dependent (K20View/SlopeDBPerOct reads, mobile-forcing helper) — each gives the exact grep + the target. ✓
