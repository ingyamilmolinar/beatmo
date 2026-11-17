# Config-only Synth Engine — Roadmap + Phase A (double-precision params) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> **Repo rule:** never `git add` / `git commit` / `git stash` (the user commits). Every "commit" step in this plan is a VERIFY CHECKPOINT instead.

**Goal:** Every sound-shaping number in the synth engine becomes instrument CONFIG (the C voices become pure functions of their input params), legacy/obsolete code is deleted, and every step is proven byte-identical — starting with switching the param block from `float` to `double` so moving a C literal into config is bit-exact.

**Architecture:** One positional param block (`modular_params`, C ↔ Go schema ↔ JS ABI) carries every knob. Phase A widens its element type to `double` (one inaudible, one-time render change for seed values that are not float32-exact, then strict byte parity). Later phases delete the legacy layer, reshape the ABI, and lift the ~1,030 sound-shaping C literals into params one voice family at a time, each verified bit-identical against a pre-change render dump.

**Tech Stack:** C (`src/c`, native via cgo `build/libdrums.a`, browser via emscripten `src/js/drums.single.js`), Go (`src/go/internal/audio`), JS (`src/js/audio.js`, `src/js/synth_render_worker.js`), generated `src/js/synth_param_abi.gen.js`.

**Spec:** this conversation's audit (2026-10-02, recorded in the "Roadmap" section below) + user decisions: (1) drop old-project import support entirely; (2) switch params to double to preserve byte parity; (3) lift every number into config first, keep variant selectors (merge into shared blocks later); (4) lookup tables (string bodies ~180 values, voice formants ~300) become editable config.

## Global Constraints

- Bundled Go only: `.tools/go/bin/go` (tests: `cd src/go && ../../.tools/go/bin/go test ...`; fast path adds `-tags test -modfile=go.test.mod`).
- After ANY C edit: `make build/libdrums.a` (cgo links a PREBUILT archive — without it native tests silently run stale C). Browser: `make wasm` (rebuilds `src/js/drums.single.js` + `src/js/main.wasm`).
- The modular schema is a flat positional ABI: C `modular.h` field order == Go `synth_param_schema.go` order == generated JS ABI. Regenerate the JS ABI with `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-synth-abi > ../js/synth_param_abi.gen.js` (writes STDOUT — must redirect).
- A shipped sound may change ONLY in the steps that say so, and only within the stated bound; everywhere else renders must be BYTE-IDENTICAL (verified by the render-parity tool built in Task A1).
- Never commit audio files; render dumps live in the session scratchpad or `$BEATMO_RENDER_PARITY_DIR`.

## Review Focus

1. **WASM struct layout under double:** JS must write the block as `Float64Array` into `HEAPF64` at an 8-byte-aligned pointer, in BOTH the worker path (`synth_render_worker.js`) and the sync fallback (`audio.js renderVoiceSync`). A path left on `Float32Array`/`HEAPF32` reads garbage → silent or noisy voices. Pinned by Task A3 (worker vs sync PCM identity + the ABI browser test).
2. **The legacy 7-field `_p` arm** (`synth_params`, still float) must keep `Float32Array` until Phase B deletes it — the element type must follow the block, not be global. Pinned in Task A3.
3. **NaN sentinels** (`kp_get(v, literal)`, legacy bindings send NaN) must still select the literal after `float`→`double`. Pinned by the existing legacy oracle golden + a dedicated NaN case in Task A2.
4. **Silent large changes hiding behind "expected" re-pins:** the one-time change must be ≤ the float32 rounding of the seed values (≥ 120 dB SNR per instrument). Pinned by Task A1's compare gate before any golden is re-pinned (Task A4).
5. **Export→import identity** (`make test-real`, `!test` build) must still pass — it renders through the same block.

---

## Roadmap (all phases)

| Phase | Deliverable | Sound changes? | Plan |
|---|---|---|---|
| **A** | Param block `float` → `double`; render-parity tool (dump + compare) | One-time ≤ float32 rounding of seed values (≥ 120 dB SNR), then none | **this document** |
| **B** | Delete legacy: `legacyCompatRecipes` (24 recipes), `{kick,snare,tom,cymbal,fm,bass}_modular_{binding,push,native}.go` (~2.4k LOC), family scaffolding, 7-field `synth_params` + empty `ParamRenderFuncs`/`builtinRecipeRenderers`, `legacy_instrument_ids.go`, `migrate_gen_type.go`, `LegacyVersion` + absent-`schema_version` migration (keep a foreign-JSON reject), dead C voices (non-modal hihat/ride, aliases, tombstones), test-only C (`pan.c`, `wt_osc_process*`, `adsr_*`, `dsp_units_c.go`), unused WASM exports, JS dead arms (duplicate `RENDER` tables, family blocks, 7-field arm, `MODULAR_ALLOC_FLOOR`), legacy-pinning tests (~3.4k LOC). `sub-bass`/`sub-bass-drive` become `modularInstrumentDefs` rows whose seed spells out the former NaN fallbacks | None (bit-exact under double) | written when A lands |
| **C** | ABI reshape: family blocks slot-scoped (drop the ~857 floats no seed uses); Go struct + marshal GENERATED from the schema (7 hand edit points → 1); `kp_get`/`cym_modal_param`/`mp_get` strict — no NaN/0 "use the C default" path, every field required (kills silent zeros) | None by design; ledgered exceptions (see Results) | [`2026-10-03-config-only-synth-engine-phase-c.md`](2026-10-03-config-only-synth-engine-phase-c.md) — landed 2026-10-03 |
| **D** | Lift sound-shaping literals → params, one family per task, each bit-exact: POST + reverb + KS + clap + analytic (~30) → tom + snare (~113) → cymbal (~90) → FM (~41 + topology/op tables) → kick v0–v8 (~155, incl. variant-8 mode tables) → waveguides/bow/formant scalars; variants remain algorithm selectors | None (bit-exact) | written when C lands |
| **E** | Tables as editable config: string-body tables (violin/guitar/cello/steel, ~183 values) and voice formant tables (~304 values) become table-valued params with an Advanced editor | None (bit-exact) | written when D lands |
| **F** | Guards: a test that parses every voice function in `modular_stages.c`/`modular.c` and fails on any numeric literal outside a structural allowlist (2π, cents, Nyquist clamp, epsilons) with a per-function ratchet budget shrinking to 0; `known_dead_seed_params.txt` shrunk to empty; docs (AGENTS.synth*) | None | written when E lands |

Insert/send FX literals (`insert_fx.c` ~219, `effects.c` ~7) are part of an instrument's sound via `DefaultFX`; they are folded into Phase D as a final family.

---

## Phase A — File map

- **Create** `src/go/internal/audio/render_parity_tool_test.go` — env-gated (`!test && !js`): `TestRenderParityDump` writes every `BuiltinInstrumentIDs` one-shot render (float32 PCM) to `$BEATMO_RENDER_PARITY_DIR/<label>/<id>.f32`; `TestRenderParityCompare` compares two labels per instrument: bit-identical? else SNR dB; fails below a threshold. Reused by every later phase.
- **Modify** `src/c/modular.h` — every `float` field of `modular_params` → `double`.
- **Modify** `src/c/synth_params.h:84` — `kp_get(float v, …)` → `kp_get(double v, …)`.
- **Modify** `src/c/modular_stages.c:2547` — `cym_modal_param(float v, …)` → `double v`.
- **Modify** `src/c/modular.c:72` — `mp_get` parameter/return types follow the block (`double`), assigning into the float `synth_params` POST block exactly as before.
- **Modify** `src/go/internal/audio/modular_c.go` — `toCModularParams`: `C.float(` → `C.double(` for every modular field.
- **Modify** `src/go/internal/audio/modular_abi_drift_test.go` — `cStructFields` accepts `double` declarations.
- **Modify** `src/js/audio.js` — modular param block built as `Float64Array`; `renderVoiceSync` sizes/writes by the array's `BYTES_PER_ELEMENT` (`HEAPF64` for 8, `HEAPF32` for 4).
- **Modify** `src/js/synth_render_worker.js` — same element-size-aware write.
- **Regenerate** `src/js/synth_param_abi.gen.js` (unchanged names; confirm), `src/js/drums.single.js` + `src/js/main.wasm` (`make wasm`).
- **Re-pin** `src/go/internal/audio/synth_golden_pinned_test.go` (+ any other byte-exact golden the suite reports), regenerate loudness if its drift test trips.

---

### Task A1: Render-parity tool (dump + compare)

**Files:**
- Create: `src/go/internal/audio/render_parity_tool_test.go`

**Interfaces:**
- Produces: `TestRenderParityDump` (env `BEATMO_RENDER_PARITY_DIR`, `BEATMO_RENDER_PARITY_LABEL`), `TestRenderParityCompare` (env `BEATMO_RENDER_PARITY_DIR`, `BEATMO_RENDER_PARITY_A`, `BEATMO_RENDER_PARITY_B`, `BEATMO_RENDER_PARITY_MIN_SNR_DB` default `+Inf` meaning bit-exact required). Both `t.Skip` when the dir is unset. Uses existing `RenderInstrumentOneShotRaw(id) ([]float32, int)`, `Reset()`, `ResetInstruments()`, `BuiltinInstrumentIDs`.

- [ ] **Step 1: Write the tool**

```go
//go:build !test && !js

package audio

import (
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
)

// render_parity_tool_test.go — the byte-parity gate for the config-only engine
// refactor (docs/superpowers/plans/2026-10-02-config-only-synth-engine.md).
// Dump every built-in instrument's one-shot render before a change, dump again
// after, compare: every instrument must be BIT-IDENTICAL unless the step allows
// a bounded change (BEATMO_RENDER_PARITY_MIN_SNR_DB). Env-gated: skips unless
// BEATMO_RENDER_PARITY_DIR is set. Dumps never live in the repo.

func renderParityDir(t *testing.T) string {
	d := os.Getenv("BEATMO_RENDER_PARITY_DIR")
	if d == "" {
		t.Skip("BEATMO_RENDER_PARITY_DIR unset")
	}
	return d
}

func TestRenderParityDump(t *testing.T) {
	dir := renderParityDir(t)
	label := os.Getenv("BEATMO_RENDER_PARITY_LABEL")
	if label == "" {
		t.Fatal("BEATMO_RENDER_PARITY_LABEL unset")
	}
	out := filepath.Join(dir, label)
	if err := os.MkdirAll(out, 0o755); err != nil {
		t.Fatal(err)
	}
	Reset()
	ResetInstruments()
	ids := append([]string(nil), BuiltinInstrumentIDs...)
	sort.Strings(ids)
	for _, id := range ids {
		buf, sr := RenderInstrumentOneShotRaw(id)
		b := make([]byte, 4+4*len(buf))
		binary.LittleEndian.PutUint32(b, uint32(sr))
		for i, v := range buf {
			binary.LittleEndian.PutUint32(b[4+4*i:], math.Float32bits(v))
		}
		if err := os.WriteFile(filepath.Join(out, id+".f32"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("dumped %d instruments to %s", len(ids), out)
}

func readParity(t *testing.T, path string) (int, []float32) {
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	sr := int(binary.LittleEndian.Uint32(b))
	n := (len(b) - 4) / 4
	out := make([]float32, n)
	for i := range out {
		out[i] = math.Float32frombits(binary.LittleEndian.Uint32(b[4+4*i:]))
	}
	return sr, out
}

func TestRenderParityCompare(t *testing.T) {
	dir := renderParityDir(t)
	a, b := os.Getenv("BEATMO_RENDER_PARITY_A"), os.Getenv("BEATMO_RENDER_PARITY_B")
	if a == "" || b == "" {
		t.Fatal("BEATMO_RENDER_PARITY_A / _B unset")
	}
	minSNR := math.Inf(1)
	if v := os.Getenv("BEATMO_RENDER_PARITY_MIN_SNR_DB"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil {
			t.Fatal(err)
		}
		minSNR = f
	}
	files, _ := filepath.Glob(filepath.Join(dir, a, "*.f32"))
	if len(files) == 0 {
		t.Fatalf("no dumps under %s", filepath.Join(dir, a))
	}
	identical, changed := 0, 0
	for _, fa := range files {
		id := filepath.Base(fa[:len(fa)-4])
		srA, xa := readParity(t, fa)
		srB, xb := readParity(t, filepath.Join(dir, b, id+".f32"))
		if srA != srB || len(xa) != len(xb) {
			t.Errorf("%s: shape changed sr %d→%d len %d→%d", id, srA, srB, len(xa), len(xb))
			continue
		}
		var sig, noise float64
		same := true
		for i := range xa {
			if math.Float32bits(xa[i]) != math.Float32bits(xb[i]) {
				same = false
			}
			d := float64(xa[i]) - float64(xb[i])
			sig += float64(xa[i]) * float64(xa[i])
			noise += d * d
		}
		if same {
			identical++
			continue
		}
		changed++
		snr := 10 * math.Log10(sig/(noise+1e-300))
		if snr < minSNR {
			t.Errorf("%s: SNR %.1f dB < %.1f dB (not bit-identical)", id, snr, minSNR)
		} else {
			t.Logf("%s: changed, SNR %.1f dB", id, snr)
		}
	}
	t.Logf("%d bit-identical, %d changed", identical, changed)
}
```

- [ ] **Step 2: Prove it discriminates (self-check)**

```bash
cd src/go && D=$(mktemp -d)
BEATMO_RENDER_PARITY_DIR=$D BEATMO_RENDER_PARITY_LABEL=a ../../.tools/go/bin/go test -count=1 -run TestRenderParityDump ./internal/audio/
BEATMO_RENDER_PARITY_DIR=$D BEATMO_RENDER_PARITY_LABEL=b ../../.tools/go/bin/go test -count=1 -run TestRenderParityDump ./internal/audio/
BEATMO_RENDER_PARITY_DIR=$D BEATMO_RENDER_PARITY_A=a BEATMO_RENDER_PARITY_B=b ../../.tools/go/bin/go test -count=1 -v -run TestRenderParityCompare ./internal/audio/ | tail -3
```
Expected: PASS, "150 bit-identical, 0 changed" (renders are deterministic). Then flip one sample in `$D/b/kick.f32` (e.g. `printf '\x01' | dd of=$D/b/kick.f32 bs=1 seek=4000 count=1 conv=notrunc`) and rerun Compare: Expected FAIL naming `kick` with an SNR value.

- [ ] **Step 3: Capture the Phase-A baseline**

```bash
export BEATMO_RENDER_PARITY_DIR=$HOME/.beatmo/render-parity
cd src/go && BEATMO_RENDER_PARITY_LABEL=pre-double ../../.tools/go/bin/go test -count=1 -run TestRenderParityDump ./internal/audio/
```
Expected: "dumped 150 instruments".

- [ ] **Step 4: VERIFY CHECKPOINT** — `../../.tools/go/bin/go vet ./internal/audio/` clean; the tool skips in a normal `go test ./internal/audio/` run.

---

### Task A2: C + Go — `modular_params` fields become `double`

**Files:**
- Modify: `src/c/modular.h` (struct `modular_params`, lines ~29-608)
- Modify: `src/c/synth_params.h:84` (`kp_get`), `src/c/modular_stages.c:2547` (`cym_modal_param`), `src/c/modular.c:72` (`mp_get`)
- Modify: `src/go/internal/audio/modular_c.go` (`toCModularParams`)
- Modify: `src/go/internal/audio/modular_abi_drift_test.go` (`cStructFields`)
- Test: `src/go/internal/audio/modular_abi_drift_test.go`, new case in `src/go/internal/audio/modular_render_test.go`

**Interfaces:**
- Consumes: Task A1 parity tool.
- Produces: `modular_params` with every field `double`; `kp_get(double v, double fallback)`; JS must now write 8-byte elements (Task A3).

- [ ] **Step 1: Make the ABI drift test accept `double` and pin the element type**

In `cStructFields`, the field regex currently matches `float` declarations only; extend it to `(?:float|double)`. Add:

```go
// TestModularParamsAreDouble pins Phase A of the config-only refactor: the
// modular block is double so a literal lifted out of C into config is the SAME
// 64-bit value (bit-exact), not its float32 rounding.
func TestModularParamsAreDouble(t *testing.T) {
	root := repoRootForABI(t)
	raw, err := os.ReadFile(filepath.Join(root, "src/c/modular.h"))
	if err != nil {
		t.Fatal(err)
	}
	body := structBody(t, string(raw), "modular_params")
	if regexp.MustCompile(`(?m)^\s*float\s`).MatchString(body) {
		t.Fatal("modular_params still declares float fields — every field must be double")
	}
}
```
(`repoRootForABI` / `structBody`: reuse the root lookup and the struct-body extraction `cStructFields` already does — factor that extraction into `structBody(t, text, name) string` in the same file.)

- [ ] **Step 2: Run it to see it fail**

Run: `cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestModularParamsAreDouble|TestModularABI' ./internal/audio/`
Expected: FAIL "modular_params still declares float fields".

- [ ] **Step 3: Switch the C struct and accessors**

```bash
cd /home/ymolinar/Repos/beatmo
python3 - <<'EOF'
import re
p='src/c/modular.h'; s=open(p).read()
a=s.index('typedef struct {', s.index('modular_params is a SECOND'))
b=s.index('} modular_params;', a)
body=re.sub(r'(?m)^(\s*)float(\s)', r'\1double\2', s[a:b])
open(p,'w').write(s[:a]+body+s[b:])
EOF
sed -i 's/static inline double kp_get(float v, double fallback) {/static inline double kp_get(double v, double fallback) {/' src/c/synth_params.h
sed -i 's/static double cym_modal_param(float v, double def) {/static double cym_modal_param(double v, double def) {/' src/c/modular_stages.c
grep -n "static float mp_get" -A8 src/c/modular.c
```
Change `mp_get` to take and return `double` (its callers assign into the `synth_params` POST block's `float` fields — the implicit narrowing there is the SAME float32 value as before, so POST stays bit-identical in this phase).

- [ ] **Step 4: Switch the Go marshal**

In `toCModularParams` (`modular_c.go`), every `C.float(p.X…)` for a modular field becomes `C.double(p.X…)`:
```bash
cd src/go && python3 - <<'EOF'
p='internal/audio/modular_c.go'; s=open(p).read()
a=s.index('func toCModularParams'); b=s.index('\n}\n', a)
s=s[:a]+s[a:b].replace('C.float(', 'C.double(')+s[b:]
open(p,'w').write(s)
EOF
grep -c "C.double(" internal/audio/modular_c.go
```

- [ ] **Step 5: Add the NaN-sentinel case**

In `modular_render_test.go`, add a test that renders a legacy-bound kick (NaN knobs → `kp_get` literal) before/after is covered by the existing legacy oracle golden; add an explicit guard:
```go
// TestKpGetNaNStillSelectsTheLiteral: after float→double, a NaN field must still
// resolve to the C literal (legacy bindings rely on it until Phase B removes them).
func TestKpGetNaNStillSelectsTheLiteral(t *testing.T) {
	seed := kickSeedWith(RecipeParams{"voice_freq_hz": 55, "gen1_kick_variant": 0})
	mp := recipeParamsToModular(MergeRecipeDefaults("synth-modular", seed))
	mp.GenKickH2[0] = math.NaN() // → C literal 0.40
	a := make([]float32, 4410)
	renderModularP(a, 44100, len(a), mp)
	mp.GenKickH2[0] = 0.40
	b := make([]float32, 4410)
	renderModularP(b, 44100, len(b), mp)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("sample %d: NaN render %v != literal render %v", i, a[i], b[i])
		}
	}
}
```

- [ ] **Step 6: Rebuild and run**

```bash
cd /home/ymolinar/Repos/beatmo && make build/libdrums.a
cd src/go && ../../.tools/go/bin/go test -count=1 -run 'TestModularParamsAreDouble|TestModularABI|TestKpGetNaNStillSelectsTheLiteral|TestModularGenBankSchemaShape|TestRenderModularP_EveryParamMutatesInContext' ./internal/audio/
```
Expected: PASS.

- [ ] **Step 7: Parity gate (the one allowed change)**

```bash
cd src/go && BEATMO_RENDER_PARITY_LABEL=post-double ../../.tools/go/bin/go test -count=1 -run TestRenderParityDump ./internal/audio/
BEATMO_RENDER_PARITY_A=pre-double BEATMO_RENDER_PARITY_B=post-double BEATMO_RENDER_PARITY_MIN_SNR_DB=120 \
  ../../.tools/go/bin/go test -count=1 -v -run TestRenderParityCompare ./internal/audio/ | tail -40
```
Expected: PASS; every changed instrument ≥ 120 dB (float32 rounding of a seed value is ~−145 dB relative). Any instrument below 120 dB is a bug (wrong field type / misread) — stop and investigate.

- [ ] **Step 8: VERIFY CHECKPOINT** — record the changed-instrument list + min SNR in the Phase A notes (memory + this plan's "Results" section).

---

### Task A3: JS — the modular block is written as `Float64Array`

**Files:**
- Modify: `src/js/audio.js` (~4164-4186 param build; `renderVoiceSync` ~1592-1612)
- Modify: `src/js/synth_render_worker.js` (~61-76)
- Test: `src/js/synth_abi_consistency.browser.test.js`, `src/js/xplat_audio_compare.browser.test.js`

**Interfaces:**
- Consumes: Task A2's `double` struct.
- Produces: element-size-aware param writes (modular = 8 bytes, legacy 7-field `_p` = 4 bytes until Phase B).

- [ ] **Step 1: Build the modular block as Float64Array**

In `audio.js` where `paramArr` is allocated:
```js
    // The modular block is DOUBLE (modular.h, config-only refactor Phase A):
    // a literal lifted from C into config must arrive as the same 64-bit value.
    // The legacy 7-field synth_params block is still float until Phase B.
    paramArr = useModular ? new Float64Array(allocCount) : new Float32Array(allocCount); // zero-initialised
```

- [ ] **Step 2: Element-size-aware heap write (sync path)**

Replace the param malloc/write in `renderVoiceSync`:
```js
    if (paramArr) {
      const count = Math.max(paramArr.length, allocCount || 0);
      const bpe = paramArr.BYTES_PER_ELEMENT; // 8 = modular (double), 4 = legacy synth_params
      paramsPtr = m._malloc(count * bpe);
      if (!paramsPtr) throw new Error('Failed to allocate memory for param block.');
      const pheap = bpe === 8
        ? m.HEAPF64.subarray(paramsPtr >> 3, (paramsPtr >> 3) + count)
        : m.HEAPF32.subarray(paramsPtr >> 2, (paramsPtr >> 2) + count);
      pheap.fill(0);
      pheap.set(paramArr);
```
(emscripten `malloc` returns 8-byte-aligned pointers, so `>> 3` is exact.)

- [ ] **Step 3: Same in the worker**

In `synth_render_worker.js`:
```js
    if (msg.params && msg.params.length > 0) {
      const allocCount = Math.max(msg.params.length, msg.allocCount | 0);
      const bpe = msg.params.BYTES_PER_ELEMENT; // structured clone keeps the typed-array kind
      paramsPtr = m._malloc(allocCount * bpe);
      if (!paramsPtr) throw new Error("malloc(params) failed");
      const pheap = bpe === 8
        ? m.HEAPF64.subarray(paramsPtr >> 3, (paramsPtr >> 3) + allocCount)
        : m.HEAPF32.subarray(paramsPtr >> 2, (paramsPtr >> 2) + allocCount);
```
Also update the JSDoc on line ~21 (`params:Float64Array|Float32Array|null`). If the emscripten build does not export `HEAPF64`, add it to the `EXPORTED_RUNTIME_METHODS` list in the Makefile's emcc flags (check `grep -n HEAPF32 Makefile`).

- [ ] **Step 4: Worker vs sync identity test**

Add to `synth_abi_consistency.browser.test.js` a case that renders one modular instrument (e.g. `kick-boom-bap`) through the worker and through `renderVoiceSync` (force via the existing test hook that disables the worker, or call `__testCaptureSynthRender(id, { sampleRate: 48000 })` with and without `__beatmoForceSyncRender = true`) and asserts the two PCM arrays are identical sample-for-sample.

- [ ] **Step 5: Rebuild + browser tests**

```bash
cd /home/ymolinar/Repos/beatmo && make wasm
GO=$(pwd)/.tools/go/bin/go node src/js/synth_abi_consistency.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/synth_save_export_import.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
```
Expected: all PASS (xplat compares WASM against a FRESH native render, so both sides moved together).

- [ ] **Step 6: VERIFY CHECKPOINT** — browser and native agree; note results.

---

### Task A4: Re-pin goldens, full suites

**Files:**
- Modify: `src/go/internal/audio/synth_golden_pinned_test.go` (+ any other byte-exact golden the suites report: modular identity golden, FM preset golden, `testdata/legacy_oracle_golden*` fixture, `testdata/audio_tab_tap_golden.json`)
- Maybe regenerate: `internal/audio/instrument_loudness_gen.go` + `src/js/instrument_loudness.gen.js` (only if `loudness_drift_test` trips), `instrument_fundamentals.gen.go` (only if its drift test trips)

- [ ] **Step 1: Re-pin the synth goldens**

```bash
cd src/go && ../../.tools/go/bin/go test -count=1 -run TestSynthGoldenIdentityCapture -v ./internal/audio/ | grep '^GOLDEN' > /tmp/goldens.txt
```
Replace the `synthGoldens` map body with the captured lines and add a dated note: "Re-pinned 2026-10-XX (config-only Phase A: modular_params float→double). Every changed id verified ≥ 120 dB SNR vs the pre-switch render (TestRenderParityCompare); the change is the float32 rounding of seed values that are not float32-exact."
Expected changed ids == the Task A2 Step 7 changed list (cross-check; any extra id is a bug).

- [ ] **Step 2: Full suites**

```bash
cd src/go && ../../.tools/go/bin/go test -count=1 -timeout 2700s ./internal/audio/ ./cmd/...
../../.tools/go/bin/go test -tags test -modfile=go.test.mod -count=1 -timeout 2700s ./...
cd /home/ymolinar/Repos/beatmo && make test-real
```
Expected: PASS except failures proven unrelated (re-run in isolation; compare against the pre-change tree's result for the same test). Re-pin only goldens whose change is explained by Task A2's parity list.

- [ ] **Step 3: VERIFY CHECKPOINT** — write the Phase A results (changed ids, min SNR, struct size 5,520 → 11,040 B) into this plan and the memory note; then write the Phase B plan.

---

## Results

**Phase A (landed 2026-10-02):** `modular_params` is 205 `double` fields (1,380 values, 5,520 → 11,040 B). Proof the switch is exact: with params rounded back to float32 (`BEATMO_MODULAR_PARAMS_F32=1` / `renderModularRoundParamsToFloat32`) all 150 instruments render bit-identical to the pre-switch dump. With exact delivery: 39 identical, 111 changed — long ringing tones drift ≤2.3e-5 (−93 dBFS; a ~3e-8 relative pitch change accumulating as phase), kick-boom-bap's 30 ms hold lands one sample later (max 1.6e-3, −56 dBFS). The plan's ≥120 dB SNR gate was the wrong instrument (waveform SNR punishes inaudible phase drift); replaced by the bit-exact F32 proof. JS writes the modular block as Float64Array/HEAPF64 in both the worker and the sync path (browser test `synth_param_block_double` RED: all voices silent → GREEN: worker == sync bit-identical). Re-pinned: synth goldens (exactly the 111), legacy oracle fixture, refs thresholds golden, fundamentals (conga −0.0001 Hz). New tools: `render_parity_tool_test.go` (dump/compare), `BEATMO_MODULAR_PARAMS_F32` causation probe.

**Phase A final-review fixes:** (1) every READ widened too — `mp_get` (+71 double fallback literals), locals, helper params, `fm_operator`/`fm_preset`, `adsr_t`, `synth_params` + POST helpers; `TestModularParamsAreNeverNarrowed` pins 0 implicit double→float conversions across modular.c, modular_stages.c, fmsynth.c, adsr.c, drums.c (98 more ids moved: envelopes ≤0.003 dB, pitch ±0.0000 cents). Total one-time change pre-double→final: 141/150 instruments, worst kick-boom-bap −56 dBFS (one 23 µs hold sample), then strict byte parity. (2) Browser test now catches the native default-voice fallback (record carries viaWorker/nativeFallback; distinct hashes required); JS throws a named error when the C glue lacks HEAPF64 — **commit `src/js/drums.single.js` together with audio.js / synth_render_worker.js / modular.h / Makefile**. (3) Parity gate widened: every visible knob at Min/Max + melodic pitch ±12/−5/+7 (50,976 case hashes), fails on lost instruments/cases. Phase B baseline: `~/.beatmo/render-parity/phaseA-final`.
