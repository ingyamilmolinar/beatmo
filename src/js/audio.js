let modulePromise = null;
let mod;
let ctx;
const samples = {};
const sampleURLs = {};

// Output capture: intercept final mixed output for debugging/comparison
let outputCaptureNode = null;
let outputCaptureBuffer = [];
let outputCaptureEnabled = false;
let outputCaptureSampleRate = 44100;

// Hard limiter (WaveShaperNode) — clamps output to [-1, 1] matching the
// desktop's hard clamp in engine_stop.go. Transparent for single-voice signals;
// prevents clipping when multiple voices overlap. Created lazily.
let mainLimiter = null;

function ensureLimiter() {
  if (mainLimiter) return mainLimiter;
  if (!hasCtx()) return null;
  const c = ctx;
  mainLimiter = c.createWaveShaper();
  const curveLen = 8192;
  const curve = new Float32Array(curveLen);
  for (let i = 0; i < curveLen; i++) {
    const x = (i / (curveLen - 1)) * 2 - 1;
    curve[i] = Math.max(-1, Math.min(1, x));
  }
  mainLimiter.curve = curve;
  mainLimiter.oversample = 'none';
  mainLimiter.connect(c.destination);
  return mainLimiter;
}

// Returns the correct final audio destination. All channel outputs should
// connect to this node. Routes through limiter → [capture →] destination.
function audioDestination() {
  const limiter = ensureLimiter();
  if (!limiter && hasCtx()) {
    // Limiter creation failed somehow; fallback to raw destination.
    return ctx.destination;
  }
  return limiter;
}

// Debug: ring buffer of recent audio events
const __DBG_CAP = 200;
if (typeof window !== 'undefined' && !window.__audioDebug) window.__audioDebug = [];
function dbg(tag, data) {
  if (typeof window === 'undefined') return;
  if (!window.BEATMO_AUDIO_DEBUG) return;  // Check dynamically
  try {
    const entry = { t: Date.now(), tag, ...(data || {}) };
    if (Array.isArray(window.__audioDebug)) {
      window.__audioDebug.push(entry);
      if (window.__audioDebug.length > __DBG_CAP) window.__audioDebug.shift();
    }
    if (window.BEATMO_AUDIO_DEBUG && console && console.info) {
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
const AUDIO_MIN_LEAD_SEC = 0.004;

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
  // If no context exists yet, the events are still queued — they'll be
  // flushed once the context reaches 'running' via the statechange listener.
  if (hasCtx()) {
    if (ctx.state === 'suspended') {
      try { ctx.resume(); } catch (_) {}
    }
  }
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
    // If no AudioContext exists yet, defer — events stay queued until a user
    // gesture creates the context and the statechange listener flushes.
    if (!hasCtx()) {
      return;
    }
    // On mobile, AudioContext starts suspended and currentTime stays at 0.
    // Scheduling sounds while suspended produces timestamps that are already
    // in the past once the context resumes, causing silent playback.
    // Defer the flush until the context is running.
    if (ctx.state === 'suspended') {
      ctx.resume().then(() => {
        if (queueSize() > 0) scheduleAudioFlush(true);
      }).catch(() => {
        if (queueSize() > 0) scheduleAudioFlush(true);
      });
      return;
    }
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
      // Omit `when` so the event plays at current time instead of the
      // original (now stale) timestamp that was computed before the
      // sample finished rendering.
      enqueueAudioEvents([{ id, vol, pitch, dur }]);
    }).catch((err) => dbg('audio.render.defer.error', { id, err: String(err) }));
    return;
  }
  if (render) {
    // Lazily create the AudioBuffer from cached Float32Array on first play.
    const buf = render.buffer || ensureAudioBuffer(id);
    if (!buf) {
      dbg('audio.render.nobuf', { id });
      return;
    }
    const src = ctx.createBufferSource();
    trackSource(id, src);
    try { src.playbackRate.setValueAtTime(rate, when); } catch (_) { src.playbackRate.value = rate; }
    src.buffer = buf;
    // Anti-pop: per-source fade GainNode with 5ms fade-in ramp.
    const fadeGain = ctx.createGain();
    fadeGain.gain.setValueAtTime(0, when);
    fadeGain.gain.linearRampToValueAtTime(1, when + 0.005);
    src.connect(fadeGain);
    fadeGain.connect(getBus(id, vol));
    src._antiPopGain = fadeGain; // stash for stopSound fade-out
    observeScheduleTiming(ctx, when, now);
    try { recordSamples(render.data, vol); } catch (_) {}
    src.start(when);
    return;
  }
  const buf = samples[id];
  if (!render && !buf) {
    if (sampleURLs[id]) {
      ensureSampleBuffer(id).then(() => {
        // Omit `when` — same stale-timestamp fix as the render path above.
        enqueueAudioEvents([{ id, vol, pitch, dur }]);
      }).catch((err) => dbg('audio.sample.defer.error', { id, err: String(err) }));
      return;
    }
  }
  if (buf) {
    const src = ctx.createBufferSource();
    trackSource(id, src);
    src.buffer = buf;
    try { src.playbackRate.setValueAtTime(rate, when); } catch (_) { src.playbackRate.value = rate; }
    // Anti-pop: per-source fade GainNode with 5ms fade-in ramp.
    const fadeGain = ctx.createGain();
    fadeGain.gain.setValueAtTime(0, when);
    fadeGain.gain.linearRampToValueAtTime(1, when + 0.005);
    src.connect(fadeGain);
    fadeGain.connect(getBus(id, vol));
    src._antiPopGain = fadeGain; // stash for stopSound fade-out
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

function getSampleRate() {
  if (hasCtx() && ctx.sampleRate) return ctx.sampleRate;
  return 48000; // Safe default; most mobile/desktop browsers use 48kHz
}

// iOS speaker routing fix — configures the audio session type to 'playback'
// so WebAudio output routes through the media channel (speaker) instead of
// the ringer channel (headphones only). Safari 16.4+ supports this API.
function configureAudioSession() {
  try {
    if (typeof navigator !== 'undefined' && navigator.audioSession) {
      navigator.audioSession.type = 'playback';
    }
  } catch (_) {}
}
// Apply at module load time (no-op on browsers that don't support audioSession).
configureAudioSession();

// iOS speaker routing fallback — a silent looping <audio> element forces iOS
// to merge the ringer and media audio channels, enabling speaker output for
// WebAudio even on iOS < 16.4 where navigator.audioSession isn't available.
let silentAudioEl = null;
const SILENT_WAV_DATA_URI = 'data:audio/wav;base64,UklGRiYAAABXQVZFZm10IBAAAAABAAEARKwAABCxAgACABAAZGF0YQIAAAAAAA==';

function ensureSilentAudioElement() {
  if (silentAudioEl) return silentAudioEl;
  try {
    silentAudioEl = document.createElement('audio');
    silentAudioEl.src = SILENT_WAV_DATA_URI;
    silentAudioEl.setAttribute('playsinline', '');
    silentAudioEl.loop = true;
    silentAudioEl.volume = 0.01;
    silentAudioEl.style.display = 'none';
    document.body.appendChild(silentAudioEl);
  } catch (_) {}
  return silentAudioEl;
}

// Audio unlock state — declared before getCtx() so the statechange listener can
// reference them without hitting the temporal dead zone.
let _audioUnlocked = false;
const UNLOCK_EVENTS = ['touchstart', 'touchend', 'pointerdown', 'mousedown', 'keydown'];

// Pending operations queue — stores channel volume/EQ/mute changes made before
// the AudioContext exists. Applied once the context first reaches 'running'.
const pendingChannelOps = [];
let pendingOpsApplied = false;

// Returns true if an AudioContext already exists (without creating one).
function hasCtx() {
  return !!ctx;
}

function applyPendingOps() {
  if (pendingOpsApplied) return;
  pendingOpsApplied = true;
  while (pendingChannelOps.length > 0) {
    const op = pendingChannelOps.shift();
    try { op(); } catch (_) {}
  }
}

// Internal: actually creates the AudioContext. Only called from user gesture
// handlers (unlockAudio) or from code that absolutely needs a context now.
function createCtx() {
  if (ctx) return ctx;
  ctx = new (window.AudioContext || window.webkitAudioContext)();
  try {
    if (typeof window !== 'undefined') {
      window.__audioCtx = ctx;
      window.__audioCtxSR = ctx.sampleRate;
    }
  } catch (_) {}
  // Track state transitions so we know when the context is truly unlocked.
  try {
    ctx.addEventListener('statechange', () => {
      dbg('ctx.statechange', { state: ctx.state, t: ctx.currentTime });
      if (ctx.state === 'running') {
        _audioUnlocked = true;
        // Apply any channel ops that were queued before context existed.
        applyPendingOps();
        // Ensure the main node and limiter are wired up now that context is live.
        ensureMainNode();
        // Flush any audio events that were deferred while the context was suspended.
        if (queueSize() > 0) {
          scheduleAudioFlush(true);
        }
      }
    });
  } catch (_) {}
  // If the context starts directly in 'running' (e.g. autoplay policy disabled),
  // the statechange event never fires. Apply pending ops immediately.
  if (ctx.state === 'running') {
    _audioUnlocked = true;
    applyPendingOps();
    ensureMainNode();
    if (queueSize() > 0) {
      scheduleAudioFlush(true);
    }
  }
  dbg('ctx.new', { sr: ctx.sampleRate, state: ctx.state });
  return ctx;
}

function getCtx() {
  if (!ctx) {
    // Create the context on demand. On mobile this may start suspended, but
    // the statechange listener + unlock gestures will resume it.
    createCtx();
  }
  // Try to resume if the context is suspended (common on first user gesture).
  if (ctx.state === 'suspended') {
    try { ctx.resume(); dbg('ctx.resume', { state: ctx.state }); } catch (e) { dbg('ctx.resume.error', { err: String(e) }); }
  }
  return ctx;
}

// Robust multi-event audio unlock for mobile browsers.
// iOS Safari requires a silent buffer play inside a user gesture to truly unlock.
// Multiple event types ensure we catch the first interaction regardless of input method.
// Listeners are NEVER removed — if the context re-suspends (tab switch, phone call,
// iOS background), the next user gesture will re-unlock it automatically.
function unlockAudio() {
  // Fast path: if context is already running, nothing to do.
  if (ctx && ctx.state === 'running') return;
  try {
    // 1. Configure audio session (Safari 16.4+ speaker routing).
    configureAudioSession();
    // 2. Ensure silent <audio> element and play it (iOS < 16.4 fallback).
    try {
      const el = ensureSilentAudioElement();
      if (el) el.play().catch(() => {});
    } catch (_) {}
    // 3. Create AudioContext on first gesture if it doesn't exist yet.
    const c = createCtx();
    // 4. Play a tiny silent buffer — required by some iOS versions to truly unlock.
    try {
      const silentBuf = c.createBuffer(1, 1, c.sampleRate);
      const src = c.createBufferSource();
      src.buffer = silentBuf;
      src.connect(c.destination);
      src.start(0);
    } catch (_) {}
    // 5. Resume suspended context.
    if (c.state === 'suspended') {
      c.resume().then(() => {
        dbg('ctx.unlocked.gesture', { state: c.state, t: c.currentTime });
      }).catch(() => {});
    }
  } catch (e) {
    dbg('ctx.unlock.error', { err: String(e) });
  }
}

try {
  if (typeof document !== 'undefined' && document.addEventListener) {
    for (const evt of UNLOCK_EVENTS) {
      document.addEventListener(evt, unlockAudio, { capture: true, passive: true });
    }
  }
} catch (_) {}

// Re-activate audio when the page becomes visible again (e.g. after a tab
// switch or returning from the lock screen on mobile). iOS suspends the
// AudioContext when the page is backgrounded.
try {
  if (typeof document !== 'undefined' && document.addEventListener) {
    document.addEventListener('visibilitychange', () => {
      if (document.visibilityState !== 'visible') return;
      if (!hasCtx()) return;
      if (ctx.state === 'suspended') {
        ctx.resume().catch(() => {});
      }
      if (silentAudioEl) silentAudioEl.play().catch(() => {});
      if (queueSize() > 0) scheduleAudioFlush(true);
    });
  }
} catch (_) {}

// Stop all active audio and suspend the AudioContext when the page is being
// unloaded (tab close, navigation). Uses 'pagehide' instead of 'beforeunload'
// for mobile compatibility (iOS fires pagehide but not beforeunload).
try {
  if (typeof window !== 'undefined') {
    window.addEventListener('pagehide', () => {
      for (const [_key, set] of activeSources) {
        for (const src of Array.from(set)) {
          try { src.stop(); } catch (_) {}
          try { src.disconnect(); } catch (_) {}
        }
        set.clear();
      }
      activeSources.clear();
      if (hasCtx()) {
        try { ctx.close(); } catch (_) {}
      }
    });
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
  'kick-1': 'render_kick_punchy',
  'hihat-1': 'render_open_hihat',
  'tom-1': 'render_tom',
  'clap-1': 'render_clap',
  'cowbell-1': 'render_cowbell',
  'bass-guitar-1': 'render_bass_guitar',
  'sub-bass-1': 'render_sub_bass',
  // Variant set 2: more obviously digital/lofi flavours.
  'snare-2': 'render_snare',
  'kick-2': 'render_kick_lofi',
  'hihat-2': 'render_hihat',
  'tom-2': 'render_tom',
  'clap-2': 'render_clap',
  'cowbell-2': 'render_cowbell',
  // New distinct instruments.
  'rimshot': 'render_snare_rimshot',
  'sidestick': 'render_snare_sidestick',
  'kick-deep': 'render_kick_deep',
  'shaker': 'render_shaker',
  'ride': 'render_ride',
  'crash': 'render_crash',
  // Variant set 3: expressive dynamics.
  'snare-ghost': 'render_snare',
  'kick-tight': 'render_kick_tight',
  'hihat-pedal': 'render_hihat',
  'clap-tight': 'render_clap',
  // FM synthesis instruments.
  'fm-bass':     'render_fm_bass',
  'fm-bell':     'render_fm_bell',
  'fm-lead':     'render_fm_lead',
  'fm-epiano':   'render_fm_epiano',
  'fm-pluck':    'render_fm_pluck',
  'fm-bass-1':   'render_fm_bass',
  'fm-bell-1':   'render_fm_bell',
  'fm-lead-1':   'render_fm_lead',
  'fm-epiano-1': 'render_fm_epiano',
  'fm-pluck-1':  'render_fm_pluck',
};

const RENDER_INFO = {
  snare:   { seconds: 1.0,  amp: 0.8 },
  kick:    { seconds: 0.5,  amp: 0.8 },
  hihat:   { seconds: 0.25, amp: 0.8 },
  tom:     { seconds: 0.5,  amp: 0.8 },
  clap:    { seconds: 0.5,  amp: 0.8 },
  cowbell: { seconds: 0.4,  amp: 0.8 },
  'bass-guitar': { seconds: 1.5, amp: 0.8 },
  'sub-bass':    { seconds: 2.0, amp: 0.8 },
  'snare-1':        { seconds: 0.9,  amp: 0.8 },
  'kick-1':         { seconds: 0.6,  amp: 0.8 },
  'hihat-1':        { seconds: 0.7,  amp: 0.8 },
  'tom-1':          { seconds: 0.6,  amp: 0.8 },
  'clap-1':         { seconds: 0.45, amp: 0.8 },
  'cowbell-1':      { seconds: 0.5,  amp: 0.8 },
  'bass-guitar-1':  { seconds: 1.2,  amp: 0.8 },
  'sub-bass-1':     { seconds: 1.5,  amp: 0.8 },
  'snare-2':        { seconds: 0.7,  amp: 0.8 },
  'kick-2':         { seconds: 0.5,  amp: 0.8 },
  'hihat-2':        { seconds: 0.22, amp: 0.8 },
  'tom-2':          { seconds: 0.45, amp: 0.8 },
  'clap-2':         { seconds: 0.4,  amp: 0.8 },
  'cowbell-2':      { seconds: 0.4,  amp: 0.8 },
  'rimshot':        { seconds: 0.3,  amp: 0.8 },
  'sidestick':      { seconds: 0.25, amp: 0.8 },
  'kick-deep':      { seconds: 0.8,  amp: 0.8 },
  'shaker':         { seconds: 0.3,  amp: 0.8 },
  'ride':           { seconds: 1.0,  amp: 0.8 },
  'crash':          { seconds: 1.5,  amp: 0.8 },
  'snare-ghost':    { seconds: 0.5,  amp: 0.8 },
  'kick-tight':     { seconds: 0.35, amp: 0.8 },
  'hihat-pedal':    { seconds: 0.15, amp: 0.8 },
  'clap-tight':     { seconds: 0.3,  amp: 0.8 },
  // FM synthesis instruments.
  'fm-bass':        { seconds: 1.5,  amp: 0.8 },
  'fm-bell':        { seconds: 2.0,  amp: 0.8 },
  'fm-lead':        { seconds: 1.0,  amp: 0.8 },
  'fm-epiano':      { seconds: 2.0,  amp: 0.8 },
  'fm-pluck':       { seconds: 0.5,  amp: 0.8 },
  'fm-bass-1':      { seconds: 1.0,  amp: 0.8 },
  'fm-bell-1':      { seconds: 1.5,  amp: 0.8 },
  'fm-lead-1':      { seconds: 0.7,  amp: 0.8 },
  'fm-epiano-1':    { seconds: 1.5,  amp: 0.8 },
  'fm-pluck-1':     { seconds: 0.3,  amp: 0.8 },
};

const renderCache = new Map();
const pendingRenderEnsures = new Map();
const pendingSampleLoads = new Map();
// Volume buses reduce per-voice GainNode creation overhead by reusing
// a small set of GainNodes per instrument and quantized volume.
const volumeBus = new Map(); // key: `${id}|${qVol}` -> GainNode
const VOL_Q = 16;
const MAX_BUSES_PER_INST = 16; // soft-cap per instrument
const _EVICT_AGE_SEC = 2.0;     // only evict buses unused for this time
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

// Master compressor: inserted between mainChannelNode and the limiter.
// Uses WebAudio's native DynamicsCompressorNode with drum-optimized defaults.
let mainCompressor = null;

function ensureCompressor() {
  if (mainCompressor) return mainCompressor;
  if (!hasCtx()) return null;
  const c = ctx;
  mainCompressor = c.createDynamicsCompressor();
  mainCompressor.threshold.value = -6;   // dB
  mainCompressor.ratio.value = 4;        // 4:1
  mainCompressor.attack.value = 0.001;   // 1ms
  mainCompressor.release.value = 0.05;   // 50ms
  mainCompressor.knee.value = 3;         // 3dB soft knee
  mainCompressor.connect(audioDestination());
  return mainCompressor;
}

function ensureMainNode() {
  if (!mainChannelNode) {
    // Don't force context creation — if no context exists yet, defer.
    if (!hasCtx()) return null;
    const c = ctx;
    mainChannelNode = c.createGain();
    mainChannelNode.gain.value = 1;
    // Route through compressor before limiter.
    const comp = ensureCompressor();
    mainChannelNode.connect(comp || audioDestination());
    channelNodes.set('main', { ingress: mainChannelNode, gain: mainChannelNode, analyser: null, eqChain: [] });
  }
  return mainChannelNode;
}

// ==== Send Effects (Delay + Reverb) ====
// Shared send buses: each instrument channel can route signal via send gains.

let delaySendBus = null;  // GainNode → DelayNode feedback loop → mainChannel
let reverbSendBus = null; // GainNode → ConvolverNode → mainChannel
const sendGains = new Map(); // id -> { delay: GainNode, reverb: GainNode }

function ensureDelaySendBus() {
  if (delaySendBus) return delaySendBus;
  if (!hasCtx()) return null;
  const c = ctx;

  // Delay line: 300ms with LP-filtered feedback for warm tape echo.
  const delayTime = 0.3;
  const feedback = 0.3;
  const dampFreq = 3000;

  const inputGain = c.createGain();
  inputGain.gain.value = 1;

  const delay = c.createDelay(1.0);
  delay.delayTime.value = delayTime;

  const fbGain = c.createGain();
  fbGain.gain.value = feedback;

  const lpFilter = c.createBiquadFilter();
  lpFilter.type = 'lowpass';
  lpFilter.frequency.value = dampFreq;
  lpFilter.Q.value = 0.707;

  // Signal flow: input → delay → LP → fbGain → delay (feedback loop)
  //                            ↓
  //                     mainChannelNode
  inputGain.connect(delay);
  delay.connect(lpFilter);
  lpFilter.connect(fbGain);
  fbGain.connect(delay); // feedback
  delay.connect(ensureMainNode()); // wet output

  delaySendBus = inputGain;
  return delaySendBus;
}

function ensureReverbSendBus() {
  if (reverbSendBus) return reverbSendBus;
  if (!hasCtx()) return null;
  const c = ctx;

  const inputGain = c.createGain();
  inputGain.gain.value = 1;

  // Generate a synthetic impulse response (Schroeder-style room).
  const irLength = Math.floor(c.sampleRate * 1.5); // 1.5s reverb tail
  const irBuffer = c.createBuffer(1, irLength, c.sampleRate);
  const irData = irBuffer.getChannelData(0);
  for (let i = 0; i < irLength; i++) {
    const t = i / c.sampleRate;
    // Exponential decay with random noise.
    irData[i] = (Math.random() * 2 - 1) * Math.exp(-3 * t);
  }

  const convolver = c.createConvolver();
  convolver.buffer = irBuffer;

  const wetGain = c.createGain();
  wetGain.gain.value = 0.3;

  inputGain.connect(convolver);
  convolver.connect(wetGain);
  wetGain.connect(ensureMainNode());

  reverbSendBus = inputGain;
  return reverbSendBus;
}

function ensureSendGains(id) {
  if (sendGains.has(id)) return sendGains.get(id);
  if (!hasCtx()) return null;
  const c = ctx;
  const chain = getChannelChain(id);
  if (!chain) return null;

  const delaySend = c.createGain();
  delaySend.gain.value = 0; // default: no send
  chain.gain.connect(delaySend);
  const delayBus = ensureDelaySendBus();
  if (delayBus) delaySend.connect(delayBus);

  const reverbSend = c.createGain();
  reverbSend.gain.value = 0; // default: no send
  chain.gain.connect(reverbSend);
  const reverbBus = ensureReverbSendBus();
  if (reverbBus) reverbSend.connect(reverbBus);

  const sg = { delay: delaySend, reverb: reverbSend };
  sendGains.set(id, sg);
  return sg;
}

// Expose send level control to Go/WASM.
window.setDelaySend = (id, amount) => {
  const sg = ensureSendGains(id);
  if (sg) sg.delay.gain.value = Math.max(0, Math.min(1, amount || 0));
};

window.setReverbSend = (id, amount) => {
  const sg = ensureSendGains(id);
  if (sg) sg.reverb.gain.value = Math.max(0, Math.min(1, amount || 0));
};

window.delaySend = (id) => {
  const sg = sendGains.get(id);
  return sg ? sg.delay.gain.value : 0;
};

window.reverbSend = (id) => {
  const sg = sendGains.get(id);
  return sg ? sg.reverb.gain.value : 0;
};

function buildChannelPipeline(id) {
  if (!hasCtx()) return null;
  const c = ctx;
  const ingress = c.createGain();
  ingress.gain.value = 1;
  const gain = c.createGain();
  gain.gain.value = 1;
  ingress.connect(gain);
  const main = ensureMainNode();
  if (main) gain.connect(main);
  const chain = { ingress, gain, analyser: null, eqChain: [], insertFX: [] };
  channelNodes.set(id, chain);
  return chain;
}

function getChannelChain(id) {
  const key = id || 'main';
  if (key === 'main') {
    const main = ensureMainNode();
    if (!main) return null;
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
  if (!chain) return null;
  return chain.ingress;
}

export function setChannelVolume(id, vol) {
  if (!hasCtx()) {
    // Queue the operation for when context is created.
    pendingChannelOps.push(() => setChannelVolume(id, vol));
    return;
  }
  const chain = getChannelChain(id);
  if (!chain) return;
  chain.gain.gain.value = clampVolume(vol);
}

export function channelVolume(id) {
  if (!hasCtx()) return 1; // default volume when no context
  const chain = getChannelChain(id);
  if (!chain) return 1;
  return chain.gain.gain.value;
}

export function setMainVolume(vol) { setChannelVolume('main', vol); }
export function mainVolume() { return channelVolume('main'); }

// Stereo panning via StereoPannerNode. pan: -1 (left) to +1 (right), 0 = center.
export function setChannelPan(id, pan) {
  if (!hasCtx()) {
    pendingChannelOps.push(() => setChannelPan(id, pan));
    return;
  }
  const chain = getChannelChain(id);
  if (!chain) return;
  // Create StereoPannerNode lazily.
  if (!chain.panner) {
    const c = ctx;
    chain.panner = c.createStereoPanner();
    // Insert panner after gain, before EQ chain / destination.
    // The rewireChannel function will include it in the signal path.
    chain.panner.pan.value = Math.max(-1, Math.min(1, pan));
  } else {
    chain.panner.pan.value = Math.max(-1, Math.min(1, pan));
  }
}

export function channelPan(id) {
  if (!hasCtx()) return 0;
  const chain = getChannelChain(id);
  if (!chain || !chain.panner) return 0;
  return chain.panner.pan.value;
}

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
    case 'lowpass': node.type = 'lowpass'; break;
    case 'highpass': node.type = 'highpass'; break;
    default: node.type = 'peaking'; break;
  }
  return node;
}

// Band definitions for the 10-band ISO standard EQ - matches Go's defaultBandDefs
const BAND_DEFS = [
  { loHz: 22, hiHz: 44 },      // 31 Hz
  { loHz: 44, hiHz: 88 },      // 62 Hz
  { loHz: 88, hiHz: 177 },     // 125 Hz
  { loHz: 177, hiHz: 354 },    // 250 Hz
  { loHz: 354, hiHz: 707 },    // 500 Hz
  { loHz: 707, hiHz: 1414 },   // 1 kHz
  { loHz: 1414, hiHz: 2828 },  // 2 kHz
  { loHz: 2828, hiHz: 5657 },  // 4 kHz
  { loHz: 5657, hiHz: 11314 }, // 8 kHz
  { loHz: 11314, hiHz: 20000 }, // 16 kHz
];

// Creates a serial peaking-EQ multiband processor. Each band is a peaking
// filter in series — muted bands get a deep cut (-60 dB), others get the
// requested gain. Serial topology means cuts can only reduce signal, never
// boost, avoiding the gain-staging issues of parallel crossover approaches.
function createMultibandProcessor(c, bands) {
  const numBands = bands.length;
  if (numBands === 0) {
    const passthrough = c.createGain();
    passthrough.gain.value = 1;
    return { input: passthrough, output: passthrough, isMultiband: true };
  }

  const allMuted = bands.every((b) => b?.muted === true);
  if (allMuted) {
    const silentInput = c.createGain();
    silentInput.gain.value = 0;
    return { input: silentInput, output: silentInput, isMultiband: true };
  }

  const input = c.createGain();
  input.gain.value = 1;

  const defs = numBands === BAND_DEFS.length ? BAND_DEFS : BAND_DEFS.slice(0, numBands);
  let lastNode = input;

  for (let i = 0; i < numBands; i++) {
    const band = bands[i];
    const def = defs[i] || { loHz: 20, hiHz: 20000 };
    const isMuted = band?.muted === true;
    const gainDB = Number.isFinite(band?.gainDB) ? band.gainDB : (band?.gain || 0);

    // Skip bands at unity gain that aren't muted
    if (!isMuted && Math.abs(gainDB) < 0.01) continue;

    const effectiveGainDB = isMuted ? -60 : gainDB;
    const centerFreq = Math.sqrt(def.loHz * def.hiHz);
    const bwOctaves = Math.log2(def.hiHz / def.loHz);
    const Q = 1 / (2 * Math.sinh(Math.LN2 / 2 * bwOctaves));

    const peaking = c.createBiquadFilter();
    peaking.type = 'peaking';
    peaking.frequency.value = centerFreq;
    peaking.Q.value = Q;
    peaking.gain.value = effectiveGainDB;
    lastNode.connect(peaking);
    lastNode = peaking;

    // For muted bands, cascade a second peaking filter at the same settings
    // to deepen the notch across the full band width (-120 dB at center).
    if (isMuted) {
      const peaking2 = c.createBiquadFilter();
      peaking2.type = 'peaking';
      peaking2.frequency.value = centerFreq;
      peaking2.Q.value = Q;
      peaking2.gain.value = effectiveGainDB;
      lastNode.connect(peaking2);
      lastNode = peaking2;
    }
  }

  const output = c.createGain();
  output.gain.value = 1;
  lastNode.connect(output);

  return { input, output, isMultiband: true };
}

// ─── INSERT EFFECTS ───

// AudioWorklet-based insert effects: uses Emscripten-compiled C effects
// for exact parity with desktop and full bitcrusher support on WASM.
let workletReady = false;
let _workletInitPromise = null;

// Try to register the AudioWorklet processor. Called once after AudioContext creation.
async function _initInsertFXWorklet() {
  if (workletReady || !hasCtx()) return false;
  try {
    // Resolve worklet URL relative to audio.js location.
    const workletURL = new URL('./insert_fx_worklet.js', import.meta.url).href;
    await ctx.audioWorklet.addModule(workletURL);
    workletReady = true;
    dbg('insert.worklet.ready');
    return true;
  } catch (err) {
    dbg('insert.worklet.fallback', { err: String(err) });
    return false;
  }
}

// Create an AudioWorkletNode for insert effects on a channel.
function _createWorkletInsertNode(c, slots) {
  if (!workletReady) return null;
  try {
    const node = new AudioWorkletNode(c, 'insert-fx-processor', {
      numberOfInputs: 1,
      numberOfOutputs: 1,
      outputChannelCount: [1],
    });

    // Send module initialization (the worklet needs access to the WASM functions).
    // Since we can't share the module directly, we send the compiled slots config
    // and the worklet uses its own module loading.
    // For now, we configure the effect chain and it processes natively.
    if (mod) {
      // The worklet loads its own WASM module. Send configuration only.
      node.port.postMessage({ type: 'configure', slots });
    }

    return node;
  } catch (err) {
    dbg('insert.worklet.create.error', { err: String(err) });
    return null;
  }
}

// Creates a WebAudio subgraph for a single insert effect slot.
// Returns { input, output, nodes } or null if the effect type is unknown.
function createInsertEffectSubgraph(c, slot) {
  const p = slot.params || {};
  const mix = Math.max(0, Math.min(1, p.mix ?? 1));

  switch (slot.type) {
    case 'distortion': {
      // WaveShaperNode with tanh curve controlled by drive.
      const drive = Math.max(1, Math.min(20, p.drive ?? 2));
      const ws = c.createWaveShaper();
      const samples = 8192;
      const curve = new Float32Array(samples);
      for (let i = 0; i < samples; i++) {
        const x = (i * 2) / samples - 1;
        curve[i] = Math.tanh(x * drive);
      }
      ws.curve = curve;
      ws.oversample = '2x';
      // Wet/dry via parallel gain nodes.
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wet = c.createGain(); wet.gain.value = mix;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(ws); ws.connect(wet); wet.connect(output);
      return { input, output, nodes: [ws, dry, wet] };
    }
    case 'delay': {
      const time = Math.max(0.01, Math.min(1, (p.time ?? 250) / 1000));
      const feedback = Math.max(0, Math.min(0.95, p.feedback ?? 0.4));
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wet = c.createGain(); wet.gain.value = mix;
      const delay = c.createDelay(2);
      delay.delayTime.value = time;
      const fbGain = c.createGain(); fbGain.gain.value = feedback;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(delay); delay.connect(wet); wet.connect(output);
      delay.connect(fbGain); fbGain.connect(delay);
      return { input, output, nodes: [delay, fbGain, dry, wet] };
    }
    case 'reverb': {
      const room = Math.max(0, Math.min(1, p.room ?? 0.5));
      const damping = Math.max(0, Math.min(1, p.damping ?? 0.5));
      // Synthetic impulse response scaled by room size.
      const irLen = Math.floor(c.sampleRate * (0.5 + room * 2.5));
      const irBuf = c.createBuffer(1, irLen, c.sampleRate);
      const irData = irBuf.getChannelData(0);
      const decay = 2 + damping * 6;
      for (let i = 0; i < irLen; i++) {
        const t = i / c.sampleRate;
        irData[i] = (Math.random() * 2 - 1) * Math.exp(-decay * t);
      }
      const conv = c.createConvolver();
      conv.buffer = irBuf;
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wet = c.createGain(); wet.gain.value = mix;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(conv); conv.connect(wet); wet.connect(output);
      return { input, output, nodes: [conv, dry, wet] };
    }
    case 'chorus': {
      const rate = Math.max(0.1, Math.min(10, p.rate ?? 1.5));
      const depth = Math.max(0, Math.min(0.02, (p.depth ?? 5) / 1000));
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wet = c.createGain(); wet.gain.value = mix;
      const delay = c.createDelay(0.1);
      delay.delayTime.value = 0.01;
      const lfo = c.createOscillator();
      lfo.type = 'sine';
      lfo.frequency.value = rate;
      const lfoGain = c.createGain();
      lfoGain.gain.value = depth;
      lfo.connect(lfoGain);
      lfoGain.connect(delay.delayTime);
      lfo.start();
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(delay); delay.connect(wet); wet.connect(output);
      return { input, output, nodes: [delay, lfo, lfoGain, dry, wet], oscillators: [lfo] };
    }
    case 'bitcrusher': {
      // Bitcrusher is hard to do with native nodes — use a simple gain as
      // a placeholder. The Go desktop side does real bitcrushing.
      const input = c.createGain(); input.gain.value = 1;
      return { input, output: input, nodes: [] };
    }
    case 'filter': {
      const mode = Math.round(p.mode ?? 0);
      const cutoff = Math.max(20, Math.min(20000, p.cutoff ?? 1000));
      const q = Math.max(0.1, Math.min(10, p.q ?? 0.707));
      const bq = c.createBiquadFilter();
      bq.type = mode === 2 ? 'bandpass' : mode === 1 ? 'highpass' : 'lowpass';
      bq.frequency.value = cutoff;
      bq.Q.value = q;
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wet = c.createGain(); wet.gain.value = mix;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(bq); bq.connect(wet); wet.connect(output);
      return { input, output, nodes: [bq, dry, wet] };
    }
    case 'waveshaper': {
      const drive = Math.max(1, Math.min(20, p.drive ?? 2));
      const curve_type = Math.round(p.curve ?? 0);
      const ws = c.createWaveShaper();
      const samples = 8192;
      const curve = new Float32Array(samples);
      for (let i = 0; i < samples; i++) {
        const x = (i * 2) / samples - 1;
        const driven = x * drive;
        switch (curve_type) {
        case 1: curve[i] = Math.max(-1, Math.min(1, driven)); break;
        case 2: { let v = driven; while (v > 1 || v < -1) { if (v > 1) v = 2 - v; if (v < -1) v = -2 - v; } curve[i] = v; break; }
        case 3: curve[i] = Math.sin(driven * Math.PI * 0.5); break;
        default: curve[i] = Math.tanh(driven); break;
        }
      }
      ws.curve = curve;
      ws.oversample = '2x';
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wet = c.createGain(); wet.gain.value = mix;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(ws); ws.connect(wet); wet.connect(output);
      return { input, output, nodes: [ws, dry, wet] };
    }
    case 'ringmod': {
      const freq = Math.max(20, Math.min(5000, p.frequency ?? 440));
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wet = c.createGain(); wet.gain.value = 0;
      const carrier = c.createOscillator();
      carrier.type = (p.shape ?? 0) >= 0.5 ? 'square' : 'sine';
      carrier.frequency.value = freq;
      const modGain = c.createGain(); modGain.gain.value = mix;
      carrier.connect(modGain);
      modGain.connect(input.gain);
      carrier.start();
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(output);
      return { input, output, nodes: [carrier, modGain, dry, wet], oscillators: [carrier] };
    }
    case 'tremolo': {
      const rate = Math.max(0.5, Math.min(20, p.rate ?? 4));
      const depth = Math.max(0, Math.min(1, p.depth ?? 0.5));
      const input = c.createGain(); input.gain.value = 1;
      const tremGain = c.createGain(); tremGain.gain.value = 1 - depth * 0.5;
      const lfo = c.createOscillator();
      lfo.type = 'sine';
      lfo.frequency.value = rate;
      const lfoGain = c.createGain(); lfoGain.gain.value = depth * 0.5;
      lfo.connect(lfoGain);
      lfoGain.connect(tremGain.gain);
      lfo.start();
      const output = c.createGain(); output.gain.value = 1;
      input.connect(tremGain); tremGain.connect(output);
      return { input, output, nodes: [tremGain, lfo, lfoGain], oscillators: [lfo] };
    }
    case 'gate':
    case 'transient':
    case 'pitchshift': {
      // No good native WebAudio equivalent; passthrough placeholder.
      const input = c.createGain(); input.gain.value = 1;
      return { input, output: input, nodes: [] };
    }
    case 'limiter':
    case 'compressor': {
      const comp = c.createDynamicsCompressor();
      comp.threshold.value = p.threshold ?? -20;
      comp.ratio.value = p.ratio ?? 12;
      comp.attack.value = (p.attack ?? 10) / 1000;
      comp.release.value = (p.release ?? 100) / 1000;
      const input = c.createGain(); input.gain.value = 1;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(comp); comp.connect(output);
      return { input, output, nodes: [comp] };
    }
    case 'flanger': {
      const rate = Math.max(0.1, Math.min(10, p.rate ?? 0.5));
      const depth = Math.max(0, Math.min(0.01, (p.depth ?? 3) / 1000));
      const feedback = Math.max(-0.95, Math.min(0.95, p.feedback ?? 0.5));
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wet = c.createGain(); wet.gain.value = mix;
      const delay = c.createDelay(0.05);
      delay.delayTime.value = 0.003;
      const lfo = c.createOscillator(); lfo.type = 'sine'; lfo.frequency.value = rate;
      const lfoGain = c.createGain(); lfoGain.gain.value = depth;
      lfo.connect(lfoGain); lfoGain.connect(delay.delayTime); lfo.start();
      const fbGain = c.createGain(); fbGain.gain.value = feedback;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(delay); delay.connect(wet); wet.connect(output);
      delay.connect(fbGain); fbGain.connect(delay);
      return { input, output, nodes: [delay, lfo, lfoGain, fbGain, dry, wet], oscillators: [lfo] };
    }
    case 'phaser':
    case 'autowah': {
      // Complex filter chain not well-supported by native nodes; passthrough.
      const input = c.createGain(); input.gain.value = 1;
      return { input, output: input, nodes: [] };
    }
    case 'tape': {
      // Saturation via waveshaper + warmth LP filter
      const drive = Math.max(1, Math.min(10, p.drive ?? 2));
      const ws = c.createWaveShaper();
      const samples = 8192;
      const curve = new Float32Array(samples);
      const norm = Math.tanh(drive);
      for (let i = 0; i < samples; i++) {
        const x = (i * 2) / samples - 1;
        curve[i] = norm > 0.001 ? Math.tanh(drive * x) / norm : x;
      }
      ws.curve = curve; ws.oversample = '2x';
      const warmth = Math.max(0, Math.min(1, p.warmth ?? 0.5));
      const lp = c.createBiquadFilter(); lp.type = 'lowpass';
      lp.frequency.value = 2000 + (1 - warmth) * 18000; lp.Q.value = 0.707;
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wetG = c.createGain(); wetG.gain.value = mix;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(ws); ws.connect(lp); lp.connect(wetG); wetG.connect(output);
      return { input, output, nodes: [ws, lp, dry, wetG] };
    }
    default:
      return null;
  }
}

// Disconnect and clean up insert effect subgraphs on a channel chain.
function disconnectInsertEffects(chain) {
  if (!chain.insertFX) return;
  for (const fx of chain.insertFX) {
    if (fx.isWorklet) continue; // Worklet node is kept alive, just reconfigured.
    try { fx.input.disconnect(); } catch (_) {}
    try { fx.output.disconnect(); } catch (_) {}
    if (fx.nodes) {
      for (const n of fx.nodes) { try { n.disconnect(); } catch (_) {} }
    }
    if (fx.oscillators) {
      for (const o of fx.oscillators) { try { o.stop(); } catch (_) {} }
    }
  }
  chain.insertFX = [];
}

// updateInsertEffects rebuilds the insert effect WebAudio subgraph for
// an instrument channel. Called from Go via platformInsertEffectsChanged.
// slotsJSON is a JSON-encoded array of EffectSlot objects.
//
// Uses AudioWorklet (C effects) when available, with fallback to WebAudio
// node graph (JS re-implementations) for browsers that don't support worklets.
function updateInsertEffects(id, slotsJSON) {
  if (!hasCtx()) return;
  const chain = getChannelChain(id);
  if (!chain) return;

  let slots;
  try {
    slots = typeof slotsJSON === 'string' ? JSON.parse(slotsJSON) : slotsJSON;
  } catch (_) {
    slots = [];
  }
  if (!Array.isArray(slots)) slots = [];

  // Tear down old insert effects (both worklet and WebAudio nodes).
  disconnectInsertEffects(chain);

  // If worklet node exists for this channel, update it with new config.
  if (chain.workletInsertNode) {
    const enabledSlots = slots.filter(s => s.enabled);
    if (enabledSlots.length > 0) {
      chain.workletInsertNode.port.postMessage({ type: 'configure', slots: enabledSlots });
      // Wrap worklet node as a single "insert FX" entry for rewireChannel.
      chain.insertFX = [{
        input: chain.workletInsertNode,
        output: chain.workletInsertNode,
        nodes: [],
        isWorklet: true,
      }];
    } else {
      // No enabled effects — disconnect worklet.
      try { chain.workletInsertNode.disconnect(); } catch (_) {}
      chain.workletInsertNode = null;
      chain.insertFX = [];
    }
  } else {
    // Fallback: build WebAudio node graph subgraphs.
    const c = ctx;
    chain.insertFX = [];
    for (const slot of slots) {
      if (!slot.enabled) continue;
      const sg = createInsertEffectSubgraph(c, slot);
      if (sg) chain.insertFX.push(sg);
    }
  }

  // Rewire the channel to include insert effects.
  const oldMb = chain.multibandProc;
  rewireChannel(chain, oldMb);
}

window.updateInsertEffects = (id, slotsJSON) => {
  try { updateInsertEffects(id, slotsJSON); }
  catch (err) { dbg('insert.effect.error', { id, err: String(err) }); }
};

function rewireChannel(chain, oldMultibandProc = null) {
  try { chain.ingress.disconnect(); } catch (_) {}
  // Disconnect insert FX outgoing connections only (preserve internal wiring).
  if (chain.insertFX) {
    for (const fx of chain.insertFX) {
      try { fx.output.disconnect(); } catch (_) {}
    }
  }
  if (chain.eqChain) {
    for (const f of chain.eqChain) { try { f.disconnect(); } catch (_) {} }
  }
  // Disconnect OLD multiband processor (passed explicitly), not the current one.
  // This prevents breaking the internal connections of a newly created multiband processor.
  if (oldMultibandProc) {
    try { oldMultibandProc.input.disconnect(); } catch (_) {}
    try { oldMultibandProc.output.disconnect(); } catch (_) {}
  }
  if (chain.preEQAnalyser) { try { chain.preEQAnalyser.disconnect(); } catch (_) {} }
  if (chain.analyser) { try { chain.analyser.disconnect(); } catch (_) {} }

  // Helper: wire insert effects into the signal chain starting from `node`.
  // Returns the last node in the chain after insert effects.
  function wireInsertFX(node) {
    if (chain.insertFX && chain.insertFX.length > 0) {
      for (const fx of chain.insertFX) {
        node.connect(fx.input);
        node = fx.output;
      }
    }
    return node;
  }

  // Special-case the main bus (ingress === gain) to avoid creating a feedback
  // loop when an analyser/EQ is inserted. We tap the analyser and then go
  // straight to the destination.
  if (chain.ingress === chain.gain) {
    let node = chain.ingress;
    // Insert effects (before EQ)
    node = wireInsertFX(node);
    // Pre-EQ analyser tap (after inserts, before EQ) — fan-out only
    if (chain.preEQAnalyser) {
      node.connect(chain.preEQAnalyser);
    }
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
      chain.analyser.connect(audioDestination());
    } else {
      node.connect(audioDestination());
    }
    return;
  }

  let node = chain.ingress;
  // Insert effects (before EQ)
  node = wireInsertFX(node);
  // Pre-EQ analyser tap (after inserts, before EQ) — fan-out only
  if (chain.preEQAnalyser) {
    node.connect(chain.preEQAnalyser);
  }
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
  if (!hasCtx()) {
    // Queue the operation for when context is created.
    pendingChannelOps.push(() => setChannelEQ(id, bands));
    return;
  }
  const chain = getChannelChain(id);
  if (!chain) return;
  const c = ctx;
  if (!Array.isArray(bands) || bands.length === 0) {
    const oldMb = chain.multibandProc;
    chain.eqChain = [];
    chain.multibandProc = null;
    rewireChannel(chain, oldMb);
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
    const oldMb = chain.multibandProc;
    const silenceGain = c.createGain();
    silenceGain.gain.value = 0;
    chain.eqChain = [silenceGain];
    chain.multibandProc = null;
    rewireChannel(chain, oldMb);
    return;
  }

  // Check if any band is muted - if so, use multiband processor for proper isolation
  if (mutedCount > 0) {
    const oldMb = chain.multibandProc;
    const mb = createMultibandProcessor(c, bands);
    chain.multibandProc = mb;
    chain.eqChain = [];
    rewireChannel(chain, oldMb);
    return;
  }

  // No muting - use simple biquad chain for efficiency
  const oldMb = chain.multibandProc;
  chain.multibandProc = null;
  chain.eqChain = bands.map((b) => makeBiquad(c, b));
  rewireChannel(chain, oldMb);
}

export function enableChannelAnalyzer(id, opts = {}) {
  if (!hasCtx()) {
    pendingChannelOps.push(() => enableChannelAnalyzer(id, opts));
    return;
  }
  const chain = getChannelChain(id);
  if (!chain) return;
  const c = ctx;
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
  const a = chain?.analyser;
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

export function enablePreEQAnalyzer(id, opts = {}) {
  if (!hasCtx()) {
    pendingChannelOps.push(() => enablePreEQAnalyzer(id, opts));
    return;
  }
  const chain = getChannelChain(id);
  if (!chain) return;
  const c = ctx;
  const fft = (() => {
    const v = opts.fftSize || 512;
    return Math.pow(2, Math.round(Math.log2(Math.max(64, Math.min(8192, v)))));
  })();
  const a = c.createAnalyser();
  a.fftSize = fft;
  a.smoothingTimeConstant = Math.max(0, Math.min(0.95, opts.smoothing || 0.0));
  chain.preEQAnalyser = a;
  chain._preEQTimeBuf = new Float32Array(a.fftSize);
  chain._preEQFreqBuf = new Float32Array(a.frequencyBinCount);
  rewireChannel(chain);
}

export function preEQAnalyzerSnapshot(id, bins = 64) {
  const chain = getChannelChain(id);
  const a = chain?.preEQAnalyser;
  if (!a) {
    return { rms: 0, peak: 0, spectrum: [], wave: [] };
  }
  const time = chain._preEQTimeBuf || new Float32Array(a.fftSize);
  const freq = chain._preEQFreqBuf || new Float32Array(a.frequencyBinCount);
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
    spectrum.push(Math.pow(10, max / 20));
  }
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
  const now = (hasCtx() ? ctx.currentTime : 0) || 0;
  let bus = volumeBus.get(key);
  let per = busIndex.get(id);
  if (!per) { per = new Map(); busIndex.set(id, per); }
  if (!bus) {
    const c = getCtx();
    bus = c.createGain();
    bus.gain.value = q / VOL_Q;
    const sink = channelSink(id);
    if (sink) bus.connect(sink);
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

function maybeEvictBuses(id, _now) {
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
// Exposed as window.audioReady so index.html can gate game start on it.
async function warmupAudio() {
  await ensureModule();
  const ids = ['snare', 'kick', 'hihat', 'tom', 'clap', 'cowbell',
               'rimshot', 'sidestick', 'kick-deep', 'shaker', 'ride', 'crash',
               'kick-tight'];
  await Promise.all(ids.map(id => {
    if (RENDER[id] && !renderCache.has(id)) {
      return ensureRenderedSample(id).catch(() => {});
    }
    return Promise.resolve();
  }));
}
window.audioReady = warmupAudio();

// --- Media Session API (mobile lock screen / notification controls) ---

/* global mediaSessionPlay, mediaSessionPause, startPlay, stopPlay, isPlaying, getBPM */
function setupMediaSession() {
  if (!('mediaSession' in navigator)) return;
  navigator.mediaSession.metadata = new MediaMetadata({
    title: 'Beatmo',
    artist: 'Beatmo',
  });
  navigator.mediaSession.setActionHandler('play', () => {
    if (typeof mediaSessionPlay === 'function') {
      mediaSessionPlay();
    } else if (typeof startPlay === 'function') {
      startPlay();
    }
  });
  navigator.mediaSession.setActionHandler('pause', () => {
    if (typeof mediaSessionPause === 'function') {
      mediaSessionPause();
    } else if (typeof stopPlay === 'function') {
      stopPlay();
    }
    if (silentAudioEl) silentAudioEl.pause();
  });
  navigator.mediaSession.setActionHandler('stop', () => {
    if (typeof stopPlay === 'function') stopPlay();
    if (silentAudioEl) silentAudioEl.pause();
  });
}

window.updateMediaSessionState = function() {
  if (!('mediaSession' in navigator)) return;
  const playing = typeof isPlaying === 'function' && isPlaying();
  navigator.mediaSession.playbackState = playing ? 'playing' : 'paused';
  const bpm = typeof getBPM === 'function' ? getBPM() : 0;
  if (bpm > 0) {
    navigator.mediaSession.metadata = new MediaMetadata({
      title: 'Beatmo - ' + bpm + ' BPM',
      artist: 'Beatmo',
    });
  }
  if (silentAudioEl) {
    if (playing) {
      silentAudioEl.play().catch(() => {});
    } else {
      silentAudioEl.pause();
    }
  }
};

setupMediaSession();

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

// Phase A: Render C synth to raw Float32Array without creating an AudioContext.
// This allows the warmup IIFE to pre-render samples before any user gesture,
// which is critical on mobile where premature AudioContext creation starts suspended.
async function ensureRenderedSample(id) {
  if (!RENDER[id]) {
    return null;
  }
  // Use getSampleRate() to avoid creating AudioContext prematurely.
  // If ctx already exists we get the real rate; otherwise 48000 default.
  const sr = Math.max(8000, Math.min(192000, getSampleRate()));
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
  const info = RENDER_INFO[id] || { seconds: 0.5, amp: 0.8 };
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
    // Store raw Float32Array + metadata. AudioBuffer is created lazily in
    // ensureAudioBuffer() when playback actually needs it, so this path
    // never forces an AudioContext into existence.
    const record = { buffer: null, data, sr, frames };
    renderCache.set(id, record);
    try {
      if (typeof window !== 'undefined') {
        const metrics = window.__audioMetrics || (window.__audioMetrics = { renders: {}, cacheHits: {} });
        metrics.renders[id] = (metrics.renders[id] || 0) + 1;
        const meta = window.__renderMeta || (window.__renderMeta = {});
        meta[id] = { sr, frames, seconds: frames / sr };
      }
    } catch (_) {}
    return record;
  } finally {
    m._free(ptr);
  }
}

// Phase B: Lazily create an AudioBuffer from the cached Float32Array.
// Called at play time when we actually need a buffer source node.
// WebAudio handles resampling automatically if the buffer's sample rate
// differs from the context's rate, so we don't need to re-render.
function ensureAudioBuffer(id) {
  const record = renderCache.get(id);
  if (!record || !record.data) return null;
  if (record.buffer) return record.buffer;
  const c = getCtx();
  const buffer = c.createBuffer(1, record.frames, record.sr);
  buffer.copyToChannel(record.data, 0);
  record.buffer = buffer;
  samples[id] = buffer;
  return buffer;
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
      ensureAudioBuffer(id);
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
      ensureAudioBuffer(id);
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
// Uses a 20ms gain ramp to prevent audible clicks (anti-pop).
window.stopSound = (id) => {
  const key = id != null ? String(id) : '';
  if (!key) return;
  const set = activeSources.get(key);
  if (!set || set.size === 0) return;
  dbg('stopSound', { id: key, count: set.size });
  const now = hasCtx() ? ctx.currentTime : 0;
  const fadeOutSec = 0.02; // 20ms fade-out
  for (const src of Array.from(set)) {
    try {
      // Use the anti-pop GainNode for smooth fade-out if available.
      const fadeGain = src._antiPopGain;
      if (fadeGain && hasCtx()) {
        fadeGain.gain.cancelScheduledValues(now);
        fadeGain.gain.setValueAtTime(fadeGain.gain.value, now);
        fadeGain.gain.linearRampToValueAtTime(0, now + fadeOutSec);
        src.stop(now + fadeOutSec + 0.001);
      } else {
        src.stop();
      }
    } catch (_) {
      try { src.stop(); } catch (_2) {}
    }
    // Disconnect after fade completes (or immediately if no ctx).
    try {
      if (hasCtx()) {
        setTimeout(() => { try { src.disconnect(); } catch (_) {} }, (fadeOutSec + 0.01) * 1000);
      } else {
        src.disconnect();
      }
    } catch (_) {}
  }
  // Clean up tracking after fade-out window.
  const cleanupMs = hasCtx() ? (fadeOutSec + 0.05) * 1000 : 0;
  setTimeout(() => {
    try { set.clear(); } catch (_) {}
    try { activeSources.delete(key); } catch (_) {}
  }, cleanupMs);
};

window.setChannelVolume = (id, vol) => {
  try { setChannelVolume(id, vol); } catch (err) { dbg('channel.set.error', { id, err: String(err) }); }
};

window.channelVolume = (id) => {
  try { return channelVolume(id); } catch (err) { dbg('channel.get.error', { id, err: String(err) }); return 1; }
};

window.setMainVolume = (vol) => window.setChannelVolume('main', vol);
window.mainVolume = () => window.channelVolume('main');

window.setChannelPan = (id, pan) => {
  try { setChannelPan(id, pan); } catch (err) { dbg('channel.pan.error', { id, err: String(err) }); }
};

window.channelPan = (id) => {
  try { return channelPan(id); } catch (err) { dbg('channel.pan.get.error', { id, err: String(err) }); return 0; }
};

window.setChannelEQ = (id, bands) => {
  try { setChannelEQ(id, bands); } catch (err) { dbg('channel.eq.error', { id, err: String(err) }); }
};

window.enableChannelAnalyzer = (id, windowSize) => {
  try { enableChannelAnalyzer(id, { fftSize: windowSize }); } catch (err) { dbg('channel.analyzer.error', { id, err: String(err) }); }
};

window.channelAnalyzerSnapshot = (id) => {
  try { return channelAnalyzerSnapshot(id); } catch (err) { dbg('channel.analyzer.snap.error', { id, err: String(err) }); return { rms: 0, peak: 0, spectrum: [], wave: [] }; }
};

window.enablePreEQAnalyzer = (id, windowSize) => {
  try { enablePreEQAnalyzer(id, { fftSize: windowSize }); } catch (err) { dbg('preEQ.analyzer.error', { id, err: String(err) }); }
};

window.preEQAnalyzerSnapshot = (id) => {
  try { return preEQAnalyzerSnapshot(id); } catch (err) { dbg('preEQ.analyzer.snap.error', { id, err: String(err) }); return { rms: 0, peak: 0, spectrum: [], wave: [] }; }
};

// Batch scheduling API to reduce Go→JS crossings in WASM builds.
// Expects an Array of objects: { id, vol, pitch, dur, when? }.
// Uses a synchronous fast path when samples are pre-rendered/decoded.
window.playSoundsBatch = (arr) => {
  try {
    if (!Array.isArray(arr)) {
      if (window.BEATMO_AUDIO_DEBUG && arr && typeof arr === 'object') {
        dbg('play.params.enqueue', { id: arr.id, vol: arr.vol, pitch: arr.pitch, dur: arr.dur, when: arr.when });
      }
      enqueueAudioEvents([arr]);
    } else {
      if (window.BEATMO_AUDIO_DEBUG) {
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
window.audioNow = () => {
  try {
    // Don't create a context just to read the time.
    if (!hasCtx()) return 0;
    return ctx.currentTime;
  } catch (_) { return 0; }
};
window.resumeAudio = () => {
  try {
    // Don't create context here — wait for a gesture via unlockAudio.
    if (!hasCtx()) return null;
    // Keep the silent <audio> element playing to maintain iOS speaker routing.
    if (silentAudioEl) silentAudioEl.play().catch(() => {});
    const c = ctx;
    // Play a silent buffer to unlock on iOS when called from a gesture handler.
    if (c.state !== 'running') {
      try {
        const sb = c.createBuffer(1, 1, c.sampleRate);
        const src = c.createBufferSource();
        src.buffer = sb;
        src.connect(c.destination);
        src.start(0);
      } catch (_) {}
    }
    const p = c.resume();
    // After resume succeeds, flush any audio events that were deferred while
    // the context was suspended. This is the most reliable flush trigger on
    // mobile — the user just tapped Play, so the browser honors ctx.resume().
    if (p && typeof p.then === 'function') {
      p.then(() => {
        if (queueSize() > 0) scheduleAudioFlush(true);
      }).catch(() => {});
    }
    return p;
  } catch (_) { return null; }
};

// Download a JSON blob with a given filename.
window.downloadJSON = (name, text) => {
  try {
    const blob = new Blob([text], { type: 'application/json' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = name || 'beatmo-export.json';
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
  // Check for a pending mobile file pick from the gesture-based rect system.
  const pending = window._fpConsumePending?.('import');
  if (pending) {
    console.log('[IMPORT] consuming pending mobile file pick');
    pending.then(r => resolve(r?.data || '')).catch(() => resolve(''));
    return;
  }
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

// Open a file picker for WAV files. Returns a Promise<{url, name} | null>.
// On mobile, checks for a pending pick from the gesture-based rect system.
window.openWAVFile = () => new Promise((resolve) => {
  const pending = window._fpConsumePending?.('upload');
  if (pending) {
    pending.then(r => resolve(r ? { url: r.data, name: r.name } : null)).catch(() => resolve(null));
    return;
  }
  // Desktop fallback: create file input directly.
  try {
    const input = document.createElement('input');
    input.type = 'file';
    input.accept = '.wav';
    let settled = false;
    const settle = (val) => { if (!settled) { settled = true; resolve(val); } };
    input.onchange = () => {
      const file = input.files && input.files[0];
      if (!file) { settle(null); input.remove(); return; }
      const name = file.name || '';
      if (!name.toLowerCase().endsWith('.wav')) { settle(null); input.remove(); return; }
      try {
        const url = URL.createObjectURL(file);
        settle({ url, name });
      } catch (_) { settle(null); }
      input.remove();
    };
    document.body.appendChild(input);
    input.click();
    setTimeout(() => { try { if (!settled) { settle(null); input.remove(); } } catch(_){} }, 5000);
  } catch (err) { console.error('openWAVFile failed', err); resolve(null); }
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
    if (rec) {
      if (rec.buffer) return rec.buffer.duration || (rec.data ? rec.data.length / (rec.sr || 44100) : 0);
      if (rec.data) return rec.data.length / (rec.sr || 44100);
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
    pendingRenderEnsures.clear();
  } catch (_) {}
};
window.getRenderCacheStats = () => {
  const stats = {};
  try {
    renderCache.forEach((rec, id) => {
      const dur = rec && rec.buffer ? rec.buffer.duration || 0 : (rec && rec.data ? rec.data.length / (rec.sr || 44100) : 0);
      stats[id] = {
        duration: dur,
        frames: rec && rec.data ? rec.data.length : rec && rec.buffer ? rec.buffer.length || 0 : 0,
      };
    });
  } catch (_) {}
  return stats;
};

// Get the raw cached sample data for an instrument (for testing/debugging)
window.getCachedRenderData = (id) => {
  try {
    const rec = renderCache.get(id);
    if (rec && rec.data) {
      return Array.from(rec.data);
    }
  } catch (_) {}
  return null;
};
window.ensureSynthSample = async (id) => {
  try {
    const rec = await ensureRenderedSample(id);
    return !!rec;
  } catch (_) {
    return false;
  }
};

// ============================================================================
// Output Capture API - for comparing JS vs Go audio output
// ============================================================================

// Enable output capture mode - creates capture node and inserts it into the audio chain.
// Can be called at any time; will properly rewire if audio is already playing.
window.enableOutputCapture = () => {
  window.__captureOutput = true;
  outputCaptureEnabled = false;
  outputCaptureBuffer = [];

  // Create/get the audio context and main node
  const c = getCtx();
  ensureMainNode(); // Force creation of main channel node

  if (outputCaptureNode) {
    // Already have capture node
    return;
  }

  outputCaptureSampleRate = c.sampleRate;
  outputCaptureNode = c.createScriptProcessor(256, 1, 1);
  outputCaptureNode.onaudioprocess = (e) => {
    if (outputCaptureEnabled) {
      const input = e.inputBuffer.getChannelData(0);
      for (let i = 0; i < input.length; i++) {
        outputCaptureBuffer.push(input[i]);
      }
    }
    // Pass through to output
    const output = e.outputBuffer.getChannelData(0);
    const input = e.inputBuffer.getChannelData(0);
    for (let i = 0; i < input.length; i++) {
      output[i] = input[i];
    }
  };

  // Insert capture node between the limiter and the hardware destination.
  // The limiter is always the last processing node before output.
  const limiter = ensureLimiter();
  try { limiter.disconnect(c.destination); } catch (_) {}
  limiter.connect(outputCaptureNode);
  outputCaptureNode.connect(c.destination);

  console.log('[AUDIO CAPTURE] Capture node inserted into audio chain');
};

// Start capturing output samples
window.startOutputCapture = () => {
  outputCaptureBuffer = [];
  if (!outputCaptureNode) {
    window.enableOutputCapture();
  } else {
    // Re-verify capture chain is intact — Chrome's ScriptProcessorNode can
    // receive all-zero input after extended use across multiple start/stop
    // cycles. Re-wiring the limiter → captureNode → destination path fixes it.
    const c = getCtx();
    const limiter = ensureLimiter();
    if (limiter && c) {
      try { limiter.disconnect(); } catch (_) {}
      try { outputCaptureNode.disconnect(); } catch (_) {}
      limiter.connect(outputCaptureNode);
      outputCaptureNode.connect(c.destination);
    }
  }
  // Set enabled AFTER enableOutputCapture (which resets it to false).
  outputCaptureEnabled = true;
  console.log('[AUDIO CAPTURE] Started capturing output at', outputCaptureSampleRate, 'Hz');
};

// Stop capturing and return the captured samples as Float32Array
window.stopOutputCapture = () => {
  outputCaptureEnabled = false;
  const result = new Float32Array(outputCaptureBuffer);
  console.log('[AUDIO CAPTURE] Stopped. Captured', result.length, 'samples (',
    (result.length / outputCaptureSampleRate).toFixed(3), 'seconds)');
  return result;
};

// Get current capture buffer without stopping
window.getOutputCapture = () => {
  return new Float32Array(outputCaptureBuffer);
};

// Get capture statistics
window.getOutputCaptureStats = () => {
  return {
    enabled: outputCaptureEnabled,
    samples: outputCaptureBuffer.length,
    sampleRate: outputCaptureSampleRate,
    durationSec: outputCaptureBuffer.length / outputCaptureSampleRate,
    hasNode: !!outputCaptureNode,
  };
};

// Download captured audio as WAV file
window.downloadOutputCapture = (filename = 'js-audio-capture.wav') => {
  const samples = outputCaptureBuffer;
  if (samples.length === 0) {
    console.warn('[AUDIO CAPTURE] No samples captured');
    return;
  }

  // Create WAV file
  const sampleRate = outputCaptureSampleRate;
  const numChannels = 1;
  const bitsPerSample = 16;
  const bytesPerSample = bitsPerSample / 8;
  const blockAlign = numChannels * bytesPerSample;
  const byteRate = sampleRate * blockAlign;
  const dataSize = samples.length * bytesPerSample;
  const fileSize = 44 + dataSize;

  const buffer = new ArrayBuffer(fileSize);
  const view = new DataView(buffer);

  // WAV header
  const writeString = (offset, str) => {
    for (let i = 0; i < str.length; i++) {
      view.setUint8(offset + i, str.charCodeAt(i));
    }
  };

  writeString(0, 'RIFF');
  view.setUint32(4, fileSize - 8, true);
  writeString(8, 'WAVE');
  writeString(12, 'fmt ');
  view.setUint32(16, 16, true); // fmt chunk size
  view.setUint16(20, 1, true);  // PCM format
  view.setUint16(22, numChannels, true);
  view.setUint32(24, sampleRate, true);
  view.setUint32(28, byteRate, true);
  view.setUint16(32, blockAlign, true);
  view.setUint16(34, bitsPerSample, true);
  writeString(36, 'data');
  view.setUint32(40, dataSize, true);

  // Write samples as 16-bit PCM
  let offset = 44;
  for (let i = 0; i < samples.length; i++) {
    let sample = samples[i];
    // Clamp to [-1, 1]
    if (sample > 1) sample = 1;
    else if (sample < -1) sample = -1;
    // Convert to 16-bit signed integer
    const int16 = Math.floor(sample * 32767);
    view.setInt16(offset, int16, true);
    offset += 2;
  }

  // Download
  const blob = new Blob([buffer], { type: 'audio/wav' });
  const a = document.createElement('a');
  a.href = URL.createObjectURL(blob);
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  setTimeout(() => {
    URL.revokeObjectURL(a.href);
    a.remove();
  }, 100);

  console.log('[AUDIO CAPTURE] Downloaded', filename, '- ',
    samples.length, 'samples,', (samples.length / sampleRate).toFixed(3), 'seconds');
};

// Clear capture buffer
window.clearOutputCapture = () => {
  outputCaptureBuffer = [];
};

// Force-destroy the capture node so next startOutputCapture() creates a fresh one.
// Used as retry recovery when the ScriptProcessorNode becomes unreliable.
window.resetOutputCaptureNode = () => {
  if (outputCaptureNode) {
    try { outputCaptureNode.disconnect(); } catch (_) {}
    outputCaptureNode = null;
  }
  outputCaptureEnabled = false;
  outputCaptureBuffer = [];
  const c = hasCtx() ? ctx : null;
  const limiter = mainLimiter;
  if (limiter && c) {
    try { limiter.disconnect(); } catch (_) {}
    limiter.connect(c.destination);
  }
};

// ─── Multi-Channel Recording Capture (off main thread) ─────────────────────
//
// The recording pipeline runs entirely on background threads:
//
//   ┌─────────────────────────────────────────────────────────┐
//   │ Audio rendering thread (per-channel AudioWorkletNode)   │
//   │   recording-capture-processor → posts batches via       │
//   │   transferred MessagePort directly to encoder Worker    │
//   └─────────────────────────────────────────────────────────┘
//                            │ ZERO-COPY transferable
//                            ▼
//   ┌─────────────────────────────────────────────────────────┐
//   │ Web Worker (recording_encoder_worker.js)                │
//   │   WAV-encodes per channel, bundles into stored zip      │
//   │   on finalize, posts Blob back here.                    │
//   └─────────────────────────────────────────────────────────┘
//                            │ postMessage({type:'blob', blob})
//                            ▼
//   ┌─────────────────────────────────────────────────────────┐
//   │ Main thread (this module)                               │
//   │   triggerRecordingDownload(blob, filename)              │
//   │   — only invoked once per recording session.            │
//   └─────────────────────────────────────────────────────────┘
//
// Once a session is wired (after startMultiChannelCapture resolves),
// the main thread is idle in the data path. The audio thread does the
// 128-frame copy + post; the worker does the encode + zip; the main
// thread only relays the resulting Blob into a download anchor.

let _captureWorkletReadyPromise = null;
let _recordingState = null;  // { worker, nodes: Map(id, {node, msgChan, chain, isMaster}), format, sampleRate, lastStats, autoStopFired }
let _recordingFinalizePromise = null;  // resolves with {blob, meta} on finalize

async function _ensureCaptureWorklet() {
  if (!_captureWorkletReadyPromise) {
    // Force context creation if needed (idempotent).
    getCtx();
    if (!ctx || !ctx.audioWorklet) {
      _captureWorkletReadyPromise = Promise.resolve(false);
      return false;
    }
    _captureWorkletReadyPromise = (async () => {
      try {
        const url = new URL('./recording_capture_worklet.js', import.meta.url).href;
        await ctx.audioWorklet.addModule(url);
        dbg('recording.worklet.ready');
        return true;
      } catch (err) {
        console.error('[RECORDING] failed to register capture worklet:', err);
        return false;
      }
    })();
  }
  return _captureWorkletReadyPromise;
}

function _buildMasterTap(c) {
  // The master tap reads what hits the destination — we tap the limiter
  // (last node before destination) the same way startOutputCapture does.
  // We do NOT splice the captureNode INTO the chain; we add a parallel
  // path so the user-audible output is unaffected.
  const limiter = ensureLimiter ? ensureLimiter() : null;
  return limiter || c.destination;  // fallback shouldn't happen in prod
}

// startMultiChannelCapture(instrumentIDs: string[], optsJSON?: string) -> Promise<{ok: bool, error?: string}>
// Called by Go when recording starts. Spins up the worker, registers the
// worklet (if needed), creates per-channel AudioWorkletNodes, and wires
// MessagePorts so the audio thread talks directly to the worker.
//
// Returns a Promise that resolves once the worker is ready and all
// channels are configured. Caller should await before considering the
// recording "started".
window.startMultiChannelCapture = async (instrumentIDs, optsJSON) => {
  const c = getCtx();
  if (!c) {
    return { ok: false, error: 'AudioContext unavailable' };
  }
  if (_recordingState) {
    // Tear down any leftover state defensively.
    try { await window.stopMultiChannelCapture(); } catch (_) {}
  }

  const ok = await _ensureCaptureWorklet();
  if (!ok) {
    return { ok: false, error: 'AudioWorklet not supported' };
  }

  let opts = {};
  if (optsJSON && typeof optsJSON === 'string') {
    try { opts = JSON.parse(optsJSON); } catch (_) { opts = {}; }
  }
  const format = (opts.format || 'wav24');

  // Spawn worker. The URL must resolve relative to this module so it
  // works under both file:// and http:// loads.
  let worker;
  try {
    worker = new Worker(new URL('./recording_encoder_worker.js', import.meta.url),
      { type: 'classic' });
  } catch (err) {
    return { ok: false, error: 'Worker unavailable: ' + String(err) };
  }

  const state = {
    worker,
    nodes: new Map(),  // id → {node, chain, isMaster}
    format,
    sampleRate: c.sampleRate,
    lastStats: { droppedSamples: 0, bytesUsed: 0, queuedBatches: 0, maxQueueDepth: 0, elapsedSec: 0, activeChannels: 0 },
    autoStopFired: false,
    autoStopReason: '',
  };
  _recordingState = state;
  _recordingFinalizePromise = null;

  // Wait for worker ready, then route follow-on messages to handlers.
  await new Promise((resolve, reject) => {
    let timer = setTimeout(() => reject(new Error('encoder worker init timeout')), 5000);
    worker.onmessage = (e) => {
      const m = e.data;
      if (m && m.type === 'ready') {
        clearTimeout(timer);
        worker.onmessage = (ev) => _handleWorkerMessage(state, ev.data);
        resolve();
      }
    };
    worker.onerror = (err) => {
      clearTimeout(timer);
      reject(err);
    };
    worker.postMessage({
      type: 'init',
      format,
      sampleRate: c.sampleRate,
      bytesPerChannelMax: opts.bytesPerChannelMax,
      durationMaxSec: opts.durationMaxSec,
    });
  });

  // Per-channel wiring: for each instrument id and the master, create:
  //   - AudioWorkletNode (capture processor)
  //   - MessageChannel (port1 → worklet, port2 → worker)
  //   - source → captureNode → ctx.destination (parallel tap; output is
  //     pass-through inside the worklet so it doesn't sink the audio)
  const setups = [];
  // Per-instrument first…
  for (const id of instrumentIDs) {
    const chain = channelNodes.get(id);
    if (!chain || !chain.gain) continue;
    setups.push({ id, source: chain.gain, chain, isMaster: false, filename: id });
  }
  // …then master.
  setups.push({ id: 'master', source: _buildMasterTap(c), chain: null, isMaster: true, filename: 'master' });

  for (const setup of setups) {
    const node = new AudioWorkletNode(c, 'recording-capture-processor', {
      numberOfInputs: 1,
      numberOfOutputs: 1,
      outputChannelCount: [1],
    });
    const channel = new MessageChannel();

    // Attach the node listener BEFORE configuring so 'configured' isn't lost.
    const configured = new Promise((resolve) => {
      node.port.onmessage = (e) => {
        const m = e.data;
        if (m && m.type === 'configured') resolve();
      };
    });
    // Hand the worker port to the worklet (transferred).
    node.port.postMessage({
      type: 'configure',
      channelId: setup.id,
      workerPort: channel.port1,
    }, [channel.port1]);

    // Hand the worklet's other port-end to the worker (transferred).
    worker.postMessage({
      type: 'addChannel',
      id: setup.id,
      name: setup.id,
      filename: setup.filename + '.wav',
      port: channel.port2,
    }, [channel.port2]);

    await configured;

    setup.source.connect(node);
    node.connect(c.destination);  // parallel tap; pass-through
    state.nodes.set(setup.id, { node, chain: setup.chain, isMaster: setup.isMaster, source: setup.source });
  }

  dbg('recording.session.started', { channels: state.nodes.size, format });
  return { ok: true };
};

function _handleWorkerMessage(state, msg) {
  if (!msg || typeof msg !== 'object') return;
  switch (msg.type) {
  case 'stats':
    state.lastStats = msg.stats;
    break;
  case 'autoStop':
    state.autoStopFired = true;
    state.autoStopReason = msg.reason || '';
    dbg('recording.autoStop', { reason: msg.reason });
    // Best-effort auto-stop: tell each worklet to stop posting. The
    // Go side will see the autoStop flag in the next stats poll.
    for (const entry of state.nodes.values()) {
      try { entry.node.port.postMessage({ type: 'flush' }); } catch (_) {}
    }
    break;
  case 'blob':
    if (state._finalizeResolve) {
      state._finalizeResolve({
        blob: msg.blob,
        meta: msg.meta,
        autoStopped: !!msg.autoStopped,
        sessionStats: msg.sessionStats || null,
      });
      state._finalizeResolve = null;
    }
    break;
  }
}

// stopMultiChannelCapture(metaJSON?: string) -> Promise<{
//   blobURL: string, filename: string, channels: [...], autoStopped: bool,
//   stats: {...}, sampleRate: int
// }>
//
// Called by Go when recording stops. Tells each worklet to flush and
// post 'done' to the worker; tells the worker to finalize (build WAV
// headers + zip); returns once the worker has emitted the Blob.
//
// Returns an object whose `blobURL` is a fresh object URL the caller can
// drop into an <a download> click. Caller is responsible for revoking.
window.stopMultiChannelCapture = async (metaJSON) => {
  const state = _recordingState;
  if (!state) {
    return { error: 'not recording' };
  }
  let meta = {};
  if (metaJSON && typeof metaJSON === 'string') {
    try { meta = JSON.parse(metaJSON); } catch (_) { meta = {}; }
  }

  // Tell every worklet to stop accepting input and post their final 'done'
  // to the worker. The worker tracks per-channel done state.
  for (const [id, entry] of state.nodes) {
    try { entry.node.port.postMessage({ type: 'stop' }); } catch (_) {}
    try {
      if (entry.source && entry.source.disconnect) entry.source.disconnect(entry.node);
    } catch (_) {}
    try { entry.node.disconnect(); } catch (_) {}
  }

  // Set up finalize promise BEFORE posting finalize.
  const finalizePromise = new Promise((resolve, reject) => {
    state._finalizeResolve = resolve;
    setTimeout(() => reject(new Error('finalize timeout')), 30000);
  });
  // Build channel metadata in the worker's expected shape.
  const channelMeta = [];
  for (const [id, entry] of state.nodes) {
    channelMeta.push({
      id,
      name: id,
      filename: id + '.wav',
    });
  }
  state.worker.postMessage({
    type: 'finalize',
    channels: channelMeta,
    bpm: meta.bpm || 0,
    timestamp: meta.timestamp || '',
    duration: meta.duration || 0,
  });

  let result;
  try {
    result = await finalizePromise;
  } catch (err) {
    _recordingState = null;
    try { state.worker.terminate(); } catch (_) {}
    return { error: String(err) };
  }

  // Build a download URL the Go side can pass into an anchor click.
  const blobURL = URL.createObjectURL(result.blob);
  const filename = (meta.filenamePrefix || 'beatmo-recording-') +
    (meta.timestamp || new Date().toISOString().replace(/[:.]/g, '-')) + '.zip';

  // Tear down worker; one-shot per session.
  try { state.worker.terminate(); } catch (_) {}
  _recordingState = null;

  return {
    blobURL,
    filename,
    size: result.blob.size,
    sampleRate: state.sampleRate,
    autoStopped: !!result.autoStopped,
    stats: result.sessionStats || state.lastStats,
    channels: result.meta || channelMeta,
  };
};

// recordingStatsSnapshot() — non-blocking poll for perfStats() integration.
// Returns the most recent stats the worker published. The worker only
// publishes on demand (via {type:'stats'} request) but we periodically
// request and cache; perfStats() reads the cached value cheaply.
window.recordingStatsSnapshot = () => {
  if (!_recordingState) {
    return { active: false, droppedSamples: 0, bytesUsed: 0, queuedBatches: 0,
             maxQueueDepth: 0, elapsedSec: 0, activeChannels: 0,
             autoStopped: false, autoStopReason: '' };
  }
  const s = _recordingState.lastStats || {};
  return {
    active: true,
    droppedSamples: s.droppedSamples || 0,
    bytesUsed: s.bytesUsed || 0,
    queuedBatches: s.queuedBatches || 0,
    maxQueueDepth: s.maxQueueDepth || 0,
    elapsedSec: s.elapsedSec || 0,
    activeChannels: s.activeChannels || _recordingState.nodes.size,
    autoStopped: !!_recordingState.autoStopFired,
    autoStopReason: _recordingState.autoStopReason || '',
  };
};

// recordingRequestStats() — send a stats request to the worker. Result
// arrives asynchronously and updates the cache read by recordingStatsSnapshot.
// Called periodically from the Go-side perf snapshot path.
window.recordingRequestStats = () => {
  if (_recordingState && _recordingState.worker) {
    try { _recordingState.worker.postMessage({ type: 'stats' }); } catch (_) {}
  }
};

// recordingTriggerDownload(blobURL, filename) — fires the anchor click
// for the download. Kept separate so Go can call it explicitly within
// the user-activation window of the stop button click.
window.recordingTriggerDownload = (blobURL, filename) => {
  if (!blobURL) return false;
  try {
    const a = document.createElement('a');
    a.href = blobURL;
    a.download = filename || 'beatmo-recording.zip';
    document.body.appendChild(a);
    a.click();
    setTimeout(() => {
      try { URL.revokeObjectURL(blobURL); } catch (_) {}
      try { a.remove(); } catch (_) {}
    }, 1000);
    return true;
  } catch (err) {
    console.error('[RECORDING] download trigger failed:', err);
    return false;
  }
};

// Speaker routing diagnostics — used by mobile_speaker_routing tests.
window.__hasSilentAudioEl = () => !!silentAudioEl;
window.__silentAudioElState = () => {
  if (!silentAudioEl) return null;
  return { paused: silentAudioEl.paused, readyState: silentAudioEl.readyState,
           volume: silentAudioEl.volume, loop: silentAudioEl.loop, src: !!silentAudioEl.src };
};
window.__audioSessionType = () => {
  try { return navigator?.audioSession?.type ?? null; } catch (_) { return null; }
};
window.__getMainLimiter = () => mainLimiter;
