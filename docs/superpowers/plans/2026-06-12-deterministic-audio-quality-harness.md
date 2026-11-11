# Deterministic Audio-Quality Harness + Node-Rule Choppiness Fix — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a deterministic offline harness that renders a *complex node-rule circuit* through a *trivially-predictable instrument*, captures the sequenced master PCM, and validates it byte-for-byte / DSP-exact (catching dropped/doubled/mistimed triggers, NaN, denormals, clipping, added noise); extend the benchmark to attribute CPU cost across synth, sampler, **and node-logic**; then fix the node-rule choppiness root cause the bench pins.

**Architecture:** Compose existing primitives — `mixer.Read()` block loop (no Oto) + `audio.StartOutputCapture/StopOutputCapture` (post-master-chain float64 PCM) + `Predictor.AudibleAt/TriggeredAt` (deterministic trigger schedule) + `audio.SetNowForTest` (virtual clock) — into a reusable `RenderCircuitOffline` test helper. A pure-Go impulse instrument makes the expected master analytically known. The same offline machinery deterministically times `Predictor.Ensure()` under escalating node-rule load, giving a Go analog of the existing OfflineAudioContext FX-cost bench. The choppiness is a *realtime delivery* problem, so the offline render validates **signal correctness**; a separate deterministic **CPU-cost** measurement attributes the **realtime starvation** cause.

**Tech Stack:** Go 1.23 (`.tools/go/bin/go`, `-tags test -modfile=go.test.mod` fast path; some audio tests need `!test` + xvfb), JS/Playwright OfflineAudioContext bench, WASM parity.

---

## Background / Evidence (do not re-derive)

- **Symptom:** lag/choppiness in playback, **worse when node rules are added**; reported on **both desktop and browser, worse on WASM**.
- **Prior session (browser):** seqFireLate "Stage A" 150–255 ms, **chain-independent** — single WASM thread saturated by Ebiten draw starves the sequencer goroutine. Lookahead (4–150 ms) absorbs steady lateness (overdue≈0); spikes exceed it → intermittent clicks.
- **"Both platforms" refines this:** desktop runs the sequencer on a preemptive goroutine, so draw-starvation can't be the *whole* story. There must be a **shared, platform-independent cost on the scheduling critical path** that grows with node-rule complexity. Tracing the path (`core/engine/predictor_*.go`, `internal/ui/game_sequencer_schedule.go`):
  - A node-logic edit sets `predDirty=true` (`predictor_api.go:12`).
  - Next `Ensure()` **force-rebuilds the whole window and re-walks history `O(windowStart × rows)`** (`predictor_compute.go:154-257`), independent of how small the edit was.
  - `trigger_if_prev_*` logic kinds add an `O(≤64)` backward walk **per cell per row** during that rebuild (`predictor_logic.go:74-101`).
  - This cost is on the scheduling thread on **both** platforms; WASM is worse because it *also* contends with draw on one thread.
- **Reverb change (uncommitted in `audio.js`):** ConvolverNode→algorithmic Schroeder. Real CPU win (FX bench), but node rules add **no** reverb, so it **cannot** be the node-rule symptom's cause. Evaluate keep-vs-revert on bench data (Phase 3); user is open to either.
- **Existing primitives confirmed present** (file:line in the tasks below). Reverb FX-cost bench: `scripts/bench_audio_node_cost.mjs` (running).

**Key insight that shapes the harness:** an offline/deterministic render is **always clean** (samples are *computed* correctly; the glitch is in *realtime delivery*). Therefore:
- **Offline render → signal-correctness** (Phase 1): dropped/doubled/mistimed triggers, NaN, denormal, clipping, noise, mixer bugs. Byte-exact, CI-able.
- **Deterministic CPU-cost of `Ensure()`/scheduling → realtime starvation cause** (Phase 2): attributes the choppiness to node-logic without needing real-HW realtime.

---

## File Structure

- `src/go/internal/audio/offline_render.go` (NEW, `//go:build !test`) — `RenderCircuitOffline` + deterministic impulse voice; the reusable harness. `!test` because it needs the real mixer (the `test` tag stubs EQ; we want the real master chain). Lives in `audio` to reach unexported `mixer`.
- `src/go/internal/audio/offline_render_test.go` (NEW, `!test`) — unit tests for the harness primitive (impulse placement, silence floor, determinism/golden hash).
- `src/go/internal/audio/audio_quality_harness_test.go` (NEW, `!test`) — the headline "complex circuit, simple audio" validation: builds a node-rule-heavy circuit, renders sequenced master, asserts analytic + golden.
- `src/go/internal/audio/node_logic_cost_test.go` (NEW, `!test`) — deterministic `Ensure()`/render CPU-cost measurement across node-rule load (the synth/sampler/node-logic bench coverage, Go side).
- `scripts/bench_audio_node_cost.mjs` (MODIFY) — add `synth`/`sampler` voice-cost rows so the JS bench covers more than insert-FX.
- `src/js/audio_quality_harness.browser.test.js` (NEW) — WASM parity of the Phase-1 harness (same circuit, OfflineAudioContext-style capture, DSP-tolerance compare to the Go golden).
- Phase 4 fix files: TBD by Phase-2 measurement (likely `core/engine/predictor_compute.go` and/or `predictor_api.go`).

---

## Task 1: Deterministic impulse voice + `RenderCircuitOffline` primitive

**Files:**
- Create: `src/go/internal/audio/offline_render.go`
- Test: `src/go/internal/audio/offline_render_test.go`

Reference primitives (read before writing):
- `engine_mixer.go` — `mixer` struct fields, `Schedule(id, v Voice, delaySamples int)`, `Read([]byte)`.
- `engine_stop.go:183-240` — `StartOutputCapture/StopOutputCapture/ClearOutputCapture` (float64 PCM, post-EQ/compressor/limiter).
- `mixer_signal_test.go:35-44` — `readMixerSamples` block-drive pattern (build a fresh `mixer{}` with the buffer fields; no Oto).
- `engine_state.go:43-60` — `SetNowForTest`.
- `core/engine/predictor_*.go` — `Predictor`, `Ensure(horizon)`, `AudibleAt(row, idx)`.

- [ ] **Step 1: Write the failing test** — a single impulse scheduled at a known sample offset lands at that offset and is silent elsewhere.

```go
//go:build !test

package audio

import (
	"math"
	"testing"
)

// impulseVoice renders a single full-scale sample then silence — the simplest
// analytically-predictable signal. After the master chain (headroom, EQ,
// limiter) it stays a localized transient surrounded by a silence floor.
func TestImpulseVoiceLandsAtOffset(t *testing.T) {
	const sr = 48000
	m := newTestMixer()
	m.Schedule("kick", newImpulseVoice(), 240) // 5ms in
	got := renderMixer(m, 480)                  // 10ms
	peakIdx, peak := argmaxAbs(got)
	if peak < 0.01 {
		t.Fatalf("impulse not audible: peak=%g", peak)
	}
	if peakIdx < 235 || peakIdx > 250 {
		t.Fatalf("impulse at sample %d, want ~240", peakIdx)
	}
	// Pre-onset must be silent (no smear backwards in time).
	for i := 0; i < 230; i++ {
		if math.Abs(got[i]) > 1e-6 {
			t.Fatalf("pre-onset energy at %d = %g", i, got[i])
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/ -run TestImpulseVoiceLandsAtOffset -v` → FAIL (undefined `newTestMixer`/`newImpulseVoice`/`renderMixer`/`argmaxAbs`/`newImpulseVoice`).

- [ ] **Step 3: Implement** `offline_render.go` — impulse voice, mixer test-constructor, block-drive renderer, helpers. Mirror the `mixer{}` field set used in `mixer_signal_test.go` / `template_audio_quality_test.go` exactly (copy whatever buffer fields those construct).

```go
//go:build !test

package audio

import "math"

// impulseVoice: one full-scale sample, then done. Deterministic, allocation-free.
type impulseVoice struct{ fired bool }

func newImpulseVoice() Voice { return &impulseVoice{} }
func (v *impulseVoice) Sample() (float64, bool) {
	if v.fired {
		return 0, true
	}
	v.fired = true
	return 1.0, false
}

// newTestMixer builds a mixer with no Oto, real (non-stub) processing. Copy the
// exact field initialization from template_audio_quality_test.go's mixer setup.
func newTestMixer() *mixer {
	return &mixer{
		workBuf:   make([]float64, blockSize),
		voiceTemp: make([]float64, blockSize),
		masterBuf: make([]float64, blockSize),
		postFXBuf: make([]float64, blockSize),
		postEQBuf: make([]float64, blockSize),
		instSlots: make(map[string]int),
	}
}

// renderMixer drives the mixer block-by-block and returns float64 PCM via the
// output-capture seam (post master chain).
func renderMixer(m *mixer, nSamples int) []float64 {
	ClearOutputCapture()
	StartOutputCapture()
	buf := make([]byte, blockSize*2)
	for done := 0; done < nSamples; {
		n := blockSize
		if done+n > nSamples {
			n = nSamples - done
		}
		m.Read(buf[:n*2])
		done += n
	}
	return StopOutputCapture()
}

func argmaxAbs(x []float64) (int, float64) {
	bi, bv := 0, 0.0
	for i, v := range x {
		if a := math.Abs(v); a > bv {
			bv, bi = a, i
		}
	}
	return bi, bv
}
```

> If `Voice`'s real method signature differs (e.g. `Sample() float64` + separate done flag, or a block interface), match it — read `engine_mixer.go` for the `Voice` interface and adapt `impulseVoice`. Do **not** invent a signature.

- [ ] **Step 4: Run to verify it passes** — same command → PASS. If the impulse smears pre-onset (anti-pop fade-in), widen the pre-onset silence window minimally and document why; do not loosen the noise floor below `1e-6` without a one-line rationale.

- [ ] **Step 5: Commit** — `git add src/go/internal/audio/offline_render.go src/go/internal/audio/offline_render_test.go && git commit -m "test(audio): deterministic impulse voice + offline mixer render primitive"`

---

## Task 2: Enumerate the predictor trigger schedule for a circuit over [0, T)

**Files:**
- Modify: `src/go/internal/audio/offline_render.go`
- Test: `src/go/internal/audio/offline_render_test.go`

Goal: given a built `*engine.Predictor` (or the game's circuit), a BPM, a subdivision count `div`, and a duration, return `[]trigger{row, absIdx, sampleOffset}` deterministically. This is the ground-truth schedule the harness validates the audio against.

- [ ] **Step 1: Write the failing test** — a 4-on-the-floor single-row circuit yields exactly `div`-spaced triggers.

```go
func TestEnumerateTriggersFourOnFloor(t *testing.T) {
	pred := buildTestPredictorEveryStep(t, /*rows=*/1, /*div=*/4) // helper: 1 row firing every subdivision
	trigs := enumerateTriggers(pred, 1, 4, /*bpm=*/120, /*sr=*/48000, /*durSec=*/1.0)
	// 120bpm, div=4 → 8 subdiv/sec → 8 triggers in 1s.
	if len(trigs) != 8 {
		t.Fatalf("got %d triggers, want 8", len(trigs))
	}
	step := 48000.0 * 60.0 / 120.0 / 4.0 // samples per subdivision
	for i, tr := range trigs {
		want := int(float64(i) * step)
		if abs(tr.sampleOffset-want) > 1 {
			t.Fatalf("trigger %d at %d, want ~%d", i, tr.sampleOffset, want)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `-run TestEnumerateTriggersFourOnFloor` → FAIL (undefined helpers).

- [ ] **Step 3: Implement** `enumerateTriggers` and the `buildTestPredictorEveryStep` helper. Use `pred.Ensure(targetIdx+1)` then loop `abs` in `[0, targetIdx)` × rows calling `pred.AudibleAt(row, abs)`; `sampleOffset = round(abs/div * 60/bpm * sr)`. For the test predictor, construct the smallest `engine` graph that fires every step (read `core/engine` test helpers / `predictor_*_test.go` for the canonical construction; reuse, don't reinvent).

```go
type trigger struct {
	row, absIdx, sampleOffset int
}

func enumerateTriggers(p *engine.Predictor, rows, div, bpm, sr int, durSec float64) []trigger {
	stepSec := 60.0 / float64(bpm) / float64(div)
	target := int(durSec/stepSec) + 1
	p.Ensure(target)
	var out []trigger
	for abs := 0; abs < target; abs++ {
		for r := 0; r < rows; r++ {
			if p.AudibleAt(r, abs) {
				off := int(float64(abs)*stepSec*float64(sr) + 0.5)
				out = append(out, trigger{r, abs, off})
			}
		}
	}
	return out
}
```

- [ ] **Step 4: Run to verify it passes** — PASS.
- [ ] **Step 5: Commit** — `git commit -am "test(audio): deterministic predictor trigger enumeration over a duration"`

---

## Task 3: `RenderCircuitOffline` — sequenced master from a circuit + impulse instrument

**Files:**
- Modify: `src/go/internal/audio/offline_render.go`
- Test: `src/go/internal/audio/offline_render_test.go`

- [ ] **Step 1: Write the failing test** — render the 4-on-floor circuit through the impulse voice; assert one transient per enumerated trigger, silence between, no NaN/clip.

```go
func TestRenderCircuitOfflineImpulseTrain(t *testing.T) {
	pred := buildTestPredictorEveryStep(t, 1, 4)
	trigs := enumerateTriggers(pred, 1, 4, 120, 48000, 1.0)
	master := renderCircuitOffline(trigs, newImpulseVoice, 48000, 1.0)

	assertNoNaNOrClip(t, master)               // |x|<=1, no NaN/Inf
	onsets := detectOnsets(master, 1e-3)       // energy-rise indices
	if len(onsets) != len(trigs) {
		t.Fatalf("detected %d onsets, want %d (dropped/doubled trigger)", len(onsets), len(trigs))
	}
	for i := range trigs {
		if abs(onsets[i]-trigs[i].sampleOffset) > 8 { // <=8 samples = 0.17ms
			t.Fatalf("onset %d at %d, want %d (mistimed)", i, onsets[i], trigs[i].sampleOffset)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL (undefined `renderCircuitOffline`/`assertNoNaNOrClip`/`detectOnsets`).

- [ ] **Step 3: Implement** `renderCircuitOffline(trigs, voiceFactory, sr, durSec)`: fresh `newTestMixer()`, `m.Schedule(id, voiceFactory(), tr.sampleOffset)` for each trigger, then `renderMixer(m, int(durSec*sr))`. Implement `assertNoNaNOrClip` and `detectOnsets` (rising-edge over a threshold with a refractory gap of, say, 64 samples so one transient = one onset).

- [ ] **Step 4: Run to verify it passes** — PASS.
- [ ] **Step 5: Commit** — `git commit -am "feat(audio): RenderCircuitOffline — deterministic sequenced master render"`

---

## Task 4: Headline harness — complex node-rule circuit, simple audio, analytic + golden

**Files:**
- Create: `src/go/internal/audio/audio_quality_harness_test.go`

This is the deliverable the user described. A *complex* graph (many nodes, mixed logic kinds incl. `every_n_triggers`, `skip_every_n`, fixed-seed `probability`, `trigger_if_prev_*`) routed to the *single* impulse instrument. The predictor is ground truth; the audio must match.

- [ ] **Step 1: Write the failing test.**

```go
func TestComplexCircuitSimpleAudioByteExact(t *testing.T) {
	// Build a node-rule-heavy circuit (see helper) that nonetheless drives one
	// deterministic impulse instrument. Predictor schedule = ground truth.
	pred, rows, div := buildComplexNodeRuleCircuit(t)
	trigs := enumerateTriggers(pred, rows, div, 120, 48000, 4.0)
	if len(trigs) < 20 {
		t.Fatalf("complex circuit produced only %d triggers; want a busy schedule", len(trigs))
	}
	master := renderCircuitOffline(trigs, newImpulseVoice, 48000, 4.0)

	// (a) Structural: audio matches predictor exactly — no dropped/doubled/mistimed.
	assertNoNaNOrClip(t, master)
	onsets := detectOnsets(master, 1e-3)
	if len(onsets) != len(trigs) {
		t.Fatalf("onsets=%d triggers=%d", len(onsets), len(trigs))
	}
	for i := range trigs {
		if abs(onsets[i]-trigs[i].sampleOffset) > 8 {
			t.Fatalf("onset %d mistimed: %d vs %d", i, onsets[i], trigs[i].sampleOffset)
		}
	}
	// (b) Silence floor between bursts: no spurious noise (denormal/leak).
	assertInterOnsetSilence(t, master, onsets, /*floor=*/1e-4)

	// (c) Byte-exact golden: any signal regression flips the hash.
	got := hashFloat32From64(master)
	const golden = "" // fill from first green run; pin it.
	if golden != "" && got != golden {
		t.Fatalf("master hash drift: got %s want %s", got, golden)
	}
	t.Logf("master sha256=%s samples=%d onsets=%d", got, len(master), len(onsets))
}
```

- [ ] **Step 2: Run to verify it fails** — FAIL (undefined `buildComplexNodeRuleCircuit`, `assertInterOnsetSilence`, `hashFloat32From64`).

- [ ] **Step 3: Implement helpers.** `buildComplexNodeRuleCircuit` constructs the graph via the canonical `core/engine`/`core/model` test constructors (reuse predictor test helpers; set `LogicKind`/`LogicN`/`LogicP` via `SetNodeParams`). `probability` must use the predictor's deterministic hash roll (no RNG) so it's reproducible. `hashFloat32From64` reuses the `hashFloat32` pattern from `fm_golden_test.go:67` (convert float64→float32 first). `assertInterOnsetSilence` checks the midpoint between consecutive onsets is below `floor`.

- [ ] **Step 4: Run, read the logged hash, pin `golden`, re-run** → PASS. Command: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ./internal/audio/ -run TestComplexCircuitSimpleAudioByteExact -v`.

- [ ] **Step 5: Commit** — `git commit -am "test(audio): complex-circuit/simple-audio deterministic master validation (analytic + golden)"`

---

## Task 5: Node-logic CPU-cost measurement (the bench coverage gap)

**Files:**
- Create: `src/go/internal/audio/node_logic_cost_test.go`

Deterministically attribute scheduling-path CPU cost to node-rule load — the Go analog of the OfflineAudioContext FX bench, covering the path FX-cost can't (predictor/scheduler). This both **covers** the user's "synth/sampler/node-logic" ask and **localizes** the choppiness cause before Phase 4.

- [ ] **Step 1: Write the measurement test** (a `testing.B`-style timed assertion run under `-run`, or a real `Benchmark`). Escalate node-rule complexity and time the *steady-state* `Ensure()` window-extension AND the *post-edit* dirty rebuild.

```go
// Measures Predictor.Ensure cost vs node-rule load. Not a pass/fail gate by
// default; it prints a table and (optionally) gates a regression ceiling.
func TestNodeLogicEnsureCostTable(t *testing.T) {
	cases := []struct{ name string; build func(*testing.T) (*engine.Predictor, int, int) }{
		{"static-no-rules", buildStaticCircuit},
		{"every_n", buildEveryNCircuit},
		{"prev_triggered", buildPrevTriggeredCircuit}, // O(<=64) backward walk
		{"mixed-heavy", func(t *testing.T) (*engine.Predictor, int, int) {
			p, r, d := buildComplexNodeRuleCircuit(t); return p, r, d
		}},
	}
	for _, c := range cases {
		p, _, div := c.build(t)
		horizon := 60 * div // ~ seconds of schedule
		// Steady-state extension cost (no dirty):
		ext := timeEnsureSteady(p, horizon)
		// Post-edit dirty rebuild cost (the predDirty path):
		reb := timeEnsureAfterDirtyEdit(p, horizon)
		t.Logf("%-18s ensure-steady=%8.3fms  rebuild-after-edit=%8.3fms", c.name, ext, reb)
	}
}
```

- [ ] **Step 2: Implement** `timeEnsureSteady` (advance horizon in increments, sum wall-time) and `timeEnsureAfterDirtyEdit` (mutate one node param via `SetNodeParams` to set `predDirty`, then time the next `Ensure`). Use `runtime.GC()` before timing; median of N.

- [ ] **Step 3: Run and capture the table** — `cd src/go && ../../.tools/go/bin/go test -tags '' ./internal/audio/ -run TestNodeLogicEnsureCostTable -v 2>&1 | tee bench-results/node_logic_cost_$(date +%s).log`. (Predictor lives in `core/engine` and builds under the default tag; adjust the package path if the helper must live in `core/engine` instead — put the test where the constructors are cheapest to reuse.)

- [ ] **Step 4: Interpret** — record in the log doc: does `rebuild-after-edit` dwarf `ensure-steady`? Does `prev_triggered`/`mixed-heavy` blow up steady cost? This **chooses the Phase-4 fix target** with data.

- [ ] **Step 5: Commit** — `git commit -am "test(audio): deterministic node-logic Ensure/rebuild CPU-cost table"`

---

## Task 6: Extend the JS FX bench to synth + sampler voice cost

**Files:**
- Modify: `scripts/bench_audio_node_cost.mjs`

The user asked the benchmark to "also contain synth, sampler and node logic changes." Node-logic is covered deterministically in Task 5 (Go). Here, add synth/sampler **voice** render-cost rows to the existing OfflineAudioContext bench so the JS side isn't FX-only.

- [ ] **Step 1:** Add two builders to the `builders` map mirroring how `audio.js` builds a played synth/sampler source: a `BufferSource` feeding the per-voice anti-pop `fadeGain` (target `CHAIN_SPEC.voiceHeadroom`) → bus. For synth use a representative pre-rendered buffer (white-noise stand-in is fine for *cost*, since cost is dominated by node graph, not buffer contents); for sampler, the same with a longer buffer + `playbackRate`. Add `"synth"`, `"sampler"` to the `effects` array.
- [ ] **Step 2:** Run `node scripts/bench_audio_node_cost.mjs` → confirm the new rows render and rank. Save output to `bench-results/`.
- [ ] **Step 3: Commit** — `git commit -am "bench(audio): add synth + sampler voice cost to node-cost bench"`

---

## Task 7: WASM parity of the harness

**Files:**
- Create: `src/js/audio_quality_harness.browser.test.js`

Prove the *signal-correctness* harness holds across the WASM↔WebAudio boundary (the platform the user says is worst), reusing the Phase-1 circuit.

- [ ] **Step 1:** Import the same complex node-rule circuit JSON via `importJSON`, enumerate the WASM predictor schedule (`audibleAt(row, abs)`), and capture master output. Prefer OfflineAudioContext-style deterministic capture; fall back to the recording pipeline only if needed. Compare onset count/positions to the Go golden's trigger schedule with DSP tolerance (correlation ≥ 0.999 on the onset map; per-onset position ≤ a few ms — WebAudio adds lookahead/quantum latency, so positions are offset-but-consistent, not sample-identical).
- [ ] **Step 2:** Run `GO=$(pwd)/.tools/go/bin/go node src/js/audio_quality_harness.browser.test.js` → PASS. Add to `scripts/browser-test-config.sh` if it should run in CI.
- [ ] **Step 3: Commit** — `git commit -am "test(js): WASM parity for the deterministic audio-quality harness"`

---

## Task 8: Evaluate the reverb change on data; recommend keep/revert

**Files:** none (analysis) — then act on the decision.

- [ ] **Step 1:** Read the completed `bench_audio_node_cost.mjs` output (reverb_old vs reverb per-node ms). Confirm/quantify the CPU delta.
- [ ] **Step 2:** Confirm node-rule choppiness is **unaffected** by reverb (Task 5 table has no reverb term — node-logic cost is reverb-independent, which is the point).
- [ ] **Step 3:** Recommendation: keep the Schroeder reverb **iff** (a) it's a real CPU win AND (b) `xplat_insert_fx_parity`/`worklet_insert_effects` still green AND (c) the slightly-different tail is acceptable (it is — matches desktop). Otherwise revert. Present the data + recommendation to the user; do not silently decide a sound-affecting change.

---

## Task 9: Fix the node-rule choppiness root cause (measurement-driven)

**Files:** TBD by Task 5 (most likely `src/go/core/engine/predictor_compute.go`, `predictor_api.go`).

> This task is intentionally specified *after* measurement. The candidate, per the evidence, is: a single node-param edit forces a **full-window history re-walk** (`predictor_compute.go:154-257`) via the `predDirty` flag. If Task 5 confirms `rebuild-after-edit` is the dominant cost, the fix is to **scope the rebuild to the affected row(s)/forward window** instead of re-walking `[0, windowStart)` for every row on any param change.

- [ ] **Step 1: Failing test** — assert `timeEnsureAfterDirtyEdit` for a single-node param change is within a small multiple of `ensure-steady` (i.e. edit cost is bounded, not `O(full history × rows)`). Write it to fail against current behavior.
- [ ] **Step 2:** Implement the minimal scoping fix the measurement justifies (e.g. per-row dirty set; forward-only rebuild from the rebase point; preserve historical per-row counters incrementally rather than recomputing from 0). One change at a time.
- [ ] **Step 3: Verify** — (a) the cost test passes; (b) **`TestComplexCircuitSimpleAudioByteExact` golden is UNCHANGED** (fix must not alter the signal — same triggers, same audio); (c) predictor parity suites still green (`go test ./core/engine/...`, `xplat_parity.browser.test.js`).
- [ ] **Step 4: Commit** — `git commit -m "perf(predictor): bound node-param edit rebuild to affected window (node-rule choppiness)"`

---

## Self-Review notes

- **Spec coverage:** harness (T1–T4, T7) ✓; benchmark covers synth/sampler/node-logic (T5 Go node-logic, T6 JS synth/sampler) ✓; byte-for-byte buffer validation (T4 golden + structural) ✓; "complex circuit, simple audio" (T4) ✓; reverb evaluation (T8) ✓; node-rule fix (T9) ✓; "validate audio not noisy beyond known signal" (T4 `assertInterOnsetSilence` + `assertNoNaNOrClip`) ✓.
- **Build tags:** harness is `!test` (needs real mixer/EQ, not the `gainProcessor` stub) and most runs need `xvfb-run`. The predictor cost test (T5) builds under default tag (predictor has no Ebiten/Oto dep) — place it where constructors are cheapest.
- **Risk:** the exact `mixer{}` field set and `Voice` interface signature must be copied from current source, not the illustrative code above — verify against `engine_mixer.go`/`template_audio_quality_test.go` at implementation time.
- **Determinism risk:** `probability` logic must use the predictor's deterministic roll, never `math/rand`, or the golden won't pin.
