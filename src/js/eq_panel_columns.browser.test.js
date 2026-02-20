import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build the full WASM so we exercise the real DrumView layout.
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

// Minimal static server.
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
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);

await page.waitForFunction(() =>
  typeof setEQView === 'function' &&
  typeof eqBandsSnapshot === 'function' &&
  typeof forceDraw === 'function'
);
await assertSimpleDrawMode(page, false, "eq panel columns");

// Reset EQ gains to 0 dB — the startup demo may have non-zero EQ values.
await page.evaluate(() => {
  importJSON?.(JSON.stringify({ version: 1, eq: { gains_db: [] } }));
  forceDraw?.();
});
await page.waitForTimeout(200);

// Override analyzer snapshot with a synthetic spectrum so bands are predictable.
const snap = await page.evaluate(() => { const spec = new Array(512).fill(0);
  // Emphasize low and mid-high.
  for (let i = 0; i < spec.length; i++) { if (i < 40) spec[i] = 0.9;           // Sub/Bass
    else if (i < 120) spec[i] = 0.6;     // LowMid
    else if (i < 200) spec[i] = 0.2;     // Mid
    else if (i < 360) spec[i] = 0.4;     // HighMid/Presence
    else spec[i] = 0.05;                 // Air tail
  }
  window.channelAnalyzerSnapshot = () => ({ rms: 0.5, peak: 0.9, spectrum: spec, wave: [] });
  setEQView?.("eq");
  forceDraw?.();
  forceDraw?.();
  return { bands: eqBandsSnapshot?.(),
    controls: eqControlsSnapshot?.(),
  };
});

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "eq_panel_columns");
await browser.close();
server.close();

if (!snap || !snap.bands || !Array.isArray(snap.bands.names) || !Array.isArray(snap.bands.values)) { throw new Error("eqBandsSnapshot missing names/values");
}
if (snap.bands.names.length < 10 || snap.bands.values.length < 10) { throw new Error(`expected >=10 EQ bands, got names=${snap.bands.names.length}, values=${snap.bands.values.length}`);
}
const peak = Math.max(...snap.bands.values);
if (!(peak > 0.05)) { throw new Error(`expected EQ bands to register energy; peak=${peak}`);
}
// Low bands should be stronger than air tail.
if (!(snap.bands.values[0] > snap.bands.values[snap.bands.values.length - 1])) { throw new Error(`expected lowest band > highest band; got ${snap.bands.values[0]} vs ${snap.bands.values.at(-1)}`);
}
// Controls snapshot mirrors slider state (all centered initially).
if (!snap.controls || !Array.isArray(snap.controls.gainsDB) || snap.controls.gainsDB.length < 10) { throw new Error('missing EQ control gains snapshot');
}
if (Math.max(...snap.controls.gainsDB.map(Math.abs)) > 0.1) { throw new Error('expected default gains near 0 dB');
}
