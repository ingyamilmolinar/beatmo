import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on('console', (msg) => { try { console.log('[PAGE]', msg.type(), msg.text()); } catch(_) {} });

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof ensureDefaultPath === 'function');
await page.evaluate(() => ensureDefaultPath());
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, true, "pause-resume burst");
await clearSchedulerMismatches(page);
await page.evaluate(() => startPlay());
await page.waitForTimeout(200);

await assertNoSchedulerMismatches(page, "pause-resume burst: scheduler mismatches before pause");
// Pause
await page.evaluate(() => togglePlay());
await page.waitForTimeout(50);

// Reset audio debug, then resume and capture events for ~30ms
await page.waitForFunction(() => typeof resetAudioDebug === 'function' && typeof getAudioDebug === 'function');
await page.evaluate(() => resetAudioDebug());
await page.evaluate(() => togglePlay()); // resume
await page.waitForTimeout(30);
const events = await page.evaluate(() => getAudioDebug());
await assertNoSchedulerMismatches(page, "pause-resume burst: scheduler mismatches after resume");

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "pause_resume_burst");
await browser.close();
server.close();

// Count scheduling events
const playCalls = events.filter((e) => e && e.tag && String(e.tag).startsWith('play.params.call')).length;
// With 32 subdiv and ~120bpm, expected <= ~2 in 20-30ms; allow a small margin.
if (playCalls > 3) { throw new Error(`burst on resume: ${playCalls} events in ~30ms`);
}
console.log('pause-resume no-burst verified');
