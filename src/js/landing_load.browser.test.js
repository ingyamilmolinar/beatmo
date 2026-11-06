// landing_load.browser.test.js
//
// Cold-load evidence for the landing page (spec
// docs/superpowers/specs/2026-10-23-landing-page-design.md): measures the
// game's time-to-interactive with and without the landing under THROTTLED
// network + CPU, and checks the landing's boundary behaviour (shell paints,
// crystal draws, Play enables on readiness and hands over, the opt-out skip,
// muted video). The landing's full behaviour is landing_behavior.browser.test.js.
//
// Why throttled: on localhost bandwidth is infinite, so a landing that steals
// bandwidth from main.wasm would pass any TTI gate by construction.
//
// Every run is COLD (a fresh browser context = empty HTTP cache + storage).
// Arms run interleaved so drift in machine load hits every arm alike.
//
// Env:
//   WASM_FILE=<path>          main.wasm to serve (default src/js/main.wasm)
//   LANDING_RUNS=<n>          cold runs per arm (default 3)
//   LANDING_PROFILE=4g|slow4g|desktop|none   (default 4g)
//   LANDING_ARMS=a,b          subset of arms (baseline,landing,skip,nolanding)
//   BASELINE_INDEX=<path>     index.html for the "baseline" arm (no landing)
//   LANDING_ENCODING=gzip|br  main.wasm content-encoding (default gzip)
//   LANDING_TTI_MAX_RATIO     landing/baseline median TTI gate (default 1.10)
//   LANDING_BENCH_OUT=<path>  write the JSON results here
//   LANDING_CHANNEL=chrome    browser channel (default chrome; "" = bundled Chromium)

import { chromium } from "playwright";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { createLandingServer } from "./landing_server.js";

const jsDir = path.dirname(fileURLToPath(import.meta.url));
const WASM_FILE = process.env.WASM_FILE || path.join(jsDir, "main.wasm");
const RUNS = Number(process.env.LANDING_RUNS || (process.env.BASELINE_INDEX ? 3 : 1));
const PROFILE = process.env.LANDING_PROFILE || "4g";
const ENCODING = process.env.LANDING_ENCODING || "gzip";
const MAX_RATIO = Number(process.env.LANDING_TTI_MAX_RATIO || 1.1);
const BASELINE_INDEX = process.env.BASELINE_INDEX || "";
const ARMS = (process.env.LANDING_ARMS || (BASELINE_INDEX ? "baseline,landing,skip" : "landing,skip")).split(",");

// Throughput in bytes/s, latency in ms. "4g" ~ a decent phone connection,
// "slow4g" the Lighthouse mobile profile. CPU rate 4 = a mid-range phone.
const PROFILES = {
  "4g": { latency: 85, down: (9e6 / 8), up: (1.5e6 / 8), cpu: 4, viewport: { width: 390, height: 844 }, mobile: true },
  slow4g: { latency: 150, down: (1.6e6 / 8), up: (0.75e6 / 8), cpu: 4, viewport: { width: 390, height: 844 }, mobile: true },
  desktop: { latency: 40, down: (30e6 / 8), up: (5e6 / 8), cpu: 1, viewport: { width: 1280, height: 720 }, mobile: false },
  none: null,
};
const prof = PROFILES[PROFILE];

// Server: mirrors production (landing_server.js) — text + wasm pre-compressed
// (gzip, or br for main.wasm when asked), video served raw with Range support,
// index.html stamped with a content hash exactly like the deploy does.
const server = createLandingServer({ wasmFile: WASM_FILE, encoding: ENCODING, level: 9, baselineIndex: BASELINE_INDEX });
await new Promise((r) => server.listen(0, r));
const base = `http://localhost:${server.address().port}`;
// Real Chrome when installed: Playwright's bundled Chromium has no H.264, so
// the landing would (correctly) never download its video there.
const CHANNEL = process.env.LANDING_CHANNEL ?? "chrome";
let browser;
try {
  browser = await chromium.launch({ channel: CHANNEL || undefined, args: ["--autoplay-policy=no-user-gesture-required"] });
} catch (_) {
  console.log(`(channel ${CHANNEL} unavailable — bundled Chromium, no H.264: video bytes will read 0)`);
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
}

const errors = [];
const check = (cond, msg) => { if (!cond) errors.push(msg); };

async function coldPage(viewport) {
  const ctx = await browser.newContext({ viewport: viewport || prof?.viewport || { width: 1280, height: 720 }, isMobile: !!prof?.mobile, hasTouch: !!prof?.mobile });
  const page = await ctx.newPage();
  const cdp = await ctx.newCDPSession(page);
  await cdp.send("Network.enable");
  if (prof) {
    await cdp.send("Network.emulateNetworkConditions", { offline: false, latency: prof.latency, downloadThroughput: prof.down, uploadThroughput: prof.up });
    await cdp.send("Emulation.setCPUThrottlingRate", { rate: prof.cpu });
  }
  const bytes = new Map();
  const urls = new Map();
  cdp.on("Network.requestWillBeSent", (e) => urls.set(e.requestId, e.request.url));
  cdp.on("Network.dataReceived", (e) => {
    const u = urls.get(e.requestId) || "?";
    bytes.set(u, (bytes.get(u) || 0) + e.encodedDataLength);
  });
  return { ctx, page, cdp, bytes };
}

// The game is interactive when its first full frame has drawn AND the playback
// readiness gate (circuit warm-up) is open — the same condition the landing's
// Play button waits for.
const READY_FN = () => {
  const b = window.__beatmoBoot;
  if (!b || b.firstFrame == null) return false;
  try { return !!(typeof warmupProgress === "function" && warmupProgress().ready); } catch (_) { return false; }
};

async function measure(arm) {
  const { ctx, page, bytes } = await coldPage();
  try {
    if (arm === "skip") {
      // A visitor who ticked "Do not show tutorial demo again" on an earlier
      // visit. Only the decision path is verified (the landing never appears,
      // nothing of it is fetched) — the skip arm's timing is not compared.
      await page.addInitScript(() => { try { localStorage.setItem("beatmo.landing.optOut", "1"); } catch (_) {} });
    }
    const t0 = Date.now();
    await page.goto(`${base}/?arm=${arm}${arm === "nolanding" ? "&landing=0" : ""}${arm === "videolate" ? "&video=late" : ""}${arm === "nocrystal" ? "&crystal=0" : ""}`);
    await page.waitForFunction(READY_FN, null, { timeout: 240000, polling: 100 });
    const wall = Date.now() - t0;
    const r = await page.evaluate(() => {
      const nav = performance.getEntriesByType("navigation")[0];
      const fcp = performance.getEntriesByName("first-contentful-paint")[0];
      const res = performance.getEntriesByType("resource").map((e) => ({ name: e.name.split("/").pop(), start: e.startTime, end: e.responseEnd, enc: e.encodedBodySize }));
      return {
        boot: { ...window.__beatmoBoot },
        fcp: fcp ? fcp.startTime : null,
        dcl: nav ? nav.domContentLoadedEventEnd : null,
        landingShown: document.documentElement.hasAttribute("data-landing"),
        res,
      };
    });
    let crystal = null, handover = null, videoBeforeReady = null;
    if (r.landingShown) {
      crystal = await page.evaluate(() => window.__beatmoLanding && window.__beatmoLanding.stats()) || { frames: 0, p50: 0, p95: 0, nodes: 0 };
      videoBeforeReady = r.res.filter((e) => /\.(mp4|webm)$/.test(e.name)).map((e) => e.start);
      // Play hands over: the landing goes, the game's transport runs.
      const t1 = Date.now();
      await page.click("#lc-play");
      await page.waitForFunction(() => !document.getElementById("landing") && typeof isPlaying === "function" && isPlaying(), null, { timeout: 20000, polling: 100 });
      handover = Date.now() - t1;
    }
    const landingJsFetched = r.res.some((e) => /^landing/.test(e.name));
    const wasm = r.res.find((e) => /^main(\.[0-9a-f]+)?\.wasm$/.test(e.name));
    const totalBytes = [...bytes.values()].reduce((a, b) => a + b, 0);
    const videoBytes = [...bytes.entries()].filter(([u]) => /\.(mp4|webm)/.test(u)).reduce((a, [, b]) => a + b, 0);
    return {
      arm,
      ttiFirstFrame: r.boot.firstFrame,
      readyWall: wall,
      fcp: r.fcp,
      wasmFetchStart: wasm ? wasm.start : null,
      wasmDone: wasm ? wasm.end : null,
      landingShown: r.landingShown,
      bytesBeforeReady: totalBytes,
      videoBytesBeforeReady: videoBytes,
      videoStartBeforeReady: videoBeforeReady,
      landingJsFetched,
      crystal,
      handoverMs: handover,
    };
  } finally {
    await ctx.close();
  }
}

const median = (xs) => { const s = xs.filter((x) => x != null).sort((a, b) => a - b); return s.length ? s[Math.floor(s.length / 2)] : null; };

try {
  const results = [];
  for (let i = 0; i < RUNS; i++) {
    for (const arm of ARMS) {
      const m = await measure(arm);
      results.push(m);
      if (m.crystal) console.log(`    crystal: ${m.crystal.frames} frames, draw p50=${m.crystal.p50.toFixed(2)}ms p95=${m.crystal.p95.toFixed(2)}ms, ${m.crystal.nodes} nodes; handover ${m.handoverMs}ms; video requests at ${JSON.stringify(m.videoStartBeforeReady)}`);
      console.log(`[${PROFILE}] run ${i + 1} ${arm.padEnd(8)} fcp=${m.fcp?.toFixed(0)} wasmStart=${m.wasmFetchStart?.toFixed(0)} wasmDone=${m.wasmDone?.toFixed(0)} firstFrame=${m.ttiFirstFrame?.toFixed(0)} ready=${m.readyWall} landing=${m.landingShown} bytes=${(m.bytesBeforeReady / 1e6).toFixed(2)}MB video=${(m.videoBytesBeforeReady / 1e6).toFixed(2)}MB`);
    }
  }
  const summary = {};
  for (const arm of ARMS) {
    const rs = results.filter((r) => r.arm === arm);
    summary[arm] = {
      fcp: median(rs.map((r) => r.fcp)),
      wasmFetchStart: median(rs.map((r) => r.wasmFetchStart)),
      wasmDone: median(rs.map((r) => r.wasmDone)),
      ttiFirstFrame: median(rs.map((r) => r.ttiFirstFrame)),
      readyWall: median(rs.map((r) => r.readyWall)),
      bytesBeforeReady: median(rs.map((r) => r.bytesBeforeReady)),
      videoBytesBeforeReady: median(rs.map((r) => r.videoBytesBeforeReady)),
    };
  }
  console.log(`\n=== cold load, profile=${PROFILE}, encoding=${ENCODING}, runs=${RUNS} (medians, ms from navigation) ===`);
  console.table(summary);
  if (process.env.LANDING_BENCH_OUT) {
    fs.writeFileSync(process.env.LANDING_BENCH_OUT, JSON.stringify({ profile: PROFILE, encoding: ENCODING, runs: RUNS, wasmBytes: server.wasmBytes(), summary, results }, null, 2));
  }

  // Gates.
  for (const r of results) {
    if (["landing", "videolate", "nocrystal"].includes(r.arm)) check(r.landingShown, `landing arm: landing not shown (run ${JSON.stringify(r)})`);
    if (r.arm === "skip") {
      check(!r.landingShown, "skip arm: a visitor who opted out saw the landing");
      check(!r.landingJsFetched, "skip arm: landing files were fetched although the landing was skipped");
    }
    if (r.arm === "landing") {
      check(r.crystal && r.crystal.frames > 10, `landing arm: the crystal drew ${r.crystal ? r.crystal.frames : 0} frames`);
      check(r.handoverMs != null, "landing arm: Play did not hand over to a playing game");
      for (const t of r.videoStartBeforeReady || []) check(t >= r.wasmDone - 1, `landing arm: video requested at ${t.toFixed(0)}ms, before the game's download finished (${r.wasmDone.toFixed(0)}ms)`);
    }
  }
  if (summary.baseline && summary.landing) {
    const ratio = summary.landing.ttiFirstFrame / summary.baseline.ttiFirstFrame;
    console.log(`TTI ratio landing/baseline = ${ratio.toFixed(3)} (gate ${MAX_RATIO})`);
    check(ratio <= MAX_RATIO, `landing TTI ${summary.landing.ttiFirstFrame.toFixed(0)}ms > ${MAX_RATIO}x baseline ${summary.baseline.ttiFirstFrame.toFixed(0)}ms`);
  }
} finally {
  await browser.close();
  server.close();
}

if (errors.length) {
  console.error("\nlanding_load: FAILED");
  for (const e of errors) console.error("  - " + e);
  process.exit(1);
}
console.log("\nlanding_load: OK");
