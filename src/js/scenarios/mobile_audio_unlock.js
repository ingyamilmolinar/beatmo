// Two shells, one set of assertions: the Playwright file wraps each scenario in a page.evaluate, test_runner.html runs
// them on a real phone. Only the pure in-page scenarios live here; deferred-context and re-suspension need multi-context
// orchestration.

import { register } from "./registry.js";
import { pressPlayWhenReady } from "./playback_ready.js";

// Volume calls made before the context exists are applied once it runs AND audio plays at that volume: "click Play,
// no audio" could be either failing.
export async function scenarioQueuedChannelOpsAppliedAfterUnlock() {
  // Before or after creation, pendingChannelOps handles it.
  try { window.setChannelVolume("kick", 0.5); } catch (e) { /* setter may not exist on stale builds */ }

  // On real iOS the user's tap already supplied the gesture.
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

  // So an async render cannot push the trigger past the capture window.
  try { await window.audioReady; } catch (_) {}
  if (typeof window.ensureSynthSample === "function") {
    try { await window.ensureSynthSample("kick"); } catch (_) {}
  }

  // Recorded synchronously inside processAudioEvent, so it survives contention that starves a ScriptProcessor.
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

// The "click Play, nothing happens" repro on iOS: a 2-node loop, startPlay, ~1.5 s of live output, RMS > 0; the report
// says whether dispatch, routing or context state failed.
export async function scenarioPlaybackAfterUnlock() {
  // Real iOS already had a gesture; this one matters only headless.
  try {
    document.dispatchEvent(new Event("pointerdown"));
    window.resumeAudio?.();
  } catch (_) {}
  await new Promise((r) => setTimeout(r, 200));

  // The same circuit as the Playwright scenario, so behaviour matches CI.
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
  // Play is ignored while the warm-up gate is closed.
  const play = await pressPlayWhenReady();
  if (!play.ok) return { pass: false, reason: play.reason, error: play.error };

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
