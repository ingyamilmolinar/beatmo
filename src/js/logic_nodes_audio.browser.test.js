import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Chromium installed if missing
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const pageErrors = [];
page.on('pageerror', (err) => pageErrors.push(err.message));

const goto = async () => {
  pageErrors.length = 0;
  await page.goto(`http://localhost:${port}/`, { timeout: 60000 });
};
const waitReady = async () => {
  // Single combined wait for both WASM exports and audio.js globals.
  // Under parallel execution, page initialization can be slow.
  await page.waitForFunction(() =>
    typeof ensureDefaultPath === 'function' &&
    typeof startOutputCapture === 'function' &&
    typeof stopOutputCapture === 'function',
    {},
    { timeout: 60000 }
  );
  if (pageErrors.length > 0) {
    throw new Error(`Page errors during init: ${pageErrors.join('; ')}`);
  }
  // Unlock AudioContext with user gesture.
  await page.evaluate(() => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
  });
  await page.waitForTimeout(100);
  // Wait for AudioContext to reach "running" state.
  await page.waitForFunction(
    () => window.__audioCtx && window.__audioCtx.state === "running",
    {},
    { timeout: 15000 }
  );
  // Wait for synth sample rendering to complete.
  await page.evaluate(() => window.audioReady);
};

const rms = (arr) => { let s = 0; for (let i=0;i<arr.length;i++){ const v=arr[i]; s += v*v; }
  return Math.sqrt(s / Math.max(1, arr.length));
};

// Poll getOutputCapture() for non-zero samples. Returns when signal detected or deadline expires.
async function waitForAudioSignal(page, deadlineMs = 5000, pollMs = 100, tailMs = 300) {
  const start = Date.now();
  while (Date.now() - start < deadlineMs) {
    const hasSignal = await page.evaluate(() => {
      const snap = typeof getOutputCapture === "function" ? getOutputCapture() : null;
      if (!snap || snap.length === 0) return false;
      for (let i = 0; i < snap.length; i++) {
        if (Math.abs(snap[i]) > 1e-6) return true;
      }
      return false;
    });
    if (hasSignal) {
      await new Promise((r) => setTimeout(r, tailMs));
      return;
    }
    await new Promise((r) => setTimeout(r, pollMs));
  }
}

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

  // Use audio.js's built-in output capture instead of ScriptProcessorNode override.
  await page.evaluate(() => { startOutputCapture(); startPlay(); });

  if (type === 'regular') {
    // For regular nodes, poll for non-zero audio signal.
    await waitForAudioSignal(page);
  } else {
    // For silent/mute nodes, wait a fixed duration since no audio will appear.
    await new Promise((r) => setTimeout(r, 1500));
  }

  const samples = await page.evaluate(() => Array.from(stopOutputCapture()));
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

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "logic_nodes_audio");
await browser.close();
server.close();
console.log('node logic audio scenarios verified', { rSilent, rMute, rRegular });
