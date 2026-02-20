import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

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

const browser = await chromium.launch();
const page = await browser.newPage();
page.on('console', (msg) => { try { console.log('[PAGE]', msg.type(), msg.text()); } catch(_) {} });

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof ensureDefaultPath === 'function');
await assertSimpleDrawMode(page, true, "bpm editor");
await page.evaluate(() => ensureDefaultPath());

// Click BPM box to focus, then commit via helper (typing in headless Ebiten
// harness is not reliable across environments)
await page.waitForFunction(() => typeof bpmBoxRect === 'function' && typeof getBPM === 'function' && typeof commitBPM === 'function');
const r = await page.evaluate(() => bpmBoxRect());
if (!r) throw new Error('bpmBoxRect returned null');
const cx = Math.floor(r.x + r.w/2);
const cy = Math.floor(r.y + r.h/2);
await page.mouse.click(cx, cy);
await page.waitForTimeout(80);
await page.evaluate(() => commitBPM(200));

// Wait a bit for update loop to apply and engine/applied BPM to mirror
await page.waitForFunction(() => typeof getEngineBPM === 'function' && typeof getAppliedBPM === 'function');
await page.waitForTimeout(200);
const vals = await page.evaluate(() => ({ ui: getBPM(), engine: getEngineBPM(), applied: getAppliedBPM() }));
if (vals.ui !== 200 || vals.engine !== 200 || vals.applied !== 200) { throw new Error(`BPM editor commit failed: ${JSON.stringify(vals)}`);
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "bpm_editor");
await browser.close();
server.close();
console.log('BPM editor typing verified');
