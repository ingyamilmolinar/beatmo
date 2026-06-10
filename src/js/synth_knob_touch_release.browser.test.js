/**
 * Synth-tab Knob Touch Release — Bridge Plumbing Assertions
 *
 * COVERED-BY-GO: src/go/internal/ui/synth_knob_touch_lifecycle_test.go
 *   (drives globalTouchState → updateTouchOverride → DrumViewTree dispatcher
 *    → synthKnobHitAdapter.OnRelease with the real coord plumbing, asserts
 *    knob.Value and the audio param do NOT snap to max on touch lift)
 * COVERED-BY-GO: src/go/internal/ui/touch_release_position_test.go
 *   (asserts cursorPosition() holds the lift coords on the release frame,
 *    not stale (0,0))
 * COVERED-BY-GO: src/go/internal/ui/knob_release_no_remutate_test.go
 *   (asserts Knob.HandleInputResult on release does not recompute the
 *    value from incoming (mx,my) — release is commit-only)
 *
 * Per CLAUDE.md JS Test Charter, this file restricts itself to bridge
 * plumbing assertions: that the JS exports the regression depends on exist
 * and are callable, and that programmatic param writes propagate. The
 * full real-touch end-to-end (the touch-override → dispatcher → knob
 * release lifecycle, and the "value must not snap to max on lift" guard)
 * is covered by the Go tests above, which exercise the exact same
 * dispatcher + touch-override code paths.
 *
 * What this JS file actually asserts (and ONLY this):
 *   - the synth-tab exports (setEQTab, synthKnobRects, getInstrumentParams,
 *     setInstrumentParam) are present and callable;
 *   - synthKnobRects() returns a non-empty knob set with finite [min,max];
 *   - setInstrumentParam → getInstrumentParams is an EXACT round-trip for a
 *     value at ~30% of range (distinct from min, max, and the bipolar
 *     midpoint), across both a bipolar and a unipolar param — so a
 *     snap-to-min, snap-to-max, or snap-to-any-fixed-value bug in the audio
 *     bridge fails here. (It does NOT drive real touch/knob input; see the
 *     Go tests above for that.)
 *
 * CHIP-STRIP REDESIGN: the Synth tab now renders one chip per pipeline stage
 * (VOICE·OSC·FM·PITCH·LFO·BURST·ENVELOPE·FILTER·POST) with exactly ONE stage
 * expanded. synthKnobRects() still returns one entry per wired knob with its
 * [min,max], but knobs of collapsed stages report ZERO-SIZE rects (w=0,h=0).
 * The round-trips below write via setInstrumentParam by param NAME (not by
 * rect), so they are layout-independent; we still call selectSynthSection so
 * the laid-out knob set is exercised and the plumbing matches the real flow.
 */

import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
  // Build the root cmd package only; ./cmd/... would match sibling tool
  // packages and fail with "multiple packages to non-directory".
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const iPhone = devices["iPhone 12 landscape"];
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const context = await browser.newContext({ ...iPhone, hasTouch: true });
const page = await context.newPage();
await page.goto(`http://localhost:${port}/`);

await page.waitForFunction(
  () =>
    typeof ensureDefaultPath === "function" &&
    typeof setEQTab === "function" &&
    typeof synthKnobRects === "function" &&
    typeof getInstrumentParams === "function" &&
    typeof setInstrumentParam === "function" &&
    typeof exportJSON === "function" &&
    typeof forceDraw === "function"
);

let passed = 0;
let failed = 0;
function assert(condition, message) {
  if (!condition) { console.error(`  FAIL: ${message}`); failed++; return false; }
  console.log(`  PASS: ${message}`);
  passed++;
  return true;
}

try {
  await page.evaluate(() => { ensureDefaultPath(); forceDraw(); });
  await page.waitForTimeout(200);

  // ── Plumbing 1: every JS bridge the regression depends on is callable. ──
  const probes = await page.evaluate(() => {
    const out = {};
    try {
      out.setEQTab = typeof setEQTab === "function";
      setEQTab("synth");
      out.setEQTabOK = true;
    } catch (e) {
      out.setEQTabOK = false;
    }
    out.synthKnobRects = typeof synthKnobRects === "function";
    out.getInstrumentParams = typeof getInstrumentParams === "function";
    out.setInstrumentParam = typeof setInstrumentParam === "function";
    return out;
  });
  assert(probes.setEQTab && probes.setEQTabOK, "setEQTab('synth') callable");
  assert(probes.synthKnobRects, "synthKnobRects export present");
  assert(probes.getInstrumentParams, "getInstrumentParams export present");
  assert(probes.setInstrumentParam, "setInstrumentParam export present");

  // CHIP-STRIP: open a knobbed stage so at least one stage's knobs are laid
  // out (non-zero rects). VOICE is the first knobbed stage for drum recipes;
  // guarded for safety since older builds lack the export.
  await page.evaluate(() => { if (typeof selectSynthSection === "function") selectSynthSection("VOICE"); });
  await page.waitForTimeout(150);
  await page.evaluate(() => forceDraw());

  // ── Plumbing 2: synthKnobRects returns the wired knob set with finite
  //    ranges, so the e2e test's coord computations always have data. (Knobs
  //    of collapsed stages report zero-size rects; min/max are always present.)
  const knobs = await page.evaluate(() => synthKnobRects());
  assert(Array.isArray(knobs) && knobs.length > 0, `synthKnobRects returns ≥1 knob (got ${knobs.length})`);
  const allRanged = knobs.every(k =>
    typeof k.name === "string" && Number.isFinite(k.min) && Number.isFinite(k.max) && k.max > k.min
  );
  assert(allRanged, "every knob has a finite [min, max] range and a name");

  // ── Plumbing 3: programmatic param writes via setInstrumentParam round-
  //    trip through getInstrumentParams. This is the bridge the dispatcher
  //    uses on every OnDrag/OnRelease — if it breaks, every knob drag breaks
  //    end-to-end.
  const instID = await page.evaluate(() => {
    try {
      const j = JSON.parse(exportJSON());
      if (j.instruments && j.instruments.length > 0) return j.instruments[0].id || "";
    } catch (_) {}
    return "";
  });
  if (assert(instID, `Resolved an instrument id from exportJSON (${instID})`)) {
    // Pick distinct probe params so a single-param coincidence can't mask a
    // snap bug: one bipolar (min<0<max, e.g. pitch) and one unipolar
    // (min>=0). For each, write a value at ~30% of the range — provably
    // distinct from BOTH min and max — then assert EXACT round-trip equality.
    // A snap-to-max, snap-to-min, OR snap-to-any-other-value bug now fails.
    const bipolar = knobs.find(k => k.min < 0 && k.max > 0);
    const unipolar = knobs.find(k => k.min >= 0 && k.max > k.min);

    // The original regression was "mid-range write snaps to max". The probe
    // value at 30% of range is far from max, far from min, and far from the
    // bipolar midpoint (0), so a wrong readback of any of those values fails.
    const probeVal = (k) => k.min + (k.max - k.min) * 0.3;

    async function roundTrips(label, k) {
      if (!assert(k != null, `${label} probe knob present`)) return;
      const name = k.name;
      const written = probeVal(k);
      await page.evaluate(([id, n, v]) => setInstrumentParam(id, n, v), [instID, name, written]);
      const got = await page.evaluate(([id, n]) => {
        const p = getInstrumentParams(id);
        return p ? p[n] : undefined;
      }, [instID, name]);
      // EXACT round-trip (tight epsilon). Distinct from min, max, and 0.
      const eps = 1e-6;
      const distinctFromMin = Math.abs(written - k.min) > 1e-3;
      const distinctFromMax = Math.abs(written - k.max) > 1e-3;
      assert(
        distinctFromMin && distinctFromMax,
        `${label} probe value=${written} is distinct from min=${k.min} and max=${k.max}`
      );
      assert(
        typeof got === "number" && Math.abs(got - written) < eps,
        `${label} setInstrumentParam(${instID}, ${name}, ${written}) EXACT round-trip (readback=${got})`
      );
    }

    await roundTrips("bipolar", bipolar);
    await roundTrips("unipolar", unipolar);
  }
} finally {
  await context.close();
  await browser.close();
  server.close();
}

console.log(`\nsynth_knob_touch_release: ${passed} passed, ${failed} failed`);
if (failed > 0) process.exit(1);
