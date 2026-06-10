// sampler_persistence_idb.browser.test.js
//
// Validates the browser cross-session persistence boundary for the Sampler tab:
// a sample written via idbPutSample survives a page reload and reads back
// byte-identical via idbGetAllSamples, and idbDeleteSample removes it. This is
// the IndexedDB storage layer the wasm userprefs.SampleStore backend uses;
// localStorage can't hold PCM, so this can only be exercised in a real browser.
//
// COVERED-BY-GO:
//   internal/userprefs/sample_store_test.go   (desktop file round-trip)
//   internal/audio/user_samples_test.go        (ApplySavedSamples re-register)
//   internal/ui/sampler_export_test.go          (export/import embed)
//
// Reuses the playtest.wasm harness so audio.js (which defines the idb* shim)
// loads in its normal Go-WASM context.

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(__dirname, "../go");

const GO = resolveGoBinary();
if (!shouldSkipWasmBuild("playtest.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "playtest.wasm"), "./internal/audio/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build failed");
}

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const server = http.createServer((req, res) => {
  if (req.url === "/play.html") {
    const html = `<!DOCTYPE html><html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('playtest.wasm'), go.importObject).then(r => go.run(r.instance));
</script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, req.url.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    const ct = filePath.endsWith(".wasm") ? "application/wasm" : "application/javascript";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch();
const page = await browser.newPage();

await page.goto(`http://localhost:${port}/play.html`);
await page.waitForFunction(() => typeof window.idbPutSample === "function" && typeof window.idbGetAllSamples === "function");

// Write a sample to IndexedDB.
const id = "user.sample.persisttest";
await page.evaluate(async (id) => {
  const bytes = new Uint8Array([1, 2, 3, 4, 5, 6, 7, 8]);
  await window.idbPutSample(id, 44100, bytes);
}, id);

// Reload the page — IndexedDB persists per origin across reloads.
await page.reload();
await page.waitForFunction(() => typeof window.idbGetAllSamples === "function");

const loaded = await page.evaluate(async (id) => {
  const all = await window.idbGetAllSamples();
  const rec = all.find((r) => r.id === id);
  if (!rec) return { found: false };
  return { found: true, sr: rec.sr, bytes: Array.from(rec.bytes) };
}, id);

if (!loaded.found) {
  await browser.close(); server.close();
  throw new Error("sample did not survive reload (IndexedDB persistence broken)");
}
if (loaded.sr !== 44100 || loaded.bytes.join(",") !== "1,2,3,4,5,6,7,8") {
  await browser.close(); server.close();
  throw new Error(`reloaded sample mismatch: sr=${loaded.sr} bytes=${loaded.bytes}`);
}

// Delete removes it.
const afterDelete = await page.evaluate(async (id) => {
  await window.idbDeleteSample(id);
  const all = await window.idbGetAllSamples();
  return all.some((r) => r.id === id);
}, id);

await browser.close();
server.close();

if (afterDelete) {
  throw new Error("idbDeleteSample did not remove the sample");
}

console.log(`[sampler-persistence-idb] OK: sample survived reload + delete removed it`);
