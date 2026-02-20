/**
 * Coverage collection helpers for browser tests.
 *
 * When COVERAGE=1 is set, browser tests flush Go coverage data from the
 * instrumented WASM binary before closing the browser. The raw coverage
 * files are written to coverage/browser-raw/ and later merged with
 * `go tool covdata`.
 *
 * When COVERAGE is not set, all functions are no-ops.
 */

import fs from "fs";
import path from "path";
import crypto from "crypto";

/**
 * Check if coverage collection is enabled.
 * @returns {boolean}
 */
export function isCoverageEnabled() {
  return process.env.COVERAGE === "1";
}

/**
 * Flush Go coverage data from the WASM binary running in the page.
 *
 * Calls flushGoCoverage() via page.evaluate(), decodes the base64 meta
 * and counters blobs, and writes them as covmeta.<hash> and
 * covcounters.<hash>.<pid>.<ts> files that `go tool covdata` can read.
 *
 * @param {import('playwright').Page} page - Playwright page object
 * @param {string} outDir - Directory to write coverage files to
 * @param {string} label - Test label (used in filenames for traceability)
 */
export async function flushCoverage(page, outDir, label) {
  if (!isCoverageEnabled()) return;

  let data;
  try {
    data = await page.evaluate(() => {
      if (typeof flushGoCoverage !== "function") return null;
      return flushGoCoverage();
    });
  } catch (e) {
    console.warn(`[coverage] Failed to flush coverage for ${label}: ${e.message}`);
    return;
  }

  if (!data || !data.meta || !data.counters) {
    return;
  }

  const metaBuf = Buffer.from(data.meta, "base64");
  const countersBuf = Buffer.from(data.counters, "base64");

  // Hash the meta content for consistent filenames
  const hash = crypto.createHash("sha256").update(metaBuf).digest("hex").slice(0, 32);
  const pid = process.pid;
  const ts = Date.now();

  fs.mkdirSync(outDir, { recursive: true });

  const metaFile = path.join(outDir, `covmeta.${hash}`);
  const countersFile = path.join(outDir, `covcounters.${hash}.${pid}.${ts}`);

  fs.writeFileSync(metaFile, metaBuf);
  fs.writeFileSync(countersFile, countersBuf);
}

/**
 * Clear Go coverage counters in the WASM binary.
 *
 * @param {import('playwright').Page} page - Playwright page object
 * @returns {Promise<boolean>} true if counters were cleared
 */
export async function clearCoverage(page) {
  if (!isCoverageEnabled()) return false;
  try {
    return await page.evaluate(() => {
      if (typeof clearGoCoverage !== "function") return false;
      return clearGoCoverage();
    });
  } catch {
    return false;
  }
}

export { isJSCoverageEnabled, startJSCoverage, stopAndWriteJSCoverage } from "./js_coverage_helpers.js";
