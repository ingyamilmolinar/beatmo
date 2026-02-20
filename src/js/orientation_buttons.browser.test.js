import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, assertSimpleDrawMode, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build WASM target
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

// Minimal static server
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

async function setupPage(width, height) {
  const page = await browser.newPage({ viewport: { width, height } });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() =>
    typeof drumBounds === "function" &&
    typeof forceDraw === "function" &&
    typeof playBtnRect === "function" &&
    typeof splitX === "function" &&
    typeof layoutHorizontal === "function"
  );
  await assertSimpleDrawMode(page, false, `orientation_buttons ${width}x${height}`);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);
  return page;
}

// Test 1: Landscape buttons inside drum bounds
console.log("Test 1: Landscape buttons inside drum bounds");
{
  const page = await setupPage(844, 390);

  const bounds = await page.evaluate(() => drumBounds?.());
  const playRect = await page.evaluate(() => playBtnRect?.());
  const stopRect = await page.evaluate(() => stopBtnRect?.());
  const sx = await page.evaluate(() => splitX?.());

  console.log(`  drumBounds: x=${bounds.x} y=${bounds.y} w=${bounds.w} h=${bounds.h}`);
  console.log(`  playBtn: x=${playRect.x} y=${playRect.y} w=${playRect.w} h=${playRect.h}`);
  console.log(`  stopBtn: x=${stopRect.x} y=${stopRect.y} w=${stopRect.w} h=${stopRect.h}`);
  console.log(`  splitX=${sx}`);

  if (playRect.x < sx) {
    throw new Error(`playBtn x=${playRect.x} < splitX=${sx} — button outside drum pane`);
  }
  if (stopRect.x < sx) {
    throw new Error(`stopBtn x=${stopRect.x} < splitX=${sx} — button outside drum pane`);
  }
  if (playRect.x < bounds.x) {
    throw new Error(`playBtn x=${playRect.x} < drumBounds.x=${bounds.x}`);
  }
  if (stopRect.x < bounds.x) {
    throw new Error(`stopBtn x=${stopRect.x} < drumBounds.x=${bounds.x}`);
  }

  await page.close();
}

// Test 2: Portrait->Landscape buttons update
console.log("Test 2: Portrait -> Landscape buttons update");
{
  const page = await setupPage(390, 844);

  let playRect = await page.evaluate(() => playBtnRect?.());
  console.log(`  Portrait playBtn: x=${playRect.x} y=${playRect.y}`);

  // Portrait: buttons should be near x=0
  if (playRect.x > 50) {
    throw new Error(`portrait: playBtn.x=${playRect.x} expected near 0`);
  }

  // Rotate to landscape
  await page.setViewportSize({ width: 844, height: 390 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  playRect = await page.evaluate(() => playBtnRect?.());
  const sx = await page.evaluate(() => splitX?.());
  console.log(`  Landscape playBtn: x=${playRect.x} y=${playRect.y}, splitX=${sx}`);

  if (playRect.x < sx) {
    throw new Error(`landscape: playBtn.x=${playRect.x} < splitX=${sx} — button didn't move`);
  }

  await page.close();
}

// Test 3: Round trip Portrait->Landscape->Portrait
console.log("Test 3: Round trip buttons remain valid");
{
  const page = await setupPage(390, 844);

  // Portrait
  let bounds = await page.evaluate(() => drumBounds?.());
  let playRect = await page.evaluate(() => playBtnRect?.());
  console.log(`  Portrait: drumBounds x=${bounds.x}, playBtn x=${playRect.x}`);
  if (playRect.x < bounds.x || playRect.x + playRect.w > bounds.x + bounds.w) {
    throw new Error(`portrait: playBtn outside drumBounds`);
  }

  // Landscape
  await page.setViewportSize({ width: 844, height: 390 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  bounds = await page.evaluate(() => drumBounds?.());
  playRect = await page.evaluate(() => playBtnRect?.());
  console.log(`  Landscape: drumBounds x=${bounds.x}, playBtn x=${playRect.x}`);
  if (playRect.x < bounds.x || playRect.x + playRect.w > bounds.x + bounds.w) {
    throw new Error(`landscape: playBtn outside drumBounds`);
  }

  // Portrait again
  await page.setViewportSize({ width: 390, height: 844 });
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(300);

  bounds = await page.evaluate(() => drumBounds?.());
  playRect = await page.evaluate(() => playBtnRect?.());
  console.log(`  Portrait again: drumBounds x=${bounds.x}, playBtn x=${playRect.x}`);
  if (playRect.x < bounds.x || playRect.x + playRect.w > bounds.x + bounds.w) {
    throw new Error(`portrait-2: playBtn outside drumBounds`);
  }

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "orientation_buttons");
  await page.close();
}

await browser.close();
server.close();
console.log("Orientation buttons tests completed");
