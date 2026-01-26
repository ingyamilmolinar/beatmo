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

const port = 8390 + Math.floor(Math.random() * 1000);
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
  await page.waitForFunction(() => typeof addNode === "function");
  await assertSimpleDrawMode(page, false, "circuit sync");
  await clearSchedulerMismatches(page);

  await page.evaluate(() => { const ensureNode = (i, j, type) => { const existing = nodeIdAt?.(i, j);
      if (existing == null || existing < 0) { addNode?.(i, j, type);
      }
    };
    ensureNode(0, 0, "regular");
    ensureNode(1, 0, "regular");
    ensureNode(2, 0, "regular");
    deleteEdgeGrid?.(0, 0, 1, 0);
    deleteEdgeGrid?.(1, 0, 2, 0);
    deleteEdgeGrid?.(2, 0, 0, 0);
    addEdgeGrid?.(0, 0, 1, 0);
    addEdgeGrid?.(1, 0, 2, 0);
    addEdgeGrid?.(2, 0, 0, 0);
    setOrigin?.(0, 0, 0);
    updateBeatInfosJS?.();
    forceDraw?.();
    forceDraw?.();
  });

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
} finally { if (browser) await browser.close();
  server.close();
}
