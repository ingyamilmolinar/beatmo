/** Bridge plumbing only: the Synth exports exist, synthControlRects() has finite ranges, and setInstrumentParam →
 * getInstrumentParams round-trips EXACTLY at 30 % of range. COVERED-BY-GO: synth_knob_touch_lifecycle_test.go,
 * touch_release_position_test.go, knob_release_no_remutate_test.go */

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
  // Only ./cmd: ./cmd/... would match tool packages and fail the single -o output.
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
    typeof synthControlRects === "function" &&
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
    out.synthControlRects = typeof synthControlRects === "function";
    out.getInstrumentParams = typeof getInstrumentParams === "function";
    out.setInstrumentParam = typeof setInstrumentParam === "function";
    return out;
  });
  assert(probes.setEQTab && probes.setEQTabOK, "setEQTab('synth') callable");
  assert(probes.synthControlRects, "synthControlRects export present");
  assert(probes.getInstrumentParams, "getInstrumentParams export present");
  assert(probes.setInstrumentParam, "setInstrumentParam export present");

  // So at least one stage's knobs are laid out.
  await page.evaluate(() => { if (typeof selectSynthSection === "function") selectSynthSection("VOICE"); });
  await page.waitForTimeout(150);
  await page.evaluate(() => forceDraw());

  // Collapsed stages report zero-size rects; min/max are always present.
  const knobs = await page.evaluate(() => synthControlRects());
  assert(Array.isArray(knobs) && knobs.length > 0, `synthControlRects returns ≥1 knob (got ${knobs.length})`);
  const allRanged = knobs.every(k =>
    typeof k.name === "string" && Number.isFinite(k.min) && Number.isFinite(k.max) && k.max > k.min
  );
  assert(allRanged, "every knob has a finite [min, max] range and a name");

  // The bridge every drag and release uses.
  const instID = await page.evaluate(() => {
    try {
      const j = JSON.parse(exportJSON());
      if (j.instruments && j.instruments.length > 0) return j.instruments[0].id || "";
    } catch (_) {}
    return "";
  });
  if (assert(instID, `Resolved an instrument id from exportJSON (${instID})`)) {
    // One bipolar and one unipolar knob of the instrument's OWN continuous values, so a snap to min, max or any fixed
    // value fails; an enum never holds a 30 % value.
    const { defs, declared } = await page.evaluate((id) => ({ defs: instrumentParamDefs(id), declared: getInstrumentParams(id) }), instID);
    const continuous = defs.filter(d => d.name in declared && d.group !== "hidden" && !(d.enum && d.enum.length) &&
      Number.isFinite(d.min) && Number.isFinite(d.max) && d.max > d.min);
    const bipolar = continuous.find(k => k.min < 0 && k.max > 0);
    const unipolar = continuous.find(k => k.min >= 0 && k.max > k.min);

    // Far from min, max and the bipolar midpoint.
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
