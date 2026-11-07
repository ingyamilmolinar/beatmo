// The demo with every instrument carrying insert effects, shaped EQ, HPF+LPF and every synth stage on: end-to-end
// latency and audio consistency (clock drift, gaps, clicks in the recorded master), thresholds tight on purpose. Stage A
// is the reliable signal: Ebiten draw saturates the single WASM thread, so the sequencer fires late regardless of the chain.

import { chromium } from "playwright";
import { spawnSync } from "child_process";
import http from "http";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";
import { resolveGoBinary, shouldSkipWasmBuild, flushCoverage, isCoverageEnabled } from "./browser_test_helpers.js";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const jsDir = __dirname;
const goDir = path.resolve(jsDir, "../go");
const GO = resolveGoBinary();

const BPM = Number(process.env.FX_STRESS_BPM ?? "180");
const DURATION_MS = Number(process.env.FX_STRESS_DURATION_MS ?? "12000");
const OVERDUE_MAX = Number(process.env.FX_STRESS_OVERDUE_MAX ?? "0");
const SMALL_LEAD_MAX = Number(process.env.FX_STRESS_SMALL_LEAD_MAX ?? "0");
const QLAT_MAX_MS = Number(process.env.FX_STRESS_QLAT_MAX_MS ?? "8");
const DRIFT_MAX = Number(process.env.FX_STRESS_DRIFT_MAX ?? "0.02");
const GAP_MAX_MS = Number(process.env.FX_STRESS_GAP_MAX_MS ?? "120");
const SILENT_FRAC_MAX = Number(process.env.FX_STRESS_SILENT_FRAC_MAX ?? "0.05");
const LAG_MAX_MS = Number(process.env.FX_STRESS_LAG_MAX_MS ?? "1");
// 0 isolates the worklet load from the rest of the heavy circuit.
const FX_COUNT = Math.max(0, Math.min(3, Number(process.env.FX_STRESS_FX_COUNT ?? "3")));
const STAGEA_P99_MS = Number(process.env.FX_STRESS_STAGEA_P99_MS ?? "50");
const STAGEA_MAX_MS = Number(process.env.FX_STRESS_STAGEA_MAX_MS ?? "120");

if (!shouldSkipWasmBuild("main.wasm")) {
  const build = spawnSync(
    GO,
    ["build", "-ldflags", "-X main.defaultLog=INFO", "-o", path.join(jsDir, "main.wasm"), "./cmd"],
    { cwd: goDir, env: { ...process.env, GOOS: "js", GOARCH: "wasm" }, stdio: "inherit" },
  );
  if (build.status !== 0) throw new Error("go build main wasm failed");
}

const server = http.createServer((req, res) => {
  const file = req.url === "/" ? "/index.html" : req.url;
  const filePath = path.join(jsDir, file.replace(/^\//, ""));
  fs.readFile(filePath, (err, data) => {
    if (err) { res.writeHead(404); res.end(); return; }
    let ct = "text/plain";
    if (filePath.endsWith(".html")) ct = "text/html";
    else if (filePath.endsWith(".js")) ct = "application/javascript";
    else if (filePath.endsWith(".wasm")) ct = "application/wasm";
    res.writeHead(200, { "Content-Type": ct });
    res.end(data);
  });
});
await new Promise((r) => server.listen(0, r));
const port = server.address().port;

const failures = [];
function check(name, cond, detail = "") {
  if (cond) console.log(`✓ ${name}`);
  else { console.error(`✗ ${name} — ${detail}`); failures.push(name); }
}

// The same shape as recording_lifecycle's.
function parseZip(buf) {
  const u8 = new Uint8Array(buf);
  const dv = new DataView(u8.buffer, u8.byteOffset, u8.byteLength);
  let eocd = -1;
  for (let i = u8.length - 22; i >= 0; i--) {
    if (dv.getUint32(i, true) === 0x06054b50) { eocd = i; break; }
  }
  if (eocd <= 0) throw new Error("ZIP: EOCD not found");
  const cdEntries = dv.getUint16(eocd + 10, true);
  const cdOff = dv.getUint32(eocd + 16, true);
  const out = {};
  let p = cdOff;
  for (let i = 0; i < cdEntries; i++) {
    const uncompSize = dv.getUint32(p + 24, true);
    const nameLen = dv.getUint16(p + 28, true);
    const extraLen = dv.getUint16(p + 30, true);
    const commentLen = dv.getUint16(p + 32, true);
    const localOff = dv.getUint32(p + 42, true);
    const name = new TextDecoder().decode(u8.subarray(p + 46, p + 46 + nameLen));
    p += 46 + nameLen + extraLen + commentLen;
    const lNameLen = dv.getUint16(localOff + 26, true);
    const lExtraLen = dv.getUint16(localOff + 28, true);
    const dataStart = localOff + 30 + lNameLen + lExtraLen;
    out[name] = u8.subarray(dataStart, dataStart + uncompSize);
  }
  return out;
}

function decodeWav(buf) {
  const dv = new DataView(buf.buffer, buf.byteOffset, buf.byteLength);
  const fmtCode = dv.getUint16(20, true);
  const channels = dv.getUint16(22, true);
  const sampleRate = dv.getUint32(24, true);
  const bitDepth = dv.getUint16(34, true);
  let p = 36;
  while (p < buf.length) {
    const id = dv.getUint32(p, false);
    const size = dv.getUint32(p + 4, true);
    p += 8;
    if (id === 0x64617461) {
      let samples;
      if (fmtCode === 3 && bitDepth === 32) {
        const n = size / 4;
        samples = new Float32Array(n);
        for (let i = 0; i < n; i++) samples[i] = dv.getFloat32(p + i * 4, true);
      } else if (fmtCode === 1 && bitDepth === 24) {
        const n = Math.floor(size / 3);
        samples = new Float32Array(n);
        for (let i = 0; i < n; i++) {
          let v = buf[p + i * 3] | (buf[p + i * 3 + 1] << 8) | (buf[p + i * 3 + 2] << 16);
          if (v & 0x800000) v -= 0x1000000;
          samples[i] = v / 8388608;
        }
      } else if (fmtCode === 1 && bitDepth === 16) {
        const n = size / 2;
        samples = new Float32Array(n);
        for (let i = 0; i < n; i++) samples[i] = dv.getInt16(p + i * 2, true) / 32768;
      } else {
        throw new Error(`WAV: unsupported fmt=${fmtCode} bits=${bitDepth}`);
      }
      return { sampleRate, channels, bitDepth, samples };
    }
    p += size;
  }
  throw new Error("WAV: no data chunk");
}

// Consistency metrics over the captured master: silent-gap windows + clicks.
function masterConsistency(wav) {
  const { samples, sampleRate, channels } = wav;
  const frames = Math.floor(samples.length / channels);
  const win = Math.floor(sampleRate * 0.02); // 20ms windows
  const nWin = Math.floor(frames / win);
  const winRMS = new Float64Array(nWin);
  for (let w = 0; w < nWin; w++) {
    let sum = 0;
    for (let i = w * win; i < (w + 1) * win; i++) {
      const v = samples[i * channels]; // L channel
      sum += v * v;
    }
    winRMS[w] = Math.sqrt(sum / win);
  }
  const sorted = Array.from(winRMS).sort((a, b) => a - b);
  const median = sorted[Math.floor(sorted.length / 2)] || 0;
  const floor = Math.max(median * 0.02, 1e-5);
  // Skip leading windows before first signal (recording starts before audio).
  let first = 0;
  while (first < nWin && winRMS[first] < floor) first++;
  let last = nWin - 1;
  while (last > first && winRMS[last] < floor) last--;
  let silent = 0;
  let gapRun = 0;
  let longestGapMs = 0;
  for (let w = first; w <= last; w++) {
    if (winRMS[w] < floor) {
      silent++;
      gapRun++;
      longestGapMs = Math.max(longestGapMs, gapRun * 20);
    } else {
      gapRun = 0;
    }
  }
  const active = last - first + 1;
  // Jumps no rendered voice chain produces: underruns splicing the stream.
  let clicks = 0;
  let maxJump = 0;
  for (let i = 1; i < frames; i++) {
    const d = Math.abs(samples[i * channels] - samples[(i - 1) * channels]);
    if (d > maxJump) maxJump = d;
    if (d > 0.6) clicks++;
  }
  let rms = 0;
  for (let i = 0; i < frames; i++) rms += samples[i * channels] * samples[i * channels];
  rms = Math.sqrt(rms / frames);
  return {
    rms, median, floor,
    windows: nWin, activeWindows: active,
    silentWindows: silent, silentFrac: active > 0 ? silent / active : 1,
    longestGapMs, clicks, maxJump,
    durationSec: frames / sampleRate,
  };
}

let browser;
try {
  browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
  const page = await browser.newPage();
  if (process.env.TEST_LOG) page.on("console", (m) => console.log(`  [page] ${m.text()}`));
  // Keyed by BEATMO_* names, into window.__beatmoEnv before init.
  if (process.env.BEATMO_ENV_OVERRIDE) {
    try {
      const ov = JSON.parse(process.env.BEATMO_ENV_OVERRIDE);
      console.log(`[fx-stress] env override: ${JSON.stringify(ov)}`);
      // Non-strings are silently coerced.
      await page.addInitScript((cfg) => {
        window.__beatmoEnv = Object.fromEntries(
          Object.entries(cfg).map(([k, v]) => [k, String(v)]),
        );
      }, ov);
    } catch (e) {
      console.warn(`[fx-stress] invalid BEATMO_ENV_OVERRIDE: ${e.message}`);
    }
  }
  await page.goto(`http://localhost:${port}/`);
  await page.waitForFunction(() => typeof startPlay === "function" && typeof exportJSON === "function" && typeof startRecording === "function");
  await page.waitForTimeout(300);

  // ── Build the heavy circuit: export the demo, crank every instrument ──
  const importErr = await page.evaluate((fxCount) => {
    // Through each instrument's document, so the export carries them in instrument_docs.
    const tweaks = {
      osc_enabled: 1, fm_enabled: 1, env_enabled: 1, filter_enabled: 1, drive_enabled: 1,
      post_decay: 0.8,
      pitchenv_enabled: 1, pitchenv_amt: 0.4, pitchenv_decay: 0.3,
      lfo_enabled: 1, lfo_rate: 4, lfo_depth: 0.3,
      burst_enabled: 1, burst_sharp: 0.5,
    };
    for (const inst of JSON.parse(exportJSON()).instruments || []) {
      for (const [k, v] of Object.entries(tweaks)) setInstrumentParam(inst.id, k, v);
    }
    const doc = JSON.parse(exportJSON());
    const fxPool = [
      ["distortion", "delay", "reverb"],
      ["chorus", "filter", "compressor"],
      ["phaser", "flanger", "tremolo"],
      ["bitcrusher", "tape", "limiter"],
    ];
    const eqGains = [3, -2, 2, -1, 1, 2, -2, 2, -3, 2];
    doc.instruments = (doc.instruments || []).map((inst, i) => {
      const fx = fxPool[i % fxPool.length].slice(0, fxCount).map((type) => ({ type, enabled: true }));
      return Object.assign({}, inst, {
        effects: fx,
        eq: {
          gains_db: eqGains,
          hpf_enabled: true, hpf_cutoff_hz: 60,
          lpf_enabled: true, lpf_cutoff_hz: 12000,
        },
      });
    });
    doc.eq = Object.assign({}, doc.eq || {}, { gains_db: eqGains });
    window.__fxStressDoc = doc; // keep for debugging
    return importJSON(JSON.stringify(doc));
  }, FX_COUNT);
  check("heavy circuit imports cleanly", importErr === "", `importJSON returned ${JSON.stringify(importErr)}`);
  if (importErr !== "") throw new Error("import failed; aborting");

  // Let the import settle (UI rebuild + render cache warm).
  await page.evaluate(() => { forceDraw?.(); forceDraw?.(); });
  await page.waitForTimeout(800);

  const fxSummary = await page.evaluate(() => {
    const doc = JSON.parse(exportJSON());
    return (doc.instruments || []).map((i) => ({
      id: i.id,
      fx: (i.effects || []).map((e) => e.type).join("+"),
      hpf: !!(i.eq && i.eq.hpf_enabled),
      lpf: !!(i.eq && i.eq.lpf_enabled),
      // A stage the instrument ships ON counts like one turned on.
      modulators: ["pitchenv_enabled", "lfo_enabled", "burst_enabled"]
        .filter((k) => ((getInstrumentParams(i.id) || {})[k] || 0) >= 0.5).length,
    }));
  });
  console.log("[fx-stress] circuit:", JSON.stringify(fxSummary));
  check(
    "every instrument carries effects + HPF + LPF + modulator stages",
    fxSummary.length > 0 && fxSummary.every((s) => s.fx.length > 0 && s.hpf && s.lpf && s.modulators === 3),
    JSON.stringify(fxSummary),
  );

  // Play is ignored while the warm-up gate is closed.
  await page.waitForFunction(() => typeof warmupProgress !== "function" || warmupProgress().ready === true, null, { timeout: 30000, polling: 100 });
  const downloadPromise = page.waitForEvent("download", { timeout: 30000 });
  const clocks0 = await page.evaluate(async (bpm) => {
    document.dispatchEvent(new Event("pointerdown"));
    resumeAudio?.();
    resetPerfStats?.();
    resetAudioScheduleMetrics?.();
    resetThreeStageLatency?.();
    setBPM(bpm);
    startPlay();
    await window.startRecording("wav24");
    return { wall: performance.now(), audio: window.__audioCtx ? window.__audioCtx.currentTime : null };
  }, BPM);

  // Tells SUSTAINED drift from the worklet-init transient.
  const driftSeries = [];
  let prevSample = clocks0;
  const secs = Math.round(DURATION_MS / 1000);
  for (let s = 0; s < secs; s++) {
    await page.waitForTimeout(1000);
    const cur = await page.evaluate(() => ({
      wall: performance.now(),
      audio: window.__audioCtx ? window.__audioCtx.currentTime : null,
    }));
    if (prevSample.audio != null && cur.audio != null) {
      const wd = (cur.wall - prevSample.wall) / 1000;
      const ad = cur.audio - prevSample.audio;
      driftSeries.push(wd > 0 ? +(((wd - ad) / wd) * 100).toFixed(2) : 0);
    }
    prevSample = cur;
  }
  console.log(`[fx-stress] per-second drift%: [${driftSeries.join(", ")}]`);

  const clocks1 = prevSample;
  const result = await page.evaluate(async () => {
    stopPlay();
    const rec = await window.stopRecording();
    return {
      rec,
      perf: perfStats(),
      sched: getAudioScheduleMetrics(),
      threeStage: typeof getThreeStageLatency === "function" ? getThreeStageLatency() : null,
    };
  });

  console.log("[fx-stress] perfStats:", JSON.stringify(result.perf));
  console.log("[fx-stress] schedMetrics:", JSON.stringify(result.sched));
  console.log("[fx-stress] threeStage:", JSON.stringify(result.threeStage));

  // audioEnq counts events and audioDeq batches, so 6:1 is normal batching; audioQLatMax is the queue signal.
  const sched = result.sched || {};
  check("audio events flowed", (sched.count ?? 0) > 50, `count=${sched.count}`);
  check(`overdue ≤ ${OVERDUE_MAX}`, (sched.overdue ?? 1e9) <= OVERDUE_MAX, `overdue=${sched.overdue}`);
  check(`smallLeadCount ≤ ${SMALL_LEAD_MAX}`, (sched.smallLeadCount ?? 1e9) <= SMALL_LEAD_MAX, `smallLeadCount=${sched.smallLeadCount}`);
  check("minLead ≥ 3ms", (sched.minLead ?? -1) >= 0.003, `minLead=${sched.minLead}`);
  const qlat = result.perf?.audioQLatMax ?? 1e9;
  check(`audioQLatMax ≤ ${QLAT_MAX_MS}ms`, qlat <= QLAT_MAX_MS, `audioQLatMax=${qlat}ms`);
  if (result.threeStage && result.threeStage.lead) {
    const lagMax = (result.threeStage.lead.maxLag ?? 0) * 1000;
    check(`Stage C lag ≤ ${LAG_MAX_MS}ms`, lagMax <= LAG_MAX_MS, `maxLag=${lagMax.toFixed(2)}ms`);
  }
  // A healthy sequencer fires within a tick or two of its deadline.
  if (result.threeStage && result.threeStage.seqFireLate) {
    const a = result.threeStage.seqFireLate;
    const p99 = (a.p99 ?? 1e9) * 1000;
    const max = (a.max ?? 1e9) * 1000;
    check(`Stage A seqFire p99 ≤ ${STAGEA_P99_MS}ms`, p99 <= STAGEA_P99_MS, `p99=${p99.toFixed(0)}ms`);
    check(`Stage A seqFire max ≤ ${STAGEA_MAX_MS}ms`, max <= STAGEA_MAX_MS, `max=${max.toFixed(0)}ms`);
  } else {
    check("Stage A metrics available", false, "getThreeStageLatency returned no seqFireLate");
  }

  // ── Render-thread realtime check ──
  if (clocks0.audio != null && clocks1.audio != null) {
    const wallSec = (clocks1.wall - clocks0.wall) / 1000;
    const audioSec = clocks1.audio - clocks0.audio;
    const drift = Math.abs(wallSec - audioSec) / wallSec;
    console.log(`[fx-stress] clocks: wall=${wallSec.toFixed(3)}s audio=${audioSec.toFixed(3)}s drift=${(drift * 100).toFixed(2)}%`);
    check(`audio clock keeps realtime (drift ≤ ${DRIFT_MAX * 100}%)`, drift <= DRIFT_MAX, `drift=${(drift * 100).toFixed(2)}%`);
  } else {
    check("audio clock readable", false, "window.audioCtx not exposed");
  }

  // ── Captured-audio consistency ──
  const download = await downloadPromise;
  const zipBytes = fs.readFileSync(await download.path());
  const entries = parseZip(zipBytes);
  const masterName = Object.keys(entries).find((n) => n.includes("master"));
  check("master capture present in zip", !!masterName, `entries=${Object.keys(entries).join(",")}`);
  if (masterName) {
    const wav = decodeWav(entries[masterName]);
    const m = masterConsistency(wav);
    console.log("[fx-stress] master:", JSON.stringify({ ...m, rms: m.rms.toFixed(5), median: m.median.toFixed(5) }));
    check("master has real signal (RMS ≥ 1e-4)", m.rms >= 1e-4, `rms=${m.rms}`);
    check(
      `captured duration ≈ recorded window`,
      m.durationSec >= (DURATION_MS / 1000) * 0.9,
      `captured=${m.durationSec.toFixed(2)}s window=${(DURATION_MS / 1000).toFixed(2)}s`,
    );
    check(`longest silent gap ≤ ${GAP_MAX_MS}ms`, m.longestGapMs <= GAP_MAX_MS, `longestGapMs=${m.longestGapMs}`);
    check(
      `silent fraction ≤ ${SILENT_FRAC_MAX * 100}%`,
      m.silentFrac <= SILENT_FRAC_MAX,
      `silentFrac=${(m.silentFrac * 100).toFixed(1)}% (${m.silentWindows}/${m.activeWindows} windows)`,
    );
    check("no hard discontinuities (clicks)", m.clicks === 0, `clicks=${m.clicks} maxJump=${m.maxJump.toFixed(3)}`);
  }

  if (isCoverageEnabled()) await flushCoverage(page, new URL("../../coverage/browser-raw", import.meta.url).pathname, "fx_chain_stress");

  console.log(`\n[fx-stress] ${failures.length === 0 ? "PASS" : `FAIL (${failures.length}): ${failures.join("; ")}`}`);
} catch (err) {
  console.error("FATAL:", err);
  failures.push(String(err));
} finally {
  if (browser) await browser.close();
  server.close();
}
process.exit(failures.length === 0 ? 0 : 1);
