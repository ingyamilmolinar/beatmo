import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const port = 8270 + Math.floor(Math.random() * 1000);
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const build = spawnSync(
  GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/play_ui.html" : req.url;
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
await new Promise((r) => server.listen(port, r));

const browser = await chromium.launch();
const page = await browser.newPage();

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof ensureDefaultPath === 'function');
await assertSimpleDrawMode(page, true, "pan camera");
await page.evaluate(() => ensureDefaultPath());
await page.waitForFunction(() => typeof nodeRect === 'function');

const r0 = await page.evaluate(() => nodeRect(0,0));
if (!r0) throw new Error('nodeRect returned null');
const c0 = { x: Math.floor(r0.x + r0.w/2), y: Math.floor(r0.y + r0.h/2) };

// Pan camera by a known pixel offset and verify node screen position follows.
const dx = 42, dy = 18;
await page.waitForFunction(() => typeof panBy === 'function');
await page.evaluate(({dx,dy}) => panBy(dx, dy), { dx, dy });
await page.waitForTimeout(50);

const r1 = await page.evaluate(() => nodeRect(0,0));
const c1 = { x: Math.floor(r1.x + r1.w/2), y: Math.floor(r1.y + r1.h/2) };
const tol = 3;
if (Math.abs((c1.x - c0.x) - dx) > tol || Math.abs((c1.y - c0.y) - dy) > tol) { throw new Error(`panBy mismatch: moved=(${c1.x-c0.x},${c1.y-c0.y}) want~=(${dx},${dy})`);
}

await browser.close();
server.close();
console.log('camera panBy verified');
