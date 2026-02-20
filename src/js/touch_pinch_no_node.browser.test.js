import { chromium, devices } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, assertSimpleDrawMode, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";
import { cdpPinch, cdpTwoFingerPan } from "./touch_cdp_helpers.js";

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

// Use iPhone 12 landscape device emulation
const iPhone = devices['iPhone 12 landscape'];
const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const context = await browser.newContext({
  ...iPhone,
  hasTouch: true,
});
const page = await context.newPage();
await page.goto(`http://localhost:${port}/`);

// Wait for WASM to be ready
await page.waitForFunction(() =>
  typeof totalNodes === "function" &&
  typeof camScale === "function" &&
  typeof ensureDefaultPath === "function"
);
await assertSimpleDrawMode(page, false, "pinch no node");

// Set up default circuit
await page.evaluate(() => ensureDefaultPath());
await page.waitForTimeout(200);

let passed = 0;
let failed = 0;

function assert(condition, message) {
  if (!condition) {
    console.error(`  FAIL: ${message}`);
    failed++;
    return false;
  }
  console.log(`  PASS: ${message}`);
  passed++;
  return true;
}

console.log("Testing pinch-end does not create nodes...");

// Test 1: Pinch zoom out then release — no spurious node creation
console.log("Test 1: Pinch zoom does not create nodes on release");
{
  const nodesBefore = await page.evaluate(() => totalNodes());
  const gridY = 80; // Y in grid area (above drum view)

  await cdpPinch(page, 200, gridY, 60, 120, 8, 30);
  await page.waitForTimeout(200); // let cooldown expire

  const nodesAfter = await page.evaluate(() => totalNodes());
  assert(nodesAfter === nodesBefore,
    `Nodes unchanged after pinch (before=${nodesBefore}, after=${nodesAfter})`);
}

// Test 2: Pinch zoom in then release — no spurious node creation
console.log("Test 2: Pinch zoom in does not create nodes");
{
  const nodesBefore = await page.evaluate(() => totalNodes());
  const gridY = 80;

  await cdpPinch(page, 200, gridY, 120, 60, 8, 30);
  await page.waitForTimeout(200);

  const nodesAfter = await page.evaluate(() => totalNodes());
  assert(nodesAfter === nodesBefore,
    `Nodes unchanged after pinch-in (before=${nodesBefore}, after=${nodesAfter})`);
}

// Test 3: Two-finger pan then release — no spurious node creation
console.log("Test 3: Two-finger pan does not create nodes on release");
{
  const nodesBefore = await page.evaluate(() => totalNodes());
  const gridY = 80;

  await cdpTwoFingerPan(page, 200, gridY, 50, 30, 100, 8, 30);
  await page.waitForTimeout(200);

  const nodesAfter = await page.evaluate(() => totalNodes());
  assert(nodesAfter === nodesBefore,
    `Nodes unchanged after two-finger pan (before=${nodesBefore}, after=${nodesAfter})`);
}

// Test 4: Asymmetric pinch end (simulated by rapid pinch) — no spurious node creation
console.log("Test 4: Rapid pinch with quick lift does not create nodes");
{
  const nodesBefore = await page.evaluate(() => totalNodes());
  const gridY = 80;

  // Fast pinch with minimal delay to stress the cooldown timing
  await cdpPinch(page, 250, gridY, 40, 100, 5, 10);
  await page.waitForTimeout(300);

  const nodesAfter = await page.evaluate(() => totalNodes());
  assert(nodesAfter === nodesBefore,
    `Nodes unchanged after rapid pinch (before=${nodesBefore}, after=${nodesAfter})`);
}

// Summary
console.log(`\nResults: ${passed} passed, ${failed} failed`);

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "touch_pinch_no_node");
await browser.close();
server.close();

if (failed > 0) {
  process.exit(1);
}
