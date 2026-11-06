# BPM Editor Unification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **REPO RULE — NEVER COMMIT:** Never `git add`/commit/stage/stash/restore/reset (subagents too). Every "Checkpoint" runs tests and leaves changes uncommitted. Do NOT include or run git-write commands.

**Goal:** Route BPM editing through the single shared `ParamValueEditor` (tap the BPM readout → popup editor), deleting the bespoke `commitBPMText`/`bpmPrev`/`bpmErrorAnim`/focus path so caret/Enter/Esc/soft-keyboard arrows work on BPM exactly as on EQ — one editing component.

**Architecture:** `TransportZone` gains its own `paramEditor *ParamValueEditor` (mirroring `EQPanelZone`), pumped in `TransportZone.Update`, drawn in `TransportZone.Draw`. `bpmBox` survives as a **display-only** readout (its sprite-cached rendering is reused) — never focused; a tap opens the shared editor over its rect. `valueEditorActive()` is extended to cover the transport editor so the keyboard-yield guard + arrow handling already built for EQ apply to BPM unchanged. The dead `Transport` struct in `transport.go` (only used by `transport_test.go`) is out of scope.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`). Fast path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`. Vet (source of truth; ignore gopls phantom diagnostics): `../../.tools/go/bin/go vet -tags test ./internal/ui/`. Go binary: `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`.

> **Spec refinement note:** The approved spec said "delete `bpmBox`". This plan keeps `bpmBox` as a *non-interactive display readout* (it already renders the value through the transport sprite cache) and deletes only the *editing* path. This achieves the same goal — one editing component, tap-to-popup — with materially lower risk (no sprite-cache rendering rewrite) and smaller test churn (display reads of `bpmBox().Value()`/`.Rect` survive; only editing-specific refs are rewritten).

---

## File Structure

- **`transport_zone.go`** (modify) — add `paramEditor`, `bpmSpec`, `openBPMEditor`, `bpmOpenAdapter`; make `bpmBox` display-only; pump+draw the editor; delete `commitBPMText`/`forceBlurBPM`/`bpmErrorAnim` field/`bpmPrev` + the per-frame mobile `bpm` register/poll.
- **`game_input_shortcuts.go`** (modify) — extend `valueEditorActive()` with the transport editor.
- **`drumview_layout.go`** (modify) — remove the per-frame `mobileInputRegister("bpm", …)`.
- **`drumview_toolbar.go`, `drumview_update.go`** (modify) — repoint `BPMErrorAnim()` plumbing to the editor's flash.
- **Tests** — `transport_zone_test.go`, `bpm_ui_test.go`, `bpm_editor_validation_test.go`, `bpm_stepper_mobile_test.go`, `drumview_update_test.go`, `focus_ring_blur_on_resize_test.go`, `input_isolation_test.go`, `transport_input_isolation_test.go`, `drumview_test.go`, plus a new `bpm_editor_functional_test.go`.

`transport.go` / `transport_test.go` (the dead `Transport`) are NOT touched.

---

## Task 1: Add the transport-owned shared editor (additive)

**Files:** Modify `transport_zone.go`, `game_input_shortcuts.go`; Test `bpm_editor_functional_test.go` (create).

- [ ] **Step 1: Write the failing functional test**

Create `src/go/internal/ui/bpm_editor_functional_test.go`:

```go
//go:build test

package ui

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// Opens the BPM editor via the production open path and drives arrow keys
// through the REAL Game.Update loop (the fakeInput harness lives in
// text_input_escape_functional_test.go).
func TestFunctional_ArrowKeys_BPMEditor(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	tz := g.drum.transportZone
	if tz == nil {
		t.Skip("no transport zone")
	}
	tz.openBPMEditor()
	if tz.paramEditor == nil || !tz.paramEditor.Active() {
		t.Fatalf("openBPMEditor did not open the editor")
	}
	start := tz.paramEditor.ti.cursor
	if start == 0 {
		t.Fatalf("precondition: prefilled caret at end (>0), got %d", start)
	}
	cam := g.cam.OffsetX

	fi.pressKey(t, g, ebiten.KeyLeft)
	if g.cam.OffsetX != cam {
		t.Errorf("Left arrow panned grid while editing BPM (%v -> %v)", cam, g.cam.OffsetX)
	}
	if !tz.paramEditor.Active() {
		t.Fatalf("Left arrow closed the BPM editor")
	}
	if tz.paramEditor.ti.cursor != start-1 {
		t.Fatalf("Left arrow did NOT move caret: cursor=%d want %d", tz.paramEditor.ti.cursor, start-1)
	}
}
```

- [ ] **Step 2: Run — verify it fails**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestFunctional_ArrowKeys_BPMEditor`
Expected: FAIL — `tz.openBPMEditor`/`tz.paramEditor` undefined (build error).

- [ ] **Step 3: Add the editor field + spec + open + adapter to `transport_zone.go`**

In the `TransportZone` struct (near `bpmBox *TextInput`, ~line 60) add:
```go
	paramEditor *ParamValueEditor // shared numeric editor for BPM (tap-to-popup)
```

Where `bpmBox` is constructed (`NewTextInput(... BPMBoxStyle)`, ~line 230) add right after:
```go
	z.paramEditor = NewParamValueEditor()
```

Add these methods (place near `commitBPMText`, ~line 1164):
```go
// bpmSpec drives the shared editor for the BPM value: integer in [1,maxBPM],
// rejecting invalid input with the same "Invalid BPM" toast as the legacy path.
func (z *TransportZone) bpmSpec() ValueSpec {
	return ValueSpec{
		Format: func(v float64) string { return strconv.Itoa(int(v)) },
		Parse: func(s string) (float64, bool) {
			if v, ok := parseBPM(strings.TrimSpace(s)); ok {
				return float64(v), true
			}
			if z.callbacks.OnNotifyError != nil {
				z.callbacks.OnNotifyError("Invalid BPM")
			}
			return 0, false
		},
		MinW:   60,
		Style:  BPMBoxStyle,
		MaxLen: 4,
	}
}

// openBPMEditor opens the shared editor over the BPM readout, pre-filled with
// the current tempo. Commit clamps via bpmSpec and writes through SetBPM.
func (z *TransportZone) openBPMEditor() {
	if z.paramEditor == nil {
		z.paramEditor = NewParamValueEditor()
	}
	anchor := z.bpmBox.Rect
	z.paramEditor.OpenValue(ValueOpen{
		Spec:          z.bpmSpec(),
		Anchor:        anchor,
		Clamp:         z.rect,
		MobileInputID: "bpm",
		Get:           func() float64 { return float64(z.bpm) },
		Set:           func(v float64) { z.SetBPM(int(v)) },
	})
}

// bpmOpenAdapter opens the shared BPM editor on press.
type bpmOpenAdapter struct{ z *TransportZone }

func (a *bpmOpenAdapter) OnPress(x, y int) InputResult {
	a.z.openBPMEditor()
	return InputConsumed
}
func (a *bpmOpenAdapter) OnDrag(x, y int)                     {}
func (a *bpmOpenAdapter) OnRelease(x, y int)                  {}
func (a *bpmOpenAdapter) OnWheel(x, y, steps int) InputResult { return InputIgnored }
```
Confirm `z.rect` is the TransportZone's bounds field (grep `z.rect` in the file); if the field is named differently, use the real name.

In `TransportZone.Update` (the method that pumps the BPM box, ~line 438) — leave the existing bpmBox logic for now (Task 2 removes it) but ADD, at the very top after the `z.bpmBox == nil` guard:
```go
	if z.paramEditor != nil {
		z.paramEditor.Update()
	}
```

In `TransportZone.Draw` (the direct path, ~line 1528, after `z.bpmBox.Draw(dst)` and its flash block) add:
```go
	if z.paramEditor != nil {
		z.paramEditor.Draw(dst)
	}
```

- [ ] **Step 4: Extend `valueEditorActive()`**

In `game_input_shortcuts.go`, in `(*DrumView).valueEditorActive()` add a third check:
```go
	if dv.transportZone != nil && dv.transportZone.paramEditor != nil && dv.transportZone.paramEditor.Active() {
		return true
	}
```

- [ ] **Step 5: Run — verify pass**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestFunctional_ArrowKeys_BPMEditor|BPM|Transport' -count=1`
Expected: PASS (new test + existing BPM/transport tests still pass — bpmBox editing path untouched). Then `../../.tools/go/bin/go vet -tags test ./internal/ui/` → clean.

- [ ] **Step 6: Checkpoint (no commit).**

---

## Task 2: Flip BPM to the editor, delete the bespoke path, rewrite tests (atomic)

**Files:** Modify `transport_zone.go`, `drumview_layout.go`, `drumview_toolbar.go`, `drumview_update.go`; rewrite test refs across the test files listed in File Structure.

This is atomic: deleting `commitBPMText`/`bpmErrorAnim`/`bpmPrev` breaks every test referencing them, so source + tests change together (the package's tests compile as one unit). Mirrors the EQ 4b migration.

- [ ] **Step 1: Write the failing functional tests (Enter/Esc + soft-keyboard arrow)**

Append to `bpm_editor_functional_test.go`:
```go
func TestFunctional_BPMEditor_EnterCommits(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	tz := g.drum.transportZone
	if tz == nil {
		t.Skip("no transport zone")
	}
	box := g.drum.bpmBox()
	if box == nil || box.Rect.Empty() {
		t.Skip("no bpm readout rect")
	}
	r := box.Rect
	fi.clickAt(t, g, (r.Min.X+r.Max.X)/2, (r.Min.Y+r.Max.Y)/2)
	if !tz.paramEditor.Active() {
		t.Fatalf("tap did not open BPM editor")
	}
	tz.paramEditor.ti.SetText("155")
	fi.pressKey(t, g, ebiten.KeyEnter)
	if g.drum.BPM() != 155 {
		t.Fatalf("Enter should commit BPM 155, got %d", g.drum.BPM())
	}
}

func TestFunctional_BPMEditor_EscReverts(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	tz := g.drum.transportZone
	if tz == nil {
		t.Skip("no transport zone")
	}
	orig := g.drum.BPM()
	tz.openBPMEditor()
	tz.paramEditor.ti.SetText("999")
	fi.pressKey(t, g, ebiten.KeyEscape)
	if tz.paramEditor.Active() {
		t.Fatalf("Esc did not close the BPM editor")
	}
	if g.drum.BPM() != orig {
		t.Fatalf("Esc must not write: BPM=%d want %d", g.drum.BPM(), orig)
	}
}

func TestFunctional_SoftKeyboardArrow_BPMEditor(t *testing.T) {
	g := newDesktopGame(t)
	fi := newFakeInput(1280, 720)
	restore := installFakeInput(fi)
	defer restore()
	for i := 0; i < 3; i++ {
		fi.frame(t, g)
	}
	tz := g.drum.transportZone
	if tz == nil {
		t.Skip("no transport zone")
	}
	tz.openBPMEditor()
	start := tz.paramEditor.ti.cursor
	if start == 0 {
		t.Fatalf("precondition caret at end")
	}
	// WASM path: only the forwarded signal, no canvas key.
	SignalSoftKeyboardArrowLeft()
	fi.frame(t, g)
	if tz.paramEditor.ti.cursor != start-1 {
		t.Fatalf("soft-keyboard Left did not move BPM caret: %d want %d", tz.paramEditor.ti.cursor, start-1)
	}
}
```

- [ ] **Step 2: Run — verify fail**

Run: `cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'TestFunctional_BPMEditor_EnterCommits|TestFunctional_BPMEditor_EscReverts|TestFunctional_SoftKeyboardArrow_BPMEditor'`
Expected: FAIL — tapping the box still focuses the legacy `bpmBox` (no editor) / mismatches. (If they happen to pass because Task 1 left the editor pumped, they still meaningfully gate Step 3's flip; proceed.)

- [ ] **Step 3: Make `bpmBox` display-only + wire the tap to the editor**

In `transport_zone.go` `TransportZone.Update` (~line 438–494): delete the legacy BPM editing body — the `mobileBPMActive` poll block (443–461), the `z.bpmBox.Update()` call + Enter/focus-gain/blur handling (473–494). Keep the `z.paramEditor.Update()` call added in Task 1, and keep syncing the readout text:
```go
	if z.bpmBox == nil {
		return
	}
	if z.paramEditor != nil {
		z.paramEditor.Update()
	}
	// Keep the display readout text in sync with the live tempo (the box is
	// never focused now — editing happens in the shared editor on tap).
	if !z.paramEditor.Active() {
		z.bpmBox.SetText(strconv.Itoa(z.bpm))
	}
```

In `rebuildHitAreas` (~line 1014) replace the BPM hit area handler:
```go
		Handler: &bpmOpenAdapter{z: z},
```
(was `&textInputHitAdapter{ti: z.bpmBox, consume: true}`) — keep the same `Rect: z.bpmBox.Rect`, `ZIndex: zIdx + 1`, `Tag: "transport-bpm-box"`.

- [ ] **Step 4: Delete the bespoke editing symbols + the error-flash field**

In `transport_zone.go` delete: `commitBPMText`, `forceBlurBPM`, the `bpmPrev` field, and the `bpmErrorAnim` field. Remove the two `if z.bpmErrorAnim > 0 { … }` flash blocks at the cached draw (~1472–1475) and direct draw (~1529–1531) — the editor's own `errorAnim` flash (drawn by `z.paramEditor.Draw`) replaces them. Replace `BPMErrorAnim()` (~line 663) with:
```go
func (z *TransportZone) BPMErrorAnim() float64 {
	if z.paramEditor != nil {
		return z.paramEditor.errorAnim
	}
	return 0
}
```
Grep `bpmErrorAnim`, `bpmPrev`, `commitBPMText`, `forceBlurBPM` in `transport_zone.go` → zero remaining.

- [ ] **Step 5: Remove the per-frame mobile `bpm` registration**

In `drumview_layout.go` (~line 866–870) delete the `mobileInputRegister("bpm", …)` block — the editor registers the native input on open (`MobileInputID: "bpm"` in `openBPMEditor`).

- [ ] **Step 6: Fix the `bpmErrorAnim` plumbing in `drumview_toolbar.go` + `drumview_update.go`**

`drumview_toolbar.go` (~line 19–30) reads/writes `dv.transportZone.bpmErrorAnim` directly (now deleted). Replace those reads/writes with `dv.transportZone.BPMErrorAnim()` (read-only). Read the block; the intent is "propagate the higher of dv.bpmErrorAnim and the zone's flash". Since the zone flash is now editor-owned (read-only here), keep `dv.bpmErrorAnim` as the toolbar's own dec,aying mirror seeded from `BPMErrorAnim()`:
```go
	if z := dv.transportZone.BPMErrorAnim(); z > dv.bpmErrorAnim {
		dv.bpmErrorAnim = z
	}
	decay(&dv.bpmErrorAnim)
```
`drumview_update.go:62` already uses `dv.transportZone.BPMErrorAnim()` — no change.

- [ ] **Step 7: Rewrite test references (editing-specific only)**

Display reads survive (`bpmBox().Value()`, `bpmBox().Rect`). Rewrite only editing refs. Old→new mapping (read each test, preserve intent):
- `commitBPMText()` (after setting text) → `openBPMEditor(); paramEditor.ti.SetText(x); paramEditor.commit()`.
- `bpmBox().Focused()` as an *editing* signal → `paramEditor.Active()`.
- `bpmPrev` save/assert → editor revert: open, SetText, `paramEditor.cancel()`, assert `BPM()` unchanged.
- `bpmErrorAnim` field reads → `BPMErrorAnim()`.
- Focus-gain-clears / focus-gain-prefill tests → open the editor and assert `paramEditor.ti.Value()` is the prefilled tempo (the readout box is no longer focusable).
- Tests that click the BPM box expecting `bpmBox().Focused()==true` → assert `paramEditor.Active()==true` instead.
Files to sweep (counts are total bpm-symbol refs, most are display reads that stay): `transport_zone_test.go` (55), `bpm_ui_test.go` (10), `bpm_editor_validation_test.go` (6), `bpm_stepper_mobile_test.go`, `drumview_update_test.go` (20), `focus_ring_blur_on_resize_test.go` (12), `input_isolation_test.go` (8), `transport_input_isolation_test.go`, `drumview_test.go` (8). If a test's sole purpose was the deleted focus-clear mechanism, convert it to the editor-prefill equivalent (don't drop the assertion). Do NOT touch `transport_test.go` (the dead `Transport`).

- [ ] **Step 8: Verify**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go vet -tags test ./internal/ui/
../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run 'BPM|Transport|bpm|Esc|ArrowKeys|SoftKeyboard|FocusRing|InputIsolation' -count=1
grep -rn "commitBPMText\|forceBlurBPM\|bpmPrev\|\.bpmErrorAnim" internal/ui/ | grep -v transport_test.go
```
Expected: vet exit 0; tests PASS; grep finds zero references (outside the dead `transport_test.go`/`transport.go` `Transport`). Then verify WASM still compiles: `GOOS=js GOARCH=wasm /home/ymolinar/Repos/beatmo/.tools/go/bin/go build -o /dev/null ./internal/ui/`.

Pre-existing flaky note (per project memory): `TestRowEQDivider_GlowsWhenCursorProbeDisabled` and `TestSynthKnobDrawsConceptInk` fail under batch ordering / external churn and are unrelated — confirm any failure is one of these (passes in isolation, untouched file) before treating it as a regression.

- [ ] **Step 9: Checkpoint (no commit).**

---

## Task 3: Geometry guard + final sweep

**Files:** Test `transport_zone_test.go` (add one); verification only otherwise.

- [ ] **Step 1: Write a layout guard test**

The readout must keep the exact rect the box had, so the `±` group outline doesn't shift. Add to `transport_zone_test.go`:
```go
func TestBPMReadoutRectMatchesGroupOutline(t *testing.T) {
	z, _ := newTestTransportZone()
	registerTransportZone(z, image.Rect(10, 10, 500, 90))
	r := z.bpmBox.Rect
	if r.Empty() {
		t.Fatalf("bpm readout rect empty after layout")
	}
	// The group outline must still be computed from the readout rect.
	if z.bpmGroupRect.Empty() || !r.Overlaps(z.bpmGroupRect) {
		t.Fatalf("bpm group outline (%v) no longer encloses the readout (%v)", z.bpmGroupRect, r)
	}
}
```
(Mirror the real `newTestTransportZone`/`registerTransportZone` helper signatures in that file.)

- [ ] **Step 2: Run it** — `... -run TestBPMReadoutRectMatchesGroupOutline` → PASS (geometry unchanged since `bpmBox.Rect` assignment sites are untouched).

- [ ] **Step 3: Full relevant sweep + discipline guards**

Run:
```bash
cd /home/ymolinar/Repos/beatmo/src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ \
  -run 'BPM|Transport|bpm|ArrowKeys|SoftKeyboard|Esc|ParamValueEditor|TestDesignMDDrift|TestTokenDiscipline|ZoneInput|ActionKeysIgnored|ArrowKeysPan' -count=1
```
Expected: PASS (keyboard-yield guards `TestArrowKeysPan`/`TestActionKeysIgnoredWhileTextInputFocused` still pass; zone isolation intact).

- [ ] **Step 4: Report** — files touched, the deleted symbols (`commitBPMText`/`forceBlurBPM`/`bpmPrev`/`bpmErrorAnim`), test outcomes; leave uncommitted.

---

## Self-Review Notes (author)

- **Spec coverage:** readout+tap-to-popup → Task 1/2; `bpmSpec`+`OnNotifyError` → Task 1; deletions (`commitBPMText`/`bpmPrev`/`bpmErrorAnim`/`forceBlurBPM`/per-frame mobile register) → Task 2; `±` stepper untouched → by omission; geometry preserved → Task 3; functional arrow/Enter/Esc/soft-keyboard tests → Tasks 1–2; `valueEditorActive` covers BPM → Task 1 Step 4.
- **Spec deviation (documented):** `bpmBox` kept as display-only readout instead of deleted — lower-risk, same goal; called out in the header note.
- **Type consistency:** `paramEditor`, `bpmSpec`, `openBPMEditor`, `bpmOpenAdapter`, `BPMErrorAnim()` used consistently; `HitHandler` method set matches `eqDBOpenAdapter` (incl. `OnWheel`).
- **Read-first unknowns (flagged inline):** exact `z.rect` field name, `newTestTransportZone`/`registerTransportZone` signatures, the precise line ranges of the legacy BPM update body (shift as the file changes).
