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
  if (!DEBUG_ENABLED) return;
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

// Track active AudioBufferSourceNodes so mute nodes can stop in-flight sounds.
// Map: instrument/sample id -> Set<AudioBufferSourceNode>
const activeSources = new Map();

function trackSource(id, src) {
  if (!id || !src) return;
  let set = activeSources.get(id);
  if (!set) {
    set = new Set();
    activeSources.set(id, set);
  }
  set.add(src);
  const cleanup = (ev) => {
    try { set.delete(src); } catch (_) {}
    if (set.size === 0) {
      try { activeSources.delete(id); } catch (_) {}
    }
    return ev;
  };
  try {
    if (typeof src.addEventListener === 'function') {
      src.addEventListener('ended', cleanup, { once: true });
      return;
    }
  } catch (_) {}
  const prev = src.onended;
  src.onended = (ev) => {
    try { cleanup(ev); } catch (_) {}
    try { if (typeof prev === 'function') prev(ev); } catch (_) {}
  };
}

const MAX_SAMPLES = 2048;
const HISTORY_LIMIT = 180;
const SCHEDULE_WARMUP_SEC = 0.5;
const SMALL_LEAD_SEC = 0.003;

const scheduleMetrics = {
  reset() {
    this.count = 0;
    this.sumLead = 0;
    this.minLead = Infinity;
    this.maxLead = -Infinity;
    this.overdue = 0;
    this.worstLag = 0;
    this.smallLeadCount = 0;
    this.leadSamples = [];
    this.sumLag = 0;
    this.maxLag = 0;
    this.lagSamples = [];
    this.history = new Map();
    this.historyOrder = [];
    this.startAt = null;
  },
  observe(now, when, lead, lag) {
    if (!Number.isFinite(lead)) return;
    if (this.startAt == null) {
      this.startAt = now;
    }
    if (now - this.startAt < SCHEDULE_WARMUP_SEC) {
      return;
    }
    if (this.count === 0) {
      this.minLead = this.maxLead = lead;
      this.worstLag = 0;
    } else {
      if (lead < this.minLead) this.minLead = lead;
      if (lead > this.maxLead) this.maxLead = lead;
    }
    this.count += 1;
    this.sumLead += lead;
    if (lead < 0) {
      this.overdue += 1;
      if (lead < this.worstLag) this.worstLag = lead;
    } else if (lead < SMALL_LEAD_SEC) {
      this.smallLeadCount += 1;
    }
    pushSample(this.leadSamples, lead);

    const lagVal = Number.isFinite(lag) ? Math.max(0, lag) : 0;
    if (lagVal > 0) {
      this.sumLag += lagVal;
      if (lagVal > this.maxLag) this.maxLag = lagVal;
      pushSample(this.lagSamples, lagVal);
    }

    const sec = Math.floor(now);
    let bucket = this.history.get(sec);
    if (!bucket) {
      bucket = {
        sec,
        count: 0,
        sumLead: 0,
        leadSamples: [],
        sumLag: 0,
        lagSamples: [],
      };
      this.history.set(sec, bucket);
      this.historyOrder.push(sec);
      if (this.historyOrder.length > HISTORY_LIMIT) {
        const oldSec = this.historyOrder.shift();
        this.history.delete(oldSec);
      }
    }
    bucket.count += 1;
    bucket.sumLead += lead;
    pushSample(bucket.leadSamples, lead);
    if (lagVal > 0) {
      bucket.sumLag += lagVal;
      pushSample(bucket.lagSamples, lagVal);
    }
  },
  snapshot() {
    const history = this.historyOrder.map((sec) => {
      const bucket = this.history.get(sec);
      if (!bucket) return null;
      const avgLead = bucket.count ? bucket.sumLead / bucket.count : null;
      const avgLag = bucket.lagSamples.length ? bucket.sumLag / bucket.lagSamples.length : 0;
      return {
        sec,
        count: bucket.count,
        avgLead,
        leadP90: percentile(bucket.leadSamples, 0.9),
        leadP99: percentile(bucket.leadSamples, 0.99),
        avgLag,
        lagP90: percentile(bucket.lagSamples, 0.9),
        lagP99: percentile(bucket.lagSamples, 0.99),
      };
    }).filter(Boolean);

    return {
      count: this.count,
      minLead: this.count ? this.minLead : null,
      maxLead: this.count ? this.maxLead : null,
      avgLead: this.count ? this.sumLead / this.count : null,
      overdue: this.overdue,
      worstLag: this.worstLag,
      smallLeadCount: this.smallLeadCount,
      leadP90: percentile(this.leadSamples, 0.9),
      leadP99: percentile(this.leadSamples, 0.99),
      avgLag: this.lagSamples.length ? this.sumLag / this.lagSamples.length : 0,
      maxLag: this.maxLag,
      lagP90: percentile(this.lagSamples, 0.9),
      lagP99: percentile(this.lagSamples, 0.99),
      history,
    };
  },
};
scheduleMetrics.reset();

const AUDIO_QUEUE_MAX = 8192;
const AUDIO_FLUSH_CHUNK = 64;
const AUDIO_FLUSH_BUDGET_MS = 1.2;
const AUDIO_MIN_LEAD_SEC = 0.008;

const audioQueue = [];
let audioQueueHead = 0;
let audioFlushScheduled = false;
let audioFlushInFlight = false;

function queueSize() {
  return audioQueue.length - audioQueueHead;
}

function sanitizeAudioEvent(ev) {
  if (!ev) return null;
  const id = ev.id != null ? String(ev.id) : '';
  if (!id) return null;
  const vol = Number.isFinite(ev.vol) ? ev.vol : 1.0;
  const pitch = Number.isFinite(ev.pitch) ? ev.pitch : 0.0;
  const dur = Number.isFinite(ev.dur) && ev.dur > 0 ? ev.dur : 1.0;
  const when = Number.isFinite(ev.when) ? ev.when : NaN;
  return { id, vol, pitch, dur, when };
}

function enqueueAudioEvents(events) {
  if (!Array.isArray(events) || events.length === 0) {
    return;
  }
  // Ensure the audio context is created/resumed before scheduling.
  getCtx();
  let added = 0;
  for (let i = 0; i < events.length; i++) {
    const ev = sanitizeAudioEvent(events[i]);
    if (!ev) {
      continue;
    }
    audioQueue.push(ev);
    added++;
  }
  if (!added) {
    return;
  }
  const size = queueSize();
  if (size > AUDIO_QUEUE_MAX) {
    const drop = size - AUDIO_QUEUE_MAX;
    audioQueueHead = Math.min(audioQueueHead + drop, audioQueue.length);
    dbg('audio.queue.trim', { drop, size: queueSize() });
  }
  scheduleAudioFlush();
}

function scheduleAudioFlush(useTimeout = false) {
  if (audioFlushScheduled) {
    return;
  }
  audioFlushScheduled = true;
  const runner = flushAudioQueue;
  if (useTimeout && typeof setTimeout === 'function') {
    setTimeout(runner, 0);
  } else if (typeof queueMicrotask === 'function') {
    queueMicrotask(runner);
  } else if (typeof Promise !== 'undefined' && Promise.resolve) {
    Promise.resolve().then(runner);
  } else if (typeof setTimeout === 'function') {
    setTimeout(runner, 0);
  }
}

function nextAudioEvent() {
  while (audioQueueHead < audioQueue.length) {
    const ev = audioQueue[audioQueueHead];
    audioQueue[audioQueueHead] = null;
    audioQueueHead++;
    if (ev) {
      return ev;
    }
  }
  audioQueue.length = 0;
  audioQueueHead = 0;
  return null;
}

function flushAudioQueue() {
  if (audioFlushInFlight) {
    audioFlushScheduled = false;
    scheduleAudioFlush(true);
    return;
  }
  audioFlushScheduled = false;
  audioFlushInFlight = true;
  let deferToTimeout = false;
  try {
    if (queueSize() === 0) {
      return;
    }
    const ctx = getCtx();
    const hasPerf = typeof performance !== 'undefined' && typeof performance.now === 'function';
    const start = hasPerf ? performance.now() : 0;
    const deadline = hasPerf ? start + AUDIO_FLUSH_BUDGET_MS : 0;
    let processed = 0;
    while (queueSize() > 0) {
      const ev = nextAudioEvent();
      if (!ev) {
        break;
      }
      processAudioEvent(ev, ctx);
      processed++;
      if (processed >= AUDIO_FLUSH_CHUNK) {
        deferToTimeout = true;
        break;
      }
      if (deadline && performance.now() > deadline) {
        deferToTimeout = true;
        break;
      }
    }
    if (audioQueueHead > 0 && audioQueueHead >= audioQueue.length) {
      audioQueue.length = 0;
      audioQueueHead = 0;
    } else if (audioQueueHead > 1024 && audioQueueHead > audioQueue.length / 2) {
      audioQueue.splice(0, audioQueueHead);
      audioQueueHead = 0;
    }
  } finally {
    audioFlushInFlight = false;
  }
  if (queueSize() > 0) {
    scheduleAudioFlush(deferToTimeout);
  }
}

function processAudioEvent(ev, ctx) {
  const id = ev.id;
  if (!id) return;
  const vol = Number.isFinite(ev.vol) ? ev.vol : 1.0;
  const pitch = Number.isFinite(ev.pitch) ? ev.pitch : 0.0;
  const dur = Number.isFinite(ev.dur) && ev.dur > 0 ? ev.dur : 1.0;
  const rate = Math.pow(2, pitch / 12) / dur;
  const now = ctx.currentTime || 0;
  let when = Number.isFinite(ev.when) ? ev.when : NaN;
  const minWhen = now + AUDIO_MIN_LEAD_SEC;
  if (!Number.isFinite(when)) {
    when = minWhen;
  } else if (when < minWhen) {
    when = minWhen;
  }
  const render = renderCache.get(id);
  if (!render && RENDER[id]) {
    ensureRenderReady(id).then(() => {
      enqueueAudioEvents([{ id, vol, pitch, dur, when }]);
    }).catch((err) => dbg('audio.render.defer.error', { id, err: String(err) }));
    return;
  }
  if (render) {
    const src = ctx.createBufferSource();
    trackSource(id, src);
    try { src.playbackRate.setValueAtTime(rate, when); } catch (_) { src.playbackRate.value = rate; }
    src.buffer = render.buffer;
    src.connect(getBus(id, vol));
    observeScheduleTiming(ctx, when, now);
    try { recordSamples(render.data, vol); } catch (_) {}
    src.start(when);
    return;
  }
  const buf = samples[id];
  if (!render && !buf) {
    if (sampleURLs[id]) {
      ensureSampleBuffer(id).then(() => {
        enqueueAudioEvents([{ id, vol, pitch, dur, when }]);
      }).catch((err) => dbg('audio.sample.defer.error', { id, err: String(err) }));
      return;
    }
  }
  if (buf) {
    const src = ctx.createBufferSource();
    trackSource(id, src);
    src.buffer = buf;
    try { src.playbackRate.setValueAtTime(rate, when); } catch (_) { src.playbackRate.value = rate; }
    src.connect(getBus(id, vol));
    observeScheduleTiming(ctx, when, now);
    try {
      const channel = buf.numberOfChannels > 0 ? buf.getChannelData(0) : null;
      if (channel) recordSamples(channel, vol);
    } catch (_) {}
    src.start(when);
    return;
  }
  playSoundParams(id, vol, pitch, dur, when).catch((err) => dbg('play.batch.fallback.error', { id, err: String(err) }));
}

function pushSample(arr, value) {
  if (!Number.isFinite(value)) return;
  if (arr.length < MAX_SAMPLES) {
    arr.push(value);
  } else {
    const idx = Math.floor(Math.random() * arr.length);
    arr[idx] = value;
  }
}

function percentile(samples, q) {
  if (!samples || samples.length === 0 || !Number.isFinite(q)) {
    return null;
  }
  const sorted = samples.slice().sort((a, b) => a - b);
  const idx = Math.min(sorted.length - 1, Math.max(0, Math.floor(q * (sorted.length - 1))));
  return sorted[idx];
}

function observeScheduleTiming(ctx, when, nowOverride) {
  if (!ctx) return;
  const now = Number.isFinite(nowOverride) ? nowOverride : (ctx.currentTime || 0);
  const lead = when - now;
  const lag = now - when;
  scheduleMetrics.observe(now, when, lead, lag);
}

function resetAudioScheduleMetrics() {
  scheduleMetrics.reset();
}

function getAudioScheduleMetrics() {
  return scheduleMetrics.snapshot();
}

if (typeof window !== 'undefined') {
  window.resetAudioScheduleMetrics = resetAudioScheduleMetrics;
  window.getAudioScheduleMetrics = getAudioScheduleMetrics;
}

function getCtx() {
  if (!ctx) {
    ctx = new (window.AudioContext || window.webkitAudioContext)();
    try {
      if (typeof window !== 'undefined') {
        window.__audioCtx = ctx;
        window.__audioCtxSR = ctx.sampleRate;
      }
    } catch (_) {}
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
  cowbell: 'render_cowbell',
  // Bass instruments.
  'bass-guitar': 'render_bass_guitar',
  'sub-bass': 'render_sub_bass',
  // Variant set 1: slightly brighter/tighter flavours.
  'snare-1': 'render_snare',
  'kick-1': 'render_kick',
  'hihat-1': 'render_open_hihat',
  'tom-1': 'render_tom',
  'clap-1': 'render_clap',
  'cowbell-1': 'render_cowbell',
  'bass-guitar-1': 'render_bass_guitar',
  'sub-bass-1': 'render_sub_bass',
  // Variant set 2: more obviously digital/lofi flavours.
  'snare-2': 'render_snare',
  'kick-2': 'render_kick',
  'hihat-2': 'render_hihat',
  'tom-2': 'render_tom',
  'clap-2': 'render_clap',
  'cowbell-2': 'render_cowbell',
};

const RENDER_INFO = {
  // Longer snare render to capture body + ring.
  snare: { seconds: 1.0, amp: 0.2 },
  kick: { seconds: 0.5, amp: 1.0 },
  // Slightly longer closed hat sample to match Go synth.
  hihat: { seconds: 0.25, amp: 0.6 },
  tom: { seconds: 0.5, amp: 0.6 },
  clap: { seconds: 0.5, amp: 0.6 },
  cowbell: { seconds: 0.4, amp: 0.7 },
  // Bass instruments: longer sustain for bass tones.
  'bass-guitar': { seconds: 1.5, amp: 0.8 },
  'sub-bass': { seconds: 2.0, amp: 1.0 },
  // Variants reuse the same render functions but can tweak length/amp
  // to hint at different flavours on the WebAudio side.
  'snare-1': { seconds: 0.9, amp: 0.25 },
  'kick-1': { seconds: 0.6, amp: 1.0 },
  'hihat-1': { seconds: 0.7, amp: 0.75 },
  'tom-1': { seconds: 0.6, amp: 0.65 },
  'clap-1': { seconds: 0.45, amp: 0.7 },
  'cowbell-1': { seconds: 0.5, amp: 0.8 },
  'bass-guitar-1': { seconds: 1.2, amp: 0.9 },
  'sub-bass-1': { seconds: 1.5, amp: 1.0 },
  'snare-2': { seconds: 0.7, amp: 0.28 },
  'kick-2': { seconds: 0.5, amp: 1.0 },
  'hihat-2': { seconds: 0.22, amp: 0.65 },
  'tom-2': { seconds: 0.45, amp: 0.6 },
  'clap-2': { seconds: 0.4, amp: 0.65 },
  'cowbell-2': { seconds: 0.4, amp: 0.75 },
};

const renderCache = new Map();
const pendingRenderEnsures = new Map();
const pendingSampleLoads = new Map();
// Volume buses reduce per-voice GainNode creation overhead by reusing
// a small set of GainNodes per instrument and quantized volume.
const volumeBus = new Map(); // key: `${id}|${qVol}` -> GainNode
const VOL_Q = 16;
const MAX_BUSES_PER_INST = 16; // soft-cap per instrument
const EVICT_AGE_SEC = 2.0;     // only evict buses unused for this time
// Index per instrument for LRU bookkeeping: id -> Map(q -> { node, lastUsed })
const busIndex = new Map();
const channelNodes = new Map(); // id -> { ingress, gain, analyser, eqChain }
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
    channelNodes.set('main', { ingress: mainChannelNode, gain: mainChannelNode, analyser: null, eqChain: [] });
  }
  return mainChannelNode;
}

function buildChannelPipeline(id) {
  const c = getCtx();
  const ingress = c.createGain();
  ingress.gain.value = 1;
  const gain = c.createGain();
  gain.gain.value = 1;
  ingress.connect(gain);
  gain.connect(ensureMainNode());
  const chain = { ingress, gain, analyser: null, eqChain: [] };
  channelNodes.set(id, chain);
  return chain;
}

function getChannelChain(id) {
  const key = id || 'main';
  if (key === 'main') {
    ensureMainNode();
    return channelNodes.get('main');
  }
  let chain = channelNodes.get(key);
  if (!chain) {
    chain = buildChannelPipeline(key);
  }
  return chain;
}

function channelSink(id) {
  const chain = getChannelChain(id);
  return chain.ingress;
}

export function setChannelVolume(id, vol) {
  const chain = getChannelChain(id);
  chain.gain.gain.value = clampVolume(vol);
}

export function channelVolume(id) {
  const chain = getChannelChain(id);
  return chain.gain.gain.value;
}

export function setMainVolume(vol) { setChannelVolume('main', vol); }
export function mainVolume() { return channelVolume('main'); }

function makeBiquad(c, band) {
  const type = (band && band.type) || 'peaking';
  const freq = Math.max(10, Math.min(20000, band?.freq || 1000));
  const q = Math.max(0.001, Math.min(50, band?.q || 1));
  const gain = Number.isFinite(band?.gainDB) ? band.gainDB : (band?.gain || 0);
  const node = c.createBiquadFilter();
  node.frequency.value = freq;
  node.Q.value = q;
  node.gain.value = gain;
  switch (type) {
    case 'lowshelf': node.type = 'lowshelf'; break;
    case 'highshelf': node.type = 'highshelf'; break;
    default: node.type = 'peaking'; break;
  }
  return node;
}

// Band definitions for the 10-band EQ - matches Go's defaultBandDefs
const BAND_DEFS = [
  { loHz: 20, hiHz: 40 },
  { loHz: 40, hiHz: 80 },
  { loHz: 80, hiHz: 160 },
  { loHz: 160, hiHz: 315 },
  { loHz: 315, hiHz: 630 },
  { loHz: 630, hiHz: 1250 },
  { loHz: 1250, hiHz: 2500 },
  { loHz: 2500, hiHz: 5000 },
  { loHz: 5000, hiHz: 10000 },
  { loHz: 10000, hiHz: 20000 },
];

// Creates a parallel multiband processor where each band is isolated using crossover filters.
// This ensures muting one band has zero effect on adjacent bands.
function createMultibandProcessor(c, bands) {
  const numBands = bands.length;
  if (numBands === 0) {
    // No bands - passthrough
    const passthrough = c.createGain();
    passthrough.gain.value = 1;
    return { input: passthrough, output: passthrough, isMultiband: true };
  }

  // Safety check: if all bands are muted, return a silent processor
  // (This shouldn't happen as setChannelEQ checks allMuted first, but just in case)
  const allMuted = bands.every((b) => b?.muted === true);
  if (allMuted) {
    const silentInput = c.createGain();
    silentInput.gain.value = 0;
    return { input: silentInput, output: silentInput, isMultiband: true };
  }

  const input = c.createGain();
  input.gain.value = 1;
  const output = c.createGain();
  output.gain.value = 1;

  const defs = numBands === BAND_DEFS.length ? BAND_DEFS : BAND_DEFS.slice(0, numBands);

  for (let i = 0; i < numBands; i++) {
    const band = bands[i];
    const def = defs[i] || { loHz: 20, hiHz: 20000 };
    const isMuted = band?.muted === true;
    const gainDB = Number.isFinite(band?.gainDB) ? band.gainDB : (band?.gain || 0);
    const linearGain = isMuted ? 0 : Math.pow(10, gainDB / 20);

    // Build the filter chain for this band
    let node = input;

    // Highpass at loHz - two cascaded stages for LR4 (-24dB/octave)
    if (i > 0) {
      const hp1 = c.createBiquadFilter();
      hp1.type = 'highpass';
      hp1.frequency.value = def.loHz;
      hp1.Q.value = 0.707; // Butterworth
      node.connect(hp1);

      const hp2 = c.createBiquadFilter();
      hp2.type = 'highpass';
      hp2.frequency.value = def.loHz;
      hp2.Q.value = 0.707; // Butterworth
      hp1.connect(hp2);
      node = hp2;
    }

    // Lowpass at hiHz - two cascaded stages for LR4 (-24dB/octave)
    if (i < numBands - 1) {
      const lp1 = c.createBiquadFilter();
      lp1.type = 'lowpass';
      lp1.frequency.value = def.hiHz;
      lp1.Q.value = 0.707; // Butterworth
      node.connect(lp1);

      const lp2 = c.createBiquadFilter();
      lp2.type = 'lowpass';
      lp2.frequency.value = def.hiHz;
      lp2.Q.value = 0.707; // Butterworth
      lp1.connect(lp2);
      node = lp2;
    }

    // Per-band gain (0 if muted, otherwise dB-to-linear)
    const gainNode = c.createGain();
    gainNode.gain.value = linearGain;
    node.connect(gainNode);
    gainNode.connect(output);
  }

  return { input, output, isMultiband: true };
}

function rewireChannel(chain) {
  try { chain.ingress.disconnect(); } catch (_) {}
  if (chain.eqChain) {
    for (const f of chain.eqChain) { try { f.disconnect(); } catch (_) {} }
  }
  if (chain.multibandProc) {
    try { chain.multibandProc.input.disconnect(); } catch (_) {}
    try { chain.multibandProc.output.disconnect(); } catch (_) {}
  }
  if (chain.analyser) { try { chain.analyser.disconnect(); } catch (_) {} }
  // Special-case the main bus (ingress === gain) to avoid creating a feedback
  // loop when an analyser/EQ is inserted. We tap the analyser and then go
  // straight to the destination.
  if (chain.ingress === chain.gain) {
    let node = chain.ingress;
    // Use multiband processor if present, otherwise use eqChain
    if (chain.multibandProc) {
      node.connect(chain.multibandProc.input);
      node = chain.multibandProc.output;
    } else if (chain.eqChain) {
      for (const f of chain.eqChain) {
        node.connect(f);
        node = f;
      }
    }
    if (chain.analyser) {
      node.connect(chain.analyser);
      chain.analyser.connect(getCtx().destination);
    } else {
      node.connect(getCtx().destination);
    }
    return;
  }

  let node = chain.ingress;
  // Use multiband processor if present, otherwise use eqChain
  if (chain.multibandProc) {
    node.connect(chain.multibandProc.input);
    node = chain.multibandProc.output;
  } else if (chain.eqChain) {
    for (const f of chain.eqChain) {
      node.connect(f);
      node = f;
    }
  }
  if (chain.analyser) {
    node.connect(chain.analyser);
    node = chain.analyser;
  }
  node.connect(chain.gain);
}

export function setChannelEQ(id, bands = []) {
  const chain = getChannelChain(id);
  const c = getCtx();
  if (!Array.isArray(bands) || bands.length === 0) {
    chain.eqChain = [];
    chain.multibandProc = null;
    rewireChannel(chain);
    return;
  }

  // Count muted bands explicitly to avoid any truthy/falsy edge cases
  let mutedCount = 0;
  for (let i = 0; i < bands.length; i++) {
    if (bands[i]?.muted === true) {
      mutedCount++;
    }
  }

  // Check if all bands are muted - if so, use a gain node set to 0 for complete silence
  if (mutedCount === bands.length) {
    const silenceGain = c.createGain();
    silenceGain.gain.value = 0;
    chain.eqChain = [silenceGain];
    chain.multibandProc = null;
    rewireChannel(chain);
    return;
  }

  // Check if any band is muted - if so, use multiband processor for proper isolation
  if (mutedCount > 0) {
    const mb = createMultibandProcessor(c, bands);
    chain.multibandProc = mb;
    chain.eqChain = [];
    rewireChannel(chain);
    return;
  }

  // No muting - use simple biquad chain for efficiency
  chain.multibandProc = null;
  chain.eqChain = bands.map((b) => makeBiquad(c, b));
  rewireChannel(chain);
}

export function enableChannelAnalyzer(id, opts = {}) {
  const chain = getChannelChain(id);
  const c = getCtx();
  const fft = (() => {
    const v = opts.fftSize || 512;
    const pow = Math.pow(2, Math.round(Math.log2(Math.max(64, Math.min(8192, v)))));
    return pow;
  })();
  const a = c.createAnalyser();
  a.fftSize = fft;
  a.smoothingTimeConstant = Math.max(0, Math.min(0.95, opts.smoothing || 0.0));
  chain.analyser = a;
  chain._timeBuf = new Float32Array(a.fftSize);
  chain._freqBuf = new Float32Array(a.frequencyBinCount);
  rewireChannel(chain);
}

export function channelAnalyzerSnapshot(id, bins = 64) {
  const chain = getChannelChain(id);
  const a = chain.analyser;
  if (!a) {
    return { rms: 0, peak: 0, spectrum: [], wave: [] };
  }
  const time = chain._timeBuf || new Float32Array(a.fftSize);
  const freq = chain._freqBuf || new Float32Array(a.frequencyBinCount);
  a.getFloatTimeDomainData(time);
  a.getFloatFrequencyData(freq);
  let sum = 0;
  let peak = 0;
  for (let i = 0; i < time.length; i++) {
    const v = time[i];
    sum += v * v;
    const av = Math.abs(v);
    if (av > peak) peak = av;
  }
  const rms = Math.sqrt(sum / time.length);
  const spectrum = [];
  const n = freq.length;
  const step = Math.max(1, Math.floor(n / Math.max(1, bins)));
  for (let i = 0; i < n; i += step) {
    let max = -Infinity;
    const end = Math.min(n, i + step);
    for (let j = i; j < end; j++) {
      if (freq[j] > max) max = freq[j];
    }
    // Convert dB to linear magnitude [0..1].
    spectrum.push(Math.pow(10, max / 20));
  }
  // Copy waveform (time domain) so callers can draw it; clamp to [-1,1].
  const wave = Array.from(time, (v) => {
    if (v > 1) return 1;
    if (v < -1) return -1;
    return v;
  });
  return { rms, peak, spectrum, wave };
}

function getBus(id, vol) {
  const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
  const q = Math.round(v * VOL_Q);
  const key = id + '|' + q;
  const now = getCtx().currentTime || 0;
  let bus = volumeBus.get(key);
  let per = busIndex.get(id);
  if (!per) { per = new Map(); busIndex.set(id, per); }
  if (!bus) {
    const c = getCtx();
    bus = c.createGain();
    bus.gain.value = q / VOL_Q;
    bus.connect(channelSink(id));
    volumeBus.set(key, bus);
    per.set(q, { node: bus, lastUsed: now });
    maybeEvictBuses(id, now);
    return bus;
  }
  const meta = per.get(q) || { node: bus, lastUsed: 0 };
  meta.lastUsed = now;
  per.set(q, meta);
  return bus;
}

function maybeEvictBuses(id, now) {
  const per = busIndex.get(id);
  if (!per || per.size <= MAX_BUSES_PER_INST) return;
  const entries = Array.from(per.entries());
  entries.sort((a, b) => (a[1].lastUsed - b[1].lastUsed));
  for (const [q, meta] of entries) {
    if (per.size <= MAX_BUSES_PER_INST) break;
    if (!meta || !meta.node) continue;
    try { meta.node.disconnect(); } catch (_) {}
    per.delete(q);
    volumeBus.delete(id + '|' + q);
  }
}

// Warm up common synthesized instruments so hot path is synchronous.
(async () => {
  try {
    await ensureModule();
    const ids = ['snare', 'kick', 'hihat', 'tom', 'clap', 'cowbell'];
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
  const ctxLocal = getCtx();
  const sr = Math.max(8000, Math.min(192000, ctxLocal && ctxLocal.sampleRate ? ctxLocal.sampleRate : 44100));
  // If cache exists but was built for a different sample rate, rebuild so pitch
  // and duration stay correct on devices that default to 48 kHz.
  if (renderCache.has(id)) {
    const hit = renderCache.get(id);
    if (hit && hit.sr === sr) {
      try {
        if (typeof window !== 'undefined') {
          const metrics = window.__audioMetrics || (window.__audioMetrics = { renders: {}, cacheHits: {} });
          metrics.cacheHits[id] = (metrics.cacheHits[id] || 0) + 1;
        }
      } catch (_) {}
      return hit;
    }
    // Drop stale entry so we regenerate with the correct rate.
    renderCache.delete(id);
  }
  const info = RENDER_INFO[id] || { seconds: 0.5, amp: 0.6 };
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
    const buffer = ctxLocal.createBuffer(1, frames, sr);
    buffer.copyToChannel(data, 0);
    const record = { buffer, data, sr, frames };
    renderCache.set(id, record);
    samples[id] = buffer;
    try {
      if (typeof window !== 'undefined') {
        const metrics = window.__audioMetrics || (window.__audioMetrics = { renders: {}, cacheHits: {} });
        metrics.renders[id] = (metrics.renders[id] || 0) + 1;
        const meta = window.__renderMeta || (window.__renderMeta = {});
        meta[id] = { sr, frames, seconds: frames / sr, ctxSR: ctxLocal.sampleRate };
      }
    } catch (_) {}
    return record;
  } finally {
    m._free(ptr);
  }
}

function ensureRenderReady(id) {
  if (renderCache.has(id)) {
    return Promise.resolve(renderCache.get(id));
  }
  if (!RENDER[id]) {
    return Promise.resolve(null);
  }
  let pending = pendingRenderEnsures.get(id);
  if (pending) {
    return pending;
  }
  pending = ensureRenderedSample(id).then((record) => {
    pendingRenderEnsures.delete(id);
    return record;
  }).catch((err) => {
    pendingRenderEnsures.delete(id);
    throw err;
  });
  pendingRenderEnsures.set(id, pending);
  return pending;
}

export async function loadWav(id, url) {
  const res = await fetch(url);
  const arr = await res.arrayBuffer();
  const buf = await getCtx().decodeAudioData(arr);
  samples[id] = buf;
  dbg('sample.loaded', { id, frames: buf.length, sr: buf.sampleRate });
}

function ensureSampleBuffer(id) {
  if (samples[id]) {
    return Promise.resolve(samples[id]);
  }
  const url = sampleURLs[id];
  if (!url) {
    return Promise.resolve(null);
  }
  let pending = pendingSampleLoads.get(id);
  if (pending) {
    return pending;
  }
  pending = loadWav(id, url).then(() => {
    pendingSampleLoads.delete(id);
    return samples[id] || null;
  }).catch((err) => {
    pendingSampleLoads.delete(id);
    throw err;
  });
  pendingSampleLoads.set(id, pending);
  return pending;
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
      observeScheduleTiming(ctx, t);
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
  trackSource(id, src);
  const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
  const t = typeof when === 'number' && Number.isFinite(when) ? when : ctx.currentTime;
  observeScheduleTiming(ctx, t);
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
      trackSource(id, src);
      const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
      const t = typeof when === 'number' && Number.isFinite(when) ? when : ctx.currentTime;
      observeScheduleTiming(ctx, t);
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
  trackSource(id, src);
  const v = Math.max(0, Math.min(1, Number.isFinite(vol) ? vol : 1.0));
  const t = typeof when === 'number' && Number.isFinite(when) ? when : ctx.currentTime;
  observeScheduleTiming(ctx, t);
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
  dbg('play.enqueue', { id, vol, when });
  enqueueAudioEvents([{ id, vol, pitch: 0, dur: 1, when }]);
};

window.playSoundParams = async (id, vol, pitch, dur, when) => {
  dbg('play.params.enqueue', { id, vol, pitch, dur, when });
  enqueueAudioEvents([{ id, vol, pitch, dur, when }]);
};

// stopSound(id) stops any currently scheduled/playing sources for that id.
// This is invoked by Go mute nodes via the WASM bridge.
window.stopSound = (id) => {
  const key = id != null ? String(id) : '';
  if (!key) return;
  const set = activeSources.get(key);
  if (!set || set.size === 0) return;
  dbg('stopSound', { id: key, count: set.size });
  for (const src of Array.from(set)) {
    try { src.stop(); } catch (_) {}
    try { src.disconnect(); } catch (_) {}
  }
  try { set.clear(); } catch (_) {}
  try { activeSources.delete(key); } catch (_) {}
};

window.setChannelVolume = (id, vol) => {
  try { setChannelVolume(id, vol); } catch (err) { dbg('channel.set.error', { id, err: String(err) }); }
};

window.channelVolume = (id) => {
  try { return channelVolume(id); } catch (err) { dbg('channel.get.error', { id, err: String(err) }); return 1; }
};

window.setMainVolume = (vol) => window.setChannelVolume('main', vol);
window.mainVolume = () => window.channelVolume('main');

window.setChannelEQ = (id, bands) => {
  try { setChannelEQ(id, bands); } catch (err) { dbg('channel.eq.error', { id, err: String(err) }); }
};

window.enableChannelAnalyzer = (id, windowSize) => {
  try { enableChannelAnalyzer(id, { fftSize: windowSize }); } catch (err) { dbg('channel.analyzer.error', { id, err: String(err) }); }
};

window.channelAnalyzerSnapshot = (id) => {
  try { return channelAnalyzerSnapshot(id); } catch (err) { dbg('channel.analyzer.snap.error', { id, err: String(err) }); return { rms: 0, peak: 0, spectrum: [], wave: [] }; }
};

// Batch scheduling API to reduce Go→JS crossings in WASM builds.
// Expects an Array of objects: { id, vol, pitch, dur, when? }.
// Uses a synchronous fast path when samples are pre-rendered/decoded.
window.playSoundsBatch = (arr) => {
  try {
    if (!Array.isArray(arr)) {
      if (DEBUG_ENABLED && arr && typeof arr === 'object') {
        dbg('play.params.enqueue', { id: arr.id, vol: arr.vol, pitch: arr.pitch, dur: arr.dur, when: arr.when });
      }
      enqueueAudioEvents([arr]);
    } else {
      if (DEBUG_ENABLED) {
        for (const item of arr) {
          if (item && typeof item === 'object') {
            dbg('play.params.enqueue', { id: item.id, vol: item.vol, pitch: item.pitch, dur: item.dur, when: item.when });
          }
        }
      }
      enqueueAudioEvents(arr);
    }
  } catch (err) {
    console.error('[AUDIOJS] playSoundsBatch error', err);
  }
};

// Flat batch scheduling API (ids + typed arrays) to reduce Go<->JS overhead.
window.playSoundsBatchFlat = (ids, vols, pitches, durs, whens, hasWhen) => {
  try {
    if (!ids || ids.length === 0) {
      return;
    }
    const n = ids.length;
    const events = new Array(n);
    for (let i = 0; i < n; i++) {
      const id = ids[i];
      if (!id) continue;
      const ev = {
        id,
        vol: Number.isFinite(vols?.[i]) ? vols[i] : 1.0,
        pitch: Number.isFinite(pitches?.[i]) ? pitches[i] : 0.0,
        dur: Number.isFinite(durs?.[i]) && durs[i] > 0 ? durs[i] : 1.0,
      };
      const when = Number.isFinite(whens?.[i]) ? whens[i] : NaN;
      if (hasWhen && hasWhen[i] && Number.isFinite(when)) {
        ev.when = when;
      }
      events[i] = ev;
    }
    enqueueAudioEvents(events);
  } catch (err) {
    console.error('[AUDIOJS] playSoundsBatchFlat error', err);
  }
};

window.loadWav = async (id, url) => {
  try {
    await loadWav(id, url);
  } catch (err) {
    console.error('Error loading wav:', err);
  }
};

// Time and resume bridges used by Go. Defined once here.
window.audioNow = () => { try { return getCtx().currentTime; } catch (_) { return 0; } };
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

// Download a CSV blob. Safe no-op on errors.
window.downloadCSV = (name, text) => {
  try {
    const blob = new Blob([text], { type: 'text/csv' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = name || 'audio-history.csv';
    document.body.appendChild(a);
    a.click();
    setTimeout(() => { URL.revokeObjectURL(a.href); a.remove(); }, 100);
  } catch (err) {
    console.error('downloadCSV failed', err);
  }
};

// Export the per-second audio schedule history to CSV (returns string).
// Options: { download?: boolean, filename?: string, includeSummary?: boolean }
window.exportAudioHistoryCSV = (opts = {}) => {
  try {
    const snap = getAudioScheduleMetrics();
    const rows = [];
    const header = ['sec','count','avgLead','leadP90','leadP99','avgLag','lagP90','lagP99'];
    rows.push(header.join(','));
    (snap.history || []).forEach(h => {
      const vals = [h.sec, h.count,
        fmtNum(h.avgLead), fmtNum(h.leadP90), fmtNum(h.leadP99),
        fmtNum(h.avgLag), fmtNum(h.lagP90), fmtNum(h.lagP99)];
      rows.push(vals.join(','));
    });
    if (opts.includeSummary) {
      rows.push('');
      rows.push(['summary','','avgLead','leadP90','leadP99','avgLag','lagP90','lagP99'].join(','));
      rows.push(['',snap.count,
        fmtNum(snap.avgLead), fmtNum(snap.leadP90), fmtNum(snap.leadP99),
        fmtNum(snap.avgLag), fmtNum(snap.lagP90), fmtNum(snap.lagP99)].join(','));
    }
    const csv = rows.join('\n');
    if (opts.download) {
      window.downloadCSV(opts.filename || 'audio-history.csv', csv);
    }
    return csv;
  } catch (err) {
    console.error('exportAudioHistoryCSV failed', err);
    return '';
  }
};

function fmtNum(v) {
  if (v == null) return '';
  if (!Number.isFinite(v)) return '';
  // Use seconds as stored; callers may format ms externally.
  return String(+v);
}

// Open a file picker and read a JSON file as text. Returns a Promise.
window.openJSONFile = () => new Promise((resolve) => {
  console.log('[IMPORT] openJSONFile invoked');
  try {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = 'application/json,.json';
    let settled = false;
    const settle = (val) => { if (!settled) { try { settled = true; resolve(val); } catch(_){} } };
    input.onchange = async () => {
      console.log('[IMPORT] input onchange fired');
      const file = input.files && input.files[0];
      if (!file) { settle(''); setTimeout(() => input.remove(), 0); return; }
      // Guard: reject overly large JSON to avoid jank and memory blowups.
      const MAX = 5 * 1024 * 1024; // 5MB
      if (typeof file.size === 'number' && file.size > MAX) {
        console.warn('[IMPORT] JSON file too large, rejecting:', file.size);
        settle('');
        setTimeout(() => input.remove(), 0);
        return;
      }
      console.log('[IMPORT] reading file', file.name, file.size);
      const text = await file.text();
      settle(text);
      setTimeout(() => input.remove(), 0);
    };
    document.body.appendChild(input);
    console.log('[IMPORT] clicking file input');
    input.click();
    // Fallback: if user cancels and 'change' does not fire, release after 3s.
    setTimeout(() => { try { if (!settled) { console.warn('[IMPORT] file picker timeout (cancel?)'); settle(''); input.remove(); } } catch(_){} }, 3000);
  } catch (err) { console.error('openJSONFile failed', err); resolve(''); }
});

// Register a sample URL for lazy loading on first use.
window.registerWav = (id, url) => {
  sampleURLs[id] = url;
};

// Expose audio context current time for Go WASM scheduling.
// Authoritative definitions above; no duplicate fallbacks.

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
window.__audioBusCountById = (id) => {
  try { const per = busIndex.get(id); return per ? per.size : 0; } catch (_) { return 0; }
};
window.resetRenderCache = () => {
  try {
    renderCache.clear();
  } catch (_) {}
};
window.getRenderCacheStats = () => {
  const stats = {};
  try {
    renderCache.forEach((rec, id) => {
      stats[id] = {
        duration: rec && rec.buffer ? rec.buffer.duration || 0 : 0,
        frames: rec && rec.data ? rec.data.length : rec && rec.buffer ? rec.buffer.length || 0 : 0,
      };
    });
  } catch (_) {}
  return stats;
};
window.ensureSynthSample = async (id) => {
  try {
    const rec = await ensureRenderedSample(id);
    return !!rec;
  } catch (_) {
    return false;
  }
};
