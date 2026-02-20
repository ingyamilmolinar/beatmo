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
  await page.waitForFunction(() => typeof addNode === "function");
  await assertSimpleDrawMode(page, false, "circuit sync");
  await clearSchedulerMismatches(page);

  // Import a clean 3-node circuit to isolate from the startup demo.
  // Without this, the demo's node at (0,0) retains existing edges that
  // cause the beat path traversal to follow the demo circuit instead of
  // the test's loop — especially under CPU pressure from parallel tests.
  await page.evaluate(() => {
    importJSON?.(JSON.stringify({
      version: 1, subdiv: 8, bpm: 120,
      instruments: [{ name: "Kick", id: "kick", kind: "builtin", volume: 1, origin: 0, color: "#C87850FF" }],
      nodes: [
        { id: 0, i: 0, j: 0, type: "regular", inputs: [2], outputs: [1] },
        { id: 1, i: 1, j: 0, type: "regular", inputs: [0], outputs: [2] },
        { id: 2, i: 2, j: 0, type: "regular", inputs: [1], outputs: [0] }
      ]
    }));
    forceDraw?.();
    forceDraw?.();
  });
  await page.waitForTimeout(200);

  const snapshot = async (gridI, gridJ) => { return await page.evaluate(({ gridI, gridJ }) => { const targetId = nodeIdAt?.(gridI, gridJ);
      const state = dumpRowState?.(0);
      const steps = Array.from(rowWindow?.(0) ?? []);
      const offset = state?.timelineOffset ?? 0;
      const hits = [];
      if (targetId != null && targetId >= 0 && state) { for (let i = 0; i < steps.length; i++) { const abs = offset + i;
          const info = beatInfoAt?.(0, abs);
          if (info && info.nodeId === targetId) { hits.push({ index: i, abs, step: !!steps[i], visible: !!visibleAt?.(0, abs), triggered: !!triggeredAt?.(0, abs) });
          }
        }
      }
      return { targetId, offset, steps, hits };
    }, { gridI, gridJ });
  };

  const before = await snapshot(1, 0);
  if (before.targetId == null || before.targetId < 0) { throw new Error("circuit sync: expected middle node before delete");
  }
  if (!before.hits.length || !before.hits.some((h) => h.step)) { throw new Error("circuit sync: expected active step for middle node before delete");
  }
  const anchorIndex = before.hits[0].index;

  await page.evaluate(() => { deleteNodeGrid?.(1, 0);
    forceDraw?.();
  });
  await page.waitForTimeout(50);

  const afterDelete = await snapshot(1, 0);
  if (afterDelete.targetId !== -1 && afterDelete.targetId !== null) { throw new Error(`circuit sync: unexpected node persisted after delete (id=${afterDelete.targetId})`);
  }
  if (anchorIndex < afterDelete.steps.length && afterDelete.steps[anchorIndex]) { throw new Error("circuit sync: step remained active after deleting node");
  }

  await page.evaluate(() => { addNode?.(1, 0, "regular");
    deleteEdgeGrid?.(0, 0, 2, 0);
    addEdgeGrid?.(0, 0, 1, 0);
    addEdgeGrid?.(1, 0, 2, 0);
    addEdgeGrid?.(2, 0, 0, 0);
    forceDraw?.();
  });
  await page.waitForTimeout(50);

  const afterReadd = await snapshot(1, 0);
  if (afterReadd.targetId == null || afterReadd.targetId < 0) { throw new Error("circuit sync: re-added node not found at grid (1,0)");
  }
  if (!afterReadd.hits.length || !afterReadd.hits.some((h) => h.step)) { throw new Error("circuit sync: expected active steps after re-adding node");
  }
  for (const hit of afterReadd.hits) { if (hit.step !== hit.visible) { throw new Error(`circuit sync: stale visible mismatch at abs=${hit.abs}`);
    }
  }
  await assertNoSchedulerMismatches(page, "circuit sync: scheduler mismatches");
  console.log("circuit_sync.browser.test: PASS", { before: before.hits.length, after: afterReadd.hits.length });
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "circuit_sync");
} finally { 
 if (browser) await browser.close();
  server.close();
}
