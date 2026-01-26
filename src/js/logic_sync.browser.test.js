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

const port = 8380 + Math.floor(Math.random() * 1000);
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
await new Promise((resolve) => server.listen(port, resolve));

let browser;
try { browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function");
  await assertSimpleDrawMode(page, false, "logic sync");
  await clearSchedulerMismatches(page);

  await page.evaluate(() => { const startId = nodeIdAt?.(0, 0);
    const midId = nodeIdAt?.(1, 0);
    if (startId == null || startId < 0) { addNode?.(0, 0, "regular");
    }
    if (midId == null || midId < 0) { addNode?.(1, 0, "regular");
    }
    deleteEdgeGrid?.(0, 0, 1, 0);
    deleteEdgeGrid?.(1, 0, 0, 0);
    addEdgeGrid?.(0, 0, 1, 0);
    addEdgeGrid?.(1, 0, 0, 0);
    setOrigin?.(0, 0, 0);
    updateBeatInfosJS?.();
    forceDraw?.();
    forceDraw?.();
  });

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
} finally { if (browser) await browser.close();
  server.close();
}
