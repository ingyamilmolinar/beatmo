import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(GO, ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."], { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
if (build.status !== 0) throw new Error("go build wasm failed");
}

const server = http.createServer((req, res) => { const f = req.url === "/" ? "/index.html" : req.url;
  const fp = path.join(jsDir, f.replace(/^\//, ""));
  if (!fs.existsSync(fp)) { res.writeHead(404); res.end(); return; }
  const ct = fp.endsWith(".html") ? "text/html" : fp.endsWith(".js") ? "application/javascript" : fp.endsWith(".wasm") ? "application/wasm" : "text/plain";
  res.writeHead(200, { "Content-Type": ct });
  res.end(fs.readFileSync(fp));
});
await new Promise(r => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, false, "prob edit sync");
await clearSchedulerMismatches(page);

// Simple 1-beat loop with two regular nodes and one probability node; toggle probability while playing.
await page.evaluate(() => { addNode(0,0,'regular'); addNode(1,0,'regular'); addNode(2,0,'regular');
  addEdgeGrid(0,0,1,0); addEdgeGrid(1,0,2,0); addEdgeGrid(2,0,0,0);
  setNodeLogicGrid(1,0,'probability', 0, 0.3);
  setBPM(180); setFollow(false); startPlay();
});

await page.waitForTimeout(200);

// Capture snapshot at current index and check 1 beat into future alignment after toggling probability to 1.0
const subdiv = await page.evaluate(() => gridSubdiv());
await page.evaluate(async (subdiv) => { function sleep(ms){ return new Promise(r=>setTimeout(r,ms)); }
  const row = 0;
  const idxs0 = nextBeatIdxs();
  const start = Math.max(0, idxs0[row]);
  setNodeLogicGrid(1,0,'probability', 0, 1.0);
  // validate future for 1 beat
  const end = start + subdiv;
  for (let a = start; a < end; a++) { ensure(a+1);
    const want = visibleAt(row, a);
    const until = performance.now() + 200;
    while ((nextBeatIdxs()[row]-1) < a && performance.now() < until) { await sleep(4); }
    const j = a - drumOffset();
    const steps = rowSteps(row);
    if (j >= 0 && j < steps.length) { const got = !!steps[j];
      if (got !== !!want) throw new Error(`prob future mismatch at abs=${a} got=${got} want=${want}`);
    }
  }
}, subdiv);

await assertNoSchedulerMismatches(page, "prob edit sync: scheduler mismatches");
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "prob_edit_sync");
await browser.close();
server.close();
