import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Build the UI playtest wasm that keeps Go alive and exposes export/import.
import { spawnSync } from "child_process";
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

// Serve the wasm app
const server = http.createServer((req, res) => { const p = req.url === "/" ? "/play_ui.html" : req.url;
  const filePath = path.join(jsDir, p);
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
    const ct = filePath.endsWith(".wasm") ? "application/wasm" : filePath.endsWith(".html") ? "text/html" : "application/javascript";
    res.writeHead(200, {"Content-Type": ct});
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch();
const page = await browser.newPage();

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, true, "import/export");
await clearSchedulerMismatches(page);
await page.evaluate(() => startPlay());
await page.waitForFunction(() => typeof exportJSON === 'function' && typeof importJSON === 'function');

// Intercept download and import pickers by stubbing helpers.
await page.evaluate(() => { window.__exportText = "";
  window.downloadJSON = (name, text) => { window.__exportText = text; };
});

// Export current state to JSON.
const initialJSON = await page.evaluate(() => exportJSON());
if (!initialJSON || initialJSON.indexOf('"version"') < 0) { throw new Error("exportJSON returned empty or invalid JSON");
}
await assertNoSchedulerMismatches(page, "import/export: scheduler mismatches before import");

// Import the same JSON and export again to verify the path.
await page.evaluate((txt) => importJSON(txt), initialJSON);
const roundtrip = await page.evaluate(() => exportJSON());
if (!roundtrip || roundtrip.indexOf('"version"') < 0) { throw new Error("roundtrip exportJSON failed");
}
await assertNoSchedulerMismatches(page, "import/export: scheduler mismatches after import");

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "import_export");
await browser.close();
server.close();
console.log("import/export browser flow verified");
