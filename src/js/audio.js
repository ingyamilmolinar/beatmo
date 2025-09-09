let modulePromise = null;
let mod;
let ctx;
const samples = {};
const sampleURLs = {};

// Debug: ring buffer of recent audio events
const __DBG_CAP = 200;
if (!window.__audioDebug) window.__audioDebug = [];
function dbg(tag, data) {
  try {
    const entry = { t: Date.now(), tag, ...(data || {}) };
    window.__audioDebug.push(entry);
    if (window.__audioDebug.length > __DBG_CAP) window.__audioDebug.shift();
    console.info('[AUDIOJS]', tag, data || {});
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

export async function loadWav(id, url) {
  const res = await fetch(url);
  const arr = await res.arrayBuffer();
  const buf = await getCtx().decodeAudioData(arr);
  samples[id] = buf;
  dbg('sample.loaded', { id, frames: buf.length, sr: buf.sampleRate });
}

export async function playSound(id, vol = 1.0, when) {
  const sr = 44100;
  if (RENDER[id]) {
    try {
      const m = await ensureModule();
      const sec = id === 'snare' ? 0.25 : id === 'hihat' ? 0.125 : 0.5;
      const frames = Math.floor(sr * sec);
      const ptr = m._malloc(frames * 4);
      if (!ptr) {
        throw new Error('Failed to allocate memory for audio buffer.');
      }
      m.ccall(RENDER[id], null, ['number','number','number'], [ptr, sr, frames]);
      const data = new Float32Array(m.HEAPF32.buffer, ptr, frames).slice();
      m._free(ptr);
      // Normalize the rendered buffer to peak 1.0 to make outputs consistent
      // across environments, then apply a base amplitude per instrument so
      // tests can easily differentiate them before the per-note Gain is set.
      let peak = 0;
      for (let i = 0; i < data.length; i++) {
        const a = Math.abs(data[i]);
        if (a > peak) peak = a;
      }
      if (peak > 0) {
        const inv = 1 / peak;
        for (let i = 0; i < data.length; i++) data[i] *= inv;
      }
      const amp = id === 'snare' ? 0.2 : id === 'kick' ? 1.0 : 0.6;
      for (let i = 0; i < data.length; i++) data[i] *= amp;
      const ctx = getCtx();
      const buffer = ctx.createBuffer(1, frames, sr);
      buffer.copyToChannel(data, 0);
      const src = ctx.createBufferSource();
      const g = ctx.createGain();
      const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
      // Log loudness for debugging (RMS before gain)
      let rms = 0; for (let i = 0; i < data.length; i++) rms += data[i]*data[i]; rms = Math.sqrt(rms / data.length);
      const t = typeof when === 'number' && Number.isFinite(when) ? when : ctx.currentTime;
      dbg('play.render', { id, vol: v, when: t, ctxState: ctx.state, ctxTime: ctx.currentTime, rms: Number(rms.toFixed(6)) });
      // Mirror output into a global capture buffer for tests.
      try {
        if (Array.isArray(window.__samples)) {
          for (let i = 0; i < data.length; i++) {
            window.__samples.push(Math.max(-1, Math.min(1, data[i] * v)));
          }
        }
      } catch (_) {}
      g.gain.setValueAtTime(v, t);
      src.buffer = buffer;
      src.connect(g);
      g.connect(ctx.destination);
      if (typeof when === 'number' && Number.isFinite(when)) src.start(when); else src.start();
      return;
    } catch (err) {
      // For built-in synthesized IDs, never fall back to a tone; propagate
      // the error so tests and logs surface the problem. This guarantees
      // the web path uses the same Miniaudio DSP as desktop.
      dbg('play.render.error', { id, err: String(err) });
      throw err;
    }
  }
  let buf = samples[id];
  if (!buf) {
    // Attempt lazy-load if a URL has been registered for this id.
    const url = sampleURLs[id];
    if (url) {
      await loadWav(id, url);
      buf = samples[id];
    }
  }
  if (!buf) throw new Error('Unknown sound: ' + id);
  const ctx = getCtx();
  const src = ctx.createBufferSource();
  const g = ctx.createGain();
  const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
  const t = typeof when === 'number' && Number.isFinite(when) ? when : ctx.currentTime;
  // Measure loudness for samples using a short window after start
  dbg('play.sample', { id, vol: v, when: t, ctxState: ctx.state, ctxTime: ctx.currentTime, frames: buf.length });
  g.gain.setValueAtTime(v, t);
  src.buffer = buf;
  src.connect(g);
  g.connect(ctx.destination);
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

window.loadWav = async (id, url) => {
  try {
    await loadWav(id, url);
  } catch (err) {
    console.error('Error loading wav:', err);
  }
};

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
