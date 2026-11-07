/** A melodic note's DURATION never changes its PITCH in the browser: each subject through the sequencer's batch path at
 * durations 1/0.75/0.5/0.25 must sound within ±30 cents and END sooner (gated, not stretched). COVERED-BY-GO:
 * cmd/gen-showcase/played_duration_test.go */

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
  waitForAudioReady,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
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

const hardTimer = setTimeout(() => {
  console.error("melodic_duration_pitch: hard timeout (240s) — aborting");
  process.exit(1);
}, 240_000);

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("pageerror", (e) => { console.log("[PAGEERROR]", String(e)); });

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof window.playSoundsBatch === "function", null, { timeout: 60_000 });
await page.waitForFunction(() => window.audioReady !== undefined, null, { timeout: 60_000 });
await waitForAudioReady(page);

// voice-soprano is the reported voice; violin isolates the duration rule from melodic-set membership.
const SUBJECTS = [
  { id: "voice-soprano", pitch: 0 },
  { id: "voice-soprano", pitch: 7 },
  { id: "violin", pitch: 0 },
];
const DURS = [1, 0.75, 0.5, 0.25];
const TOL_CENTS = 30;

// YIN in 80-1500 Hz takes the FIRST fitting period, so a vibrato-smeared later one cannot win as with raw
// autocorrelation.
function detectF0(x, sr) {
  const minLag = Math.floor(sr / 1500), maxLag = Math.ceil(sr / 80);
  const n = x.length - maxLag;
  if (n <= 0) return NaN;
  const d = new Float64Array(maxLag + 2);
  for (let lag = 1; lag <= maxLag + 1; lag++) {
    let s = 0;
    for (let i = 0; i < n; i++) { const t = x[i] - x[i + lag]; s += t * t; }
    d[lag] = s;
  }
  const cmnd = new Float64Array(maxLag + 2);
  cmnd[0] = 1;
  let run = 0;
  for (let lag = 1; lag <= maxLag + 1; lag++) { run += d[lag]; cmnd[lag] = d[lag] * lag / (run || 1e-20); }
  let lag = -1;
  for (let l = minLag; l <= maxLag; l++) {
    if (cmnd[l] < 0.15) { while (l + 1 <= maxLag && cmnd[l + 1] < cmnd[l]) l++; lag = l; break; }
  }
  if (lag < 0) {
    let best = Infinity;
    for (let l = minLag; l <= maxLag; l++) if (cmnd[l] < best) { best = cmnd[l]; lag = l; }
  }
  const a = cmnd[lag - 1], b = cmnd[lag], c = cmnd[lag + 1];
  const den = a - 2 * b + c;
  return sr / (lag + (den !== 0 ? 0.5 * (a - c) / den : 0));
}

async function playAndCapture(id, pitch, dur) {
  const res = await page.evaluate(async ({ id, pitch, dur }) => {
    window.startOutputCapture();
    await new Promise((r) => setTimeout(r, 60));
    window.playSoundsBatch({ id, vol: 0.8, pitch, dur });
    await new Promise((r) => setTimeout(r, 1300));
    if (window.flushOutputCapture) await window.flushOutputCapture();
    const data = window.stopOutputCapture ? window.stopOutputCapture() : window.getOutputCapture();
    return { data: Array.from(data), sr: window.getOutputCaptureStats().sampleRate };
  }, { id, pitch, dur });
  const x = Float32Array.from(res.data);
  const sr = res.sr || 48000;
  let peak = 0;
  for (const v of x) peak = Math.max(peak, Math.abs(v));
  if (peak < 1e-3) return { silent: true };
  const thr = 0.05 * peak;
  let onset = 0;
  while (onset < x.length && Math.abs(x[onset]) < thr) onset++;
  let end = x.length - 1;
  while (end > onset && Math.abs(x[end]) < thr) end--;
  const a = onset + Math.round(0.04 * sr);
  const f0 = detectF0(x.subarray(a, a + Math.round(0.08 * sr)), sr);
  return { f0, soundingSec: (end - onset) / sr, peak };
}

await page.evaluate(() => window.enableOutputCapture());
// A cold pitch DEFERS its note, which would put the onset outside the capture.
for (const s of SUBJECTS) {
  await page.evaluate(async ({ id, pitch }) => {
    window.playSoundsBatch({ id, vol: 0.001, pitch, dur: 1 });
  }, s);
}
await page.waitForTimeout(2500);

const failures = [];
for (const s of SUBJECTS) {
  const results = [];
  for (const dur of DURS) results.push({ dur, ...(await playAndCapture(s.id, s.pitch, dur)) });
  const ref = results[0];
  for (const r of results) {
    const cents = Number.isFinite(r.f0) && Number.isFinite(ref.f0) ? 1200 * Math.log2(r.f0 / ref.f0) : NaN;
    console.log(`${s.id} pitch ${s.pitch} dur ${r.dur}: ${r.silent ? "SILENT" : `f0 ${r.f0.toFixed(1)} Hz (${cents >= 0 ? "+" : ""}${cents.toFixed(0)} cents vs dur 1), sounds ${r.soundingSec.toFixed(3)} s`}`);
    if (r.silent) { failures.push(`${s.id} p${s.pitch} dur ${r.dur}: silent`); continue; }
    if (!Number.isFinite(cents)) { failures.push(`${s.id} p${s.pitch} dur ${r.dur}: no f0`); continue; }
    if (Math.abs(cents) > TOL_CENTS) failures.push(`${s.id} p${s.pitch} dur ${r.dur}: ${cents.toFixed(0)} cents off the dur-1 note`);
  }
  // A note sounds min(natural decay, d × buffer), so only short durations are visibly cut.
  const q = results.find((r) => r.dur === 0.25), h = results.find((r) => r.dur === 0.5);
  if (q && h && !q.silent && !h.silent) {
    if (!(q.soundingSec < 0.6 * ref.soundingSec)) {
      failures.push(`${s.id} p${s.pitch}: dur 0.25 sounded ${q.soundingSec.toFixed(3)} s vs ${ref.soundingSec.toFixed(3)} s at dur 1 — not gated`);
    }
    const want = Math.min(ref.soundingSec, 2 * q.soundingSec);
    if (Math.abs(h.soundingSec - want) > 0.08) {
      failures.push(`${s.id} p${s.pitch}: dur 0.5 sounded ${h.soundingSec.toFixed(3)} s, want ~${want.toFixed(3)} s`);
    }
  }
}

if (isCoverageEnabled()) await flushCoverage(page, "melodic_duration_pitch");
clearTimeout(hardTimer);
await browser.close();
server.close();

if (failures.length) {
  console.error("FAIL:\n  " + failures.join("\n  "));
  process.exit(1);
}
console.log("TEST PASSED - melodic note duration never changes its pitch in the browser");
