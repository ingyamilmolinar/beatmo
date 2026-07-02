/**
 * WASM bridge smoke test
 *
 * THIS IS THE CANONICAL HOME for asserting that the JS↔Go WASM bridge is wired
 * up. Every JS export registered by `src/go/internal/ui/js_exports_*.go` MUST
 * appear in the catalogue below — it's a registry of the JS surface area.
 *
 * What this test does:
 *   1. Boots the play_ui WASM harness with a default circuit.
 *   2. Asserts every catalogued export name is a function on the global scope
 *      after WASM init (existence check — 260+ exports).
 *   3. For exports with safe, well-defined args, calls them and verifies the
 *      return value's shape (typeof / Array.isArray / required keys).
 *
 * What this test DOES NOT do (intentionally):
 *   - Verify that the underlying Go logic is correct. That's covered by the
 *     Go suite (`internal/ui`, `core/engine`, `core/model`, `internal/audio`,
 *     `internal/timeline`). If you care about logic correctness, write a Go
 *     test, not a browser test.
 *   - Verify WebAudio/AudioWorklet behavior. That belongs in the
 *     `webaudio_*` / `worklet_*` / `recording_*` browser tests.
 *
 * When adding a new JS export in Go:
 *   - Add an entry to the catalogue below (existence at minimum, callable if
 *     safe args exist). Do NOT create a new dedicated `*.browser.test.js`
 *     file just to verify the export got registered.
 */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  flushCoverage,
  isCoverageEnabled,
} from "./browser_test_helpers.js";
import { smokeExport, assertExportExists } from "./bridge_smoke_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// ─── Harness boot (same pattern as wasm_input_sanity.browser.test.js) ───
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    {
      cwd: goDir,
      env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
      stdio: "inherit",
    },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("console", (msg) => {
  try {
    const t = msg.text();
    if (t.startsWith("[bridge-smoke]") || msg.type() === "error") {
      console.log("[PAGE]", msg.type(), t);
    }
  } catch (_) {}
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof ensureDefaultPath === "function");
await page.evaluate(() => ensureDefaultPath());
// Seed a known node so node-id-based exports have something to work with.
await page.evaluate(() => {
  if (typeof addNode === "function" && typeof nodeIdAt === "function") {
    if (nodeIdAt(0, 0) < 0) addNode(0, 0, "regular");
  }
});
await page.waitForTimeout(100);

// ─── Catalogue: every JS export, grouped by source file ───────────────────
//
// Each entry is one of:
//   { name }                                      — existence-only (skipCall: true)
//   { name, args, returns }                       — call and check shape
//
// `returns` shape grammar — see bridge_smoke_helpers.js:
//   "boolean" | "number" | "string" | "object" | "array" | "undefined"
//   "any"                                            (matches anything)
//   "nullable"                                       (null/undefined accepted)
//   { kind: "object", keys: ["x","y"] }              (required keys subset)

const CATALOGUE = {
  // ───────── coverage (js_exports_coverage.go) ─────────
  coverage: [
    { name: "clearGoCoverage", args: [], returns: "any", skipCall: false },
    // flushGoCoverage writes a file; existence-only.
    { name: "flushGoCoverage", skipCall: true },
  ],

  // ───────── eq + scope + widgets (js_exports_eq_widgets.go) ─────────
  eq_widgets: [
    { name: "addCustomWidget", skipCall: true },
    { name: "downloadScopeExport", skipCall: true },
    { name: "enableScopeExport", args: [false], returns: "any" },
    { name: "eqBandsSnapshot", args: [], returns: "any" },
    { name: "eqControlsSnapshot", args: [], returns: "any" },
    { name: "eqFreqResponse", args: [], returns: "any" },
    { name: "forceScopeExportSnapshot", skipCall: true },
    { name: "fullLayoutSnapshot", args: [], returns: "any" },
    { name: "moveWidget", skipCall: true },
    { name: "nudgeWidgetSplit", skipCall: true },
    { name: "probeAnalyzerState", args: [], returns: "any" },
    { name: "probeScopeState", args: [], returns: "any" },
    { name: "scopeExportBufferLen", args: [], returns: "number" },
    { name: "setEQChannel", skipCall: true }, // mutating; covered by chain_per_instrument_traces.browser.test.js
    { name: "setEQHPF", skipCall: true },
    { name: "setEQLPF", skipCall: true },
    { name: "setEQTab", args: [0], returns: "any" },
    { name: "setEQView", args: [0], returns: "any" },
    // setViewMode(slug): mobile bottom-nav / view-mode switch used by the LLM
    // agent sanity tests. "pads" = the default Rows view (safe to call).
    { name: "setViewMode", args: ["pads"], returns: "any" },
    // setSubdivisions(n): direct deterministic subdiv setter (agent set_subdiv tool).
    { name: "setSubdivisions", args: [8], returns: "boolean" },
    // Audio-panel chrome read-back getters (agent sanity-test checkpoints).
    { name: "spectrumSlope", args: [], returns: "any" },
    { name: "preOverlay", args: [], returns: "any" },
    { name: "k20View", args: [], returns: "any" },
    { name: "freqScaleLog", args: [], returns: "any" },
    { name: "chainDisplayMode", args: [], returns: "any" },
    { name: "autoGain", args: [], returns: "any" },
    { name: "scopeFrozen", args: [], returns: "any" },
    // Synth-tab redesign exports (per hey-please-review-the-vectorized-rocket.md).
    // synthKnobRects returns one entry per wired knob, but collapsed (non-
    // selected) stages have zero-size rects (w=0,h=0) — only the selected
    // stage's knobs are drag-targetable in the chip-strip layout.
    { name: "synthKnobRects", args: [], returns: "any" },
    // Pipeline chip strip: synthChipRects lists one chip per stage in pipeline
    // order; selectSynthSection opens a stage by chip label (next frame).
    { name: "synthChipRects", args: [], returns: "any" },
    { name: "selectSynthSection", args: ["OSC"], returns: "any" },
    // Synth footer + Save As dialog (added in the synth-footer fix; see
    // src/js/synth_panel_footer.browser.test.js for the end-to-end flow).
    { name: "synthFooterButtonRects", args: [], returns: "any" },
    { name: "synthSaveAsDialogState", args: [], returns: "any" },
    { name: "synthSaveAsDialogSetValue", skipCall: true }, // mutating
    { name: "synthSaveAsDialogConfirm", skipCall: true },  // mutating; closes dialog if open
    { name: "synthSaveAsDialogCancel", skipCall: true },   // mutating; closes dialog if open
    { name: "setScopeTaps", skipCall: true },
    { name: "toggleWidget", skipCall: true },
    { name: "widgetLayoutSnapshot", args: [], returns: "any" },
  ],

  // ───────── graph + UI buttons + rows (js_exports_graph_ui.go) ─────────
  graph_ui: [
    { name: "addDrumRow", skipCall: true },
    { name: "addEdgeGrid", skipCall: true }, // mutating; covered by graph Go tests
    { name: "addNode", skipCall: true }, // mutating; covered by graph Go tests
    { name: "addRowBtnRect", args: [], returns: "any" },
    { name: "applySubdivValue", skipCall: true },
    { name: "bpmBoxRect", args: [], returns: "any" },
    { name: "bpmDecBtnRect", args: [], returns: "any" },
    { name: "bpmIncBtnRect", args: [], returns: "any" },
    { name: "clickTimelineAt", skipCall: true },
    { name: "closeAllPopups", args: [], returns: "any" },
    { name: "closeInstMenu", args: [], returns: "any" },
    { name: "closeNodeMenu", args: [], returns: "any" },
    { name: "colorMenuOpenState", args: [], returns: "any" },
    { name: "colorWheelRect", args: [], returns: "any" },
    { name: "commitRename", skipCall: true },
    { name: "contextMenuClick", skipCall: true },
    { name: "contextMenuItems", args: [], returns: "any" },
    { name: "contextMenuOpen", args: [], returns: "boolean" },
    { name: "contextMenuRect", args: [], returns: "any" },
    { name: "contextMenuRow", args: [], returns: "number" },
    { name: "debugGridInputState", args: [], returns: "any" },
    { name: "deleteEdgeGrid", skipCall: true },
    { name: "deleteNodeGrid", skipCall: true },
    { name: "drumBounds", args: [], returns: "any" },
    { name: "drumLength", args: [], returns: "number" },
    { name: "drumOffset", args: [], returns: "number" },
    { name: "ensureDefaultPath", args: [], returns: "any" },
    { name: "exportBtnRect", args: [], returns: "any" },
    { name: "exportJSON", args: [], returns: "string" },
    { name: "fxPanelOpen", args: [], returns: "boolean" },
    { name: "fxPanelRect", args: [], returns: "any" },
    { name: "fxPanelRow", args: [], returns: "number" },
    { name: "getAppliedBPM", args: [], returns: "number" },
    { name: "getBPM", args: [], returns: "number" },
    { name: "getClickNode", args: [], returns: "any" },
    { name: "getEngineBPM", args: [], returns: "number" },
    { name: "getFastPath", args: [], returns: "boolean" },
    { name: "getLeftPrev", args: [], returns: "any" },
    { name: "getPendingClick", args: [], returns: "any" },
    { name: "importBtnRect", args: [], returns: "any" },
    { name: "importJSON", skipCall: true }, // mutates; round-trip tests live in Go
    { name: "instMenuBackBtnRect", args: [], returns: "any" },
    { name: "instMenuBreadcrumbPath", args: [], returns: "any" },
    { name: "instMenuCategoryRects", args: [], returns: "any" },
    { name: "instMenuClickBack", skipCall: true },
    { name: "instMenuDebugState", args: [], returns: "any" },
    { name: "instMenuFavoritesViewActive", args: [], returns: "boolean" },
    { name: "instMenuHasScroll", args: [], returns: "boolean" },
    { name: "instMenuItemRects", args: [], returns: "any" },
    { name: "instMenuOpenState", args: [], returns: "any" },
    { name: "instMenuPageState", args: [], returns: "any" },
    { name: "instMenuRenderedOrder", args: [], returns: "any" },
    { name: "instMenuScrollBarRect", args: [], returns: "any" },
    { name: "instMenuScrollOffset", args: [], returns: "number" },
    { name: "instMenuSearchRect", args: [], returns: "any" },
    { name: "instMenuScrollThumbRect", args: [], returns: "any" },
    { name: "instMenuSelectCategory", skipCall: true },
    { name: "instMenuSelectItem", skipCall: true },
    { name: "instrumentFavoriteKeys", args: [], returns: "any" },
    { name: "instrumentIsFavorite", args: ["test-only-id"], returns: "boolean" },
    { name: "instrumentSetFavorite", skipCall: true },
    { name: "projectPinsList", args: [], returns: "any" },
    { name: "setProjectPinsListForTest", skipCall: true }, // test-only mutation
    { name: "instOptions", args: [], returns: "any" },
    { name: "isNamingOpen", args: [], returns: "boolean" },
    { name: "isPlaying", args: [], returns: "boolean" },
    { name: "isUploading", args: [], returns: "boolean" },
    { name: "kbProxyFocused", args: [], returns: "boolean" },
    { name: "kbProxyInputMode", args: [], returns: "any" },
    { name: "layoutHorizontal", args: [], returns: "boolean" },
    { name: "lenDecBtnRect", args: [], returns: "any" },
    { name: "lenIncBtnRect", args: [], returns: "any" },
    { name: "longPressDeleteRect", args: [], returns: "any" },
    { name: "mainVolume", args: [], returns: "number" },
    { name: "mobileInputActive", args: [], returns: "any" },
    { name: "mobileInputAnyActive", args: [], returns: "boolean" },
    { name: "moveNodeGrid", skipCall: true },
    { name: "nodeActionAt", skipCall: true },
    { name: "nodeInfo", args: [-1], returns: "any" }, // -1 => safe miss
    { name: "nodeMenuAction", skipCall: true },
    { name: "nodeMenuNodeId", args: [], returns: "number" },
    { name: "nodeMenuOpen", args: [], returns: "boolean" },
    { name: "nodeMenuRect", args: [], returns: "any" },
    { name: "nodeParams", args: [-1], returns: "any" },
    { name: "openColorMenu", skipCall: true },
    { name: "openContextMenuJS", skipCall: true },
    { name: "openFXPanelJS", skipCall: true },
    { name: "openInstMenu", skipCall: true },
    { name: "openNodeSidebar", skipCall: true },
    { name: "openRenameBox", skipCall: true },
    { name: "openSubdivMenu", skipCall: true },
    { name: "pickColorAtWheel", skipCall: true },
    { name: "playBtnRect", args: [], returns: "any" },
    { name: "recBtnRect", args: [], returns: "any" },
    { name: "portalStackLen", args: [], returns: "number" },
    { name: "portalTopID", args: [], returns: "any" },
    { name: "renameBoxRect", args: [], returns: "any" },
    { name: "rowColor", args: [0], returns: "any" },
    { name: "rowColorBtnRect", args: [0], returns: "any" },
    { name: "rowEditBtnRect", args: [0], returns: "any" },
    { name: "rowFXBtnRect", args: [0], returns: "any" },
    { name: "rowInstrument", args: [0], returns: "any" },
    { name: "rowLabelRect", args: [0], returns: "any" },
    { name: "rowLabelText", args: [0], returns: "string" },
    { name: "rowMuteBtnRect", args: [0], returns: "any" },
    { name: "rowMuted", args: [0], returns: "boolean" },
    { name: "rowOffset", args: [0], returns: "number" },
    { name: "rowSoloBtnRect", args: [0], returns: "any" },
    { name: "rowSoloed", args: [0], returns: "boolean" },
    { name: "rowVolume", args: [0], returns: "number" },
    { name: "scrollBarRect", args: [], returns: "any" },
    { name: "scrollThumbRect", args: [], returns: "any" },
    { name: "selectedNodeId", args: [], returns: "any" },
    { name: "setDrumLength", skipCall: true },
    { name: "setFastPath", skipCall: true },
    { name: "setMainVolume", skipCall: true },
    { name: "setNodeLogicGrid", skipCall: true },
    { name: "setOrigin", skipCall: true },
    { name: "setRowInstrument", skipCall: true },
    { name: "setRowOffset", skipCall: true },
    { name: "setRowStep", skipCall: true },
    { name: "setRowVolume", skipCall: true },
    { name: "setSplitY", skipCall: true },
    { name: "setTimelineBeats", skipCall: true },
    { name: "sidebarContentHeight", args: [], returns: "number" },
    { name: "sidebarDebugState", args: [], returns: "any" },
    { name: "sidebarExpandAllSections", skipCall: true },
    { name: "sidebarHasScroll", args: [], returns: "boolean" },
    { name: "sidebarPanelRect", args: [], returns: "any" },
    { name: "sidebarScrollBarRect", args: [], returns: "any" },
    { name: "sidebarScrollOffset", args: [], returns: "number" },
    { name: "sidebarScrollThumbRect", args: [], returns: "any" },
    { name: "sidebarSectionOpen", args: ["transport"], returns: "any" },
    { name: "sidebarSectionRect", args: ["transport"], returns: "any" },
    { name: "sliderRect", args: [0], returns: "any" },
    { name: "splitX", args: [], returns: "number" },
    { name: "splitY", args: [], returns: "number" },
    { name: "stopBtnRect", args: [], returns: "any" },
    { name: "subdivBtnRect", args: [], returns: "any" },
    { name: "subdivMenuItemRects", args: [], returns: "any" },
    { name: "timelineBeats", args: [], returns: "number" },
    { name: "timelineRect", args: [], returns: "any" },
    { name: "timelineUnitsPerBeat", args: [], returns: "number" },
    { name: "toggleMute", skipCall: true },
    { name: "toggleSolo", skipCall: true },
    { name: "totalNodes", args: [], returns: "number" },
    { name: "totalRows", args: [], returns: "number" },
    { name: "totalVisibleRows", args: [], returns: "number" },
    { name: "triggerOnce", skipCall: true },
    { name: "updateBeatInfos", args: [], returns: "any" },
    { name: "uploadBtnRect", args: [], returns: "any" },
  ],

  // ───────── harness extras (js_exports_harness.go) ─────────
  harness: [
    { name: "buildPerfRect", args: [], returns: "any" },
    { name: "camOffset", args: [], returns: { kind: "object", keys: ["x", "y"] } },
    { name: "camScale", args: [], returns: "number" },
    { name: "centerCamera", skipCall: true },
    { name: "currentBeat", args: [], returns: "number" },
    { name: "debugDrumLayout", args: [], returns: "any" },
    { name: "debugDrumRender", args: [], returns: "any" },
    { name: "drumRowCount", args: [], returns: "number" },
    { name: "gridToScreen", args: [0, 0], returns: { kind: "object", keys: ["x", "y"] } },
    { name: "incrementBPM", skipCall: true },
    { name: "instrumentsList", args: [], returns: "any" },
    { name: "nodeHighlightedAt", args: [0, 0], returns: "boolean" },
    { name: "nodeIdAt", args: [0, 0], returns: "number" },
    { name: "nodeRect", args: [0, 0], returns: "any" },
    { name: "openNodeMenu", skipCall: true },
    { name: "panBy", skipCall: true },
    { name: "rowsContentVisibleCount", args: [], returns: "number" },
    { name: "setCamOffset", skipCall: true },
    { name: "setCamScale", skipCall: true },
    { name: "setNodeLogicCallbackGrid", skipCall: true },
    { name: "uiLayoutOk", args: [], returns: "boolean" },
    { name: "visibleRows", args: [], returns: "number" },
    { name: "zoomAt", skipCall: true },
  ],

  // ───────── init (js_exports_init.go) ─────────
  // initJS() runs at WASM start and only fans out to per-subsystem
  // registrar functions; it has no JS-callable exports of its own.
  init: [],

  // ───────── insert effects (js_exports_insert_effects.go) ─────────
  insert_effects: [
    { name: "addInsertEffect", skipCall: true }, // covered by Go internal/audio
    // Returns a native JS array of {type, enabled, params} — do NOT JSON.parse.
    { name: "getInsertEffects", args: ["kick"], returns: "array" },
    // Returns a native JS object keyed by effect type — do NOT JSON.parse.
    { name: "insertEffectCatalog", args: [], returns: "object" },
    { name: "moveInsertEffect", skipCall: true },
    { name: "removeInsertEffect", skipCall: true },
    { name: "setInsertEffectParam", skipCall: true },
    { name: "toggleInsertEffect", skipCall: true },
  ],

  // ───────── synth recipe (js_exports_synth_recipe.go, Phase 5) ─────────
  // Per-instrument SynthRecipe params + recipe registry. Mirrors the
  // insert-effect surface so browser tests can drive the registry
  // identically. setInstrumentParam mutates audio state; we skip the
  // call here and let instrument_params.browser.test.js exercise it.
  synth_recipe: [
    { name: "setInstrumentParam", skipCall: true },
    { name: "setInstrumentParams", skipCall: true },
    // Mutates the active audio-panel tab — existence-only.
    { name: "setActiveEQTab", skipCall: true },
    { name: "getInstrumentParams", args: ["kick"], returns: "object" },
    { name: "resetInstrumentParams", skipCall: true },
    // Mirror the Synth-tab Save / Reset buttons for an explicit instrument id.
    // They mutate the recipe registry; behavior is covered by
    // webaudio_synth_save_persist.browser.test.js, so skipCall here.
    { name: "saveActiveRecipe", skipCall: true },
    { name: "resetActiveRecipe", skipCall: true },
    { name: "recipeForInstrument", args: ["kick"], returns: "string" },
    // Stage 5 debug bridge: renders the right-pane synth mirror for the active
    // synth instrument and returns the PCM sample count (0 when no synth tab is
    // active). Safe + idempotent to call. DSP correctness is covered in Go
    // (synth_mirror_test.go); this only checks the WASM↔audio boundary.
    { name: "synthMirrorPCMLen", args: [], returns: "number" },
    // 32-bit fingerprint of the mirror PCM content; a browser test compares it
    // before/after a knob change to prove the WASM mirror reacts to params.
    { name: "synthMirrorPCMChecksum", args: [], returns: "number" },
    // Returns a native JS object: { recipeID: { displayName, category, params } }.
    { name: "synthRecipeCatalog", args: [], returns: "object" },
    { name: "recipeDefaultParams", args: ["drum-kick"], returns: "object" },
    // Phase 5: user-recipe lifecycle (save / clone / delete / list /
    // export / import). The write paths mutate persistent storage
    // (localStorage) — skipCall here keeps the smoke test idempotent.
    // existence-only assertion catches "Go forgot to register the
    // export" regressions; semantic coverage lives in dedicated tests.
    { name: "saveUserRecipe", skipCall: true },
    { name: "createUserRecipe", skipCall: true },
    { name: "deleteUserRecipe", skipCall: true },
    { name: "listUserRecipes", args: [], returns: "object" },
    { name: "exportUserRecipe", args: ["drum-snare"], returns: "string" },
    { name: "importUserRecipe", skipCall: true },
    // Phase 6: kit (instrument-set / drum-kit) infra. No UI in this
    // round; the exports exist for browser tests + future picker UI.
    // applyKit / createKit mutate process state — skipCall keeps the
    // smoke test idempotent. listKits is read-only.
    { name: "applyKit", skipCall: true },
    { name: "listKits", args: [], returns: "object" },
    { name: "createKit", skipCall: true },
    { name: "setKitMember", skipCall: true },
    { name: "deleteKit", skipCall: true },
  ],

  // ───────── sampler tab (sampler_panel_zone.go + audio.js bridge) ─────────
  // The Go↔WebAudio seam for the Sampler tab. registerSamplePCM installs a
  // baked PCM buffer as a playable instrument; captureInstrumentPCM returns a
  // rendered one-shot for "From Synth"; decodeWavToPCM decodes a picked WAV for
  // "Load WAV". All take typed-array / URL args or mutate audio state, so
  // skipCall — semantic coverage lives in sampler_register_pcm.browser.test.js.
  sampler: [
    { name: "registerSamplePCM", skipCall: true },
    { name: "captureInstrumentPCM", skipCall: true },
    { name: "decodeWavToPCM", skipCall: true },
    // Cross-session sample persistence (IndexedDB) — wasm SampleStore backend.
    { name: "idbGetAllSamples", skipCall: true },
    { name: "idbPutSample", skipCall: true },
    { name: "idbDeleteSample", skipCall: true },
  ],

  // ───────── playback + perf (js_exports_playback_perf.go) ─────────
  playback_perf: [
    { name: "forceDraw", args: [], returns: "any" },
    { name: "forceGameTick", args: [], returns: "any" },
    { name: "forceDrag", args: [10, 10, 20, 20, 2], returns: "any" },
    {
      name: "gridCacheInfo",
      args: [],
      returns: { kind: "object", keys: ["tileReady", "cacheReady", "simpleDraw"] },
    },
    { name: "perfStats", args: [], returns: "object" },
    { name: "resetPerfStats", args: [], returns: "any" },
    { name: "rowsLayerState", args: [], returns: "any" },
    { name: "setAudioLookahead", skipCall: true },
    { name: "setBPM", skipCall: true },
    { name: "setPerfFastPath", skipCall: true },
    { name: "setSimpleDraw", skipCall: true },
    { name: "slimBarDiag", args: [], returns: "any" }, // drum slim-bar render diagnostic
    { name: "startPlay", skipCall: true },
    { name: "stopPlay", skipCall: true },
    { name: "syncHighlights", args: [], returns: "any" },
    // Diagnostics + perf probes (read-only; safe to call).
    { name: "dumpHeapProbe", args: [], returns: "any" },
    { name: "dumpLevelsLatch", args: [], returns: "any" },
    { name: "getThreeStageLatency", args: [], returns: "any" },
    // Mutating perf/sequencer controls — existence-only.
    { name: "resetThreeStageLatency", skipCall: true },
    { name: "tickSequencer", skipCall: true },
  ],

  // ───────── diagnostics (js_exports_diag.go) ─────────
  diag: [
    { name: "analyzerBridgeStats", args: [], returns: "any" },
    { name: "memSizes", args: [], returns: "any" },
    { name: "resetAnalyzerBridgeStats", skipCall: true },
  ],

  // ───────── media session (game_media_session_wasm.go) ─────────
  // Start/stop playback as side effects — existence-only.
  media_session: [
    { name: "mediaSessionPlay", skipCall: true },
    { name: "mediaSessionPause", skipCall: true },
  ],

  // ───────── recording (js_exports_recording.go) ─────────
  // Recording requires real WebAudio + AudioWorklet; out of scope for smoke.
  // These are existence-only here — the recording_*.browser.test.js suite
  // owns their behavioral testing.
  recording: [
    { name: "availableFormats", args: [], returns: "any" },
    { name: "isRecording", args: [], returns: "boolean" },
    { name: "recordingElapsedMs", args: [], returns: "number" },
    { name: "saveRecording", skipCall: true },
    { name: "startRecording", skipCall: true },
    { name: "stopRecording", skipCall: true },
  ],

  // ───────── scenes + popups + master mix (js_exports_scenes.go) ─────────
  scenes: [
    // Mobile file-picker overlay → Go entry points: the real <input type=file>
    // overlay's change handler calls these to run the import/upload flow after a
    // pick (see mobile_import_gesture.browser.test.js). Mutating; existence-only.
    { name: "_fpStartImport", skipCall: true },
    { name: "_fpStartUpload", skipCall: true },
    { name: "closeColorMenu", args: [], returns: "any" },
    { name: "closeContextMenu", args: [], returns: "any" },
    { name: "closeFXPanel", args: [], returns: "any" },
    { name: "closeInstrumentMenu", args: [], returns: "any" },
    { name: "closeOverflowMenu", args: [], returns: "any" },
    { name: "closeSubdivMenu", args: [], returns: "any" },
    { name: "closeVolumePopup", args: [], returns: "any" },
    { name: "listScenes", args: [], returns: "any" },
    { name: "listSubjects", args: [], returns: "any" },
    { name: "openColorMenu", skipCall: true },
    { name: "openContextMenu", skipCall: true },
    { name: "openFXPanel", skipCall: true },
    { name: "openInstrumentMenu", skipCall: true },
    { name: "openMasterVolumePopup", skipCall: true },
    { name: "openOverflowMenu", skipCall: true },
    { name: "openSubdivMenu", skipCall: true },
    { name: "openVolumePopup", skipCall: true },
    { name: "runScene", skipCall: true },
    { name: "runSceneMobile", skipCall: true },
    { name: "setEQBandGain", skipCall: true },
    { name: "setMasterVolume", skipCall: true },
    { name: "subjectRectJS", args: [""], returns: "any" },
  ],

  // ───────── timeline + predictor (js_exports_timeline_predictor.go) ─────────
  timeline_predictor: [
    { name: "audibleAt", args: [0, 0], returns: "boolean" },
    { name: "beatInfoAt", args: [0, 0], returns: "any" },
    { name: "clearSchedulerMismatches", args: [], returns: "any" },
    { name: "dumpRowState", args: [0], returns: "any" },
    { name: "dumpTimelineSegments", args: [0], returns: "any" },
    { name: "ensure", args: [16], returns: "any" },
    { name: "gridSubdiv", args: [], returns: "number" },
    { name: "hasAnyRowHighlight", args: [], returns: "boolean" },
    { name: "hasHighlight", args: [0], returns: "boolean" },
    { name: "hasRealtimeHighlight", args: [0], returns: "boolean" },
    { name: "nextBeatIdxs", args: [], returns: "any" },
    { name: "nodeAnimValue", args: [0, 0], returns: "any" },
    { name: "nodeHighlightUntilValue", args: [0, 0], returns: "any" },
    { name: "nodeSuccessorsGrid", args: [0, 0], returns: "any" },
    { name: "predictorAudibleSnapshot", args: [], returns: "any" },
    { name: "recentSchedulerMismatches", args: [], returns: "any" },
    { name: "rowBeatCount", args: [0], returns: "number" },
    { name: "rowCacheOffset", args: [0], returns: "any" },
    { name: "rowSteps", args: [0], returns: "any" },
    { name: "rowWindow", args: [0], returns: "any" },
    { name: "setFollow", skipCall: true },
    { name: "timelineCommittedRange", args: [], returns: "any" },
    { name: "togglePlay", skipCall: true },
    { name: "transportSnapshot", args: [], returns: { kind: "object", keys: ["playing"] } },
    { name: "triggeredAt", args: [0, 0], returns: "boolean" },
    { name: "visibleAt", args: [0, 0], returns: "boolean" },
  ],

  // ───────── touch debug (js_exports_touch_debug.go) ─────────
  touch_debug: [
    { name: "clearTouchEventLog", args: [], returns: "any" },
    { name: "getDevicePixelRatio", args: [], returns: "number" },
    { name: "getTouchScreenSize", args: [], returns: "any" },
    { name: "isSmallScreenMode", args: [], returns: "boolean" },
    { name: "setTouchDebug", skipCall: true },
    { name: "touchDebugState", args: [], returns: "any" },
    { name: "touchEventLog", args: [], returns: "any" },
  ],
};

// ─── Run the smoke pass ────────────────────────────────────────────────────
let totalExports = 0;
let totalCalled = 0;
const failures = [];

for (const [group, entries] of Object.entries(CATALOGUE)) {
  console.log(`[bridge-smoke] group: ${group} (${entries.length} exports)`);
  for (const entry of entries) {
    totalExports++;
    try {
      // Always assert existence.
      await assertExportExists(page, entry.name);
      if (!entry.skipCall) {
        await smokeExport(page, entry);
        totalCalled++;
      }
    } catch (err) {
      failures.push({ group, name: entry.name, error: String(err.message || err) });
    }
  }
}

if (failures.length > 0) {
  console.error("[bridge-smoke] FAILURES:");
  for (const f of failures) {
    console.error(`  ${f.group}/${f.name}: ${f.error}`);
  }
}

console.log(
  `[bridge-smoke] catalogued=${totalExports} called=${totalCalled} existence-only=${totalExports - totalCalled} failures=${failures.length}`,
);

// Verify catalogue covers the actual JS export surface — fail if Go added an
// export we haven't catalogued, so this test stays the canonical registry.
const catalogueNames = new Set();
for (const entries of Object.values(CATALOGUE)) {
  for (const e of entries) catalogueNames.add(e.name);
}
const missingFromCatalogue = await page.evaluate((knownNames) => {
  const known = new Set(knownNames);
  // We can't enumerate "all Go-registered exports" from JS directly, but we
  // can spot-check a curated set of names that MUST be present and report
  // any name in `known` that isn't a function (which would mean the
  // catalogue lists a nonexistent export — also a problem).
  const ghosts = [];
  for (const n of known) {
    if (typeof globalThis[n] !== "function") ghosts.push(n);
  }
  return ghosts;
}, [...catalogueNames]);

if (missingFromCatalogue.length > 0) {
  console.error(
    `[bridge-smoke] catalogued names not exposed as functions: ${JSON.stringify(missingFromCatalogue)}`,
  );
}

// ─── fullLayoutSnapshot enriched-fields check (LLM agent sanity-test deps) ──
// The agent harness drives the new audio-panel tabs / mobile bottom-nav and
// verifies objective state via these fields. Assert their shape + plumbing.
{
  const snapCheck = await page.evaluate(() => {
    const out = { errs: [] };
    const snap = typeof fullLayoutSnapshot === "function" ? fullLayoutSnapshot() : null;
    if (!snap) { out.errs.push("fullLayoutSnapshot returned null"); return out; }
    const st = snap.state || {};
    if (typeof st.subdiv !== "number") out.errs.push(`state.subdiv not number: ${st.subdiv}`);
    if (typeof st.activeTab !== "string") out.errs.push(`state.activeTab not string: ${st.activeTab}`);
    if (typeof st.viewMode !== "string") out.errs.push(`state.viewMode not string: ${st.viewMode}`);
    if (typeof st.channel !== "string") out.errs.push(`state.channel not string: ${st.channel}`);
    if (!snap.tabs || typeof snap.tabs !== "object") out.errs.push("snap.tabs missing");
    else if (!snap.tabs.eq) out.errs.push("snap.tabs.eq missing");
    if (!snap.bottomNav || typeof snap.bottomNav !== "object") out.errs.push("snap.bottomNav missing");
    if (!snap.chrome || typeof snap.chrome !== "object") out.errs.push("snap.chrome missing");
    // Chrome read-back getters must be callable and return bool/number/null (not throw).
    for (const g of ["spectrumSlope", "preOverlay", "k20View", "freqScaleLog", "chainDisplayMode", "autoGain", "scopeFrozen"]) {
      if (typeof globalThis[g] !== "function") { out.errs.push(`${g} not a function`); continue; }
      try {
        const v = globalThis[g]();
        if (v !== null && typeof v !== "boolean" && typeof v !== "number") out.errs.push(`${g}() returned ${typeof v}`);
      } catch (e) { out.errs.push(`${g}() threw: ${e.message}`); }
    }
    if (typeof forceDrag !== "function") out.errs.push("forceDrag not a function");

    // activeTab plumbing via setEQTab (incl. new canonical slugs).
    if (typeof setEQTab === "function") {
      setEQTab("wave");
      const a = fullLayoutSnapshot().state.activeTab;
      if (a !== "wave") out.errs.push(`setEQTab(wave) → activeTab=${a}`);
      setEQTab("sampler");
      const b = fullLayoutSnapshot().state.activeTab;
      if (b !== "sampler") out.errs.push(`setEQTab(sampler) → activeTab=${b}`);
      setEQTab("eq");
    }
    // viewMode plumbing via setViewMode.
    if (typeof setViewMode === "function") {
      setViewMode("eq");
      const v = fullLayoutSnapshot().state.viewMode;
      if (v !== "eq") out.errs.push(`setViewMode(eq) → viewMode=${v}`);
      setViewMode("pads");
      const p = fullLayoutSnapshot().state.viewMode;
      if (p !== "pads") out.errs.push(`setViewMode(pads) → viewMode=${p}`);
    }
    // Deterministic BPM/subdiv setters used by the agent's set_bpm/set_subdiv tools.
    // Subdivision can only change while STOPPED (validateSubdivisions guards on
    // g.Playing()), which mirrors the test flow (Phase 2 stops before Phase 3).
    if (typeof stopPlay === "function") stopPlay();
    if (typeof setBPM === "function") {
      setBPM(90);
      const b = fullLayoutSnapshot().state.bpm;
      if (b !== 90) out.errs.push(`setBPM(90) → state.bpm=${b}`);
    }
    // New state fields used by the agentic build/edit/gesture tests.
    if (typeof st.totalNodes !== "number") out.errs.push(`state.totalNodes not number: ${st.totalNodes}`);
    if (typeof st.camScale !== "number") out.errs.push(`state.camScale not number: ${st.camScale}`);
    if (typeof st.camOffsetX !== "number") out.errs.push(`state.camOffsetX not number: ${st.camOffsetX}`);
    if (typeof st.camOffsetY !== "number") out.errs.push(`state.camOffsetY not number: ${st.camOffsetY}`);

    // Camera setters must drive the state the gesture tests checkpoint.
    if (typeof setCamScale === "function") {
      setCamScale(2);
      const cs = fullLayoutSnapshot().state.camScale;
      if (cs !== 2) out.errs.push(`setCamScale(2) → state.camScale=${cs}`);
      setCamScale(1);
    }
    if (typeof setCamOffset === "function") {
      setCamOffset(123, -45);
      const s2 = fullLayoutSnapshot().state;
      if (s2.camOffsetX !== 123 || s2.camOffsetY !== -45) out.errs.push(`setCamOffset(123,-45) → (${s2.camOffsetX},${s2.camOffsetY})`);
    }

    // export/import roundtrip via the engine path (export_import_roundtrip test).
    if (typeof exportJSON === "function" && typeof importJSON === "function") {
      const before = fullLayoutSnapshot().state.totalRows;
      const saved = exportJSON();
      try {
        const doc = JSON.parse(saved);
        if (!Array.isArray(doc.instruments) || !Array.isArray(doc.nodes)) out.errs.push("exportJSON missing instruments/nodes arrays");
      } catch (e) {
        out.errs.push("exportJSON did not return valid JSON");
      }
      importJSON(saved);
      const after = fullLayoutSnapshot().state.totalRows;
      if (after !== before) out.errs.push(`export→import roundtrip changed totalRows ${before}→${after}`);
    }

    // setSubdivisions plumbing: a no-op set to the CURRENT value must succeed
    // (returns true, value unchanged). We can't assert a *change* here because
    // the catalogue leaves an unpredictable subdiv and the engine legitimately
    // rejects lowering that would orphan misaligned demo nodes. The real 8→16
    // raise is asserted by the agent sanity test's subdiv_16 checkpoint.
    if (typeof setSubdivisions === "function") {
      const cur = fullLayoutSnapshot().state.subdiv;
      const r = setSubdivisions(cur);
      const sd = fullLayoutSnapshot().state.subdiv;
      if (r !== true || sd !== cur) out.errs.push(`setSubdivisions(${cur}) no-op: ret=${r} subdiv=${sd}`);
    }
    return out;
  });
  if (snapCheck.errs.length > 0) {
    for (const e of snapCheck.errs) failures.push({ group: "fullLayoutSnapshot-fields", name: e, error: e });
    console.error(`[bridge-smoke] fullLayoutSnapshot field check FAILED:`, snapCheck.errs);
  } else {
    console.log("[bridge-smoke] fullLayoutSnapshot enriched fields OK (state/tabs/bottomNav + setEQTab/setViewMode plumbing)");
  }
}

if (isCoverageEnabled()) {
  await flushCoverage(
    page,
    new URL("../../coverage/browser-raw", import.meta.url).pathname,
    "wasm_bridge_smoke",
  );
}
await browser.close();
server.close();

if (failures.length > 0 || missingFromCatalogue.length > 0) {
  throw new Error(
    `bridge-smoke failed: ${failures.length} call failure(s), ${missingFromCatalogue.length} missing export(s)`,
  );
}
if (totalCalled < 80) {
  throw new Error(`bridge-smoke: only called ${totalCalled} exports, expected ≥80`);
}
console.log("WASM bridge smoke test PASSED");
