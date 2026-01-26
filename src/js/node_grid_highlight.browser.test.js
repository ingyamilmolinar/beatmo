import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");

const port = 8490 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => { const file = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") { const html = `<!doctype html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
  window.__ready = new Promise(r => { const iv = setInterval(() => { if (typeof startPlay === 'function' && typeof buildPerfRect === 'function') { clearInterval(iv); r(true); } }, 10);
  });
</script>`;
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
await new Promise((r) => server.listen(port, r));

let browser;
try { browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => window.__ready);
  await assertSimpleDrawMode(page, true, "node grid highlight");
  await page.evaluate(() => { if (typeof resumeAudio === 'function') resumeAudio(); });
  await clearSchedulerMismatches(page);

  // Build a 1x1 loop and start.
  await page.evaluate(() => {
    buildPerfRect(1, 1);
    setBPM(150);
    startPlay();
  });

  // Poll for node highlight on any of the 4 nodes of the 1x1 rectangle
  let ok = false;
  const coords = [[0,0],[1,0],[1,1],[0,1]];
  for (let t = 0; t < 30 && !ok; t++) { await page.waitForTimeout(100);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    ok = await page.evaluate((coords) => { if (typeof nodeHighlightedAt !== 'function') return false;
      for (const [i,j] of coords) { if (nodeHighlightedAt(i,j)) return true;
      }
      return false;
    }, coords);
  }
  if (!ok) throw new Error('no node highlight detected on main grid');
  await assertNoSchedulerMismatches(page, "node grid highlight: scheduler mismatches");
} finally { if (browser) await browser.close();
  server.close();
}
