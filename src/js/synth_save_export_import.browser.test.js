/** Whether the pushed block survives Save, Save As, export and import is visible only here: every row's full voice PCM
 * (hashed) in the same page, a reloaded profile (Saves reloaded at boot) and a clean context (an empty library). The
 * master mix is pinned natively; here its config is pinned through the re-export bytes. */
import * as pw from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

const BOOT_TIMEOUT_MS = 90000;
let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

const READY = () =>
  typeof window.exportJSON === "function" &&
  typeof window.importJSON === "function" &&
  typeof window.saveActiveRecipe === "function" &&
  typeof window.setInstrumentParam === "function" &&
  typeof window.setSampleEdit === "function" &&
  typeof window.setRowInstrument === "function" &&
  typeof window.saveActiveRecipeAs === "function" &&
  typeof window.getInstrumentDoc === "function" &&
  typeof window.getInstrumentParams === "function" &&
  typeof window.captureInstrumentPCM === "function" &&
  typeof window.__testCaptureSynthRender === "function";

/** FNV-1a over the float32 bits: any single-sample change moves the hash. */
const INSTALL_HELPERS = () => {
  window.__rowIDs = () => {
    const ids = [];
    for (let i = 0; i < window.drumRowCount(); i++) {
      const id = window.rowInstrument(i);
      if (id && !ids.includes(id)) ids.push(id);
    }
    return ids;
  };
  window.__voiceHash = async (id) => {
    const r = await window.__testCaptureSynthRender(id);
    if (!r) return "none";
    const pcm = window.captureInstrumentPCM(id, false);
    if (!pcm) return "none";
    const u8 = pcm.bytes;
    let h = 0x811c9dc5;
    for (let i = 0; i < u8.length; i++) { h ^= u8[i]; h = Math.imul(h, 0x01000193) >>> 0; }
    return `${pcm.length}@${pcm.sr}:${h.toString(16)}`;
  };
  window.__voices = async () => {
    const out = {};
    for (const id of window.__rowIDs()) out[id] = await window.__voiceHash(id);
    return out;
  };
};

async function openPage(context, port) {
  const page = await context.newPage();
  const pageErrors = [];
  page.on("pageerror", (e) => pageErrors.push(String(e.message)));
  if (process.env.TEST_LOG) page.on("console", (m) => console.log("[page]", m.type(), m.text()));
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  await page.waitForFunction(READY, null, { timeout: BOOT_TIMEOUT_MS, polling: 500 });
  await page.evaluate(INSTALL_HELPERS);
  return { page, pageErrors };
}

/** Compare two {id: hash} maps; returns the ids whose voice differs. */
function voiceDiff(want, got) {
  const bad = [];
  for (const id of Object.keys(want)) if (want[id] !== got[id]) bad.push(`${id}: ${want[id]} -> ${got[id]}`);
  return bad;
}

if (!buildMainWasm({ logLevel: "ERROR" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;
const launchOpts = { args: ["--autoplay-policy=no-user-gesture-required"] };
const browser = await pw.chromium.launch(launchOpts);

try {
  const profile = await browser.newContext();
  const { page: a, pageErrors } = await openPage(profile, port);

  // ── 1. Edit everything the file must carry, the way the UI does ──────────
  const edited = await a.evaluate(async () => {
    const out = { steps: {} };
    const ids = window.__rowIDs();
    out.ids = ids;
    out.origins = Object.fromEntries(ids.map((id) => [id, (JSON.parse(window.getInstrumentDoc(id) || "{}").origin || "")]));
    out.steps.loaded = await window.__voices();
    // Reopened later in a profile with the user's Saves, the Saved instruments must play the SAVED tone.
    out.original = window.exportJSON();

    // "Selected instruments": swap the LAST row to a different instrument.
    const swapTo = "fm-bell";
    window.setRowInstrument(ids.length - 1, swapTo);
    out.swapped = window.rowInstrument(ids.length - 1);

    // One write at a time, as the Synth tab does; sorted keys with a stride so every module is touched several times.
    const knob = (id, stride, bump) => {
      const defs = window.getInstrumentParams(id) || {};
      const keys = Object.keys(defs).sort().filter((k) => !/(_enabled|_on|_type|variant|order|voices)$/.test(k));
      for (let n = 0; n < keys.length; n += stride) {
        const k = keys[n];
        const v = defs[k];
        window.setInstrumentParam(id, k, v === 0 ? bump : v * (1 + bump));
      }
    };
    for (const id of window.__rowIDs()) knob(id, 7, 0.15);
    out.steps.edited = await window.__voices();

    // Synth-tab SAVE over the existing instrument, for every row.
    for (const id of window.__rowIDs()) window.saveActiveRecipe(id);
    out.steps.saved = await window.__voices();

    // On a clean machine the file is the only carrier of a Save As tone.
    const rows = window.__rowIDs();
    knob(rows[2], 5, 0.3);
    out.steps.beforeSaveAs = await window.__voiceHash(rows[2]);
    out.saveAsID = window.saveActiveRecipeAs(rows[2], "My Tuned " + rows[2]);
    out.steps.afterSaveAs = await window.__voiceHash(rows[2]);

    // Saved again: a user's Save trumps the circuit's document on open, so an unsaved edit would come back as the Saved tone.
    knob(rows[0], 11, -0.2);
    window.saveActiveRecipe(rows[0]);

    // Sampler edit (non-destructive descriptor) on the second row.
    window.setSampleEdit(rows[1], {
      start_frac: 0.02, end_frac: 0.8, transpose_semis: 3, detune_cents: -12,
      gain_db: -2.5, fade: 1, reverse: 0, normalize: 1,
    });

    // Insert FX with non-default params, and a per-instrument + master EQ.
    window.addInsertEffect(rows[0], "distortion");
    window.setInsertEffectParam(rows[0], 0, "drive", 1.7);
    window.setInsertEffectParam(rows[0], 0, "mix", 0.45);
    window.setEQChannel(rows[0]);
    window.setEQBandGain(2, 5.5);
    window.setEQHPF(true, 65);
    window.setEQChannel("main");
    window.setEQBandGain(7, -3);
    out.fx = window.getInsertEffects(rows[0]);

    out.steps.final = await window.__voices();
    out.exported = window.exportJSON();
    return out;
  });
  console.log(`[TEST] rows ${edited.ids.join(", ")} (last swapped to ${edited.swapped}); origins ${JSON.stringify(edited.origins)}`);

  // Preconditions: the edits must be audible, or nothing below proves anything.
  const moved = Object.keys(edited.steps.edited).filter((id) => edited.steps.loaded[id] && edited.steps.loaded[id] !== edited.steps.edited[id]);
  if (moved.length < 3) fail(`knob edits moved only ${moved.length} voices (${moved.join(", ")}) — the test is not exercising the synth`);
  if (edited.steps.final[edited.ids[1]] === edited.steps.saved[edited.ids[1]]) fail(`the sample edit on ${edited.ids[1]} did not change its voice`);
  if (!edited.exported) fail("exportJSON returned nothing");

  // Save copies the ALREADY-HEARD document; the row's block does not change.
  for (const d of voiceDiff(edited.steps.edited, edited.steps.saved)) fail(`Synth-tab Save changed the voice: ${d}`);
  if (!edited.saveAsID) fail("Save-As (saveActiveRecipeAs) saved nothing");
  if (edited.steps.beforeSaveAs !== edited.steps.afterSaveAs) {
    fail(`Synth-tab Save-As changed the voice of ${edited.ids[2]}: ${edited.steps.beforeSaveAs} -> ${edited.steps.afterSaveAs}`);
  }

  // ── 3. The same page re-imports its own file ─────────────────────────────
  const same = await a.evaluate(async (doc) => {
    const err = window.importJSON(doc);
    return { err, voices: await window.__voices(), exported: window.exportJSON(), fx: window.getInsertEffects(window.__rowIDs()[0]) };
  }, edited.exported);
  const check = (label, r) => {
    if (r.err) { fail(`${label}: import rejected the file: ${r.err}`); return; }
    const bad = voiceDiff(edited.steps.final, r.voices);
    for (const d of bad) fail(`${label}: voice differs from what was exported: ${d}`);
    if (r.exported !== edited.exported) {
      fail(`${label}: re-export is not byte-identical to the imported file`);
      const la = edited.exported.split("\n"), lb = r.exported.split("\n");
      for (let i = 0, shown = 0; i < Math.max(la.length, lb.length) && shown < 8; i++) {
        if (la[i] !== lb[i]) { console.error(`    line ${i}: exported ${(la[i] || "<missing>").trim()} | re-export ${(lb[i] || "<missing>").trim()}`); shown++; }
      }
    }
    if (JSON.stringify(sortFX(r.fx)) !== JSON.stringify(sortFX(edited.fx))) fail(`${label}: insert-FX chain not reconstructed: ${JSON.stringify(r.fx)}`);
    if (!bad.length && r.exported === edited.exported) console.log(`[OK] ${label}: every voice byte-identical, re-export identical`);
  };
  check("same page", same);

  // ── 4. A reloaded page in the same profile (Saves in the library) ────────
  const { page: b } = await openPage(profile, port);
  check("reloaded page", await b.evaluate(async (doc) => {
    const err = window.importJSON(doc);
    return { err, voices: await window.__voices(), exported: window.exportJSON(), fx: window.getInsertEffects(window.__rowIDs()[0]) };
  }, edited.exported));

  // Saves loaded at boot: each Saved instrument the circuit carries plays the user's tone, not the circuit's.
  const reopened = await b.evaluate(async (doc) => {
    const err = window.importJSON(doc);
    return { err, voices: await window.__voices() };
  }, edited.original);
  if (reopened.err) fail(`reopen original: import rejected the file: ${reopened.err}`);
  else {
    const [r0, r1, r2] = edited.ids;
    const want = { [r0]: edited.steps.final[r0], [r1]: edited.steps.saved[r1], [r2]: edited.steps.saved[r2] };
    const bad = voiceDiff(want, reopened.voices);
    for (const d of bad) fail(`reopen original: a Saved instrument did not play the user's saved tone: ${d}`);
    if (want[r0] === edited.steps.loaded[r0]) fail("reopen original: premise — the Save did not move row 0's voice");
    if (!bad.length) console.log("[OK] reopen original: the user's Saves trump the circuit's instruments");
  }

  // ── 5. A clean browser context: another machine, an empty library ────────
  const clean = await browser.newContext();
  const { page: c } = await openPage(clean, port);
  check("clean profile", await c.evaluate(async (doc) => {
    const err = window.importJSON(doc);
    return { err, voices: await window.__voices(), exported: window.exportJSON(), fx: window.getInsertEffects(window.__rowIDs()[0]) };
  }, edited.exported));

  if (pageErrors.length) console.error("[page errors]", pageErrors.slice(0, 3));
} finally {
  await browser.close();
  server.close();
}

/** getInsertEffects ranges a Go map for params — compare with sorted keys. */
function sortFX(fx) {
  return (fx || []).map((s) => ({ ...s, params: Object.fromEntries(Object.entries(s.params || {}).sort()) }));
}

if (failed) process.exit(1);
console.log("[PASS] edit -> Save -> export -> import plays back byte-identical voices");
