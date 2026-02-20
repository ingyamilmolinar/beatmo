import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  [
    "build",
    "-ldflags",
    "-X main.defaultLog=INFO",
    "-o",
    path.join(jsDir, "main.wasm"),
    "./cmd/...",
  ],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404);
      res.end();
      return;
    }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((resolve) => server.listen(0, resolve));
const port = server.address().port;

let browser;
try { browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function");
  await assertSimpleDrawMode(page, false, "logic sync");
  await clearSchedulerMismatches(page);

  // Import a clean 2-node circuit to isolate from the startup demo.
  // Without this, the demo's node at (0,0) retains existing edges that
  // cause the beat path traversal to follow the demo circuit instead of
  // the test's loop — especially under CPU pressure from parallel tests.
  await page.evaluate(() => {
    importJSON?.(JSON.stringify({
      version: 1, subdiv: 8, bpm: 120,
      instruments: [{ name: "Kick", id: "kick", kind: "builtin", volume: 1, origin: 0, color: "#C87850FF" }],
      nodes: [
        { id: 0, i: 0, j: 0, type: "regular", inputs: [1], outputs: [1] },
        { id: 1, i: 1, j: 0, type: "regular", inputs: [0], outputs: [0] }
      ]
    }));
    forceDraw?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const collect = async () => { return await page.evaluate(() => { const targetId = nodeIdAt?.(1, 0);
      const state = dumpRowState?.(0);
      const steps = Array.from(rowWindow?.(0) ?? []);
      const hits = [];
      if (targetId != null && targetId >= 0 && state) { const offset = state.timelineOffset ?? 0;
        for (let i = 0; i < steps.length; i++) { const abs = offset + i;
          const info = beatInfoAt?.(0, abs);
          if (info && info.nodeId === targetId) { const visible = visibleAt?.(0, abs);
            const triggered = triggeredAt?.(0, abs);
            hits.push({ index: i, abs, step: !!steps[i], visible: !!visible, triggered: !!triggered, type: info.type });
          }
        }
      }
      return { targetId, hits };
    });
  };

  const before = await collect();
  if (before.targetId == null || before.targetId < 0) { throw new Error("logic sync: target node missing before change");
  }
  if (!before.hits.length || !before.hits.some((h) => h.step)) { throw new Error("logic sync: expected active steps before logic change");
  }

  await page.evaluate(() => { setNodeLogicGrid?.(1, 0, "probability", 0, 0);
    forceDraw?.();
  });
  await page.waitForTimeout(50);

  const after = await collect();
  if (after.targetId !== before.targetId) { throw new Error(`logic sync: target node id mismatch ${before.targetId} -> ${after.targetId}`);
  }
  if (!after.hits.length) { throw new Error("logic sync: predictor hits missing after change");
  }
  for (const hit of after.hits) { if (hit.step || hit.triggered || hit.visible) { throw new Error(`logic sync: stale activity at abs=${hit.abs} index=${hit.index}`);
    }
  }
  await assertNoSchedulerMismatches(page, "logic sync: scheduler mismatches");
  console.log("logic_sync.browser.test: PASS", { before: before.hits.length, after: after.hits.length });
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "logic_sync");
} finally { 
 if (browser) await browser.close();
  server.close();
}
