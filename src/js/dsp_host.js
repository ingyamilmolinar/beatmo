/**
 * dsp_host.js — the C stages after the voice, driven from JavaScript.
 *
 * Desktop runs these exact C functions through cgo (insert_fx_c.go,
 * mix_dsp_c.go); the browser runs them from drums.single.js in three places:
 *
 *   - the beatmo-dsp AudioWorklet (dsp_worklet.js): every channel's insert
 *     chain and EQ, and the master soft clip, on the audio thread;
 *   - a ScriptProcessor on the main thread, only where AudioWorklet does not
 *     exist (an insecure origin such as `make serve-lan`);
 *   - the voice render (voice_post.js): a document's voice FX.
 *
 * Nothing here decides a value: Go sends complete parameter sets
 * (InsertEffectInstance), complete EQ programs (BuildEQProgram) and the
 * soft-clip knee (CHAIN_SPEC); the C engine orders the parameters itself
 * (ifx_param_names). So the browser computes what desktop computes.
 *
 * Every function takes the Emscripten module `m` it runs in.
 */

// ASCII strings into / out of the C heap. (AudioWorkletGlobalScope has no
// TextEncoder in every engine, and every name here is ASCII.)
function withCString(m, s, fn) {
  const p = m._malloc(s.length + 1);
  try {
    for (let i = 0; i < s.length; i++) m.HEAPU8[p + i] = s.charCodeAt(i) & 0x7f;
    m.HEAPU8[p + s.length] = 0;
    return fn(p);
  } finally {
    m._free(p);
  }
}

function readCString(m, p) {
  let s = "";
  for (let i = p; m.HEAPU8[i] !== 0; i++) s += String.fromCharCode(m.HEAPU8[i]);
  return s;
}

const paramNameCache = new WeakMap(); // module -> Map(type -> string[] | null)

/** The parameters of an effect type in the order its C init reads them. */
export function effectParamNames(m, type) {
  let byType = paramNameCache.get(m);
  if (!byType) {
    byType = new Map();
    paramNameCache.set(m, byType);
  }
  if (!byType.has(type)) {
    const s = withCString(m, String(type), (tp) => readCString(m, m._ifx_param_names(tp)));
    byType.set(type, s ? s.split(",") : null);
  }
  return byType.get(type);
}

/**
 * A C effect instance (0 when the type is unknown or the heap is full — the
 * caller then passes audio through, like desktop's cEffect). params is the
 * effect's COMPLETE parameter set; a name it lacks reads 0, exactly as Go's
 * map lookup does.
 */
export function createEffect(m, type, sr, params) {
  const names = effectParamNames(m, type);
  if (!names) return 0;
  const vp = m._malloc(4 * Math.max(1, names.length));
  if (!vp) return 0;
  try {
    names.forEach((n, i) => { m.HEAPF32[(vp >> 2) + i] = Number(params && params[n]) || 0; });
    return withCString(m, String(type), (tp) => m._ifx_create(tp, sr, vp, names.length));
  } finally {
    m._free(vp);
  }
}

export function setEffectParam(m, h, name, value) {
  if (!h) return;
  withCString(m, String(name), (np) => m._ifx_set(h, np, value));
}

/** Two heap scratch buffers of n floats each, grown on demand. */
class Scratch {
  constructor(m) { this.m = m; this.n = 0; this.a = 0; this.b = 0; }
  ensure(n) {
    if (n <= this.n) return;
    this.free();
    this.a = this.m._malloc(4 * n);
    this.b = this.m._malloc(4 * n);
    this.n = (this.a && this.b) ? n : 0;
  }
  free() {
    if (this.a) this.m._free(this.a);
    if (this.b) this.m._free(this.b);
    this.a = this.b = 0;
    this.n = 0;
  }
}

/**
 * A channel's insert chain: C instances reconciled by uid, so a structural
 * edit keeps every surviving effect's state (desktop keeps its Go processors
 * the same way). A disabled slot keeps its instance and is skipped.
 */
export class InsertChain {
  constructor(m, sr) {
    this.m = m;
    this.sr = sr;
    this.slots = []; // { uid, type, enabled, h }
    this.scratch = new Scratch(m);
    this.refused = [];
  }

  update(chain) {
    const m = this.m;
    const live = new Map(this.slots.map((s) => [s.uid, s]));
    const next = [];
    this.refused = [];
    for (const e of Array.isArray(chain) ? chain : []) {
      let s = live.get(e.uid);
      if (s && s.type === e.type) {
        live.delete(e.uid);
      } else {
        s = { uid: e.uid, type: e.type, h: createEffect(m, e.type, this.sr, e.params) };
        if (!s.h) this.refused.push(e.type);
      }
      s.enabled = !!e.enabled;
      next.push(s);
    }
    for (const s of live.values()) if (s.h) m._ifx_destroy(s.h);
    this.slots = next;
  }

  setParam(uid, name, value) {
    const s = this.slots.find((x) => x.uid === uid);
    if (s) setEffectParam(this.m, s.h, name, value);
  }

  clear() { for (const s of this.slots) if (s.h) this.m._ifx_clear(s.h); }

  /** in → out (Float32Arrays of equal length). */
  process(input, output) {
    const m = this.m;
    const n = output.length;
    const run = this.slots.filter((s) => s.enabled && s.h);
    if (run.length === 0) { output.set(input); return; }
    this.scratch.ensure(n);
    if (!this.scratch.n) { output.set(input); return; }
    let src = this.scratch.a;
    let dst = this.scratch.b;
    m.HEAPF32.set(input, src >> 2);
    for (const s of run) {
      m._ifx_run(s.h, src, dst, n);
      [src, dst] = [dst, src];
    }
    output.set(m.HEAPF32.subarray(src >> 2, (src >> 2) + n));
  }

  destroy() {
    for (const s of this.slots) if (s.h) this.m._ifx_destroy(s.h);
    this.slots = [];
    this.scratch.free();
  }
}

/** A channel EQ running a Go-built program; same shape = retuned in place. */
export class ChannelEQ {
  constructor(m) { this.m = m; this.h = 0; this.scratch = new Scratch(m); }

  load(prog) {
    const m = this.m;
    if (!prog || prog.length === 0) {
      if (this.h) m._eq_destroy(this.h);
      this.h = 0;
      return;
    }
    const p = m._malloc(8 * prog.length);
    if (!p) return;
    try {
      m.HEAPF64.set(prog, p >> 3);
      if (this.h && m._eq_update(this.h, p, prog.length) === 1) return;
      const h = m._eq_create(p, prog.length);
      if (this.h) m._eq_destroy(this.h);
      this.h = h;
    } finally {
      m._free(p);
    }
  }

  clear() { if (this.h) this.m._eq_clear(this.h); }

  process(input, output) {
    const m = this.m;
    const n = output.length;
    if (!this.h) { output.set(input); return; }
    this.scratch.ensure(n);
    if (!this.scratch.n) { output.set(input); return; }
    m.HEAPF32.set(input, this.scratch.a >> 2);
    m._eq_run(this.h, this.scratch.a, this.scratch.b, n);
    output.set(m.HEAPF32.subarray(this.scratch.b >> 2, (this.scratch.b >> 2) + n));
  }

  destroy() {
    if (this.h) this.m._eq_destroy(this.h);
    this.h = 0;
    this.scratch.free();
  }
}

/** The master soft clip (mix_dsp.c soft_clip). */
export class SoftClip {
  constructor(m, threshold) { this.m = m; this.t = threshold; this.scratch = new Scratch(m); }
  clear() {}
  process(input, output) {
    const m = this.m;
    const n = output.length;
    this.scratch.ensure(n);
    if (!this.scratch.n) { output.set(input); return; }
    m.HEAPF32.set(input, this.scratch.a >> 2);
    m._soft_clip_run(this.scratch.a, this.scratch.b, n, this.t);
    output.set(m.HEAPF32.subarray(this.scratch.b >> 2, (this.scratch.b >> 2) + n));
  }
  destroy() { this.scratch.free(); }
}

/**
 * One DSP stage of a given kind, configured by messages:
 *   inserts  { chain: [InsertEffectInstance] } | { param: { uid, name, value } }
 *   eq       { program: Float64Array | null }
 *   softclip (configured at construction: { threshold })
 *   any      { clear: true } — ResetDSPState: forget every delay line / filter
 */
export function createStage(m, kind, sr, init, onQuiet) {
  let dsp;
  if (kind === "inserts") dsp = new InsertChain(m, sr);
  else if (kind === "eq") dsp = new ChannelEQ(m);
  else if (kind === "softclip") dsp = new SoftClip(m, init && Number.isFinite(init.threshold) ? init.threshold : 1);
  else return null;
  // Residency (audio.js): {watchQuiet: {frames, amp}} arms a watch that calls
  // onQuiet once the stage's input AND output have stayed at or below amp for
  // that many frames (counted from the arming); {watchQuiet: 0} disarms it.
  let quiet = 0;
  let watch = null;
  return {
    kind,
    message(d) {
      if (!d) return;
      if ("watchQuiet" in d) {
        watch = d.watchQuiet && d.watchQuiet.frames > 0 ? d.watchQuiet : null;
        quiet = 0;
      }
      if (d.clear) dsp.clear();
      if (kind === "inserts" && d.chain) dsp.update(d.chain);
      if (kind === "inserts" && d.param) dsp.setParam(d.param.uid, d.param.name, d.param.value);
      if (kind === "eq" && "program" in d) dsp.load(d.program);
    },
    process(input, output) {
      dsp.process(input, output);
      if (!watch) return;
      const a = watch.amp;
      let last = -1;
      for (let i = output.length - 1; i >= 0; i--) {
        if (Math.abs(input[i]) > a || Math.abs(output[i]) > a) { last = i; break; }
      }
      quiet = last < 0 ? quiet + output.length : output.length - 1 - last;
      if (quiet >= watch.frames) {
        watch = null;
        if (onQuiet) onQuiet();
      }
    },
    refused: () => (dsp.refused ? dsp.refused.slice() : []),
    destroy: () => dsp.destroy(),
  };
}

/**
 * A document's voice FX applied to a whole rendered voice, in order, each a
 * fresh C instance (desktop applyDefaultFX). fx = [{ type, params }].
 */
export function applyEffectsToBuffer(m, data, sr, fx) {
  const n = data.length;
  if (!n || !Array.isArray(fx) || fx.length === 0) return data;
  const a = m._malloc(4 * n);
  const b = m._malloc(4 * n);
  const made = [];
  try {
    if (!a || !b) throw new Error("voice FX: malloc failed");
    m.HEAPF32.set(data, a >> 2);
    let src = a;
    let dst = b;
    for (const e of fx) {
      const h = createEffect(m, e.type, sr, e.params);
      if (!h) throw new Error("voice FX: the C engine has no effect " + e.type + " — run `make wasm`");
      made.push(h);
      m._ifx_run(h, src, dst, n);
      [src, dst] = [dst, src];
    }
    data.set(m.HEAPF32.subarray(src >> 2, (src >> 2) + n));
  } finally {
    for (const h of made) m._ifx_destroy(h);
    if (a) m._free(a);
    if (b) m._free(b);
  }
  return data;
}
