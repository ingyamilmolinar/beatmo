import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const port = 8300 + Math.floor(Math.random() * 1000);

// Build lightweight UI WASM that exposes JS helpers without running Ebiten.
const goDir = path.resolve(jsDir, "../go");
const GO = process.env.GO || "go";
const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"], {
  cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit"
});
if (build.status !== 0) throw new Error("go build play_ui failed");

const server = http.createServer((req, res) => {
  const p = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") {
    const html = `<!DOCTYPE html><html><body>
<script type=\"module\" src=\"audio.js\"></script>
<script src=\"wasm_exec.js\"></script>
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
  const filePath = path.join(jsDir, p.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    const ct = filePath.endsWith(".wasm") ? "application/wasm" : filePath.endsWith(".html") ? "text/html" : "application/javascript";
    res.writeHead(200, {"Content-Type": ct});
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch();
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
// Wait for helpers
await page.waitForFunction(() => typeof timelineRect === 'function');
await page.waitForFunction(() => typeof setTimelineBeats === 'function');
await page.waitForFunction(() => typeof setDrumLength === 'function');
await page.evaluate(() => { setDrumLength(8); setTimelineBeats(100); });

// Simulate a timeline click at ~75%
const before = await page.evaluate(() => drumOffset());
await page.evaluate(() => clickTimelineAt(0.75));
const after = await page.evaluate(() => drumOffset());

if (after === before) {
  throw new Error(`timeline click did not change offset: ${before}`);
}

// Verify centering math: desired = center - length/2 clamped
const tb = await page.evaluate(() => timelineBeats());
const len = await page.evaluate(() => drumLength());
const center = Math.floor(0.75 * tb);
let desired = center - Math.floor(len/2);
const maxOff = Math.max(0, tb - len);
desired = Math.max(0, Math.min(maxOff, desired));
if (Math.abs(after - desired) > 1) {
  throw new Error(`offset not centered: got=${after} want~=${desired}`);
}

await browser.close();
server.close();
console.log("timeline click centering verified");
