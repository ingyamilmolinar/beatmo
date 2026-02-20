import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const repoRoot = path.resolve(jsDir, "..", "..");
const GO = resolveGoBinary();

// Build WASM
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

// HTTP server
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const client = await page.context().newCDPSession(page);

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function");

// Build circuit
await page.evaluate(() => {
  resetAudioScheduleMetrics?.();
  resetPerfStats?.();
  buildPerfRect(4, 1);
  forceDraw?.(); forceDraw?.();
  setBPM(280);
});

console.log("Starting CDP CPU + Heap profiling (8s at BPM=280, 4 rows)...\n");

// Start CPU profiler
await client.send("Profiler.enable");
await client.send("Profiler.setSamplingInterval", { interval: 100 });
await client.send("Profiler.start");

// Start heap sampling
await client.send("HeapProfiler.enable");
await client.send("HeapProfiler.startSampling", { samplingInterval: 1024 });

// Unlock AudioContext — page.evaluate() doesn't trigger user gesture events.
await page.evaluate(() => {
  document.dispatchEvent(new Event('pointerdown'));
  resumeAudio?.();
});

// Run playback
await page.evaluate(() => startPlay());
await page.waitForTimeout(8000);
await page.evaluate(() => stopPlay?.());

// Collect audio metrics alongside profiling
const audioMetrics = await page.evaluate(() => getAudioScheduleMetrics?.());
const perfStatsResult = await page.evaluate(() => perfStats?.());

// Stop CPU profiler
const { profile: cpuProfile } = await client.send("Profiler.stop");
await client.send("Profiler.disable");

// Stop heap sampler
const { profile: heapProfile } = await client.send("HeapProfiler.stopSampling");
await client.send("HeapProfiler.disable");

// Save raw profiles
const traceDir = path.join(repoRoot, "trace");
if (!fs.existsSync(traceDir)) fs.mkdirSync(traceDir);
fs.writeFileSync(path.join(traceDir, "cpu_profile.json"), JSON.stringify(cpuProfile, null, 2));
fs.writeFileSync(path.join(traceDir, "heap_profile.json"), JSON.stringify(heapProfile, null, 2));

// ---------------------------------------------------------------------------
// Parse CPU profile: compute self-time per function
// ---------------------------------------------------------------------------
console.log("=== CPU Profile Analysis ===\n");

const nodeMap = new Map();
if (cpuProfile && cpuProfile.nodes) {
  for (const node of cpuProfile.nodes) {
    nodeMap.set(node.id, node);
  }
}

// Compute self-time from time deltas and samples
const selfTimes = new Map();
if (cpuProfile && cpuProfile.samples && cpuProfile.timeDeltas) {
  for (let i = 0; i < cpuProfile.samples.length; i++) {
    const nodeId = cpuProfile.samples[i];
    const delta = cpuProfile.timeDeltas[i] || 0;
    selfTimes.set(nodeId, (selfTimes.get(nodeId) || 0) + delta);
  }
}

// Aggregate by function name
const funcTimes = new Map();
for (const [nodeId, time] of selfTimes) {
  const node = nodeMap.get(nodeId);
  if (!node || !node.callFrame) continue;
  const name = node.callFrame.functionName || "(anonymous)";
  funcTimes.set(name, (funcTimes.get(name) || 0) + time);
}

const totalCpuTime = Array.from(selfTimes.values()).reduce((a, b) => a + b, 0);
const sortedFuncs = Array.from(funcTimes.entries())
  .map(([name, time]) => ({ name, timeUs: time, pct: totalCpuTime > 0 ? (time / totalCpuTime * 100) : 0 }))
  .sort((a, b) => b.timeUs - a.timeUs);

console.log("Top 20 functions by self-time:");
console.log("  %CPU  | Time (ms) | Function");
console.log("  ------|-----------|--------");
for (const f of sortedFuncs.slice(0, 20)) {
  console.log(`  ${f.pct.toFixed(1).padStart(5)}% | ${(f.timeUs / 1000).toFixed(1).padStart(9)} | ${f.name}`);
}

// Filter for audio-related functions
const audioKeywords = ["audio", "play", "flush", "schedule", "wasm", "Go", "render", "processAudio", "enqueue", "observe", "Sample", "voice", "mix", "buffer"];
const audioFuncs = sortedFuncs.filter(f =>
  audioKeywords.some(kw => f.name.toLowerCase().includes(kw.toLowerCase()))
);

if (audioFuncs.length > 0) {
  console.log("\nAudio-related functions:");
  console.log("  %CPU  | Time (ms) | Function");
  console.log("  ------|-----------|--------");
  for (const f of audioFuncs.slice(0, 15)) {
    console.log(`  ${f.pct.toFixed(1).padStart(5)}% | ${(f.timeUs / 1000).toFixed(1).padStart(9)} | ${f.name}`);
  }
}

// Compute audio flush % and WASM overhead %
let audioFlushPct = 0;
let wasmOverheadPct = 0;
for (const f of sortedFuncs) {
  const lc = f.name.toLowerCase();
  if (lc.includes("flush") && lc.includes("audio")) audioFlushPct += f.pct;
  if (lc.includes("wasm") || lc.includes("go.") || lc === "go") wasmOverheadPct += f.pct;
}

// ---------------------------------------------------------------------------
// Parse heap profile
// ---------------------------------------------------------------------------
console.log("\n=== Heap Profile Analysis ===\n");

const allocByFunc = new Map();
if (heapProfile && heapProfile.samples) {
  for (const sample of heapProfile.samples) {
    const size = sample.size || 0;
    const count = sample.count || 0;
    const stack = sample.stack || [];
    // Use top frame as the function
    const topFrame = stack.length > 0 ? stack[0] : null;
    const name = topFrame ? (topFrame.functionName || "(anonymous)") : "(unknown)";
    const prev = allocByFunc.get(name) || { size: 0, count: 0 };
    allocByFunc.set(name, { size: prev.size + size, count: prev.count + count });
  }
}

const sortedAllocs = Array.from(allocByFunc.entries())
  .map(([name, { size, count }]) => ({ name, sizeKB: size / 1024, count }))
  .sort((a, b) => b.sizeKB - a.sizeKB);

if (sortedAllocs.length > 0) {
  console.log("Top 15 allocators by size:");
  console.log("  Size (KB) | Count  | Function");
  console.log("  ----------|--------|--------");
  for (const a of sortedAllocs.slice(0, 15)) {
    console.log(`  ${a.sizeKB.toFixed(1).padStart(9)} | ${String(a.count).padStart(6)} | ${a.name}`);
  }
}

// ---------------------------------------------------------------------------
// Audio metrics + perf stats
// ---------------------------------------------------------------------------
console.log("\n=== Audio Schedule Metrics ===\n");
if (audioMetrics) {
  const fmtMs = (s) => s == null ? "null" : (s * 1000).toFixed(2) + "ms";
  console.log(`  count=${audioMetrics.count} overdue=${audioMetrics.overdue}`);
  console.log(`  minLead=${fmtMs(audioMetrics.minLead)} avgLag=${fmtMs(audioMetrics.avgLag)}`);
  console.log(`  lagP90=${fmtMs(audioMetrics.lagP90)} lagP99=${fmtMs(audioMetrics.lagP99)}`);
  console.log(`  maxLag=${fmtMs(audioMetrics.maxLag)} smallLeadCount=${audioMetrics.smallLeadCount}`);
}

console.log("\n=== Perf Stats ===\n");
if (perfStatsResult) {
  console.log(`  FPS avg: ${perfStatsResult.fpsAvg?.toFixed(1)}`);
  console.log(`  Update avg: ${perfStatsResult.updateAvgMS?.toFixed(3)}ms`);
  console.log(`  Draw avg: ${perfStatsResult.drawAvgMS?.toFixed(3)}ms`);
  console.log(`  Audio call avg: ${perfStatsResult.audioCallAvg?.toFixed(3)}ms`);
}

// ---------------------------------------------------------------------------
// Soft assertions (warnings, not failures)
// ---------------------------------------------------------------------------
console.log("\n=== Soft Assertions ===\n");
const warnings = [];

if (audioFlushPct > 5) {
  warnings.push(`Audio flush CPU usage ${audioFlushPct.toFixed(1)}% exceeds 5% target`);
}
if (wasmOverheadPct > 15) {
  warnings.push(`WASM/Go overhead ${wasmOverheadPct.toFixed(1)}% exceeds 15% target`);
}
if (audioMetrics && audioMetrics.lagP99 != null && audioMetrics.lagP99 > 0.020) {
  warnings.push(`lagP99 ${(audioMetrics.lagP99 * 1000).toFixed(2)}ms exceeds 20ms during profiling`);
}

if (warnings.length > 0) {
  for (const w of warnings) console.log(`  WARN: ${w}`);
} else {
  console.log("  All soft assertions passed");
}

console.log(`\nProfiles saved to: ${traceDir}/cpu_profile.json, ${traceDir}/heap_profile.json`);

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_profile_cdp");
await browser.close();
server.close();

console.log("\naudio_profile_cdp: complete");
