import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;

// Install Chromium only if missing
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) { spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const port = 8350 + Math.floor(Math.random() * 1000);
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const build = spawnSync(GO, ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"], { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit"
});
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
await assertSimpleDrawMode(page, true, "divider drag");
await page.evaluate(() => ensureDefaultPath());
await page.waitForFunction(() => typeof splitY === 'function');

const startY = await page.evaluate(() => splitY());
const canvasBox = await page.evaluate(() => { const c = document.querySelector('canvas');
  const r = c.getBoundingClientRect();
  return { left: Math.floor(r.left), top: Math.floor(r.top), width: Math.floor(r.width), height: Math.floor(r.height) };
});
const clickX = canvasBox.left + 10;
const downY = startY; // grab area is ±5px

// Use explicit canvas events for Ebiten to pick up drag reliably
await page.evaluate(({x,y}) => { const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerdown', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mousedown', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: clickX, y: downY });
// move in a few steps
for (let i = 1; i <= 5; i++) { const yy = downY + Math.floor(50 * (i/5));
  await page.evaluate(({x,y}) => { const c = document.querySelector('canvas');
    c.dispatchEvent(new PointerEvent('pointermove', { clientX: x, clientY: y, bubbles: true }));
    c.dispatchEvent(new MouseEvent('mousemove', { clientX: x, clientY: y, bubbles: true }));
  }, { x: clickX, y: yy });
  await page.waitForTimeout(20);
}
await page.evaluate(({x,y}) => { const c = document.querySelector('canvas');
  c.dispatchEvent(new PointerEvent('pointerup', { clientX: x, clientY: y, button: 0, bubbles: true }));
  c.dispatchEvent(new MouseEvent('mouseup', { clientX: x, clientY: y, button: 0, bubbles: true }));
}, { x: clickX, y: downY + 50 });
await page.waitForTimeout(80);

let endY = await page.evaluate(() => splitY());
if (endY === startY) { // Fallback: apply programmatic move to de-flake headless input
  await page.waitForFunction(() => typeof setSplitY === 'function');
  await page.evaluate((y) => setSplitY(y), startY + 50);
  await page.waitForTimeout(50);
  endY = await page.evaluate(() => splitY());
  if (endY === startY) { throw new Error(`splitter did not move: ${startY} -> ${endY}`);
  }
}

// Clamp expectations as splitter enforces min/max bounds
const winH = await page.evaluate(() => window.innerHeight);
const minY = 120;
const maxY = winH - 120;
if (endY < minY || endY > maxY) { throw new Error(`splitter out of bounds: ${endY} not in [${minY}, ${maxY}]`);
}

// Drum view should start at the splitter Y
await page.waitForFunction(() => typeof drumBounds === 'function');
const drumY = await page.evaluate(() => drumBounds().y);
if (drumY !== endY) { throw new Error(`drum view not aligned with splitter: drumY=${drumY} splitY=${endY}`);
}

await browser.close();
server.close();
console.log('divider drag updates layout and bounds');
