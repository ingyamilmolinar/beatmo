/** Only what Go cannot reach: localStorage really persists a favorite across a reload. COVERED-BY-GO:
 * searchable_list_favorites_test.go, favorites_test.go, userprefs/store_test.go, inst_menu_favorites_test.go */

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import {
  resolveGoBinary,
  shouldSkipWasmBuild,
} from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

// Ensure Playwright Chromium is available.
const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

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

// Static file server.
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
const context = await browser.newContext();
const page = await context.newPage();
page.on("console", (msg) => {
  const t = msg.text();
  if (msg.type() === "error" || t.includes("favorites")) {
    console.log("[PAGE]", msg.type(), t);
  }
});

const FAV_ID = "test-favorite-instrument-id";
// Single canonical localStorage key — userprefs.LocalStorageKey in Go.
const STORAGE_KEY = "beatmo.prefs";

// ─── First load: seed a favorite via the JS bridge ─────────────────────
await page.goto(`http://localhost:${port}/`);
await page.waitForFunction(() => typeof instrumentSetFavorite === "function", null, {
  timeout: 30_000,
});

// Clear any prior state from previous test runs in this storage origin.
await page.evaluate((key) => localStorage.removeItem(key), STORAGE_KEY);

const setOK = await page.evaluate((id) => instrumentSetFavorite(id, true), FAV_ID);
if (setOK !== true) {
  throw new Error(`instrumentSetFavorite returned ${setOK}, expected true`);
}

// Read-side sanity: in-process Favorites() store sees the toggle.
const isFavBefore = await page.evaluate((id) => instrumentIsFavorite(id), FAV_ID);
if (isFavBefore !== true) {
  throw new Error(`instrumentIsFavorite(${FAV_ID}) before reload = ${isFavBefore}, expected true`);
}

// Robust to an async write path.
await page.waitForFunction(
  ({ key, id }) => {
    const raw = localStorage.getItem(key);
    if (!raw) return false;
    try {
      const doc = JSON.parse(raw);
      return Array.isArray(doc.favorites) && doc.favorites.includes(id);
    } catch (_) {
      return false;
    }
  },
  { key: STORAGE_KEY, id: FAV_ID },
  { timeout: 5_000 },
);

console.log("[bridge-favorites] write verified — reloading page");

// ─── Reload: full WASM re-init ──────────────────────────────────────────
await page.reload();
await page.waitForFunction(() => typeof instrumentIsFavorite === "function", null, {
  timeout: 30_000,
});

// Loaded from localStorage during bootstrap.
const isFavAfter = await page.evaluate((id) => instrumentIsFavorite(id), FAV_ID);
if (isFavAfter !== true) {
  throw new Error(`instrumentIsFavorite(${FAV_ID}) after reload = ${isFavAfter}, expected true`);
}

// localStorage value should still be intact (untouched by reload).
const storedAfter = await page.evaluate((key) => localStorage.getItem(key), STORAGE_KEY);
if (!storedAfter || !storedAfter.includes(FAV_ID)) {
  throw new Error(
    `localStorage[${STORAGE_KEY}] after reload = ${storedAfter}, expected to contain ${FAV_ID}`,
  );
}

// Toggle off — verify removal propagates and persists.
await page.evaluate((id) => instrumentSetFavorite(id, false), FAV_ID);
await page.waitForFunction(
  ({ key, id }) => {
    const raw = localStorage.getItem(key);
    if (!raw) return true;
    try {
      const doc = JSON.parse(raw);
      return !doc.favorites || !doc.favorites.includes(id);
    } catch (_) {
      return false;
    }
  },
  { key: STORAGE_KEY, id: FAV_ID },
  { timeout: 5_000 },
);

const isFavAfterUntoggle = await page.evaluate((id) => instrumentIsFavorite(id), FAV_ID);
if (isFavAfterUntoggle !== false) {
  throw new Error(`instrumentIsFavorite(${FAV_ID}) after untoggle = ${isFavAfterUntoggle}, expected false`);
}

console.log("[bridge-favorites] OK — favorites persist across reload via localStorage");

await context.close();
await browser.close();
server.close();
process.exit(0);
