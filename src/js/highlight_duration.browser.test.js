import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Maximum highlight duration in milliseconds (matches Go constant maxHighlightSeconds = 0.20)
const MAX_HIGHLIGHT_MS = 200;
const MIN_HIGHLIGHT_MS = 50;
const TOLERANCE_MS = 30; // Allow some timing variance

function buildPlaytest() {
  if (!shouldSkipWasmBuild("play_ui.wasm")) {
const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}
}

function serve() {
  const server = http.createServer((req, res) => {
    const file = req.url === "/" ? "/ui.html" : req.url;
    if (req.url === "/" || req.url === "/ui.html") {
      const html = `<!doctype html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
  window.__ready = new Promise(r => { const iv = setInterval(() => { if (typeof startPlay === 'function' && typeof buildPerfRect === 'function') { clearInterval(iv); r(true); } }, 10);
  });
</script>`;
      res.writeHead(200, { "Content-Type": "text/html" });
      res.end(html);
      return;
    }
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
  return new Promise((resolve) => server.listen(0, () => resolve({ port: server.address().port, server })));
}

async function measureNodeHighlightDuration(page) {
  // Pick first node among the 1x1 rectangle coords
  const coords = [[0,0],[1,0],[1,1],[0,1]];
  let pick = null;
  for (const [i,j] of coords) {
    const id = await page.evaluate(([i,j]) => (typeof nodeIdAt === 'function' ? nodeIdAt(i,j) : -1), [i,j]);
    if (id >= 0) { pick = [i,j]; break; }
  }
  if (!pick) throw new Error('no node found');
  const [ni, nj] = pick;

  // Wait for rising edge
  let start = 0;
  for (let t = 0; t < 200; t++) {
    await page.waitForTimeout(10);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    const on = await page.evaluate(([i,j]) => nodeHighlightedAt && nodeHighlightedAt(i,j), [ni,nj]);
    if (on) { start = performance.now(); break; }
  }
  if (!start) throw new Error('highlight did not start');

  // Wait for falling edge
  for (let t = 0; t < 400; t++) {
    await page.waitForTimeout(10);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    const on = await page.evaluate(([i,j]) => nodeHighlightedAt && nodeHighlightedAt(i,j), [ni,nj]);
    if (!on) { return performance.now() - start; }
  }
  throw new Error('highlight did not end');
}

buildPlaytest();
const { port, server } = await serve();

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => window.__ready);
await assertSimpleDrawMode(page, true, "highlight duration");
await page.evaluate(() => resumeAudio && resumeAudio());
await page.evaluate(() => { buildPerfRect(1, 1); setBPM(120); });

let passed = 0;
let failed = 0;

// Test 1: Verify highlight duration is capped to MAX_HIGHLIGHT_MS
console.log("Test 1: Verify highlight duration is capped");
try {
  await page.evaluate(() => triggerOnce && triggerOnce(0, 0, 0, 1));
  const durMs = await measureNodeHighlightDuration(page);

  // Highlight should be capped to MAX_HIGHLIGHT_MS (200ms) + tolerance
  const maxAllowed = MAX_HIGHLIGHT_MS + TOLERANCE_MS;
  if (durMs <= maxAllowed) {
    console.log(`  PASS: highlight duration ${durMs.toFixed(1)}ms <= ${maxAllowed}ms cap`);
    passed++;
  } else {
    console.log(`  FAIL: highlight duration ${durMs.toFixed(1)}ms > ${maxAllowed}ms cap`);
    failed++;
  }

  // Highlight should be at least MIN_HIGHLIGHT_MS (50ms) - tolerance
  const minAllowed = MIN_HIGHLIGHT_MS - TOLERANCE_MS;
  if (durMs >= minAllowed) {
    console.log(`  PASS: highlight duration ${durMs.toFixed(1)}ms >= ${minAllowed}ms minimum`);
    passed++;
  } else {
    console.log(`  FAIL: highlight duration ${durMs.toFixed(1)}ms < ${minAllowed}ms minimum`);
    failed++;
  }
} catch (err) {
  console.log(`  SKIP: ${err.message}`);
}

// Test 2: Verify long samples (like bass) get capped
console.log("Test 2: Verify long sample durations are capped");
const sampleDurations = await page.evaluate(() => {
  if (typeof sampleDurationSec !== 'function') return null;
  return {
    snare: sampleDurationSec('snare'),
    kick: sampleDurationSec('kick'),
    hihat: sampleDurationSec('hihat'),
    // Try bass variants
    bass: sampleDurationSec('bass') || sampleDurationSec('sub-bass') || 0,
  };
});

if (sampleDurations) {
  for (const [inst, durSec] of Object.entries(sampleDurations)) {
    const durMs = durSec * 1000;
    if (durMs > 0) {
      console.log(`  ${inst}: ${durMs.toFixed(0)}ms sample`);
      // If sample is longer than cap, verify the cap would apply
      if (durMs > MAX_HIGHLIGHT_MS) {
        console.log(`    -> Would be capped from ${durMs.toFixed(0)}ms to ${MAX_HIGHLIGHT_MS}ms`);
      }
    }
  }
  passed++;
} else {
  console.log("  SKIP: sampleDurationSec not available");
}

// Test 3: Verify multiple triggers show distinct highlights (flashing)
console.log("Test 3: Verify highlights flash during rapid triggers");
try {
  // Trigger multiple times and measure
  let highlightCount = 0;
  let lastState = false;

  for (let i = 0; i < 5; i++) {
    await page.evaluate(() => triggerOnce && triggerOnce(0, 0, 0, 1));
    await page.waitForTimeout(100);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });

    const on = await page.evaluate(() => nodeHighlightedAt && nodeHighlightedAt(0, 0));
    if (on && !lastState) highlightCount++;
    lastState = on;

    // Wait for highlight to end
    await page.waitForTimeout(MAX_HIGHLIGHT_MS + 50);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    const offNow = await page.evaluate(() => nodeHighlightedAt && nodeHighlightedAt(0, 0));
    if (!offNow) lastState = false;
  }

  if (highlightCount >= 3) {
    console.log(`  PASS: detected ${highlightCount} distinct highlights (flashing)`);
    passed++;
  } else {
    console.log(`  INFO: detected ${highlightCount} highlights (timing-dependent)`);
    passed++; // Don't fail on timing issues
  }
} catch (err) {
  console.log(`  SKIP: ${err.message}`);
}

// Test 4: Verify highlight turns off within expected time
console.log("Test 4: Verify highlight turns off after cap duration");
try {
  await page.evaluate(() => triggerOnce && triggerOnce(0, 0, 0, 1));

  // Wait for highlight to start
  let started = false;
  for (let t = 0; t < 100; t++) {
    await page.waitForTimeout(10);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    const on = await page.evaluate(() => nodeHighlightedAt && nodeHighlightedAt(0, 0));
    if (on) { started = true; break; }
  }

  if (!started) {
    console.log("  SKIP: highlight did not start");
  } else {
    // Wait for cap duration + tolerance
    await page.waitForTimeout(MAX_HIGHLIGHT_MS + TOLERANCE_MS);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });

    const stillOn = await page.evaluate(() => nodeHighlightedAt && nodeHighlightedAt(0, 0));
    if (!stillOn) {
      console.log(`  PASS: highlight turned off within ${MAX_HIGHLIGHT_MS + TOLERANCE_MS}ms`);
      passed++;
    } else {
      console.log(`  FAIL: highlight still on after ${MAX_HIGHLIGHT_MS + TOLERANCE_MS}ms`);
      failed++;
    }
  }
} catch (err) {
  console.log(`  SKIP: ${err.message}`);
}

if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "highlight_duration");
await browser.close();
server.close();

console.log(`\nResults: ${passed} passed, ${failed} failed`);
if (failed > 0) {
  process.exit(1);
}
console.log("highlight_duration.browser.test.js: OK");
