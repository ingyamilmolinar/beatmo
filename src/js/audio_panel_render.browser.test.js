/** The live WASM↔WebAudio path the Go pixel test cannot reach: each audio tab's crop scene plays, and the cropped panel
 * must not be dominated by one colour (chrome drawn, data area blank).
 * COVERED-BY-GO: internal/ui/audio_panel_render_pixels_test.go (renderer given data) */

import { chromium } from "playwright";
import { PNG } from "pngjs";
import { spawnSync } from "child_process";
import path from "path";
import fs from "fs";
import { fileURLToPath } from "url";
import { startServer, initPage } from "./visual_test_helpers.js";
import { resolveGoBinary } from "./browser_test_helpers.js";
import { flushCoverage, isCoverageEnabled } from "./coverage_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "..", "go");

// "./cmd", not "./cmd/...", which builds desktop-only helpers that fail under GOOS=js.
function buildWasmSafely() {
  if (process.env.WASM_PREBUILT === "1") {
    const wasmPath = path.join(jsDir, "main.wasm");
    if (!fs.existsSync(wasmPath)) {
      throw new Error(`WASM_PREBUILT=1 but main.wasm missing at ${wasmPath}`);
    }
    return;
  }
  const GO = resolveGoBinary();
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    {
      cwd: goDir,
      env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
      stdio: "inherit",
    }
  );
  if (build.status !== 0) {
    throw new Error("go build main.wasm failed");
  }
}

// ─── Helpers ─────────────────────────────────────────────────────────

/** Colours binned to 4 bits per channel so anti-aliasing does not fragment them: a flat panel reads ≈ 1.0, one with a
 * trace < 0.97. */
function colorHistogram({ width, height, data }) {
  const total = width * height;
  if (total === 0) return { dominantPct: 1, distinctColors: 0, total: 0 };
  const buckets = new Map();
  for (let i = 0; i < data.length; i += 4) {
    // Skip transparent pixels (treat as bg).
    if (data[i + 3] < 8) continue;
    // 4-bit-per-channel quantization (16 levels each).
    const r = data[i] >> 4;
    const g = data[i + 1] >> 4;
    const b = data[i + 2] >> 4;
    const key = (r << 8) | (g << 4) | b;
    buckets.set(key, (buckets.get(key) || 0) + 1);
  }
  let max = 0;
  for (const c of buckets.values()) {
    if (c > max) max = c;
  }
  return {
    dominantPct: max / total,
    distinctColors: buckets.size,
    total,
  };
}

function extractClip(pngBuf, rect) {
  const png = PNG.sync.read(pngBuf);
  const x0 = Math.max(0, Math.floor(rect.x));
  const y0 = Math.max(0, Math.floor(rect.y));
  const x1 = Math.min(png.width, Math.floor(rect.x + rect.w));
  const y1 = Math.min(png.height, Math.floor(rect.y + rect.h));
  const w = x1 - x0;
  const h = y1 - y0;
  if (w <= 0 || h <= 0) return { width: 0, height: 0, data: new Uint8Array(0) };
  const data = new Uint8Array(w * h * 4);
  for (let row = 0; row < h; row++) {
    const srcOff = ((y0 + row) * png.width + x0) * 4;
    const dstOff = row * w * 4;
    data.set(png.data.subarray(srcOff, srcOff + w * 4), dstOff);
  }
  return { width: w, height: h, data };
}

async function getSubjectRect(page, name) {
  return page.evaluate(
    (sub) =>
      typeof subjectRectJS === "function" ? subjectRectJS(sub) : null,
    name
  );
}

async function settle(page, ms = 600) {
  // Prime a few frames so layout and caches populate, then wait for the analyser buffers to fill.
  for (let i = 0; i < 6; i++) {
    await page.evaluate(() => forceDraw?.());
    await page.waitForTimeout(40);
  }
  await page.waitForTimeout(ms);
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(60);
}

// ─── Test Cases ──────────────────────────────────────────────────────

// A blank panel reads > 0.99; one rendering data, even quietly, drops below 0.97.
const TABS = [
  { tab: "wave",     scene: "crop_eq_tab_wave",     subject: "eq_tab_wave",     dominantPctMax: 0.98 },
  { tab: "spectrum", scene: "crop_eq_tab_spectrum", subject: "eq_tab_spectrum", dominantPctMax: 0.98 },
  { tab: "levels",   scene: "crop_eq_tab_levels",   subject: "eq_tab_levels",   dominantPctMax: 0.985 },
  { tab: "chain",    scene: "crop_chain_default",   subject: "chain",           dominantPctMax: 0.98 },
];

// ─── Test ────────────────────────────────────────────────────────────

console.log("Building WASM...");
buildWasmSafely();

const { server, port } = await startServer();
const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});

let failures = 0;

// The bug can live in either layout path.
const VIEWPORTS = [
  { name: "desktop_1280x720", width: 1280, height: 720, useMobile: false },
  { name: "mobile_390x844",   width: 390,  height: 844, useMobile: true  },
];

for (const vp of VIEWPORTS) {
  console.log(`\n──────── viewport: ${vp.name} ────────`);
  const page = await initPage(browser, { width: vp.width, height: vp.height }, port);

  try {
    for (const c of TABS) {
      console.log(`\n=== ${c.tab} (${c.scene}) [${vp.name}] ===`);

      // The mobile variant picks the mobile layout path.
      const ok = await page.evaluate(
        ({ name, mobile }) => {
          if (mobile && typeof runSceneMobile === "function") {
            return runSceneMobile(name);
          }
          return typeof runScene === "function" ? runScene(name) : false;
        },
        { name: c.scene, mobile: vp.useMobile }
      );
      if (!ok) {
        console.error(`  FAIL: runScene(${c.scene}) returned falsy`);
        failures++;
        continue;
      }

      // Tells "no signal" (a real bug) from "playback never started" (a test bug).
      await settle(page, 800);
      const playing = await page.evaluate(() =>
        typeof isPlaying === "function" ? isPlaying() : null
      );
      if (playing === false) {
        console.error("  FAIL: isPlaying()=false after scene setup — playback never started");
        failures++;
        continue;
      }

      // Cropped panel screenshot.
      const rect = await getSubjectRect(page, c.subject);
      if (!rect || !rect.visible || rect.w <= 0 || rect.h <= 0) {
        console.error(`  FAIL: subjectRectJS(${c.subject}) returned ${JSON.stringify(rect)}`);
        failures++;
        continue;
      }
      console.log(`  Subject rect: ${rect.w}x${rect.h} at (${rect.x},${rect.y})`);

      const png = await page.screenshot({
        type: "png",
        clip: { x: rect.x, y: rect.y, width: rect.w, height: rect.h },
      });

      const region = extractClip(png, { x: 0, y: 0, w: rect.w, h: rect.h });
      const hist = colorHistogram(region);
      console.log(
        `  pixels=${hist.total} distinctColors=${hist.distinctColors} dominantPct=${hist.dominantPct.toFixed(4)}`
      );

      if (hist.dominantPct > c.dominantPctMax) {
        console.error(
          `  FAIL: dominantPct ${hist.dominantPct.toFixed(4)} > ${c.dominantPctMax} — panel is dominated by a single color (likely the bg fill); no data trace rendered during playback`
        );
        failures++;
        continue;
      }
      if (hist.distinctColors < 4) {
        console.error(
          `  FAIL: only ${hist.distinctColors} distinct colors — chrome+data should produce many more`
        );
        failures++;
        continue;
      }
      console.log(`  PASS (${c.tab} on ${vp.name})`);

      // Stop playback + reset for next scene.
      await page.evaluate(() => {
        try { stopPlay?.(); } catch (_) {}
      });
      await page.waitForTimeout(100);
    }
  } finally {
    if (isCoverageEnabled()) {
      try {
        await flushCoverage(
          page,
          new URL("../../coverage/browser-raw", import.meta.url).pathname,
          `audio_panel_render_${vp.name}`
        );
      } catch (_) {}
    }
    await page.close();
  }
}

await browser.close();
server.close();

if (failures > 0) {
  console.error(`\n${failures} audio-panel render check(s) failed`);
  process.exit(1);
}

console.log("\nAll audio-panel render checks passed.");
