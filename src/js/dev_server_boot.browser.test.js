// The page must BOOT under `make serve`'s real server, not a Node replica: index.html fetches main.wasm before
// audio.js's imports, Chromium stops reading an unread body after a few MB, and a one-connection server then never
// answers the imports (a silent "Loading..."). Run: GO=… node src/js/dev_server_boot.browser.test.js (DEV_BOOT_ENGINES=)

import { chromium, firefox, webkit } from "playwright";
import { spawn, spawnSync } from "child_process";
import net from "net";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild } from "./browser_test_helpers.js";

const jsDir = path.dirname(fileURLToPath(import.meta.url));
const repoRoot = path.resolve(jsDir, "../..");
const goDir = path.resolve(jsDir, "../go");
const BOOT_TIMEOUT_MS = Number(process.env.DEV_BOOT_TIMEOUT_MS || 60000);
const ENGINES = { chromium, firefox, webkit };
const engineNames = (process.env.DEV_BOOT_ENGINES || "chromium,firefox,webkit").split(",").map((s) => s.trim()).filter(Boolean);

if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    resolveGoBinary(),
    ["build", "-ldflags", "-s -w -X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd/"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

async function freePort() {
  return new Promise((resolve, reject) => {
    const s = net.createServer();
    s.once("error", reject);
    s.listen(0, "127.0.0.1", () => { const { port } = s.address(); s.close(() => resolve(port)); });
  });
}

async function waitListening(port, ms = 10000) {
  const deadline = Date.now() + ms;
  while (Date.now() < deadline) {
    const ok = await new Promise((resolve) => {
      const c = net.connect(port, "127.0.0.1", () => { c.end(); resolve(true); });
      c.once("error", () => resolve(false));
    });
    if (ok) return;
    await new Promise((r) => setTimeout(r, 100));
  }
  throw new Error(`serve_nocache.py never listened on ${port}`);
}

// Exactly what `make serve` runs (Makefile: cd src/js && python3 ../../scripts/serve_nocache.py 8080).
const port = await freePort();
const server = spawn("python3", [path.join(repoRoot, "scripts/serve_nocache.py"), String(port)], { cwd: jsDir, stdio: "ignore" });
const failures = [];
let checks = 0;
function check(ok, msg) {
  checks++;
  if (!ok) { failures.push(msg); console.error(`  FAIL ${msg}`); } else console.log(`  ok   ${msg}`);
}

// Which boot milestone it stopped at and which requests never completed.
async function bootState(page) {
  return page.evaluate(() => ({
    boot: window.__beatmoBoot || null,
    audioJsRan: typeof window.audioReady !== "undefined",
    readyState: document.readyState,
  })).catch((e) => ({ error: String(e) }));
}

try {
  await waitListening(port);
  for (const name of engineNames) {
    const engine = ENGINES[name];
    if (!engine) throw new Error(`unknown engine ${name}`);
    console.log(`\n== ${name}`);
    const browser = await engine.launch();
    try {
      for (const variant of [
        { label: "dev page", query: "", landing: false },
        { label: "?landing=1", query: "?landing=1", landing: true },
      ]) {
        const page = await browser.newPage();
        const pending = new Set();
        page.on("request", (r) => pending.add(r));
        page.on("requestfinished", (r) => pending.delete(r));
        page.on("requestfailed", (r) => pending.delete(r));
        const errors = [];
        page.on("pageerror", (e) => errors.push(e.message));
        // "commit", not "load": a hung boot never fires load, and the diagnosis is the point.
        await page.goto(`http://127.0.0.1:${port}/${variant.query}`, { waitUntil: "commit" });
        const ready = variant.landing
          ? () => document.documentElement.hasAttribute("data-landing") &&
              !!(window.__beatmoBoot && window.__beatmoBoot.firstFrame) &&
              document.getElementById("lc-play")?.classList.contains("ready")
          : () => !!(window.__beatmoBoot && window.__beatmoBoot.firstFrame);
        let booted = true;
        try {
          await page.waitForFunction(ready, null, { timeout: BOOT_TIMEOUT_MS, polling: 250 });
        } catch (_) {
          booted = false;
        }
        const st = await bootState(page);
        const stuck = [...pending].map((r) => new URL(r.url()).pathname);
        check(booted, `${name} ${variant.label}: boots under \`make serve\` within ${BOOT_TIMEOUT_MS} ms` +
          (booted ? ` (firstFrame ${Math.round(st.boot?.firstFrame ?? -1)} ms)` :
            ` -- stuck at ${JSON.stringify(st)}; requests never answered: ${JSON.stringify(stuck)}`));
        const landing = await page.evaluate(() => document.documentElement.hasAttribute("data-landing")).catch(() => null);
        check(landing === variant.landing, variant.landing
          ? `${name} ${variant.label}: the landing is shown`
          : `${name} ${variant.label}: an unstamped dev page shows no landing`);
        check(errors.length === 0, `${name} ${variant.label}: no page errors ${errors.length ? JSON.stringify(errors) : ""}`);
        await page.close();
      }
    } finally {
      await browser.close();
    }
  }
} finally {
  server.kill();
}

if (failures.length) {
  console.error(`\ndev_server_boot: ${failures.length}/${checks} checks FAILED`);
  for (const f of failures) console.error(`  - ${f}`);
  process.exit(1);
}
console.log(`\ndev_server_boot: all ${checks} checks passed`);
