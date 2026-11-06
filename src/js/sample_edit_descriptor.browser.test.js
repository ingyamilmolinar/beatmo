/**
 * Non-destructive sample-edit descriptor — Go↔JS boundary coverage.
 *
 * A synth instrument with a saved Sampler edit KEEPS its C-synth render
 * (RENDER[id]); the edit (trim/pitch/gain/reverse/normalize/fade) is applied
 * to the freshly-rendered buffer inside ensureRenderedSample, mirroring the
 * native dispatcher (tryRecipeVoice → ApplySampleEditToBuffer). This test
 * pins the JS side of that boundary:
 *
 *  1. applySampleEdit matches Go's BakeSample bit-for-bit (within float32
 *     tolerance) on a deterministic golden generated from the REAL Go
 *     implementation (C renders are noise-seeded, so a re-render comparison
 *     would not be reproducible). Go-side dispatcher==BakeSample is pinned by
 *     TestSampleEditDispatch_TransformMatchesBakeSample (Go, native build).
 *  2. window.updateSampleEdit (the Go platformSampleEditChanged bridge target)
 *     edits the next render; the identity edit clears the descriptor.
 *  3. A SAMPLE-based instrument must keep its audio across updateSampleEdit
 *     (same renderCache-drop guard as _updateInstrumentParams — the cache
 *     holds a sample's ONLY PCM copy).
 *  4. captureInstrumentPCM(id, true) returns the UN-edited render for the
 *     Sampler editor (raw capture), longer than the trimmed edited render.
 *  5. FULL-PIPELINE parity: the production WASM render of a descriptor-bearing
 *     instrument (ensureRenderedSample: C render → normalize → gain →
 *     applySampleEdit) matches the native dispatcher's full pipeline
 *     (recipe.Render → normalizeAndScale → ApplySampleEditToBuffer) on a
 *     trim+gain+reverse edit. Item 1 pins the TRANSFORM in isolation; this
 *     pins WHERE in the pipeline it is applied — a regression that moved the
 *     edit before normalize/gain on either side passes 1 but fails 5.
 *     Golden (PIPE_GOLD): regenerate with
 *       cd src/go && BEATMO_AUDIO_SAMPLE_RATE=48000 \
 *         ../../.tools/go/bin/go run ./cmd/gen-sample-edit-golden
 *     Tolerances are xplat-style (CGo vs Emscripten float drift, see
 *     xplat_audio_compare.browser.test.js), far below pipeline-order errors.
 *
 * Golden vectors: generated from Go with BakeSample's transforms in its order
 * (TrimSample → ReverseSample → ResampleSemitones → NormalizePeak → ApplyGainDB
 * → ApplyFades) on
 *   src[i] = float32(sin(2π·4·i/64)·(1−i/64)), i in [0,64)
 *   edit = {StartFrac:0.125, EndFrac:0.875, TransposeSemis:3, DetuneCents:-15,
 *           GainDB:-6, Reverse:true, Normalize:true}, fade 0.25 ms
 *   sr = 48000
 * The fade is the wire `FadeMs` Go sends with a Fade-on edit (audio.SampleFadeMs
 * in the app; a shorter one here so the 40-sample vector keeps a middle). See
 * sample_edit.go BakeSample; regenerate with a scratch test against
 * internal/audio if the transform order ever changes.
 *
 * Run: GO=/abs/path/.tools/go/bin/go node src/js/sample_edit_descriptor.browser.test.js
 */
import { chromium } from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

const GOLD_INPUT = [0, 0.376704, 0.6850097, 0.8805727, 0.9375, 0.85170144, 0.6408155, 0.34082744, 1.07156596e-16, -0.32886857, -0.59662133, -0.7650877, -0.8125, -0.7362165, -0.5524272, -0.292992, -1.8369701e-16, 0.28103316, 0.508233, 0.6496028, 0.6875, 0.62073153, 0.46403882, 0.24515657, 2.2962126e-16, -0.23319772, -0.41984466, -0.5341179, -0.5625, -0.50524664, -0.37565047, -0.19732115, -2.4492937e-16, 0.1853623, 0.3314563, 0.41863292, 0.4375, 0.3897617, 0.28726214, 0.14948572, 2.2962126e-16, -0.13752685, -0.24306795, -0.30314797, -0.3125, -0.27427673, -0.19887379, -0.10165029, -1.8369701e-16, 0.08969143, 0.15467961, 0.18766303, 0.1875, 0.1587918, 0.110485435, 0.05381486, 1.07156596e-16, -0.041856002, -0.06629126, -0.07217809, -0.0625, -0.043306854, -0.022097087, -0.0059794285];
const GOLD_EXPECTED = [0, 0.006354005, 0.01803493, 0.030015977, 0.03500044, 0.025743587, -0.002397074, -0.047122397, -0.098744184, -0.14286375, -0.16273963, -0.14372216, -0.07505058, 0.031221665, 0.14023331, 0.22874738, 0.2758375, 0.2655928, 0.19147521, 0.071182534, -0.073118895, -0.21279998, -0.31814137, -0.35792005, -0.32029998, -0.21212934, -0.05184476, 0.13048586, 0.27322316, 0.33782676, 0.32331914, 0.2441628, 0.1281402, 0.008518636, -0.08396395, -0.12817833, -0.12319904, -0.0835312, -0.0336188, -0];
const GOLD_EDIT = { StartFrac: 0.125, EndFrac: 0.875, TransposeSemis: 3, DetuneCents: -15, GainDB: -6, Fade: true, FadeMs: 0.25, Reverse: true, Normalize: true };

// Full-pipeline golden: native render of "kick" with PIPE_EDIT applied by the
// voice dispatcher (audio.RenderInstrumentOneShot). Fingerprint shape mirrors
// __testCaptureSynthRender: head = data[0:64], tail = data[len-256:len-192].
// Regenerate: cd src/go && BEATMO_AUDIO_SAMPLE_RATE=48000 ../../.tools/go/bin/go run ./cmd/gen-sample-edit-golden
const PIPE_EDIT = { StartFrac: 0.25, EndFrac: 0.875, GainDB: -6, Reverse: true };
const PIPE_GOLD = {
  length: 15000, peak: 0.1150633841753006, rms: 0.03025083278811243,
  head: [-0.0017168628, -0.0017119884, -0.0017070469, -0.0017020524, -0.0016968483, -0.0016915286, -0.0016861543, -0.0016807425, -0.0016751433, -0.0016694589, -0.0016637533, -0.0016580345, -0.0016521277, -0.0016460866, -0.0016400733, -0.0016339885, -0.0016276826, -0.0016213683, -0.0016149707, -0.0016084415, -0.0016017993, -0.001595012, -0.0015882096, -0.0015813009, -0.0015743538, -0.0015672997, -0.0015601353, -0.0015528874, -0.0015456387, -0.0015382271, -0.0015307883, -0.0015231952, -0.0015155986, -0.0015077716, -0.0015000018, -0.0014919942, -0.0014839338, -0.0014758268, -0.0014676861, -0.0014594774, -0.0014511257, -0.0014427253, -0.0014341965, -0.0014255747, -0.0014168414, -0.0014081264, -0.0013993582, -0.0013903695, -0.0013814277, -0.0013723787, -0.0013631633, -0.0013539286, -0.0013446696, -0.0013352403, -0.0013256962, -0.0013162243, -0.0013065989, -0.0012968915, -0.0012871091, -0.0012772883, -0.0012672974, -0.0012571953, -0.0012470364, -0.0012367829],
  tail: [-0.11478149, -0.11477125, -0.11485361, -0.11479106, -0.11480079, -0.114837736, -0.114890695, -0.11491728, -0.1148754, -0.11478721, -0.11476948, -0.11469331, -0.114737965, -0.114649385, -0.11455364, -0.1146169, -0.114523135, -0.114584655, -0.11469486, -0.114762545, -0.11471967, -0.11483151, -0.11489956, -0.11495727, -0.115005955, -0.11504441, -0.115063384, -0.114936225, -0.114947684, -0.115016, -0.11493818, -0.11481121, -0.11475247, -0.11464212, -0.11463685, -0.11450822, -0.114420064, -0.11440198, -0.11431382, -0.11425016, -0.1142401, -0.1142417, -0.11409999, -0.11394585, -0.11388819, -0.11372751, -0.1137191, -0.113613054, -0.11368141, -0.113618076, -0.11356023, -0.11339209, -0.1132246, -0.11309917, -0.112919606, -0.11282703, -0.11286244, -0.11265003, -0.11268738, -0.11253957, -0.11238519, -0.11216197, -0.11202821, -0.11190461],
};

if (!buildMainWasm({ logLevel: "INFO" })) throw new Error("go build main.wasm failed");
const server = await createServer();
const port = server.address().port;

const browser = await chromium.launch({ args: ["--autoplay-policy=no-user-gesture-required"] });
const page = await browser.newPage();
const pageErrors = [];
page.on("pageerror", (e) => pageErrors.push(String(e.message)));
if (process.env.TEST_LOG) page.on("console", (m) => { try { console.log("[PAGE]", m.type(), m.text()); } catch (_) {} });

let failed = false;
const fail = (m) => { failed = true; console.error("[FAIL] " + m); };

try {
  await page.goto(`http://localhost:${port}/`, { waitUntil: "load" });
  // updateSampleEdit / __testApplySampleEdit / __testCaptureSynthRender are all
  // defined at audio.js MODULE LOAD — they exist before the Go runtime boots.
  // The full-pipeline parity (item 5) renders a MIGRATED instrument (kick →
  // render_modular_p), whose param block (its document, flattened) Go pushes via
  // setInstrumentBlock during its synchronous init. setInstrumentParam is
  // registered in that SAME init, so gating on it guarantees the block is there;
  // without this gate the test could render kick before any block arrives. Mirrors instrument_params_audible's gate.
  await page.waitForFunction(() => typeof window.updateSampleEdit === "function" && typeof window.__testApplySampleEdit === "function" && typeof window.__testCaptureSynthRender === "function" && typeof window.setInstrumentParam === "function", null, { timeout: 30000 });

  // ── 1. Transform parity with Go BakeSample (golden) ──────────────────────
  const got = await page.evaluate(({ input, edit }) =>
    window.__testApplySampleEdit(input, 48000, JSON.stringify(edit)), { input: GOLD_INPUT, edit: GOLD_EDIT });
  if (got.length !== GOLD_EXPECTED.length) {
    fail(`transform parity: length ${got.length} != Go golden ${GOLD_EXPECTED.length}`);
  } else {
    let maxDiff = 0;
    for (let i = 0; i < got.length; i++) maxDiff = Math.max(maxDiff, Math.abs(got[i] - GOLD_EXPECTED[i]));
    console.log(`[TEST] transform parity maxDiff=${maxDiff.toExponential(2)}`);
    if (!(maxDiff < 1e-6)) fail(`transform parity: maxDiff=${maxDiff} (JS applySampleEdit drifted from Go BakeSample)`);
  }

  // ── 2-4. Bridge + render integration on a real C-synth instrument ────────
  const INST = "snare";
  const r = await page.evaluate(async (id) => {
    const sleep = (ms) => new Promise((res) => setTimeout(res, ms));
    const out = {};
    const base = await window.__testCaptureSynthRender(id);
    out.baseLen = base ? base.length : 0;

    // Half-trim via the Go bridge target → next render must halve.
    window.updateSampleEdit(id, JSON.stringify({ StartFrac: 0, EndFrac: 0.5 }));
    const edited = await window.__testCaptureSynthRender(id);
    out.editedLen = edited ? edited.length : 0;

    // Raw capture (Sampler editor) must see the UN-edited length. First call
    // kicks an async un-edited render; retry like the Go side does.
    let raw = null;
    for (let i = 0; i < 50 && !raw; i++) {
      raw = window.captureInstrumentPCM(id, true);
      if (!raw) await sleep(50);
    }
    out.rawLen = raw ? raw.length : 0;

    // Identity edit clears the descriptor → render restores to full length.
    window.updateSampleEdit(id, JSON.stringify({ StartFrac: 0, EndFrac: 1 }));
    const restored = await window.__testCaptureSynthRender(id);
    out.restoredLen = restored ? restored.length : 0;

    // Sample-based instrument: descriptor change must NOT drop its only PCM.
    const sr = 44100, frames = Math.floor(sr * 0.25);
    const f32 = new Float32Array(frames);
    for (let i = 0; i < frames; i++) f32[i] = Math.sin(2 * Math.PI * 220 * i / sr);
    window.registerSamplePCM("user.sample.editguard", new Uint8Array(f32.buffer.slice(0)), sr);
    window.updateSampleEdit("user.sample.editguard", JSON.stringify({ StartFrac: 0, EndFrac: 0.5 }));
    const cap = window.captureInstrumentPCM("user.sample.editguard");
    out.sampleLen = cap ? cap.length : 0;
    out.sampleWant = frames;
    return out;
  }, INST);

  console.log(`[TEST] lens base=${r.baseLen} edited=${r.editedLen} raw=${r.rawLen} restored=${r.restoredLen} sample=${r.sampleLen}/${r.sampleWant}`);
  if (!(r.baseLen > 0)) fail("base render is empty — setup issue");
  if (!(Math.abs(r.editedLen - Math.floor(r.baseLen / 2)) <= 1)) fail(`half-trim edit: editedLen=${r.editedLen}, want ~${Math.floor(r.baseLen / 2)}`);
  if (!(r.rawLen === r.baseLen)) fail(`raw capture: rawLen=${r.rawLen}, want un-edited ${r.baseLen}`);
  if (!(r.restoredLen === r.baseLen)) fail(`identity edit must clear: restoredLen=${r.restoredLen}, want ${r.baseLen}`);
  if (!(r.sampleLen === r.sampleWant)) fail(`sample instrument PCM dropped by updateSampleEdit: len=${r.sampleLen}, want ${r.sampleWant}`);

  // ── 5. Full-pipeline parity vs the native dispatcher (kick golden) ───────
  const pipe = await page.evaluate(async (edit) => {
    window.updateSampleEdit("kick", JSON.stringify(edit));
    const fp = await window.__testCaptureSynthRender("kick");
    // Clear the descriptor so later runs/tests see the shipped kick.
    window.updateSampleEdit("kick", JSON.stringify({ StartFrac: 0, EndFrac: 1 }));
    return fp;
  }, PIPE_EDIT);
  if (!pipe || !pipe.length) {
    fail("pipeline parity: kick render with descriptor produced nothing");
  } else {
    // Length is integer math on both sides (floor(0.5*48000) trimmed by the
    // same BakeSample port) — must match exactly.
    if (pipe.length !== PIPE_GOLD.length) fail(`pipeline parity: length=${pipe.length}, native golden=${PIPE_GOLD.length}`);
    // CGo vs Emscripten float drift is ~1e-6..1e-4 (see xplat warnings); a
    // pipeline-order regression (edit before normalize/gain) shifts peak/rms
    // by >10x and head/tail samples wholesale.
    const rel = (a, b) => Math.abs(a - b) / Math.max(Math.abs(b), 1e-12);
    if (rel(pipe.peak, PIPE_GOLD.peak) > 0.02) fail(`pipeline parity: peak=${pipe.peak}, native=${PIPE_GOLD.peak} (>2% off — edit applied at the wrong pipeline stage?)`);
    if (rel(pipe.rms, PIPE_GOLD.rms) > 0.02) fail(`pipeline parity: rms=${pipe.rms}, native=${PIPE_GOLD.rms} (>2% off)`);
    const segDiff = (got, want, name) => {
      if (!got || got.length !== want.length) { fail(`pipeline parity: ${name} length ${got ? got.length : 0} != ${want.length}`); return; }
      let maxDiff = 0;
      for (let i = 0; i < want.length; i++) maxDiff = Math.max(maxDiff, Math.abs(got[i] - want[i]));
      console.log(`[TEST] pipeline parity ${name} maxDiff=${maxDiff.toExponential(2)}`);
      if (!(maxDiff < 2e-3)) fail(`pipeline parity: ${name} maxDiff=${maxDiff} vs native golden (trim/reverse/gain placement drifted)`);
    };
    segDiff(pipe.head, PIPE_GOLD.head, "head");
    segDiff(pipe.tail, PIPE_GOLD.tail, "tail");
  }
} catch (e) {
  fail("exception: " + (e && e.message ? e.message : String(e)));
} finally {
  await browser.close();
  server.close();
}

if (pageErrors.length) console.log("[TEST] page errors:", pageErrors.slice(0, 10));
if (failed || pageErrors.length) { console.error("[TEST] sample-edit descriptor: FAIL"); process.exit(1); }
console.log("[TEST] sample-edit descriptor: PASS");
