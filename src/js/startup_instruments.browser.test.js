// startup_instruments.browser.test.js
//
// Verifies that the WASM build includes all instruments referenced by the
// startup demo. Before the fix, the WASM instrument list only had 6-18
// instruments while the startup demo used kick-tight and ride, causing
// "missing instrument" errors in the UI.
//
// Usage: GO=/path/to/.tools/go/bin/go node src/js/startup_instruments.browser.test.js

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const repoRoot = path.resolve(jsDir, "..", "..");
const goDir = path.resolve(repoRoot, "src/go");
const GO = resolveGoBinary();

// Build WASM binary.
if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

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

let exitCode = 0;
let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

  // ========================================================================
  // Scenario 1: instrumentsList() contains all startup demo instruments
  // ========================================================================
  console.log("--- Scenario 1: WASM instrument list includes startup demo instruments ---");
  {
    const page = await browser.newPage();
    await page.goto(`http://localhost:${port}/`);

    // Wait for WASM + audio ready.
    await page.waitForFunction(() =>
      typeof instrumentsList === "function" &&
      typeof importJSON === "function" &&
      typeof totalRows === "function",
      { timeout: 30000 }
    );

    // The startup demo instruments that triggered the bug.
    const result = await page.evaluate(() => {
      const ids = instrumentsList();
      const set = new Set(ids);
      const required = ["kick-tight", "snare", "hihat", "clap", "tom", "ride"];
      const missing = required.filter(id => !set.has(id));
      return { total: ids.length, missing, ids: Array.from(ids) };
    });

    console.log(`  Total instruments: ${result.total}`);
    if (result.missing.length > 0) {
      throw new Error(`Scenario 1 FAIL: startup demo instruments missing from WASM: ${result.missing.join(", ")}`);
    }
    if (result.total < 32) {
      throw new Error(`Scenario 1 FAIL: expected at least 32 instruments, got ${result.total}`);
    }
    console.log("  PASS: all startup demo instruments present in WASM instrument list");
    await page.close();
  }

  // ========================================================================
  // Scenario 2: Startup demo import produces valid rows with known instruments
  // ========================================================================
  console.log("\n--- Scenario 2: Startup demo rows have recognized instruments ---");
  {
    const page = await browser.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() =>
      typeof instrumentsList === "function" &&
      typeof rowInstrument === "function" &&
      typeof totalRows === "function" &&
      typeof forceDraw === "function",
      { timeout: 30000 }
    );

    // The startup demo is loaded by default, so rows should already exist.
    // Verify each row's instrument is in the instrument list.
    const result = await page.evaluate(() => {
      forceDraw?.();
      const ids = new Set(instrumentsList());
      const rows = totalRows();
      const rowInfo = [];
      const missingRows = [];
      for (let r = 0; r < rows; r++) {
        const inst = rowInstrument(r);
        rowInfo.push({ row: r, instrument: inst, known: ids.has(inst) });
        if (!ids.has(inst)) {
          missingRows.push({ row: r, instrument: inst });
        }
      }
      return { rows, rowInfo, missingRows };
    });

    console.log(`  Rows: ${result.rows}`);
    for (const ri of result.rowInfo) {
      console.log(`    Row ${ri.row}: ${ri.instrument} ${ri.known ? "(OK)" : "(MISSING!)"}`);
    }

    if (result.missingRows.length > 0) {
      const details = result.missingRows.map(r => `row ${r.row}: ${r.instrument}`).join(", ");
      throw new Error(`Scenario 2 FAIL: rows with unrecognized instruments: ${details}`);
    }
    console.log("  PASS: all startup demo rows use recognized instruments");
    await page.close();
  }

  // ========================================================================
  // Scenario 3: Instrument list structural invariants
  // ========================================================================
  console.log("\n--- Scenario 3: Instrument list structural invariants ---");
  {
    const page = await browser.newPage();
    await page.goto(`http://localhost:${port}/`);

    await page.waitForFunction(() => typeof instrumentsList === "function", { timeout: 30000 });

    const result = await page.evaluate(() => {
      const ids = Array.from(instrumentsList());
      const errors = [];

      // 1. No duplicates
      const seen = new Set();
      for (const id of ids) {
        if (seen.has(id)) errors.push(`duplicate ID: ${id}`);
        seen.add(id);
      }

      // 2. All IDs are non-empty strings
      for (let i = 0; i < ids.length; i++) {
        if (typeof ids[i] !== "string" || ids[i] === "") {
          errors.push(`index ${i}: invalid ID (${JSON.stringify(ids[i])})`);
        }
      }

      // 3. Core kit occupies first 6 positions in stable order
      const coreKit = ["snare", "kick", "hihat", "tom", "clap", "cowbell"];
      for (let i = 0; i < coreKit.length; i++) {
        if (ids[i] !== coreKit[i]) {
          errors.push(`index ${i}: expected core instrument ${coreKit[i]}, got ${ids[i]}`);
        }
      }

      // 4. Minimum count
      if (ids.length < 32) {
        errors.push(`expected at least 32 instruments, got ${ids.length}`);
      }

      // 5. Built-in synths before samples: all sample-* IDs come after non-sample-* IDs
      let lastNonSample = -1;
      let firstSample = ids.length;
      for (let i = 0; i < ids.length; i++) {
        if (ids[i].startsWith("sample-")) {
          if (firstSample === ids.length) firstSample = i;
        } else {
          lastNonSample = i;
        }
      }
      if (lastNonSample >= firstSample) {
        errors.push(`non-sample instrument at index ${lastNonSample} appears after sample at index ${firstSample}`);
      }

      return { count: ids.length, errors };
    });

    console.log(`  Total instruments: ${result.count}`);
    if (result.errors.length > 0) {
      for (const e of result.errors) console.log(`  FAIL: ${e}`);
      throw new Error(`Scenario 3 FAIL: ${result.errors.length} invariant violation(s)`);
    }
    console.log("  PASS: no duplicates, core kit first, synths before samples, count >= 32");
    if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "startup_instruments");
    await page.close();
  }

  console.log("\nAll startup instrument tests passed.");
} catch (err) {
  console.error("FAIL:", err.message);
  exitCode = 1;
} finally {
  if (browser) await browser.close();
  server.close();
}

process.exit(exitCode);
