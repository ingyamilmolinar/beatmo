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

const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=ERROR", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build wasm failed");

const port = 8450 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => { const f = req.url === "/" ? "/index.html" : req.url;
  const fp = path.join(jsDir, f.replace(/^\//, ""));
  if (!fs.existsSync(fp)) { res.writeHead(404); res.end(); return; }
  const ct = fp.endsWith(".html") ? "text/html" : fp.endsWith(".js") ? "application/javascript" : fp.endsWith(".wasm") ? "application/wasm" : "text/plain";
  res.writeHead(200, { "Content-Type": ct });
  res.end(fs.readFileSync(fp));
});
await new Promise((resolve) => server.listen(port, resolve));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === "function");
await assertSimpleDrawMode(page, false, "mute logic");
await clearSchedulerMismatches(page);

const setup = await page.evaluate(() => { resetAudioDebug();
  setFollow(false);
  const startId = addNode(0, 5, "regular");
  const muteId = addNode(1, 5, "mute");
  const tailId = addNode(2, 5, "regular");
  addEdgeGrid(0, 5, 1, 5);
  addEdgeGrid(1, 5, 2, 5);
  addEdgeGrid(2, 5, 0, 5);
  setNodeLogicGrid(1, 5, "every_n_triggers", 2, 0);
  setOrigin(0, 0, 5);
  updateBeatInfosJS();
  const cycleLen = rowBeatCount(0);
  let muteIdx = -1;
  let tailIdx = -1;
  for (let i = 0; i < cycleLen; i++) { const info = beatInfoAt(0, i);
    if (!info) continue;
    if (info.nodeId === muteId) muteIdx = i;
    if (info.nodeId === tailId) tailIdx = i;
  }
  setBPM(180);
  startPlay();
  return { startId, muteId, tailId, cycleLen, muteIdx, tailIdx };
});

if (setup.muteIdx < 0 || setup.tailIdx < 0) { throw new Error(`failed to locate mute/tail indices: ${JSON.stringify(setup)}`);
}

await page.waitForTimeout(250);

const pattern = await page.evaluate(({ cycleLen, tailIdx }) => { const values = [];
  const ensureVisible = (idx) => { ensure(idx + 1); return !!visibleAt(0, idx); };
  for (let cycle = 0; cycle < 4; cycle++) { const idx = tailIdx + cycle * cycleLen;
    values.push(ensureVisible(idx));
  }
  return values;
}, setup);

const expected = [true, false, true, false];
if (pattern.length !== expected.length || pattern.some((v, i) => v !== expected[i])) { throw new Error(`mute logic mismatch: got=${pattern.join(",")} want=${expected.join(",")}`);
}

const triggeredPattern = await page.evaluate(({ cycleLen, muteIdx }) => { const values = [];
  for (let cycle = 0; cycle < 4; cycle++) { const idx = muteIdx + cycle * cycleLen;
    ensure(idx + 1);
    values.push(triggeredAt(0, idx));
  }
  return values;
}, setup);

const expectedTriggers = [false, true, false, true];
if (triggeredPattern.length !== expectedTriggers.length || triggeredPattern.some((v, i) => v !== expectedTriggers[i])) { throw new Error(`mute trigger mismatch: got=${triggeredPattern.join(",")} want=${expectedTriggers.join(",")}`);
}

await page.waitForFunction((id) => nodeAnimValue(id) > 0, setup.muteId, { timeout: 2000 });

await assertNoSchedulerMismatches(page, "mute logic: scheduler mismatches");
await browser.close();
server.close();
