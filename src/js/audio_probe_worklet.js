/**
 * Audio playback probe — AudioWorkletProcessor `beatmo-audio-probe`.
 *
 * Measures what the listener actually HEARS on one bus (a channel or the
 * master), from the audio rendering thread. Everything upstream of this node
 * (Go predictor → scheduler → syscall/js bridge → audio.js queue →
 * BufferSource.start) only tells us what was ASKED for; this tap reports what
 * the render graph actually produced and when, so a note that was scheduled
 * but never sounded, sounded late, or sounded with a glitch is visible.
 *
 * Runs on the audio thread, so a stalled main thread (Go-WASM GC, a slow
 * Ebiten frame) cannot delay or distort its readings — probed 2026-09-30: a
 * 300 ms main-thread busy-loop left this processor's callback cadence
 * untouched (max inter-callback gap 13 ms, the 512-frame device burst).
 *
 * Acoustically transparent: zero outputs, never writes audio. A zero-output
 * AudioWorkletNode is an "automatic pull" node in Chromium, so it is rendered
 * without being wired to ctx.destination.
 *
 * Per-quantum work is O(128) with no allocation in steady state; results go
 * to the main thread as one small message every `reportFrames` frames.
 *
 * What it detects (all positions are absolute context frames, i.e.
 * `currentFrame + i`, so onset time in ctx seconds = frame / sampleRate — the
 * same clock as BufferSource.start(when)):
 *
 *   onsets    — a hop whose peak rises `riseDb` above a peak-hold envelope
 *               and above `floorDb`. `peak` is the loudest sample in the
 *               `levelWindowSec` after the onset (per-note level, so a note
 *               that plays at the wrong level / gets cut shows up).
 *   clicks    — a sample-to-sample step far larger than the local mean
 *               derivative (a discontinuity: a voice cut mid-cycle, a node
 *               disconnected under a sounding voice). Heuristic; noise-like
 *               instruments raise the local derivative so they don't trip it.
 *   cuts      — a loud hop followed immediately by digital silence (a hard
 *               stop, not a decay).
 *   clipped   — samples at/over full scale.
 *   env       — the peak of every ENV_DECIM hops (64 frames, 1.3 ms @ 48 kHz),
 *               so the analysis can tell a note whose onset was MASKED by a
 *               still-ringing previous note (sound present) from one that is
 *               genuinely SILENT (sound missing).
 *   drift     — wall clock (Date.now) minus audio clock (frames/sampleRate)
 *               per callback, min/max per report. The render thread is pulled
 *               by the device; when it fails to deliver in time (underrun)
 *               the device plays silence and the context clock falls behind
 *               the wall clock for good, so a rise of the per-report MINIMUM
 *               drift is an audible dropout of that length. `performance` is
 *               not exposed in AudioWorkletGlobalScope; Date.now is (1 ms).
 *
 * Messages:
 *   Main → Worklet: { type: 'stop' }
 *   Worklet → Main: { type: 'report', channelId, startFrame, endFrame,
 *                     onsets: [{f, peak}], clicks: [f], cuts: [f],
 *                     env: Float32Array (one peak per ENV_FRAMES), envFrames,
 *                     clipped, sumSq, n, minDriftMs, maxDriftMs,
 *                     maxGapMs, callbacks }
 *                   { type: 'done', channelId }
 */

/* eslint-disable no-undef */

const HOP = 32; // onset/cut analysis granularity: 0.67 ms @ 48 kHz
const ENV_DECIM = 2; // hops per env point
const ENV_FRAMES = HOP * ENV_DECIM;

class AudioProbeProcessor extends AudioWorkletProcessor {
  constructor(options) {
    super();
    const o = (options && options.processorOptions) || {};
    this._channelId = o.channelId || '';
    this._floor = Math.pow(10, (Number.isFinite(o.floorDb) ? o.floorDb : -50) / 20);
    this._rise = Math.pow(10, (Number.isFinite(o.riseDb) ? o.riseDb : 4) / 20);
    this._refractory = Math.round((Number.isFinite(o.refractorySec) ? o.refractorySec : 0.03) * sampleRate);
    this._levelWindow = Math.round((Number.isFinite(o.levelWindowSec) ? o.levelWindowSec : 0.02) * sampleRate);
    this._reportFrames = Math.max(128, Math.round((Number.isFinite(o.reportSec) ? o.reportSec : 0.1) * sampleRate));
    this._clickAbs = Number.isFinite(o.clickAbs) ? o.clickAbs : 0.25;
    this._clickRatio = Number.isFinite(o.clickRatio) ? o.clickRatio : 12;
    // Peak-hold envelope release: ~50 ms time constant per hop.
    this._envRelease = Math.exp(-HOP / (sampleRate * 0.05));

    this._hold = 0;
    this._hopPeak = 0;
    this._hopFill = 0;
    this._prevHopPeak = 0;
    this._lastOnset = -Infinity;
    this._pending = null; // {f, peak, until}
    this._prev = 0;
    this._meanDeriv = 0; // one-pole mean of |x[n]-x[n-1]|
    this._derivCoef = 1 / 64;
    this._zeroRun = 0;
    this._loudBeforeRun = false;

    this._wall0 = -1;
    this._frame0 = 0;
    this._lastWall = -1;
    this._stopped = false;
    this._resetReport(currentFrame);

    this.port.onmessage = (e) => {
      const m = e.data;
      if (m && m.type === 'stop') {
        this._flushPending(true);
        this._post();
        this._stopped = true;
        this.port.postMessage({ type: 'done', channelId: this._channelId });
      }
    };
  }

  _resetReport(frame) {
    this._rStart = frame;
    this._onsets = [];
    this._clicks = [];
    this._cuts = [];
    this._clipped = 0;
    this._sumSq = 0;
    this._n = 0;
    this._minDrift = Infinity;
    this._maxDrift = -Infinity;
    this._maxGap = 0;
    this._callbacks = 0;
    // env point k covers ≈[envStart + k*ENV_FRAMES, +ENV_FRAMES) (± one hop:
    // hops are not phase-aligned to ENV_FRAMES; plenty for masked-vs-silent).
    this._envStart = Math.ceil(frame / ENV_FRAMES) * ENV_FRAMES;
    this._env = new Float32Array(Math.ceil(this._reportFrames / ENV_FRAMES) + 8);
    this._envN = 0;
    this._envAcc = 0;
    this._envHops = 0;
  }

  _post() {
    this.port.postMessage({
      type: 'report',
      channelId: this._channelId,
      startFrame: this._rStart,
      endFrame: currentFrame + 128,
      onsets: this._onsets,
      clicks: this._clicks,
      cuts: this._cuts,
      clipped: this._clipped,
      sumSq: this._sumSq,
      n: this._n,
      minDriftMs: Number.isFinite(this._minDrift) ? this._minDrift : null,
      maxDriftMs: Number.isFinite(this._maxDrift) ? this._maxDrift : null,
      maxGapMs: this._maxGap,
      callbacks: this._callbacks,
      env: this._env.slice(0, this._envN),
      envStart: this._envStart,
      envFrames: ENV_FRAMES,
    });
    this._resetReport(currentFrame + 128);
  }

  _flushPending(force, frame) {
    const p = this._pending;
    if (!p) return;
    if (force || frame >= p.until) {
      this._onsets.push({ f: p.f, peak: p.peak });
      this._pending = null;
    }
  }

  process(inputs) {
    if (this._stopped) return false;

    // Clock drift: wall vs audio. Sampled once per callback.
    const wall = Date.now();
    if (this._wall0 < 0) {
      this._wall0 = wall;
      this._frame0 = currentFrame;
    }
    const drift = (wall - this._wall0) - ((currentFrame - this._frame0) / sampleRate) * 1000;
    if (drift < this._minDrift) this._minDrift = drift;
    if (drift > this._maxDrift) this._maxDrift = drift;
    if (this._lastWall >= 0 && wall - this._lastWall > this._maxGap) this._maxGap = wall - this._lastWall;
    this._lastWall = wall;
    this._callbacks++;

    const input = inputs[0];
    const n = input && input[0] ? input[0].length : 128;
    const l = input && input[0];
    const r = input && input[1];
    const base = currentFrame;

    for (let i = 0; i < n; i++) {
      // Downmix: an unconnected input arrives as zero channels = silence.
      const x = l ? (r ? 0.5 * (l[i] + r[i]) : l[i]) : 0;
      const ax = x < 0 ? -x : x;
      this._sumSq += x * x;
      if (ax >= 0.999) this._clipped++;

      // Click: a step far above the local mean derivative.
      const d = x - this._prev;
      const ad = d < 0 ? -d : d;
      if (ad > this._clickAbs && ad > this._clickRatio * (this._meanDeriv + 1e-4)) {
        this._clicks.push(base + i);
      }
      this._meanDeriv += (ad - this._meanDeriv) * this._derivCoef;
      this._prev = x;

      // Per-note level: running peak of the pending onset's window.
      if (this._pending) {
        if (ax > this._pending.peak) this._pending.peak = ax;
      }

      if (ax > this._hopPeak) this._hopPeak = ax;
      if (++this._hopFill === HOP) {
        const f = base + i - HOP + 1; // first frame of this hop
        const hp = this._hopPeak;
        this._flushPending(false, f);
        if (hp > this._floor && hp > this._hold * this._rise && hp > this._prevHopPeak * 1.25 &&
            f - this._lastOnset >= this._refractory) {
          this._flushPending(true);
          this._pending = { f, peak: hp, until: f + this._levelWindow };
          this._lastOnset = f;
        }
        // Hard cut: this hop is digital silence right after a loud one.
        if (hp < 1e-6 && this._prevHopPeak > 0.02) this._cuts.push(f);
        this._hold = hp > this._hold ? hp : this._hold * this._envRelease;
        if (f >= this._envStart) {
          if (hp > this._envAcc) this._envAcc = hp;
          if (++this._envHops === ENV_DECIM) {
            if (this._envN < this._env.length) this._env[this._envN++] = this._envAcc;
            this._envAcc = 0;
            this._envHops = 0;
          }
        }
        this._prevHopPeak = hp;
        this._hopPeak = 0;
        this._hopFill = 0;
      }
    }
    this._n += n;

    if (currentFrame + 128 - this._rStart >= this._reportFrames) this._post();
    return true;
  }
}

registerProcessor('beatmo-audio-probe', AudioProbeProcessor);
