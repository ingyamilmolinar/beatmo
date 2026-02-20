import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(__dirname, "..", "..");

/**
 * @deprecated Use server.listen(0) + server.address().port instead.
 * All browser tests now use OS-assigned ports to avoid EADDRINUSE collisions.
 */
export function getTestPort(defaultBase = 8500, randomRange = 1000) {
  if (process.env.BROWSER_TEST_PORT_BASE) {
    return parseInt(process.env.BROWSER_TEST_PORT_BASE, 10);
  }
  return defaultBase + Math.floor(Math.random() * randomRange);
}

/**
 * Check if WASM was pre-built. Returns true if build should be skipped.
 * Throws if WASM_PREBUILT=1 but the target file is missing.
 */
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
