import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Ensure Playwright's Chromium is installed only if missing.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const port = 8150 + Math.floor(Math.random() * 1000);

// Build a lightweight UI WASM that exports JS hooks without running Ebiten.
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");

const server = http.createServer((req, res) => { try { console.log('[SRV]', req.url); } catch(_) {}
  const file = req.url === "/" ? "/ui.html" : req.url;
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
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404);
      res.end();
      return;
    }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();
page.on('console', (msg) => { try { console.log('[PAGE]', msg.type(), msg.text()); } catch(_) {} });

// Intercept WebAudio to capture output and timing.
await page.addInitScript(() => { // Pre-initialize capture arrays so helpers can use them before audio starts
  window.__samples = [];
  window.__marks = [];
  window.__done = false;
  window.__firstSampleTime = undefined;
  window.__vols = [];

  const RealAC = window.AudioContext || window.webkitAudioContext;
  const SAMPLE_TARGET = 60000;
  class TestAC extends RealAC { constructor(opts) { super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(256, 1, 1);
      window.__sampleRate = this.sampleRate;
      sp.addEventListener("audioprocess", (e) => { const data = e.inputBuffer.getChannelData(0);
        if (window.__firstSampleTime === undefined) { for (let i = 0; i < data.length; i++) { if (data[i] !== 0) { window.__firstSampleTime = performance.now();
              break;
            }
          }
        }
        window.__samples.push(...data);
        if (window.__samples.length >= SAMPLE_TARGET) { window.__done = true;
        }
      });
      sp.connect(dest);
      Object.defineProperty(this, "destination", { value: sp });
    }
  }
  window.AudioContext = TestAC;
  window.webkitAudioContext = TestAC;
  // Helper to mark timestamps inside the page context
  window.__mark = () => { window.__marks.push(performance.now()); };
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof sliderRect === 'function');
await assertSimpleDrawMode(page, true, "slider volume");
await page.waitForFunction(() => typeof rowVolume === 'function');

// Programmatically set low and high volumes and verify.
await page.evaluate(() => setRowVolume(0, 0.1));
const lowVol = await page.evaluate(() => rowVolume(0));
await page.evaluate(() => setRowVolume(0, 1.0));
const highVol = await page.evaluate(() => rowVolume(0));
await browser.close();
server.close();

if (!(highVol > lowVol * 2.0)) { throw new Error(`slider volume ineffective: lowVol=${lowVol.toFixed(2)} highVol=${highVol.toFixed(2)}`);
}

console.log("slider volume change verified", { lowVol, highVol });
