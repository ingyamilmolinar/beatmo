# DrumView Component Tree Redesign

Restructure the monolithic DrumView (~600 fields, ~1200-line layout, ~700-line update) into a hierarchical zone-based component tree with strict lifecycle phases, declarative input routing, and portal-based overlays.

## Design Decisions

| Decision | Choice |
|----------|--------|
| Scope | DrumView internals only; Game-level InputDispatcher, Sidebar, Splitter unchanged |
| Depth | Zone-level (~5 zones); buttons/sliders remain lightweight helpers inside zones |
| Popup model | Portal: zones own popup data/logic, shared OverlayPortal renders + isolates input |
| Input model | Hit-test tree walk: zones declare HitAreas, tree routes by z-index |
| Lifecycle | Strict 4-phase: Layout → Update → Input → Draw per frame |
| Migration | Evolutionary extraction: one zone at a time, DrumView shrinks incrementally |

## Core Abstractions

### Zone Interface

```go
type Zone interface {
    ID() string
    Layout(rect image.Rectangle)   // compute internal geometry + hit areas
    Update()                       // per-frame state logic (animations, smoothing, pending results)
    HitAreas() []HitArea           // declare interactive regions (from Layout results)
    Draw(screen *ebiten.Image)     // render (never set geometry here)
    NeedsLayout() bool
    Invalidate()
    HandleKey(key ebiten.Key) InputResult       // keyboard routing
    HandleChars(chars []rune) InputResult        // text input routing
}
```

Layout is lazy — only runs when `NeedsLayout()` returns true or the zone's rect changes. `Update()` runs every frame for per-frame state logic (animations, smoothing, channel results). Draw must never compute geometry.

### Shared State (Read-Only Context)

Zones that need to read shared mutable state receive a `DrumViewState` accessor at construction:

```go
type DrumViewState struct {
    IsPlaying    func() bool
    CurrentBeat  func() int
    BPM          func() int
    Rows         func() []DrumRow
    Predictor    func() *predictor.Predictor
    Timeline     func() *timeline.Timeline
    IsSmallScreen func() bool
    ScreenSize   func() (int, int)
}
```

Function fields (not direct pointers) so zones always read current values without holding stale references. Injected at construction alongside callbacks.

### HitArea and HitHandler

```go
type HitArea struct {
    Rect    image.Rectangle
    ZIndex  int
    Handler HitHandler
    Tag     string          // debug label
    Touch   bool            // if true, HitIndex expands rect by TouchMinTarget()
}

type HitHandler interface {
    OnPress(x, y int) InputResult   // InputCaptured → drag, InputConsumed → tap
    OnDrag(x, y int)                // called each frame while captured
    OnRelease(x, y int)             // called when press ends
    OnWheel(x, y, steps int) InputResult
}
```

Replaces the current `HandleInput(x, y int, pressed bool) InputResult` pattern. The tree owns capture state — one `capturedHandler` per tree, not per-zone.

**Touch target expansion**: `HitArea.Touch = true` tells the HitIndex to expand the rect by `TouchMinTarget()` when querying on touch devices. The zone sets this flag; the HitIndex handles expansion during `At()`.

**Button adapter**: Buttons fire `OnClick` in `OnPress` and return `InputConsumed`. For `Button.Repeat` (held-button repeat like BPM +/-), `OnPress` returns `InputCaptured` and `OnDrag` fires the repeat callback each frame. `OnRelease` ends the repeat.

**Scroll disambiguation**: The `DeferredTap` pattern (press → wait → if no scroll committed → fire tap on release) is implemented in zone-local HitHandlers. The handler's `OnPress` records the press position and returns `InputCaptured`. `OnDrag` checks scroll threshold — if exceeded, the handler transitions to scroll mode. `OnRelease` fires the tap if scroll was not committed.

### DrumViewTree (Orchestrator)

Runs each frame (4 phases):

1. **Layout phase** — for each zone with `NeedsLayout()`, call `Layout(rect)`, then `hitIndex.Update(zone.ID(), zone.HitAreas())`. WidgetBoard assigns zone rects; DrumViewTree calls `wb.SetBounds()` when DrumView bounds change, then distributes `wb.Rect(widget)` to each zone.
2. **Update phase** — call `zone.Update()` for each zone (animations, smoothing, pending channel results).
3. **Input phase** — if captured handler exists, route to it. Otherwise, on new press, query `hitIndex.At(x, y)` and dispatch to highest-z-index handler. Modal overlays filter out non-modal areas. Wheel events route to the hit area under cursor (or captured handler). Keyboard/char events route to the focused zone (only one zone receives keyboard focus at a time).
4. **Draw phase** — draw zones by z-order, then `overlayPortal.Draw(screen)` on top.

### OverlayPortal

```go
type OverlayPortal struct {
    stack []PortalEntry    // ordered stack: last entry = topmost
}

type PortalEntry struct {
    ID      string
    Owner   Zone
    Overlay PortalOverlay   // renamed to avoid conflict with existing Overlay interface
    Modal   bool
    Anchor  image.Rectangle
}

type PortalOverlay interface {
    Layout(anchor, screenBounds image.Rectangle)
    HitAreas() []HitArea
    Draw(screen *ebiten.Image)
    ShouldClose() bool
}
```

The `PortalOverlay` interface is distinct from the existing `Overlay` interface in `overlay_stack.go`. During migration, both coexist; Phase 6 removes the old one.

Zones open overlays via `portal.Open(entry)`. The portal:
- Maintains a stack: `stack[len-1]` is the topmost overlay
- Registers overlay HitAreas in the HitIndex at z-index 300+ (higher index in stack = higher z-index)
- When modal, `hitIndex.At()` returns only the topmost modal overlay's areas
- Click outside topmost modal → `portal.CloseTop()` + suppress. Multiple modals are closed one at a time (top-down).
- Draws overlays in stack order (bottom to top), modal backdrop drawn before the modal overlay
- Wheel events inside a modal overlay are handled by the overlay; outside are blocked

### HitIndex

```go
type HitIndex struct { areas []indexedHitArea }

func (idx *HitIndex) Update(zoneID string, areas []HitArea)
func (idx *HitIndex) UpdatePortal(overlayID string, areas []HitArea)
func (idx *HitIndex) RemovePortal(overlayID string)
func (idx *HitIndex) At(x, y int) []indexedHitArea  // z-sorted, modal-filtered, touch-expanded
```

**Z-index conventions**:

| Layer | Z-index range | Examples |
|-------|---------------|----------|
| Base zones | 100-199 | TransportZone(100), RowRackZone(110), TimelineZone(120), EQPanelZone(130) |
| Layout resize | 200 | WidgetBoard divider drag |
| Overlays | 300-399 | Portal overlays: 300 + stack index |

### Inter-Zone Communication

Callback structs injected at construction:

```go
type EQCallbacks struct {
    OnGainChange func(band int, db float64)
    OnMuteToggle func(band int)
    OnChannelChange func(channelID string)
}
```

Synchronous, type-safe, testable. Zones don't reference Game or each other.

## Zone Decomposition

### TransportZone
- **Owns**: playBtn, stopBtn, bpmDecBtn, bpmBox, bpmIncBtn, subdivBtn, uploadBtn, importBtn, exportBtn, saveBtn, mainVolSlider, mainVolGroup, trackBtn, viewSwitchBtn, overflowBtn
- **Update()**: BPM delta accumulation, button animation decay, pending import/upload channel results
- **Popups**: subdiv dropdown, overflow menu, master volume popup (mobile) → portal
- **Keyboard**: BPM text input focus
- **Responsive**: desktop single-row, mobile two-row with overflow

### RowRackZone
- **Owns**: per-row structs (see below), rowVolGroup (SliderGroup), addRowBtn (FAB), rowScroll (`ScrollBehavior`)
- **Update()**: scroll momentum, label width recalculation
- **Popups**: context menu, instrument menu, color wheel, rename, FX panel, row volume popup (mobile) → portal
- **Key change**: 10+ parallel arrays (`rowLabels[]`, `rowEditBtns[]`, etc.) become `[]rowEntry`. The existing `RowButtonGroup` struct is absorbed into `rowEntry` (adding `colorBtn` and `volSlider`, keeping `Delete`).

```go
type rowEntry struct {
    label     Button
    volSlider Slider
    muteBtn   Button
    soloBtn   Button
    fxBtn     Button
    colorBtn  Button
    editBtn   Button
    saveBtn   Button
    originBtn Button
    deleteBtn Button
    menuBtn   Button    // mobile only
}
```

### TimelineZone
- **Owns**: drumRows[] (Steps, CellTypes), rowCache[], rowsLayer, highlightPositions[], gridScroll, progressBar
- **Update()**: highlight animation, scroll momentum
- **Reads** (via `DrumViewState`): Predictor, Timeline, isPlaying, currentBeat
- **Popups**: none currently

### EQPanelZone
- **Owns**: eqSliders[10], eqMuteBtns[10], eqBandBtns[10], eqSliderGroup, eqToggleBtn, eqChannelBtn, eqBandGainsDB[10], eqBandMuted[10], eqActiveChannel, eqWaveformMode, spectrumSnapshot
- **Update()**: spectrum snapshot polling, EQ band animation smoothing (`eqBandVals`/`eqLastBands`)
- **Popups**: EQ slider popup (mobile), channel dropdown → portal
- **Responsive**: desktop → 10 vertical sliders; mobile → 10 band buttons + popup

### OverlayPortal
- **Owns**: active overlay stack
- **Hit areas**: from active overlays (registered dynamically)
- **Draws**: on top of everything

## Reuse Map (No Rewrite)

| Component | Location | Usage |
|-----------|----------|-------|
| `Slider` | `slider.go` | Adapted to HitHandler (OnPress→capture, OnDrag→update, OnRelease→commit) |
| `SliderGroup` | `slider_group.go` | Adapted: internal capture stays, exposes one HitArea per group (bounding rect). Group still owns which-slider-is-active state; tree capture delegates to group's handler. |
| `Button` | `uigrid.go` | Adapted to HitHandler (OnPress→fire OnClick + InputConsumed; Repeat mode: OnPress→InputCaptured, OnDrag→fire repeat, OnRelease→stop) |
| `GridLayout` | `uigrid.go` | Used within zones for nested layout |
| `InputResult` | `input_handler.go` | Unchanged (Ignored/Consumed/Captured) |
| `ScrollBehavior` | `scroll_behavior.go` | Internal to RowRackZone, handles scroll/touch/momentum |
| `TextInput` | `textinput.go` | BPM box in TransportZone; receives keyboard via zone's HandleChars |
| `SliderPopup` | `drumview_eq_popup.go` | Wraps into PortalOverlay interface |
| `WidgetBoard` | `drumview_layout.go` | DrumViewTree calls wb.SetBounds() on resize, distributes wb.Rect() to zones |
| `DeferredTap` | `deferred_tap.go` | Used inside zone HitHandlers for scroll-tap disambiguation |
| `RowButtonGroup` | `row_button_group.go` | Absorbed into rowEntry struct in RowRackZone |

### SliderGroup Adaptation Detail

`SliderGroup` currently implements `HandleInput(mx, my, pressed) InputResult` and tracks which slider is active internally. The adaptation wraps it as a single HitHandler registered with a bounding rect covering all sliders. The group's `OnPress` delegates to its internal `HandleInput` logic. The group keeps its internal `activeIdx` tracking — the tree doesn't see individual sliders, only the group as a single capture target. `HitAreas()` returns one HitArea per group (not per slider) with the group's handler.

## Migration Phases

### Phase 1: Infrastructure
- **New files**: `zone.go`, `hit_index.go`, `portal.go`, `drumview_tree.go`
- **Tests**: `zone_test.go`, `hit_index_test.go`, `portal_test.go`
- **Integration**: DrumView.Update() calls tree.Update() — tree runs lifecycle on registered zones
- **Includes a trivial test zone** (single button) that exercises the full 4-phase lifecycle end-to-end, validating the infrastructure before any real zone extraction
- **Bridging**: `tree.suppress` syncs bidirectionally with the global `suppressClicksUntilRelease` during migration. Both flags read/write each other. Removed in Phase 6.

### Phase 2: EQPanelZone (highest bug ROI)
- **New files**: `eq_panel_zone.go`, `eq_panel_zone_test.go`
- **Moves**: ~15 `eqXxx` fields, `drawEQ()`, `handleEQCurveDrag()`, `openEQPopup()`, EQ input handling
- **Popups migrated to portal**: `eqPopupOverlay` (SliderPopup), `eqChannelDropdown`
- **Validates**: lifecycle, HitArea registration, Portal, responsive layout, SliderGroup integration
- **DrumView shrinks**: ~15 fields, ~300 lines

### Phase 3: TransportZone
- **New files**: `transport_zone.go`, `transport_zone_test.go`
- **Moves**: ~15 transport fields, transport layout/input/draw code
- **Popups migrated to portal**: `subdivMenuOverlay`, `overflowMenuOverlay`, `masterVolPopupOverlay`
- **DrumView shrinks**: ~15 fields, ~200 lines

### Phase 4: RowRackZone (biggest payoff)
- **New files**: `row_rack_zone.go`, `row_rack_zone_test.go`
- **Moves**: 10+ parallel arrays → `[]rowEntry`, 5 overlay types, SliderGroup, FAB, scroller
- **Popups migrated to portal**: `contextMenuOverlay`, `instMenuOverlay`, `colorWheelOverlay`, `renameOverlay`, `fxPanelOverlay`, `volPopupOverlay`
- **DrumView shrinks**: ~50 fields, ~400 lines

### Phase 5: TimelineZone
- **New files**: `timeline_zone.go`, `timeline_zone_test.go`
- **Moves**: drumRows, sprite caches, highlights, grid scroll, progress bar
- **After this**: DrumView is a thin shell (~50-80 fields, wiring + Game bridge)

### Phase 6: Overlay Cleanup
- **Removes**: `DrumViewRoot`, `OverlayComponentAdapter`, old `Overlay` interface, `OverlayStack`, `overlay_stack.go`, `drumview_root_component.go`, `base_component.go`
- **Removes**: suppress bridging (tree.suppress ↔ suppressClicksUntilRelease sync)
- **Verifies**: all overlay types migrated — context menu, overflow menu, FX panel, subdiv menu, rename, color wheel, instrument menu, EQ popup, channel dropdown, volume popups (row + master + EQ)
- **Result**: all overlay dispatch through OverlayPortal, old overlay system deleted

## Behaviors Preserved

| Behavior | Current Implementation | New Implementation |
|----------|----------------------|-------------------|
| Click-outside-close | `OverlayStack` + `suppressClicksUntilRelease` | `HitIndex.At()` returns empty + `portal.CloseTop()` + tree suppress |
| Slider capture | Per-slider `dragging` bool + `SliderGroup` | `OnPress` returns `InputCaptured`, tree owns `capturedHandler`, SliderGroup keeps internal active-index tracking |
| Button repeat (BPM +/-) | `Button.Repeat` + pressed-every-frame | `OnPress→InputCaptured`, `OnDrag→repeat tick`, `OnRelease→stop` |
| Deferred tap (mobile) | Per-overlay `DeferredTap` state | Zone HitHandler: `OnPress→InputCaptured`, `OnDrag→check scroll threshold`, `OnRelease→fire tap if no scroll` |
| Touch scroll disambiguation | `ScrollBehavior` internal | `ScrollBehavior` inside RowRackZone (unchanged) |
| Suppress until release | Global `suppressClicksUntilRelease` | `tree.suppress` flag, bridged to global during migration, sole owner after Phase 6 |
| Layout resize | `LayoutResizeHandler` | Separate handler in tree at z-index 200 (between zones and overlays) |
| Sprite caching | `rowCache[]`, `rowsLayer`, `rowControlsCache` | Moves into TimelineZone / RowRackZone |
| Touch target expansion | `Button.HandleInputResult` expands by `TouchMinTarget()` | `HitArea.Touch = true`, HitIndex expands during `At()` |
| Keyboard/text input | Direct in DrumView.Update() | `HandleKey`/`HandleChars` on focused zone; tree tracks which zone has keyboard focus |

## Testing Strategy

Each phase includes tests that validate:
1. **Layout sets geometry** — hit rects are non-zero after Layout, before Draw is called
2. **Input routes correctly** — press inside hit area → handler called; press outside → ignored
3. **Capture semantics** — OnPress returning InputCaptured → all subsequent input goes to that handler
4. **Portal modality** — modal overlay open → zone hit areas not returned by HitIndex
5. **Callback firing** — zone actions trigger the correct callbacks with correct arguments
6. **Responsive layout** — zone adapts correctly when given small (mobile) vs large (desktop) rects
7. **Touch expansion** — HitArea with Touch=true matches at TouchMinTarget() distance
8. **Suppress bridging** — tree.suppress and global suppressClicksUntilRelease stay in sync during migration

Existing tests (`eq_slider_click_test.go`, `button_handler_test.go`, `slider_handler_test.go`, `input_isolation_test.go`, `mobile_overlay_tap_test.go`) continue to pass throughout migration — they exercise the public DrumView API which remains stable.

## Verification

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/... -v -count=1
```
