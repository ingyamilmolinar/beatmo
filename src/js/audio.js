import {
  MODULAR_PARAM_COUNT,
  MODULAR_PARAM_INDEX,
  MODULAR_PARAM_TABLES,
} from './synth_param_abi.gen.js';

// CHAIN_SPEC (generated from chain_spec.go) holds every value the voice-to-speaker path uses on both platforms. The DSP
// is desktop's C in the beatmo-dsp worklet and a note is desktop's played voice; xplat_audio_stages compares every
// stage with desktop's per sample.
import { CHAIN_SPEC } from './chain_spec.gen.js';
import { createStage } from './dsp_host.js';
import { gateLength, playedVoiceSamples } from './played_voice.js';
import { applyVoicePost } from './voice_post.js';


// Fail LOUD on a stale synth_param_abi.gen.js: an undersized block makes the C voice read uninitialised fields and
// silence the generator with no error.
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
  // So the browser ABI guard test can see a stale committed gen file.
  window.__synthABI = {
    modularCount: MODULAR_PARAM_COUNT,
    stale: !MODULAR_ABI_OK,
    hasModularFields: MODULAR_REQUIRED_FIELDS.every((f) => Number.isFinite(MODULAR_PARAM_INDEX[f])),
  };
  // A build marker: no banner after a reload means a CACHED old audio.js (hard-reload, or the no-cache `make serve`).
  // Bump it when editing this file.
  window.__audioJsBuild = 'SYNTHDBG-6';
  try {
    console.log('%c[AUDIOJS] loaded build ' + window.__audioJsBuild +
      ' — synth edits restore a factory render shadowed by sample PCM (frozen-instrument fix)', 'color:#0a0;font-weight:bold');
  } catch (_) {}
}

let modulePromise = null;
let mod;
let ctx;

// Captured on the AUDIO thread by a worklet tap off the master, the ScriptProcessor only a fallback: a main-thread
// callback lost most blocks with the app running. Posted blocks queue instead of dropping; flushOutputCapture() waits
// for them.
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

// The last node before ctx.destination, after the soft clip: what the device receives, where output capture and the
// recording master tap read (as on desktop). Created lazily.
let masterOut = null;
// Desktop's C soft clip on the DSP host; null until that host is up.
let masterClip = null;

function ensureMasterOut() {
  if (masterOut) return masterOut;
  if (!hasCtx()) return null;
  masterOut = ctx.createGain();
  masterOut.gain.value = 1;
  masterOut.connect(ctx.destination);
  return masterOut;
}

// The soft clip, or masterOut itself until the DSP host is up.
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

// So mute nodes can stop in-flight sounds: id → Set<AudioBufferSourceNode>.
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

// The lane commits notes up to its horizon ahead and must take them back when an edit changes them; vid 0 (a preview)
// is never touched here.
const LANE_CANCEL_GUARD_SEC = 0.008; // closer to start than this = "started"
const laneVoices = new Map();      // vid → { src, gain, inst, when, pk }
const laneTombstones = new Set();  // vids cancelled before they reached start()
let laneGeneration = 0;            // bumped by cancelAllLaneFuture

function laneRegister(vid, src, gain, inst, when, pk, level) {
  if (!vid) return;
  laneVoices.set(vid, { src, gain, inst, when, pk, level });
  // Only if still THIS source: a swapped voice's stale source ends after the fresh one registered under the same vid.
  try { src.addEventListener('ended', () => { if (laneVoices.get(vid)?.src === src) laneVoices.delete(vid); }, { once: true }); } catch (_) {}
}

// A vid not started anywhere yet (queued, awaiting a render, a stale-race replay) is tombstoned so the replay drops it.
// Returns true if it already started.
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

// Consumed by the replay they target; those whose replay never comes are pruned oldest-first.
const LANE_TOMBSTONES_MAX = 4096;
function laneTombstone(vid) {
  laneTombstones.add(vid);
  if (laneTombstones.size > LANE_TOMBSTONES_MAX) {
    for (const old of laneTombstones) { laneTombstones.delete(old); if (laneTombstones.size <= LANE_TOMBSTONES_MAX / 2) break; }
  }
}

// Re-starts a voice scheduled on a STALE render on the fresh one, same start, if not yet started; otherwise the stale
// voice plays (timing wins, timbre lags one note).
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
  // Returns the vids REFUSED because they started, so Go drops their replacements: never the old AND the new voice.
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

  // A mute node: voices starting before t fade linearly over stopFadeMs from t's sample, as desktop's RequestStopAt; later
  // ones are spared. The gain is constant until a stop, so nothing needs cancel-and-hold (which Firefox lacks).
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

// The lateness scheduleMetrics CANNOT see: a cache miss defers the event and replays it later, and the schedule metric
// only sees the rewritten time. renderMs* is a render as the trigger path felt it, deferMs* the signed deviation from
// the intended `when` (deferAbsMsP90 is the gate), workerQueuePeak the in-flight maximum.
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
      // Played a previous buffer ON TIME because the fresh render missed its deadline.
      stalePlays: this.stalePlays,
      // Played the nearest cached pitch resampled, on time, while the exact pitch rendered.
      neighborPlays: this.neighborPlays,
    };
  },
};
renderLatencyMetrics.reset();
if (typeof window !== 'undefined') {
  window.getRenderLatencyMetrics = () => renderLatencyMetrics.snapshot();
  window.resetRenderLatencyMetrics = () => renderLatencyMetrics.reset();
}

// {row, abs, idealWhen, startWhen, ctxAtStart} at each real start() of a SEQUENCER note (row ≥ 0, threaded from the
// batch bridge), for Go's emission-drift checker via getTimingTraces(). Previews are never recorded.
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

// Every other timing metric stops at the `when` handed to start(); this detects each note's ONSET in the rendered
// audio on the audio thread, per channel and at the output. Run `await startAudioProbe()`, play, `await
// audioProbeReport()`. Zero cost while off.
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
      // The analysis matches the onset that much later.
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

// Taps every channel bus (post EQ/FX/fader) and the final output; restarting discards the previous log.
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
  // A gap here is time the Go scheduler could not run; the probe shows whether audio suffered.
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
// None: every event in a batch (≤ 32) is due within the lookahead, and deferring one hands it to a timer that cannot
// fire until the frame ends. A hit costs microseconds; a miss only starts an async render.
const AUDIO_SYNC_FLUSH_BUDGET_MS = Infinity;
const AUDIO_MIN_LEAD_SEC = 0.004;

const audioQueue = [];
let audioQueueHead = 0;
let audioFlushScheduled = false;
let audioFlushInFlight = false;

// So a render may not block the main thread while the user hears playback.
let lastScheduledPlayWhen = -1;

// While playing, a slow worker render must NEVER fall back to a synchronous main-thread render (it froze UI and
// audio for seconds); the note uses the neighbour fallback or drops. The sync path stays for idle warm-up and offline.
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
  // origWhen keeps the intended start across replays; allowStale makes a stale-race replay play the stale record rather
  // than race again.
  const origWhen = Number.isFinite(ev.origWhen) ? ev.origWhen : NaN;
  const allowStale = ev.allowStale === true;
  // row < 0 marks a preview, never recorded.
  const row = Number.isFinite(ev.row) ? ev.row : -1;
  const abs = Number.isFinite(ev.abs) ? ev.abs : -1;
  // Measurement only: they never influence WHEN a note plays.
  const ideal = Number.isFinite(ev.ideal) && ev.ideal > 0 ? ev.ideal : NaN;
  const grid = Number.isFinite(ev.grid) && ev.grid > 0 ? ev.grid : NaN;
  const pk = Number.isFinite(ev.pk) ? ev.pk : 0;
  // laneGen is the lane generation it was enqueued under (cancelAllLaneFuture bumps it).
  const vid = Number.isFinite(ev.vid) && ev.vid > 0 ? ev.vid : 0;
  const laneGen = Number.isFinite(ev.laneGen) ? ev.laneGen : undefined;
  // Same vid, same start, no new request.
  const laneFresh = vid > 0 && ev.laneFresh === true;
  return { id, vol, pitch, dur, when, origWhen, allowStale, row, abs, ideal, grid, pk, vid, laneGen, laneFresh };
}

// Without it a deferred note reaches start() anonymous and the probe cannot attribute its late onset.
function carryTiming(from, to) {
  to.ideal = from.ideal;
  to.grid = from.grid;
  to.pk = from.pk;
  to.vid = from.vid;
  to.laneGen = from.laneGen;
  return to;
}

// sync starts the batch NOW, inside the Go→JS call: a microtask waits for Go to return to the event loop, which on
// WASM is the end of the whole Ebiten frame, so notes sat behind slow frames and started late.
function enqueueAudioEvents(events, sync = false) {
  if (!Array.isArray(events) || events.length === 0) {
    return;
  }
  // Without a context the events stay queued until it reaches 'running'.
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
  // A/B diagnostics only; never set in production.
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

// Stable: simultaneous events keep their arrival order. An ordered queue costs one pass.
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
    // Until a user gesture creates the context.
    if (!hasCtx()) {
      return;
    }
    // A suspended context's clock stays at 0, so times scheduled now are already past when it resumes: silence.
    if (ctx.state === 'suspended') {
      ctx.resume().then(() => {
        if (queueSize() > 0) scheduleAudioFlush(true);
      }).catch(() => {
        if (queueSize() > 0) scheduleAudioFlush(true);
      });
      return;
    }
    // A lane flush hands over a horizon at once; a note due in 10 ms must not wait behind ones due in a second.
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
    // Tombstoned while queued, or committed before a cancelAllLaneFuture: drop.
    if (laneTombstones.has(ev.vid) || ev.laneGen !== laneGeneration) {
      laneTombstones.delete(ev.vid);
      if (audioProbe && ev.pk) audioProbe.noteCancelled(ev.pk);
      return;
    }
  }
  const vol = Number.isFinite(ev.vol) ? ev.vol : 1.0;
  const pitch = Number.isFinite(ev.pitch) ? ev.pitch : 0.0;
  const dur = Number.isFinite(ev.dur) && ev.dur > 0 ? ev.dur : 1.0;
  // Survives defer and stale-race replays.
  const row = Number.isFinite(ev.row) ? ev.row : -1;
  const abs = Number.isFinite(ev.abs) ? ev.abs : -1;
  // Desktop's played voice: a melodic voice renders AT its pitch and Duration < 1 GATES it; anything else is resampled by
  // 2^(pitch/12)/dur. played_voice.js builds the samples; WebAudio only plays them at rate 1.
  const melodic = isMelodicInstrument(id);
  const renderPitch = melodic ? roundPitchForCache(pitch) : 0;
  const rate = melodic ? 1 : Math.pow(2, pitch / 12) / dur;
  const gate = melodic && dur < 1 ? dur : 1;
  const cacheKey = melodic ? cacheKeyFor(id, renderPitch) : id;
  // The C stages must be on the audio thread before a note reaches them; only a context's first moments wait.
  if (dspHostState(ctx) === 'loading') {
    dspReadyFor(ctx).then(() => enqueueAudioEvents([ev]));
    return;
  }
  const now = ctx.currentTime || 0;
  let when = Number.isFinite(ev.when) ? ev.when : NaN;
  // The request BEFORE the min-lead floor: observing the clamped value made every late note read as lead +4 ms.
  const requestedWhen = when;
  // An offline render has no clock to beat: a note at frame 0 stays there.
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
  // The FIRST intended `when`, so a note never loses its grid position across replays.
  const intendedWhen = Number.isFinite(ev.origWhen) ? ev.origWhen
    : (Number.isFinite(requestedWhen) ? requestedWhen : when);
  const timing = { ideal: ev.ideal, grid: ev.grid, pk: ev.pk, vid: ev.vid, laneGen: ev.laneGen };
  let laneStale = false;
  if (render && renderRecordStale(id, render) && ev.allowStale !== true) {
    // A live edit made the record stale: race the fresh render against the note's deadline and play whichever is ready ON
    // TIME. Timing is never sacrificed; timbre lags at most one note. Previews wait a short budget for the new timbre.
    const gridLocked = Number.isFinite(ev.when);
    if (!(ev.vid && gridLocked)) {
      raceFreshRenderAgainstDeadline(ctx, carryTiming(timing, { id, vol, pitch, dur, row, abs }), melodic ? pitch : undefined, intendedWhen, gridLocked);
      return;
    }
    // A lane voice never waits on the race, whose replay is a main-thread timer a stall would delay: the stale buffer is
    // scheduled now and swapped if the fresh render lands first.
    laneStale = true;
  }
  if (!render && RENDER[id]) {
    if (melodic) {
      // A wrong formant for one note beats a note off the grid; the exact pitch renders in the background.
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
    // Replays at the ORIGINAL `when`: on grid if the render beats the deadline, otherwise late by the overshoot only.
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
      // The signed deviation the schedule metrics cannot see.
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

// Headroom × the exact volume, as desktop's scaledVoice × mixHeadroom.
function voiceLevel(vol) {
  return CHAIN_SPEC.voiceHeadroom * Math.max(0, Number.isFinite(vol) ? vol : 1);
}

// WebAudio would otherwise start between samples, interpolating every sample of the voice.
function frameTime(c, t) {
  const sr = c.sampleRate || CHAIN_SPEC.sampleRate;
  return Math.round(t * sr) / sr;
}

// Desktop's fade-in baked in; cached per render record (a newer render is a new record).
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
  // The samples before the gain, so the stage test compares them with desktop's played voice.
  window.__playedVoiceForTest = (data, sr, melodic, pitch, dur) => {
    const d = dur > 0 ? dur : 1;
    const g = melodic && d < 1 ? gateLength(data.length, d, sr, CHAIN_SPEC.gateFadeMs) : null;
    return playedVoiceSamples(Float32Array.from(data), g, melodic ? 1 : Math.pow(2, pitch / 12) / d, Math.trunc((sr * CHAIN_SPEC.fadeInMs) / 1000));
  };
}

// row/abs/idealWhen thread sequencer identity into timingTraces.
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

// Covers re-enqueue, flush and the lazy AudioBuffer wrap, with headroom.
const FRESH_DEADLINE_MARGIN_SEC = 0.012;
// Previews are not grid-locked, so freshness beats the nominal start; sized to cover one voice render.
const FRESH_NOWAIT_BUDGET_SEC = 0.08;

// Exactly one replay fires, and a failed render falls back too, so an edit never silences a note. The replay carries
// allowStale: one race per note bounds the slip, where re-racing would drift later each cycle.
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

// The engine's rate, whatever the device runs at.
function getSampleRate() {
  return CHAIN_SPEC.sampleRate;
}

// 'playback' routes WebAudio through the media channel (speaker), not the ringer channel. Safari 16.4+.
function configureAudioSession() {
  try {
    if (typeof navigator !== 'undefined' && navigator.audioSession) {
      navigator.audioSession.type = 'playback';
    }
  } catch (_) {}
}
// Apply at module load time (no-op on browsers that don't support audioSession).
configureAudioSession();

// A silent looping <audio> merges iOS's ringer and media channels, for speaker output before navigator.audioSession.
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

// Before getCtx(), so the statechange listener avoids the temporal dead zone.
let _audioUnlocked = false;
const UNLOCK_EVENTS = ['touchstart', 'touchend', 'pointerdown', 'mousedown', 'keydown'];

// Volume/EQ/mute changes made before the context exists, applied when it first reaches 'running'.
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

// Only from gesture handlers or code that needs a context now.
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
        // Ensure the main node and the master output are wired up now that the context is live.
        ensureMainNode();
        // Flush any audio events that were deferred while the context was suspended.
        if (queueSize() > 0) {
          scheduleAudioFlush(true);
        }
      }
    });
  } catch (_) {}
  // A context that starts 'running' never fires statechange.
  if (ctx.state === 'running') {
    _audioUnlocked = true;
    applyPendingOps();
    ensureMainNode();
    if (queueSize() > 0) {
      scheduleAudioFlush(true);
    }
  }
  dbg('ctx.new', { sr: ctx.sampleRate, state: ctx.state });
  // Chains built before they land are rebuilt onto them.
  ensureDSPHost(ctx);
  return ctx;
}

// An engine that refuses the rate gets its own; voices still render at the engine rate and WebAudio converts.
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
    // It may start suspended on mobile; the unlock gestures resume it.
    createCtx();
  }
  // Try to resume if the context is suspended (common on first user gesture).
  if (ctx.state === 'suspended') {
    try { ctx.resume(); dbg('ctx.resume', { state: ctx.state }); } catch (e) { dbg('ctx.resume.error', { err: String(e) }); }
  }
  return ctx;
}

// Called at import by the readiness gate, so a circuit imported before the unlock gesture builds its graph while Play is
// disabled instead of at the first note. Wiring nodes needs no 'running' context; idempotent.
window.flushPendingChannelOps = () => {
  getCtx();
  applyPendingOps();
  ensureMainNode();
};

// iOS needs a silent buffer played inside a gesture to truly unlock. Listeners are NEVER removed: a re-suspended context
// (tab switch, call, background) unlocks again on the next gesture.
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

// iOS suspends the context when the page is backgrounded.
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

// 'pagehide', not 'beforeunload': iOS fires only pagehide.
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
// A glue built from an older header reads later fields at shifted offsets, wrong voices with no error: refused loudly.
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

// A render is 30-40 ms for a long voice; on the main thread it stalled the UI and the scheduler on every edit. A pool of
// workers each hosting the DSP module renders off-thread (output is worker-independent); capped low for memory and a
// free core, created lazily since many pages never render.
const RENDER_WORKER_MAX = 4;
function renderWorkerCount() {
  try {
    // Test/bench only, never set in production.
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

// Never at module load: a page that never renders never pays for a Worker.
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

// Least-loaded, not round-robin: render cost varies 20× between a drum and a bowed string, so a blind rotation stacks
// expensive items on one worker.
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

// Rejects on no worker, error or timeout; during playback callers do NOT sync-render. Params are cloned, not
// transferred, for the sync fallback. One pending map across the pool: its size is the workerQueuePeak metric.
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
      // voice_post.js, the same function as the main-thread fallback, so results stay bit-identical.
      entry.worker.postMessage({ type: 'render', reqId, renderFn, sr, frames, allocCount: allocCount || 0, params: paramArr || null, post: post || null });
    } catch (e) {
      synthWorkerPending.delete(reqId); clearTimeout(t); entry.inflight--; reject(e);
    }
  });
}

// The fallback when no worker is available; post-processed like the worker's when post is set.
function renderVoiceSync(m, renderFn, sr, frames, paramArr, allocCount, post) {
  const ptr = m._malloc(frames * 4);
  if (!ptr) throw new Error('Failed to allocate memory for audio buffer.');
  let paramsPtr = 0;
  try {
    // Every render passes a full param block: the engine has no defaults.
    if (!paramArr) throw new Error('renderVoiceSync: no param block for ' + renderFn);
    const count = Math.max(paramArr.length, allocCount || 0);
    // Emscripten malloc is 8-byte aligned, so >> 3 is exact.
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

// Filled by Go through setInstrumentBlock: audio.js keeps no parameter maps or instrument table of its own.
const RENDER = {};
const RENDER_INFO = {};

// Melodic voices render AT the played pitch, as on desktop, so the formant stays at an absolute Hz instead of
// sliding with resampling. Keyed `${id}:${pitch}`; the set is each document's render.melodic.
const MELODIC_INSTRUMENTS = new Set();

function isMelodicInstrument(id) {
  return MELODIC_INSTRUMENTS.has(id);
}

// Test seam: production uses isMelodicInstrument directly.
export const __isMelodicInstrument = isMelodicInstrument;

// The EXACT pitch, as desktop renders it; only NaN and -0 are normalised, so equal pitches share a key.
function roundPitchForCache(pitch) {
  const p = Number.isFinite(pitch) ? pitch : 0;
  return p === 0 ? 0 : p;
}

// Non-melodic ids (and melodic at pitch 0) keep the bare-id key the other call sites use.
function cacheKeyFor(id, roundedPitch) {
  if (isMelodicInstrument(id) && roundedPitch !== 0) {
    return id + ':' + roundedPitch;
  }
  return id;
}

// The bare id and every per-pitch variant, so an edit re-renders every played pitch.
function evictRenderCache(id) {
  renderCache.delete(id);
  if (isMelodicInstrument(id)) {
    const prefix = id + ':';
    for (const key of renderCache.keys()) {
      if (key.startsWith(prefix)) renderCache.delete(key);
    }
  }
}

// Test-only: forces an instrument fully cold to measure switching to a never-rendered one.
if (typeof window !== 'undefined') {
  window.__evictRenderCacheForTest = (id) => {
    evictRenderCache(id);
    rawRenderCache.delete(id);
  };
}

// An edit bumps a per-instrument GENERATION instead of evicting: a record from an older generation is STALE but plays ON
// TIME while a background render replaces it (evicting left nothing playable and every note deferred off the grid).
// In-flight dedup plus generation equality coalesces a drag storm to one render per (id, pitch).
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

// The nearest cached pitch, resampled at the intended time while the exact pitch renders: a wrong formant for one note
// beats a note off the grid.
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

// Exported so browser tests can probe re-renders. Melodic non-zero pitches key `${id}:${roundedPitch}`, everything else
// the bare id.
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

// Test seam: what a channel does to a voice between playSound and the master (EQ, inserts, gain), so a capture that
// disagrees with its voice is explained rather than guessed at.
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
  // The master is 'main' only; an empty id names no channel and fails closed, since mapping it to master made a probe of a
  // nonexistent channel read the master and agree with every other tap.
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

// A chain outside Go's held set is RELEASED (stages disposed, output disconnected) only once nothing in it can sound: no
// voice, and every stage quiet for the hold plus the chain's memory (a delay's echo). Its settings stay; the next voice
// wakes it fresh. Desktop's mixer does the same.
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

// Insertion order is LRU order (a hit re-inserts). Over Go's budget the oldest go first, never a held instrument's, one with
// a voice in flight, or a user sample's PCM; nothing before a held set is declared.
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

// Every channel's inserts and EQ and the master soft clip are desktop's C on the beatmo-dsp worklet, configured entirely by
// Go. Channel: ingress → [synth an.] → [fx] → [preEQ an.] → [eq] → [an.] → gain → main; master: gain → [eq] → soft clip →
// masterOut. An edit is a message to a live node; only a channel's first fx/eq node rewires. No worklet: the ScriptProcessor fallback.

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

// The pitch shifter's read heads trail by up to one window, so the probe matches the onset that much later instead of
// calling the note silent.
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

// The ONLY parameter source a render reads; a melodic per-pitch render patches the pitch index of a copy.
const instrumentBlocks = new Map();

// A pushed block never evicts that PCM; Go drops the override when the synth should sound again.
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

// From Go at boot and on every document change. Marks renders STALE (generation bump), so the stale buffer keeps playing
// on time while the new one renders.
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

// Pre-renders a switched row's likely pitches off-thread so the first notes hit a warm cache. Best-effort: deduped, and a
// missed warm falls back to the on-demand render.
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

// The SAME key the trigger reads plus the staleness check, so the warm-up skips voices already cached.
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

// The callback fires ONCE PER ITEM, failures included, or the readiness gate waits out its deadline; `token` is passed back
// untouched. Go calls this once per item in a tight loop that never yields, so every call only ENQUEUES and a pump
// dispatches 3 jobs per slice (~one frame), then yields: inline dispatch froze the main thread for 0.5 s at import.
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
          // A backstop: each job already guards itself, but an escaped throw would abort the loop and leave queued items without done().
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
    // A stuck `true` would stop warm-up for the rest of the session.
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
        // A synchronous throw from one bad id would escape into the pump loop; a failed render still counts as done, or the gate waits
        // out its deadline.
        dbg('warm.pump.dispatch.error', { err: String(err) });
        fire(entry && entry[2]);
      }
    });
  }
  if (!warmPumpRunning) runWarmPump();
};

// Test-only: a fresh render's tail-energy summary. Window-scoped and underscored, never read in production.
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

// Test-only: pins applySampleEdit to Go's BakeSample on a deterministic input (noise-seeded renders are not reproducible).
window.__testApplySampleEdit = (arr, sr, editJSON) =>
  Array.from(applySampleEdit(Float32Array.from(arr), sr, JSON.parse(editJSON)));

// Mirrors Go's sampleEdits: the Sampler edit is applied to the fresh C render, keeping the instrument a SYNTH. Fields match
// Go's SampleEdit JSON plus FadeMs.
const instrumentSampleEdits = new Map();

// The Sampler's raw capture shows the source and overlays the edit itself, so it must not see renderCache's edited render.
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
  // A sample instrument's renderCache entry is its ONLY PCM copy; the edit applies only to re-derivable synth renders.
  // Generation bump, not evict: the stale render plays on time.
  if (RENDER[id]) {
    bumpInstrumentParamsGen(id);
  }
}

// The JS port of Go's BakeSample, in its fixed order (trim → reverse → resample → normalize → gain → fades); Math.fround
// mirrors Go's float32 at every step.
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

// Analysers are IN-LINE: a fan-out tap unreachable from the destination can be skipped by Chrome, its buffer all zeros.
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

// program is Go's BuildEQProgram or null. Same shape: retuned in place, state kept; a new shape starts fresh, as on desktop.
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
  // Reuse a matching analyser: channel switches re-issue all three enables, and rebuilding thrice raced a fresh tap that never
  // saw its ingress again.
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

// One AnalyserNode serves both domains with fftSize set by spectrum resolution, but the time-domain consumers need no more
// than they had, so the EXPORT is capped to the most recent samples.
const WAVE_EXPORT_DEFAULT = 512;

function fillSnapshotBuffersForChain(chain, analyserKey, timeBufKey, freqBufKey, bins) {
  const a = chain[analyserKey];
  // Buffers are PER ANALYSER: with taps of different fftSizes one shared array was a readback bug waiting for a reordering.
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
  // bins === 0: SCALARS ONLY (the Meters tab and non-detail rows), skipping the frequency read and wave copy.
  const scalarsOnly = bins === 0;
  if (!scalarsOnly) a.getFloatFrequencyData(freq);
  // rms and peak are measured over the SAME exported slice, so `wave` and `rms` describe one window.
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
  // No plain arrays: the Go bridge reads the U8 views.
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

// At the ingress, before FX and EQ: in WASM the anti-pop envelope is already in the voice, so StageSynth and StageAntiPop
// share this tap.
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

// audioReady warms the ENGINE, never an instrument (which voices matter is Go's): a couple of throwaway bare-voice renders
// per worker and on the main module compile and tier up the WASM, or the first notes are scheduled late. Nothing cached.
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

// Both render paths arrive post-processed (voice_post.js); then the Sampler edit, Go's BakeSample, on the fresh render.
// Shared with renderDocPCM, so the graph is the played voice.
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
  // The regular cache holds the EDITED buffer, so a raw render bypasses it for writes.
  const skipEdit = !!(opts && opts.skipEdit);
  const sampleEdit = skipEdit ? null : (instrumentSampleEdits.get(id) || null);
  // Test seam: engines disagree on the device rate, so a cross-engine comparison pins one. Never cached.
  const srOverride = (opts && Number.isFinite(opts.sampleRate) && opts.sampleRate > 0) ? opts.sampleRate : 0;
  const bypassCache = (skipEdit && instrumentSampleEdits.has(id)) || srOverride > 0;
  // 0 keeps the bare-id key.
  const roundedPitch = (opts && Number.isFinite(opts.pitch)) ? roundPitchForCache(opts.pitch) : 0;
  const atPitch = isMelodicInstrument(id) && roundedPitch !== 0;
  const cacheKey = atPitch ? cacheKeyFor(id, roundedPitch) : id;
  // Use getSampleRate() to avoid creating AudioContext prematurely.
  // The engine rate (CHAIN_SPEC.sampleRate), whatever the device runs at.
  const sr = Math.max(8000, Math.min(192000, srOverride || getSampleRate()));
  // Captured BEFORE the async render, so a mid-render edit leaves the new record stale and the next trigger re-renders.
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
  // Copied, never a heap view, so it can be cloned to the worker and reused by the fallback; a melodic render patches only the
  // copy's pitch.
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

  // The worker renders; the synchronous path is the degraded fallback. A FAILED render throws: the previous record keeps
  // playing on time, never a substitute voice, which would be another instrument.
  let data = null;
  let viaWorker = false;
  // P4: the worker applies the voice post-processing when it renders, so the
  // main thread's per-render O(frames) passes only run on the sync fallback.
  const post = voicePost(info);
  // Request to cached buffer as the trigger path feels it; queue depth sampled at request time.
  const renderT0 = (typeof performance !== 'undefined' && performance.now) ? performance.now() : 0;
  const renderQueueDepth = synthWorkerPending.size;
  // During playback a slow worker render must never block the main thread: it throws "render deferred" and the neighbour
  // fallback or one dropped note covers it.
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
  // The AudioBuffer is made lazily, so this never forces a context into existence. gen is the generation it reflects;
  // viaWorker tells tests which path rendered.
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

// At play time only; WebAudio resamples if the buffer's rate differs from the context's.
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

// Replaces any synth render; wrapped lazily like rendered synths (no premature context on mobile). RENDER[id] and stale
// buffers are cleared so playback resolves the PCM.
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

// Restores the synth mapping so a factory reset makes the synth audible again; safe for pure user samples.
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

// null when nothing is rendered yet, starting a render so a retry succeeds. raw=true with an edit present captures the
// UN-edited render.
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

// The Synth tab's graph render: as a trigger plays it, Sampler edit included, never touching the playback cache, and it
// WAITS while any playback render is queued so a graph never delays a note.
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

// At the FILE'S OWN rate, channels averaged as desktop's decoder does; Go converts the rate with desktop's resampler, so the
// browser's never touches it.
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

// PCM is too large for localStorage, so the wasm SampleStore persists here: bytes are little-endian float32.
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

// Exported so test harnesses can force a re-render.
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

// fftSize sets frequency resolution, waveSamples the time-domain export, independently; Go passes both.
window.enableChannelAnalyzer = (id, fftSize, waveSamples) => {
  try { enableChannelAnalyzer(id, { fftSize, waveSamples }); } catch (err) { dbg('channel.analyzer.error', { id, err: String(err) }); }
};

// Go passes the analyser's own bin count: the grouping is LINEAR, so any downsample costs resolution where the bands are
// narrowest (a 64-bin default emptied every band under ~375 Hz).
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

// Flat typed arrays cut bridge overhead. Optional trailing args: rows/abss (sequencer identity for timingTraces, -1 =
// unrecorded), ideals/grids (probe timing), vids (lane voice ids; 0 = a preview).
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

// Mirrors stampedFileName: a colliding name is renamed by the browser, each engine spelling the uniquifier differently, so
// never ask for one that can collide.
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
    // '' only on a genuine cancel, never a blind timer: timers run while the modal dialog is open, and a timeout dropped the
    // real selection. Go's ~10 s guard covers browsers without the 'cancel' event.
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
// 0 before the chain exists; tests check pre-context effects applied after unlock.
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
    // cycles. Re-wiring masterOut → captureNode → destination fixes it.
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

// The tap posts blocks and this reply on the same FIFO port, so it lands after them; immediate for the ScriptProcessor
// fallback.
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

// The LIVE master chain for chain_spec_parity; forces the lazy nodes first. null with no context.
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

// Off the main thread end to end: per-channel worklets post batches straight to the encoder worker over transferred
// ports; the worker encodes and zips; the main thread only relays the final Blob into a download.

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
  // masterOut, after the soft clip (desktop records the same point), on a parallel path so the audible output is unaffected.
  return ensureMasterOut() || c.destination;  // fallback shouldn't happen in prod
}

// Resolves once the worker is ready and every channel configured; await it before treating the recording as started.
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

  // The capture node emits SILENCE, so connecting it to the destination keeps it in the render graph without adding signal.
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

// Flushes every worklet, finalizes in the worker, and returns once the Blob exists; the caller revokes blobURL.
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

// Non-blocking: the cached stats from the periodic requests.
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

// Renders notes through THIS module's graph on an OfflineAudioContext and returns the requested taps, for per-sample
// comparison with desktop; the live graph is set aside meanwhile. spec = {frames, sampleRate, channels | 'live',
// masterVolume, masterEQ, notes [{id, at, pitch, vol, dur, lane}], stops, taps [{name, channel, stage}]}.
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
