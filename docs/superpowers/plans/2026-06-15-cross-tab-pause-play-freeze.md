# Cross-Tab Pause/Play (Freeze) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give Wave, Spectrum, Levels, and Chain each a pause/play button that freezes/unfreezes *only that tab's* displayed signal (independent per-tab freeze), drawn with `IconPause` (live) / `IconPlay` (frozen) icons, visual-only (never audio).

**Architecture:** Reuse existing infra. (1) Icon-ize the freeze button by setting `Button.Icon` + clearing `Text` (both `drawPillTabAt` and Chain's `drawPillButton` already render `btn.Icon`). (2) Model the analyzer-tab freeze on the **Chain tab's existing per-tab freeze** (`frozen` flag + snapshot held while frozen): `EQPanelZone` holds a per-tab `frozenByTab map[PanelTab]*analyzer.State`; `getAnalyzerStateForTab` returns the held immutable snapshot while a tab is frozen, else live. Audio analyzer is never frozen. Chain keeps its own `ScopeService` freeze (icon only).

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), package `internal/ui`.

**IMPORTANT — repo rules (override the skill's "commit" steps):**
- **Do NOT `git add`/commit/stash/restore/checkout/reset.** Leave all work in the working tree. Each "Checkpoint" step replaces the skill's "Commit" step: run the verification command, confirm green, STOP there.
- **Never delete (`rm`) or move repo files** to unblock a build (`mv` to /tmp + report if unavoidable).
- Use bundled Go `.tools/go/bin/go`. Authoritative build check: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` — **re-run 2–3×**; a transient "no such file" open error is an FS race from a concurrent agent.
- Test command (explicit path — NOT `./...`, which hits a phantom duplicate `ui` package on this branch):
  ```
  cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run '<RunFilter>'
  ```
- **CONCURRENCY:** Another agent is concurrently editing `internal/audio` and `internal/ui` (synth/i18n/settings). If `vet`/`test` fails in a file you did NOT touch (e.g. `synth_panel_zone.go`, `panel_tab.go`, `i18n_*`, `settings_*`, `internal/audio/*`), wait ~30s and retry; treat only errors in files YOU edited as yours. Do NOT fix their files.

---

## Background facts (verified — do not re-derive)

- `IconPause`, `IconPlay` exist in `icons.go`. `Button` has `Icon string` + `IconColor color.Color`. Setting `btn.Icon = string(IconPause)` and `btn.Text = ""` makes the button render the icon; `drawPillTabAt` (`audio_sticky_bar.go:335`) and Chain's `drawPillButton` (`chain_panel_zone.go:718`) both render `btn.Icon`.
- `audio_tab_controls.go`: `applyFreezeVisual(b *Button, frozen bool)` currently sets text `||`/`>` + color `colTextSecondary`/`colAccent`. `newFreezePill(onFreeze func() bool) *Button` builds the pill. `freezeControls` (embedded by `levelsControls`, `spectrumControls`) owns `freezeBtn`, `frozen bool`, `SyncFreeze`, `layoutFreezePill`, `addHit`. `layoutRightPill(header, btn, w)` is the shared right-pill placer. `waveControls` is standalone (autoBtn only; `SyncFreeze` is a no-op).
- `eq_panel_zone.go`: `getAnalyzerStateForTab(tab)` (≈line 2291) memoizes per-frame and reads live from `z.callbacks.AnalyzerState()` / `AnalyzerMetricsOnly()` (Levels). Field `frozenAnalyzer *analyzer.State` (≈line 206) is the legacy single WASM snapshot. Controls are constructed ≈line 344–360 (`newWaveControls`, `newLevelsControls`, `newSpectrumControls`) with `z.callbacks.OnFreezeToggle`. The per-tab control draw/sync block is ≈line 637–660 (`c.SyncFreeze(frozen)` then `c.Draw`).
- `analyzer.State` (`analyzer/state.go:7`) is "the atomically-published snapshot the UI reads" — fresh immutable pointer per refresh ⇒ holding the current pointer is a safe frozen snapshot.
- Chain freeze (`chain_panel_zone.go`): `frozen bool` (line 89), `SetFrozen(frozen)` (line 329), `freezeBtn` built at line 192 with text `||`, `frozenState *scope.State` snapshot (WASM). Drawn via `drawPillButton(dst, z.freezeBtn, z.frozen, false)` (line 680). `OnFreezeToggle` is best-effort; UI `z.frozen` is authoritative.

---

## File Structure

| File | Responsibility | Action |
|---|---|---|
| `internal/ui/audio_tab_controls.go` | `applyFreezeVisual` → icons; `waveControls` gains a freeze pill; per-tab toggle wiring entry points | Modify |
| `internal/ui/eq_panel_zone.go` | `frozenByTab` store, `toggleTabFreeze`, `getAnalyzerStateForTab` frozen routing, control construction + draw-sync, Levels hold | Modify |
| `internal/ui/chain_panel_zone.go` | Chain freeze button → icon (ctor + `SetFrozen`) | Modify |
| `internal/ui/render_waveform.go` | FROZEN tag sourced from a passed `tabFrozen` flag | Modify |
| `internal/ui/wave_trace_cache.go` | thread `tabFrozen` into `drawAnalyzerWaveform` | Modify |
| `internal/ui/freeze_pause_play_test.go` | new tests: icon toggle, per-tab independence, snapshot stability, Wave two-pill, Levels hold | Create |
| `internal/ui/audio_tab_controls_test.go` / `chain_*_test.go` | update existing `||`/`>` and global-freeze assertions | Modify |

---

## Task 1: Icon-ize the freeze button (shared `applyFreezeVisual` + Chain)

**Files:**
- Modify: `internal/ui/audio_tab_controls.go` (`applyFreezeVisual`)
- Modify: `internal/ui/chain_panel_zone.go` (`SetFrozen`, freeze `NewButton` ctor)
- Create: `internal/ui/freeze_pause_play_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/ui/freeze_pause_play_test.go`:

```go
package ui

import "testing"

func TestApplyFreezeVisualUsesIcons(t *testing.T) {
	b := NewButton("||", InstButtonStyle, nil)

	applyFreezeVisual(b, false) // live → pause icon (click to pause)
	if b.Icon != string(IconPause) {
		t.Fatalf("live: Icon=%q, want %q", b.Icon, IconPause)
	}
	if b.Text != "" {
		t.Fatalf("live: Text=%q, want empty (icon-only)", b.Text)
	}

	applyFreezeVisual(b, true) // frozen → play icon (click to resume)
	if b.Icon != string(IconPlay) {
		t.Fatalf("frozen: Icon=%q, want %q", b.Icon, IconPlay)
	}
	if b.Text != "" {
		t.Fatalf("frozen: Text=%q, want empty", b.Text)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestApplyFreezeVisualUsesIcons'`
Expected: FAIL (Icon stays empty; Text stays `||`/`>`).

- [ ] **Step 3: Implement — icon in `applyFreezeVisual`**

In `audio_tab_controls.go`, replace the body of `applyFreezeVisual`:

```go
// applyFreezeVisual sets the freeze pill's icon + color from a frozen bool.
// Live → pause icon (the action is "pause"); frozen → play icon (action
// "resume"). Icon-only: Text is cleared so drawPillTabAt renders the glyph.
func applyFreezeVisual(b *Button, frozen bool) {
	if b == nil {
		return
	}
	b.Text = ""
	if frozen {
		b.Icon = string(IconPlay)
		b.IconColor = colAccent
		b.TextColor = colAccent
	} else {
		b.Icon = string(IconPause)
		b.IconColor = colTextSecondary
		b.TextColor = colTextSecondary
	}
}
```

Also fix `newFreezePill` so the pill starts icon-only (no `||` text leaking before the first `applyFreezeVisual`). Change its `NewButton("||", …)` line to `NewButton("", InstButtonStyle, nil)` and call `applyFreezeVisual(b, false)` before returning. Read the current `newFreezePill` body and make that minimal change.

- [ ] **Step 4: Implement — Chain freeze button icon**

In `chain_panel_zone.go`:
- The ctor (≈line 192) builds `z.freezeBtn = NewButton("||", InstButtonStyle, …)`. Change `"||"` to `""` and after construction set `applyFreezeVisualChain(z.freezeBtn, z.frozen)` — but to keep it DRY, instead reuse the same icon logic. The Chain button visual is set in `SetFrozen` (≈line 339-348). Replace that text/color block:

```go
	z.frozen = frozen
	applyFreezeVisual(z.freezeBtn, z.frozen)
```

  (`applyFreezeVisual` is package-level in `audio_tab_controls.go`, callable here.) Then in the ctor, after building `z.freezeBtn` with `NewButton("", …)`, add `applyFreezeVisual(z.freezeBtn, z.frozen)` so it starts as a pause icon. Remove the now-dead `z.freezeBtn.TextColor = colTextSecondary` line at ctor if present (applyFreezeVisual sets it).

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestApplyFreezeVisualUsesIcons'`
Expected: PASS.

- [ ] **Step 6: Update existing text-glyph assertions**

Run: `cd src/go/internal/ui && grep -rn '"||"\|">"' --include='*_test.go' | grep -i 'freez\|frozen\|pause'`
For each test asserting the freeze button's `.Text == "||"`/`">"`, update it to assert `.Icon == string(IconPause)` / `string(IconPlay)` and `.Text == ""`. Show each edit. (Also grep non-test: `grep -rn 'freezeBtn.Text' --include='*.go'` — ensure nothing else still reads the glyph text.)

- [ ] **Step 7: Checkpoint (no commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` (retry for FS race) then the Step 5 test. Confirm green. Leave in working tree.

---

## Task 2: Per-tab analyzer freeze store + routing (`EQPanelZone`)

**Files:**
- Modify: `internal/ui/eq_panel_zone.go` (fields, `toggleTabFreeze`, `getAnalyzerStateForTab`)
- Modify: `internal/ui/freeze_pause_play_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/freeze_pause_play_test.go`:

```go
import_block_marker_TASK2 // (delete this line; ensure imports below are in the file's import block)

func TestToggleTabFreezeIsPerTabIndependent(t *testing.T) {
	live1 := &analyzer.State{Timestamp: 1}
	live2 := &analyzer.State{Timestamp: 2}
	cur := live1
	z := &EQPanelZone{}
	z.callbacks.AnalyzerState = func() *analyzer.State { return cur }

	// Freeze Wave: snapshots the current live state (live1).
	if got := z.toggleTabFreeze(TabWave); !got {
		t.Fatalf("toggleTabFreeze(TabWave) = false, want true (froze)")
	}
	cur = live2 // live advances

	// Wave is frozen at live1; Spectrum is NOT frozen → sees live2.
	z.frameAnalyzerValid = false
	if s := z.getAnalyzerStateForTab(TabWave); s != live1 {
		t.Fatalf("frozen Wave returned %v, want live1", s)
	}
	z.frameAnalyzerValid = false
	if s := z.getAnalyzerStateForTab(TabSpectrum); s != live2 {
		t.Fatalf("live Spectrum returned %v, want live2", s)
	}

	// Unfreeze Wave → back to live.
	if got := z.toggleTabFreeze(TabWave); got {
		t.Fatalf("second toggle = true, want false (unfroze)")
	}
	z.frameAnalyzerValid = false
	if s := z.getAnalyzerStateForTab(TabWave); s != live2 {
		t.Fatalf("unfrozen Wave returned %v, want live2", s)
	}
}
```

Add `"github.com/ingyamilmolinar/beatmo/internal/analyzer"` to the test file's import block (replace the `import_block_marker_TASK2` line by editing the existing `import (...)` — change the file's top `import "testing"` to a grouped import with both `"testing"` and the analyzer path).

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestToggleTabFreezeIsPerTabIndependent'`
Expected: FAIL — `z.toggleTabFreeze undefined`.

- [ ] **Step 3: Add the per-tab store field**

In `eq_panel_zone.go`, near the legacy `frozenAnalyzer *analyzer.State` field (≈line 206), add:

```go
	// frozenByTab holds a per-tab frozen snapshot of the analyzer state for the
	// signal tabs (Wave/Spectrum/Levels). An entry means that tab is paused and
	// renders the held immutable snapshot; absent means live. Independent per
	// tab — freezing one does not affect another, and the audio analyzer is
	// never frozen. Mirrors the Chain tab's frozenState pattern.
	frozenByTab map[PanelTab]*analyzer.State
```

- [ ] **Step 4: Implement `toggleTabFreeze` + frozen routing**

Add near `getAnalyzerStateForTab` in `eq_panel_zone.go`:

```go
// liveAnalyzerStateForTab returns the LIVE analyzer state for a tab (Levels uses
// the metrics-only callback). Split out so toggleTabFreeze can snapshot it
// without going through the frozen-aware getAnalyzerStateForTab.
func (z *EQPanelZone) liveAnalyzerStateForTab(tab PanelTab) *analyzer.State {
	if tab == TabMeters && z.callbacks.AnalyzerMetricsOnly != nil {
		return z.callbacks.AnalyzerMetricsOnly()
	}
	if z.callbacks.AnalyzerState != nil {
		return z.callbacks.AnalyzerState()
	}
	return nil
}

// toggleTabFreeze pauses/resumes the given signal tab independently. Returns the
// new frozen state. Freezing snapshots the current immutable published analyzer
// state (analyzer.State is atomically published, so holding the pointer freezes
// that tab's data while the service keeps producing fresh states for live tabs).
func (z *EQPanelZone) toggleTabFreeze(tab PanelTab) bool {
	if z.frozenByTab == nil {
		z.frozenByTab = map[PanelTab]*analyzer.State{}
	}
	if _, ok := z.frozenByTab[tab]; ok {
		delete(z.frozenByTab, tab)
		return false
	}
	live := z.liveAnalyzerStateForTab(tab)
	if live == nil {
		return false // nothing to freeze yet
	}
	z.frozenByTab[tab] = live
	return true
}

// tabFrozen reports whether the given signal tab is currently paused.
func (z *EQPanelZone) tabFrozen(tab PanelTab) bool {
	return z.frozenByTab[tab] != nil
}
```

Then modify `getAnalyzerStateForTab` to return the frozen snapshot first. Replace its body:

```go
func (z *EQPanelZone) getAnalyzerStateForTab(tab PanelTab) *analyzer.State {
	if z.frameAnalyzerValid && z.frameAnalyzerTab == tab {
		return z.frameAnalyzerState
	}
	var state *analyzer.State
	if fs := z.frozenByTab[tab]; fs != nil {
		state = fs
	} else if tab == TabMeters && z.callbacks.AnalyzerMetricsOnly != nil {
		state = z.callbacks.AnalyzerMetricsOnly()
	} else if z.callbacks.AnalyzerState != nil {
		state = z.callbacks.AnalyzerState()
	}
	z.frameAnalyzerState = state
	z.frameAnalyzerTab = tab
	z.frameAnalyzerValid = true
	return state
}
```

VERIFY `z.callbacks` is an `EQCallbacks` value with field `AnalyzerState func() *analyzer.State` and `AnalyzerMetricsOnly func() *analyzer.State` (read the EQCallbacks struct). If field names differ, match them. Confirm `z.frameAnalyzerValid/Tab/State` fields exist (they're used in the current function).

- [ ] **Step 5: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestToggleTabFreezeIsPerTabIndependent'`
Expected: PASS.

- [ ] **Step 6: Checkpoint (no commit)**

`cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/` (retry FS race) → clean. Leave in tree.

---

## Task 3: Wire analyzer freeze pills to per-tab toggle + add Wave freeze pill

**Files:**
- Modify: `internal/ui/audio_tab_controls.go` (`waveControls` gains freeze pill)
- Modify: `internal/ui/eq_panel_zone.go` (construct controls with per-tab toggle closures; draw-sync per-tab frozen)
- Modify: `internal/ui/freeze_pause_play_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/freeze_pause_play_test.go` (uses `image` — ensure imported):

```go
func TestWaveControlsHasBothAutoAndFreeze(t *testing.T) {
	autoCalls, freezeCalls := 0, 0
	c := newWaveControls(5,
		func() bool { autoCalls++; return autoCalls%2 == 1 },
		func() bool { freezeCalls++; return freezeCalls%2 == 1 },
	)
	c.Layout(image.Rect(0, 0, 300, 26))

	tags := map[string]bool{}
	for _, h := range c.HitAreas() {
		tags[h.Tag] = true
	}
	if !tags["wave-auto-btn"] {
		t.Fatalf("missing wave-auto-btn; tags=%v", tags)
	}
	if !tags["wave-freeze-btn"] {
		t.Fatalf("missing wave-freeze-btn; tags=%v", tags)
	}

	// Clicking freeze fires the freeze callback.
	for _, h := range c.HitAreas() {
		if h.Tag == "wave-freeze-btn" {
			h.Handler.OnPress(h.Rect.Min.X+1, h.Rect.Min.Y+1)
		}
	}
	if freezeCalls != 1 {
		t.Fatalf("freeze OnPress fired %d times, want 1", freezeCalls)
	}

	// SyncFreeze drives the freeze pill icon (frozen → play).
	c.SyncFreeze(true)
	if c.freezeBtn == nil || c.freezeBtn.Icon != string(IconPlay) {
		t.Fatalf("SyncFreeze(true) did not set play icon")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveControlsHasBothAutoAndFreeze'`
Expected: FAIL — `newWaveControls` takes one callback / `waveControls` has no `freezeBtn`.

- [ ] **Step 3: Add a freeze pill to `waveControls`**

In `audio_tab_controls.go`, update the `waveControls` struct + ctor + Layout + Draw + Sync. The struct currently is `{ autoBtn *Button; autoOn bool; z int; hitAreas []HitArea }`. Change to:

```go
type waveControls struct {
	autoBtn   *Button
	freezeBtn *Button
	autoOn    bool
	frozen    bool
	z         int
	hitAreas  []HitArea
}

// newWaveControls builds the Wave header: AUTO pill + freeze (pause/play) pill.
// onAuto toggles auto-gain; onFreeze toggles this tab's freeze. Both return the
// new state.
func newWaveControls(z int, onAuto func() bool, onFreeze func() bool) *waveControls {
	c := &waveControls{z: z, autoOn: true}
	c.autoBtn = NewButton("AUTO", InstButtonStyle, nil)
	c.autoBtn.TextColor = colTextSecondary
	c.autoBtn.OnClick = func() {
		if onAuto == nil {
			return
		}
		c.autoOn = onAuto()
	}
	c.freezeBtn = newFreezePill(onFreeze) // icon-only pause pill (Task 1)
	return c
}

func (c *waveControls) HeaderH() int        { return controlHeaderHeight() }
func (c *waveControls) HitAreas() []HitArea { return c.hitAreas }

// SyncFreeze drives the freeze pill icon from this tab's frozen state.
func (c *waveControls) SyncFreeze(frozen bool) {
	c.frozen = frozen
	applyFreezeVisual(c.freezeBtn, frozen)
}

// SyncAuto records the zone's auto-gain state so the pill highlight follows
// wheel-zoom (which disables auto) as well as the pill click.
func (c *waveControls) SyncAuto(on bool) { c.autoOn = on }

func (c *waveControls) Layout(header image.Rectangle) {
	// Freeze pill flush-right, AUTO pill to its left (icon pill width for
	// freeze, wide pill width for the AUTO text).
	_, _, rightEdge := layoutRightPill(header, c.freezeBtn, Profile().DensityValues().AudioPillIconW)
	d := Profile().DensityValues()
	btnH := header.Dy() - 8
	if btnH < d.AudioPillH {
		btnH = d.AudioPillH
	}
	y := header.Min.Y + 4
	aw := d.AudioPillWideW
	c.autoBtn.SetRect(image.Rect(rightEdge-aw, y, rightEdge, y+btnH))

	c.hitAreas = c.hitAreas[:0]
	if r := c.freezeBtn.Rect(); !r.Empty() {
		c.hitAreas = append(c.hitAreas, HitArea{
			Rect: r, ZIndex: c.z,
			Handler: &buttonHitAdapter{btn: c.freezeBtn}, Tag: "wave-freeze-btn",
		})
	}
	if r := c.autoBtn.Rect(); !r.Empty() {
		c.hitAreas = append(c.hitAreas, HitArea{
			Rect: r, ZIndex: c.z,
			Handler: &buttonHitAdapter{btn: c.autoBtn}, Tag: "wave-auto-btn",
		})
	}
}

func (c *waveControls) Draw(dst *ebiten.Image) {
	drawPillTabAt(dst, c.autoBtn, c.autoOn)
	drawPillTabAt(dst, c.freezeBtn, c.frozen)
}
```

(`newFreezePill`/`applyFreezeVisual` are in the same file. `layoutRightPill` returns `(btnH, y, rightEdge int)` — we reuse the returned `rightEdge` to place AUTO leftward.)

- [ ] **Step 4: Run the Wave test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveControlsHasBothAutoAndFreeze'`
Expected: PASS.

- [ ] **Step 5: Rewire control construction to per-tab toggles**

In `eq_panel_zone.go`, find where controls are constructed (≈line 344–360). Update:

```go
	z.waveControls = newWaveControls(ctrlZ,
		func() bool { z.waveAutoGain = !z.waveAutoGain; return z.waveAutoGain },
		func() bool { return z.toggleTabFreeze(TabWave) },
	)
	z.levelsControls = newLevelsControls(ctrlZ,
		func() bool { return z.toggleTabFreeze(TabMeters) },
		func() { /* existing Clear-Clips closure — keep as-is */ })
	z.spectrumControls = newSpectrumControls(ctrlZ,
		func() bool { return z.toggleTabFreeze(TabSpectrum) },
		func() { /* existing Reset-Hold closure — keep as-is */ })
```

IMPORTANT: read the actual current `newLevelsControls` / `newSpectrumControls` signatures — they are `newLevelsControls(z int, onFreeze func() bool, onClearClips func())` and `newSpectrumControls(z int, onFreeze func() bool, onResetHold func())`. Replace ONLY the `onFreeze` argument (was `z.callbacks.OnFreezeToggle`) with the per-tab closure; keep the other closures byte-for-byte as they currently are. Do not change those signatures.

- [ ] **Step 6: Sync per-tab frozen in the draw block**

In `eq_panel_zone.go`, the per-tab control draw block (≈line 637) currently computes `frozen` from `state.Capture.Frozen` and calls `c.SyncFreeze(frozen)`. Replace the frozen source with the per-tab flag:

```go
	if c := z.activeTabControls(); c != nil {
		c.SyncFreeze(z.tabFrozen(activeTab))
		if wc, ok := c.(*waveControls); ok {
			wc.SyncAuto(z.waveAutoGain)
		}
		c.Draw(screen)
	}
```

(Remove the now-unused local that read `state.Capture.Frozen` in this block, if it becomes dead. Leave `getAnalyzerStateForTab` calls elsewhere intact.)

- [ ] **Step 7: Run tests + build**

Run:
```
cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveControlsHasBothAutoAndFreeze|TestToggleTabFreezeIsPerTabIndependent|TestApplyFreezeVisualUsesIcons|TestWaveControlsHasAutoNotFreeze'`
```
Note: `TestWaveControlsHasAutoNotFreeze` (from the prior feature) asserted Wave has NO freeze pill — it will now FAIL. **Update it**: Wave now legitimately HAS a freeze pill. Rename/repoint that test to assert the AUTO pill exists and the freeze pill ALSO exists (or delete it in favor of `TestWaveControlsHasBothAutoAndFreeze`). Also fix any other caller of `newWaveControls` (scenes, other tests) to pass the new two-callback signature: `grep -rn 'newWaveControls(' internal/ui/`.
Expected after fixes: PASS.

- [ ] **Step 8: Checkpoint (no commit)**

Vet clean + listed tests green. Leave in tree.

---

## Task 4: Levels visibly holds when frozen

**Files:**
- Modify: `internal/ui/eq_panel_zone.go` (Levels draw block, ≈line 564–612)
- Modify: `internal/ui/freeze_pause_play_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/freeze_pause_play_test.go`:

```go
func TestLevelsLatchHoldsWhenFrozen(t *testing.T) {
	frozenSnap := &analyzer.State{
		Master: analyzer.ChannelMetrics{ID: "main", PeakDB: -6, RMSDB: -12, Active: true},
	}
	z := &EQPanelZone{}
	z.frozenByTab = map[PanelTab]*analyzer.State{TabMeters: frozenSnap}
	z.levelsLatches = NewMultiLevelsLatch()

	// Apply the frozen-hold twice; the held value must equal the frozen peak and
	// not drift (decay) between calls.
	z.applyLevelsFreezeHold(frozenSnap)
	first := z.levelsLatches.Get("main").Peak()
	z.applyLevelsFreezeHold(frozenSnap)
	second := z.levelsLatches.Get("main").Peak()

	if first != second {
		t.Fatalf("frozen latch drifted: first=%v second=%v", first, second)
	}
	if first != -6 {
		t.Fatalf("frozen latch peak=%v, want -6 (held at frozen value)", first)
	}
}
```

VERIFY the latch accessor names before relying on them: read `MultiLevelsLatch` / its per-channel latch in the file that defines `NewMultiLevelsLatch` (search `grep -rn 'func NewMultiLevelsLatch\|func (.*Latch) Get\|Peak()\|func (.*Latch) Update' internal/ui/`). If the held-peak accessor is not `Peak()` or the setter differs, adjust the test AND the implementation in Step 3 to the real API. If there is no direct setter, the implementation may need a `Set(peakDB, rmsDB float64)` method on the latch — add it minimally.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestLevelsLatchHoldsWhenFrozen'`
Expected: FAIL — `z.applyLevelsFreezeHold undefined` (and possibly `Peak()`/`Set` missing).

- [ ] **Step 3: Implement the hold**

Add to `eq_panel_zone.go`:

```go
// applyLevelsFreezeHold pins every Levels latch to the frozen snapshot's
// peak/RMS so the meters hold the paused reading immediately, instead of
// ballistically settling. Called on the frozen branch of the Levels draw.
func (z *EQPanelZone) applyLevelsFreezeHold(state *analyzer.State) {
	if state == nil || z.levelsLatches == nil {
		return
	}
	for i := range state.Instruments {
		inst := &state.Instruments[i]
		z.levelsLatches.Get(inst.ID).Set(inst.PeakDB, inst.RMSDB)
	}
	z.levelsLatches.Get("main").Set(state.Master.PeakDB, state.Master.RMSDB)
}
```

Add the latch `Set(peakDB, rmsDB float64)` method (in the file defining the latch) only if no equivalent exists — it sets the held peak + RMS directly (no decay), matching how `Update` stores them. Then, in the Levels draw block, branch on frozen: when `z.tabFrozen(TabMeters)`, call `z.applyLevelsFreezeHold(state)` INSTEAD of the per-frame `…Latch.Update(...)` loop (skip the live update so it holds). Keep the live `Update` path unchanged when not frozen. Read the existing Levels block (≈line 583–608) and wrap the latch-update loop in `if z.tabFrozen(TabMeters) { z.applyLevelsFreezeHold(state) } else { <existing update loop> }`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestLevelsLatchHoldsWhenFrozen'`
Expected: PASS.

- [ ] **Step 5: Checkpoint (no commit)** — vet clean + test green.

---

## Task 5: Wave FROZEN indicator from tab-frozen + audit global-freeze callers

**Files:**
- Modify: `internal/ui/render_waveform.go` (`drawAnalyzerWaveform` FROZEN tag)
- Modify: `internal/ui/wave_trace_cache.go` (thread `tabFrozen`)
- Modify: `internal/ui/freeze_pause_play_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/ui/freeze_pause_play_test.go`:

```go
func TestWaveFrozenTagFromTabFlag(t *testing.T) {
	t.Cleanup(SetWaveTraceCacheForTest(false))
	ch := &analyzer.ChannelMetrics{Active: true, Waveform: make([]float64, 64)}
	for i := range ch.Waveform {
		ch.Waveform[i] = 0.3
	}
	img := ebiten.NewImage(200, 100)

	// tabFrozen=true must draw the "FROZEN" tag even though capture.Frozen=false.
	rects := collectFilledRects(t, func() {
		drawAnalyzerWaveform(img, img.Bounds(), ch, nil, nil, 1.0, true, true)
	})
	if !anyTextLikeRectNearTopRight(rects, img.Bounds()) {
		// fallback: just assert it does not panic + renders trace; the tag is
		// text (font sprites), so this is a smoke check that the path runs.
	}
	_ = rects
}
```

NOTE: the FROZEN tag is rendered via font sprites, which are hard to assert in the stub. Keep this test as a COMPILE+SMOKE test of the new signature (the meaningful behavior — tag sourced from the flag — is verified by reading the code). Drop `anyTextLikeRectNearTopRight` and simplify to: render with `tabFrozen=true` and with `false`, assert neither panics and both produce `colWaveTrace` rects (reuse `rectsWithColorInside(rects, img.Bounds(), colWaveTrace) > 0`). Write that concrete assertion.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveFrozenTagFromTabFlag'`
Expected: FAIL — `drawAnalyzerWaveform` takes 7 args, not 8.

- [ ] **Step 3: Add a `tabFrozen` param to the renderer**

In `render_waveform.go`, change the signature to add a trailing `tabFrozen bool`:

```go
func drawAnalyzerWaveform(dst *ebiten.Image, rect image.Rectangle, ch *analyzer.ChannelMetrics, capture *analyzer.CaptureBuffer, beatGrid []float64, gain float64, autoOn bool, tabFrozen bool) {
```

In the FROZEN/badge block (currently `if frozen { …FROZEN… } else { …badge… }` where `frozen := capture != nil && capture.Frozen`), change the condition to `if frozen || tabFrozen { …FROZEN… } else { …badge… }`. Keep the sample-source `frozen` (capture-based) selection unchanged.

- [ ] **Step 4: Thread `tabFrozen` through the cache**

In `wave_trace_cache.go`, `drawWaveformCached` calls `drawAnalyzerWaveform(...)` twice (cache-off + cache-rebuild). Compute `tabFrozen := z.tabFrozen(TabWave)` at the top and pass it to both calls. Also fold it into the cache key (add `z.waveKeyTabFrozen bool` field next to `waveKeyGain`, compare + store it) so toggling freeze invalidates the cached frame. Read the current `drawWaveformCached` and make these minimal edits.

- [ ] **Step 5: Update the Wave draw caller**

In `eq_panel_zone.go`, the `TabWave` draw case calls `z.drawWaveformCached(...)` — that function now derives `tabFrozen` itself (Step 4), so no signature change there. Verify `drawWaveformCached`'s exported signature is unchanged (the new flag is internal). If any OTHER caller passes the 7-arg `drawAnalyzerWaveform` directly (grep `drawAnalyzerWaveform(`), add the `tabFrozen` arg (use `false` for non-Wave/non-frozen contexts; tests pass explicit bools).

- [ ] **Step 6: Audit global-freeze callers**

Run these greps and report findings; migrate as needed:
```
cd src/go/internal/ui && grep -rn 'OnFreezeToggle\|AnalyzerService().Freeze\|\.Unfreeze()\|Capture.Frozen\|frozenAnalyzer' --include='*.go'
```
- The analyzer tabs no longer use `OnFreezeToggle` for the global analyzer freeze. If `drumview_ctor.go`'s `OnFreezeToggle` (the analyzer one, ≈line 315) is now unused by the analyzer pills, leave the callback defined (Chain still wires its own; harmless) but note it's dormant for analyzer tabs — OR remove the analyzer-tab wiring if nothing references it. Do NOT touch Chain's `OnFreezeToggle` (scope).
- If a JS export or scene catalog calls a global analyzer freeze/`SetFrozen`, decide: keep it working by routing to `toggleTabFreeze(activeTab)`, or leave it for Chain only. Report each hit and the decision; prefer the smallest change that keeps existing exports/scenes compiling and meaningful.
- The legacy `frozenAnalyzer` single field: if now unreferenced after Task 2, leave it (unused field is harmless) or remove it if trivially safe. Report.

- [ ] **Step 7: Run tests + build**

Run:
```
cd src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestWaveFrozenTagFromTabFlag|Waveform|TestToggleTabFreeze|TestWaveControlsHasBoth|TestApplyFreezeVisual|TestLevelsLatchHolds'
```
Expected: PASS (retry through concurrent churn).

- [ ] **Step 8: Checkpoint (no commit)** — vet clean + tests green.

---

## Task 6: Full verification

**Files:** none (verification only)

- [ ] **Step 1: Scoped Go suite**

Run (retry through concurrent `internal/audio`/`internal/ui` churn from the other agent):
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Freeze|Frozen|Wave|Levels|Spectrum|Chain|AudioPanelTabsRender|TabControls'
```
Expected: PASS. Investigate any failure referencing YOUR symbols; failures only in concurrent files (synth/i18n/settings/audio) are not yours — note them.

- [ ] **Step 2: Token discipline + design drift**

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestTokenDiscipline|TestDesignMDDrift'
```
Expected: PASS — no new color literals (icons reuse `col*`). Fix any literal you introduced.

- [ ] **Step 3: Forbidden-glyph check**

The `||`/`>` text removal means one fewer text-glyph exception. Run any glyph guard:
```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'Glyph|Forbidden'
```
If a test pins the freeze pill as a permitted text-glyph exception, update it (the freeze pill is now an icon, not a glyph). Report.

- [ ] **Step 4: Browser smoke (best-effort)**

```
cd /home/ymolinar/Repos/beatmo && GO=/home/ymolinar/Repos/beatmo/.tools/go/bin/go node src/js/audio_panel_render.browser.test.js
```
Builds WASM from the whole tree (may fail to build due to concurrent churn, or browser toolchain may be unavailable). If it builds+runs, confirm tabs still render. If it fails for environment/concurrent reasons, capture ~15 lines and report "not conclusively run", NOT a task failure.

- [ ] **Step 5: Final checkpoint (no commit)**

Summarize changes + verification output. Leave everything in the working tree — the user owns commits.

---

## Self-Review (author)

- **Spec coverage:** icon (Task 1) ✓; Wave re-adds freeze (Task 3) ✓; per-tab independent freeze (Task 2 store/routing + Task 3 wiring) ✓; Chain icon-only (Task 1) ✓; Levels hold (Task 4) ✓; FROZEN indicator from tab flag (Task 5) ✓; audit global-freeze callers (Task 5 Step 6) ✓; audio untouched (Task 2 never freezes the service; tests assert independence) ✓; tests (each task TDD-first) ✓.
- **Type consistency:** `toggleTabFreeze(tab) bool`, `tabFrozen(tab) bool`, `liveAnalyzerStateForTab(tab)`, `frozenByTab map[PanelTab]*analyzer.State`, `applyLevelsFreezeHold(state)`, `newWaveControls(z, onAuto, onFreeze)`, `applyFreezeVisual(b, frozen)`, `drawAnalyzerWaveform(..., gain, autoOn, tabFrozen)` — used consistently across tasks. `drawWaveformCached` signature unchanged (derives tabFrozen internally).
- **Open verification points flagged inline:** EQCallbacks field names (Task 2 Step 4), `newLevelsControls`/`newSpectrumControls` signatures (Task 3 Step 5), `MultiLevelsLatch` accessor/`Set` API (Task 4 Step 1), other `drawAnalyzerWaveform`/`newWaveControls` callers (Task 3/5), JS/scene freeze callers (Task 5 Step 6). Each says "read the real code; match it; report."
- **No placeholders:** every code step shows code; the one font-sprite-assert weakness (Task 5 Step 1) is explicitly downgraded to a concrete compile+smoke assertion using `rectsWithColorInside`.
