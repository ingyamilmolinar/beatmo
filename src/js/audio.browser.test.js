import {chromium} from 'playwright';
import {spawnSync} from 'child_process';
import http from 'http';
import fs from 'fs';
import path from 'path';
import {fileURLToPath} from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(__dirname, '../go');

// build wasm harness
const build = spawnSync('go', ['build', '-o', path.join(jsDir, 'playtest.wasm'), './internal/audio/playtest'], {
  cwd: goDir,
  env: {...process.env, GOOS: 'js', GOARCH: 'wasm'},
  stdio: 'inherit'
});
if (build.status !== 0) {
  throw new Error('go build failed');
}

// ensure playwright browsers are installed
spawnSync('npx', ['playwright', 'install', 'chromium'], {cwd: jsDir, stdio: 'inherit'});
spawnSync('npx', ['playwright', 'install-deps', 'chromium'], {cwd: jsDir, stdio: 'inherit'});

const port = 8123 + Math.floor(Math.random()*1000);
const server = http.createServer((req, res) => {
  let filePath;
  if (req.url === '/play.html') {
    const html = `<!DOCTYPE html><html><body>
<script src="wasm_exec.js"></script>
<script type="module">
import './audio.js';
const go = new Go();
WebAssembly.instantiateStreaming(fetch('playtest.wasm'), go.importObject).then(r => go.run(r.instance));
</script>
</body></html>`;
    res.writeHead(200, {'Content-Type': 'text/html'});
    res.end(html);
    return;
  }
  filePath = path.join(jsDir, req.url);
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404); res.end(); return;
    }
    const ct = filePath.endsWith('.wasm') ? 'application/wasm' : 'application/javascript';
    res.writeHead(200, {'Content-Type': ct});
    res.end(data);
  });
});
await new Promise(r => server.listen(port, r));

const browser = await chromium.launch({args:['--autoplay-policy=no-user-gesture-required']});
const page = await browser.newPage();
const logs = [];
page.on('console', msg => logs.push(msg.text()));
await page.goto(`http://localhost:${port}/play.html`);
// wait for snare callback log
await page.waitForEvent('console', {predicate: msg => msg.text().includes('[audio] snare callback'), timeout: 5000});
await browser.close();
server.close();
console.log(logs.join('\n'));
