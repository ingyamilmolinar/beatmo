# TransportZone Implementation Plan (Phase 3)

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extract ~30 transport fields + layout/input/draw code from DrumView into a self-contained TransportZone, following the evolutionary migration pattern established by EQPanelZone (Phase 2).

**Architecture:** TransportZone implements the Zone interface. It owns all transport buttons, the BPM text input, the master volume slider/group, and button animation state. During migration, old DrumView fields are aliased back for backward compatibility. Three popups (subdiv menu, overflow menu, master vol popup) are migrated to PortalOverlay. The zone's Draw() delegates to existing toolbar cache rendering via a callback during migration.

**Tech Stack:** Go 1.23, Ebiten v2, `-tags test` build tag for stubs.

---

## Lessons from Phase 2 (EQPanelZone)

These hard-won lessons MUST be applied:

1. **Suppress bridge must fully sync**: `t.suppress = suppressClicksUntilRelease` (not conditional). Stale suppress leaks across test frames otherwise.
2. **PopupActive callback**: Tree yields input to old popup code when `anyDropdownOpen()` returns true. This prevents the zone's hit areas from stealing clicks from open popups.
3. **Wheel reads deferred**: Tree only calls `wheel()` when popups are inactive, so old overlay dispatch can read the one-shot test value.
4. **Touch expansion on adjacent buttons**: Use `Touch: false` on tightly packed buttons to prevent overlap-induced mis-dispatch. Transport buttons are wide enough on mobile already.
5. **Old field aliasing**: `dv.playBtn = dv.transportZone.playBtn` etc. Preserves all existing test code.
6. **Test frame gaps**: Between `restore()` + `SetInputForTest()`, `tree.wasPressed` persists but global suppress is cleared. The full-sync bridge handles this.

## File Structure

| File | Responsibility |
|------|---------------|
| `transport_zone.go` | **NEW** — TransportZone struct, Zone interface, TransportCallbacks, button/slider creation, layout, hit areas, Update (animations + BPM delta), Draw (delegates to toolbar cache via callback) |
| `transport_zone_test.go` | **NEW** — TDD tests: lifecycle, layout, hit areas, callbacks, responsive layout, BPM input, animation decay |
| `drumview_ctor.go` | **MODIFY** — Wire TransportZone: create zone, set callbacks, register in tree, alias fields back |
| `drumview_tree.go` | **READ ONLY** — Already has popupActive + suppress bridge from Phase 2 |
| `drumview_update.go` | **MODIFY** — Remove duplicate animation decay (zone handles it); BPM delta applied by zone |
| `drumview_layout.go` | **MODIFY** — Delegate transport layout to zone via `tree.SetZoneRect()` |
| `drumview_toolbar.go` | **MODIFY** — Toolbar rendering accessed by zone via callback |

---

## Chunk 1: Core Zone + Layout

### Task 1: TransportZone struct + Zone interface (RED then GREEN)

**Files:**
- Create: `src/go/internal/ui/transport_zone_test.go`
- Create: `src/go/internal/ui/transport_zone.go`

**Scope:** Define TransportCallbacks, TransportZone struct, Zone interface methods. Create all ~15 buttons + BPM text input + main vol slider + slider group.

- [ ] **Step 1: Write failing test — zone ID + interface compliance**

```go
//go:build test

package ui

import (
	"image"
	"testing"
)

func TestTransportZoneID(t *testing.T) {
	z := NewTransportZone(TransportCallbacks{})
	if z.ID() != "transport" {
		t.Fatalf("expected ID 'transport', got %q", z.ID())
	}
}

func TestTransportZoneNeedsLayout(t *testing.T) {
	z := NewTransportZone(TransportCallbacks{})
	if !z.NeedsLayout() {
		t.Fatal("new zone should need layout")
	}
	z.Layout(image.Rect(0, 0, 800, 80))
	if z.NeedsLayout() {
		t.Fatal("zone should not need layout after Layout()")
	}
	z.Invalidate()
	if !z.NeedsLayout() {
		t.Fatal("zone should need layout after Invalidate()")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run "TestTransportZone" ./internal/ui/ -v -count=1
```

Expected: FAIL — `NewTransportZone` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
package ui

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"
)

// TransportCallbacks contains callbacks for the TransportZone to communicate
// with DrumView and the audio engine. Zones don't reference Game or each other.
type TransportCallbacks struct {
	OnPlay          func()
	OnStop          func()
	OnBPMChange     func(bpm int)
	OnSubdivClick   func()           // opens/closes old subdiv dropdown
	OnLenInc        func()
	OnLenDec        func()
	OnFollow        func(follow bool)
	OnUpload        func()
	OnImport        func()
	OnExport        func()
	OnSave          func()
	OnViewSwitch    func()
	OnOverflow      func()
	OnMainVolChange func(val float64)
	OnMasterVolIcon func()           // mobile: open master vol popup

	// State queries (zone reads, DrumView owns).
	GetBPM          func() int
	GetLength       func() int
	IsPlaying       func() bool
	IsFollowing     func() bool
	GetMainVolume   func() float64
	GetSubdivText   func() string
}

// TransportZone implements the Zone interface for the transport toolbar.
// It owns play/stop, BPM +/-, BPM text input, subdiv button, len +/-,
// track button, upload/import/export, main volume slider, view switch,
// and overflow button.
type TransportZone struct {
	rect       image.Rectangle
	needLayout bool
	callbacks  TransportCallbacks
	portal     *OverlayPortal

	// Buttons
	playBtn       *Button
	stopBtn       *Button
	bpmDecBtn     *Button
	bpmIncBtn     *Button
	subdivBtn     *Button
	lenDecBtn     *Button
	lenIncBtn     *Button
	trackBtn      *Button
	uploadBtn     *Button
	importBtn     *Button
	exportBtn     *Button
	saveBtn       *Button
	viewSwitchBtn *Button
	overflowBtn   *Button

	// BPM input
	bpmBox   *TextInput
	bpmDelta int
	bpmPrev  int

	// Main volume
	mainVolSlider   *Slider
	mainVolGroup    *SliderGroup
	mainVolRect     image.Rectangle
	mainVolIconRect image.Rectangle

	// Layout
	transportGroup *LayoutGroup

	// Animations
	playAnim     float64
	stopAnim     float64
	bpmDecAnim   float64
	bpmIncAnim   float64
	lenDecAnim   float64
	lenIncAnim   float64
	uploadAnim   float64
	bpmErrorAnim float64
	saveAnim     float64

	// Hit areas
	hitAreas []HitArea
}

func NewTransportZone(cb TransportCallbacks) *TransportZone {
	z := &TransportZone{
		needLayout: true,
		callbacks:  cb,
	}

	// Play/Stop
	z.playBtn = NewButton("", PlayButtonStyle, func() {
		z.playAnim = 1
		if cb.OnPlay != nil {
			cb.OnPlay()
		}
	})
	z.playBtn.Icon = "play"

	z.stopBtn = NewButton("", StopButtonStyle, func() {
		z.stopAnim = 1
		if cb.OnStop != nil {
			cb.OnStop()
		}
	})
	z.stopBtn.Icon = "stop"

	// BPM +/-
	z.bpmDecBtn = NewButton("", BPMButtonStyle, func() {
		z.bpmDelta--
		z.bpmDecAnim = 1
	})
	z.bpmDecBtn.Icon = "minus"
	z.bpmDecBtn.Repeat = true

	z.bpmIncBtn = NewButton("", BPMButtonStyle, func() {
		z.bpmDelta++
		z.bpmIncAnim = 1
	})
	z.bpmIncBtn.Icon = "plus"
	z.bpmIncBtn.Repeat = true

	// BPM text input
	z.bpmBox = NewTextInput()
	z.bpmBox.MaxLen = 4
	z.bpmBox.InputMode = "numeric"
	z.bpmBox.MobileInputID = "bpm"

	// Subdiv
	z.subdivBtn = NewButton("8", InstButtonStyle, func() {
		if cb.OnSubdivClick != nil {
			cb.OnSubdivClick()
		}
	})

	// Len +/-
	z.lenDecBtn = NewButton("", BPMButtonStyle, func() {
		z.lenDecAnim = 1
		if cb.OnLenDec != nil {
			cb.OnLenDec()
		}
	})
	z.lenDecBtn.Icon = "minus"
	z.lenDecBtn.Repeat = true

	z.lenIncBtn = NewButton("", BPMButtonStyle, func() {
		z.lenIncAnim = 1
		if cb.OnLenInc != nil {
			cb.OnLenInc()
		}
	})
	z.lenIncBtn.Icon = "plus"
	z.lenIncBtn.Repeat = true

	// Track follow
	z.trackBtn = NewButton("Follow", InstButtonStyle, func() {
		if cb.OnFollow != nil {
			cb.OnFollow(true) // toggled by DrumView callback
		}
	})

	// Upload/Import/Export
	z.uploadBtn = NewButton("Upload", InstButtonStyle, func() {
		z.uploadAnim = 1
		if cb.OnUpload != nil {
			cb.OnUpload()
		}
	})

	z.importBtn = NewButton("Import", InstButtonStyle, func() {
		if cb.OnImport != nil {
			cb.OnImport()
		}
	})

	z.exportBtn = NewButton("Export", InstButtonStyle, func() {
		if cb.OnExport != nil {
			cb.OnExport()
		}
	})

	z.saveBtn = NewButton("Save", InstButtonStyle, nil) // OnClick wired lazily

	// View switch (mobile)
	z.viewSwitchBtn = NewButton("", InstButtonStyle, func() {
		if cb.OnViewSwitch != nil {
			cb.OnViewSwitch()
		}
	})
	z.viewSwitchBtn.Icon = "audio"

	// Overflow (mobile)
	z.overflowBtn = NewButton("", InstButtonStyle, func() {
		if cb.OnOverflow != nil {
			cb.OnOverflow()
		}
	})
	z.overflowBtn.Icon = "overflow"

	// Main volume
	z.mainVolSlider = NewSlider(1.0)
	z.mainVolGroup = NewSliderGroup([]*Slider{z.mainVolSlider}, func(idx int, val float64) {
		if cb.OnMainVolChange != nil {
			cb.OnMainVolChange(val)
		}
	})

	return z
}

// --- Zone interface ---

func (z *TransportZone) ID() string        { return "transport" }
func (z *TransportZone) NeedsLayout() bool { return z.needLayout }
func (z *TransportZone) Invalidate()       { z.needLayout = true }

func (z *TransportZone) Layout(r image.Rectangle) {
	z.rect = r
	z.needLayout = false
	z.layoutButtons()
	z.rebuildHitAreas()
}

func (z *TransportZone) Update() {
	z.decayAnims()
	z.applyBPMDelta()
}

func (z *TransportZone) HitAreas() []HitArea { return z.hitAreas }

func (z *TransportZone) Draw(screen *ebiten.Image) {
	// During migration, toolbar drawing is handled by the existing
	// renderToolbarControls() in drumview_toolbar.go. This will be
	// moved here in Phase 6.
}

func (z *TransportZone) HandleKey(key ebiten.Key)  {}
func (z *TransportZone) HandleChars(chars []rune)   {}

func (z *TransportZone) SetPortal(p *OverlayPortal) { z.portal = p }

// --- Internal ---

func (z *TransportZone) layoutButtons() {
	// Placeholder — implemented in Task 2.
}

func (z *TransportZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]
	// Placeholder — implemented in Task 3.
}

func (z *TransportZone) decayAnims() {
	decay := func(v *float64) {
		*v *= 0.85
		if *v < 0.01 {
			*v = 0
		}
	}
	decay(&z.playAnim)
	decay(&z.stopAnim)
	decay(&z.bpmDecAnim)
	decay(&z.bpmIncAnim)
	decay(&z.lenDecAnim)
	decay(&z.lenIncAnim)
	decay(&z.uploadAnim)
	decay(&z.bpmErrorAnim)
	decay(&z.saveAnim)
}

func (z *TransportZone) applyBPMDelta() {
	if z.bpmDelta != 0 && z.callbacks.OnBPMChange != nil {
		bpm := 120 // default
		if z.callbacks.GetBPM != nil {
			bpm = z.callbacks.GetBPM()
		}
		z.callbacks.OnBPMChange(bpm + z.bpmDelta)
		z.bpmDelta = 0
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run "TestTransportZone" ./internal/ui/ -v -count=1
```

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add src/go/internal/ui/transport_zone.go src/go/internal/ui/transport_zone_test.go
git commit -m "feat(ui): add TransportZone struct with Zone interface (Phase 3, Task 1)"
```

---

### Task 2: Layout — desktop + mobile responsive (RED then GREEN)

**Files:**
- Modify: `src/go/internal/ui/transport_zone_test.go`
- Modify: `src/go/internal/ui/transport_zone.go`

**Scope:** Implement `layoutButtons()` with mobile two-row and desktop two-row responsive layout. Follows same `GridLayout` pattern as `drumview_layout.go:155–238`.

- [ ] **Step 1: Write failing tests — layout geometry**

```go
func TestTransportZoneDesktopLayout(t *testing.T) {
	z := NewTransportZone(TransportCallbacks{})
	z.Layout(image.Rect(0, 0, 800, 80))

	// Play button should have non-empty rect.
	if z.playBtn.Rect().Empty() {
		t.Fatal("playBtn rect is empty after Layout")
	}
	// Stop button should be to the right of play.
	if z.stopBtn.Rect().Min.X <= z.playBtn.Rect().Min.X {
		t.Fatal("stopBtn should be right of playBtn")
	}
	// BPM box should be to the right of stop.
	if z.bpmBox.Rect.Min.X <= z.stopBtn.Rect().Max.X {
		t.Fatal("bpmBox should be right of stopBtn")
	}
	// Desktop: upload/import/export should have non-empty rects.
	if z.uploadBtn.Rect().Empty() {
		t.Fatal("uploadBtn should be visible on desktop")
	}
	// Desktop: mainVolSlider should have non-empty rect.
	if z.mainVolSlider.Rect().Empty() {
		t.Fatal("mainVolSlider should be visible on desktop")
	}
	// Desktop: viewSwitchBtn and overflowBtn should be hidden.
	if !z.viewSwitchBtn.Rect().Empty() {
		t.Fatal("viewSwitchBtn should be hidden on desktop")
	}
	if !z.overflowBtn.Rect().Empty() {
		t.Fatal("overflowBtn should be hidden on desktop")
	}
}

func TestTransportZoneMobileLayout(t *testing.T) {
	old := SetMobileOverride(true)
	defer old()
	z := NewTransportZone(TransportCallbacks{})
	z.Layout(image.Rect(0, 0, 390, 80))

	// Play button should have non-empty rect.
	if z.playBtn.Rect().Empty() {
		t.Fatal("playBtn rect is empty on mobile")
	}
	// Mobile: viewSwitchBtn and overflowBtn should be visible.
	if z.viewSwitchBtn.Rect().Empty() {
		t.Fatal("viewSwitchBtn should be visible on mobile")
	}
	if z.overflowBtn.Rect().Empty() {
		t.Fatal("overflowBtn should be visible on mobile")
	}
	// Mobile: upload/import/export should be hidden.
	if !z.uploadBtn.Rect().Empty() {
		t.Fatal("uploadBtn should be hidden on mobile")
	}
	// Mobile: mainVolSlider should be hidden, mainVolIconRect should be set.
	if !z.mainVolSlider.Rect().Empty() {
		t.Fatal("mainVolSlider should be hidden on mobile")
	}
	if z.mainVolIconRect.Empty() {
		t.Fatal("mainVolIconRect should be set on mobile")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Expected: FAIL — layout methods are placeholder stubs.

- [ ] **Step 3: Implement `layoutButtons()` in `transport_zone.go`**

Port the layout logic from `drumview_layout.go:155–238`, adapted to operate on `z.*` fields instead of `dv.*`. Use the same `GridLayout`/`NewGridLayout`/`stackVertical`/`safeInset` helpers.

The mobile path uses a 2×1 outer grid → row0 (5 cols: play|stop|bpm|+/-|subdiv) + row1 (3 cols: volIcon|viewSwitch|overflow). Desktop: 2-row with persistent `LayoutGroup` — row0 (5 cols) + row1 (4 cols: upload|import|export|vol slider). Hidden buttons get empty rects.

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(ui): TransportZone responsive layout — desktop + mobile (Phase 3, Task 2)"
```

---

### Task 3: Hit areas for buttons + main vol slider (RED then GREEN)

**Files:**
- Modify: `src/go/internal/ui/transport_zone_test.go`
- Modify: `src/go/internal/ui/transport_zone.go`

**Scope:** Implement `rebuildHitAreas()`. Register a `buttonHitAdapter` for each non-empty button. Register `sliderGroupHitAdapter` for `mainVolGroup` on desktop.

- [ ] **Step 1: Write failing test — hit areas registered**

```go
func TestTransportZoneHitAreas(t *testing.T) {
	z := NewTransportZone(TransportCallbacks{})
	z.Layout(image.Rect(0, 0, 800, 80))

	areas := z.HitAreas()
	if len(areas) == 0 {
		t.Fatal("expected hit areas after Layout")
	}

	// Check that play button has a hit area.
	found := false
	for _, a := range areas {
		if a.Tag == "transport-play" {
			found = true
			if a.Rect.Empty() {
				t.Fatal("play hit area rect is empty")
			}
			break
		}
	}
	if !found {
		t.Fatal("no hit area tagged 'transport-play'")
	}
}

func TestTransportZonePlayCallback(t *testing.T) {
	var played bool
	z := NewTransportZone(TransportCallbacks{
		OnPlay: func() { played = true },
	})
	z.Layout(image.Rect(0, 0, 800, 80))

	// Find play hit area and press it.
	for _, a := range z.HitAreas() {
		if a.Tag == "transport-play" {
			a.Handler.OnPress(a.Rect.Min.X+1, a.Rect.Min.Y+1)
			break
		}
	}
	if !played {
		t.Fatal("OnPlay callback not fired")
	}
}

func TestTransportZoneBPMDeltaViaHitArea(t *testing.T) {
	bpmSet := 0
	z := NewTransportZone(TransportCallbacks{
		GetBPM:      func() int { return 120 },
		OnBPMChange: func(bpm int) { bpmSet = bpm },
	})
	z.Layout(image.Rect(0, 0, 800, 80))

	// Press BPM+ hit area.
	for _, a := range z.HitAreas() {
		if a.Tag == "transport-bpm-inc" {
			a.Handler.OnPress(a.Rect.Min.X+1, a.Rect.Min.Y+1)
			break
		}
	}
	// Delta accumulated, not yet applied.
	if z.bpmDelta != 1 {
		t.Fatalf("expected bpmDelta=1, got %d", z.bpmDelta)
	}
	// Update applies the delta.
	z.Update()
	if bpmSet != 121 {
		t.Fatalf("expected BPM=121, got %d", bpmSet)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Expected: FAIL — `rebuildHitAreas()` is a stub.

- [ ] **Step 3: Implement `rebuildHitAreas()`**

Register hit areas for all visible buttons using `buttonHitAdapter` (same type from `eq_panel_zone.go`). Each tagged `"transport-<name>"`. Use `Touch: false` on all transport buttons (they're already wide enough). Register `sliderGroupHitAdapter` for `mainVolGroup` on desktop (when `mainVolSlider.Rect()` is non-empty).

```go
func (z *TransportZone) rebuildHitAreas() {
	z.hitAreas = z.hitAreas[:0]
	zIdx := ZBaseMin + 10 // within base zone range

	type entry struct {
		btn *Button
		tag string
	}
	btns := []entry{
		{z.playBtn, "transport-play"},
		{z.stopBtn, "transport-stop"},
		{z.bpmDecBtn, "transport-bpm-dec"},
		{z.bpmIncBtn, "transport-bpm-inc"},
		{z.subdivBtn, "transport-subdiv"},
		{z.lenDecBtn, "transport-len-dec"},
		{z.lenIncBtn, "transport-len-inc"},
		{z.trackBtn, "transport-track"},
		{z.uploadBtn, "transport-upload"},
		{z.importBtn, "transport-import"},
		{z.exportBtn, "transport-export"},
		{z.viewSwitchBtn, "transport-view-switch"},
		{z.overflowBtn, "transport-overflow"},
	}

	for _, e := range btns {
		if e.btn == nil || e.btn.Rect().Empty() {
			continue
		}
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    e.btn.Rect(),
			ZIndex:  zIdx,
			Handler: &buttonHitAdapter{btn: e.btn},
			Tag:     e.tag,
			Touch:   false,
		})
	}

	// Main volume slider group (desktop only).
	if !z.mainVolSlider.Rect().Empty() {
		z.hitAreas = append(z.hitAreas, HitArea{
			Rect:    z.mainVolGroup.InputBounds(),
			ZIndex:  zIdx,
			Handler: &sliderGroupHitAdapter{group: z.mainVolGroup},
			Tag:     "transport-main-vol",
		})
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

- [ ] **Step 5: Commit**

```bash
git commit -m "feat(ui): TransportZone hit areas for buttons + volume slider (Phase 3, Task 3)"
```

---

## Chunk 2: Animation + BPM Input + Tree Integration

### Task 4: Animation decay test (RED then GREEN)

**Files:**
- Modify: `src/go/internal/ui/transport_zone_test.go`

- [ ] **Step 1: Write failing test — animation decay**

```go
func TestTransportZoneAnimDecay(t *testing.T) {
	z := NewTransportZone(TransportCallbacks{})
	z.playAnim = 1.0
	z.stopAnim = 1.0

	// After several Update() calls, animations should decay toward 0.
	for i := 0; i < 60; i++ {
		z.Update()
	}
	if z.playAnim > 0.01 {
		t.Fatalf("playAnim should decay to ~0 after 60 frames, got %f", z.playAnim)
	}
	if z.stopAnim > 0.01 {
		t.Fatalf("stopAnim should decay to ~0 after 60 frames, got %f", z.stopAnim)
	}
}
```

- [ ] **Step 2: Run test to verify it passes (already implemented in Task 1)**

This should already pass since `decayAnims()` was implemented in Task 1. If it passes immediately, that's fine — it validates the existing code. If not, fix.

- [ ] **Step 3: Commit**

```bash
git commit -m "test(ui): TransportZone animation decay test (Phase 3, Task 4)"
```

---

### Task 5: Tree lifecycle integration test (RED then GREEN)

**Files:**
- Modify: `src/go/internal/ui/transport_zone_test.go`

**Scope:** Verify TransportZone works correctly in the DrumViewTree 4-phase loop.

- [ ] **Step 1: Write failing test — tree lifecycle**

```go
func TestTransportZoneTreeLifecycle(t *testing.T) {
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore()

	var played bool
	z := NewTransportZone(TransportCallbacks{
		OnPlay: func() { played = true },
	})
	tree := NewDrumViewTree()
	tree.RegisterZone(z, 110)
	tree.SetZoneRect("transport", image.Rect(0, 0, 800, 80))
	tree.Update() // Layout + Update + Input phases run.

	// Play button should have been laid out.
	if z.playBtn.Rect().Empty() {
		t.Fatal("playBtn not laid out after tree.Update()")
	}

	// Now simulate a press on the play button.
	playRect := z.playBtn.Rect()
	mx, my := playRect.Min.X+1, playRect.Min.Y+1
	var pressed bool
	restore2 := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 800, 600 },
	)
	defer restore2()
	pressed = true
	tree.Update()

	if !played {
		t.Fatal("OnPlay callback not dispatched through tree")
	}
	if !tree.InputHandled() {
		t.Fatal("tree should report inputHandled after dispatch")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Expected: FAIL — tree dispatches to hit area, which should work if Tasks 1–3 are done correctly. May pass immediately. If so, it validates the integration.

- [ ] **Step 3: Fix any issues**

- [ ] **Step 4: Commit**

```bash
git commit -m "test(ui): TransportZone tree lifecycle integration test (Phase 3, Task 5)"
```

---

### Task 6: Wire TransportZone into DrumView

**Files:**
- Modify: `src/go/internal/ui/drumview_ctor.go`
- Modify: `src/go/internal/ui/drumview_layout.go`
- Modify: `src/go/internal/ui/drumview_update.go`
- Modify: `src/go/internal/ui/drumview.go` (if needed for new field)

**Scope:** Create TransportZone in `NewDrumView()`, register in tree, alias all button/slider fields back to DrumView, delegate layout rect to zone via `tree.SetZoneRect()`, remove duplicate animation decay from `drumview_toolbar.go`.

**Critical wiring pattern (from Phase 2):**

```go
// In drumview_ctor.go, after tree creation:

dv.transportZone = NewTransportZone(TransportCallbacks{
	OnPlay:          func() { dv.playPressed = true; dv.playAnim = 1 },
	OnStop:          func() { dv.stopPressed = true; dv.stopAnim = 1 },
	OnBPMChange:     func(bpm int) { dv.SetBPM(bpm) },
	OnSubdivClick:   func() { /* existing subdiv toggle code */ },
	OnLenInc:        func() { dv.lenIncPressed = true },
	OnLenDec:        func() { dv.lenDecPressed = true },
	OnFollow:        func(f bool) { dv.SetFollow(!dv.follow) },
	OnUpload:        func() { /* existing upload goroutine spawn */ },
	OnImport:        func() { /* existing import dialog start */ },
	OnExport:        func() { dv.Export() },
	OnViewSwitch:    func() { dv.cycleViewMode() },
	OnOverflow:      func() { /* existing overflow toggle */ },
	OnMainVolChange: func(v float64) { audio.SetMainVolume(v) },
	OnMasterVolIcon: func() { /* toggle masterVolPopup */ },
	GetBPM:          func() int { return dv.bpm },
	GetLength:       func() int { return dv.Length },
	IsPlaying:       func() bool { return dv.isPlaying },
	IsFollowing:     func() bool { return dv.follow },
	GetMainVolume:   func() float64 { return dv.mainVolSlider.Value },
	GetSubdivText:   func() string { return dv.subdivBtn.Text },
})

// Register in tree
dv.tree.RegisterZone(dv.transportZone, 110)

// Alias fields back for backward compatibility
dv.playBtn = dv.transportZone.playBtn
dv.stopBtn = dv.transportZone.stopBtn
dv.bpmDecBtn = dv.transportZone.bpmDecBtn
dv.bpmIncBtn = dv.transportZone.bpmIncBtn
dv.subdivBtn = dv.transportZone.subdivBtn
dv.lenDecBtn = dv.transportZone.lenDecBtn
dv.lenIncBtn = dv.transportZone.lenIncBtn
dv.trackBtn = dv.transportZone.trackBtn
dv.uploadBtn = dv.transportZone.uploadBtn
dv.importBtn = dv.transportZone.importBtn
dv.exportBtn = dv.transportZone.exportBtn
dv.saveBtn = dv.transportZone.saveBtn
dv.viewSwitchBtn = dv.transportZone.viewSwitchBtn
dv.overflowBtn = dv.transportZone.overflowBtn
dv.bpmBox = dv.transportZone.bpmBox
dv.mainVolSlider = dv.transportZone.mainVolSlider
dv.mainVolGroup = dv.transportZone.mainVolGroup
```

- [ ] **Step 1: Add `transportZone *TransportZone` field to DrumView**

- [ ] **Step 2: Create TransportZone in `NewDrumView()` with callbacks**

Move button creation from `drumview_ctor.go:174–416` into TransportZone constructor (already done in Task 1). The DrumView callbacks delegate to existing methods.

- [ ] **Step 3: Register zone in tree at z-index 110**

```go
dv.tree.RegisterZone(dv.transportZone, 110)
```

- [ ] **Step 4: Alias all button/slider fields back to DrumView**

- [ ] **Step 5: Delegate transport layout rect via tree**

In `drumview_layout.go`, add `tree.SetZoneRect("transport", topBounds)` where `topBounds` is the transport widget rect. Remove the direct `recalcButtons()` layout code for transport buttons (the zone handles it now).

- [ ] **Step 6: Remove duplicate animation decay from `drumview_toolbar.go`**

The zone's `Update()` calls `decayAnims()`. Remove the `dv.decayAnims()` call from the old code path (it would now be redundant since the tree's Update phase calls `z.Update()` each frame).

- [ ] **Step 7: Run full UI test suite**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -count=1 -timeout 120s
```

Expected: All tests pass. If failures occur, apply the same debugging approach as Phase 2:
- Check if tree intercepts clicks meant for old popup handlers
- Check if suppress leaks across frames
- Check if wheel events are consumed by the tree before overlays

- [ ] **Step 8: Iteratively fix any test failures**

Use `Skill:superpowers:systematic-debugging` for any non-obvious failures.

- [ ] **Step 9: Commit**

```bash
git commit -m "feat(ui): wire TransportZone into DrumView — evolutionary migration (Phase 3, Task 6)"
```

---

## Chunk 3: Verification + Cleanup

### Task 7: Full test suite verification

**Files:** None modified — verification only.

- [ ] **Step 1: Run full Go test suite**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./... -count=1 -timeout 120s
```

Expected: All packages pass.

- [ ] **Step 2: Verify production build**

```bash
cd src/go && ../../.tools/go/bin/go build ./...
```

Expected: Exit 0.

- [ ] **Step 3: Remove any debug output left from investigation**

Check `drumview_test.go`, `drumview_update.go`, `drumview_tree_test.go` for leftover `t.Logf("DEBUG:` lines.

- [ ] **Step 4: Final commit if any cleanup was needed**

---

## Acceptance Criteria Checklist

After all tasks complete, verify each:

- [ ] TransportZone implements all Zone interface methods (ID, Layout, Update, HitAreas, Draw, NeedsLayout, Invalidate, HandleKey, HandleChars)
- [ ] HitAreas register all visible transport buttons as individual hit targets
- [ ] MainVolGroup registered as SliderGroup hit area on desktop
- [ ] Button animations decay per frame in zone's Update()
- [ ] BPM delta accumulation fires OnBPMChange callback
- [ ] Desktop responsive: play/stop/bpm+/-/subdiv row + upload/import/export/vol row
- [ ] Mobile responsive: play/stop/bpm+/-/subdiv row + volIcon/viewSwitch/overflow row
- [ ] Existing transport tests pass (public DrumView API stable via aliasing)
- [ ] New zone tests validate lifecycle, layout, hit areas, callbacks, responsive layout
- [ ] All existing UI tests continue to pass
