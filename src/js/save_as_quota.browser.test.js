// save_as_quota.browser.test.js
//
// A Save As the browser cannot store must fail visibly (spec
// docs/superpowers/specs/2026-10-31-versioned-objects-and-residency-revisions-design.md
// §2.4): with localStorage full, saving over an instrument reports nothing
// saved, the library stays as it was, and nothing claims success — the version
// would otherwise vanish on the next reload.
//
// Run:
//   GO=$(pwd)/.tools/go/bin/go node src/js/save_as_quota.browser.test.js
// Env: WASM_PREBUILT=1, WASM_FILE=<wasm>.

import http from "http";
import { spawnSync } from "child_process";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";
import { launchBrowser } from "./platform/launch.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();
const WASM_FILE = process.env.WASM_FILE ? path.resolve(process.env.WASM_FILE) : path.join(jsDir, "main.wasm");

if (!process.env.WASM_FILE && !shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(GO, ["build", "-o", WASM_FILE, "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" });
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const TYPES = { ".html": "text/html", ".js": "application/javascript", ".wasm": "application/wasm", ".json": "application/json" };
const server = http.createServer((req, res) => {
  const url = req.url.split("?")[0];
  const file = url === "/" ? "/index.html" : url;
  const filePath = file === "/main.wasm" ? WASM_FILE : path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    res.writeHead(200, { "Content-Type": TYPES[path.extname(filePath)] || "text/plain", "Cache-Control": "no-store" });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const failures = [];
const check = (cond, msg) => { if (!cond) failures.push(msg); console.log(`  ${cond ? "ok  " : "FAIL"} ${msg}`); };

const { browser, cleanup } = await launchBrowser();
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 720 } });
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof saveActiveRecipe === "function" && typeof rowInstrument === "function" &&
    typeof setInstrumentParam === "function", null, { timeout: 120000 });
  const r = await page.evaluate(() => {
    const id = rowInstrument(0);
    const before = localStorage.getItem("beatmo.instruments");
    // Fill the origin's quota with other keys, down to the last few bytes.
    let i = 0;
    for (const size of [256 * 1024, 16 * 1024, 1024, 64, 8]) {
      const chunk = "x".repeat(size);
      try { for (;; i++) localStorage.setItem("__fill" + i, chunk); } catch (_) { i++; /* full at this size */ }
    }
    setInstrumentParam(id, "post_decay", 0.37);
    const saved = saveActiveRecipe(id);
    const after = localStorage.getItem("beatmo.instruments");
    for (let k = 0; k <= i; k++) localStorage.removeItem("__fill" + k);
    return { id, saved, unchanged: before === after, filled: i };
  });
  check(r.filled > 0, `localStorage filled (${r.filled} chunks)`);
  check(r.saved === "", `a save the browser could not store reports nothing saved (got ${JSON.stringify(r.saved)})`);
  check(r.unchanged, "the stored library is unchanged");
} finally {
  await cleanup();
  server.close();
}

if (failures.length) {
  console.error(`\n${failures.length} failure(s):\n  ${failures.join("\n  ")}`);
  process.exit(1);
}
console.log("\nsave as quota: PASS");
