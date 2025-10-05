import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Chromium installed if missing
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const port = 8250 + Math.floor(Math.random() * 1000);
const goDir = path.resolve(jsDir, "../go");
const GO = process.env.GO || "go";
const build = spawnSync(
  GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") {
    const html = `<!DOCTYPE html><html><body>
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
  fs.readFile(fp, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
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

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof ensureDefaultPath === 'function');
await page.evaluate(() => ensureDefaultPath());
await page.waitForFunction(() => typeof startPlay === 'function');
await page.evaluate(() => startPlay());
// let things advance a bit
await page.waitForTimeout(200);

// Capture paused reference index using predictor visibility
await page.waitForFunction(() => typeof currentBeat === 'function' && typeof gridSubdiv === 'function' && typeof visibleAt === 'function');
const before = await page.evaluate(() => {
  const beat = currentBeat();
  const div = gridSubdiv();
  const abs = Math.floor(beat * div);
  // Find a nearby visible index to treat as the frozen marker.
  let ref = abs;
  for (let d = 0; d <= 4; d++) {
    if (visibleAt(0, abs + d)) { ref = abs + d; break; }
    if (visibleAt(0, abs - d)) { ref = abs - d; break; }
  }
  return { beat, div, abs, ref };
});

// Pause
await page.evaluate(() => togglePlay());
await page.waitForTimeout(50);

// Record paused index
const pausedAbs = await page.evaluate(() => Math.floor(currentBeat() * gridSubdiv()));

// Sample over ~300ms and verify index does not advance further
const samples = [];
for (let i = 0; i < 6; i++) {
  const snap = await page.evaluate(() => {
    const beat = currentBeat();
    const div = gridSubdiv();
    const abs = Math.floor(beat * div);
    return { beat, div, abs };
  });
  samples.push(snap);
  await page.waitForTimeout(50);
}

await browser.close();
server.close();

// Check invariants: absolute index frozen while paused
const idxFrozen = samples.every(s => s.abs === pausedAbs);
if (!idxFrozen) {
  throw new Error(`index advanced while paused: paused=${pausedAbs} samples=${samples.map(s=>s.abs).join(',')}`);
}
console.log('pause no-advance verified');
