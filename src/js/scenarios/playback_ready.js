// scenarios/playback_ready.js
//
// In-page twin of audio_probe_bench_helpers.js#startPlaybackWhenReady, for the
// scenario modules that also run on real phones (test_runner.html), where there
// is no Playwright `page` to wait from.
//
// The Play button is deliberately DISABLED while the playback-readiness gate
// warms the circuit's voices, and a press during warm-up is ignored, never
// queued ("a play that fires seconds later, unprompted, is worse than one that
// did not fire" — game_update.go). A user cannot press it then; a scenario that
// does is testing something no user can do, and passes or fails on how fast the
// warm-up happens to be. These wait for the same gate the button renders from.

/** Resolves true once the readiness gate is open (or the build has no gate). */
export async function waitForPlaybackReady(timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (typeof window.warmupProgress !== "function" || window.warmupProgress().ready === true) return true;
    await new Promise((r) => setTimeout(r, 50));
  }
  return false;
}

/**
 * Presses Play (the startPlay export) once the gate is open and confirms the
 * transport is running. Returns { ok, reason }.
 */
export async function pressPlayWhenReady(timeoutMs = 20000) {
  if (!(await waitForPlaybackReady(timeoutMs))) return { ok: false, reason: "warmup_never_ready" };
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (window.isPlaying?.()) return { ok: true };
    try { window.startPlay?.(); } catch (e) { return { ok: false, reason: "startPlay_threw", error: String(e && e.message || e) }; }
    await new Promise((r) => setTimeout(r, 100));
    if (typeof window.isPlaying !== "function" || window.isPlaying()) return { ok: true };
  }
  return { ok: false, reason: "transport_never_started" };
}
