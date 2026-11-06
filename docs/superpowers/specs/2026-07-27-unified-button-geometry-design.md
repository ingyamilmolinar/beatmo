# Unified button geometry — role-based corner radius + the anti-pill invariant

Date: 2026-07-27
Status: implemented

Two things were discovered during implementation and changed the design; both
are folded into the sections below and called out here:

1. **A second alignment bug.** `safeInsetTransport` clamped its inset budget
   across both axes at once, so a 1 px cell-width difference (grid layouts hand
   out leftover pixels one at a time) changed the *vertical* inset. Removing the
   record demote was not enough on its own — at 414 px wide, record and play
   still differed by 2 px. Fixed by clamping each axis independently
   (`clampInsetAxis`).
2. **`drawCapsule` routes THROUGH `drawRoundedRect`, not around it.** The first
   attempt gave capsules their own rasteriser path, which broke ~20 existing
   slider/thumb tests that intercept the `drawRoundedRect` func var to capture
   draw calls — they suddenly saw zero draws. Keeping exactly one interceptable
   chokepoint matters more than the convenience of a second entry point, so
   capsule intent is now carried by a sentinel radius (`radiusCapsule = -1`)
   that the clamp recognises. It cannot be reached by typing a plausible pixel
   value, so the escape stays deliberate.

## Problem

Three symptoms, two root causes.

### Symptom 1 — the mobile record button is short, with a seam above it

`transport_zone.go:786` deliberately shrinks the mobile record button by
`recordDemoteInsetMobile = 8` px on the top **and** bottom edges:

```go
if dy := recordDemoteInsetMobile; recordR.Dy()-2*dy >= recordDemoteFloorPx {
    recordR = image.Rect(recordR.Min.X, recordR.Min.Y+dy, recordR.Max.X, recordR.Max.Y-dy)
}
```

A pixel probe of a real 390×844 portrait capture confirms the effect. At the
record column (x≈51–74) the surrounding chip fill runs `y=394..447`, but the
button body only covers `y=400..439`. The two ~6 px bands left above and below
read as a horizontal line cutting across the top of the control's slot — the
reported bug.

The demote was added for critique item "B12": record should not sit at equal
visual weight with play/stop and invite an accidental mid-jam record. Geometry
was the wrong axis for that. Visual weight is already carried by tone — record
draws `colRecordIdle` (a muted red ring) against play's saturated green
triangle. Shrinking the box bought nothing and broke cluster alignment.

### Symptom 2 — controls render as pills

`RadiusMD = 12 px` is applied to controls that are 22–26 px tall. Since
`drawRoundedRect` clamps radius to `min(w,h)/2`, a 24 px-tall chip with a
requested radius of 12 lands exactly at the clamp and renders as a **full
pill**. Confirmed offenders in captured screenshots:

- the beat counter (`⊕ Beat 1 · 0:00`) — `drawFlatPanelButton` →
  `popupButtonRadius()` → `RadiusMD`
- the synth action bar — Preview / Save / Save As / Reset
- the row-rack control strip and the grid top-right corner buttons

Controls that already look right (audio tab strip, mobile bottom-nav keycaps)
are the ones using 4 px.

### Symptom 3 — the radius vocabulary invites the mistake

DESIGN.md's `rounded:` scale is raw t-shirt sizes (`xxs…xl`, plus `full: 999px`).
Nothing in the token names says which step a *button* should take, and the
accompanying prose actively pushes upward:

> The scale is bumped one notch over a "professional/tactical" baseline so the
> whole UI reads as cushioned/touchable rather than wireframe-strict.

Root causes: **(a)** no role layer over the raw scale, so call sites guess;
**(b)** the only geometric backstop is `min(w,h)/2`, which is the definition of
a pill rather than a guard against one.

## Design

### 1. Role tokens over the raw scale

Add a semantic layer to `rounded:` in the DESIGN.md YAML front matter, mirroring
the raw/semantic split the color tokens already use. The raw `xxs…xl` steps stay
as the values the role tokens resolve to.

| Role token | px | Applies to |
|---|---|---|
| `rounded.none` | 0 | Anything that tiles: grid cells, meter segments, track fills |
| `rounded.control` | 4 | **Every** interactive control — buttons, chips, tabs, keycaps, text inputs, synth controls, dropdowns, steppers |
| `rounded.container` | 8 | Popups, menus, cards, tooltips, panels, FX cards |
| `rounded.sheet` | 20 | Mobile bottom sheet and full-width overlays |

`rounded.full` (999 px) is **deleted**. It is already unreferenced in Go, and
the name is now forbidden — a pill is no longer a shape the system can express.

The generator (`src/go/cmd/gen_design_tokens`) emits one `genRounded<Key>` const
per `rounded:` entry, so the new keys produce `genRoundedNone`,
`genRoundedControl`, `genRoundedContainer`, `genRoundedSheet` with no generator
change. `touch_sizes.go` gains the matching exported aliases `RadiusNone`,
`RadiusControl`, `RadiusContainer`, `RadiusSheet`, and drops `RadiusFull`.

Components in the YAML reference the role tokens by name
(`rounded: "{rounded.control}"`), which the existing `roundedPx` resolver
already handles.

### 2. The anti-pill invariant

`drawRoundedRect` (`drawing.go:238`) is a single package-level func var — every
rounded corner in the app passes through it. It already clamps:

```go
maxR := minI(r.Dx(), r.Dy()) / 2
```

Change `/ 2` to `/ 4`. That one edit makes a pill unrepresentable at runtime:
a 24 px chip can never exceed 6 px, while a 250 px bottom sheet still gets its
full 20 px.

The clamp alone would silently paper over bad call sites, so enforcement is
split:

- **Clamp = runtime guarantee.** A pill cannot be drawn, whatever a call site asks for.
- **Test = source hygiene.** `TestNoPillShapedControls` replaces the
  `drawRoundedRect` func var with an interceptor (so it observes the
  **pre-clamp** requested radius), drives every scene in
  `internal/ui/scene_catalog.go` at both desktop and mobile profiles, and fails
  on any call where `radius > min(w,h)/4`. A call site that over-asks fails the
  test even though the clamp would have saved it on screen.

This is why the clamp is a backstop and not the design: controls land at 4 px
because `rounded.control` says so, not because they got clamped to 6.

### 3. Cluster alignment

Generalize the record-button bug into an invariant: **controls that share a
cluster share their vertical bounds.** Same `Min.Y`, same `Max.Y`, same `Dy`.

`TestClusterControlsShareBounds` asserts this for:

- the transport row (play, stop, record, BPM stepper, subdiv)
- the synth action bar (Preview, Save, Save As, Reset)
- the row-rack control strip (mute, solo, fx, kebab)
- the audio tab strip

Written first, red at HEAD on the transport row.

### 4. Removing the record demote

Delete `recordDemoteInsetMobile`, `recordDemoteFloorPx`, and the inset block in
`layoutMobile`. Record receives the same `safeInsetTransport(cell, pad)`
treatment as play and stop.

Two existing tests reference the demote and need updating rather than deleting,
since they still assert something real (record stays at or above touch-min):
`touch_sizes_test.go` (`TestMobileTransportButtons_AtTouchMin`, and the comment
at `:164`), `mobile_bottom_action_bar_test.go:182`, and
`transport_record_fix_test.go`.

### 5. Call-site sweep

Mechanical, driven by the token rename:

| Current | Becomes | Count |
|---|---|---|
| `button-*` specs at `{rounded.md}` | `{rounded.control}` | 11 |
| `button-*` / `input-*` specs at `{rounded.sm}` | `{rounded.control}` | 13 |
| `panel`, `fx-card`, `viz-canvas`, `splitter-handle-mobile` | `{rounded.container}` | 4 |
| `bottom-sheet` at `{rounded.xl}` | `{rounded.sheet}` | 1 |

Go-side:

- `popupButtonRadius()` returns `RadiusControl` — this alone fixes the beat
  counter and every other `drawFlatPanelButton` caller.
- `pillRadius := RadiusMD / 2` in `segmented_control.go:157` and
  `audio_sticky_bar.go:489` becomes `RadiusControl`. Same 4 px, but it stops
  hand-computing a radius from an unrelated token — the "reuse the styling
  infrastructure" part of the ask.
- Transport and file-ops group containers (`transport_zone.go:1766,1778`) use
  `RadiusContainer`.
- `game_node_sidebar.go:1525`, `game_group_menu.go:1088`,
  `logs_datetime_picker.go:259` at `RadiusLG` become `RadiusContainer`.

### 6. Elements the clamp must not break

The clamp is global, so genuinely circular elements need checking:

- the bottom-sheet drag handle pill (`menu_chrome.go:309`)
- the volume slider thumb (`thumbD/2` in the slider code)
- the node/signal graph geometry (`NodeUI.Radius`, `SignalUI.Radius` — these are
  a different `Radius` field on `NodeStyle`/`SignalStyle` and do **not** flow
  through `drawRoundedRect`, so they are unaffected)

Where an element is legitimately a circle or a capsule rather than a rounded
rect, it gets its own primitive (`drawCapsule`) rather than fighting the clamp.
Which of the two above need it is decided from the screenshots, not guessed
here.

## Testing

New:

- `TestNoPillShapedControls` — scene-sweep interceptor, both profiles.
- `TestClusterControlsShareBounds` — vertical-bounds invariant per cluster.
- `TestTransportRecordMatchesPlayStopBounds` — the specific regression, red at HEAD.

Updated:

- `design_md_drift_test.go` — want-map gains the four role tokens.
- `touch_sizes_test.go`, `mobile_bottom_action_bar_test.go`,
  `transport_record_fix_test.go` — drop demote references, keep touch-min asserts.

Existing guards that must stay green: `TestDesignMDDrift`, `TestTokenDiscipline`,
`TestNoBareRadiusLiterals`, `TestDesignMDLintSnapshot`.

## Verification

CLAUDE.md makes screenshot inspection mandatory for UI work. Capture before and
after at both profiles — transport, synth tab, node long-press menu, context
menu, EQ panel — open every PNG, and re-capture until it reads right. Green Go
tests do not close this out.

## Out of scope

- Changing button *colors*, fills, or the flat-tonal interaction deltas.
- The `spacing:` / `alpha:` scales.
- Icon geometry (`icon.radius: 2` is the icon canvas, not a control corner).
- Making icon-only buttons square. The mobile transport bar is too narrow to
  fit square controls at the 44 px touch minimum; this was considered and
  deliberately excluded.
