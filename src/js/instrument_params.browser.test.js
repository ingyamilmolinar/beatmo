/** The bridge for per-instrument params: set/get, each instrument's document JSON, its knob schema, and reset. No
 * audio capture: the C render is verified natively and cross-platform elsewhere. */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
  flushCoverage,
  isCoverageEnabled,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
let snareDefs = [];

// Ensure Chromium is installed.
const chromiumPath = path.join(
  jsDir,
  "node_modules",
  ".cache",
  "ms-playwright",
  "chromium",
);
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], {
    cwd: jsDir,
    stdio: "inherit",
  });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Build WASM (skip when WASM_PREBUILT=1 and the binary already exists).
if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    {
      cwd: goDir,
      env: { ...process.env, GOOS: "js", GOARCH: "wasm" },
      stdio: "inherit",
    },
  );
  if (build.status !== 0) throw new Error("go build play_ui failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/play_ui.html" : req.url;
  const fp = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(fp, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
    let ct = "text/plain";
    if (fp.endsWith(".html")) ct = "text/html";
    else if (fp.endsWith(".js")) ct = "application/javascript";
    else if (fp.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({
  args: ["--autoplay-policy=no-user-gesture-required"],
});
const page = await browser.newPage();
page.on("console", (msg) => {
  try {
    console.log("[PAGE]", msg.type(), msg.text());
  } catch (_) {}
});

await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof setInstrumentParam === "function");
await page.waitForFunction(() => typeof getInstrumentParams === "function");
await page.waitForFunction(() => typeof getInstrumentDoc === "function");
await page.waitForFunction(() => typeof instrumentParamDefs === "function");
await page.waitForFunction(() => typeof window.setInstrumentBlock === "function");

console.log("[TEST] All synth-recipe exports present.");

// Which values a document holds is Go's; the BRIDGE owes a parseable document for every instrument the app plays.
{
  const got = await page.evaluate(() => Object.fromEntries(
    ["snare", "kick", "fm-bass", "fm-pluck-tight"].map((id) => [id, getInstrumentDoc(id)]),
  ));
  for (const [id, raw] of Object.entries(got)) {
    if (typeof raw !== "string" || raw === "") {
      throw new Error(`getInstrumentDoc(${id}) returned ${JSON.stringify(raw)}; want document JSON`);
    }
    const d = JSON.parse(raw);
    if (d.id !== id || !d.origin || !d.stages || !d.stages.core || !d.stages.finish) {
      throw new Error(`getInstrumentDoc(${id}): id=${d.id} origin=${d.origin} stages=${Object.keys(d.stages || {})}`);
    }
  }
  const none = await page.evaluate(() => getInstrumentDoc("no-such-instrument"));
  if (none !== "") throw new Error(`getInstrumentDoc(unknown) = ${JSON.stringify(none)}, want ""`);
  console.log("[TEST] getInstrumentDoc OK.");
}

// The exact set is Go's (instrument_param_defs_test.go).
{
  snareDefs = await page.evaluate(() => instrumentParamDefs("snare"));
  if (!Array.isArray(snareDefs) || snareDefs.length < 10) {
    throw new Error(`instrumentParamDefs(snare) returned ${snareDefs && snareDefs.length} defs`);
  }
  const names = new Set(snareDefs.map((p) => p.name));
  for (const name of ["gen1_snare_tune", "post_decay", "gain"]) {
    if (!names.has(name)) throw new Error(`instrumentParamDefs(snare) missing ${name}`);
  }
  // The FINISH fade, [0, 1] (1 = no fade).
  const decayDef = snareDefs.find((p) => p.name === "post_decay");
  if (decayDef.min !== 0 || decayDef.max !== 1) {
    throw new Error(`post_decay range=[${decayDef.min},${decayDef.max}], want [0,1]`);
  }
  const none = await page.evaluate(() => instrumentParamDefs("no-such-instrument"));
  if (!Array.isArray(none) || none.length !== 0) throw new Error("instrumentParamDefs(unknown) must be []");
  console.log(`[TEST] instrumentParamDefs OK (${snareDefs.length} defs).`);
}

// Every edit pushes the document's block through setInstrumentBlock.
let snareLibrary;
{
  await page.evaluate(() => resetInstrumentParams("snare"));
  snareLibrary = await page.evaluate(() => getInstrumentParams("snare"));

  // Wrapped, not replaced: the original still runs.
  await page.evaluate(() => {
    window.__synthParamUpdates = [];
    const orig = window.setInstrumentBlock;
    window.setInstrumentBlock = (id, ...rest) => {
      window.__synthParamUpdates.push({ id });
      return orig(id, ...rest);
    };
  });

  await page.evaluate(() => setInstrumentParam("snare", "amp_decay", 0.5));
  await page.evaluate(() => setInstrumentParam("snare", "drive", 0.7));

  const got = await page.evaluate(() => getInstrumentParams("snare"));
  if (got.amp_decay !== 0.5) {
    throw new Error(`getInstrumentParams.amp_decay=${got.amp_decay}, want 0.5`);
  }
  if (got.drive !== 0.7) {
    throw new Error(`getInstrumentParams.drive=${got.drive}, want 0.7`);
  }

  const updates = await page.evaluate(() => window.__synthParamUpdates);
  if (updates.length !== 2) {
    throw new Error(
      `expected 2 setInstrumentBlock pushes; got ${updates.length}`,
    );
  }
  console.log(
    `[TEST] setInstrumentParam round-trip OK (${updates.length} updates fired).`,
  );
}

// Pushed once.
{
  await page.evaluate(() => (window.__synthParamUpdates = []));
  await page.evaluate(() => resetInstrumentParams("snare"));
  const got = await page.evaluate(() => getInstrumentParams("snare"));
  if (got.amp_decay !== snareLibrary.amp_decay || got.drive !== snareLibrary.drive) {
    throw new Error(`after reset, got amp_decay=${got.amp_decay} drive=${got.drive}, want the library ${snareLibrary.amp_decay} / ${snareLibrary.drive}`);
  }
  const updates = await page.evaluate(() => window.__synthParamUpdates);
  if (updates.length !== 1) {
    throw new Error(
      `expected 1 setInstrumentBlock push on reset; got ${updates.length}`,
    );
  }
  console.log("[TEST] resetInstrumentParams OK.");
}

// Each def's default is the document's value.
{
  const vals = await page.evaluate(() => getInstrumentParams("snare"));
  for (const d of snareDefs) {
    if (d.name in vals && vals[d.name] !== d.default) {
      throw new Error(`snare ${d.name}: def default ${d.default}, document value ${vals[d.name]}`);
    }
  }
  console.log("[TEST] instrumentParamDefs defaults == document values OK.");
}

console.log("[TEST] All instrument-params bridge checks passed.");

if (isCoverageEnabled())
  await flushCoverage(
    page,
    new URL("../../coverage/browser-raw", import.meta.url).pathname,
    "instrument_params",
  );
await browser.close();
server.close();
console.log("instrument_params browser test PASSED");
