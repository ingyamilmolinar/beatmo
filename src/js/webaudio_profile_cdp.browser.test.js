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

// ---------------------------------------------------------------------------
// Scenario definitions
// ---------------------------------------------------------------------------
const DURATION_SEC = 15;

// ---------------------------------------------------------------------------
// CI gate configuration
//
// By DEFAULT this profiler is a hard gate: a regression in audio-scheduler
// health or CPU/WASM overhead exits non-zero and fails CI. Set PROFILE_SOFT=1
// to downgrade every hard failure to an informational WARN (recovers the old
// flight-recorder behavior for ad-hoc profiling runs).
//
// IMPORTANT: lagP90/lagP99 are ONLY populated when an event is OVERDUE
// (lead < 0). On a healthy run they are null/0, so they are NOT gated here —
// they are printed for diagnostics only. We gate on the metrics that ARE
// populated on a healthy run: overdue, minLead, smallLeadCount, and the
// CPU/WASM-overhead figures from the CDP profile.
// ---------------------------------------------------------------------------
const PROFILE_SOFT = process.env.PROFILE_SOFT === "1";

const GATES = {
  // Audio scheduler health (populated on every run). These are the load-bearing
  // real-time-safety gates: a regression here means audible glitches.
  overdueMax: 0,            // no event may be scheduled in the past
  minLeadMinMs: 3,          // every event must be enqueued >=3ms ahead of its play time
  smallLeadMax: 0,          // no event may have <3ms lead (Go-side smallLeadCount)
  // CPU / WASM overhead caps (from the CDP CPU profile). These are regression
  // gates, not the aspirational 5%/15% design targets: the current healthy WASM
  // build self-samples audio-flush ~0-2% and WASM/Go runtime ~16-19% (with
  // CDP-sampling-interval jitter of a couple points). Caps carry headroom above
  // the observed healthy baseline so they catch a genuine regression (e.g. a new
  // per-event allocation or a doubling of runtime overhead) without flapping on
  // the green build. Tighten these as the build improves toward the design
  // targets noted in the informational tables above.
  audioFlushPctMax: 8,      // healthy ~0-2%; trips on a real audio-flush hot path
  wasmOverheadPctMax: 25,   // healthy ~16-19%; trips on a real runtime-overhead regression
};

// Accumulates hard-failure descriptions across all scenarios.
const hardFailures = [];

const scenarios = [
  {
    name: "Synthetic 4x1 @ BPM=280",
    setup: `buildPerfRect(4, 1); setBPM(280);`,
    bpm: 280,
    rows: 4,
  },
  {
    name: "Startup Demo @ BPM=200",
    // Startup demo auto-loads; just set BPM
    setup: `setBPM(200);`,
    bpm: 200,
    rows: null, // will query
  },
  {
    name: "Extreme 8x1 @ BPM=320",
    setup: `buildPerfRect(8, 1); setBPM(320);`,
    bpm: 320,
    rows: 8,
  },
];

// PROFILE_SCENARIOS=<n> limits the run to the first N scenarios (for fast
// verification / CI smoke). Default: run all scenarios.
const scenarioLimit = process.env.PROFILE_SCENARIOS ? parseInt(process.env.PROFILE_SCENARIOS, 10) : scenarios.length;
const activeScenarios = scenarios.slice(0, Math.max(1, scenarioLimit || scenarios.length));

const allResults = [];
const traceDir = path.join(repoRoot, "trace");
if (!fs.existsSync(traceDir)) fs.mkdirSync(traceDir);

function fmtMs(sec) {
  return sec == null ? "null" : (sec * 1000).toFixed(2) + "ms";
}

for (const scenario of activeScenarios) {
  console.log(`\n${"=".repeat(72)}`);
  console.log(`Scenario: ${scenario.name} (${DURATION_SEC}s)`);
  console.log(`${"=".repeat(72)}\n`);

  const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  const client = await page.context().newCDPSession(page);

  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof startPlay === "function", { timeout: 30000 });

  // ---------------------------------------------------------------------------
  // 1a. Capture CDP Performance.getMetrics BEFORE
  // ---------------------------------------------------------------------------
  await client.send("Performance.enable");
  const metricsBefore = await client.send("Performance.getMetrics");
  const beforeMap = new Map(metricsBefore.metrics.map(m => [m.name, m.value]));

  // ---------------------------------------------------------------------------
  // 1b. Inject JS-side crossing instrumentation
  //
  // We wrap the Go→JS batch entry points (playSoundsBatch / playSoundsBatchFlat)
  // to measure the time spent enqueuing each batch on the JS side. This is the
  // only instrumentation that actually produces data; the previously-present
  // __flushSampler / _origFlush / _origProcess hooks were dead (flushAudioQueue
  // and processAudioEvent are module-scoped and never exposed on window), so
  // they have been removed.
  // ---------------------------------------------------------------------------
  await page.evaluate(() => {
    window.__flushProfile = {
      batchArrivals: [],      // { timestamp, batchSize, source }
      crossingTimings: [],    // { jsEntryTime, enqueueDuration, batchSize }
    };
    const fp = window.__flushProfile;

    // Wrap the Go→JS batch entry points to capture crossing timings.
    const origBatch = window.playSoundsBatch;
    const origBatchFlat = window.playSoundsBatchFlat;

    if (origBatch) {
      window.playSoundsBatch = (arr) => {
        const t0 = performance.now();
        const size = Array.isArray(arr) ? arr.length : 1;
        origBatch(arr);
        const t1 = performance.now();
        fp.batchArrivals.push({ timestamp: t0, batchSize: size, source: "batch" });
        fp.crossingTimings.push({ jsEntryTime: t0, enqueueDuration: t1 - t0, batchSize: size });
      };
    }

    if (origBatchFlat) {
      window.playSoundsBatchFlat = (ids, vols, pitches, durs, whens, hasWhen) => {
        const t0 = performance.now();
        const size = ids ? ids.length : 0;
        origBatchFlat(ids, vols, pitches, durs, whens, hasWhen);
        const t1 = performance.now();
        fp.batchArrivals.push({ timestamp: t0, batchSize: size, source: "flat" });
        fp.crossingTimings.push({ jsEntryTime: t0, enqueueDuration: t1 - t0, batchSize: size });
      };
    }
  });

  // ---------------------------------------------------------------------------
  // Build circuit and reset metrics
  // ---------------------------------------------------------------------------
  await page.evaluate((setup) => {
    resetAudioScheduleMetrics?.();
    resetPerfStats?.();
    eval(setup);
    forceDraw?.(); forceDraw?.();
  }, scenario.setup);

  // ---------------------------------------------------------------------------
  // Start CDP CPU + Heap profiling
  // ---------------------------------------------------------------------------
  await client.send("Profiler.enable");
  await client.send("Profiler.setSamplingInterval", { interval: 100 });
  await client.send("Profiler.start");

  await client.send("HeapProfiler.enable");
  await client.send("HeapProfiler.startSampling", { samplingInterval: 1024 });

  // Unlock AudioContext
  await page.evaluate(() => {
    document.dispatchEvent(new Event('pointerdown'));
    resumeAudio?.();
  });
  await page.waitForTimeout(300);

  // ---------------------------------------------------------------------------
  // Run playback
  // ---------------------------------------------------------------------------
  await page.evaluate(() => startPlay());
  await page.waitForTimeout(DURATION_SEC * 1000);
  await page.evaluate(() => stopPlay?.());

  // ---------------------------------------------------------------------------
  // Collect all metrics
  // ---------------------------------------------------------------------------
  const audioMetrics = await page.evaluate(() => getAudioScheduleMetrics?.());
  const perfStatsResult = await page.evaluate(() => perfStats?.());
  const rowCount = await page.evaluate(() => totalRows?.());
  const flushProfile = await page.evaluate(() => window.__flushProfile);

  // 1a. CDP Performance.getMetrics AFTER
  const metricsAfter = await client.send("Performance.getMetrics");
  const afterMap = new Map(metricsAfter.metrics.map(m => [m.name, m.value]));

  // Stop CPU profiler
  const { profile: cpuProfile } = await client.send("Profiler.stop");
  await client.send("Profiler.disable");

  // Stop heap sampler
  const { profile: heapProfile } = await client.send("HeapProfiler.stopSampling");
  await client.send("HeapProfiler.disable");

  await client.send("Performance.disable");

  // Save raw profiles for first scenario
  if (allResults.length === 0) {
    fs.writeFileSync(path.join(traceDir, "cpu_profile.json"), JSON.stringify(cpuProfile, null, 2));
    fs.writeFileSync(path.join(traceDir, "heap_profile.json"), JSON.stringify(heapProfile, null, 2));
  }
  // Save per-scenario profiles
  const safeScenarioName = scenario.name.replace(/[^a-zA-Z0-9]/g, "_").toLowerCase();
  fs.writeFileSync(path.join(traceDir, `cpu_profile_${safeScenarioName}.json`), JSON.stringify(cpuProfile, null, 2));
  fs.writeFileSync(path.join(traceDir, `heap_profile_${safeScenarioName}.json`), JSON.stringify(heapProfile, null, 2));

  // ---------------------------------------------------------------------------
  // Parse CPU profile
  // ---------------------------------------------------------------------------
  console.log("=== CPU Profile Analysis ===\n");

  const nodeMap = new Map();
  if (cpuProfile && cpuProfile.nodes) {
    for (const node of cpuProfile.nodes) {
      nodeMap.set(node.id, node);
    }
  }

  const selfTimes = new Map();
  if (cpuProfile && cpuProfile.samples && cpuProfile.timeDeltas) {
    for (let i = 0; i < cpuProfile.samples.length; i++) {
      const nodeId = cpuProfile.samples[i];
      const delta = cpuProfile.timeDeltas[i] || 0;
      selfTimes.set(nodeId, (selfTimes.get(nodeId) || 0) + delta);
    }
  }

  const funcTimes = new Map();
  for (const [nodeId, time] of selfTimes) {
    const node = nodeMap.get(nodeId);
    if (!node || !node.callFrame) continue;
    const name = node.callFrame.functionName || "(anonymous)";
    const url = node.callFrame.url || "";
    const key = url ? `${name} [${path.basename(url)}]` : name;
    const prev = funcTimes.get(key) || { timeUs: 0, url };
    funcTimes.set(key, { timeUs: prev.timeUs + time, url });
  }

  const totalCpuTime = Array.from(selfTimes.values()).reduce((a, b) => a + b, 0);
  const sortedFuncs = Array.from(funcTimes.entries())
    .map(([name, { timeUs, url }]) => ({ name, timeUs, url, pct: totalCpuTime > 0 ? (timeUs / totalCpuTime * 100) : 0 }))
    .sort((a, b) => b.timeUs - a.timeUs);

  console.log("Top 25 functions by self-time:");
  console.log("  %CPU  | Time (ms) | Function");
  console.log("  ------|-----------|--------");
  for (const f of sortedFuncs.slice(0, 25)) {
    console.log(`  ${f.pct.toFixed(1).padStart(5)}% | ${(f.timeUs / 1000).toFixed(1).padStart(9)} | ${f.name}`);
  }

  // Audio-related functions
  const audioKeywords = ["audio", "play", "flush", "schedule", "wasm", "Go", "render", "processAudio", "enqueue", "observe", "Sample", "voice", "mix", "buffer", "getBus", "channel", "gain", "source", "start"];
  const audioFuncs = sortedFuncs.filter(f =>
    audioKeywords.some(kw => f.name.toLowerCase().includes(kw.toLowerCase()))
  );

  if (audioFuncs.length > 0) {
    console.log("\nAudio-related functions:");
    console.log("  %CPU  | Time (ms) | Function");
    console.log("  ------|-----------|--------");
    for (const f of audioFuncs.slice(0, 20)) {
      console.log(`  ${f.pct.toFixed(1).padStart(5)}% | ${(f.timeUs / 1000).toFixed(1).padStart(9)} | ${f.name}`);
    }
  }

  // Compute category percentages
  let audioFlushPct = 0;
  let wasmOverheadPct = 0;
  let schedulePct = 0;
  for (const f of sortedFuncs) {
    const lc = f.name.toLowerCase();
    if (lc.includes("flush") && lc.includes("audio")) audioFlushPct += f.pct;
    if (lc.includes("wasm") || lc.includes("go.") || lc === "go" || lc.includes("_wasm_") || lc.includes("syscall") || lc.includes("runtime.")) wasmOverheadPct += f.pct;
    if (lc.includes("schedule") || lc.includes("enqueue") || lc.includes("observe")) schedulePct += f.pct;
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
  // CDP Performance Metrics delta
  // ---------------------------------------------------------------------------
  console.log("\n=== CDP Performance Metrics (delta) ===\n");
  const interestingMetrics = [
    "JSHeapUsedSize", "JSHeapTotalSize", "ScriptDuration", "TaskDuration",
    "LayoutDuration", "RecalcStyleDuration", "Nodes", "JSEventListeners",
  ];
  for (const name of interestingMetrics) {
    const before = beforeMap.get(name) ?? 0;
    const after = afterMap.get(name) ?? 0;
    const delta = after - before;
    const suffix = name.includes("Size") ? ` (${(delta / 1024 / 1024).toFixed(2)} MB)` : "";
    console.log(`  ${name}: ${delta.toFixed(3)}${suffix}`);
  }

  // ---------------------------------------------------------------------------
  // Go-to-JS crossing analysis
  // ---------------------------------------------------------------------------
  console.log("\n=== Go-to-JS Crossing Analysis ===\n");
  if (flushProfile && flushProfile.crossingTimings.length > 0) {
    const crossings = flushProfile.crossingTimings;
    const durations = crossings.map(c => c.enqueueDuration);
    const batchSizes = crossings.map(c => c.batchSize);
    durations.sort((a, b) => a - b);
    batchSizes.sort((a, b) => a - b);

    const avg = durations.reduce((a, b) => a + b, 0) / durations.length;
    const p50 = durations[Math.floor(durations.length * 0.5)] || 0;
    const p90 = durations[Math.floor(durations.length * 0.9)] || 0;
    const p99 = durations[Math.floor(durations.length * 0.99)] || 0;
    const max = durations[durations.length - 1] || 0;
    const totalCrossings = crossings.length;

    const avgBatch = batchSizes.reduce((a, b) => a + b, 0) / batchSizes.length;
    const maxBatch = batchSizes[batchSizes.length - 1] || 0;

    console.log(`  Total crossings: ${totalCrossings}`);
    console.log(`  Avg enqueue duration: ${avg.toFixed(3)}ms`);
    console.log(`  P50: ${p50.toFixed(3)}ms  P90: ${p90.toFixed(3)}ms  P99: ${p99.toFixed(3)}ms  Max: ${max.toFixed(3)}ms`);
    console.log(`  Avg batch size: ${avgBatch.toFixed(1)}  Max batch size: ${maxBatch}`);

    // Compare with Go-side audioCallAvg
    if (perfStatsResult && perfStatsResult.audioCallAvg != null) {
      const goCallAvg = perfStatsResult.audioCallAvg;
      const jsSideAvg = avg;
      const overhead = goCallAvg - jsSideAvg;
      console.log(`\n  Go audioCallAvg: ${goCallAvg.toFixed(3)}ms`);
      console.log(`  JS enqueue avg:  ${jsSideAvg.toFixed(3)}ms`);
      console.log(`  Go→JS overhead:  ${overhead.toFixed(3)}ms (${((overhead / goCallAvg) * 100).toFixed(1)}% of total)`);
    }

    // Source breakdown
    const bySrc = {};
    for (const c of crossings) {
      bySrc[c.source] = (bySrc[c.source] || 0) + 1;
    }
    console.log(`  Source breakdown: ${JSON.stringify(bySrc)}`);
  } else {
    console.log("  No crossing data captured (batch functions may not have been called)");
  }

  // ---------------------------------------------------------------------------
  // Audio metrics + perf stats
  // ---------------------------------------------------------------------------
  console.log("\n=== Audio Schedule Metrics ===\n");
  if (audioMetrics) {
    console.log(`  count=${audioMetrics.count} overdue=${audioMetrics.overdue}`);
    console.log(`  minLead=${fmtMs(audioMetrics.minLead)} maxLead=${fmtMs(audioMetrics.maxLead)} avgLead=${fmtMs(audioMetrics.avgLead)}`);
    console.log(`  avgLag=${fmtMs(audioMetrics.avgLag)} lagP90=${fmtMs(audioMetrics.lagP90)} lagP99=${fmtMs(audioMetrics.lagP99)} maxLag=${fmtMs(audioMetrics.maxLag)}`);
    console.log(`  smallLeadCount=${audioMetrics.smallLeadCount}`);
    console.log(`  leadP90=${fmtMs(audioMetrics.leadP90)} leadP99=${fmtMs(audioMetrics.leadP99)}`);

    // Lead time distribution histogram
    if (audioMetrics.history && audioMetrics.history.length > 0) {
      console.log(`\n  Time-bucketed history (${audioMetrics.history.length} buckets):`);
      console.log("    Sec | Count | AvgLead   | LeadP90   | LeadP99   | AvgLag    | LagP90");
      console.log("    ----|-------|-----------|-----------|-----------|-----------|-------");
      for (const h of audioMetrics.history.slice(0, 20)) {
        console.log(`    ${String(h.sec).padStart(3)} | ${String(h.count).padStart(5)} | ${fmtMs(h.avgLead).padStart(9)} | ${fmtMs(h.leadP90).padStart(9)} | ${fmtMs(h.leadP99).padStart(9)} | ${fmtMs(h.avgLag).padStart(9)} | ${fmtMs(h.lagP90).padStart(5)}`);
      }
    }
  }

  console.log("\n=== Perf Stats ===\n");
  if (perfStatsResult) {
    console.log(`  FPS avg: ${perfStatsResult.fpsAvg?.toFixed(1)}`);
    console.log(`  Update avg: ${perfStatsResult.updateAvgMS?.toFixed(3)}ms  max: ${perfStatsResult.updateMaxMS?.toFixed(3)}ms`);
    console.log(`  Draw avg: ${perfStatsResult.drawAvgMS?.toFixed(3)}ms  max: ${perfStatsResult.drawMaxMS?.toFixed(3)}ms`);
    console.log(`  Audio call avg: ${perfStatsResult.audioCallAvg?.toFixed(3)}ms  max: ${perfStatsResult.audioCallMax?.toFixed(3)}ms`);
    console.log(`  Audio QLat avg: ${perfStatsResult.audioQLatAvg?.toFixed(3)}ms  max: ${perfStatsResult.audioQLatMax?.toFixed(3)}ms`);
    console.log(`  Audio enq: ${perfStatsResult.audioEnq}  deq: ${perfStatsResult.audioDeq}`);
    console.log(`  Go heap: ${perfStatsResult.heapAllocKB}KB  sys: ${perfStatsResult.heapSysKB}KB  objects: ${perfStatsResult.heapObjects}`);
    console.log(`  Goroutines: ${perfStatsResult.goroutines}`);
  }

  // ---------------------------------------------------------------------------
  // Batch arrival pattern analysis
  // ---------------------------------------------------------------------------
  console.log("\n=== Batch Arrival Patterns ===\n");
  if (flushProfile && flushProfile.batchArrivals.length > 0) {
    const arrivals = flushProfile.batchArrivals;
    const sizes = arrivals.map(a => a.batchSize);
    sizes.sort((a, b) => a - b);
    const sizeAvg = sizes.reduce((a, b) => a + b, 0) / sizes.length;
    const sizeP50 = sizes[Math.floor(sizes.length * 0.5)] || 0;
    const sizeP90 = sizes[Math.floor(sizes.length * 0.9)] || 0;
    const sizeMax = sizes[sizes.length - 1] || 0;

    console.log(`  Total batches: ${arrivals.length}`);
    console.log(`  Batch size avg: ${sizeAvg.toFixed(1)}  P50: ${sizeP50}  P90: ${sizeP90}  Max: ${sizeMax}`);

    // Inter-arrival times
    if (arrivals.length > 1) {
      const gaps = [];
      for (let i = 1; i < arrivals.length; i++) {
        gaps.push(arrivals[i].timestamp - arrivals[i - 1].timestamp);
      }
      gaps.sort((a, b) => a - b);
      const gapAvg = gaps.reduce((a, b) => a + b, 0) / gaps.length;
      const gapP50 = gaps[Math.floor(gaps.length * 0.5)] || 0;
      const gapP90 = gaps[Math.floor(gaps.length * 0.9)] || 0;
      const gapMin = gaps[0] || 0;
      console.log(`  Inter-arrival gap avg: ${gapAvg.toFixed(1)}ms  P50: ${gapP50.toFixed(1)}ms  P90: ${gapP90.toFixed(1)}ms  Min: ${gapMin.toFixed(3)}ms`);
    }

    // Size histogram
    const sizeHist = {};
    for (const s of sizes) {
      const bucket = s <= 1 ? "1" : s <= 2 ? "2" : s <= 4 ? "3-4" : s <= 8 ? "5-8" : s <= 16 ? "9-16" : "17+";
      sizeHist[bucket] = (sizeHist[bucket] || 0) + 1;
    }
    console.log(`  Batch size histogram: ${JSON.stringify(sizeHist)}`);
  }

  // ---------------------------------------------------------------------------
  // Gate checks (hard by default) + informational warnings
  // ---------------------------------------------------------------------------
  console.log("\n=== Gate Checks ===\n");
  const warnings = [];   // informational only — never affects exit code
  const failures = [];   // hard failures — set non-zero exit unless PROFILE_SOFT

  // --- Hard gates: audio scheduler health (populated on a healthy run) ---
  if (audioMetrics) {
    if (audioMetrics.overdue > GATES.overdueMax) {
      failures.push(`${audioMetrics.overdue} overdue audio events detected (cap=${GATES.overdueMax})`);
    }
    // minLead is in seconds; a healthy run keeps every event >=3ms ahead.
    const minLeadMs = audioMetrics.minLead != null ? audioMetrics.minLead * 1000 : null;
    if (minLeadMs != null && minLeadMs < GATES.minLeadMinMs) {
      failures.push(`minLead ${minLeadMs.toFixed(2)}ms below ${GATES.minLeadMinMs}ms floor`);
    }
    if (audioMetrics.smallLeadCount != null && audioMetrics.smallLeadCount > GATES.smallLeadMax) {
      const pct = audioMetrics.count > 0 ? ((audioMetrics.smallLeadCount / audioMetrics.count) * 100).toFixed(1) : "?";
      failures.push(`${audioMetrics.smallLeadCount} events with <3ms lead (${pct}%, cap=${GATES.smallLeadMax})`);
    }
  } else {
    failures.push("no audio schedule metrics collected (getAudioScheduleMetrics returned nothing)");
  }

  // --- Hard gates: CPU / WASM overhead caps (from the CDP CPU profile) ---
  if (audioFlushPct > GATES.audioFlushPctMax) {
    failures.push(`audio-flush CPU ${audioFlushPct.toFixed(1)}% exceeds ${GATES.audioFlushPctMax}% cap`);
  }
  if (wasmOverheadPct > GATES.wasmOverheadPctMax) {
    failures.push(`WASM/Go overhead ${wasmOverheadPct.toFixed(1)}% exceeds ${GATES.wasmOverheadPctMax}% cap`);
  }

  // --- Informational only: lagP99 is null/0 on a healthy run (lead>=0), so
  //     this is surfaced as a WARN, never a hard failure. ---
  if (audioMetrics && audioMetrics.lagP99 != null && audioMetrics.lagP99 > 0.020) {
    warnings.push(`lagP99 ${(audioMetrics.lagP99 * 1000).toFixed(2)}ms exceeds 20ms during profiling (diagnostic only)`);
  }

  if (failures.length > 0) {
    const tag = PROFILE_SOFT ? "WARN (soft mode)" : "FAIL";
    for (const f of failures) console.log(`  ${tag}: ${f}`);
    if (!PROFILE_SOFT) {
      for (const f of failures) hardFailures.push(`[${scenario.name}] ${f}`);
    }
  } else {
    console.log("  All hard gates passed");
  }
  if (warnings.length > 0) {
    for (const w of warnings) console.log(`  WARN: ${w}`);
  }

  // Store results for summary
  allResults.push({
    scenario: scenario.name,
    bpm: scenario.bpm,
    rows: rowCount ?? scenario.rows ?? "?",
    perfStats: perfStatsResult,
    audioMetrics,
    flushProfile,
    cpuSummary: {
      audioFlushPct,
      wasmOverheadPct,
      schedulePct,
      top5: sortedFuncs.slice(0, 5).map(f => ({ name: f.name, pct: f.pct })),
    },
    heapSummary: {
      top5: sortedAllocs.slice(0, 5).map(a => ({ name: a.name, sizeKB: a.sizeKB, count: a.count })),
    },
    cdpMetrics: {
      jsHeapDelta: (afterMap.get("JSHeapUsedSize") ?? 0) - (beforeMap.get("JSHeapUsedSize") ?? 0),
      scriptDuration: (afterMap.get("ScriptDuration") ?? 0) - (beforeMap.get("ScriptDuration") ?? 0),
      taskDuration: (afterMap.get("TaskDuration") ?? 0) - (beforeMap.get("TaskDuration") ?? 0),
    },
    crossingSummary: flushProfile && flushProfile.crossingTimings.length > 0 ? (() => {
      const ds = flushProfile.crossingTimings.map(c => c.enqueueDuration).sort((a, b) => a - b);
      return {
        count: ds.length,
        avg: ds.reduce((a, b) => a + b, 0) / ds.length,
        p90: ds[Math.floor(ds.length * 0.9)] || 0,
        p99: ds[Math.floor(ds.length * 0.99)] || 0,
        max: ds[ds.length - 1] || 0,
      };
    })() : null,
    warnings,
    failures,
  });

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "audio_profile_cdp");
  await browser.close();
}

// ---------------------------------------------------------------------------
// Cross-scenario comparison table
// ---------------------------------------------------------------------------
console.log(`\n${"=".repeat(100)}`);
console.log("CROSS-SCENARIO COMPARISON");
console.log(`${"=".repeat(100)}\n`);

console.log("╔══════════════════════════════════╦═══════╦═════════╦═════════╦═══════╦═════════╦═════════╦═══════════╗");
console.log("║ Scenario                         ║  FPS  ║ Upd Avg ║ Drw Avg ║ Count ║ Overdue ║ lagP99  ║ SmallLead ║");
console.log("╠══════════════════════════════════╬═══════╬═════════╬═════════╬═══════╬═════════╬═════════╬═══════════╣");

for (const r of allResults) {
  const fps = r.perfStats?.fpsAvg?.toFixed(1) ?? "?";
  const updAvg = r.perfStats?.updateAvgMS?.toFixed(2) ?? "?";
  const drwAvg = r.perfStats?.drawAvgMS?.toFixed(2) ?? "?";
  const count = r.audioMetrics?.count ?? "?";
  const overdue = r.audioMetrics?.overdue ?? "?";
  const lagP99 = fmtMs(r.audioMetrics?.lagP99);
  const smallLead = r.audioMetrics?.smallLeadCount ?? "?";
  console.log(`║ ${r.scenario.padEnd(32)} ║ ${String(fps).padStart(5)} ║ ${String(updAvg).padStart(7)} ║ ${String(drwAvg).padStart(7)} ║ ${String(count).padStart(5)} ║ ${String(overdue).padStart(7)} ║ ${lagP99.padStart(7)} ║ ${String(smallLead).padStart(9)} ║`);
}
console.log("╚══════════════════════════════════╩═══════╩═════════╩═════════╩═══════╩═════════╩═════════╩═══════════╝");

// Go-to-JS crossing comparison
console.log("\n╔══════════════════════════════════╦═══════════╦═════════╦═════════╦═════════╦═══════════════════╗");
console.log("║ Scenario                         ║ Crossings ║ Avg ms  ║ P90 ms  ║ P99 ms  ║ GoCallAvg-JSAvg   ║");
console.log("╠══════════════════════════════════╬═══════════╬═════════╬═════════╬═════════╬═══════════════════╣");

for (const r of allResults) {
  const cs = r.crossingSummary;
  const goAvg = r.perfStats?.audioCallAvg ?? 0;
  const jsAvg = cs ? cs.avg : 0;
  const overhead = goAvg - jsAvg;
  console.log(`║ ${r.scenario.padEnd(32)} ║ ${String(cs?.count ?? 0).padStart(9)} ║ ${(cs?.avg ?? 0).toFixed(3).padStart(7)} ║ ${(cs?.p90 ?? 0).toFixed(3).padStart(7)} ║ ${(cs?.p99 ?? 0).toFixed(3).padStart(7)} ║ ${overhead.toFixed(3).padStart(7)}ms (overhead) ║`);
}
console.log("╚══════════════════════════════════╩═══════════╩═════════╩═════════╩═════════╩═══════════════════╝");

// Save combined results JSON
fs.writeFileSync(path.join(traceDir, "cdp_profile_results.json"), JSON.stringify(allResults, null, 2));

console.log(`\nProfiles saved to: ${traceDir}/`);

// ---------------------------------------------------------------------------
// Final verdict — set non-zero exit on any hard gate failure (default ON).
// ---------------------------------------------------------------------------
server.close();

if (hardFailures.length > 0) {
  console.error(`\n${"!".repeat(72)}`);
  console.error(`audio_profile_cdp: FAILED — ${hardFailures.length} hard gate violation(s):`);
  for (const f of hardFailures) console.error(`  - ${f}`);
  console.error(`(set PROFILE_SOFT=1 to downgrade these to warnings)`);
  console.error(`${"!".repeat(72)}`);
  process.exit(1);
}

console.log("\naudio_profile_cdp: complete (all hard gates passed)");
