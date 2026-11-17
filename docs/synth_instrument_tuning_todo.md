# Synth Instrument Tuning — TODO / Status

Living checklist of every **modular synth instrument** and how well its sound matches a real instrument. The goal: progressively tune each against a reference recording, or consciously **skip / delete** the ones that don't mimic a specific acoustic instrument.

**Methodology:** see [`AGENTS.synth.md`](../AGENTS.synth.md) § "Tuning a Melodic Instrument from a Reference WAV" (apples-to-apples FFT matching) and § "Digital-Waveguide Physical Models". Per-instrument recipes: [`AGENTS.synth.instruments.md`](../AGENTS.synth.instruments.md).

**⚠️ PITCH-MATCH GATE** (learned the hard way): always render the synth at the reference's **measured fundamental** (`-pitch = 12·log2(f0_ref/220)`), never the filename — and verify `f0_synth ≈ f0_ref` within cents before comparing. A file named `..._g2` had its fundamental at G5.

Legend: ✅ tuned & user-approved · 🎵 intentional saved sound · 🎛️ variant needs re-sync · ⬜ needs tuning (reference required) · 🔌 skip candidate (not a specific acoustic instrument)

---

## ✅ Tuned & user-approved (reference-matched, 2026-06)

| Instrument | Approach | Reference |
|---|---|---|
| `violin` | Bowed-string waveguide (`osc_type:7`) + violin body | mtg violin-d5 — "amazing" |
| `cello` | Bowed-string waveguide (`osc_type:7`) + cello body (`body_model:3`) | xserra cello-d2 — "amazing" |
| `oboe` | Reed waveguide (`osc_type:9`, STK clarinet config) | acclivity oboe-a440 — "great" |
| `flute` | KS blown-tube + air + pitch-vibrato (anti-organ) | sgossner flute-e5 — "better" |
| `trumpet` | Additive brass formant (H3-dominant) + crescendo bloom | sgossner trumpet — "better" |
| `french-horn` | Additive H2-dominant ("covered" tone) | henkonen fhorn — "awesome" |
| `guitar-nylon` | Karplus-Strong + warm body (`body_model:2`, low-Q upper) | kyster 2-oct-c — "better" |
| `guitar-steel` | Karplus-Strong + bright steel body (`body_model:4`, high-Q ring) | (same family) — happy |
| `harp` | Karplus-Strong, bright + long ring | daphne celtic-harp (G5!) — "great" |
| `sax` | Hybrid additive + formant-filtered saw (sines pin H1-H4+H6; saw→BP@820 + saw→BP@2050 formants; breath @1.2 kHz) | mtg sax-baritone (TRUE f0 122.8 Hz/B2 — earlier "C2"/"C3" readings were octave artifacts) — matched on all fixed-grid metrics 2026-07-04; user ear-approved: "sounds great" |
| `piano-grand` | Clean additive struck-string (sine + decaying partials, exp decay) | pinkyfinger piano-g (G5!) — noise removed, pure note |
| `piano-felt` | Clean darker/softer struck-string (same approach, duller) | sibling of grand — pure, mellow |

## 🎵 Intentional saved sounds (final, not reference-matched by design)

| Instrument | Notes |
|---|---|
| `organ-church` | "Church Organ" — additive happy-accident from cello tuning. Keep. |
| `scifi-lead` | "Sci-Fi Lead" — subtractive-saw happy-accident. Keep. |
| `guitar-steel-warm` | Mellower steel (steel body, softer pluck) — derived from tuned steel. OK. |

---

## ✅ Variants re-synced (2026-06-27)

`oboe-full`→reed waveguide · `trumpet-mellow`→softer additive formant · `french-horn-loud`→louder additive H2-dominant. (cello-warm/violin-ensemble/flute-breathy/guitar-nylon-bright still additive, lower priority.)

## 🎛️ Variants needing re-sync (remaining) (base instrument moved to a NEW model; these still use the OLD approach)

| Instrument | Current | Action |
|---|---|---|
| `oboe-full` | saw + filter (OLD) | ⬜ Re-point at the reed waveguide (`osc_type:9`) like the base oboe, or make it a louder/fuller reed. |
| `trumpet-mellow` | saw (OLD) | ⬜ Re-do as additive brass formant (softer/darker) like the base trumpet. |
| `french-horn-loud` | saw (OLD) | ⬜ Re-do as additive H2-dominant (louder/brassier) like the base horn. |
| `cello-warm` | additive (OLD) | ⬜ Re-point at the bowed waveguide, or accept as an additive "warm pad cello". |
| `violin-ensemble` | additive + unison | 🎛️ Ensemble = a SECTION (many voices) → additive/unison is defensible; verify it still sits well next to the new solo violin. Lower priority. |
| `guitar-nylon-bright` | KS variant | 🎛️ Verify it reads as a brighter nylon (not steel) given the body-model split. Low priority. |
| `flute-breathy` | additive/gen (OLD) | ⬜ Re-sync with the new KS-blown-tube flute (breathier variant). |

---

## ⬜ Un-tuned — mimics a real instrument, needs a REFERENCE WAV + tuning

| Instrument | Current | Notes / suggested approach |
|---|---|---|
| `organ` | additive | Real pipe organ (additive is the correct model) — could tune to a reference, or treat as a generic organ and skip. |

---

## 🔌 Skip candidates (generic synths / not a specific acoustic instrument)

These are not trying to imitate one real instrument; **decide keep-as-is or delete** — don't reference-tune.

| Instrument | Why skip |
|---|---|
| `modular` | The base generic modular synth voice (the editable template). Keep. |
| `pad` / `modular-pad` | Synth pad — a synth sound, not an acoustic instrument. Keep as-is. |
| `bass` ("Synth Bass") | A synth bass — generic electronic. Keep as-is (or give a reference if an acoustic bass is wanted). |
| `conga` (quinto muted slap) + `conga-open` (open tone) + `conga-tumba` (low bass) | Percussion — tuned via the drum/membrane approach, not the melodic-WAV loop. Separate track. |
| `guitar-electric`, `guitar-electric-neck` | Electric guitar — no acoustic reference (the sound is the amp/pickups). Keep as synth-electric. |

---

## How to work this list

1. Pick a ⬜ item. Obtain a CC0/PD reference WAV (or have the user provide one).
2. Follow the tuning loop in `AGENTS.synth.md` (measure ref's true f0 → render synth at that pitch → FFT A/B → adjust seed → iterate).
3. Add a `physical_models_test.go` case if it becomes a shipped physical model.
4. Update the table above (move to ✅) + record what worked in `AGENTS.synth.instruments.md` + memory.
5. For 🔌 items: confirm with the user whether to keep or delete; if delete, remove the 8 wiring touchpoints (see [reference: add a builtin modular instrument] in memory) + regen `instrument_ids_gen.go`.
