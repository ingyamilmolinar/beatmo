# Mobile-friendly scrollable knob grid: drag-vs-tap + scroll affordances (Synth + Sampler)

- **Date:** 2026-07-18
- **Branch:** add-core-node-types (builds on `2026-07-18-popup-input-handling-design.md`)
- **Status:** DESIGN — awaiting user review before planning.
- **Package:** `src/go/internal/ui`

## Problem

On the Synth and Sampler tabs the per-stage **knob grid** (`ControlGrid` in the detail
pane) is a vertically scrollable list, but on mobile the scroll barely works and there
is no visual signal that the grid *is* scrollable.

Two concrete defects:

1. **A finger that lands on a knob cannot scroll.** Knob hit areas are published at
   `ZEQPanel+2`, above the scroll-body catch-all at `ZEQPanel+1`. `synthControlHitAdapter.OnPress`
   (and the identical `samplerKnobHitAdapter.OnPress`) **opens the value-wheel popup on
   press and returns `InputConsumed`** (`synth_panel_zone.go:3569-3583`,
   `sampler_panel_zone.go:1493-1505`). So a press that begins on a knob is consumed as a
   popup-open; it can never become a scroll. Scrolling only engages when the press starts
   on empty card whitespace (the `synthSectionScrollAdapter` / `samplerKnobScrollAdapter`
   body catch-all), which is rare on dense mobile cards. There is **no tap-vs-scroll
   disambiguation** on the knob path — unlike the FX panel, context menu, and every
   `MenuScroll` list, which use `DeferredTap` + `ScrollingCommitted()`.

2. **No affordance that the grid is scrollable.** The scrollbar draws only when
   `HasScroll()` and is a thin track; there is no second cue (fade edge / peek row) telling
   the user more controls exist below.

## Root-cause detail

- The knob adapters open the popup on the **physical down** frame and return
  `InputConsumed`, so `DrumViewTree.capturedHandler` stays `nil`. During the injected
  tap replay (`injectTouchTap`, mobile), the injection-edge clear at
  `drumview_tree.go:588` requires `capturedHandler == nil` — it fires and, combined with
  the popup's anchor-tap-to-close, is exactly the open-then-close family the sibling spec
  (`2026-07-18-popup-input-handling-design.md`, Part A) already fought with the
  `portalOpenedThisPress` guard.
- The `ControlGrid` scroll uses the opt-in **step-drag** path
  (`ControlGrid.BeginContentDrag`/`DragTo`, one row per `controlGridDragStepPx=44` px). It
  deliberately has no momentum/`TouchScroller`. That is fine to keep — the missing piece is
  the *decision* between "this gesture is a tap on a control" and "this gesture is a scroll,"
  not the scroll mechanics.

## Design

Two independent changes, both landing in shared `ControlGrid`-adjacent code so the Synth
and Sampler tabs (both route through `ControlGrid`) get them from one place.

### Part 1 — Drag-vs-tap arbitration on the knob path (the core fix)

Change the knob adapters from **open-on-press / `InputConsumed`** to
**capture-on-press / decide-on-release**, gated by the standard `tapMaxMovePx` (~10 px)
slop that every other scrollable surface already uses.

New tiny shared helper (`drag_scroll_arbiter.go`) — deliberately **not** named `tapGate`
(that name is the Part-B mobile tap *debounce* on `DrumView`) nor `DeferredTap` (that fires
a stored position; here we drive a live scroll):

```go
// dragScrollArbiter decides, over one press gesture, whether the finger tapped a
// control or swept to scroll the list it sits in. Bound to one *ControlGrid for the
// gesture's lifetime. Zero value is ready.
type dragScrollArbiter struct {
    startY   int
    scrolled bool   // travel crossed the tap slop → this is a scroll, not a tap
    active   bool
}

func (a *dragScrollArbiter) begin(y int, g *ControlGrid) {
    *a = dragScrollArbiter{startY: y, active: true}
    if g != nil { g.BeginContentDrag(y) } // natural: content follows finger
}
func (a *dragScrollArbiter) drag(y int, g *ControlGrid) (scrolled bool) {
    if !a.active { return false }
    if abs(y-a.startY) >= tapMaxMovePx { a.scrolled = true }
    if g != nil { return g.DragTo(y) }
    return false
}
// end returns wasTap: true when the gesture never crossed the slop (open the control).
func (a *dragScrollArbiter) end(g *ControlGrid) (wasTap bool) {
    wasTap = a.active && !a.scrolled
    if g != nil { g.EndDrag() }
    a.active = false
    return wasTap
}
```

Knob adapter changes (`synthControlHitAdapter`, `samplerKnobHitAdapter`), each holding one
`dragScrollArbiter` value and looking up its owning grid (`sectionGrid(selectedSection)`
for synth, `knobGrid` for sampler):

- **`OnPress`** → `arbiter.begin(y, grid)`; return **`InputCaptured`**. Do **not** open the
  popup. (Numeric-editor-open and paramEditor-active early-outs stay as `InputConsumed`.)
- **`OnDrag`** → `if arbiter.drag(y, grid) { invalidate() }` so the next Layout re-windows.
- **`OnRelease`** → `if arbiter.end(grid) { open the value-wheel popup for idx }`. A
  committed scroll opens nothing.
- **`OnWheel`** → unchanged (value nudge on desktop).

**Why this is correct on mobile (verified against the dispatch code):**

- *Live scroll* (finger sweeps): classified `GestureSingleFingerDrag`, mapped to mouse by
  the touch override. `OnPress`→`OnDrag`(scrolls, crosses slop)→`OnRelease`(scroll, no
  popup). `injectTouchTap` never fires for a drag gesture, so there is no replay.
- *Tap* (finger stationary): whether the press is seen live-then-replayed or synthesised
  entirely by `injectTouchTap`, the knob's `capturedHandler` stays set for the whole
  gesture. Because line 588's injection-edge clear requires `capturedHandler == nil`, the
  replay is **not** re-armed as a fresh press over the just-opened popup — so the
  open-then-close bug the sibling spec fixed **cannot occur on this path**, without adding
  any new guard. The popup opens once, on the (possibly injected) `OnRelease`, and stays.
- *Desktop*: a click = press+release with no travel = tap = popup opens on release (matches
  how `MenuScroll` menus already fire); a click-drag scrolls; wheel still nudges. The only
  behaviour change is open-on-release instead of open-on-press, consistent with the
  DeferredTap doctrine used everywhere else in the codebase.

The body catch-all adapters (`synthSectionScrollAdapter`, `samplerKnobScrollAdapter`) stay
for presses on empty card space and are unchanged — they already capture and step-drag the
same grid, so both entry points drive one scroll.

### Part 2 — Scroll affordances: fade edges + peek row (draw-only)

All in `ControlGrid` + the two detail-pane draw sites, so both tabs share it. No new design
tokens: sizes derive from the existing `rowH` and existing spacing tokens (per
minimal-UX / three-axis discipline — reserve tokens only if a value genuinely diverges).

**Peek row.** When the grid overflows, `Layout` reserves a bottom **peek band** of
`peekH ≈ 0.4 * rowH` so `visRows = (rect.Dy() − peekH) / rowH` (floored, min 1; if there is
no room for a peek, skip it and behave as today). The window height stays constant while
scrolling (computed whenever the grid overflows at all), avoiding reflow jitter. A new
`PeekRect(i) (image.Rectangle, bool)` returns the **clipped top-sliver rect** for the first
off-window row's cells; the detail-pane draw loop renders those knobs into a clipped
subimage. **The peek row is draw-only — it publishes no hit area** (no half-tappable knobs;
the user scrolls it fully in, then taps). When scrolled to the bottom (`First == maxFirst`)
there is no next row, so nothing is drawn in the band.

**Fade edges.** Using the existing `fillVerticalGradient` (drawing.go:59), the draw sites
paint:
- a **bottom fade** over the peek band (bg-transparent → bg-opaque, top→bottom) whenever
  more rows exist below, so content dissolves into the panel — the presence of the fade *is*
  the "more below" signal;
- a **top fade** (bg-opaque → bg-transparent) whenever `First > 0`.

Fade height ≈ the peek band. Both fades hide at their respective ends of travel.

**Scrollbar.** Unchanged mechanism (`ControlGrid.Draw` → `ScrollBehavior.Draw`), kept as the
explicit second cue; the spec verifies it renders on mobile whenever `HasScroll()`.

### Scope

Both tabs go through `ControlGrid`, so Part 2 is one change in `control_grid.go` +
`drawSynthDetailPane` and the sampler detail draw. Part 1 touches the two knob adapters and
adds the small `drag_scroll_arbiter.go`. Reuses `fillVerticalGradient`, `tapMaxMovePx`, and
the existing step-drag scroll — no new input or drawing primitives, no new tokens.

## Non-goals

- No change to popup *positioning* (below-anchor placement stays) or the anchor
  tap-to-close *feature* (a deliberate second tap on a knob still toggles its popup).
- No momentum / `TouchScroller` on the knob grid — step-drag feel is retained.
- No change to chips (they wrap, never scroll) or the enable pill.
- No new design tokens; sizes derive from `rowH` + existing spacing.
- The sibling spec's `portalOpenedThisPress` guard is **not** modified — capture-on-press
  makes the knob path self-guarding; the guard still protects other press-opened popups.

## Testing (TDD, functional — drive real `Update()`)

Per repo convention, drive the real `DrumView.Update()` / tree dispatch rather than poking
internals ([[feedback_functional_tests_over_isolation]]). Reuse
`mobileSynthWheelSceneSetup` (`synth_wheel_popup_isolation_test.go`) and the sampler card
harness (`sampler_card_test.go`).

**Part 1 (both tabs):**
- Tap on a knob (press + release, no travel) → wheel popup opens (RED first: today it opens
  on press; assert open-on-release).
- Sweep on a knob (press, travel > `tapMaxMovePx`, release) → the grid's `VS.First`
  advanced **and** the popup did **not** open.
- Dead-zone boundary: travel < `tapMaxMovePx` → still a tap (popup opens).
- Mobile open-then-close regression: a clean injected tap on a knob leaves the popup **open**
  after settle (guards that capture-on-press keeps line 588 suppressed).
- Body-whitespace press still scrolls (unchanged path stays green).
- Update `sampler_card_test.go` / `param_card_input_test.go`: press→**release** to open
  (they currently assert open-on-press).

**Part 2 (both tabs):**
- With an overflowing stage, `PeekRect` returns a visible clipped rect for the first
  off-window row and **that row publishes no knob hit area**.
- Fades: top fade absent at `First==0`, present after scrolling down; bottom fade present
  while more rows exist, absent at `First==maxFirst`.
- Scrollbar renders (drawRect interceptor / composited-pixel read) on mobile when
  `HasScroll()`.
- Window size (`visRows`) is stable across scroll positions (no reflow jitter).

## Verification commands

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  -run 'SynthWheel|Sampler|ControlGrid|Scroll|ParamCard' ./internal/ui/
# Full ui suite ~600s; run in -run splits under `timeout 1500`.
GOOS=js GOARCH=wasm ../../.tools/go/bin/go build ./...   # confirm wasm still builds
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
```

## Risks / gotchas (carried from this branch's prior sessions)

- **Git is read-only.** Never stash/add/commit — leave everything uncommitted for user
  review ([[feedback_never_stage_or_commit]]). A prior subagent `git stash -u` deleted
  2,614 tracked files; do edits inline, not via a subagent, while the tree is edited in
  parallel ([[feedback_subagents_unsafe_in_concurrent_tree]]).
- User WIP files are not gofmt-1.26-clean — format only files this change touches; never run
  package-wide `gofmt -w`.
- Full `internal/ui` suite ~600s, trips 900s only under `./...` CPU contention; run targeted
  `-run` splits under `timeout 1500`.
- This branch has pre-existing unrelated RED tests (node-logic volume; instrument-menu
  `-run` ordering leak; `audio/songrender` wasm build). Prove causation before attributing
  any failure to this change ([[feedback_no_preexisting_failure_baselines]]).
- Reserving `peekH` changes `visRows` — audit `sampler_scroll_test.go` and any test asserting
  a specific visible-row count; update with a one-line rationale.
- Per-frame alloc budgets on the Synth tab are tight (`synth_live_param_edit_alloc_test.go`);
  the peek draw must not add per-frame allocation on the hot path.
```