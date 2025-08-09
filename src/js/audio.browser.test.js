import {chromium} from 'playwright';
import {spawnSync} from 'child_process';
import http from 'http';
import fs from 'fs';
import path from 'path';
import {fileURLToPath} from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(__dirname, '../go');

// Build the tiny harness that invokes audio.Play("snare").
const build = spawnSync('go', ['build', '-o', path.join(jsDir, 'playtest.wasm'), './internal/audio/playtest'], {
  cwd: goDir,
  env: {...process.env, GOOS: 'js', GOARCH: 'wasm'},
  stdio: 'inherit'
});
if (build.status !== 0) {
  throw new Error('go build failed');
}

// Ensure playwright and its dependencies are installed.
spawnSync('npx', ['playwright', 'install', 'chromium'], {cwd: jsDir, stdio: 'inherit'});
spawnSync('npx', ['playwright', 'install-deps', 'chromium'], {cwd: jsDir, stdio: 'inherit'});

const port = 8123 + Math.floor(Math.random()*1000);
const server = http.createServer((req, res) => {
  if (req.url === '/play.html') {
    const html = `<!DOCTYPE html><html><body>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('playtest.wasm'), go.importObject).then(r => go.run(r.instance));
</script>
</body></html>`;
    res.writeHead(200, {'Content-Type': 'text/html'});
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, req.url);
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    const ct = filePath.endsWith('.wasm') ? 'application/wasm' : 'application/javascript';
    res.writeHead(200, {'Content-Type': ct});
    res.end(data);
  });
});
await new Promise(r => server.listen(port, r));

const browser = await chromium.launch({args:['--autoplay-policy=no-user-gesture-required']});
const page = await browser.newPage();

// Hook into Web Audio to capture raw samples from the ScriptProcessorNode.
await page.addInitScript(() => {
  const RealAC = window.AudioContext || window.webkitAudioContext;
  class TestAC extends RealAC {
    constructor(opts) {
      super(opts);
      const dest = super.destination;
      const sp = this.createScriptProcessor(4096, 1, 1);
      sp.addEventListener('audioprocess', e => {
        const data = e.inputBuffer.getChannelData(0);
        if (!window.__lastSamples && Array.from(data).some(v => v !== 0)) {
          window.__lastSamples = Array.from(data);
        }
      });
      sp.connect(dest);
      Object.defineProperty(this, 'destination', {value: sp});
    }
  }
  window.AudioContext = TestAC;
  window.webkitAudioContext = TestAC;
});

await page.goto(`http://localhost:${port}/play.html`);
await page.waitForFunction(() => window.__wasmReady === true);
// Trigger the resume handler registered by oto's driver.
await page.evaluate(() => document.dispatchEvent(new Event('mousedown')));
// Wait for audio to be processed.
await page.waitForFunction(() => window.__lastSamples && window.__lastSamples.some(v => v !== 0), {}, {timeout: 5000});
const samples = await page.evaluate(() => window.__lastSamples);
await browser.close();
server.close();

if (!samples || !samples.some(v => v !== 0)) {
  throw new Error('no audio samples captured');
}

console.log('captured audio samples:', samples.slice(0, 8).map(v => v.toFixed(5)));
