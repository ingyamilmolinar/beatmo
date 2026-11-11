// scenarios/webaudio_output_capture.js
//
// Pure in-browser scenario bodies extracted from
// src/js/webaudio_output_capture.browser.test.js. The Playwright shell
// invokes them via page.evaluate(); the in-page test_runner.html invokes
// them directly. Same assertions, two execution shells.
//
// Each scenario assumes window already has the WASM-side exports
// (playSound, startOutputCapture, etc.) and an unlocked AudioContext.
// Setup (ctx unlock, ensure synth sample, retries) is the caller's job —
// the test_runner does that once before running the suite.

import { register } from "./registry.js";

const MAX_RETRIES = 3;

function rmsAndPeak(arr) {
  let peak = 0;
  let sumSq = 0;
  let nonZero = false;
  for (let i = 0; i < arr.length; i++) {
    const v = arr[i];
    const a = v < 0 ? -v : v;
    if (a > 0.0001) nonZero = true;
    if (a > peak) peak = a;
    sumSq += v * v;
  }
  const rms = arr.length ? Math.sqrt(sumSq / arr.length) : 0;
  return { rms, peak, nonZero };
}

async function warmupCapture() {
  // Mirrors the existing test's warmup loop — without it, the very first
  // playSound after a fresh AudioContext can land before the capture node
  // has fully connected, yielding zero samples.
  if (typeof window.ensureSynthSample === "function") {
    try { await window.ensureSynthSample("kick"); } catch (_) {}
  }
  window.startOutputCapture();
  window.playSound("kick", 1.0);
  const deadline = Date.now() + 5000;
  while (Date.now() < deadline) {
    const snap = window.getOutputCapture?.();
    if (snap && snap.length > 0) {
      for (let i = 0; i < snap.length; i++) {
        if (Math.abs(snap[i]) > 0.01) return true;
      }
    }
    await new Promise((r) => setTimeout(r, 100));
  }
  return false;
}

// ─── Scenario 1: capture round-trip ────────────────────────────────────────
export async function scenarioCaptureRoundtrip() {
  let lastObserved = null;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    if (attempt > 1 && typeof window.resetOutputCaptureNode === "function") {
      window.resetOutputCaptureNode();
    }
    const warmedUp = await warmupCapture();
    if (warmedUp) {
      await new Promise((r) => setTimeout(r, 300));
    } else {
      window.playSound("kick", 1.0);
      await new Promise((r) => setTimeout(r, 1500));
    }
    window.clearOutputCapture();

    for (let i = 0; i < 3; i++) {
      window.playSound("kick", 1.0);
      await new Promise((r) => setTimeout(r, 400));
    }
    const captured = window.stopOutputCapture();
    const { rms, peak, nonZero } = rmsAndPeak(captured);
    lastObserved = { attempt, samples: captured.length, rms, peak, nonZero };
    if (nonZero && captured.length > 0) {
      return { pass: true, ...lastObserved };
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  return {
    pass: false,
    reason: lastObserved && lastObserved.samples === 0 ? "zero_samples" : "all_samples_zero",
    ...lastObserved,
  };
}
register({
  suite: "webaudio_output_capture",
  name: "capture_roundtrip",
  fn: scenarioCaptureRoundtrip,
  description: "Start capture → play kick × 3 → stop → assert non-zero RMS",
});

// ─── Scenario 2: mid-stream capture growth ─────────────────────────────────
export async function scenarioMidStreamGrowth() {
  let last = null;
  for (let attempt = 1; attempt <= MAX_RETRIES; attempt++) {
    if (attempt > 1 && typeof window.resetOutputCaptureNode === "function") {
      window.resetOutputCaptureNode();
    }
    const warmedUp = await warmupCapture();
    if (!warmedUp) {
      window.playSound("kick", 1.0);
      await new Promise((r) => setTimeout(r, 1500));
    }
    window.clearOutputCapture();

    window.playSound("kick", 1.0);
    await new Promise((r) => setTimeout(r, 500));
    const midLen = window.getOutputCapture().length;

    window.playSound("kick", 1.0);
    await new Promise((r) => setTimeout(r, 500));
    const afterMoreLen = window.getOutputCapture().length;

    const finalLen = window.stopOutputCapture().length;
    last = { attempt, midLen, afterMoreLen, finalLen };

    if (midLen > 0 && afterMoreLen > midLen && finalLen >= afterMoreLen) {
      return { pass: true, ...last };
    }
    await new Promise((r) => setTimeout(r, 1000));
  }
  let reason = "unknown";
  if (!last) reason = "no_data";
  else if (last.midLen === 0) reason = "mid_stream_empty";
  else if (last.afterMoreLen <= last.midLen) reason = "buffer_did_not_grow";
  else if (last.finalLen < last.afterMoreLen) reason = "final_lt_after_more";
  return { pass: false, reason, ...last };
}
register({
  suite: "webaudio_output_capture",
  name: "mid_stream_growth",
  fn: scenarioMidStreamGrowth,
  description: "Snapshot capture buffer mid-stream and verify it keeps growing",
});

// ─── Scenario 3: capture stats shape ───────────────────────────────────────
export async function scenarioCaptureStats() {
  const before = window.getOutputCaptureStats();
  window.startOutputCapture();
  const during = window.getOutputCaptureStats();
  window.playSound("kick", 1.0);
  await new Promise((r) => setTimeout(r, 500));
  const withData = window.getOutputCaptureStats();
  window.stopOutputCapture();
  const after = window.getOutputCaptureStats();

  const failures = [];
  if (during.enabled !== true) failures.push("during.enabled !== true");
  if (after.enabled !== false) failures.push("after.enabled !== false");
  if (!Number.isFinite(withData.sampleRate) || withData.sampleRate <= 0) failures.push("sampleRate invalid");
  if (during.hasNode !== true) failures.push("during.hasNode !== true");
  if (withData.samples > 0 && withData.durationSec <= 0) failures.push("durationSec invalid");

  return {
    pass: failures.length === 0,
    reason: failures[0] || null,
    failures,
    sampleRate: withData.sampleRate,
    samples: withData.samples,
    durationSec: withData.durationSec,
  };
}
register({
  suite: "webaudio_output_capture",
  name: "capture_stats",
  fn: scenarioCaptureStats,
  description: "getOutputCaptureStats returns correct enabled/hasNode/sampleRate",
});

// ─── Scenario 4: clear buffer ──────────────────────────────────────────────
export async function scenarioClearBuffer() {
  window.startOutputCapture();
  window.playSound("kick", 1.0);
  await new Promise((r) => setTimeout(r, 500));
  const beforeClear = window.getOutputCapture().length;
  window.clearOutputCapture();
  const afterClear = window.getOutputCapture().length;
  const stats = window.getOutputCaptureStats();
  window.stopOutputCapture();

  const ok = afterClear === 0 && stats.enabled === true;
  return {
    pass: ok,
    reason: !ok ? (afterClear !== 0 ? "buffer_not_empty_after_clear" : "enabled_flipped_off") : null,
    beforeClear,
    afterClear,
    enabledAfterClear: stats.enabled,
  };
}
register({
  suite: "webaudio_output_capture",
  name: "clear_buffer",
  fn: scenarioClearBuffer,
  description: "clearOutputCapture empties buffer without disabling capture",
});

// ─── Scenario 5: reset capture node ────────────────────────────────────────
export async function scenarioResetNode() {
  const before = window.getOutputCaptureStats();
  window.resetOutputCaptureNode();
  const after = window.getOutputCaptureStats();
  const ok = after.hasNode === false && after.enabled === false && after.samples === 0;
  return {
    pass: ok,
    reason: !ok ? "stats_not_zeroed_after_reset" : null,
    hasNodeBefore: before.hasNode,
    hasNodeAfter: after.hasNode,
    enabledAfter: after.enabled,
    samplesAfter: after.samples,
  };
}
register({
  suite: "webaudio_output_capture",
  name: "reset_node",
  fn: scenarioResetNode,
  description: "resetOutputCaptureNode tears down the node and zeros stats",
});
