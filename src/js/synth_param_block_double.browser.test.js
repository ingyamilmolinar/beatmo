/** C reads the block as doubles, so BOTH render paths (the worker and the sync fallback) must write a Float64Array at an
 * 8-byte-aligned pointer; a Float32 path hands C a reinterpreted block. Each voice must render finite and audible, the two
 * paths bit-identical. */
import * as pw from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

const BOOT_TIMEOUT_MS = 90000;
const IDS = ["kick-boom-bap", "kick-fat", "kick-acoustic", "cello", "fm-bell", "snare-punch", "hihat"];

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

// A render racing the boot-time seeding differs from every later one.
const READY = () =>
  typeof window.captureInstrumentPCM === "function" &&
  typeof window.__testCaptureSynthRender === "function" &&
  typeof window.warmupProgress === "function" && !!(window.warmupProgress() || {}).ready;

if (!buildMainWasm({ logLevel: "ERROR" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;
const browser = await pw.chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });

try {
  const page = await browser.newPage();
  const pageErrors = [];
  page.on("pageerror", (e) => pageErrors.push(String(e.message)));
  if (process.env.TEST_LOG) page.on("console", (m) => console.log("[page]", m.type(), m.text()));
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  await page.waitForFunction(READY, null, { timeout: BOOT_TIMEOUT_MS, polling: 500 });

  const res = await page.evaluate(async (ids) => {
    const voice = async (id) => {
      const r = await window.__testCaptureSynthRender(id);
      if (!r) return { id, err: "no render" };
      const pcm = window.captureInstrumentPCM(id, false);
      if (!pcm) return { id, err: "no pcm" };
      const u8 = pcm.bytes;
      let h = 0x811c9dc5;
      for (let i = 0; i < u8.length; i++) { h ^= u8[i]; h = Math.imul(h, 0x01000193) >>> 0; }
      const f = new Float32Array(u8.buffer, u8.byteOffset, u8.byteLength >> 2);
      let finite = true;
      for (let i = 0; i < f.length; i++) if (!Number.isFinite(f[i])) { finite = false; break; }
      return { id, hash: `${pcm.length}@${pcm.sr}:${h.toString(16)}`, peak: r.peak, finite,
        viaWorker: r.viaWorker };
    };
    const out = [];
    for (const id of ids) {
      window.__beatmoForceSyncRender = false;
      await voice(id); // priming render: settles any per-instrument lazy state
      const w = await voice(id);
      window.__beatmoForceSyncRender = true;
      const s = await voice(id);
      window.__beatmoForceSyncRender = false;
      out.push({ id, worker: w, sync: s });
    }
    return out;
  }, IDS);

  for (const { id, worker, sync } of res) {
    console.log(`[TEST] ${id}: worker ${worker.hash} peak=${worker.peak?.toFixed(3)} | sync ${sync.hash} peak=${sync.peak?.toFixed(3)}`);
    for (const [path, v] of [["worker", worker], ["sync", sync]]) {
      if (v.err) { fail(`${id} (${path}): ${v.err}`); continue; }
      if (!v.finite) fail(`${id} (${path}): non-finite samples — C read a garbage param block`);
      if (!(v.peak > 0.05 && v.peak <= 1.5)) fail(`${id} (${path}): peak ${v.peak} out of range — silent or exploded voice`);
    }
    // A failed render THROWS (no substitute voice); each path must also be the one asked for.
    if (worker.viaWorker !== true) fail(`${id}: worker run did not render on the worker (viaWorker=${worker.viaWorker})`);
    if (sync.viaWorker !== false) fail(`${id}: forced-sync run rendered on the worker (viaWorker=${sync.viaWorker})`);
    if (worker.hash && sync.hash && worker.hash !== sync.hash) {
      fail(`${id}: worker render ${worker.hash} != sync render ${sync.hash} — the two paths write the param block differently`);
    }
  }
  // Identical renders would mean they all fell back to one voice.
  const hashes = res.map((r) => r.worker.hash);
  if (new Set(hashes).size !== hashes.length) fail(`instrument renders are not all distinct: ${hashes.join(", ")}`);
  // A glue built from an older header reads later fields at shifted offsets.
  const abi = await page.evaluate(() => ({ n: window.__synthEngineParamCount, stale: window.__synthABIStale }));
  if (!(abi.n > 0) || abi.stale !== false) {
    fail(`engine ABI check: compiled param count ${abi.n}, stale=${abi.stale} — drums.single.js does not match synth_param_abi.gen.js`);
  }
  if (pageErrors.length) fail("page errors: " + pageErrors.join(" | "));
} finally {
  await browser.close();
  server.close();
}

if (failed) { console.error("[TEST] synth param block double: FAIL"); process.exit(1); }
console.log("[TEST] synth param block double: PASS");
