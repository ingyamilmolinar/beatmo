/**
 * LLM Visual Testing — Agent Loop
 *
 * Drives a Claude Haiku agent that visually interacts with Beatmo in the browser.
 * The agent takes screenshots, sends them to Claude via the computer use API,
 * receives click/type/scroll actions, and executes them via Playwright.
 *
 * Supports two platforms:
 *   - "desktop" (default): Standard mouse/keyboard interaction
 *   - "mobile": Mouse actions auto-translate to CDP touch events; adds touch_gesture tool
 *
 * Usage:
 *   const agent = createAgent(page, { recorder });
 *   const result = await agent.run("Click Play, wait 3 seconds, click Stop.");
 */

import {
  cdpTap,
  cdpLongPress,
  cdpPinch,
  cdpDrag,
  cdpTwoFingerPan,
  getCanvasInfo,
} from "../touch_cdp_helpers.js";
import fs from "fs";
import { compareCheckpoint, matchesExpectation } from "./checkpoints.js";
import { normCoord, normalizeComputerAction } from "./action_normalize.js";
import { parseZip, decodeWav, rms } from "./recording_decode.js";

const SYSTEM_PROMPT = `You are testing a drum machine web app called Beatmo.

The screen is split into two panes:
- GRID PANE (top ~40%): 2D grid of colored square nodes connected by edges with directional arrows. Nodes glow when "fired" during playback.
- DRUM PANE (bottom ~60%): Contains the transport bar, instrument rows, and a tabbed AUDIO PANEL.

DRUM PANE layout (top to bottom):
- TRANSPORT BAR (single row at top of drum pane):
  Left to right: Play (triangle), Stop (square), Rec (record dot), BPM input (text box), BPM +/- (stacked arrows), volume icon, Subdiv button, Length +/-, Track/Follow, Upload, Import/Export (share), a lock icon, and a "Beat N · time" readout. Master volume is a small slider/icon on desktop.
- INSTRUMENT ROWS: each row shows, left to right: instrument LABEL, a VOLUME slider (small speaker icon + track), M (mute), S (solo), FX, and a "⋯" KEBAB (overflow) button.
  IMPORTANT: color, rename/edit, origin, and delete are NOT inline buttons anymore — they live inside the row's ⋯ overflow menu. Use the open_row_menu + menu_click tools to reach them (see TOOLS).
- ADD ROW (+) button below the last row.
- AUDIO PANEL (bottom): a tabbed analysis panel with SEVEN tabs — EQ, Wave, Spectrum, Levels, Chain, Synth, Sampler. The left chip is the CHANNEL selector ("Master" or an instrument); HP/LP filter buttons sit next to it. The EQ tab has 10 draggable band handles; other tabs show waveform / spectrum / meters / signal-chain / synth-knob / sampler views. Switch tabs with the switch_tab tool.

IMPORTANT: Play is a small triangle in the transport bar, directly below the grid pane; Stop is the square immediately to its right. Do NOT confuse them with the audio-panel controls at the very bottom or with instrument labels in the drum rows.

You can click grid cells to add/remove nodes, shift-drag to create edges. Click a node to open its sidebar (parameters + logic).

IMPORTANT — COORDINATE HINTS:
After every screenshot, you receive text with exact center coordinates for ALL buttons, per-row controls, EQ controls, scrollbar, and UI state. These are pixel-perfect. ALWAYS use these coordinates instead of guessing from the screenshot.

Format: "Buttons: play=(33,337) | stop=(77,337) | ..."
Per-row: "Row0: label=(50,400) | mute=(100,400) | menu=(300,400) | ..."
EQ: "EQ: channelBtn=(x,y) | hpfBtn=(x,y) | band0=(x,y) | ..."
State: "State: playing=true bpm=120 rows=6"

You can also call the query_ui tool to get the full layout snapshot as JSON (rects {x,y,w,h} for every element, plus snap.tabs, snap.bottomNav, and snap.state with bpm/subdiv/activeTab/viewMode/channel and per-row muted/soloed).

CONSTRAINTS:
- The BPM field is an Ebiten-rendered text input. Click it to focus, select all (Ctrl+A), type a new value, and press Enter. This is faster than clicking +/- repeatedly.
- Volume / master sliders respond to mouse drag, not click.
- EQ band handles are draggable; drag up/down to change gain.

TOOLS — prefer these deterministic tools over pixel-clicking:
- click_ui {button, row?}: click a named button at exact pixel coords. Global: play, stop, bpmInc, bpmDec, subdiv, lenInc, lenDec, track, addRow, upload, import, export, mainVol, eqToggle. EQ: eqChannel, hpf, lpf. Per-row (pass row, 0-based): label, mute, solo, fx, volume, menu. Tabs/nav fallbacks: "tab:wave", "nav:eq".
- repeat_click_ui {button, count, row?}: click N times in one round-trip (BPM +/-, etc.).
- open_row_menu {row}: open a row's ⋯ overflow menu and get its item labels. THIS is how you reach color/rename/origin/delete.
- menu_click {label}: click an item in the open row menu by exact label.
- set_bpm {value}: set BPM to an exact value DETERMINISTICALLY. ALWAYS use this for BPM — do NOT type into the BPM field (keyboard entry does not reliably commit headless).
- set_subdiv {value}: set the subdivision (4/8/16/32) DETERMINISTICALLY. ALWAYS use this — do NOT tap the subdiv dropdown items.
- switch_tab {slug}: switch the audio panel to eq | wave | spectrum | levels | chain | synth | sampler.
- export_circuit / import_circuit: capture the circuit (returns totalRows/totalNodes/bpm/subdiv) and re-import it later — for export/import roundtrip checks.
- drag {x0,y0,x1,y1}: DESKTOP real force-ticked drag for EQ band handles / synth knobs / sliders (read the handle rect from query_ui first).
- read_export {export, args?, path?}: read a WASM getter value (record a baseline before an interaction).
- call_export {export, args?}: invoke a WASM SETTER through its real handler + apply it. Use for continuous controls a pixel-drag can't move headless (EQ band gain via setEQBandGain, synth/sampler params via setInstrumentParam), then verify with checkpoint_export.
- checkpoint_export {name, export, args?, path?, expect}: assert a WASM getter value (exact or relational) and record a checkpoint — THIS is how you verify per-tab interactions resiliently (e.g. an EQ band drag changed eqControlsSnapshot().gainsDB[i]).
- measure_audio {name?}: record real playback + hard-verify the captured audio RMS > floor (audio-capture gate).
- checkpoint {name, expect}: assert objective state and get an immediate PASS/FAIL. USE THIS to prove every state change (see CHECKPOINTS).

AUDIO-PANEL CHROME: tab-chrome pills are clickable by name with click_ui — "chrome:slope", "chrome:pre",
"chrome:k20", "chrome:clearClips", "chrome:logFreq", "chrome:resetHold" (sticky bar) and
"chrome:overlay"/"chrome:split"/"chrome:diff"/"chrome:ag"/"chrome:freeze" (Chain). Verify the effect with
checkpoint_export against the matching getter (spectrumSlope/preOverlay/k20View/freqScaleLog/chainDisplayMode/autoGain/scopeFrozen).
- query_ui: full layout JSON. computer: grid clicks, drags, slider drags, scroll (only where no deterministic tool exists).

The State hint line each turn reports: playing, bpm, rows, nodes, subdiv, camScale, camOffset.
checkpoint expectations may be an exact value OR a relational object — {gt|lt|gte|lte|ne: N} —
for results that are directional, not exact (e.g. a pinch must change camScale, a delete must
drop nodes): read the current value from the State hint, perform the action, then assert e.g.
checkpoint {name:"zoomed_in", expect:{camScale:{gt: <value you read>}}}.

EFFICIENCY RULES (avoid wasting iterations/money):
- Prefer the deterministic tools (click_ui, set_bpm, set_subdiv, switch_tab/switch_view, open_row_menu/menu_click) over the raw computer tool. The computer tool is only for grid/canvas interactions, slider drags, and the color wheel.
- If an action does not take effect after 2 attempts, record an ISSUE and MOVE ON — never loop retrying the same action.
- When the checklist is complete, output the Final Summary and STOP (end your turn). Do NOT keep exploring or taking extra screenshots once the required steps are done.

CHECKPOINTS (how you prove the app works):
After any action that should change state, call checkpoint with the expected values. Examples:
- after Play: checkpoint {name:"playing", expect:{isPlaying:true}}
- after setting BPM 90: checkpoint {name:"bpm_90", expect:{bpm:90}}
- after subdiv→16: checkpoint {name:"subdiv_16", expect:{subdiv:16}}
- after muting row 1: checkpoint {name:"row1_muted", expect:{row:1, muted:true}}
- after switch_tab wave: checkpoint {name:"tab_wave", expect:{activeTab:"wave"}}
The test gate requires every named checkpoint the task asks for to PASS. If a checkpoint FAILs, the verdict text tells you the actual value — retry the action or report an ISSUE.

NODE SIDEBAR:
- Click a grid node to select it and open the sidebar panel.
- The sidebar has collapsible sections (Volume, Pitch, Duration, Logic, Groove, Audible). Click a section header to expand.
- The Logic section has a dropdown showing logic-kind options. After changing logic, the drum row's predicted step pattern updates automatically.

Always report any visual bugs you notice: misaligned elements, overlapping text,
broken rendering, unresponsive buttons, missing visual feedback, etc.
Prefix issues with "Bug:" for functional problems or "Glitch:" for visual issues.

STRUCTURED BUG REPORTING:
When you find an issue, report it on its own line in this exact format:

  ISSUE: [P0|P1|P2|P3] [Bug|Glitch|Suggestion] - <description>

Severity guide:
- P0 (Critical): Crash, data loss, feature completely broken
- P1 (Major): Feature partially broken, wrong behavior, blocks workflow
- P2 (Minor): Cosmetic issue affecting usability (clipped text, hard-to-click button)
- P3 (Trivial): Minor visual polish (slight misalignment, color inconsistency)

Report issues inline as you discover them. Do NOT wait until the final summary.

When you have completed the requested task, provide a summary of what you did,
what you observed, and any issues found. Then stop.`;

const MOBILE_SYSTEM_PROMPT_SUFFIX = `

This is a MOBILE test running on a touchscreen device.
- The layout is stacked (grid on top, drums on bottom).
- Your mouse clicks are translated to touch taps automatically.
- For MULTI-TOUCH gestures (pinch zoom, two-finger pan), use the touch_gesture tool.
- Available gestures: tap, long_press, pinch_in, pinch_out, two_finger_pan, swipe.
- Touch targets are larger than desktop (44px row height, 44px min target).

MOBILE TRANSPORT (compact, single row):
  Play | Stop | BPM input | BPM ± | volume | Subdiv | Len ± | Overflow (⋯)
  No master volume slider on mobile.

MOBILE BOTTOM-NAV (this replaced the old binary ViewSwitch):
  A segmented control across the very bottom with these segments:
    Pads | EQ | Wave | Spec | Lvl | Chn | Syn | Smpl
  "Pads" shows the drum rows; the others show the audio panel on that tab.
  Switch views with the switch_view tool, e.g. switch_view {slug:"eq"} or {slug:"pads"}.
  Valid slugs: pads, eq, wave, spectrum, levels, chain, synth, sampler.
  Verify with checkpoint {expect:{viewMode:"<slug>"}}.
  WARNING: there is no "viewSwitch" toggle anymore — do not rely on it.

MOBILE ROW CONTROLS:
  Each row shows Label | Vol | M | S | FX inline (same as desktop).
  - Mute / Solo / FX are INLINE buttons → use click_ui button="mute"/"solo"/"fx" row=N.
  - Instrument / Rename / Origin / Delete are in the row CONTEXT MENU → open_row_menu {row}, then menu_click {label}.
  The context menu does NOT contain Mute/Solo/Color/Effects — do not look for them there.
  Color is not exposed on the mobile row (no swatch); treat it as unavailable on mobile.

MOBILE CONTEXT MENU (bottom sheet) — items are ONLY:
  Instrument, Rename, Origin, Delete.
  Always call open_row_menu first to read the EXACT labels, then menu_click.

MOBILE VOLUME POPUP:
  click_ui button="volume" row=N opens a vertical slider popup.
  Drag the slider vertically (computer tool) to adjust volume. Tap outside to close.

MOBILE NODE DELETE (long-press popup):
  Long-pressing a grid node opens a Move | Connect | Delete popup; you slide to a button and
  release. Use the delete_node_longpress {x,y} tool (x,y = a node's on-canvas pixel) to perform
  that whole gesture, then checkpoint {totalNodes:{lt:<before>}}.

MOBILE CAMERA GESTURES:
  pinch_out/pinch_in change zoom (camScale); swipe / two_finger_pan move the camera (camOffset).
  Read camScale / camOffset from the State hint BEFORE the gesture, then checkpoint the change
  relationally, e.g. {camScale:{gt:<S0>}} or {camOffsetX:{ne:<X0>}}.

MOBILE NODE SIDEBAR:
  Tap a grid node to open a left-anchored sidebar (Instrument header; collapsible
  Volume/Pitch/Duration/Logic/Groove/Audible sections). The Logic section has a
  dropdown: None, Trigger Every N, Skip Every N, Probability, If Prev Skipped,
  If Prev Triggered ("Skip Every N" defaults N=2). After changing logic, the drum
  row's step pattern should visibly change. Tap outside to close.

MOBILE OVERFLOW MENU:
  click_ui button="overflow" opens a menu with Upload, Import, Export.`;

const TOUCH_GESTURE_TOOL = {
  name: "touch_gesture",
  description:
    "Perform a multi-touch gesture on the touchscreen. Use for: pinch zoom, two-finger pan, long press, swipe. Single taps are handled automatically via mouse click translation.",
  input_schema: {
    type: "object",
    properties: {
      gesture: {
        type: "string",
        enum: ["tap", "long_press", "pinch_in", "pinch_out", "two_finger_pan", "swipe"],
        description: "The gesture type to perform",
      },
      x: { type: "number", description: "Center X coordinate" },
      y: { type: "number", description: "Center Y coordinate" },
      duration: {
        type: "number",
        description: "Hold duration in ms (for long_press, default 600)",
      },
      delta_x: {
        type: "number",
        description: "Horizontal movement in px (for pan, swipe)",
      },
      delta_y: {
        type: "number",
        description: "Vertical movement in px (for pan, swipe)",
      },
    },
    required: ["gesture", "x", "y"],
  },
};

const QUERY_UI_TOOL = {
  name: "query_ui",
  description:
    "Get exact pixel positions of ALL UI buttons, controls, sliders, and EQ elements. Returns a JSON object with rects {x, y, w, h} for every interactive element. The center of a rect is (x + w/2, y + h/2). Use this to get precise click targets.",
  input_schema: {
    type: "object",
    properties: {},
    required: [],
  },
};

const CLICK_UI_TOOL = {
  name: "click_ui",
  description: "Click a UI button by name at its exact pixel position. " +
    "Resolves coordinates from the layout engine, then performs a real mouse click+hold. " +
    "Global buttons: play, stop, bpmInc, bpmDec, subdiv, lenInc, lenDec, track, " +
    "addRow, upload, import, export, overflow, viewSwitch, eqToggle, mainVol. " +
    "EQ buttons: eqChannel, hpf, lpf. " +
    "Row buttons (requires 'row', 0-based): label, edit, color, mute, solo, fx, origin, delete, volume. " +
    "Returns a screenshot showing the result.",
  input_schema: {
    type: "object",
    properties: {
      button: { type: "string", description: "Button name to click" },
      row: { type: "number", description: "Row index (0-based) for per-row buttons. Omit for global/EQ buttons." },
      hold_ms: { type: "number", description: "Additional hold duration in ms after the game loop processes the click. Default: 0." },
    },
    required: ["button"],
  },
};

const REPEAT_CLICK_UI_TOOL = {
  name: "repeat_click_ui",
  description: "Click a UI button multiple times in rapid succession. " +
    "Equivalent to calling click_ui N times but in a single round-trip. " +
    "Returns a screenshot after all clicks. " +
    "Same button names as click_ui. Row buttons require 'row' (0-based).",
  input_schema: {
    type: "object",
    properties: {
      button: { type: "string", description: "Button name to click" },
      count: { type: "number", description: "Number of times to click (1-20)" },
      row: { type: "number", description: "Row index (0-based) for per-row buttons." },
      delay_ms: { type: "number", description: "Delay between clicks in ms. Default: 150." },
    },
    required: ["button", "count"],
  },
};

// Deterministic surface-navigation tools. These wrap stable WASM JS exports
// (openContextMenuJS / contextMenuClick / setEQTab / setViewMode) so the agent
// reaches the row overflow menu, the 7-tab audio panel, and the mobile bottom-
// nav by NAME instead of fragile pixel math. The old UI had inline row buttons
// and a binary viewSwitch; both are gone (see the system prompt).

const OPEN_ROW_MENU_TOOL = {
  name: "open_row_menu",
  description:
    "Open a drum row's overflow (⋯ kebab) context menu and return its item labels. " +
    "On desktop, color/rename/edit/origin/delete live ONLY in this menu (no inline buttons). " +
    "On mobile, every per-row action lives here too. After this, call menu_click with one of the returned labels.",
  input_schema: {
    type: "object",
    properties: { row: { type: "number", description: "Row index (0-based)." } },
    required: ["row"],
  },
};

const MENU_CLICK_TOOL = {
  name: "menu_click",
  description:
    "Click an item in the currently-open row context menu by its EXACT label " +
    '(as returned by open_row_menu), e.g. "Mute", "Solo", "Color", "Rename", "Effects", "Origin", "Delete", "Instrument".',
  input_schema: {
    type: "object",
    properties: { label: { type: "string", description: "Exact menu item label." } },
    required: ["label"],
  },
};

const SWITCH_TAB_TOOL = {
  name: "switch_tab",
  description:
    "DESKTOP: switch the audio panel tab by slug — one of: eq, wave, spectrum, levels, chain, synth, sampler. " +
    "Use this (not pixel clicks) to navigate the 7-tab audio panel.",
  input_schema: {
    type: "object",
    properties: { slug: { type: "string", description: "Tab slug." } },
    required: ["slug"],
  },
};

const SWITCH_VIEW_TOOL = {
  name: "switch_view",
  description:
    "MOBILE: switch the bottom-nav view by slug — one of: pads, eq, wave, spectrum, levels, chain, synth, sampler. " +
    "'pads' shows the drum rows; the others show the matching audio panel. This replaces the old viewSwitch toggle.",
  input_schema: {
    type: "object",
    properties: { slug: { type: "string", description: "View-mode slug." } },
    required: ["slug"],
  },
};

const DRAG_TOOL = {
  name: "drag",
  description:
    "DESKTOP: perform a REAL left-button drag from (x0,y0) to (x1,y1) that reliably registers " +
    "in headless (force-ticked through the game loop). Use this to move EQ band handles, synth " +
    "knobs, and volume sliders — read the handle rect from query_ui, then drag from its center to " +
    "the target. Verify the effect with read_export/checkpoint_export.",
  input_schema: {
    type: "object",
    properties: {
      x0: { type: "number" }, y0: { type: "number" },
      x1: { type: "number" }, y1: { type: "number" },
      steps: { type: "number", description: "Interpolation steps (default 8)." },
    },
    required: ["x0", "y0", "x1", "y1"],
  },
};

const READ_EXPORT_TOOL = {
  name: "read_export",
  description:
    "Read a value from a WASM JS export (a getter) so you can record a baseline before an " +
    "interaction. Calls globalThis[export](...args) and drills an optional dot/bracket path. " +
    'Examples: {export:"eqControlsSnapshot", path:"gainsDB.3"}, {export:"getInstrumentParams", ' +
    'args:["kick-1"], path:"osc_tune"}, {export:"spectrumSlope"}, {export:"rowVolume", args:[0]}.',
  input_schema: {
    type: "object",
    properties: {
      export: { type: "string", description: "Exported getter name." },
      args: { type: "array", description: "Arguments to pass.", items: {} },
      path: { type: "string", description: "Optional dot/bracket path into the returned value." },
    },
    required: ["export"],
  },
};

const CALL_EXPORT_TOOL = {
  name: "call_export",
  description:
    "Invoke a WASM SETTER export with args, then apply it (force-ticks the game loop). Use to " +
    "deterministically drive a continuous control through its REAL handler when a pixel-drag won't " +
    "register headless — e.g. EQ band gain or a synth/sampler param — then verify with checkpoint_export. " +
    'Examples: {export:"setEQBandGain", args:["main",3,6]}, {export:"setInstrumentParam", args:["kick-1","osc_tune",0.5]}.',
  input_schema: {
    type: "object",
    properties: {
      export: { type: "string", description: "Exported setter name." },
      args: { type: "array", description: "Arguments to pass.", items: {} },
    },
    required: ["export"],
  },
};

const CHECKPOINT_EXPORT_TOOL = {
  name: "checkpoint_export",
  description:
    "Assert a value read from a WASM getter and record a PASS/FAIL checkpoint. Same read as " +
    "read_export, then compares with `expect` (exact OR relational {gt|lt|gte|lte|ne|eq}). " +
    "This is how you objectively verify a per-tab interaction changed real state, resiliently. " +
    'Example: {name:"eq_band_changed", export:"eqControlsSnapshot", path:"gainsDB.3", expect:{ne:<baseline>}}.',
  input_schema: {
    type: "object",
    properties: {
      name: { type: "string", description: "Unique checkpoint name." },
      export: { type: "string", description: "Exported getter name." },
      args: { type: "array", description: "Arguments to pass.", items: {} },
      path: { type: "string", description: "Optional dot/bracket path into the returned value." },
      expect: { description: "Expected value (literal) or relational object {gt|lt|gte|lte|ne|eq:N}." },
    },
    required: ["name", "export", "expect"],
  },
};

const DELETE_NODE_LONGPRESS_TOOL = {
  name: "delete_node_longpress",
  description:
    "MOBILE: delete a grid node via the real long-press popup. Pass the on-canvas pixel (x,y) " +
    "of a visible node (read it from the screenshot). The tool press-holds to open the " +
    "Move/Connect/Delete popup, slides to the Delete button, and releases. Then verify with " +
    "checkpoint {expect:{totalNodes:{lt:<count before>}}}. If totalNodes didn't drop, the press " +
    "likely missed the node — retry on a clearer node pixel.",
  input_schema: {
    type: "object",
    properties: {
      x: { type: "number", description: "Canvas X of the node to delete." },
      y: { type: "number", description: "Canvas Y of the node to delete." },
    },
    required: ["x", "y"],
  },
};

const MEASURE_AUDIO_TOOL = {
  name: "measure_audio",
  description:
    "Record real playback and HARD-VERIFY audio is captured end-to-end: starts the recorder, plays the " +
    "circuit AND fires explicit hits on every instrument for ~2.3s, stops, decodes the captured " +
    "master.wav, and records a checkpoint that its RMS exceeds the silence floor. Robust to circuit " +
    "state. Call once per test; it manages its own play/stop.",
  input_schema: {
    type: "object",
    properties: {
      name: { type: "string", description: "Checkpoint name (default audio_audible)." },
      floor: { type: "number", description: "RMS silence floor (default 0.001)." },
    },
    required: [],
  },
};

const CHECKPOINT_TOOL = {
  name: "checkpoint",
  description:
    "Record an OBJECTIVE assertion against the live engine state and get an immediate PASS/FAIL verdict. " +
    "Call this after an action that should change state, with a unique name and the expected values. " +
    "Supported expect keys: bpm, subdiv, totalRows, isPlaying, activeTab, viewMode, channel (scalars), " +
    "and per-row {row, muted, soloed}. The verdict is returned so you can self-correct, and the test gate " +
    "requires every named checkpoint to PASS — so checkpoints are how you PROVE a step worked.",
  input_schema: {
    type: "object",
    properties: {
      name: { type: "string", description: 'Unique checkpoint name, e.g. "bpm_90".' },
      expect: {
        type: "object",
        description: "Expected state values to assert.",
        properties: {
          bpm: { type: "number" },
          subdiv: { type: "number" },
          totalRows: { type: "number" },
          isPlaying: { type: "boolean" },
          activeTab: { type: "string" },
          viewMode: { type: "string" },
          channel: { type: "string" },
          row: { type: "number" },
          muted: { type: "boolean" },
          soloed: { type: "boolean" },
        },
      },
    },
    required: ["name", "expect"],
  },
};

const SET_BPM_TOOL = {
  name: "set_bpm",
  description:
    "Set the BPM to an EXACT value deterministically (goes through the real SetBPM handler). " +
    "Use this instead of typing into the BPM field — keyboard text entry does not reliably commit " +
    "in this headless build. After it, verify with checkpoint {expect:{bpm:<value>}}.",
  input_schema: {
    type: "object",
    properties: { value: { type: "number", description: "Target BPM, e.g. 90." } },
    required: ["value"],
  },
};

const SET_SUBDIV_TOOL = {
  name: "set_subdiv",
  description:
    "Set the subdivision to one of 4/8/16/32 deterministically (clicks the real subdiv menu item handler). " +
    "Use this instead of tapping the dropdown. After it, verify with checkpoint {expect:{subdiv:<value>}}.",
  input_schema: {
    type: "object",
    properties: { value: { type: "number", description: "Target subdivision: 4, 8, 16, or 32." } },
    required: ["value"],
  },
};

const EXPORT_CIRCUIT_TOOL = {
  name: "export_circuit",
  description:
    "Capture the current circuit via exportJSON() and stash it in the harness. Returns " +
    "{totalRows,totalNodes,bpm,subdiv} so you can checkpoint those exact values after a later " +
    "import_circuit. Use this for the export/import roundtrip test.",
  input_schema: { type: "object", properties: {}, required: [] },
};

const IMPORT_CIRCUIT_TOOL = {
  name: "import_circuit",
  description:
    "Re-import the circuit previously stashed by export_circuit (via importJSON()). Use after " +
    "mutating the circuit to confirm the roundtrip restores it, then checkpoint totalRows/" +
    "totalNodes/bpm/subdiv against the values export_circuit returned.",
  input_schema: { type: "object", properties: {}, required: [] },
};

/**
 * Query fullLayoutSnapshot() and build a compact coordinate hint string.
 * Returns null if the function is not available.
 *
 * @param {import('playwright').Page} page
 * @returns {Promise<string|null>}
 */
async function buildUIHints(page) {
  return page.evaluate(() => {
    if (typeof fullLayoutSnapshot !== "function") return null;
    const snap = fullLayoutSnapshot();
    if (!snap) return null;
    const center = (r) =>
      r && r.w > 0 && r.h > 0
        ? `(${r.x + Math.round(r.w / 2)}, ${r.y + Math.round(r.h / 2)})`
        : null;
    const lines = [];

    // Transport buttons
    const b = snap.buttons || {};
    const transport = [];
    for (const [name, rect] of Object.entries(b)) {
      const c = center(rect);
      if (c) transport.push(`${name}=${c}`);
    }
    if (transport.length) lines.push("Buttons: " + transport.join(" | "));

    // Per-row controls
    const rows = snap.rows || [];
    for (let i = 0; i < rows.length; i++) {
      const row = rows[i];
      if (!row) continue;
      const parts = [];
      for (const [name, rect] of Object.entries(row)) {
        const c = center(rect);
        if (c) parts.push(`${name}=${c}`);
      }
      if (parts.length) lines.push(`Row${i}: ${parts.join(" | ")}`);
    }

    // EQ controls
    const eq = snap.eq || {};
    const eqParts = [];
    for (const [name, val] of Object.entries(eq)) {
      if (name === "sliders" && Array.isArray(val)) {
        val.forEach((r, j) => {
          const c = center(r);
          if (c) eqParts.push(`band${j}=${c}`);
        });
      } else if (name === "bands" && Array.isArray(val)) {
        // EQ band handle centres ({x,y}) — drag targets for changing band gain.
        val.forEach((p, j) => {
          if (p && typeof p.x === "number") eqParts.push(`band${j}=(${p.x}, ${p.y})`);
        });
      } else if (name === "muteBtns" && Array.isArray(val)) {
        val.forEach((r, j) => {
          const c = center(r);
          if (c) eqParts.push(`mute${j}=${c}`);
        });
      } else if (name === "rect") {
        // skip the overall rect
      } else {
        const c = center(val);
        if (c) eqParts.push(`${name}=${c}`);
      }
    }
    if (eqParts.length) lines.push("EQ: " + eqParts.join(" | "));

    // Scrollbar
    const sc = center(snap.scrollBar);
    const st = center(snap.scrollThumb);
    if (sc || st)
      lines.push(`Scroll: bar=${sc || "n/a"} thumb=${st || "n/a"}`);

    // State (incl. node count + camera, for build/edit/gesture checkpoints)
    const s = snap.state || {};
    lines.push(
      `State: playing=${s.isPlaying} bpm=${s.bpm} rows=${s.totalRows} nodes=${s.totalNodes}` +
      ` subdiv=${s.subdiv} camScale=${s.camScale} camOffset=(${s.camOffsetX},${s.camOffsetY})`
    );

    return lines.join("\n");
  }).catch(() => null);
}

/**
 * Resolve a button name to its rect from a fullLayoutSnapshot.
 *
 * @param {Object} snap - Result of fullLayoutSnapshot()
 * @param {string} button - Button name
 * @param {number|undefined} row - Row index for per-row buttons
 * @returns {{x: number, y: number, w: number, h: number}|null}
 */
function resolveButtonRect(snap, button, row) {
  // Row buttons
  if (row !== undefined && row !== null) {
    const rows = snap.rows || [];
    if (row >= 0 && row < rows.length && rows[row]) {
      return rows[row][button] || null;
    }
    return null;
  }
  // Global buttons
  if (snap.buttons && snap.buttons[button]) {
    return snap.buttons[button];
  }
  // EQ buttons (different key names in snapshot)
  if (snap.eq) {
    const eqMap = { eqChannel: "channelBtn", hpf: "hpfBtn", lpf: "lpfBtn" };
    const eqKey = eqMap[button];
    if (eqKey && snap.eq[eqKey]) return snap.eq[eqKey];
  }
  // Audio-panel tabs (desktop): "tab:wave" → snap.tabs.wave
  if (button.startsWith("tab:") && snap.tabs) {
    return snap.tabs[button.slice(4)] || null;
  }
  // Mobile bottom-nav: "nav:eq" → snap.bottomNav.eq
  if (button.startsWith("nav:") && snap.bottomNav) {
    return snap.bottomNav[button.slice(4)] || null;
  }
  // Audio-panel chrome pills: "chrome:slope" → snap.chrome.slope
  if (button.startsWith("chrome:") && snap.chrome) {
    return snap.chrome[button.slice(7)] || null;
  }
  return null;
}

/**
 * Execute a touch gesture via CDP helpers.
 *
 * @param {import('playwright').Page} page
 * @param {Object} input - The touch_gesture tool input
 * @param {boolean} verbose
 */
async function executeTouchGesture(page, input, verbose) {
  const { gesture, x, y, duration, delta_x, delta_y } = input;

  if (verbose) {
    const extra = [];
    if (duration) extra.push(`duration=${duration}`);
    if (delta_x) extra.push(`dx=${delta_x}`);
    if (delta_y) extra.push(`dy=${delta_y}`);
    console.log(`  [touch] ${gesture} (${x}, ${y})${extra.length ? " " + extra.join(", ") : ""}`);
  }

  switch (gesture) {
    case "tap":
      await cdpTap(page, x, y);
      break;

    case "long_press":
      await cdpLongPress(page, x, y, duration ?? 600);
      break;

    case "pinch_in":
      // Pinch in = zoom out: fingers start spread (140px) and come together (40px)
      await cdpPinch(page, x, y, 140, 40);
      break;

    case "pinch_out":
      // Pinch out = zoom in: fingers start close (40px) and spread apart (140px)
      await cdpPinch(page, x, y, 40, 140);
      break;

    case "two_finger_pan":
      await cdpTwoFingerPan(page, x, y, delta_x ?? 0, delta_y ?? 0);
      break;

    case "swipe":
      await cdpDrag(page, x, y, x + (delta_x ?? 0), y + (delta_y ?? 0));
      break;

    default:
      if (verbose) {
        console.log(`  [touch] Unknown gesture: ${gesture}`);
      }
  }
}

/**
 * Capture the WebGL canvas content directly within the page, bypassing
 * CDP Page.captureScreenshot which hangs in headless mode when captureStream
 * is active. forceDraw() issues WebGL commands synchronously (WASM), and
 * drawImage() copies the drawing buffer before the browser presents/clears
 * it (same event-loop task, so preserveDrawingBuffer:false is fine).
 *
 * @param {import('playwright').Page} pg
 * @returns {Promise<Buffer|null>}
 */
async function captureCanvas(pg) {
  const b64 = await pg.evaluate(() => {
    if (typeof forceDraw === "function") forceDraw();
    const c = document.querySelector("canvas");
    if (!c) return null;
    const tmp = document.createElement("canvas");
    tmp.width = c.width;
    tmp.height = c.height;
    const ctx = tmp.getContext("2d");
    ctx.drawImage(c, 0, 0);
    return tmp.toDataURL("image/png").split(",")[1];
  });
  return b64 ? Buffer.from(b64, "base64") : null;
}

/**
 * Execute a Claude computer-use action via Playwright.
 * When platform is "mobile", mouse actions are translated to CDP touch events.
 *
 * @param {import('playwright').Page} page
 * @param {Object} action - Claude's tool_use input
 * @param {boolean} verbose - Log actions to console
 * @param {string} platform - "desktop" or "mobile"
 * @returns {Promise<Buffer|null>} Screenshot buffer if action was "screenshot", null otherwise
 */
async function executeAction(page, action, verbose, platform) {
  const { action: actionType, text, key, duration } = action;
  const coordinate = normCoord(action.coordinate);
  const start_coordinate = normCoord(action.start_coordinate);

  if (verbose) {
    const coords = coordinate ? ` (${coordinate[0]}, ${coordinate[1]})` : "";
    const extra = text ? ` "${text}"` : key ? ` "${key}"` : "";
    const prefix = platform === "mobile" ? "[mobile] " : "";
    console.log(`  [action] ${prefix}${actionType}${coords}${extra}`);
  }

  // Mobile mouse→touch translation
  if (platform === "mobile") {
    switch (actionType) {
      case "screenshot":
        return await captureCanvas(page) ?? await page.screenshot({ type: "png", timeout: 5000 });

      case "left_click":
        if (coordinate) {
          await cdpTap(page, coordinate[0], coordinate[1]);
        }
        return null;

      case "left_click_drag":
        if (start_coordinate && coordinate) {
          await cdpDrag(
            page,
            start_coordinate[0],
            start_coordinate[1],
            coordinate[0],
            coordinate[1]
          );
        }
        return null;

      case "right_click":
        // Right-click on mobile → long press (opens context menu)
        if (coordinate) {
          if (verbose) console.log(`  [mobile] right_click → long_press`);
          await cdpLongPress(page, coordinate[0], coordinate[1], 600);
        }
        return null;

      case "double_click":
        // Double-click on mobile → two quick taps
        if (coordinate) {
          await cdpTap(page, coordinate[0], coordinate[1]);
          await page.waitForTimeout(50);
          await cdpTap(page, coordinate[0], coordinate[1]);
        }
        return null;

      case "scroll":
        if (coordinate) {
          const dir = action.scroll_direction ?? "down";
          const amount = (action.scroll_amount ?? 3) * 100;
          const deltaX = dir === "left" ? -amount : dir === "right" ? amount : 0;
          const deltaY = dir === "up" ? -amount : dir === "down" ? amount : 0;
          await cdpDrag(
            page,
            coordinate[0],
            coordinate[1],
            coordinate[0] - deltaX,
            coordinate[1] - deltaY
          );
        }
        return null;

      case "wait":
        await page.waitForTimeout(duration ?? 1000);
        return null;

      // Keyboard actions work the same on mobile
      case "type":
      case "key":
        break;

      default:
        // Fall through to desktop handler for other actions
        break;
    }
  }

  // Desktop execution (and fallthrough for mobile keyboard actions)
  switch (actionType) {
    case "screenshot":
      return await captureCanvas(page) ?? await page.screenshot({ type: "png", timeout: 5000 });

    case "left_click":
      if (coordinate) {
        const mods = text ? text.split("+").map((m) => m.trim()) : [];
        for (const mod of mods) await page.keyboard.down(mod);
        // Force a game tick with the mouse at the click position.
        // In headless Chromium with SwiftShader, rAF fires infrequently
        // so Ebiten's Update() may never see the pressed state.
        // forceGameTick() overrides input and runs Update() directly.
        await page.mouse.move(coordinate[0], coordinate[1]);
        await page.mouse.down();
        await page.evaluate(([x, y]) => {
          if (typeof forceGameTick === 'function') {
            forceGameTick(x, y);
          }
        }, [coordinate[0], coordinate[1]]);
        await page.mouse.up();
        for (const mod of mods.reverse()) await page.keyboard.up(mod);
      }
      break;

    case "right_click":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
        await page.mouse.down({ button: "right" });
        // forceGameTick only overrides left button; for right-click,
        // fall back to rAF wait with long timeout.
        await page.evaluate(() => new Promise(r => {
          requestAnimationFrame(r);
          setTimeout(r, 5000);
        }));
        await page.mouse.up({ button: "right" });
      }
      break;

    case "double_click":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
        await page.mouse.down();
        await page.evaluate(([x, y]) => {
          if (typeof forceGameTick === 'function') forceGameTick(x, y);
        }, [coordinate[0], coordinate[1]]);
        await page.mouse.up();
        await page.waitForTimeout(50);
        await page.mouse.down();
        await page.evaluate(([x, y]) => {
          if (typeof forceGameTick === 'function') forceGameTick(x, y);
        }, [coordinate[0], coordinate[1]]);
        await page.mouse.up();
      }
      break;

    case "middle_click":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
        await page.mouse.down({ button: "middle" });
        await page.evaluate(() => new Promise(r => {
          requestAnimationFrame(r);
          setTimeout(r, 5000);
        }));
        await page.mouse.up({ button: "middle" });
      }
      break;

    case "type":
      if (text) {
        await page.keyboard.type(text);
      }
      break;

    case "key":
      if (key) {
        await page.keyboard.press(key);
      }
      break;

    case "scroll":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
        const dir = action.scroll_direction ?? "down";
        const amount = (action.scroll_amount ?? 3) * 100;
        const deltaX = dir === "left" ? -amount : dir === "right" ? amount : 0;
        const deltaY = dir === "up" ? -amount : dir === "down" ? amount : 0;
        await page.mouse.wheel(deltaX, deltaY);
      }
      break;

    case "mouse_move":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
      }
      break;

    case "left_click_drag":
      if (start_coordinate && coordinate) {
        // Prefer the force-ticked forceDrag (reliable headless); fall back to a
        // real mouse drag if the export isn't present.
        const usedForceDrag = await page.evaluate(([a, b, c, d]) => {
          if (typeof forceDrag === "function") { forceDrag(a, b, c, d, 10); return true; }
          return false;
        }, [start_coordinate[0], start_coordinate[1], coordinate[0], coordinate[1]]);
        if (!usedForceDrag) {
          await page.mouse.move(start_coordinate[0], start_coordinate[1]);
          await page.mouse.down();
          await page.mouse.move(coordinate[0], coordinate[1], { steps: 10 });
          await page.mouse.up();
        }
      }
      break;

    case "wait":
      await page.waitForTimeout(duration ?? 1000);
      break;

    case "triple_click":
      if (coordinate) {
        await page.mouse.click(coordinate[0], coordinate[1], { clickCount: 3 });
      }
      break;

    case "hold_key":
      if (key) {
        await page.keyboard.down(key);
        await page.waitForTimeout((duration ?? 1) * 1000);
        await page.keyboard.up(key);
      }
      break;

    case "left_mouse_down":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
      }
      await page.mouse.down();
      break;

    case "left_mouse_up":
      if (coordinate) {
        await page.mouse.move(coordinate[0], coordinate[1]);
      }
      await page.mouse.up();
      break;

    default:
      if (verbose) {
        console.log(`  [action] Unknown action: ${actionType}`);
      }
  }

  return null;
}

/**
 * Create an agent that drives Claude Haiku to interact with Beatmo.
 *
 * @param {import('playwright').Page} page - Playwright page (use recorder.getWrappedPage() for recording)
 * @param {Object} options
 * @param {string} options.model - Claude model (default: claude-haiku-4-5-20251001)
 * @param {number} options.maxIterations - Max agent turns (default: 50)
 * @param {Object} options.screenshotSize - {width, height} matching viewport
 * @param {string} options.apiKey - Anthropic API key
 * @param {boolean} options.verbose - Log actions to console (default: true)
 * @param {string} options.platform - "desktop" or "mobile" (default: "desktop")
 * @returns {Agent}
 */
export function createAgent(page, options = {}) {
  const model = options.model ?? "claude-haiku-4-5-20251001";
  const maxIterations = options.maxIterations ?? 50;
  const screenshotSize = options.screenshotSize ?? { width: 1280, height: 720 };
  const apiKey = options.apiKey ?? process.env.ANTHROPIC_API_KEY;
  const verbose = options.verbose ?? true;
  const platform = options.platform ?? "desktop";

  if (!apiKey) {
    throw new Error("ANTHROPIC_API_KEY is required. Set it via env or pass apiKey option.");
  }

  let stopped = false;

  // Build system prompt based on platform
  const systemPrompt =
    platform === "mobile" ? SYSTEM_PROMPT + MOBILE_SYSTEM_PROMPT_SUFFIX : SYSTEM_PROMPT;

  // Build tools array — always include query_ui; add touch_gesture for mobile
  const tools = [
    {
      type: "computer_20250124",
      name: "computer",
      display_width_px: screenshotSize.width,
      display_height_px: screenshotSize.height,
    },
    QUERY_UI_TOOL,
    CLICK_UI_TOOL,
    REPEAT_CLICK_UI_TOOL,
    OPEN_ROW_MENU_TOOL,
    MENU_CLICK_TOOL,
    SET_BPM_TOOL,
    SET_SUBDIV_TOOL,
    EXPORT_CIRCUIT_TOOL,
    IMPORT_CIRCUIT_TOOL,
    READ_EXPORT_TOOL,
    CALL_EXPORT_TOOL,
    CHECKPOINT_EXPORT_TOOL,
    MEASURE_AUDIO_TOOL,
    CHECKPOINT_TOOL,
  ];
  if (platform !== "mobile") {
    // Desktop real handle/knob/slider drags (force-ticked).
    tools.push(DRAG_TOOL);
  }
  if (platform === "mobile") {
    tools.push(TOUCH_GESTURE_TOOL);
    // Mobile navigates the bottom-nav segmented control by view-mode slug.
    tools.push(SWITCH_VIEW_TOOL);
    // Mobile node deletion via the long-press popup.
    tools.push(DELETE_NODE_LONGPRESS_TOOL);
  } else {
    // Desktop navigates the 7-tab audio panel by tab slug.
    tools.push(SWITCH_TAB_TOOL);
  }

  /**
   * Take a screenshot with retry logic. Falls back to a 1x1 transparent PNG
   * placeholder if the page is unresponsive.
   */
  async function safeScreenshot(retries = 2) {
    for (let attempt = 0; attempt <= retries; attempt++) {
      try {
        // Primary: capture canvas in-page (avoids CDP compositor hang with captureStream)
        const buf = await captureCanvas(page);
        if (buf) return buf;
        // Fallback: CDP screenshot (if no canvas found)
        return await page.screenshot({ type: "png", timeout: 5000 });
      } catch (err) {
        if (attempt < retries) {
          if (verbose) console.log(`  [screenshot] Attempt ${attempt + 1} failed, retrying...`);
          await new Promise((r) => setTimeout(r, 1000));
        } else {
          if (verbose) console.log(`  [screenshot] All attempts failed: ${err.message}`);
          return null;
        }
      }
    }
    return null;
  }

  async function callClaude(messages) {
    const body = {
      model,
      max_tokens: 2048,
      system: systemPrompt,
      tools,
      messages,
    };

    // Retry with exponential backoff for transient errors (429, 5xx)
    const maxRetries = 3;
    for (let attempt = 0; attempt <= maxRetries; attempt++) {
      const controller = new AbortController();
      const timeoutId = setTimeout(() => controller.abort(), 60000);

      let response;
      try {
        response = await fetch("https://api.anthropic.com/v1/messages", {
          method: "POST",
          signal: controller.signal,
          headers: {
            "Content-Type": "application/json",
            "x-api-key": apiKey,
            "anthropic-version": "2023-06-01",
            "anthropic-beta": "computer-use-2025-01-24",
          },
          body: JSON.stringify(body),
        });
      } catch (err) {
        clearTimeout(timeoutId);
        if (err.name === "AbortError") {
          throw new Error("Claude API request timed out (60s)");
        }
        throw err;
      } finally {
        clearTimeout(timeoutId);
      }

      // Retry on rate limit or server errors
      if ((response.status === 429 || response.status >= 500) && attempt < maxRetries) {
        let delay;
        if (response.status === 429) {
          // Rate limit — respect Retry-After header or use longer backoff
          const retryAfter = response.headers.get("retry-after");
          delay = retryAfter
            ? parseInt(retryAfter) * 1000
            : Math.min(5000 * Math.pow(2, attempt), 60000);
        } else {
          delay = Math.min(1000 * Math.pow(2, attempt), 30000);
        }
        if (verbose) {
          console.log(`  [api] ${response.status} — retrying in ${delay}ms (attempt ${attempt + 1}/${maxRetries})`);
        }
        await new Promise((r) => setTimeout(r, delay));
        continue;
      }

      if (!response.ok) {
        const errBody = await response.text();
        throw new Error(`Claude API error (${response.status}): ${errBody}`);
      }

      return response.json();
    }
  }

  return {
    /**
     * Run the agent loop with the given task prompt.
     *
     * @param {string} taskPrompt - What the agent should do
     * @returns {Promise<AgentResult>}
     */
    async run(taskPrompt) {
      const startTime = Date.now();
      const agentLog = [];    // Full message log for debugging
      const issues = [];      // Issues reported by Claude
      const checkpoints = []; // Objective state assertions (see checkpoint tool)
      let stashedCircuit = null; // export_circuit stashes exportJSON() here for import_circuit
      let iterations = 0;
      let totalInputTokens = 0;
      let totalOutputTokens = 0;
      stopped = false;

      if (verbose) {
        console.log(`[agent] Starting with model=${model}, platform=${platform}, maxIterations=${maxIterations}`);
        console.log(`[agent] Task: ${taskPrompt}`);
      }

      // Take initial screenshot
      const initialPng = await safeScreenshot();
      if (!initialPng) {
        return {
          task: taskPrompt, model, platform, iterations: 0, durationMs: Date.now() - startTime,
          inputTokens: 0, outputTokens: 0, estimatedCost: 0,
          issues: [], checkpoints: [], agentLog: [], summary: "",
          error: "Failed to take initial screenshot — page may be unresponsive",
        };
      }
      const initialB64 = initialPng.toString("base64");

      // Build initial message with screenshot + UI coordinate hints
      const initialContent = [
        {
          type: "text",
          text: taskPrompt,
        },
        {
          type: "image",
          source: {
            type: "base64",
            media_type: "image/png",
            data: initialB64,
          },
        },
      ];

      // Inject UI coordinate hints with initial screenshot
      const initialHints = await buildUIHints(page);
      if (initialHints) {
        initialContent.push({ type: "text", text: initialHints });
      }

      const messages = [
        {
          role: "user",
          content: initialContent,
        },
      ];

      let consecutiveScreenshotFailures = 0;
      const maxConsecutiveFailures = 3;

      while (iterations < maxIterations && !stopped) {
        iterations++;
        consecutiveScreenshotFailures = 0; // Reset per iteration

        if (verbose) {
          console.log(`\n[agent] --- Iteration ${iterations}/${maxIterations} ---`);
        }

        // Prune old screenshots to prevent context window overflow
        pruneConversationImages(messages);

        // Call Claude
        let response;
        try {
          response = await callClaude(messages);
        } catch (err) {
          console.error(`[agent] API error: ${err.message}`);
          agentLog.push({
            iteration: iterations,
            error: err.message,
            t: Date.now() - startTime,
          });
          break;
        }

        // Track usage
        if (response.usage) {
          totalInputTokens += response.usage.input_tokens ?? 0;
          totalOutputTokens += response.usage.output_tokens ?? 0;
        }

        agentLog.push({
          iteration: iterations,
          t: Date.now() - startTime,
          response: {
            stop_reason: response.stop_reason,
            content: response.content.map((b) => {
              // Don't store full image data in log — just mark presence
              if (b.type === "image") return { type: "image", note: "screenshot omitted" };
              return b;
            }),
            usage: response.usage,
          },
        });

        // Helper: detect positive/negated observations that should not count as issues.
        // E.g. "✅ No overlapping elements" or "No broken UI components".
        const ISSUE_KEYWORDS_RE = /(?:overlap|broken|unresponsive|misalign|artifact|bug|glitch|rendering error|truncat|clipped|stuck|suggestion)/i;
        function isNegatedObservation(text) {
          if (/✅/.test(text)) return true;
          if (/\b(?:no|not|without|none|zero)\s+\w*\s*/i.test(text) && ISSUE_KEYWORDS_RE.test(text)) {
            // Ensure the negation word appears BEFORE the keyword
            const negMatch = text.search(/\b(?:no|not|without|none|zero)\b/i);
            const kwMatch = text.search(ISSUE_KEYWORDS_RE);
            if (negMatch >= 0 && kwMatch >= 0 && negMatch < kwMatch) return true;
          }
          return false;
        }

        // Process response content blocks
        const toolResults = [];

        for (const block of response.content) {
          if (block.type === "text" && block.text) {
            if (verbose) {
              console.log(`  [claude] ${block.text.slice(0, 200)}${block.text.length > 200 ? "..." : ""}`);
            }

            // Extract issues from Claude's text
            const lines = block.text.split("\n");
            let inIssuesSection = false;
            for (const line of lines) {
              const trimmed = line.trim();
              const lower = trimmed.toLowerCase();

              // New structured format: ISSUE: P1 Bug - description
              const structuredMatch = trimmed.match(/^ISSUE:\s*(P[0-3])\s+(Bug|Glitch|Suggestion)\s*[-–—]\s*(.+)/i);
              if (structuredMatch) {
                issues.push(trimmed);
                continue;
              }

              if (/^(issues|bugs|problems|findings)\s*(found)?:/i.test(trimmed)) {
                inIssuesSection = true;
                continue;
              }
              if (inIssuesSection && trimmed === "") {
                inIssuesSection = false;
                continue;
              }

              if (inIssuesSection && /^[-*•]\s/.test(trimmed)) {
                if (!isNegatedObservation(trimmed)) {
                  issues.push(trimmed.replace(/^[-*•]\s*/, ""));
                }
                continue;
              }

              // Paragraph-form "Bug: ..." or "Glitch: ..." or "Suggestion: ..." at start of line
              if (/^(bug|glitch|suggestion)\s*:/i.test(trimmed)) {
                if (!isNegatedObservation(trimmed)) {
                  issues.push(trimmed);
                }
                continue;
              }

              // Numbered list items: "1. Bug: ..." or "2. Glitch: ..."
              if (/^\d+\.\s+(bug|glitch|suggestion)\s*:/i.test(trimmed)) {
                if (!isNegatedObservation(trimmed)) {
                  issues.push(trimmed.replace(/^\d+\.\s*/, ""));
                }
                continue;
              }

              if (/^[-*•]\s/.test(trimmed) && (
                lower.includes("bug:") ||
                lower.includes("glitch:") ||
                lower.includes("suggestion:") ||
                lower.includes("broken") ||
                lower.includes("misalign") ||
                lower.includes("overlap") ||
                lower.includes("unresponsive") ||
                lower.includes("truncat") ||
                lower.includes("clipped") ||
                lower.includes("stuck") ||
                lower.includes("visual artifact") ||
                lower.includes("rendering error")
              )) {
                if (!isNegatedObservation(trimmed)) {
                  issues.push(trimmed.replace(/^[-*•]\s*/, ""));
                }
              }
            }
          }

          if (block.type === "tool_use") {
            let screenshotResult = null;

            try {
              if (block.name === "query_ui") {
                // Handle query_ui tool — return full layout snapshot JSON
                const snapshot = await page.evaluate(() => {
                  if (typeof fullLayoutSnapshot !== "function") return null;
                  return fullLayoutSnapshot();
                });
                toolResults.push({
                  type: "tool_result",
                  tool_use_id: block.id,
                  content: [
                    {
                      type: "text",
                      text: snapshot
                        ? JSON.stringify(snapshot, null, 2)
                        : "fullLayoutSnapshot not available",
                    },
                  ],
                });
              } else if (block.name === "click_ui") {
                // Handle click_ui tool — resolve button rect and perform real mouse click
                const { button, row, hold_ms } = block.input;
                const snap = await page.evaluate(() => {
                  if (typeof fullLayoutSnapshot !== "function") return null;
                  return fullLayoutSnapshot();
                });

                if (!snap) {
                  toolResults.push({
                    type: "tool_result",
                    tool_use_id: block.id,
                    content: [{ type: "text", text: "fullLayoutSnapshot not available — WASM may not be loaded yet." }],
                    is_error: true,
                  });
                } else {
                  const rect = resolveButtonRect(snap, button, row);
                  if (!rect || rect.w <= 0 || rect.h <= 0) {
                    const available = [];
                    if (snap.buttons) available.push("Global: " + Object.keys(snap.buttons).join(", "));
                    if (snap.rows) {
                      snap.rows.forEach((r, i) => {
                        if (r) available.push(`Row${i}: ` + Object.keys(r).join(", "));
                      });
                    }
                    if (snap.eq) available.push("EQ: eqChannel, hpf, lpf");
                    toolResults.push({
                      type: "tool_result",
                      tool_use_id: block.id,
                      content: [{ type: "text", text: `Button "${button}"${row != null ? ` (row ${row})` : ""} not found. Available: ${available.join(" | ")}` }],
                      is_error: true,
                    });
                  } else {
                    const cx = rect.x + Math.round(rect.w / 2);
                    const cy = rect.y + Math.round(rect.h / 2);
                    if (verbose) {
                      console.log(`  [click_ui] ${button}${row != null ? `[row${row}]` : ""} → (${cx}, ${cy})`);
                    }
                    await page.mouse.move(cx, cy);
                    await page.mouse.down();
                    // Force a game tick with overridden input so the button
                    // handler fires regardless of rAF timing. In headless
                    // Chromium with SwiftShader, rAF fires infrequently and
                    // Update() may never run while the mouse is held.
                    await page.evaluate(([x, y]) => {
                      if (typeof forceGameTick === 'function') {
                        forceGameTick(x, y);
                      }
                    }, [cx, cy]);
                    if (hold_ms > 0) await page.waitForTimeout(hold_ms);
                    await page.mouse.up();

                    // Settle, then screenshot
                    await page.waitForTimeout(200);
                    const png = await safeScreenshot();
                    if (png) {
                      const b64 = png.toString("base64");
                      const toolResultContent = [
                        { type: "image", source: { type: "base64", media_type: "image/png", data: b64 } },
                      ];
                      const uiHints = await buildUIHints(page);
                      if (uiHints) toolResultContent.push({ type: "text", text: uiHints });
                      toolResults.push({ type: "tool_result", tool_use_id: block.id, content: toolResultContent });
                    } else {
                      toolResults.push({
                        type: "tool_result",
                        tool_use_id: block.id,
                        content: [{ type: "text", text: `Clicked ${button} at (${cx}, ${cy}) but screenshot failed.` }],
                        is_error: true,
                      });
                      consecutiveScreenshotFailures++;
                    }
                  }
                }
              } else if (block.name === "repeat_click_ui") {
                // Handle repeat_click_ui tool — click button N times in a single round-trip
                const { button, count: rawCount, row, delay_ms } = block.input;
                const clickCount = Math.max(1, Math.min(20, rawCount ?? 1));
                const delayBetween = delay_ms ?? 150;

                let lastError = null;
                for (let clickIdx = 0; clickIdx < clickCount; clickIdx++) {
                  // Re-resolve rect each iteration (button position may shift, e.g. BPM text width)
                  const snap = await page.evaluate(() => {
                    if (typeof fullLayoutSnapshot !== "function") return null;
                    return fullLayoutSnapshot();
                  });
                  if (!snap) { lastError = "fullLayoutSnapshot not available"; break; }

                  const rect = resolveButtonRect(snap, button, row);
                  if (!rect || rect.w <= 0 || rect.h <= 0) {
                    lastError = `Button "${button}"${row != null ? ` (row ${row})` : ""} not found on click ${clickIdx + 1}`;
                    break;
                  }

                  const cx = rect.x + Math.round(rect.w / 2);
                  const cy = rect.y + Math.round(rect.h / 2);
                  if (verbose && clickIdx === 0) {
                    console.log(`  [repeat_click_ui] ${button}${row != null ? `[row${row}]` : ""} ×${clickCount} → (${cx}, ${cy})`);
                  }

                  await page.mouse.move(cx, cy);
                  await page.mouse.down();
                  await page.evaluate(([x, y]) => {
                    if (typeof forceGameTick === 'function') forceGameTick(x, y);
                  }, [cx, cy]);
                  await page.mouse.up();

                  if (clickIdx < clickCount - 1) {
                    await page.waitForTimeout(delayBetween);
                  }
                }

                if (lastError) {
                  toolResults.push({
                    type: "tool_result",
                    tool_use_id: block.id,
                    content: [{ type: "text", text: lastError }],
                    is_error: true,
                  });
                } else {
                  // Settle, then screenshot
                  await page.waitForTimeout(200);
                  const png = await safeScreenshot();
                  if (png) {
                    const b64 = png.toString("base64");
                    const toolResultContent = [
                      { type: "image", source: { type: "base64", media_type: "image/png", data: b64 } },
                    ];
                    const uiHints = await buildUIHints(page);
                    if (uiHints) toolResultContent.push({ type: "text", text: uiHints });
                    toolResults.push({ type: "tool_result", tool_use_id: block.id, content: toolResultContent });
                  } else {
                    toolResults.push({
                      type: "tool_result",
                      tool_use_id: block.id,
                      content: [{ type: "text", text: `Clicked ${button} ×${clickCount} but screenshot failed.` }],
                      is_error: true,
                    });
                    consecutiveScreenshotFailures++;
                  }
                }
              } else if (block.name === "open_row_menu") {
                // Open the row's overflow context menu and return its labels.
                const { row } = block.input;
                const items = await page.evaluate((r) => {
                  if (typeof openContextMenuJS !== "function") return null;
                  openContextMenuJS(r);
                  if (typeof forceDraw === "function") forceDraw();
                  return typeof contextMenuItems === "function" ? contextMenuItems() : [];
                }, row);
                await page.waitForTimeout(200);
                const png = await safeScreenshot();
                const content = [];
                if (items) {
                  const labels = items.filter((it) => it && !it.divider).map((it) => it.label);
                  content.push({ type: "text", text: `Row ${row} menu items: ${JSON.stringify(labels)}. Use menu_click with one of these exact labels.` });
                } else {
                  content.push({ type: "text", text: "open_row_menu failed — openContextMenuJS not available." });
                }
                if (png) {
                  content.push({ type: "image", source: { type: "base64", media_type: "image/png", data: png.toString("base64") } });
                  const uiHints = await buildUIHints(page);
                  if (uiHints) content.push({ type: "text", text: uiHints });
                }
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content, is_error: !items });
              } else if (block.name === "menu_click") {
                // Click a context-menu item by exact label.
                const { label } = block.input;
                const ok = await page.evaluate((l) => {
                  if (typeof contextMenuClick !== "function") return false;
                  const r = contextMenuClick(l);
                  if (typeof forceDraw === "function") forceDraw();
                  return r;
                }, label);
                await page.waitForTimeout(250);
                const png = await safeScreenshot();
                const content = [{ type: "text", text: ok ? `Clicked menu item "${label}".` : `Menu item "${label}" not found or menu not open. Call open_row_menu first and use an exact label.` }];
                if (png) {
                  content.push({ type: "image", source: { type: "base64", media_type: "image/png", data: png.toString("base64") } });
                  const uiHints = await buildUIHints(page);
                  if (uiHints) content.push({ type: "text", text: uiHints });
                }
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content, is_error: !ok });
              } else if (block.name === "set_bpm") {
                // Deterministic BPM set via the real SetBPM handler.
                const v = Math.round(Number(block.input.value));
                const ok = await page.evaluate((n) => {
                  if (typeof setBPM !== "function") return false;
                  setBPM(n);
                  if (typeof forceDraw === "function") forceDraw();
                  return true;
                }, v);
                await page.waitForTimeout(150);
                const png = await safeScreenshot();
                const content = [{ type: "text", text: ok ? `Set BPM to ${v}. Verify with checkpoint {bpm:${v}}.` : "set_bpm failed — setBPM unavailable." }];
                if (png) {
                  content.push({ type: "image", source: { type: "base64", media_type: "image/png", data: png.toString("base64") } });
                  const uiHints = await buildUIHints(page);
                  if (uiHints) content.push({ type: "text", text: uiHints });
                }
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content, is_error: !ok });
              } else if (block.name === "set_subdiv") {
                // Deterministic subdivision set via the canonical SetSubdivisions
                // handler (setSubdivisions), which doesn't depend on the dropdown
                // menu being built/open. Falls back to the menu path if absent.
                const v = Math.round(Number(block.input.value));
                const ok = await page.evaluate((n) => {
                  if (typeof setSubdivisions === "function") {
                    const r = setSubdivisions(n);
                    if (typeof forceDraw === "function") forceDraw();
                    return r;
                  }
                  if (typeof applySubdivValue === "function") {
                    if (typeof openSubdivMenu === "function") openSubdivMenu();
                    applySubdivValue(n);
                    if (typeof closeSubdivMenu === "function") closeSubdivMenu();
                    if (typeof forceDraw === "function") forceDraw();
                    return true;
                  }
                  return false;
                }, v);
                await page.waitForTimeout(150);
                const png = await safeScreenshot();
                const content = [{ type: "text", text: ok ? `Set subdivision to ${v}. Verify with checkpoint {subdiv:${v}}.` : "set_subdiv failed — applySubdivValue unavailable." }];
                if (png) {
                  content.push({ type: "image", source: { type: "base64", media_type: "image/png", data: png.toString("base64") } });
                  const uiHints = await buildUIHints(page);
                  if (uiHints) content.push({ type: "text", text: uiHints });
                }
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content, is_error: !ok });
              } else if (block.name === "switch_tab") {
                // Desktop: switch the audio panel tab by slug via setEQTab.
                const { slug } = block.input;
                const ok = await page.evaluate((s) => {
                  if (typeof setEQTab !== "function") return false;
                  setEQTab(s);
                  if (typeof forceDraw === "function") forceDraw();
                  return true;
                }, slug);
                await page.waitForTimeout(250);
                const png = await safeScreenshot();
                const content = [{ type: "text", text: ok ? `Switched audio tab to "${slug}". Verify with a checkpoint {activeTab:"${slug}"}.` : "switch_tab failed — setEQTab not available." }];
                if (png) {
                  content.push({ type: "image", source: { type: "base64", media_type: "image/png", data: png.toString("base64") } });
                  const uiHints = await buildUIHints(page);
                  if (uiHints) content.push({ type: "text", text: uiHints });
                }
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content, is_error: !ok });
              } else if (block.name === "switch_view") {
                // Mobile: switch the bottom-nav view by slug via setViewMode.
                const { slug } = block.input;
                const ok = await page.evaluate((s) => {
                  if (typeof setViewMode !== "function") return false;
                  const r = setViewMode(s);
                  if (typeof forceDraw === "function") forceDraw();
                  return r;
                }, slug);
                await page.waitForTimeout(250);
                const png = await safeScreenshot();
                const content = [{ type: "text", text: ok ? `Switched view to "${slug}". Verify with a checkpoint {viewMode:"${slug}"}.` : `switch_view failed for "${slug}" — unknown slug or setViewMode unavailable.` }];
                if (png) {
                  content.push({ type: "image", source: { type: "base64", media_type: "image/png", data: png.toString("base64") } });
                  const uiHints = await buildUIHints(page);
                  if (uiHints) content.push({ type: "text", text: uiHints });
                }
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content, is_error: !ok });
              } else if (block.name === "export_circuit") {
                // Capture + stash the circuit JSON; return a summary of its shape.
                const info = await page.evaluate(() => {
                  if (typeof exportJSON !== "function") return null;
                  const json = exportJSON();
                  let doc = {};
                  try { doc = JSON.parse(json); } catch (_) {}
                  return {
                    json,
                    totalRows: Array.isArray(doc.instruments) ? doc.instruments.length : null,
                    totalNodes: Array.isArray(doc.nodes) ? doc.nodes.length : null,
                    bpm: doc.bpm, subdiv: doc.subdiv,
                  };
                });
                if (info && info.json) stashedCircuit = info.json;
                const text = info
                  ? `Exported & stashed circuit: totalRows=${info.totalRows}, totalNodes=${info.totalNodes}, bpm=${info.bpm}, subdiv=${info.subdiv}. After mutating, call import_circuit then checkpoint these values.`
                  : "export_circuit failed — exportJSON unavailable.";
                if (verbose) console.log(`  [export_circuit] ${text}`);
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content: [{ type: "text", text }], is_error: !info });
              } else if (block.name === "import_circuit") {
                // Re-import the stashed circuit JSON.
                if (!stashedCircuit) {
                  toolResults.push({ type: "tool_result", tool_use_id: block.id, content: [{ type: "text", text: "import_circuit: nothing stashed — call export_circuit first." }], is_error: true });
                } else {
                  const ok = await page.evaluate((j) => {
                    if (typeof importJSON !== "function") return false;
                    importJSON(j);
                    if (typeof forceDraw === "function") forceDraw();
                    return true;
                  }, stashedCircuit);
                  await page.waitForTimeout(300);
                  const png = await safeScreenshot();
                  const content = [{ type: "text", text: ok ? "Re-imported the stashed circuit. Now checkpoint totalRows/totalNodes/bpm/subdiv against the export_circuit values." : "import_circuit failed — importJSON unavailable." }];
                  if (png) {
                    content.push({ type: "image", source: { type: "base64", media_type: "image/png", data: png.toString("base64") } });
                    const uiHints = await buildUIHints(page);
                    if (uiHints) content.push({ type: "text", text: uiHints });
                  }
                  toolResults.push({ type: "tool_result", tool_use_id: block.id, content, is_error: !ok });
                }
              } else if (block.name === "drag") {
                // Real force-ticked drag (EQ band / knob / slider).
                const { x0, y0, x1, y1, steps } = block.input;
                const ok = await page.evaluate(([a, b, c, d, s]) => {
                  if (typeof forceDrag !== "function") return false;
                  forceDrag(a, b, c, d, s || 8);
                  if (typeof forceDraw === "function") forceDraw();
                  return true;
                }, [x0, y0, x1, y1, steps]);
                await page.waitForTimeout(150);
                const png = await safeScreenshot();
                const content = [{ type: "text", text: ok ? `Dragged (${x0},${y0})→(${x1},${y1}). Verify with checkpoint_export.` : "drag failed — forceDrag unavailable." }];
                if (png) {
                  content.push({ type: "image", source: { type: "base64", media_type: "image/png", data: png.toString("base64") } });
                  const uiHints = await buildUIHints(page);
                  if (uiHints) content.push({ type: "text", text: uiHints });
                }
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content, is_error: !ok });
              } else if (block.name === "call_export") {
                // Invoke a setter export, then apply queued UI actions.
                const res = await page.evaluate(({ exp, args }) => {
                  const fn = globalThis[exp];
                  if (typeof fn !== "function") return { err: `no export "${exp}"` };
                  let ret;
                  try { ret = fn(...(Array.isArray(args) ? args : [])); } catch (e) { return { err: String(e.message || e) }; }
                  try {
                    if (typeof forceGameTick === "function") { forceGameTick(); forceGameTick(); }
                    if (typeof forceDraw === "function") forceDraw();
                  } catch (_) {}
                  return { ret: typeof ret === "object" ? "(object)" : ret };
                }, { exp: block.input.export, args: block.input.args || [] });
                await page.waitForTimeout(120);
                const text = res.err
                  ? `call_export error: ${res.err}`
                  : `called ${block.input.export}(${JSON.stringify(block.input.args || [])}) → ${JSON.stringify(res.ret)}. Verify with checkpoint_export.`;
                if (verbose) console.log(`  [call_export] ${text}`);
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content: [{ type: "text", text }], is_error: !!res.err });
              } else if (block.name === "read_export" || block.name === "checkpoint_export") {
                // Read a value from a WASM getter (+ optional dot/bracket path).
                const inp = block.input;
                const res = await page.evaluate(({ exp, args, path }) => {
                  const fn = globalThis[exp];
                  if (typeof fn !== "function") return { err: `no export "${exp}"` };
                  let v;
                  try { v = fn(...(Array.isArray(args) ? args : [])); } catch (e) { return { err: String(e.message || e) }; }
                  if (path) {
                    for (const k of String(path).split(/[.[\]]+/).filter(Boolean)) {
                      if (v == null) break;
                      v = v[k];
                    }
                  }
                  return { value: v };
                }, { exp: inp.export, args: inp.args || [], path: inp.path || null });
                const key = `${inp.export}${inp.path ? "." + inp.path : ""}`;
                if (block.name === "read_export") {
                  const text = res.err ? `read_export error: ${res.err}` : `${key} = ${JSON.stringify(res.value)}`;
                  toolResults.push({ type: "tool_result", tool_use_id: block.id, content: [{ type: "text", text }], is_error: !!res.err });
                } else {
                  const pass = !res.err && matchesExpectation(res.value, inp.expect);
                  checkpoints.push({
                    name: inp.name,
                    pass,
                    expected: inp.expect,
                    mismatches: pass ? [] : [{ key, expected: inp.expect, actual: res.err ? "error:" + res.err : res.value }],
                    t: Date.now() - startTime,
                  });
                  const text = res.err
                    ? `CHECKPOINT_EXPORT "${inp.name}" FAIL — ${res.err}`
                    : `CHECKPOINT_EXPORT "${inp.name}" ${pass ? "PASS" : "FAIL"} — ${key}=${JSON.stringify(res.value)} vs ${JSON.stringify(inp.expect)}`;
                  if (verbose) console.log(`  [checkpoint_export] ${text}`);
                  toolResults.push({ type: "tool_result", tool_use_id: block.id, content: [{ type: "text", text }] });
                }
              } else if (block.name === "measure_audio") {
                // Hard-gate audio: record real playback → decode WAV → assert RMS > floor.
                const cpName = block.input.name || "audio_audible";
                const floor = typeof block.input.floor === "number" ? block.input.floor : 0.001;
                let rmsVal = null;
                let err = null;
                try {
                  await page.evaluate(() => {
                    try { if (typeof resumeAudio === "function") resumeAudio(); } catch (_) {}
                    if (typeof stopPlay === "function") stopPlay();
                  });
                  const downloadPromise = page.waitForEvent("download", { timeout: 20000 });
                  // Instrument ids to trigger directly (robust signal source,
                  // independent of whether the current circuit's sequencer is
                  // triggering loudly — playSound bypasses the sequencer).
                  const instIds = await page.evaluate(() => {
                    try { return (JSON.parse(exportJSON()).instruments || []).map((i) => i.id); } catch (_) { return []; }
                  });
                  await page.evaluate(async () => {
                    if (typeof startRecording === "function") await startRecording("wav24");
                    if (typeof startPlay === "function") startPlay();
                  });
                  // Fire explicit hits on every instrument across the window so the
                  // captured master has guaranteed loud transients.
                  for (let round = 0; round < 8; round++) {
                    await page.evaluate((ids) => {
                      for (const id of ids) { try { if (window.playSound) window.playSound(id, 1.0); } catch (_) {} }
                    }, instIds);
                    await page.waitForTimeout(280);
                  }
                  await page.evaluate(async () => {
                    if (typeof stopPlay === "function") stopPlay();
                    if (typeof stopRecording === "function") await stopRecording();
                  });
                  const download = await downloadPromise;
                  const dp = await download.path();
                  const bytes = fs.readFileSync(dp);
                  const entries = parseZip(bytes);
                  const wav = entries["master.wav"];
                  if (!wav) err = "no master.wav in recording zip";
                  else rmsVal = rms(decodeWav(wav).samples);
                } catch (e) {
                  err = String(e.message || e);
                }
                const pass = !err && rmsVal != null && rmsVal > floor;
                checkpoints.push({
                  name: cpName,
                  pass,
                  expected: { rmsGt: floor },
                  mismatches: pass ? [] : [{ key: "master.wav RMS", expected: `>${floor}`, actual: err ? "error:" + err : rmsVal }],
                  t: Date.now() - startTime,
                });
                const text = err
                  ? `MEASURE_AUDIO "${cpName}" FAIL — ${err}`
                  : `MEASURE_AUDIO "${cpName}" ${pass ? "PASS" : "FAIL"} — master RMS=${rmsVal.toFixed(5)} (floor ${floor})`;
                if (verbose) console.log(`  [measure_audio] ${text}`);
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content: [{ type: "text", text }] });
              } else if (block.name === "checkpoint") {
                // Record an objective assertion against the live engine state.
                const { name: cpName, expect } = block.input;
                const snap = await page.evaluate(() =>
                  typeof fullLayoutSnapshot === "function" ? fullLayoutSnapshot() : null
                );
                const verdict = compareCheckpoint(snap, expect || {});
                checkpoints.push({
                  name: cpName,
                  pass: verdict.pass,
                  expected: expect,
                  mismatches: verdict.mismatches,
                  t: Date.now() - startTime,
                });
                const detail = verdict.pass
                  ? ""
                  : " — " + verdict.mismatches.map((m) => `${m.key}: expected ${JSON.stringify(m.expected)}, got ${JSON.stringify(m.actual)}`).join("; ");
                const text = `CHECKPOINT "${cpName}" ${verdict.pass ? "PASS" : "FAIL"}${detail}`;
                if (verbose) console.log(`  [checkpoint] ${text}`);
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content: [{ type: "text", text }] });
              } else if (block.name === "delete_node_longpress") {
                // Faithful long-press → slide-to-Delete → release on a grid node.
                // Reads the live longPressDeleteRect mid-hold (popup is open) so the
                // slide targets the real Delete button. (See cdpLongPressAndSlide.)
                const { x, y } = block.input;
                let detail = "delete_node_longpress: ";
                try {
                  const info = await getCanvasInfo(page);
                  const cdp = await page.context().newCDPSession(page);
                  try {
                    await cdp.send("Input.dispatchTouchEvent", { type: "touchStart", touchPoints: [{ x: info.left + x, y: info.top + y, id: 1 }] });
                    await page.waitForTimeout(700); // hold → popup opens
                    const del = await page.evaluate(() => (typeof longPressDeleteRect === "function" ? longPressDeleteRect() : null));
                    if (del && del.w > 0 && del.h > 0) {
                      const tx = info.left + del.x + del.w / 2;
                      const ty = info.top + del.y + del.h / 2;
                      await cdp.send("Input.dispatchTouchEvent", { type: "touchMove", touchPoints: [{ x: tx, y: ty, id: 1 }] });
                      await page.waitForTimeout(120);
                      detail += "slid to Delete button and released.";
                    } else {
                      detail += "long-press popup did not open (Delete rect empty) — the press may have missed a node.";
                    }
                    await cdp.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
                  } finally {
                    await cdp.detach();
                  }
                } catch (e) {
                  detail += `error: ${e.message}`;
                }
                await page.waitForTimeout(250);
                const png = await safeScreenshot();
                const content = [{ type: "text", text: detail + " Verify with checkpoint {totalNodes:{lt:<before>}}." }];
                if (png) {
                  content.push({ type: "image", source: { type: "base64", media_type: "image/png", data: png.toString("base64") } });
                  const uiHints = await buildUIHints(page);
                  if (uiHints) content.push({ type: "text", text: uiHints });
                }
                toolResults.push({ type: "tool_result", tool_use_id: block.id, content });
              } else if (block.name === "touch_gesture") {
                // Handle custom touch_gesture tool
                await executeTouchGesture(page, block.input, verbose);

                // Delay for gesture to take effect
                await page.waitForTimeout(500);
              } else {
                // Handle computer tool actions. Weaker models sometimes call a
                // BARE action verb as the tool name (e.g. name="left_click")
                // instead of the canonical computer tool with input.action, and
                // sometimes pass a stringified coordinate. normalizeComputerAction
                // coerces both so the tap actually executes (see action_normalize.js).
                const actionInput = normalizeComputerAction(block.name, block.input);
                screenshotResult = await executeAction(page, actionInput, verbose, platform);

                // Small delay after actions to let the UI update
                if (actionInput.action !== "screenshot" && actionInput.action !== "wait") {
                  await page.waitForTimeout(300);
                }
              }

              // Take screenshot for tool result (skip for tools that handle their own results)
              const selfHandledTools = new Set([
                "query_ui", "click_ui", "repeat_click_ui",
                "open_row_menu", "menu_click", "switch_tab", "switch_view", "checkpoint",
                "set_bpm", "set_subdiv", "export_circuit", "import_circuit", "delete_node_longpress",
                "drag", "read_export", "call_export", "checkpoint_export", "measure_audio",
              ]);
              if (!selfHandledTools.has(block.name)) {
                const png = screenshotResult ?? await safeScreenshot();

                if (png) {
                  const b64 = png.toString("base64");
                  const toolResultContent = [
                    {
                      type: "image",
                      source: {
                        type: "base64",
                        media_type: "image/png",
                        data: b64,
                      },
                    },
                  ];

                  // Inject UI coordinate hints alongside every screenshot
                  const uiHints = await buildUIHints(page);
                  if (uiHints) {
                    toolResultContent.push({ type: "text", text: uiHints });
                  }

                  toolResults.push({
                    type: "tool_result",
                    tool_use_id: block.id,
                    content: toolResultContent,
                  });
                } else {
                  // Screenshot failed — send text fallback so the agent can continue
                  toolResults.push({
                    type: "tool_result",
                    tool_use_id: block.id,
                    content: [
                      {
                        type: "text",
                        text: "Screenshot temporarily unavailable — the page may be slow. Try a simpler action or wait a moment.",
                      },
                    ],
                    is_error: true,
                  });
                  consecutiveScreenshotFailures++;
                }
              }
            } catch (actionErr) {
              if (verbose) {
                console.log(`  [action] Error: ${actionErr.message}`);
              }
              agentLog.push({
                iteration: iterations,
                actionError: actionErr.message,
                t: Date.now() - startTime,
              });
              toolResults.push({
                type: "tool_result",
                tool_use_id: block.id,
                content: [
                  {
                    type: "text",
                    text: `Action failed: ${actionErr.message}. Try a different action.`,
                  },
                ],
                is_error: true,
              });
              consecutiveScreenshotFailures++;
            }
          }
        }

        // Append assistant response to conversation
        messages.push({ role: "assistant", content: response.content });

        // If there were tool uses, send results back
        if (toolResults.length > 0) {
          messages.push({ role: "user", content: toolResults });
        }

        // Bail out if page is consistently unresponsive
        if (consecutiveScreenshotFailures >= maxConsecutiveFailures) {
          if (verbose) {
            console.log(`\n[agent] Page unresponsive (${maxConsecutiveFailures} consecutive failures) — stopping`);
          }
          agentLog.push({
            iteration: iterations,
            error: `Stopped: ${maxConsecutiveFailures} consecutive screenshot/action failures`,
            t: Date.now() - startTime,
          });
          break;
        }

        // Check if agent is done
        if (response.stop_reason === "end_turn") {
          if (verbose) {
            console.log(`\n[agent] Agent finished (end_turn)`);
          }
          break;
        }

        // Pace iterations to stay within rate limits
        const iterationPaceMs = options.iterationPaceMs ?? 2000;
        if (iterations < maxIterations && !stopped && iterationPaceMs > 0) {
          await new Promise((r) => setTimeout(r, iterationPaceMs));
        }
      }

      if (iterations >= maxIterations && verbose) {
        console.log(`\n[agent] Reached max iterations (${maxIterations})`);
      }

      const durationMs = Date.now() - startTime;
      const estimatedCost = estimateCost(totalInputTokens, totalOutputTokens, model);

      const result = {
        task: taskPrompt,
        model,
        platform,
        iterations,
        durationMs,
        inputTokens: totalInputTokens,
        outputTokens: totalOutputTokens,
        estimatedCost,
        issues: [...new Set(issues)], // Deduplicate
        checkpoints,
        agentLog,
        summary: extractSummary(agentLog),
      };

      if (verbose) {
        console.log(`\n[agent] Done: ${iterations} iterations, ${(durationMs / 1000).toFixed(1)}s, ~$${estimatedCost.toFixed(4)}`);
        if (result.issues.length > 0) {
          console.log(`[agent] Issues found: ${result.issues.length}`);
        }
      }

      return result;
    },

    stop() {
      stopped = true;
    },
  };
}

/**
 * Prune old screenshot images from conversation history to prevent unbounded growth.
 * Keeps the first message (task prompt + initial screenshot) and the last `keepRecent`
 * message pairs intact. For older messages, replaces base64 image content with a
 * text placeholder.
 *
 * @param {Array} messages - The conversation messages array (mutated in place)
 * @param {number} keepRecent - Number of recent message pairs to keep images for (default: 6)
 */
function pruneConversationImages(messages, keepRecent = 6) {
  const protectedTail = keepRecent * 2;
  const pruneEnd = messages.length - protectedTail;

  for (let i = 1; i < pruneEnd; i++) {
    const msg = messages[i];
    if (!msg.content || !Array.isArray(msg.content)) continue;

    msg.content = msg.content.map((block) => {
      if (block.type === "image" && block.source?.type === "base64") {
        return { type: "text", text: "[screenshot pruned from history]" };
      }
      if (block.type === "tool_result" && Array.isArray(block.content)) {
        block.content = block.content.map((inner) => {
          if (inner.type === "image" && inner.source?.type === "base64") {
            return { type: "text", text: "[screenshot pruned from history]" };
          }
          return inner;
        });
      }
      return block;
    });
  }
}

/**
 * Estimate API cost based on token counts.
 */
function estimateCost(inputTokens, outputTokens, model) {
  const isHaiku = model.includes("haiku");
  const inputRate = isHaiku ? 0.80 : 3.00;
  const outputRate = isHaiku ? 4.00 : 15.00;
  return (inputTokens / 1_000_000) * inputRate + (outputTokens / 1_000_000) * outputRate;
}

/**
 * Extract a summary from the last text block in the agent log.
 */
function extractSummary(agentLog) {
  for (let i = agentLog.length - 1; i >= 0; i--) {
    const entry = agentLog[i];
    if (!entry.response?.content) continue;
    for (let j = entry.response.content.length - 1; j >= 0; j--) {
      const block = entry.response.content[j];
      if (block.type === "text" && block.text) {
        return block.text;
      }
    }
  }
  return "";
}
