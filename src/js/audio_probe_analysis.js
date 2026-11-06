// audio_probe_analysis.js — pure analysis of a playback-probe log.
//
// Input: the raw log from `stopAudioProbe()` / `audioProbeRaw()` (audio.js):
//   requests[]  one per sequencer note that reached audio.js
//               {pk, id, row, abs, when, ideal, grid, ctx, perf}
//                 grid  = predictor trigger time (ctx s, groove, NO lookahead)
//                 ideal = scheduler's requested start, before any clamp
//                 when  = start after Go's dispatch floor
//                 ctx/perf = clocks when the request ARRIVED in audio.js
//   starts[]    one per BufferSource.start() of such a note
//               {pk, id, startWhen, ctx, perf, path}
//   reports{}   per tapped bus ('__master__' = final output) — the audio-thread
//               probe reports (audio_probe_worklet.js): onsets, clicks, cuts,
//               clipped, drift.
//   loopGaps[]  main-thread event-loop stalls > 25 ms {ctx, ms}
//
// Output: one report answering "what did the listener hear, and when,
// relative to what the predictor said should fire?" All times in ms.
//
// No DOM / WebAudio dependencies: Node benches import this directly and the
// page imports it lazily (audioProbeReport), so both run the same code.

const DEFAULTS = {
  preTolSec: 0.004,    // an onset may be detected this much before start()
  // …and at most this long after it. BufferSource.start(when) is sample-
  // accurate, so a real onset sits within the 5 ms anti-pop ramp + detector
  // latency of `startWhen`; anything later is some other event.
  // A channel whose insert chain delays its whole signal (a pitch shifter is
  // a 20-100 ms delay line) extends this per note by the latency audio.js
  // recorded at start() (starts[].fxLatency): the onset is then matched — the
  // note is HEARD, with its real lag in the lag stats — instead of being
  // called silent. Only a declared latency widens the window.
  maxLagSec: 0.02,
  // Unmatched note with the bus louder than this right after startWhen had
  // its onset MASKED by a still-sounding note (heard, just not separable);
  // below it the note is SILENT — the audio is genuinely missing.
  silentDb: -50,
  // A note requested at (near-)zero volume can't be told from a missing one.
  minJudgedVol: 0.05,
  edgeSec: 0.15,       // ignore notes this close to either end of the log
  mergeSec: 0.03,      // notes closer than the detector's refractory share an onset
  levelOutlierDb: 3,   // per-note level deviating more than this = inconsistent
  slipStepMs: 4,       // a clock-slip step smaller than this is device-burst noise
  // A note starting within this of an edit is "already started" for the
  // edit-staleness metric (the lane's cancel guard).
  editGuardSec: 0.008,
  // Notes this long after a vol edit are certainly fresh on every arm
  // (legacy lookahead ≤ 150 ms): they teach the channel's node volumes.
  editSettleSec: 0.3,
};

export function stats(values) {
  const v = values.filter((x) => Number.isFinite(x)).sort((a, b) => a - b);
  const n = v.length;
  if (n === 0) return { n: 0, min: null, mean: null, p50: null, p90: null, p95: null, p99: null, max: null, std: null };
  const q = (p) => v[Math.min(n - 1, Math.max(0, Math.ceil(p * n) - 1))];
  const mean = v.reduce((a, b) => a + b, 0) / n;
  const std = Math.sqrt(v.reduce((a, b) => a + (b - mean) * (b - mean), 0) / n);
  const r = (x) => Math.round(x * 1000) / 1000;
  return { n, min: r(v[0]), mean: r(mean), p50: r(q(0.5)), p90: r(q(0.9)), p95: r(q(0.95)), p99: r(q(0.99)), max: r(v[n - 1]), std: r(std) };
}

function median(v) {
  const s = v.slice().sort((a, b) => a - b);
  return s.length ? s[Math.floor(s.length / 2)] : NaN;
}

// Clock slips (audible dropouts): the per-report MINIMUM of wall-minus-audio
// drift is flat while the device is fed in time, and rises for good by the
// length of every underrun. A forward-min over a few reports rejects the
// device-burst jitter; each rise past slipStepMs is one dropout.
export function clockSlips(reports, slipStepMs = DEFAULTS.slipStepMs) {
  const d = (reports || []).map((r) => r.minDriftMs).filter((x) => Number.isFinite(x));
  if (d.length < 4) return { count: 0, totalMs: 0, maxMs: 0, driftMs: 0 };
  const W = 3;
  const env = [];
  for (let i = 0; i + W <= d.length; i++) env.push(Math.min(...d.slice(i, i + W)));
  // A rise spread over consecutive windows (the smoothing straddling one
  // underrun) is ONE dropout, sized from where it started.
  let count = 0, total = 0, max = 0, floor = env[0], evBase = null;
  for (let i = 1; i < env.length; i++) {
    const step = env[i] - floor;
    if (step > slipStepMs) {
      if (evBase == null) { count++; evBase = floor; }
      total += step;
      if (env[i] - evBase > max) max = env[i] - evBase;
      floor = env[i];
    } else {
      evBase = null;
      if (env[i] < floor) floor = env[i];
    }
  }
  const r = (x) => Math.round(x * 10) / 10;
  return { count, totalMs: r(total), maxMs: r(max), driftMs: r(env[env.length - 1] - env[0]) };
}

function busSummary(reports, sr) {
  let clicks = 0, cuts = 0, clipped = 0, sumSq = 0, n = 0, maxGapMs = 0;
  const onsets = [];
  const env = []; // [{start, frames, data}] — coarse peak envelope
  let start = Infinity, end = -Infinity;
  for (const r of reports || []) {
    clicks += r.clicks.length;
    cuts += r.cuts.length;
    clipped += r.clipped;
    sumSq += r.sumSq;
    n += r.n;
    if (r.maxGapMs > maxGapMs) maxGapMs = r.maxGapMs;
    if (r.startFrame < start) start = r.startFrame;
    if (r.endFrame > end) end = r.endFrame;
    for (const o of r.onsets) onsets.push({ t: o.f / sr, peak: o.peak, used: false });
    if (r.env && r.envFrames) env.push({ start: r.envStart, frames: r.envFrames, data: r.env });
  }
  onsets.sort((a, b) => a.t - b.t);
  const rms = n ? Math.sqrt(sumSq / n) : 0;
  return {
    onsets, clicks, cuts, clipped, maxGapMs, env,
    rmsDb: rms > 0 ? Math.round(20 * Math.log10(rms) * 10) / 10 : null,
    startSec: Number.isFinite(start) ? start / sr : null,
    endSec: Number.isFinite(end) ? end / sr : null,
  };
}

// Peak of the bus envelope over [t0, t1) seconds (0 when not covered).
function envPeak(bus, t0, t1, sr) {
  let peak = 0;
  const f0 = t0 * sr, f1 = t1 * sr;
  for (const seg of bus.env) {
    const segEnd = seg.start + seg.data.length * seg.frames;
    if (segEnd <= f0 || seg.start >= f1) continue;
    const i0 = Math.max(0, Math.floor((f0 - seg.start) / seg.frames));
    const i1 = Math.min(seg.data.length, Math.ceil((f1 - seg.start) / seg.frames));
    for (let i = i0; i < i1; i++) if (seg.data[i] > peak) peak = seg.data[i];
  }
  return peak;
}

export function analyzeAudioProbe(raw, opts = {}) {
  const o = { ...DEFAULTS, ...opts };
  const sr = raw.sampleRate || 48000;
  const reqByPk = new Map();
  for (const r of raw.requests || []) reqByPk.set(r.pk, r);
  // Lane voices cancelled BEFORE they started are intentionally silent: they
  // are neither "missed" nor "dropped".
  const cancelAt = new Map();
  for (const c of raw.cancels || []) if (!cancelAt.has(c.pk)) cancelAt.set(c.pk, c.ctx);
  let cancelledCount = 0;
  const startByPk = new Map();
  let duplicateStarts = 0;
  for (const s of raw.starts || []) {
    if (startByPk.has(s.pk)) {
      // A lane voice swapped onto its fresh render re-starts at the same
      // `when`: the later start is the one heard, not a duplicate.
      if (s.path === "lane-fresh") startByPk.set(s.pk, s);
      else duplicateStarts++;
      continue;
    }
    startByPk.set(s.pk, s);
  }

  const buses = {};
  for (const [id, reps] of Object.entries(raw.reports || {})) buses[id] = busSummary(reps, sr);
  const master = buses.__master__ || null;

  // The window every tapped bus covers; notes outside it can't be judged.
  let covStart = -Infinity, covEnd = Infinity;
  for (const b of Object.values(buses)) {
    if (b.startSec != null) covStart = Math.max(covStart, b.startSec);
    if (b.endSec != null) covEnd = Math.min(covEnd, b.endSec);
  }
  const judgeFrom = covStart + o.edgeSec;
  const judgeTo = covEnd - o.edgeSec;

  // ── Per-note matching, per channel ──
  const notes = [];
  const byChannel = new Map();
  for (const s of startByPk.values()) {
    const req = reqByPk.get(s.pk);
    if (!req) continue;
    if (cancelAt.has(s.pk) && s.startWhen > cancelAt.get(s.pk)) { cancelledCount++; continue; }
    const n = { ...req, startWhen: s.startWhen, startCtx: s.ctx, startPerf: s.perf, path: s.path, fxLatency: s.fxLatency || 0, onset: null, peak: null, merged: false };
    notes.push(n);
    if (!byChannel.has(n.id)) byChannel.set(n.id, []);
    byChannel.get(n.id).push(n);
  }
  const perChannel = {};
  const silentLin = Math.pow(10, o.silentDb / 20);
  for (const [id, list] of byChannel) {
    list.sort((a, b) => a.startWhen - b.startWhen);
    const bus = buses[id];
    const pc = perChannel[id] = { notes: 0, heard: 0, heardLate: 0, masked: 0, silent: 0, spurious: 0, tapped: !!bus };
    if (!bus) continue;
    let oi = 0;
    let lastHeard = null;
    for (let i = 0; i < list.length; i++) {
      const n = list[i];
      if (n.startWhen < judgeFrom || n.startWhen > judgeTo) continue;
      pc.notes++;
      // A note starting within the detector's refractory of the previous
      // heard note shares its onset (a chord on one instrument, a flam).
      if (lastHeard && n.startWhen - lastHeard.startWhen < o.mergeSec) {
        n.merged = true;
        n.onset = lastHeard.onset;
        n.peak = lastHeard.peak;
        pc.heard++;
        continue;
      }
      const next = list[i + 1];
      const lo = n.startWhen - o.preTolSec;
      const fxLat = n.fxLatency || 0;
      let hi = n.startWhen + o.maxLagSec + fxLat;
      if (next && next.startWhen + fxLat - o.preTolSec < hi) hi = Math.max(lo + 0.001, next.startWhen + fxLat - o.preTolSec);
      while (oi < bus.onsets.length && bus.onsets[oi].t < lo) oi++;
      const on = oi < bus.onsets.length && bus.onsets[oi].t <= hi ? bus.onsets[oi] : null;
      if (on) {
        on.used = true;
        oi++;
        n.onset = on.t;
        n.peak = on.peak;
        pc.heard++;
        if (on.t > n.startWhen + o.maxLagSec) { n.heardLate = true; pc.heardLate++; }
        lastHeard = n;
      } else if ((n.vol ?? 1) >= o.minJudgedVol && bus.env.length && envPeak(bus, n.startWhen, n.startWhen + o.maxLagSec + fxLat + 0.01, sr) < silentLin) {
        n.silent = true;
        pc.silent++;
      } else {
        n.masked = true;
        pc.masked++;
      }
    }
    pc.spurious = bus.onsets.filter((x) => !x.used && x.t >= judgeFrom && x.t <= judgeTo).length;
  }

  const judged = notes.filter((n) => n.startWhen >= judgeFrom && n.startWhen <= judgeTo && perChannel[n.id]?.tapped);
  const heard = judged.filter((n) => n.onset != null);
  const silent = judged.filter((n) => n.silent).length;
  const masked = judged.filter((n) => n.masked).length;
  const ms = (x) => x * 1000;

  // Requested but never started (dropped on the floor) — judged on arrival time.
  const startedPks = new Set(startByPk.keys());
  const dropped = (raw.requests || []).filter((r) => !startedPks.has(r.pk) && !cancelAt.has(r.pk) &&
    Number.isFinite(r.ctx) && r.ctx >= judgeFrom && r.ctx <= judgeTo).length;

  // ── Timing ──
  // Engine timing is judged on notes whose channel adds no latency of its
  // own. A note behind a pitch shifter (starts[].fxLatency) sounds up to one
  // window late, and unevenly so — that is the EFFECT, reported on its own as
  // fxDelayed, never as the scheduler being late.
  const timed = heard.filter((n) => !(n.fxLatency > 0));
  const fxDelayedNotes = heard.filter((n) => n.fxLatency > 0 && n.ideal != null);
  const lagVsGrid = timed.filter((n) => n.grid != null).map((n) => ms(n.onset - n.grid));
  const lagVsIdeal = timed.filter((n) => n.ideal != null).map((n) => ms(n.onset - n.ideal));
  const startLate = judged.filter((n) => n.ideal != null).map((n) => ms(n.startWhen - n.ideal));
  const detect = timed.filter((n) => !n.merged).map((n) => ms(n.onset - n.startWhen));
  const arrivalLead = judged.filter((n) => n.ideal != null).map((n) => ms(n.ideal - n.ctx));
  // Notes that audio.js can start synchronously (render-cache hit, raw sample,
  // neighbour-pitch fallback) vs ones that legitimately wait for a worker
  // render (cold miss, stale-while-revalidate race).
  const SYNC_PATHS = new Set(['hit', 'sample', 'neighbor']);
  const jsQueue = judged.filter((n) => SYNC_PATHS.has(n.path)).map((n) => n.startPerf - n.perf);
  const renderWait = judged.filter((n) => !SYNC_PATHS.has(n.path)).map((n) => n.startPerf - n.perf);
  const lateBy = (thr) => lagVsIdeal.filter((x) => x > thr).length;

  // Evenness: inter-onset interval error vs the predictor's intervals, per
  // channel. A late note, a lookahead jump, a slip — all make the rhythm
  // uneven, which is what "stutter" sounds like.
  const ioiErr = [];
  for (const list of byChannel.values()) {
    let prev = null;
    for (const n of list) {
      if (n.onset == null || n.grid == null || n.merged || n.fxLatency > 0) { continue; }
      if (prev && n.grid > prev.grid) ioiErr.push(Math.abs(ms((n.onset - prev.onset) - (n.grid - prev.grid))));
      prev = n;
    }
  }

  // Lookahead retargets: the cushion (ideal - grid) changing mid-play shifts
  // every later note by the delta — a tempo hiccup even with a perfect engine.
  const byTime = judged.filter((n) => n.grid != null && n.ideal != null).sort((a, b) => a.grid - b.grid);
  let lookaheadShifts = 0, maxLookaheadShiftMs = 0;
  const lookaheads = [];
  for (let i = 0; i < byTime.length; i++) {
    const la = ms(byTime[i].ideal - byTime[i].grid);
    lookaheads.push(la);
    if (i > 0) {
      const d = Math.abs(la - ms(byTime[i - 1].ideal - byTime[i - 1].grid));
      if (d > 1) { lookaheadShifts++; if (d > maxLookaheadShiftMs) maxLookaheadShiftMs = d; }
    }
  }

  // ── Playback quality ──
  // Per-note level consistency: identical notes (id|vol|pitch) should peak at
  // the same level; a truncated, stale, neighbour-pitch or cut voice won't.
  const groups = new Map();
  for (const n of heard) {
    if (n.merged || !(n.peak > 0)) continue;
    const k = `${n.id}|${n.path}|${Number(n.vol ?? 1).toFixed(3)}|${Number(n.pitch ?? 0).toFixed(2)}`;
    if (!groups.has(k)) groups.set(k, []);
    groups.get(k).push(n.peak);
  }
  let levelJudged = 0, levelOutliers = 0;
  for (const peaks of groups.values()) {
    if (peaks.length < 4) continue;
    const m = median(peaks);
    for (const p of peaks) {
      levelJudged++;
      if (Math.abs(20 * Math.log10(p / m)) > o.levelOutlierDb) levelOutliers++;
    }
  }

  const channelGlitches = { clicks: 0, cuts: 0 };
  for (const [id, b] of Object.entries(buses)) {
    if (id === '__master__') continue;
    channelGlitches.clicks += b.clicks;
    channelGlitches.cuts += b.cuts;
  }
  const clockReports = raw.reports?.__master__ || Object.values(raw.reports || {})[0] || [];
  const slips = clockSlips(clockReports, o.slipStepMs);

  const pathCounts = {};
  for (const n of judged) pathCounts[n.path] = (pathCounts[n.path] || 0) + 1;

  // ── Edit staleness ── notes that START after an edit (+ guard) yet still
  // reflect the pre-edit state. Per kind:
  //   mute   — a sounding note on the muted channel (heard or masked)
  //   unmute — nothing is stale (missing notes show up as `missed`)
  //   vol    — a note whose vol is explained by the PREVIOUS row volume
  //            (prev × k) and not by the new one (value × k), for a node
  //            volume k of that channel. K is learned from SETTLED notes —
  //            ≥ editSettleSec after a vol edit (and before the next) or
  //            before the first one — so node volumes ≠ 1 are never mistaken
  //            for staleness; a note explained by neither stays unjudged.
  const marks = (o.editMarks || []).slice().sort((a, b) => a.t - b.t);
  const r6 = (x) => Math.round(x * 1e6) / 1e6;
  const nodeVolsByChannel = new Map();
  for (const ch of new Set(marks.filter((m) => m.kind === 'vol').map((m) => m.channel))) {
    const chMarks = marks.filter((m) => m.kind === 'vol' && m.channel === ch);
    const K = new Set();
    for (const n of judged) {
      if (n.id !== ch || !Number.isFinite(n.vol)) continue;
      let rowVol = null;
      if (n.startWhen < chMarks[0].t) rowVol = Number.isFinite(chMarks[0].prev) ? chMarks[0].prev : null;
      for (let i = 0; i < chMarks.length; i++) {
        const m = chMarks[i], next = chMarks[i + 1];
        if (n.startWhen >= m.t + o.editSettleSec && (!next || n.startWhen < next.t)) rowVol = m.value;
      }
      if (rowVol > 0) K.add(r6(n.vol / rowVol));
    }
    nodeVolsByChannel.set(ch, K);
  }
  const perKind = {};
  let staleTotal = 0;
  const staleMsAll = [];
  marks.forEach((m, mi) => {
    const until = mi + 1 < marks.length ? marks[mi + 1].t : Infinity;
    const after = judged.filter((n) => n.id === m.channel && n.startWhen > m.t + o.editGuardSec && n.startWhen < until);
    let stale = [];
    if (m.kind === 'mute') stale = after.filter((n) => n.onset != null || n.masked);
    else if (m.kind === 'vol' && Number.isFinite(m.prev) && m.prev > 0) {
      const K = nodeVolsByChannel.get(m.channel) || new Set();
      stale = after.filter((n) => !K.has(r6((n.vol ?? 1) / m.value)) && K.has(r6((n.vol ?? 1) / m.prev)));
    }
    const k = perKind[m.kind] = perKind[m.kind] || { count: 0, staleNotes: 0, maxStaleMs: 0 };
    k.count++;
    k.staleNotes += stale.length;
    for (const n of stale) {
      const ms = (n.startWhen - m.t) * 1000;
      staleMsAll.push(ms);
      if (ms > k.maxStaleMs) k.maxStaleMs = Math.round(ms * 10) / 10;
    }
    staleTotal += stale.length;
  });

  const gaps = (raw.loopGaps || []).map((g) => g.ms);
  return {
    windowSec: Number.isFinite(judgeTo - judgeFrom) ? Math.round((judgeTo - judgeFrom) * 100) / 100 : 0,
    outputLatencyMs: raw.outputLatency != null ? Math.round(ms(raw.outputLatency) * 10) / 10 : null,
    baseLatencyMs: raw.baseLatency != null ? Math.round(ms(raw.baseLatency) * 10) / 10 : null,
    notes: {
      requested: (raw.requests || []).length,
      judged: judged.length,
      // onset detected and matched → timing measured
      heard: heard.length,
      // …of which arrived after maxLagSec on a channel whose insert chain
      // declared latency (starts[].fxLatency): heard, its lag in the stats
      heardLate: heard.filter((n) => n.heardLate).length,
      // no separable onset but the bus was sounding → present, untimed
      masked,
      // bus silent where the note should be → the audio is MISSING
      missed: silent,
      dropped,
      // lane voices cancelled before they started (an edit replaced them)
      cancelled: cancelledCount,
      duplicateStarts,
      spurious: Object.values(perChannel).reduce((a, c) => a + c.spurious, 0),
      paths: pathCounts,
    },
    // onset − predictor trigger time: the full "predicted → heard" latency,
    // including the deliberate lookahead cushion (add outputLatencyMs for
    // the speaker). Its spread, not its level, is the defect signal.
    lagVsGridMs: stats(lagVsGrid),
    // onset − scheduler's request: unplanned lateness only.
    lagVsIdealMs: stats(lagVsIdeal),
    // Notes on channels whose insert chain delays the sound (a pitch shifter):
    // how late they are HEARD vs the scheduler request. Kept out of the engine
    // timing above; an effect with latency makes its row audibly late.
    fxDelayed: { notes: fxDelayedNotes.length, lagMs: stats(fxDelayedNotes.map((n) => ms(n.onset - n.ideal))) },
    late: { over5ms: lateBy(5), over10ms: lateBy(10), over20ms: lateBy(20), over50ms: lateBy(50) },
    // BufferSource.start(when) − request: lateness introduced before audio.
    startLateMs: stats(startLate),
    // how early the request reached audio.js (negative = already late)
    arrivalLeadMs: stats(arrivalLead),
    arrivedLate: arrivalLead.filter((x) => x < 0).length,
    // request arrival → start() call for notes audio.js can start at once:
    // pure queueing behind the main thread (should be ~0 — sync flush)
    jsQueueMs: stats(jsQueue),
    // request → start() for notes that waited on a worker render
    renderWaitMs: stats(renderWait),
    // onset − start(when): detector bias; a large value = audio late INSIDE the graph
    detectMs: stats(detect),
    ioiErrMs: stats(ioiErr),
    lookahead: { ms: stats(lookaheads), shifts: lookaheadShifts, maxShiftMs: Math.round(maxLookaheadShiftMs * 10) / 10 },
    quality: {
      levelJudged,
      levelOutliers,
      masterClicks: master ? master.clicks : null,
      masterCuts: master ? master.cuts : null,
      masterClipped: master ? master.clipped : null,
      masterRmsDb: master ? master.rmsDb : null,
      channelClicks: channelGlitches.clicks,
      channelCuts: channelGlitches.cuts,
    },
    audioThread: {
      slips: slips.count,
      slipMs: slips.totalMs,
      maxSlipMs: slips.maxMs,
      clockDriftMs: slips.driftMs,
      maxCallbackGapMs: Math.max(0, ...Object.values(buses).map((b) => b.maxGapMs)),
    },
    mainThread: {
      stalls: gaps.length,
      over50ms: gaps.filter((x) => x > 50).length,
      over100ms: gaps.filter((x) => x > 100).length,
      maxMs: Math.round((raw.loopMaxMs || 0) * 10) / 10,
    },
    // notes that started after an edit but still sounded like the old state
    edits: { count: marks.length, staleNotes: staleTotal, staleMs: stats(staleMsAll), perKind },
    perChannel,
    // signal glitches per tapped bus ('__master__' = final output)
    perBus: Object.fromEntries(Object.entries(buses).map(([id, b]) => [id, { clicks: b.clicks, cuts: b.cuts, clipped: b.clipped, rmsDb: b.rmsDb }])),
    truncated: !!raw.truncated,
  };
}

// One-line human summary for bench logs.
export function formatAudioProbeReport(r) {
  if (!r) return 'probe: no data';
  const f = (x) => (x == null ? 'n/a' : Number(x).toFixed(1));
  return [
    `notes heard ${r.notes.heard}/${r.notes.judged} (late-heard ${r.notes.heardLate ?? 0}, masked ${r.notes.masked}, missed ${r.notes.missed}, dropped ${r.notes.dropped}, spurious ${r.notes.spurious})`,
    `lag vs grid p50 ${f(r.lagVsGridMs.p50)} p99 ${f(r.lagVsGridMs.p99)} max ${f(r.lagVsGridMs.max)} ms`,
    `lag vs request p50 ${f(r.lagVsIdealMs.p50)} p99 ${f(r.lagVsIdealMs.p99)} max ${f(r.lagVsIdealMs.max)} (late>10ms ${r.late.over10ms}, >20ms ${r.late.over20ms})${r.fxDelayed && r.fxDelayed.notes ? ` | behind an effect's own latency: ${r.fxDelayed.notes} notes, lag p99 ${f(r.fxDelayed.lagMs.p99)} ms` : ''}`,
    `IOI err p95 ${f(r.ioiErrMs.p95)} max ${f(r.ioiErrMs.max)} ms`,
    `lookahead shifts ${r.lookahead.shifts} (max ${f(r.lookahead.maxShiftMs)} ms)`,
    `jsQueue p99 ${f(r.jsQueueMs.p99)} ms`,
    `slips ${r.audioThread.slips} (${f(r.audioThread.slipMs)} ms)`,
    `clicks ${r.quality.masterClicks}/${r.quality.channelClicks} cuts ${r.quality.masterCuts}/${r.quality.channelCuts} clip ${r.quality.masterClipped}`,
    `level outliers ${r.quality.levelOutliers}/${r.quality.levelJudged}`,
    `main stalls >50ms ${r.mainThread.over50ms} max ${f(r.mainThread.maxMs)} ms`,
    `edits ${r.edits.count} stale ${r.edits.staleNotes} (max ${f(r.edits.staleMs.max)} ms)`,
  ].join(' | ');
}
