// COVERED-BY-GO: none, since Go cannot see when WebAudio actually starts a scheduled source. The timingTraces ring must
// record real sequencer playback for Go's emission-drift checker.
import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import { startPlaybackWhenReady } from "./audio_probe_bench_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// The full build: the playtest harness has no sequencer to thread Row/Abs through.
if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const pageErrors = [];
page.on("pageerror", (e) => pageErrors.push(String(e.message)));
page.on("console", (m) => {
  if (m.type() === "error") pageErrors.push("console.error: " + m.text());
});

// On exports that register only once the whole bridge is installed: startPlay alone races ahead of init.
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(
  () =>
    typeof startPlay === "function" &&
    typeof stopPlay === "function" &&
    typeof getTimingTraces === "function" &&
    typeof resetTimingTraces === "function" &&
    typeof buildPerfRect === "function",
);

await page.evaluate(() => {
  document.dispatchEvent(new Event("pointerdown"));
  resumeAudio?.();
  resetTimingTraces();
  buildPerfRect(2, 1);
  forceDraw?.(); forceDraw?.();
  // Batches start voices SYNCHRONOUSLY inside the Go→JS call and Go drains the ring right after, before control returns,
  // so a poll between frames sees it empty. Go resolves resetTimingTraces by name on every drain, so wrapping it captures
  // exactly what the checker receives.
  window.__ttCaptured = [];
  const origReset = window.resetTimingTraces;
  window.resetTimingTraces = () => {
    const snap = window.getTimingTraces();
    if (snap.length) window.__ttCaptured.push(...snap);
    return origReset();
  };
});
// A bare startPlay() is ignored while the new rows' voices warm up.
await startPlaybackWhenReady(page, { bpm: 240 });

// Go drains and resets the ring on every batch, so it never accumulates: poll during playback and keep the largest
// snapshot. A non-empty one proves recording works and that polling, as the drain does, never throws.
let best = [];
let pollThrew = false;
for (let i = 0; i < 30; i++) {
  await page.waitForTimeout(100);
  let snap;
  try {
    snap = await page.evaluate(() => window.getTimingTraces());
  } catch (e) {
    pollThrew = true;
    break;
  }
  if (Array.isArray(snap) && snap.length > best.length) best = snap;
}
const drained = await page.evaluate(() => window.__ttCaptured.slice());
if (drained.length > best.length) best = drained;

await page.evaluate(() => { stopPlay?.(); });

if (pollThrew) {
  throw new Error("getTimingTraces() threw while polling during playback");
}
if (best.length === 0) {
  throw new Error("no timing traces observed during playback (Go's Stage-D drain may be over-eager, or Task 4.1 recording broke)");
}
for (const tr of best) {
  if (typeof tr.startWhen !== "number" || !Number.isFinite(tr.startWhen)) {
    throw new Error("trace missing numeric startWhen: " + JSON.stringify(tr));
  }
  if (typeof tr.ctxAtStart !== "number" || !Number.isFinite(tr.ctxAtStart)) {
    throw new Error("trace missing numeric ctxAtStart: " + JSON.stringify(tr));
  }
  if (typeof tr.row !== "number" || tr.row < 0) {
    throw new Error("trace has non-sequencer row (should never be recorded): " + JSON.stringify(tr));
  }
}
console.log("PASS timing_traces:", best.length, "traces (best snapshot while Go Stage-D drain is concurrently active)");

// The browser checker is log-only, so a drain + reset must never wedge recording. Reset and read share ONE evaluate:
// stopPlay() takes effect a frame later and queued events keep pushing traces, so two round trips race.
const { before, after } = await page.evaluate(() => {
  const before = window.getTimingTraces().length;
  window.resetTimingTraces();
  return { before, after: window.getTimingTraces().length };
});
if (after !== 0) {
  throw new Error("resetTimingTraces() did not clear the ring: len=" + after + " (was " + before + " before reset)");
}

const capturedBeforeReplay = await page.evaluate(() => window.__ttCaptured.length);
await startPlaybackWhenReady(page, { bpm: 240 });

let bestAfterReplay = 0;
for (let i = 0; i < 20; i++) {
  await page.waitForTimeout(100);
  const len = await page.evaluate((b) => window.getTimingTraces().length + window.__ttCaptured.length - b, capturedBeforeReplay);
  if (len > bestAfterReplay) bestAfterReplay = len;
}

await page.evaluate(() => { stopPlay?.(); });

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "timing_traces");
await browser.close();
server.close();

if (bestAfterReplay === 0) {
  throw new Error("no traces observed after resetTimingTraces() + replay — drain/reset cycle wedged recording");
}
const fatalErrors = pageErrors.filter((e) => /panic|already exited/i.test(e));
if (fatalErrors.length > 0) {
  throw new Error("WASM panic/crash detected during Stage-D drain: " + fatalErrors.join(" | "));
}

console.log("PASS timing_traces armed drain: reset cleared", before, "traces; reset+replay observed", bestAfterReplay, "traces; drain ran without throwing");
