// scenarios/e2e_transport.js
//
// Pure in-browser scenarios extracted from
// src/js/e2e_transport.browser.test.js. Same assertions as the existing
// Playwright spec; what changes is the click driver:
//
//   - Playwright shell: page.mouse.click() — produces trusted MouseEvents
//     that Ebiten's input layer treats like a real user.
//   - In-page on a phone: we have to synthesize events from JS, which
//     iOS Safari marks as `isTrusted: false`. Ebiten's input.go reads the
//     pointer state regardless of trust, so it generally works — but the
//     gold standard reproduction is the user actually tapping the
//     on-canvas button with their finger; see scenarioPlayButtonUserTap.
//
// The "*UserTap" scenarios deliberately do NOT auto-click anything. They
// arm a capture window, ask test_runner_overlay.js to step out of the way
// (pointer-events: none + cosmetic dim), then wait for the user's finger
// to land on the actual Beatmo Play button on the canvas underneath. This
// is the only path that 100% reproduces the beatmo.io flow: real touch +
// Ebiten input + Go-side click handler + audio dispatch.

import { register } from "./registry.js";

function dispatchClickAtClientPoint(target, clientX, clientY) {
  // Fire the whole pointer + touch + mouse sequence; Ebiten's input layer
  // is forgiving but different browsers route differently. isTrusted will
  // be false on all of these (in-page JS cannot synthesize trusted events).
  const opts = { bubbles: true, cancelable: true, clientX, clientY, button: 0, buttons: 1, view: window };
  try { target.dispatchEvent(new PointerEvent("pointerdown", { ...opts, pointerType: "touch", isPrimary: true, pointerId: 1 })); } catch (_) {}
  try {
    const t = new Touch({ identifier: 1, target, clientX, clientY, screenX: clientX, screenY: clientY, pageX: clientX, pageY: clientY });
    target.dispatchEvent(new TouchEvent("touchstart", { bubbles: true, cancelable: true, touches: [t], targetTouches: [t], changedTouches: [t] }));
  } catch (_) {}
  target.dispatchEvent(new MouseEvent("mousedown", opts));
  // 50ms hold (matches existing clickAndHold default)
  return new Promise((resolve) => {
    setTimeout(() => {
      try { target.dispatchEvent(new PointerEvent("pointerup", { ...opts, pointerType: "touch", isPrimary: true, pointerId: 1, buttons: 0 })); } catch (_) {}
      try {
        const t = new Touch({ identifier: 1, target, clientX, clientY, screenX: clientX, screenY: clientY, pageX: clientX, pageY: clientY });
        target.dispatchEvent(new TouchEvent("touchend", { bubbles: true, cancelable: true, touches: [], targetTouches: [], changedTouches: [t] }));
      } catch (_) {}
      target.dispatchEvent(new MouseEvent("mouseup", { ...opts, buttons: 0 }));
      resolve();
    }, 50);
  });
}

function getCanvas() {
  return document.querySelector("canvas");
}

function canvasClientPointFor(rect) {
  const c = getCanvas();
  if (!c) return null;
  const cr = c.getBoundingClientRect();
  return {
    clientX: cr.left + rect.x + rect.w / 2,
    clientY: cr.top + rect.y + rect.h / 2,
  };
}

async function waitFor(predicate, timeoutMs, pollMs = 100) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (predicate()) return true;
    await new Promise((r) => setTimeout(r, pollMs));
  }
  return predicate();
}

// ─── Scenario 1: synthetic Play-button click ───────────────────────────────
// Direct port of e2e_transport's Test 1, with the click synthesized in-page.
// Catches "Play-button rect missing/zero" + "click never reaches Ebiten" +
// "Ebiten saw the click but Go-side handler did nothing".
export async function scenarioPlayButtonSyntheticClick() {
  const playRect = window.playBtnRect?.();
  if (!playRect || !(playRect.w > 0) || !(playRect.h > 0)) {
    return { pass: false, reason: "play_rect_invalid", playRect };
  }
  const pt = canvasClientPointFor(playRect);
  if (!pt) return { pass: false, reason: "no_canvas" };

  const wasPlayingBefore = !!window.isPlaying?.();
  if (wasPlayingBefore) {
    try { window.stopPlay?.(); } catch (_) {}
    await new Promise((r) => setTimeout(r, 200));
  }

  await dispatchClickAtClientPoint(getCanvas(), pt.clientX, pt.clientY);
  const flipped = await waitFor(() => !!window.isPlaying?.(), 1500);
  const isPlayingAfter = !!window.isPlaying?.();
  // Cleanup so subsequent runs don't leave the transport mid-flight.
  try { if (isPlayingAfter) window.stopPlay?.(); } catch (_) {}

  return {
    pass: flipped,
    reason: flipped ? null : "isPlaying_did_not_flip",
    playRect,
    canvasClientPoint: pt,
    wasPlayingBefore,
    isPlayingAfter,
  };
}
register({
  suite: "e2e_transport",
  name: "play_button_synthetic_click",
  fn: scenarioPlayButtonSyntheticClick,
  description: "Synthetic touch/mouse event at playBtnRect coords → assert isPlaying flips",
});

// ─── Scenario 2: synthetic Stop-button click ───────────────────────────────
export async function scenarioStopButtonSyntheticClick() {
  // Make sure something IS playing to begin with.
  if (!window.isPlaying?.()) {
    try { window.startPlay?.(); } catch (_) {}
    await new Promise((r) => setTimeout(r, 200));
  }
  if (!window.isPlaying?.()) {
    return { pass: false, reason: "could_not_start_playback_to_stop" };
  }
  const stopRect = window.stopBtnRect?.();
  if (!stopRect || !(stopRect.w > 0) || !(stopRect.h > 0)) {
    try { window.stopPlay?.(); } catch (_) {}
    return { pass: false, reason: "stop_rect_invalid", stopRect };
  }
  const pt = canvasClientPointFor(stopRect);
  await dispatchClickAtClientPoint(getCanvas(), pt.clientX, pt.clientY);
  const flipped = await waitFor(() => !window.isPlaying?.(), 1500);
  const isPlayingAfter = !!window.isPlaying?.();
  try { if (isPlayingAfter) window.stopPlay?.(); } catch (_) {}
  return {
    pass: flipped,
    reason: flipped ? null : "isPlaying_did_not_flip_off",
    stopRect,
    canvasClientPoint: pt,
    isPlayingAfter,
  };
}
register({
  suite: "e2e_transport",
  name: "stop_button_synthetic_click",
  fn: scenarioStopButtonSyntheticClick,
  description: "Synthetic click on stopBtnRect → assert isPlaying flips off",
});

// ─── Scenario 3: synthetic BPM ± ───────────────────────────────────────────
export async function scenarioBpmIncrementSynthetic() {
  const incRect = window.bpmIncBtnRect?.();
  if (!incRect || !(incRect.w > 0) || !(incRect.h > 0)) {
    return { pass: false, reason: "bpm_inc_rect_invalid", incRect };
  }
  const pt = canvasClientPointFor(incRect);
  const before = window.getBPM?.();
  for (let i = 0; i < 3; i++) {
    await dispatchClickAtClientPoint(getCanvas(), pt.clientX, pt.clientY);
    await new Promise((r) => setTimeout(r, 90));
  }
  // Unlike Play/Stop (whose OnClick applies its effect synchronously), the
  // BPM +/- buttons only accumulate a delta in OnClick; TransportZone.Update()
  // applies it via SetBPM on a *subsequent* frame. Under heavy parallel CI
  // load a frame can take longer than the inter-click delay, so a single
  // synchronous read can observe the pre-apply value (the cause of the flaky
  // "bpm_did_not_increase" with delta:0 in the 4-job parallel phase). Poll for
  // the deferred update instead, mirroring the Play/Stop waitFor(() => isPlaying).
  await waitFor(() => {
    const v = window.getBPM?.();
    return Number.isFinite(v) && Number.isFinite(before) && v > before;
  }, 1500);
  const after = window.getBPM?.();
  return {
    pass: Number.isFinite(after) && Number.isFinite(before) && after > before,
    reason: (Number.isFinite(after) && after > before) ? null : "bpm_did_not_increase",
    before, after, delta: (after ?? 0) - (before ?? 0),
  };
}
register({
  suite: "e2e_transport",
  name: "bpm_increment_synthetic",
  fn: scenarioBpmIncrementSynthetic,
  description: "3 synthetic clicks on BPM+ → assert getBPM() increased",
});

// ─── Scenario 4: PRODUCTION REPRODUCTION ───────────────────────────────────
// The bug class the user is hitting: "click Play on beatmo.io, no audio".
// This scenario asks the user to physically tap the on-canvas Play button
// while audio capture runs. The runner overlay (test_runner_overlay.js)
// disables pointer events for the duration of the window so the finger
// actually lands on the canvas Play button under the overlay.
//
// The result decomposes the silence into four diagnoses (the same shape
// the deleted sanity probe used, now grounded in the real production
// flow):
//   - tap_not_received   : isPlaying never flipped → Ebiten didn't see the tap
//   - tap_received_silent: isPlaying flipped but capture stayed zero
//                          → audio dispatch / output chain broken on iOS
//   - extreme_peak       : clipping outside [-10, +10]
//   - pass               : real tap → real audio
export async function scenarioPlayButtonUserTap(opts = {}) {
  const windowMs = opts.windowMs ?? 8000;
  const capturePostMs = opts.capturePostMs ?? 2500;

  const playRect = window.playBtnRect?.();
  if (!playRect || !(playRect.w > 0) || !(playRect.h > 0)) {
    return { pass: false, reason: "play_rect_invalid", playRect };
  }

  // Stop any in-flight playback so we're measuring this tap specifically.
  if (window.isPlaying?.()) {
    try { window.stopPlay?.(); } catch (_) {}
    await new Promise((r) => setTimeout(r, 200));
  }

  // Arm capture BEFORE asking the user to tap so we don't miss the onset.
  try { window.startOutputCapture?.(); } catch (e) {
    return { pass: false, reason: "startOutputCapture_threw", error: String(e?.message || e) };
  }

  // Signal to the overlay that we need pointer events to pass through to
  // the canvas. The overlay listens for this event and dims+disables
  // itself for the duration.
  document.dispatchEvent(new CustomEvent("beatmo:runner-needs-canvas", {
    detail: { reason: "user_tap_play", windowMs, playRect },
  }));

  const tStart = Date.now();
  let flippedAt = 0;
  const flipped = await waitFor(() => {
    if (window.isPlaying?.()) { flippedAt = Date.now(); return true; }
    return false;
  }, windowMs);

  // Hand control back to the overlay either way (it re-enables itself when
  // it sees the result event below, but also has its own timer fallback).
  document.dispatchEvent(new CustomEvent("beatmo:runner-release-canvas"));

  if (!flipped) {
    let captured;
    try { captured = window.stopOutputCapture?.() || []; } catch (_) { captured = []; }
    return {
      pass: false,
      reason: "tap_not_received",
      playRect,
      windowMs,
      isPlayingAfter: !!window.isPlaying?.(),
      capturedSamples: captured.length || 0,
    };
  }

  // Got a tap. Let it play for capturePostMs more, then stop everything.
  await new Promise((r) => setTimeout(r, capturePostMs));
  try { window.stopPlay?.(); } catch (_) {}
  await new Promise((r) => setTimeout(r, 200));

  let captured;
  try { captured = Array.from(window.stopOutputCapture?.() || []); } catch (e) {
    return { pass: false, reason: "stopOutputCapture_threw", error: String(e?.message || e) };
  }
  let peak = 0, sumSq = 0;
  for (let i = 0; i < captured.length; i++) {
    const a = Math.abs(captured[i]);
    if (a > peak) peak = a;
    sumSq += captured[i] * captured[i];
  }
  const rms = captured.length ? Math.sqrt(sumSq / captured.length) : 0;
  const ctxState = window.__audioCtx?.state || "no-context";
  const ctxTime = window.__audioCtx?.currentTime || 0;
  const tapLatencyMs = flippedAt - tStart;

  if (captured.length === 0) {
    return { pass: false, reason: "tap_received_silent", playRect, tapLatencyMs, ctxState, ctxTime };
  }
  if (rms < 0.0001) {
    return { pass: false, reason: "tap_received_silent", playRect, tapLatencyMs, samples: captured.length, peak, rms, ctxState, ctxTime };
  }
  if (peak > 10) {
    return { pass: false, reason: "extreme_peak", playRect, tapLatencyMs, samples: captured.length, peak, rms, ctxState, ctxTime };
  }
  return { pass: true, playRect, tapLatencyMs, samples: captured.length, peak, rms, ctxState, ctxTime };
}
register({
  suite: "e2e_transport",
  name: "play_button_user_tap_reproduction",
  fn: scenarioPlayButtonUserTap,
  description: "REAL finger tap on the on-canvas Play button + live audio capture (the bug-reproduction scenario)",
});
