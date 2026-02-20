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

// Build WASM
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(GO, ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."], { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
if (build.status !== 0) throw new Error("go build wasm failed");
}

// Serve assets
const server = http.createServer((req, res) => { let f = req.url === "/" ? "/index.html" : req.url;
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
await assertSimpleDrawMode(page, false, "live edit sync");
await clearSchedulerMismatches(page);

// Build two loops and start playback
await page.evaluate(() => { // row0 rectangle 0,0..2,2
  addNode(0,0,'regular'); addNode(2,0,'regular'); addNode(2,2,'regular'); addNode(0,2,'regular');
  addEdgeGrid(0,0,2,0); addEdgeGrid(2,0,2,2); addEdgeGrid(2,2,0,2); addEdgeGrid(0,2,0,0);
  // row1 triangle at 6..7
  addNode(6,0,'regular'); addNode(7,0,'regular'); addNode(7,1,'regular');
  addEdgeGrid(6,0,7,0); addEdgeGrid(7,0,7,1); addEdgeGrid(7,1,6,0);
  // add row 1 and set origin
  // Re-use dropdown-free helpers: assume origin rows map by earliest node index
  // For tests it suffices to ensure a second circuit exists; the UI builds rows automatically on demand in wasm perf harnesses.
  setBPM(120); setFollow(false);
  startPlay();
});

// Let it run and freeze some past
await page.waitForTimeout(300);

// Helper to fetch state
async function snapshot(row) { const [idxs, off, len, steps] = await Promise.all([
    page.evaluate(() => nextBeatIdxs()),
    page.evaluate(() => drumOffset()),
    page.evaluate(() => drumLength()),
    page.evaluate(r => rowSteps(r), row)
  ]);
  return { idxs, off, len, steps };
}

function validateInWindow(state, row) { const last = Math.max(0, state.idxs[row] - 1);
  if (last < state.off || last >= state.off + state.len) throw new Error(`row ${row} last not in window`);
}

// futureAbs generator removed (was unused)

// Before edit validation: ensure window aligned for both rows
let s0 = await snapshot(0);
let s1 = await snapshot(1);
validateInWindow(s0, 0); validateInWindow(s1, 1);
const beforeState = await page.evaluate(() => dumpRowState ? dumpRowState(0) : null);

// Live edit: add a detour B->X->Y->C ahead of playhead in row 0
await page.evaluate(() => { addNode(3,0,'regular'); addNode(3,1,'regular');
  deleteEdgeGrid(2,0,2,2);
  addEdgeGrid(2,0,3,0); addEdgeGrid(3,0,3,1); addEdgeGrid(3,1,2,2);
  updateBeatInfosJS();
});

// Validate future for 1 beat matches predictor
const subdiv = await page.evaluate(() => gridSubdiv());
const beatsAhead = 1;
await page.evaluate(async ({beats, subdiv}) => { function sleep(ms){ return new Promise(r=>setTimeout(r,ms)); }
  const idxs = nextBeatIdxs();
  const row = 0;
  const start = Math.max(0, idxs[row]);
  const end = start + beats * subdiv;
  for (let a = start; a < end; a++) { ensure(a+1);
    const want = visibleAt(row, a);
    // Wait until UI has advanced to at least 'a' (or a+1 highlighted)
    const until = performance.now() + 200;
    while (performance.now() < until) { const cur = nextBeatIdxs()[row] - 1;
      if (cur >= a) break; await sleep(4);
    }
    const steps = rowSteps(row);
    const j = a - drumOffset();
    if (j >= 0 && j < steps.length) { const got = !!steps[j];
      if (got !== !!want) throw new Error(`future mismatch at abs=${a} got=${got} want=${want}`);
    }
  }
}, {beats: beatsAhead, subdiv});

const afterState = await page.evaluate(() => dumpRowState ? dumpRowState(0) : null);
if (!beforeState || !afterState) { throw new Error('row state dumps unavailable');
}
const freeze = Math.min(beforeState.frozenUpTo, afterState.frozenUpTo);
const offsetBefore = beforeState.timelineOffset ?? 0;
const pastBefore = beforeState.timelinePast ?? [];
const maskBefore = beforeState.timelinePastMask ?? [];
const offsetAfter = afterState.timelineOffset ?? 0;
const pastAfter = afterState.timelinePast ?? [];
const maskAfter = afterState.timelinePastMask ?? [];
for (let idx = 0; idx < maskBefore.length; idx++) { if (!maskBefore[idx]) continue;
  const abs = offsetBefore + idx;
  if (abs > freeze) continue;
  const relAfter = abs - offsetAfter;
  if (relAfter < 0 || relAfter >= maskAfter.length || !maskAfter[relAfter]) { console.error("debug timeline mismatch", { abs,
      relAfter,
      freeze,
      offsetBefore,
      offsetAfter,
      maskBefore,
      maskAfter,
      beforeState,
      afterState,
    });
    throw new Error(`committed past entry at abs=${abs} missing after edit`);
  }
  const beforeVal = !!pastBefore[idx];
  const afterVal = !!pastAfter[relAfter];
  if (beforeVal !== afterVal) { throw new Error(`committed past entry changed at abs=${abs}: before=${beforeVal} after=${afterVal}`);
  }
}

await assertNoSchedulerMismatches(page, "live edit sync: scheduler mismatches");
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "live_edit_sync");
await browser.close();
await new Promise((resolve) => server.close(resolve));
