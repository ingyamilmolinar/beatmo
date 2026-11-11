/**
 * Off-main-thread synth renderer (WASM perf fix).
 *
 * The synth voice renderer (`render_modular_p`) is a synchronous
 * C function that take ~30–40 ms for a 2 s melodic / physical-model voice.
 * Running them via `ccall` on the main thread blocked the UI + audio scheduler
 * whenever a live param edit invalidated the cache during playback (see
 * webaudio_synth_param_stress.browser.test.js). This worker hosts its OWN
 * instance of the SINGLE_FILE Emscripten DSP module (drums.single.js — the
 * wasm is embedded, so no extra asset wiring) and renders off the main thread;
 * the rendered Float32Array is transferred back so the main thread never blocks.
 *
 * The render functions take ALL of their input through arguments
 * (sr / frames / a flat param block) — they read no JS-side global state — so a
 * second module instance produces the same output as the main-thread instance.
 * The main thread owns the param-ABI resolution and ships a ready-to-copy flat
 * Float32Array; this worker is a dumb renderer.
 *
 * Protocol (main → worker):
 *   { type:'render', reqId, renderFn, sr, frames, allocCount,
 *     params:Float64Array|Float32Array|null, post:object|null }
 * Reply (worker → main):
 *   { type:'rendered', reqId, data:Float32Array(frames) }   (data.buffer transferred)
 *   { type:'error',    reqId, err:string }
 *   { type:'ready' }                                          (after module init)
 *
 * post (P4): when set, the worker also applies the voice post-processing —
 * voice FX, gate tail, peak-normalise × amplitude, end fade (voice_post.js,
 * desktop's renderDocBlock step for step) — before transfer, keeping those
 * O(frames) passes off the main thread. The main-thread fallback runs the same
 * function, so both paths stay bit-identical.
 */

import { applyVoicePost } from "./voice_post.js";

let modulePromise = null;

async function getModule() {
  if (!modulePromise) {
    const glue = await import("./drums.single.js");
    modulePromise = glue.default({});
  }
  return modulePromise;
}

// Warm the module up front so the first real render is fast.
getModule()
  .then(() => { try { postMessage({ type: "ready" }); } catch (_) {} })
  .catch((e) => { try { postMessage({ type: "initerror", err: String(e) }); } catch (_) {} });

self.onmessage = async (ev) => {
  const msg = ev.data || {};
  if (msg.type !== "render") return;
  const { reqId, renderFn, sr, frames } = msg;
  let m;
  try {
    m = await getModule();
  } catch (e) {
    postMessage({ type: "error", reqId, err: "module init failed: " + String(e) });
    return;
  }
  const n = Math.max(1, frames | 0);
  let ptr = 0;
  let paramsPtr = 0;
  try {
    ptr = m._malloc(n * 4);
    if (!ptr) throw new Error("malloc(output) failed");
    // Every render passes a full param block: the engine has no defaults.
    if (!(msg.params && msg.params.length > 0)) throw new Error("no param block for " + renderFn);
    const allocCount = Math.max(msg.params.length, msg.allocCount | 0);
    // The modular param block is double (structured clone keeps the
    // Float64Array); write it through HEAPF64.
    if (!m.HEAPF64) throw new Error("drums module has no HEAPF64 — the C glue (drums.single.js) is older than the double param block; run `make wasm`");
    paramsPtr = m._malloc(allocCount * 8);
    if (!paramsPtr) throw new Error("malloc(params) failed");
    const pheap = m.HEAPF64.subarray(paramsPtr >> 3, (paramsPtr >> 3) + allocCount);
    pheap.fill(0);
    pheap.set(msg.params);
    m.ccall(renderFn, null, ["number", "number", "number", "number"], [ptr, sr, n, paramsPtr]);
    const heap = m.HEAPF32.subarray(ptr >> 2, (ptr >> 2) + n);
    const data = new Float32Array(n);
    data.set(heap);
    applyVoicePost(m, data, sr, msg.post || null);
    postMessage({ type: "rendered", reqId, data }, [data.buffer]);
  } catch (e) {
    postMessage({ type: "error", reqId, err: String(e && e.message ? e.message : e) });
  } finally {
    if (ptr) { try { m._free(ptr); } catch (_) {} }
    if (paramsPtr) { try { m._free(paramsPtr); } catch (_) {} }
  }
};
