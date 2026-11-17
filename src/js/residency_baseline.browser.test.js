// residency_baseline.browser.test.js
//
// Evidence for residency decisions (spec
// docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md
// §1.1), in the real app on the startup demo:
//
//   boot     what exists before playing: WebAudio chains, beatmo-dsp stages,
//            voice renders (count + bytes), param blocks pushed, Go's
//            registered instruments and mixer channels.
//   playing  the same after a few seconds of playback, plus what is HEARD
//            (playback probe).
//   switch   a row switched to a different instrument every 500 ms while
//            playing — the switch rule's cost, judged on the heard audio.
//
// Writes bench-results/residency_baseline_<date>.json. Hard-gates silent and
// dropped notes (a 2026-10-31 run caught a release cutting the probe's channel
// tap this way); lateness and clicks are reported unless AUDIO_PROBE_STRICT=1. Skips (exit 0, reported) on a machine above RESIDENCY_MAX_LOAD.
//
// Run:
//   GO=$(pwd)/.tools/go/bin/go node src/js/residency_baseline.browser.test.js
// Env: WASM_PREBUILT=1, WASM_FILE=<wasm>, RESIDENCY_SECS (default 8),
//      RESIDENCY_MAX_LOAD (default 15).

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
// A silent or dropped note is a hard failure. Lateness (headless software GL
// dominates it — CLAUDE.md) and clicks (one in ~3 runs of the demo, with
// channel release disabled too — 2026-10-31) are reported unless
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
