// chain_spec_parity.browser.test.js
//
// The browser's LIVE audio graph is built as the engine specifies
// (chain_spec.gen.js, generated from Go's chain_spec.go):
//
//   - the AudioContext runs at CHAIN_SPEC.sampleRate — the rate desktop runs
//     at and every voice renders at;
//   - the C stages after the voice (insert effects, EQ, the master soft clip)
//     run on the beatmo-dsp AudioWorklet;
//   - where AudioWorklet is unavailable (an insecure origin) the same C runs on
//     the main-thread fallback host, still ending in the soft clip.
//
// What those stages compute is compared with desktop, sample by sample, by
// xplat_audio_stages.browser.test.js; this test pins the live wiring in a page
// with no Go (only audio.js), in Chromium and WebKit.
//
// Usage: node src/js/chain_spec_parity.browser.test.js

import { chromium, webkit } from "playwright";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { CHAIN_SPEC } from "./chain_spec.gen.js";

const jsDir = path.dirname(fileURLToPath(import.meta.url));

const server = http.createServer((req, res) => {
  if (req.url.startsWith("/test.html")) {
    const forceSP = req.url.includes("host=sp");
    res.writeHead(200, { "Content-Type": "text/html" });
    res.end(`<!DOCTYPE html><html><body><script type="module">
  ${forceSP ? "window.__beatmoDSPHost = 'scriptprocessor';" : ""}
  import('./audio.js').then(() => { window.__ready = true; });
</script></body></html>`);
    return;
  }
  const filePath = path.join(jsDir, req.url.replace(/^\//, "").split("?")[0]);
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    res.writeHead(200, { "Content-Type": filePath.endsWith(".js") ? "application/javascript" : "text/plain" });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, "127.0.0.1", r));
const port = server.address().port;

let failed = false;
const fail = (m) => { console.log(`  FAIL: ${m}`); failed = true; };
const ok = (m) => console.log(`  OK: ${m}`);

for (const [name, bt, args] of [["chromium", chromium, ["--autoplay-policy=no-user-gesture-required"]], ["webkit", webkit, []]]) {
  const browser = await bt.launch({ args });
  try {
    for (const [host, query] of [["worklet", ""], ["scriptprocessor", "?host=sp"]]) {
      const page = await browser.newPage();
      page.on("console", (msg) => { if (process.env.TEST_LOG) console.log("[PAGE]", msg.type(), msg.text()); });
      await page.goto(`http://localhost:${port}/test.html${query}`);
      await page.waitForFunction(() => window.__ready === true, null, { timeout: 15000 });
      const cfg = await page.evaluate(() => window.getChainConfigForTest());
      console.log(`--- ${name}, ${host} host ---`);
      if (!cfg) { fail(`${name}: no AudioContext`); continue; }
      if (cfg.sampleRate !== CHAIN_SPEC.sampleRate) fail(`AudioContext at ${cfg.sampleRate} Hz, the engine runs at ${CHAIN_SPEC.sampleRate}`);
      else ok(`AudioContext at ${cfg.sampleRate} Hz`);
      if (cfg.dspHost !== host) fail(`DSP host ${cfg.dspHost}, want ${host}`);
      else ok(`C stages on the ${host} host`);
      if (cfg.masterSoftClip !== host) fail(`master soft clip on ${cfg.masterSoftClip}, want ${host}`);
      else ok("master soft clip in place");
      await page.close();
    }
  } catch (err) {
    console.error("ERROR:", err);
    failed = true;
  } finally {
    await browser.close();
  }
}
server.close();
if (failed) {
  console.log("\nCHAIN SPEC PARITY: FAIL");
  process.exit(1);
}
console.log("\nCHAIN SPEC PARITY: PASS");
