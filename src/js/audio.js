let modulePromise = null;
let mod;
let ctx;
const samples = {};
const sampleURLs = {};

const DEBUG_ENABLED = typeof window !== 'undefined' && !!window.TUNKUL_AUDIO_DEBUG;

// Debug: ring buffer of recent audio events
const __DBG_CAP = 200;
if (typeof window !== 'undefined' && !window.__audioDebug) window.__audioDebug = [];
function dbg(tag, data) {
  if (typeof window === 'undefined') return;
  try {
    const entry = { t: Date.now(), tag, ...(data || {}) };
    if (Array.isArray(window.__audioDebug)) {
      window.__audioDebug.push(entry);
      if (window.__audioDebug.length > __DBG_CAP) window.__audioDebug.shift();
    }
    if (DEBUG_ENABLED && console && console.info) {
      console.info('[AUDIOJS]', tag, data || {});
    }
  } catch (_) {}
}

function getCtx() {
  if (!ctx) {
    ctx = new (window.AudioContext || window.webkitAudioContext)();
    dbg('ctx.new', { sr: ctx.sampleRate, state: ctx.state });
  }
  // Try to resume if the context is suspended (common on first user gesture).
  if (ctx.state === 'suspended') {
    try { ctx.resume(); dbg('ctx.resume', { state: ctx.state }); } catch (e) { dbg('ctx.resume.error', { err: String(e) }); }
  }
  return ctx;
}

// Try to resume on first user gesture in stricter browsers.
try {
  if (typeof document !== 'undefined' && document.addEventListener) {
    document.addEventListener('pointerdown', () => {
      try { getCtx().resume(); dbg('ctx.resumed.gesture', { state: getCtx().state, t: getCtx().currentTime }); } catch(e) { dbg('ctx.gesture.error', { err: String(e) }); }
    }, { once: true, passive: true });
  }
} catch (_) {}

// Load the Emscripten module with robust fallbacks across server layouts.
async function loadDSPModule() {
  dbg('dsp.load.begin');
  try {
    // Use SINGLE_FILE build (no wasm fetch).
    const m = await import('./drums.single.js');
    const inst = await m.default({});
    dbg('dsp.load.ready', { mode: 'single-file' });
    return inst;
  } catch (e1) {
    dbg('dsp.load.try.locateFile.audio', { err: String(e1) });
    try {
      // Fallback: try non-single-file with wasm next to audio.js.
      const m = await import('./drums.single.js');
      const inst = await m.default({ locateFile: (p) => new URL(p, import.meta.url).href });
      dbg('dsp.load.ready', { mode: 'locateFile:audio' });
      return inst;
    } catch (e2) {
      dbg('dsp.load.try.locateFile.srcjs', { err: String(e2) });
      // Try wasm under /src/js/ (common when serving repo root).
      const base = (typeof window !== 'undefined' && window.location && window.location.origin) ? window.location.origin : '';
      const url = (p) => base + '/src/js/' + p;
      try {
        const m = await import('./drums.single.js');
        const inst = await m.default({ locateFile: url });
        dbg('dsp.load.ready', { mode: 'locateFile:srcjs' });
        return inst;
      } catch (e3) {
        dbg('dsp.load.error', { err: String(e3) });
        throw e3;
      }
    }
  }
}

if (!modulePromise) {
  modulePromise = loadDSPModule();
}
async function ensureModule() {
  mod = await modulePromise;
  try { dbg('dsp.exports', { hasCcall: !!mod.ccall, hasMalloc: !!mod._malloc, hasHeap: !!mod.HEAPF32 }); } catch (e) { dbg('dsp.exports.error', { err: String(e) }); }
  return mod;
}

const RENDER = {
  snare: 'render_snare',
  kick: 'render_kick',
  hihat: 'render_hihat',
  tom: 'render_tom',
  clap: 'render_clap',
};

const RENDER_INFO = {
  snare: { seconds: 0.25, amp: 0.2 },
  kick: { seconds: 0.5, amp: 1.0 },
  hihat: { seconds: 0.125, amp: 0.6 },
  tom: { seconds: 0.5, amp: 0.6 },
  clap: { seconds: 0.5, amp: 0.6 },
};

const renderCache = new Map();
// Volume buses reduce per-voice GainNode creation overhead by reusing
// a small set of GainNodes per instrument and quantized volume.
const volumeBus = new Map(); // key: `${id}|${qVol}` -> GainNode
const VOL_Q = 16;
const channelNodes = new Map(); // id -> GainNode
let mainChannelNode = null;

function clampVolume(vol) {
  if (!Number.isFinite(vol)) return 1;
  if (vol < 0) return 0;
  if (vol > 4) return 4; // allow some headroom for boosts
  return vol;
}

function ensureMainNode() {
  if (!mainChannelNode) {
    const c = getCtx();
    mainChannelNode = c.createGain();
    mainChannelNode.gain.value = 1;
    mainChannelNode.connect(c.destination);
    channelNodes.set('main', mainChannelNode);
  }
  return mainChannelNode;
}

function getChannelNode(id) {
  const key = id || 'main';
  if (key === 'main') {
    return ensureMainNode();
  }
  let node = channelNodes.get(key);
  if (!node) {
    const c = getCtx();
    node = c.createGain();
    node.gain.value = 1;
    node.connect(ensureMainNode());
    channelNodes.set(key, node);
  }
  return node;
}

export function setChannelVolume(id, vol) {
  const node = getChannelNode(id);
  node.gain.value = clampVolume(vol);
}

export function channelVolume(id) {
  const node = getChannelNode(id);
  return node.gain.value;
}

export function setMainVolume(vol) { setChannelVolume('main', vol); }
export function mainVolume() { return channelVolume('main'); }

function getBus(id, vol) {
  const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
  const q = Math.round(v * VOL_Q);
  const key = id + '|' + q;
  let bus = volumeBus.get(key);
  if (!bus) {
    const c = getCtx();
    bus = c.createGain();
    bus.gain.value = q / VOL_Q;
    bus.connect(getChannelNode(id));
    volumeBus.set(key, bus);
  }
  return bus;
}

// Warm up common synthesized instruments so hot path is synchronous.
(async () => {
  try {
    await ensureModule();
    const ids = ['snare', 'kick', 'hihat', 'tom', 'clap'];
    for (const id of ids) {
      if (RENDER[id] && !renderCache.has(id)) {
        // Fire and forget; ensures renderCache fills soon after load.
        ensureRenderedSample(id).catch(() => {});
      }
    }
  } catch (_) {}
})();

function recordSamples(data, gain) {
  try {
    if (!Array.isArray(window.__samples)) return;
    if (!window.__captureSamples) return;
    const g = Number.isFinite(gain) ? gain : 1;
    for (let i = 0; i < data.length; i++) {
      window.__samples.push(Math.max(-1, Math.min(1, data[i] * g)));
    }
  } catch (_) {}
}

async function ensureRenderedSample(id) {
  if (!RENDER[id]) {
    return null;
  }
  if (renderCache.has(id)) {
    const hit = renderCache.get(id);
    try {
      if (typeof window !== 'undefined') {
        const metrics = window.__audioMetrics || (window.__audioMetrics = { renders: {}, cacheHits: {} });
        metrics.cacheHits[id] = (metrics.cacheHits[id] || 0) + 1;
      }
    } catch (_) {}
    return hit;
  }
  const info = RENDER_INFO[id] || { seconds: 0.5, amp: 0.6 };
  const sr = 44100;
  const frames = Math.max(1, Math.floor(sr * info.seconds));
  const m = await ensureModule();
  const ptr = m._malloc(frames * 4);
  if (!ptr) throw new Error('Failed to allocate memory for audio buffer.');
  try {
    m.ccall(RENDER[id], null, ['number', 'number', 'number'], [ptr, sr, frames]);
    const heap = m.HEAPF32.subarray(ptr >> 2, (ptr >> 2) + frames);
    const data = new Float32Array(frames);
    data.set(heap);
    let peak = 0;
    for (let i = 0; i < data.length; i++) {
      const a = Math.abs(data[i]);
      if (a > peak) peak = a;
    }
    if (peak > 0) {
      const inv = 1 / peak;
      for (let i = 0; i < data.length; i++) data[i] *= inv;
    }
    const amp = Number.isFinite(info.amp) ? info.amp : 0.6;
    for (let i = 0; i < data.length; i++) data[i] *= amp;
    const bufferCtx = getCtx();
    const buffer = bufferCtx.createBuffer(1, frames, sr);
    buffer.copyToChannel(data, 0);
    const record = { buffer, data, sr };
    renderCache.set(id, record);
    samples[id] = buffer;
    try {
      if (typeof window !== 'undefined') {
        const metrics = window.__audioMetrics || (window.__audioMetrics = { renders: {}, cacheHits: {} });
        metrics.renders[id] = (metrics.renders[id] || 0) + 1;
      }
    } catch (_) {}
    return record;
  } finally {
    m._free(ptr);
  }
}

export async function loadWav(id, url) {
  const res = await fetch(url);
  const arr = await res.arrayBuffer();
  const buf = await getCtx().decodeAudioData(arr);
  samples[id] = buf;
  dbg('sample.loaded', { id, frames: buf.length, sr: buf.sampleRate });
}

export async function playSound(id, vol = 1.0, when) {
  if (RENDER[id]) {
    try {
      const record = await ensureRenderedSample(id);
      if (!record) throw new Error('render info missing for ' + id);
      const ctx = getCtx();
      const src = ctx.createBufferSource();
      const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
      const t = typeof when === 'number' && Number.isFinite(when) ? when : ctx.currentTime;
      dbg('play.render', { id, vol: v, when: t, ctxState: ctx.state, ctxTime: ctx.currentTime });
      recordSamples(record.data, v);
      src.buffer = record.buffer;
      src.connect(getBus(id, v));
      if (typeof when === 'number' && Number.isFinite(when)) src.start(when); else src.start();
      return;
    } catch (err) {
      dbg('play.render.error', { id, err: String(err) });
      throw err;
    }
  }
  let buf = samples[id];
  if (!buf) {
    const url = sampleURLs[id];
    if (url) {
      await loadWav(id, url);
      buf = samples[id];
    }
  }
  if (!buf) throw new Error('Unknown sound: ' + id);
  const ctx = getCtx();
  const src = ctx.createBufferSource();
  const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
  const t = typeof when === 'number' && Number.isFinite(when) ? when : ctx.currentTime;
  dbg('play.sample', { id, vol: v, when: t, ctxState: ctx.state, ctxTime: ctx.currentTime, frames: buf.length });
  try {
    const channel = buf.numberOfChannels > 0 ? buf.getChannelData(0) : null;
    if (channel) recordSamples(channel, v);
  } catch (_) {}
  src.buffer = buf;
  src.connect(getBus(id, v));
  if (typeof when === 'number' && Number.isFinite(when)) src.start(when); else src.start();
}

// Extended path with pitch (semitones) and duration multiplier.
export async function playSoundParams(id, vol = 1.0, pitch = 0.0, dur = 1.0, when) {
  const rate = Math.pow(2, pitch / 12) / (dur > 0 ? dur : 1);
  if (RENDER[id]) {
    const record = await ensureRenderedSample(id);
    if (record) {
      const ctx = getCtx();
      const src = ctx.createBufferSource();
      const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
      const t = typeof when === 'number' && Number.isFinite(when) ? when : ctx.currentTime;
      try { src.playbackRate.setValueAtTime(rate, t); } catch (_) { src.playbackRate.value = rate; }
      recordSamples(record.data, v);
      src.buffer = record.buffer;
      src.connect(getBus(id, v));
      if (typeof when === 'number' && Number.isFinite(when)) src.start(when); else src.start();
      return;
    }
  }
  let buf = samples[id];
  if (!buf) {
    const url = sampleURLs[id];
    if (url) { await loadWav(id, url); buf = samples[id]; }
  }
  if (!buf) throw new Error('Unknown sound: ' + id);
  const ctx = getCtx();
  const src = ctx.createBufferSource();
  const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
  const t = typeof when === 'number' && Number.isFinite(when) ? when : ctx.currentTime;
  src.buffer = buf;
  try { src.playbackRate.setValueAtTime(rate, t); } catch (_) { src.playbackRate.value = rate; }
  try {
    const channel = buf.numberOfChannels > 0 ? buf.getChannelData(0) : null;
    if (channel) recordSamples(channel, v);
  } catch (_) {}
  src.connect(getBus(id, v));
  if (typeof when === 'number' && Number.isFinite(when)) src.start(when); else src.start();
}

// Expose for Go
window.playSound = async (id, vol, when) => {
  try {
    dbg('play.call', { id, vol, when });
    await playSound(id, vol, when);
  } catch (err) {
    dbg('play.error', { id, err: String(err) });
  }
};

window.playSoundParams = async (id, vol, pitch, dur, when) => {
  try {
    dbg('play.params.call', { id, vol, pitch, dur, when });
    await playSoundParams(id, vol, pitch, dur, when);
  } catch (err) {
    dbg('play.params.error', { id, err: String(err) });
  }
};

window.setChannelVolume = (id, vol) => {
  try { setChannelVolume(id, vol); } catch (err) { dbg('channel.set.error', { id, err: String(err) }); }
};

window.channelVolume = (id) => {
  try { return channelVolume(id); } catch (err) { dbg('channel.get.error', { id, err: String(err) }); return 1; }
};

window.setMainVolume = (vol) => window.setChannelVolume('main', vol);
window.mainVolume = () => window.channelVolume('main');

// Batch scheduling API to reduce Go→JS crossings in WASM builds.
// Expects an Array of objects: { id, vol, pitch, dur, when? }.
// Uses a synchronous fast path when samples are pre-rendered/decoded.
window.playSoundsBatch = (arr) => {
  try {
    const ctx = getCtx();
    const now = ctx.currentTime;
    for (let i = 0; i < arr.length; i++) {
      const ev = arr[i] || {};
      const id = String(ev.id || '');
      if (!id) continue;
      const vol = Number.isFinite(ev.vol) ? ev.vol : 1.0;
      const pitch = Number.isFinite(ev.pitch) ? ev.pitch : 0.0;
      const dur = Number.isFinite(ev.dur) && ev.dur > 0 ? ev.dur : 1.0;
      const when = (typeof ev.when === 'number' && Number.isFinite(ev.when)) ? ev.when : now;

      const rate = Math.pow(2, pitch / 12) / dur;

      // Prefer rendered sample fast path if available.
      const rec = renderCache.get(id);
      if (rec) {
        const src = ctx.createBufferSource();
        try { src.playbackRate.setValueAtTime(rate, when); } catch (_) { src.playbackRate.value = rate; }
        src.buffer = rec.buffer;
        src.connect(getBus(id, vol));
        src.start(when);
        continue;
      }

      // Fall back to decoded WAV sample if present.
      const buf = samples[id];
      if (buf) {
        const src = ctx.createBufferSource();
        src.buffer = buf;
        try { src.playbackRate.setValueAtTime(rate, when); } catch (_) { src.playbackRate.value = rate; }
        src.connect(getBus(id, vol));
        src.start(when);
        continue;
      }

      // As a last resort, queue async path (rare after warmup).
      // Intentionally no await to keep batch call synchronous.
      playSoundParams(id, vol, pitch, dur, when);
    }
  } catch (err) {
    console.error('[AUDIOJS] playSoundsBatch error', err);
  }
};

window.loadWav = async (id, url) => {
  try {
    await loadWav(id, url);
  } catch (err) {
    console.error('Error loading wav:', err);
  }
};

// Time and resume bridges used by Go.
window.audioNow = () => {
  try { return getCtx().currentTime; } catch (_) { return 0; }
};
window.resumeAudio = () => { try { return getCtx().resume(); } catch (_) { return null; } };

// Download a JSON blob with a given filename.
window.downloadJSON = (name, text) => {
  try {
    const blob = new Blob([text], { type: 'application/json' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = name || 'tunkul-export.json';
    document.body.appendChild(a);
    a.click();
    setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 100);
  } catch (err) {
    console.error('downloadJSON failed', err);
  }
};

// Open a file picker and read a JSON file as text. Returns a Promise.
window.openJSONFile = () => new Promise((resolve) => {
  console.log('[IMPORT] openJSONFile invoked');
  try {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'application/json,.json';
    input.onchange = async () => {
      console.log('[IMPORT] input onchange fired');
      const file = input.files && input.files[0];
      if (!file) { resolve(''); return; }
      console.log('[IMPORT] reading file', file.name, file.size);
      const text = await file.text();
      resolve(text);
      setTimeout(() => input.remove(), 0);
    };
    document.body.appendChild(input);
    console.log('[IMPORT] clicking file input');
    input.click();
  } catch (err) { console.error('openJSONFile failed', err); resolve(''); }
});

// Register a sample URL for lazy loading on first use.
window.registerWav = (id, url) => {
  sampleURLs[id] = url;
};

// Expose audio context current time for Go WASM scheduling.
window.audioNow = () => getCtx().currentTime;

// Expose a resume helper for Go WASM and tests to invoke after a gesture.
window.resumeAudio = async () => {
  try { await getCtx().resume(); dbg('ctx.resumed', { state: getCtx().state, t: getCtx().currentTime }); } catch (e) { dbg('ctx.resumed.error', { err: String(e) }); }
};

// Debug helpers
window.getAudioDebug = () => (Array.isArray(window.__audioDebug) ? window.__audioDebug.slice() : []);
window.resetAudioDebug = () => { try { window.__audioDebug = []; } catch(_){} };
// Sample duration query (seconds) for instruments.
window.sampleDurationSec = (id) => {
  try {
    // Prefer synthesized record if present
    const rec = renderCache.get(id);
    if (rec && rec.buffer) {
      return rec.buffer.duration || (rec.data ? rec.data.length / (rec.sr || 44100) : 0);
    }
    // Fallback to decoded WAV buffer
    const buf = samples[id];
    if (buf && typeof buf.duration === 'number') return buf.duration;
  } catch (_) {}
  return 0;
};
// Internal metric: number of reusable volume buses.
window.__audioBusCount = () => {
  try { return volumeBus.size || 0; } catch (_) { return 0; }
};
