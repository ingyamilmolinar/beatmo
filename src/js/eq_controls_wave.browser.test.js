import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build full WASM app.
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");

const port = 8425 + Math.floor(Math.random() * 400);
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
await page.waitForFunction(() =>
  typeof setEQView === 'function' &&
  typeof forceDraw === 'function' &&
  typeof eqControlsSnapshot === 'function'
);
await assertSimpleDrawMode(page, false, "eq controls wave");

const result = await page.evaluate(async () => { document.dispatchEvent(new Event('pointerdown'));
  resumeAudio?.();
  setEQView?.("wave");
  // Ensure analyzer snapshot is available before EQ changes.
  forceDraw?.();
  await new Promise((r) => setTimeout(r, 50));
  if (typeof playSound === 'function') {
    playSound('snare', 1.0);
  }
  await new Promise((r) => setTimeout(r, 40));
  const before = channelAnalyzerSnapshot('main');
  // Switch to EQ view and ensure waveform still present after applying a gain.
  setEQView?.("eq");
  await new Promise((r) => setTimeout(r, 20));
  forceDraw?.();
  // Manually nudge gains by calling setChannelEQ through window shim.
  if (typeof setChannelEQ === 'function') { setChannelEQ('main', [{ freq: 80, q: 1, gainDB: 6 }]);
  }
  setEQView?.("wave");
  await new Promise((r) => setTimeout(r, 20));
  forceDraw?.();
  if (typeof playSound === 'function') {
    playSound('snare', 1.0);
  }
  await new Promise((r) => setTimeout(r, 40));
  const after = channelAnalyzerSnapshot('main');
  const wavePeak = Math.max(...(after.wave || []));
  return { before, after, wavePeak };
});

await browser.close();
server.close();

if (!result.after || !result.after.wave || result.after.wave.length === 0) { throw new Error("waveform missing after EQ apply");
}
if (!(result.wavePeak > 0.01)) { throw new Error(`waveform peak too low after EQ apply: ${result.wavePeak}`);
}
