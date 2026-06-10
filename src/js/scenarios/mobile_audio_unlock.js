// scenarios/mobile_audio_unlock.js
//
// Pure in-browser scenarios extracted from
// src/js/mobile_audio_unlock.browser.test.js. Same assertions, two shells:
//
//   - The Playwright shell (the existing test file) wraps each scenario in
//     a per-context page.evaluate so the mobile-device emulation +
//     gesture-deferral plumbing keeps working.
//   - The in-page test_runner.html invokes them directly on the real phone,
//     where the AudioContext + touch unlock are already real.
//
// Only the two scenarios that are pure in-browser logic are factored here.
// Scenarios 1 (deferred AudioContext) and 3 (re-suspension recovery) require
// multi-context Playwright orchestration with synthetic suspends; they stay
// in the Playwright shell.

import { register } from "./registry.js";

// ─── Scenario 2: queued channel ops applied after unlock ───────────────────
// Verifies that setChannelVolume calls made before AudioContext creation are
// queued and applied once the context is running, AND that audio plays at
// the queued volume. The user's beatmo.io "click Play, no audio" failure
// could surface as either "queued vol never applied" or "audio path silent
// after unlock" — both are caught here.
export async function scenarioQueuedChannelOpsAppliedAfterUnlock() {
  // Queue the volume change first (may happen before or after ctx creation
  // depending on autoplay policy — either way pendingChannelOps handles it).
  try { window.setChannelVolume("kick", 0.5); } catch (e) { /* setter may not exist on stale builds */ }

  // Trigger context creation/resume via a synthetic gesture. On real iOS,
  // the user's tap already provided the gesture before this scenario runs.
  try {
    document.dispatchEvent(new Event("pointerdown"));
    window.resumeAudio?.();
  } catch (_) {}
  await new Promise((r) => setTimeout(r, 300));

  const vol = (typeof window.channelVolume === "function") ? window.channelVolume("kick") : NaN;
  if (!Number.isFinite(vol) || Math.abs(vol - 0.5) > 0.01) {
    return {
      pass: false,
      reason: "channel_volume_not_applied",
      expectedVolume: 0.5,
      observedVolume: vol,
    };
  }

  // Ensure the C synth sample is warmed up before capture so async render
  // doesn't push the trigger past the capture window.
  try { await window.audioReady; } catch (_) {}
  if (typeof window.ensureSynthSample === "function") {
    try { await window.ensureSynthSample("kick"); } catch (_) {}
  }

  // Use the recordSamples capture path (window.__samples) — it fires
  // synchronously inside processAudioEvent so it survives CPU contention
  // that can starve a ScriptProcessorNode.
  window.__samples = [];
  window.__captureSamples = true;
  try { await window.playSound("kick", 1.0); } catch (e) {
    window.__captureSamples = false;
    return { pass: false, reason: "playSound_threw", error: String(e && e.message || e), observedVolume: vol };
  }
  const captured = window.__samples.slice();
  window.__captureSamples = false;

  let peak = 0;
  for (let i = 0; i < captured.length; i++) {
    const a = Math.abs(captured[i]);
    if (a > peak) peak = a;
  }
  if (captured.length === 0) {
    return { pass: false, reason: "no_samples_captured", observedVolume: vol, peak };
  }
  if (peak < 0.001) {
    return { pass: false, reason: "no_audio_energy", observedVolume: vol, samples: captured.length, peak };
  }
  return { pass: true, observedVolume: vol, samples: captured.length, peak };
}
register({
  suite: "mobile_audio_unlock",
  name: "queued_channel_ops_after_unlock",
  fn: scenarioQueuedChannelOpsAppliedAfterUnlock,
  description: "Pre-unlock setChannelVolume sticks AND kick produces real audio energy",
});

// ─── Scenario 4: playback (Play button) produces audio after touch unlock ──
// THIS is the scenario that reproduces the beatmo.io "click Play, nothing
// happens" failure on iOS Safari: builds a 2-node loop, calls startPlay,
// captures the live output for ~1.5s, asserts RMS > 0. If this fails on
// the phone, the bug is reproduced and the report tells you whether it
// failed at scheduler dispatch, audio routing, or context state.
export async function scenarioPlaybackAfterUnlock() {
  // Real iOS already provided a gesture (the button tap that started the
  // suite). Synthetic gesture only matters in headless Chromium.
  try {
    document.dispatchEvent(new Event("pointerdown"));
    window.resumeAudio?.();
  } catch (_) {}
  await new Promise((r) => setTimeout(r, 200));

  // Build a minimal circuit: two nodes in a loop. This mirrors the existing
  // Playwright scenario 4 setup exactly so behavior matches CI.
  try {
    window.addNode?.(0, 0, "regular");
    window.addNode?.(4, 0, "regular");
    window.addEdgeGrid?.(0, 0, 4, 0);
    window.addEdgeGrid?.(4, 0, 0, 0);
    window.updateBeatInfos?.();
    window.forceDraw?.();
  } catch (e) {
    return { pass: false, reason: "circuit_build_failed", error: String(e && e.message || e) };
  }
  await new Promise((r) => setTimeout(r, 200));

  try { window.enableOutputCapture?.(); } catch (_) {}
  try { window.startOutputCapture?.(); } catch (e) {
    return { pass: false, reason: "startOutputCapture_threw", error: String(e && e.message || e) };
  }
  try { window.startPlay?.(); } catch (e) {
    return { pass: false, reason: "startPlay_threw", error: String(e && e.message || e) };
  }

  await new Promise((r) => setTimeout(r, 1500));

  try { window.stopPlay?.(); } catch (_) {}
  await new Promise((r) => setTimeout(r, 200));

  let captured;
  try { captured = Array.from(window.stopOutputCapture?.() || []); } catch (e) {
    return { pass: false, reason: "stopOutputCapture_threw", error: String(e && e.message || e) };
  }
  let peak = 0;
  let sumSq = 0;
  for (let i = 0; i < captured.length; i++) {
    const a = Math.abs(captured[i]);
    if (a > peak) peak = a;
    sumSq += captured[i] * captured[i];
  }
  const rms = captured.length ? Math.sqrt(sumSq / captured.length) : 0;
  const ctxState = window.__audioCtx?.state || "no-context";
  const ctxTime = window.__audioCtx?.currentTime || 0;

  if (captured.length === 0) {
    return { pass: false, reason: "no_samples", ctxState, ctxTime };
  }
  if (rms < 0.0001) {
    return { pass: false, reason: "no_audio_energy", samples: captured.length, peak, rms, ctxState, ctxTime };
  }
  if (peak > 10) {
    return { pass: false, reason: "extreme_peak_clip", samples: captured.length, peak, rms, ctxState, ctxTime };
  }
  return { pass: true, samples: captured.length, peak, rms, ctxState, ctxTime };
}
register({
  suite: "mobile_audio_unlock",
  name: "playback_after_unlock",
  fn: scenarioPlaybackAfterUnlock,
  description: "Build 2-node loop → startPlay → capture 1.5s → assert non-zero audio (the Play-button path)",
});
