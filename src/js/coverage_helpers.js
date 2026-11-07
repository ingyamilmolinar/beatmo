/** With COVERAGE=1, Go coverage is flushed from the instrumented WASM before the browser closes, into
 * coverage/browser-raw/ for `go tool covdata`; otherwise every function is a no-op. */

import fs from "fs";
import path from "path";
import crypto from "crypto";

export function isCoverageEnabled() {
  return process.env.COVERAGE === "1";
}

/** Written as covmeta.<hash> / covcounters.<hash>.<pid>.<ts>, the names `go tool covdata` reads. */
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

/** Returns true if the counters were cleared. */
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
