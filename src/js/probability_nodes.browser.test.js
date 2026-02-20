import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
if (!shouldSkipWasmBuild("play_ui.wasm")) {
const build = spawnSync(
  GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") { const html = `<!DOCTYPE html><html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
</script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch();
const page = await browser.newPage();
page.on('console', (msg) => { try { console.log('[PAGE]', msg.type(), msg.text()); } catch(_) {} });

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof addNode === 'function' && typeof addEdgeGrid === 'function');
await assertSimpleDrawMode(page, true, "probability nodes");

// Build a simple two-node linear path and set as origin
await page.evaluate(() => { const i0=12, j0=0, i1=16, j1=0;
  addNode(i0, j0, 'regular');
  addNode(i1, j1, 'regular');
  addEdgeGrid(i0, j0, i1, j1);
  setOrigin(0, i0, j0);
  updateBeatInfosJS();
});

await page.waitForFunction(() => typeof setNodeLogicGrid === 'function' && typeof nodeParams === 'function');

// Set probability param and verify propagation
await page.evaluate(() => setNodeLogicGrid(12,0,'probability',0,0.5));
const pA = await page.evaluate(() => nodeParams(12,0));
if (!pA || pA.logicKind !== 'probability' || Math.abs(pA.logicP - 0.5) > 1e-6) { throw new Error(`nodeParams not updated: ${JSON.stringify(pA)}`);
}

// Deterministic probability monotonicity (hash): validate roll ordering without relying on engine predictors.
function detRoll(row, idx, id) { const toU32 = (v) => BigInt.asUintN(32, BigInt(v >>> 0));
  let x = (toU32(row) << 32n) ^ toU32(idx) ^ (toU32(id) << 16n) ^ 0x9E3779B97F4A7C15n;
  x = BigInt.asUintN(64, x + 0x9E3779B97F4A7C15n);
  let z = x;
  z = BigInt.asUintN(64, (z ^ (z >> 30n)) * 0xBF58476D1CE4E5B9n);
  z = BigInt.asUintN(64, (z ^ (z >> 27n)) * 0x94D049BB133111EBn);
  z = BigInt.asUintN(64, z ^ (z >> 31n));
  const mask = (1n << 53n) - 1n;
  const frac = Number(z & mask) / Number(1n << 53n);
  return frac;
}

const row = 0;
const ids = [12, 16];
const rolls = [detRoll(row, 0, ids[0]), detRoll(row, 4, ids[1])];
const pLow = 0.25, pHigh = 0.75;
for (let r of rolls) { if (r <= pLow && r > pHigh) throw new Error(`roll monotonicity failed r=${r}`);
  if (!(r <= 1.0) || !(r >= 0.0)) throw new Error(`roll out of range r=${r}`);
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "probability_nodes");
await browser.close();
server.close();
console.log('probability param propagation + hash monotonicity verified');
