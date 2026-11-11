# Ear-Verdict Round 1 — Findings

**Date:** 2026-08-21
**Data:** `src/go/internal/audio/testdata/ear_verdicts.jsonl` (44 verdicts, one per
instrument family)
**Method:** `make ab-all` — for each family, the real reference plays first, then
each synthesized option. Every clip is level-matched (RMS) and the synth is
rendered at the reference's **measured** pitch, so the listener judges timbre
rather than loudness or a different note.
**Program context:** SP1 of
`2026-08-19-reference-grade-templates-program-design.md`.

## Why this file exists

The program's stated purpose is to make instrument realism *measurable*. Its
central risk, drawn from this project's own history, is that a metric improves
while the sound gets worse — `synth-match` once improved multi-band fingerprint
distance on winds and brass by +17…+42% and every one of those instruments was
then judged bad by ear.

These 44 verdicts are the defence against repeating that. They are the labelled
set any composite realism score must rank correctly **before** it is trusted to
drive tuning. A metric that scores the ❌ group above the ✅ group is disqualified,
regardless of how principled it looks.

## Results

**✅ Sounds right — 7**
double-bass, harpsichord, koto, marimba, organ-church, vibraphone, xylophone

**⚠️ Partly — 21**
bassoon, clarinet, cowbell, crash, dnb-kick, glass-armonica, glockenspiel,
kick-acoustic, pan-flute, piano-felt, piano-grand, ride, sax, shaker, sidestick,
singing-bowl, trombone, tuba, viola, voice-ahh, zgump-kick

**❌ Does not sound right — 8**
clap, harp, hihat, kalimba, oboe, rimshot, snare, tubular-bells

**Multi-option families — which variant won**

| family | chosen | over |
|---|---|---|
| cello | `cello` | `cello-warm` |
| conga | `conga` | `conga-open`, `conga-tumba` |
| flute | `flute-breathy` | `flute` |
| french-horn | `french-horn` | `french-horn-loud` |
| guitar-nylon | `guitar-nylon` | `guitar-nylon-bright` |
| tom | `high-tom-organic` | `tom` |
| trumpet | `trumpet` | `trumpet-mellow` |
| violin | `violin` | `violin-ensemble` |

## Finding 1 — struck and plucked win; sustained loses

Six of the seven ✅ are struck or plucked (marimba, vibraphone, xylophone,
harpsichord, koto), plus organ. **Every continuously-excited instrument — bowed,
blown, reed — landed in "partly" or worse.**

This independently reproduces an older, separately-derived conclusion: that
Karplus-Strong plucked strings are the engine's strongest organic voice, and that
sustained tone is where the synthesis is weakest. Two independent routes reaching
the same conclusion is worth more than either alone.

The mechanism is plausible: a struck or plucked note is defined by its attack
transient and decay envelope, which the engine models well. A sustained note is
defined by what happens *during* the sustain — micro-variation, breath or bow
noise, partial decorrelation — which is exactly the region where the analysis has
no metrics today (see the program spec's SP2 section).

## Finding 2 — three failures that break the pattern

`harp`, `kalimba` and `tubular-bells` are all struck or plucked, yet all three
failed. They therefore do **not** share the sustained-tone weakness and need
their own diagnosis. Whatever is wrong with them is specific and probably
cheaper to fix than the wind problem. `oboe` is the fourth ❌ that is not
percussion, and belongs with the winds.

The remaining four failures — `clap`, `hihat`, `rimshot`, `snare` — are all
noise-led percussion, which is a coherent group and suggests a shared cause.

## Finding 3 — the plainer variant wins, except when noise helps

Six of the eight variant choices went to the **plainer** option: `cello` over
`cello-warm`, `trumpet` over `trumpet-mellow`, `violin` over `violin-ensemble`,
`french-horn` over `french-horn-loud`, `guitar-nylon` over `guitar-nylon-bright`,
`conga` over its open/tumba siblings.

This matches the recorded lesson that `unison_voices > 1` turns any solo
instrument into a synth-string pad — the single most common cause of a patch
sounding like "synth strings" rather than the instrument. **Actionable:** audit
every seed for `unison_voices > 1` on instruments meant to be solo.

The two exceptions moved in the opposite direction, toward **more** noise and
body: `flute-breathy` beat `flute`, and `high-tom-organic` beat `tom`. So the
principle is not "plainer is always better" — it is that *ensemble/chorus* hurts
while *breath and body noise* helps. Those are different knobs and should not be
conflated.

## What this implies for the remaining sub-projects

- **SP2 (calibrated analysis):** these 44 rows are the validation set. Rank
  correlation against them is the acceptance gate for any composite score. The
  ✅/❌ split also predicts where new metrics are needed most — sustain-region
  descriptors, since that is precisely where the failures cluster.
- **SP4 (timbre tuning):** start with the 8 ❌ for headroom, but treat
  harp/kalimba/tubular-bells as a separate investigation from the winds, and the
  four noise-led percussion failures as a third. Three groups, three causes.
- **Cross-cutting:** the unison audit from Finding 3 is cheap and can be done
  before any of the above.

## Known limitations of this round

- **No prose notes.** All 44 verdicts carry a choice but no "why". A second pass
  over the ❌ and ⚠️ entries is needed to capture what specifically sounds wrong;
  that text is what makes a metric provable rather than merely correlated.
- **16 references are 128 kbps MP3 previews** (disclosed per-entry in the
  catalogue). Their HF above ~14 kHz is codec-limited, so any verdict resting on
  "air" or noise-floor character is less reliable for those instruments.
- **One listener, one session, one note per instrument.** No repeat trials, so
  ordering and fatigue effects are unmeasured. Treat individual verdicts as
  indicative and the group-level patterns as the durable signal.
