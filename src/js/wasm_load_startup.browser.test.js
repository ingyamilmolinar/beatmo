// The real build's load waterfall and TTI (window.__beatmoBoot.firstFrame: instantiated, running, exports callable,
// first full frame painted), served gzip like the deploy, which also proves instantiateStreaming works through gzip.
// Budgets: WASM_LOAD_TTI_MAX_MS, WASM_LOAD_INSTANTIATE_MAX_MS, WASM_LOAD_TRANSFER_MAX_BYTES.

import { chromium } from "playwright";
import http from "http";
import zlib from "zlib";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Transfer size is deterministic, so it is the TIGHT gate, matching the Go asset-size gate; an unstripped or
// uncompressed build exceeds it.
const TRANSFER_MAX_BYTES = Number(process.env.WASM_LOAD_TRANSFER_MAX_BYTES || 6868172);
// Timing varies wildly by hardware (a throttled software-GL box takes ~22 s for what a real machine loads in a few), so
// these are CI-safe ceilings against hangs and gross regressions; tighten per runner through the env.
const TTI_MAX_MS = Number(process.env.WASM_LOAD_TTI_MAX_MS || 30000);
const INSTANTIATE_MAX_MS = Number(process.env.WASM_LOAD_INSTANTIATE_MAX_MS || 18000);

// Build main.wasm with production strip flags so TTI/transfer reflect prod.
if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-s -w -X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

function contentType(file) {
  if (file.endsWith(".html")) return "text/html; charset=utf-8";
  if (file.endsWith(".js")) return "application/javascript; charset=utf-8";
  if (file.endsWith(".wasm")) return "application/wasm";
  return "application/octet-stream";
}

// Server that gzip-encodes text/wasm assets, mirroring production.
const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url.split("?")[0];
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    const ct = contentType(filePath);
    const gzippable = /\.(wasm|js|html)$/.test(filePath);
    if (gzippable) {
      const body = zlib.gzipSync(data, { level: 9 });
      res.writeHead(200, {
        "Content-Type": ct,
        "Content-Encoding": "gzip",
        "Content-Length": body.length,
      });
      res.end(body);
    } else {
      res.writeHead(200, { "Content-Type": ct, "Content-Length": data.length });
      res.end(data);
    }
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const errors = [];
function hardAssert(cond, msg) {
  if (!cond) { errors.push(msg); }
}
const ms = (n) => (n == null ? "null" : n.toFixed(1) + "ms");

try {
  const page = await browser.newPage();
  await page.goto(`http://localhost:${port}/`);

  // The ceiling guards against a hung boot.
  await page.waitForFunction(
    () => window.__beatmoBoot && window.__beatmoBoot.firstFrame != null, null,
    { timeout: 30000 }
  );

  const boot = await page.evaluate(() => ({ ...window.__beatmoBoot }));

  // Compressed transfer size of main.wasm from the Resource Timing API.
  const wasmTiming = await page.evaluate(() => {
    const e = performance
      .getEntriesByType("resource")
      .find((r) => r.name.endsWith("main.wasm"));
    return e ? { encoded: e.encodedBodySize, decoded: e.decodedBodySize, dur: e.duration } : null;
  });

  // Derive the waterfall (all boot.* are performance.now() ms from navigation).
  const tti = boot.firstFrame; // navigation -> first interactive frame
  const instantiate = boot.wasmInstantiated - boot.wasmFetchStart;
  const wasmBoot = boot.firstFrame - boot.scriptStart; // wasm boot cost only

  console.log("\n=== WASM load waterfall (ms from navigation) ===");
  console.log(`  scriptStart       ${ms(boot.scriptStart)}`);
  console.log(`  wasmFetchStart    ${ms(boot.wasmFetchStart)}`);
  console.log(`  wasmInstantiated  ${ms(boot.wasmInstantiated)}   (+${ms(instantiate)} fetch+compile)`);
  console.log(`  goRun             ${ms(boot.goRun)}`);
  console.log(`  exportsReady      ${ms(boot.exportsReady)}`);
  console.log(`  firstFrame (TTI)  ${ms(boot.firstFrame)}`);
  console.log(`  --> wasm boot cost (scriptStart->firstFrame): ${ms(wasmBoot)}`);
  if (wasmTiming) {
    console.log(
      `  main.wasm transfer: encoded=${(wasmTiming.encoded / 1048576).toFixed(2)} MiB ` +
      `decoded=${(wasmTiming.decoded / 1048576).toFixed(2)} MiB ` +
      `(${(wasmTiming.decoded / Math.max(1, wasmTiming.encoded)).toFixed(2)}x, dur=${ms(wasmTiming.dur)})`
    );
  }
  console.log(`  budgets: TTI<=${TTI_MAX_MS}ms instantiate<=${INSTANTIATE_MAX_MS}ms transfer<=${(TRANSFER_MAX_BYTES / 1048576).toFixed(2)} MiB`);

  // Sanity: the waterfall must be fully populated and monotonic.
  hardAssert(boot.scriptStart != null, "boot.scriptStart not recorded");
  hardAssert(boot.wasmInstantiated != null, "boot.wasmInstantiated not recorded");
  hardAssert(boot.exportsReady != null, "boot.exportsReady not recorded");
  hardAssert(boot.firstFrame != null, "boot.firstFrame (TTI) not recorded");
  hardAssert(
    boot.scriptStart <= boot.wasmInstantiated && boot.wasmInstantiated <= boot.goRun &&
    boot.goRun <= boot.exportsReady && boot.exportsReady <= boot.firstFrame,
    `waterfall not monotonic: ${JSON.stringify(boot)}`
  );

  // Guard: gzip Content-Encoding actually engaged (encoded must be < decoded).
  hardAssert(wasmTiming != null, "no main.wasm resource timing entry");
  if (wasmTiming) {
    hardAssert(
      wasmTiming.encoded > 0 && wasmTiming.encoded < wasmTiming.decoded,
      `gzip not engaged: encoded=${wasmTiming.encoded} decoded=${wasmTiming.decoded}`
    );
  }

  // Budget gates.
  hardAssert(tti <= TTI_MAX_MS, `TTI ${ms(tti)} exceeds budget ${TTI_MAX_MS}ms (override WASM_LOAD_TTI_MAX_MS)`);
  hardAssert(instantiate <= INSTANTIATE_MAX_MS, `instantiate ${ms(instantiate)} exceeds budget ${INSTANTIATE_MAX_MS}ms (override WASM_LOAD_INSTANTIATE_MAX_MS)`);
  if (wasmTiming) {
    hardAssert(
      wasmTiming.encoded <= TRANSFER_MAX_BYTES,
      `main.wasm transfer ${(wasmTiming.encoded / 1048576).toFixed(2)} MiB exceeds budget ${(TRANSFER_MAX_BYTES / 1048576).toFixed(2)} MiB (override WASM_LOAD_TRANSFER_MAX_BYTES)`
    );
  }

  await page.close();
} finally {
  await browser.close();
  server.close();
}

if (errors.length) {
  console.error("\nwasm_load_startup: FAILED");
  for (const e of errors) console.error(`  - ${e}`);
  process.exit(1);
}
console.log("\nwasm_load_startup: all budgets passed");
