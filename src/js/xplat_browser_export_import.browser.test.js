/**
 * Cross-ENGINE project export/import parity — Chromium vs Firefox vs WebKit.
 *
 * JS Test Charter §5 (cross-platform parity). A project file written by one
 * browser must be byte-identical to the one another browser writes from the
 * same state, and must import into another engine to exactly the same state —
 * otherwise a circuit saved on a phone and opened on a desktop is a different
 * circuit.
 *
 * WHY THIS CAN BE ASSERTED AT ALL. The project JSON is produced by Go compiled
 * to WASM (`exportJSON` → `projectFile` → `json.MarshalIndent`). WASM float
 * semantics are spec-deterministic and Go's encoder + `strconv` float
 * formatting are host-independent, so the bytes are a pure function of engine
 * state. The state itself is Go-side too: master volume and pan read
 * `chanMgr` (channels.go is build-tag-free), NOT the WebAudio nodes — JS is
 * only a sink for those. That insulation is the property under test: the day a
 * getter starts reading back from an AudioParam (a float32, rounded by the
 * browser) or from anything else engine-implemented, these assertions break.
 *
 * WHAT IS DELIBERATELY *NOT* ASSERTED: the master mix. Per-channel EQ is a
 * `BiquadFilterNode`, the sends are a `DelayNode` plus a Schroeder core built
 * from native nodes, and pan is a `StereoPannerNode` — all implemented BY the
 * browser, with their own coefficient math and denormal handling. Byte-identical
 * master audio across engines is not achievable and pretending otherwise would
 * make this test a liar. Master-mix byte identity is pinned on native instead,
 * against the real C DSP:
 *   internal/ui/export_import_master_audio_identity_test.go
 *
 * What IS asserted about audio is the part that is genuinely engine-independent:
 * the synth VOICE, rendered by the C DSP inside the same WASM module on every
 * engine (`__testCaptureSynthRender`) — at a PINNED 48 kHz. Engines disagree on
 * the AudioContext rate (headless WebKit runs 44.1 kHz, Chromium 48 kHz) and a
 * voice is rendered at the device rate, so comparing default renders compared
 * two different sample rates and reported every voice as a "divergence".
 *
 * Engines that are not installed are skipped; the test needs at least two to
 * mean anything and reports what it actually compared.
 *
 * Run: GO=/abs/path/.tools/go/bin/go node src/js/xplat_browser_export_import.browser.test.js
 */
import * as pw from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

const ENGINES = ["chromium", "firefox", "webkit"];
const BOOT_TIMEOUT_MS = 90000;

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

/**
 * The edit applied identically on every engine. Runs inside the page.
 * Driven through the same JS exports the UI uses, so the resulting state is a
 * state a user can actually reach.
 */
const EDIT_AND_EXPORT = async () => {
  const out = {};
  const count = window.drumRowCount();
  const ids = [];
  for (let i = 0; i < count; i++) {
    const id = window.rowInstrument(i);
    if (id) ids.push(id);
  }
  out.ids = ids;
  if (ids.length === 0) return out;

  // Synth knobs — the sparse overlay shape the Synth tab writes.
  window.setInstrumentParam(ids[0], "pitchenv_amt", 17);
  window.setInstrumentParam(ids[0], "drive", 0.62);
  if (ids[1]) window.setInstrumentParam(ids[1], "drive", 0.31);

  // Insert-effect chain + a param edit through the audio layer.
  window.addInsertEffect(ids[0], "distortion");
  window.setInsertEffectParam(ids[0], 0, "drive", 1.7);
  window.setInsertEffectParam(ids[0], 0, "mix", 0.45);

  // EQ: a band gain plus both filters.
  window.setEQChannel(ids[0]);
  window.setEQBandGain(2, 5.5);
  window.setEQHPF(true, 65);
  window.setEQLPF(true, 9500);

  // Transport + master.
  window.setBPM(137);
  window.setMainVolume(0.72);

  out.exported = window.exportJSON();
  out.instParams = {};
  for (const id of ids) out.instParams[id] = window.getInstrumentParams(id);
  out.insertFX = window.getInsertEffects(ids[0]);
  out.render = {};
  for (const id of ids) out.render[id] = await window.__testCaptureSynthRender(id, { sampleRate: 48000 });
  return out;
};

const READY = () =>
  typeof window.exportJSON === "function" &&
  typeof window.importJSON === "function" &&
  typeof window.setInstrumentParam === "function" &&
  typeof window.setInsertEffectParam === "function" &&
  typeof window.addInsertEffect === "function" &&
  typeof window.setEQBandGain === "function" &&
  typeof window.drumRowCount === "function" &&
  typeof window.rowInstrument === "function" &&
  typeof window.__testCaptureSynthRender === "function";

async function openEngine(engine, port) {
  const launchOpts = engine === "chromium"
    ? { args: ["--autoplay-policy=no-user-gesture-required"] }
    : {};
  const browser = await pw[engine].launch(launchOpts);
  try {
    const page = await browser.newPage();
    const pageErrors = [];
    page.on("pageerror", (e) => pageErrors.push(String(e.message)));
    if (process.env.TEST_LOG) page.on("console", (m) => console.log(`[${engine}]`, m.type(), m.text()));
    await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
    await page.waitForFunction(READY, null, { timeout: BOOT_TIMEOUT_MS, polling: 500 });
    return { browser, page, pageErrors };
  } catch (err) {
    // An engine that launched but failed to boot is SKIPPED by the caller —
    // close it here, or its browser process keeps node alive after the test
    // has already printed its verdict (the run then dies on the outer timeout).
    await browser.close().catch(() => {});
    throw err;
  }
}

/** Fingerprint of a __testCaptureSynthRender result, stable to stringify. */
const voiceFP = (r) => (r ? JSON.stringify({ len: r.length, peak: r.peak, rms: r.rms, head: r.head, tail: r.tail }) : "null");

/**
 * Compare two insert-effect chains by VALUE, returning a description of the
 * first difference or "" when equal.
 *
 * Not JSON.stringify: `getInsertEffects` builds its param object by ranging a
 * Go map, so key ORDER varies run to run and engine to engine while the values
 * are identical. (The exported FILE is unaffected — Go's json encoder sorts map
 * keys — which is why the byte comparison above is still exact.) Comparing the
 * serialized form here would fail on nothing but map iteration order.
 */
function insertFXDiff(a, b) {
  a = a || []; b = b || [];
  if (a.length !== b.length) return `slot count ${a.length} -> ${b.length}`;
  for (let i = 0; i < a.length; i++) {
    if (a[i].type !== b[i].type) return `slot ${i} type ${a[i].type} -> ${b[i].type}`;
    if (!!a[i].enabled !== !!b[i].enabled) return `slot ${i} enabled ${a[i].enabled} -> ${b[i].enabled}`;
    const pa = a[i].params || {}, pb = b[i].params || {};
    const keys = new Set([...Object.keys(pa), ...Object.keys(pb)]);
    for (const k of keys) {
      if (pa[k] !== pb[k]) return `slot ${i} (${a[i].type}) param "${k}" ${pa[k]} -> ${pb[k]}`;
    }
  }
  return "";
}

/** Report the first few differing lines of two JSON documents. */
function reportJSONDiff(labelA, a, labelB, b, limit = 10) {
  const la = a.split("\n"), lb = b.split("\n");
  let shown = 0;
  for (let i = 0; i < Math.max(la.length, lb.length) && shown < limit; i++) {
    if (la[i] !== lb[i]) {
      console.error(`    line ${i}:\n      ${labelA}: ${(la[i] || "<missing>").trim()}\n      ${labelB}: ${(lb[i] || "<missing>").trim()}`);
      shown++;
    }
  }
}

if (!buildMainWasm({ logLevel: "ERROR" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;

const sessions = {};
const results = {};
try {
  // ── Phase 1: the same edit on every available engine ─────────────────────
  for (const engine of ENGINES) {
    let s;
    try {
      s = await openEngine(engine, port);
    } catch (err) {
      console.log(`[SKIP] ${engine} unavailable: ${String(err.message).split("\n")[0].slice(0, 120)}`);
      continue;
    }
    sessions[engine] = s;
    try {
      const r = await s.page.evaluate(EDIT_AND_EXPORT);
      if (!r.exported) { fail(`${engine}: exportJSON returned nothing`); continue; }
      results[engine] = r;
      console.log(`[TEST] ${engine}: exported ${r.exported.length} bytes, ${r.ids.length} rows`);
    } catch (err) {
      fail(`${engine}: edit/export threw: ${String(err.message).slice(0, 200)}`);
    }
    if (s.pageErrors.length) console.error(`[${engine}] page errors:`, s.pageErrors.slice(0, 3));
  }

  const names = Object.keys(results);
  if (names.length < 2) {
    console.log(`[SKIP] need at least two engines to compare, got: ${names.join(", ") || "none"}`);
  } else {
    const base = names[0];

    // ── 1. Same state ⇒ same bytes on every engine ─────────────────────────
    for (const n of names.slice(1)) {
      if (results[base].exported !== results[n].exported) {
        fail(`export bytes differ between ${base} (${results[base].exported.length}B) and ${n} (${results[n].exported.length}B)`);
        reportJSONDiff(base, results[base].exported, n, results[n].exported);
      } else {
        console.log(`[OK] export bytes identical: ${base} == ${n}`);
      }
    }

    // ── 2. The synth voice (C in WASM) is engine-independent ───────────────
    for (const n of names.slice(1)) {
      for (const id of Object.keys(results[base].render)) {
        const a = voiceFP(results[base].render[id]);
        const b = voiceFP(results[n].render[id]);
        if (a !== b) {
          fail(`voice render for ${id} differs between ${base} and ${n} — the C DSP runs in the same WASM module on both, so this is a real divergence, not a WebAudio difference`);
          console.error(`      ${base}: ${a.slice(0, 200)}`);
          console.error(`      ${n}: ${b.slice(0, 200)}`);
        }
      }
      console.log(`[OK] synth voice renders identical: ${base} == ${n}`);
    }

    // ── 3. The handoff: export HERE, import THERE, re-export ──────────────
    // The actual user story. Each engine imports every other engine's file and
    // must re-export it unchanged, which also proves the import reconstructed
    // the same state rather than merely echoing bytes it never parsed.
    for (const from of names) {
      for (const to of names) {
        if (from === to) continue;
        const round = await sessions[to].page.evaluate(async (doc) => {
          const err = window.importJSON(doc);
          if (err) return { err };
          const out = { exported: window.exportJSON(), render: {} };
          const count = window.drumRowCount();
          for (let i = 0; i < count; i++) {
            const id = window.rowInstrument(i);
            if (id) out.render[id] = await window.__testCaptureSynthRender(id, { sampleRate: 48000 });
          }
          out.insertFX = window.getInsertEffects(Object.keys(out.render)[0]);
          return out;
        }, results[from].exported);

        if (round.err) { fail(`${from} -> ${to}: import rejected the file: ${round.err}`); continue; }
        if (round.exported !== results[from].exported) {
          fail(`${from} -> ${to}: re-export is not byte-identical to the imported file`);
          reportJSONDiff(`${from} (original)`, results[from].exported, `${to} (re-export)`, round.exported);
          continue;
        }
        // Insert-FX chains carry float params through the JSON; compare the
        // reconstructed chain too so a silently-dropped param is named.
        const fxDiff = insertFXDiff(results[from].insertFX, round.insertFX);
        if (fxDiff) {
          fail(`${from} -> ${to}: insert-effect chain not reconstructed — ${fxDiff}`);
          continue;
        }
        let voiceBad = false;
        for (const id of Object.keys(results[from].render)) {
          if (voiceFP(results[from].render[id]) !== voiceFP(round.render[id])) {
            fail(`${from} -> ${to}: voice render for ${id} changed across the handoff`);
            voiceBad = true;
          }
        }
        if (!voiceBad) console.log(`[OK] handoff ${from} -> ${to}: bytes, FX chain and voices all identical`);
      }
    }
  }
} finally {
  for (const s of Object.values(sessions)) {
    try { await s.browser.close(); } catch (_) {}
  }
  server.close();
}

if (failed) process.exit(1);
console.log("[PASS] cross-engine export/import parity");
