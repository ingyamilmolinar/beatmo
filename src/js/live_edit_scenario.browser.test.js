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

const wasmBuild = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (wasmBuild.status !== 0) throw new Error("go build main wasm failed");

const tunkulFixture = fs.readFileSync(path.resolve(jsDir, "../../tunkul.json"), "utf8");

const port = 8420 + Math.floor(Math.random() * 400);
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
try { const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof importJSON === "function");
  await assertSimpleDrawMode(page, false, "live edit scenario");

  await page.evaluate((json) => { setFollow?.(false);
    importJSON(json);
    forceDraw?.();
    resumeAudio?.();
  }, tunkulFixture);

await page.waitForFunction(() => typeof startPlay === "function");
await page.waitForFunction(() => typeof dumpRowState === "function");

  await clearSchedulerMismatches(page);
  await page.evaluate(() => { window.__liveEditCtx = {};
    startPlay();
  });

  await page.waitForTimeout(400);
  await page.evaluate(() => forceDraw?.());

  const scenarios = [
    "detour-add",
    "detour-restore",
    "logic-probability",
    "logic-none",
    "node-remove-readd",
    "logic-skip",
    "logic-reset",
    "logic-immediate",
    "logic-immediate-reset",
    "logic-past",
    "logic-past-reset",
  ];

  for (const label of scenarios) { const summary = await page.evaluate(async ({ label }) => { const rowCount = typeof totalRows === "function" ? totalRows() : 0;
      const rows = rowCount > 0 ? Array.from({ length: rowCount }, (_, idx) => idx) : [0];
      const capture = (row) => { const state = typeof dumpRowState === "function" ? dumpRowState(row) : null;
        const offset = typeof drumOffset === "function" ? drumOffset() : 0;
        const window = typeof rowWindow === "function" ? rowWindow(row) : [];
        const map = {};
        if (Array.isArray(window)) { for (let i = 0; i < window.length; i++) { map[offset + i] = !!window[i];
          }
        }
        const freezeVal = state && typeof state.frozenUpTo === "number" ? state.frozenUpTo : state && typeof state.FrozenUpTo === "number" ? state.FrozenUpTo : -1;
        const nextVal = state && typeof state.nextBeatIdx === "number" ? state.nextBeatIdx : state && typeof state.NextBeatIdx === "number" ? state.NextBeatIdx : 0;
        const toArray = (arr) => (Array.isArray(arr) ? Array.from(arr) : []);
        const timeline = state
          ? { offset: state.timelineOffset ?? offset,
              past: toArray(state.timelinePast),
              pastMask: toArray(state.timelinePastMask),
              present: toArray(state.timelinePresent),
              future: toArray(state.timelineFuture),
            }
          : { offset, past: [], pastMask: [], present: [], future: [] };
        const committed = {};
        if (timeline.pastMask) { for (let i = 0; i < timeline.pastMask.length; i++) { if (!timeline.pastMask[i]) continue;
            committed[timeline.offset + i] = !!timeline.past[i];
          }
        }
        return { offset,
          next: nextVal,
          freeze: freezeVal,
          window: map,
          committed,
          timeline,
        };
      };

      const ensureFuture = (need) => (typeof ensure === "function" ? ensure(need) : undefined);

      const waitForFutureAlignment = async (row, scenarioLabel) => { const subdiv = typeof gridSubdiv === "function" ? gridSubdiv() : 32;
        const deadline = performance.now() + 1200;
        let attempts = 0;
        let lastDiff = null;
        while (performance.now() < deadline) { const snap = capture(row);
          const next = snap.next ?? 0;
          const horizon = next + subdiv;
          let mismatch = false;
          const diffs = [];
          for (let abs = next; abs < horizon; abs++) { if (!(abs in snap.window)) continue;
            const relVal = !!snap.window[abs];
            const pred = typeof visibleAt === "function" ? !!visibleAt(row, abs) : relVal;
            if (relVal !== pred) { mismatch = true;
              const info = typeof beatInfoAt === "function" ? beatInfoAt(row, abs) : null;
              diffs.push({ abs, relVal, pred, info });
            }
          }
          if (!mismatch) { return snap;
          }
          lastDiff = { attempts, next, horizon, diffs, offset: snap.offset, freeze: snap.freeze };
          ensureFuture(next + subdiv * 2);
          updateBeatInfosJS?.();
          forceDraw?.();
          await new Promise((resolve) => setTimeout(resolve, 50));
          attempts++;
        }
        throw new Error(`future alignment failed for row ${row} during ${scenarioLabel}: ${JSON.stringify(lastDiff)}`);
      };

      const mutate = async (row, label) => { const subdiv = typeof gridSubdiv === "function" ? gridSubdiv() : 32;
        const idxs = typeof nextBeatIdxs === "function" ? nextBeatIdxs() : [];
        const next = Array.isArray(idxs) && row < idxs.length ? idxs[row] : 0;
        const findPair = (start) => { let prev = null;
          let prevAbs = -1;
          for (let abs = start; abs < start + 512; abs++) { const info = typeof beatInfoAt === "function" ? beatInfoAt(row, abs) : null;
            if (!info || info.nodeId === undefined || info.nodeId < 0) continue;
            if (info.type !== "regular") continue;
            if (prev && info.nodeId !== prev.nodeId) { return { from: prev, to: info, absFrom: prevAbs, absTo: abs };
            }
            prev = info;
            prevAbs = abs;
          }
          throw new Error("future pair not found");
        };

        const ensureButtonClick = (x, y) => { const canvas = document.querySelector("canvas");
          if (!canvas) return;
          const evts = [
            new PointerEvent("pointerdown", { clientX: x, clientY: y, button: 0, bubbles: true }),
            new MouseEvent("mousedown", { clientX: x, clientY: y, button: 0, bubbles: true }),
            new PointerEvent("pointerup", { clientX: x, clientY: y, button: 0, bubbles: true }),
            new MouseEvent("mouseup", { clientX: x, clientY: y, button: 0, bubbles: true }),
          ];
          for (const evt of evts) canvas.dispatchEvent(evt);
        };

        const ctx = window.__liveEditCtx || (window.__liveEditCtx = {});
        const gatherSuccessors = (rowIdx, absTo) => { const list = [];
          const limit = (typeof gridSubdiv === "function" ? gridSubdiv() : 32) * 4;
          for (let delta = 1; delta < limit; delta++) { const info = typeof beatInfoAt === "function" ? beatInfoAt(rowIdx, absTo + delta) : null;
            if (!info || info.nodeId === undefined || info.nodeId < 0) { break;
            }
            if (info.type !== "regular") { continue;
            }
            if (!list.some((s) => s.i === info.i && s.j === info.j)) { list.push({ i: info.i, j: info.j, nodeId: info.nodeId, abs: absTo + delta });
            }
            break;
          }
          return list;
        };
        const normalizeSuccessors = (raw, absBase) => { if (!Array.isArray(raw)) { return [];
          }
          const out = [];
          for (const item of raw) { if (!item) continue;
            const iVal = Number(item.i);
            const jVal = Number(item.j);
            if (!Number.isFinite(iVal) || !Number.isFinite(jVal)) continue;
            let nodeId = item.nodeId;
            if (typeof nodeId !== "number") { if (typeof item.nodeID === "number") { nodeId = item.nodeID;
              } else if (typeof item.nodeid === "number") { nodeId = item.nodeid;
              }
            }
            const absVal = typeof item.abs === "number" ? item.abs : (typeof absBase === "number" ? absBase + 1 : -1);
            out.push({ i: iVal, j: jVal, nodeId: typeof nodeId === "number" ? nodeId : -1, abs: absVal });
          }
          return out;
        };
        const resolveSuccessors = (rowIdx, absTo, toInfo) => { if (toInfo && typeof nodeSuccessorsGrid === "function") { const raw = nodeSuccessorsGrid(toInfo.i, toInfo.j);
            const candidates = normalizeSuccessors(raw, absTo);
            if (candidates.length) { return candidates;
            }
          }
          return gatherSuccessors(rowIdx, absTo);
        };
        const findRegularForward = (rowIdx, startAbs) => { const limit = (typeof gridSubdiv === "function" ? gridSubdiv() : 32) * 4;
          for (let delta = 0; delta < limit; delta++) { const info = typeof beatInfoAt === "function" ? beatInfoAt(rowIdx, startAbs + delta) : null;
            if (!info || info.nodeId === undefined || info.nodeId < 0) continue;
            if (info.type !== "regular") continue;
            return { info, abs: startAbs + delta };
          }
          return null;
        };
        const findRegularBackward = (rowIdx, startAbs, offsetBase) => { const limit = (typeof gridSubdiv === "function" ? gridSubdiv() : 32) * 4;
          for (let delta = 0; delta < limit; delta++) { const abs = startAbs - delta;
            if (abs < offsetBase) break;
            const info = typeof beatInfoAt === "function" ? beatInfoAt(rowIdx, abs) : null;
            if (!info || info.nodeId === undefined || info.nodeId < 0) continue;
            if (info.type !== "regular") continue;
            return { info, abs };
          }
          return null;
        };
        const ensureContext = () => { if (!ctx.detour) { const basePair = findPair(next + subdiv);
            const successors = resolveSuccessors(row, basePair.absTo, basePair.to);
            ctx.detour = { row,
              from: basePair.from,
              to: basePair.to,
              absFrom: basePair.absFrom,
              absTo: basePair.absTo,
              successors,
            };
            if (!ctx.detour.next) { ctx.detour.next = successors.length
                ? { i: successors[0].i, j: successors[0].j, nodeId: successors[0].nodeId }
                : (typeof beatInfoAt === "function" ? beatInfoAt(row, basePair.absTo + 1) : null);
            }
          } else if (!ctx.detour.successors || ctx.detour.successors.length === 0) { const successors = resolveSuccessors(row, ctx.detour.absTo ?? (next + subdiv), ctx.detour.to ?? null);
            ctx.detour.successors = successors;
            if (successors.length) { ctx.detour.next = { i: successors[0].i, j: successors[0].j, nodeId: successors[0].nodeId };
            }
          }
          return ctx.detour;
        };

        switch (label) { case "detour-add": { const pair = findPair(next + subdiv);
            const dir = Math.sign(pair.to.i - pair.from.i) || 1;
            const mid1 = { i: pair.from.i + dir, j: pair.from.j };
            while (typeof nodeIdAt === "function" && nodeIdAt(mid1.i, mid1.j) !== -1) { mid1.i += dir;
            }
            const mid2 = { i: mid1.i, j: pair.from.j + 1 };
            while (typeof nodeIdAt === "function" && nodeIdAt(mid2.i, mid2.j) !== -1) { mid2.j += 1;
            }
            if (typeof addNode !== "function" || typeof addEdgeGrid !== "function") break;
            addNode(mid1.i, mid1.j, "regular");
            addNode(mid2.i, mid2.j, "regular");
            deleteEdgeGrid?.(pair.from.i, pair.from.j, pair.to.i, pair.to.j);
            addEdgeGrid(pair.from.i, pair.from.j, mid1.i, mid1.j);
            addEdgeGrid(mid1.i, mid1.j, mid2.i, mid2.j);
            addEdgeGrid(mid2.i, mid2.j, pair.to.i, pair.to.j);
            ctx.detour = { row,
              from: pair.from,
              to: pair.to,
              mid1,
              mid2,
              next: typeof beatInfoAt === "function" ? beatInfoAt(row, pair.absTo + 1) : null,
              absFrom: pair.absFrom,
              absTo: pair.absTo,
            };
            break;
          }
          case "detour-restore": { const info = ensureContext();
            if (info && typeof deleteEdgeGrid === "function") { deleteEdgeGrid(info.from.i, info.from.j, info.mid1.i, info.mid1.j);
              deleteEdgeGrid(info.mid1.i, info.mid1.j, info.mid2.i, info.mid2.j);
              deleteEdgeGrid(info.mid2.i, info.mid2.j, info.to.i, info.to.j);
              deleteNodeGrid?.(info.mid1.i, info.mid1.j);
              deleteNodeGrid?.(info.mid2.i, info.mid2.j);
              addEdgeGrid?.(info.from.i, info.from.j, info.to.i, info.to.j);
            }
            break;
          }
          case "logic-probability": { const info = ensureContext();
            setNodeLogicGrid?.(info.to.i, info.to.j, "probability", 0, 0.35);
            break;
          }
          case "logic-none": { const info = ensureContext();
            setNodeLogicGrid?.(info.to.i, info.to.j, "none", 0, 0);
            break;
          }
          case "node-remove-readd": { const info = ensureContext();
            const successors = resolveSuccessors(row, info.absTo ?? (next + subdiv), info.to);
            deleteEdgeGrid?.(info.from.i, info.from.j, info.to.i, info.to.j);
            for (const succ of successors) { deleteEdgeGrid?.(info.to.i, info.to.j, succ.i, succ.j);
            }
            deleteNodeGrid?.(info.to.i, info.to.j);
            addNode?.(info.to.i, info.to.j, "regular");
            addEdgeGrid?.(info.from.i, info.from.j, info.to.i, info.to.j);
            for (const succ of successors) { addEdgeGrid?.(info.to.i, info.to.j, succ.i, succ.j);
            }
            const newId = typeof nodeIdAt === "function" ? nodeIdAt(info.to.i, info.to.j) : -1;
            ctx.detour.to = { nodeId: newId,
              i: info.to.i,
              j: info.to.j,
              type: "regular",
            };
            ctx.detour.successors = successors;
            ctx.detour.next = successors.length ? { i: successors[0].i, j: successors[0].j, nodeId: successors[0].nodeId } : null;
            break;
          }
          case "logic-skip": { const info = ensureContext();
            setNodeLogicGrid?.(info.to.i, info.to.j, "skip_every_n", 3, 0);
            break;
          }
          case "logic-reset": { const info = ensureContext();
            setNodeLogicGrid?.(info.to.i, info.to.j, "none", 0, 0);
            break;
          }
          case "logic-immediate": { const snap = capture(row);
            const targetAbs = snap.next ?? next;
            const found = findRegularForward(row, targetAbs);
            if (!found) { throw new Error("immediate target not found");
            }
            setNodeLogicGrid?.(found.info.i, found.info.j, "skip_every_n", 2, 0);
            break;
          }
          case "logic-immediate-reset": { const snap = capture(row);
            const targetAbs = snap.next ?? next;
            const found = findRegularForward(row, targetAbs);
            if (!found) { throw new Error("immediate target not found");
            }
            setNodeLogicGrid?.(found.info.i, found.info.j, "none", 0, 0);
            break;
          }
          case "logic-past": { const snap = capture(row);
            let targetAbs = snap.freeze;
            if (targetAbs === undefined || targetAbs === null || targetAbs < 0) { targetAbs = (snap.next ?? next) - 1;
            }
            const found = findRegularBackward(row, targetAbs, snap.offset ?? 0);
            if (!found) { throw new Error("past target not found");
            }
            setNodeLogicGrid?.(found.info.i, found.info.j, "probability", 0, 0.5);
            break;
          }
          case "logic-past-reset": { const snap = capture(row);
            let targetAbs = snap.freeze;
            if (targetAbs === undefined || targetAbs === null || targetAbs < 0) { targetAbs = (snap.next ?? next) - 1;
            }
            const found = findRegularBackward(row, targetAbs, snap.offset ?? 0);
            if (!found) { throw new Error("past target not found");
            }
            setNodeLogicGrid?.(found.info.i, found.info.j, "none", 0, 0);
            break;
          }
          default:
            break;
        }

        updateBeatInfosJS?.();
        ensureFuture(next + subdiv * 2);
        forceDraw?.();
      };

      const compare = (before, after, row) => { const subdiv = typeof gridSubdiv === "function" ? gridSubdiv() : 32;
        const next = after.next ?? 0;
        const freeze = Math.min(before.freeze ?? -1, after.freeze ?? -1);
        const pastDiffs = [];
        const futureDiffs = [];
        const timelineDiffs = [];

        const beforeTimeline = before.timeline || { offset: before.offset, past: [], pastMask: [], present: [], future: [] };
        const afterTimeline = after.timeline || { offset: after.offset, past: [], pastMask: [], present: [], future: [] };
        if (beforeTimeline.offset !== afterTimeline.offset) { timelineDiffs.push({ type: "offset", before: beforeTimeline.offset, after: afterTimeline.offset });
        }

        const beforeKeys = Object.keys(before.window).map(Number);
        for (const abs of beforeKeys) { if (abs <= freeze) { const a = after.window[abs];
            if (a !== before.window[abs]) { pastDiffs.push({ abs, before: before.window[abs], after: a });
            }
          }
        }
        const committedDiffs = [];
        for (const [absStr, beforeVal] of Object.entries(before.committed || {})) { const abs = Number(absStr);
          if (abs > freeze) continue;
          const cur = after.committed && Object.prototype.hasOwnProperty.call(after.committed, absStr) ? after.committed[absStr] : undefined;
          if (cur !== beforeVal) { committedDiffs.push({ abs, before: beforeVal, after: cur });
          }
        }
        const horizon = next + subdiv;
        for (let abs = next; abs < horizon; abs++) { const pred = typeof visibleAt === "function" ? !!visibleAt(row, abs) : after.window[abs];
          const idx = abs - afterTimeline.offset;
          if (idx >= 0 && idx < afterTimeline.present.length) { const segVal = afterTimeline.present[idx];
            if (segVal !== pred) { futureDiffs.push({ abs, segVal, pred });
            }
            continue;
          }
          if (idx >= 0 && idx < afterTimeline.future.length) { const segVal = afterTimeline.future[idx];
            if (segVal !== pred) { futureDiffs.push({ abs, segVal, pred });
            }
          }
        }
        return { freeze, next, pastDiffs, committedDiffs, futureDiffs, timelineDiffs };
      };

      const beforeSnapshots = rows.map((row) => ({ row, snap: capture(row) }));
      await mutate(0, label);
      const afterSnapshots = [];
      for (const row of rows) { const snap = await waitForFutureAlignment(row, label);
        afterSnapshots.push({ row, snap });
      }
      const comparisons = rows.map((row, idx) => { const before = beforeSnapshots[idx].snap;
        const after = afterSnapshots[idx].snap;
        return { row,
          ...compare(before, after, row),
        };
      });

      return { label,
        comparisons,
      };
    }, { label });

    console.log(`[SCENARIO] ${label}`, summary);
    for (const comp of summary.comparisons) { if (comp.pastDiffs.length) { throw new Error(`scenario ${label} row ${comp.row} past changed: ${JSON.stringify(comp.pastDiffs)}`);
      }
      if (comp.committedDiffs.length) { throw new Error(`scenario ${label} row ${comp.row} committed mismatch: ${JSON.stringify(comp.committedDiffs)}`);
      }
      if (comp.futureDiffs.length) { throw new Error(`scenario ${label} row ${comp.row} future mismatch: ${JSON.stringify(comp.futureDiffs)}`);
      }
      if (comp.timelineDiffs.length) { throw new Error(`scenario ${label} row ${comp.row} timeline mismatch: ${JSON.stringify(comp.timelineDiffs)}`);
      }
    }
  }
  await assertNoSchedulerMismatches(page, "live edit scenario: scheduler mismatches");
} finally { await browser.close();
  await new Promise((resolve) => server.close(resolve));
}
