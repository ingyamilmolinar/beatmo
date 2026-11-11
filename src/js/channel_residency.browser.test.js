// channel_residency.browser.test.js
//
// Audio resources follow use (spec
// docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md
// §1.2), in the real app:
//
//   boot      channels exist only for what the circuit uses (it was one
//             WebAudio chain per registered instrument: 151).
//   playing   only the instruments the rows play keep a chain; one nothing
//             plays is released once silent.
//   tail      a row switched away while its delay rings keeps the channel
//             until the echoes have died away (never cut), then releases it.
//   A→B→A     switching back within the hold releases nothing.
//   budget    over the render budget, renders of instruments no row plays go
//             first; the rows' renders stay.
//
// Run:
//   GO=$(pwd)/.tools/go/bin/go node src/js/channel_residency.browser.test.js
// Env: WASM_PREBUILT=1, WASM_FILE=<wasm>.

import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { launchBrowser } from "./platform/launch.js";
import { startPlaybackWhenReady } from "./audio_probe_bench_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const WASM_FILE = process.env.WASM_FILE ? path.resolve(process.env.WASM_FILE) : path.join(jsDir, "main.wasm");

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
const check = (cond, msg) => { if (!cond) failures.push(msg); console.log(`  ${cond ? "ok  " : "FAIL"} ${msg}`); };

const { browser, cleanup } = await launchBrowser();
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  const pageErrors = [];
  page.on("pageerror", (e) => pageErrors.push(e.message));
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof startPlay === "function" && typeof audioGraphStats === "function" &&
    typeof drumRowCount === "function", null, { timeout: 120000 });

  const stats = () => page.evaluate(() => audioGraphStats());
  const rowInstruments = () => page.evaluate(() => {
    const ids = new Set();
    for (let r = 0; r < drumRowCount(); r++) ids.add(rowInstrument(r));
    return [...ids];
  });

  console.log("boot");
  const boot = await stats();
  const bootRows = await rowInstruments();
  check(boot.liveChannels <= bootRows.length + 1,
    `boot builds channels only for what the circuit uses, not one per registered instrument: liveChannels=${boot.liveChannels} ${JSON.stringify(boot.liveChannelIds)}`);

  console.log("playing");
  await startPlaybackWhenReady(page, {});
  await page.waitForTimeout(3000);
  const used = await rowInstruments();
  const playing = await stats();
  const strays = playing.liveChannelIds.filter((id) => !used.includes(id));
  check(playing.liveChannels > 0 && strays.length === 0,
    `only the rows' instruments keep a channel: ${JSON.stringify(playing.liveChannelIds)}, rows play ${JSON.stringify(used)}`);

  console.log("a ringing tail keeps its channel");
  // The hi-hat row (a short voice: its echoes have silent gaps between them,
  // which a release that ignored the delay's memory would fall into) gets a
  // 1 s delay with feedback 0.3, plays a while, then is switched to an
  // instrument no row plays.
  const tailRow = await page.evaluate(() => {
    for (let r = 0; r < drumRowCount(); r++) if (/hihat/.test(rowInstrument(r))) return r;
    return 0;
  });
  const ringing = await page.evaluate((r) => rowInstrument(r), tailRow);
  await page.evaluate((id) => {
    const i = addInsertEffect(id, "delay");
    setInsertEffectParam(id, i, "time", 1000);
    setInsertEffectParam(id, i, "feedback", 0.3);
    setInsertEffectParam(id, i, "mix", 1);
  }, ringing);
  await page.waitForTimeout(2000);
  const other = await page.evaluate((used) => {
    for (const id of ["cowbell", "clap", "shaker", "conga", "kick"]) if (!used.includes(id)) return id;
    return null;
  }, used);
  const releasedAfter = await page.evaluate(async ([r, id, ring]) => {
    setRowInstrument(r, id);
    const t0 = performance.now();
    while (performance.now() - t0 < 20000) {
      await new Promise((res) => setTimeout(res, 100));
      if (!audioGraphStats().liveChannelIds.includes(ring)) return (performance.now() - t0) / 1000;
    }
    return null;
  }, [tailRow, other, ringing]);
  console.log(`  ${ringing} released ${releasedAfter} s after the switch`);
  check(releasedAfter !== null && releasedAfter > 2.5, `${ringing}'s delay echoes after the switch keep its channel live, then it is released (after ${releasedAfter} s)`);

  console.log("switching back within the hold rebuilds nothing");
  const back = await page.evaluate(() => rowInstrument(1));
  const alt = await page.evaluate((used) => {
    for (const id of ["tom", "clap", "shaker", "conga", "rim"]) if (!used.includes(id)) return id;
    return null;
  }, used);
  const keptLive = await page.evaluate(async ([a, b]) => {
    setRowInstrument(1, a);
    setRowInstrument(1, b);
    const t0 = performance.now();
    while (performance.now() - t0 < 1500) {
      if (!audioGraphStats().liveChannelIds.includes(b)) return false;
      await new Promise((res) => setTimeout(res, 50));
    }
    return true;
  }, [alt, back]);
  check(keptLive, `A→B→A keeps ${back}'s channel live throughout`);

  console.log("render budget");
  // A render of an instrument no row plays is evicted first when the budget
  // is exceeded; the rows' renders are never evicted.
  const spare = await page.evaluate(async (playing) => {
    for (const id of ["shaker", "conga", "tambourine", "rim"]) {
      if (playing.includes(id)) continue;
      await ensureSynthSample(id);
      return id;
    }
    return null;
  }, await rowInstruments());
  const budget = await page.evaluate(async (id) => {
    const had = voiceCachedFor(id, 0);
    setRenderCacheBudget(1);
    const rows = [];
    for (let r = 0; r < drumRowCount(); r++) rows.push(rowInstrument(r));
    const s = audioGraphStats();
    return { had, kept: voiceCachedFor(id, 0), rowsKept: rows.filter((x) => voiceCachedFor(x, 0)).length, rows: rows.length, evicted: s.rendersEvicted };
  }, spare);
  check(budget.had && !budget.kept && budget.evicted > 0, `over budget, ${spare}'s render (no row plays it) is evicted: ${JSON.stringify(budget)}`);
  check(budget.rowsKept > 0, `the rows' renders are kept: ${budget.rowsKept}/${budget.rows} still cached`);

  check(pageErrors.length === 0, `no page errors (${pageErrors.join(" | ")})`);
} finally {
  await cleanup();
  server.close();
}

if (failures.length) {
  console.error(`\n${failures.length} failure(s):\n  ${failures.join("\n  ")}`);
  process.exit(1);
}
console.log("\nchannel residency: PASS");
