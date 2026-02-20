import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Chromium is available for CI/headless.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

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

// Minimal server for the harness files.
const server = http.createServer((req, res) => { const file = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") { const html = `<!DOCTYPE html><html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
  window.__marks = [];
  window.__mark = () => { window.__marks.push(performance.now()); };
  // Helper: wait until Go exported functions exist
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
  page.on('console', msg => { if (msg.type() === 'error' || msg.type() === 'warning') { console.log('[page]', msg.type(), msg.text());
    }
  });
  page.on('pageerror', err => console.error('[pageerror]', err));
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => window.__ready);
  await assertSimpleDrawMode(page, true, "perf.browser");

  // Build a moderate graph: 2 rectangles with side=1 to stress scheduling without overwhelming the WASM thread.
  await page.evaluate(() => buildPerfRect(2, 1));
  await page.evaluate(() => setBPM(120));
  await clearSchedulerMismatches(page);
  await page.evaluate(() => startPlay());

  await page.evaluate(() => resetPerfStats?.());
  const initial = await page.evaluate(() => perfStats());
  console.log('initial stats', initial);

  await new Promise(resolve => setTimeout(resolve, 1000));

  const stats = await page.evaluate(() => perfStats());
  await assertNoSchedulerMismatches(page, "perf.browser: scheduler mismatches");
  await page.evaluate(() => { if (typeof stopPlay === "function") stopPlay();
  });
  console.log('perf.browser: ui', stats);

  if (!stats || stats.frames <= 0) { throw new Error('no frames recorded in WASM harness');
  }

  const baseUpdateMax = Number(process.env.PERF_BROWSER_UPDATE_MAX_MS ?? "3.5");
  const updateJitter = Number(process.env.PERF_BROWSER_UPDATE_JITTER_MS ?? "0.25");
  const maxUpdateAvg = baseUpdateMax + updateJitter;
  const minFps = Number(process.env.PERF_BROWSER_FPS_MIN ?? "6");
  if (stats.updateAvgMS > maxUpdateAvg) { throw new Error(`updateAvgMS ${stats.updateAvgMS.toFixed(3)}ms exceeded limit ${maxUpdateAvg}ms (base ${baseUpdateMax} + jitter ${updateJitter})`);
  }
  if (stats.fpsAvg < minFps) { throw new Error(`fpsAvg ${stats.fpsAvg.toFixed(2)} below floor ${minFps}`);
  }
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "perf");
} finally { if (browser) { 
 await browser.close();
  }
  server.close();
}
