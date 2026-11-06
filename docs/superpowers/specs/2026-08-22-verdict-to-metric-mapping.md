# From Ear Verdicts to Metrics — the mapping

**Date:** 2026-08-22
**Evidence:** 10 annotated verdicts in
`src/go/internal/audio/testdata/ear_verdicts.jsonl` (8 failures + bassoon + cello)
**Prior:** `2026-08-21-ear-verdict-round-1-findings.md`

## The central design consequence

Every complaint below is **relative to a reference we already hold**. Not one of
them needs an absolute threshold, a trained model, or a perceptual study. Each is
a *signed delta on one axis*, and the sign says which way to move.

This is what makes the listener's own observation load-bearing:

> "if you see one sounds woody and shouldn't and another one needs to sound
> woody/organic"

The same axis appears with **opposite signs on different instruments**. A metric
that hard-codes "less noise is better" would fix the oboe and ruin the cello. So
the unit of measurement is `delta = synth − reference`, per axis, per instrument,
and "better" always means "closer to this instrument's own reference".

## Verdict → axis table

| instrument | what the listener heard | axis | sign |
|---|---|---|---|
| oboe | "extremely breathy, remove most of it, overpowering" | noise fraction | synth **too high** |
| snare | "ghost sound, breathy, fuzzy, hissy" | ~~noise fraction, too high~~ **spectral flatness, too LOW** ⚠ | synth has MORE noise energy but the WRONG KIND (broadband→wire-like vs coloured→hiss-like); see correction note below |
| clap | "too noisy throughout, loud hissing, very breathy" | noise fraction **and its decay** | too high, and too *sustained* |
| cello | "more like a flute or oboe than a cello string" | noise fraction + harmonic density | synth **too low / too pure** |
| harp | "needs much longer decay, let it ring" | decay / ring time | synth **too short** |
| hihat | "bright and short; real is vibrational, resonant, shh" | decay time + tonality | too short, **too tonal** |
| tubular-bells | "simple clean tone vs resonant wavy metallic ring that prolongs" | decay + inharmonicity + beating | too short, too harmonic, no modulation |
| kalimba | "sounds like a plucked string; needs the wooden marimba feel" | partial ratios | **too harmonic** (needs inharmonic bar modes) |
| rimshot | "sounds like a marimba/wooden snare; real is full, deep, lower, rattly, cracky" | ~~centroid + f0 + attack crest~~ **noise fraction, too LOW** ⚠ | synth is missing the rattle/vibrational noise component entirely; see correction note below |
| bassoon | "needs more nasal, a bit too dark and mellow" | centroid + formant | **too low** |

> ⚠ **Correction (2026-08-22, `refs diagnose` acceptance run):** the snare and
> rimshot rows above were my own PREDICTIONS, made before the diagnostic tool
> existed. Running the real tool superseded both:
>
> - **rimshot**: I guessed "centroid/f0 too high". The tool instead measured
>   noise fraction 0.770→0.155 and spectral flatness 0.317→0.021 (both
>   MISMATCH, "too pure/tonal"), plus release far too fast. Re-reading the
>   listener's words: *"rattly/vibrational and cracky (snap)"* — **"rattly"
>   is a noise descriptor**, not a brightness one. The tool found the more
>   fundamental cause (a missing rattle/noise component) where the
>   prediction pointed at brightness.
> - **snare**: I predicted a single "noise too high" axis. The tool reports
>   noise fraction 0.815→0.933 (drift, MORE noise) but spectral flatness
>   0.599→0.204 (MISMATCH, FAR more tonal) — not a contradiction but the
>   real diagnosis: the synth has more noise energy of the WRONG KIND.
>   Broadband (high-flatness) noise reads as snare wires; band-limited/
>   coloured (low-flatness) noise reads as hiss — precisely *"so breathy and
>   fuzzy and hissy"*. Also `ring T60` 2.4x too long, matching *"sounds like
>   a ghost sound"* (a ghost note lingers).
>
> This is a result worth recording, not an embarrassment to hide: the tool
> being right and the human prediction being wrong is exactly what an
> independent diagnostic is *for*. `cmd/refs/diagnose_acceptance_test.go`'s
> `primaryVerdicts`/`bonusVerdicts` tables carry matching inline comments.
> **Design consequence:** noise *quantity* (fraction) and noise *character*
> (flatness) are distinct axes and must both be reported side by side —
> they already are; when they disagree, that disagreement is itself the
> diagnostic signal, not noise to average away.

## Four axes cover all ten

**1. Noise / harmonic balance** — catches oboe, snare, clap, cello, **and
rimshot** (5/10, per the correction above), in *both* directions and via
*both* sub-axes (fraction AND character can disagree, which is itself
diagnostic — see the snare correction). Primitives already in `fingerprint`:
`HNR`, `NoiseRatio`, `NoiseProfile.ResidualRatio`, `SpectralFlatness`.

**2. Temporal envelope, segmented** — catches harp, hihat, tubular-bells, clap
(4/10). This is the listener's explicit request: *"split it up into multiple
sections and also get duration of attack, release, peak, etc"*. Partially present
(`AttackTimeSec`, `DecaySlope`, `SustainLevel`, `ReleaseTimeSec`, `BandT60`,
`KickSegmentAnalyze`) but never generalised beyond kicks, and never reported as a
reference-vs-synth delta.

**3. Partial structure — harmonic vs inharmonic** — catches kalimba,
tubular-bells, hihat (3/10). The discriminator is sharp and classical: a string's
partials sit near integer multiples (1, 2, 3…), a struck **bar**'s do not
(a marimba bar's first overtone is tuned near 4:1), and a **metal tube**'s are
inharmonic and dense. `Inharmonicity` exists; what is missing is reporting the
measured partial *ratios* against the reference's, which is precisely what
separates "plucked string" from "wooden bar" — the kalimba complaint verbatim.

**4. Brightness / formant** — catches bassoon, hihat (2/10; rimshot moved to
axis 1 per the correction above — its centroid mismatch is real but
downstream of the missing noise component, not the root complaint).
`SpectralCentroid`, `SpectralRolloff`, `SpectralEnv` formant peaks all exist.

A fifth, needed only by tubular-bells: **slow amplitude modulation** ("wavy…
prolongs and waves in time") — beating between close inharmonic partials.
`Vibrato`/`Coupling` provide the machinery.

## Why "a simple statistical or timing metric should catch them" is correct

The listener's intuition holds, and the harp is the clearest case: a ring time of
~160 ms against a reference's ~1130 ms is a **7× difference on a single scalar**.
No perceptual modelling required — it is a ratio, and it was audible instantly.
The same is true of "too breathy" (a fraction) and "too high" (a centroid).

The reason these were not already caught is not that the metrics are hard. It is
that `Distance()` collapses everything into one weighted scalar, so a 7× decay
error and a small spectral improvement can cancel out — which is exactly how the
earlier winds work scored +17…+42% while sounding worse. **Per-axis, signed,
against the reference, reported separately** is the fix; a single number is the
thing to avoid.

## Acceptance test for the work

The diagnostic is only trustworthy if it independently rediscovers what the
listener already said. For each of the ten instruments above, the report must
flag the axis named in the table, with the correct sign — *without* being told
the verdict. Any instrument it fails to flag is a missing metric; any instrument
where it flags the wrong axis or sign is a broken one. That is a concrete,
falsifiable gate, and it must be a committed test.
