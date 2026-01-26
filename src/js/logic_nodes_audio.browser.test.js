import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Chromium installed if missing
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const port = 8280 + Math.floor(Math.random() * 1000);
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const build = spawnSync(
  GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");

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
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();

// Inject capture AudioContext
await page.addInitScript(() => { const RealAC = window.AudioContext || window.webkitAudioContext;
  const SAMPLE_TARGET = 48000; // ~1s at 48kHz
  class TestAC extends RealAC { constructor(opts) { super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(256, 1, 1);
      window.__samples = [];
      sp.addEventListener("audioprocess", (e) => { const data = e.inputBuffer.getChannelData(0);
        window.__samples.push(...data);
        if (window.__samples.length >= SAMPLE_TARGET) window.__done = true;
      });
      sp.connect(dest);
      Object.defineProperty(this, "destination", { value: sp });
    }
  }
  window.AudioContext = TestAC;
  window.webkitAudioContext = TestAC;
});

const goto = async () => { await page.goto(`http://localhost:${port}/`); };
const waitReady = async () => { await page.waitForFunction(() => typeof ensureDefaultPath === 'function');
};

const rms = (arr) => { let s = 0; for (let i=0;i<arr.length;i++){ const v=arr[i]; s += v*v; }
  return Math.sqrt(s / Math.max(1, arr.length));
};

// Scenario helper: build path with node type and optional logic, run, capture RMS
async function scenario({ type, logicKind, logicP }) { await goto();
  await waitReady();
  await assertSimpleDrawMode(page, true, "logic nodes audio");
  // Build two-node path at offset grid coords to avoid default
  await page.evaluate(({type}) => { // types: 'regular' | 'silent' | 'mute'
    const t = type;
    const i0=20, j0=0, i1=24, j1=0;
    addNode(i0, j0, t);
    addNode(i1, j1, t);
    addEdgeGrid(i0, j0, i1, j1);
    setOrigin(0, i0, j0);
    updateBeatInfosJS();
  }, { type });
  if (logicKind) { await page.evaluate(({kind, p}) => setNodeLogicGrid(20, 0, kind, 0, p ?? 0), { kind: logicKind, p: logicP });
    await page.evaluate(({kind, p}) => setNodeLogicGrid(24, 0, kind, 0, p ?? 0), { kind: logicKind, p: logicP });
  }
  await page.waitForFunction(() => typeof startPlay === 'function');
  await clearSchedulerMismatches(page);
  await page.evaluate(() => { window.__samples = []; window.__done = false; startPlay(); });
  await page.waitForFunction(() => window.__done === true, {}, { timeout: 5000 });
  const samples = await page.evaluate(() => window.__samples);
  await assertNoSchedulerMismatches(page, `logic nodes audio (${type}): scheduler mismatches`);
  return rms(samples);
}

// Silent nodes: expect near-zero RMS
const rSilent = await scenario({ type: 'silent' });
if (rSilent > 1e-3) throw new Error(`silent path produced audio: rms=${rSilent}`);

// Mute nodes: expect near-zero RMS (no audible triggers)
const rMute = await scenario({ type: 'mute' });
if (rMute > 1e-3) throw new Error(`mute path produced audio: rms=${rMute}`);

// Regular nodes: expect non-zero RMS
const rRegular = await scenario({ type: 'regular' });
if (rRegular < 1e-3) throw new Error(`regular path produced no audio: rms=${rRegular}`);

// Probability is covered deterministically in probability_nodes.browser.test.js

await browser.close();
server.close();
console.log('node logic audio scenarios verified', { rSilent, rMute, rRegular });
