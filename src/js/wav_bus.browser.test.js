import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

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

const port = 8450 + Math.floor(Math.random() * 1000);
const server = http.createServer((req, res) => { const file = req.url === "/" ? "/ui.html" : req.url;
  if (req.url === "/" || req.url === "/ui.html") { const html = `<!doctype html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
  window.__ready = new Promise(r => { const iv = setInterval(() => { if (typeof startPlay === 'function') { clearInterval(iv); r(true); } }, 10);
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
  await assertSimpleDrawMode(page, true, "wav bus");
  await page.evaluate(() => { if (typeof resumeAudio === 'function') resumeAudio(); });

  // Register a fake sample id mapping so playSound can resolve without fetch; reusing DSP renders is fine for bus.
  await page.evaluate(() => { window.registerWav('snare', 'data:audio/wav;base64,'); // ignored; render path used
  });
  // Trigger many events with varying volumes to populate buses.
  await page.evaluate(async () => { for (let i = 0; i < 50; i++) { const v = (i % 16) / 16;
      await window.playSoundParams('snare', v, 0, 1.0, window.audioNow());
    }
  });
  const buses = await page.evaluate(() => window.__audioBusCount ? window.__audioBusCount() : -1);
  console.log('wav_bus: buses', buses);
  if (buses < 8 || buses > 32) { throw new Error('unexpected number of volume buses: ' + buses);
  }
} finally { if (browser) await browser.close();
  server.close();
}
