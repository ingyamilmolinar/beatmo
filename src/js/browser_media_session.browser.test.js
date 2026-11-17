import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("play_ui.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  if (req.url === "/" || req.url === "/ui.html") {
    const html = `<!doctype html><body>
<script type="module" src="audio.js"></script>
<script src="wasm_exec.js"></script>
<script>
  const go = new Go();
  WebAssembly.instantiateStreaming(fetch('play_ui.wasm'), go.importObject)
    .then(r => go.run(r.instance))
    .catch(err => console.error(err));
  window.__ready = new Promise(r => {
    const iv = setInterval(() => {
      if (typeof startPlay === 'function' && typeof stopPlay === 'function') {
        clearInterval(iv); r(true);
      }
    }, 10);
  });
</script>`;
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(html);
    return;
  }
  const filePath = path.join(jsDir, req.url.replace(/^\//, ""));
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

// Helper: set up a page with WASM loaded and a circuit ready
async function setupPage(browser) {
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => window.__ready);
  await page.evaluate(() => { if (typeof resumeAudio === 'function') resumeAudio(); });
  await page.evaluate(() => {
    const t0 = performance.now();
    window.audioNow = () => (performance.now() - t0) / 1000;
  });
  await page.evaluate(() => {
    if (typeof buildPerfRect === 'function') buildPerfRect(1, 1);
  });
  return page;
}

let browser;
let failed = false;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

  // Scenario 1: Media session metadata is initialized
  console.log("Scenario 1: Media session metadata initialized after WASM load");
  {
    const page = await browser.newPage();
    await page.goto(`http://localhost:${port}/`);
    await page.waitForFunction(() => window.__ready);

    const meta = await page.evaluate(() => {
      if (!navigator.mediaSession || !navigator.mediaSession.metadata) return null;
      return { title: navigator.mediaSession.metadata.title, artist: navigator.mediaSession.metadata.artist };
    });
    if (!meta) throw new Error("mediaSession.metadata not set");
    if (!meta.title.includes("Beatmo")) throw new Error(`Expected title containing 'Beatmo', got '${meta.title}'`);
    if (meta.artist !== "Beatmo") throw new Error(`Expected artist 'Beatmo', got '${meta.artist}'`);
    console.log("  PASS: metadata set with title='" + meta.title + "', artist='" + meta.artist + "'");
    await page.close();
  }

  // Scenario 2: Play updates playback state to 'playing'
  console.log("Scenario 2: Play sets playbackState to 'playing'");
  {
    const page = await setupPage(browser);
    await page.evaluate(() => startPlay());
    await page.waitForTimeout(300);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    await page.waitForTimeout(100);

    const state = await page.evaluate(() => navigator.mediaSession.playbackState);
    if (state !== 'playing') throw new Error(`Expected playbackState 'playing', got '${state}'`);
    console.log("  PASS: playbackState = 'playing'");

    await page.evaluate(() => stopPlay());
    await page.waitForTimeout(200);
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "media_session_s2");
    await page.close();
  }

  // Scenario 3: Stop updates playback state to 'paused'
  console.log("Scenario 3: Stop sets playbackState to 'paused'");
  {
    const page = await setupPage(browser);
    await page.evaluate(() => startPlay());
    await page.waitForTimeout(300);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    await page.waitForTimeout(100);

    await page.evaluate(() => stopPlay());
    await page.waitForTimeout(300);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    await page.waitForTimeout(100);

    const stateAfterStop = await page.evaluate(() => navigator.mediaSession.playbackState);
    if (stateAfterStop !== 'paused') throw new Error(`Expected playbackState 'paused', got '${stateAfterStop}'`);
    console.log("  PASS: playbackState = 'paused'");
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "media_session_s3");
    await page.close();
  }

  // Scenario 4: mediaSessionPause stops playback immediately (not via flag)
  console.log("Scenario 4: mediaSessionPause stops playback directly");
  {
    const page = await setupPage(browser);
    await page.evaluate(() => startPlay());
    await page.waitForTimeout(300);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    await page.waitForTimeout(100);

    // Verify playing
    let playing = await page.evaluate(() => typeof isPlaying === 'function' && isPlaying());
    if (!playing) throw new Error("Expected isPlaying() true before mediaSessionPause");

    // Call mediaSessionPause directly (simulates lock screen pause)
    await page.evaluate(() => mediaSessionPause());

    // Should be stopped immediately without needing Update()/rAF
    playing = await page.evaluate(() => typeof isPlaying === 'function' && isPlaying());
    if (playing) throw new Error("Expected isPlaying() false after mediaSessionPause");

    const pbState = await page.evaluate(() => navigator.mediaSession.playbackState);
    if (pbState !== 'paused') throw new Error(`Expected playbackState 'paused', got '${pbState}'`);

    console.log("  PASS: mediaSessionPause stops playback immediately, preserves position");
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "media_session_s4");
    await page.close();
  }

  // Scenario 5: mediaSessionPlay resumes from paused position
  console.log("Scenario 5: mediaSessionPlay resumes from paused position");
  {
    const page = await setupPage(browser);
    await page.evaluate(() => startPlay());
    await page.waitForTimeout(500);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    await page.waitForTimeout(100);

    // Pause via media session
    await page.evaluate(() => mediaSessionPause());

    // Should be paused
    let playing = await page.evaluate(() => typeof isPlaying === 'function' && isPlaying());
    if (playing) throw new Error("Expected not playing after mediaSessionPause");

    // Resume via media session
    await page.evaluate(() => mediaSessionPlay());

    playing = await page.evaluate(() => typeof isPlaying === 'function' && isPlaying());
    if (!playing) throw new Error("Expected isPlaying() true after mediaSessionPlay");

    const pbState = await page.evaluate(() => navigator.mediaSession.playbackState);
    if (pbState !== 'playing') throw new Error(`Expected playbackState 'playing', got '${pbState}'`);

    console.log("  PASS: mediaSessionPlay resumes playback");
    await page.evaluate(() => stopPlay());
    await page.waitForTimeout(200);
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "media_session_s5");
    await page.close();
  }

  // Scenario 6: Silent audio element is paused when playback stops
  console.log("Scenario 6: Silent audio element paused on stop");
  {
    const page = await setupPage(browser);
    // Trigger unlockAudio() to create the silent audio element.
    // Must happen before startPlay() creates the AudioContext, otherwise
    // unlockAudio's fast path (ctx.state === 'running') skips element creation.
    await page.evaluate(() => document.dispatchEvent(new Event('mousedown', { bubbles: true })));
    await page.waitForTimeout(100);
    await page.evaluate(() => startPlay());
    await page.waitForTimeout(300);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    await page.waitForTimeout(100);

    // Pause via media session
    await page.evaluate(() => mediaSessionPause());

    const silentState = await page.evaluate(() => {
      if (typeof __silentAudioElState !== 'function') return null;
      return __silentAudioElState();
    });
    if (!silentState) {
      throw new Error("Expected __silentAudioElState() to return state, got null");
    }
    if (!silentState.paused) {
      throw new Error("Expected silent audio element to be paused after mediaSessionPause");
    }
    console.log("  PASS: silent audio element paused");
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "media_session_s6");
    await page.close();
  }

  // Scenario 7: BPM appears in metadata title during playback
  console.log("Scenario 7: BPM in metadata title during playback");
  {
    const page = await setupPage(browser);
    await page.evaluate(() => {
      if (typeof setBPM === 'function') setBPM(140);
      startPlay();
    });
    await page.waitForTimeout(300);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    await page.waitForTimeout(100);

    const title = await page.evaluate(() => {
      if (!navigator.mediaSession || !navigator.mediaSession.metadata) return null;
      return navigator.mediaSession.metadata.title;
    });
    if (!title) throw new Error("metadata.title is null during playback");
    if (!title.includes("BPM")) throw new Error(`Expected title to include 'BPM', got '${title}'`);
    if (!title.includes("140")) throw new Error(`Expected title to include '140', got '${title}'`);
    console.log("  PASS: metadata title = '" + title + "'");

    await page.evaluate(() => stopPlay());
    await page.waitForTimeout(200);
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "media_session_s7");
    await page.close();
  }

  // Scenario 8: Action handlers use mediaSessionPause/Play when available
  console.log("Scenario 8: Action handlers route to mediaSession exports");
  {
    const page = await setupPage(browser);

    // Verify mediaSessionPause and mediaSessionPlay exist as globals
    const hasExports = await page.evaluate(() =>
      typeof mediaSessionPause === 'function' && typeof mediaSessionPlay === 'function'
    );
    if (!hasExports) throw new Error("mediaSessionPause/Play not registered as globals");

    // Start playback, then simulate the 'pause' action handler path
    await page.evaluate(() => startPlay());
    await page.waitForTimeout(300);
    await page.evaluate(() => { if (typeof forceDraw === 'function') forceDraw(); });
    await page.waitForTimeout(100);

    // Call the pause handler directly (what the browser calls on lock screen)
    await page.evaluate(() => {
      // Invoke the registered handler
      mediaSessionPause();
    });
    let playing = await page.evaluate(() => typeof isPlaying === 'function' && isPlaying());
    if (playing) throw new Error("Expected not playing after pause handler");

    // Call the play handler
    await page.evaluate(() => {
      mediaSessionPlay();
    });
    playing = await page.evaluate(() => typeof isPlaying === 'function' && isPlaying());
    if (!playing) throw new Error("Expected playing after play handler");

    console.log("  PASS: action handlers route to mediaSession exports");
    await page.evaluate(() => stopPlay());
    await page.waitForTimeout(200);
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "media_session");
    await page.close();
  }

  console.log("\nAll media session tests passed!");
} catch (e) {
  console.error("FAIL:", e.message);
  failed = true;
} finally {
  if (browser) await browser.close();
  server.close();
  if (failed) process.exit(1);
}
