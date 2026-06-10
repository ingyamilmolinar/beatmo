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
 *       cd src/go && AUDIO_SAMPLE_RATE=48000 \
 *         ../../.tools/go/bin/go run ./cmd/gen-sample-edit-golden
 *     Tolerances are xplat-style (CGo vs Emscripten float drift, see
 *     xplat_audio_compare.browser.test.js), far below pipeline-order errors.
 *
 * Golden vectors: generated from Go via audio.BakeSample with
 *   src[i] = float32(sin(2π·4·i/64)·(1−i/64)), i in [0,64)
 *   edit = {StartFrac:0.125, EndFrac:0.875, TransposeSemis:3, DetuneCents:-15,
 *           GainDB:-6, FadeInMs:0.2, FadeOutMs:0.3, Reverse:true, Normalize:true}
 *   sr = 48000
 * (see sample_edit.go BakeSample; regenerate with a scratch `go run` against
 *  internal/audio if the transform order ever changes).
 *
 * Run: GO=/abs/path/.tools/go/bin/go node src/js/sample_edit_descriptor.browser.test.js
 */
import { chromium } from "playwright";
import { buildMainWasm, createServer } from "./real_input_test_helpers.js";

const GOLD_INPUT = [0, 0.376704, 0.6850097, 0.8805727, 0.9375, 0.85170144, 0.6408155, 0.34082744, 1.07156596e-16, -0.32886857, -0.59662133, -0.7650877, -0.8125, -0.7362165, -0.5524272, -0.292992, -1.8369701e-16, 0.28103316, 0.508233, 0.6496028, 0.6875, 0.62073153, 0.46403882, 0.24515657, 2.2962126e-16, -0.23319772, -0.41984466, -0.5341179, -0.5625, -0.50524664, -0.37565047, -0.19732115, -2.4492937e-16, 0.1853623, 0.3314563, 0.41863292, 0.4375, 0.3897617, 0.28726214, 0.14948572, 2.2962126e-16, -0.13752685, -0.24306795, -0.30314797, -0.3125, -0.27427673, -0.19887379, -0.10165029, -1.8369701e-16, 0.08969143, 0.15467961, 0.18766303, 0.1875, 0.1587918, 0.110485435, 0.05381486, 1.07156596e-16, -0.041856002, -0.06629126, -0.07217809, -0.0625, -0.043306854, -0.022097087, -0.0059794285];
const GOLD_EXPECTED = [0, 0.008472007, 0.024046572, 0.040021304, 0.04666725, 0.034324784, -0.0031960986, -0.06282986, -0.13165891, -0.190485, -0.19528757, -0.15678781, -0.07505058, 0.031221665, 0.14023331, 0.22874738, 0.2758375, 0.2655928, 0.19147521, 0.071182534, -0.073118895, -0.21279998, -0.31814137, -0.35792005, -0.32029998, -0.21212934, -0.04814156, 0.111845024, 0.23419128, 0.2895658, 0.2771307, 0.20928241, 0.109834455, 0.007301688, -0.07196911, -0.10986714, -0.10559918, -0.07159817, -0.028816113, -0];
const GOLD_EDIT = { StartFrac: 0.125, EndFrac: 0.875, TransposeSemis: 3, DetuneCents: -15, GainDB: -6, FadeInMs: 0.2, FadeOutMs: 0.3, Reverse: true, Normalize: true };

// Full-pipeline golden: native render of "kick" with PIPE_EDIT applied by the
// voice dispatcher (audio.RenderInstrumentOneShot). Fingerprint shape mirrors
// __testCaptureSynthRender: head = data[0:64], tail = data[len-256:len-192].
// Regenerate: cd src/go && AUDIO_SAMPLE_RATE=48000 .tools go run ./cmd/gen-sample-edit-golden
const PIPE_EDIT = { StartFrac: 0.25, EndFrac: 0.875, GainDB: -6, Reverse: true };
const PIPE_GOLD = {
  length: 15000, peak: 0.067180395, rms: 0.016524324,
  head: [0.00076768914, 0.0007702655, 0.00077278976, 0.0007752513, 0.000777767, 0.0007802672, 0.00078270683, 0.00078507396, 0.0007874794, 0.0007898479, 0.00079213246, 0.000794327, 0.000796562, 0.0007987971, 0.0008009133, 0.0008029839, 0.0008051201, 0.0008071649, 0.000809174, 0.0008111832, 0.0008131795, 0.0008151862, 0.00081710826, 0.00081901293, 0.0008208506, 0.00082267204, 0.0008244801, 0.00082625577, 0.0008279374, 0.0008296461, 0.00083128066, 0.0008329368, 0.00083450245, 0.00083614595, 0.00083765533, 0.0008392486, 0.00084078946, 0.0008422737, 0.0008436917, 0.00084507035, 0.00084646454, 0.000847805, 0.00084915134, 0.0008504776, 0.0008517979, 0.00085301645, 0.0008541868, 0.0008554326, 0.000856557, 0.000857674, 0.000858828, 0.00085991056, 0.00086092565, 0.00086198194, 0.0008630385, 0.0008639571, 0.00086490595, 0.00086583185, 0.0008667304, 0.000867575, 0.00086846313, 0.0008693522, 0.0008702014, 0.00087104016],
  tail: [0.036700904, 0.03700682, 0.037235357, 0.037567656, 0.03783818, 0.038081385, 0.038305864, 0.03854397, 0.03882785, 0.039140675, 0.039393507, 0.039684627, 0.03987752, 0.040165756, 0.040453378, 0.0406139, 0.040887978, 0.041037902, 0.04114485, 0.041278336, 0.041490223, 0.04157855, 0.041694466, 0.041812584, 0.041931905, 0.042053383, 0.042184103, 0.04242053, 0.04254584, 0.042622358, 0.042804632, 0.043018814, 0.043175586, 0.043366197, 0.043471385, 0.04366502, 0.043822445, 0.04392116, 0.04406777, 0.044190347, 0.04426678, 0.04432895, 0.044494703, 0.0446645, 0.044755537, 0.044919483, 0.044962257, 0.045073826, 0.0450474, 0.04511563, 0.045174222, 0.045311123, 0.045442022, 0.045535434, 0.045664426, 0.045721777, 0.045676354, 0.0458135, 0.045755364, 0.045832176, 0.045908254, 0.046030857, 0.04607979, 0.046115223],
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
  await page.waitForFunction(() => typeof window.updateSampleEdit === "function" && typeof window.__testApplySampleEdit === "function" && typeof window.__testCaptureSynthRender === "function", { timeout: 30000 });

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
