# Lock/Free Timeline Keycap — Consistency Pass

**Date:** 2026-07-22
**Status:** Approved (design)
**Area:** `internal/ui` — timeline header band (transport / timeline zone)

## Problem

The timeline **lock/free toggle** (`trackBtn`, aka the track/follow keycap — closed
padlock = "tracking, timeline follows playhead", open padlock = "free scroll") is
visually inconsistent with the rest of the transport chrome:

- **Desktop:** the keycap is cramped and the padlock glyph reads as *tiny*. The
  glyph (`drawTrackIcon` / `drawTrackOffIcon`) is a deliberately-slim 9px-wide body
  on a 24px icon grid, so even inside an 18%-margin keycap it carries far less
  visual weight than the neighbouring `IconTarget` on the beat-counter keycap or
  the play/stop/record transport icons.
- **Mobile:** the keycap grows to a **44px-tall square** (touch-min met by
  geometric inflation in `drumview_layout.go` — `bottom = min.Y + TouchMinTarget()`),
  while the adjacent beat-counter pill stays ~26px tall and rounded. The two are
  mismatched in height; the lock reads as a flat, oversized box floating between
  the volume speaker and the counter rather than a matched keycap.

Net effect: the control does not read as "a button in the same family" and does not
match the DESIGN.md keycap-elevation / icon treatment used everywhere else.

## Root causes

1. **Slim glyph.** The padlock body (`x 7.5..16.5`, 9 wide) and narrow shackle
   occupy only ~37% of the 24px icon canvas width → looks tiny at any keycap size.
2. **Divergent chrome & height.** The track button renders via generic `Button.Draw`
   with `TransportMiscStyle`, and its rect height is derived independently
   (desktop = band height; mobile = 44px). The beat-counter keycap, by contrast, is
   drawn by `drawBeatCounter` via `drawKeycapPanelButton` at a `pillH` derived from
   text height + pad + wall depth, vertically centred in the band. The two never
   share a height, so they never visually pair.
3. **Touch-min by bloat.** Mobile inflates the *visible* keycap to 44px instead of
   using the `HitArea.Touch=true` + `ClipRect` skirt the zone-input-isolation
   contract prescribes (CLAUDE.md: "Don't physically enlarge a zone's hit-area Rect
   … use `HitArea.Touch` + `ClipRect`").

## Goals

- The lock/free control reads as an **icon-only matte keycap** that is visually
  identical in chrome and height to the beat-counter keycap it sits beside, and
  in the same family as the play/stop transport keycaps — on **both** desktop and
  mobile.
- The padlock icon carries visual weight comparable to `IconTarget` and the
  transport icons; closed=tracking / open=free silhouettes preserved.
- Touch target ≥ 44px on mobile **without** inflating the visible keycap.
- No new design tokens; no change to toggle semantics or colours.

## Non-goals

- No change to what the toggle *does* (follow-playhead on/off), its callback wiring,
  or its position order (still the left anchor of the header band, left of the
  beat-counter keycap).
- No change to the beat-counter keycap itself beyond it remaining the height
  reference the lock keycap matches.
- No text label on the control (icon-only, per approved treatment).

## Design

### 1. Bolder padlock glyph

Widen the lock body and shackle in `drawTrackIcon` / `drawTrackOffIcon`
(`drawing_icons.go`) so the closed/open padlock fills its canvas with weight
comparable to `IconTarget`:

- Body widened from 9px to ~12–13px wide (e.g. `x 5.5..18.5`), keeping it centred
  on the 24-grid and vertically where it is (`y ~11..20`).
- Shackle scaled to match the wider body (wider arch + legs seating into the new
  body top), preserving the closed (seated both legs) vs open (right leg lifted,
  Lucide-unlock) silhouettes.
- Keyhole re-centred/scaled to the wider body.

Both glyphs stay single-colour vector strokes (no new tokens; icon colour continues
to come from `IconColor` set by `syncTrackBtnVisual`). Existing icon unit/snapshot
guards (if any reference these draw funcs) are updated as needed.

### 2. Unified keycap chrome + matched height

Render the track/lock keycap so it is chrome- and height-identical to the
beat-counter keycap:

- **Height:** size the track keycap's visible rect to the **same pill height** the
  beat counter uses (`th + 2*beatCounterPillPadY + genGeomKeycapWallDepth`, clamped
  to band height), vertically centred in the header band — on both platforms. This
  replaces the mobile "grow to 44px" branch and the desktop "match band height"
  branch in `drumview_layout.go` with a single band-centred pill height matching the
  counter.
- **Chrome:** the keycap continues to use `TransportMiscStyle` fill/border (the same
  fill/border the beat counter passes to `drawKeycapPanelButton`) with hover-lift, so
  the two caps are pixel-consistent. If `Button.Draw`'s keycap treatment already
  matches `drawKeycapPanelButton` closely enough, keep rendering via the `Button`;
  otherwise route the track button's draw through the same keycap helper so corner
  radius / wall depth / hover exactly match. (Decision made during implementation by
  direct pixel comparison in the crop scene — the acceptance bar is "the two caps are
  indistinguishable in chrome and height".)
- **Width:** unchanged in intent (left-anchor strip); may narrow slightly to a
  square-ish icon keycap now that the height matches the counter, so it reads as a
  compact square cap rather than a wide slab.

### 3. Touch-min via hit-area skirt (mobile)

Meet the 44px finger target by expanding the **hit area**, not the visible keycap:

- In the track button's hit-area registration (`timeline_zone.go` `HitAreas()`),
  when `Profile().IsMobile()` set `HitArea.Touch = true` and a `ClipRect` that
  inflates the button rect to ≥ `TouchMinTarget()` (bounded skirt), rather than the
  layout growing the visible rect.
- The visible keycap stays at the beat-counter pill height; the touch skirt is
  invisible and bounded, and cannot leak into a sibling zone (it clips to the header
  band). This follows `timelineScrubHitAdapter` / the input-isolation precedent.

### 4. Semantics / colour (unchanged)

`syncTrackBtnVisual` keeps its two-state model: active (tracking) =
`colFollowActive` (bright neutral), free = `colTextDisabled` (dim neutral), same
chrome both states — mirroring the play/stop icon on/off model. No token changes.

## Components touched

| File | Change |
|---|---|
| `internal/ui/drawing_icons.go` | Widen `drawTrackIcon` / `drawTrackOffIcon` glyphs. |
| `internal/ui/drumview_layout.go` | Track keycap rect → beat-counter pill height, band-centred, both platforms (remove mobile 44px inflation). |
| `internal/ui/timeline_zone.go` | Track button hit-area gains mobile `Touch`+`ClipRect` skirt; (optional) route track button draw through `drawKeycapPanelButton` if `Button.Draw` chrome doesn't match. |
| `internal/ui/transport_zone.go` | Only if keycap draw routing changes (syncTrackBtnVisual stays). |
| `internal/ui/scene_catalog.go` | Add a desktop crop scene pairing the lock keycap + beat counter for regression capture. |
| `internal/ui/screenshot_subjects.go` (+ owning accessor) | Subject rect for the new crop scene if a `crop_*` subject is added. |

## Verification (MANDATORY — UI change)

Per CLAUDE.md's UI verification loop, screenshots must be captured and inspected:

1. **Desktop:** `transport_idle`, `transport_playing` — lock keycap matches the
   beat-counter keycap in chrome + height; padlock legible and weighty; tracking vs
   free states distinguishable.
2. **Mobile:** `mobile_track_chip_following`, `mobile_track_chip_free` — lock keycap
   aligned with the beat-counter pill (matched height, centred), not a floating
   oversized box; still finger-tappable (hit-area skirt).
3. **New crop scene** (desktop, and mobile if applicable) capturing the
   lock-keycap + beat-counter pair as an ongoing regression surface.
4. Confirm no overlap/clipping in the header band at both profiles; re-capture until
   correct.

Automated guards to keep green: `scene_crop_test.go` (new crop scene's subject rect
fits and is smaller than full screen), `zone_input_isolation_discipline_test.go`
(track button catch-all/skirt discipline), `token_discipline_test.go` /
`design_md_drift_test.go` (no new literals/tokens), plus the fast `-tags test`
`internal/ui` suite.

## Risks / trade-offs

- **Glyph guards:** widening the padlock may trip an icon pixel/snapshot test if one
  pins these draw funcs — update alongside.
- **Height reference coupling:** the track keycap now derives its height from the
  beat-counter pill formula; if that formula changes later both caps move together
  (intended — they are a pair).
- **Hit-area skirt:** must clip to the header band so the enlarged mobile touch
  target cannot reach the transport speaker/beat-counter hit areas (verify via the
  input-isolation discipline test).
