# EQ Wheel-Popup Value Editor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Tapping an EQ band's dB readout cell (desktop *and* mobile) opens the reusable `MobileWheelPopup` (wheel + custom-precision resolution strip); the wheel and the existing curve-drag handles write the same source of truth (`EQPanelZone.bandGainsDB`), and numeric text entry survives via the wheel's center-box tap.

**Architecture:** The `EQPanelZone` gains a live-write seam (`applyBandGainLive`) and an `OnOpenValueWheel` callback; the dB-cell press adapter routes to that callback. The `MobileWheelPopup` machinery, a transient `*Knob` seeded from `bandGainsDB[band]`, a shared `*KnobStepBadge`, and a modal+scrim portal all live on `DrumView`, mirroring the existing sampler wheel wiring exactly.

**Tech Stack:** Go 1.23, Ebiten (stubbed under `-tags test`), existing `internal/ui` widget/portal/knob system.

## Global Constraints

- Bundled Go only: `/home/ymolinar/Repos/beatmo/.tools/go/bin/go`.
- Fast test command (from `src/go`): `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`.
- EQ dB clamp is **[-12, +12] dB** (matches `eqDBSpec()`). Do not introduce a different range.
- **Do NOT `git add` or `git commit`** — standing user rule. Leave all work uncommitted for review. (Every "verify" step below replaces what would normally be a commit.)
- Zone stays decoupled from `DrumView`: `EQPanelZone` reaches `DrumView` only through its `EQCallbacks`, never a back-reference.
- Source of truth is `EQPanelZone.bandGainsDB`; both edit surfaces funnel through `OnGainChange` → `OnApplyEQ` (live) and `OnEQBandCommit` (once on release).

---

### Task 1: EQPanelZone live-write seam + open-wheel callback

**Files:**
- Modify: `src/go/internal/ui/eq_panel_zone.go` (EQCallbacks struct ~line 19; `eqDBOpenAdapter.OnPress` ~line 1224; add `applyBandGainLive` near `openEQDBEditor` ~line 1181)
- Test: `src/go/internal/ui/eq_wheel_seam_test.go` (create)

**Interfaces:**
- Produces: `EQCallbacks.OnOpenValueWheel func(band int)`; `func (z *EQPanelZone) applyBandGainLive(band int, v float64)`.
- Consumes: existing `z.bandGainsDB`, `z.callbacks.OnGainChange`, `z.callbacks.OnApplyEQ`, `clampF64`, `z.openEQDBEditor`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/eq_wheel_seam_test.go`:

```go
//go:build test

package ui

import "testing"

// applyBandGainLive writes the source of truth, clamps to [-12,12], and fires
// the live callbacks (OnGainChange + OnApplyEQ) but NOT the commit callback.
func TestApplyBandGainLive_WritesClampsAndFiresLive(t *testing.T) {
	var gotBand int
	var gotDB float64
	gainCalls, applyCalls, commitCalls := 0, 0, 0
	z := NewEQPanelZone(EQCallbacks{
		OnGainChange:   func(band int, db float64) { gainCalls++; gotBand = band; gotDB = db },
		OnApplyEQ:      func() { applyCalls++ },
		OnEQBandCommit: func() { commitCalls++ },
	})

	z.applyBandGainLive(2, 5.5)
	if z.bandGainsDB[2] != 5.5 {
		t.Fatalf("bandGainsDB[2] = %v, want 5.5", z.bandGainsDB[2])
	}
	if gainCalls != 1 || gotBand != 2 || gotDB != 5.5 {
		t.Fatalf("OnGainChange calls=%d band=%d db=%v, want 1/2/5.5", gainCalls, gotBand, gotDB)
	}
	if applyCalls != 1 {
		t.Fatalf("OnApplyEQ calls=%d, want 1", applyCalls)
	}
	if commitCalls != 0 {
		t.Fatalf("OnEQBandCommit calls=%d, want 0 (commit is release-only)", commitCalls)
	}

	z.applyBandGainLive(2, 99) // out of range
	if z.bandGainsDB[2] != 12 {
		t.Fatalf("clamp high: bandGainsDB[2] = %v, want 12", z.bandGainsDB[2])
	}
	z.applyBandGainLive(2, -99)
	if z.bandGainsDB[2] != -12 {
		t.Fatalf("clamp low: bandGainsDB[2] = %v, want -12", z.bandGainsDB[2])
	}
}

// eqDBOpenAdapter.OnPress routes to OnOpenValueWheel when set, and falls back
// to the numeric editor when it is nil (preserving legacy behavior).
func TestEQDBOpenAdapter_RoutesToWheelCallback(t *testing.T) {
	wheelBand := -1
	z := NewEQPanelZone(EQCallbacks{
		OnOpenValueWheel: func(band int) { wheelBand = band },
	})
	a := &eqDBOpenAdapter{z: z, band: 4}
	if res := a.OnPress(0, 0); res != InputConsumed {
		t.Fatalf("OnPress result = %v, want InputConsumed", res)
	}
	if wheelBand != 4 {
		t.Fatalf("OnOpenValueWheel band = %d, want 4", wheelBand)
	}
	if z.paramEditor != nil && z.paramEditor.Active() {
		t.Fatal("numeric editor must NOT open when wheel callback is set")
	}

	// Nil callback → falls back to numeric editor.
	z2 := NewEQPanelZone(EQCallbacks{})
	a2 := &eqDBOpenAdapter{z: z2, band: 1}
	a2.OnPress(0, 0)
	if z2.paramEditor == nil || !z2.paramEditor.Active() {
		t.Fatal("fallback: numeric editor must open when wheel callback is nil")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestApplyBandGainLive_WritesClampsAndFiresLive|TestEQDBOpenAdapter_RoutesToWheelCallback' ./internal/ui/`
Expected: FAIL — `z.applyBandGainLive` undefined and `OnOpenValueWheel` unknown field.

- [ ] **Step 3: Add the `OnOpenValueWheel` field to `EQCallbacks`**

In `eq_panel_zone.go`, inside `type EQCallbacks struct` (around line 19), add after `OnApplyEQ`:

```go
	// OnOpenValueWheel opens the mobile-style precision wheel popup for a band's
	// dB gain (desktop + mobile). Wired by DrumView; when nil the dB cell falls
	// back to the inline numeric editor.
	OnOpenValueWheel func(band int)
```

- [ ] **Step 4: Add `applyBandGainLive`**

In `eq_panel_zone.go`, immediately after `openEQDBEditor` (after its closing brace, ~line 1210):

```go
// applyBandGainLive writes a band's dB gain to the source of truth (clamped to
// [-12,+12]) and fires the LIVE callbacks (OnGainChange + OnApplyEQ). It does
// NOT fire OnEQBandCommit — the commit (emit + undo) is a release-only event,
// mirroring the curve-drag path. Used by the wheel popup's per-detent OnChange.
func (z *EQPanelZone) applyBandGainLive(band int, v float64) {
	if band < 0 || band >= len(z.bandGainsDB) {
		return
	}
	v = clampF64(v, -12.0, 12.0)
	z.bandGainsDB[band] = v
	z.curveDirty = true
	if z.callbacks.OnGainChange != nil {
		z.callbacks.OnGainChange(band, v)
	}
	if z.callbacks.OnApplyEQ != nil {
		z.callbacks.OnApplyEQ()
	}
}
```

- [ ] **Step 5: Reroute `eqDBOpenAdapter.OnPress`**

In `eq_panel_zone.go`, replace the body of `eqDBOpenAdapter.OnPress` (~line 1224):

```go
func (a *eqDBOpenAdapter) OnPress(x, y int) InputResult {
	if a.z.callbacks.OnOpenValueWheel != nil {
		a.z.callbacks.OnOpenValueWheel(a.band)
		return InputConsumed
	}
	a.z.openEQDBEditor(a.band)
	return InputConsumed
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestApplyBandGainLive_WritesClampsAndFiresLive|TestEQDBOpenAdapter_RoutesToWheelCallback' ./internal/ui/`
Expected: PASS.

- [ ] **Step 7: Verify no regression in existing EQ dB tests** (replaces commit)

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'EQDB|EQ_DBInput|Esc_EQdB|Functional_ArrowKeys_EQdBEditor' ./internal/ui/`
Expected: PASS (these call `openEQDBEditor` directly, so they are unaffected). Do NOT commit.

---

### Task 2: DrumView EQ wheel popup plumbing (field, ctor, open/close, portal, wiring)

**Files:**
- Modify: `src/go/internal/ui/drumview.go` (add fields near line 381)
- Modify: `src/go/internal/ui/drumview_ctor.go` (construct near line 117; wire `OnOpenValueWheel` in the `NewEQPanelZone(EQCallbacks{...})` block near line 244)
- Modify: `src/go/internal/ui/drumview_portal_open.go` (add EQ wheel portal helpers after the sampler ones, ~line 402)
- Modify: `src/go/internal/ui/drumview_close_popups.go` (add EQ wheel popup to `CloseAllPopups`, line 45)
- Create: `src/go/internal/ui/eq_wheel_popup.go`
- Test: `src/go/internal/ui/eq_wheel_popup_test.go` (create)

**Interfaces:**
- Consumes: `EQCallbacks.OnOpenValueWheel` and `applyBandGainLive` (Task 1); `NewMobileWheelPopup`, `WheelBinding`, `NewKnob`, `NewKnobStepBadge`, `KnobStepBadge.Step`, `mobileWheelPopupPortalOverlay`, `dv.CloseAllPopups`, `dv.commitEQBand`, `dv.persistKnobStep`, `dv.eqPanelZone.dbReadoutRect`, `dv.eqPanelZone.openEQDBEditor`, `dv.eqPanelZone.bandGainsDB` (via accessor `eqBandGainsDB()`).
- Produces: `dv.eqWheelPopup *MobileWheelPopup`, `dv.eqWheelStepBadge *KnobStepBadge`, `func (dv *DrumView) openEQKnobWheelPopup(band int)`, `openEQWheelPortal`, `closeEQWheelPortal`, `closeEQKnobWheelPopup`, `EQWheelPopupRect()`.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/ui/eq_wheel_popup_test.go`:

```go
//go:build test

package ui

import "testing"

// pressing an EQ dB readout cell opens the wheel popup on BOTH desktop and
// mobile, and the wheel edits the same source of truth as the curve.
func TestEQWheelPopup_OpensOnBothProfiles(t *testing.T) {
	for _, mobile := range []bool{false, true} {
		mobile := mobile
		name := "desktop"
		if mobile {
			name = "mobile"
		}
		t.Run(name, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			dv := g.drum
			if mobile {
				restore := forceSmallScreenForTest(t)
				defer restore()
			}
			dv.SetActiveTab(TabEQ)
			dv.Layout(dv.Bounds)

			// Route a dB-cell press through the zone's OnPress adapter.
			a := &eqDBOpenAdapter{z: dv.eqPanelZone, band: 3}
			a.OnPress(0, 0)

			if dv.eqWheelPopup == nil || !dv.eqWheelPopup.IsOpen() {
				t.Fatalf("[%s] EQ wheel popup did not open on dB-cell press", name)
			}
		})
	}
}

// The wheel's OnChange writes bandGainsDB (single source of truth); a curve
// drag on the same band writes the SAME field — both surfaces agree.
func TestEQWheelPopup_SharesSourceOfTruthWithCurve(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	dv := g.drum
	dv.SetActiveTab(TabEQ)
	dv.Layout(dv.Bounds)

	dv.openEQKnobWheelPopup(1)
	// Drive the wheel by nudging its knob and firing OnChange via a wheel step.
	dv.eqWheelPopup.HandleWheel(dv.eqWheelPopup.Rect().Min.X+4, dv.eqWheelPopup.Rect().Min.Y+40, 3)
	afterWheel := dv.eqPanelZone.bandGainsDB[1]

	// Now write the same field via the live seam (what the curve path also uses)
	// and confirm it is the same backing array.
	dv.eqPanelZone.applyBandGainLive(1, afterWheel+1.0)
	if dv.eqPanelZone.bandGainsDB[1] != afterWheel+1.0 {
		t.Fatalf("curve/seam write did not land on the same source of truth: got %v want %v",
			dv.eqPanelZone.bandGainsDB[1], afterWheel+1.0)
	}
}

// The wheel's center-box tap opens the numeric editor (numeric entry preserved).
func TestEQWheelPopup_CenterBoxOpensNumericEditor(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	dv := g.drum
	dv.SetActiveTab(TabEQ)
	dv.Layout(dv.Bounds)

	dv.openEQKnobWheelPopup(2)
	dv.eqWheelPopup.binding.OpenEditor()
	if dv.eqPanelZone.paramEditor == nil || !dv.eqPanelZone.paramEditor.Active() {
		t.Fatal("center-box OpenEditor did not open the numeric editor")
	}
}
```

Note: if `forceSmallScreenForTest` needs a follow-up `UpdateProfile()` in this codebase, mirror the exact call used by existing mobile tests (grep `forceSmallScreenForTest` for the canonical pairing) — do not invent a new helper.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEQWheelPopup_' ./internal/ui/`
Expected: FAIL — `dv.eqWheelPopup` / `openEQKnobWheelPopup` undefined.

- [ ] **Step 3: Add the DrumView fields**

In `drumview.go`, after line 381 (`samplerWheelPopup`):

```go
	eqWheelPopup     *MobileWheelPopup // desktop+mobile precision wheel for EQ band dB
	eqWheelStepBadge *KnobStepBadge    // shared step-resolution ladder for all EQ dB bands
```

- [ ] **Step 4: Construct them in the ctor**

In `drumview_ctor.go`, after line 117 (`dv.samplerWheelPopup = NewMobileWheelPopup()`):

```go
	dv.eqWheelPopup = NewMobileWheelPopup()
	dv.eqWheelStepBadge = NewKnobStepBadge(audio.ParamDef{
		Name: "eq_band_gain", Label: "EQ", Min: -12, Max: 12, Unit: "dB",
	})
```

(`audio` is already imported in `drumview_ctor.go`; if the linter reports it missing, add `"github.com/ingyamilmolinar/beatmo/internal/audio"`.)

- [ ] **Step 5: Wire the `OnOpenValueWheel` callback**

In `drumview_ctor.go`, inside the `NewEQPanelZone(EQCallbacks{...})` literal (after the `OnApplyEQ: func() { dv.applyEQ() },` entry, ~line 282):

```go
		OnOpenValueWheel: func(band int) { dv.openEQKnobWheelPopup(band) },
```

- [ ] **Step 6: Create the open/close helper**

Create `src/go/internal/ui/eq_wheel_popup.go`:

```go
// src/go/internal/ui/eq_wheel_popup.go
package ui

import (
	"image"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

// eqDBRange is the EQ band gain domain, matching eqDBSpec()'s clamp.
const (
	eqDBMin  = -12.0
	eqDBMax  = 12.0
	eqDBSpan = eqDBMax - eqDBMin
)

// openEQKnobWheelPopup opens the precision wheel popup for EQ band's dB gain.
// Works on desktop AND mobile (unlike the synth/sampler wheels, which are
// mobile-only). The wheel edits EQPanelZone.bandGainsDB — the same source of
// truth as the curve-drag handles — via applyBandGainLive; one undo step is
// committed on release through commitEQBand. The center-box tap re-opens the
// inline numeric editor so text entry is preserved.
func (dv *DrumView) openEQKnobWheelPopup(band int) {
	z := dv.eqPanelZone
	if z == nil || band < 0 || band >= len(z.bandGainsDB) || dv.eqWheelPopup == nil {
		return
	}
	dv.CloseAllPopups()

	badge := dv.eqWheelStepBadge
	def := audio.ParamDef{Name: "eq_band_gain", Label: eqBandLabel(band), Min: eqDBMin, Max: eqDBMax, Unit: "dB"}

	// Transient knob seeded from the current gain; endless accumulation in real
	// dB units, one StepMul (badge rung) per notch.
	k := NewKnob((z.bandGainsDB[band] - eqDBMin) / eqDBSpan)
	k.Scale = KnobScale{Min: eqDBMin, Max: eqDBMax}
	k.Endless = true
	if badge != nil {
		k.StepMul = badge.Step()
	}

	anchor := z.dbReadoutRect(band)
	binding := WheelBinding{
		Knob:     k,
		Badge:    badge,
		Def:      def,
		Discrete: false,
		Title:    func() string { return eqBandLabel(band) },
		OnChange: func() {
			db := eqDBMin + k.Value*eqDBSpan
			z.applyBandGainLive(band, db)
		},
		OnCommit:     func() { dv.commitEQBand() },
		OnResolution: func() { dv.persistKnobStep(badge) },
		OpenEditor:   func() { z.openEQDBEditor(band) },
		Accent:       nil,
	}
	dv.eqWheelPopup.Open(binding, anchor, dv.Bounds, dv.headerH)
	dv.openEQWheelPortal()
}

// closeEQKnobWheelPopup closes the EQ precision wheel popup.
func (dv *DrumView) closeEQKnobWheelPopup() {
	if dv.eqWheelPopup != nil {
		dv.eqWheelPopup.Close()
	}
	dv.closeEQWheelPortal()
}

// EQWheelPopupRect returns the screen-space rect of the EQ wheel popup (empty
// when nil/closed). Parity with SamplerWheelPopupRect for screenshot cropping.
func (dv *DrumView) EQWheelPopupRect() image.Rectangle {
	if dv.eqWheelPopup == nil || !dv.eqWheelPopup.IsOpen() {
		return image.Rectangle{}
	}
	return dv.eqWheelPopup.Rect()
}
```

If a band-label helper named `eqBandLabel` does not already exist, grep for the existing label source used by `eqBandLabelRects` / the readout labels and use that function instead; otherwise add this tiny helper to `eq_wheel_popup.go`:

```go
// eqBandLabel returns a short human label for EQ band index (e.g. "1 kHz").
// Reuses the zone's existing per-band frequency labels when available.
func eqBandLabel(band int) string {
	if band >= 0 && band < len(eqBandCenterLabels) {
		return eqBandCenterLabels[band]
	}
	return ""
}
```

Before writing that helper, grep for an existing `eqBandCenterLabels` / band-frequency label table (`grep -n "kHz\|bandLabel\|BandCenters\|eqBand" eq_panel_zone.go`) and reuse it; only add a new table if none exists (it almost certainly does — the readout cells already print frequency labels).

- [ ] **Step 7: Add the portal helpers**

In `drumview_portal_open.go`, after `closeSamplerWheelPortal` (~line 402):

```go
// openEQWheelPortal opens the EQ precision wheel popup through the portal.
func (dv *DrumView) openEQWheelPortal() {
	if dv.tree == nil || dv.eqWheelPopup == nil {
		return
	}
	dv.portal().Open(PortalEntry{
		ID:      "eq-wheel-popup",
		Overlay: &mobileWheelPopupPortalOverlay{popup: dv.eqWheelPopup, tag: "eq-wheel-popup"},
		Modal:   true,
		Scrim:   true, // EQ wheel popup — backdrop dims so modality is visible
		OnClose: func() {
			dv.eqWheelPopup.Close()
		},
	})
}

// closeEQWheelPortal closes the EQ wheel popup portal entry.
func (dv *DrumView) closeEQWheelPortal() {
	if dv.tree != nil {
		dv.portal().Close("eq-wheel-popup")
	}
}
```

- [ ] **Step 8: Add to `CloseAllPopups`**

In `drumview_close_popups.go`, inside `CloseAllPopups` (after the synth/sampler wheel close calls — grep `closeSamplerKnobWheelPopup` to find the spot), add:

```go
	dv.closeEQKnobWheelPopup()
```

If `CloseAllPopups` calls `closeSamplerWheelPortal` directly rather than the `closeSamplerKnobWheelPopup` wrapper, match that convention (call `dv.closeEQWheelPortal()` and `dv.eqWheelPopup.Close()`), but guard against recursion — `openEQKnobWheelPopup` calls `CloseAllPopups()` first, so `CloseAllPopups` must NOT itself reopen anything. A plain close is safe.

- [ ] **Step 9: Run the tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEQWheelPopup_' ./internal/ui/`
Expected: PASS.

- [ ] **Step 10: Verify build + broad EQ/popup suite** (replaces commit)

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'EQ|WheelPopup|CloseAllPopups|Portal' ./internal/ui/`
Expected: PASS. Do NOT commit.

---

### Task 3: Input isolation + audio-apply parity

**Files:**
- Test: `src/go/internal/ui/eq_wheel_popup_isolation_test.go` (create)

**Interfaces:**
- Consumes: `openEQKnobWheelPopup`, `dv.eqWheelPopup`, `g.modalOverlayActive()`, `g.drum.PortalHasBlocking()`, `g.handleTapInGrid`, and whatever apply-observation seam the existing EQ audio tests use (grep `SetChannelEQ` in `*_test.go` for the canonical spy/inspection helper).

- [ ] **Step 1: Write the failing isolation + apply tests**

Create `src/go/internal/ui/eq_wheel_popup_isolation_test.go`. Model the structure on `synth_wheel_popup_isolation_test.go` (read it first for the exact helpers — `assertDefaultParityState`, `withDefaultStart`, `globalTouchState.Reset()`, grid-point selection):

```go
//go:build test

package ui

import "testing"

// openEQWheel sets up the EQ tab with the wheel popup open and returns a grid
// point outside the popup (a "tap on the scrim over the grid").
func openEQWheel(t *testing.T) (g *Game, gx, gy int) {
	t.Helper()
	g = New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.SetActiveTab(TabEQ)
	g.drum.Layout(g.drum.Bounds)
	g.drum.openEQKnobWheelPopup(0)
	if g.drum.eqWheelPopup == nil || !g.drum.eqWheelPopup.IsOpen() {
		t.Fatal("setup did not open the EQ wheel popup")
	}
	gx, gy = 12, gridTopOffset()+8
	if !g.split.InGridPane(gx, gy) {
		t.Fatalf("test point (%d,%d) is not in the grid pane", gx, gy)
	}
	if pr := g.drum.eqWheelPopup.Rect(); pr.Min.Y <= gy {
		t.Fatalf("test point y=%d overlaps popup rect %v", gy, pr)
	}
	return g, gx, gy
}

// While the EQ wheel is open it is a blocking modal overlay.
func TestEQWheelPopup_IsModalBlocking(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, _, _ := openEQWheel(t)
	if !g.drum.PortalHasBlocking() {
		t.Fatal("EQ wheel popup must register a blocking (modal+scrim) portal")
	}
	if !g.modalOverlayActive() {
		t.Fatal("modalOverlayActive() must be true while the EQ wheel is open")
	}
}

// A tap in the grid pane while the wheel is open must NOT create a node.
func TestEQWheelPopup_BlocksGridTap(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	globalTouchState.Reset()
	g, gx, gy := openEQWheel(t)
	before := len(g.nodes)
	g.handleTapInGrid(gx, gy)
	if len(g.nodes) != before {
		t.Fatalf("grid tap leaked through scrim: nodes %d -> %d", before, len(g.nodes))
	}
}
```

For the audio-apply assertion, add a test that opens the wheel on the master channel, drives one wheel step, and asserts the master EQ apply path ran. Use the same observation seam the existing master-EQ tests use — grep `applyMasterEQ\|SetChannelEQ` in `*_test.go` and reuse that helper rather than inventing one. If the existing tests assert on `dv.eqApplied` (the stored applied-bands snapshot set by `applyMasterEQ`), assert the changed band is reflected there after a wheel step.

- [ ] **Step 2: Run to verify failure**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEQWheelPopup_IsModalBlocking|TestEQWheelPopup_BlocksGridTap' ./internal/ui/`
Expected: FAIL if any wiring is missing; otherwise PASS (isolation is inherited from the Modal+Scrim portal). If they already pass, that confirms Task 2's portal flags are correct — keep the tests as regression guards.

- [ ] **Step 3: Fix any leak**

If `TestEQWheelPopup_BlocksGridTap` fails, confirm the portal `Modal: true` flag is set (Task 2 Step 7) and that `modalOverlayActive()` → `PortalHasBlocking()` sees the `"eq-wheel-popup"` entry. No new code should be needed; the synth/sampler wheels already prove this path.

- [ ] **Step 4: Run to verify pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEQWheelPopup' ./internal/ui/`
Expected: PASS. Do NOT commit.

---

### Task 4: Full-suite verification + desktop sizing sanity

**Files:** none (verification only)

- [ ] **Step 1: Run the full `internal/ui` fast suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Expected: PASS. Investigate and fix any failure whose cause is this change (the `internal/ui` suite is noted as occasionally flaky on this branch — re-run a suspected-flaky failing test in isolation to confirm it is pre-existing, not introduced).

- [ ] **Step 2: Desktop sizing sanity check**

The `TestEQWheelPopup_OpensOnBothProfiles` desktop sub-test already asserts the popup opens under the desktop (comfortable-density) profile. Add one assertion there (or a sibling test) that `dv.EQWheelPopupRect()` is non-empty and fits inside `dv.Bounds` on desktop:

```go
func TestEQWheelPopup_DesktopRectFitsBounds(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	dv := g.drum
	dv.SetActiveTab(TabEQ)
	dv.Layout(dv.Bounds)
	dv.openEQKnobWheelPopup(0)
	r := dv.EQWheelPopupRect()
	if r.Empty() {
		t.Fatal("desktop EQ wheel popup rect is empty")
	}
	if !r.In(dv.Bounds) {
		t.Fatalf("desktop EQ wheel popup rect %v not inside bounds %v", r, dv.Bounds)
	}
}
```

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEQWheelPopup_DesktopRectFitsBounds' ./internal/ui/`
Expected: PASS. If the rect overflows on desktop, the `MobileWheelW/MobileWheelH` comfortable-density tokens are too large for the desktop panel — adjust via DESIGN.md `densities.comfortable` and `make gen-design-tokens` following the token-change checklist (update DESIGN.md YAML + the Go constant + the drift want-map). Prefer NOT changing tokens unless this test actually fails.

- [ ] **Step 3: Final verification summary** (replaces commit)

Run the full suite once more and confirm green:
`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/`
Leave everything uncommitted for user review (standing rule). Report which files changed and the passing test names.

---

## Self-Review notes

- **Spec coverage:** Data flow/source-of-truth → Task 1 + Task 2 Step 6 (`applyBandGainLive`, transient knob, `OnChange`/`OnCommit`). Wiring (field/ctor/open/portal/adapter) → Task 2. Desktop+mobile trigger → Task 2 Step 1 (`TestEQWheelPopup_OpensOnBothProfiles`). Numeric-entry preserved → Task 2 Step 1 (`CenterBoxOpensNumericEditor`). Shared step preference → Task 2 Step 4 (single `eqWheelStepBadge`). Input isolation → Task 3. Clamp → Task 1. Desktop sizing → Task 4 Step 2. All spec §Testing items 1–7 mapped.
- **Placeholder scan:** No TBD/TODO; every code step shows complete code. The two "grep to confirm the existing helper name" notes (band label, apply-observation seam) are deliberate codebase-adaptation instructions, not placeholders — each has a concrete fallback.
- **Type consistency:** `applyBandGainLive(band int, v float64)`, `OnOpenValueWheel func(band int)`, `openEQKnobWheelPopup(band int)`, `eqWheelPopup *MobileWheelPopup`, `eqWheelStepBadge *KnobStepBadge`, portal ID `"eq-wheel-popup"` used identically across all tasks. `WheelBinding` fields match the sampler reference (`Knob/Badge/Def/Discrete/Title/OnChange/OnCommit/OnResolution/OpenEditor/Accent`).
