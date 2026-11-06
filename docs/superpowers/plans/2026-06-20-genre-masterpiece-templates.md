# Genre-Defining Masterpiece Templates — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans (inline) to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace all 16 built-in template circuits with 15 genre-defining masterpiece recreations (one per genre), and add three new instruments (organ, clavinet, saxophone) built on the existing modular synth.

**Architecture:** Templates are generated from `cmd/gen-showcase/specs.go` (`showcases()` → per-row `Hit{Step,Pitch,Vol,Dur}`); `gen-showcase` lays out node-loops and writes `internal/assets/templates/*.json`; a drift test pins JSON↔spec. New instruments reuse `renderModular` (no C/DSP) following the orchestral-seed precedent: seed + engine entry + recipe descriptor + binding + dispatch flag + regenerated IDs.

**Tech Stack:** Go 1.23, bundled `.tools/go/bin/go`, modular C synth (via existing `renderModular`).

**Spec:** `docs/superpowers/specs/2026-06-20-genre-masterpiece-templates-design.md` — §5 has the per-template musical blueprints (the note-level authoring target).

## Global Constraints

- **Bundled Go only:** `cd src/go && ../../.tools/go/bin/go ...`.
- **DO NOT commit or `git add`.** Standing user preference — leave all work uncommitted. Every "verify" checkpoint below replaces the usual commit step.
- **16th-note grid, 4/4:** `totalSteps = Bars*16`; beats at 0/4/8/12; offbeats 2/6/10/14; no triplets/odd meters. Swing = late "and" (step 6/14) + softer velocity.
- **Pitch = semitones from A3** (A3=0). Preserve interval/contour; transpose freely. Reference table at top of `specs.go`.
- **No clipping:** aligned-downbeat master peak must stay < 0.9 (`template_master_mix_test.go`). Keep per-row `Volume` moderate (mirror existing specs: lead ~0.85, comp ~0.6–0.75, drums 0.5–1.0).
- **Colors auto-assigned** by `build()` from `templates.InstrumentSequence` (row N → seq[N%len]); never set per-spec color.
- **Note-level content is authored at execution** against spec §5 blueprints, then ear-tuned collaboratively with the user after first listen. The plan fixes structure, instruments, keys, tempos, bars, and section shapes; exact `Hit` values are produced per task.

---

## Task 1: Add the Organ instrument

**Files:**
- Modify: `internal/audio/instrument_seeds.go` (add `organSeed`)
- Modify: `internal/audio/engine_instruments.go` (add `"organ"` CVariantInstrument)
- Modify: `internal/audio/synth_recipe.go` (descriptor + binding)
- Modify: `internal/audio/synth_recipe_dispatch.go` (melodicRecipeIDs)
- Regenerate: `internal/audio/instrument_ids_gen.go`

**Interfaces produced:** instrument id `"organ"` → recipe `"synth-modular-organ"`, pitch-aware (re-renders at node pitch).

- [ ] **Step 1: Add `organSeed`** to `instrument_seeds.go`. Additive drawbar sines (Hammond ratios) on the existing modular gens; sustained envelope; shallow Leslie LFO. Model on `pianoGrandSeed`/`fluteSeed` (additive `gen*_source:1, gen*_wave:0`). Starter:

```go
// organSeed: additive Hammond-style drawbars — sine partials at organ footage
// ratios (16',8',5⅓',4',2⅔',2',1') with a sustained envelope and a shallow
// Leslie chorus (LFO→pitch). OSC disabled; gen slots set the drawbar spectrum.
var organSeed = RecipeParams{
	"osc_enabled":   0,
	"filter_type":   0,
	"filter_cutoff": 6000,
	"amp_attack":    0.012, "amp_decay": 0.05, "amp_sustain": 1.0, "amp_release": 0.10,
	"lfo_enabled":   1, "lfo_rate": 6.2, "lfo_depth": 0.05, "lfo_target": 1, "lfo_delay": 0.0,
	"gen1_source": 1, "gen1_wave": 0, "gen1_freq_mode": 0, "gen1_freq": 0.5, "gen1_gain": 0.30, // 16'
	"gen2_source": 1, "gen2_wave": 0, "gen2_freq_mode": 0, "gen2_freq": 1.0, "gen2_gain": 0.55, // 8'
	"gen3_source": 1, "gen3_wave": 0, "gen3_freq_mode": 0, "gen3_freq": 1.5, "gen3_gain": 0.22, // 5⅓'
	"gen4_source": 1, "gen4_wave": 0, "gen4_freq_mode": 0, "gen4_freq": 2.0, "gen4_gain": 0.45, // 4'
	"gen5_source": 1, "gen5_wave": 0, "gen5_freq_mode": 0, "gen5_freq": 3.0, "gen5_gain": 0.18, // 2⅔'
	"gen6_source": 1, "gen6_wave": 0, "gen6_freq_mode": 0, "gen6_freq": 4.0, "gen6_gain": 0.20, // 2'
	"gen7_source": 1, "gen7_wave": 0, "gen7_freq_mode": 0, "gen7_freq": 6.0, "gen7_gain": 0.10, // 1⅓'
	"gain": 0.85,
}
```

- [ ] **Step 2: Register the engine instrument** in `engine_instruments.go` (after the brass family, near the modular keys/winds block):

```go
		// Organ — additive drawbars (masterpiece templates: Bach/house/techno/reggae/salsa).
		"organ": CVariantInstrument{
			Name:   "organ",
			Render: renderModular,
			Beats:  2.0,
		},
```

- [ ] **Step 3: Add recipe descriptor + binding** in `synth_recipe.go`. In the descriptor list (after `synth-modular-conga`, line ~472):

```go
	// Organ — additive drawbars (masterpiece template set).
	{ID: "synth-modular-organ", Display: "Organ", Category: modularRecipeCategory, Seed: organSeed},
```

In `builtinInstrumentRecipeBindings` (after `"conga"`, line ~561):

```go
	"organ": "synth-modular-organ",
```

- [ ] **Step 4: Mark pitch-aware** in `synth_recipe_dispatch.go` `melodicRecipeIDs`:

```go
	// Organ (masterpiece set — pitched melodic instrument)
	"synth-modular-organ": true,
```

- [ ] **Step 5: Regenerate the IDs file:**

Run: `cd src/go/internal/audio && ../../../../.tools/go/bin/go run gen_instrument_ids.go`
Expected: `instrument_ids_gen.go` now contains `"organ"`.

- [ ] **Step 6: Verify** instrument wiring + seed distinctness + pitch render:

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run 'Instrument|Seed|Recipe|PitchTable|MelodicPitch'`
Expected: PASS. If a seed-distinctness or tonal-quality test fails, adjust `organSeed` gains/partials and re-run.

---

## Task 2: Add the Clavinet instrument

**Files:** same five files as Task 1.
**Interfaces produced:** instrument id `"clavinet"` → recipe `"synth-modular-clavinet"`, pitch-aware.

- [ ] **Step 1: Add `clavinetSeed`** (Karplus-Strong pluck, bright/percussive, short sustain — model on `guitarElectricSeed`):

```go
// clavinetSeed: Karplus-Strong pluck voiced bright and percussive with a fast
// decay and a narrow upper band for the funky "clav bite" (Superstition).
var clavinetSeed = RecipeParams{
	"osc_enabled":      0,
	"gen1_source":      3,      // Karplus-Strong
	"gen1_gain":        1.0,
	"gen1_ks_sustain":  0.965,  // shorter ring than guitar (0.99)
	"gen1_ks_pluck":    0.85,   // hard, bright attack
	"filter_type":      0,
	"filter_cutoff":    6500,
	"filter_resonance": 1.4,
	"amp_attack":       0.0005,
	"amp_decay":        0.9,
	"amp_sustain":      0.0,
	"amp_release":      0.12,
	"gain":             0.9,
}
```

- [ ] **Step 2: Engine entry** in `engine_instruments.go` (next to the guitars, KS family uses `Beats: 1.5`):

```go
		// Clavinet — Karplus-Strong (funk masterpiece: Superstition).
		"clavinet": CVariantInstrument{
			Name:   "clavinet",
			Render: renderModular,
			Beats:  1.5,
		},
```

- [ ] **Step 3: Descriptor + binding** in `synth_recipe.go`:

```go
	{ID: "synth-modular-clavinet", Display: "Clavinet", Category: modularRecipeCategory, Seed: clavinetSeed},
```
```go
	"clavinet": "synth-modular-clavinet",
```

- [ ] **Step 4: Pitch-aware flag** in `synth_recipe_dispatch.go`:

```go
	"synth-modular-clavinet": true,
```

- [ ] **Step 5: Regenerate IDs** — `cd src/go/internal/audio && ../../../../.tools/go/bin/go run gen_instrument_ids.go` (now contains `"clavinet"`).

- [ ] **Step 6: Verify** — same command as Task 1 Step 6. Tune `clavinetSeed` if tonal/distinctness checks fail.

---

## Task 3: Add the Saxophone instrument

**Files:** same five files.
**Interfaces produced:** instrument id `"sax"` → recipe `"synth-modular-sax"`, pitch-aware.

- [ ] **Step 1: Add `saxSeed`** (subtractive reed: saw → band-pass formant → filter-envelope + vibrato — model on `oboeSeed`/`trumpetSeed`). Tenor formants: lower BP center than oboe, breathier noise tap:

```go
// saxSeed: SUBTRACTIVE reed (saw → band-pass formant → opening filtenv), tenor-sax
// formants (lower/broader than oboe), breath-noise sheen, vibrato LFO on sustain.
var saxSeed = RecipeParams{
	"osc_type":         1,     // saw
	"osc_enabled":      1,
	"filter_type":      5,     // band-pass formant (mirrors oboe/trumpet)
	"filter_cutoff":    1100,
	"filter_resonance": 2.2,
	"amp_attack":       0.04, "amp_decay": 0.10, "amp_sustain": 0.85, "amp_release": 0.16,
	"lfo_enabled":      1, "lfo_rate": 5.0, "lfo_depth": 0.10, "lfo_target": 1, "lfo_delay": 0.25,
	"gen1_source": 2, "gen1_gain": 0.05, "gen1_filt_type": 5, "gen1_filt_freq": 2600, "gen1_filt_q": 1.0, // breath sheen
	"filtenv_enabled": 1, "filtenv_amt": 2.4, "filtenv_decay": 0.18,
	"gain": 0.9,
}
```

- [ ] **Step 2: Engine entry** (`Beats: 2.0`, winds/brass):

```go
		// Saxophone — subtractive reed (jazz: So What; bossa: Girl from Ipanema).
		"sax": CVariantInstrument{
			Name:   "sax",
			Render: renderModular,
			Beats:  2.0,
		},
```

- [ ] **Step 3: Descriptor + binding** in `synth_recipe.go`:

```go
	{ID: "synth-modular-sax", Display: "Saxophone", Category: modularRecipeCategory, Seed: saxSeed},
```
```go
	"sax": "synth-modular-sax",
```

- [ ] **Step 4: Pitch-aware flag** in `synth_recipe_dispatch.go`:

```go
	"synth-modular-sax": true,
```

- [ ] **Step 5: Regenerate IDs** — now contains `"sax"`.

- [ ] **Step 6: Verify** — same command. Then run the broader audio suite once:

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/...`
Expected: PASS (instrument parity now includes organ/clavinet/sax; tune seeds for any tonal failures).

---

## Task 4: Author the 15 template specs

**Files:**
- Rewrite: `src/go/cmd/gen-showcase/specs.go` — replace the body of `showcases()` (the current 9 `Showcase` literals) with 15 new ones; extend the pitch-reference header comment if new octaves are used.

**Interfaces consumed:** `Showcase{Stem,BPM,Subdiv,Bars,Insts,Rows}`, `InstSpec{ID,Name,Volume,Pan,ReverbSend,SynthParams}`, `RowSpec{Inst,Hits}`, `Hit{Step,Pitch,Vol,Dur}` (all in `gen-showcase/main.go`). New instrument ids available: `"organ"`, `"clavinet"`, `"sax"`.

Author each per spec §5. Stems, BPM, Subdiv=16, Bars, and the §1 genre band:

| Stem | BPM | Bars | Instruments (row order) |
|---|---|---|---|
| `bach-toccata` | 70 | 8 | organ (lead), organ (pedal/bass), violin-ensemble |
| `vivaldi-spring` | 110 | 8 | violin-ensemble, violin (solo/birdsong), cello, organ (continuo) |
| `mozart-k545` | 120 | 8 | piano-grand (melody), piano-felt (Alberti bass) |
| `eagles-hotel-california` | 75 | 8 | guitar-steel (arp), bass-guitar, guitar-electric (lead A), guitar-electric (lead B/3rds), kick, snare, hihat |
| `toto-africa` | 93 | 16 | fm-bell (ostinato), synth-bass, piano-grand (chords), kick, snare, hihat, shaker |
| `miles-so-what` | 136 | 16 | synth-bass, piano-felt (So-What chord), trumpet (head), sax (2nd voice), ride, hihat |
| `puente-ran-kan-kan` | 160 | 8 | piano-grand (montuno), trumpet, sax, synth-bass (tumbao), sidestick (clave), cowbell, conga (open), conga (slap) |
| `wonder-superstition` | 100 | 8 | clavinet (riff), clavinet (octave), synth-bass, kick, snare, hihat, trumpet, sax |
| `fingers-can-you-feel-it` | 122 | 16 | kick, clap, hihat, synth-bass, organ (pad lo), organ (pad hi) |
| `dre-g-thang` | 93 | 8 | fm-lead (whistle), synth-bass, piano-grand (comp), kick, snare, hihat |
| `may-strings-of-life` | 126 | 16 | piano-grand (stabs), violin-ensemble (stabs), organ (held), synth-bass, kick, clap, hihat |
| `marley-exodus` | 98 | 8 | bass-guitar, guitar-electric (skank), organ (bubble), kick, snare, hihat, trumpet, sax |
| `chic-good-times` | 112 | 16 | bass-guitar, guitar-electric (chuck), violin-ensemble (strings), fm-epiano, kick, hihat, clap |
| `jobim-ipanema` | 130 | 16 | guitar-nylon (comp), sax (melody), flute (answer), synth-bass, shaker, sidestick, piano-felt |
| `bbking-thrill-is-gone` | 98 | 12 | guitar-electric (lead), bass-guitar, violin-ensemble (strings), organ (comp), kick, snare, hihat |

(BPMs chosen inside the §1 genre bands. Stems chosen so `displayName` title-cases readably; if a display name reads poorly, switch to a stem→title map in `templates.go` — Task 6 note.)

- [ ] **Step 1: Replace `showcases()`** with the 15 `Showcase` literals, authoring each row's `Hit` list from the §5 blueprints (riffs, chord arpeggios, basslines, montunos, grooves, melodies). Keep velocities moderate for the no-clip constraint.

- [ ] **Step 2: Build the generator** to catch Go errors before regenerating:

Run: `cd src/go && ../../.tools/go/bin/go build ./cmd/gen-showcase`
Expected: builds clean.

---

## Task 5: Regenerate JSON & swap the template set

**Files:**
- Delete: `internal/assets/templates/{rock,hip-hop,pop,funk,salsa,house,techno,classical-canon,classical-ode,rock-anthem,pop-ballad,jazz-swing,salsa-mambo,funk-groove,house-piano,hiphop-boombap}.json`
- Create (generated): the 15 new `internal/assets/templates/<stem>.json`
- Modify: `internal/assets/templates.go` (`templateOrder` + doc comment)

- [ ] **Step 1: Regenerate** the 15 new JSON files:

Run: `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-showcase`
Expected: 15 new `<stem>.json` written under `internal/assets/templates/`.

- [ ] **Step 2: Delete the 16 old template JSONs** (the generator only writes the new stems; the old files must be removed so the embed set is exactly the 15):

Run: `cd src/go/internal/assets/templates && rm -f rock.json hip-hop.json pop.json funk.json salsa.json house.json techno.json classical-canon.json classical-ode.json rock-anthem.json pop-ballad.json jazz-swing.json salsa-mambo.json funk-groove.json house-piano.json hiphop-boombap.json`
Expected: only the 15 new stems remain. Verify: `ls` shows 15 files.

- [ ] **Step 3: Update `templateOrder`** in `templates.go` to the 15 stems in menu order (Classical → Bossa → Jazz → Blues → Funk → Disco → Rock → Reggae → Pop → Hip-Hop → House → Techno → Salsa):

```go
var templateOrder = []string{
	"bach-toccata", "vivaldi-spring", "mozart-k545",
	"jobim-ipanema", "miles-so-what", "bbking-thrill-is-gone",
	"wonder-superstition", "chic-good-times",
	"eagles-hotel-california", "marley-exodus", "toto-africa",
	"dre-g-thang", "fingers-can-you-feel-it", "may-strings-of-life",
	"puente-ran-kan-kan",
}
```

- [ ] **Step 4: Update the doc comment** above `templateOrder`/`Template` to drop the "original 7 hand-authored vs generated" split — all 15 are now generated by `cmd/gen-showcase`.

- [ ] **Step 5: Verify drift** (committed JSON == `build()` output):

Run: `cd src/go && ../../.tools/go/bin/go test ./cmd/gen-showcase/`
Expected: PASS (TestDrift green for all 15).

---

## Task 6: Update template guard tests & full verification

**Files:**
- Modify: `internal/assets/templates_test.go` (3 hardcoded lists)

- [ ] **Step 1: Update `TestTemplates_OrderedAndNonEmpty`** `wantGenres` to the 15 stems in the Task 5 order.

- [ ] **Step 2: Update `TestTemplates_DisplayDerivedFromStem`** `want` map — set each value to what `displayName(stem)` yields (e.g. `"bach-toccata": "Bach-Toccata"`, `"miles-so-what": "Miles-So-What"`). If any reads poorly, introduce a stem→display map in `templates.go` and update the test accordingly.

- [ ] **Step 3: Update `TestTemplates_BPMInGenreBand`** `band` map — one entry per new stem with a band that brackets its BPM (e.g. `"bach-toccata": {60,90}`, `"miles-so-what": {120,160}`, `"puente-ran-kan-kan": {140,180}`, …).

- [ ] **Step 4: Verify assets package:**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/`
Expected: PASS.

- [ ] **Step 5: Verify template audio quality (no clipping, valid instruments, tonal):**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run 'Template'`
Expected: PASS. If a template clips (aligned peak > 0.9), lower the loudest rows' `Volume` in `specs.go`, regenerate (Task 5 Step 1), re-run.

- [ ] **Step 6: Full fast suite:**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/... ./internal/audio/... ./cmd/gen-showcase/`
Expected: PASS.

- [ ] **Step 7: Build desktop + report for listening.**

Run: `cd src/go && ../../.tools/go/bin/go build ./...`
Then tell the user how to listen: `make run`, open the template/genre menu, load each of the 15 templates, and play. Collect notes for the collaborative tweak pass (circuits + the organ/clavinet/sax seeds).

---

## Self-Review notes

- **Spec coverage:** Tasks 1–3 cover the three new instruments (§3); Task 4 covers all 15 blueprints (§5); Tasks 5–6 cover the registration/replacement mechanics (§6) and the test contract (§7). The triplet/odd-meter risk (§8) is handled by selection (Mozart over Moonlight, all picks 4/4).
- **Type consistency:** instrument ids `organ`/`clavinet`/`sax`; recipe ids `synth-modular-{organ,clavinet,sax}`; all five touchpoints listed identically per instrument.
- **Open item carried to execution:** display-name readability (Task 6 Step 2) — fall back to a stem→display map if title-casing a stem reads poorly.
