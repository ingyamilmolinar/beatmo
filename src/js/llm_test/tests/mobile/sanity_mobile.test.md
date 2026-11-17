---
name: sanity_mobile
platform: mobile
device: mobile-portrait
tags: [sanity, mobile, regression]
fixture: null
max_iterations: 90
viewport: 390x844
required_checkpoints: [init_rows, playing, stopped, bpm_90, subdiv_16, row1_muted, row1_unmuted, view_eq, view_synth, view_pads, zoomed_in, panned, node_deleted, audio_audible]
---

# Sanity: Mobile (portrait 390×844)

Exercises the mobile-specific surfaces end-to-end: compact transport, the bottom-nav views, inline
row controls + context menu, camera gestures, long-press node delete, and real audio capture. A
**LIVE DEMO STATE** block (prepended) lists the rows + node count — treat it as ground truth.

## UI reminders (mobile)
- Rows show `Label | Vol | M | S | FX` inline → Mute/Solo via `click_ui button="mute"/"solo" row=N`.
- The row context menu (`open_row_menu`) has Rename/Color/Origin/Delete (no Instrument, no Mute/Solo). Change the instrument by tapping the row **LABEL** (`click_ui button="label" row=N`) — that opens the instrument picker (a bottom sheet on mobile) on every platform.
- Bottom-nav views via `switch_view {slug}`; the State hint reports `viewMode`, `camScale`, `camOffset`, `nodes`.

## Rules
- Deterministic tools where possible (`click_ui`, `set_bpm`, `set_subdiv`, `switch_view`, `open_row_menu`/`menu_click`, `measure_audio`); `touch_gesture` for pinch/pan; `delete_node_longpress` for node delete.
- Gesture/camera checks are RELATIONAL vs a baseline you read from the State hint. 2-attempt retry then `ISSUE:` and move on. No page reload. **STOP after the summary.**

## Phase 1 — Transport
1. Screenshot. `checkpoint {name:"init_rows", expect:{totalRows: BASE_ROWS}}`.
2. `click_ui button="play"` → `checkpoint {name:"playing", expect:{isPlaying:true}}`; `click_ui button="stop"` → `checkpoint {name:"stopped", expect:{isPlaying:false}}`.
3. `set_bpm {value:90}` → `checkpoint {name:"bpm_90", expect:{bpm:90}}`; `set_subdiv {value:16}` → `checkpoint {name:"subdiv_16", expect:{subdiv:16}}`.

## Phase 2 — Rows (inline + context menu)
4. `click_ui button="mute" row=1` → `checkpoint {name:"row1_muted", expect:{row:1, muted:true}}`; `click_ui button="mute" row=1` → `checkpoint {name:"row1_unmuted", expect:{row:1, muted:false}}`.
5. (Exercise) `click_ui button="label" row=3` to open the instrument picker (bottom sheet) → `computer` tap a different instrument item; report `ISSUE` if the picker doesn't open. Then `open_row_menu {row:3}` and confirm its items are exactly Rename/Color/Origin/Delete (no Instrument); `menu_click {label:"Color"}` and observe — report `ISSUE` if a menu item is wrong.

## Phase 3 — Bottom-nav views
6. `switch_view {slug:"eq"}` → `checkpoint {name:"view_eq", expect:{viewMode:"eq"}}`. (Exercise) on EQ, toggle `click_ui button="chrome:k20"` or drag a band — observe.
7. `switch_view {slug:"synth"}` → `checkpoint {name:"view_synth", expect:{viewMode:"synth"}}`.
8. (Exercise) visit `wave`, `spectrum`, `levels`, `chain` with `switch_view`; note whether each renders data.
9. `switch_view {slug:"pads"}` → `checkpoint {name:"view_pads", expect:{viewMode:"pads"}}` (back to the rows).

## Phase 4 — Node delete (long-press popup) — do this BEFORE the camera gestures, at default zoom
10. Get an EXACT node pixel (don't eyeball — nodes are tiny): `read_export {export:"nodeRect", args:[0,0]}` → `{x,y,w,h}` (the demo has a node at grid 0,0; nodeRect returns its CURRENT on-screen rect, accounting for the camera). Note `nodes=` (N0).
11. `delete_node_longpress {x: <x + w/2>, y: <y + h/2>}` (use the rect's centre). `checkpoint {name:"node_deleted", expect:{totalNodes:{lt: N0}}}`. If it didn't drop, the press missed — re-read nodeRect and retry once at the fresh centre.

## Phase 5 — Camera gestures
12. Note `camScale=` (S0). `touch_gesture {gesture:"pinch_out", x:<grid cx>, y:<grid cy>}`. `checkpoint {name:"zoomed_in", expect:{camScale:{gt: S0}}}`.
13. Note `camOffset=(X0,Y0)`. `touch_gesture {gesture:"two_finger_pan", x:<grid cx>, y:<grid cy>, delta_x:100, delta_y:0}`. `checkpoint {name:"panned", expect:{camOffsetX:{ne: X0}}}`.

## Phase 6 — Audio capture
14. `measure_audio {name:"audio_audible"}` (records real playback, hard-verifies captured RMS>0.001).

Report any issues as `ISSUE:` lines, output a one-line summary, and STOP.
