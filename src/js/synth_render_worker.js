/** A render is 30-40 ms for a long voice and blocked the UI and scheduler on the main thread, so this worker hosts its OWN
 * DSP module (the wasm is embedded in the glue) and transfers the buffer back. The renderers read only their arguments,
 * so output equals the main thread's. With `post` it also runs voice_post.js, the same function as the fallback. */

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
    // Structured clone keeps the Float64Array; the block is written through HEAPF64.
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
