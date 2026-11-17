# Config-only Synth Engine — Phase D (every sound-shaping number in the voice engine becomes config) Implementation Plan

> **SUPERSEDED (2026-10-04)** by `docs/superpowers/specs/2026-10-04-instrument-config-documents-design.md`: lifted literals must live in each instrument's own document, not as table defaults. Kept for its literal inventory, guard design and per-literal procedure, which sub-project 2 reuses.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> **Repo rule:** never `git add` / `git commit` / `git stash` (the user commits). Every "commit" step is a VERIFY CHECKPOINT: run the named suite, read the output, append a ledger line.

**Goal:** No number that shapes an instrument's sound is written in the C voice engine any more: every one is a field of the param block, with the old literal as its value, so a voice is a pure function of config — bit-exact with today's sound.

**Architecture:** Each sound-shaping literal in `src/c/modular_stages.c` / `src/c/modular.c` becomes ONE row in the ABI table `modularFields` (`internal/audio/modular_fields.go`), appended in a per-voice block whose rows carry the literal as `Identity`, the instrument that exercises it as `Probe`, and its role as `Doc`; the C code reads `p->gen_<name>[k]` (or `p-><name>` for a global) in place of the literal. Because the identity IS the old literal and every row is appended (existing offsets never move), every render stays bit-identical with zero seed churn. A voice "variant" that only picks constants out of a per-variant table (the tom) stops being a variant: its constants become shared fields that each seed spells, and the variant field is deleted. Two guards make the work measurable and keep it done: a per-function literal budget over the C sources (`TestVoiceFunctionsCarryNoSoundLiterals`, every task drives its functions to 0) and a liveness test (`TestLiftedFieldsAreLive`: moving any lifted field changes its probe instrument's render, and its range ends never blow the voice up). Lifted fields are config (project files, instrument bundles, `setInstrumentParam`) and hidden from the Synth tab; Phase E's Advanced editor surfaces them together with the table-valued params.

**Tech Stack:** C (`src/c`, native via cgo `build/libdrums.a`, browser via emscripten `src/js/drums.single.js`), Go (`src/go/internal/audio`, `src/go/cmd/gen-synth-abi`), JS (`src/js/synth_param_abi.gen.js`, generated).

**Spec:** roadmap `docs/superpowers/plans/2026-10-02-config-only-synth-engine.md`, Phase D row: "Lift sound-shaping literals → params, one family per task, each bit-exact: POST + reverb + KS + clap + analytic → tom + snare → cymbal → FM (+ topology/op tables) → kick v0–v8 (incl. variant-8 mode tables) → waveguides/bow/formant scalars; variants remain algorithm selectors". User mandate (2026-10-02): "all instruments provide CONFIG-only params to the engine and there are no hardcoded synth params that are part of the instrument … The user must be able to change every single part of the synth pipeline … pure functions that do not hardcode params." Phase C ledger + rulings: `.superpowers/sdd/2026-10-02-config-only-synth-engine/progress.md`. Phase D parity baseline: `~/.beatmo/render-parity/Ffix2`.

## Global Constraints

- Bundled Go only: `.tools/go/bin/go`. Fast path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ...`; real path: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test ...`.
- Concurrent sessions edit the tree: run Go tests with `-overlay=$S/overlay/overlay.json` after `python3 $S/refresh_overlay.py` (`$S` = the session scratchpad). Never touch the audio-lane, groove-template/gen-showcase, startup-demo or theme files.
- After ANY C edit: `make build/libdrums.a` (cgo links a PREBUILT archive). Browser: `rm -f src/js/drums.single.js && make wasm`.
- Regenerate both ABI files with `make gen-synth-abi` after any `modularFields` edit (never redirect into the header).
- **No sound changes in Phase D.** All 150 default renders and every existing engine case stay BIT-IDENTICAL against the previous checkpoint. The only allowed parity deltas are the ones a task names (Task D2: the cases of the deleted `gen1_tom_variant` field are "lost").
- **Append-only ABI.** New fields are appended at the END of `modularFields`, in new blocks; no existing row moves or changes identity (the one exception: Task D2 deletes `tom_variant`).
- **One field per ROLE, never per value.** Two literals that happen to be equal (`0.85` as a fundamental level and `0.85` as a softsat scale) are two fields. A literal used by two voices is two fields (one per voice block).
- **Float literals stay float.** A literal written with an `f` suffix (`1.3f`) is read as `(float)p->gen_x[k]`; a double literal is read as `p->gen_x[k]`. Identity is the literal's decimal text (`1.3`, not `1.2999999523`), because `(float)1.3 == 1.3f` exactly.
- Lifted fields are `SynthHiddenGroup` (config, not Synth-tab knobs). No i18n/gloss work in Phase D.
- Insert/send FX literals (`insert_fx.c`, `effects.c`, and their JS twin `insert_fx_kernels.js`) are OUT of Phase D — see Task D10 (separate plan).
- Never commit audio; render dumps live under `~/.beatmo/render-parity`.

## Review Focus

1. **A float literal lifted as a double read** (`* 1.3f` → `* p->x`) moves float arithmetic into double and every sample drifts. Expected: bit-identical renders. Pinned by each task's parity checkpoint (150 defaults + all engine cases bit-identical) and the "float literals stay float" rule in the per-literal procedure.
2. **One field coupling two roles** (lifting every `0.85` in a voice into one `kx_level`): editing the field then changes two unrelated things. Expected: every role its own knob. Pinned by the procedure's naming rule and `TestLiftedFieldsHaveOneSiteEach` (Task D0): a lifted field name appears at exactly one read site in the C sources.
3. **A lifted literal on a code path no shipped instrument reaches** (blown Karplus–Strong, an unused variant branch): the field is config nobody can hear, and a later edit could break it unseen. Expected: every lifted field audibly does something. Pinned by `TestLiftedFieldsAreLive` (Task D0): every lifted row needs a `Probe` (+ optional `ProbeSet` context) in which moving it changes the render.
4. **A config edit at a lifted field's range end blows the voice up** (frequency 0, a pole at 1 — the Phase C snare-tune class) and the FINISH guard turns it into silence. Expected: any value inside the field's bounds renders a finite, sounding voice. Pinned by `TestLiftedFieldExtremesStayAudible` (Task D0): at Min and at Max the probe render is finite and not mostly exact zeros (rows that legitimately mute, like an output gain, set `MayMute: true`).
5. **A saved project carrying a deleted field** (`gen1_tom_variant`, Task D2) — expected: import rejects it with a message naming the field (the C6 contract), the user's stored recipes drop it with a WARN; never a silently different tom. Pinned by Task D2's `TestImportRejectsRetiredTomVariant`.

---

## The per-literal procedure (every lift task follows it verbatim)

For each literal the guard lists in a function the task owns:

1. **Classify.**
   - **Structural** (stays a literal): `2π`/unit conversions (cents↔ratio `1200`, dB `20`/`10`, seconds↔samples), sample-index/time arithmetic, Nyquist/stability guards (`0.45·sr`, `0.99` pole caps), epsilons/denormal floors, buffer and array sizes, loop bounds over a fixed table, the final ±1 output-safety clamp, integer selector comparisons (`variant == 3`), noise-draw counts that fix the RNG stream order. Mark the line: append `/* structural: <one of the reasons above> */`. A line holding both a structural and a sound literal gets the sound literal lifted; the marker then covers only what remains.
   - **Sound-shaping** (everything else: frequencies, ratios, rates, levels, mixes, filter coefficients, curve exponents, saturation amounts, harmonic multipliers, per-partial tables of ≤ 16 entries): lift it.
2. **Name it** `<prefix>_<role>` with the voice prefix from the table below and a role in snake_case that says what the number does (`kpunch_click_decay`, `kpunch_fund_level`, `kpunch_h2_ratio`). A small per-voice table (`baseFreq[6]`) lifts as one row per entry: `ohh_partial1_hz` … `ohh_partial6_hz`.
3. **Add the row** at the END of `modularFields`, in the voice's block (create it the first time):
   ```go
   {Block: "kickpunchy", Name: "kpunch_click_decay", Slots: 1, Identity: 120.0, Probe: "kick-snap", Doc: "click envelope decay rate (1/s)"},
   ```
   - `Slots` = the family's slot capacity (kick/tom/snare/clap/fm/ks/bass = 1, cymbal = 3, generic gen bank = 12); `0` for a voice in `modular.c` that reads globals (`p->x`, not `p->gen_x[k]`).
   - `Identity` = the literal's decimal text exactly (`0.80`, `120.0`, `1.3` for `1.3f`).
   - `Probe` = an instrument id whose seed plays this code path; when none does, add `ProbeSet: RecipeParams{...}` to switch the path on (e.g. `{"gen1_ks_blow": 1}` on `bass-guitar`).
   - `Min`/`Max` only when the default rule (`liftedBounds`, Task D0) is wrong for the role; set `MayMute: true` only for a pure output/mix gain whose Min legitimately silences the voice.
4. **Read it in C**: `exp(-120.0 * tSec)` → `exp(-p->gen_kpunch_click_decay[k] * tSec)`; `* 1.3f` → `* (float)p->gen_kpunch_out_gain[k]`; a global: `p->osc_xxx`. Never hoist a read into an expression that changes evaluation order (`a * 0.85 * b` stays `a * p->x * b`, not `a * b * p->x`).
5. After the function is done: `make gen-synth-abi && make build/libdrums.a`, run the guard + liveness tests for the task's prefix, then continue with the next function.

Voice prefixes (block names in parentheses): `kpunch` (kickpunchy), `ktight` (kicktight), `kmodal` (kickmodal), `khyb` (kickhybrid), `kbase` (kickbase), `klofi` (kicklofi), `kdeep` (kickdeep), `kacou` (kickacoustic), `kphys` (kickphysical), `kslot` (kickslot), `tom` (tom — shared fields, Task D2), `rim` (snarerim), `sstick` (snaresidestick), `smain` (snaremain), `sacou` (snareacoustic), `sslot` (snareslot), `clap` (clap), `ohh` (cymopen), `cbell` (cymcowbell), `shk` (cymshaker), `crash` (cymcrash), `plate` (cymmodal), `cslot` (cymslot), `fmv` (fmvoice), `fmp` (fmpreset), `ks` (ks), `abass` (analytic), `bank` (genbank), `gfilt` (genfilter), `fin` (finish), `osc` (core), `uni` (unison), `phys` (physical), `vib` (vibrato), `bow` (bowed), `brass` (brass), `reed` (reed), `flute` (flute), `sax` (sax), `fmt` (formant).

## Verify checkpoint (the same at the end of every task)

```bash
S=/tmp/claude-1000/-home-ymolinar-Repos-beatmo/<session>/scratchpad   # this session's scratchpad
R=/home/ymolinar/Repos/beatmo; GO=$R/.tools/go/bin/go; P=$HOME/.beatmo/render-parity
python3 $S/refresh_overlay.py >/dev/null; O=$S/overlay/overlay.json
cd $R && make gen-synth-abi && make build/libdrums.a
cd src/go
$GO test -tags test -modfile=go.test.mod -overlay=$O ./internal/audio/ -count=1          # fast audio
xvfb-run -a $GO test -overlay=$O ./internal/audio/ -count=1 -timeout 30m               # real audio (guard, liveness, extremes, goldens)
rm -rf $P/<LABEL>
BEATMO_RENDER_PARITY_DIR=$P BEATMO_RENDER_PARITY_LABEL=<LABEL> xvfb-run -a $GO test -overlay=$O -run TestRenderParityDump -count=1 -timeout 3600s ./internal/audio/
BEATMO_RENDER_PARITY_DIR=$P BEATMO_RENDER_PARITY_A=<PREV> BEATMO_RENDER_PARITY_B=<LABEL> BEATMO_RENDER_PARITY_DROPPED="<task's dropped globs>" xvfb-run -a $GO test -overlay=$O -run TestRenderParityCompare -count=1 -v ./internal/audio/
cd $R && rm -f src/js/drums.single.js && make wasm
for t in xplat_audio_compare modular_synth_audible synth_param_block_double wasm_bridge_smoke; do GO=$GO WASM_PREBUILT=1 node src/js/$t.browser.test.js || echo "FAIL $t"; done
```

Expected: fast + real audio PASS; compare prints `150 bit-identical, 0 changed` and `cases: N bit-identical, 0 changed, … 0 lost` (Task D2: lost = the `gen1_tom_variant` cases only); all four browser tests PASS. Ledger line: `Task D<n>: complete — <functions> budget → 0, <k> fields lifted (<prefixes>), parity <PREV>→<LABEL> bit-identical; browser 4/4`.

---

## File map

- **Modify** `src/go/internal/audio/modular_fields.go` — `modularField` gains `Probe`, `ProbeSet`, `Min`, `Max`, `MayMute`; every task appends its voice blocks (D0–D9).
- **Modify** `src/go/internal/audio/modular_recipe.go` — one generic loop turns every probed (lifted) row into a hidden ParamDef with `liftedBounds` (D0); `tom_variant` bounds removed (D2).
- **Create** `src/go/internal/audio/voice_literal_guard_test.go` + `testdata/voice_literal_budget.json` — the per-function literal budget (D0; each task edits the JSON).
- **Create** `src/go/internal/audio/lifted_fields_test.go` — liveness, extremes and one-site tests (D0).
- **Modify** `src/go/internal/audio/modular_fields_test.go` — layout pin becomes append-only + `MODULAR_LAYOUT_UPDATE=1` writer (D0).
- **Modify** `src/c/modular_stages.c`, `src/c/modular.c` — literal reads → field reads, structural markers (D0–D9).
- **Modify** `src/go/internal/audio/instrument_seeds.go`, `modular_family_stages.go` — tom seeds spell their constants, `tom_variant` knob removed (D2).
- **Regenerated** `src/c/modular_params.gen.h`, `src/js/synth_param_abi.gen.js` (every task).
- **Modify** docs: `CLAUDE.md`, `AGENTS.synth.md`, `AGENTS.synth.instruments.md`, roadmap row (D11).

---

### Task D0: Guards, lifted-field plumbing, and the pilot voice (punchy kick)

**Files:**
- Modify: `src/go/internal/audio/modular_fields.go` (struct + punchy block)
- Modify: `src/go/internal/audio/modular_recipe.go` (generic lifted ParamDefs)
- Modify: `src/go/internal/audio/modular_fields_test.go` (append-only pin)
- Create: `src/go/internal/audio/voice_literal_guard_test.go`, `src/go/internal/audio/testdata/voice_literal_budget.json`
- Create: `src/go/internal/audio/lifted_fields_test.go`
- Modify: `src/c/modular_stages.c` (`gen_kick_punchy`)

**Interfaces:**
- Produces: `modularField{Block, Name string; Slots int; Identity float64; Doc string; Probe string; ProbeSet RecipeParams; Min, Max float64; MayMute bool}`; `liftedFields() []modularField` (rows with `Probe != ""`); `liftedBounds(f modularField) [2]float64`; `liftedParamName(f modularField) string` (`gen1_<Name>` when `Slots > 0`, else `Name`); the budget file format `{"<file>:<function>": count}`; `renderInstrumentWith(t, id, over RecipeParams) []float32` (moved from `finish_stage_engine_test.go`, real build).

- [ ] **Step 1: Write the literal guard (failing on purpose for the pilot).** `voice_literal_guard_test.go` (no build tag; uses `repoRootForC` from `c_header_parity_test.go`):

```go
//go:build !js

package audio

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// voice_literal_guard_test.go — every number that shapes a sound lives in the
// param block (Phase D). The guard counts the numeric literals left in each
// C voice function and pins the count in testdata/voice_literal_budget.json.
// A count ABOVE budget is a regression (a hardcoded sound number came back); a
// count BELOW budget means a lift landed and the budget must be tightened in
// the same change (ratchet). Free: 0, 1 (and their .0/f spellings), 2π, lines
// marked "structural:", and the Phase-E table initializers named below.

var (
	literalRe     = regexp.MustCompile(`(?:^|[^\w.])((?:\d+\.\d*|\.\d+|\d+)(?:[eE][-+]?\d+)?[fF]?)(?:[^\w.]|$)`)
	freeLiterals  = map[string]bool{"0": true, "1": true, "0.0": true, "1.0": true, "0.0f": true, "1.0f": true, "0.f": true, "1.f": true}
	twoPiRe       = regexp.MustCompile(`\b2\.0f?\s*\*\s*M_PI\b`)
	piDefineRe    = regexp.MustCompile(`#define\s+M_PI\s+[0-9.]+`)
	phaseETableRe = regexp.MustCompile(`\b(violin|guitar|cello|steel)[FQG]\b|\bform_(hz|db|bw|sing_hz)\b`)
	funcStartRe   = regexp.MustCompile(`^(?:static\s+)?(?:inline\s+)?(?:EXPORT\s+)?[\w\s*]+?\b(\w+)\s*\([^;]*$`)
	blockCmtRe    = regexp.MustCompile(`(?s)/\*.*?\*/`)
	lineCmtRe     = regexp.MustCompile(`//[^\n]*`)
)

type voiceLiteral struct {
	Fn   string
	Line int
	Tok  string
}

func scanVoiceLiterals(t *testing.T, file string) []voiceLiteral {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRootForC(t), "src", "c", file))
	if err != nil {
		t.Fatal(err)
	}
	rawLines := strings.Split(string(raw), "\n")
	marked := map[int]bool{}
	for i, l := range rawLines {
		if strings.Contains(l, "structural:") {
			marked[i] = true
		}
	}
	src := blockCmtRe.ReplaceAllStringFunc(string(raw), func(m string) string { return strings.Repeat("\n", strings.Count(m, "\n")) })
	src = lineCmtRe.ReplaceAllString(src, "")
	src = twoPiRe.ReplaceAllString(src, "M_PI2")
	src = piDefineRe.ReplaceAllString(src, "")
	var out []voiceLiteral
	fn, depth, inE := "<file>", 0, false
	for i, l := range strings.Split(src, "\n") {
		if depth == 0 {
			if m := funcStartRe.FindStringSubmatch(l); m != nil {
				switch m[1] {
				case "if", "for", "while", "switch":
				default:
					fn = m[1]
				}
			}
		}
		if phaseETableRe.MatchString(l) && strings.Contains(l, "=") && strings.Contains(l, "{") {
			inE = true
		}
		if !inE && !marked[i] && !strings.HasPrefix(strings.TrimSpace(l), "#include") {
			for _, m := range literalRe.FindAllStringSubmatch(l, -1) {
				if !freeLiterals[m[1]] {
					out = append(out, voiceLiteral{Fn: fn, Line: i + 1, Tok: m[1]})
				}
			}
		}
		if inE && strings.Contains(l, "};") {
			inE = false
		}
		depth += strings.Count(l, "{") - strings.Count(l, "}")
		if depth == 0 && strings.Contains(l, "}") {
			fn = "<file>"
		}
	}
	return out
}

func TestVoiceFunctionsCarryNoSoundLiterals(t *testing.T) {
	got := map[string][]voiceLiteral{}
	for _, f := range []string{"modular_stages.c", "modular.c"} {
		for _, v := range scanVoiceLiterals(t, f) {
			key := f + ":" + v.Fn
			got[key] = append(got[key], v)
		}
	}
	path := "testdata/voice_literal_budget.json"
	if os.Getenv("VOICE_LITERAL_BUDGET_UPDATE") == "1" {
		counts := map[string]int{}
		for k, v := range got {
			counts[k] = len(v)
		}
		b, _ := json.MarshalIndent(counts, "", "  ")
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var budget map[string]int
	if err := json.Unmarshal(raw, &budget); err != nil {
		t.Fatal(err)
	}
	keys := map[string]bool{}
	for k := range got {
		keys[k] = true
	}
	for k := range budget {
		keys[k] = true
	}
	var sorted []string
	for k := range keys {
		sorted = append(sorted, k)
	}
	sort.Strings(sorted)
	for _, k := range sorted {
		n, want := len(got[k]), budget[k]
		switch {
		case n > want:
			var where []string
			for _, v := range got[k] {
				where = append(where, fmt.Sprintf("%d:%s", v.Line, v.Tok))
			}
			t.Errorf("%s: %d sound literals, budget %d — lift them into modularFields or mark the line \"structural: <reason>\": %s",
				k, n, want, strings.Join(where, " "))
		case n < want:
			t.Errorf("%s: %d literals, budget %d — tighten testdata/voice_literal_budget.json to %d (ratchet)", k, n, want, n)
		}
	}
}
```

- [ ] **Step 2: Freeze today's counts.** Run `cd src/go && VOICE_LITERAL_BUDGET_UPDATE=1 ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run TestVoiceFunctionsCarryNoSoundLiterals ./internal/audio/`, then without the env var. Expected: PASS; the JSON lists ~50 functions totalling ~978 (largest: `gen_kick_physical` 106, `modular_gen_slot_fm` 75, `render_modular_p` 61, `modular_gen_slot_tom` 55). Then set `"modular_stages.c:gen_kick_punchy": 0` in the JSON and run again. Expected: FAIL naming 11 literals with line numbers (`0.80 0.20 120.0 2.0 0.85 60.0 1.3f` and four more).

- [ ] **Step 3: Write the lifted-field tests (failing: no probed rows yet / no fields).** `lifted_fields_test.go`:

```go
//go:build !test && !js

package audio

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// lifted_fields_test.go — every literal lifted into modularFields (Phase D)
// is a live, safe knob: moving it changes the render of the instrument that
// plays its code path (Probe [+ProbeSet]), and its range ends leave a finite,
// sounding voice.

func probeRender(t *testing.T, f modularField, v float64) []float32 {
	t.Helper()
	over := RecipeParams{}
	for k, x := range f.ProbeSet {
		over[k] = x
	}
	over[liftedParamName(f)] = v
	return renderInstrumentWith(t, f.Probe, over)
}

func TestLiftedFieldsAreLive(t *testing.T) {
	if len(liftedFields()) == 0 {
		t.Fatal("no lifted fields — Phase D has not started")
	}
	for _, f := range liftedFields() {
		b := liftedBounds(f)
		base := f.Identity
		if v, ok := MergedInstrumentParamsRO(f.Probe)[liftedParamName(f)]; ok {
			base = v
		}
		alt := base * 1.25
		if base == 0 {
			alt = b[0] + 0.5*(b[1]-b[0])
		}
		alt = math.Min(math.Max(alt, b[0]), b[1])
		if alt == base {
			alt = b[0]
		}
		if equalF32(probeRender(t, f, base), probeRender(t, f, alt)) {
			t.Errorf("%s (%s): moving it %v→%v does not change %s's render — dead field or wrong Probe", liftedParamName(f), f.Doc, base, alt, f.Probe)
		}
	}
}

func TestLiftedFieldExtremesStayAudible(t *testing.T) {
	for _, f := range liftedFields() {
		b := liftedBounds(f)
		for _, v := range []float64{b[0], b[1]} {
			buf := probeRender(t, f, v)
			zeros, bad := 0, 0
			for _, s := range buf {
				if s == 0 {
					zeros++
				}
				if math.IsNaN(float64(s)) || math.IsInf(float64(s), 0) {
					bad++
				}
			}
			if bad > 0 || (!f.MayMute && zeros > len(buf)/2) {
				t.Errorf("%s=%v on %s: %d non-finite, %d/%d exact zeros — the voice blew up or went silent inside the field's bounds",
					liftedParamName(f), v, f.Probe, bad, zeros, len(buf))
			}
		}
	}
}

// TestLiftedFieldsHaveOneSiteEach: a lifted field is ONE role, read at ONE
// place in the C voice code (Review Focus 2).
func TestLiftedFieldsHaveOneSiteEach(t *testing.T) {
	var src []byte
	for _, f := range []string{"modular_stages.c", "modular.c"} {
		b, err := os.ReadFile(filepath.Join(repoRootForC(t), "src", "c", f))
		if err != nil {
			t.Fatal(err)
		}
		src = append(src, b...)
	}
	for _, f := range liftedFields() {
		field := f.Name
		if f.Slots > 0 {
			field = "gen_" + f.Name
		}
		n := len(regexp.MustCompile(`p->`+regexp.QuoteMeta(field)+`\b`).FindAll(src, -1))
		if n != 1 {
			t.Errorf("%s is read at %d sites in the C voice code, want exactly 1", field, n)
		}
	}
}
```

Move `renderFinish` from `finish_stage_engine_test.go` to `lifted_fields_test.go` as `renderInstrumentWith` (same body; update its two callers). Run: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test -overlay=$O ./internal/audio/ -run 'TestLifted' -count=1`. Expected: FAIL — `undefined: liftedFields` (compile).

- [ ] **Step 4: Plumbing.** In `modular_fields.go` extend the struct and add the helpers:

```go
type modularField struct {
	Block    string
	Name     string
	Slots    int
	Identity float64
	Doc      string
	// Phase D (lifted literals). Probe is the instrument whose seed plays the
	// code path the field shapes; ProbeSet switches that path on when no seed
	// does. Min/Max override liftedBounds; MayMute marks a pure gain whose Min
	// legitimately silences the voice.
	Probe    string
	ProbeSet RecipeParams
	Min, Max float64
	MayMute  bool
}

// liftedFields returns the rows lifted out of C voice code (they carry a Probe).
func liftedFields() []modularField {
	var out []modularField
	for _, f := range modularFields {
		if f.Probe != "" {
			out = append(out, f)
		}
	}
	return out
}

// liftedParamName is the param a lifted row is addressed by (slot 1 for a
// per-slot field: every family voice that owns a lifted block plays slot 1).
func liftedParamName(f modularField) string {
	if f.Slots > 0 {
		return "gen1_" + f.Name
	}
	return f.Name
}

// liftedBounds is a lifted field's editable range: the row's own Min/Max when
// set, else 0..4× a positive identity (at least 0..1), 4×..0 for a negative
// one, and 0..1 for a zero identity.
func liftedBounds(f modularField) [2]float64 {
	if f.Min != 0 || f.Max != 0 {
		return [2]float64{f.Min, f.Max}
	}
	switch {
	case f.Identity > 0:
		return [2]float64{0, math.Max(1, 4*f.Identity)}
	case f.Identity < 0:
		return [2]float64{4 * f.Identity, 0}
	}
	return [2]float64{0, 1}
}
```

In `modular_recipe.go`, at the end of `ModularSynthParamDefs` (after the last block loop), add:

```go
	// Phase D: every literal lifted out of the C voice code is config — a
	// hidden ParamDef (editable through project files, bundles and
	// setInstrumentParam; Phase E's Advanced editor surfaces them).
	for _, f := range liftedFields() {
		b := liftedBounds(f)
		if f.Slots > 0 {
			defs = appendGenSlotDefs(defs, f, b, ident)
		} else {
			defs = append(defs, ParamDef{Name: f.Name, Label: f.Name, Group: SynthHiddenGroup, Min: b[0], Max: b[1], Default: f.Identity})
		}
	}
```

(Check `appendGenSlotDefs`'s group assignment: lifted rows must land in `SynthHiddenGroup`; if the helper derives the group from the block, pass the hidden group explicitly.)

- [ ] **Step 5: Layout pin becomes append-only.** In `TestModularFieldsReproduceTheShippedLayout`, replace the length equality with: every frozen field keeps its index and identity, and extra fields may follow; add `MODULAR_LAYOUT_UPDATE=1` to rewrite `testdata/modular_schema_layout.json` from the live schema (used only by Task D2). The error for a shorter schema stays (`schema has %d fields, shipped layout %d`).

- [ ] **Step 6: Lift the pilot.** Apply the per-literal procedure to `gen_kick_punchy`: rows (block `kickpunchy`, `Slots: 1`, `Probe: "kick-snap"`):

```go
	{Block: "kickpunchy", Name: "kpunch_click_lp_keep", Slots: 1, Identity: 0.80, Probe: "kick-snap", Doc: "click one-pole low-pass: weight kept from the previous state"},
	{Block: "kickpunchy", Name: "kpunch_click_lp_alpha", Slots: 1, Identity: 0.20, Probe: "kick-snap", Doc: "click one-pole low-pass: weight of the new sample"},
	{Block: "kickpunchy", Name: "kpunch_click_decay", Slots: 1, Identity: 120.0, Probe: "kick-snap", Doc: "click envelope decay rate (1/s)"},
	{Block: "kickpunchy", Name: "kpunch_h2_ratio", Slots: 1, Identity: 2.0, Probe: "kick-snap", Doc: "second partial frequency as a multiple of f0"},
	{Block: "kickpunchy", Name: "kpunch_fund_level", Slots: 1, Identity: 0.85, Probe: "kick-snap", Doc: "fundamental level in the tonal mix"},
	{Block: "kickpunchy", Name: "kpunch_attack_decay", Slots: 1, Identity: 60.0, Probe: "kick-snap", Doc: "attack-shape envelope decay rate (1/s)"},
	{Block: "kickpunchy", Name: "kpunch_out_gain", Slots: 1, Identity: 1.3, Probe: "kick-snap", MayMute: true, Doc: "output gain after the soft saturator (float)"},
```

C edits in `gen_kick_punchy`: `sp_onepole_mix(&lpClick, p->gen_kpunch_click_lp_keep[k], p->gen_kpunch_click_lp_alpha[k], clickRaw)`, `exp(-p->gen_kpunch_click_decay[k] * tSec)`, `double f1 = f0 * p->gen_kpunch_h2_ratio[k];`, `s0 * env0 * p->gen_kpunch_fund_level[k] + s1 * env1 * kH2Gain`, `exp(-p->gen_kpunch_attack_decay[k] * tSec)`, `softsat_shared((float)mixed * (float)kSat) * (float)p->gen_kpunch_out_gain[k]`. The guard lists the remaining four literals of the function: classify each by the procedure (lift or mark structural). Then `make gen-synth-abi && make build/libdrums.a`.

- [ ] **Step 7: Run the task's tests.** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -overlay=$O -run 'TestVoiceFunctionsCarryNoSoundLiterals|TestModularFieldsReproduceTheShippedLayout|TestGeneratedCHeaderIsCurrent' ./internal/audio/` and `xvfb-run -a ../../.tools/go/bin/go test -overlay=$O -run 'TestLifted|TestLinkedEngineMatchesTheSchema' ./internal/audio/`. Expected: PASS (the budget for `gen_kick_punchy` is 0 and met).

- [ ] **Step 8: VERIFY CHECKPOINT** (label `D0`, previous `Ffix2`). Expected: as in "Verify checkpoint".

---

### Task D1: Shared stages — FINISH, generator filter, gen bank, Karplus–Strong, analytic bass, clap

**Files:** Modify `src/c/modular.c` (`modular_finish`), `src/c/modular_stages.c` (`gen_slot_filter_init`, `gen_slot_filter_tick`, `mod_biquad_set_coeffs`, `modular_gen_bank_render`, `modular_gen_slot_ks`, `modular_gen_slot_analytic`, `modular_gen_slot_clap`), `modular_fields.go`, `testdata/voice_literal_budget.json`.

**Interfaces:** Consumes D0 (`modularField.Probe/ProbeSet/Min/Max/MayMute`, `liftedBounds`, the guard). Produces blocks `finish` (`fin_*`, `Slots: 0`), `genfilter` (`gfilt_*`, `Slots: 12`), `genbank` (`bank_*`), `ks` (`ks_*`, `Slots: 1`), `analytic` (`abass_*`, `Slots: 1`), `clap` (`clap_*`, `Slots: 1`).

Budget (guard counts today): `modular_finish` 16, `gen_slot_filter_init` 2, `gen_slot_filter_tick` 4, `mod_biquad_set_coeffs` 16, `modular_gen_bank_render` 49, `modular_gen_slot_ks` 16, `modular_gen_slot_analytic` 6, `modular_gen_slot_clap` 12.

- [ ] **Step 1: RED.** Set those eight entries to 0 in `testdata/voice_literal_budget.json`; run the guard. Expected: FAIL listing 121 literals with line numbers.
- [ ] **Step 2: Lift `modular_finish`.** Sound literals: the Low Boost cutoff `120.0`, Crisp `4000.0` + `8000.0`, Darken `2000.0` + `6000.0`, Crunch `3.0` and the 0.5 blend factors (`fin_body_cutoff_hz`, `fin_crisp_base_hz`, `fin_crisp_span_hz`, `fin_crisp_blend`, `fin_darken_base_hz`, `fin_darken_span_hz`, `fin_crunch_drive_span`, `fin_body_dry_cut`); `Probe: "kick"`, `ProbeSet` switching the op on (e.g. `{"post_body": 0.5}` for the body rows, `{"post_brightness": 0.5}`, `{"post_darken": 0.5}`, `{"post_drive": 0.5}`). Structural: the `0.01f` and `1.0` Length bounds (`structural: stability guard`). The FINISH tests from Phase C (`TestFinishKnobExtremesStayFiniteAndBounded`, `TestFinishKnobsRespondProportionally`) must stay green.
- [ ] **Step 3: Lift the shared generator filter and biquad helpers.** `mod_biquad_set_coeffs`'s `20.0`/`0.99` are frequency/pole safety guards → `structural: stability guard`. Any default Q or slope literal is sound → `gfilt_*` (`Slots: 12`, `Probe`: an instrument whose seed enables `gen1_filt_*`; find one with `grep -n '"gen1_filt' src/go/internal/audio/instrument_seeds.go`).
- [ ] **Step 4: Lift `modular_gen_bank_render`** (`bank_*`): mix weights, decay/pole coefficients, the `0.13` and similar constants. Per-slot loop bounds (`32`, slot counts) are structural.
- [ ] **Step 5: Lift `modular_gen_slot_ks`** (`ks_*`, `Probe: "bass-guitar"`): the `0.005` attack ramp, `0.9f` filter weight, `0.008`, the blown-loop `1.05` regeneration, `0.10` breath low-pass, the 4096 buffer sizes stay structural. Blown-only literals use `ProbeSet: RecipeParams{"gen1_ks_blow": 1}` (they are unreachable from any seed — Review Focus 3).
- [ ] **Step 6: Lift `modular_gen_slot_analytic`** (`abass_*`, `Probe: "sub-bass"`) and `modular_gen_slot_clap` (`clap_*`, `Probe: "clap"`): the clap's `1800.0` Hz band-pass, `1.5` Q, burst offsets `0.000 0.015 0.030`, `0.060`, `0.7`, `80.0`.
- [ ] **Step 7: Tests.** `make gen-synth-abi && make build/libdrums.a`; guard (fast) + `TestLifted|TestFinish|TestZeroIsARealValue|TestSnareVoiceStaysStable` (real). Expected: PASS.
- [ ] **Step 8: VERIFY CHECKPOINT** (label `D1`, previous `D0`).

---

### Task D2: Tom — the variant table becomes the seeds' config; `tom_variant` is deleted

**Files:** Modify `src/c/modular_stages.c` (`modular_gen_slot_tom`), `modular_fields.go` (`tom` block), `modular_recipe.go` (`tomBounds`), `modular_family_stages.go` (the `tom_variant` knob row), `instrument_seeds.go` (tom seeds), `testdata/voice_literal_budget.json`, `testdata/modular_schema_layout.json` (re-frozen), `src/go/internal/ui/import_unknown_param_test.go` (new case).

**Interfaces:** Consumes D0. Produces shared tom fields `gen1_tom_<role>` (in the existing `tom` block, `Slots: 1`): `tom_fstart_mul`, `tom_fend_mul`, `tom_fund_env_mul`, `tom_o1_env_rate`, `tom_o2_env_rate`, `tom_res_amt`, `tom_res_rate`, `tom_lp_alpha`, `tom_hp_alpha`, `tom_attack_rate`, `tom_ambient_rate`, `tom_global_fade` (identity = the variant-0 value of each table: 1.25, 0.85, 1.15, 4.0, 5.5, 0.4, 80.0, 0.12, 0.92, 120.0, 6.0, 0.05), plus `tom_*` rows for every other literal of the voice. Deletes `tom_variant`.

The tom's `variant` only indexes twelve 3-entry constant tables (`fStartMul[3]` … `globalFade[3]`, `modular_stages.c` ≈ L1437-1448); the code path is identical for all three. A variant that selects no algorithm is not a selector — it is three presets hardcoded in the engine.

- [ ] **Step 1: RED (import contract).** Add to `import_unknown_param_test.go`:

```go
// TestImportRejectsRetiredTomVariant: Phase D deleted gen1_tom_variant (the
// tom's constants are config now). A project still carrying it is rejected by
// name, never imported as a silently different tom (Review Focus 5).
func TestImportRejectsRetiredTomVariant(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	doc := []byte(`{"schema_version":"` + schema.Version + `","subdiv":4,"bpm":120,
	 "instruments":[{"name":"Tom","id":"tom","kind":"builtin","volume":1,"origin":0,"color":"#FFFFFFFF",
	   "recipe":"synth-modular-tom","synth_params":{"gen1_tom_variant":1}}],
	 "nodes":[{"id":0,"i":0,"j":0,"type":"regular"}]}`)
	if err := g.Import(doc); err == nil || !strings.Contains(err.Error(), "gen1_tom_variant") {
		t.Fatalf("Import = %v, want a rejection naming gen1_tom_variant", err)
	}
}
```

Run (fast ui, with overlay). Expected: FAIL (import accepted — the field still exists).

- [ ] **Step 2: RED (guard).** Set `modular_stages.c:modular_gen_slot_tom` to 0. Expected: FAIL listing 55 literals.
- [ ] **Step 3: Seeds first.** For the three tom seeds (`instrument_seeds.go` ≈ L2253 / L2267 / L2281: variants 0, 1, 2) spell the twelve table values of their variant (variant 1 = the second entry of each table, variant 2 = the third), and remove `"gen1_tom_variant"` from all three. Check every other seed built from these (`grep -n 'tomSeed\|tomHighSeed\|tomLowSeed\|tomSeedWith' src/go/internal/audio/*.go`) inherits through the spread.
- [ ] **Step 4: C.** Delete the twelve tables and the `variant` read/clamp; each `table[variant]` becomes `p->gen_tom_<role>[k]`; lift the voice's remaining literals by the procedure (`Probe: "tom"`; the high/low-specific shared fields probe `tom` too — the field is shared).
- [ ] **Step 5: Go.** Delete the `tom_variant` row from `modularFields`, its bound in `tomBounds`, its knob row in `modular_family_stages.go` (and any enum label / plain-English entry referencing it: `grep -rn 'tom_variant' src/go`). `make gen-synth-abi`; re-freeze the layout: `MODULAR_LAYOUT_UPDATE=1 go test -tags test -modfile=go.test.mod -run TestModularFieldsReproduceTheShippedLayout ./internal/audio/`, then rerun without it.
- [ ] **Step 6: Tests.** `make build/libdrums.a`; guard, `TestLifted*`, `TestSeedParamsAreLiveInTheirOwnInstrumentContext` (real), `TestImportRejectsRetiredTomVariant` (fast ui). Expected: PASS. A seed key the liveness test calls dead (a table value equal for all variants is still live — every row is read) → fix the seed, never baseline.
- [ ] **Step 7: VERIFY CHECKPOINT** (label `D2`, previous `D1`, `BEATMO_RENDER_PARITY_DROPPED="gen1_tom_variant"`). Expected: 150 defaults bit-identical; cases bit-identical except the `engine:gen1_tom_variant@*` cases reported lost, plus NEW `engine:gen1_tom_*@min|max|zero` cases on the tom instruments (their seeds now spell the shared fields, which makes them seed-set). Also run `synth_save_export_import` and `import_synth_render` browser tests.

---

### Task D3: Snare family — rimshot, sidestick, main, acoustic, the slot dispatcher and `snare_fc`

**Files:** Modify `src/c/modular_stages.c` (`gen_snare_rimshot`, `gen_snare_sidestick`, `gen_snare_main`, `gen_snare_acoustic`, the `kSnareAcoustic*` file-scope tables, `modular_gen_slot_snare`, `snare_fc`), `modular_fields.go`, `testdata/voice_literal_budget.json`.

**Interfaces:** Consumes D0. Produces blocks `snarerim` (`rim_*`, `Probe: "rimshot"`), `snaresidestick` (`sstick_*`, `Probe: "sidestick"`), `snaremain` (`smain_*`, `Probe: "snare"`), `snareacoustic` (`sacou_*`, `Probe: "snare-acoustic"`), `snareslot` (`sslot_*`); all `Slots: 1`.

Budget today: rimshot 21, sidestick 13, main 47, acoustic 37, `modular_gen_slot_snare` 4, `snare_fc` 1, file-scope `kSnareAcousticRatio/Gain/Spread` 15 (`modular_stages.c:<file>`).

- [ ] **Step 1: RED.** Set the six functions to 0 and lower `modular_stages.c:<file>` by 15. Expected: FAIL listing them.
- [ ] **Step 2: Lift.** By the procedure, function by function. The acoustic snare's three 4-entry mode tables become twelve rows (`sacou_mode1_ratio` … `sacou_mode4_spread`). `snare_fc`'s `0.45` is `structural: stability guard`. The `0.02` per-hit detune steps (`1.0 + 0.02 * seed`) are sound → `<prefix>_hit_detune`.
- [ ] **Step 3: Tests.** Rebuild; guard + `TestLifted*` + `TestSnareVoiceStaysStableAcrossTuneRange` + `TestZeroIsARealValue`. Expected: PASS.
- [ ] **Step 4: VERIFY CHECKPOINT** (label `D3`, previous `D2`).

---

### Task D4: Cymbal family — open hi-hat, cowbell, shaker, crash, the modal plate, the slot dispatcher

**Files:** Modify `src/c/modular_stages.c` (`gen_cymbal_open`, `gen_cymbal_cowbell`, `gen_cymbal_shaker`, `gen_cymbal_crash`, `gen_cymbal_modal`, `modular_gen_slot_cymbal`, `cym_uniform01`), `modular_fields.go`, budget JSON.

**Interfaces:** Consumes D0. Produces blocks `cymopen` (`ohh_*`, `Probe: "hihat-open-bright"` — check which seed plays variant 1's partial voice with `cym_modes < 1`), `cymcowbell` (`cbell_*`, `Probe: "cowbell"`), `cymshaker` (`shk_*`, `Probe: "shaker"`), `cymcrash` (`crash_*`, `Probe: "crash"`), `cymmodal` additions (`plate_*`, `Probe: "ride"` / `"hihat"`), `cymslot` (`cslot_*`); all `Slots: 3` (the cymbal capacity) — `liftedParamName` addresses slot 1, so a probe whose cymbal voice plays in slot 2/3 (`conga-organic` plays its plate in slot 3) must instead use a probe with the voice in slot 1.

Budget today: open 28, cowbell 25, shaker 20, crash 39, modal 46, `modular_gen_slot_cymbal` 5, `cym_uniform01` 3.

- [ ] **Step 1: RED.** Those seven entries → 0. Expected: FAIL listing 166 literals.
- [ ] **Step 2: Lift.** Per-partial tables (`baseFreq[6]`, crash `baseFreq[8]`/`baseGain[8]`, cowbell `freqs[4]`/`gains[4]`, shaker `burstAmp[4]`) lift one row per entry. The plate's `CYM_MODAL_MAX_MODES` and the `0.45·sr` cap stay structural. `cym_uniform01`'s scaling constants are RNG arithmetic (`structural: noise-draw arithmetic`) only if they convert the integer draw to [0,1); anything that colours the noise is sound.
- [ ] **Step 3: Tests.** Rebuild; guard + `TestLifted*` + `TestZeroIsARealValue` (the hi-hat `gen1_cym_modes` case). Expected: PASS.
- [ ] **Step 4: VERIFY CHECKPOINT** (label `D4`, previous `D3`).

---

### Task D5: FM — the voice, its operator/topology constants, and `build_fm_preset`

**Files:** Modify `src/c/modular_stages.c` (`modular_gen_slot_fm`), `src/c/modular.c` (`build_fm_preset`), `modular_fields.go`, budget JSON.

**Interfaces:** Consumes D0. Produces block `fmvoice` (`fmv_*`, `Slots: 1`, probes `fm-bass` / `fm-bell` / `fm-epiano` / `fm-lead` / `fm-pluck` by which variant the literal belongs to) and `fmpreset` (`fmp_*`, `Slots: 0`, `Probe`: an instrument with `osc_type = 4`).

Budget today: `modular_gen_slot_fm` 75, `build_fm_preset` 9.

The FM variant selects a routing TOPOLOGY (which operator modulates which) — an algorithm, so it stays a selector. Every per-variant constant around it (operator envelope times `0.001f`, `0.3f`, levels, the pitch-envelope shape) is sound and lifts per variant: `fmv_<variant>_<op>_<role>` (`fmv_bass_op1_attack`), each probed by the instrument playing that variant.

- [ ] **Step 1: RED.** Both entries → 0. Expected: FAIL listing 84 literals.
- [ ] **Step 2: Lift.** The routing matrix entries that are 0/1 connection flags are structural (`structural: topology`); every non-0/1 number is lifted. Many literals carry an `f` suffix: read them with `(float)` (Review Focus 1) — the Phase C ruling removed the FM voice's knob NARROWING, not the float literals' own float-ness; keep each expression's type exactly as today.
- [ ] **Step 3: Tests + VERIFY CHECKPOINT** (label `D5`, previous `D4`). The 9 FM goldens re-pinned in Phase C must not move (`TestSynthGoldenByteIdentity`).

---

### Task D6: Kicks v0–v7 and the kick slot dispatcher

**Files:** Modify `src/c/modular_stages.c` (`gen_kick_acoustic`, `gen_kick_tight`, `gen_kick_modal`, `gen_kick_hybrid`, `gen_kick_base`, `gen_kick_lofi`, `gen_kick_deep`, `modular_gen_slot_kick`), `modular_fields.go`, budget JSON.

**Interfaces:** Consumes D0 (`gen_kick_punchy` is done). Produces blocks `kickacoustic` (`kacou_*`), `kicktight` (`ktight_*`), `kickmodal` (`kmodal_*`), `kickhybrid` (`khyb_*`), `kickbase` (`kbase_*`), `kicklofi` (`klofi_*`), `kickdeep` (`kdeep_*`), `kickslot` (`kslot_*`); all `Slots: 1`. Probes: the instrument whose seed selects that variant (`grep -n 'kick_variant' src/go/internal/audio/instrument_seeds.go`; base → `kick`, deep → `kick-deep`, lofi → `kick-lofi`, tight → `kick-tight`, …).

Budget today: acoustic 19, tight 29, modal 25, hybrid 22, base 22, lofi 20, deep 18, slot 8.

- [ ] **Step 1: RED.** Those eight entries → 0. Expected: FAIL listing 163 literals.
- [ ] **Step 2: Lift** by the procedure. The kicks repeat roles across variants (click decay, fundamental level, saturation scale): each voice gets its OWN row (`ktight_click_decay`, `kbase_click_decay`) — never a shared one.
- [ ] **Step 3: Tests + VERIFY CHECKPOINT** (label `D6`, previous `D5`).

---

### Task D7: The physical kick (variant 8) and its mode tables

**Files:** Modify `src/c/modular_stages.c` (`gen_kick_physical`, `KPHYS_*`), `modular_fields.go`, budget JSON.

**Interfaces:** Consumes D0. Produces block `kickphysical` (`kphys_*`, `Slots: 1`, `Probe`: the variant-8 instrument — `grep -n 'physicalKickSeed(' src/go/internal/audio/instrument_seeds.go`).

Budget today: `gen_kick_physical` 106 (incl. `harm[4]`, `membr[4]`, `shRatio[7]`, `shGain[7]`).

- [ ] **Step 1: RED.** → 0. Expected: FAIL listing 106 literals.
- [ ] **Step 2: Lift.** Tables one row per entry (`kphys_harm2_ratio` … `kphys_shell7_gain`); `KPHYS_MODES`/`KPHYS_SHELL` stay as structural array sizes. Existing `kick_mode_*` fields (Phase B/C) are untouched.
- [ ] **Step 3: Tests + VERIFY CHECKPOINT** (label `D7`, previous `D6`).

---

### Task D8: The modular core in `modular.c`

**Files:** Modify `src/c/modular.c` (`render_modular_p`, `modular_source_unison`, `modular_source_physical`, `modular_apply_vibrato_delay`, `modular_vibrato_phase`, `modular_source_wt_modulated`, `modular_table_for`, `modular_source_noise`, file scope incl. `MODULAR_BASE_FREQ`), `modular_fields.go`, budget JSON.

**Interfaces:** Consumes D0. Produces global (`Slots: 0`) blocks `core` (`osc_*`, `env_*`, `filt_*`, `lfo_*`, `burst_*`, `body_*` roles), `unison` (`uni_*`), `physical` (`phys_*`), `vibrato` (`vib_*`). Probes: `modular` / `modular-pad` with `ProbeSet` enabling the stage (`{"lfo_enabled": 1}`, `{"unison_voices": 3}`…).

Budget today: `render_modular_p` 61, unison 40, physical 17, vibrato-delay 5, vibrato-phase 3, wt-modulated 3, table-for 3, noise 1, `modular.c:<file>` 11.

`MODULAR_BASE_FREQ` (220 Hz, "pitch 0 sounds A3") is the convention every note name and the melodic table rely on (CLAUDE.md, universal note names): lift it as `osc_base_hz` (identity 220.0) — then `InstrumentPitchInfo`'s `220.0` constant must read the field's identity (`ModularParamSchemaIdentity()["osc_base_hz"]`) so the two can never disagree; extend `TestPitchOffsetTracksLiveParamEdits` with an `osc_base_hz` edit.

- [ ] **Step 1: RED.** Those entries → 0 (file scope: to the count of remaining structural `#define`s). Expected: FAIL.
- [ ] **Step 2: Lift**; band-limit harmonic count `MODULAR_WT_HARMONICS` and table sizes are structural.
- [ ] **Step 3: Tests** incl. `TestRenderModularP_EveryParamMutatesInContext`, `TestPitchOffsetTracksLiveParamEdits`, `TestResampleMovesTheNote` + **VERIFY CHECKPOINT** (label `D8`, previous `D7`).

---

### Task D9: Waveguides and the formant scalars

**Files:** Modify `src/c/modular.c` (`render_bowed_string`, `render_brass`, `render_reed`, `render_flute`, `render_sax`), `src/c/modular_stages.c` (`formant_bank_tune`, `modular_formant_process`), `modular_fields.go`, budget JSON.

**Interfaces:** Consumes D0. Produces global blocks `bowed` (`bow_*` — the existing `osc_bow_*` fields stay), `brass`, `reed`, `flute`, `sax` (the existing `osc_sax_*` stay), `formant` (`fmt_*`). Probes: `violin` / `cello-warm`, `french-horn`, `clarinet`, `flute`, `sax`, `voice-ahh` (check ids in `modular_instruments.go`).

Budget today: bowed 23, brass 11, reed 11, flute 14, sax 20, `formant_bank_tune` 12, `modular_formant_process` 13. Out of scope (Phase E): the string-body tables (`violin/guitar/cello/steel F/Q/G`) and the formant tables (`form_hz/db/bw/sing_hz`) — the guard already excludes them.

- [ ] **Step 1: RED** → 0. **Step 2: Lift**; delay-line sizes (`4096`, `8192`) and the bow ramp's pre-warm lengths (`110` periods, the 250 ms floor — the mode-lock fix) are sound-shaping: the ramp length decides whether the bow locks onto its fundamental, so lift them (`bow_ramp_periods`, `bow_ramp_min_sec`) with `Min` bounds that keep the fix (`Min: 110` / `Min: 0.25`) — Review Focus 4.
- [ ] **Step 3: Tests** incl. the bowed-string tuning guard (`grep -rn 'func Test.*Bow' src/go/internal/audio`) + **VERIFY CHECKPOINT** (label `D9`, previous `D8`). At the end of D9 every entry of the budget JSON is 0 except structural-only `<file>` scopes.

---

### Task D10: Insert/send FX — scope the follow-up plan (no code)

The FX literals (`insert_fx.c` 221, `effects.c` 1, and the JS twin `insert_fx_kernels.js` that must stay bit-identical to it — `insert_fx_worklet_parity.browser.test.js`) ride a different ABI: per-effect positional `ifx_*_init(...)` arguments plus `ifx_*_set_param(name, value)`, carried by `EffectSlot` param maps in projects. Lifting them means a per-effect param schema shared by C, the JS kernel and the Go `EffectDef` — a different mechanism from the modular block.

- [ ] **Step 1:** Write `docs/superpowers/plans/<date>-config-only-synth-engine-phase-d-fx.md` with the writing-plans skill after D9 lands (inventory per effect, the param-schema design, the JS-kernel parity gate). Ledger: `Task D10: complete — FX plan written`.

---

### Task D11: Docs, full verification, final review

**Files:** Modify `CLAUDE.md` (the "param block is ONE generated table" bullet: no sound literal in voice code; the guard + liveness tests; the per-literal procedure in one line), `AGENTS.synth.md` (§ Param block ABI: lifted blocks, `Probe`, `liftedBounds`, the budget JSON), `AGENTS.synth.instruments.md` ("Adding knobs to a gen-slot voice" checklist: every new constant is a lifted row; the tom has no variant), roadmap row D → this plan.

- [ ] **Step 1: Docs** as listed.
- [ ] **Step 2: Full verification** — parity `Ffix2` vs final with `BEATMO_RENDER_PARITY_DROPPED="gen1_tom_variant"` (150 bit-identical, 0 changed); fast + real Go suites (failure set must equal the Phase C final set or be explained); `rm -f src/js/drums.single.js && make wasm`; `WASM_PREBUILT=1 ./scripts/run-browser-tests.sh --no-fail-fast`; `make coverage-c-check` then `make build/libdrums.a`; `TestWasmAssetSizeBudget`. Dump `phaseD-final` as the Phase E baseline.
- [ ] **Step 3: Final review** — whole-branch reviewer (most capable model) with this plan, the Review Focus and the ledger rulings; one fix pass, each fix RED→GREEN.
