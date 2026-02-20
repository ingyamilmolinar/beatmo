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
  GO,
  ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
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
await assertSimpleDrawMode(page, true, "playback highlight");
await clearSchedulerMismatches(page);
await page.evaluate(() => startPlay());

// Sample visibleAt around the current beat over ~1.5s to ensure progression.
await page.waitForFunction(() => typeof gridSubdiv === 'function' && typeof visibleAt === 'function' && typeof currentBeat === 'function');
const seen = await page.evaluate(async () => { const div = gridSubdiv();
  const seenIdx = new Set();
  const samples = 30;
  for (let i = 0; i < samples; i++) { const beat = currentBeat();
    const abs = Math.floor(beat * div);
    // Make sure predictor buffers cover the probed region.
    if (typeof ensure === 'function') { ensure(abs + div * 2);
    }
    // Record the exact abs if visible, otherwise probe +/- 1 around it.
    if (visibleAt(0, abs)) { seenIdx.add(abs);
    } else if (visibleAt(0, abs-1)) { seenIdx.add(abs-1);
    } else if (visibleAt(0, abs+1)) { seenIdx.add(abs+1);
    }
    await new Promise((r) => setTimeout(r, 50));
  }
  return Array.from(seenIdx).sort((a,b)=>a-b);
});
if (!seen || seen.length < 4) { throw new Error(`insufficient highlight progression: ${JSON.stringify(seen)}`);
}

await assertNoSchedulerMismatches(page, "playback highlight: scheduler mismatches");
if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "playback_highlight");
await browser.close();
server.close();
console.log('playback highlight progression verified');
