import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
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

const port = 8385 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => { const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, false, "live edit sync (mid)");

await page.evaluate(() => { setFollow?.(true);
  // Build a 1x1 rectangle loop at (0,0)-(1,1)
  addNode?.(0,0,'regular');
  addNode?.(1,0,'regular');
  addNode?.(1,1,'regular');
  addNode?.(0,1,'regular');
  addEdgeGrid?.(0,0,1,0);
  addEdgeGrid?.(1,0,1,1);
  addEdgeGrid?.(1,1,0,1);
  addEdgeGrid?.(0,1,0,0);
  setOrigin?.(0,0,0);
  setBPM?.(180);
  forceDraw?.();
});

await clearSchedulerMismatches(page);
await page.evaluate(() => startPlay?.());
await page.waitForTimeout(400);
const before = await page.evaluate(() => ({ off: drumOffset?.(), nba: nextBeatIdxs?.(), metrics: getAudioScheduleMetrics?.() }));

// Mid-beat edit: remove and immediately re-add the same edge/node detour
await page.evaluate(() => { // delete b(1,0)->c(1,1), then re-add in same frame
  deleteEdgeGrid?.(1,0,1,1);
  addEdgeGrid?.(1,0,1,1);
  updateBeatInfosJS?.();
});

await page.waitForTimeout(500);
const after = await page.evaluate(() => ({ off: drumOffset?.(), nba: nextBeatIdxs?.(), rowsDrawn: rowsContentVisibleCount?.(), vis: visibleRows?.(), uiOk: uiLayoutOk?.(), metrics: getAudioScheduleMetrics?.() }));

await assertNoSchedulerMismatches(page, "live edit sync (mid): scheduler mismatches");
await browser.close();
server.close();

if (!(after.nba && before.nba) || after.nba[0] <= before.nba[0]) { throw new Error(`nextBeatIdx did not advance after mid-beat edit: ${before.nba} -> ${after.nba}`);
}
if ((after.rowsDrawn ?? 0) < Math.min(after.vis ?? 0, 1) && !after.uiOk) { throw new Error(`UI not fully drawn after mid-beat edit: rowsDrawn=${after.rowsDrawn} vis=${after.vis} uiOk=${after.uiOk}`);
}
if (!after.metrics || after.metrics.count <= before.metrics.count) { throw new Error('audio schedule metrics did not advance after mid-beat edit');
}
