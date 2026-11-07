// In-page twin of startPlaybackWhenReady for phones, where there is no Playwright page. Play is DISABLED while warm-up
// runs and a press then is ignored, never queued, so a scenario pressing early tests something no user can do.

/** Resolves true once the readiness gate is open (or the build has no gate). */
export async function waitForPlaybackReady(timeoutMs = 20000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (typeof window.warmupProgress !== "function" || window.warmupProgress().ready === true) return true;
    await new Promise((r) => setTimeout(r, 50));
  }
  return false;
}

/** Returns {ok, reason}. */
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
