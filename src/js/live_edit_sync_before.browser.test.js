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

const port = 8380 + Math.floor(Math.random() * 1000);
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
await assertSimpleDrawMode(page, false, "live edit sync (before)");

await page.evaluate(() => { setFollow?.(true);
  // Build a 1x1 rectangle loop at (0,0)-(1,1)
  const a = addNode?.(0,0,'regular');
  addNode?.(1,0,'regular');
  addNode?.(1,1,'regular');
  addNode?.(0,1,'regular');
  addEdgeGrid?.(0,0,1,0);
  addEdgeGrid?.(1,0,1,1);
  addEdgeGrid?.(1,1,0,1);
  addEdgeGrid?.(0,1,0,0);
  setOrigin?.(0,0,0);
  setBPM?.(200);
  forceDraw?.();
});

await clearSchedulerMismatches(page);
await page.evaluate(() => startPlay?.());
// Let it run briefly
await page.waitForTimeout(600);

// Capture current indices
let before = await page.evaluate(() => ({ off: drumOffset?.(),
  nba: nextBeatIdxs?.(),
  metrics: getAudioScheduleMetrics?.(),
}));

// Live edit in the future: add detour b(1,0)->(2,0)->(2,1)->c(1,1)
await page.evaluate(() => { addNode?.(2,0,'regular');
  addNode?.(2,1,'regular');
  deleteEdgeGrid?.(1,0,1,1);
  addEdgeGrid?.(1,0,2,0);
  addEdgeGrid?.(2,0,2,1);
  addEdgeGrid?.(2,1,1,1);
  updateBeatInfosJS?.();
});

await page.waitForTimeout(600);

let after = await page.evaluate(() => ({ off: drumOffset?.(),
  nba: nextBeatIdxs?.(),
  rowsDrawn: rowsContentVisibleCount?.(),
  vis: visibleRows?.(),
  uiOk: uiLayoutOk?.(),
  metrics: getAudioScheduleMetrics?.(),
}));

await assertNoSchedulerMismatches(page, "live edit sync (before): scheduler mismatches");
await browser.close();
server.close();

if (!after || !before) throw new Error('missing metrics');
if (!(after.nba && before.nba) || after.nba[0] <= before.nba[0]) { throw new Error(`nextBeatIdx did not advance: ${before.nba} -> ${after.nba}`);
}
if ((after.rowsDrawn ?? 0) < Math.min(after.vis ?? 0, 1) && !after.uiOk) { throw new Error(`UI not fully drawn after edit (rowsDrawn=${after.rowsDrawn} vis=${after.vis} uiOk=${after.uiOk})`);
}
if (!after.metrics || after.metrics.count <= before.metrics.count) { throw new Error('audio schedule metrics did not advance after edit');
}
