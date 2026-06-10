---
name: sanity_core
platform: desktop
tags: [sanity, core, regression]
fixture: null
max_iterations: 85
viewport: 1280x720
required_checkpoints: [init_rows, playing, stopped, bpm_90, subdiv_16, nodes_built, row1_muted, row1_unmuted, row2_soloed, row2_unsoloed, instrument_changed, fx_added, roundtrip_restored, audio_audible]
---

# Sanity: Core Game Mechanics + Audio (Desktop)

Exercises the main non-panel surfaces: transport, graph editing, rows, import/export, and real
audio capture. A **LIVE DEMO STATE** block (prepended) lists the actual rows (0-indexed, names + ids)
and node count — treat it as ground truth; map "row N" to it. Let `BASE_ROWS`/`BASE_NODES` be its
Total rows / Total nodes.

## Rules
- Prefer named/deterministic tools: `click_ui` (real click), `repeat_click_ui`, `set_bpm`, `set_subdiv`,
  `open_row_menu`/`menu_click`, `export_circuit`/`import_circuit`, `measure_audio`. Use `computer` only for
  grid clicks/drags. Verify read-backs with `read_export`/`checkpoint_export`.
- After each change, record its checkpoint. The named `required_checkpoints` are the pass/fail signal.
- 2-attempt retry then `ISSUE: [P0|P1|P2|P3] ...` and move on. No page reload. **STOP after the summary.**

## Phase 1 — Transport
1. Screenshot. `checkpoint {name:"init_rows", expect:{totalRows: BASE_ROWS}}`.
2. `click_ui button="play"` → `checkpoint {name:"playing", expect:{isPlaying:true}}`; `click_ui button="stop"` → `checkpoint {name:"stopped", expect:{isPlaying:false}}`.
3. `set_bpm {value:90}` → `checkpoint {name:"bpm_90", expect:{bpm:90}}`; `set_subdiv {value:16}` → `checkpoint {name:"subdiv_16", expect:{subdiv:16}}`.

## Phase 2 — Graph editing
4. `query_ui` for `gridPane`. `computer` left-click 3 empty spots inside it to create nodes; read `nodes=` from the State hint after each. `checkpoint {name:"nodes_built", expect:{totalNodes:{gt: BASE_NODES}}}`.
5. (Exploratory) Shift-drag from a new node to another to add an edge; click a node to open its sidebar and set Logic→Skip Every N; report `ISSUE` if anything misbehaves. (Not gated.)

## Phase 3 — Rows
6. `click_ui button="mute" row=1` → `checkpoint {name:"row1_muted", expect:{row:1, muted:true}}`; `click_ui button="mute" row=1` → `checkpoint {name:"row1_unmuted", expect:{row:1, muted:false}}`.
7. `click_ui button="solo" row=2` → `checkpoint {name:"row2_soloed", expect:{row:2, soloed:true}}`; `click_ui button="solo" row=2` → `checkpoint {name:"row2_unsoloed", expect:{row:2, soloed:false}}`.
8. **Instrument** (desktop opens the selector by clicking the LABEL — the kebab menu has no "Instrument"): `read_export {export:"rowInstrument", args:[3]}` (baseline B). `click_ui button="label" row=3` to open the instrument selector popup; pick a DIFFERENT instrument from the list (use `computer` to click an item). `checkpoint_export {name:"instrument_changed", export:"rowInstrument", args:[3], expect:{ne: B}}`.
9. **FX** (FX is the inline `fx` button on desktop — not in the kebab menu): `read_export {export:"rowInstrument", args:[0]}` to get row 0's id ID0, and `read_export {export:"getInsertEffects", args:[ID0], path:"length"}` (baseline N). `click_ui button="fx" row=0` to open the FX panel → add **Reverb** via "+". `checkpoint_export {name:"fx_added", export:"getInsertEffects", args:[ID0], path:"length", expect:{gt: N}}`. Close the panel.
10. (Exploratory) `open_row_menu {row:1}` → `menu_click {label:"Color"}` (Color IS in the desktop kebab menu) → tap a different color on the wheel; report ISSUE if it doesn't change. (Not gated.)

## Phase 4 — Import/export roundtrip
11. `export_circuit` (records orig rows/nodes/bpm/subdiv). `click_ui button="addRow"` (mutate). `import_circuit`. `checkpoint {name:"roundtrip_restored", expect:{totalRows: BASE_ROWS}}` (the added row is gone after restore).

## Phase 5 — Audio capture
12. `measure_audio {name:"audio_audible"}` — records real playback and hard-verifies the captured master RMS > 0.001.

Report any issues as `ISSUE:` lines, output a one-line summary, and STOP.
