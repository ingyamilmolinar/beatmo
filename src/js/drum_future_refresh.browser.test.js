import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const CHANGE_TIMEOUT_MS = Number(process.env.DRUM_EDIT_TIMEOUT_MS ?? "350");
const CYCLE_WAIT_MS = Number(process.env.DRUM_EDIT_CYCLE_MS ?? "220");

const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

const fixture = JSON.stringify({
  version: 1,
  subdiv: 8,
  bpm: 120,
  instruments: [
    { name: "Tom", id: "tom", kind: "builtin", volume: 1, origin: 0, color: "#7850C8FF" }
  ],
  nodes: [
    { id: 1, i: 0, j: 0, type: "regular", inputs: [4], outputs: [2] },
    { id: 2, i: 1, j: 0, type: "regular", inputs: [1], outputs: [3] },
    { id: 3, i: 2, j: 0, type: "regular", inputs: [2], outputs: [4] },
    { id: 4, i: 3, j: 0, type: "regular", inputs: [3], outputs: [1] }
  ]
});

const port = 8450 + Math.floor(Math.random() * 500);
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
await new Promise((resolve) => server.listen(port, resolve));

async function waitForCondition(page, predicate, args, timeoutMs, label) { const start = Date.now();
  while (true) { const ok = await page.evaluate(predicate, args);
    if (ok) { const elapsed = Date.now() - start;
      if (elapsed > timeoutMs) { throw new Error(`${label} exceeded ${timeoutMs}ms`);
      }
      return elapsed;
    }
    if (Date.now() - start > timeoutMs) { throw new Error(`${label} not met within ${timeoutMs}ms`);
    }
    await page.waitForTimeout(16);
  }
}

function pickFutureIndex(offset, window, nextAbs) { const start = Math.max(0, nextAbs - offset);
  for (let i = start; i < window.length; i++) { if (window[i]) return { rel: i, abs: offset + i };
  }
  return { rel: -1, abs: -1 };
}

function findNeighbor(row, startAbs, dir) { let abs = startAbs + dir;
  for (let iter = 0; iter < 256; iter++, abs += dir) { const info = beatInfoAt?.(row, abs);
    if (!info) break;
    if (info.type === "regular") return { abs, info };
  }
  return null;
}

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
try { const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof startPlay === "function");
  await assertSimpleDrawMode(page, false, "drum future refresh");

  await page.evaluate((json) => { setFollow?.(true);
    importJSON?.(json);
    updateBeatInfosJS?.();
    forceDraw?.();
    setBPM?.(120);
  }, fixture);

  await page.waitForFunction(() => typeof totalRows === "function" && totalRows() > 0);

const scenario = await page.evaluate(() => { const total = typeof totalRows === "function" ? totalRows() : 0;
  let rowIndex = -1;
  for (let i = 0; i < total; i++) { if (rowInstrument?.(i) === "tom") { rowIndex = i;
      break;
    }
  }
  if (rowIndex < 0) return null;
  const offset = drumOffset?.() ?? 0;
  const nexts = nextBeatIdxs?.() || [];
  const nextAbs = Array.isArray(nexts) && rowIndex < nexts.length ? nexts[rowIndex] : offset;
  const window = rowWindow?.(rowIndex) || [];
  let targetRel = -1;
  const start = Math.max(0, nextAbs - offset);
  for (let i = start; i < window.length; i++) { if (window[i]) { targetRel = i; break; }
  }
  if (targetRel < 0) return null;
  const targetAbs = offset + targetRel;
  const target = beatInfoAt?.(rowIndex, targetAbs);
  if (!target || target.type !== "regular") return null;
  const findNeighbor = (row, startAbs, dir) => { let abs = startAbs + dir;
    for (let iter = 0; iter < 256; iter++, abs += dir) { const info = beatInfoAt?.(row, abs);
      if (!info) break;
      if (info.type === "regular") return { abs, info };
    }
    return null;
  };
  const left = findNeighbor(rowIndex, targetAbs, -1);
  const right = findNeighbor(rowIndex, targetAbs, +1);
  if (!left || !right) return null;
  const metrics = getAudioScheduleMetrics?.();
  return { rowIndex,
    offset,
    targetRel,
    targetAbs,
    target,
    left: left.info,
    right: right.info,
    nextAbs,
    metrics
  };
});

  if (!scenario) { throw new Error("could not locate tom row future target");
  }

  const metricsBefore = scenario.metrics || { count: 0 };

  await page.evaluate(({ rowIndex, target }) => { deleteNodeGrid?.(target.i, target.j);
    updateBeatInfosJS?.();
  }, scenario);

  await waitForCondition(
    page,
    ({ rowIndex, targetRel }) => { const win = rowWindow?.(rowIndex) || [];
      return Array.isArray(win) && win.length > targetRel && win[targetRel] === false;
    },
    { rowIndex: scenario.rowIndex, targetRel: scenario.targetRel },
    CHANGE_TIMEOUT_MS,
    "future cell did not clear after delete"
  );

  const sameCycle = await page.evaluate(({ rowIndex, target, left, right, targetRel }) => { addNode?.(target.i, target.j, "regular");
    addEdgeGrid?.(left.i, left.j, target.i, target.j);
    addEdgeGrid?.(target.i, target.j, right.i, right.j);
    updateBeatInfosJS?.();
    forceDraw?.();
    const win = rowWindow?.(rowIndex) || [];
    return Array.isArray(win) && win.length > targetRel && win[targetRel] === true;
  }, scenario);

  if (!sameCycle) { throw new Error("future cell did not relight immediately after re-add");
  }

  await page.waitForTimeout(CYCLE_WAIT_MS);
  await page.evaluate(() => forceDraw?.());

const afterCycles = await page.evaluate(({ rowIndex, targetRel }) => { const win = rowWindow?.(rowIndex) || [];
  return Array.isArray(win) && win.length > targetRel && win[targetRel] === true;
}, { rowIndex: scenario.rowIndex, targetRel: scenario.targetRel });
if (!afterCycles) { throw new Error("future cell toggled off after waiting additional cycles");
}

const stateAfter = await page.evaluate(({ rowIndex }) => { const dump = typeof dumpRowState === "function" ? dumpRowState(rowIndex) : null;
  return dump
    ? { frozen: dump.frozenUpTo,
        timelineOffset: dump.timelineOffset ?? 0,
        timelinePast: dump.timelinePast ?? [],
        timelinePastMask: dump.timelinePastMask ?? [],
      }
    : null;
}, { rowIndex: scenario.rowIndex });
const targetAbs = scenario.targetAbs;
if (!stateAfter) { throw new Error("dumpRowState unavailable after edits");
}
const relAbs = targetAbs - stateAfter.timelineOffset;
if (relAbs >= 0 && relAbs < stateAfter.timelinePastMask.length && stateAfter.timelinePastMask[relAbs]) { throw new Error(`future commit persisted at abs=${targetAbs}`);
}

const metricsAfter = await page.evaluate(() => getAudioScheduleMetrics?.());
if (!metricsAfter) { throw new Error("audio schedule metrics unavailable after edits");
}
} finally { await browser.close();
  server.close();
}
