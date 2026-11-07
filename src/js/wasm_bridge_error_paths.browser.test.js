// Garbage into the bridge: malformed JSON to importJSON returns an error and leaves state alone; out-of-range indices
// return defaults; wrong-typed args through jsArgInt degrade, since a panic in a js.FuncOf callback kills the whole runtime.
// The bridge is re-probed after each call. Raw args[i].Int() sites still crash on strings and are not exercised.

import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "..", "go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) {
      res.writeHead(404);
      res.end();
      return;
    }
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

let failures = 0;
function check(name, cond, detail = "") {
  if (cond) {
    console.log(`✓ ${name}`);
  } else {
    console.error(`✗ ${name} ${detail}`);
    failures++;
  }
}

let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  if (process.env.TEST_LOG) {
    page.on("console", (msg) => console.log(`  [page] ${msg.text()}`));
  }
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof importJSON === "function" && typeof totalRows === "function");
  await page.waitForTimeout(300);

  const alive = () => page.evaluate(() => typeof totalRows() === "number");

  // ── 1. importJSON error contract ──────────────────────────────────────
  const before = await page.evaluate(() => totalRows());

  const malformed = await page.evaluate(() => importJSON("this is not json {"));
  check(
    "importJSON(malformed) returns non-empty error string",
    typeof malformed === "string" && malformed.length > 0,
    `got ${JSON.stringify(malformed)}`,
  );

  const emptyObj = await page.evaluate(() => importJSON("{}"));
  check(
    "importJSON('{}') returns a string without throwing",
    typeof emptyObj === "string",
    `got ${JSON.stringify(emptyObj)}`,
  );

  const noArgs = await page.evaluate(() => importJSON());
  check(
    "importJSON() with no args is a safe no-op",
    noArgs === null || noArgs === undefined,
    `got ${JSON.stringify(noArgs)}`,
  );

  const wrongShape = await page.evaluate(() =>
    importJSON(JSON.stringify({ version: 999, nodes: "nope" })),
  );
  check(
    "importJSON(wrong-shape doc) returns a string without throwing",
    typeof wrongShape === "string",
    `got ${JSON.stringify(wrongShape)}`,
  );

  const after = await page.evaluate(() => totalRows());
  check(
    "row count unchanged after rejected imports",
    before === after,
    `before=${before} after=${after}`,
  );
  check("runtime alive after importJSON abuse", await alive());

  // ── 2. Out-of-range indices ───────────────────────────────────────────
  const volHi = await page.evaluate(() => rowVolume(9999));
  check("rowVolume(9999) → 0", volHi === 0, `got ${volHi}`);
  const volNeg = await page.evaluate(() => rowVolume(-5));
  check("rowVolume(-5) → 0", volNeg === 0, `got ${volNeg}`);

  const ni = await page.evaluate(() => nodeInfo(9999, 9999));
  check("nodeInfo(9999, 9999) → null", ni === null || ni === undefined, `got ${JSON.stringify(ni)}`);

  const nid = await page.evaluate(() => nodeIdAt(9999, 9999));
  check("nodeIdAt(9999, 9999) → negative id", typeof nid === "number" && nid < 0, `got ${nid}`);

  // Queued mutation with an out-of-range row: must not crash the Update loop.
  await page.evaluate(() => openColorMenu(9999));
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);
  check("runtime alive after openColorMenu(9999)", await alive());
  await page.evaluate(() => closeColorMenu?.());

  // jsArgInt degrades to the default instead of panicking.
  await page.evaluate(() => openColorMenu("abc"));
  await page.evaluate(() => forceDraw?.());
  await page.waitForTimeout(200);
  check("runtime alive after openColorMenu('abc')", await alive());
  await page.evaluate(() => closeColorMenu?.());

  console.log(`\n[bridge-error-paths] failures=${failures}`);
} catch (err) {
  console.error("FATAL:", err);
  failures++;
} finally {
  if (browser) await browser.close();
  server.close();
}
process.exit(failures === 0 ? 0 : 1);
