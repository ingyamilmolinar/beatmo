import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build the tiny harness that invokes audio.Play("snare").
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "playtest.wasm"), "./internal/audio/playtest"],
  { cwd: goDir,
    env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
    stdio: "inherit",
  },
);
if (build.status !== 0) { throw new Error("go build failed");
}

// Spin up a tiny static server for the harness files.
const port = 8300 + Math.floor(Math.random() * 500);
const server = http.createServer((req, res) => { if (req.url === "/play.html") { const html = `<!doctype html><html><body>
<script type="module">
  import { enableChannelAnalyzer, channelAnalyzerSnapshot } from './audio.js';
  window.enableChannelAnalyzer = enableChannelAnalyzer;
  window.channelAnalyzerSnapshot = channelAnalyzerSnapshot;
  enableChannelAnalyzer('main', { fftSize: 1024, smoothing: 0 });
</script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('playtest.wasm'), go.importObject).then((r) => go.run(r.instance));
</script>
</body></html>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, req.url.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404);
      res.end();
      return;
    }
    const ct = filePath.endsWith(".wasm")
      ? "application/wasm"
      : "application/javascript";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/play.html`);
await page.waitForFunction(() => window.__wasmReady === true, { timeout: 5000 });
// Trigger resume (oto expects a gesture); harmless in headless.
await page.evaluate(() => document.dispatchEvent(new Event('mousedown')));
// Wait until analyzer captures a non-zero waveform.
await page.waitForFunction(() => { const snap = window.channelAnalyzerSnapshot('main');
  if (!snap.wave || snap.wave.length === 0) return false;
  const peak = snap.wave.reduce((m, v) => Math.max(m, Math.abs(v)), 0);
  return peak > 0.01;
}, { timeout: 5000 });

const snap = await page.evaluate(() => window.channelAnalyzerSnapshot('main'));
await browser.close();
server.close();

const peak = snap.wave.reduce((m, v) => Math.max(m, Math.abs(v)), 0);
if (peak <= 0.01) { throw new Error(`expected audible waveform peak >0.01, got ${peak}`);
}
