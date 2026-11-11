/**
 * JavaScript source coverage helpers for browser tests.
 *
 * Collects V8 coverage via Playwright's page.coverage API and writes
 * output in V8 JSON format that c8 can consume.
 *
 * Enabled when JS_COVERAGE=1 is set.
 */
import fs from "fs";
import path from "path";

export function isJSCoverageEnabled() {
  return process.env.JS_COVERAGE === "1";
}

export async function startJSCoverage(page) {
  if (!isJSCoverageEnabled()) return;
  await page.coverage.startJSCoverage({ reportAnonymousScripts: false });
}

export async function stopAndWriteJSCoverage(page, outDir, label) {
  if (!isJSCoverageEnabled()) return;
  let entries;
  try {
    entries = await page.coverage.stopJSCoverage();
  } catch (e) {
    console.warn(`[js-coverage] Failed to stop JS coverage for ${label}: ${e.message}`);
    return;
  }
  if (!entries || entries.length === 0) return;

  // Filter to project source files only — skip wasm_exec.js, playwright internals, chrome-extensions
  const srcEntries = entries.filter(e => {
    const url = e.url || "";
    // Keep files served from our test server (localhost)
    if (!url.includes("localhost") && !url.startsWith("file://")) return false;
    // Skip known non-project files
    if (url.includes("wasm_exec.js")) return false;
    if (url.includes("node_modules")) return false;
    if (url.includes("__playwright")) return false;
    return true;
  });

  if (srcEntries.length === 0) return;

  // Write V8 coverage JSON format that c8 can consume
  const v8Coverage = {
    result: srcEntries.map(entry => ({
      scriptId: String(Math.random()).slice(2, 10),
      url: entry.url,
      functions: entry.functions || [],
    })),
  };

  fs.mkdirSync(outDir, { recursive: true });
  const outFile = path.join(outDir, `${label}.v8cov.json`);
  fs.writeFileSync(outFile, JSON.stringify(v8Coverage, null, 2));
}
