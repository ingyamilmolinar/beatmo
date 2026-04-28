import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Build the UI playtest WASM harness.
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

// Minimal file server for the harness.
const server = http.createServer((req, res) => { const file = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") { const html = `<!DOCTYPE html><html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
  window.__ready = new Promise(r => { const iv = setInterval(() => { if (typeof buildPerfRect === 'function' && typeof setBPM === 'function' && typeof startPlay === 'function') { clearInterval(iv); r(true); }
    }, 10);
  });
  </script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
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
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

let browser;
try { browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => window.__ready);
  await assertSimpleDrawMode(page, true, "batch audio");
  await clearSchedulerMismatches(page);

  // Wrap bridge calls to count invocations.
  await page.evaluate(() => { window.__counts = { batch: 0, params: 0 };
    const ob = window.playSoundsBatch;
    const op = window.playSoundParams;
    if (typeof window.playSoundsBatch === 'function') { window.playSoundsBatch = (arr) => { window.__counts.batch++; return ob.call(window, arr); };
    }
    if (typeof window.playSoundParams === 'function') { window.playSoundParams = (id, vol, pitch, dur, when) => { window.__counts.params++; return op.call(window, id, vol, pitch, dur, when); };
    }
    if (typeof window.resumeAudio === 'function') { window.resumeAudio(); }
  });

  // Build a moderate graph and start playback.
  await page.evaluate(() => buildPerfRect(2, 1));
  await page.evaluate(() => setBPM(150));
  await page.evaluate(() => startPlay());

  await page.waitForTimeout(1200);

  const res = await page.evaluate(() => ({ counts: window.__counts, stats: perfStats() }));
  if (!res || !res.stats) throw new Error('missing perf stats');
  console.log('batch test:', res);

  // Batch path should be used and reduce dispatch count vs enqueues.
  if (!res.counts || res.counts.batch <= 0) { throw new Error('batch bridge not invoked');
  }
  if (res.counts.params > 2) { throw new Error('unexpected fallback to per-item params calls');
  }
  if (!(res.stats.audioDeq < res.stats.audioEnq)) { throw new Error(`expected fewer dispatches than enqueues; got deq=${res.stats.audioDeq} enq=${res.stats.audioEnq}`);
  }
  await assertNoSchedulerMismatches(page, "batch audio: scheduler mismatches");
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "batch_audio");
} finally { if (browser) await browser.close();
  server.close();
}
