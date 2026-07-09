/**
 * Off-main-thread synth renderer (WASM perf fix).
 *
 * The synth voice renderers (`render_modular_p`, `render_X`, …) are synchronous
 * C functions that take ~30–40 ms for a 2 s melodic / physical-model voice.
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
 *     params:Float32Array|null, postAmp:number|null }
 * Reply (worker → main):
 *   { type:'rendered', reqId, data:Float32Array(frames) }   (data.buffer transferred)
 *   { type:'error',    reqId, err:string }
 *   { type:'ready' }                                          (after module init)
 *
 * postAmp (P4): when finite, the worker also peak-normalizes then amp-scales
 * the buffer before transfer, keeping those O(frames) passes off the main
 * thread. The loops mirror audio.js's sync-fallback post-processing EXACTLY
 * (same float32 op order) so both paths stay bit-identical.
 */

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
    if (msg.params && msg.params.length > 0) {
      const allocCount = Math.max(msg.params.length, msg.allocCount | 0);
      paramsPtr = m._malloc(allocCount * 4);
      if (!paramsPtr) throw new Error("malloc(params) failed");
      const pheap = m.HEAPF32.subarray(paramsPtr >> 2, (paramsPtr >> 2) + allocCount);
      pheap.fill(0);
      pheap.set(msg.params);
      m.ccall(renderFn, null, ["number", "number", "number", "number"], [ptr, sr, n, paramsPtr]);
    } else {
      m.ccall(renderFn, null, ["number", "number", "number"], [ptr, sr, n]);
    }
    const heap = m.HEAPF32.subarray(ptr >> 2, (ptr >> 2) + n);
    const data = new Float32Array(n);
    data.set(heap);
    if (typeof msg.postAmp === "number" && Number.isFinite(msg.postAmp)) {
      let peak = 0;
      for (let i = 0; i < data.length; i++) {
        const a = Math.abs(data[i]);
        if (a > peak) peak = a;
      }
      if (peak > 0) {
        const inv = 1 / peak;
        for (let i = 0; i < data.length; i++) data[i] *= inv;
      }
      for (let i = 0; i < data.length; i++) data[i] *= msg.postAmp;
    }
    postMessage({ type: "rendered", reqId, data }, [data.buffer]);
  } catch (e) {
    postMessage({ type: "error", reqId, err: String(e && e.message ? e.message : e) });
  } finally {
    if (ptr) { try { m._free(ptr); } catch (_) {} }
    if (paramsPtr) { try { m._free(paramsPtr); } catch (_) {} }
  }
};
