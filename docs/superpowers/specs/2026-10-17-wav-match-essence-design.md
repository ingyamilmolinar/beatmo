# WAV match by instrument ESSENCE — design (2026-10-17)

`refs match` / `make wav-match` ranks every shipped instrument against an
arbitrary recording; `make ab-match` lets the ear approve the result. After
the first listening pass (4 verdicts in `wav_match_verdicts.jsonl`) the user
stopped: the rankings were "way off most of the time" — a real piano G5
matched `double-bass`, a real cello D2 matched `sax`, a french horn matched
`viola`, congas matched snares. This spec records why (measured, not
assumed) and the restructure.

## Root causes (each reproduced with a probe on the user's own WAVs)

1. **Duration was compared, not the instrument.** `RenderShipped` sizes a
   voice at 120 BPM: `render.beats: 2` = a **1.0 s** note, zero-padded to the
   reference's 2 s. Against a 2 s bowed cello (attack 1.06 s, sustain 0.64)
   every candidate measured sustain **0.00** inside the 0.3–1.3 s analysis
   window because its release had already started. The piano G5 (1.56 s) was
   compared with 1.00 s voices. Every envelope term, the whole `KickDistance`
   trace set and the "full" distance were partly measuring note length.
2. **MFCC measured the noise floor.** `computeMFCC` floors band power at an
   ABSOLUTE `1e-10`, never relative to the clip. A clean synth render puts
   empty bands at −74…−91 dB while the recording's room tone sits at
   +20…+35 dB: for `piano-grand` vs the real piano, **100 %** of the squared
   log-mel difference came from bands > 60 dB below the peak (inaudible).
   `double-bass` won because its unison/body smearing happens to fill those
   bands (58 %). On the audible bands `piano-grand` tracked the real piano at
   least as well.
3. **Each clip detected its own f0.** `FromWave` re-runs YIN on every clip,
   so the reference cello fingerprint used **146 Hz** (an octave error the
   matcher had already corrected to 73 Hz for rendering) while candidates
   used 73, 146 or 219 Hz. Partials, even/odd, tristimulus, inharmonicity and
   the harmonic trajectory were compared on DIFFERENT harmonic grids.
4. **The attack was never looked at.** Every whole-timbre descriptor uses
   the sustain window starting 0.3 s in. Hammer, pluck, bow and breath onsets
   — the cue listeners use most to name an instrument — were invisible; a
   short percussive render was fingerprinted on silence (three different
   congas scored an identical 9.466).
5. **The ranking mixed performance into identity.** `fused` (the sustained
   default) is the mean rank of `timbre`, `full` and `perc`; two of the three
   are dominated by envelope/vibrato/coupling/duration — properties of the
   performance and the clip, not of the instrument.
6. **Pitch was held fixed for percussion.** An unpitched reference was
   matched against every voice at its natural pitch, so a quinto conga was
   judged against the synth congas tuned an octave away.
7. **Slow.** 98 % of the time was analysis (`KickAnalyze` ~1.7 s and
   `FromWave` ~0.7 s per clip; all 150 renders take 2.5 s), so `-validate`
   took an hour and nothing could be iterated.

## Principle

A match answers: *which shipped instrument, played the way a user can play
it, sounds most like this recording?* Anything a user sets at play time — the
note's **pitch**, its **length** (tempo / node duration), its **level** — and
anything the recording adds — **room tone, reverb tail, mic level** — is a
nuisance and must not decide the ranking. What decides it is the instrument's
**essence**: the colour of its spectrum, its resolved harmonic structure, its
noise texture (breath, bow, hammer, wires) and how its spectrum evolves from
the attack into the body. The amplitude envelope still counts (a piano and an
organ of equal colour are different instruments) but with a low weight.

Nuisances are removed BY CONSTRUCTION, not by down-weighting:

| Nuisance | Removed by |
|---|---|
| pitch, pitched reference | melodic voice rendered AT the reference f0; a non-melodic pitched voice resampled to it when within ±12 st of its natural pitch (beyond that it is not that instrument any more, and no user plays it there) |
| pitch, unpitched reference/voice | search over a ±12 st grid (resampling, as the engine pitches drums), best kept |
| note length | voice rendered at the tempo that makes it at least as long as the reference's sounding region; only that region is compared |
| onset offset | the candidate gets the SAME leading-silence trim as the reference |
| level | everything is loudness-normalised |
| room tone / noise floor | spectra are compared as perceptual LOUDNESS (power^0.3, Stevens), where a band 60 dB down weighs ~1.6 % of the peak instead of dominating a log scale; no absolute floor anywhere |
| f0 per clip | the analysis takes the KNOWN f0 (the reference's measured pitch = every render's pitch) instead of re-detecting it |

## The essence distance (`fingerprint.AnalyzeEssence` / `EssenceTerms` / `EssenceTotals`)

One STFT (2048 / hop 512 at 44.1 kHz), banded on the ERB-rate scale (one
band per ERB, 30 Hz – 12 kHz: the auditory filter resolution; the MP3
previews in the corpus are codec-limited above ~14 kHz). ACROSS FREQUENCY
every spectrum is specific loudness (band power^0.3, each clip normalised to
its own loudest band); ACROSS TIME frames are weighted by ENERGY. The
reference defines the analysis region (onset → last frame within 30 dB of its
peak, capped by the clip); a candidate is analysed over exactly that region.

| Term | What it is | Why this form |
|---|---|---|
| `Color` | energy-weighted mean spectral SHAPE (each frame normalised to unit loudness), L1 | the tone colour; loudness across frequency so room tone 60 dB down weighs ~1.6 % |
| `Evolution` | each frame's deviation from the clip's own mean colour, L1, weighted by the reference's energy | how the colour moves (attack spectrum, a struck string darkening), orthogonal to `Color` |
| `Harmonics` | resolved partials 1–10 at the KNOWN f0, amplitude^0.6, over 0.5 s from the loudest frame (pitched references only) | the note's harmonic recipe; never re-detected per clip |
| `Texture` | per-octave NOISE share (median bin ÷ mean, ÷ ln 2: white noise ≈ 1, partials over silence ≈ 0), energy-weighted over time, compared with `min` of the two clips' octave shares | breath / bow / wires; compared only where BOTH have sound (presence is `Color`'s job) |
| `Decay` | level in dB (clipped at −30) at 30, 60, 120, 250, 500 ms after the loudest frame, mean abs diff ÷ 30 | plucked/struck decays, bowed/blown holds, a short ring vs a long one — how the instrument is PLAYED, not for how long; a log-time grid reads the decay where the direct sound dominates any room, and it stops at 0.5 s because how many SECONDS a note rings is duration |
| `Attack` | log10 of the 10→90 % rise of a 20 ms running-max envelope, rise times floored at 10 ms in the distance | the excitation's speed; an RMS over 5 ms rippled on a 63 Hz kick (1 vs 6 ms by phase); onsets under ~10 ms are all heard as instantaneous, and on a bare log scale a real harp's 16 ms pluck sat farther from the synth harp's 1 ms than from a tuba's 43 ms |

Total = Color + Evolution + Harmonics + Texture + Decay + 0.1·Attack — the
terms share a currency (a fraction of loudness, 0..1–2) and are summed as
measured. The unpitched pitch search minimises the same total (resampling a
drum moves its decay as much as its colour).

### Tried and measured, then dropped (64 catalogued references, `refs match -validate`)

| Variant | Result |
|---|---|
| each term ÷ its median over the candidate set | MRR 0.284 vs 0.335 fixed: amplified narrow terms (a 0.05 texture gap cost as much as a 0.75 colour gap) |
| texture weighted by the mean / by either clip's octave share | ≤ the `min` weighting in every run; the synth trumpet's faint noisy 4–12 kHz residue (1–2 % of its loudness) outweighed the best harmonic match in the roster |
| frames weighted by LOUDNESS over time | a −30 dB reverb tail (13 % of the hit per frame, fifty frames of it) out-voted a kick's own sound: the kick ranked itself #17 under nuisance |
| spectral flatness (geometric ÷ arithmetic mean) for texture | dominated by the partial peaks; a −12 dB breath barely moved it |
| an amplitude-envelope term (loudness, energy-weighted) | energy weighting hides the decay, so a sustained oboe and a plucked harpsichord read alike |
| colour on 3-ERB bands | MRR 0.321 vs 0.329 |
| melodic voices also tried an octave either side of their frame pitch | MRR 0.330 vs 0.329 (church organ #52 → #33, nothing else) |
| octave-frame pitch search, ×2 harmonics weight, ×½ evolution | within ±0.03 MRR of the chosen weights: noise for n = 64 |
| decay profile out to 2 s | MRR 0.409 vs 0.400 at 0.5 s — equal on the corpus, but it judged ring LENGTH: a sustained tuba out-ranked the synth harp for a real harp that rings longer than the synth (the user's own note on that harp: "needs to last for longer") |
| a per-frame masking floor (each band raised to ≥ 30 dB under the frame's loudest) | MRR 0.368 vs 0.409, kind #1 30 vs 35: content 25–35 dB down carries identity across the corpus, not only room tone |
| subtracting the WHOLE hub (not CSLS's ½) | MRR 0.412 vs 0.409 on recordings, but over-boosted the voices far from every recording (modular, FM plucks) so a re-played synth marimba ranked `modular` first |

## Evaluation

* `refs match -validate` grades every catalogued reference: own family at
  #1, in the top 3, mean reciprocal rank, plus a BROAD class (bowed / brass /
  reed / flute / plucked / keys / mallet+bell / voice / organ / synth / kick /
  snare / tom+hand drum / cymbal+hat / clap+shaker+metal) — "is it the right
  kind of instrument", which is what the user asked the matcher to get right.
  (The legacy metrics were selectable with `-metrics` for the A/B until
  2026-10-18, when they were removed -- see "the misses, by name".)
* `TestEssenceSelfIdentifiesUnderNuisance` (real build): a shipped voice
  re-rendered at another pitch and tempo, quieter, with a room-noise floor
  and a synthetic reverb tail, must still rank itself (or a same-sound twin)
  near the top of the whole roster. It needs no reference audio.
* Unit tests on synthetic signals pin each term: identity, level invariance,
  noise-floor robustness, the known-f0 contract, attack/texture
  discrimination, the reference-defined window.

## 2026-10-18 — nuisance leak in Decay, the nuisance sweep, the coverage map

### The decay was read from an arbitrary frame of a held note

`Decay` (and the window the harmonic profile is taken from) started at the
ARG-MAX frame. On a held note every plateau frame is within a hair of the
maximum, so the arg-max lands anywhere -- and a late one read the decay off
the clip's cut end. A synthetic held saw read **−16 dB** of "decay", and the
same held tone with a +0.5 dB swell late in the note scored a Decay distance
of **0.53** from itself (half the colour scale, for nothing but where a swell
fell). That is note length and performance leaking straight into the
ranking. Both now start at the ONSET of the loud part -- the first frame
within 1 dB of the loudest (`essLoudOnset`, the same rule `matchClassOf`
already used) -- and the decay never reads a frame that runs past the
analysis window (part zero padding). Guards:
`TestEssenceDecayIgnoresWhereTheHeldPlateauPeaks`,
`TestEssenceTraitsPointTheRightWay` (both fail on the old reading).
`matchHubnessVersion` 4.

### `refs match -nuisance` — does the matcher hear the INSTRUMENT?

`TestEssenceSelfIdentifiesUnderNuisance` gates one combined nuisance on 13
voices. The sweep (`make wav-match-nuisance`) re-plays 49 voices spanning
every broad kind with ONE nuisance at a time -- pitch ±5 st, a note cut short
(240 BPM, and the app's 120 BPM, where a drum stops at its shipped length;
every other case lets the voice ring out, as a recording does), a room (room
tone, −55 dB floor, −10 dB), a
Schroeder reverb at −9 dB, a ±6 dB high shelf at 2 kHz (a brighter / darker
microphone, or the row EQ a user can set), then all at once -- and ranks the
whole roster against each exactly as `refs match` ranks a download. A
nuisance that moves a voice's self-rank is leaking into the essence distance;
the per-nuisance summary names the worst cases.

### `refs match -map` — the coverage map

`make wav-match-map` ranks the downloads plus every catalogued instrument
reference (`-wavs` takes comma-separated globs; `catalogue` = every
catalogued instrument reference present locally) and writes `MAP.md` beside
the clips. A recording is LABELLED by the catalogue -- its converted file,
the local file it was adopted from, or the freesound download whose sound id
the ref's preview URL carries -- else by an instrument its filename names;
the matcher never sees the label. Every roster instrument is then:

| state | meaning |
|---|---|
| matched | a recording of it ranks it in the top 3 |
| fidelity gap | recordings of it exist, the synth ranks lower on all of them -- the map names the essence term that loses it and the DIRECTION the voice is off (`Essence.Traits`: brightness centroid, noise share, level after the peak) |
| stand-in | no recording of its own, but the closest voice to some other recording |
| uncovered | no recording of it and in no recording's top 3 |

`refs match -explain a,b -wavs <file>` prints every essence term of the top
N and of the named instruments for one recording -- how a ranking is traced
to the term that decided it.

### The note was a nuisance that still leaked (found by the sweep)

The baseline sweep (old matcher, ring-out not yet applied) held on pitch,
note length, room and microphone -- 46-49 of 49 voices first -- and broke in
two places: reverb (32/49 first, 43/49 top 3; partly a sweep artifact, below)
and INHARMONIC voices, where the note itself leaked:

* **YIN flips between divisors of an inharmonic cluster.** The synth
  kalimba's strongest partials (757, 887, 997, 1122, 1342 Hz) have no
  harmonic series; YIN read the clean voice as 252 Hz, over a −55 dB floor
  378 Hz, through reverb 443 Hz, with everything 944 Hz -- and every voice
  was played on that note, so the kalimba ranked itself #23 / #34 / #85 and
  the cowbell was skipped outright. Three changes:
  - `harmonicSieve`: the HIGHEST fundamental whose harmonics 1..8 explain
    every partial within 20 dB of the strongest, least-squares refined; a
    pitch that already explains them is never moved (it subsumes the ÷2/÷3
    `subharmonicCorrection` cases). Capped at the 8th harmonic: at 3 %
    tolerance, harmonics 9..12 of a low pitch cover a third of the spectrum
    and "explained" the kalimba cluster as 124 Hz.
  - `pitchExplains` gate: a pitch must hold ≥ 70 % of the strong-partial
    energy on its harmonics 1..8 AND its loudest partial must be one of
    harmonics 1..5 (residue pitch comes from the low harmonics). Otherwise
    the recording has no single note and is matched as unpitched -- every
    voice played at whichever pitch within an octave sounds closest.
  - `matchVoiceFrame`: each voice's pitch at node pitch 0 is measured by the
    SAME pipeline that measured the reference, and the voice is rendered so
    its measured pitch equals the reference's. The declared frame (melodic =
    A3 at pitch 0) disagreed with what the detector hears on the synth
    kalimba by 2.4 st, so a recording of that very voice was compared with it
    re-rendered 2.4 st away (0.83 against itself; now 0.006). Falls back to
    the declared frame when the voice has no usable pitch or reads > 12.5 st
    from it.
* **Drums read an octave low.** A membrane's modes sit near 1 : 1.5, so the
  shipped tom re-played +5 st read 86 Hz (the residue) instead of 171 and was
  resampled an octave away from itself (#14). A NON-melodic voice now also
  tries the octave either side of its mapped pitch (inside its ±12 st range)
  and keeps the closest; melodic voices keep the one note (octaves for them
  graded as noise).
* **Sweep artifact.** At the app's 120 BPM the shipped conga is a 0.25 s
  voice, cut mid-ring. The sweep's "recording" was that cut hit plus a reverb
  tail, against the matcher's (correctly) un-truncated conga: #33. A
  recording is never truncated, so every sweep case now rings out
  (`VoiceBPMFor` the analysis window) except the two cut-short cases.

Leave-one-out on the 64 references (`-validate`), each change ablated alone
(`MATCH_NO_*` / `ESS_ARGMAX` overlay toggles, scratch only):

| | MRR | family #1 | family top 3 | kind #1 |
|---|---|---|---|---|
| before (10-17) | 0.409 | 19 | 29 | 35 |
| all of today's changes | 0.393 | 19 | 26 | 34 |
| … without the octave tries | 0.393 | 19 | 26 | 34 |
| … without the voice frame | 0.393 | 19 | 28 | 33 |
| … without the pitch gate | 0.406 | 20 | 28 | 35 |
| … with the arg-max decay | 0.394 | 18 | 27 | 34 |

The whole MRR difference is ONE reference: the real mbira (`kalimba-vcsl`,
filed A4, read B2) is a buzzing inharmonic cluster (426-862 Hz) no pitch
explains; read as B2 it constrained the competitors to that note and the synth
kalimba came first by luck, matched honestly as unpitched it is #4 behind two
snares (its decay is too long, its colour too clean -- a fidelity gap). The
other movements are within the ±0.03 noise of n = 64. The decay fix is what
lets the right family near the top of `voice-ahh-freesound` (Decay 0.55 →
0.08 for voice-ahh / voice-opera); there an anti-hub (`cowbell-drive`, hub
1.35) then takes #1 on the ½-hub bonus -- the known cost of CSLS.

## 2026-10-18 — where the time goes

### Profile (i7-8665U, 4 cores / 8 threads)

A ranking is throughput-bound: at 6 workers the CPU is ~98 % busy, and 8
workers take the same wall time as 6 (five references: 22.7 s vs 22.5 s).
`GOGC=400` saves 3 %. CPU of five rankings (pprof, before the changes below):

| where | share |
|---|---|
| rendering the voice (`RenderAsShipped`) | 52 % -- the C voice (`render_modular_p`) 43 %, the document FX ~6 % (one cgo call PER SAMPLE) |
| essence analysis (`AnalyzeEssence`) | 33 % -- FFTs 23 % (a fifth of that recomputing bit reversals), the texture's per-octave median sort 2 % |
| measuring each voice's pitch frame (`matchVoiceFrame`) | 11 % -- 150 renders + YIN, ~1.8 s wall, once per PROCESS |
| GC | ~6 % |

The multiplier is the number of render+analyses, which is set by the pitch
plan (`matchPitchPlan`): a pitched reference costs 260-320 (melodic voices one
note, non-melodic ones the note and its octaves, a dozen unpitched voices
searched), an UNPITCHED one ~1,950 -- all 150 voices searched at 13 pitches
(the step-3 grid, then ±2 and ±1 around the best). A drum hit costs 6.5× a
note. Over the 150 voices (2 s notes, one thread) a render averages 16 ms
and an analysis 20 ms; the slowest renders are `hihat-open-bright` (153 ms)
and `ride` (102 ms).

### Exact changes (identical output)

* `wave.fft` takes its bit-reversal swaps from a per-size cache, and
  `MagnitudeSpectrum` windows straight into the transform buffer:
  `AnalyzeEssence` of a 2 s note 61 → ~45 ms, −30 % allocations. Guard
  `TestMagnitudeSpectrumIsBitIdentical` (against the original pipeline, all
  three windows, short/exact/long/degenerate inputs).
* `essMedian` selects in place instead of sorting. Guard
  `TestEssMedianIsTheSortedMiddle` (n = 1..300, ties, bit-exact).
* `applyDefaultFX` (a document's FX chain, the app's voice renders too) makes
  one block call per effect instead of one per sample. Guard
  `TestDocumentFXBlockMatchesPerSample` (every effect at its defaults, both
  ends and a random point of every parameter, random chains); every shipped
  voice at pitch 0, +7 and −5 hashed identical before/after (450 renders; 19
  voices carry FX).
* The measured voice frames persist in `$BEATMO_WAV_REFS/matches/frames.json`,
  keyed by the refs binary's own hash (a rebuilt matcher measures afresh; each
  entry's sound hash covers a retuned voice): ~1.8 s off every invocation
  after the first. Guard `TestMatchFramesPersistPerBinary`.

### Removed: settling every voice

Every voice was rendered at EVERY semitone −12..+12 against all 64
references (107,172 render+analyses), and each search strategy replayed
offline on that landscape, graded leave-one-out exactly as `-validate` grades
(the replay reproduces the Go validate's numbers):

| strategy | cost | #1 | top 3 | MRR | kind #1 | top 3 same as current |
|---|---|---|---|---|---|---|
| current: step 3 + refine 2,1 | 1.00 | 19 | 26 | 0.393 | 34 | 64/64 |
| exhaustive, every semitone | 1.96 | 20 | 26 | 0.400 | 34 | -- |
| step 6 + refine 2,1 (everyone) | 0.72 | 19 | 26 | 0.393 | 34 | -- |
| **scout step 6, settle top 30** | **0.57** | **19** | **26** | **0.393** | **34** | **64/64** |
| scout step 6, settle top 10 | 0.52 | 19 | 26 | 0.392 | 34 | 63/64 |
| scout {−12, 0, +12}, settle top 30 | 0.45 | 19 | 26 | 0.392 | 34 | 64/64 (piano-felt 22 → 60) |
| scout step 4, settle top 30 | 0.73 | 20 | 26 | 0.400 | 34 | 59/64 |

Most of the cost was the full search of voices that were never in the
running. `rankRoster` now runs in two phases:

1. **Scout** every voice: its whole fixed plan (one note, or a non-melodic
   voice's note and its octaves), or, for a search, every 6 semitones
   (−12, −6, 0, 6, 12 -- a subset of the search grid).
2. **Settle** the 30 closest by the hub-corrected distance, plus any id the
   caller names: the full search, which lands exactly where the unpruned
   search would (the scouted pitches are grid points and ties keep the
   earlier pitch -- `TestScoutedSearchSettlesWhereAFullSearchDoes`).

The first version scouted a fixed plan at its primary pitch only, as the
replay modelled it, and graded identically on the 64 references -- the octave
alternatives never matter there. They exist for a reading an octave off:
`TestEssenceSelfIdentifiesUnderNuisance` plays the tom +5 st, slower, in a
room, YIN reads its 85 Hz residue, and the tom, scouted at that pitch alone,
ranked itself #31 -- the first rank past the contenders (#1 when everyone
settles). A fixed plan is at most three renders, so it is never cut short;
the scout guard fails on the primary-pitch scout.

Every other voice keeps its scouted distance, which can only be worse than
its settled one, so (with the committed hubs) it ranks behind all 30
contenders: the cut decides only whether a distance below #30 is exact. Who settles what:

| caller | settles |
|---|---|
| `refs match`, `-map` | the top 30 + every instrument the recording is LABELLED as (catalogue / filename, with its family), so the map's fidelity-gap ranks are exact |
| `-explain a,b` | the top 30 + the named instruments |
| `-validate`, `-nuisance` | the top 30 (they grade what production ranks) |
| `-calibrate`, `refs check -only match-hubness` | everyone (`matchSettleAll`): the committed hubness stays the exhaustive calibration |

`TestRankRosterSettlesTheContenders` pins that the pruned top 3 is the
exhaustive top 3, that no voice scores closer pruned than exhaustive, and
that a named voice is exact even when its scout missed.

Grading (the shipped code, Go `-validate` against the exhaustive build):
identical headline (19 / 26 / 0.393 / 34), and in the replay an identical top
3 on 64/64 references with the committed hubs. Under `-validate`'s
leave-one-out hubs, which now come from pruned rows, eight family ranks move,
all but one pair below the top 3: conga 29 → 30, dnb-kick 15 → 13, kick-akt
1 → 2 with kick-al-bd 2 → 1, organ-church 67 → 68, shaker 63 → 79, sidestick
47 → 45, tuba 64 → 63. The nuisance sweep (49 voices × 10 cases) gives the
same self-rank in 489 of 490 cells; the one that moves, conga under reverb, is
#29 pruned vs #32 exhaustive. `TestEssenceSelfIdentifiesUnderNuisance`: 16/16
in the top 3.

Wall time, 6 workers, same machine, back to back:

| run | before | exact changes only | + two-phase + frame cache |
|---|---|---|---|
| `refs match -validate` (64 refs) | 375 s | 309 s (output byte-identical) | **175 s** (2.1×) |
| `refs match` one pitched download (harp) | 5.5 s | | **2.3 s** with the frame cache warm (3.8 s cold) |
| `refs match` one unpitched download | 20.2 s | | **9.9 s** |
| `refs match -nuisance` (490 rankings) | | 2,821 s | **1,616 s** (0.57×) |

The single-file rankings are identical before and after (same top 3, same
scores).

### Tried, or ruled out, and not worth retrying

* A coarser scout, {−12, 0, +12}: 0.45× cost, but it drops piano-felt from
  family rank 22 to 60 (a best pitch mid-octave is invisible to an octave grid).
* A denser search: exhaustive is 1.96× for +1 #1 and MRR +0.007, inside the
  ±0.03 noise of n = 64.
* More than 6 workers (same wall), `GOGC` (3 %).
* Caching renders ACROSS references by (instrument, pitch): invalid,
  `synthmatch.VoiceBPMFor` sizes the note to the clip, so a kick renders
  differently against a 0.4 s and a 2 s reference.

### Left on the table

* The C voice is now about half of every ranking; `hihat-open-bright` and
  `ride` render 3-5× slower than the median voice.
* A real-input FFT would halve the analysis FFTs, but it is not bit-identical
  (every distance moves in the last bits → recalibrate the hubness).
* Longer STFT hops cut the analysis further, but
  change what the essence measures; they need the full validate + nuisance
  pass like any other change to the distance.

## 2026-10-18 — the misses, by name, and what is signal

`RESUME=false make wav-match` on the 18 downloads still showed "weird
misses": the oboe A 440 → bassoon, the celtic harp → tuba, the piano G →
flute, the violin D5 → accordion, the cello D2 → double-bass. Each miss was
traced against what the recording IS (its catalogue entry, else its
filename), generically:

* **A per-term landscape.** Every voice at every semitone of its plan against
  all 64 catalogued references, with every essence term stored (107,172
  render+analyses, 13 min). Re-weighting, dropping a term or changing the hub
  is then an exact offline replay (it reproduces `-validate`'s 19 / 26 /
  0.393 / 34), and every miss can be attributed to the term that decided it
  (own voice − winner, per term, and the hub's share).
* **Real against real.** The essence distance between every pair of
  catalogued recordings, the candidate analysed at the reference's window,
  exactly like a render. Recordings of one instrument at matching pitches
  (the cello pair, the horn pair, the trumpets, the violins, the pianos, the
  kicks, the snares) should find each other: that grades the DISTANCE without
  any synth in the way.

### Why they miss

| cause | recordings | evidence |
|---|---|---|
| **pitch gate bug** (fixed) | the acclivity oboe A 440 | read 443 Hz correctly, then declared unpitched: the gate required the LOUDEST partial to be harmonic 1..5, and a double reed's formant puts it at the 6th (2.66 kHz; the 2nd and 3rd sit 2-3 dB under it). The same rule declared the synth cello and bassoon unpitched at their own frames (harmless there: the declared frame equals the measured one). |
| the label is not the instrument, or not its octave | harp "g2" (sounds G5: nothing at 98 Hz, partials 777 + 1553 Hz); g1_rr1 (catalogued `singing-bowl` as a GUESS -- it is a harmonic series on 49 Hz, which no singing bowl is); koto (a dan tranh glissando, no note); kalimba (an mbira with buzz); glass armonica (wine glasses); pan flute (a recorder) | pitch probe on every recording; the catalogue notes |
| **synth fidelity** -- the own voice's colour/noise is off | most: tuba, trombone, viola, organ-church, oboe, flute, pan flute, cello, violin, piano, sax, clarinet, kicks, sidestick, shaker | colour decides 30 of the 45 misses (texture, decay, evolution and attack 1-3 each, the hub 5); real recordings of the same instrument find EACH OTHER first (cello, horn, trumpet, violin, piano pairs all #1; same-kind nearest 27 of 46); and the directions are the ones the ear wrote down: oboe "way too airy/breathy" (noise 0.13 vs 0.02), cello "too airy and breathy", flute "very breathy" (0.31 vs 0.02), violin/viola "slightly fluty", harp "needs much longer decay… brighter" |
| the hub bonus of a voice far from every recording | guitar-nylon "2-oct-c" → piano-grand (raw: the guitar is closer, 0.683 vs 0.839; piano-grand's hub 1.07 is worth 0.53), g1_rr1 → fm-bass-drive (hub 1.24), voice-ahh → cowbell-drive | the known cost of CSLS; capping it is worse (below) |
| register | the harp G5 → tuba at G5 (a 0.005 tie), flute-breathy E5 → double-bass at E5, sax A4 → moog-bass | melodic voices are rendered at any note; a bass voice two octaves above its range is near-sinusoidal and matches a sparse high note. No range data exists to restrict it, and adding 150 ranges is not a simplification -- left as is |

### What is signal

Exact replays (synth vs real, 64 references, leave-one-out hubs) and the
real-vs-real retrieval (46 recordings with a same-kind partner at a matching
pitch):

| variant | synth vs real: #1 / top 3 / MRR / kind #1 | real vs real: same-kind #1 / MRR |
|---|---|---|
| all terms (as shipped) | 19 / 26 / 0.393 / 34 | 27 / 0.707 |
| without colour | 8 / 17 / 0.249 / 17 | 14 / 0.486 |
| without evolution | 16 / 25 / 0.365 / 28 | 29 / 0.725 |
| **without harmonics** | **19 / 26 / 0.394 / 32** | **26 / 0.701** |
| without texture | 18 / 25 / 0.380 / 31 | 26 / 0.677 |
| without decay | 18 / 23 / 0.379 / 32 | 27 / 0.708 |
| without attack | 17 / 26 / 0.374 / 33 | 28 / 0.722 |
| colour only | 12 / 16 / 0.275 / 25 | 25 / 0.667 |
| no hub correction | 15 / 20 / 0.325 / 28 | -- |

Colour carries most of the identity; evolution, decay, texture and attack
each pay for themselves on the synth-vs-real task (and evolution/attack are
neutral real-vs-real). The harmonic-profile term pays for nothing on either:
the ERB bands already resolve the harmonics the ear resolves, so it counted
them twice.

### Removed

* **The harmonic-profile term** (`Essence.Harm`, `EssenceScore.Harmonics`,
  the 16384-point FFT per clip). `AnalyzeEssence` no longer takes a pitch at
  all -- the pitch only decides where a voice is rendered.
* **The harmonic sieve** (`harmonicSieve`). It changed no reading on any of
  the 82 recordings (catalogue + downloads); its motivating case, the synth
  kalimba flipping between divisors, is already declined by the explained-
  energy gate; and on synth drums it read the membrane's 1 : 1.5 modes as an
  octave-low "fundamental" (tom 64 Hz for 128, tumba 55 for 110, the singing
  bowl D2 for A3).
* **The legacy distances** (`timbre`, `full`, `perc`, `fused`, the `-metric` /
  `-metrics` flags, `METRIC=` / `METRICS=`): the A/B against them is recorded
  above; they were ~10× slower and never the default.
* `-validate`'s `essence-raw` column (the hub's worth is the "no hub
  correction" row).

### Changed

* The dominance rule of `pitchExplains`: a partial within 6 dB of the
  strongest must be one of harmonics 1..5 (was: the strongest itself). The
  real mbira, read as 124 Hz, is still declined (its one loud partial, 862 Hz,
  would be the 7th); the oboe is A4.
* The `refs match` table's NAMED column is now LABELLED: what the catalogue
  (or filename) says the recording is, the rank of its closest family member,
  and that member's clip is always written (`labelCell`).

### Result

| | before | after |
|---|---|---|
| `-validate` family #1 / top 3 / MRR / kind #1 / kind top 3 | 19 / 26 / 0.393 / 34 / 40 | 20 / 26 / 0.402 / 32 / 44 |
| `TestEssenceSelfIdentifiesUnderNuisance` | 16 / 16 | 16 / 16 (12 at #1) |
| `-nuisance` (49 voices × 10 cases): itself #1 / in the top 3 | 453 / 483 of 490 | 451 / 484 (reverb #1 33 → 35; bright mic #1 46 → 43, plucked strings #1 → #2; 1,616 → 1,524 s) |
| the oboe A 440 download | unpitched, oboe #11 | A4 443 Hz, oboe #9 (the synth oboe's own fidelity gap) |
| validate wall time | 175 s | 175 s |

Every changed reference was already a miss except kick-akt (#2 → #1);
movements among the misses run both ways (double bass #40 → #25, organ #68 →
#52, harpsichord #21 → #8; flute-breathy #21 → #24, pan flute #13 → #18) --
inside the noise of n = 64, as the replay predicted.

### Tried and not worth retrying

* Capping the hub so far-from-everything voices get no bonus: at the median
  0.364 MRR, at p75 0.383, clamped to the IQR 0.357, against 0.394 -- the
  anti-hub bonus is right more often than the misses it causes.
* Hub ¼ (0.359) or ¾ (0.398, kind #1 31): no better than ½.
* Dropping any of colour / evolution / texture / decay / attack (table above).
