# `snare-acoustic` — a reference-matched acoustic snare

**Date:** 2026-09-14
**Status:** design approved, ready for an implementation plan
**Reference:** `~/Downloads/soundreality-snare-drum-acoustic-sample-455287.mp3`

## Goal

Add a new snare instrument, `snare-acoustic`, synthesized to match an open,
undamped acoustic snare recording. The existing `snare` — its sound, its id, its
recipe, its templates and its goldens — is **untouched**.

## Decisions already taken

| Question | Decision |
|---|---|
| Roster placement | New id `snare-acoustic`. `snare` keeps its id and its sound. |
| DSP approach | **A** — a new C snare variant, modal, reusing the existing snare knobs. No new ABI. |
| Reference provenance | **No licence/catalogue work.** Local dev only; the file is tuned against in place. `snare-acoustic` will read `NO REF` in `refs families`, by choice. |

Note for the record: the current `snare` *does* already have a reference — it was
tuned to `579499__yenus__fat-snare-bottom-ramon` (see the tuning comment at
`src/c/modular_stages.c:1388` and `snare-freesound-alt` in the wavref catalogue).
That does not change anything here, since `snare` is being left alone.

## 1. Measured targets

Reference: 2.712 s, mono 44.1 kHz, onset at 26.3 ms, peak 0.985, crest factor 10.9.

Mode frequencies were measured twice by independent methods (peak-picking on a
0–600 ms windowed FFT, then narrowband complex demodulation) and agree exactly.

| mode | freq | ratio | peak level | decay 0–100 ms | decay 100–400 ms |
|---|---|---|---|---|---|
| 1 (thump) | 225 Hz | 1.00 | **1.00** | 35.7 /s | 2.2 /s |
| 2 | 355 Hz | 1.57 | 0.25 | 16.5 /s | 7.2 /s |
| 3 | 520 Hz | 2.31 | 0.29 | — (rises) | 4.5 /s |
| 4 | 673 Hz | 2.99 | 0.24 | 6.3 /s | 5.4 /s |
| 5 | 838 Hz | 3.72 | 0.29 | — (rises) | 7.2 /s |
| 6 | 1015 Hz | 4.50 | 0.07 | 2.6 /s | 7.1 /s |
| wires | 2–16 kHz | — | 0.12–0.15 | 16.5–19.1 /s | −40 dB by ~270 ms |

Energy distribution by window:

| window | 100–300 Hz | 300–2000 Hz | 2–8 kHz | 8–20 kHz |
|---|---|---|---|---|
| attack 0–60 ms | **61.5 %** | 26.8 % | 9.7 % | 1.9 % |
| tail 120–400 ms | 3.6 % | **95.8 %** | 0.5 % | 0.0 % |
| ring 400–900 ms | **59.6 %** | 40.3 % | 0.1 % | 0.0 % |

### 1.1 The two-stage fundamental — the defining feature

Mode 1 is **not** a single exponential. Measured envelope of the 225 Hz mode,
relative to its own peak:

| 5 ms | 25 ms | 50 ms | 100 ms | 200 ms | 400 ms | 700 ms | 1000 ms | 1400 ms |
|---|---|---|---|---|---|---|---|---|
| −4.4 | −1.2 | −10.4 | −24.3 | −32.3 | **−31.9** | −36.4 | −56.4 | −59.3 dB |

It falls fast (≈35 /s) for the first 100 ms, then **plateaus near −32 dB and hums
out to ~700 ms** before finally dying. This is why the 400–900 ms window is 60 %
low-band energy: the last half second is carried by the fundamental, not by the
ring. Physically this is the struck head damping quickly while the shell and the
enclosed air keep resonating at the same pitch.

Modelling mode 1 as one fast exponential produces a snare that is correct for
150 ms and hollow thereafter. The two-stage envelope is a **requirement**, not a
refinement.

Overall envelope: −13 dB @ 100 ms, −29 dB @ 400 ms, −37 dB @ 900 ms,
−47 dB @ 1200 ms, −71 dB @ 1600 ms.

## 2. DSP — `gen_snare_acoustic`

New static function in `src/c/modular_stages.c`, placed beside the existing
`gen_snare_main` / `gen_snare_rimshot` / `gen_snare_sidestick`, and structured
after `gen_kick_acoustic` (`modular_stages.c:396`), which is the established
precedent for an inharmonic modal drum bank that reuses its family's knobs.

Components:

1. **Modal head bank** — 6 struck sine partials at the measured ratios
   `{1.00, 1.57, 2.31, 2.99, 3.72, 4.50}` × `fundamental`, with fixed relative
   gains from the table above. All start at full amplitude at `t=0` (no swell),
   which produces the sharp natural transient.
2. **Two-stage mode 1** —
   `A_fast·exp(−tone_decay·t) + A_slow·exp(−tail_decay·t)`, with
   `A_slow/A_fast ≈ 0.025` (−32 dB) as a fixed internal constant. This is §1.1
   and it costs no new knob.
3. **Ring damping** — modes 2–6 decay at `tail_decay` with a small fixed
   per-mode spread (measured 4.5–7.2 /s) so the bank does not read as a tidy
   synth chord.
4. **Wires** — white noise through a 2–12 kHz band (measured wire energy spans
   2–16 kHz with the top octave ~5 dB down), decaying at `noise_decay`
   (target ≈17 /s), **amplitude-coupled to the instantaneous displacement of the
   head bank**. Real wires are driven by the resonant head, so coupling makes the
   buzz breathe with the hit instead of reading as an independent noise envelope
   pasted on top.
5. **Stick transient** — short band-limited burst, `exp(−350·t)`, scaled by
   `attack`.
6. **No saturation across the summed modes.** Final soft-limit only. This is the
   kick's hard-won lesson: `tanh`-ing summed *inharmonic* partials produces
   intermodulation roughness — the "creaky door" artifact that cost a debugging
   round on `kick-acoustic`.

## 3. Variant code — and a trap to remove first

`modular_gen_slot_snare` (`modular_stages.c:1536`) dispatches the `source==7`
voices and clamps `variant` to `0..2`. The new branch is `variant == 3`, with the
clamp widened to `0..3`.

**The trap:** `snareVariantSpecs` (`snare_modular_push.go:170`) already lists
`drum-clap` with `variant: 3`. That 3 is not a snare variant at all — it is an
undocumented *sentinel* consumed by `snarePushVoiceParams`:

```go
// Clap (variant 3) → source 8; the snare-ish trio → source 7.
if variant == 3 {
    out["gen1_source"] = 8
} else {
    out["gen1_source"] = 7
    out["gen1_snare_variant"] = variant
}
```

A new `drum-snare-acoustic` row with `variant: 3` would therefore be pushed
**as a clap** (source 8), silently, with `gen1_snare_variant` never emitted at
all. The C branch would never run and the bug would present as "the new snare
sounds like a clap."

**Fix, as part of this work:** give the `snareVariantSpecs` struct an explicit
`source` field (7 for the snare-ish variants, 8 for clap) and delete the
`variant == 3` magic. `drum-snare-acoustic` then takes `{source: 7, variant: 3}`
and clap takes `{source: 8, variant: 0}` without the two meanings sharing a
number. This is a small, contained improvement to the exact code being extended.

*Minimal alternative if that refactor is unwanted:* give the acoustic snare
`variant: 4` and leave the sentinel alone. Rejected as the default because it
bakes the collision in permanently and leaves variant 3 meaning two things.

`snare_variant` bounds in `modular_recipe.go:247` widen `{0, 2}` → `{0, 3}`.

## 4. Knob mapping — no new ABI

All ten existing snare knobs carry real meaning in the modal voice, so the C
struct (`modular.h:235-244`), `synth_param_schema.go`, the WASM param block and
every ABI drift guard stay untouched. Per-variant `kp_get` NaN→literal fallbacks
are how a variant declares its own defaults.

| ABI knob | Acoustic label | Default | Source |
|---|---|---|---|
| `fundamental` (`voice_freq_hz`) | Drum Pitch | 225 Hz | measured mode 1 |
| `snare_tone2_freq` | Ring Pitch | 355 Hz | measured mode 2 |
| `snare_tone_decay` | Thump Decay | 36 | mode 1 fast stage |
| `snare_tail_decay` | Ring Decay | 5 | modes 2–6 |
| `snare_noise_decay` | Wire Decay | 17 | 2–16 kHz band |
| `snare_noise_tune` | Wire Tone | 1.0 | — |
| `snare_tone_mix` | Thump Level | 1.0 | mode 1 peak |
| `snare_noise_mix` | Ring Level | 0.3 | modes 2–6 mean |
| `snare_wire_mix` | Wires | 0.15 | wire band peak |
| `snare_attack` | Stick | 1.0 | — |
| `snare_wave` | (tone osc wave) | 0 (sine) | — |

Defaults are starting points from the measurements; the tuning harness and the
ear gate settle the final values. `snareFamilyParamDefs` gets a
`drum-snare-acoustic` row whose **labels** describe this voice ("Ring Level", not
"Noise Level") while the underlying ABI names stay identical.

**Duration:** `DurationSec` 1.4 s (vs `snare`'s 1.0), `Beats` 0.6. The ring is
only −37 dB at 900 ms and −47 dB at 1200 ms; a 1.0 s buffer would truncate
precisely the feature being built.

## 5. Wiring — table appends

| File | Change |
|---|---|
| `src/c/modular_stages.c` | `gen_snare_acoustic` + `variant == 3` branch + clamp `0..3` |
| `src/go/internal/audio/modular_recipe.go` | `snare_variant` bounds `{0,3}` |
| `src/go/internal/audio/snare_modular_push.go` | `source` field on the spec struct (§3) + `drum-snare-acoustic` row |
| `src/go/internal/audio/drum_gen_defaults.go` | `"snare"` family variant-3 defaults row |
| `src/go/internal/audio/synth_recipe_wired.go` | `snareFamilySpec` row + `recipeWiredParamDefs` entry |
| `src/go/internal/audio/synth_recipe.go` | `builtinRecipeDescriptors` `{ID:"drum-snare-acoustic", Display:"Acoustic Snare", Category:"drum"}` + `builtinInstrumentRecipeBindings` `"snare-acoustic": "drum-snare-acoustic"` |
| `src/go/internal/audio/snare_modular_native.go` | `snareAcousticVoiceParams` + `renderSnareAcousticVoice` |
| `src/go/internal/audio/engine_instruments.go` | `"snare-acoustic"` `CVariantInstrument{Beats: 0.6}`, no `DefaultFX` |
| `src/go/internal/audio/config.go`, `config_wasm.go` | `{ID:"snare-acoustic", DurationSec:1.4, Amplitude:0.8}` |
| `src/js/audio.js` | `render_modular` map entry + `RENDER_INFO` `{seconds:1.4, amp:0.8, paramBlock:'modular'}` |
| regen | `instrument_ids_gen.go` (`go generate ./internal/audio/`) |

`snare-acoustic` is percussion, so it is **not** added to `melodicRecipeIDs`.
`cmd/gen-fundamentals` will pick it up and decide its note name through
`UsableAsPitch`; given a strong 225 Hz mode it will likely resolve to a real
note, which is correct and needs no intervention.

## 6. Verification

**Ear is the gate.** Nothing is done until the user says so. The deliverable for
judgement is an A/B pair: the reference vs `snare-acoustic` rendered at matched
loudness, via `make ab INST=snare-acoustic` (or a written pair if the missing
catalogue entry blocks `ab`).

1. **Tuning harness** — `snare_acoustic_tune_test.go`, build-tagged
   `!test && !js` so it runs under `make test-real` against the real C DSP.
   Sweeps the knobs against a distance measure over the §1 targets;
   `WRITE_WAV=1` emits audition pairs. Measurement helpers stay **test-local** —
   production `AnalyzeSnare`-style analyzers were deleted once already as dead
   code.
2. **Guard** — `TestSnareAcousticIsRingingModalSnare`: mode ratios present within
   tolerance, **the fundamental still audible at 400 ms within 6 dB of its
   200 ms level** (the §1.1 plateau — the thing most likely to be lost in a
   later "simplification"), ring outlasts thump, wire band present and gone by
   ~300 ms, crest factor in range. Same anti-watered-down role as
   `TestKickAcousticIsTightOrganicKick`.
3. **Regen chain, in this order** (this trips every fresh instrument add):
   `TestSynthGoldenByteIdentity` fails "no pinned golden" → capture the new
   golden → `make measure-loudness` → **re-capture the entire `synthGoldens`
   map**, because loudness shifts every non-capped instrument sub-dB and moves
   all goldens.
4. **`make test-real`** — byte-identity, ABI drift, and the `snare` regression
   surface (its goldens must be **bit-identical**; if any `snare` golden moves,
   the change leaked and must be reverted).
5. **Browser** — `wasm_bridge_smoke.browser.test.js` for the new id, and
   `xplat_audio_compare.browser.test.js` for native/WASM render identity.
6. **Screenshot** — one capture of the instrument menu showing the new roster
   row, per the mandatory visual-verification rule, opened and inspected.

## 7. Out of scope

- Any change to `snare`, its recipe, its templates, or its goldens.
- Reference catalogue / licence / provenance entries (explicitly dropped).
- Promoting `snare-acoustic` to the default `snare` id — a separate decision to
  be made *after* hearing it, if at all.
- A full nonlinear membrane + wire-collision physical model. Held in reserve:
  the escalation trigger is the ear finding the body right but the **wires
  specifically** dead. This reference is tonal-ring-forward rather than
  buzz-forward, so the coupled-noise approximation in §2.4 should suffice.

## 8. Risks

| Risk | Mitigation |
|---|---|
| Clap sentinel collision (§3) | Explicit `source` field; caught early by a push-level assertion |
| Golden churn after `measure-loudness` | Full re-capture is part of the plan, not a surprise |
| Wire realism falls short | Escalation path to the nonlinear model is pre-agreed (§7) |
| Modal bank sounds like a tuned chord | Fixed per-mode decay spread (§2.3); the guard checks ratios, the ear checks musicality |
| Two-stage envelope lost in later edits | Explicitly asserted by the §6.2 guard |

---

## 9. As built (2026-09-14)

Implemented and verified. Deviations from the design above, and why:

| Design said | As built | Why |
|---|---|---|
| Shell/air term decays at `tail_decay` | Decays at a fixed **1.2 /s**, plus a `(t/0.95s)^6` gate | §1.1's plateau is a RATE, not just a level. At `tail_decay` (~5 /s) the hum died with the ring and the plateau vanished. The gate ends the hum by ~1 s, as measured, instead of ringing to the end of the buffer. |
| Ring level 0.3, wires 0.15 | Ring **0.6**, wires **1.3** | Both defaults were derived from PEAK-amplitude ratios, which badly under-read a band-limited source's energy. Measured against the reference: wires landed at 0.2% of attack energy vs the target 9.8%. |
| One clap sentinel to remove (`snare_modular_push.go`) | **Two** — the same `variant == 3` test also lived in `snare_modular_binding.go`, and `snarePostConfigFor` keyed the clap's macro set off `default:` | Missing the third cost a debugging round: clap's post-config silently became the base snare's, and `TestClapModularMatchesOracle` drifted. All three now key on the explicit `source`. |
| `Beats: 0.6` (sibling snares) | **`Beats: 2.8`** | `Beats`, not `config.go`'s `DurationSec`, sizes a `CVariantInstrument`'s one-shot render. At 0.6 the buffer ended at 300 ms and truncated the ring — the first guard-test run failed on exactly this. |
| Reference not catalogued | Catalogued as `snare-acoustic-local`, licence `local-dev` | `make ab` resolves its reference THROUGH the catalogue, so "no catalogue entry" and "usable with the A/B system" are mutually exclusive. The licence requirement was dropped instead (see below). |

Two tool changes were needed to get there, both small and general:

- **`refs adopt -license` is now optional** (`adopt.go`, `Makefile`), defaulting to `local-dev`. A local adoption is never committed and its audio stays outside the tree, so the licence was documentary all along; requiring it only blocked local dev. `TestRunAdopt_RequiresALicence` became `TestRunAdopt_LicenceIsOptionalAndDefaults`.
- **`refs adopt -f0` pins the measured fundamental.** YIN measured this reference at **113.8 Hz — exactly half** its true 225 Hz, the documented octave artifact on struck sources. Everything downstream trusts `Measured.F0Hz`, and `refs compare` RENDERS THE SYNTH AT IT, so the first A/B auditioned the instrument an octave low and every metric drawn from it was meaningless. `-f0 225` fixes it at the source.

Verification actually run:

- `TestSnareAcousticIsRingingModalSnare` — passes; asserts the plateau (≤8 dB drop 200→400 ms), the struck first stage, the inharmonic ring, the hum's collapse by 1.2 s, and wires present-then-gone.
- Full native `./internal/audio` suite + `./cmd/gen-fundamentals` — green.
- `-tags test` fast path (`./internal/...`, `./cmd/...`) — green except two PRE-EXISTING failures unrelated to this work (see §10).
- `snare` is **bit-identical**: its pinned golden, and `snare-lofi` / `snare-ghost` / `snare-crack` / `sidestick` / `clap` / `kick`, are unchanged; `gen_snare_main`, `gen_snare_rimshot` and `gen_snare_sidestick` are byte-identical to HEAD.
- Screenshot `instrument_menu_snare_acoustic` captured and inspected — "Snare Acoustic" renders in the Snare category.
- Measured match against the reference (level-matched A/B renders):

| | attack 100-300 / 300-2k / 2-8k | tail 300-2k | 225 Hz at 200/400/700 ms | overall at 100/400/900 ms |
|---|---|---|---|---|
| REAL | 61.2 / 27.0 / 9.8 % | 95.8 % | −32.3 / −31.9 / −36.4 dB | −13.4 / −29.5 / −37.0 dB |
| SYNTH | 57.3 / 27.4 / 8.4 % | 94.4 % | −28.2 / −30.7 / −35.1 dB | −14.6 / −27.2 / −43.4 dB |

**The ear is still the gate.** Listen with `make ab INST=snare-acoustic`.

## 10. Pre-existing failures found along the way (NOT caused by this work)

- **`TestWasmAssetSizeBudget`** — `src/js/main.wasm` is a stale unstripped local dev build (27.23 MiB raw vs a 26.50 MiB budget), timestamped before this work began. `make wasm` regenerates it.
- **Global-search instrument labels truncate.** In the instrument menu's categories/global search, querying `snare` renders EVERY snare variant's row as just "Snare", though `InstrumentDisplayName` returns distinct names ("Snare Lofi", "Snare Ghost", "Snare Crack", "Snare Acoustic"). Querying `acoustic` renders "Snare Acoustic" in full, so the label appears to be cut at the end of the match. Predates this work (the other three variants already existed) and is untouched here.
- **`slotGain` in `modular_stages.c`** is uncommitted working-tree work that was never built into `build/libdrums.a`. It is live now that the lib has been rebuilt.

---

# Part 2 — Config-first snare (2026-09-15)

Follow-on work under a new standing constraint from the user: *"always use the
synth engine by providing CONFIG-only changes (not hardcoded recipes or synth
configs) … All synths should be able to be updated in real-time by the user in
the synth tab."*

## 11. Where the codebase already stood

92 of 134 instruments were already config-first `modularInstrumentDefs` rows,
whose struct comment states the principle outright: *"Seed — THE CONFIG — the
only thing that defines the sound."* The 42 that were not are the legacy
drum/FM families. `kick-acoustic` is the model: a pure seed that even selects
its C variant from config (`gen1_kick_variant: 7`).

## 12. What this part changed

| Change | Why |
|---|---|
| **`snare_enabled`** C ABI field (`modular.h` tail) + gate in `modular_gen_slot_snare` | Mirror of `kick_enabled`. Without it a config-seed snare carries its whole voice in HIDDEN params and the user cannot touch it in the Synth tab. |
| **`snareStageMeta`** (`modular_recipe.go`) | Surfaces the slot-1 snare knobs as a visible stage with friendly labels and a live **Variant** dropdown (Snare / Rimshot / Sidestick / Acoustic). *(Superseded in §25 by the family-stage registry, which subsumes it.)* |
| UI routing: group `"snare"` → VOICE, `schemaUsesSnareStage`, `synthSnareParamHidden` | Exact mirror of the kick's gating so the knobs appear only on snare instruments. `snareActive` is threaded through `synthParamHiddenInContext`, the single source of truth. *(Superseded in §25.)* |
| `synthCategoryTableFor` now also keys on `schemaUsesSnareStage` | A config-first drum is identified by its gen-slot SOURCE, not by a bespoke family-knob name. Without this the snare was classified **melodic** and its voice knobs were unreachable. |
| `gen1_snare_*` added to the percussion macro table | So every knob is claimed by exactly one category card (the discipline test enforces this). |
| **`snare-fat`** — new instrument, `snareFatSeed` | Preserves today's sound verbatim under its own id. |
| **`snare`** — migrated to `snareTunedSeed`, retuned | Config-first and reference-matched. `Snare{}`, the `config.go`/`config_wasm.go` rows and the hand-written `drum-snare` binding are gone; `audio.js`'s hand-written entries are gone (now derived). |

`drum-snare` itself is UNTOUCHED and still backs `snare-lofi` / `snare-ghost`,
so its legacy-oracle byte-parity survives intact.

## 13. Proof the config path is faithful

`snare-fat` (a pure seed) vs the old baked voice: **peak-normalized max sample
difference 1.5e-07** — float32 rounding. The only raw difference was amplitude,
because the new id was not yet in the loudness table.

Getting there found a trap worth recording: a seed MUST disable the generic
stages (`osc_enabled/env_enabled/filter_enabled/drive_enabled = 0`, via the new
`snareStageBase()` mirroring `kickStageBase()`). Omitting them let the amp
envelope run over the voice and moved **0.84 of full scale**.

## 14. The retune

Reference `snare-freesound-alt` (the fat-bottom snare the old tuning already
targeted). `snare-vcsl`, also catalogued for `snare`, is **unusable as ground
truth**: zero energy above 2 kHz in every window — it is a snares-OFF hit
(`Snare2_HitNS`).

| | attack 100-300 / 300-2k / 2-8k | body 2-8k | tail 300-2k / 2-8k | env @100 / @200 / @400 ms | crest |
|---|---|---|---|---|---|
| REFERENCE | 71.5 / 16.2 / 11.7 % | 59.2 % | 28.6 / 62.6 % | −25.1 / −38.1 / −74.0 dB | 13.4 |
| OLD (`snare-fat`) | 73.1 / 9.8 / 13.7 % | 44.9 % | 22.6 / 31.1 % | −20.6 / −32.7 / **−41.9** dB | 14.3 |
| NEW (`snare`) | 70.3 / 14.1 / 13.0 % | 57.5 % | 27.8 / 57.7 % | −24.8 / −41.3 / −63.4 dB | 15.1 |

The old sound's attack was already right; its tail ran ~32 dB too hot and was
full of low mush where the reference is wires. Seed changes: `voice_freq_hz`
186→199, `tail_d` 10→15, `noise_d` 7→15, `tune` 0.55→0.7, `noise_m` 0.56→0.45,
`wire_m` 0.8→0.68, `tone2` 280→298.

## 15. Still outstanding

- **The rest of the snare family is NOT converted.** `snare-lofi`,
  `snare-ghost`, `snare-crack`, `rimshot`, `sidestick` and the clap family are
  still wired-recipe instruments on `drum-snare` / `drum-snare-rimshot` /
  `drum-clap`. `snare-acoustic` is likewise still a hand-written recipe.
- **`make ab INST=snare` renders an octave low.** `snare-freesound-alt` stores
  `f0 = 113 Hz` / `semis −12` and `snare-vcsl` stores 84 Hz / −17, both octave
  artifacts on a struck source, and `refs compare` renders the synth AT the
  stored pitch. `refs adopt -f0` cannot fix a COMMITTED ref. Options: a
  `pitch_override` field on `Ref` that survives re-measure (clean), or editing
  the stored `measured` block (fast, but a re-fetch overwrites it).

## 16. Snare audition candidates retired (2026-09-20)

The `snare` family was removed from `internal/audio/testdata/audition_candidates.json`.
It was stale in THREE independent ways, any one of which invalidated it:

1. **Dead overrides.** Its keys (`snare_noise_tune`, `snare_tail_decay`, …) are
   `drum-snare` parameters; `snare` renders through `synth-modular-snare`
   (`gen1_snare_*`) since Part 2. An unknown param is silently ignored, so the
   candidates rendered BYTE-IDENTICAL to the base — an audition would have
   played the same clip as both A and B. Caught by
   `TestShippedCandidates_AreLive`.
2. **Unusable reference.** It auditioned against `snare-vcsl`
   (`Snare2_HitNS_v2` — NS = no snares, 0.0% energy above 2 kHz). Wires-off
   ground truth for a wires-on snare.
3. **Superseded base.** Its base values are the pre-retune voice, now frozen as
   `snare-fat`.

The family's question is already answered: the candidates and the §14 retune
independently diagnosed the same faults and moved four of five knobs in the
SAME direction (brighter wire band, faster tail, faster noise decay, less low
noise) — the retune simply got there against a real wires-on reference. The
recorded session survives in `testdata/ear_sessions.jsonl.snare.draft.json`.

STILL OPEN (the root cause, untouched): `snare-vcsl` remains catalogued as a
reference for `snare` AND `snare-ghost`. Retiring the candidates removes the
symptom; the next person to audition ghost snares hits the same wires-off clip.

---

# Part 3 — the snare family is config-first (2026-09-20)

Every snare-family instrument is now a `modularInstrumentDefs` seed. Legacy
instruments remaining in the roster: **31 of 135** (was 41).

Converted: `snare-lofi`, `snare-ghost`, `snare-crack`, `rimshot`, `sidestick`,
`clap`, `clap-bright`, `clap-lofi`, `clap-tight`, `snare-acoustic`
(plus `snare` / `snare-fat` in Part 2).

## 17. What unblocked it: two DATA fields

`ModularInstrumentDef` gained `DefaultFX []EffectSlot` and `GateTail float64`.
That was the whole blocker. `CVariantInstrument` expresses the tail gate as a
`Post` FUNC, but **every `Post` in the engine is exactly `gateTail(buf, X)`** —
so the func was always just a number, and keeping it as one is what let these
instruments become pure config.

## 18. Byte-identity: what actually changes, and why

A converted instrument is NOT byte-identical, by a measured
**1.192e-07 = −136.5 dB below peak**. The cause is exact and general:

- the legacy binding passed **NaN**, so C's `kp_get` used its **double** literal;
- a seed carries an **explicit value**, which crosses the ABI as `C.float`
  (**float32**), so `0.55` becomes `0.550000011920929`.

Proof: re-render a seed with those knobs set to NaN and the result is
byte-identical (`maxdiff = 0`). Literals that ARE float32-exact (280, 1050, 40,
200, 1.0, 2.0) produce no delta at all — which is why `sidestick` and
`snare-lofi` converted byte-identically while `snare-ghost` and `clap` did not.

This is the accepted, one-time cost per converted instrument (user decision,
2026-09-20). The alternative — seeding NaN — preserves bytes but leaves the knob
with no displayed value in the Synth tab, defeating the point of the migration.

⚠️ **Measuring this correctly matters.** Comparing a config render against a raw
probe voice reports deltas of 0.2–1.66, because `RenderInstrumentOneShotRaw`
applies the loudness amplitude and a bare probe does not. Compare both sides
through the SAME path (`CVariantInstrument.NewVoice`) or the number is fiction.

## 19. Three real behaviour changes (NOT stale test expectations)

1. **Browser one-shot lengths.** `clap-bright/lofi/tight` and
   `snare-lofi/ghost/crack` never had a Go config entry — their durations lived
   ONLY in `audio.js`. Taking `DurationSec` from the base instrument's Go config
   would have silently changed six voices in the browser (e.g. `clap-tight`
   0.3 s → 0.5 s), because the generated table is `Object.assign`-ed last and
   wins. `DurationSec` now carries the exact previous values; the twenty dead
   hand-written `audio.js` entries were removed.
2. **Claps must not declare a pitch.** `clapSeed` initially carried
   `voice_freq_hz: 500` (copied from the legacy spec, where it existed only for
   ABI uniformity — the source==8 voice ignores it). Declaring it made the
   fundamentals generator resolve a NOTE NAME for every clap. Removed; goldens
   unmoved, which confirms the voice really does ignore it.
3. **`snare-ghost` and `clap-tight` are now level-matched** (+1.9 dB, 0.8 → 1.0),
   and `snare-crack` measures its own loudness instead of borrowing its base
   (−0.8 dB). They previously fell through to `DefaultAmplitude` by ACCIDENT —
   having no config entry meant no loudness was ever measured for them. As
   config-first instruments they have entries, so the loudness pass sees them.
   Seven unrelated instruments moved ~0.3 dB from the renormalisation.

## 20. Recipes: one deleted, four kept on purpose

`drum-snare-acoustic` was **deleted outright** — it never shipped (created
uncommitted this month), so nothing can reference it. Its wired ParamDefs,
variant-spec row, baked voice, render func and legacy-oracle exclusion went with
it.

`drum-snare`, `drum-snare-rimshot`, `drum-snare-sidestick` and `drum-clap` are
orphaned (no instrument binds them) but **kept**, recorded in one
`legacyCompatRecipes` map with a per-entry reason: a project exported before the
migration names its recipe in the file, and import must resolve it. They may
only be deleted once a project migration rewrites the old ids on load.

THREE separate tests had duplicated the same "every recipe has an instrument"
check; they now share `IsLegacyCompatRecipe` instead of three copies.

---

# Part 4 — kick family config-first + the POST-stage bug (2026-09-20)

**26 of 135 legacy instruments remain** (was 31). Converted: `kick`,
`kick-deep`, `kick-lofi`, `kick-snap`, `kick-tight`.

## 21. Seeds are GENERATED from the C source, not transcribed

Two throwaway tools did the work and should be rebuilt rather than re-derived:

- a **C-literal extractor** that walks `modular_stages.c`, maps each family's
  `variant -> gen_<fam>_*` function via its slot dispatcher, and reads every
  `kp_get(p->FIELD[k], LITERAL)` out of that function's body;
- a **seed generator** that emits Go source from those literals plus the
  variant/fundamental in the existing `*VariantSpecs` tables.

This is not a convenience. The Go spec tables list only the WIRED knobs; the
kick additionally reads `attack`, `fade` and `sat`, which the legacy binding
passed as **NaN** so C used its own literal. A seed leaving them at the schema
identity sets `gen_kick_sat = 0`, which **silences the voice outright**. The
extractor makes that class of omission impossible.

## 22. The POST stage must be in the seed (a real regression, caught by tests)

The first cut of every seed omitted `post_enabled`. The legacy binding sets
`PostEnabled = 1` plus a per-family config, so leaving it at identity `0`
skipped the whole macro stage and left **`post_pitch` / `post_decay` /
`post_drive` dead on all 17 converted instruments** — snare family included.
`TestSynthSavePersists_RenderUnchangedAfterSave` caught it
("pitch=+12 produced an identical buffer; knob has no effect").

Each seed now carries its family's POST config: `post_enabled`,
`post_decay_rate`, the `*_on` gates, and **`post_order`** (the base kick applies
drive BEFORE body, `PostOrder = 1`). Enabling POST moved **no golden** — at
identity the stage is an exact no-op, which is why the omission was invisible
until a knob was actually turned.

⚠️ `dnb-kick` and the other PRE-EXISTING modular instruments still ship with
this dead POST stage. The same one-line addition fixes them, at the cost of
re-pinning their goldens.

## 23. `pitch` vs `post_pitch` — one knob, two vocabularies

A config recipe's schema still CONTAINS a generic `pitch` param, but it is
**inert**: only the wired binding translates `pitch` into the POST field.
Presence therefore cannot tell a caller which name works — the choice has to be
made on the recipe's KIND (`transposeKnobFor` branches on whether the schema
carries `gen1_source`). Every family converted adds instruments to the `post_*`
dialect while the rest stay on the `pitch` dialect. Collapsing the two is part
of the StageDef work in §25.

## 24. The liveness ratchet's advice would have re-broken it

After the POST fix, `TestSeedParamsAreLiveInTheirOwnInstrumentContext` flagged
all 85 POST params as inert and advises "Remove it from the seed (preferred)" —
which would delete `post_enabled` and re-introduce §22 exactly. They read as
inert because the stage is an exact no-op AT IDENTITY, which is the point of the
identity; the effect appears only once a macro moves. They are baselined with
that reason recorded in `known_dead_seed_params.txt`.

## 25. RESOLVED: the family-stage registry

Before this, only **kick** and **snare** had a visible stage (`kickStageMeta`,
`snareStageMeta`). Tom, cymbal, bass and FM reached the user ONLY through the
wired-recipe path that conversion deletes, so converting them as-is would have
left ~23 instruments uneditable — the opposite of the migration's purpose. Hand-
writing four more stage metas meant repeating a TWELVE-registry pattern (ABI,
schema globals, identity map, ParamDef, stage meta, routing, hidden gate,
category table, macro claims, focus renderer, glosses, stage-matrix rule,
genbank exemption) four more times. Three of those twelve silently disagreed
the last time a family was added.

The exit — now built — is `internal/audio/modular_family_stages.go`: one
registry describing each family as DATA, with the schema generator and every UI
consumer deriving from it.

```go
type ModularFamilyStage struct {
    Group      string  // ParamDef.Group the family's knobs carry
    Source     float64 // the C gen_source that selects this voice
    Enable     string  // its enable gate, "" when the family has none
    Percussive bool    // picks the percussion macro-category table
}
```

with `modularStageKnobs` — one row per gen-slot field — carrying that field's
group, label, enum and the macro card that claims it.

### What it replaced

| was | now |
|---|---|
| `kickStageMeta` + `snareStageMeta` | one `modularStageKnobs` table |
| 10 near-identical ParamDef emission loops | one `appendGenSlotDefs` helper |
| `schemaUsesKickStage` + `schemaUsesSnareStage` | one `activeFamilyStage` |
| `synthKickParamHidden` + `synthSnareParamHidden` | one `synthFamilyParamHidden` |
| section router branching on `"kick"` then `"snare"` | `audio.FamilyStageGroupOfParam` |
| classifier naming both stage checks | `fam.Percussive` from the registry |
| genbank schema guard hardcoding `gen1_kick_`/`gen1_snare_` | `FamilyStageGroupOfParam` |

Adding a family is now **one row** plus its knob rows. Tom, cymbal and FM gained
visible stages for free (groups: kick 18, snare 11, tom 7, cymbal 7, fm 30),
with labels reused verbatim from the wired recipes so the user-facing vocabulary
and its plain-English glosses are unchanged.

`kickModeMeta` turned up unused once the loops collapsed — it would have
silently dropped the modal kick's **Mode Split / Mode Body / Mode Ring** labels
and the acoustic kick's **Reverb**. Those are registry rows now.

### The one trap: a Group is not a unique key

`"fm"` names TWO different things — the modular FM **stage** (`fm_algorithm`,
`fm_op1_ratio`, appended to every migrated recipe) and the gen-bank FM
**family** (`gen1_fm_r1`). Routing on `def.Group` alone swept the FM stage's own
knobs into the VOICE section, which left the FM stage holding nothing but its
enable toggle, so the pruner deleted it and the Synth tab lost the FM stage
entirely (`TestSynthTab_ModularSectionsHaveEnablePills`,
`TestSynthTab_EmptyVoicePruned`).

The gen-bank NAME is unambiguous, so it is the key:
`audio.FamilyStageGroupOfParam(name)` matches `gen<N>_<field>` against
`modularStageKnobs`. There is deliberately no `IsFamilyStageGroup(group)` to
reach for instead — it was written, it caused exactly this, and it is deleted.

### ABI order is unchanged

ParamDef order is pinned by index, so the collapse was verified def-by-def
against the schema, with `TestSynthGoldenByteIdentity` and the legacy oracle
still passing unmoved.

## 26. The migration finished (2026-09-21)

With the registry in place the remaining families were mechanical. Final state:

| family | instruments | gen source | delta vs the legacy recipe |
|---|---|---|---|
| kick | 5 | 5 | -135.0 … bit-identical |
| snare / clap | 12 | 7 / 8 | -139.7 … bit-identical |
| tom | 3 | 6 | **-144.5 dB** |
| cymbal (hi-hats, cowbells, shaker, ride, crash) | 10 | 9 | -141.6 … -144.0 dB |
| FM | 10 | 10 | **bit-identical, all five presets** |

`TestConfigFirstSeedsRenderTheLegacySound` pins all 23 recipe→instrument pairs
by rendering each retired recipe through `builtinFamilyRenderers` and diffing it
against the seed. Nothing else asserted this: once a recipe is kept only for
loading old projects, the claim "this seed IS that recipe's sound" had no guard.

The seeds were NOT derived from the C source. `tomVariantSpecs`,
`cymbalVariantSpecs` and `fmVariantSpecs` already tabulate every `kp_get`
literal in Go, pinned equal to the C by the at-default byte-parity contract, and
`<family>PostConfigFor` spells out each variant's POST wiring. The C-literal
extractor built for the snare was never needed again.

Seven conversions are bit-identical rather than one-ULP: those recipes wire
every knob explicitly, so neither side ever falls back to a C `double` literal
and there is no float32 substitution to make.

### What is left

`sub-bass` / `sub-bass-drive` only. Gen source 4 has no dedicated gen-slot
family block — it drives the generic analytic fields rather than a
`gen1_<family>_*` set — so there is nothing for a seed to configure and no
visible Synth-tab stage. Closing it is a C-side change, not a config one.
`modular` itself stays seedless by design (it plays the schema identity).

### Three things the migration surfaced

**A stale generated table.** `instrument_ids_gen.go` had not been regenerated
since the kick/snare/clap conversion. `ResetInstruments` seeds `instOrder`
straight from `BuiltinInstrumentIDs`, so it is the order the instrument menu
lists within a category — and it was the order `NewDrumView` picked the default
first row from. `TestInstrumentIDsGenUpToDate` now re-runs the generator over a
COPY of its inputs (running it in place would rewrite the file under test and
always pass) and was verified to fail on the exact stale file that was in tree.

**A second copy of the loudness numbers.** `TestVariantsBorrowTheirBaseLoudness`
hardcoded seventeen measured amplitudes, so re-measuring the roster staled every
one of them. Its own comment said the thing it meant to pin was the RELATIONSHIP,
so it derives from `loudnessAliases` now and encodes the real rule: an own
measurement wins, else the base's, never `DefaultAmplitude`.

**A renamed param that silently deleted two note names.** `cmd/gen-fundamentals`
prefers a DECLARED base frequency over a measured one, reading a hand-written
list `{fm_base_freq, fundamental, base_freq}` — and `instrument_fundamentals_test`
kept a second copy of that list. The FM seeds respell `fm_base_freq` as the
gen-bank `gen1_fm_base`, so the declared value vanished from both, the generator
fell back to measurement, and **fm-bell and fm-pluck dropped from 440 Hz and
196 Hz to no note name at all** (their spectra do not pass `UsableAsPitch`).
Both lists are now one shared `baseFrequencyParamNames`, and with the rename in
it every fundamental regenerates unchanged.

The blast radius of a roster change is worth stating plainly: the loudness
target is roster-derived, so measuring 10 newly-eligible variants moved every
other instrument's amplitude by ~0.3% (~0.03 dB) and re-pinned 96 of 135 synth
goldens. Inaudible, but it is why the regen chain is
`make measure-loudness` -> re-capture `synthGoldens` -> `make gen-fundamentals`,
in that order.

