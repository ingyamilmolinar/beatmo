import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Chromium is installed only if missing.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
if (!shouldSkipWasmBuild("play_ui.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => { if (req.url === "/") { const html = `<!DOCTYPE html><html><body>
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
  const fp = path.join(jsDir, req.url.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
    let ct = "application/javascript";
    if (fp.endsWith(".wasm")) ct = "application/wasm";
    else if (fp.endsWith(".html")) ct = "text/html";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on('console', (msg) => { try { console.log('[PAGE]', msg.type(), msg.text()); } catch(_) {} });

// Intercept WebAudio to capture output stream
await page.addInitScript(() => { window.__samples = [];
  window.__done = false;
  window.__firstSampleTime = undefined;
  const RealAC = window.AudioContext || window.webkitAudioContext;
  const SAMPLE_TARGET = 40000;
  class TestAC extends RealAC { constructor(opts) { super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(256, 1, 1);
      sp.addEventListener('audioprocess', (e) => { const data = e.inputBuffer.getChannelData(0);
        if (window.__firstSampleTime === undefined) { for (let i = 0; i < data.length; i++) { if (data[i] !== 0) { window.__firstSampleTime = performance.now(); break; } }
        }
        window.__samples.push(...data);
        if (window.__samples.length >= SAMPLE_TARGET) window.__done = true;
      });
      sp.connect(dest);
      Object.defineProperty(this, 'destination', { value: sp });
    }
  }
  window.AudioContext = TestAC;
  window.webkitAudioContext = TestAC;
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof ensureDefaultPath === 'function');
await page.evaluate(() => ensureDefaultPath());
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, true, "drum playback");
await clearSchedulerMismatches(page);

// Force AudioContext creation via user gesture — audio.js defers context
// creation to unlockAudio() which only runs on user gesture events.
// Without this, no context exists and all audio events stay queued forever.
await page.click('body');
await page.waitForFunction(() => window.__audioCtx && window.__audioCtx.state === 'running', {}, { timeout: 5000 });

await page.evaluate(() => startPlay());

// Try to resume audio context, then wait for sample capture
await page.evaluate(() => window.resumeAudio && window.resumeAudio());
await page.waitForFunction(() => window.__samples && window.__samples.length > 0, {}, { timeout: 15000 });
await page.waitForFunction(() => window.__done === true, {}, { timeout: 15000 });
const first = await page.evaluate(() => window.__samples.findIndex((v) => v !== 0));
const count = await page.evaluate(() => window.__samples.length);

await assertNoSchedulerMismatches(page, "drum playback: scheduler mismatches");
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "drum_playback");
await browser.close();
server.close();

if (first < 0) throw new Error('no non-zero audio samples captured from UI playback');
if (count < 20000) throw new Error(`insufficient audio samples captured: ${count}`);
console.log('UI-driven drum playback produced audio');
