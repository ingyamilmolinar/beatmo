import { chromium } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { spawnSync } from "child_process";
import { fileURLToPath } from "url";
import { assertSimpleDrawMode, resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("main.wasm")) {
const build = spawnSync(
  GO,
  ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/..."],
  { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" }
);
if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => { const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => { if (err) { res.writeHead(404); res.end(); return; }
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

let browser;
try { browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  page.on("pageerror", (err) => console.error("[pageerror]", err));
  await page.goto(`http://localhost:${port}/`);

  await page.waitForFunction(() =>
    typeof gridCacheInfo === "function" &&
    typeof visibleRows === "function" &&
    typeof perfStats === "function"
  );
  await assertSimpleDrawMode(page, false, "ui sanity");

  // --- Assertions from ui_layout ---
  const counts = await page.evaluate(() => ({ visible: visibleRows?.(),
    content: rowsContentVisibleCount?.(),
    layout: uiLayoutOk?.(),
    grid: gridCacheInfo()
  }));
  if (typeof counts.visible !== "number" || counts.visible <= 0) {
    throw new Error(`visibleRows returned invalid value: ${counts.visible}`);
  }
  if (typeof counts.content !== "number" || counts.content <= 0) {
    throw new Error(`rowsContentVisibleCount returned invalid value: ${counts.content}`);
  }
  if (counts.layout !== true) {
    throw new Error(`uiLayoutOk reported failure: ${counts.layout}`);
  }
  if (!counts.grid || counts.grid.simpleDraw !== false) {
    throw new Error(`expected simpleDraw to disable for UI, grid info: ${JSON.stringify(counts.grid)}`);
  }

  // --- Assertions from ui_presence ---
  await page.evaluate(() => { forceDraw?.(); });
  const ok1 = await page.evaluate(() => uiLayoutOk?.());
  await page.evaluate(() => {
    // Simulate a brief pan to exercise cached row shifts.
    for (let i = 0; i < 10; i++) {
      panBy?.(-2, 0);
    }
    forceDraw?.();
  });
  const ok2 = await page.evaluate(() => uiLayoutOk?.());
  await page.evaluate(() => {
    resetPerfStats?.();
    forceDraw?.();
  });
  const stats = await page.evaluate(() => perfStats?.());

  if (!ok1 || !ok2) {
    throw new Error("uiLayoutOk returned false: drum view not drawable (web)");
  }
  if (!stats || stats.frames <= 0) {
    throw new Error("perfStats.frames missing; WASM UI did not render any frames");
  }
  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "ui_sanity");
} finally { if (browser) { 
 await browser.close(); }
  server.close();
}
console.log("ui sanity tests completed");
