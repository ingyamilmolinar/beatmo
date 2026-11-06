import {
  MODULAR_PARAM_COUNT,
  MODULAR_PARAM_INDEX,
  MODULAR_PARAM_TABLES,
} from './synth_param_abi.gen.js';

// CHAIN_SPEC (generated from Go's chain_spec.go) is every value the path from a
// voice to the speaker uses on both platforms: the sample rate, the per-voice
// headroom, the fade / stop / gate times, the soft-clip knee, the master
// volume range. The DSP itself is desktop's C (dsp_host.js, run on the audio
// thread by the beatmo-dsp worklet) and a note is desktop's played voice
// (played_voice.js); xplat_audio_stages.browser.test.js compares every stage
// with desktop's, sample by sample.
import { CHAIN_SPEC } from './chain_spec.gen.js';
import { createStage } from './dsp_host.js';
import { gateLength, playedVoiceSamples } from './played_voice.js';
import { applyVoicePost } from './voice_post.js';


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
  // Stage TABLE columns ([first index, rows]) must fit in the block too.
  for (const k in MODULAR_PARAM_TABLES) {
    const [first, rows] = MODULAR_PARAM_TABLES[k];
    if (first + rows - 1 > maxIdx) maxIdx = first + rows - 1;
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
if (typeof window !== 'undefined') {
  // Test-friendly summary so the browser ABI guard test can assert the loaded
  // gen.js is consistent with the C struct (catches a stale committed file).
  window.__synthABI = {
    modularCount: MODULAR_PARAM_COUNT,
    stale: !MODULAR_ABI_OK,
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

// Output capture: intercept final mixed output for debugging/comparison.
//
// Captured on the AUDIO thread by a tiny AudioWorklet tap (parallel branch off
// the limiter) whenever the module has loaded; the ScriptProcessorNode is only
// the fallback. A ScriptProcessor runs its callback on the MAIN thread, and
// with the full app running (Go game loop + drawing) Chrome dropped most of its
// blocks: xplat_output_capture recorded fragments and silence for voices that
// were playing correctly. Worklet blocks are POSTED and queue rather than drop,
// so a busy main thread delays them but never loses them —
// window.flushOutputCapture() waits until everything captured so far arrived.
const OUTPUT_TAP_SRC = `class BeatmoOutputTap extends AudioWorkletProcessor {
  constructor() { super(); this.on = false;
    this.port.onmessage = (e) => { const d = e.data;
      if (d === 'on') this.on = true; else if (d === 'off') this.on = false;
      else if (d && d.flush !== undefined) this.port.postMessage({ flushed: d.flush }); }; }
  process(inputs) { const ch = inputs[0] && inputs[0][0]; if (this.on && ch) this.port.postMessage(ch.slice(0)); return true; }
}
registerProcessor('beatmo-output-tap', BeatmoOutputTap);`;
let outputTapModule = null; // Promise<boolean> once addModule was attempted
let outputTapReady = false;
let outputTapFlushSeq = 0;
const outputTapFlushWaiters = new Map();
function preloadOutputTap(c) {
  if (outputTapModule || !c || !c.audioWorklet) return;
  try {
    const url = URL.createObjectURL(new Blob([OUTPUT_TAP_SRC], { type: 'application/javascript' }));
    outputTapModule = c.audioWorklet.addModule(url).then(() => { outputTapReady = true; return true; }, () => false);
  } catch (_) { outputTapModule = Promise.resolve(false); }
}
let outputCaptureIsWorklet = false;
let outputCaptureNode = null;
let outputCaptureBuffer = [];
let outputCaptureEnabled = false;
let outputCaptureSampleRate = CHAIN_SPEC.sampleRate;

// masterOut is the last node before ctx.destination: what the device
// receives, after the master soft clip (desktop's output capture and
// recording tap the same point). Output capture and the recording master tap
// read it. Created lazily.
let masterOut = null;
// masterClip is the master soft clip — desktop's C (mix_dsp.c soft_clip) on
// the DSP host; null until that host is up.
let masterClip = null;

function ensureMasterOut() {
  if (masterOut) return masterOut;
  if (!hasCtx()) return null;
  masterOut = ctx.createGain();
  masterOut.gain.value = 1;
  masterOut.connect(ctx.destination);
  return masterOut;
}

// audioDestination is where the master bus's chain ends: the soft clip, which
// feeds masterOut (masterOut itself until the DSP host is up).
function audioDestination() {
  const out = ensureMasterOut();
  if (!out) return hasCtx() ? ctx.destination : null;
  if (!masterClip && dspHostUp(ctx)) {
    masterClip = dspNode(ctx, 'softclip', { threshold: CHAIN_SPEC.softClipThreshold });
    if (masterClip) masterClip.connect(out);
  }
  return masterClip || out;
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
      considerRelease(id);
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

// ───────── Audio lane registry (vid > 0) ─────────
// The Go audio lane commits sequencer notes up to its horizon (~1 s) ahead
// and must take them back when an edit changes them. vid 0 = preview / UI
// sound: never touched here.
const LANE_CANCEL_GUARD_SEC = 0.008; // closer to start than this = "started"
const laneVoices = new Map();      // vid → { src, gain, inst, when, pk }
const laneTombstones = new Set();  // vids cancelled before they reached start()
let laneGeneration = 0;            // bumped by cancelAllLaneFuture

function laneRegister(vid, src, gain, inst, when, pk, level) {
  if (!vid) return;
  laneVoices.set(vid, { src, gain, inst, when, pk, level });
  // Only if still THIS source: a swapped voice's stopped stale source ends
  // after the fresh one registered under the same vid.
  try { src.addEventListener('ended', () => { if (laneVoices.get(vid)?.src === src) laneVoices.delete(vid); }, { once: true }); } catch (_) {}
}

// laneCancelOne: silent stop of a not-yet-started lane voice. A vid not
// started yet anywhere (queued, awaiting a worker render, stale-race replay)
// is tombstoned so the replay drops it. Returns true if it already started.
function laneCancelOne(vid, now) {
  const v = laneVoices.get(vid);
  if (!v) { laneTombstone(vid); return false; }
  if (v.when <= now + LANE_CANCEL_GUARD_SEC) return true; // started: leave it (edit semantics (a))
  try { v.src.stop(0); } catch (_) {}
  try { v.src.disconnect(); } catch (_) {}
  try { v.gain.disconnect(); } catch (_) {}
  laneVoices.delete(vid);
  laneTombstone(vid); // a late replay of this vid (if any) must drop too
  if (audioProbe && v.pk) audioProbe.noteCancelled(v.pk);
  return false;
}

// Tombstones are consumed by the replay they target; ones whose replay
// never comes are pruned oldest-first so a long session cannot grow them.
const LANE_TOMBSTONES_MAX = 4096;
function laneTombstone(vid) {
  laneTombstones.add(vid);
  if (laneTombstones.size > LANE_TOMBSTONES_MAX) {
    for (const old of laneTombstones) { laneTombstones.delete(old); if (laneTombstones.size <= LANE_TOMBSTONES_MAX / 2) break; }
  }
}

// laneSwapFresh re-starts a lane voice that was scheduled on a STALE render
// on the fresh one, at the same start, if it has not started yet and the
// fresh record is current. Otherwise the stale voice plays (timing wins;
// timbre lags one note) — a newer edit brings a new lane revision anyway.
function laneSwapFresh(vid, ev, cacheKey) {
  const v = laneVoices.get(vid);
  if (!v || !hasCtx()) return;
  if (v.when <= ctx.currentTime + LANE_CANCEL_GUARD_SEC) return;
  const rec = renderCache.get(cacheKey);
  if (!rec || renderRecordStale(ev.id, rec)) return;
  try { v.src.stop(0); } catch (_) {}
  try { v.src.disconnect(); } catch (_) {}
  try { v.gain.disconnect(); } catch (_) {}
  laneVoices.delete(vid);
  ev.when = v.when;
  enqueueAudioEvents([ev]);
}

if (typeof window !== 'undefined') {
  // Returns the vids it REFUSED because they have started: Go then drops the
  // replacement of each (an atomic replace — never the old AND the new voice).
  window.cancelLaneVoices = (vids) => {
    const started = [];
    if (!vids) return started;
    const now = hasCtx() ? ctx.currentTime : 0;
    for (let i = 0; i < vids.length; i++) {
      const vid = Number(vids[i]);
      if (vid > 0 && laneCancelOne(vid, now)) started.push(vid);
    }
    return started;
  };

  // stopLaneAt: a mute node at ctx time t — fade the lane voices of inst that
  // start before t; voices starting at/after t are spared. The fade is
  // desktop's (antiPopVoice RequestStopAt): from the voice's own level,
  // linearly to zero over CHAIN_SPEC.stopFadeMs, starting on t's sample. A
  // voice's gain is constant until a stop (its fade-in is in its buffer), so
  // there is nothing to cancel-and-hold (Firefox lacks cancelAndHoldAtTime).
  window.stopLaneAt = (inst, t) => {
    if (!hasCtx()) return;
    const fade = CHAIN_SPEC.stopFadeMs / 1000;
    for (const [vid, v] of laneVoices) {
      if (v.inst !== inst || !(v.when < t)) continue;
      const at = frameTime(ctx, Math.max(t, ctx.currentTime));
      try {
        const g = v.gain.gain;
        g.cancelScheduledValues(at);
        g.setValueAtTime(v.level, at);
        g.linearRampToValueAtTime(0, at + fade);
        v.src.stop(at + fade + 1 / ctx.sampleRate);
      } catch (_) {}
      laneVoices.delete(vid);
    }
  };

  window.cancelAllLaneFuture = () => {
    laneGeneration++;
    if (!hasCtx()) return;
    const now = ctx.currentTime;
    for (const vid of Array.from(laneVoices.keys())) laneCancelOne(vid, now);
    lastScheduledPlayWhen = -1;
  };

  window.laneVoiceStateForTest = (vid) => {
    if (laneTombstones.has(vid)) return 'cancelled';
    const v = laneVoices.get(vid);
    if (!v) return 'unknown';
    return v.when <= (hasCtx() ? ctx.currentTime : 0) ? 'started' : 'scheduled';
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

// ───────── Render-latency metrics (WASM real-time observability) ─────────
// Companion to scheduleMetrics, covering the cost scheduleMetrics CANNOT see:
// a render-cache miss makes processAudioEvent DEFER the event and replay it
// after the async voice render at a fresh currentTime — so the note starts
// late relative to its musically-intended `when`, but observeScheduleTiming
// only ever sees the rewritten (healthy-lead) timestamp. These counters make
// that hidden lateness measurable:
//   - renderMs*: duration of one voice render as experienced by the trigger
//     path (worker round-trip INCLUDING queue wait, or the sync main-thread
//     ccall fallback), plus the main-thread post-process (normalize/amp/edit).
//   - deferMs*: SIGNED deviation (actual source start minus the originally-
//     intended `when`) for deferred plays. Positive = late (render+queue ate
//     more than the scheduling lead); negative = early (the replay drops the
//     intended lead and fires at now+minLead). Both directions are off-grid
//     jitter the listener hears when a live param edit evicts the cache
//     mid-playback; deferAbsMsP90 is the gate-worthy magnitude.
//   - workerQueuePeak: max in-flight render requests (single worker ⇒ queue
//     wait ≈ peak×renderMs).
const renderLatencyMetrics = {
  reset() {
    this.renders = 0;
    this.workerRenders = 0;
    this.mainThreadRenders = 0;
    this.deferredPlays = 0;
    this.renderMsSamples = [];
    this.renderMsSum = 0;
    this.renderMsMax = 0;
    this.deferAbsMsSamples = [];
    this.deferMsSum = 0;
    this.deferMsMin = 0;
    this.deferMsMax = 0;
    this.workerQueuePeak = 0;
    this.stalePlays = 0;
    this.neighborPlays = 0;
  },
  observeRender(ms, viaWorker, queueDepth) {
    if (!Number.isFinite(ms) || ms < 0) return;
    this.renders += 1;
    if (viaWorker) this.workerRenders += 1;
    else this.mainThreadRenders += 1;
    this.renderMsSum += ms;
    if (ms > this.renderMsMax) this.renderMsMax = ms;
    pushSample(this.renderMsSamples, ms);
    if (Number.isFinite(queueDepth) && queueDepth > this.workerQueuePeak) {
      this.workerQueuePeak = queueDepth;
    }
  },
  observeDefer(ms) {
    if (!Number.isFinite(ms)) return;
    if (this.deferredPlays === 0) {
      this.deferMsMin = this.deferMsMax = ms;
    } else {
      if (ms < this.deferMsMin) this.deferMsMin = ms;
      if (ms > this.deferMsMax) this.deferMsMax = ms;
    }
    this.deferredPlays += 1;
    this.deferMsSum += ms;
    pushSample(this.deferAbsMsSamples, Math.abs(ms));
  },
  snapshot() {
    return {
      renders: this.renders,
      workerRenders: this.workerRenders,
      mainThreadRenders: this.mainThreadRenders,
      renderMsAvg: this.renders ? this.renderMsSum / this.renders : 0,
      renderMsMax: this.renderMsMax,
      renderMsP90: percentile(this.renderMsSamples, 0.9),
      deferredPlays: this.deferredPlays,
      deferMsAvg: this.deferredPlays ? this.deferMsSum / this.deferredPlays : 0,
      deferMsMin: this.deferredPlays ? this.deferMsMin : 0,
      deferMsMax: this.deferredPlays ? this.deferMsMax : 0,
      deferAbsMsP90: percentile(this.deferAbsMsSamples, 0.9),
      workerQueuePeak: this.workerQueuePeak,
      // stalePlays: notes that played a previous-generation buffer ON TIME
      // because the fresh render missed the deadline (stale-while-revalidate).
      stalePlays: this.stalePlays,
      // neighborPlays: cold-miss notes that played the nearest cached pitch
      // resampled at the intended time while the exact pitch rendered.
      neighborPlays: this.neighborPlays,
    };
  },
};
renderLatencyMetrics.reset();
if (typeof window !== 'undefined') {
  window.getRenderLatencyMetrics = () => renderLatencyMetrics.snapshot();
  window.resetRenderLatencyMetrics = () => renderLatencyMetrics.reset();
}

// ───────── Timing traces (Stage-D browser emission timing, parity-timing-sync) ─────────
// A ring buffer of {row, abs, idealWhen, startWhen, ctxAtStart} recorded at
// each real BufferSource.start(when) for a SEQUENCER-identified note (row>=0
// — the WASM→JS batch bridge threads Row/Abs from audio.BatchParam, see
// batch_wasm.go). idealWhen is the pre-clamp musically-intended time when
// available, else the same as startWhen (the scheduled `when`); ctxAtStart is
// the true AudioContext clock read at the .start() call site. Fire-and-forget
// / preview plays (row<0, the default when a batch event carries no identity)
// are never recorded. Consumed by the Go-WASM Stage-D drift checker (Task
// 4.2) via window.getTimingTraces().
const TIMING_TRACES_MAX = 512;
const timingTraces = {
  buf: [],
  push(row, abs, idealWhen, startWhen, ctxAtStart) {
    if (this.buf.length >= TIMING_TRACES_MAX) this.buf.shift();
    this.buf.push({ row, abs, idealWhen, startWhen, ctxAtStart });
  },
  reset() { this.buf.length = 0; },
  snapshot() { return this.buf.slice(); },
};
if (typeof window !== 'undefined') {
  window.getTimingTraces = () => timingTraces.snapshot();
  window.resetTimingTraces = () => timingTraces.reset();
}

// ───────── Playback probe (what the listener actually HEARS) ─────────
// Opt-in measurement of the real output stream, for benches and for
// diagnosing choppy playback on real hardware (run from the console:
// `await startAudioProbe()`, play, `await audioProbeReport()`).
//
// Every other timing metric stops at the `when` handed to
// BufferSource.start. This one records, per sequencer note, the predictor's
// trigger time (grid), the scheduler's request (ideal), when JS actually
// called start() — and then detects the note's ONSET in the rendered audio,
// on the audio thread, via one `beatmo-audio-probe` AudioWorkletNode per
// channel bus plus one on the final output (audio_probe_worklet.js). The
// pure analysis (lag, jitter, missed/extra notes, clicks, cuts, clock slips)
// lives in audio_probe_analysis.js so Node benches run the same code.
//
// Zero cost while off: every hook is behind `if (audioProbe)`.
const AUDIO_PROBE_LOG_MAX = 50000;
let audioProbe = null;
let _audioProbeWorkletReady = null;

function createAudioProbeState(c, opts) {
  const st = {
    opts,
    sampleRate: c.sampleRate,
    baseLatency: Number.isFinite(c.baseLatency) ? c.baseLatency : null,
    outputLatency: Number.isFinite(c.outputLatency) ? c.outputLatency : null,
    ctxAtStart: c.currentTime,
    perfAtStart: performance.now(),
    seq: 0,
    requests: [],
    starts: [],
    cancels: [],           // lane voices cancelled before they started {pk, ctx}
    taps: new Map(),       // channelId → {node, source}
    reports: {},           // channelId → [report]
    loopGaps: [],          // main-thread event-loop stalls {ctx, ms}
    loopTimer: null,
    loopLast: performance.now(),
    loopMaxMs: 0,
    truncated: false,
    noteRequested(ev) {
      if (this.requests.length >= AUDIO_PROBE_LOG_MAX) { this.truncated = true; return; }
      ev.pk = ++this.seq;
      this.requests.push({
        pk: ev.pk, id: ev.id, row: ev.row, abs: ev.abs, vol: ev.vol, pitch: ev.pitch,
        when: Number.isFinite(ev.when) ? ev.when : null,
        ideal: Number.isFinite(ev.ideal) ? ev.ideal : null,
        grid: Number.isFinite(ev.grid) ? ev.grid : null,
        ctx: hasCtx() ? ctx.currentTime : 0,
        perf: performance.now(),
      });
    },
    noteCancelled(pk) {
      if (!pk || this.cancels.length >= AUDIO_PROBE_LOG_MAX) return;
      this.cancels.push({ pk, ctx: hasCtx() ? ctx.currentTime : 0 });
    },
    noteStarted(pk, id, startWhen, ctxNow, path) {
      if (this.starts.length >= AUDIO_PROBE_LOG_MAX) { this.truncated = true; return; }
      // fxLatency: how late the channel's insert chain makes this note sound
      // (insertChainLatencySec) — the analysis matches its onset that much later.
      const chain = channelNodes.get(id);
      const fxLatency = chain && chain.fxLatencySec ? chain.fxLatencySec : 0;
      this.starts.push({ pk, id, startWhen, ctx: ctxNow, perf: performance.now(), path, fxLatency });
    },
    tap(channelId, source) {
      if (!source || this.taps.has(channelId)) return;
      let node;
      try {
        node = new AudioWorkletNode(ctx, 'beatmo-audio-probe', {
          numberOfInputs: 1,
          numberOfOutputs: 0,
          channelCount: 2,
          channelCountMode: 'explicit',
          processorOptions: { channelId, ...(this.opts.detector || {}) },
        });
      } catch (err) {
        dbg('audio.probe.tap.error', { channelId, err: String(err) });
        return;
      }
      const list = this.reports[channelId] = this.reports[channelId] || [];
      node.port.onmessage = (e) => {
        const m = e.data;
        if (m && m.type === 'report' && list.length < AUDIO_PROBE_LOG_MAX) list.push(m);
      };
      source.connect(node);
      this.taps.set(channelId, { node, source });
    },
  };
  return st;
}

function _ensureAudioProbeWorklet(c) {
  if (!_audioProbeWorkletReady) {
    const url = new URL('./audio_probe_worklet.js', import.meta.url).href;
    _audioProbeWorkletReady = c.audioWorklet.addModule(url).then(() => true, (err) => {
      console.error('[AUDIO PROBE] worklet load failed:', err);
      _audioProbeWorkletReady = null;
      return false;
    });
  }
  return _audioProbeWorkletReady;
}

// Called from buildChannelPipeline so channels created mid-probe are tapped.
function audioProbeTapChannel(id, chain) {
  if (audioProbe && chain && id !== 'main') audioProbe.tap(id, chain.gain);
}

// startAudioProbe({master=true, channels=true, detector}) → {ok, channels}
// Taps every existing channel bus (post EQ/FX/fader, pre-pan) and the final
// output (post-limiter). Restarting discards the previous log.
window.startAudioProbe = async (opts = {}) => {
  const c = getCtx();
  if (!c || !c.audioWorklet) return { ok: false, error: 'AudioWorklet unavailable' };
  if (!(await _ensureAudioProbeWorklet(c))) return { ok: false, error: 'probe worklet failed to load' };
  if (audioProbe) await window.stopAudioProbe();
  const st = createAudioProbeState(c, opts);
  if (opts.master !== false) st.tap('__master__', ensureMasterOut() || null);
  if (opts.channels !== false) {
    for (const [id, chain] of channelNodes) {
      if (id !== 'main') st.tap(id, chain.gain);
    }
  }
  // Main-thread stall sampler: the UI side of the story. A gap here is time
  // the Go scheduler could not run; the probe shows whether audio suffered.
  st.loopTimer = setInterval(() => {
    const t = performance.now();
    const gap = t - st.loopLast;
    st.loopLast = t;
    if (gap > st.loopMaxMs) st.loopMaxMs = gap;
    if (gap > 25 && st.loopGaps.length < AUDIO_PROBE_LOG_MAX) {
      st.loopGaps.push({ ctx: hasCtx() ? ctx.currentTime : 0, ms: gap });
    }
  }, 8);
  audioProbe = st;
  return { ok: true, channels: Array.from(st.taps.keys()) };
};

function audioProbeRawOf(st) {
  return {
    sampleRate: st.sampleRate,
    baseLatency: st.baseLatency,
    outputLatency: st.outputLatency,
    ctxAtStart: st.ctxAtStart,
    ctxAtEnd: hasCtx() ? ctx.currentTime : null,
    wallMs: performance.now() - st.perfAtStart,
    requests: st.requests.slice(),
    starts: st.starts.slice(),
    cancels: st.cancels.slice(),
    reports: Object.fromEntries(Object.entries(st.reports).map(([k, v]) => [k, v.slice()])),
    loopGaps: st.loopGaps.slice(),
    loopMaxMs: st.loopMaxMs,
    truncated: st.truncated,
  };
}

// audioProbeRaw() → the raw log so far (probe keeps running).
window.audioProbeRaw = () => (audioProbe ? audioProbeRawOf(audioProbe) : null);

// stopAudioProbe() → raw log. Flushes each worklet's partial report first.
window.stopAudioProbe = async () => {
  const st = audioProbe;
  if (!st) return null;
  audioProbe = null;
  clearInterval(st.loopTimer);
  const done = [];
  for (const [channelId, tap] of st.taps) {
    done.push(new Promise((resolve) => {
      const list = st.reports[channelId];
      const t = setTimeout(resolve, 500);
      tap.node.port.onmessage = (e) => {
        const m = e.data;
        if (m && m.type === 'report' && list.length < AUDIO_PROBE_LOG_MAX) list.push(m);
        if (m && m.type === 'done') { clearTimeout(t); resolve(); }
      };
      try { tap.node.port.postMessage({ type: 'stop' }); } catch (_) { resolve(); }
    }).then(() => {
      try { tap.source.disconnect(tap.node); } catch (_) {}
    }));
  }
  await Promise.all(done);
  return audioProbeRawOf(st);
};

// audioProbeReport(opts) → analysed report of the log so far (keeps running).
window.audioProbeReport = async (opts = {}) => {
  const raw = audioProbe ? audioProbeRawOf(audioProbe) : null;
  if (!raw) return null;
  const { analyzeAudioProbe } = await import('./audio_probe_analysis.js');
  return analyzeAudioProbe(raw, opts);
};

const AUDIO_QUEUE_MAX = 8192;
const AUDIO_FLUSH_CHUNK = 64;
const AUDIO_FLUSH_BUDGET_MS = 1.2;
// Budget for the SYNCHRONOUS flush of a Go sequencer batch (see
// enqueueAudioEvents): none. Every event in a sequencer batch (≤ AudioBatchMax
// = 32) is due within the lookahead, and deferring any of them hands it back
// to a timer that cannot fire until the current frame ends — measured: the
// 4 ms budget first tried here still left 2/148 notes waiting 196 ms. A cache
// hit costs microseconds; a miss only kicks off an async worker render.
const AUDIO_SYNC_FLUSH_BUDGET_MS = Infinity;
const AUDIO_MIN_LEAD_SEC = 0.004;

const audioQueue = [];
let audioQueueHead = 0;
let audioFlushScheduled = false;
let audioFlushInFlight = false;

// Most-recent absolute ctx time a note was scheduled to start. Used by
// isPlaybackActive() to decide whether a voice render may block the main thread
// (it may NOT while the user is hearing playback — see ensureRenderedSample).
let lastScheduledPlayWhen = -1;

// isPlaybackActive: a note is scheduled within the near future or the last
// ~0.5s, i.e. the user is currently hearing audio. During playback a slow /
// timed-out worker render must NEVER fall back to a synchronous main-thread
// render (that froze UI + audio for seconds on an instrument switch); the note
// stays on the on-grid neighbor fallback or drops, and the exact render lands
// off-thread later. The sync fallback remains for idle warm-up / offline paths.
function isPlaybackActive() {
  return hasCtx() && ctx.state === 'running' &&
    lastScheduledPlayWhen >= 0 && (lastScheduledPlayWhen - ctx.currentTime) > -0.5;
}

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
  // origWhen: the musically-intended start time carried across defer/stale
  // replays (renderLatencyMetrics + grid-preserving reschedule). allowStale:
  // set by the stale race's fallback replay so processAudioEvent plays the
  // stale record instead of re-entering the race.
  const origWhen = Number.isFinite(ev.origWhen) ? ev.origWhen : NaN;
  const allowStale = ev.allowStale === true;
  // row/abs: sequencer identity threaded from the WASM batch bridge
  // (audio.BatchParam.Row/Abs, see batch_wasm.go). row<0 (the default) marks
  // a fire-and-forget/preview event — never recorded into timingTraces.
  const row = Number.isFinite(ev.row) ? ev.row : -1;
  const abs = Number.isFinite(ev.abs) ? ev.abs : -1;
  // ideal/grid/pk: measurement-only timing identity (see carryTiming). They
  // never influence WHEN a note plays.
  const ideal = Number.isFinite(ev.ideal) && ev.ideal > 0 ? ev.ideal : NaN;
  const grid = Number.isFinite(ev.grid) && ev.grid > 0 ? ev.grid : NaN;
  const pk = Number.isFinite(ev.pk) ? ev.pk : 0;
  // vid > 0: an audio-lane voice (cancellable before it starts); laneGen:
  // the lane generation it was enqueued under (cancelAllLaneFuture bumps it).
  const vid = Number.isFinite(ev.vid) && ev.vid > 0 ? ev.vid : 0;
  const laneGen = Number.isFinite(ev.laneGen) ? ev.laneGen : undefined;
  // laneFresh: this event re-starts a lane voice on its just-landed fresh
  // render (laneSwapFresh) — same vid, same start, no new request.
  const laneFresh = vid > 0 && ev.laneFresh === true;
  return { id, vol, pitch, dur, when, origWhen, allowStale, row, abs, ideal, grid, pk, vid, laneGen, laneFresh };
}

// carryTiming copies an event's measurement identity onto a replay event
// (render defer, stale race, sample load). Without it a deferred note would
// reach BufferSource.start as an anonymous event and the playback probe could
// not attribute its (late) onset to the beat that asked for it.
function carryTiming(from, to) {
  to.ideal = from.ideal;
  to.grid = from.grid;
  to.pk = from.pk;
  to.vid = from.vid;
  to.laneGen = from.laneGen;
  return to;
}

// sync=true (the Go sequencer batch bridges): start the batch's voices NOW,
// inside the Go→JS call, instead of on a microtask. A microtask cannot run
// until the JS stack unwinds — i.e. until Go returns control to the event
// loop, which on WASM is the END of the whole Ebiten frame (Update + Draw +
// every runnable goroutine). Measured 2026-09-30 with the playback probe
// (startup demo, headless): notes reached audio.js with +73..+122 ms of lead
// and then sat in this queue ~190-204 ms behind a slow frame, starting
// 74-134 ms late. That queue wait was the UI frame length, not audio work.
function enqueueAudioEvents(events, sync = false) {
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
    if (audioProbe && ev.row >= 0 && !ev.pk && !Number.isFinite(ev.origWhen)) {
      audioProbe.noteRequested(ev);
    }
    if (ev.vid && ev.laneGen === undefined) ev.laneGen = laneGeneration;
    audioQueue.push(ev);
    added++;
    if (Number.isFinite(ev.when) && ev.when > lastScheduledPlayWhen) {
      lastScheduledPlayWhen = ev.when;
    }
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
  // __beatmoAudioFlushMode = 'microtask' restores the legacy deferred flush
  // (A/B diagnostics in webaudio_audio_isolation_bench; never set in prod).
  const legacyFlush = typeof window !== 'undefined' && window.__beatmoAudioFlushMode === 'microtask';
  if (sync && !legacyFlush && !audioFlushInFlight && hasCtx() && ctx.state === 'running') {
    flushAudioQueue(AUDIO_SYNC_FLUSH_BUDGET_MS);
    return;
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

// queuedStart is an event's requested start; one without a time plays now.
function queuedStart(ev) {
  return Number.isFinite(ev.when) ? ev.when : -Infinity;
}

// orderQueueByStart sorts the pending events by requested start (stable:
// simultaneous events keep their arrival order). Already-ordered queues — the
// common case — cost one pass.
function orderQueueByStart() {
  let prev = -Infinity;
  let ordered = true;
  for (let i = audioQueueHead; i < audioQueue.length; i++) {
    const ev = audioQueue[i];
    if (!ev) continue;
    const w = queuedStart(ev);
    if (w < prev) {
      ordered = false;
      break;
    }
    prev = w;
  }
  if (ordered) return;
  const pending = [];
  for (let i = audioQueueHead; i < audioQueue.length; i++) {
    if (audioQueue[i]) pending.push(audioQueue[i]);
  }
  pending.sort((a, b) => queuedStart(a) - queuedStart(b));
  audioQueue.length = 0;
  audioQueueHead = 0;
  for (let i = 0; i < pending.length; i++) audioQueue.push(pending[i]);
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

function flushAudioQueue(budgetMs = AUDIO_FLUSH_BUDGET_MS) {
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
    // Soonest start first: a lane flush hands over a horizon of replacements
    // and new notes at once, and a note due in 10 ms must not wait behind
    // ones due in a second (knob drags: near-due replacements started
    // 0.1-16 ms late behind 80-160 far ones).
    orderQueueByStart();
    const hasPerf = typeof performance !== 'undefined' && typeof performance.now === 'function';
    const start = hasPerf ? performance.now() : 0;
    const deadline = hasPerf ? start + budgetMs : 0;
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
  if (ev.vid) {
    // A lane voice cancelled while queued or awaiting a render (tombstone),
    // or committed before a cancelAllLaneFuture (stale generation): drop.
    if (laneTombstones.has(ev.vid) || ev.laneGen !== laneGeneration) {
      laneTombstones.delete(ev.vid);
      if (audioProbe && ev.pk) audioProbe.noteCancelled(ev.pk);
      return;
    }
  }
  const vol = Number.isFinite(ev.vol) ? ev.vol : 1.0;
  const pitch = Number.isFinite(ev.pitch) ? ev.pitch : 0.0;
  const dur = Number.isFinite(ev.dur) && ev.dur > 0 ? ev.dur : 1.0;
  // Sequencer identity (row>=0), threaded from the WASM batch bridge so it
  // survives defer/stale-race replays — see sanitizeAudioEvent + timingTraces.
  const row = Number.isFinite(ev.row) ? ev.row : -1;
  const abs = Number.isFinite(ev.abs) ? ev.abs : -1;
  // A note is desktop's played voice (engine_play.go playedVoice): a melodic
  // voice renders AT its exact pitch and a Duration below 1 GATES it short
  // (dividing by dur, as for drums, played a duration-0.5 note an octave
  // high); every other voice is pitched and sized by resampling at
  // 2^(pitch/12) / dur. played_voice.js builds those samples; WebAudio only
  // plays them at rate 1.
  const melodic = isMelodicInstrument(id);
  const renderPitch = melodic ? roundPitchForCache(pitch) : 0;
  const rate = melodic ? 1 : Math.pow(2, pitch / 12) / dur;
  const gate = melodic && dur < 1 ? dur : 1;
  const cacheKey = melodic ? cacheKeyFor(id, renderPitch) : id;
  // The C stages (effects, EQ, soft clip) must be on the audio thread before a
  // note reaches them: only the first moments of a context wait here.
  if (dspHostState(ctx) === 'loading') {
    dspReadyFor(ctx).then(() => enqueueAudioEvents([ev]));
    return;
  }
  const now = ctx.currentTime || 0;
  let when = Number.isFinite(ev.when) ? ev.when : NaN;
  // The caller's requested start, BEFORE the min-lead floor below. Schedule
  // metrics observe THIS: observing the clamped value made every late note
  // read as lead=+4ms, so `overdue` could never fire in the browser.
  const requestedWhen = when;
  // The min-lead floor is for real time; an offline render (the stage-test
  // seam) has no clock to beat, so a note at frame 0 stays at frame 0.
  const minWhen = offlineRenderActive ? 0 : now + AUDIO_MIN_LEAD_SEC;
  if (!Number.isFinite(when)) {
    when = minWhen;
  } else if (when < minWhen) {
    when = minWhen;
  }
  const render = renderCache.get(cacheKey);
  if (render) touchRender(cacheKey, render);
  if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch && window.__synthEvtTrace) {
    console.log('[SYNTH-EVT]', { id, cacheKey, cached: !!render, stale: renderRecordStale(id, render), hasRENDER: !!RENDER[id], vol, pitch, dur, rate, when, now, block: instrumentBlocks.has(id) });
  }
  // The FIRST musically-intended `when`, carried across defers/replays so a
  // note never loses its grid position (and renderLatencyMetrics can report
  // the deviation when it does slip).
  const intendedWhen = Number.isFinite(ev.origWhen) ? ev.origWhen
    : (Number.isFinite(requestedWhen) ? requestedWhen : when);
  const timing = { ideal: ev.ideal, grid: ev.grid, pk: ev.pk, vid: ev.vid, laneGen: ev.laneGen };
  let laneStale = false;
  if (render && renderRecordStale(id, render) && ev.allowStale !== true) {
    // P0 — stale-while-revalidate: a live edit bumped the param generation.
    // Race the fresh render against this note's deadline: play the fresh
    // buffer at the intended `when` if it lands in time, else play the stale
    // buffer ON TIME. Timing is never sacrificed; timbre lags at most one note.
    // Events without a caller `when` (preview taps) aren't grid-locked and
    // instead wait a short freshness budget for the just-edited timbre.
    const gridLocked = Number.isFinite(ev.when);
    if (!(ev.vid && gridLocked)) {
      raceFreshRenderAgainstDeadline(ctx, carryTiming(timing, { id, vol, pitch, dur, row, abs }), melodic ? pitch : undefined, intendedWhen, gridLocked);
      return;
    }
    // A LANE voice never waits on the race: its replay is a main-thread
    // timer / promise, so a stalled main thread started the note whenever it
    // came back (an edit storm bunched 4 notes 160–415 ms late into one
    // burst). Schedule the stale buffer NOW — the audio thread owns the
    // timing — and swap to the fresh render if it lands before the start.
    laneStale = true;
  }
  if (!render && RENDER[id]) {
    if (melodic) {
      // P3 — cold-miss neighbor fallback: this exact pitch was never rendered,
      // but another pitch of the instrument is cached. Play the nearest one
      // resampled AT THE INTENDED TIME (wrong formant for one note beats a
      // note off the grid) and background-render the exact pitch for next hit.
      const near = nearestCachedPitch(id, renderPitch);
      const nearRec = near && renderCache.get(near.key);
      const nearPlayed = nearRec && playedBufferFor(nearRec, Math.pow(2, (pitch - near.pitch) / 12), gate);
      if (nearPlayed) {
        startRenderVoice(ctx, id, vol, nearPlayed, when, now, nearRec.data, row, abs, intendedWhen, timing, 'neighbor');
        renderLatencyMetrics.neighborPlays += 1;
        if (Number.isFinite(ev.origWhen)) {
          renderLatencyMetrics.observeDefer((when - ev.origWhen) * 1000);
        }
        ensureRenderReady(id, renderPitch).catch(() => {});
        return;
      }
    }
    // P1 — true first-render defer: replay at the ORIGINAL intended `when`
    // (floored to now+minLead on arrival), not at a fresh "now". If the render
    // beats the deadline the note lands exactly on grid; if not it plays as
    // soon as possible — late by the render overshoot only, never early.
    ensureRenderReady(id, melodic ? renderPitch : undefined).then(() => {
      enqueueAudioEvents([carryTiming(timing, { id, vol, pitch, dur, when: intendedWhen, origWhen: intendedWhen, row, abs })]);
    }).catch((err) => dbg('audio.render.defer.error', { id, err: String(err) }));
    return;
  }
  if (render) {
    const played = playedBufferFor(render, rate, gate);
    if (!played) {
      dbg('audio.render.nobuf', { id });
      return;
    }
    if (ev.allowStale === true && renderRecordStale(id, render)) {
      renderLatencyMetrics.stalePlays += 1;
    }
    if (Number.isFinite(ev.origWhen)) {
      // This event was deferred by a render-cache miss or stale race: record
      // the signed deviation from its musically-intended start (hidden from
      // the schedule metrics, which only see the rewritten `when`).
      renderLatencyMetrics.observeDefer((when - ev.origWhen) * 1000);
    }
    startRenderVoice(ctx, id, vol, played, when, now, render.data, row, abs, intendedWhen, timing,
      laneStale ? 'lane-stale' : ev.laneFresh ? 'lane-fresh'
        : Number.isFinite(ev.origWhen) ? (ev.allowStale === true ? 'stale' : 'deferred') : 'hit');
    if (laneStale) {
      const vid = ev.vid;
      const fresh = carryTiming(timing, { id, vol, pitch, dur, row, abs, laneFresh: true });
      ensureRenderReady(id, melodic ? renderPitch : undefined)
        .then(() => laneSwapFresh(vid, fresh, cacheKey))
        .catch(() => {}); // render failed: the stale voice simply plays
    }
    return;
  }
  dbg('audio.unknown', { id });
}

// voiceLevel is a note's gain: the per-voice headroom × its volume (row ×
// node), exact — desktop's scaledVoice × mixHeadroom. (Notes used to share
// per-volume buses that rounded the volume to 1/16: up to ±1.9 dB per note,
// and anything below 1/32 silent.)
function voiceLevel(vol) {
  return CHAIN_SPEC.voiceHeadroom * Math.max(0, Number.isFinite(vol) ? vol : 1);
}

// frameTime puts a start on the context's sample grid: a note starts on a
// sample on both platforms (WebAudio would otherwise start it between two,
// interpolating every sample of the voice).
function frameTime(c, t) {
  const sr = c.sampleRate || CHAIN_SPEC.sampleRate;
  return Math.round(t * sr) / sr;
}

// playedBufferFor is the AudioBuffer of a render's played voice at (rate,
// gate) — played_voice.js over the render's samples, with desktop's fade-in
// baked in — cached per render record (a newer render is a new record).
const playedCache = new WeakMap(); // render record → Map(variant → AudioBuffer)
const PLAYED_VARIANTS_PER_RENDER = 32;
function playedBufferFor(record, rate, gate) {
  if (!record || !record.data) return null;
  const variant = rate + '|' + gate;
  let byVariant = playedCache.get(record);
  if (!byVariant) {
    byVariant = new Map();
    playedCache.set(record, byVariant);
  }
  let buf = byVariant.get(variant);
  if (buf) {
    byVariant.delete(variant); // keep the most recently played last (LRU)
    byVariant.set(variant, buf);
    return buf;
  }
  const sr = record.sr || CHAIN_SPEC.sampleRate;
  const g = gate < 1 ? gateLength(record.data.length, gate, sr, CHAIN_SPEC.gateFadeMs) : null;
  const data = playedVoiceSamples(record.data, g, rate, Math.trunc((sr * CHAIN_SPEC.fadeInMs) / 1000));
  buf = makeAudioBuffer(data, sr);
  byVariant.set(variant, buf);
  if (byVariant.size > PLAYED_VARIANTS_PER_RENDER) byVariant.delete(byVariant.keys().next().value);
  return buf;
}

function makeAudioBuffer(data, sr) {
  const n = Math.max(1, data.length);
  let b;
  try {
    b = new AudioBuffer({ length: n, sampleRate: sr, numberOfChannels: 1 });
  } catch (_) {
    b = getCtx().createBuffer(1, n, sr);
  }
  if (data.length) b.copyToChannel(data, 0);
  return b;
}

if (typeof window !== 'undefined') {
  // Test seam: the samples a note puts into the mix (before its gain), so the
  // stage test can compare them with desktop's played voice directly.
  window.__playedVoiceForTest = (data, sr, melodic, pitch, dur) => {
    const d = dur > 0 ? dur : 1;
    const g = melodic && d < 1 ? gateLength(data.length, d, sr, CHAIN_SPEC.gateFadeMs) : null;
    return playedVoiceSamples(Float32Array.from(data), g, melodic ? 1 : Math.pow(2, pitch / 12) / d, Math.trunc((sr * CHAIN_SPEC.fadeInMs) / 1000));
  };
}

// startRenderVoice: create + connect + start one BufferSource for a played
// voice (render hit, neighbour fallback, lane swap). row/abs/idealWhen
// (optional, row defaults -1/not-recorded) thread sequencer identity into
// timingTraces — see processAudioEvent callers.
let voiceStartCount = 0; // voices started (the offline render seam checks none deferred)
let offlineRenderActive = false; // __offlineRenderForTest is scheduling

function startRenderVoice(ctx, id, vol, played, when, now, recData, row = -1, abs = -1, idealWhen, timing, path) {
  voiceStartCount++;
  const src = ctx.createBufferSource();
  trackSource(id, src);
  src.buffer = played;
  const level = voiceLevel(vol);
  const gain = ctx.createGain();
  gain.gain.value = level;
  src.connect(gain);
  const sink = channelSink(id);
  if (sink) gain.connect(sink);
  src._antiPopGain = gain; // stash for stopSound / stopLaneAt fade-out
  src._level = level;
  const at = frameTime(ctx, when);
  const swap = path === 'lane-fresh'; // the same note, already observed when it was scheduled stale
  if (!swap) observeScheduleTiming(ctx, Number.isFinite(idealWhen) ? idealWhen : at, now);
  try { if (recData && !swap) recordSamples(recData, vol); } catch (_) {}
  src.start(at);
  if (timing) laneRegister(timing.vid, src, gain, id, at, timing.pk, level);
  if (row >= 0) {
    if (!swap) timingTraces.push(row, abs, Number.isFinite(idealWhen) ? idealWhen : at, at, ctx.currentTime || 0);
    if (audioProbe && timing && timing.pk) audioProbe.noteStarted(timing.pk, id, at, ctx.currentTime || 0, path || 'hit');
  }
}

// How long before the intended `when` the stale race stops waiting for the
// fresh render and falls back to the stale buffer. Covers the re-enqueue +
// microtask flush + lazy AudioBuffer wrap (~1–3ms) with headroom.
const FRESH_DEADLINE_MARGIN_SEC = 0.012;
// Deadline for events with NO caller-supplied `when` (preview taps, immediate
// plays): they aren't grid-locked, so freshness beats the nominal now+minLead
// start — wait up to this long for the just-edited timbre before falling back
// to the stale buffer. Sized to comfortably cover one voice render (~25–40ms).
const FRESH_NOWAIT_BUDGET_SEC = 0.08;

// raceFreshRenderAgainstDeadline (P0): a stale-cache note either plays the
// FRESH render at its intended `when` (render resolved in time) or the STALE
// buffer at the same intended `when` (deadline hit first). Exactly one replay
// fires; the loser of the race no-ops. A failed render also falls back to the
// stale buffer, so an edit can never silence a note.
//
// The replay always carries allowStale:true — it means "play the best buffer
// available NOW, do not race again". Without it a fresh replay that finds the
// record stale AGAIN (edits still streaming in) would re-enter the race and
// drift later on every cycle; one race per note bounds the slip to one
// deadline. gridLocked distinguishes sequencer-scheduled events (deadline =
// the intended `when`; the grid rules) from immediate plays (deadline = a
// freshness budget; the newest timbre rules).
function raceFreshRenderAgainstDeadline(ctx, ev, pitchForRender, intendedWhen, gridLocked) {
  let decided = false;
  const replay = () => {
    if (decided) return;
    decided = true;
    enqueueAudioEvents([carryTiming(ev, { id: ev.id, vol: ev.vol, pitch: ev.pitch, dur: ev.dur, when: intendedWhen, origWhen: intendedWhen, allowStale: true, row: ev.row, abs: ev.abs })]);
  };
  ensureRenderReady(ev.id, pitchForRender).then(replay).catch(replay);
  const now = ctx.currentTime || 0;
  const deadlineSec = gridLocked
    ? (intendedWhen - now - FRESH_DEADLINE_MARGIN_SEC)
    : FRESH_NOWAIT_BUDGET_SEC;
  setTimeout(replay, Math.max(0, deadlineSec * 1000));
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

// getSampleRate is the rate every voice renders at: the engine's (desktop's
// too), whatever the device runs at.
function getSampleRate() {
  return CHAIN_SPEC.sampleRate;
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
  ctx = newEngineContext();
  preloadOutputTap(ctx);
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
  // Load the C stages (effects, EQ, soft clip) onto the audio thread up front;
  // chains built before they land are rebuilt onto them (onDSPHostReady).
  ensureDSPHost(ctx);
  return ctx;
}

// newEngineContext: an AudioContext at the engine's rate (CHAIN_SPEC.sampleRate,
// desktop's rate too). An engine that refuses the rate gets its own; voices
// still render at the engine rate and WebAudio converts them.
function newEngineContext() {
  const AC = window.AudioContext || window.webkitAudioContext;
  try {
    return new AC({ sampleRate: CHAIN_SPEC.sampleRate });
  } catch (err) {
    console.warn('[AUDIOJS] AudioContext refused', CHAIN_SPEC.sampleRate, 'Hz — using the device rate:', err);
    return new AC();
  }
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

// flushPendingChannelOps forces the AudioContext into existence and drains
// pendingChannelOps (channel volume/pan/EQ/insert-FX node construction
// deferred while no context existed). Called from Go's
// audio.EnsureAudioGraphReady (internal/audio/audio_graph_ready_js.go), which
// the playback readiness gate invokes at import time (see import.go) so a
// circuit imported BEFORE the audio-unlock gesture builds its whole
// channel/FX graph now — while the progress toast is up and Play is already
// disabled, so a stall here is invisible — instead of bursting at the very
// first triggered note. Safe to call when the queue is already drained
// (applyPendingOps is idempotent) or when ctx is still suspended: creating
// and wiring nodes does not require a 'running' context, only producing
// audible output does.
window.flushPendingChannelOps = () => {
  getCtx();
  applyPendingOps();
  ensureMainNode();
};

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
// checkEngineABI: the loaded C engine must have been compiled with the param
// layout this file marshals (synth_param_abi.gen.js). A glue built from an
// older modular_params.gen.h reads every later field at a shifted offset —
// wrong voices, no error — so refuse it loudly.
let engineABIChecked = false;
function checkEngineABI(m) {
  if (engineABIChecked) return;
  const n = (m && typeof m._modular_param_count === 'function') ? m._modular_param_count() : -1;
  if (typeof window !== 'undefined') window.__synthEngineParamCount = n;
  if (n !== MODULAR_PARAM_COUNT) {
    if (typeof window !== 'undefined') window.__synthABIStale = true;
    const msg = '[AUDIOJS] STALE drums.single.js — the C engine was compiled with ' + n +
      ' params, synth_param_abi.gen.js marshals ' + MODULAR_PARAM_COUNT + '. Run `rm -f src/js/drums.single.js && make wasm`.';
    try { console.error(msg); } catch (_) {}
    throw new Error(msg);
  }
  engineABIChecked = true;
}

async function ensureModule() {
  mod = await modulePromise;
  checkEngineABI(mod);
  try { dbg('dsp.exports', { hasCcall: !!mod.ccall, hasMalloc: !!mod._malloc, hasHeap: !!mod.HEAPF32 }); } catch (e) { dbg('dsp.exports.error', { err: String(e) }); }
  return mod;
}

// ───────── Off-main-thread synth rendering (WASM perf fix) ─────────
// The C voice renderer (render_modular_p) is synchronous and
// cost ~30–40 ms for a 2 s melodic / physical-model voice. Running them via
// ccall on the main thread blocked the UI + audio scheduler whenever a live
// param edit invalidated the render cache mid-playback (the user-reported
// "lag / jitter / audio breaks" — see webaudio_synth_param_stress.browser
// .test.js). synth_render_worker.js hosts a SECOND instance of the SINGLE_FILE
// DSP module and renders off-thread; the main thread builds the flat param
// block (the param-ABI lives in one place) and awaits a transferred buffer.
//
// The renderers read ALL input through args (sr / frames / param block) — no
// JS-side global state — so the worker's module produces identical output. If
// the worker is unavailable (no Worker support, init failure, hung render) the
// code falls back to the original synchronous main-thread ccall, so behaviour
// degrades to "pre-fix" rather than breaking.
// Render-worker POOL. A single worker serialized every cold render, so a burst
// of distinct cold pitches (a row just switched to a melodic instrument, or a
// pre-warm of its pitch set) stacked round-trips (organ ~420ms each in WASM →
// multi-second warm; a measured 91-pair import-warm circuit took 7.79s at
// ~108ms/pair, 2026-09-13). Requests are independent pure renders, so a small
// pool renders the burst in parallel; each render is dispatched to the
// least-loaded worker (per-item cost varies widely — ~2ms for a short drum
// vs. ~40ms for a bowed string — so a state-blind round-robin can stack
// several expensive items on one worker while a sibling sits idle; the
// least-loaded pick routes around that skew). Pool size is capped low to
// bound memory (each worker hosts its own SINGLE_FILE wasm module) and to
// leave a core free for the main thread + audio thread. Output is
// worker-independent, so parity (xplat_audio_compare) is unaffected — the
// pool changes only WHICH worker renders a request, never what it computes.
// Lazily created on first use (ensureSynthRenderWorkers()), NOT at module
// load: a worker that is never used should never be paid for, and several
// tests import this module without ever rendering.
const RENDER_WORKER_MAX = 4;
function renderWorkerCount() {
  try {
    // Test/bench-only override (mirrors window.__beatmoProfileOverride) so a
    // sweep can compare pool sizes without re-launching with a different
    // navigator.hardwareConcurrency. Never set in production.
    if (typeof window !== 'undefined' && Number.isFinite(window.__beatmoRenderWorkerCountOverride)) {
      return Math.max(1, window.__beatmoRenderWorkerCountOverride | 0);
    }
    const hc = (typeof navigator !== 'undefined' && navigator.hardwareConcurrency) || 2;
    return Math.max(1, Math.min(RENDER_WORKER_MAX, hc - 1));
  } catch (_) { return 1; }
}
let synthWorkerPool = null;       // array of { worker, inflight }
let synthWorkerDisabled = false;
let synthWorkerReqSeq = 0;
const synthWorkerPending = new Map(); // reqId -> { resolve, reject, t, entry }
const SYNTH_WORKER_RENDER_TIMEOUT_MS = 4000;

function createRenderWorker() {
  const w = new Worker(new URL('./synth_render_worker.js', import.meta.url), { type: 'module' });
  const entry = { worker: w, inflight: 0 };
  w.onmessage = (ev) => {
    const msg = ev.data || {};
    if (msg.type === 'rendered') {
      const p = synthWorkerPending.get(msg.reqId);
      if (p) { synthWorkerPending.delete(msg.reqId); if (p.t) clearTimeout(p.t); if (p.entry) p.entry.inflight--; p.resolve(msg.data); }
    } else if (msg.type === 'error') {
      const p = synthWorkerPending.get(msg.reqId);
      if (p) { synthWorkerPending.delete(msg.reqId); if (p.t) clearTimeout(p.t); if (p.entry) p.entry.inflight--; p.reject(new Error(msg.err || 'worker render error')); }
    } else if (msg.type === 'ready') {
      dbg('synth.worker.ready');
    } else if (msg.type === 'initerror') {
      synthWorkerDisabled = true;
      dbg('synth.worker.initerror', { err: msg.err });
      rejectAllSynthWorkerPending(new Error('worker init failed: ' + msg.err));
    }
  };
  w.onerror = (e) => {
    synthWorkerDisabled = true;
    dbg('synth.worker.error', { err: String(e && e.message ? e.message : e) });
    rejectAllSynthWorkerPending(new Error('worker error: ' + String(e && e.message ? e.message : e)));
  };
  return entry;
}

// ensureSynthRenderWorkers lazily creates the pool on first call (never at
// module load) and memoizes it — a page that never renders (several browser
// tests just import this module) never pays for a single Worker.
function ensureSynthRenderWorkers() {
  if (synthWorkerPool) return synthWorkerPool;
  if (synthWorkerDisabled) return null;
  if (typeof Worker === 'undefined') { synthWorkerDisabled = true; return null; }
  try {
    const pool = [];
    for (let i = 0; i < renderWorkerCount(); i++) pool.push(createRenderWorker());
    synthWorkerPool = pool;
    return pool;
  } catch (e) {
    synthWorkerDisabled = true;
    dbg('synth.worker.create.error', { err: String(e) });
    return null;
  }
}

// pickSynthWorker returns the least-loaded pool entry so a burst of renders
// (import warm-up, a row switching to a melodic instrument) spreads across
// every worker instead of piling on one, and — because per-item render cost
// varies widely (a short drum vs. a bowed string) — routes around a worker
// that is mid-way through an expensive item rather than queuing behind it.
// Requests are pure/stateless renders, so which worker serves a given
// request never affects the result.
function pickSynthWorker(pool) {
  let best = pool[0];
  for (let i = 1; i < pool.length; i++) {
    if (pool[i].inflight < best.inflight) best = pool[i];
  }
  return best;
}

function rejectAllSynthWorkerPending(err) {
  for (const [, p] of synthWorkerPending) { if (p.t) clearTimeout(p.t); if (p.entry) p.entry.inflight--; try { p.reject(err); } catch (_) {} }
  synthWorkerPending.clear();
}

// renderVoiceInWorker posts a render request to the least-loaded pool worker
// and resolves with the rendered Float32Array (length === frames). Rejects if
// no worker is available, on error, or on timeout — callers handle the
// reject (during playback they do NOT sync-render; see ensureRenderedSample).
// The param block is structured-cloned (NOT transferred) so the caller keeps
// it for the synchronous fallback path.
// synthWorkerPending stays ONE map keyed by request id across the whole pool
// — synthWorkerPending.size is read as the workerQueuePeak metric
// (webaudio_import_cold_bench.browser.test.js), so it must keep meaning
// "total requests in flight across the pool", not per-worker.
function renderVoiceInWorker(renderFn, sr, frames, paramArr, allocCount, post) {
  const pool = ensureSynthRenderWorkers();
  if (!pool) return Promise.reject(new Error('synth render worker unavailable'));
  const entry = pickSynthWorker(pool);
  return new Promise((resolve, reject) => {
    const reqId = ++synthWorkerReqSeq;
    const t = setTimeout(() => {
      const p = synthWorkerPending.get(reqId);
      if (p) {
        synthWorkerPending.delete(reqId);
        if (p.entry) p.entry.inflight--;
        reject(new Error('synth render worker timeout'));
      }
    }, SYNTH_WORKER_RENDER_TIMEOUT_MS);
    synthWorkerPending.set(reqId, { resolve, reject, t, entry });
    entry.inflight++;
    try {
      // post: when set, the worker also applies the voice post-processing
      // (voice_post.js — the same function as the main-thread fallback, so
      // results stay bit-identical), keeping that O(frames) work off main.
      entry.worker.postMessage({ type: 'render', reqId, renderFn, sr, frames, allocCount: allocCount || 0, params: paramArr || null, post: post || null });
    } catch (e) {
      synthWorkerPending.delete(reqId); clearTimeout(t); entry.inflight--; reject(e);
    }
  });
}

// renderVoiceSync is the original synchronous main-thread render, used as the
// fallback when the worker is unavailable and to preserve the parameterized →
// native resilience path. Returns a freshly-copied Float32Array (length
// frames), post-processed like the worker's when post is set.
function renderVoiceSync(m, renderFn, sr, frames, paramArr, allocCount, post) {
  const ptr = m._malloc(frames * 4);
  if (!ptr) throw new Error('Failed to allocate memory for audio buffer.');
  let paramsPtr = 0;
  try {
    // Every render passes a full param block: the engine has no defaults.
    if (!paramArr) throw new Error('renderVoiceSync: no param block for ' + renderFn);
    const count = Math.max(paramArr.length, allocCount || 0);
    // The modular param block is double; emscripten malloc is 8-byte aligned,
    // so >> 3 is exact.
    if (!m.HEAPF64) throw new Error('drums module has no HEAPF64 — the C glue (drums.single.js) is older than the double param block; run `make wasm`');
    paramsPtr = m._malloc(count * 8);
    if (!paramsPtr) throw new Error('Failed to allocate memory for param block.');
    const pheap = m.HEAPF64.subarray(paramsPtr >> 3, (paramsPtr >> 3) + count);
    pheap.fill(0);
    pheap.set(paramArr);
    m.ccall(renderFn, null, ['number', 'number', 'number', 'number'], [ptr, sr, frames, paramsPtr]);
    const heap = m.HEAPF32.subarray(ptr >> 2, (ptr >> 2) + frames);
    const data = new Float32Array(frames);
    data.set(heap);
    return applyVoicePost(m, data, sr, post || null);
  } finally {
    m._free(ptr);
    if (paramsPtr) m._free(paramsPtr);
  }
}

// NOTE: the render-worker pool used to be pre-warmed here at module load so
// the first live render was already off-main-thread. That cost every page
// load a pool of Workers even when nothing was ever rendered (several browser
// tests import this module without rendering). The pool is now created
// lazily on first use inside ensureSynthRenderWorkers() (called from
// renderVoiceInWorker) — the first render pays the one-time Worker spin-up,
// but that is async off-thread already and never blocks the main thread.

// RENDER / RENDER_INFO / MELODIC_INSTRUMENTS are filled by Go: every synth
// instrument is an instrument DOCUMENT, flattened in Go and pushed here with
// its render metadata (window.setInstrumentBlock below; spec 2026-10-04 §8).
// audio.js keeps no parameter maps and no instrument table of its own.
const RENDER = {};
const RENDER_INFO = {};

// ───────── Per-pitch melodic re-render (desktop↔WASM parity) ─────────
// Desktop melodic instruments RE-RENDER the C voice at the played semitone
// pitch (synth_recipe_dispatch.go: newRecipeAwareVoicePitched →
// tryRecipeVoicePitched) so the filter formant stays at an absolute Hz and
// does not slide ("munchkin") with pitch. WebAudio used to resample one base
// (pitch-0) buffer via BufferSource.playbackRate, sliding the formant. For the
// instruments below we instead render the buffer AT the rounded pitch (via the
// modular param block's `pitch` field, MODULAR_PARAM_INDEX.pitch=25 → the C
// voice's freq=220·2^(pitch/12)) and play at ~1.0 rate. The cache is keyed by
// `${id}:${roundedPitch}` so distinct pitches coexist; drums / FM / base
// modular are UNTOUCHED (bare-id key, full playbackRate resample, byte-identical
// to before).
//
// The set is each pushed document's render.melodic flag (setInstrumentBlock).
const MELODIC_INSTRUMENTS = new Set();

function isMelodicInstrument(id) {
  return MELODIC_INSTRUMENTS.has(id);
}

// Test seam: xplat_melodic_formant.browser.test.js asserts the JS melodic set
// matches the desktop melodicRecipeIDs. Underscored + window-scoped so linters /
// tree-shaking skip it; production code uses isMelodicInstrument directly.
export const __isMelodicInstrument = isMelodicInstrument;

// roundPitchForCache is the pitch a melodic voice renders at: the note's EXACT
// pitch, as desktop renders it (doc_voice_native.go writes the exact pitch into
// the block). It used to round to the nearest half semitone and bend the rest
// with playbackRate, which no platform-neutral render matches. Only NaN and
// -0 are normalised, so equal pitches share one cache key.
function roundPitchForCache(pitch) {
  const p = Number.isFinite(pitch) ? pitch : 0;
  return p === 0 ? 0 : p;
}

// cacheKeyFor returns the renderCache key for (id, rounded pitch). For melodic
// instruments the rounded pitch is folded into the key so each played pitch
// gets its own at-pitch render. Non-melodic instruments (and melodic at
// pitch 0) keep the bare-id key so all existing call sites — sampler capture,
// param-edit invalidation, registerSamplePCM — are byte-for-byte unchanged.
function cacheKeyFor(id, roundedPitch) {
  if (isMelodicInstrument(id) && roundedPitch !== 0) {
    return id + ':' + roundedPitch;
  }
  return id;
}

// evictRenderCache drops the bare-id render AND every per-pitch melodic variant
// (`${id}:<pitch>`) for id, so a param / recipe / sample-edit change re-renders
// all played pitches with the new values. Non-melodic ids only ever hold the
// bare key, so this is a single delete for them.
function evictRenderCache(id) {
  renderCache.delete(id);
  if (isMelodicInstrument(id)) {
    const prefix = id + ':';
    for (const key of renderCache.keys()) {
      if (key.startsWith(prefix)) renderCache.delete(key);
    }
  }
}

// Test-only: force an instrument fully cold (drop every rendered pitch variant
// AND the lazily-wrapped AudioBuffers) so a bench can measure the cold-start
// cost of switching a row to a never-rendered instrument. Mirrors the
// window.__test* affordances above; production code never reads it.
if (typeof window !== 'undefined') {
  window.__evictRenderCacheForTest = (id) => {
    evictRenderCache(id);
    rawRenderCache.delete(id);
  };
}

// ───────── Stale-while-revalidate param generations (WASM real-time fix) ─────────
// A live param / sample-edit push used to evictRenderCache(id) — leaving NOTHING
// playable, so every subsequent trigger deferred behind a ~30–40ms async render
// and played off the musical grid (measured 13–56ms early on fast machines, up
// to +104ms late throttled — see webaudio_render_latency_bench.browser.test.js).
// Instead, edits now bump a per-instrument GENERATION; cached records carry the
// generation they were rendered at. A record whose generation lags is STALE:
// still playable ON TIME (grid beats timbre freshness by one note), while a
// background re-render replaces it. processAudioEvent races the fresh render
// against the note's deadline and plays whichever is ready (fresh preferred).
// In-flight render dedup (pendingRenderEnsures) + generation equality is the
// storm coalescing: at most one render per (id,pitch) is in flight no matter
// how fast the knob drags.
const instrumentParamsGen = new Map(); // id -> monotonically increasing edit counter
function bumpInstrumentParamsGen(id) {
  instrumentParamsGen.set(id, (instrumentParamsGen.get(id) || 0) + 1);
}
function paramsGenOf(id) {
  return instrumentParamsGen.get(id) || 0;
}
// renderRecordStale: true when id is a re-renderable synth (RENDER[id]) and the
// cached record was rendered under an older param generation. Sample-PCM
// records (registerSamplePCM deletes RENDER[id]) are never stale.
function renderRecordStale(id, record) {
  return !!(record && RENDER[id] && (record.gen || 0) !== paramsGenOf(id));
}

// nearestCachedPitch: cold-miss fallback for melodic instruments — find the
// cached render (bare id = pitch 0, or any id:<pitch> variant) closest to the
// requested rounded pitch so the note can play resampled AT ITS INTENDED TIME
// while the exact-pitch render happens in the background. Wrong formant for
// one note beats a note off the grid.
function nearestCachedPitch(id, roundedPitch) {
  let bestKey = null;
  let bestPitch = 0;
  let bestDist = Infinity;
  const bare = renderCache.get(id);
  if (bare && bare.data) {
    bestKey = id;
    bestPitch = 0;
    bestDist = Math.abs(roundedPitch);
  }
  const prefix = id + ':';
  for (const key of renderCache.keys()) {
    if (!key.startsWith(prefix)) continue;
    const rec = renderCache.get(key);
    if (!rec || !rec.data) continue;
    const p = parseFloat(key.slice(prefix.length));
    if (!Number.isFinite(p)) continue;
    const d = Math.abs(p - roundedPitch);
    if (d < bestDist) {
      bestDist = d;
      bestKey = key;
      bestPitch = p;
    }
  }
  return bestKey === null ? null : { key: bestKey, pitch: bestPitch };
}

// renderCache holds the pre-rendered Float32Array + metadata for every
// synthesized instrument. Exported so browser tests can probe re-render
// after a setInstrumentParam mutation without re-implementing the
// module's render bookkeeping. For melodic instruments at a non-zero pitch the
// key is `${id}:${roundedPitch}` (see cacheKeyFor); every other entry is keyed
// by the bare instrument id exactly as before.
export const renderCache = new Map();
const pendingRenderEnsures = new Map();
const channelNodes = new Map(); // id -> { ingress, gain, analysers, fxChain, fxNode, eqProgram, eqNode }
let mainChannelNode = null;

// clampVolume guards a channel gain against garbage; the master range itself
// is clamped once, in Go (audio.SetMainVolume, CHAIN_SPEC.maxMasterVolume).
function clampVolume(vol) {
  if (!Number.isFinite(vol)) return 1;
  if (vol < 0) return 0;
  return vol;
}

function ensureMainNode() {
  if (!mainChannelNode) {
    // Don't force context creation — if no context exists yet, defer.
    if (!hasCtx()) return null;
    const c = ctx;
    mainChannelNode = c.createGain();
    mainChannelNode.gain.value = 1;
    mainChannelNode.connect(audioDestination());
    channelNodes.set('main', { ingress: mainChannelNode, gain: mainChannelNode, analyser: null });
  }
  return mainChannelNode;
}

// __testChannelSummary(id) — test seam: what a channel actually does to a voice
// between playSound and the master bus (EQ filters, insert effects, channel
// gain). A capture that disagrees with its voice is explained by this, not by
// guessing: an EQ high-pass or a distortion insert both reshape the waveform
// while leaving the energy envelope intact.
window.__testChannelSummary = (id) => {
  const chain = channelNodes.get(id);
  if (!chain) return null;
  return {
    gain: chain.gain && chain.gain.gain ? chain.gain.gain.value : null,
    eqProgram: chain.eqProgram ? Array.from(chain.eqProgram) : null,
    insertFX: (chain.fxChain || []).filter((e) => e && e.enabled).map((e) => e.type),
  };
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
  const chain = { ingress, gain, analyser: null, fxChain: [], fxNode: null, eqProgram: null, eqNode: null };
  channelNodes.set(id, chain);
  audioProbeTapChannel(id, chain);
  return chain;
}

// getChannelChain returns a channel's chain, building it on first use unless
// create is false (a read: volumes and analyser snapshots never build one).
function getChannelChain(id, create = true) {
  // One name per bus: the master is 'main' and only 'main' (Go's channel.ID
  // never sends ''). An omitted or empty id names no channel and fails
  // closed: mapping it to master made a probe asking for a nonexistent
  // channel read the MASTER bus and report its levels as the channel's —
  // every tap on that chain then agreed with every other, which is exactly
  // what a correct reading looks like.
  if (id === undefined || id === null || id === '') return null;
  const key = id;
  if (key === 'main') {
    const main = ensureMainNode();
    if (!main) return null;
    return channelNodes.get('main');
  }
  let chain = channelNodes.get(key);
  if (!chain && create) {
    chain = buildChannelPipeline(key);
  }
  return chain || null;
}

function channelSink(id) {
  const chain = getChannelChain(id);
  if (!chain) return null;
  if (chain.released) wakeChannel(chain); // a voice brings it back
  return chain.ingress;
}

// ─── Residency (spec 2026-10-31 §1.2) ───
//
// Go declares the instruments the rows play (setHeldChannels). A chain outside
// that set is RELEASED — its beatmo-dsp stages disposed and its output
// disconnected — only once nothing in it can still sound: no voice scheduled
// or playing, and every stage's input and output at or below
// CHAIN_SPEC.releaseQuietDB for the hold plus the chain's memory (a delay's
// echo can come back after that much silence). Its settings (insert chain, EQ
// program, analysers) stay on the chain; the next voice wakes it with fresh
// stages, as a chain built for the first time. Desktop does the same in its
// mixer (residency.go).
let heldChannels = null; // null: no circuit declared a held set — every chain is held

function channelHeld(id) { return !heldChannels || heldChannels.has(id); }

function chainMemorySec(list) {
  let sec = 0;
  for (const e of list || []) if (e && e.enabled) sec += (CHAIN_SPEC.effectMemoryMs[e.type] || 0) / 1000;
  return sec;
}

// considerRelease arms the release of an unheld, voiceless chain; called when
// the held set changes and when an instrument's last voice ends.
function considerRelease(id) {
  const chain = channelNodes.get(id);
  if (!chain || chain.released || id === 'main' || offlineRenderActive || channelHeld(id)) return;
  if (activeSources.get(id)?.size) return;
  const needSec = CHAIN_SPEC.releaseHoldMs / 1000 + chainMemorySec(chain.fxChain);
  const stages = [chain.fxNode, chain.eqNode].filter(Boolean);
  if (!stages.length) {
    // Nothing on the chain holds a sound: the hold covers its last voice's fade.
    clearTimeout(chain._releaseTimer);
    chain._releaseTimer = setTimeout(() => releaseIfStillSilent(id), needSec * 1000);
    return;
  }
  const pending = new Set(stages);
  const watch = { frames: Math.ceil(needSec * ctx.sampleRate), amp: Math.pow(10, CHAIN_SPEC.releaseQuietDB / 20) };
  for (const n of stages) {
    n._onQuiet = () => {
      pending.delete(n);
      if (!pending.size) releaseIfStillSilent(id);
    };
    n._dspPost({ watchQuiet: watch });
  }
}

function releaseIfStillSilent(id) {
  const chain = channelNodes.get(id);
  if (!chain || chain.released || offlineRenderActive || channelHeld(id) || activeSources.get(id)?.size) return;
  for (const n of [chain.fxNode, chain.eqNode]) {
    if (!n) continue;
    try { n.disconnect(); } catch (_) {}
    n._dspDispose();
  }
  chain.fxNode = null;
  chain.eqNode = null;
  // Only the gain's way to the master bus: its other listeners (the playback
  // probe's tap) stay connected across a release.
  try { chain.gain.disconnect(ensureMainNode()); } catch (_) {}
  chain.released = true;
  audioGraphStats.channelsReleased++;
}

function wakeChannel(chain) {
  chain.released = false;
  const main = ensureMainNode();
  if (main) chain.gain.connect(main);
  if (dspHostUp(ctx)) {
    if (Array.isArray(chain.fxChain) && chain.fxChain.length) chain.fxNode = dspNode(ctx, 'inserts', null, [{ chain: chain.fxChain }]);
    if (chain.eqProgram) chain.eqNode = dspNode(ctx, 'eq', null, [{ program: chain.eqProgram }]);
  }
  rewireChannel(chain);
}

// ─── Render budget (residency spec §5.5) ───
//
// renderCache's insertion order is its LRU order: a trigger's hit re-inserts
// its render (touchRender). Over the budget (Go's VoiceCacheBudgetBytes,
// setRenderCacheBudget), the oldest renders go first — never one of an
// instrument a row plays, one with a voice scheduled or playing, or a user
// sample's PCM (source data, not a render). No budget until Go sets one, and
// nothing is evicted before a circuit declares its held set.
let renderBudgetBytes = 0;

function touchRender(key, rec) {
  renderCache.delete(key);
  renderCache.set(key, rec);
}

function evictRendersOverBudget() {
  if (!renderBudgetBytes || !heldChannels) return;
  let total = 0;
  for (const r of renderCache.values()) total += r && r.data ? r.data.byteLength : 0;
  for (const [key, r] of renderCache) {
    if (total <= renderBudgetBytes) break;
    if (!r || !r.id || samplePCMIds.has(r.id) || channelHeld(r.id) || activeSources.get(r.id)?.size) continue;
    renderCache.delete(key);
    total -= r.data ? r.data.byteLength : 0;
    audioGraphStats.rendersEvicted++;
  }
}

window.setRenderCacheBudget = (bytes) => {
  renderBudgetBytes = bytes > 0 ? bytes : 0;
  evictRendersOverBudget();
};

// setHeldChannels(idsJSON): the instruments the rows play (Go's
// audio.SetHeldInstruments).
window.setHeldChannels = (idsJSON) => {
  let ids = [];
  try { ids = JSON.parse(idsJSON); } catch (_) { ids = []; }
  heldChannels = new Set(Array.isArray(ids) ? ids : []);
  for (const id of channelNodes.keys()) considerRelease(id);
  evictRendersOverBudget();
};

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
  const chain = getChannelChain(id, false);
  if (!chain) return 1;
  return chain.gain.gain.value;
}

export function setMainVolume(vol) { setChannelVolume('main', vol); }
export function mainVolume() { return channelVolume('main'); }

// ─── The C stages after the voice: insert effects, EQ, soft clip ───
//
// Every channel's insert chain and EQ, and the master soft clip, are desktop's
// C (fx_host.c, mix_dsp.c) — run on the audio thread by the beatmo-dsp
// AudioWorklet (dsp_worklet.js), which loads drums.single.js ONCE per
// AudioContext and keeps each node's state in that one heap. Go hands over
// everything the C needs and nothing is decided here: an insert chain is
// [{uid, type, enabled, params}] with complete params (Go's
// InsertEffectInstance), an EQ is a program of biquad coefficients (Go's
// BuildEQProgram), the soft-clip knee is CHAIN_SPEC's.
//
// A channel:  ingress → [synthAnalyser] → [fx node] → [preEQAnalyser]
//               → [eq node] → [analyser] → gain → main
// The master: main gain (master volume) → [synth/preEQ analysers] → [eq node]
//               → [analyser] → soft clip → masterOut → destination
//
// An edit is a message to a live node — a param retunes one C instance
// (state kept: desktop SetParam), a chain edit reconciles instances by uid
// (survivors keep their state, as desktop keeps its Go processors), an EQ edit
// of the same shape swaps coefficients and keeps the filter state. The graph
// is rewired only when a channel gains its first fx / eq node.
//
// Where AudioWorklet does not exist (an insecure origin such as `make
// serve-lan`) the same C runs on the main-thread module in a ScriptProcessor
// (its buffer adds latency there; beatmo.io is HTTPS). Until a context's DSP
// host is up, chains are wired without the stage nodes and rebuilt onto them
// when it lands; notes wait for it (processAudioEvent), so nothing plays
// unprocessed.

// audioGraphStats counts what live edits do to the graph (window.audioGraphStats).
const audioGraphStats = {
  insertChainUpdates: 0,   // updateInsertEffects: the chain reconciled in its live node
  insertParamUpdates: 0,   // retuneInsertEffect: one live C effect retuned
  dspNodesCreated: 0,      // beatmo-dsp nodes built (fx / eq / soft clip)
  eqUpdates: 0,            // setChannelEQ applied to a live eq node
  channelRewires: 0,       // rewireChannel calls (disconnect + reconnect a whole channel)
  channelsReleased: 0,     // residency: chains released once silent (considerRelease)
  rendersEvicted: 0,       // renders dropped to stay inside the render budget
};

// dspHosts: AudioContext → { state: 'loading' | 'worklet' | 'scriptprocessor' | 'none', ready, module }
const dspHosts = new WeakMap();

async function loadDSPWorklet(c) {
  try {
    await c.audioWorklet.addModule(new URL('./dsp_worklet.js', import.meta.url).href);
    const probe = new AudioWorkletNode(c, 'beatmo-dsp', {
      numberOfInputs: 0, numberOfOutputs: 1, outputChannelCount: [1], processorOptions: { kind: 'ready' },
    });
    const msg = await new Promise((res) => { probe.port.onmessage = (e) => res(e.data); });
    if (!msg || !msg.ready) throw new Error((msg && msg.error) || 'the C module did not load in the worklet');
    return true;
  } catch (err) {
    console.warn('[AUDIOJS] beatmo-dsp worklet unavailable — running the C stages on the main thread:', err);
    return false;
  }
}

function ensureDSPHost(c) {
  let h = dspHosts.get(c);
  if (h) return h;
  h = { state: 'loading', ready: null, module: null };
  dspHosts.set(c, h);
  // __beatmoDSPHost = 'scriptprocessor' forces the main-thread host (tests).
  const forceSP = typeof window !== 'undefined' && window.__beatmoDSPHost === 'scriptprocessor';
  const viaWorklet = !forceSP && !!c.audioWorklet && typeof AudioWorkletNode !== 'undefined';
  h.ready = (viaWorklet ? loadDSPWorklet(c) : Promise.resolve(false))
    .then(async (ok) => {
      if (ok) { h.state = 'worklet'; return; }
      try {
        h.module = await ensureModule();
        h.state = h.module ? 'scriptprocessor' : 'none';
      } catch (_) {
        h.state = 'none';
      }
    })
    .then(() => onDSPHostReady(c));
  return h;
}

function dspHostState(c) {
  const h = c && dspHosts.get(c);
  return h ? h.state : 'none';
}

function dspHostUp(c) {
  const s = dspHostState(c);
  return s === 'worklet' || s === 'scriptprocessor';
}

// dspNode builds one stage node (kind 'inserts' | 'eq' | 'softclip') on c's
// DSP host, configured by `messages` (dsp_host.js createStage), or null when
// the host is not up.
function dspNode(c, kind, init, messages) {
  const h = dspHosts.get(c);
  if (!h) return null;
  let node = null;
  if (h.state === 'worklet') {
    node = new AudioWorkletNode(c, 'beatmo-dsp', {
      numberOfInputs: 1, numberOfOutputs: 1, outputChannelCount: [1],
      channelCount: 1, channelCountMode: 'explicit', channelInterpretation: 'speakers',
      processorOptions: { kind, init: init || {}, messages: messages || [] },
    });
    node.port.onmessage = (e) => {
      if (e.data && e.data.refused) console.warn('[AUDIOJS] the C engine could not build', e.data.refused, '(passing through)');
      if (e.data && e.data.quiet && node._onQuiet) node._onQuiet();
    };
    node._dspPost = (d) => node.port.postMessage(d);
    node._dspDispose = () => node.port.postMessage('dispose');
    node._dspHost = 'worklet';
  } else if (h.state === 'scriptprocessor') {
    const stage = createStage(h.module, kind, c.sampleRate, init, () => node && node._onQuiet && node._onQuiet());
    if (!stage) return null;
    for (const d of messages || []) stage.message(d);
    node = c.createScriptProcessor(256, 1, 1);
    node.onaudioprocess = (e) => stage.process(e.inputBuffer.getChannelData(0), e.outputBuffer.getChannelData(0));
    node._dspPost = (d) => stage.message(d);
    node._dspDispose = () => stage.destroy();
    node._dspHost = 'scriptprocessor';
  }
  if (node) audioGraphStats.dspNodesCreated++;
  return node;
}

// onDSPHostReady moves every chain built before the host was up onto its
// stage nodes.
function onDSPHostReady(c) {
  if (c !== ctx) return;
  for (const chain of channelNodes.values()) {
    if (!chain || chain.released) continue;
    if (!chain.fxNode && Array.isArray(chain.fxChain) && chain.fxChain.length) {
      chain.fxNode = dspNode(c, 'inserts', null, [{ chain: chain.fxChain }]);
    }
    if (!chain.eqNode && chain.eqProgram) {
      chain.eqNode = dspNode(c, 'eq', null, [{ program: chain.eqProgram }]);
    }
    rewireChannel(chain);
  }
}

// dspReadyFor resolves once c's DSP host is up (or known absent).
function dspReadyFor(c) {
  return ensureDSPHost(c).ready;
}

// insertEffectHosts(id) → the host of the channel's insert chain
// ('worklet' | 'scriptprocessor'), one entry per enabled effect.
window.insertEffectHosts = (id) => {
  const chain = channelNodes.get(id);
  if (!chain || !Array.isArray(chain.fxChain)) return [];
  const host = chain.fxNode ? chain.fxNode._dspHost : 'pending';
  return chain.fxChain.filter((e) => e && e.enabled).map(() => host);
};

// updateInsertEffects(id, chainJSON): the channel's whole insert chain
// (InsertEffectInstance[]). Called from Go (platformInsertEffectsChanged).
function updateInsertEffects(id, chainJSON) {
  if (!hasCtx()) {
    // Queue for when the AudioContext exists (effects pushed by an import
    // before the unlock gesture must not be dropped).
    pendingChannelOps.push(() => updateInsertEffects(id, chainJSON));
    return;
  }
  const chain = getChannelChain(id);
  if (!chain) return;
  let list;
  try { list = typeof chainJSON === 'string' ? JSON.parse(chainJSON) : chainJSON; } catch (_) { list = []; }
  if (!Array.isArray(list)) list = [];
  chain.fxChain = list;
  chain.fxLatencySec = insertChainLatencySec(list);
  audioGraphStats.insertChainUpdates++;
  if (chain.fxNode) { chain.fxNode._dspPost({ chain: list }); return; }
  if (!list.length || chain.released || !dspHostUp(ctx)) return;
  chain.fxNode = dspNode(ctx, 'inserts', null, [{ chain: list }]);
  if (chain.fxNode) rewireChannel(chain);
}

window.updateInsertEffects = (id, chainJSON) => {
  try { updateInsertEffects(id, chainJSON); }
  catch (err) { dbg('insert.effect.error', { id, err: String(err) }); }
};

// retuneInsertEffect(id, uid, name, value): retune one live effect — the C
// effect's own smoothing applies and its state is kept (desktop SetParam).
// Called from Go (platformInsertEffectParamChanged) once per value change.
function retuneInsertEffect(id, uid, name, value) {
  const chain = hasCtx() ? channelNodes.get(id) : null;
  if (!chain || !Array.isArray(chain.fxChain)) return false;
  const e = chain.fxChain.find((x) => x && x.uid === uid);
  if (!e) return false;
  e.params = { ...(e.params || {}), [name]: value };
  chain.fxLatencySec = insertChainLatencySec(chain.fxChain);
  if (chain.fxNode) chain.fxNode._dspPost({ param: { uid, name, value } });
  audioGraphStats.insertParamUpdates++;
  return true;
}

// (Not "setInsertEffectParam": that global is Go's JS export (ui
// js_exports_insert_effects.go) — sharing it made Go's push call Go's export,
// which pushed again, until the page died.)
window.retuneInsertEffect = (id, uid, name, value) => {
  try { return retuneInsertEffect(id, uid, name, value); }
  catch (err) { dbg('insert.effect.param.error', { id, uid, err: String(err) }); return false; }
};

// insertChainLatencySec is how much later than start(when) a note on a channel
// with this chain SOUNDS: the pitch shifter's dual read heads trail the write
// head by up to one window, and at mix ≥ 0.5 the wet path is what is heard.
// The playback probe matches onsets that much later (heard, with its lag),
// instead of calling a delayed note silent.
function insertChainLatencySec(list) {
  let lat = 0;
  for (const e of list || []) {
    if (!e || !e.enabled || e.type !== 'pitchshift') continue;
    const p = e.params || {};
    if (Math.max(0, Math.min(1, p.mix ?? 1)) < 0.5) continue;
    lat += Math.max(20, Math.min(100, p.window ?? 50)) / 1000;
  }
  return lat;
}

// resetDSPState clears every delay line, envelope and filter memory — Go's
// audio.ResetDSPState (project import, offline renders) on both platforms.
window.resetDSPState = () => {
  for (const chain of channelNodes.values()) {
    if (chain && chain.fxNode) chain.fxNode._dspPost({ clear: true });
    if (chain && chain.eqNode) chain.eqNode._dspPost({ clear: true });
  }
};

function liveChannelIds() {
  return [...channelNodes.entries()].filter(([k, c]) => k !== 'main' && c && !c.released).map(([k]) => k);
}

// audioGraphStats() → the churn counters above plus the live graph size.
window.audioGraphStats = () => {
  let insertEffectsLive = 0;
  for (const chain of channelNodes.values()) {
    insertEffectsLive += (chain && Array.isArray(chain.fxChain) ? chain.fxChain.filter((e) => e && e.enabled).length : 0);
  }
  let dspStages = 0;
  for (const chain of channelNodes.values()) {
    if (chain && chain.fxNode) dspStages++;
    if (chain && chain.eqNode) dspStages++;
  }
  let renderBytes = 0;
  for (const rec of renderCache.values()) renderBytes += rec && rec.data ? rec.data.byteLength : 0;
  return {
    ...audioGraphStats,
    channelChains: channelNodes.size,
    liveChannels: liveChannelIds().length,
    liveChannelIds: liveChannelIds(),
    insertEffectsLive,
    dspStages,
    renderEntries: renderCache.size,
    renderBytes,
    blocksPushed: instrumentBlocks.size,
    dspHost: hasCtx() ? dspHostState(ctx) : 'none',
  };
};

// instrumentBlocks holds one complete modular param block per instrument, the
// instrument document flattened in Go (spec 2026-10-04 §8). It is the ONLY
// parameter source a render reads; a melodic per-pitch render patches only the
// pitch index of a copy.
const instrumentBlocks = new Map();

// samplePCMIds are the ids a registered PCM currently overrides
// (registerSamplePCM). A pushed block never evicts that PCM — Go drops the
// override (UnregisterSamplePCM, which re-pushes the block) when the synth
// should sound again.
const samplePCMIds = new Set();

// voicePost is the post-processing a voice of this render info gets after the
// C render (voice_post.js): its document's voice FX, gate tail and amplitude.
function voicePost(info) {
  return {
    amp: Number.isFinite(info.amp) ? info.amp : 0.6,
    gateTail: Number.isFinite(info.gateTail) ? info.gateTail : 0,
    fx: Array.isArray(info.fx) ? info.fx : [],
  };
}

// setInstrumentBlock is invoked by Go-WASM (instrument_docs_wasm.go) at boot
// and on every instrument-document change: the flattened block plus the
// document's render settings (seconds = the voice's fixed length, amplitude,
// gate tail, voice FX as C init calls — audio.VoiceFXCalls). Marks cached
// renders STALE (generation bump) so the next play re-renders in the
// BACKGROUND while the stale buffer keeps playing ON TIME
// (stale-while-revalidate).
window.setInstrumentBlock = (id, block, seconds, amp, melodic, gateTail, fx) => {
  if (!(block instanceof Float64Array) || block.length !== MODULAR_PARAM_COUNT) {
    throw new Error(`setInstrumentBlock(${id}): want a Float64Array(${MODULAR_PARAM_COUNT})`);
  }
  instrumentBlocks.set(id, block);
  RENDER_INFO[id] = { seconds, amp, gateTail, fx, paramBlock: 'modular' };
  if (melodic) MELODIC_INSTRUMENTS.add(id); else MELODIC_INSTRUMENTS.delete(id);
  if (samplePCMIds.has(id)) return; // a loaded sample keeps playing
  RENDER[id] = 'render_modular_p';
  bumpInstrumentParamsGen(id);
  rawRenderCache.delete(id);
};

// Test seam: the block + render metadata Go last pushed for id (a copy), so a
// browser test can re-push it (a generation bump) without going through Go.
window.__instrumentBlockForTest = (id) => {
  const b = instrumentBlocks.get(id);
  const info = RENDER_INFO[id];
  if (!b || !info) return null;
  return { block: Float64Array.from(b), seconds: info.seconds, amp: info.amp, melodic: MELODIC_INSTRUMENTS.has(id), gateTail: info.gateTail, fx: info.fx };
};

// clearInstrumentBlock forgets an instrument's block and render entry.
window.clearInstrumentBlock = (id) => {
  instrumentBlocks.delete(id);
  delete RENDER_INFO[id];
  MELODIC_INSTRUMENTS.delete(id);
  if (!samplePCMIds.has(id)) {
    delete RENDER[id];
    evictRenderCache(id);
  }
};

// warmInstrument is invoked by Go-WASM (voice_warm_js.go) when a row is switched
// to id. It pre-renders id's voices for the candidate node pitches OFF-THREAD
// (render worker) so the first notes after the switch hit a warm cache instead
// of a cold per-pitch render (the "click + laggy on instrument switch" bug).
// Non-melodic instruments share one bare-id key, so they warm once regardless
// of the pitch list. Best-effort + non-blocking: renders are deduped by
// pendingRenderEnsures and failures are swallowed (a missed warm just falls
// back to the normal on-demand render at play time).
window.warmInstrument = (id, pitchesJSON) => {
  try {
    if (!RENDER[id]) return;
    if (!isMelodicInstrument(id)) {
      ensureRenderReady(id).catch(() => {});
      return;
    }
    let pitches = [];
    try { pitches = JSON.parse(pitchesJSON); } catch (_) { pitches = []; }
    if (!Array.isArray(pitches) || pitches.length === 0) pitches = [0];
    const seen = new Set();
    for (const p of pitches) {
      const rp = roundPitchForCache(p);
      if (seen.has(rp)) continue;
      seen.add(rp);
      ensureRenderReady(id, rp).catch(() => {});
    }
  } catch (err) { dbg('warm.error', { id, err: String(err) }); }
};

// isMelodicInstrumentJS exposes the render layer's melodic predicate to Go-WASM
// (internal/audio/voice_prewarm_js.go IsMelodicInstrument) so the warm set only
// mints per-pitch items for instruments that actually keep a per-pitch cache.
window.isMelodicInstrumentJS = (id) => {
  try { return !!isMelodicInstrument(id); } catch (_) { return false; }
};

// voiceCachedFor answers "is this exact (instrument, pitch) already rendered
// and fresh?" using the SAME key the trigger path reads (cacheKeyFor over the
// rounded pitch) plus the param-generation staleness check. Go's
// audio.VoiceCached delegates here on js; before this existed it returned a
// hardcoded false, so the warm-up re-rendered voices already in the cache.
window.voiceCachedFor = (id, pitch) => {
  try {
    if (!RENDER[id]) return false;
    const rp = (isMelodicInstrument(id) && Number.isFinite(pitch)) ? roundPitchForCache(pitch) : 0;
    const rec = renderCache.get(cacheKeyFor(id, rp));
    return !!rec && !renderRecordStale(id, rec);
  } catch (_) { return false; }
};

// voiceReadyFor: would a trigger of (id, pitch) find its voice rendered? An id
// with no synth render (a sample) has nothing to wait for. The audio lane
// switches a row to an instrument only from its first ready note.
window.voiceReadyFor = (id, pitch) => !RENDER[id] || window.voiceCachedFor(id, pitch);

// warmInstrumentBatch renders a whole working set off-thread. It invokes the
// named global callback ONCE PER ITEM (success or failure) — failures still
// call back, because the readiness gate (warmup_controller.go) must never wait
// on an item that can never complete, or the toast hangs out its full
// deadline instead of opening. itemsJSON is [[instrumentID, pitch, token?], …];
// when present, `token` (opaque to this function) is passed straight back to
// the callback so a caller juggling many concurrent items — Go's
// WarmVoiceItem, see voice_prewarm_js.go — can tell which item finished. When
// itemsJSON entries carry no token (see warm_batch.browser.test.js, which
// calls this directly) the callback simply receives undefined.
//
// Dispatch is CHUNKED via a shared, persistent queue + pump. Go's actual
// caller (warmup_controller.go's feeder, via WarmVoiceItem in
// voice_prewarm_js.go) invokes this once PER ITEM, in a tight Go loop with no
// yield between calls — so chunking only the array *within* one call would
// miss the real burst: each 1-item call's dispatch work (building the
// param block + posting to the render worker) still ran synchronously inline
// before returning to Go, and Go's wasm scheduler does not yield to the
// browser event loop between plain calls either, so N single-item calls in a
// row reproduce the exact same unyielding burst a single N-item call would.
// Measured at ~450-700ms of main-thread stall for the ~91-item warm set the
// readiness gate now dispatches at import — independent of render
// worker-pool size (the stall is dispatch cost, not render cost) — long
// enough to freeze the progress toast's own animation exactly when it should
// be moving.
//
// The fix: every call only ENQUEUES its items (O(1) per item — cheap enough
// that a tight Go loop of enqueue-only calls can't itself stall the main
// thread) and starts the pump if it isn't already running. The pump drains
// WARM_BATCH_CHUNK_SIZE jobs synchronously, then yields to the event loop,
// then continues — picking up any items enqueued by later calls too, so
// repeated 1-item calls and one large batched call are dispatched identically.
// WARM_BATCH_CHUNK_SIZE=3 keeps a slice under roughly one frame (measured
// ~5-8ms dispatch cost per item, so ~15-24ms/slice) while keeping the number
// of yields — and their ~4ms-clamped setTimeout(0) overhead — small against
// the 15s gate deadline.
const WARM_BATCH_CHUNK_SIZE = 3;
const warmPumpQueue = [];
let warmPumpRunning = false;

async function runWarmPump() {
  warmPumpRunning = true;
  try {
    while (warmPumpQueue.length > 0) {
      const slice = warmPumpQueue.splice(0, WARM_BATCH_CHUNK_SIZE);
      for (const job of slice) {
        try {
          job();
        } catch (err) {
          // Backstop only — each job already guards its own body (see
          // warmInstrumentBatch below) and always fires its done callback
          // even on a synchronous throw. Without this backstop too, an
          // uncaught throw here would abort the while loop early, leaving
          // every remaining queued item's done() never called (the gate
          // waits out its full 15s deadline on those items) and — via the
          // `finally` below still running — merely resets warmPumpRunning
          // rather than the worse failure mode of leaving it stuck true.
          dbg('warm.pump.job.error', { err: String(err) });
        }
      }
      if (warmPumpQueue.length > 0) {
        // Yield to the event loop between slices so a long (or long-running,
        // via repeated single-item calls) warm set can't monopolize the main
        // thread in one synchronous burst.
        await new Promise((resolve) => setTimeout(resolve, 0));
      }
    }
  } finally {
    // Always clear the running flag, even if something above throws past
    // the per-job try/catch (e.g. a bug in the pump loop itself) — a stuck
    // `true` here means no future warmInstrumentBatch call in this tab ever
    // starts the pump again, silently killing warm-up for the rest of the
    // session.
    warmPumpRunning = false;
  }
}

window.warmInstrumentBatch = (itemsJSON, doneFnName) => {
  let items = [];
  try { items = JSON.parse(itemsJSON); } catch (_) { items = []; }
  const cb = window[doneFnName];
  const fire = (token) => {
    try { if (typeof cb === "function") cb(token); }
    catch (err) { dbg('warm.batch.cb.error', { err: String(err) }); }
  };
  if (!Array.isArray(items) || items.length === 0) return;
  for (const entry of items) {
    warmPumpQueue.push(() => {
      try {
        const id = entry && entry[0];
        const pitch = entry && entry[1];
        const token = entry && entry[2];
        if (!id || !RENDER[id]) { fire(token); return; }
        const melodic = isMelodicInstrument(id);
        ensureRenderReady(id, melodic ? pitch : undefined).then(() => fire(token)).catch(() => fire(token));
      } catch (err) {
        // ensureRenderReady/isMelodicInstrument/renderRecordStale are plain
        // (non-async) functions, so a synchronous throw from any of them —
        // one malformed instrument id in a 91-item import is enough —
        // would otherwise escape this job and propagate out of the pump's
        // while loop (see runWarmPump), permanently stranding
        // warmPumpRunning at true and killing every later warm-up in this
        // tab. A failed render must still count as done: the readiness
        // gate counts callbacks, and a lost one makes it wait out its full
        // 15s deadline instead of opening.
        dbg('warm.pump.dispatch.error', { err: String(err) });
        fire(entry && entry[2]);
      }
    });
  }
  if (!warmPumpRunning) runWarmPump();
};

// Test-only helper: forces a fresh render via ensureRenderedSample and
// returns a tail-energy summary of the resulting Float32Array. Used by
// instrument_params_audible.browser.test.js to verify
// setInstrumentParam reaches the C `_p` variant. Production code never
// reads __testCaptureSynthRender — keep it window-scoped + underscored
// so tree-shaking + linters skip it cleanly.
window.__testCaptureSynthRender = async (id, opts) => {
  // opts.sampleRate: render at that rate, bypassing (and not disturbing) the
  // playback cache — see ensureRenderedSample.
  const pinned = opts && Number.isFinite(opts.sampleRate) && opts.sampleRate > 0;
  if (!pinned) {
    renderCache.delete(id);
  }
  const rec = await ensureRenderedSample(id, pinned ? { sampleRate: opts.sampleRate } : undefined);
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
    viaWorker: rec.viaWorker,
  };
};

// Test-only helper: applies the sample-edit transform to a caller-provided
// buffer and returns the result as a plain array. Used by
// sample_edit_descriptor.browser.test.js to pin applySampleEdit to Go's
// BakeSample semantics on a deterministic input (C renders are noise-seeded,
// so pinning via a re-render is not reproducible). Production never reads it.
window.__testApplySampleEdit = (arr, sr, editJSON) =>
  Array.from(applySampleEdit(Float32Array.from(arr), sr, JSON.parse(editJSON)));

// ───────── Non-destructive sample-edit descriptors ─────────
// JS-side mirror of Go's audio.sampleEdits (sample_edit_descriptor.go). A
// descriptor keeps a synth instrument a SYNTH: the Sampler-tab edit
// (trim/pitch/gain/reverse/normalize/fade) is applied to the freshly-rendered
// C-synth buffer inside ensureRenderedSample instead of baking PCM and
// killing the render mapping. Field names match Go's SampleEdit JSON
// (StartFrac, EndFrac, TransposeSemis, DetuneCents, GainDB, Fade, Reverse,
// Normalize) plus FadeMs, the fade length Go sends with it.
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
     (e.GainDB || 0) === 0 && !e.Fade &&
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
  // Same RENDER[id] guard as setInstrumentBlock: a sample-based
  // instrument's renderCache entry holds the ONLY copy of its PCM — deleting
  // it would permanently silence the instrument. The descriptor only applies
  // to re-derivable C-synth renders anyway. Generation bump, not evict: the
  // stale render keeps playing on time while the edited one re-renders
  // (stale-while-revalidate, same as the param path).
  if (RENDER[id]) {
    bumpInstrumentParamsGen(id);
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
  const fadeMs = e.FadeMs || 0;
  if (fadeMs > 0) {
    const n = out.length;
    const fi = Math.min(Math.floor(sr * fadeMs / 1000.0), n);
    for (let i = 0; i < fi; i++) out[i] = Math.fround(out[i] * Math.fround(i / fi));
    for (let k = 0; k < fi; k++) out[n - 1 - k] = Math.fround(out[n - 1 - k] * Math.fround(k / fi));
  }
  return out;
}

// rewireChannel connects a channel's nodes in order (see the chain diagram
// above "The C stages after the voice"). The analysers are wired IN-LINE (audio
// passes through): fan-out tap nodes whose output is unreachable from
// ctx.destination can be skipped by Chrome's renderer, leaving their
// time-domain buffer all-zero (chain_per_instrument_traces.browser.test.js).
function rewireChannel(chain) {
  audioGraphStats.channelRewires++;
  const stages = [chain.synthAnalyser, chain.fxNode, chain.preEQAnalyser, chain.eqNode, chain.analyser].filter(Boolean);
  try { chain.ingress.disconnect(); } catch (_) {}
  for (const n of stages) { try { n.disconnect(); } catch (_) {} }
  let node = chain.ingress;
  for (const n of stages) { node.connect(n); node = n; }
  // The master bus (ingress === gain) ends at the soft clip; a channel at its
  // gain, which feeds the master bus.
  node.connect(chain.ingress === chain.gain ? audioDestination() : chain.gain);
}

// setChannelEQ(id, program): the channel's EQ program (Go's BuildEQProgram, a
// Float64Array), or null for no EQ. A live eq node takes it as a message — the
// C retunes in place when the shape is unchanged (state kept), or starts a
// fresh EQ for a new shape, exactly as desktop's SetChannelEQ.
export function setChannelEQ(id, program) {
  if (!hasCtx()) {
    pendingChannelOps.push(() => setChannelEQ(id, program));
    return;
  }
  const chain = getChannelChain(id);
  if (!chain) return;
  chain.eqProgram = program && program.length ? Float64Array.from(program) : null;
  if (chain.eqNode) {
    chain.eqNode._dspPost({ program: chain.eqProgram });
    audioGraphStats.eqUpdates++;
    return;
  }
  if (!chain.eqProgram || chain.released || !dspHostUp(ctx)) return;
  chain.eqNode = dspNode(ctx, 'eq', null, [{ program: chain.eqProgram }]);
  if (chain.eqNode) rewireChannel(chain);
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
  // How many time-domain samples this channel's snapshots export, kept
  // independent of fftSize. See WAVE_EXPORT_DEFAULT.
  chain._waveExportMax = Math.max(64, Math.min(fft, opts.waveSamples || WAVE_EXPORT_DEFAULT));
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
// per-analyser record in chain._taps in-place; no new objects per call.
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
// WAVE_EXPORT_DEFAULT caps the time-domain samples a snapshot exports when
// the caller did not ask for a specific number.
//
// One AnalyserNode per channel serves BOTH domains, and its fftSize is set
// by the spectrum's resolution requirement (Go: analyzer.SpectrumFFTSize —
// a display band holding no FFT bin renders as silence, so the bins have to
// be finer than the narrowest band). The time domain wants no such thing:
// the scope taps, the Wave tab and the per-stage traces all read `wave`, and
// a longer window would quadruple every one of them across the bridge for
// nothing. So the node is long and the EXPORT is capped, taking the most
// RECENT samples — which is exactly the tap those consumers had before.
const WAVE_EXPORT_DEFAULT = 512;

function fillSnapshotBuffersForChain(chain, analyserKey, timeBufKey, freqBufKey, bins) {
  const a = chain[analyserKey];
  // Output buffers are PER ANALYSER, not per chain. A chain carries up to
  // three taps (analyser / preEQAnalyser / synthAnalyser) and they used to
  // share one _snapResult and one pair of Float32Arrays, so each call
  // overwrote the previous tap's output in place. It happened to be safe
  // only because the Go bridge copies the bytes out before the next call —
  // an invariant nothing states and nothing checks. It also stopped being
  // merely theoretical once the taps got DIFFERENT fftSizes (the channel
  // analyser runs at analyzer.SpectrumFFTSize for the Spectrum tab's bin
  // width while preEQ/synth stay short): one shared array sized for the
  // largest tap, with a length field from the smallest, is a readback bug
  // waiting for a reordering.
  let taps = chain._taps;
  if (!taps) taps = chain._taps = {};
  let tap = taps[analyserKey];
  if (!tap) tap = taps[analyserKey] = { result: {} };
  const result = tap.result;
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
  // bins === 0 means the caller wants SCALARS ONLY (Go's
  // ChannelAnalyzerMetrics, which the Meters tab and every non-detail row
  // use). Skip the frequency read, the downsample loop and the wave copy
  // entirely rather than computing a spectrum for a caller that never looks
  // at it — with rows on the scalar path that is most calls per rebuild.
  const scalarsOnly = bins === 0;
  if (!scalarsOnly) a.getFloatFrequencyData(freq);
  // Export window: the most RECENT waveExportMax samples (see
  // WAVE_EXPORT_DEFAULT). rms/peak are measured over the SAME slice that is
  // exported, so a consumer reading `wave` and a consumer reading `rms`
  // never disagree about which window they are describing.
  const cap = chain._waveExportMax || WAVE_EXPORT_DEFAULT;
  const exportN = Math.min(time.length, cap);
  const off = time.length - exportN;
  let sum = 0;
  let peak = 0;
  for (let i = 0; i < exportN; i++) {
    const v = time[off + i];
    sum += v * v;
    const av = Math.abs(v);
    if (av > peak) peak = av;
  }
  const rms = exportN > 0 ? Math.sqrt(sum / exportN) : 0;
  if (scalarsOnly) {
    result.rms = rms;
    result.peak = peak;
    result.spectrumLen = 0;
    result.waveLen = 0;
    result.spectrumU8 = null;
    result.waveU8 = null;
    return result;
  }
  // Spectrum: persistent Float32Array sized to bin count.
  const n = freq.length;
  const step = Math.max(1, Math.floor(n / Math.max(1, bins)));
  const numBins = Math.ceil(n / step);
  let specOut = tap.specF32;
  if (!specOut || specOut.length < numBins) {
    specOut = tap.specF32 = new Float32Array(numBins);
    tap.specU8 = new Uint8Array(specOut.buffer);
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
  // Wave: persistent Float32Array sized to the export window, clamp to [-1,1].
  let waveOut = tap.waveF32;
  if (!waveOut || waveOut.length < exportN) {
    waveOut = tap.waveF32 = new Float32Array(exportN);
    tap.waveU8 = new Uint8Array(waveOut.buffer);
  }
  for (let i = 0; i < exportN; i++) {
    const v = time[off + i];
    waveOut[i] = v > 1 ? 1 : v < -1 ? -1 : v;
  }
  result.rms = rms;
  result.peak = peak;
  result.spectrumLen = oi;
  result.waveLen = exportN;
  result.spectrumU8 = tap.specU8;
  result.waveU8 = tap.waveU8;
  // Back-compat fields: do NOT allocate plain arrays here. Old callers
  // that read `.spectrum` / `.wave` will see undefined and must adapt;
  // the only known consumer is the Go-WASM bridge, which uses the U8
  // fast path with a fallback to the absent fields (treated as empty).
  return result;
}

export function channelAnalyzerSnapshot(id, bins = 64) {
  const chain = getChannelChain(id, false);
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
  const chain = getChannelChain(id, false);
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
  const chain = getChannelChain(id, false);
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

// audioReady resolves once the C DSP ENGINE is loaded and warm. Exposed as
// window.audioReady so index.html can gate game start on it.
//
// It warms the ENGINE, never an instrument. Which voices a session needs, and
// with which params, is Go's knowledge: the loaded circuit (the startup demo
// JSON or an import) and each instrument's bound recipe defaults. Go's circuit
// warm-up gate (warmup_controller.go → warmInstrumentBatch) renders exactly
// those, AFTER the defaults are seeded. The hardcoded instrument list that used
// to render here ran BEFORE the seed — every config-first instrument got the
// bare modular voice, which the seed then evicted.
//
// That list did do one useful thing, by accident: each render compiled and
// tiered up the WASM in a render worker before the game started. Without it the
// circuit warm-up runs on COLD workers at the moment playback begins, and the
// first notes are scheduled late (webaudio_three_stage_latency: 19 → 68 overdue
// events). So: a couple of throwaway renders of the bare voice on EVERY pool
// worker and on the main-thread module, results discarded, nothing cached.
// engineWarmBlock is a minimal sounding block for the engine warm-up (compile /
// tier-up only — never an instrument; every instrument's block comes from Go).
function engineWarmBlock() {
  const b = new Float64Array(MODULAR_PARAM_COUNT);
  b[MODULAR_PARAM_INDEX.osc_enabled] = 1;
  b[MODULAR_PARAM_INDEX.env_enabled] = 1;
  b[MODULAR_PARAM_INDEX.amp_sustain] = 1;
  b[MODULAR_PARAM_INDEX.gain] = 1;
  return b;
}

const ENGINE_WARM_RENDERS_PER_WORKER = 2;
const ENGINE_WARM_SECONDS = 0.5;
async function warmupAudio() {
  const m = await ensureModule();
  const sr = Math.max(8000, Math.min(192000, getSampleRate()));
  const frames = Math.max(1, Math.floor(sr * ENGINE_WARM_SECONDS));
  const jobs = [];
  const pool = ensureSynthRenderWorkers();
  // renderVoiceInWorker routes to the least-loaded worker, so issuing them
  // together spreads them across the whole pool.
  for (let i = 0; pool && i < pool.length * ENGINE_WARM_RENDERS_PER_WORKER; i++) {
    jobs.push(renderVoiceInWorker('render_modular_p', sr, frames, engineWarmBlock(), MODULAR_PARAM_COUNT, null).catch(() => {}));
  }
  try { renderVoiceSync(m, 'render_modular_p', sr, frames, engineWarmBlock(), MODULAR_PARAM_COUNT); } catch (_) {}
  await Promise.all(jobs);
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
// postProcessVoiceRender finishes a voice render exactly as playback plays it:
// both render paths arrive already post-processed (voice_post.js: voice FX,
// gate tail, normalise × amplitude, end fade — P4), then the non-destructive
// Sampler edit — the same transform as Go's BakeSample, applied to the fresh C
// render (mirrors tryRecipeVoice in synth_recipe_dispatch.go). The synth stays
// the source of truth: a param change re-renders through here on the next play.
// Shared by ensureRenderedSample and renderDocPCM (the Synth-tab graph render),
// so the graph is the played voice.
function postProcessVoiceRender(data, sr, sampleEdit) {
  return sampleEdit ? applySampleEdit(data, sr, sampleEdit) : data;
}

async function ensureRenderedSample(id, opts) {
  if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch && window.__synthEvtTrace) {
    console.log('[SYNTH-ERS]', 'enter', { id, hasCache: renderCache.has(id), block: instrumentBlocks.has(id) });
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
  // opts.sampleRate renders at an explicit rate instead of the device's (test
  // seam: engines disagree on the AudioContext rate — WebKit 44.1 kHz, Chromium
  // 48 kHz — so a cross-engine voice comparison must pin one). Such a render is
  // never read from or written to the playback cache.
  const srOverride = (opts && Number.isFinite(opts.sampleRate) && opts.sampleRate > 0) ? opts.sampleRate : 0;
  const bypassCache = (skipEdit && instrumentSampleEdits.has(id)) || srOverride > 0;
  // Melodic per-pitch re-render: when a rounded pitch is requested for a
  // melodic instrument, render the C voice AT that pitch and cache under the
  // pitch-folded key. roundedPitch defaults to 0 → bare-id key (unchanged for
  // drums / FM / pitch-0 melodic / sampler captures).
  const roundedPitch = (opts && Number.isFinite(opts.pitch)) ? roundPitchForCache(opts.pitch) : 0;
  const atPitch = isMelodicInstrument(id) && roundedPitch !== 0;
  const cacheKey = atPitch ? cacheKeyFor(id, roundedPitch) : id;
  // Use getSampleRate() to avoid creating AudioContext prematurely.
  // The engine rate (CHAIN_SPEC.sampleRate), whatever the device runs at.
  const sr = Math.max(8000, Math.min(192000, srOverride || getSampleRate()));
  // If cache exists but was built for a different sample rate, rebuild so pitch
  // and duration stay correct on devices that default to 48 kHz.
  // Param generation this render is being built for. Captured BEFORE the
  // async render so a mid-render edit leaves the new record already-stale and
  // the next trigger re-renders (self-coalescing: at most one render behind).
  const genAtStart = paramsGenOf(id);
  if (!bypassCache && renderCache.has(cacheKey)) {
    const hit = renderCache.get(cacheKey);
    if (hit && hit.sr === sr && (hit.gen || 0) === genAtStart) {
      try {
        if (typeof window !== 'undefined') {
          const metrics = window.__audioMetrics || (window.__audioMetrics = { renders: {}, cacheHits: {} });
          metrics.cacheHits[id] = (metrics.cacheHits[id] || 0) + 1;
        }
      } catch (_) {}
      return hit;
    }
    if (!hit || hit.sr !== sr) {
      // Wrong-rate entry can't be played; drop and regenerate.
      renderCache.delete(cacheKey);
    }
    // Param-stale entry (gen lag) stays in the cache — it is the ON-TIME
    // playback fallback until the fresh render below replaces it.
  }
  const info = RENDER_INFO[id] || { seconds: 0.5, amp: 0.8 };
  const frames = Math.max(1, Math.floor(sr * info.seconds));
  // The synchronous main-thread module is still loaded (other paths — worklets,
  // insert effects, delay/reverb — use it) and serves as the render fallback.
  const m = await ensureModule();
  // The instrument's block is its document, flattened in Go and pushed via
  // setInstrumentBlock. Copied (never a WASM heap view) so it can be
  // structured-cloned into the render worker AND reused by the sync fallback;
  // a melodic per-pitch render patches only the copy's pitch field so the C
  // voice renders at freq=220·2^(roundedPitch/12) (formant fixed, no resample).
  const pushed = instrumentBlocks.get(id);
  if (!pushed) return null; // Go has not pushed this instrument
  const paramArr = Float64Array.from(pushed);
  const allocCount = MODULAR_PARAM_COUNT;
  if (atPitch && Number.isFinite(MODULAR_PARAM_INDEX.pitch)) {
    paramArr[MODULAR_PARAM_INDEX.pitch] = roundedPitch;
  }
  const renderFn = 'render_modular_p';
  if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch) {
    const fld = (n) => (paramArr && Number.isFinite(MODULAR_PARAM_INDEX[n])) ? paramArr[MODULAR_PARAM_INDEX[n]] : 'n/a';
    const block = paramArr ? {
      osc_type: fld('osc_type'), osc_enabled: fld('osc_enabled'),
      fm_enabled: fld('fm_enabled'), env_enabled: fld('env_enabled'),
      filter_enabled: fld('filter_enabled'), drive_enabled: fld('drive_enabled'),
      gain: fld('gain'), osc_level: fld('osc_level'), level: fld('level'),
    } : null;
    console.log('[SYNTH-DISPATCH]', 'render request', { id, fn: renderFn, block });
  }
  // Test seam (synth_revoice_render_resilience.browser.test.js): force the
  // render to fail so the failure contract is exercised.
  const forceRenderThrow = (typeof window !== 'undefined' && !!window.__forceRevoiceRenderThrow);

  // ── Render: off-thread worker preferred, synchronous main-thread fallback ──
  // This is the WASM perf fix: the ~30–40 ms C render runs on the worker so a
  // live param edit that invalidates the cache mid-playback no longer blocks
  // the UI + audio scheduler. The worker is unavailable (or times out) → the
  // same render runs synchronously (degraded, not broken). A render that FAILS
  // throws: the instrument's previous (param-stale) record stays in the cache
  // and keeps playing on time, and the caller drops the note when there is
  // none. There is no substitute voice — rendering anything else in its place
  // would play a different instrument.
  let data = null;
  let viaWorker = false;
  // P4: the worker applies the voice post-processing when it renders, so the
  // main thread's per-render O(frames) passes only run on the sync fallback.
  const post = voicePost(info);
  // Render-latency observation: wall time from render request to cached
  // buffer, as experienced by the trigger path (worker queue wait + render +
  // main-thread post-process). Queue depth is sampled at request time — with
  // a single render worker, depth N means ~N×renderMs of added wait.
  const renderT0 = (typeof performance !== 'undefined' && performance.now) ? performance.now() : 0;
  const renderQueueDepth = synthWorkerPending.size;
  // While the user is hearing playback, a slow / timed-out worker render must
  // never block the main thread. When the worker path fails during playback we
  // throw "render deferred" instead of sync-rendering: the caller's on-grid
  // neighbor fallback (or a single dropped note) covers it, and the exact
  // render still lands off-thread. The synchronous fallback stays available for
  // idle warm-up and offline paths so behaviour there degrades, not breaks.
  const allowMainThreadRender = !isPlaybackActive();
  const RENDER_DEFERRED = () => new Error('render deferred: worker unavailable during playback');
  // Test seam (synth_param_block_double.browser.test.js): skip the worker so the
  // synchronous main-thread path renders — the two must agree bit for bit.
  const forceSync = (typeof window !== 'undefined' && !!window.__beatmoForceSyncRender);
  let deferred = false;
  try {
    if (forceRenderThrow) throw new Error('forced render failure (test seam)');
    try {
      if (forceSync) throw new Error('forced sync render (test seam)');
      data = await renderVoiceInWorker(renderFn, sr, frames, paramArr, allocCount, post);
      viaWorker = true;
    } catch (werr) {
      if (!allowMainThreadRender) { deferred = true; throw RENDER_DEFERRED(); }
      data = renderVoiceSync(m, renderFn, sr, frames, paramArr, allocCount, post);
      viaWorker = false;
    }
  } catch (err) {
    if (!deferred) {
      console.error('[AUDIOJS] render failed for ' + id + ' via ' + renderFn + ': ' +
        (err && err.message ? err.message : String(err)) +
        ' — its previous buffer (if any) keeps playing; no substitute voice');
    }
    throw err;
  }

  // ── Post-processing: the optional sample-edit ──
  // Both render paths arrive already post-processed (voice_post.js: voice FX,
  // gate tail, normalise × amplitude, end fade), bit-identical to each other.
  if (typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch) {
    let pk = 0;
    for (let i = 0; i < data.length; i++) { const a = Math.abs(data[i]); if (a > pk) pk = a; }
    console.log('[SYNTH-DISPATCH]', 'render result', { id, viaWorker, peak: pk, silent: !(pk > 1e-6) });
  }
  const finalData = postProcessVoiceRender(data, sr, sampleEdit);
  if (renderT0 > 0) {
    renderLatencyMetrics.observeRender(performance.now() - renderT0, viaWorker, renderQueueDepth);
  }
  // Store raw Float32Array + metadata. AudioBuffer is created lazily in
  // ensureAudioBuffer() when playback actually needs it, so this path
  // never forces an AudioContext into existence. `gen` marks which param
  // generation this render reflects (stale-while-revalidate).
  // viaWorker travels with the record so tests can tell the worker render from
  // the synchronous one.
  const record = { id, buffer: null, data: finalData, sr, frames: finalData.length, gen: genAtStart, viaWorker };
  if (!bypassCache) {
    renderCache.set(cacheKey, record);
    evictRendersOverBudget();
  }
  try {
    if (typeof window !== 'undefined') {
      const metrics = window.__audioMetrics || (window.__audioMetrics = { renders: {}, cacheHits: {} });
      if (!metrics.workerRenders) metrics.workerRenders = {};
      if (!metrics.mainThreadRenders) metrics.mainThreadRenders = {};
      metrics.renders[id] = (metrics.renders[id] || 0) + 1;
      if (viaWorker) metrics.workerRenders[id] = (metrics.workerRenders[id] || 0) + 1;
      else metrics.mainThreadRenders[id] = (metrics.mainThreadRenders[id] || 0) + 1;
      const meta = window.__renderMeta || (window.__renderMeta = {});
      meta[id] = { sr, frames, seconds: frames / sr };
    }
  } catch (_) {}
  return record;
}

// Phase B: Lazily create an AudioBuffer from the cached Float32Array.
// Called at play time when we actually need a buffer source node.
// WebAudio handles resampling automatically if the buffer's sample rate
// differs from the context's rate, so we don't need to re-render.
function ensureAudioBuffer(id, cacheKey) {
  const key = cacheKey || id;
  const record = renderCache.get(key);
  if (!record || !record.data) return null;
  if (record.buffer) return record.buffer;
  const c = getCtx();
  const buffer = c.createBuffer(1, record.frames, record.sr);
  buffer.copyToChannel(record.data, 0);
  record.buffer = buffer;
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
  const rate = sr && sr > 0 ? sr : CHAIN_SPEC.sampleRate;
  delete RENDER[id];
  samplePCMIds.add(id);
  renderCache.set(id, { buffer: null, data, sr: rate, frames });
  rawRenderCache.delete(id);
  dbg('sample.registerPCM', { id, frames, sr: rate });
}
window.registerSamplePCM = (id, u8, sr) => registerSamplePCM(id, u8, sr);

// unregisterSamplePCM is the inverse of registerSamplePCM: it drops the PCM
// override for id (clearing the cached buffer + render cache) and restores the
// built-in synth render mapping, so the Sampler/Synth "Reset to factory" makes
// the original synth audible again instead of the leftover chop. No-op-safe for
// ids that have no default render (pure user samples).
export function unregisterSamplePCM(id) {
  renderCache.delete(id);
  rawRenderCache.delete(id);
  samplePCMIds.delete(id);
  // The synth render comes back with the block Go re-pushes right after
  // (UnregisterSamplePCM → setInstrumentBlock).
  if (instrumentBlocks.has(id)) {
    RENDER[id] = 'render_modular_p';
  }
  dbg('sample.unregisterPCM', { id, restored: instrumentBlocks.has(id) });
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

// renderDocPCM renders a FLATTENED instrument document (block: Float64Array of
// MODULAR_PARAM_COUNT, with its render settings as setInstrumentBlock takes
// them) the way a trigger of instrument id plays it — the playback render
// worker (synchronous fallback only while nothing is playing), then
// postProcessVoiceRender with id's Sampler edit — and resolves with
// { bytes, length, sr } like captureInstrumentPCM. It is the Synth tab's graph
// render (Go audio.RenderDocForGraph): it never touches the playback cache, and
// it WAITS while any playback render is queued so a graph never delays a note.
export async function renderDocPCM(id, block, seconds, amp, gateTail, fx) {
  if (!(block instanceof Float64Array) || block.length !== MODULAR_PARAM_COUNT) {
    throw new Error(`renderDocPCM(${id}): want a Float64Array(${MODULAR_PARAM_COUNT})`);
  }
  const waitStart = Date.now();
  while (synthWorkerPending.size > 0 && Date.now() - waitStart < 2000) {
    await new Promise((r) => setTimeout(r, 15));
  }
  const sr = Math.max(8000, Math.min(192000, getSampleRate()));
  const frames = Math.max(1, Math.floor(sr * (Number.isFinite(seconds) && seconds > 0 ? seconds : 0.5)));
  const post = voicePost({ amp, gateTail, fx });
  const paramArr = Float64Array.from(block);
  let data;
  try {
    data = await renderVoiceInWorker('render_modular_p', sr, frames, paramArr, MODULAR_PARAM_COUNT, post);
  } catch (werr) {
    if (isPlaybackActive()) throw werr;
    const m = await ensureModule();
    data = renderVoiceSync(m, 'render_modular_p', sr, frames, paramArr, MODULAR_PARAM_COUNT, post);
  }
  const out = postProcessVoiceRender(data, sr, instrumentSampleEdits.get(id) || null);
  return { bytes: new Uint8Array(out.buffer, out.byteOffset, out.byteLength), length: out.length, sr };
}
window.renderDocPCM = (id, block, seconds, amp, gateTail, fx) => renderDocPCM(id, block, seconds, amp, gateTail, fx);

// decodeWavToPCM fetches and decodes a WAV (or any decodeAudioData-supported
// file) at the FILE'S OWN rate into mono PCM — channels averaged, as desktop's
// decoder (miniaudio, mono out) does — returning a byte view over the
// Float32Array plus its rate. Go converts it to the engine rate with the same
// resampler desktop uses (audio.RegisterSamplePCM → pcmAtEngineRate), so the
// browser's decodeAudioData resampler never touches it: decoding on an
// OfflineAudioContext at the file's rate converts nothing.
export async function decodeWavToPCM(url) {
  const res = await fetch(url);
  const arr = await res.arrayBuffer();
  const sr = wavFileSampleRate(arr) || CHAIN_SPEC.sampleRate;
  const OAC = window.OfflineAudioContext || window.webkitOfflineAudioContext;
  let buf;
  try {
    buf = await new OAC(1, 1, sr).decodeAudioData(arr.slice(0));
  } catch (_) {
    buf = await getCtx().decodeAudioData(arr); // a rate the engine cannot decode at: its own rate
  }
  const n = buf.length;
  const mono = new Float32Array(n);
  const chans = buf.numberOfChannels;
  for (let c = 0; c < chans; c++) {
    const d = buf.getChannelData(c);
    for (let i = 0; i < n; i++) mono[i] += d[i];
  }
  if (chans > 1) for (let i = 0; i < n; i++) mono[i] /= chans;
  return {
    bytes: new Uint8Array(mono.buffer, mono.byteOffset, mono.byteLength),
    length: n,
    sr: buf.sampleRate,
  };
}

// wavFileSampleRate reads a RIFF/WAVE file's sample rate from its 'fmt '
// chunk (0 when the bytes are not a WAV).
function wavFileSampleRate(arr) {
  try {
    const v = new DataView(arr);
    const tag = (o) => String.fromCharCode(v.getUint8(o), v.getUint8(o + 1), v.getUint8(o + 2), v.getUint8(o + 3));
    if (v.byteLength < 12 || tag(0) !== 'RIFF' || tag(8) !== 'WAVE') return 0;
    for (let o = 12; o + 8 <= v.byteLength;) {
      const size = v.getUint32(o + 4, true);
      if (tag(o) === 'fmt ' && o + 16 <= v.byteLength) return v.getUint32(o + 12, true);
      o += 8 + size + (size & 1);
    }
  } catch (_) {}
  return 0;
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
export function ensureRenderReady(id, pitch) {
  const trace = typeof window !== 'undefined' && window.__beatmoDebugSynthDispatch && window.__synthEvtTrace;
  // Melodic per-pitch variant: cache + pending are keyed by the pitch-folded
  // cacheKey so each played pitch warms independently. Non-melodic / pitch-0
  // callers pass no pitch → bare-id key, unchanged.
  const roundedPitch = (isMelodicInstrument(id) && Number.isFinite(pitch)) ? roundPitchForCache(pitch) : 0;
  const cacheKey = cacheKeyFor(id, roundedPitch);
  const cached = renderCache.get(cacheKey);
  if (cached && !renderRecordStale(id, cached)) {
    return Promise.resolve(cached);
  }
  if (!RENDER[id]) {
    return Promise.resolve(cached || null);
  }
  let pending = pendingRenderEnsures.get(cacheKey);
  if (pending) {
    if (trace) console.log('[SYNTH-RDY]', 'reuse pending', { id, cacheKey });
    return pending;
  }
  if (trace) console.log('[SYNTH-RDY]', 'start render', { id, cacheKey });
  pending = ensureRenderedSample(id, roundedPitch !== 0 ? { pitch: roundedPitch } : undefined).then((record) => {
    pendingRenderEnsures.delete(cacheKey);
    if (trace) console.log('[SYNTH-RDY]', 'render done', { id, cacheKey, ok: !!record, nowCached: renderCache.has(cacheKey) });
    return record;
  }).catch((err) => {
    pendingRenderEnsures.delete(cacheKey);
    if (trace) console.log('[SYNTH-RDY]', 'render THREW', { id, cacheKey, err: String(err) });
    throw err;
  });
  pendingRenderEnsures.set(cacheKey, pending);
  return pending;
}

// playSound / playSoundParams: render the voice if it is not yet, then play
// it through the one note path (processAudioEvent) — the same played voice,
// headroom, fade-in and channel as every sequencer note.
export async function playSound(id, vol = 1.0, when) {
  return playSoundParams(id, vol, 0, 1, when);
}

export async function playSoundParams(id, vol = 1.0, pitch = 0.0, dur = 1.0, when) {
  const melodic = isMelodicInstrument(id);
  if (RENDER[id]) await ensureRenderReady(id, melodic ? roundPitchForCache(pitch) : undefined);
  if (!renderCache.has(melodic ? cacheKeyFor(id, roundPitchForCache(pitch)) : id)) throw new Error('Unknown sound: ' + id);
  enqueueAudioEvents([{ id, vol, pitch, dur, when }], true);
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
  const fadeOutSec = CHAIN_SPEC.stopFadeMs / 1000; // desktop's stop fade (linear)
  const stopped = Array.from(set);
  for (const src of stopped) {
    try {
      // Use the anti-pop GainNode for smooth fade-out if available.
      const fadeGain = src._antiPopGain;
      if (fadeGain && hasCtx()) {
        fadeGain.gain.cancelScheduledValues(now);
        // From the voice's own level (its gain is constant until a stop).
        fadeGain.gain.setValueAtTime(Number.isFinite(src._level) ? src._level : fadeGain.gain.value, now);
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
  // Untrack only the sources stopped here: clearing the whole set would orphan
  // sources started during the fade window, so a later stop could not reach them.
  setTimeout(() => {
    for (const s of stopped) { try { set.delete(s); } catch (_) {} }
    if (set.size === 0) { try { activeSources.delete(key); } catch (_) {} }
  }, cleanupMs);
};

window.setChannelVolume = (id, vol) => {
  try { setChannelVolume(id, vol); } catch (err) { dbg('channel.set.error', { id, err: String(err) }); }
};

window.channelVolume = (id) => {
  try { return channelVolume(id); } catch (err) { dbg('channel.get.error', { id, err: String(err) }); return 1; }
};

// window.setMainVolume / mainVolume are Go's exports (js_exports_graph_ui.go:
// they clamp and move the transport slider); audio.js must not shadow them.

window.setChannelEQ = (id, program) => {
  try { setChannelEQ(id, program); } catch (err) { dbg('channel.eq.error', { id, err: String(err) }); }
};

// fftSize sets the FREQUENCY resolution (Go: analyzer.SpectrumFFTSize);
// waveSamples caps the TIME-domain export independently. Go passes both —
// see audio.BridgeAnalyserFFTSize / BridgeWaveformSamples. Older callers
// that pass only a window size still get the WAVE_EXPORT_DEFAULT tap.
window.enableChannelAnalyzer = (id, fftSize, waveSamples) => {
  try { enableChannelAnalyzer(id, { fftSize, waveSamples }); } catch (err) { dbg('channel.analyzer.error', { id, err: String(err) }); }
};

// bins is the number of spectrum bins to return. Go passes
// audio.BridgeSpectrumBins (== the analyser's own bin count, i.e. NO
// downsample): the grouping below is LINEAR, so any downsample costs
// resolution exactly where the display bands are narrowest — at the bottom.
// The old implicit default of 64 left every band under ~375 Hz empty.
window.channelAnalyzerSnapshot = (id, bins) => {
  try { return channelAnalyzerSnapshot(id, bins === 0 ? 0 : (bins || 64)); } catch (err) { dbg('channel.analyzer.snap.error', { id, err: String(err) }); return { rms: 0, peak: 0, spectrum: [], wave: [] }; }
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

// Batch scheduling API to reduce Go→JS crossings in WASM builds.
// Expects an Array of objects: { id, vol, pitch, dur, when? }.
// Uses a synchronous fast path when samples are pre-rendered/decoded.
window.playSoundsBatch = (arr) => {
  try {
    if (!Array.isArray(arr)) {
      if (window.BEATMO_AUDIO_DEBUG && arr && typeof arr === 'object') {
        dbg('play.params.enqueue', { id: arr.id, vol: arr.vol, pitch: arr.pitch, dur: arr.dur, when: arr.when });
      }
      enqueueAudioEvents([arr], true);
    } else {
      if (window.BEATMO_AUDIO_DEBUG) {
        for (const item of arr) {
          if (item && typeof item === 'object') {
            dbg('play.params.enqueue', { id: item.id, vol: item.vol, pitch: item.pitch, dur: item.dur, when: item.when });
          }
        }
      }
      enqueueAudioEvents(arr, true);
    }
  } catch (err) {
    console.error('[AUDIOJS] playSoundsBatch error', err);
  }
};

// Flat batch scheduling API (ids + typed arrays) to reduce Go<->JS overhead.
// rows/abss (optional trailing args): sequencer identity threaded from
// audio.BatchParam.Row/Abs (batch_wasm.go) so timingTraces (Stage-D browser
// emission timing, parity-timing-sync) can attribute each scheduled note.
// Omitted/undefined entries default to -1 (fire-and-forget, never recorded).
// ideals/grids (optional trailing args): measurement-only timing identity from
// audio.BatchParam.Ideal/Grid — the scheduler's pre-clamp request and the
// predictor's trigger time without lookahead — read by the playback probe.
// vids (optional 11th arg): audio-lane voice ids (audio.BatchParam.VoiceID);
// 0 / missing = a preview or UI sound, never touched by lane cancel/stop.
window.playSoundsBatchFlat = (ids, vols, pitches, durs, whens, hasWhen, rows, abss, ideals, grids, vids) => {
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
        row: Number.isFinite(rows?.[i]) ? rows[i] : -1,
        abs: Number.isFinite(abss?.[i]) ? abss[i] : -1,
        ideal: ideals?.[i],
        grid: grids?.[i],
        vid: vids?.[i],
      };
      const when = Number.isFinite(whens?.[i]) ? whens[i] : NaN;
      if (hasWhen && hasWhen[i] && Number.isFinite(when)) {
        ev.when = when;
      }
      events[i] = ev;
    }
    enqueueAudioEvents(events, true);
  } catch (err) {
    console.error('[AUDIOJS] playSoundsBatchFlat error', err);
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

// stampedDownloadName(base, ext) -> "<base>-YYYY-MM-DD_HHMMSS<ext>".
//
// Mirrors stampedFileName in src/go/internal/ui/download_name.go. A download
// name that can already exist in the Downloads folder is renamed by the
// browser, and every engine spells the uniquifier differently — Chrome
// "x (1).json", Firefox "x(1).json", Safari "x 2.json". The page cannot pin
// that format, so never ask for a name that can collide.
window.stampedDownloadName = (base, ext) => {
  const d = new Date();
  const p = (n) => String(n).padStart(2, '0');
  const stamp = `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}_` +
    `${p(d.getHours())}${p(d.getMinutes())}${p(d.getSeconds())}`;
  return `${base}-${stamp}${ext}`;
};

// Download a JSON blob with a given filename.
window.downloadJSON = (name, text) => {
  try {
    const blob = new Blob([text], { type: 'application/json' });
    const a = document.createElement('a');
    a.href = URL.createObjectURL(blob);
    a.download = name || window.stampedDownloadName('beatmo-export', '.json');
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
    a.download = name || window.stampedDownloadName('audio-history', '.csv');
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
      window.downloadCSV(opts.filename || window.stampedDownloadName('audio-history', '.csv'), csv);
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
  // Stash the picked filename for the Go side (read after the promise resolves)
  // so the "Loaded <name>" notification can name the file. Kept off the resolved
  // value itself so openJSONFile's string contract (and its browser test) holds.
  window.__lastImportName = '';
  // Check for a pending mobile file pick from the gesture-based rect system.
  const pending = window._fpConsumePending?.('import');
  if (pending) {
    console.log('[IMPORT] consuming pending mobile file pick');
    pending.then(r => { window.__lastImportName = r?.name || ''; resolve(r?.data || ''); }).catch(() => resolve(''));
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
      window.__lastImportName = file.name || '';
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
    if (rec && rec.data) return rec.data.length / (rec.sr || CHAIN_SPEC.sampleRate);
  } catch (_) {}
  return 0;
};
// Internal metric: number of enabled insert effects on a channel's chain.
// Returns 0 when the channel chain hasn't been built yet (no AudioContext).
// Used by tests to verify that updateInsertEffects() applied after context
// unlock when called pre-context (e.g. demo import before user gesture).
window.__channelInsertFXCount = (id) => {
  try {
    if (!hasCtx()) return 0;
    const chain = channelNodes.get(id);
    if (!chain || !Array.isArray(chain.fxChain)) return 0;
    return chain.fxChain.filter((e) => e && e.enabled).length;
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
      const dur = rec && rec.buffer ? rec.buffer.duration || 0 : (rec && rec.data ? rec.data.length / (rec.sr || CHAIN_SPEC.sampleRate) : 0);
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
  preloadOutputTap(c);
  if (outputTapReady) {
    // Audio-thread tap on a PARALLEL branch: masterOut → tap → (silent) → out.
    // The masterOut → destination path is left untouched.
    const tap = new AudioWorkletNode(c, 'beatmo-output-tap', {
      numberOfInputs: 1, numberOfOutputs: 1, outputChannelCount: [1],
      channelCount: 1, channelCountMode: 'explicit', channelInterpretation: 'speakers',
    });
    tap.port.onmessage = (e) => {
      const d = e.data;
      if (d && d.flushed !== undefined) {
        const w = outputTapFlushWaiters.get(d.flushed);
        if (w) { outputTapFlushWaiters.delete(d.flushed); w(); }
        return;
      }
      if (outputCaptureEnabled && d && d.length) {
        for (let i = 0; i < d.length; i++) outputCaptureBuffer.push(d[i]);
      }
    };
    const mute = c.createGain();
    mute.gain.value = 0;
    tap.connect(mute);
    mute.connect(c.destination); // keeps the tap pulled by the render graph
    ensureMasterOut().connect(tap);
    outputCaptureNode = tap;
    outputCaptureIsWorklet = true;
    console.log('[AUDIO CAPTURE] Audio-thread capture tap attached to the master output');
    return;
  }
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

  // Insert the capture node between masterOut and the hardware destination
  // (masterOut is always the last node before output).
  const out = ensureMasterOut();
  try { out.disconnect(c.destination); } catch (_) {}
  out.connect(outputCaptureNode);
  outputCaptureNode.connect(c.destination);

  console.log('[AUDIO CAPTURE] Capture node inserted into audio chain');
};

// Start capturing output samples
window.startOutputCapture = () => {
  outputCaptureBuffer = [];
  if (!outputCaptureNode) {
    window.enableOutputCapture();
  } else if (!outputCaptureIsWorklet) {
    // Re-verify capture chain is intact — Chrome's ScriptProcessorNode can
    // receive all-zero input after extended use across multiple start/stop
    // cycles. Re-wiring the limiter → captureNode → destination path fixes it.
    const c = getCtx();
    const out = ensureMasterOut();
    if (out && c) {
      try { out.disconnect(); } catch (_) {}
      try { outputCaptureNode.disconnect(); } catch (_) {}
      out.connect(outputCaptureNode);
      outputCaptureNode.connect(c.destination);
    }
  }
  // Set enabled AFTER enableOutputCapture (which resets it to false).
  outputCaptureEnabled = true;
  if (outputCaptureIsWorklet && outputCaptureNode) outputCaptureNode.port.postMessage('on');
  console.log('[AUDIO CAPTURE] Started capturing output at', outputCaptureSampleRate, 'Hz');
};

// Stop capturing and return the captured samples as Float32Array
window.stopOutputCapture = () => {
  outputCaptureEnabled = false;
  if (outputCaptureIsWorklet && outputCaptureNode) outputCaptureNode.port.postMessage('off');
  const result = new Float32Array(outputCaptureBuffer);
  console.log('[AUDIO CAPTURE] Stopped. Captured', result.length, 'samples (',
    (result.length / outputCaptureSampleRate).toFixed(3), 'seconds)');
  return result;
};

// flushOutputCapture resolves once every block the audio-thread tap captured
// BEFORE this call has reached outputCaptureBuffer. The tap posts blocks and
// the reply on the same port (FIFO), so the reply arrives after all of them.
// Call it before stop/get when the exact capture window matters and the main
// thread may be busy. Resolves immediately for the ScriptProcessor fallback.
window.flushOutputCapture = () => {
  if (!outputCaptureIsWorklet || !outputCaptureNode) return Promise.resolve();
  const seq = ++outputTapFlushSeq;
  return new Promise((resolve) => {
    outputTapFlushWaiters.set(seq, resolve);
    outputCaptureNode.port.postMessage({ flush: seq });
    setTimeout(() => { if (outputTapFlushWaiters.delete(seq)) resolve(); }, 5000);
  });
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
window.getChainConfigForTest = async () => {
  try { getCtx(); } catch (_) { return null; }
  if (!hasCtx()) return null;
  await dspReadyFor(ctx);
  ensureMainNode();
  rewireChannel(channelNodes.get('main'));
  return {
    spec: CHAIN_SPEC,
    sampleRate: ctx.sampleRate,
    dspHost: dspHostState(ctx),
    masterSoftClip: masterClip ? masterClip._dspHost : null,
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
    audioThread: outputCaptureIsWorklet,
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
  const wasWorklet = outputCaptureIsWorklet;
  if (outputCaptureNode) {
    try { if (wasWorklet) masterOut && masterOut.disconnect(outputCaptureNode); } catch (_) {}
    try { outputCaptureNode.disconnect(); } catch (_) {}
    outputCaptureNode = null;
  }
  outputCaptureIsWorklet = false;
  outputCaptureEnabled = false;
  outputCaptureBuffer = [];
  const c = hasCtx() ? ctx : null;
  // The worklet tap is a parallel branch — the masterOut → destination path
  // was never cut, so there is nothing to restore.
  if (masterOut && c && !wasWorklet) {
    try { masterOut.disconnect(); } catch (_) {}
    masterOut.connect(c.destination);
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
  // The master tap reads what hits the destination — masterOut, after the
  // soft clip (desktop records the same point). We do NOT splice the capture
  // node INTO the chain; we add a parallel path so the user-audible output is
  // unaffected.
  return ensureMasterOut() || c.destination;  // fallback shouldn't happen in prod
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
    a.download = filename || window.stampedDownloadName('beatmo-recording', '.zip');
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
window.__getMasterOut = () => masterOut;

// NOTE: an earlier iteration installed a 4ms setInterval calling Go's
// tickSequencer() to bypass WASM goroutine scheduling. Benchmarks showed
// it INCREASED Stage B (bridge) jitter because the extra drive
// contended for seqMu against Update() and the audioLoop drive without
// reducing Stage A meaningfully — the seqMu TryLock conflicts dominate
// any sub-frame gain. The Go-side tickSequencer export is kept available
// for tests that want explicit sequencer drives.

// ─── Offline render of the real graph (cross-platform stage tests) ───────────
//
// __offlineRenderForTest renders notes through THIS module's graph — the same
// rewireChannel / setChannelEQ / updateInsertEffects / processAudioEvent /
// startRenderVoice / stopLaneAt and the beatmo-dsp worklet — on an
// OfflineAudioContext, and returns the signal at each requested tap.
// xplat_audio_stages.browser.test.js compares those taps with desktop's
// mixer, sample by sample. The live graph is set aside for the render and
// restored after it (a test page plays nothing meanwhile).
//
// spec = {
//   frames, sampleRate,
//   channels: { id: { chain: InsertEffectInstance[], eq: number[] | null } } | 'live',
//   masterVolume, masterEQ: number[] | null,
//   notes: [{ id, at, pitch, vol, dur, lane }],  // at in frames
//   stops: [{ id, at }],                          // lane stops (stopLaneAt)
//   taps:  [{ name, channel, stage: 'voices' | 'inserts' | 'eq' }],
// }
// → { sampleRate, started, taps: { out: Float32Array, [name]: Float32Array } }
// 'live' copies each live channel's pushed configuration (what Go sent).
window.__offlineRenderForTest = async (spec) => {
  const sr = spec.sampleRate || CHAIN_SPEC.sampleRate;
  // Every voice rendered (and fresh) BEFORE the swap: a cold note would defer.
  for (const n of spec.notes || []) {
    const melodic = isMelodicInstrument(n.id);
    await ensureRenderReady(n.id, melodic ? roundPitchForCache(n.pitch) : undefined);
  }
  const live = { ctx, mainChannelNode, masterOut, masterClip, chains: new Map(channelNodes), lanes: new Map(laneVoices), sources: new Map(activeSources) };
  const channels = spec.channels === 'live' ? (() => {
    const m = {};
    for (const [id, c] of live.chains) if (id !== 'main' && c) m[id] = { chain: c.fxChain || [], eq: c.eqProgram ? Array.from(c.eqProgram) : null };
    return m;
  })() : (spec.channels || {});
  const masterVolume = spec.channels === 'live' && live.mainChannelNode ? live.mainChannelNode.gain.value : spec.masterVolume;
  const masterEQ = spec.channels === 'live' ? (live.chains.get('main') && live.chains.get('main').eqProgram) : spec.masterEQ;
  const taps = [{ name: 'out' }, ...(spec.taps || [])];
  const OAC = window.OfflineAudioContext || window.webkitOfflineAudioContext;
  const off = new OAC({ numberOfChannels: taps.length, length: spec.frames, sampleRate: sr });
  channelNodes.clear(); laneVoices.clear(); activeSources.clear();
  ctx = off; mainChannelNode = null; masterOut = null; masterClip = null;
  const offChains = [];
  try {
    await dspReadyFor(off);
    if (!dspHostUp(off)) throw new Error('no DSP host on the OfflineAudioContext: ' + dspHostState(off));
    ensureMasterOut();
    masterOut.disconnect(); // the merger below owns the destination
    ensureMainNode();
    for (const [id, c] of Object.entries(channels)) {
      getChannelChain(id);
      if (c && Array.isArray(c.chain) && c.chain.length) updateInsertEffects(id, c.chain);
      if (c && c.eq) setChannelEQ(id, c.eq);
    }
    setChannelVolume('main', masterVolume);
    if (masterEQ) setChannelEQ('main', masterEQ);
    let vid = 0;
    const before = voiceStartCount;
    offlineRenderActive = true;
    try {
      for (const n of spec.notes || []) {
        processAudioEvent({ id: n.id, vol: n.vol, pitch: n.pitch, dur: n.dur, when: n.at / sr, vid: n.lane ? 1e9 + (++vid) : 0, laneGen: laneGeneration }, off);
      }
    } finally {
      offlineRenderActive = false;
    }
    const started = voiceStartCount - before;
    for (const s of spec.stops || []) window.stopLaneAt(s.id, s.at / sr);
    const merger = off.createChannelMerger(taps.length);
    merger.connect(off.destination);
    taps.forEach((t, k) => {
      let node = masterOut;
      if (t.channel) {
        const ch = channelNodes.get(t.channel);
        if (!ch) throw new Error('tap ' + t.name + ': no channel ' + t.channel);
        node = t.stage === 'voices' ? ch.ingress
          : t.stage === 'inserts' ? (ch.fxNode || ch.ingress)
            : (ch.eqNode || ch.fxNode || ch.ingress);
      }
      node.connect(merger, 0, k);
    });
    for (const c of channelNodes.values()) offChains.push(c);
    const rendered = await off.startRendering();
    const out = { sampleRate: off.sampleRate, started, taps: {} };
    taps.forEach((t, k) => { out.taps[t.name] = rendered.getChannelData(k).slice(); });
    return out;
  } finally {
    for (const c of offChains) {
      for (const n of [c.fxNode, c.eqNode]) if (n && n._dspDispose) n._dspDispose();
    }
    if (masterClip && masterClip._dspDispose) masterClip._dspDispose();
    channelNodes.clear(); laneVoices.clear(); activeSources.clear();
    for (const [k, v] of live.chains) channelNodes.set(k, v);
    for (const [k, v] of live.lanes) laneVoices.set(k, v);
    for (const [k, v] of live.sources) activeSources.set(k, v);
    ctx = live.ctx; mainChannelNode = live.mainChannelNode; masterOut = live.masterOut; masterClip = live.masterClip;
  }
};
