// An edit during playback costs what the edit is, never a graph rebuild: real pointer edits on the demo (add an effect,
// hold and drag its slider, hold and drag an EQ band). HARD: graph churn (no rebuilds, no new worklet nodes, a still slider
// writes nothing) and the heard notes; main-thread cost reported. Run: GO=… node src/js/webaudio_live_edit_budget.browser.test.js

import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { launchBrowser } from "./platform/launch.js";
import {
  startPlaybackWhenReady, startPlaybackProbe, stopPlaybackProbe, probeSummaryFields, PROBE_STRICT,
} from "./audio_probe_bench_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const WASM_FILE = process.env.WASM_FILE ? path.resolve(process.env.WASM_FILE) : path.join(jsDir, "main.wasm");
const SECS = Number(process.env.LIVE_EDIT_SECS ?? 4);
const SHOTS = process.env.LIVE_EDIT_SHOTS ? path.resolve(process.env.LIVE_EDIT_SHOTS) : null;
if (SHOTS) fs.mkdirSync(SHOTS, { recursive: true });
const THROTTLE = Number(process.env.LIVE_EDIT_CPU_THROTTLE ?? 1);

if (!process.env.WASM_FILE && !shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(GO, ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", WASM_FILE, "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const url = req.url.split("?")[0];
  const file = url === "/" ? "/index.html" : url;
  const filePath = file === "/main.wasm" ? WASM_FILE : path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    else if (filePath.endsWith(".json")) ct = "application/json";
    res.writeHead(200, { "Content-Type": ct, "Cache-Control": "no-store" });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const failures = [];
const hard = (cond, msg) => { if (!cond) failures.push(msg); };
const soft = (cond, msg) => {
  if (cond) return;
  if (PROBE_STRICT) failures.push(msg);
  else console.log(`  [main-thread info] ${msg}`);
};

const { browser, platform, cleanup } = await launchBrowser();
const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
const consoleErrors = [];
page.on("pageerror", (e) => consoleErrors.push(e.message));
await page.addInitScript(() => {
  window.__liveEditLongTasks = [];
  try {
    new PerformanceObserver((l) => { for (const e of l.getEntries()) window.__liveEditLongTasks.push({ t: e.startTime, d: e.duration }); })
      .observe({ type: "longtask", buffered: true });
  } catch (_) { /* engines without longtask: the main-thread report reads 0 */ }
});
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function" && typeof fxPanelControlRects === "function" &&
  typeof audioGraphStats === "function", null, { timeout: 120000 });
if (THROTTLE > 1) {
  const cdp = await page.context().newCDPSession(page);
  await cdp.send("Emulation.setCPUThrottlingRate", { rate: THROTTLE });
}
await page.waitForTimeout(2000);
console.log(`platform ${platform}${process.env.TEST_HEADED === "1" ? " (headed)" : ""}, cpu throttle ${THROTTLE}x, ${SECS}s per phase`);

const row = 0;
const inst = await page.evaluate((r) => rowInstrument(r), row);

// ── real pointer helpers ──
const mid = (r) => [r.x + r.w / 2, r.y + r.h / 2];
async function tap(x, y) {
  await page.mouse.move(x, y);
  await page.waitForTimeout(60); // hover before press, like a real pointer
  await page.mouse.down();
  await page.waitForTimeout(60);
  await page.mouse.up();
  await page.waitForTimeout(200);
}
const graph = () => page.evaluate(() => audioGraphStats());
const diff = (a, b) => Object.fromEntries(Object.keys(b).map((k) => [k, b[k] - (a[k] ?? 0)]));

// `act` runs while the probe records and must take about SECS.
const rows = [];
async function phase(name, act) {
  await page.evaluate(() => { window.__liveEditT0 = performance.now(); });
  const g0 = await graph();
  const started = await startPlaybackProbe(page);
  if (!started?.ok) { failures.push(`[${name}] probe did not start: ${started?.error}`); return null; }
  const t0 = Date.now();
  await act();
  const rest = SECS * 1000 - (Date.now() - t0);
  if (rest > 0) await page.waitForTimeout(rest);
  const wallMs = Date.now() - t0;
  if (SHOTS) await page.screenshot({ path: path.join(SHOTS, `${name}.png`) });
  const { report } = await stopPlaybackProbe(page, name);
  const g1 = await graph();
  // Long tasks that STARTED inside the phase, per second of the phase.
  const longMsPerSec = await page.evaluate(() => {
    const t0 = window.__liveEditT0, t1 = performance.now();
    const ms = window.__liveEditLongTasks.filter((e) => e.t >= t0).reduce((a, e) => a + Math.min(e.d, t1 - e.t), 0);
    return ms / ((t1 - t0) / 1000);
  });
  const churn = diff(g0, g1);
  const f = probeSummaryFields(report);
  const silentCh = Object.entries(report?.perChannel || {}).filter(([, c]) => c.silent > 0).map(([id, c]) => `${id}:${c.silent}`);
  const r = { name, churn, f, longMsPerSec, silentCh, wallMs };
  rows.push(r);
  console.log(`  churn: ${JSON.stringify(churn)}`);
  hard(f.probeNotesJudged > 0, `[${name}] the probe judged no notes`);
  hard(f.probeNotesDropped === 0, `[${name}] ${f.probeNotesDropped} notes requested but never started`);
  hard(silentCh.length === 0, `[${name}] notes SILENT in the output (${silentCh.join(", ")}) — an edit cut the audio`);
  return r;
}

await startPlaybackWhenReady(page, {});
await page.waitForTimeout(1500);

// ── idle ──
const idle = await phase("idle", async () => {});
hard(idle && idle.churn.insertChainUpdates === 0 && idle.churn.channelRewires === 0,
  `[idle] plain playback churned the graph: ${JSON.stringify(idle?.churn)}`);

// ── fx-add: open the row's FX panel, + Add Effect, search, pick ──
const fxAdd = await phase("fx-add", async () => {
  const fxBtn = await page.evaluate((r) => rowFXBtnRect(r), row);
  await tap(...mid(fxBtn));
  const c0 = await page.evaluate(() => fxPanelControlRects());
  if (!c0?.add) throw new Error(`FX panel did not open with an "+ Add Effect" button: ${JSON.stringify(c0)}`);
  await tap(...mid(c0.add));
  await page.keyboard.type("pitch", { delay: 40 });
  await page.waitForTimeout(200);
  const c1 = await page.evaluate(() => fxPanelControlRects());
  const pick = (c1?.picks || []).find((p) => p.effect === "pitchshift");
  if (!pick) throw new Error(`no Pitch Shifter row in the add picker: ${JSON.stringify(c1)}`);
  await tap(...mid(pick));
});
const hosts = await page.evaluate((id) => insertEffectHosts(id), inst);
console.log(`  ${inst} insert chain after the add: ${JSON.stringify(hosts)}`);
hard(hosts.length > 0 && hosts.every((h) => h === "worklet"), `[fx-add] the chain's effects are not on the beatmo-dsp worklet: ${JSON.stringify(hosts)}`);
hard(fxAdd && fxAdd.churn.insertChainUpdates === 1,
  `[fx-add] adding ONE effect pushed the chain ${fxAdd?.churn.insertChainUpdates} times; want exactly 1`);
// The chain is ONE node: at most one built (a channel's first effect), at most one rewire.
hard(fxAdd && fxAdd.churn.dspNodesCreated <= 1 && fxAdd.churn.channelRewires <= 1,
  `[fx-add] built ${fxAdd?.churn.dspNodesCreated} DSP nodes / ${fxAdd?.churn.channelRewires} rewires for one effect`);

// The effect just added is the last slot; its Pitch slider.
const sliders = (await page.evaluate(() => fxPanelControlRects()))?.sliders || [];
const lastSlot = Math.max(-1, ...sliders.map((s) => s.slot));
const pitch = sliders.find((s) => s.slot === lastSlot && s.param === "pitch");
if (!pitch) throw new Error(`the added Pitch Shifter shows no Pitch slider: ${JSON.stringify(sliders)}`);
const [px, py] = mid(pitch);

// ── fx-slider-hold: press, then hold perfectly still ──
await page.mouse.move(px, py);
await page.waitForTimeout(60);
await page.mouse.down();
await page.waitForTimeout(300); // the press itself sets the value once
const hold = await phase("fx-slider-hold", async () => {});
hard(hold && hold.churn.insertParamUpdates === 0 && hold.churn.insertChainUpdates === 0,
  `[fx-slider-hold] a slider held still wrote the audio graph: ${JSON.stringify(hold?.churn)} — want no writes`);

// ── fx-slider-drag: back and forth (still pressed) ──
const drag = await phase("fx-slider-drag", async () => {
  const end = Date.now() + SECS * 1000 - 200;
  for (let i = 0; Date.now() < end; i++) {
    await page.mouse.move(px + Math.round(Math.min(pitch.w / 2 - 4, 70) * Math.sin(i / 6)), py);
    await page.waitForTimeout(16);
  }
});
await page.mouse.up();
await page.waitForTimeout(200);
hard(drag && drag.churn.insertChainUpdates === 0 && drag.churn.dspNodesCreated === 0 &&
  drag.churn.channelRewires === 0,
`[fx-slider-drag] a param drag rebuilt the audio graph: ${JSON.stringify(drag?.churn)} — a param edit must retune the live effect in place`);
hard(drag && drag.churn.insertParamUpdates > 0, `[fx-slider-drag] the drag made no in-place param update`);

// ── eq-band-drag: close the FX panel, drag one EQ band ──
await page.keyboard.press("Escape");
await page.waitForTimeout(200);
await page.evaluate(() => setEQTab("eq"));
await page.waitForTimeout(300);
const handles = await page.evaluate(() => eqBandHandlePoints());
if (!handles.length) throw new Error("the EQ tab shows no band handles");
const band = handles[Math.floor(handles.length / 2)];
await page.mouse.move(band.x, band.y);
await page.waitForTimeout(60);
await page.mouse.down();
await page.waitForTimeout(100); // the press must land in a frame of its own to grab the handle
await page.mouse.move(band.x, band.y - 30); // off 0 dB: the first detent may build the filters
await page.waitForTimeout(300);
const eqHold = await phase("eq-band-hold", async () => {});
hard(eqHold && eqHold.churn.eqUpdates === 0 && eqHold.churn.dspNodesCreated === 0,
  `[eq-band-hold] an EQ handle held still wrote the audio graph: ${JSON.stringify(eqHold?.churn)} — want no writes`);
const eq = await phase("eq-band-drag", async () => {
  const end = Date.now() + SECS * 1000 - 200;
  for (let i = 0; Date.now() < end; i++) {
    await page.mouse.move(band.x, band.y - 30 - Math.round(20 * Math.sin(i / 6)));
    await page.waitForTimeout(16);
  }
});
await page.mouse.up();
hard(eq && eq.churn.dspNodesCreated === 0 && eq.churn.channelRewires === 0,
  `[eq-band-drag] an EQ band drag rebuilt the channel: ${JSON.stringify(eq?.churn)} — a band edit must retune the live EQ`);
hard(eq && eq.churn.eqUpdates > 0, `[eq-band-drag] the drag made no in-place EQ update`);

// ── main-thread cost vs plain playback (timing: reported; strict enforces) ──
const base = Math.max(idle?.longMsPerSec ?? 0, 20); // ≥ 2% of a second: no ratio against ~0
for (const r of rows) {
  if (r.name === "idle") continue;
  soft(r.longMsPerSec <= 4 * base,
    `[${r.name}] main-thread long tasks ${r.longMsPerSec.toFixed(0)} ms/s vs ${(idle?.longMsPerSec ?? 0).toFixed(0)} ms/s idle (> 4×)`);
}
hard(consoleErrors.length === 0, `page errors: ${consoleErrors.slice(0, 3).join(" | ")}`);

console.log("\n phase           | chainUpd | dspNodes | rewires | paramUpd | eqUpd | heard/judged | late-heard | silent | longtask ms/s");
for (const r of rows) {
  const c = r.churn;
  console.log(` ${r.name.padEnd(15)} | ${String(c.insertChainUpdates).padStart(8)} | ${String(c.dspNodesCreated).padStart(8)} | ${String(c.channelRewires).padStart(7)} | ${String(c.insertParamUpdates).padStart(8)} | ${String(c.eqUpdates).padStart(5)} | ${String(r.f.probeNotesHeard).padStart(5)}/${String(r.f.probeNotesJudged).padEnd(6)} | ${String(r.f.probeNotesHeardLate ?? 0).padStart(10)} | ${String(r.f.probeNotesMissed).padStart(6)} | ${r.longMsPerSec.toFixed(0).padStart(6)}`);
}

await cleanup();
server.close();
if (failures.length) {
  console.log(`\nwebaudio_live_edit_budget FAILED (${failures.length}):`);
  for (const m of failures) console.log(`  - ${m}`);
  process.exit(1);
}
console.log("\nPASS webaudio_live_edit_budget");
