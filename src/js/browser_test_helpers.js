import fs from "fs";
import * as childProcess from "child_process";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..", "..");

/** Throws if WASM_PREBUILT=1 but the file is missing. */
export function shouldSkipWasmBuild(wasmFile) {
  if (process.env.WASM_PREBUILT !== "1") return false;
  const wasmPath = path.resolve(__dirname, wasmFile);
  if (!fs.existsSync(wasmPath)) {
    throw new Error(`WASM_PREBUILT=1 but ${wasmFile} not found at ${wasmPath}`);
  }
  return true;
}

export function resolveGoBinary() {
  if (process.env.GO) {
    const envGo = process.env.GO;
    return path.isAbsolute(envGo) ? envGo : path.resolve(process.cwd(), envGo);
  }
  const bundled = path.resolve(repoRoot, ".tools/go/bin/go");
  return fs.existsSync(bundled) ? bundled : "go";
}

/** Rendered FRESH by cmd/export_audio against the current tree, never from a committed snapshot: a stale snapshot posed
 * as a cross-platform bug. */
export function loadDesktopAudioReference() {
  const { spawnSync } = childProcess;
  const env = { ...process.env, CGO_ENABLED: "1" };
  delete env.GOOS; // export_audio is a native host tool (cgo)
  delete env.GOARCH;
  const r = spawnSync(resolveGoBinary(), ["run", "./cmd/export_audio"], {
    cwd: path.resolve(repoRoot, "src/go"),
    env,
    encoding: "utf8",
    maxBuffer: 1 << 30,
  });
  if (r.status !== 0) {
    throw new Error(`cmd/export_audio failed (status ${r.status}): ${(r.stderr || String(r.error || "")).slice(-2000)}`);
  }
  return JSON.parse(r.stdout);
}

export async function clearSchedulerMismatches(page) {
  await page.evaluate(() => {
    if (typeof clearSchedulerMismatches === "function") {
      clearSchedulerMismatches();
    }
  });
}

export async function schedulerMismatches(page) {
  return await page.evaluate(() => {
    if (typeof recentSchedulerMismatches !== "function") return [];
    return recentSchedulerMismatches() ?? [];
  });
}

export async function assertNoSchedulerMismatches(page, label = "scheduler mismatches") {
  const mismatches = await schedulerMismatches(page);
  if (!mismatches || !mismatches.length) return;
  const sample = mismatches.slice(-10);
  throw new Error(`${label}: ${mismatches.length} mismatch(es)\n${JSON.stringify(sample, null, 2)}`);
}

// Never await audioReady over CDP: Chromium reports "Promise was collected" when engine-warm renders resolve it
// mid-await, which Playwright rewrites as "Execution context was destroyed". An in-page .then() flag polled with
// waitForFunction is reliable. A rejection throws unless allowReject is set.
export async function waitForAudioReady(page, { timeout = 30000, allowReject = false } = {}) {
  await page.waitForFunction(() => {
    const p = window.audioReady;
    if (!p || typeof p.then !== "function") return false;
    if (window.__testAudioReadyWatch?.promise !== p) {
      const w = { promise: p, settled: false, error: null };
      window.__testAudioReadyWatch = w;
      p.then(() => { w.settled = true; }, (e) => { w.error = String(e && e.message || e); w.settled = true; });
    }
    return window.__testAudioReadyWatch.settled;
  }, null, { timeout, polling: 50 });
  const error = await page.evaluate(() => window.__testAudioReadyWatch.error);
  if (error && !allowReject) throw new Error(`window.audioReady rejected: ${error}`);
}

export { flushCoverage, isCoverageEnabled, clearCoverage } from "./coverage_helpers.js";
export { isJSCoverageEnabled, startJSCoverage, stopAndWriteJSCoverage } from "./js_coverage_helpers.js";

export async function assertSimpleDrawMode(page, expected, label = "simpleDraw") {
  await page.waitForFunction(
    (want) => typeof gridCacheInfo === "function" && gridCacheInfo()?.simpleDraw === want,
    expected
  );
  const info = await page.evaluate(() => (typeof gridCacheInfo === "function" ? gridCacheInfo() : null));
  if (!info || typeof info.simpleDraw !== "boolean") {
    throw new Error(`missing gridCacheInfo.simpleDraw for ${label}`);
  }
  if (info.simpleDraw !== expected) {
    throw new Error(`expected ${label}=${expected} got ${info.simpleDraw}`);
  }
}
