// Synth-tab Save As "over" through the WASM bridge leaves the param block audio.js
// renders from bit-identical: that block is all a browser voice reads.

// COVERED-BY-GO: internal/ui/synth_panel_save_persist_test.go, save_as_only_test.go

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

const chromiumPath = path.join(jsDir, "node_modules", ".cache", "ms-playwright", "chromium");
if (!fs.existsSync(chromiumPath)) {
  spawnSync("npx", ["playwright", "install", "chromium"], { cwd: jsDir, stdio: "inherit" });
}

const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

if (!shouldSkipWasmBuild("play_ui.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-o", path.join(jsDir, "play_ui.wasm"), "./internal/ui/playtest"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
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

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
page.on("console", (msg) => {
  try {
    if (msg.type() === "error") console.log("[PAGE]", msg.type(), msg.text());
  } catch (_) {}
});

const failures = [];
function check(cond, msg) {
  if (!cond) failures.push(msg);
}
const approx = (a, b, eps = 1e-6) => Math.abs(a - b) <= eps;

try {
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof ensureDefaultPath === "function");
  await page.evaluate(() => ensureDefaultPath());
  await page.waitForFunction(() => typeof saveActiveRecipe === "function");

  const INST = "kick";
  const K = "post_decay";
  // One evaluate, so the numeric values cross the bridge (callExport only returns a shape).
  const r = await page.evaluate(({ inst, k }) => {
    const blockOf = () => Array.from((__instrumentBlockForTest(inst) || {}).block || []);
    const orig = getInstrumentParams(inst)[k];
    const liked = orig > 0.5 ? 0.3 : 0.8;

    setInstrumentParam(inst, k, liked);
    const beforeSave = getInstrumentParams(inst)[k];
    const blockBeforeSave = blockOf();

    const savedID = saveActiveRecipe(inst);
    const afterSave = getInstrumentParams(inst)[k];
    const blockAfterSave = blockOf();
    const doc = JSON.parse(getInstrumentDoc(inst) || "{}");

    return {
      orig, liked, beforeSave, afterSave, savedID,
      origin: doc.origin || "",
      blockLen: blockBeforeSave.length,
      blockSame: blockBeforeSave.length > 0 && blockBeforeSave.length === blockAfterSave.length &&
        blockBeforeSave.every((v, i) => Object.is(v, blockAfterSave[i])),
    };
  }, { inst: INST, k: K });

  check(typeof r.orig === "number", `${INST} has no ${K} value`);
  check(approx(r.beforeSave, r.liked), `pre-save ${K}=${r.beforeSave}, want ${r.liked}`);
  check(r.savedID === INST, `saveActiveRecipe returned ${r.savedID}, want ${INST}`);
  check(approx(r.afterSave, r.liked), `post-save ${K}=${r.afterSave}, want ${r.liked}`);
  check(r.blockLen > 0, "no pushed block for the instrument (setInstrumentBlock never ran?)");
  check(r.blockSame, "Save changed the pushed param block — the browser voice would move");
  check(r.origin === INST, `saving over repointed the project copy to ${r.origin}, want ${INST}`);

  if (failures.length) {
    console.error("[synth-save-persist] FAILURES:\n  " + failures.join("\n  "));
    process.exitCode = 1;
  } else {
    console.log("[synth-save-persist] OK — Save As over leaves the pushed block untouched through the WASM bridge");
  }
} finally {
  await browser.close();
  server.close();
}
