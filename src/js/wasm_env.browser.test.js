// window.__beatmoEnv must reach Go's os.Getenv in the browser: wasm_exec.js serializes go.env into linear memory at run(),
// and an empty env silently disabled every env knob. Boots the REAL index.html and reads through __envProbe.

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// The real bootstrap, not a stand-in.
if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

function contentType(file) {
  if (file.endsWith(".html")) return "text/html; charset=utf-8";
  if (file.endsWith(".js")) return "application/javascript; charset=utf-8";
  if (file.endsWith(".wasm")) return "application/wasm";
  return "application/octet-stream";
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url.split("?")[0];
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    res.writeHead(200, { "Content-Type": contentType(filePath) });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

try {
  const page = await browser.newPage();

  // Before any page script, so __beatmoEnv exists when go.env is filled.
  await page.addInitScript(() => {
    window.__beatmoEnv = { BEATMO_WASM_ENV_PROBE: "hello-from-js" };
  });

  await page.goto(`http://localhost:${port}/`);

  // Registered synchronously during Game construction, before the first frame.
  await page.waitForFunction(() => typeof window.__envProbe === "function", {}, { timeout: 30000 });

  const got = await page.evaluate(() => window.__envProbe("BEATMO_WASM_ENV_PROBE"));
  if (got !== "hello-from-js") {
    throw new Error(`envProbe returned ${JSON.stringify(got)}, want "hello-from-js"`);
  }

  const missing = await page.evaluate(() => window.__envProbe("BEATMO_WASM_ENV_ABSENT"));
  if (missing !== "") {
    throw new Error(`absent var returned ${JSON.stringify(missing)}, want ""`);
  }

  await page.close();
} finally {
  await browser.close();
  server.close();
}

console.log("wasm_env: __beatmoEnv reaches Go's os.Getenv via go.env — PASS");
