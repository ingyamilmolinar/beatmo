// Evidence for residency decisions on the startup demo: what exists at boot, after playing, and under a row switch
// every 500 ms, judged on the heard audio. Hard-gates silent and dropped notes; lateness and clicks report unless
// AUDIO_PROBE_STRICT=1. Run: GO=… node src/js/residency_baseline.browser.test.js (RESIDENCY_SECS, RESIDENCY_MAX_LOAD)

import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { launchBrowser } from "./platform/launch.js";
import {
  startPlaybackWhenReady, startPlaybackProbe, stopPlaybackProbe, probeSummaryFields, probeGateFailures, systemLoad,
  PROBE_STRICT,
} from "./audio_probe_bench_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repo = path.resolve(jsDir, "../..");
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const WASM_FILE = process.env.WASM_FILE ? path.resolve(process.env.WASM_FILE) : path.join(jsDir, "main.wasm");
const SECS = Number(process.env.RESIDENCY_SECS ?? 8);
const MAX_LOAD = Number(process.env.RESIDENCY_MAX_LOAD ?? 15);
const SWITCH_TO = ["clap", "shaker", "conga", "cowbell", "tom", "rim", "snare", "kick"];

const load0 = systemLoad();
if (load0 > MAX_LOAD) {
  console.log(`residency baseline: SKIPPED — load ${load0} > ${MAX_LOAD} (timings would be meaningless)`);
  process.exit(0);
}

if (!process.env.WASM_FILE && !shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(GO, ["build", "-o", WASM_FILE, "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const TYPES = { ".html": "text/html", ".js": "application/javascript", ".wasm": "application/wasm", ".json": "application/json" };
const server = http.createServer((req, res) => {
  const url = req.url.split("?")[0];
  const file = url === "/" ? "/index.html" : url;
  const filePath = file === "/main.wasm" ? WASM_FILE : path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    res.writeHead(200, { "Content-Type": TYPES[path.extname(filePath)] || "text/plain", "Cache-Control": "no-store" });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const failures = [];
// Lateness (dominated by headless software GL) and clicks (seen with release disabled too) report unless
// AUDIO_PROBE_STRICT=1.
const gate = (report, label) => {
  for (const f of probeGateFailures(report, undefined, label)) {
    if (PROBE_STRICT || /missed|dropped|judged no notes/.test(f)) failures.push(f);
    else console.log(`  [timing info] ${f}`);
  }
};
const result = { date: new Date().toISOString(), load: load0, secs: SECS };
const { browser, platform, cleanup } = await launchBrowser();
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof startPlay === "function" && typeof audioGraphStats === "function" &&
    typeof perfStats === "function", null, { timeout: 120000 });
  const snapshot = () => page.evaluate(() => {
    const a = audioGraphStats();
    const p = perfStats();
    return {
      liveChannels: a.liveChannels, dspStages: a.dspStages, renderEntries: a.renderEntries,
      renderMB: +(a.renderBytes / 2 ** 20).toFixed(2), blocksPushed: a.blocksPushed,
      channelsReleased: a.channelsReleased, rendersEvicted: a.rendersEvicted,
      goMixerChannels: p.mixerChannels, goRegisteredInstruments: p.registeredInstruments, heapKB: p.heapAllocKB,
    };
  });

  result.boot = await snapshot();
  console.log("boot   ", JSON.stringify(result.boot));

  await startPlaybackWhenReady(page, {});
  await page.waitForTimeout(1500);
  let started = await startPlaybackProbe(page);
  if (!started?.ok) throw new Error(`probe did not start: ${started?.error}`);
  await page.waitForTimeout(SECS * 1000);
  let { report } = await stopPlaybackProbe(page, "playing");
  result.playing = { ...(await snapshot()), heard: probeSummaryFields(report) };
  gate(report, "playing");
  console.log("playing", JSON.stringify(result.playing));

  started = await startPlaybackProbe(page);
  if (!started?.ok) throw new Error(`probe did not start: ${started?.error}`);
  const switches = await page.evaluate(async ([to, secs]) => {
    let n = 0;
    const t0 = performance.now();
    while (performance.now() - t0 < secs * 1000) {
      setRowInstrument(1, to[n % to.length]);
      n++;
      await new Promise((r) => setTimeout(r, 500));
    }
    return n;
  }, [SWITCH_TO, SECS]);
  ({ report } = await stopPlaybackProbe(page, "switch"));
  result.switch = { switches, ...(await snapshot()), heard: probeSummaryFields(report) };
  gate(report, "switch");
  console.log("switch ", JSON.stringify(result.switch));
  result.platform = platform;
} finally {
  await cleanup();
  server.close();
}

const out = path.join(repo, "bench-results", `residency_baseline_${result.date.slice(0, 10)}.json`);
fs.mkdirSync(path.dirname(out), { recursive: true });
fs.writeFileSync(out, JSON.stringify(result, null, 2));
console.log(`wrote ${path.relative(repo, out)}`);
if (failures.length) {
  console.error(`\n${failures.length} failure(s):\n  ${failures.join("\n  ")}`);
  process.exit(1);
}
console.log("residency baseline: PASS");
