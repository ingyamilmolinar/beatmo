/**
 * The modular param block is DOUBLE (config-only engine refactor, Phase A —
 * docs/superpowers/plans/2026-10-02-config-only-synth-engine.md).
 *
 * C reads `modular_params` as 8-byte doubles, so BOTH JS render paths must
 * write the block as a Float64Array into HEAPF64 at an 8-byte-aligned pointer:
 * the off-thread worker (synth_render_worker.js) and the synchronous fallback
 * (audio.js renderVoiceSync). A path left on Float32Array/HEAPF32 hands C a
 * reinterpreted block — wrong variant, wrong pitch, silence or NaN.
 *
 * For a spread of shipped modular voices this asserts:
 *   1. the render is finite and audible (peak in a sane range), and
 *   2. the worker render and the forced-sync render are BIT-IDENTICAL.
 * The native side is pinned by Go's TestModularParamsAreDouble + the render
 * parity tool; cross-platform agreement by xplat_audio_compare.
 */
import * as pw from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

const BOOT_TIMEOUT_MS = 90000;
const IDS = ["kick-boom-bap", "kick-fat", "kick-acoustic", "cello", "fm-bell", "snare-punch", "hihat"];

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

// Wait for the circuit warm-up gate: recipe defaults are seeded to the platform
// asynchronously at boot, and a render that races the seed differs from every
// later one (measured: ride's first render != its steady state in BOTH paths).
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
    // A failed render (e.g. a stale drums.single.js glue with no HEAPF64) now
    // THROWS — there is no substitute voice — so a capture that returned
    // rendered for real. Still require each path to be the one asked for.
    if (worker.viaWorker !== true) fail(`${id}: worker run did not render on the worker (viaWorker=${worker.viaWorker})`);
    if (sync.viaWorker !== false) fail(`${id}: forced-sync run rendered on the worker (viaWorker=${sync.viaWorker})`);
    if (worker.hash && sync.hash && worker.hash !== sync.hash) {
      fail(`${id}: worker render ${worker.hash} != sync render ${sync.hash} — the two paths write the param block differently`);
    }
  }
  // Every voice is a different instrument: identical renders mean they all fell
  // back to one default voice.
  const hashes = res.map((r) => r.worker.hash);
  if (new Set(hashes).size !== hashes.length) fail(`instrument renders are not all distinct: ${hashes.join(", ")}`);
  // The loaded C engine was compiled with the layout audio.js marshals: a glue
  // built from an older header reads every later field at a shifted offset.
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
