import { chromium } from "playwright";
import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertNoSchedulerMismatches, assertSimpleDrawMode, clearSchedulerMismatches, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Build main WASM
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

const port = 8375 + Math.floor(Math.random() * 1000);
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
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof startPlay === 'function');
await assertSimpleDrawMode(page, false, "track center");

await page.evaluate(() => { setFollow?.(true);
  buildPerfRect(2, 1);
  setBPM?.(200);
  forceDraw?.();
});

await clearSchedulerMismatches(page);
await page.evaluate(() => startPlay?.());
await page.waitForTimeout(1800);

const result = await page.evaluate(() => { try { const t = timelineRect?.();
    if (!t) return false;
    const center = t.x + t.w / 2;
    const n = drumLength?.() ?? 0;
    const off = drumOffset?.() ?? 0;
    const units = timelineUnitsPerBeat?.() ?? 1;
    const beat = currentBeat?.() ?? 0;
    const curIdx = Math.round(beat * units);
    const j = curIdx - off;
    if (n <= 0 || j < 0 || j >= n) return false;
    const x0 = t.x + (j * t.w) / n;
    const x1 = t.x + ((j + 1) * t.w) / n;
    const tolerance = Math.max(8, Math.floor(n / 4));
    const midIdx = Math.round((j + 0.5));
    const ok = Math.abs(midIdx - n / 2) <= tolerance;
    return { ok, n, off, curIdx, j, center, x0, x1, units, tolerance };
  } catch { return false; }
});

console.log('track_center result:', result);
const ok = result && result.ok;

await assertNoSchedulerMismatches(page, "track center: scheduler mismatches");
await browser.close();
server.close();

if (!ok) { throw new Error("tracking highlight not centered over timeline center");
}
