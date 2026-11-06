# Synth Controls — Beginner-First Recursive Drill-Down (Proposal)

**Date:** 2026-07-21 (rev. 2026-07-22 — beginner-first + research-informed)
**Status:** Proposal (no code changes)
**Author:** Claude (Opus 4.8) + ingyamilmolinar

## Goal

Make the Synth tab usable by someone who has **never touched a DAW or a synth**.
They should get great sounds by tweaking a handful of controls named in plain
English, each showing a little picture of what it does — while advanced users can
drill all the way down to the ~60 raw params and lose nothing. Backed by research
into how Serum / Vital / Surge XT / Pigments / Zebra / FM8 solve the same problem
(see `2026-07-21-softsynth-comparison-research.md`).

## The core model: one control, drilled recursively

Everything is the **same card** the Synth tab already uses — we keep that UI. What
changes:

1. **Cards show high-level ideas, not raw params.** "Brightness", not
   "filter_cutoff / filter_resonance / filter_type".
2. **Every card has a one-liner and a little graph** of what it does and where it
   is right now (its current level).
3. **Tapping a card navigates *into* it** — it reveals that control's inner
   controls, in place, as the exact same kind of cards. (Today a tap opens the
   value dial; now a tap on a *group* card drills down instead.)
4. **A leaf control** — one that has no inner controls — opens the value dial/wheel
   on tap, exactly as today.
5. **A "Basic" button** (breadcrumb) climbs back up toward the top level.
6. **Basic and Advanced look identical.** There is no separate "advanced panel" —
   depth is just how far you've drilled. A beginner stays near the top; a pro
   drills down. Same cards, same graphs, same gestures, every level.

```
BASIC (root)              tap "Brightness"        tap "Cutoff" (a leaf)
┌──────────┐┌──────────┐   ┌──────────┐┌────────┐   ┌─────────────────┐
│Brightness││ Motion   │ → │ Cutoff   ││Resonan.│ → │  value wheel     │
│ ◑ graph  ││ ∿ graph  │   │ ⌣ graph  ││ ∿ graph│   │  (existing dial) │
│one-liner ││one-liner │   │[ Basic ◀ ]         │   │[ Basic ◀ ]       │
└──────────┘└──────────┘   └──────────┘└────────┘   └─────────────────┘
```

**Adjust vs. drill (reusing the existing gesture split):** the Synth cards already
distinguish *drag* from *tap* (drag-vs-tap wheel model). We reuse it:
- **Drag on a card** = adjust it right there (a group card drags the whole macro;
  a leaf drags its value). No navigation.
- **Tap on a card** = drill into it (group) or open the precise dial (leaf).

So a beginner can nudge "Brightness" without ever drilling; a curious user taps to
see what's inside; a pro taps down to the raw filter cutoff. One mental model.

## What the beginner lands on (the root)

The root is a lean set of **plain-English outcome controls** — the primary surface
the research says to lead with (Surge/Pigments cluster at 4–8 top macros). Typical
melodic instrument:

| Card | One-liner | Graph | Drills into |
|---|---|---|---|
| **Sound** | "The basic character of the sound." | the waveform | osc type / FM / wave |
| **Brightness** | "Dull and dark, or bright and sharp?" | spectrum tilt | cutoff, resonance, type |
| **Attack** | "Instant, or fades in?" | rising curve | amp attack (+ filter bloom) |
| **Length** | "A short pluck, or a long held note?" | decay curve | decay, sustain, release |
| **Width** | "One thin voice, or a big wide group?" | spreading dots | voices, detune, mix |
| **Motion** | "Perfectly still, or alive and moving?" | flat ↔ wavy line | drift, vibrato, humanize, scatter |
| **Body** | "How real/acoustic it sounds." | resonance bumps | body model, mix |
| **Warmth** | "Clean, or warm and gritty?" | soft-clip curve | drive, saturation |

That's ~6–8 cards a beginner can master. Each is a lens over existing params (see
the mapping in the catalog below).

## Hide what isn't in play (borrowed from u-he Zebra)

Zebra only draws modules that are actually in use; NN/g's research says the biggest
win is choosing what *not* to show. So the root **only shows cards relevant to this
instrument**:
- **Sound** always shows.
- **Body** only for instruments whose resonator is meaningful (strings, plucks).
- **FM Amount** only appears when the voice is FM.
- **Vibrato/Motion** hidden on a one-shot percussion voice.
- A disabled stage collapses to nothing at the root (its enable lives one level in).

Beatmo already has per-stage enable pills and identity-off states, so this is
mostly "don't render a root card whose stage is off/irrelevant" — cheap, and it
shrinks the surface far more than tiering alone.

## The macro → param mapping (the tree's wiring)

Each group card drives its children through a curve. Ranges illustrative.

| Group card | Children it drives (0 → 1) |
|---|---|
| **Brightness** | `filter_cutoff` 400→14 kHz (log); resonance held ~0.8 |
| **Attack** | `amp_attack` 0.001→1.0 s (log) [+ optional filter `Bloom`] |
| **Length** | plucky (`amp_decay` 0.3 / `sustain` 0 / `release` 0.15) → pad (2.5 / 0.9 / 1.0) |
| **Width** | `unison_voices` 1→7 + `unison_detune` 8→28¢ + `unison_mix` 0.4→0.85 |
| **Motion** | `unison_drift`, `ens_scatter`, `ens_vib_depth`, `ens_humanize`, `ens_jitter` (0 → alive) |
| **Body** | `body_mix` 0→1.2 (+ `body_model` picker child) |
| **Warmth** | `drive` 0→1 |
| **Vibrato** | `lfo_depth` 0→0.4 (auto-enable; rate/target as children) |
| **FM Amount** | scales `fm_op*_depth` together 0→~4 (algorithm/ratios as children) |
| **Punch** (perc) | `pitchenv_amt` 0→12 st (+ time child) |

The biggest wins are the same two the research flags: **ENSEMBLE (10 params →
Width + Motion)** and **FM (13 → FM Amount)**.

## The graphs (visual language, kid-readable)

Reuse Beatmo's existing kid-readable synth graph vocabulary (the preview pane
already draws osc + ADSR + filter). Each card carries a *tiny* version:
- **Sound** → the waveform outline.
- **Brightness / Cutoff** → a spectrum tilt or a filter curve with the corner marked.
- **Attack / Length** → an envelope curve (the segment that card controls emphasized).
- **Width** → dots/voices spreading apart.
- **Motion / Vibrato** → a flat line that becomes wavy/organic as the value rises.
- **Body** → resonance bumps.
- **Warmth** → a soft-clipping (tanh) curve that bends more with drive.
- **Amount/level leaves** → a radial or fill showing the current value.

The graph doubles as the "current level" readout, so a card needs no numbers to
tell a beginner where it sits.

## When a beginner drills and edits (macro ↔ inner sync)

Recommended rule (kept from rev.1, "Model A"): moving a group card re-derives its
children from the curve; editing a child directly marks the parent **"Custom"** (a
small dot on the parent card) — the parent still works, nudging it re-applies the
curve (with undo). Simple, no hidden link-state to explain. Loading a hand-tuned
patch (the 42 shipped instruments) reverse-maps each group card to the nearest
point on its curve, or shows "Custom" if the params don't lie on the curve — the
sound is never silently rewritten.

## What stays granular (don't over-simplify)

- **Discrete character pickers stay pickers**, never fused into a continuous macro:
  `osc_type`, `body_model`, `fm_algorithm`, `filter_type`, `lfo_target`,
  `formant_vowel/voice_type`, `kick_variant`. They live as their own child cards.
- **Every raw param remains reachable** by drilling — nothing is deleted, so no
  flexibility is lost. Advanced users just live a level or two deeper.
- **Enable pills stay** at each stage's level.

## Keep the current UI — what changes vs. what stays

- **Stays:** the card look, the drag-to-adjust wheel, the graphs, the section
  chrome, the sticky bar (Preview/Save/⋯).
- **Changes:** (a) card *contents* become high-level + one-liner + graph; (b) a tap
  on a group card *navigates deeper* instead of opening the dial; (c) a breadcrumb
  with a **"Basic"** button appears while drilled in; (d) the root shows the lean
  macro set, hiding irrelevant stages.

Implementation stays a **UI-only lens** — a card tree descriptor over the *existing*
ParamDefs (sits beside `advancedTierParams`). No new persisted state, no ABI /
recipe-hash / golden / export / plugin change. Writing a card calls the same
`audio.SetInstrumentParam` a slider calls today. Zero audio-engine risk.

## What the research changed vs. rev.1

- **Promoted the global "Quick Shape" macros to the primary root** (they were
  "optional" before). Field evidence: top synths lead with 4–8 outcome macros.
- **Added Zebra-style hide-unused** at the root.
- **Unified basic/advanced into one recursive card view** (was a 3-tier
  macro/essential/advanced split) — matches the user's "same design at every
  level, click to go deeper, Basic to go up."
- **Kept the divergence honest:** commercial macros are mostly *blank/assignable*
  power-user tools; Beatmo's *curated semantic* macros are more beginner-focused
  but less externally validated. Closest precedents: FM8 "Easy Edit Page",
  Pigments "PLAY" view.

## Rollout

1. Card-tree navigation shell (drill/tap/Basic breadcrumb) over the existing cards,
   with the 3 highest-value groups wired: **Brightness, Length, Width** — collapses
   ~15 raw knobs into 3 root cards and proves the recursion.
2. Add graphs per card + the one-liners (extend PlainEnglish).
3. Add Attack, Motion, Body, Warmth, Vibrato; wire hide-unused at the root.
4. FM Amount / Punch / Formant groups for the families that need them.
5. Consider a "surprise me" randomizer as a second beginner on-ramp (a known
   pattern the research couldn't quantify — worth a small experiment).
