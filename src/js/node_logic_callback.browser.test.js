import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

const port = 8620 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  if (!fs.existsSync(filePath)) {
    res.writeHead(404);
    res.end();
    return;
  }
  let ct = "text/plain";
  if (filePath.endsWith(".html")) ct = "text/html";
  else if (filePath.endsWith(".js")) ct = "application/javascript";
  else if (filePath.endsWith(".wasm")) ct = "application/wasm";
  res.writeHead(200, { "Content-Type": ct });
  res.end(fs.readFileSync(filePath));
});
await new Promise((resolve) => server.listen(port, resolve));

let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof startPlay === "function" && typeof setNodeLogicCallbackGrid === "function");
  await assertSimpleDrawMode(page, false, "node logic callback");
  await clearSchedulerMismatches(page);

  await page.evaluate(() => {
    if (typeof resetAudioDebug === "function") resetAudioDebug();
    addNode?.(0, 0, "regular");
    addEdgeGrid?.(0, 0, 0, 0);
    setOrigin?.(0, 0, 0);
    updateBeatInfosJS?.();
    setBPM?.(180);
    setNodeLogicCallbackGrid?.(0, 0, "param_boost");
    window.__logicMark1 = Date.now();
    startPlay?.();
  });

  await page.waitForTimeout(250);

  const boost = await page.evaluate(() => {
    const mark = window.__logicMark1 || 0;
    const entries = (getAudioDebug?.() ?? []).filter((e) => e && e.tag === "play.params.enqueue" && e.t >= mark);
    const hit = entries.find((e) => Math.abs(e.pitch - 3) < 0.05 && Math.abs(e.dur - 2) < 0.05 && e.vol <= 0.6);
    return { count: entries.length, hit: !!hit, sample: hit || null };
  });
  if (!boost.hit) {
    throw new Error(`node logic callback: expected param_boost audio event, got ${boost.count} entries sample=${JSON.stringify(boost.sample)}`);
  }

  await page.evaluate(() => {
    setNodeLogicCallbackGrid?.(0, 0, "param_drop");
    window.__logicMark2 = Date.now();
  });

  await page.waitForTimeout(250);

  const drop = await page.evaluate(() => {
    const mark = window.__logicMark2 || 0;
    const entries = (getAudioDebug?.() ?? []).filter((e) => e && e.tag === "play.params.enqueue" && e.t >= mark);
    const hit = entries.find((e) => Math.abs(e.pitch + 2) < 0.05 && Math.abs(e.dur - 0.5) < 0.05 && e.vol <= 0.3);
    return { count: entries.length, hit: !!hit, sample: hit || null };
  });
  if (!drop.hit) {
    throw new Error(`node logic callback: expected param_drop audio event, got ${drop.count} entries sample=${JSON.stringify(drop.sample)}`);
  }

  await page.evaluate(() => {
    setNodeLogicCallbackGrid?.(0, 0, "disable_even");
    if (typeof updateBeatInfosJS === "function") updateBeatInfosJS();
    if (typeof ensure === "function") ensure(8);
    window.__logicDisablePattern = predictorAudibleSnapshot?.(0, 0, 6) ?? [];
  });

  const disablePattern = await page.evaluate(() => window.__logicDisablePattern ?? []);
  const expectedDisable = [1, 0, 1, 0, 1, 0];
  if (disablePattern.length !== expectedDisable.length || disablePattern.some((v, i) => v !== expectedDisable[i])) {
    throw new Error(`node logic callback: disable_even predictor mismatch got=${JSON.stringify(disablePattern)} want=${JSON.stringify(expectedDisable)}`);
  }

  const collectEnqueues = async (kind, waitMs) => {
    await page.evaluate((k) => {
      setNodeLogicCallbackGrid?.(0, 0, k);
      if (typeof updateBeatInfosJS === "function") updateBeatInfosJS();
      if (typeof resetAudioDebug === "function") resetAudioDebug();
      window.__logicAudioMark = Date.now();
    }, kind);
    await page.waitForTimeout(waitMs);
    return await page.evaluate(() => {
      const mark = window.__logicAudioMark || 0;
      const entries = (getAudioDebug?.() ?? []).filter((e) => e && e.tag === "play.params.enqueue" && e.t >= mark);
      return entries.length;
    });
  };

  let waitMs = 300;
  let baselineCount = await collectEnqueues("none", waitMs);
  if (baselineCount < 4) {
    waitMs = 600;
    baselineCount = await collectEnqueues("none", waitMs);
  }
  if (baselineCount < 4) {
    throw new Error(`node logic callback: insufficient baseline audio events (${baselineCount})`);
  }
  const disabledCount = await collectEnqueues("disable_even", waitMs);
  if (disabledCount >= baselineCount) {
    throw new Error(`node logic callback: disable_even did not reduce events (baseline=${baselineCount}, disabled=${disabledCount})`);
  }
  const maxAllowed = Math.ceil(baselineCount * 0.7);
  if (disabledCount > maxAllowed) {
    throw new Error(`node logic callback: disable_even reduction too small (baseline=${baselineCount}, disabled=${disabledCount}, maxAllowed=${maxAllowed})`);
  }

  await assertNoSchedulerMismatches(page, "node logic callback: scheduler mismatches");
  console.log("node_logic_callback.browser.test: PASS", { boost: boost.sample, drop: drop.sample });
} finally {
  if (browser) await browser.close();
  server.close();
}
