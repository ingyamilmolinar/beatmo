import {
  SYNTH_PARAM_COUNT,
  SYNTH_PARAM_INDEX,
  SYNTH_PARAM_IDENTITY,
  MODULAR_PARAM_COUNT,
  MODULAR_PARAM_INDEX,
  MODULAR_PARAM_IDENTITY,
  // Kick family migrated to the modular engine (Phase-3): no KICK_PARAM_* block.
  // Tom family migrated to the modular engine (Phase-4): no TOM_PARAM_* block.
  // Snare family migrated to the modular engine (Phase-5): no SNARE_PARAM_* block.
  // Cymbal family migrated to the modular engine (Phase-6): no CYMBAL_PARAM_* block.
  // Bass family migrated to the modular engine (Phase-2): no BASS_PARAM_* block.
  // FM family migrated to the modular engine (Phase-7, LAST): no FM_PARAM_* block.
} from './synth_param_abi.gen.js';

// CHAIN_SPEC is the single source of truth (generated from Go's chain_spec.go)
// for the post-voice master/mix chain — compressor params, soft-clip threshold,
// per-voice headroom, target sample rate. The desktop Go/C mixer and this
// WebAudio chain MUST stay configured identically or the same instrument sounds
// different across platforms (the exact bug this prevents). ensureCompressor +
// ensureLimiter read from it; chain_spec_parity.browser.test.js asserts the
// live nodes match it; the Go drift test keeps the gen file current.
import { CHAIN_SPEC } from './chain_spec.gen.js';

// Family param blocks (native-deprecation migration). An instrument whose
// RENDER_INFO carries paramBlock:'<family>' fills this block instead of the
// legacy 7-field synth_params. Each embeds the synth_params base at indices
// 0..6; family fields default to NaN ("keep the C engine's literal").
//
// EMPTY after Phase-7: every legacy family (bass/kick/tom/snare/cymbal/FM) has
// migrated to paramBlock:'modular' (render_modular_p). No family block remains;
// the modular block (validateModularABI) covers every migrated instrument.
const FAMILY_PARAM_BLOCKS = {};

// ABI consistency guard. synth_param_abi.gen.js is generated from the Go schema
// (which is drift-tested against the C modular_params struct). If it ships stale
// — e.g. a `make wasm` that skipped the gen-synth-abi step before this was wired
// into the build — the modular param block is undersized and the C voice reads
// uninitialised fields, silencing the generator the moment an instrument is
// re-voiced off Native. That presented as "audio is gone when I change the
// oscillator". Fail LOUD here so a stale ABI is a visible console error, never a
// silent dead generator. The browser guard test (synth_abi_consistency...) and
// the Go drift test (synth_param_schema_test.go) keep it green in CI.
const MODULAR_REQUIRED_FIELDS = [
  'osc_type', 'osc_enabled', 'fm_enabled', 'env_enabled',
  'filter_enabled', 'drive_enabled', 'noise_seed',
];
// Lower bound on the modular param block size (floats). The C modular_params
// struct currently has 33 fields; 64 is a generous floor so renderToCache never
// hands render_modular_p an out-of-bounds buffer even if the generated ABI lags
// the struct. NOT the source of truth — just a safety net (see validateModularABI).
const MODULAR_ALLOC_FLOOR = 64;
function validateModularABI() {
  const missing = MODULAR_REQUIRED_FIELDS.filter(
    (f) => !Number.isFinite(MODULAR_PARAM_INDEX[f]),
  );
  let maxIdx = -1;
  for (const k in MODULAR_PARAM_INDEX) {
    if (Number.isFinite(MODULAR_PARAM_INDEX[k]) && MODULAR_PARAM_INDEX[k] > maxIdx) {
      maxIdx = MODULAR_PARAM_INDEX[k];
    }
  }
  const undersized = MODULAR_PARAM_COUNT <= maxIdx;
  if (missing.length || undersized) {
    const msg =
      '[AUDIOJS] STALE synth_param_abi.gen.js — the modular voice ABI is out of ' +
      'sync with the C struct. Re-voiced instruments will be SILENT. ' +
      'Run `make gen-synth-abi` (or rebuild via `make wasm`). ' +
      `missing=[${missing.join(',')}] count=${MODULAR_PARAM_COUNT} maxIndex=${maxIdx}`;
    try { console.error(msg); } catch (_) {}
    if (typeof window !== 'undefined') window.__synthABIStale = true;
    return false;
  }
  if (typeof window !== 'undefined') window.__synthABIStale = false;
  return true;
}
const MODULAR_ABI_OK = validateModularABI();
// Family block guards — same stale-ABI failure mode as the modular block:
// an undersized/missing family block would feed the render_*_p engines
// garbage overrides. Family identities are NaN ("keep the engine literal"),
// so a correctly-sized block is always safe; these guards catch the sizing.
const FAMILY_REQUIRED_FIELDS = {
  // EMPTY after Phase-7 — every legacy family migrated to the modular block
  // (validateModularABI covers them all): fm/cymbal/bass/kick/tom/snare.
};
function validateFamilyABI(family) {
  const blk = FAMILY_PARAM_BLOCKS[family];
  const required = FAMILY_REQUIRED_FIELDS[family] || [];
  const missing = required.filter((f) => !Number.isFinite(blk.index[f]));
  let maxIdx = -1;
  for (const k in blk.index) {
    if (Number.isFinite(blk.index[k]) && blk.index[k] > maxIdx) {
      maxIdx = blk.index[k];
    }
  }
  const undersized = blk.count <= maxIdx;
  if (missing.length || undersized) {
    const msg =
      `[AUDIOJS] STALE synth_param_abi.gen.js — the ${family} family ABI is out ` +
      'of sync with its C struct. Edited instruments may render garbage. ' +
      'Run `make gen-synth-abi` (or rebuild via `make wasm`). ' +
      `missing=[${missing.join(',')}] count=${blk.count} maxIndex=${maxIdx}`;
    try { console.error(msg); } catch (_) {}
    if (typeof window !== 'undefined') window.__synthABIStale = true;
    return false;
  }
  return true;
}
const FAMILY_ABI_OK = Object.keys(FAMILY_PARAM_BLOCKS).every((f) => validateFamilyABI(f));
if (typeof window !== 'undefined') {
  // Test-friendly summary so the browser ABI guard test can assert the loaded
  // gen.js is consistent with the C struct (catches a stale committed file).
  window.__synthABI = {
    modularCount: MODULAR_PARAM_COUNT,
    synthCount: SYNTH_PARAM_COUNT,
    stale: !MODULAR_ABI_OK || !FAMILY_ABI_OK,
    hasModularFields: MODULAR_REQUIRED_FIELDS.every((f) => Number.isFinite(MODULAR_PARAM_INDEX[f])),
  };
  // Unmissable load banner with a build marker. If you do NOT see this line in
  // the console after a reload, your browser is running a CACHED old audio.js
  // (the re-voice fallback + [SYNTH-*] logging are NOT active) — hard-reload
  // (Ctrl/Cmd+Shift+R) or use the no-cache `make serve`. Bump the marker when
  // editing audio.js so a stale copy is obvious.
  window.__audioJsBuild = 'SYNTHDBG-6';
  try {
    console.log('%c[AUDIOJS] loaded build ' + window.__audioJsBuild +
      ' — synth edits restore a factory render shadowed by sample PCM (frozen-instrument fix)', 'color:#0a0;font-weight:bold');
  } catch (_) {}
}

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

// Limiter (WaveShaperNode) matching the desktop master output stage in
// engine_stop.go: a tanh SOFT-CLIP above CHAIN_SPEC.softClipThreshold, then a
// hard clamp to [-1, 1]. Previously this was a pure hard clamp with no
// soft-clip, so loud peaks sounded harsher in the browser than on desktop —
// part of the audible desktop↔browser divergence. Created lazily.
let mainLimiter = null;

// softClipSample mirrors engine_stop.go's final conversion: gently saturate
// peaks above the threshold with tanh, then safety-clamp to [-1, 1]. Shared by
// the limiter curve and getChainConfigForTest so the test asserts the real fn.
function softClipSample(x) {
  const t = CHAIN_SPEC.softClipThreshold;
  let y = x;
  if (y > t) y = t * Math.tanh(y / t);
  else if (y < -t) y = -t * Math.tanh(y / -t);
  return Math.max(-1, Math.min(1, y));
}

function ensureLimiter() {
  if (mainLimiter) return mainLimiter;
  if (!hasCtx()) return null;
  const c = ctx;
  mainLimiter = c.createWaveShaper();
  const curveLen = 8192;
  const curve = new Float32Array(curveLen);
  for (let i = 0; i < curveLen; i++) {
    const x = (i / (curveLen - 1)) * 2 - 1;
    curve[i] = softClipSample(x);
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
  if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch && window.__synthEvtTrace) {
    console.log('[SYNTH-EVT]', { id, cached: !!render, hasRENDER: !!RENDER[id], vol, pitch, dur, rate, when, now, params: instrumentParamsFor(id) });
  }
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
    // Anti-pop: per-source fade GainNode with 5ms fade-in ramp. The ramp target
    // is CHAIN_SPEC.voiceHeadroom (NOT 1.0): native applies this per-voice
    // attenuation before summing voices (engine_stop.go renderVoiceIntoInstBuf),
    // so WebAudio must too — otherwise N voices sum hot and slam the master
    // compressor/limiter into audible clipping (browser-only mix distortion).
    const fadeGain = ctx.createGain();
    fadeGain.gain.setValueAtTime(0, when);
    fadeGain.gain.linearRampToValueAtTime(CHAIN_SPEC.voiceHeadroom, when + 0.005);
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
    // Anti-pop: per-source fade GainNode with 5ms fade-in ramp. The ramp target
    // is CHAIN_SPEC.voiceHeadroom (NOT 1.0): native applies this per-voice
    // attenuation before summing voices (engine_stop.go renderVoiceIntoInstBuf),
    // so WebAudio must too — otherwise N voices sum hot and slam the master
    // compressor/limiter into audible clipping (browser-only mix distortion).
    const fadeGain = ctx.createGain();
    fadeGain.gain.setValueAtTime(0, when);
    fadeGain.gain.linearRampToValueAtTime(CHAIN_SPEC.voiceHeadroom, when + 0.005);
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
  // Snare family migrated to the unified modular engine (Phase-5). Render through
  // render_modular / render_modular_p; the binding-translated modular default
  // block is seeded into JS at bootstrap (seedInstrumentDefaults) and per-edit
  // pushes carry modular-named params (synth_recipe_wasm.go seam).
  snare: 'render_modular',
  // Kick family migrated to the unified modular engine (Phase-3). Render through
  // render_modular / render_modular_p; the binding-translated modular default
  // block is seeded into JS at bootstrap (seedInstrumentDefaults) and per-edit
  // pushes carry modular-named params (synth_recipe_wasm.go seam).
  kick: 'render_modular',
  // Cymbal family migrated to the unified modular engine (Phase-6). Render through
  // render_modular / render_modular_p; the binding-translated modular default
  // block is seeded into JS at bootstrap (seedInstrumentDefaults) and per-edit
  // pushes carry modular-named params (synth_recipe_wasm.go seam).
  hihat: 'render_modular',
  // Tom family migrated to the unified modular engine (Phase-4). Render through
  // render_modular / render_modular_p; the binding-translated modular default
  // block is seeded into JS at bootstrap (seedInstrumentDefaults) and per-edit
  // pushes carry modular-named params (synth_recipe_wasm.go seam).
  tom: 'render_modular',
  clap: 'render_modular',
  cowbell: 'render_modular',
  // Bass instruments — migrated to the unified modular engine (Phase-2).
  // Render through render_modular / render_modular_p; the binding-translated
  // modular default block is seeded into JS at bootstrap (seedInstrumentDefaults)
  // and per-edit pushes carry modular-named params (synth_recipe_wasm.go seam).
  'bass-guitar': 'render_modular',
  'sub-bass': 'render_modular',
  // Variant set 1: slightly brighter/tighter flavours.
  'snare-1': 'render_modular',
  'kick-1': 'render_modular',
  'hihat-1': 'render_modular',
  'tom-1': 'render_modular',
  'clap-1': 'render_modular',
  'cowbell-1': 'render_modular',
  'bass-guitar-1': 'render_modular',
  'sub-bass-1': 'render_modular',
  // Variant set 2: more obviously digital/lofi flavours.
  'snare-2': 'render_modular',
  'kick-2': 'render_modular',
  'hihat-2': 'render_modular',
  'tom-2': 'render_modular',
  'clap-2': 'render_modular',
  'cowbell-2': 'render_modular',
  // New distinct instruments.
  'rimshot': 'render_modular',
  'sidestick': 'render_modular',
  'kick-deep': 'render_modular',
  'shaker': 'render_modular',
  'ride': 'render_modular',
  'crash': 'render_modular',
  // Variant set 3: expressive dynamics.
  'snare-ghost': 'render_modular',
  'kick-tight': 'render_modular',
  'hihat-pedal': 'render_modular',
  'clap-tight': 'render_modular',
  // FM synthesis instruments — migrated to the modular engine (Phase-7, the LAST
  // legacy family). Render through render_modular / render_modular_p; the
  // binding-translated modular default (fmRecipeToModular → source==10 FM voice)
  // is seeded into JS at bootstrap via seedInstrumentDefaults.
  'fm-bass':     'render_modular',
  'fm-bell':     'render_modular',
  'fm-lead':     'render_modular',
  'fm-epiano':   'render_modular',
  'fm-pluck':    'render_modular',
  'fm-bass-1':   'render_modular',
  'fm-bell-1':   'render_modular',
  'fm-lead-1':   'render_modular',
  'fm-epiano-1': 'render_modular',
  'fm-pluck-1':  'render_modular',
  // Unified modular synth voice (fully user-editable pipeline).
  'modular': 'render_modular',
  // Second shipped modular preset — same C renderer, pad defaults seeded
  // into JS at bootstrap via seedInstrumentDefaults.
  'modular-pad': 'render_modular',
};

const RENDER_INFO = {
  snare:   { seconds: 1.0,  amp: 0.8, paramBlock: 'modular' },
  kick:    { seconds: 0.5,  amp: 0.8, paramBlock: 'modular' },
  hihat:   { seconds: 0.25, amp: 0.8 , paramBlock: 'modular' },
  tom:     { seconds: 0.5,  amp: 0.8, paramBlock: 'modular' },
  clap:    { seconds: 0.5,  amp: 0.8, paramBlock: 'modular' },
  cowbell: { seconds: 0.4,  amp: 0.8 , paramBlock: 'modular' },
  'bass-guitar': { seconds: 1.5, amp: 0.8 , paramBlock: 'modular' },
  'sub-bass':    { seconds: 2.0, amp: 0.8 , paramBlock: 'modular' },
  'snare-1':        { seconds: 0.9,  amp: 0.8, paramBlock: 'modular' },
  'kick-1':         { seconds: 0.6,  amp: 0.8, paramBlock: 'modular' },
  'hihat-1':        { seconds: 0.7,  amp: 0.8 , paramBlock: 'modular' },
  'tom-1':          { seconds: 0.6,  amp: 0.8, paramBlock: 'modular' },
  'clap-1':         { seconds: 0.45, amp: 0.8, paramBlock: 'modular' },
  'cowbell-1':      { seconds: 0.5,  amp: 0.8 , paramBlock: 'modular' },
  'bass-guitar-1':  { seconds: 1.2,  amp: 0.8 , paramBlock: 'modular' },
  'sub-bass-1':     { seconds: 1.5,  amp: 0.8 , paramBlock: 'modular' },
  'snare-2':        { seconds: 0.7,  amp: 0.8, paramBlock: 'modular' },
  'kick-2':         { seconds: 0.5,  amp: 0.8, paramBlock: 'modular' },
  'hihat-2':        { seconds: 0.22, amp: 0.8 , paramBlock: 'modular' },
  'tom-2':          { seconds: 0.45, amp: 0.8, paramBlock: 'modular' },
  'clap-2':         { seconds: 0.4,  amp: 0.8, paramBlock: 'modular' },
  'cowbell-2':      { seconds: 0.4,  amp: 0.8 , paramBlock: 'modular' },
  'rimshot':        { seconds: 0.3,  amp: 0.8, paramBlock: 'modular' },
  'sidestick':      { seconds: 0.25, amp: 0.8, paramBlock: 'modular' },
  'kick-deep':      { seconds: 0.8,  amp: 0.8, paramBlock: 'modular' },
  'shaker':         { seconds: 0.3,  amp: 0.8 , paramBlock: 'modular' },
  'ride':           { seconds: 1.0,  amp: 0.8 , paramBlock: 'modular' },
  'crash':          { seconds: 1.5,  amp: 0.8 , paramBlock: 'modular' },
  'snare-ghost':    { seconds: 0.5,  amp: 0.8, paramBlock: 'modular' },
  'kick-tight':     { seconds: 0.35, amp: 0.8, paramBlock: 'modular' },
  'hihat-pedal':    { seconds: 0.15, amp: 0.8 , paramBlock: 'modular' },
  'clap-tight':     { seconds: 0.3,  amp: 0.8, paramBlock: 'modular' },
  // FM synthesis instruments — migrated to the modular engine (Phase-7, the LAST
  // legacy family). FM knobs travel in the wide modular_params block (gen_fm_*
  // gen-slot columns, source==10) via the Go-side binding; the JS side fills the
  // modular block and the binding-translated default is seeded at bootstrap.
  'fm-bass':        { seconds: 1.5,  amp: 0.8, paramBlock: 'modular' },
  'fm-bell':        { seconds: 2.0,  amp: 0.8, paramBlock: 'modular' },
  'fm-lead':        { seconds: 1.0,  amp: 0.8, paramBlock: 'modular' },
  'fm-epiano':      { seconds: 2.0,  amp: 0.8, paramBlock: 'modular' },
  'fm-pluck':       { seconds: 0.5,  amp: 0.8, paramBlock: 'modular' },
  'fm-bass-1':      { seconds: 1.0,  amp: 0.8, paramBlock: 'modular' },
  'fm-bell-1':      { seconds: 1.5,  amp: 0.8, paramBlock: 'modular' },
  'fm-lead-1':      { seconds: 0.7,  amp: 0.8, paramBlock: 'modular' },
  'fm-epiano-1':    { seconds: 1.5,  amp: 0.8, paramBlock: 'modular' },
  'fm-pluck-1':     { seconds: 0.3,  amp: 0.8, paramBlock: 'modular' },
  // Modular voice instrument uses the wide modular_params block (paramBlock).
  'modular':        { seconds: 1.0,  amp: 0.8, paramBlock: 'modular' },
  'modular-pad':    { seconds: 1.0,  amp: 0.8, paramBlock: 'modular' },
};

// renderCache holds the pre-rendered Float32Array + metadata for every
// synthesized instrument. Exported so browser tests can probe re-render
// after a setInstrumentParam mutation without re-implementing the
// module's render bookkeeping.
export const renderCache = new Map();
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
  // Configuration from CHAIN_SPEC (Go chain_spec.go) so this matches the
  // desktop master compressor. attack/release are SECONDS here, so the spec's
  // millisecond values are ÷1000. (The DynamicsCompressorNode algorithm still
  // differs from the Go peak-detect compressor — matching params is as close as
  // the two algorithms allow; full-mix parity is correlation-graded.)
  mainCompressor.threshold.value = CHAIN_SPEC.compressorThresholdDb;
  mainCompressor.ratio.value = CHAIN_SPEC.compressorRatio;
  mainCompressor.attack.value = CHAIN_SPEC.compressorAttackMs / 1000;
  mainCompressor.release.value = CHAIN_SPEC.compressorReleaseMs / 1000;
  mainCompressor.knee.value = CHAIN_SPEC.compressorKneeDb;
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

// Shared analyser tapped on the wet output of both send buses (delay tail +
// reverb tail). Created lazily by enableSendBusAnalyzer; existing buses are
// re-tapped at that point. Feeds the Chain panel's StageSends comparison.
let sendBusAnalyser = null;
let sendBusTimeBuf = null;
let sendBusFreqBuf = null;

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
  //                     mainChannelNode + sendBusAnalyser (fan-out)
  inputGain.connect(delay);
  delay.connect(lpFilter);
  lpFilter.connect(fbGain);
  fbGain.connect(delay); // feedback
  delay.connect(ensureMainNode()); // wet output
  if (sendBusAnalyser) {
    delay.connect(sendBusAnalyser); // fan-out tap for StageSends
  }

  delaySendBus = inputGain;
  return delaySendBus;
}

function ensureReverbSendBus() {
  if (reverbSendBus) return reverbSendBus;
  if (!hasCtx()) return null;
  const c = ctx;

  const inputGain = c.createGain();
  inputGain.gain.value = 1;

  // Algorithmic Schroeder reverb (native nodes), same as the insert reverb —
  // replaces a ConvolverNode (CPU hog) and matches desktop's Schroeder reverb.
  // Medium room/damping for a ~1.5s shared send tail.
  const { input: rvInput, wet: rvWet } = buildSchroederReverbCore(c, 0.6, 0.4);

  const wetGain = c.createGain();
  wetGain.gain.value = 0.3;

  inputGain.connect(rvInput);
  rvWet.connect(wetGain);
  wetGain.connect(ensureMainNode());
  if (sendBusAnalyser) {
    wetGain.connect(sendBusAnalyser); // fan-out tap for StageSends
  }

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

// Read-only observable for the sanity probe (sanity_audio_probe.js).
if (typeof window !== 'undefined') {
  try { Object.defineProperty(window, '__beatmoWorkletReady', { get: () => workletReady }); } catch (_) {}
}

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

// buildSchroederReverbCore builds a Freeverb/Schroeder reverb out of native
// WebAudio nodes — 4 damped-feedback comb filters + 2 allpass stages — matching
// the desktop C reverb (src/c/insert_fx.c ifx_reverb: COMB_DELAYS, AP_DELAYS,
// fb = 0.7 + 0.28*room, allpass g = 0.5, one-pole damping). It replaces the
// previous ConvolverNode reverb, which was BOTH the dominant audio-thread CPU
// cost (an OfflineAudioContext benchmark measured a 2.5s-IR convolver at ~36×
// a gain node and ~3× the next-most-expensive effect — the cause of choppy
// playback under heavy FX on real hardware) AND a sound mismatch vs desktop's
// Schroeder reverb. The algorithmic version is ~6.7× cheaper (15% of the
// convolver cost) and closer to desktop. Returns { input, wet } where `wet` is
// the 100%-wet reverb output (callers add their own dry/mix).
//
// Native-node caveat: WebAudio adds a 1-render-quantum (128-sample) latency to
// every feedback loop, so the effective comb/allpass times are ~2.7ms longer
// than the C engine's; the reverb character is preserved, the tail is not
// sample-identical. (Exact parity would require reviving the insert-FX worklet
// running ifx_reverb; that reintroduces the per-channel WASM cost this fix
// removes, so it is intentionally not used for the default reverb.)
function buildSchroederReverbCore(c, room, damping) {
  const sr = c.sampleRate;
  const COMB = [1116, 1188, 1277, 1356]; // samples @ 44100 (C: COMB_DELAYS)
  const AP = [556, 441];                 // samples @ 44100 (C: AP_DELAYS)
  const fb = 0.7 + 0.28 * Math.max(0, Math.min(1, room)); // C: fb formula
  // C damping is a one-pole coef == damping; map to a biquad lowpass cutoff
  // fc where damping = exp(-2π fc / sr)  ⇒  fc = -ln(damping)·sr/2π.
  const dClamp = Math.max(1e-4, Math.min(0.9999, damping));
  let fc = (-Math.log(dClamp) * sr) / (2 * Math.PI);
  fc = Math.max(200, Math.min(sr * 0.45, fc));

  const input = c.createGain();
  const combSum = c.createGain();
  combSum.gain.value = 1 / COMB.length;
  for (const d of COMB) {
    const delay = c.createDelay(0.1);
    delay.delayTime.value = d / 44100;
    const fbg = c.createGain();
    fbg.gain.value = fb;
    // input → delay → out(combSum); delay → fbg → delay (feedback)
    input.connect(delay);
    delay.connect(combSum);
    delay.connect(fbg);
    fbg.connect(delay);
  }
  // Damping: one lowpass on the comb sum. The C engine damps inside each comb's
  // one-pole feedback (per-comb); native WebAudio has no cheap one-pole, so both
  // a per-comb biquad and this single comb-sum biquad are approximations — the
  // comb-sum placement is ~29% cheaper (OfflineAudioContext-measured) and brings
  // the reverb in line with the compressor instead of being the dominant cost.
  const damp = c.createBiquadFilter();
  damp.type = "lowpass";
  damp.frequency.value = fc;
  combSum.connect(damp);
  // Two Schroeder allpass stages in series: y = d[n-M] - g·x ; d[n] = x + g·d[n-M].
  let node = damp;
  const g = 0.5;
  for (const d of AP) {
    const delay = c.createDelay(0.05);
    delay.delayTime.value = d / 44100;
    const ff = c.createGain();
    ff.gain.value = -g; // feedforward -g·x
    const fbg = c.createGain();
    fbg.gain.value = g; // feedback g·d
    const out = c.createGain();
    node.connect(delay);
    node.connect(ff);
    delay.connect(out);
    ff.connect(out);
    delay.connect(fbg);
    fbg.connect(delay);
    node = out;
  }
  return { input, wet: node };
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
      // Algorithmic Schroeder reverb (native nodes) — ~6.7× cheaper than the
      // old per-channel ConvolverNode, which OfflineAudioContext benchmarking
      // pinned as the dominant audio-thread CPU cost behind choppy heavy-FX
      // playback (and matches desktop's Schroeder reverb). See
      // buildSchroederReverbCore + bench-results/choppy_audio_fx_chain_findings.
      const { input: rvInput, wet: rvWet } = buildSchroederReverbCore(c, room, damping);
      const input = c.createGain(); input.gain.value = 1;
      const dry = c.createGain(); dry.gain.value = 1 - mix;
      const wet = c.createGain(); wet.gain.value = mix;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(dry); dry.connect(output);
      input.connect(rvInput); rvWet.connect(wet); wet.connect(output);
      return { input, output, nodes: [dry, wet] };
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
    case 'pitchshift': {
      // No good native WebAudio equivalent; passthrough placeholder.
      const input = c.createGain(); input.gain.value = 1;
      return { input, output: input, nodes: [] };
    }
    case 'transient': {
      // Mirrors fx_transient.go / ifx_transient_process: dual-envelope shaper.
      // attack/sustain are 0-200 (% mapped to 0-2 gain); speed is 1-50ms.
      const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v));
      const attackGain = clamp(p.attack ?? 100, 0, 200) / 100;
      const sustainGain = clamp(p.sustain ?? 100, 0, 200) / 100;
      const speedMs = clamp(p.speed ?? 10, 1, 50);
      const sr = c.sampleRate || 44100;
      const fastCoef = Math.exp(-1 / (speedMs * 0.001 * sr));
      const fastRelCoef = Math.exp(-1 / (speedMs * 0.003 * sr));
      const slowCoef = Math.exp(-1 / (0.1 * sr));
      let fastEnv = 0, slowEnv = 0;
      const sp = c.createScriptProcessor(256, 1, 1);
      sp.onaudioprocess = (e) => {
        const inBuf = e.inputBuffer.getChannelData(0);
        const outBuf = e.outputBuffer.getChannelData(0);
        for (let i = 0; i < inBuf.length; i++) {
          const x = inBuf[i];
          const ax = Math.abs(x);
          if (ax > fastEnv) fastEnv = fastCoef * fastEnv + (1 - fastCoef) * ax;
          else fastEnv = fastRelCoef * fastEnv;
          slowEnv = slowCoef * slowEnv + (1 - slowCoef) * ax;
          const denom = slowEnv > 1e-10 ? slowEnv : 1e-10;
          let trans = (fastEnv - slowEnv) / denom;
          if (trans < 0) trans = 0;
          if (trans > 1) trans = 1;
          outBuf[i] = x * (attackGain * trans + sustainGain * (1 - trans));
        }
      };
      const input = c.createGain(); input.gain.value = 1;
      const output = c.createGain(); output.gain.value = 1;
      input.connect(sp); sp.connect(output);
      return { input, output, nodes: [sp] };
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
  if (!hasCtx()) {
    // Queue the operation for when the AudioContext is created. Without this,
    // insert effects pushed from Go during early startup (e.g. demo Import()
    // before the user gesture that unlocks audio on mobile) would be silently
    // dropped — the audible effect would only kick in after the user toggled
    // it off/on, which would fire updateInsertEffects() again with a live ctx.
    pendingChannelOps.push(() => updateInsertEffects(id, slotsJSON));
    return;
  }
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

// Phase 5: per-instrument SynthRecipe params (8 generic knobs). Storage
// is a JS-side mirror of the Go audio.instrumentParamsMgr so the WASM
// render path can pull params at trigger time without crossing the
// Go↔JS boundary every play.
const instrumentSynthParams = new Map();

// instrumentDefaultParams holds the shipped recipe-default param block for any
// preset whose defaults diverge from the C render built-ins (e.g. modular-pad).
// Seeded once at bootstrap by Go via seedInstrumentDefaults. renderToCache
// falls back to this map when there is no user overlay, so a divergent preset
// renders its intended sound before any edit — and a Reset (which clears the
// overlay) returns to the preset defaults, not the base render defaults.
const instrumentDefaultParams = new Map();

// seedInstrumentDefaults is invoked by Go-WASM (platformInstrumentDefaultsPush
// in synth_recipe_wasm.go) once at startup for each seeded preset. Distinct
// from updateInstrumentParams: this is the base layer, not a user edit.
window.seedInstrumentDefaults = (id, paramsJSON) => {
  try {
    const params = JSON.parse(paramsJSON);
    if (params && typeof params === 'object' && Object.keys(params).length > 0) {
      instrumentDefaultParams.set(id, params);
    } else {
      instrumentDefaultParams.delete(id);
    }
    // A late seed (after first render) must re-render with the preset.
    renderCache.delete(id);
    delete samples[id];
    rawRenderCache.delete(id);
  } catch (err) { dbg('synth.defaults.error', { id, err: String(err) }); }
};

// updateInstrumentRecipe is invoked by Go-WASM (platformInstrumentRecipeChanged
// in synth_recipe_wasm.go) whenever an instrument's recipe binding changes —
// a MigrateGenType re-voice at import, or a project whose instrument carries a
// recipe differing from the static table. The exemplar is a builtin instrument
// id whose static RENDER/RENDER_DEFAULTS/RENDER_INFO entries already carry the
// recipe's render function + param block; copying them keeps those tables the
// single source of truth (no recipe→renderer map to drift). Without this push
// the browser kept rendering the OLD voice for a rebound instrument while the
// desktop dispatch followed the new binding.
const renderInfoFactory = new Map(); // id -> RENDER_INFO entry before its first rebind
window.updateInstrumentRecipe = (id, recipeID, exemplar) => {
  try {
    const target = exemplar && RENDER_DEFAULTS[exemplar] ? exemplar : null;
    if (!target) {
      // No builtin exemplar (user Save-As clone, or a cleared binding):
      // restore the instrument's own factory entries when it has them so a
      // later re-bind to its native recipe sounds right again.
      if (!recipeID && RENDER_DEFAULTS[id] && RENDER[id] !== RENDER_DEFAULTS[id]) {
        RENDER[id] = RENDER_DEFAULTS[id];
        if (renderInfoFactory.has(id)) RENDER_INFO[id] = renderInfoFactory.get(id);
        renderCache.delete(id);
        delete samples[id];
        rawRenderCache.delete(id);
      }
      return;
    }
    const wantFn = RENDER_DEFAULTS[target];
    const wantBlock = RENDER_INFO[target] && RENDER_INFO[target].paramBlock;
    const haveBlock = RENDER_INFO[id] && RENDER_INFO[id].paramBlock;
    if (RENDER[id] === wantFn && haveBlock === wantBlock) return; // bootstrap factory re-bind: no-op
    if (!renderInfoFactory.has(id) && RENDER_INFO[id]) renderInfoFactory.set(id, RENDER_INFO[id]);
    RENDER[id] = wantFn;
    RENDER_INFO[id] = { ...RENDER_INFO[target] };
    renderCache.delete(id);
    delete samples[id];
    rawRenderCache.delete(id);
    dbg('synth.recipe.rebind', { id, recipeID, exemplar });
  } catch (err) { dbg('synth.recipe.error', { id, recipeID, err: String(err) }); }
};

// updateInstrumentParams is invoked by Go-WASM (platformInstrumentParamsChanged
// in synth_recipe_wasm.go) whenever the user edits a per-instrument knob via
// the Synth tab. Stores the params + invalidates the renderCache so the next
// trigger re-renders via the parameterized C function (`render_X_p`).
window.updateInstrumentParams = (id, paramsJSON) => {
  try { _updateInstrumentParams(id, paramsJSON); }
  catch (err) { dbg('synth.param.error', { id, err: String(err) }); }
};

// Test-only helper: forces a fresh render via ensureRenderedSample and
// returns a tail-energy summary of the resulting Float32Array. Used by
// instrument_params_audible.browser.test.js to verify
// setInstrumentParam reaches the C `_p` variant. Production code never
// reads __testCaptureSynthRender — keep it window-scoped + underscored
// so tree-shaking + linters skip it cleanly.
window.__testCaptureSynthRender = async (id) => {
  renderCache.delete(id);
  delete samples[id];
  const rec = await ensureRenderedSample(id);
  if (!rec || !rec.data) return null;
  const data = rec.data;
  let tailRMS = 0;
  const tailHalf = data.subarray(Math.floor(data.length / 2));
  for (let i = 0; i < tailHalf.length; i++) tailRMS += tailHalf[i] * tailHalf[i];
  tailRMS = Math.sqrt(tailRMS / tailHalf.length);
  // Peak + full-buffer RMS over the WHOLE render — the silence detector for
  // the generator-selector tests (a percussive drum has ~0 tail RMS but a
  // non-zero peak, so tailRMS alone can't tell "re-voiced" from "silent").
  let peak = 0;
  let rms = 0;
  for (let i = 0; i < data.length; i++) {
    const a = Math.abs(data[i]);
    if (a > peak) peak = a;
    rms += data[i] * data[i];
  }
  rms = Math.sqrt(rms / Math.max(1, data.length));
  // Return head + tail slice + tailRMS for the assertions.
  return {
    head: Array.from(data.slice(0, 64)),
    tail: Array.from(data.slice(Math.max(0, data.length - 256), Math.max(0, data.length - 192))),
    tailRMS,
    peak,
    rms,
    length: data.length,
  };
};

// Test-only helper: applies the sample-edit transform to a caller-provided
// buffer and returns the result as a plain array. Used by
// sample_edit_descriptor.browser.test.js to pin applySampleEdit to Go's
// BakeSample semantics on a deterministic input (C renders are noise-seeded,
// so pinning via a re-render is not reproducible). Production never reads it.
window.__testApplySampleEdit = (arr, sr, editJSON) =>
  Array.from(applySampleEdit(Float32Array.from(arr), sr, JSON.parse(editJSON)));

function _updateInstrumentParams(id, paramsJSON) {
  let params = null;
  try { params = JSON.parse(paramsJSON); } catch (_) { params = null; }
  // gen_type-era hygiene: the Generator selector was removed by the
  // native-deprecation migration (Go strips/migrates it at import; this is
  // the defensive belt for any stale caller still pushing the key).
  if (params && typeof params === 'object') delete params.gen_type;
  if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch) {
    console.log('[SYNTH-UPD]', 'updateInstrumentParams', { id, params, RENDER: !!RENDER[id], hadCache: renderCache.has(id) });
  }
  if (params && typeof params === 'object' && Object.keys(params).length > 0) {
    instrumentSynthParams.set(id, params);
  } else {
    instrumentSynthParams.delete(id);
  }
  // Drop cached render so the next play re-runs through `render_X_p` with the
  // new values — but ONLY for a C-synth instrument (RENDER[id] present), whose
  // renderCache entry is re-derivable. For a SAMPLE-based instrument
  // (registerSamplePCM deleted RENDER[id]), the renderCache entry holds the ONLY
  // copy of its PCM: deleting it permanently silences the instrument because
  // processAudioEvent can't re-render it (no RENDER[id], no URL) and falls through
  // to "Unknown sound". A raw sample isn't synth-parameterised, so a knob /
  // knob edit must leave its buffer intact. This is the "changing the generator
  // on a soloed (sample) instrument kills the audio, dead until reload" bug.
  // Guard: synth_sample_param_change_keeps_audio.browser.test.js.
  if (RENDER[id]) {
    renderCache.delete(id);
    delete samples[id];
    rawRenderCache.delete(id);
  } else if (RENDER_DEFAULTS[id] !== undefined) {
    // A synth edit arrived for a FACTORY instrument whose render mapping was
    // deleted by a sample registration (in-session WAV-save over a builtin,
    // or any stale PCM override that slipped past the startup skip in Go's
    // ApplySavedSamples). Desktop precedence: the recipe path wins as soon as
    // params exist — mirror it. Restore the factory render and drop the
    // frozen PCM so the next trigger re-renders the synth with the new
    // params. Without this the instrument is FROZEN: the edit lands in
    // instrumentSynthParams but the cached PCM keeps playing unchanged — the
    // "knobs / stage toggles / Save do nothing on WASM (desktop fine)" bug.
    // Pure user samples (no factory render) keep the silence protection
    // above. Regression: synth_edits_override_stale_sample.browser.test.js.
    RENDER[id] = RENDER_DEFAULTS[id];
    renderCache.delete(id);
    delete samples[id];
    rawRenderCache.delete(id);
  }
}

// instrumentParamsFor returns the JS-side mirror of the Go manager's
// per-instrument params: the user overlay MERGED OVER the bootstrap-seeded
// preset defaults (modular-pad), else null when neither exists. The merge
// mirrors the desktop dispatch (MergeRecipeDefaults: registered defaults
// under the overlay) — an OR here dropped the entire seed the moment ONE
// knob was edited, re-voicing the pad as the base modular voice for every
// elided key. Per-key overlay still wins; clearing the overlay (Reset)
// falls back to the full preset.
function instrumentParamsFor(id) {
  const overlay = instrumentSynthParams.get(id);
  const defaults = instrumentDefaultParams.get(id);
  if (!overlay && !defaults) return null;
  if (!defaults) return overlay;
  if (!overlay) return defaults;
  return { ...defaults, ...overlay };
}

// ───────── Non-destructive sample-edit descriptors ─────────
// JS-side mirror of Go's audio.sampleEdits (sample_edit_descriptor.go). A
// descriptor keeps a synth instrument a SYNTH: the Sampler-tab edit
// (trim/pitch/gain/reverse/normalize/fade) is applied to the freshly-rendered
// C-synth buffer inside ensureRenderedSample instead of baking PCM and
// killing the render mapping. Field names match Go's SampleEdit JSON
// (StartFrac, EndFrac, TransposeSemis, DetuneCents, GainDB, FadeInMs,
// FadeOutMs, Reverse, Normalize).
const instrumentSampleEdits = new Map();

// rawRenderCache holds UN-edited renders for the Sampler editor's raw capture
// (captureInstrumentPCM(id, true)): the editor shows the source waveform and
// overlays the saved edit itself, so it must not see the edited render that
// renderCache holds. Entries drop alongside renderCache on any param change.
const rawRenderCache = new Map();

// sampleEditIsIdentity mirrors Go's SampleEdit.isIdentity: full range kept,
// every transform neutral — Go pushes the identity edit on Clear so the entry
// is deleted here.
function sampleEditIsIdentity(e) {
  return !e ||
    ((e.StartFrac || 0) === 0 && (e.EndFrac === undefined ? 1 : e.EndFrac) === 1 &&
     (e.TransposeSemis || 0) === 0 && (e.DetuneCents || 0) === 0 &&
     (e.GainDB || 0) === 0 && (e.FadeInMs || 0) === 0 && (e.FadeOutMs || 0) === 0 &&
     !e.Reverse && !e.Normalize);
}

// updateSampleEdit is invoked by Go-WASM (platformSampleEditChanged in
// synth_recipe_wasm.go) whenever a sample-edit descriptor is set or cleared.
window.updateSampleEdit = (id, editJSON) => {
  try { _updateSampleEdit(id, editJSON); }
  catch (err) { dbg('sample.edit.error', { id, err: String(err) }); }
};

function _updateSampleEdit(id, editJSON) {
  let e = null;
  try { e = JSON.parse(editJSON); } catch (_) { e = null; }
  if (e && typeof e === 'object' && !sampleEditIsIdentity(e)) {
    instrumentSampleEdits.set(id, e);
  } else {
    instrumentSampleEdits.delete(id);
  }
  // Same RENDER[id] guard as _updateInstrumentParams: a sample-based
  // instrument's renderCache entry holds the ONLY copy of its PCM — deleting
  // it would permanently silence the instrument. The descriptor only applies
  // to re-derivable C-synth renders anyway.
  if (RENDER[id]) {
    renderCache.delete(id);
    delete samples[id];
  }
}

// applySampleEdit is the JS port of Go's BakeSample (sample_edit.go) — the
// SAME fixed transform order: trim → reverse → resample(pitch) → normalize →
// gain → fades. Math.fround mirrors Go's float32 arithmetic at every
// intermediate so WASM output matches the native render within parity
// tolerance (xplat_audio_compare.browser.test.js).
function applySampleEdit(src, sr, e) {
  const clamp01 = (v) => (v < 0 ? 0 : (v > 1 ? 1 : v));
  // TrimSample
  let out;
  {
    const n = src.length;
    const s = Math.floor(clamp01(e.StartFrac || 0) * n);
    const eIdx = Math.min(Math.floor(clamp01(e.EndFrac === undefined ? 1 : e.EndFrac) * n), n);
    out = (n === 0 || s >= eIdx) ? new Float32Array(0) : src.slice(s, eIdx);
  }
  // ReverseSample
  if (e.Reverse) out.reverse();
  // ResampleSemitones (linear interp, mirrors Go resampleByStep)
  const semis = e.TransposeSemis || 0;
  const cents = e.DetuneCents || 0;
  if (semis !== 0 || cents !== 0) {
    const n = out.length;
    if (n > 0) {
      let step = Math.pow(2, (semis + cents / 100.0) / 12.0);
      if (step <= 0) step = 1;
      const outN = Math.max(Math.floor((n - 1) / step) + 1, 1);
      const res = new Float32Array(outN);
      for (let i = 0; i < outN; i++) {
        const pos = i * step;
        const i0 = Math.floor(pos);
        if (i0 >= n - 1) { res[i] = out[n - 1]; continue; }
        const frac = Math.fround(pos - i0);
        res[i] = Math.fround(out[i0] + Math.fround(Math.fround(out[i0 + 1] - out[i0]) * frac));
      }
      out = res;
    } else {
      out = new Float32Array(0);
    }
  }
  // NormalizePeak (float32 reciprocal, like Go's `g := 1.0 / peak`)
  if (e.Normalize) {
    let peak = 0;
    for (let i = 0; i < out.length; i++) {
      const a = Math.abs(out[i]);
      if (a > peak) peak = a;
    }
    if (peak !== 0) {
      const g = Math.fround(1.0 / peak);
      for (let i = 0; i < out.length; i++) out[i] = Math.fround(out[i] * g);
    }
  }
  // ApplyGainDB
  const db = e.GainDB || 0;
  if (db !== 0) {
    const lin = Math.fround(Math.pow(10, db / 20.0));
    for (let i = 0; i < out.length; i++) out[i] = Math.fround(out[i] * lin);
  }
  // ApplyFades (linear, float32 ratio like Go's float32(i)/float32(fi))
  const fadeIn = e.FadeInMs || 0;
  const fadeOut = e.FadeOutMs || 0;
  if (fadeIn > 0 || fadeOut > 0) {
    const n = out.length;
    const fi = Math.min(Math.floor(sr * fadeIn / 1000.0), n);
    for (let i = 0; i < fi; i++) out[i] = Math.fround(out[i] * Math.fround(i / fi));
    const fo = Math.min(Math.floor(sr * fadeOut / 1000.0), n);
    for (let k = 0; k < fo; k++) out[n - 1 - k] = Math.fround(out[n - 1 - k] * Math.fround(k / fo));
  }
  return out;
}

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
  if (chain.synthAnalyser) { try { chain.synthAnalyser.disconnect(); } catch (_) {} }
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
    // Synth analyser (BEFORE inserts) — feeds scope.StageSynth /
    // scope.StageAntiPop. Wired IN-LINE (audio passes through), not as a
    // fan-out: fan-out tap nodes whose output is unreachable from
    // ctx.destination can be skipped by Chrome's renderer, leaving their
    // time-domain buffer all-zero even though ingress is receiving audio.
    // AnalyserNode passes audio through unchanged, so in-line wiring is
    // transparent to the audible signal. See
    // chain_per_instrument_traces.browser.test.js — case "antipop_vs_eq".
    if (chain.synthAnalyser) {
      node.connect(chain.synthAnalyser);
      node = chain.synthAnalyser;
    }
    // Insert effects (before EQ)
    node = wireInsertFX(node);
    // Pre-EQ analyser (after inserts, before EQ) — feeds scope.StageInsertFX.
    // Also wired IN-LINE; see synth-analyser comment above for rationale.
    if (chain.preEQAnalyser) {
      node.connect(chain.preEQAnalyser);
      node = chain.preEQAnalyser;
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
  // Synth analyser (BEFORE inserts) — see main-bus branch above for the
  // reason these analysers are wired IN-LINE rather than as fan-out taps.
  if (chain.synthAnalyser) {
    node.connect(chain.synthAnalyser);
    node = chain.synthAnalyser;
  }
  // Insert effects (before EQ)
  node = wireInsertFX(node);
  // Pre-EQ analyser (after inserts, before EQ) — feeds scope.StageInsertFX.
  if (chain.preEQAnalyser) {
    node.connect(chain.preEQAnalyser);
    node = chain.preEQAnalyser;
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
  // Idempotent: reuse the existing analyser when its fftSize already matches.
  // setEQActiveChannel re-issues all three Enable*Analyzer calls every time
  // the user switches channels; rebuilding the AudioNode + rewiring three
  // times in a row triggers a WebAudio race where the freshly-created
  // synth/preEQ tap occasionally never sees its ingress fan-out
  // re-established. See chain_per_instrument_traces.browser.test.js.
  if (chain.analyser && chain.analyser.fftSize === fft) {
    return;
  }
  const a = c.createAnalyser();
  a.fftSize = fft;
  a.smoothingTimeConstant = Math.max(0, Math.min(0.95, opts.smoothing || 0.0));
  chain.analyser = a;
  chain._timeBuf = new Float32Array(a.fftSize);
  chain._freqBuf = new Float32Array(a.frequencyBinCount);
  rewireChannel(chain);
}

// fillSnapshotBuffersForChain populates the chain's persistent typed-array
// output buffers from the given AnalyserNode. Returns the result object
// (also persistent on the chain) so the Go-WASM bridge can read it via
// js.CopyBytesToGo on the Uint8Array views, eliminating the per-element
// js.Value allocation loop that was the WASM OOM driver. Mutates the
// chain's _snapResult in-place; no new objects are allocated per call.
//
// Output object shape:
//   { rms: number, peak: number,
//     spectrumLen: int, waveLen: int,
//     spectrumU8: Uint8Array, waveU8: Uint8Array,
//     // Back-compat: kept for any caller still using plain arrays.
//     spectrum: Array<number>, wave: Array<number> }
//
// The U8 views are 1:1 byte views over the same persistent Float32Array
// buffers used to compute the data — no extra copy. Reading on the Go
// side requires only ONE bulk CopyBytesToGo per array (vs. 64+512 per-
// element js.Value allocations under the old plain-array contract).
function fillSnapshotBuffersForChain(chain, analyserKey, timeBufKey, freqBufKey, bins) {
  const a = chain[analyserKey];
  let result = chain._snapResult || {};
  chain._snapResult = result;
  if (!a) {
    result.rms = 0;
    result.peak = 0;
    result.spectrumLen = 0;
    result.waveLen = 0;
    result.spectrumU8 = null;
    result.waveU8 = null;
    return result;
  }
  const time = chain[timeBufKey] || new Float32Array(a.fftSize);
  const freq = chain[freqBufKey] || new Float32Array(a.frequencyBinCount);
  chain[timeBufKey] = time;
  chain[freqBufKey] = freq;
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
  // Spectrum: persistent Float32Array sized to bin count.
  const n = freq.length;
  const step = Math.max(1, Math.floor(n / Math.max(1, bins)));
  const numBins = Math.ceil(n / step);
  let specOut = chain._specF32;
  if (!specOut || specOut.length < numBins) {
    specOut = chain._specF32 = new Float32Array(numBins);
    chain._specU8 = new Uint8Array(specOut.buffer);
  }
  let oi = 0;
  for (let i = 0; i < n; i += step) {
    let max = -Infinity;
    const end = Math.min(n, i + step);
    for (let j = i; j < end; j++) {
      if (freq[j] > max) max = freq[j];
    }
    specOut[oi++] = Math.pow(10, max / 20);
  }
  // Wave: persistent Float32Array sized to fftSize, clamp to [-1,1].
  let waveOut = chain._waveF32;
  if (!waveOut || waveOut.length < time.length) {
    waveOut = chain._waveF32 = new Float32Array(time.length);
    chain._waveU8 = new Uint8Array(waveOut.buffer);
  }
  for (let i = 0; i < time.length; i++) {
    const v = time[i];
    waveOut[i] = v > 1 ? 1 : v < -1 ? -1 : v;
  }
  result.rms = rms;
  result.peak = peak;
  result.spectrumLen = oi;
  result.waveLen = time.length;
  result.spectrumU8 = chain._specU8;
  result.waveU8 = chain._waveU8;
  // Back-compat fields: do NOT allocate plain arrays here. Old callers
  // that read `.spectrum` / `.wave` will see undefined and must adapt;
  // the only known consumer is the Go-WASM bridge, which uses the U8
  // fast path with a fallback to the absent fields (treated as empty).
  return result;
}

export function channelAnalyzerSnapshot(id, bins = 64) {
  const chain = getChannelChain(id);
  if (!chain) {
    return { rms: 0, peak: 0, spectrumLen: 0, waveLen: 0, spectrumU8: null, waveU8: null };
  }
  return fillSnapshotBuffersForChain(chain, "analyser", "_timeBuf", "_freqBuf", bins);
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
  // Idempotent — see enableChannelAnalyzer for the rationale.
  if (chain.preEQAnalyser && chain.preEQAnalyser.fftSize === fft) {
    return;
  }
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
  if (!chain) {
    return { rms: 0, peak: 0, spectrumLen: 0, waveLen: 0, spectrumU8: null, waveU8: null };
  }
  // The pre-EQ tap has its own persistent typed-array slots (_preEQSnap*)
  // so it doesn't clobber the post-EQ snapshot buffers when both are read
  // within the same Draw (Chain panel does this).
  let chainTap = chain._preEQTap;
  if (!chainTap) {
    chainTap = chain._preEQTap = {};
  }
  // Mirror analyser → analyserKey + buf keys via the chainTap object,
  // which already lives on the chain and survives renderer rebuilds.
  chainTap.analyser = chain.preEQAnalyser;
  chainTap._timeBuf = chain._preEQTimeBuf;
  chainTap._freqBuf = chain._preEQFreqBuf;
  const r = fillSnapshotBuffersForChain(chainTap, "analyser", "_timeBuf", "_freqBuf", bins);
  // Persist buf references back onto the parent chain so the next call
  // reuses them (avoids fresh Float32Array allocation when the chain
  // tap-state was just initialized).
  chain._preEQTimeBuf = chainTap._timeBuf;
  chain._preEQFreqBuf = chainTap._freqBuf;
  return r;
}

// enableSynthAnalyzer attaches an AnalyserNode at the channel ingress, BEFORE
// any insert FX or EQ. The Chain panel uses this tap for both scope.StageSynth
// (pre-everything) and scope.StageAntiPop (in WASM the per-source anti-pop
// envelope is already applied at the source before reaching the channel bus).
export function enableSynthAnalyzer(id, opts = {}) {
  if (!hasCtx()) {
    pendingChannelOps.push(() => enableSynthAnalyzer(id, opts));
    return;
  }
  const chain = getChannelChain(id);
  if (!chain) return;
  const c = ctx;
  const fft = (() => {
    const v = opts.fftSize || 512;
    return Math.pow(2, Math.round(Math.log2(Math.max(64, Math.min(8192, v)))));
  })();
  // Idempotent — see enableChannelAnalyzer for the rationale.
  if (chain.synthAnalyser && chain.synthAnalyser.fftSize === fft) {
    return;
  }
  const a = c.createAnalyser();
  a.fftSize = fft;
  a.smoothingTimeConstant = Math.max(0, Math.min(0.95, opts.smoothing || 0.0));
  chain.synthAnalyser = a;
  chain._synthTimeBuf = new Float32Array(a.fftSize);
  chain._synthFreqBuf = new Float32Array(a.frequencyBinCount);
  rewireChannel(chain);
}

export function synthAnalyzerSnapshot(id, bins = 64) {
  const chain = getChannelChain(id);
  if (!chain) {
    return { rms: 0, peak: 0, spectrumLen: 0, waveLen: 0, spectrumU8: null, waveU8: null };
  }
  let chainTap = chain._synthTap;
  if (!chainTap) {
    chainTap = chain._synthTap = {};
  }
  chainTap.analyser = chain.synthAnalyser;
  chainTap._timeBuf = chain._synthTimeBuf;
  chainTap._freqBuf = chain._synthFreqBuf;
  const r = fillSnapshotBuffersForChain(chainTap, "analyser", "_timeBuf", "_freqBuf", bins);
  chain._synthTimeBuf = chainTap._timeBuf;
  chain._synthFreqBuf = chainTap._freqBuf;
  return r;
}

// enableSendBusAnalyzer creates the shared analyser that taps the summed
// send-bus returns (delay tail + reverb tail). Any send buses already
// instantiated when this runs are re-tapped so the analyser starts seeing
// audio immediately; future buses pick it up at construction time.
export function enableSendBusAnalyzer(opts = {}) {
  if (!hasCtx()) {
    pendingChannelOps.push(() => enableSendBusAnalyzer(opts));
    return;
  }
  const c = ctx;
  const fft = (() => {
    const v = opts.fftSize || 512;
    return Math.pow(2, Math.round(Math.log2(Math.max(64, Math.min(8192, v)))));
  })();
  if (!sendBusAnalyser) {
    sendBusAnalyser = c.createAnalyser();
    sendBusAnalyser.fftSize = fft;
    sendBusAnalyser.smoothingTimeConstant = Math.max(0, Math.min(0.95, opts.smoothing || 0.0));
    sendBusTimeBuf = new Float32Array(sendBusAnalyser.fftSize);
    sendBusFreqBuf = new Float32Array(sendBusAnalyser.frequencyBinCount);
  }
  // Tap whichever buses already exist. The bus-creation code paths also check
  // `sendBusAnalyser` and tap on construction for buses built later.
  if (delaySendBus) {
    // delaySendBus is the input gain; the wet output is the DelayNode itself,
    // which we can't reach directly from here. Tapping inputGain is wrong
    // because that's the dry summing point with no echo. Instead the wet
    // node was tapped inline in ensureDelaySendBus; do nothing here for
    // already-built buses other than refresh references.
  }
  if (reverbSendBus) {
    // Same rationale as above: the wet wetGain was tapped inline in
    // ensureReverbSendBus.
  }
}

// sendBusChainProxy is a module-scope proxy object that mimics the per-
// channel chain shape so fillSnapshotBuffersForChain can be reused.
// Module-scope because sendBusAnalyser/sendBus*Buf are module globals.
const sendBusChainProxy = {};

export function sendBusAnalyzerSnapshot(bins = 64) {
  sendBusChainProxy.analyser = sendBusAnalyser;
  sendBusChainProxy._timeBuf = sendBusTimeBuf;
  sendBusChainProxy._freqBuf = sendBusFreqBuf;
  const r = fillSnapshotBuffersForChain(sendBusChainProxy, "analyser", "_timeBuf", "_freqBuf", bins);
  sendBusTimeBuf = sendBusChainProxy._timeBuf;
  sendBusFreqBuf = sendBusChainProxy._freqBuf;
  return r;
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
async function ensureRenderedSample(id, opts) {
  if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch && window.__synthEvtTrace) {
    console.log('[SYNTH-ERS]', 'enter', { id, hasCache: renderCache.has(id), params: instrumentParamsFor(id) });
  }
  if (!RENDER[id]) {
    return null;
  }
  // skipEdit renders the UN-edited source for the Sampler editor's raw capture.
  // When an edit descriptor exists, the regular renderCache holds the EDITED
  // buffer, so a skipEdit render must bypass the cache in both directions
  // (read above is fine — see the bypass below — and write at the end).
  const skipEdit = !!(opts && opts.skipEdit);
  const sampleEdit = skipEdit ? null : (instrumentSampleEdits.get(id) || null);
  const bypassCache = skipEdit && instrumentSampleEdits.has(id);
  // Use getSampleRate() to avoid creating AudioContext prematurely.
  // If ctx already exists we get the real rate; otherwise 48000 default.
  const sr = Math.max(8000, Math.min(192000, getSampleRate()));
  // If cache exists but was built for a different sample rate, rebuild so pitch
  // and duration stay correct on devices that default to 48 kHz.
  if (!bypassCache && renderCache.has(id)) {
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
  // Phase 5: when user has edited synth params for this instrument,
  // route through the parameterized C variant (`render_X_p`). The
  // synth_params struct is 8 floats (32 bytes); we allocate it in the
  // WASM heap, fill it from instrumentSynthParams[id], and free after.
  const params = instrumentParamsFor(id);
  // Select the param block by the instrument's render family. The legacy
  // bespoke renderers use the 7-field synth_params block; the unified modular
  // voice (paramBlock:'modular') uses the wide modular_params block. Both are
  // schema-keyed flat float blocks generated from synth_param_abi.gen.js, so
  // the C struct field order is the single source of truth (drift-tested by
  // synth_param_schema_test.go).
  const isModular = !!(info && info.paramBlock === 'modular');
  // Family instruments (paramBlock 'fm' / 'kick' / …) use their family block
  // (synth_params base at offsets 0..6 + curated family knobs after),
  // mirroring the desktop builtinFamilyRenderers dispatch. Family fields
  // default to NaN ("keep the engine literal") via the block's identity map,
  // so unedited knobs never perturb the native sound.
  const familyBlock = (info && !isModular && FAMILY_PARAM_BLOCKS[info.paramBlock]) || null;
  const useModular = isModular;
  const fam = useModular ? null : familyBlock;
  const PARAM_COUNT = useModular ? MODULAR_PARAM_COUNT : (fam ? fam.count : SYNTH_PARAM_COUNT);
  const PARAM_INDEX = useModular ? MODULAR_PARAM_INDEX : (fam ? fam.index : SYNTH_PARAM_INDEX);
  const PARAM_IDENTITY = useModular ? MODULAR_PARAM_IDENTITY : (fam ? fam.identity : SYNTH_PARAM_IDENTITY);
  let paramsPtr = 0;
  if (params) {
    // Phase 2 of the live-instrument synthesis remediation plan replaced
    // the hand-maintained positional heap[0..5] writes with a schema-keyed
    // write loop driven by PARAM_INDEX from synth_param_abi.gen.js. The C
    // struct field order is the source of truth; the gen file is regenerated
    // from src/go/internal/audio/synth_param_schema.go. If the Go schema and
    // the gen file drift, synth_param_schema_test.go (Go side) fails first.
    //
    // CRITICAL: unset knobs MUST resolve to their C-side IDENTITY values
    // (the *_PARAM_IDENTITY map mirrors the C fallbacks). Passing 0 for decay
    // causes the legacy `_p` variant to divide-by-zero on the envelope formula
    // (`1 / decayMul`), producing NaN output that silences the channel.
    // Allocate with a safety floor and ZERO the block. The floor guards against
    // a stale/undersized ABI (the silence bug): the C modular_params struct is
    // wider than the legacy synth_params, so if MODULAR_PARAM_COUNT ever lags the
    // struct, an exact-size malloc would let the C voice read past the buffer
    // (uninitialised heap → random/garbage enable flags → dead generator). A
    // generous zeroed block keeps every read in-bounds and deterministic.
    const allocCount = useModular ? Math.max(PARAM_COUNT, MODULAR_ALLOC_FLOOR) : PARAM_COUNT;
    paramsPtr = m._malloc(allocCount * 4);
    if (paramsPtr) {
      // NOTE: named pheap (NOT heap) on purpose — a second `const heap` for the
      // OUTPUT buffer is declared later in this function. Reusing the name here
      // creates a temporal-dead-zone trap for any later reference to `heap`
      // before that declaration. Keep these two views distinctly named.
      const pheap = m.HEAPF32.subarray(paramsPtr >> 2, (paramsPtr >> 2) + allocCount);
      pheap.fill(0);
      for (const [name, idx] of Object.entries(PARAM_INDEX)) {
        const v = params[name];
        pheap[idx] = (typeof v === 'number' && Number.isFinite(v))
          ? v
          : PARAM_IDENTITY[name];
      }
    }
  }
  try {
    if (paramsPtr) {
      const renderFnP = useModular ? 'render_modular_p' : (RENDER[id] + '_p');
      if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch) {
        // Dump the exact modular heap fields most likely to silence a re-voiced
        // voice so a repro shows WHICH field is wrong (vs. inferring from RMS).
        // Read via paramsPtr directly — do NOT reference the block-scoped `heap`
        // (a second `const heap` is declared later in this fn; touching the name
        // here would hit its temporal dead zone and throw).
        const fld = (n) => (paramsPtr && Number.isFinite(MODULAR_PARAM_INDEX[n]))
          ? m.HEAPF32[(paramsPtr >> 2) + MODULAR_PARAM_INDEX[n]] : 'n/a';
        const block = useModular ? {
          osc_type: fld('osc_type'), osc_enabled: fld('osc_enabled'),
          fm_enabled: fld('fm_enabled'), env_enabled: fld('env_enabled'),
          filter_enabled: fld('filter_enabled'), drive_enabled: fld('drive_enabled'),
          gain: fld('gain'), osc_level: fld('osc_level'), level: fld('level'),
        } : null;
        console.log('[SYNTH-DISPATCH]', 'parameterized render', { id, fn: renderFnP, useModular, block, params });
      }
      try {
        // Test seam (consistent with window.__beatmoDebugSynthDispatch above):
        // synth_revoice_render_resilience.browser.test.js arms this flag to force
        // the parameterized / re-voice render to throw, exercising the native
        // fallback below. Production code never sets it.
        if (typeof window !== 'undefined' && window.__forceRevoiceRenderThrow) {
          throw new Error('forced re-voice render throw (test seam __forceRevoiceRenderThrow)');
        }
        m.ccall(renderFnP, null, ['number', 'number', 'number', 'number'], [ptr, sr, frames, paramsPtr]);
      } catch (perr) {
        // A throw in the parameterized / re-voice render (e.g. a missing C export
        // in a stale audio module, or a bad ccall arg) must NOT permanently
        // silence the instrument. processAudioEvent drops the hit and re-renders
        // on every subsequent hit, so a persistent throw here leaves the row dead
        // until reload — exactly the user-reported "audio instantly stops for that
        // instrument". Fall back to the instrument's native renderer so it keeps
        // sounding (degraded, not dead) and surface the real error LOUDLY.
        console.error('[AUDIOJS] parameterized render threw for ' + id + ' via ' + renderFnP +
          '; falling back to native ' + RENDER[id] + '. err=' + (perr && perr.message ? perr.message : String(perr)));
        m.ccall(RENDER[id], null, ['number', 'number', 'number'], [ptr, sr, frames]);
      }
    } else {
      if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch) {
        console.log('[SYNTH-DISPATCH]', 'unparameterized render', { id, fn: RENDER[id] });
      }
      m.ccall(RENDER[id], null, ['number', 'number', 'number'], [ptr, sr, frames]);
    }
    const heap = m.HEAPF32.subarray(ptr >> 2, (ptr >> 2) + frames);
    const data = new Float32Array(frames);
    data.set(heap);
    let peak = 0;
    for (let i = 0; i < data.length; i++) {
      const a = Math.abs(data[i]);
      if (a > peak) peak = a;
    }
    if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch) {
      console.log('[SYNTH-DISPATCH]', 'render result', { id, rawPeak: peak, silent: !(peak > 1e-6) });
    }
    if (peak > 0) {
      const inv = 1 / peak;
      for (let i = 0; i < data.length; i++) data[i] *= inv;
    }
    const amp = Number.isFinite(info.amp) ? info.amp : 0.6;
    for (let i = 0; i < data.length; i++) data[i] *= amp;
    // Non-destructive Sampler edit: same transform as Go's BakeSample, applied
    // to the fresh C render (mirrors tryRecipeVoice in synth_recipe_dispatch.go).
    // The synth stays the source of truth — a param change re-renders through
    // this path on the next play, so synth edits always take effect.
    let finalData = data;
    if (sampleEdit) {
      finalData = applySampleEdit(data, sr, sampleEdit);
    }
    // Store raw Float32Array + metadata. AudioBuffer is created lazily in
    // ensureAudioBuffer() when playback actually needs it, so this path
    // never forces an AudioContext into existence.
    const record = { buffer: null, data: finalData, sr, frames: finalData.length };
    if (!bypassCache) {
      renderCache.set(id, record);
    }
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
    if (paramsPtr) {
      m._free(paramsPtr);
    }
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

// registerSamplePCM installs a raw PCM buffer (produced by the Go Sampler tab)
// as the sound for id, replacing any C-synth render. The Float32Array is stored
// in renderCache so the AudioBuffer is wrapped lazily in ensureAudioBuffer()
// (parity with rendered synths — no premature AudioContext on mobile). RENDER[id]
// and any stale AudioBuffer are cleared so playback resolves the new PCM (this
// is how "Save" overrides an existing synth instrument with a sample).
export function registerSamplePCM(id, u8, sr) {
  const frames = Math.floor((u8 && u8.byteLength ? u8.byteLength : 0) / 4);
  const data = new Float32Array(frames);
  if (frames > 0) {
    data.set(new Float32Array(u8.buffer, u8.byteOffset, frames));
  }
  const rate = sr && sr > 0 ? sr : 44100;
  delete RENDER[id];
  renderCache.set(id, { buffer: null, data, sr: rate, frames });
  delete samples[id];
  rawRenderCache.delete(id);
  dbg('sample.registerPCM', { id, frames, sr: rate });
}
window.registerSamplePCM = (id, u8, sr) => registerSamplePCM(id, u8, sr);

// RENDER_DEFAULTS preserves the as-shipped id→C-render mapping so a factory
// Reset can restore an instrument's synth render after registerSamplePCM
// deleted RENDER[id] to override it with a chopped sample.
const RENDER_DEFAULTS = { ...RENDER };

// unregisterSamplePCM is the inverse of registerSamplePCM: it drops the PCM
// override for id (clearing the cached buffer + render cache) and restores the
// built-in synth render mapping, so the Sampler/Synth "Reset to factory" makes
// the original synth audible again instead of the leftover chop. No-op-safe for
// ids that have no default render (pure user samples).
export function unregisterSamplePCM(id) {
  renderCache.delete(id);
  delete samples[id];
  rawRenderCache.delete(id);
  if (RENDER_DEFAULTS[id] !== undefined) {
    RENDER[id] = RENDER_DEFAULTS[id];
  }
  dbg('sample.unregisterPCM', { id, restored: RENDER_DEFAULTS[id] !== undefined });
}
window.unregisterSamplePCM = (id) => unregisterSamplePCM(id);

// captureInstrumentPCM returns the rendered one-shot PCM for id (as a byte view
// over its Float32Array) plus its sample rate, for the Go Sampler "From Synth"
// capture. Returns null when nothing is rendered yet, kicking off a render so a
// retry succeeds. With raw=true and a sample-edit descriptor present, the
// capture comes from an UN-edited render (rawRenderCache) so the Sampler
// editor sees the source waveform and overlays the saved edit itself.
export function captureInstrumentPCM(id, raw) {
  let record;
  if (raw && instrumentSampleEdits.has(id) && RENDER[id]) {
    record = rawRenderCache.get(id);
    if (!record || !record.data) {
      // Kick off an async un-edited render; the Go side retries on null
      // (same contract as the regular ensureRenderReady warm-up below).
      ensureRenderedSample(id, { skipEdit: true })
        .then((rec) => { if (rec && rec.data) rawRenderCache.set(id, rec); })
        .catch(() => {});
      record = rawRenderCache.get(id);
    }
  } else {
    record = renderCache.get(id);
    if ((!record || !record.data) && RENDER[id]) {
      try { ensureRenderReady(id); } catch (_) {}
      record = renderCache.get(id);
    }
  }
  if (!record || !record.data) return null;
  const data = record.data;
  return {
    bytes: new Uint8Array(data.buffer, data.byteOffset, data.byteLength),
    length: data.length,
    sr: record.sr,
  };
}
window.captureInstrumentPCM = (id, raw) => captureInstrumentPCM(id, raw);

// decodeWavToPCM fetches and decodes a WAV (or any decodeAudioData-supported)
// file at the given URL into mono PCM, returning a byte view over the
// Float32Array plus its sample rate. Used by the Go Sampler tab's "Load WAV"
// on the browser so the samples can be drawn + edited in Go.
export async function decodeWavToPCM(url) {
  const res = await fetch(url);
  const arr = await res.arrayBuffer();
  const buf = await getCtx().decodeAudioData(arr);
  const f32 = buf.numberOfChannels > 0 ? buf.getChannelData(0) : new Float32Array(0);
  return {
    bytes: new Uint8Array(f32.buffer, f32.byteOffset, f32.byteLength),
    length: f32.length,
    sr: buf.sampleRate,
  };
}
window.decodeWavToPCM = (url) => decodeWavToPCM(url);

// ───────── Sampler cross-session persistence (IndexedDB) ─────────
// User samples (PCM) are too large for localStorage's ~5 MB cap, so the Go
// userprefs SampleStore (wasm backend) persists them here. id = instrument id,
// sr = sample rate, bytes = little-endian float32 PCM as a Uint8Array.
const IDB_NAME = "beatmo";
const IDB_SAMPLES_STORE = "samples";
function idbOpenSamples() {
  return new Promise((resolve, reject) => {
    if (typeof indexedDB === "undefined") { reject(new Error("no indexedDB")); return; }
    const req = indexedDB.open(IDB_NAME, 1);
    req.onupgradeneeded = () => {
      const db = req.result;
      if (!db.objectStoreNames.contains(IDB_SAMPLES_STORE)) {
        db.createObjectStore(IDB_SAMPLES_STORE, { keyPath: "id" });
      }
    };
    req.onsuccess = () => resolve(req.result);
    req.onerror = () => reject(req.error);
  });
}
window.idbGetAllSamples = async () => {
  try {
    const db = await idbOpenSamples();
    return await new Promise((resolve, reject) => {
      const tx = db.transaction(IDB_SAMPLES_STORE, "readonly");
      const req = tx.objectStore(IDB_SAMPLES_STORE).getAll();
      req.onsuccess = () => resolve((req.result || []).map((r) => ({ id: r.id, sr: r.sr, bytes: r.bytes })));
      req.onerror = () => reject(req.error);
    });
  } catch (_) { return []; }
};
window.idbPutSample = async (id, sr, u8) => {
  try {
    const db = await idbOpenSamples();
    const bytes = new Uint8Array(u8.length);
    bytes.set(u8);
    await new Promise((resolve, reject) => {
      const tx = db.transaction(IDB_SAMPLES_STORE, "readwrite");
      tx.objectStore(IDB_SAMPLES_STORE).put({ id, sr, bytes });
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
  } catch (_) {}
};
window.idbDeleteSample = async (id) => {
  try {
    const db = await idbOpenSamples();
    await new Promise((resolve, reject) => {
      const tx = db.transaction(IDB_SAMPLES_STORE, "readwrite");
      tx.objectStore(IDB_SAMPLES_STORE).delete(id);
      tx.oncomplete = () => resolve();
      tx.onerror = () => reject(tx.error);
    });
  } catch (_) {}
};

// ensureRenderReady is exported so test harnesses can force a re-render
// after invalidating the cache via setInstrumentParam (which renderCache
// .delete's the entry). Production code reaches it through
// enqueueAudioEvents / playSound; tests use it directly.
export function ensureRenderReady(id) {
  const trace = typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch && window.__synthEvtTrace;
  if (renderCache.has(id)) {
    return Promise.resolve(renderCache.get(id));
  }
  if (!RENDER[id]) {
    return Promise.resolve(null);
  }
  let pending = pendingRenderEnsures.get(id);
  if (pending) {
    if (trace) console.log('[SYNTH-RDY]', 'reuse pending', { id });
    return pending;
  }
  if (trace) console.log('[SYNTH-RDY]', 'start render', { id });
  pending = ensureRenderedSample(id).then((record) => {
    pendingRenderEnsures.delete(id);
    if (trace) console.log('[SYNTH-RDY]', 'render done', { id, ok: !!record, nowCached: renderCache.has(id) });
    return record;
  }).catch((err) => {
    pendingRenderEnsures.delete(id);
    if (trace) console.log('[SYNTH-RDY]', 'render THREW', { id, err: String(err) });
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
  if (!buf && renderCache.has(id)) {
    ensureAudioBuffer(id);
    buf = samples[id];
  }
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
  if (!buf && renderCache.has(id)) {
    ensureAudioBuffer(id);
    buf = samples[id];
  }
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

window.enableSynthAnalyzer = (id, windowSize) => {
  try { enableSynthAnalyzer(id, { fftSize: windowSize }); } catch (err) { dbg('synth.analyzer.error', { id, err: String(err) }); }
};

window.synthAnalyzerSnapshot = (id) => {
  try { return synthAnalyzerSnapshot(id); } catch (err) { dbg('synth.analyzer.snap.error', { id, err: String(err) }); return { rms: 0, peak: 0, spectrum: [], wave: [] }; }
};

window.enableSendBusAnalyzer = (windowSize) => {
  try { enableSendBusAnalyzer({ fftSize: windowSize }); } catch (err) { dbg('sendbus.analyzer.error', { err: String(err) }); }
};

window.sendBusAnalyzerSnapshot = () => {
  try { return sendBusAnalyzerSnapshot(); } catch (err) { dbg('sendbus.analyzer.snap.error', { err: String(err) }); return { rms: 0, peak: 0, spectrum: [], wave: [] }; }
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
    // Resolve empty ONLY on a genuine cancel. Never arm a blind timer: the
    // native dialog is modal but JS timers keep running, so a fixed timeout
    // resolves '' while the user is still browsing — the real selection then
    // arrives after the promise is settled and is silently dropped ("nothing
    // happens" on import). Modern browsers fire a 'cancel' event on the input
    // when the dialog is dismissed without a choice; the Go-side ~10s frame
    // guard (drumview_update.go) releases the import flow on browsers that don't.
    input.addEventListener('cancel', () => {
      console.warn('[IMPORT] file picker canceled');
      settle('');
      setTimeout(() => { try { input.remove(); } catch (_) {} }, 0);
    });
    document.body.appendChild(input);
    console.log('[IMPORT] clicking file input');
    input.click();
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
// Internal metric: number of insert FX subgraphs currently wired on a channel.
// Returns 0 when the channel chain hasn't been built yet (no AudioContext).
// Used by tests to verify that updateInsertEffects() applied after context
// unlock when called pre-context (e.g. demo import before user gesture).
window.__channelInsertFXCount = (id) => {
  try {
    if (!hasCtx()) return 0;
    const chain = channelNodes.get(id || 'main');
    if (!chain || !Array.isArray(chain.insertFX)) return 0;
    return chain.insertFX.length;
  } catch (_) { return 0; }
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

// Cross-platform chain-parity introspection. Returns the LIVE master-chain
// configuration so chain_spec_parity.browser.test.js can assert the browser
// nodes equal CHAIN_SPEC (the Go source of truth). Forces the lazy nodes to
// exist first so a test can read real values, not nulls. Returns null if no
// AudioContext can be created (headless without audio).
window.getChainConfigForTest = () => {
  // Force the AudioContext + lazy chain nodes into existence. Compressor and
  // WaveShaper nodes build fine on a suspended context (no running device
  // needed), so this works headless.
  try { getCtx(); } catch (_) { return null; }
  if (!hasCtx()) return null;
  const comp = ensureCompressor();
  ensureLimiter();
  if (!comp) return null;
  // Probe the soft-clip curve at a level above threshold to prove the limiter
  // applies the tanh knee (not a pure hard clamp). Compare to softClipSample.
  const probe = 1.5;
  return {
    spec: CHAIN_SPEC,
    compressor: {
      thresholdDb: comp.threshold.value,
      ratio: comp.ratio.value,
      attackSec: comp.attack.value,
      releaseSec: comp.release.value,
      kneeDb: comp.knee.value,
    },
    softClip: {
      threshold: CHAIN_SPEC.softClipThreshold,
      // Expected limiter output for `probe`, so the test can confirm the
      // WaveShaper curve was built with the soft-clip (value < probe, < 1).
      probeIn: probe,
      probeOut: softClipSample(probe),
    },
  };
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
  //   - source → captureNode → ctx.destination (parallel tap; the worklet
  //     emits SILENCE, so connecting it to destination keeps it in the active
  //     render graph without adding any signal to the audible mix)
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
    node.connect(c.destination);  // parallel tap; worklet emits silence (see worklet)
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

// NOTE: an earlier iteration installed a 4ms setInterval calling Go's
// tickSequencer() to bypass WASM goroutine scheduling. Benchmarks showed
// it INCREASED Stage B (bridge) jitter because the extra drive
// contended for seqMu against Update() and the audioLoop drive without
// reducing Stage A meaningfully — the seqMu TryLock conflicts dominate
// any sub-frame gain. The Go-side tickSequencer export is kept available
// for tests that want explicit sequencer drives.
