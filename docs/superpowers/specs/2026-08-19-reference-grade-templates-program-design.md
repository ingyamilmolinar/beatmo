# Reference-Grade Templates — Program Design

**Date:** 2026-08-19
**Status:** APPROVED (program level). SP1 specced in full in
`2026-08-19-reference-audio-corpus-design.md`; SP2–SP5 get their own
spec→plan cycles.

## Goal

Make every Beatmo template circuit sound as close as possible to the real
recording it recreates, along **all four** realism axes — score fidelity,
performance, timbre, and mix — and make that closeness **measurable**, so
tuning stops being a sequence of ear-guesses.

Secondary goal, equally load-bearing: ship at least one circuit per
instrument (excluding voice), each paired 1:1 with an isolated reference
recording, so every instrument has a tight, reproducible tuning loop.

## Why this needs a program, not a spec

The work decomposes into five subsystems with a real dependency order.
Each is independently shippable and independently valuable.

```
SP1 reference corpus ──┬─→ SP2 calibrated analysis ──┬─→ SP4 timbre tuning + solo circuits
                       │                             └─→ SP5 mix realism
                       └─────────────────────────────→ SP3 score + performance fidelity
```

## The two failure modes this program exists to prevent

Both are documented in this repo's own history and both must be designed
against, not assumed away.

**1. The metric lied.** `synth-match` improved the fingerprint distance on
winds and brass by +17…+42% against isolated University of Iowa samples.
Every one of those instruments was then judged **bad by ear**. A lower
multi-band distance to one sustained note does not mean an instrument
sounds real. Any composite score this program ships must be *validated
against recorded listening verdicts* before it is trusted to drive tuning.

**2. Metric magnitude matched; character was opposite.** The violin's
amplitude-fluctuation statistic (std/mean) was tuned to 0.40 against a real
0.64 and still sounded wrong, because the real signal's variation is a
slow bow-stroke *shape* while the synth's was fast chaos. Scalar summaries
of time-varying behaviour are not enough; the program needs descriptors
that capture **shape and rate**, not just magnitude.

## Sub-projects

### SP1 — Reference corpus and provenance

*Ships first; everything else resolves references through it.*

A single committed catalogue maps a reference id to its instrument(s) or
template, its upstream source and licence, a deterministic conversion
spec, the sha256 of the **converted** file, and audio-**measured**
metadata (f0, peak, RMS).

Two tiers, and licence plus size decide storage:

- **Tier I — song references.** Public-domain full-piece recordings for
  the classical templates, from archive.org (including the mirrored
  `collection:musopen`) and IMSLP. Tens of MB each, so **never
  committed** — fetched into `$BEATMO_WAV_REFS`, with every consumer
  skipping cleanly when absent, per the existing `internal/wavref`
  contract. These drive `songmatch.Compare`: tempo, rhythm, key, and
  mix-band balance.
- **Tier II — instrument references.** One isolated note or short phrase
  per acoustic instrument, clipped to ≤3 s, from University of Iowa MIS
  (unrestricted) and the Versilian Community Sample Library (CC0 1.0).
  Licence-clean and ~5 MB in total, so these **are committed**, exactly as
  five Iowa MIS clips already are — which is what makes SP2's calibration
  and SP4's tuning reproducible in CI with no environment setup. These
  drive per-instrument timbre work and are the higher-signal tier.

Full design: `2026-08-19-reference-audio-corpus-design.md`.

### SP2 — Calibrated analysis

The existing `internal/audio/fingerprint` package is already broad —
vibrato, per-harmonic trajectories, spectral envelope and formants,
dynamic brightness/loudness coupling, harmonic/residual decomposition,
MFCC, roughness, band T60, plus dedicated kick, snare, piano and wind
analyzers. The deficiency is **not** metric count. It is threefold:

**(a) Nothing is calibrated.** `Distance`'s term weights are hand-picked.
No evidence exists that any metric, or their weighted sum, ranks
instruments the way a listener does. SP2 commits an **ear-verdict
corpus** — every listening judgement this project has recorded (organ ✅,
steel guitar ✅, flute ✅ after the blown-waveguide rebuild; winds and
brass ❌; grand and felt piano ⚠️ "lo-fi, indistinguishable"), each with
its symptom vocabulary ("organ", "sci-fi synth", "mandolin", "bassy",
"industrial noise"). Candidate metrics are then kept only if they rank
those verdicts correctly. The composite realism score is a *fitted*
result, not an assumed one, and its reported confidence is the measured
rank correlation.

**(b) Three metric families are genuinely missing:**

- **Envelope shape and modulation rate.** Not "how much does amplitude
  vary" but "in what shape and at what rate" — the normalised amplitude
  envelope compared curve-to-curve against the reference, plus the
  modulation spectrum of that envelope, which separates a slow bow stroke
  from fast chaos at identical variance. This is the direct fix for
  failure mode 2.
- **Partial decorrelation.** Phase-locked additive sines read as
  artificial; a real harmonic is a narrow band of slightly detuned,
  randomly phased components beating against one another. Needs a
  per-partial bandwidth and phase-coherence measure, which is what
  distinguishes "sterile" from "alive" and currently has no metric at all.
- **Structural discriminators for the recurring failure words.** The same
  four complaints recur across instruments — "organ" (dead-steady tone),
  "sci-fi synth" (perfectly flat sustain plus resonant sweep), "mandolin"
  (ring time far too short), "bassy" (fundamental over-strong relative to
  the band profile). Each gets one explicit detector, validated against
  the verdicts that produced the word.

**(c) There is no single report.** Every past A/B comparison was an ad-hoc
`uv`/numpy script, rewritten each time. SP2 ships one command producing
the harmonic-ladder table, the envelope curve, band energies, time
evolution, and the calibrated score — plus per-instrument goldens so a
regression is caught rather than re-discovered by ear.

**Whole-signal analysis** replaces first-window analysis wherever the
latter survives; a 16384-sample opening window mis-reads any
attack-heavy or fast passage.

### SP3 — Score sourcing and performance fidelity

> **AMENDED 2026-09-02** by
> `2026-09-02-public-domain-template-curation-design.md`. The retention
> clause below — "twenty-one are hand-authored … and must stay that way" —
> is **superseded**. Those twenty-one templates were removed rather than
> kept: a template recreating a copyrighted song from MIDI research has no
> obtainable score, so it can never satisfy this programme's own fidelity
> criterion. The library is now 24 templates, every one generated from a
> committed CC0 score.
>
> The rest of SP3 is **delivered**. The importer was extended past pitch
> and onset to carry dynamics, articulation, ties, multiple voices and
> tempo marks, and to map them onto per-node volume, duration and groove
> drift. `TestTemplatesComplyWithSourceOfTruth` and the musical-fidelity
> tests were not extended but **replaced**: they asserted a hand-written
> table of researched keys and tempi, which could not catch a wrong note.
> The circuits are now checked against the scores themselves.
>
> Idiomatic humanisation (swing, ghost notes, accent patterns) no longer
> applies — there are no hand-authored templates left to apply it to.

Original text, for the record:

> Eleven templates are already generated directly from embedded MusicXML via
> `internal/musicxml`; twenty-one are hand-authored from cited MIDI research,
> and must stay that way — the sheet music for a copyrighted song cannot be
> embedded any more than its recording can.
>
> Work: source real MusicXML or kern for every public-domain piece still
> hand-transcribed (`pachelbel-violin` and `marcello-oboe` are explicitly
> flagged TRANSCRIBED, not source-verified), plus scores for the new solo
> circuits. Then extend the importer beyond pitch and onset to carry
> **dynamics, articulation, slurs and ties, multiple voices, and tempo
> marks**, and map them onto Beatmo's per-node volume, duration and pitch —
> so a circuit stops reading as quantised machine playback. Idiomatic
> humanisation (swing, ghost notes, accent patterns) applies to the
> hand-authored genre templates where no score exists.
>
> Existing guards extend rather than being replaced: `TestDrift`,
> `TestTemplatesComplyWithSourceOfTruth`, and the musical-fidelity tests.

### SP4 — Per-instrument circuits and timbre tuning

> **RESCOPED 2026-09-02.** The instrument count below counted the removed
> templates. The public-domain library exercises ~25 distinct instruments,
> mostly orchestral and keyboard; the rest stay registered and playable but
> are no longer reached by a template. "Each instrument gets a circuit whose
> notes come from a real part for that instrument" is therefore
> unreachable for the synthetic families (808s, Reese basses, FM voices),
> which have no notated part anywhere — for those, tuning proceeds against
> SP1/SP2 alone, with no circuit.

Fifty-seven distinct base instruments appear across the templates
(fifty-five excluding the two voice instruments). Each gets at least one
dedicated circuit whose notes come from a real part for that instrument.
Variants (`cello-warm`, `trumpet-mellow`, `guitar-steel-warm`, …) get a
circuit too, but share their base's reference and are tuned as a
documented character offset rather than matched independently.

Tuning then runs per instrument through SP2's report against SP1's
reference. C-engine expansion is permitted **only when the diagnostic
proves the current pipeline cannot reach the reference** — the precedent
additions (`gen_ks_blow` blown waveguide, `filtenv_attack`, `body_model`
resonator bank, `bow_dynamics`) each followed a measured gap. Every
addition follows the existing full param-add checklist and must leave the
audio goldens byte-identical when the feature is off.

### SP5 — Mix realism

`songmatch`'s band attribution and per-stem analysis set per-row volume,
pan, delay/reverb sends and EQ so the rendered master's band balance
matches the reference recording's, replacing hand-picked levels. Applies
to the eleven templates with a Tier-I reference; the rest inherit
family-level mix conventions.

## Scope boundaries

- **Nothing copyrighted is committed** — not audio, not sheet music. The
  twenty-one copyrighted templates record `song_ref: none` with the reason,
  and are validated at instrument and arrangement level only.
- **Synthetic-by-design instruments have no acoustic ground truth.**
  `fm-*`, `bass-808`, `kick-electro`, `scifi-lead`, `modular-pad`,
  `ensemble-lead`, `viola-pad`, `ghost-bass` are marked `synthetic` in the
  catalogue and are counted as covered, not missing.
- **Known source gaps are recorded, not faked.** No CC0 isolated reference
  was found for `guitar-steel`, `guitar-electric`, electric `bass-guitar`,
  `clavinet`, or a Hammond-style `organ`. These are tracked as
  `needs-source`; the locally collected freesound CC0 files already in the
  working tree cover some of them and are registered as `local` entries.
- **Ear verdicts remain the final authority.** The calibrated score exists
  to make listening *rarer and better targeted*, never to replace it.

## Success criteria

1. Every acoustic instrument resolves to a reference; every gap is
   explicitly classified as `synthetic` or `needs-source`. (SP1)
2. The composite realism score's rank correlation against the ear-verdict
   corpus is measured and reported; metrics that fail to discriminate are
   dropped rather than kept. (SP2)
3. Every public-domain template is generated from a sourced score, not a
   transcription. (SP3)
4. Every instrument excluding voice has at least one circuit and a
   before/after realism report. (SP4)
5. Each Tier-I template's rendered master matches its reference on tempo,
   key (transposition-invariant) and band balance within the configured
   tolerance. (SP5)
