// test_runner_overlay.js
//
// Injected into production index.html by createServer when the URL is
// /?run=tests. The page below this overlay is the EXACT production app —
// same canvas, same inline scripts, same _kbProxy/_mobileInput/_fp globals,
// same WASM boot — so the runner reproduces the on-device infra 100%.
//
// Two execution modes:
//   - "Run all" / per-scenario buttons run the scenario in-page and render
//     PASS/FAIL + details.
//   - The "user-tap" reproduction scenario dispatches the custom event
//     beatmo:runner-needs-canvas; this overlay listens for that, dims and
//     disables pointer events so the user's finger lands on the real
//     on-canvas Play button underneath, then re-enables itself on
//     beatmo:runner-release-canvas (or an auto-restore timer).

(async () => {
  // Wait for the production WASM boot to finish wiring its JS exports.
  async function waitForWasm() {
    const deadline = Date.now() + 30000;
    while (Date.now() < deadline) {
      if (
        typeof window.playSound === "function" &&
        typeof window.startPlay === "function" &&
        typeof window.playBtnRect === "function" &&
        typeof window.startOutputCapture === "function"
      ) return true;
      await new Promise((r) => setTimeout(r, 100));
    }
    return false;
  }

  const css = `
    #beatmo-runner-root {
      position: fixed; left: 0; right: 0; top: 0;
      max-height: 60vh; max-height: 60dvh;
      background: rgba(15,15,20,0.96); color: #eee;
      z-index: 2147483646;
      font-family: -apple-system, system-ui, sans-serif;
      overflow-y: auto;
      -webkit-overflow-scrolling: touch;
      box-shadow: 0 4px 24px rgba(0,0,0,0.5);
      transition: opacity 200ms, transform 200ms;
    }
    #beatmo-runner-root.pass-through {
      opacity: 0.18;
      pointer-events: none;
    }
    #beatmo-runner-root.minimized {
      transform: translateY(calc(-100% + 44px));
    }
    .br-hdr { display: flex; align-items: center; gap: 8px; padding: 8px 12px; background: #1a1a26; border-bottom: 1px solid #252535; }
    .br-title { font-weight: 600; font-size: 14px; color: #bbb; flex: 1; }
    .br-mini, .br-runall {
      font-size: 13px; padding: 6px 10px; border-radius: 6px; border: 0; cursor: pointer; font-weight: 600;
      -webkit-tap-highlight-color: transparent;
    }
    .br-runall { background: #2a7eff; color: #fff; }
    .br-mini { background: #2a2a36; color: #bbb; }
    .br-body { padding: 8px 12px; }
    .br-status { font-size: 12px; color: #88a; margin-bottom: 8px; }
    .br-status.bad { color: #ff8585; }
    .br-suite { margin-top: 10px; }
    .br-suite h3 { font-size: 11px; margin: 0 0 6px 0; color: #779; font-weight: 600; letter-spacing: 0.5px; text-transform: uppercase; }
    .br-card { background: #16161e; border-radius: 8px; padding: 8px 10px; margin-bottom: 6px; }
    .br-row { display: flex; align-items: center; gap: 8px; justify-content: space-between; }
    .br-name { font-weight: 600; font-size: 13px; color: #ddd; }
    .br-desc { font-size: 11px; color: #889; line-height: 1.3; margin-top: 2px; }
    .br-actions { display: flex; gap: 6px; align-items: center; }
    .br-run { padding: 6px 10px; font-size: 12px; background: #2a7eff; color: #fff; border: 0; border-radius: 5px; font-weight: 600; -webkit-tap-highlight-color: transparent; }
    .br-verdict { font-weight: 700; font-size: 11px; padding: 3px 8px; border-radius: 4px; }
    .br-verdict.pass { background: #1f6f37; color: #d6ffe1; }
    .br-verdict.fail { background: #8b1f1f; color: #ffe1e1; }
    .br-verdict.running { background: #444; color: #ddd; }
    .br-verdict.pending { background: #25252e; color: #777; }
    .br-details { font-family: ui-monospace, monospace; font-size: 10px; color: #aac; word-break: break-all; white-space: pre-wrap; margin-top: 6px; padding-top: 6px; border-top: 1px solid #25252e; }
    .br-floor { padding: 8px 12px; font-size: 11px; color: #99a; border-top: 1px solid #25252e; background: #1a1a26; position: sticky; bottom: 0; }
    .br-floor .count.pass { color: #88ff9a; font-weight: 700; }
    .br-floor .count.fail { color: #ff8585; font-weight: 700; }
    .br-needs-canvas-banner {
      position: fixed; bottom: 16px; left: 16px; right: 16px;
      background: #ff8800; color: #1a1a00; font-weight: 700;
      padding: 12px 16px; border-radius: 10px; z-index: 2147483647;
      text-align: center; font-size: 14px;
      box-shadow: 0 4px 16px rgba(0,0,0,0.6);
    }
  `;
  const style = document.createElement("style");
  style.textContent = css;
  document.head.appendChild(style);

  const root = document.createElement("div");
  root.id = "beatmo-runner-root";
  root.innerHTML = `
    <div class="br-hdr">
      <span class="br-title">Beatmo · in-page test runner</span>
      <button class="br-runall" id="br-runall">Run all</button>
      <button class="br-mini" id="br-mini">Hide</button>
    </div>
    <div class="br-body">
      <div class="br-status" id="br-status">Waiting for WASM…</div>
      <div id="br-suites"></div>
    </div>
    <div class="br-floor"><span id="br-counts">—</span></div>
  `;
  document.body.appendChild(root);

  const statusEl = root.querySelector("#br-status");
  const suitesEl = root.querySelector("#br-suites");
  const runAllBtn = root.querySelector("#br-runall");
  const miniBtn = root.querySelector("#br-mini");
  const countsEl = root.querySelector("#br-counts");

  miniBtn.addEventListener("click", () => {
    const minimized = root.classList.toggle("minimized");
    miniBtn.textContent = minimized ? "Show" : "Hide";
  });

  const wasmOk = await waitForWasm();
  if (!wasmOk) {
    statusEl.classList.add("bad");
    statusEl.textContent = "WASM boot timed out (no playSound/startPlay/playBtnRect after 30s).";
    return;
  }

  // Importing the scenario modules triggers their register() side-effects.
  // ADD SUITES HERE to surface more existing tests in the runner.
  const { list } = await import("./scenarios/registry.js");
  await Promise.all([
    import("./scenarios/webaudio_output_capture.js"),
    import("./scenarios/mobile_audio_unlock.js"),
    import("./scenarios/e2e_transport.js"),
  ]);

  const scenarios = list();
  const cells = new Map();

  function render() {
    const bySuite = new Map();
    for (const s of scenarios) {
      if (!bySuite.has(s.suite)) bySuite.set(s.suite, []);
      bySuite.get(s.suite).push(s);
    }
    suitesEl.innerHTML = "";
    for (const [suite, items] of bySuite) {
      const div = document.createElement("div");
      div.className = "br-suite";
      const h3 = document.createElement("h3");
      h3.textContent = suite;
      div.appendChild(h3);
      for (const sc of items) {
        const card = document.createElement("div"); card.className = "br-card";
        const row = document.createElement("div"); row.className = "br-row";
        const left = document.createElement("div");
        const name = document.createElement("div"); name.className = "br-name"; name.textContent = sc.name;
        const desc = document.createElement("div"); desc.className = "br-desc"; desc.textContent = sc.description || "";
        left.appendChild(name); left.appendChild(desc);
        const actions = document.createElement("div"); actions.className = "br-actions";
        const verdict = document.createElement("span"); verdict.className = "br-verdict pending"; verdict.textContent = "pending";
        const runBtn = document.createElement("button"); runBtn.className = "br-run"; runBtn.textContent = "Run";
        runBtn.addEventListener("click", () => runOne(sc));
        actions.appendChild(verdict); actions.appendChild(runBtn);
        row.appendChild(left); row.appendChild(actions);
        const details = document.createElement("div"); details.className = "br-details"; details.style.display = "none";
        card.appendChild(row); card.appendChild(details);
        div.appendChild(card);
        cells.set(sc.suite + "/" + sc.name, { verdictEl: verdict, detailsEl: details, runBtn });
      }
      suitesEl.appendChild(div);
    }
  }

  function updateCounts() {
    let pass = 0, fail = 0;
    for (const sc of scenarios) {
      const c = cells.get(sc.suite + "/" + sc.name);
      if (c.verdictEl.classList.contains("pass")) pass++;
      else if (c.verdictEl.classList.contains("fail")) fail++;
    }
    countsEl.innerHTML =
      `<span class="count pass">${pass} pass</span> · ` +
      `<span class="count fail">${fail} fail</span> · ${scenarios.length} total`;
  }

  // Pass-through mode: scenarios fire beatmo:runner-needs-canvas when they
  // need the user's finger to reach the canvas underneath us.
  let needsCanvasBanner = null;
  let releaseTimer = null;
  function enterPassThrough(detail) {
    root.classList.add("pass-through");
    if (releaseTimer) clearTimeout(releaseTimer);
    if (!needsCanvasBanner) {
      needsCanvasBanner = document.createElement("div");
      needsCanvasBanner.className = "br-needs-canvas-banner";
      document.body.appendChild(needsCanvasBanner);
    }
    const ms = (detail && detail.windowMs) || 8000;
    needsCanvasBanner.textContent = `↓  TAP the Play button on the game below (${Math.round(ms/1000)}s)  ↓`;
    // Safety: auto-restore even if the scenario forgot to release.
    releaseTimer = setTimeout(() => exitPassThrough(), ms + 1500);
  }
  function exitPassThrough() {
    root.classList.remove("pass-through");
    if (needsCanvasBanner) { needsCanvasBanner.remove(); needsCanvasBanner = null; }
    if (releaseTimer) { clearTimeout(releaseTimer); releaseTimer = null; }
  }
  document.addEventListener("beatmo:runner-needs-canvas", (e) => enterPassThrough(e.detail));
  document.addEventListener("beatmo:runner-release-canvas", () => exitPassThrough());

  async function runOne(sc) {
    const c = cells.get(sc.suite + "/" + sc.name);
    c.verdictEl.className = "br-verdict running"; c.verdictEl.textContent = "running…";
    c.detailsEl.style.display = "none";
    c.runBtn.disabled = true;
    let result;
    try {
      result = await sc.fn();
    } catch (e) {
      result = { pass: false, reason: "threw", error: String(e?.message || e) };
    }
    c.verdictEl.className = "br-verdict " + (result.pass ? "pass" : "fail");
    c.verdictEl.textContent = result.pass ? "PASS" : "FAIL";
    c.detailsEl.style.display = "block";
    c.detailsEl.textContent = JSON.stringify(result, null, 2);
    c.runBtn.disabled = false;
    updateCounts();
    return result;
  }

  async function runAll() {
    runAllBtn.disabled = true;
    runAllBtn.textContent = "Running…";
    for (const sc of scenarios) {
      // Skip user-tap scenarios in "Run all" — they need a human to tap the
      // canvas and would block the whole run. Users invoke them individually.
      if (sc.name.includes("user_tap")) continue;
      await runOne(sc);
    }
    runAllBtn.disabled = false;
    runAllBtn.textContent = "Run all";
  }

  runAllBtn.addEventListener("click", async () => {
    try { window.resumeAudio?.(); } catch (_) {}
    statusEl.textContent = "AudioContext: " + (window.__audioCtx?.state || "?") + " · sample rate " + (window.__audioCtx?.sampleRate || "?") + " Hz";
    await runAll();
  });

  render();
  updateCounts();
  statusEl.textContent =
    "Production index.html · " + scenarios.length + " scenarios registered. " +
    "Tap a scenario's Run, or Run all (the user-tap reproduction is skipped from Run all — invoke it directly).";
})();
