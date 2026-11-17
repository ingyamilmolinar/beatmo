# Candidate evidence — 2026-09-04 targeted battery & audition (Task 15)

This is the sheet the listener reads next to the audition session. For each of
the three families under audition it holds:

1. **Caveats for the listener** — what the numbers below cannot tell you for
   this family: gate exemptions and why, axes that do not measure at all, axes
   that carry the tool's own reliability caveat, and any organic-range exit.
   Read this first.
2. The **current version's** full `refs diagnose -ablate` output.
3. Each candidate's **overrides** and the **hypothesis** that cites them, then
   that candidate's full `refs diagnose` output and its `refs organic`
   placement.

Every fenced block below is verbatim tool output, captured by re-running the
command shown on its first line. Nothing is retyped or summarised into a table,
so the NOTE column — including the reliability CAVEATs the tool attaches to
some rows — is intact. A row that is absent from a block is absent because the
tool did not emit it, and the family's caveats section says why.

"Bound axes" are the five descriptors the family's audition questions are bound
to in `cmd/refs/session_questions.go`; they are what
`TestShippedCandidates_PassPreAuditionGate` counts, minus that family's
exemptions.

Reproduce any block with (from `src/go`, with the corpus env):

```
BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs AUDIO_SAMPLE_RATE=48000 refs <the command shown>
```

`-override` on both `diagnose` and `organic` was added by this task so a
proposed candidate can be measured without editing its seed — the candidates
live only in `internal/audio/testdata/audition_candidates.json`, and unknown
override keys are rejected rather than silently dropped.

**Read the numbers with the thresholds in mind.** Task 8 calibrated the verdict
bands against the corpus, which LOOSENED several verdict-anchored axes: the
oboe's `noise persistence` 0.860 vs 0.338 (2.54x) reads `ok` on the current
version even though the listener's complaint is exactly that breathiness. The
gate therefore also applies a threshold-independent DIRECTION criterion on the
listener-named axes per family — bells `mode.beating_depth` /
`mode.count_per_octave`, snare `noise.flatness` / `env.decay_t20`, oboe
`noise.persistence` / `timbre.brightness_ratio` — requiring a candidate to
close at least 25% of the |synth − reference| gap on one of them and to widen
none by more than 25%. Those percentages are quoted in each hypothesis.

---


## tubular-bells

Listener's words this round: **a simple clean tone where the reference is a resonant, wavy, metallic ring — and it should PROLONG**

Bound axes (`familyQuestions["tubular-bells"]`): `band.t60`, `mode.beating_depth`, `mode.count_per_octave`, `timbre.brightness_ratio`, `onset.bite_ratio`.

### Caveats for the listener

- **`timbre.brightness_ratio` is EXEMPT from this family's gate.** The axis is
  `SpectralCentroid / F0Hz`. The synth's centroid is 1314 Hz against the
  reference's 1855 Hz — already the DARKER of the two — so the whole 4.58x
  delta lives in the denominator: on this inharmonic bell `DetectF0` latches
  144 Hz, a quarter of the 577 Hz mode anchor. Overrides that "fix" the ratio
  move the centroid only 1314 -> 1334 Hz; what they actually do is feed the
  detector a harmonic skeleton the reference does not have, which drags
  `mode anchor` from ok (+1%) to drift (-19%). Treat the brightness question as
  unanswered by the numbers here and judge it purely by ear.
- **Both candidates deliberately leave the ring alone.** `amp_decay` and
  `amp_release` are untouched, so `ring T60` stays ok (2.98 s vs the
  reference's 2.97 s). An earlier draft shortened them to 1.6 s and turned
  `ring T60` into a MISMATCH reading "decays far too fast" — the exact opposite
  of the listener's "prolongs". `gen1_freq` (the hum partial, derived from this
  same reference) is untouched for the same reason: moving it to 0.5x made
  `DetectF0` lock but pushed `mode anchor` to -19%.
- **`band T60` is still drift on A** (mean 2.43x long, worst 4k 6.52x). B
  clears it. If A sounds right otherwise, that axis is the next thing to move.
- **The strike gets MUCH clankier on both candidates — this is the likeliest
  ear disagreement.** `onset bite` reads 660.6 on the current version against
  a reference of 29.4 (22.50x); it goes to **2539.9 on A (86.52x)** and
  **2635.7 on B (89.78x)**. Both stay `drift` on either side of the move, so
  no gate criterion fires and it is not one of this family's direction axes —
  but the direction is unambiguously WRONG and the size is large. The
  mechanisms differ: A adds four partials that all sound at the onset, and B
  shortens the upper modes so there is less energy in the 300-600 ms window
  the ratio divides by. Listen for a hard metallic clank on the attack; if
  either candidate reads as "too much strike", this row predicted it.
- **`mode density` is one of this family's gate DIRECTION axes, and it scores
  ANTI on the verdict corpus.** `refs validate` puts it at AUC **0.05** — the
  lowest of every axis in the battery, and far enough below 0.50 to be ordered
  BACKWARDS rather than merely uninformative: on the families the listener
  graded, the ones that sounded WORSE read BETTER on this axis. So a candidate
  that moves mode density toward the reference may be moving the sound away
  from what the listener wants, and the gate's direction criterion cannot tell
  you which. Read the "metallic complexity" question as the authority here and
  the number as a hypothesis. (The corpus is small — 8 good and 6 bad families
  — so 0.05 is not a settled measurement either; what it rules out is treating
  the direction as trustworthy.)
- **Organic range:** see the placement blocks below. The current version
  already sits outside the real-recording band on `modulation bands` (above),
  `modulation flatness` (below), `onset flatness` (below) and
  `residual centroid` (below).

### Current version — full `refs diagnose -ablate`

```
$ refs diagnose -instrument tubular-bells -ref tubular-bells-vcsl -ablate

=== tubular-bells (reference: tubular-bells-vcsl) ===

NOTE: the reference clip is TRUNCATED to 3.00s by the corpus's fetch config (convert.max_sec) --
the real instrument recording may ring/decay further than this reference can show. Pass
-dur <seconds> to render the SYNTH for longer (the reference itself cannot be extended without
re-fetching from the network).

-- noise / harmonic balance --
AXIS                 REFERENCE  SYNTH    DELTA   VERDICT  NOTE
noise fraction       0.875      0.850    -0.025  drift    synth is too pure/tonal (missing noise character) (CAVEAT: reference partial series deviation=0.52 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
spectral flatness    0.030      0.000    -0.030  ok       (CAVEAT: reference partial series deviation=0.52 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
residual centroid    1920 Hz    1362 Hz  -29%    ok       
noise decay vs tone  0.964      n/a      n/a              NOT COMPARABLE -- measurable on the reference but NOT on the synth: both tracks still above -20 dB at clip end. That asymmetry is itself the finding: the synth's signal character differs from the reference's.

-- temporal envelope --
AXIS             REFERENCE   SYNTH        DELTA                   VERDICT  NOTE
attack           0.020 s     0.010 s      2.0x short              ok       
ring T60         2.970 s     2.980 s      rate -4.3 -> -5.0 dB/s  ok       T60 NOT OBSERVABLE: both clips still ring above -60dB at clip end (ref clamped at 2.97s, synth at 2.98s) -- comparing overall decay RATE instead (ref -4.3 dB/s, synth -5.0 dB/s)
release          2.590 s     2.320 s      1.1x short              ok       
decay T10        1.270 s     2.170 s      1.7x long               ok       
decay T20        n/a         n/a          n/a                              NOT MEASURABLE on either side: clip ended before -20 dB was reached (still above it at 2.97s), so this is a lower bound rather than a measured decay time
decay T30        n/a         n/a          n/a                              NOT MEASURABLE on either side: clip ended before -30 dB was reached (still above it at 2.97s), so this is a lower bound rather than a measured decay time
energy sections  0.600       0.508        mean|Δ| 0.052           ok       
decay            0.140 s     0.170 s      1.2x long               ok       
decay rate       -31.9 dB/s  -11.07 dB/s  +20.83                  drift    synth is too high on this axis
peak time        0.020 s     0.010 s      2.0x short              ok       
swell            0.000 s     0.000 s      n/a                     ok       
total audible    3.000 s     3.000 s      1.0x long               ok       

-- partial structure --
AXIS                      REFERENCE  SYNTH   DELTA    VERDICT   NOTE
inharmonicity             0.009      0.004   -0.005   MISMATCH  synth partials are too harmonic (needs inharmonic bar/tube modes)
partial series deviation  0.523      1.653   +1.130   drift     synth partials skip ranks worse than the reference (less like a consecutive harmonic series)
spectral irregularity     11.444     33.699  +22.254  drift     synth is too high on this axis
  ref partials (x f0):   0.62, 1.01, 1.47, 1.71, 2.01, 2.61, 3.26
  synth partials (x f0): 4.01, 6.47, 6.50, 9.51, 11.07, 13.01, 16.89, 21.09

-- brightness / formant --
AXIS      REFERENCE  SYNTH    DELTA  VERDICT  NOTE
centroid  1855 Hz    1314 Hz  -29%   ok       
rolloff   1873 Hz    1593 Hz  -15%   ok       

-- modulation --
AXIS                         REFERENCE  SYNTH     DELTA          VERDICT   NOTE
sustain modulation           0.058      0.229     +0.171         drift     synth sustain wobbles more than the reference
loudness modulation rate     1.008 Hz   4.703 Hz  +3.695         MISMATCH  synth is too high on this axis
loudness modulation depth    0.079      0.134     +0.055         ok        
modulation bands             0.976      0.019     mean|Δ| 0.641  MISMATCH  
modulation flatness          0.117      0.062     -0.055         drift     synth is too low on this axis
brightness modulation depth  0.227      0.027     -0.199         drift     synth is too low on this axis

-- resolved decay --
AXIS                      REFERENCE            SYNTH                 DELTA                              VERDICT  NOTE
decay per mode            -1.921 dB/s          -4.57 dB/s            mean|Δ| 14.5 dB/s 14.530           ok       
decay slope vs frequency  -8.325 dB/s per kHz  -0.9659 dB/s per kHz  +7.359                             ok       
band T60                  -1.000 s             -1.000 s              mean 2.43x (worst 4k 6.52x) 1.280  drift    synth is too high on this axis

-- partial decorrelation --
AXIS                  REFERENCE  SYNTH  DELTA   VERDICT  NOTE
partial independence  0.884      0.168  -0.716  drift    synth is too low on this axis

-- struck string --
AXIS                   REFERENCE    SYNTH    DELTA            VERDICT   NOTE
string stiffness B     0.001        0.000    -0.000           drift     synth is too low on this axis
double-decay ratio     3.976        38.070   9.58x            drift     synth is too high on this axis
prompt decay rate      28.606       28.451   -0.155           ok        
aftersound decay rate  7.195        0.747    -6.448           ok        
partial stretch        9.118 cents  0 cents  mean|Δ| 159.901  MISMATCH  

-- wind attack --
AXIS                        REFERENCE  SYNTH    DELTA             VERDICT   NOTE
attack rise time            0.006 s    0.000 s  n/a               MISMATCH  synth is too low on this axis
attack brightening          -7040 Hz   259 Hz   n/a               MISMATCH  synth is too high on this axis
temporal centroid           1.123 s    1.122 s  1.0x short        ok        
attack centroid trajectory  3974 Hz    1332 Hz  mean|Δ| 2018.673  MISMATCH  

-- modal structure --
AXIS              REFERENCE  SYNTH     DELTA                       VERDICT  NOTE
mode anchor       574 Hz     577 Hz    +1%                         ok       
mode ratios       0 cents    0 cents   mean gap 271 cents 270.533  ok       
mode density      1.898      1.107     0.58x                       ok       
mode harmonicity  0.032      0.086     +0.054                      ok       
beating rate      1.05 Hz    4.745 Hz  +3.694                      drift    synth is too high on this axis
beating depth     0.611      0.490     -0.121                      drift    synth is too low on this axis

-- onset character --
AXIS               REFERENCE  SYNTH    DELTA   VERDICT  NOTE
onset bite         29.357     660.612  22.50x  drift    synth is too high on this axis
onset flatness     0.328      0.373    +0.045  ok       
onset pitch glide  n/a        n/a      n/a              NOT MEASURABLE on either side: pitch not detectable in both onset windows

-- perceptual character --
AXIS               REFERENCE   SYNTH         DELTA          VERDICT   NOTE
brightness ratio   1.991       9.127         4.58x          MISMATCH  synth is too high on this axis
harmonic slope     -21 dB/oct  10.28 dB/oct  +31.28         MISMATCH  synth is too high on this axis
tristimulus        0.758       0.000         mean|Δ| 0.505  drift     
noise persistence  0.534       1.006         1.88x          ok        
air-band flatness  0.203       0.001         -0.202         drift     synth is too low on this axis

-- vibrato --
AXIS              REFERENCE  SYNTH  DELTA  VERDICT  NOTE
vibrato rate      n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato extent    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato jitter    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato AM depth  n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)

-- formants / body resonance --
AXIS            REFERENCE  SYNTH     DELTA  VERDICT   NOTE
first formant   1292 Hz    1564 Hz   +21%   ok        
second formant  3470 Hz    664.8 Hz  -81%   MISMATCH  synth is too low on this axis
bridge hill     n/a        2869 Hz   n/a              NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.

-- cepstral timbre --
AXIS          REFERENCE  SYNTH     DELTA           VERDICT   NOTE
MFCC profile  135.724    -128.784  mean|Δ| 56.045  MISMATCH  

-- whole-signal statistics --
AXIS                  REFERENCE      SYNTH           DELTA       VERDICT   NOTE
centroid slope        -477.1 Hz/s    -41.9 Hz/s      +435.2      ok        
centroid curvature    535.8 Hz/s²    5.882 Hz/s²     -529.9      ok        
centroid spread       186.8 Hz       107.4 Hz        -42%        ok        
loudness slope        -0.03759 1/s   -0.03741 1/s    +0.0001818  ok        
loudness curvature    0.01633 1/s²   -0.001298 1/s²  -0.01763    ok        
loudness gesture      0.642          0.524           -0.118      ok        
loudness spread       0.035          0.058           +0.023      ok        
flatness slope        -0.009727 1/s  -0.0001267 1/s  +0.009601   ok        
flatness median       0.016          0.000           -0.015      ok        
noise-fraction slope  -0.08447 1/s   0.0009648 1/s   +0.08544    drift     synth is too high on this axis
peak noise fraction   0.371          0.851           +0.480      MISMATCH  synth is too high on this axis
crest median          2.128          2.788           1.31x       drift     synth is too high on this axis
harmonic bloom        -10.74 dB/s    0.2464 dB/s     +10.99      MISMATCH  synth is too high on this axis


-- stage sensitivity (which stage moves each MISMATCH axis; not a decomposition, does not sum) --
AXIS                        TOP STAGES (Δnorm above the no-effect floor when the stage is at identity)
inharmonicity               gen6 1.68  gen4 1.28  gen2 1.15
partial stretch             gen4 0.97  gen2 0.41  gen3 0.03
attack rise time            gen2 1.00  gen3 1.00  gen1 0.00
attack brightening          gen2 2.97  gen3 2.96  gen1 2.72
attack centroid trajectory  gen1 0.08  gen8 0.06  gen7 0.06
loudness modulation rate    gen2 5.00  gen3 5.00  gen1 0.00
modulation bands            gen2 1.29  gen3 1.29  gen1 0.01
brightness ratio            gen4 1.99  gen2 0.03  gen7 0.02
harmonic slope              gen4 4.30  gen6 0.68  gen7 0.21
second formant              gen1 8.48  gen5 5.27  gen4 4.44
MFCC profile                gen1 0.70  gen8 0.50  gen4 0.32
peak noise fraction         gen1 0.90  gen5 0.82  gen2 0.60
harmonic bloom              gen4 0.12  gen1 0.04  gen2 0.02
```

<details><summary>Current version — <code>refs organic</code> placement</summary>

```
$ refs organic -instrument tubular-bells -pitch 0

=== tubular-bells — organic axes against the real-recording range (p10–p90 over 59 references) ===

AXIS                         VALUE   REAL p10  p50      p90       WHERE   POSITION
loudness modulation rate     1.01    0.672     0.672    1.34      inside  +0.50
loudness modulation depth    0.117   0.037     0.24     1.34      inside  -0.09
modulation bands             0.991   0.448     0.839    0.928     above   +0.32
modulation flatness          0.175   0.183     0.488    0.604     below   -0.74
brightness modulation depth  0.0212  0.0162    0.0823   0.41      inside  -0.16
partial bandwidth            n/a                                          does not apply to this signal
partial independence         0.266   0.0917    0.503    1         inside  -0.26
onset bite                   8.8     0.0708    1        1.6e+03   inside  +0.00
onset flatness               0.0684  0.114     0.26     0.407     below   -0.65
onset pitch glide            n/a                                          pitch not detectable in both onset windows
residual centroid            352     1.92e+03  3.1e+03  5.58e+03  below   -0.75
noise decay vs tone          n/a                                          both tracks still above -20 dB at clip end
noise persistence            0.973   0.101     0.598    2.27      inside  +0.17
vibrato jitter               n/a                                          no vibrato detected (band power below the noise floor, or extent under 2 cents)
crest median                 2.43    1.65      2.38     3.24      inside  +0.03

POSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; 'low-n (n=k)' means fewer than 10 references qualified for that axis, too few to trust a band.
'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.
```

</details>

### Candidate A — the reference's missing modes, real beating

Overrides:

```json
{
  "lfo_enabled": 0,
  "gen3_freq": 1.004,
  "gen9_source": 1,
  "gen9_freq": 3.4527,
  "gen9_gain": 0.14,
  "gen9_env_fast_mix": 1,
  "gen9_env_fast_rate": 0.34,
  "gen10_source": 1,
  "gen10_freq": 3.9711,
  "gen10_gain": 0.12,
  "gen10_env_fast_mix": 1,
  "gen10_env_fast_rate": 0.4,
  "gen11_source": 1,
  "gen11_freq": 4.6961,
  "gen11_gain": 0.1,
  "gen11_env_fast_mix": 1,
  "gen11_env_fast_rate": 0.48,
  "gen12_source": 1,
  "gen12_freq": 5.188,
  "gen12_gain": 0.08,
  "gen12_env_fast_mix": 1,
  "gen12_env_fast_rate": 0.54
}
```

Hypothesis (verbatim from `audition_candidates.json`):

> The listener heard 'a simple clean tone' where the reference is 'a resonant wavy metallic ring', and two bound axes say the same thing: mode density 1.107 vs 1.898 (0.58x -- the 'too plain' end of the metallic-complexity question) and beating depth 0.490 vs 0.611 ('too steady'). Reading the reference's own mode ratios out of the descriptor (12 modes at 0, 846, 1509, 1768, 2048, 2498, 2882, 2973, 3215, 3505, 3678, 4178 cents above its 574 Hz anchor) against the synth's (only the first 7: 0, 831, 1495, 1757, 2037, 2490, 2874) shows exactly what is missing: the synth reproduces the reference's lower seven modes to within ~15 cents and has NO partner for its top five. So the mean 271-cent 'mode ratios' gap is not mistuning, it is absence. Slots gen9-gen12 are unused, so the four lowest missing modes go in at the reference's measured ratios -- 2973/3215/3505/3678 cents = 3.4527/3.9711/4.6961/5.1880 x the voice fundamental, each with its own env_fast_rate so it decays independently: mode density 1.107 -> 1.582 (gap to the reference -60%). For the waviness, gen3 is the gen2 beat partner; moving it 1.005 -> 1.0040 halves the beat interval and beating depth reads 0.620 vs 0.611 (drift -> ok, gap -92%), with lfo_enabled 0 so the wobble is REAL partial beating rather than a 3.2 Hz tremolo layered on top. Nothing here touches amp_decay/amp_release or gen1_freq: ring T60 stays ok at 2.98 s vs 2.97 s and mode anchor stays ok at 577 Hz vs 574 Hz. The PREDICTIVE axis improves too -- inharmonicity 0.00398 -> 0.00450 against the reference's 0.00928 -- because the added modes are inharmonic. band T60 and onset bite remain the two bound drifts.

<details><summary>Full <code>refs diagnose</code> output for candidate A</summary>

```
$ refs diagnose -instrument tubular-bells -ref tubular-bells-vcsl -override lfo_enabled=0,gen3_freq=1.004,gen9_source=1,gen9_freq=3.4527,gen9_gain=0.14,gen9_env_fast_mix=1,gen9_env_fast_rate=0.34,gen10_source=1,gen10_freq=3.9711,gen10_gain=0.12,gen10_env_fast_mix=1,gen10_env_fast_rate=0.4,gen11_source=1,gen11_freq=4.6961,gen11_gain=0.1,gen11_env_fast_mix=1,gen11_env_fast_rate=0.48,gen12_source=1,gen12_freq=5.188,gen12_gain=0.08,gen12_env_fast_mix=1,gen12_env_fast_rate=0.54

=== tubular-bells (reference: tubular-bells-vcsl) ===

NOTE: the reference clip is TRUNCATED to 3.00s by the corpus's fetch config (convert.max_sec) --
the real instrument recording may ring/decay further than this reference can show. Pass
-dur <seconds> to render the SYNTH for longer (the reference itself cannot be extended without
re-fetching from the network).

-- noise / harmonic balance --
AXIS                 REFERENCE  SYNTH    DELTA   VERDICT  NOTE
noise fraction       0.875      0.866    -0.009  ok       (CAVEAT: reference partial series deviation=0.52 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
spectral flatness    0.030      0.000    -0.030  ok       (CAVEAT: reference partial series deviation=0.52 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
residual centroid    1920 Hz    1798 Hz  -6%     ok       
noise decay vs tone  0.964      n/a      n/a              NOT COMPARABLE -- measurable on the reference but NOT on the synth: both tracks still above -20 dB at clip end. That asymmetry is itself the finding: the synth's signal character differs from the reference's.

-- temporal envelope --
AXIS             REFERENCE   SYNTH        DELTA                   VERDICT  NOTE
attack           0.020 s     0.010 s      2.0x short              ok       
ring T60         2.970 s     2.980 s      rate -4.3 -> -5.0 dB/s  ok       T60 NOT OBSERVABLE: both clips still ring above -60dB at clip end (ref clamped at 2.97s, synth at 2.98s) -- comparing overall decay RATE instead (ref -4.3 dB/s, synth -5.0 dB/s)
release          2.590 s     2.170 s      1.2x short              ok       
decay T10        1.270 s     2.180 s      1.7x long               ok       
decay T20        n/a         n/a          n/a                              NOT MEASURABLE on either side: clip ended before -20 dB was reached (still above it at 2.97s), so this is a lower bound rather than a measured decay time
decay T30        n/a         n/a          n/a                              NOT MEASURABLE on either side: clip ended before -30 dB was reached (still above it at 2.97s), so this is a lower bound rather than a measured decay time
energy sections  0.600       0.499        mean|Δ| 0.057           ok       
decay            0.140 s     0.220 s      1.6x long               ok       
decay rate       -31.9 dB/s  -8.136 dB/s  +23.76                  drift    synth is too high on this axis
peak time        0.020 s     0.010 s      2.0x short              ok       
swell            0.000 s     0.000 s      n/a                     ok       
total audible    3.000 s     3.000 s      1.0x long               ok       

-- partial structure --
AXIS                      REFERENCE  SYNTH   DELTA    VERDICT   NOTE
inharmonicity             0.009      0.005   -0.005   MISMATCH  synth partials are too harmonic (needs inharmonic bar/tube modes)
partial series deviation  0.523      2.128   +1.605   MISMATCH  synth partials skip ranks worse than the reference (less like a consecutive harmonic series)
spectral irregularity     11.444     33.579  +22.134  drift     synth is too high on this axis
  ref partials (x f0):   0.62, 1.01, 1.47, 1.71, 2.01, 2.61, 3.26
  synth partials (x f0): 4.01, 6.48, 9.52, 11.07, 13.01, 16.90, 22.35, 25.71

-- brightness / formant --
AXIS      REFERENCE  SYNTH    DELTA  VERDICT  NOTE
centroid  1855 Hz    1684 Hz  -9%    ok       
rolloff   1873 Hz    1593 Hz  -15%   ok       

-- modulation --
AXIS                         REFERENCE  SYNTH     DELTA          VERDICT   NOTE
sustain modulation           0.058      0.226     +0.168         drift     synth sustain wobbles more than the reference
loudness modulation rate     1.008 Hz   3.695 Hz  +2.688         MISMATCH  synth is too high on this axis
loudness modulation depth    0.079      0.137     +0.058         ok        
modulation bands             0.976      0.018     mean|Δ| 0.645  MISMATCH  
modulation flatness          0.117      0.052     -0.065         drift     synth is too low on this axis
brightness modulation depth  0.227      0.037     -0.190         drift     synth is too low on this axis

-- resolved decay --
AXIS                      REFERENCE            SYNTH                DELTA                              VERDICT  NOTE
decay per mode            -1.921 dB/s          -4.57 dB/s           mean|Δ| 21.0 dB/s 21.047           ok       
decay slope vs frequency  -8.325 dB/s per kHz  -1.031 dB/s per kHz  +7.294                             ok       
band T60                  -1.000 s             -1.000 s             mean 2.36x (worst 4k 5.98x) 1.239  drift    synth is too high on this axis

-- partial decorrelation --
AXIS                  REFERENCE  SYNTH  DELTA   VERDICT  NOTE
partial independence  0.884      0.173  -0.710  drift    synth is too low on this axis

-- struck string --
AXIS                   REFERENCE    SYNTH    DELTA            VERDICT   NOTE
string stiffness B     0.001        0.000    -0.000           drift     synth is too low on this axis
double-decay ratio     3.976        100.841  25.36x           MISMATCH  synth is too high on this axis
prompt decay rate      28.606       45.043   +16.437          ok        
aftersound decay rate  7.195        0.447    -6.748           ok        
partial stretch        9.118 cents  0 cents  mean|Δ| 121.543  drift     

-- wind attack --
AXIS                        REFERENCE  SYNTH    DELTA             VERDICT   NOTE
attack rise time            0.006 s    0.000 s  n/a               MISMATCH  synth is too low on this axis
attack brightening          -7040 Hz   -896 Hz  n/a               ok        
temporal centroid           1.123 s    1.120 s  1.0x short        ok        
attack centroid trajectory  3974 Hz    1939 Hz  mean|Δ| 1563.046  drift     

-- modal structure --
AXIS              REFERENCE  SYNTH     DELTA                     VERDICT  NOTE
mode anchor       574 Hz     577 Hz    +1%                       ok       
mode ratios       0 cents    0 cents   mean gap 76 cents 76.242  ok       
mode density      1.898      1.582     0.83x                     ok       
mode harmonicity  0.032      0.074     +0.042                    ok       
beating rate      1.05 Hz    3.589 Hz  +2.538                    drift    synth is too high on this axis
beating depth     0.611      0.620     +0.009                    ok       

-- onset character --
AXIS               REFERENCE  SYNTH     DELTA   VERDICT  NOTE
onset bite         29.357     2539.855  86.52x  drift    synth is too high on this axis
onset flatness     0.328      0.362     +0.035  ok       
onset pitch glide  n/a        n/a       n/a              NOT MEASURABLE on either side: pitch not detectable in both onset windows

-- perceptual character --
AXIS               REFERENCE   SYNTH         DELTA          VERDICT   NOTE
brightness ratio   1.991       11.704        5.88x          MISMATCH  synth is too high on this axis
harmonic slope     -21 dB/oct  10.39 dB/oct  +31.39         MISMATCH  synth is too high on this axis
tristimulus        0.758       0.000         mean|Δ| 0.505  drift     
noise persistence  0.534       1.004         1.88x          ok        
air-band flatness  0.203       0.004         -0.200         drift     synth is too low on this axis

-- vibrato --
AXIS              REFERENCE  SYNTH  DELTA  VERDICT  NOTE
vibrato rate      n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato extent    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato jitter    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato AM depth  n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)

-- formants / body resonance --
AXIS            REFERENCE  SYNTH     DELTA  VERDICT   NOTE
first formant   1292 Hz    705.2 Hz  -45%   ok        
second formant  3470 Hz    1572 Hz   -55%   MISMATCH  synth is too low on this axis
bridge hill     n/a        n/a       n/a              NOT MEASURABLE on either side: no formant peak found in this band

-- cepstral timbre --
AXIS          REFERENCE  SYNTH    DELTA           VERDICT   NOTE
MFCC profile  135.724    -32.557  mean|Δ| 49.366  MISMATCH  

-- whole-signal statistics --
AXIS                  REFERENCE      SYNTH            DELTA       VERDICT   NOTE
centroid slope        -477.1 Hz/s    -137.2 Hz/s      +339.9      ok        
centroid curvature    535.8 Hz/s²    19.75 Hz/s²      -516        ok        
centroid spread       186.8 Hz       219.7 Hz         +18%        ok        
loudness slope        -0.03759 1/s   -0.03765 1/s     -5.336e-05  ok        
loudness curvature    0.01633 1/s²   -0.0004197 1/s²  -0.01675    ok        
loudness gesture      0.642          0.528            -0.114      ok        
loudness spread       0.035          0.059            +0.023      drift     synth is too high on this axis
flatness slope        -0.009727 1/s  -0.000896 1/s    +0.008831   ok        
flatness median       0.016          0.000            -0.015      ok        
noise-fraction slope  -0.08447 1/s   -0.0004228 1/s   +0.08405    drift     synth is too high on this axis
peak noise fraction   0.371          0.853            +0.482      MISMATCH  synth is too high on this axis
crest median          2.128          2.737            1.29x       drift     synth is too high on this axis
harmonic bloom        -10.74 dB/s    0.4093 dB/s      +11.15      MISMATCH  synth is too high on this axis
```

</details>

<details><summary><code>refs organic</code> placement for candidate A</summary>

```
$ refs organic -instrument tubular-bells -pitch 0 -override lfo_enabled=0,gen3_freq=1.004,gen9_source=1,gen9_freq=3.4527,gen9_gain=0.14,gen9_env_fast_mix=1,gen9_env_fast_rate=0.34,gen10_source=1,gen10_freq=3.9711,gen10_gain=0.12,gen10_env_fast_mix=1,gen10_env_fast_rate=0.4,gen11_source=1,gen11_freq=4.6961,gen11_gain=0.1,gen11_env_fast_mix=1,gen11_env_fast_rate=0.48,gen12_source=1,gen12_freq=5.188,gen12_gain=0.08,gen12_env_fast_mix=1,gen12_env_fast_rate=0.54

=== tubular-bells — organic axes against the real-recording range (p10–p90 over 59 references) ===

AXIS                         VALUE   REAL p10  p50      p90       WHERE   POSITION
loudness modulation rate     1.01    0.672     0.672    1.34      inside  +0.50
loudness modulation depth    0.103   0.037     0.24     1.34      inside  -0.10
modulation bands             0.982   0.448     0.839    0.928     above   +0.30
modulation flatness          0.227   0.183     0.488    0.604     inside  -0.62
brightness modulation depth  0.0264  0.0162    0.0823   0.41      inside  -0.14
partial bandwidth            n/a                                          does not apply to this signal
partial independence         0.14    0.0917    0.503    1         inside  -0.40
onset bite                   7.05    0.0708    1        1.6e+03   inside  +0.00
onset flatness               0.0436  0.114     0.26     0.407     below   -0.74
onset pitch glide            n/a                                          does not apply to this signal
residual centroid            630     1.92e+03  3.1e+03  5.58e+03  below   -0.67
noise decay vs tone          n/a                                          no reliable fundamental: F0 detection returned 221.2 Hz at confidence 0.50 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
noise persistence            0.944   0.101     0.598    2.27      inside  +0.16
vibrato jitter               n/a                                          does not apply to this signal
crest median                 2.54    1.65      2.38     3.24      inside  +0.10

POSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; 'low-n (n=k)' means fewer than 10 references qualified for that axis, too few to trust a band.
'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.
```

</details>

### Candidate B — per-mode decay: the hum outlives the strike

Overrides:

```json
{
  "gen1_env_fast_rate": 0,
  "gen8_env_fast_rate": 5.3,
  "gen3_freq": 1.004
}
```

Hypothesis (verbatim from `audition_candidates.json`):

> A structurally different route to the same 'resonant wavy ring': leave the mode bank and the LFO alone and reshape WHICH modes last. The reference's decay-per-mode vector is -1.9, -3.4, -5.8, -63.8, -13.6, -23.7, -50.4, -43.3 dB/s -- a 33x spread in which the hum outlives everything and the upper modes die fast. The synth's is -4.6, -5.2, -5.3, -5.7, -6.2, -6.9 dB/s: every mode decaying at the same rate because the shared amp envelope dominates, which is precisely why it reads as one flat 'clean tone'. gen1_env_fast_rate 0.03 -> 0 lets the hum ride the amp envelope alone (the reference's slowest mode), and gen8_env_fast_rate 0.30 -> 5.30 puts the top mode on the reference's own -50 dB/s. That alone takes band T60 from drift (mean 2.43x long, worst 4k 6.52x) to ok (mean 1.51x, worst 500 2.06x). It does NOT help the strike: onset bite goes 660.6 -> 2635.7 against the reference's 29.4 (89.78x, drift before and after, and further away) -- the shorter upper modes leave less energy in the 300-600 ms window the ratio divides by, so the clank reads louder. That is the row to listen to on this candidate. gen3_freq 1.005 -> 1.0040 is shared with A -- it is the only lever measured to move beating depth toward the reference (0.490 -> 0.624, drift -> ok) -- but everything else differs: A adds four partials and removes the LFO, B adds none and keeps it. Deliberately NOT changed: amp_decay/amp_release (ring T60 stays ok, 2.98 s vs 2.97 s -- the listener asked for a ring that PROLONGS) and gen1_freq (mode anchor stays ok at 577 Hz). Only gen6/gen7 were left at their seed rates: speeding them to the reference's -13.6/-23.7 dB/s strips enough upper partial energy to drag inharmonicity from 0.0040 to 0.0029, away from the reference's 0.0093, and `refs validate` scores inharmonicity AUC 1.00 against the ear verdicts. Bound drift left: onset bite only.

<details><summary>Full <code>refs diagnose</code> output for candidate B</summary>

```
$ refs diagnose -instrument tubular-bells -ref tubular-bells-vcsl -override gen1_env_fast_rate=0,gen8_env_fast_rate=5.3,gen3_freq=1.004

=== tubular-bells (reference: tubular-bells-vcsl) ===

NOTE: the reference clip is TRUNCATED to 3.00s by the corpus's fetch config (convert.max_sec) --
the real instrument recording may ring/decay further than this reference can show. Pass
-dur <seconds> to render the SYNTH for longer (the reference itself cannot be extended without
re-fetching from the network).

-- noise / harmonic balance --
AXIS                 REFERENCE  SYNTH    DELTA   VERDICT  NOTE
noise fraction       0.875      0.856    -0.020  drift    synth is too pure/tonal (missing noise character) (CAVEAT: reference partial series deviation=0.52 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
spectral flatness    0.030      0.000    -0.030  ok       (CAVEAT: reference partial series deviation=0.52 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
residual centroid    1920 Hz    1271 Hz  -34%    ok       
noise decay vs tone  0.964      n/a      n/a              NOT COMPARABLE -- measurable on the reference but NOT on the synth: both tracks still above -20 dB at clip end. That asymmetry is itself the finding: the synth's signal character differs from the reference's.

-- temporal envelope --
AXIS             REFERENCE   SYNTH       DELTA                   VERDICT  NOTE
attack           0.020 s     0.010 s     2.0x short              ok       
ring T60         2.970 s     2.980 s     rate -4.3 -> -4.8 dB/s  ok       T60 NOT OBSERVABLE: both clips still ring above -60dB at clip end (ref clamped at 2.97s, synth at 2.98s) -- comparing overall decay RATE instead (ref -4.3 dB/s, synth -4.8 dB/s)
release          2.590 s     2.170 s     1.2x short              ok       
decay T10        1.270 s     2.180 s     1.7x long               ok       
decay T20        n/a         n/a         n/a                              NOT MEASURABLE on either side: clip ended before -20 dB was reached (still above it at 2.97s), so this is a lower bound rather than a measured decay time
decay T30        n/a         n/a         n/a                              NOT MEASURABLE on either side: clip ended before -30 dB was reached (still above it at 2.97s), so this is a lower bound rather than a measured decay time
energy sections  0.600       0.489       mean|Δ| 0.061           ok       
decay            0.140 s     0.220 s     1.6x long               ok       
decay rate       -31.9 dB/s  -10.4 dB/s  +21.5                   drift    synth is too high on this axis
peak time        0.020 s     0.010 s     2.0x short              ok       
swell            0.000 s     0.000 s     n/a                     ok       
total audible    3.000 s     3.000 s     1.0x long               ok       

-- partial structure --
AXIS                      REFERENCE  SYNTH   DELTA    VERDICT   NOTE
inharmonicity             0.009      0.004   -0.005   MISMATCH  synth partials are too harmonic (needs inharmonic bar/tube modes)
partial series deviation  0.523      2.101   +1.578   MISMATCH  synth partials skip ranks worse than the reference (less like a consecutive harmonic series)
spectral irregularity     11.444     33.554  +22.109  drift     synth is too high on this axis
  ref partials (x f0):   0.62, 1.01, 1.47, 1.71, 2.01, 2.61, 3.26
  synth partials (x f0): 4.01, 6.48, 9.51, 11.07, 13.01, 16.89

-- brightness / formant --
AXIS      REFERENCE  SYNTH    DELTA  VERDICT  NOTE
centroid  1855 Hz    1239 Hz  -33%   ok       
rolloff   1873 Hz    1591 Hz  -15%   ok       

-- modulation --
AXIS                         REFERENCE  SYNTH     DELTA          VERDICT   NOTE
sustain modulation           0.058      0.225     +0.168         drift     synth sustain wobbles more than the reference
loudness modulation rate     1.008 Hz   3.695 Hz  +2.688         MISMATCH  synth is too high on this axis
loudness modulation depth    0.079      0.138     +0.059         ok        
modulation bands             0.976      0.019     mean|Δ| 0.644  MISMATCH  
modulation flatness          0.117      0.051     -0.066         drift     synth is too low on this axis
brightness modulation depth  0.227      0.027     -0.200         drift     synth is too low on this axis

-- resolved decay --
AXIS                      REFERENCE            SYNTH                 DELTA                               VERDICT  NOTE
decay per mode            -1.921 dB/s          -4.31 dB/s            mean|Δ| 14.8 dB/s 14.774            ok       
decay slope vs frequency  -8.325 dB/s per kHz  -0.9144 dB/s per kHz  +7.411                              ok       
band T60                  -1.000 s             -1.000 s              mean 1.51x (worst 500 2.06x) 0.596  ok       

-- partial decorrelation --
AXIS                  REFERENCE  SYNTH  DELTA   VERDICT  NOTE
partial independence  0.884      0.237  -0.646  drift    synth is too low on this axis

-- struck string --
AXIS                   REFERENCE    SYNTH    DELTA            VERDICT   NOTE
string stiffness B     0.001        0.000    -0.000           drift     synth is too low on this axis
double-decay ratio     3.976        132.678  33.37x           MISMATCH  synth is too high on this axis
prompt decay rate      28.606       49.901   +21.296          ok        
aftersound decay rate  7.195        0.376    -6.819           ok        
partial stretch        9.118 cents  0 cents  mean|Δ| 121.921  drift     

-- wind attack --
AXIS                        REFERENCE  SYNTH    DELTA             VERDICT   NOTE
attack rise time            0.006 s    0.000 s  n/a               MISMATCH  synth is too low on this axis
attack brightening          -7040 Hz   -593 Hz  n/a               ok        
temporal centroid           1.123 s    1.129 s  1.0x long         ok        
attack centroid trajectory  3974 Hz    1367 Hz  mean|Δ| 2070.403  MISMATCH  

-- modal structure --
AXIS              REFERENCE  SYNTH     DELTA                       VERDICT  NOTE
mode anchor       574 Hz     577 Hz    +1%                         ok       
mode ratios       0 cents    0 cents   mean gap 463 cents 462.943  ok       
mode density      1.898      0.949     0.50x                       ok       
mode harmonicity  0.032      0.081     +0.049                      ok       
beating rate      1.05 Hz    3.589 Hz  +2.538                      drift    synth is too high on this axis
beating depth     0.611      0.624     +0.013                      ok       

-- onset character --
AXIS               REFERENCE  SYNTH       DELTA   VERDICT  NOTE
onset bite         29.357     2635.705    89.78x  drift    synth is too high on this axis
onset flatness     0.328      0.370       +0.043  ok       
onset pitch glide  n/a        1050 cents  n/a              NOT COMPARABLE -- measurable on the synth but NOT on the reference: pitch not detectable in both onset windows. That asymmetry is itself the finding: the reference's signal character differs from the synth's.

-- perceptual character --
AXIS               REFERENCE   SYNTH         DELTA          VERDICT   NOTE
brightness ratio   1.991       8.603         4.32x          MISMATCH  synth is too high on this axis
harmonic slope     -21 dB/oct  10.35 dB/oct  +31.35         MISMATCH  synth is too high on this axis
tristimulus        0.758       0.000         mean|Δ| 0.505  drift     
noise persistence  0.534       1.009         1.89x          ok        
air-band flatness  0.203       0.006         -0.197         drift     synth is too low on this axis

-- vibrato --
AXIS              REFERENCE  SYNTH  DELTA  VERDICT  NOTE
vibrato rate      n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato extent    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato jitter    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato AM depth  n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)

-- formants / body resonance --
AXIS            REFERENCE  SYNTH     DELTA  VERDICT   NOTE
first formant   1292 Hz    689.1 Hz  -47%   ok        
second formant  3470 Hz    1540 Hz   -56%   MISMATCH  synth is too low on this axis
bridge hill     n/a        n/a       n/a              NOT MEASURABLE on either side: no formant peak found in this band

-- cepstral timbre --
AXIS          REFERENCE  SYNTH     DELTA           VERDICT   NOTE
MFCC profile  135.724    -136.800  mean|Δ| 54.917  MISMATCH  

-- whole-signal statistics --
AXIS                  REFERENCE      SYNTH           DELTA       VERDICT   NOTE
centroid slope        -477.1 Hz/s    -46.45 Hz/s     +430.6      ok        
centroid curvature    535.8 Hz/s²    18.63 Hz/s²     -517.1      ok        
centroid spread       186.8 Hz       95.06 Hz        -49%        ok        
loudness slope        -0.03759 1/s   -0.03677 1/s    +0.0008213  ok        
loudness curvature    0.01633 1/s²   -0.001074 1/s²  -0.01741    ok        
loudness gesture      0.642          0.528           -0.114      ok        
loudness spread       0.035          0.059           +0.023      drift     synth is too high on this axis
flatness slope        -0.009727 1/s  -0.0005008 1/s  +0.009227   ok        
flatness median       0.016          0.000           -0.015      ok        
noise-fraction slope  -0.08447 1/s   0.001449 1/s    +0.08592    drift     synth is too high on this axis
peak noise fraction   0.371          0.845           +0.474      MISMATCH  synth is too high on this axis
crest median          2.128          2.733           1.28x       drift     synth is too high on this axis
harmonic bloom        -10.74 dB/s    0.3531 dB/s     +11.09      MISMATCH  synth is too high on this axis
```

</details>

<details><summary><code>refs organic</code> placement for candidate B</summary>

```
$ refs organic -instrument tubular-bells -pitch 0 -override gen1_env_fast_rate=0,gen8_env_fast_rate=5.3,gen3_freq=1.004

=== tubular-bells — organic axes against the real-recording range (p10–p90 over 59 references) ===

AXIS                         VALUE   REAL p10  p50      p90       WHERE   POSITION
loudness modulation rate     1.01    0.672     0.672    1.34      inside  +0.50
loudness modulation depth    0.109   0.037     0.24     1.34      inside  -0.10
modulation bands             0.976   0.448     0.839    0.928     above   +0.28
modulation flatness          0.227   0.183     0.488    0.604     inside  -0.62
brightness modulation depth  0.0177  0.0162    0.0823   0.41      inside  -0.16
partial bandwidth            n/a                                          does not apply to this signal
partial independence         0.185   0.0917    0.503    1         inside  -0.35
onset bite                   7.14    0.0708    1        1.6e+03   inside  +0.00
onset flatness               0.0658  0.114     0.26     0.407     below   -0.66
onset pitch glide            n/a                                          does not apply to this signal
residual centroid            329     1.92e+03  3.1e+03  5.58e+03  below   -0.76
noise decay vs tone          n/a                                          no reliable fundamental: F0 detection returned 221.3 Hz at confidence 0.57 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
noise persistence            0.967   0.101     0.598    2.27      inside  +0.17
vibrato jitter               n/a                                          does not apply to this signal
crest median                 2.48    1.65      2.38     3.24      inside  +0.06

POSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; 'low-n (n=k)' means fewer than 10 references qualified for that axis, too few to trust a band.
'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.
```

</details>


## snare

Listener's words this round: **breathy, fuzzy, hissy, ghost-like (it lingers)**

Bound axes (`familyQuestions["snare"]`): `noise.flatness`, `env.decay_t20`, `snare.low_body_frac`, `onset.bite_ratio`, `noise.fraction`.

### Caveats for the listener

- **Only THREE of the snare's five bound axes measure at all, and one of the
  three carries a tool caveat.**
  - `snare.low_body_frac` produces NO ROW at all. The descriptor's `Applies` is
    `RequiresPercussive` (`!Silent && NoSustainPlateau`), applicability is
    evaluated against the REFERENCE bundle, and the `snare-vcsl` reference
    establishes a sustain plateau — so the entire `snare.*` group is dropped
    for this pair. The "body / low thump" question has no measurement behind it
    here; it is an ear-only question.
  - `noise.fraction` is n/a on the REFERENCE: `no reliable fundamental: F0
    detection returned 84.0 Hz at confidence 0.42 (below 0.65)`. The "tonal
    ring" question likewise has no measurement behind it.
  - `spectral flatness` — the axis carrying the MISMATCH and the listener's
    "hissy" — ships with the tool's own reliability CAVEAT, kept verbatim in
    the blocks below: *reference partial series deviation=0.71 — this
    instrument's true partials are strongly non-harmonic-series, so NoiseRatio's
    harmonic-bin assumption likely misclassifies real tonal energy as noise;
    treat this axis as unreliable here*. The move 0.155 -> 0.227 (A) / 0.233
    (B) is real, but the axis is a proxy.
  That leaves `env.decay_t20` and `onset.bite_ratio` as the only unqualified
  live rows, so the snare's ear verdict carries more weight than usual.
- **Organic range:** both candidates push `onset bite` far above the p90 of the
  real-recording band in the 2 s reference-free render (position +778 for A,
  +2528 for B, against +2.78 for the current version). This is a
  render-duration artifact — onset bite divides the first 30 ms by the
  300-600 ms window, and with `snare_tail_decay` at 19-21 the snare is
  essentially silent by 300 ms in a 2 s render. At the reference's own 0.89 s
  length the same axis reads 889 (A) and 1494 (B) against the reference's 923.
  Both candidates also newly exit the band on `onset flatness` and
  `crest median`. Judge these by ear, not by the position numbers.

### Current version — full `refs diagnose -ablate`

```
$ refs diagnose -instrument snare -ref snare-vcsl -ablate

=== snare (reference: snare-vcsl) ===

-- noise / harmonic balance --
AXIS                 REFERENCE  SYNTH    DELTA   VERDICT   NOTE
noise fraction       n/a        n/a      n/a               NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold (CAVEAT: reference partial series deviation=0.71 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
spectral flatness    0.259      0.155    -0.104  MISMATCH  synth is too pure/tonal (missing noise character) (CAVEAT: reference partial series deviation=0.71 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
residual centroid    2789 Hz    2283 Hz  -18%    ok        
noise decay vs tone  n/a        n/a      n/a               NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold

-- temporal envelope --
AXIS             REFERENCE    SYNTH        DELTA                     VERDICT   NOTE
attack           0.020 s      0.000 s      n/a                       MISMATCH  synth attack is far too fast
ring T60         0.860 s      0.880 s      rate -49.0 -> -36.4 dB/s  drift     T60 NOT OBSERVABLE: both clips still ring above -60dB at clip end (ref clamped at 0.86s, synth at 0.88s) -- comparing overall decay RATE instead (ref -49.0 dB/s, synth -36.4 dB/s); synth decay/ring is far too sustained
release          0.000 s      0.650 s      n/a                       MISMATCH  synth release is far too slow
decay T10        0.080 s      0.080 s      1.0x long                 ok        
decay T20        0.160 s      0.250 s      1.6x long                 drift     synth is too high on this axis
decay T30        0.240 s      0.620 s      2.6x long                 MISMATCH  synth is too high on this axis
energy sections  0.997        0.959        mean|Δ| 0.019             ok        
decay            0.480 s      0.150 s      3.2x short                ok        
decay rate       -100.2 dB/s  -106.5 dB/s  -6.288                    ok        
peak time        0.020 s      0.000 s      n/a                       MISMATCH  synth is too low on this axis
swell            0.000 s      0.000 s      n/a                       ok        
total audible    0.890 s      0.890 s      1.0x long                 ok        

-- partial structure --
AXIS                      REFERENCE  SYNTH  DELTA  VERDICT  NOTE
inharmonicity             n/a        n/a    n/a             NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
partial series deviation  n/a        n/a    n/a             NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
  ref partials (x f0):   0.47, 0.58, 0.64, 0.91, 0.95, 1.32, 2.53, 2.63
  synth partials (x f0): 0.71, 1.01, 1.26, 1.51, 5.20, 9.41, 10.72, 13.39

-- brightness / formant --
AXIS      REFERENCE  SYNTH    DELTA  VERDICT  NOTE
centroid  2643 Hz    2281 Hz  -14%   ok       
rolloff   221 Hz     1639 Hz  +643%  drift    synth has too much high-frequency energy

-- modulation --
AXIS                         REFERENCE  SYNTH     DELTA          VERDICT   NOTE
sustain modulation           0.439      0.132     -0.307         MISMATCH  synth sustain is missing amplitude modulation (flat/organ-like)
loudness modulation rate     1.344 Hz   1.344 Hz  +0             ok        
loudness modulation depth    0.628      0.363     -0.265         drift     synth is too low on this axis
modulation bands             0.726      0.696     mean|Δ| 0.020  ok        
modulation flatness          0.606      0.613     +0.007         ok        
brightness modulation depth  0.056      0.014     -0.042         ok        

-- resolved decay --
AXIS                      REFERENCE    SYNTH        DELTA                               VERDICT  NOTE
decay per mode            -112.4 dB/s  -101.4 dB/s  mean|Δ| 25.1 dB/s 25.109            drift    
decay slope vs frequency  n/a          n/a          n/a                                          NOT MEASURABLE on either side: fewer than three strong modes with a fitted decay
band T60                  1.644 s      0.829 s      mean 2.77x (worst 250 3.20x) 1.472  drift    synth is too high on this axis

-- partial decorrelation --
AXIS                  REFERENCE  SYNTH  DELTA  VERDICT  NOTE
partial independence  0.166      n/a    n/a             NOT COMPARABLE -- measurable on the reference but NOT on the synth: no pair of partials shared eight frames. That asymmetry is itself the finding: the synth's signal character differs from the reference's.

-- wind attack --
AXIS                        REFERENCE  SYNTH    DELTA             VERDICT   NOTE
attack rise time            0.012 s    0.000 s  n/a               MISMATCH  synth is too low on this axis
attack brightening          2671 Hz    1499 Hz  -44%              MISMATCH  synth is too low on this axis
temporal centroid           0.097 s    0.170 s  1.8x long         drift     synth is too high on this axis
attack centroid trajectory  658.3 Hz   1810 Hz  mean|Δ| 1253.175  drift     

-- modal structure --
AXIS              REFERENCE  SYNTH    DELTA                     VERDICT   NOTE
mode anchor       219 Hz     69 Hz    -68%                      MISMATCH  synth is too low on this axis
mode ratios       0 cents    0 cents  mean gap 41 cents 41.098  ok        
mode density      0.316      0.158    0.50x                     ok        
mode harmonicity  0.957      0.739    -0.219                    ok        
beating rate      1.794 Hz   n/a      n/a                                 NOT COMPARABLE -- measurable on the reference but NOT on the synth: no per-mode ripple that is not common to every mode. That asymmetry is itself the finding: the synth's signal character differs from the reference's.
beating depth     0.415      n/a      n/a                                 NOT COMPARABLE -- measurable on the reference but NOT on the synth: no per-mode ripple that is not common to every mode. That asymmetry is itself the finding: the synth's signal character differs from the reference's.

-- onset character --
AXIS            REFERENCE  SYNTH    DELTA   VERDICT  NOTE
onset bite      922.934    144.126  0.16x   ok       
onset flatness  0.144      0.263    +0.118  drift    synth is too high on this axis

-- perceptual character --
AXIS               REFERENCE  SYNTH  DELTA   VERDICT  NOTE
noise persistence  1.178      1.262  1.07x   ok       
air-band flatness  0.758      0.332  -0.426  drift    synth is too low on this axis

-- formants / body resonance --
AXIS            REFERENCE  SYNTH     DELTA  VERDICT  NOTE
first formant   n/a        837.1 Hz  n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.
second formant  n/a        1871 Hz   n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.
bridge hill     n/a        1871 Hz   n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.

-- cepstral timbre --
AXIS          REFERENCE  SYNTH    DELTA           VERDICT  NOTE
MFCC profile  -41.568    129.068  mean|Δ| 22.003  drift    

-- whole-signal statistics --
AXIS                REFERENCE    SYNTH         DELTA     VERDICT   NOTE
centroid slope      3169 Hz/s    141.8 Hz/s    -3027     MISMATCH  synth is too low on this axis
centroid curvature  -5098 Hz/s²  -2014 Hz/s²   +3084     MISMATCH  synth is too high on this axis
centroid spread     1657 Hz      197.9 Hz      -88%      drift     synth is too low on this axis
loudness slope      -0.1368 1/s  -0.1232 1/s   +0.01358  ok        
loudness curvature  0.5682 1/s²  0.3969 1/s²   -0.1713   MISMATCH  synth is too low on this axis
loudness gesture    0.690        0.817         +0.127    ok        
loudness spread     0.010        0.027         +0.017    ok        
flatness slope      0.366 1/s    0.009755 1/s  -0.3563   MISMATCH  synth is too low on this axis
flatness median     0.196        0.150         -0.046    drift     synth is too low on this axis
crest median        2.265        2.994         1.32x     drift     synth is too high on this axis


-- stage sensitivity (which stage moves each MISMATCH axis; not a decomposition, does not sum) --
AXIS                TOP STAGES (Δnorm above the no-effect floor when the stage is at identity)
spectral flatness   post 2.91  fm 0.00
attack              fm 0.00  post 0.00
release             post 0.22  fm 0.00
decay T30           post 0.85  fm 0.00
peak time           fm 0.00  post 0.00
sustain modulation  post 0.44  fm 0.00
attack rise time    fm 0.00  post 0.00
attack brightening  post 0.52  fm 0.00
mode anchor         post 32.76  fm 0.00
centroid slope      post 2.23  fm 0.00
centroid curvature  post 0.74  fm 0.00
loudness curvature  post 0.17  fm 0.00
flatness slope      post 1.79  fm 0.00
```

<details><summary>Current version — <code>refs organic</code> placement</summary>

```
$ refs organic -instrument snare -pitch 0

=== snare — organic axes against the real-recording range (p10–p90 over 59 references) ===

AXIS                         VALUE     REAL p10  p50      p90       WHERE   POSITION
loudness modulation rate     0.672     0.672     0.672    1.34      inside  +0.00
loudness modulation depth    2.42      0.037     0.24     1.34      above   +1.67
modulation bands             0.879     0.448     0.839    0.928     inside  +0.08
modulation flatness          0.568     0.183     0.488    0.604     inside  +0.19
brightness modulation depth  0.0721    0.0162    0.0823   0.41      inside  -0.03
partial bandwidth            n/a                                            does not apply to this signal
partial independence         n/a                                            no pair of partials shared eight frames
onset bite                   4.46e+03  0.0708    1        1.6e+03   above   +2.78
onset flatness               0.354     0.114     0.26     0.407     inside  +0.32
onset pitch glide            n/a                                            does not apply to this signal
residual centroid            4.42e+03  1.92e+03  3.1e+03  5.58e+03  inside  +0.36
noise decay vs tone          n/a                                            no reliable fundamental: F0 detection returned 48.4 Hz at confidence 0.00 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
noise persistence            0         0.101     0.598    2.27      below   -0.28
vibrato jitter               n/a                                            does not apply to this signal
crest median                 2.82      1.65      2.38     3.24      inside  +0.28

POSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; 'low-n (n=k)' means fewer than 10 references qualified for that axis, too few to trust a band.
'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.
```

</details>

### Candidate A — broadband wire, no lingering tail

Overrides:

```json
{
  "snare_noise_tune": 1.4,
  "snare_tail_decay": 19,
  "snare_noise_decay": 20,
  "snare_wire_mix": 1.1,
  "snare_noise_mix": 0.34
}
```

Hypothesis (verbatim from `audition_candidates.json`):

> spectral flatness 0.155 vs 0.259 is the bound MISMATCH ('too pure/tonal'); ablation names post 2.91 (fm 0.00), but the snare's stages are set by its push migration so the reachable knobs are the legacy ones. snare_noise_tune scales EVERY noise filter centre at once (LP 350xtune, BPs at 1800/3200/4500xtune), so 0.55 -> 1.4 spreads the wire band from 990-2475 Hz up to 2520-6300 Hz: flatness 0.155 -> 0.227 vs 0.259 (MISMATCH -> ok). That is also the listener's 'breathy, fuzzy, hissy' -- a narrow band of hiss reading as breath rather than as wire. snare_tail_decay 10 -> 19 and snare_noise_decay 7 -> 20 answer 'ghost-like (lingers)': decay T20 0.25 s -> 0.20 s vs ref 0.16 s (drift -> ok). snare_wire_mix 0.8 -> 1.1 with snare_noise_mix 0.56 -> 0.34 keeps the level while swapping the low breathy noise bed for the wire, landing onset bite 144 -> 889 vs ref 923 (0.96x).

<details><summary>Full <code>refs diagnose</code> output for candidate A</summary>

```
$ refs diagnose -instrument snare -ref snare-vcsl -override snare_noise_tune=1.4,snare_tail_decay=19,snare_noise_decay=20,snare_wire_mix=1.1,snare_noise_mix=0.34

=== snare (reference: snare-vcsl) ===

-- noise / harmonic balance --
AXIS                 REFERENCE  SYNTH    DELTA   VERDICT  NOTE
noise fraction       n/a        n/a      n/a              NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold (CAVEAT: reference partial series deviation=0.71 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
spectral flatness    0.259      0.227    -0.032  ok       (CAVEAT: reference partial series deviation=0.71 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
residual centroid    2789 Hz    3313 Hz  +19%    ok       
noise decay vs tone  n/a        n/a      n/a              NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold

-- temporal envelope --
AXIS             REFERENCE    SYNTH        DELTA          VERDICT   NOTE
attack           0.020 s      0.000 s      n/a            MISMATCH  synth attack is far too fast
ring T60         0.860 s      0.850 s      1.0x short     ok        (NOTE: reference clip ended before -60dB was reached; its value is a LOWER BOUND, not a measured T60)
release          0.000 s      0.000 s      n/a            ok        
decay T10        0.080 s      0.080 s      1.0x long      ok        
decay T20        0.160 s      0.200 s      1.2x long      ok        
decay T30        0.240 s      0.330 s      1.4x long      drift     synth is too high on this axis
energy sections  0.997        0.984        mean|Δ| 0.006  ok        
decay            0.480 s      0.850 s      1.8x long      ok        
decay rate       -100.2 dB/s  -65.74 dB/s  +34.44         drift     synth is too high on this axis
peak time        0.020 s      0.000 s      n/a            MISMATCH  synth is too low on this axis
swell            0.000 s      0.000 s      n/a            ok        
total audible    0.890 s      0.860 s      1.0x short     drift     synth is too low on this axis

-- partial structure --
AXIS                      REFERENCE  SYNTH  DELTA  VERDICT  NOTE
inharmonicity             n/a        n/a    n/a             NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
partial series deviation  n/a        n/a    n/a             NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
  ref partials (x f0):   0.47, 0.58, 0.64, 0.91, 0.95, 1.32, 2.53, 2.63
  synth partials (x f0): 1.55, 1.66, 1.88, 2.21, 2.54, 2.71, 3.41, 4.56

-- brightness / formant --
AXIS      REFERENCE  SYNTH    DELTA   VERDICT   NOTE
centroid  2643 Hz    3306 Hz  +25%    ok        
rolloff   221 Hz     2996 Hz  +1257%  MISMATCH  synth has too much high-frequency energy

-- modulation --
AXIS                         REFERENCE  SYNTH     DELTA          VERDICT   NOTE
sustain modulation           0.439      0.000     -0.439         MISMATCH  synth sustain is missing amplitude modulation (flat/organ-like)
loudness modulation rate     1.344 Hz   1.344 Hz  +0             ok        
loudness modulation depth    0.628      0.497     -0.132         ok        
modulation bands             0.726      0.701     mean|Δ| 0.016  ok        
modulation flatness          0.606      0.604     -0.002         ok        
brightness modulation depth  0.056      0.006     -0.050         ok        

-- resolved decay --
AXIS                      REFERENCE    SYNTH        DELTA                               VERDICT  NOTE
decay per mode            -112.4 dB/s  -131.7 dB/s  mean|Δ| 19.8 dB/s 19.754            ok       
decay slope vs frequency  n/a          n/a          n/a                                          NOT MEASURABLE on either side: fewer than three strong modes with a fitted decay
band T60                  1.644 s      0.445 s      mean 2.02x (worst 125 3.69x) 1.012  drift    synth is too high on this axis

-- partial decorrelation --
AXIS                  REFERENCE  SYNTH  DELTA  VERDICT  NOTE
partial independence  0.166      n/a    n/a             NOT COMPARABLE -- measurable on the reference but NOT on the synth: no pair of partials shared eight frames. That asymmetry is itself the finding: the synth's signal character differs from the reference's.

-- wind attack --
AXIS                        REFERENCE  SYNTH    DELTA             VERDICT   NOTE
attack rise time            0.012 s    0.000 s  n/a               MISMATCH  synth is too low on this axis
attack brightening          2671 Hz    856 Hz   -68%              MISMATCH  synth is too low on this axis
temporal centroid           0.097 s    0.105 s  1.1x long         ok        
attack centroid trajectory  658.3 Hz   2944 Hz  mean|Δ| 2290.132  MISMATCH  

-- modal structure --
AXIS              REFERENCE  SYNTH     DELTA                     VERDICT   NOTE
mode anchor       219 Hz     69 Hz     -68%                      MISMATCH  synth is too low on this axis
mode ratios       0 cents    0 cents   mean gap 41 cents 41.331  ok        
mode density      0.316      0.158     0.50x                     ok        
mode harmonicity  0.957      0.760     -0.197                    ok        
beating rate      1.794 Hz   3.076 Hz  +1.282                    drift     synth is too high on this axis
beating depth     0.415      0.135     -0.281                    MISMATCH  synth is too low on this axis

-- onset character --
AXIS            REFERENCE  SYNTH    DELTA   VERDICT   NOTE
onset bite      922.928    889.277  0.96x   ok        
onset flatness  0.144      0.309    +0.164  MISMATCH  synth is too high on this axis

-- perceptual character --
AXIS               REFERENCE  SYNTH  DELTA   VERDICT  NOTE
noise persistence  1.178      0.976  0.83x   ok       
air-band flatness  0.758      0.323  -0.435  drift    synth is too low on this axis

-- formants / body resonance --
AXIS            REFERENCE  SYNTH    DELTA  VERDICT  NOTE
first formant   n/a        1015 Hz  n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.
second formant  n/a        1962 Hz  n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.
bridge hill     n/a        1962 Hz  n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.

-- cepstral timbre --
AXIS          REFERENCE  SYNTH   DELTA           VERDICT  NOTE
MFCC profile  -41.565    91.579  mean|Δ| 19.837  ok       

-- whole-signal statistics --
AXIS                REFERENCE    SYNTH        DELTA      VERDICT   NOTE
centroid slope      3169 Hz/s    386.1 Hz/s   -2783      MISMATCH  synth is too low on this axis
centroid curvature  -5098 Hz/s²  -1334 Hz/s²  +3764      MISMATCH  synth is too high on this axis
centroid spread     1657 Hz      135.7 Hz     -92%       drift     synth is too low on this axis
loudness slope      -0.149 1/s   -0.1401 1/s  +0.008861  ok        
loudness curvature  0.6186 1/s²  0.4995 1/s²  -0.1191    drift     synth is too low on this axis
loudness gesture    0.690        1.000        +0.310     drift     synth is too high on this axis
loudness spread     0.011        0.026        +0.015     ok        
flatness slope      0.366 1/s    0.02036 1/s  -0.3456    MISMATCH  synth is too low on this axis
flatness median     0.196        0.223        +0.027     ok        
crest median        2.265        3.089        1.36x      drift     synth is too high on this axis
```

</details>

<details><summary><code>refs organic</code> placement for candidate A</summary>

```
$ refs organic -instrument snare -pitch 0 -override snare_noise_tune=1.4,snare_tail_decay=19,snare_noise_decay=20,snare_wire_mix=1.1,snare_noise_mix=0.34

=== snare — organic axes against the real-recording range (p10–p90 over 59 references) ===

AXIS                         VALUE     REAL p10  p50     p90      WHERE   POSITION
loudness modulation rate     0.672     0.672     0.672   1.34     inside  +0.00
loudness modulation depth    3.25      0.037     0.24    1.34     above   +2.31
modulation bands             0.874     0.448     0.839   0.928    inside  +0.07
modulation flatness          0.571     0.183     0.488   0.604    inside  +0.20
brightness modulation depth  0.000773  0.0162    0.0823  0.41     below   -0.21
partial bandwidth            n/a                                          does not apply to this signal
partial independence         n/a                                          does not apply to this signal
onset bite                   1.25e+06  0.0708    1       1.6e+03  above   +778.15
onset flatness               0.523     0.114     0.26    0.407    above   +0.90
onset pitch glide            n/a                                          does not apply to this signal
residual centroid            n/a                                          does not apply to this signal
noise decay vs tone          n/a                                          no reliable fundamental: F0 detection returned 123.8 Hz at confidence 0.00 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
noise persistence            0.752     0.101     0.598   2.27     inside  +0.07
vibrato jitter               n/a                                          does not apply to this signal
crest median                 3.57      1.65      2.38    3.24     above   +0.75

POSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; 'low-n (n=k)' means fewer than 10 references qualified for that axis, too few to trust a band.
'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.
```

</details>

### Candidate B — body vs wire rebalance

Overrides:

```json
{
  "snare_noise_tune": 1.5,
  "snare_tail_decay": 21,
  "snare_noise_decay": 22,
  "snare_tone2_freq": 194,
  "snare_tone_mix": 0.9,
  "snare_wire_mix": 1.2,
  "snare_noise_mix": 0.4
}
```

Hypothesis (verbatim from `audition_candidates.json`):

> A's decay fix with the body re-sited from the reference's own snare.* rows: snare_tone2_freq 280 -> 194 Hz puts the second body mode on the reference's measured body fundamental, snare_tone_mix 1.12 -> 0.9 and snare_noise_mix 0.56 -> 0.40 shift the balance toward the reference's low body fraction 0.446 / high buzz fraction 0.344, and snare_noise_tune 1.5 + snare_wire_mix 1.2 push the wire further than A. spectral flatness 0.155 -> 0.233 vs 0.259 (MISMATCH -> ok), decay T20 0.25 -> 0.17 s vs 0.16 s (drift -> ok). B is the 'more crack, less thump' end of A: onset bite 1494 vs ref 923 (1.6x, still ok) where A sits at 0.96x.

<details><summary>Full <code>refs diagnose</code> output for candidate B</summary>

```
$ refs diagnose -instrument snare -ref snare-vcsl -override snare_noise_tune=1.5,snare_tail_decay=21,snare_noise_decay=22,snare_tone2_freq=194,snare_tone_mix=0.9,snare_wire_mix=1.2,snare_noise_mix=0.4

=== snare (reference: snare-vcsl) ===

-- noise / harmonic balance --
AXIS                 REFERENCE  SYNTH  DELTA   VERDICT  NOTE
noise fraction       n/a        n/a    n/a              NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold (CAVEAT: reference partial series deviation=0.71 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
spectral flatness    0.259      0.233  -0.026  ok       (CAVEAT: reference partial series deviation=0.71 -- this instrument's true partials are strongly non-harmonic-series, so NoiseRatio's harmonic-bin assumption likely misclassifies real tonal energy as noise; treat this axis as unreliable here, see partial ratios below)
residual centroid    2789 Hz    n/a    n/a              NOT COMPARABLE -- measurable on the reference but NOT on the synth: no reliable fundamental and fewer than two strong tracked modes: nothing to notch out. That asymmetry is itself the finding: the synth's signal character differs from the reference's.
noise decay vs tone  n/a        n/a    n/a              NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold

-- temporal envelope --
AXIS             REFERENCE    SYNTH        DELTA          VERDICT   NOTE
attack           0.020 s      0.000 s      n/a            MISMATCH  synth attack is far too fast
ring T60         0.860 s      0.760 s      1.1x short     ok        (NOTE: reference clip ended before -60dB was reached; its value is a LOWER BOUND, not a measured T60)
release          0.000 s      0.000 s      n/a            ok        
decay T10        0.080 s      0.070 s      1.1x short     ok        
decay T20        0.160 s      0.170 s      1.1x long      ok        
decay T30        0.240 s      0.320 s      1.3x long      drift     synth is too high on this axis
energy sections  0.997        0.984        mean|Δ| 0.006  ok        
decay            0.480 s      0.760 s      1.6x long      ok        
decay rate       -100.2 dB/s  -72.34 dB/s  +27.84         drift     synth is too high on this axis
peak time        0.020 s      0.000 s      n/a            MISMATCH  synth is too low on this axis
swell            0.000 s      0.000 s      n/a            ok        
total audible    0.890 s      0.770 s      1.2x short     MISMATCH  synth is too low on this axis

-- partial structure --
AXIS                      REFERENCE  SYNTH  DELTA  VERDICT  NOTE
inharmonicity             n/a        n/a    n/a             NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
partial series deviation  n/a        n/a    n/a             NOT MEASURABLE on either side: no reliable fundamental: F0 detection returned 84.0 Hz at confidence 0.42 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
  ref partials (x f0):   0.47, 0.58, 0.64, 0.91, 0.95, 1.32, 2.53, 2.63
  synth partials (x f0): 10.52, 13.96, 14.83, 16.04, 17.14, 21.57, 28.80, 30.04

-- brightness / formant --
AXIS      REFERENCE  SYNTH    DELTA   VERDICT   NOTE
centroid  2643 Hz    3387 Hz  +28%    ok        
rolloff   221 Hz     3079 Hz  +1295%  MISMATCH  synth has too much high-frequency energy

-- modulation --
AXIS                         REFERENCE  SYNTH     DELTA          VERDICT   NOTE
sustain modulation           0.439      0.000     -0.439         MISMATCH  synth sustain is missing amplitude modulation (flat/organ-like)
loudness modulation rate     1.344 Hz   1.344 Hz  +0             ok        
loudness modulation depth    0.628      0.552     -0.076         ok        
modulation bands             0.726      0.704     mean|Δ| 0.014  ok        
modulation flatness          0.606      0.603     -0.003         ok        
brightness modulation depth  0.056      0.003     -0.054         ok        

-- resolved decay --
AXIS                      REFERENCE    SYNTH        DELTA                               VERDICT  NOTE
decay per mode            -112.4 dB/s  -120.3 dB/s  mean|Δ| 7.9 dB/s 7.851              ok       
decay slope vs frequency  n/a          n/a          n/a                                          NOT MEASURABLE on either side: fewer than three strong modes with a fitted decay
band T60                  1.644 s      0.414 s      mean 1.89x (worst 125 3.97x) 0.916  ok       

-- partial decorrelation --
AXIS                  REFERENCE  SYNTH  DELTA  VERDICT  NOTE
partial independence  0.166      n/a    n/a             NOT COMPARABLE -- measurable on the reference but NOT on the synth: no pair of partials shared eight frames. That asymmetry is itself the finding: the synth's signal character differs from the reference's.

-- wind attack --
AXIS                        REFERENCE  SYNTH    DELTA             VERDICT   NOTE
attack rise time            0.012 s    0.000 s  n/a               MISMATCH  synth is too low on this axis
attack brightening          2671 Hz    636 Hz   -76%              MISMATCH  synth is too low on this axis
temporal centroid           0.097 s    0.095 s  1.0x short        ok        
attack centroid trajectory  658.3 Hz   3133 Hz  mean|Δ| 2442.014  MISMATCH  

-- modal structure --
AXIS              REFERENCE  SYNTH     DELTA   VERDICT   NOTE
mode anchor       219 Hz     70 Hz     -68%    MISMATCH  synth is too low on this axis
mode ratios       0 cents    n/a       n/a               NOT COMPARABLE -- measurable on the reference but NOT on the synth: fewer than two stable spectral modes were tracked. That asymmetry is itself the finding: the synth's signal character differs from the reference's.
mode density      0.316      n/a       n/a               NOT COMPARABLE -- measurable on the reference but NOT on the synth: fewer than two stable spectral modes were tracked. That asymmetry is itself the finding: the synth's signal character differs from the reference's.
mode harmonicity  0.957      n/a       n/a               NOT COMPARABLE -- measurable on the reference but NOT on the synth: fewer than two stable spectral modes were tracked. That asymmetry is itself the finding: the synth's signal character differs from the reference's.
beating rate      1.794 Hz   3.076 Hz  +1.282  drift     synth is too high on this axis
beating depth     0.415      0.668     +0.252  MISMATCH  synth is too high on this axis

-- onset character --
AXIS            REFERENCE  SYNTH     DELTA   VERDICT  NOTE
onset bite      922.887    1493.722  1.62x   ok       
onset flatness  0.144      0.281     +0.137  drift    synth is too high on this axis

-- perceptual character --
AXIS               REFERENCE  SYNTH  DELTA   VERDICT  NOTE
noise persistence  1.178      1.065  0.90x   ok       
air-band flatness  0.758      0.323  -0.435  drift    synth is too low on this axis

-- formants / body resonance --
AXIS            REFERENCE  SYNTH    DELTA  VERDICT  NOTE
first formant   n/a        1044 Hz  n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.
second formant  n/a        1976 Hz  n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.
bridge hill     n/a        1976 Hz  n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no formant peak found in this band. That asymmetry is itself the finding: the reference's signal character differs from the synth's.

-- cepstral timbre --
AXIS          REFERENCE  SYNTH   DELTA           VERDICT  NOTE
MFCC profile  -41.566    80.599  mean|Δ| 18.972  ok       

-- whole-signal statistics --
AXIS                REFERENCE    SYNTH         DELTA     VERDICT   NOTE
centroid slope      3169 Hz/s    243.9 Hz/s    -2925     MISMATCH  synth is too low on this axis
centroid curvature  -5098 Hz/s²  -933.8 Hz/s²  +4164     MISMATCH  synth is too high on this axis
centroid spread     1657 Hz      125.8 Hz      -92%      drift     synth is too low on this axis
loudness slope      -0.1531 1/s  -0.1385 1/s   +0.01455  ok        
loudness curvature  0.6357 1/s²  0.5159 1/s²   -0.1198   drift     synth is too low on this axis
loudness gesture    0.690        1.000         +0.310    drift     synth is too high on this axis
loudness spread     0.011        0.024         +0.013    ok        
flatness slope      0.366 1/s    0.01073 1/s   -0.3553   MISMATCH  synth is too low on this axis
flatness median     0.196        0.230         +0.035    ok        
crest median        2.265        3.091         1.36x     drift     synth is too high on this axis
```

</details>

<details><summary><code>refs organic</code> placement for candidate B</summary>

```
$ refs organic -instrument snare -pitch 0 -override snare_noise_tune=1.5,snare_tail_decay=21,snare_noise_decay=22,snare_tone2_freq=194,snare_tone_mix=0.9,snare_wire_mix=1.2,snare_noise_mix=0.4

=== snare — organic axes against the real-recording range (p10–p90 over 59 references) ===

AXIS                         VALUE     REAL p10  p50     p90      WHERE   POSITION
loudness modulation rate     0.672     0.672     0.672   1.34     inside  +0.00
loudness modulation depth    3.43      0.037     0.24    1.34     above   +2.44
modulation bands             0.874     0.448     0.839   0.928    inside  +0.07
modulation flatness          0.571     0.183     0.488   0.604    inside  +0.20
brightness modulation depth  0.000645  0.0162    0.0823  0.41     below   -0.21
partial bandwidth            n/a                                          does not apply to this signal
partial independence         n/a                                          does not apply to this signal
onset bite                   4.06e+06  0.0708    1       1.6e+03  above   +2528.27
onset flatness               0.538     0.114     0.26    0.407    above   +0.95
onset pitch glide            n/a                                          does not apply to this signal
residual centroid            n/a                                          does not apply to this signal
noise decay vs tone          n/a                                          no reliable fundamental: F0 detection returned 123.8 Hz at confidence 0.00 (below 0.65), so the harmonic-series assumption this measurement rests on does not hold
noise persistence            0.585     0.101     0.598   2.27     inside  -0.01
vibrato jitter               n/a                                          does not apply to this signal
crest median                 3.6       1.65      2.38    3.24     above   +0.76

POSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; 'low-n (n=k)' means fewer than 10 references qualified for that axis, too few to trust a band.
'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.
```

</details>


## oboe

Listener's words this round: **way too airy / breathy, needs warmness**

Bound axes (`familyQuestions["oboe"]`): `noise.persistence`, `timbre.brightness_ratio`, `formant.f1_hz`, `mod.am.flatness`, `env.attack`.

### Caveats for the listener

- **`env.attack` is EXEMPT from this family's gate.** The reference's 1.14 s
  attack is the PERFORMANCE, not the instrument: `oboe-A4` is a swelled note.
  Matching it needs `amp_attack` around 1.0 s, which makes the voice unable to
  speak under a sequencer trigger — a real regression traded for a number. An
  earlier draft of both candidates did exactly that; both now keep the shipped
  0.012 s tongued onset, so `attack` stays MISMATCH **by design**. Do not read
  that row as a defect, and do not judge "attack speed" against it.
- **`ring T60` is MISMATCH on the current version and on both candidates**
  (reference 0.39 s vs synth 1.54 s). It is not a bound axis, so the gate does
  not judge it, and it has the same root cause as the exempt attack: the
  reference is a swelled-and-released note while the synth holds a steady
  sustain. Worth an ear check ("does it hang on too long?") even though no
  question asks it.
- **A and B disagree about brightness on purpose.** A reopens the low-pass
  (`filter_cutoff` 7000 -> 9000) and lands `brightness ratio` 6.62 against the
  reference's 6.92. B never touches the filter and instead thins the
  fundamental (`gen2_gain`/`gen3_gain`) under a formant bank, landing 6.16. If
  one reads as "warm" and the other as "thin", that difference is the whole
  point of the pair.
- **`formant_mix` is 0.35 on B**, under the 0.5 the synth guide warns about: a
  fixed formant bank vocodes the source when mixed high and the oboe starts
  reading as a sung vowel rather than a reed. If B sounds "vocal" rather than
  "nasal", that is the knob.
- **Organic range: both candidates introduce a NEW exit.** The current
  version sits outside the real-recording band on `loudness modulation depth`
  (below), `modulation bands` (below) and `modulation flatness` (above). A and
  B add a fourth: **`partial independence` 0.0104 (A) / 0.0115 (B) against a
  p10 of 0.0917 — position -0.54, below the band**, where the current version
  reads 0.168 and is inside it (-0.37). The reference-matched diagnose says
  the same thing from the other side: the axis goes from `ok` (synth 0.532 vs
  reference 0.340) to `drift` (0.091 on A, 0.095 on B, "synth is too low on
  this axis"). Low partial independence means the partials all rise and fall
  together instead of breathing separately — the quality an ear tends to call
  *synthetic*. It is not a bound axis, so nothing in the gate looks at it.
  This is the cost of removing the constant noise bed, and it is worth an ear
  check on both candidates.

### Current version — full `refs diagnose -ablate`

```
$ refs diagnose -instrument oboe -ref oboe-A4 -ablate

=== oboe (reference: oboe-A4) ===

-- noise / harmonic balance --
AXIS                 REFERENCE  SYNTH    DELTA   VERDICT  NOTE
noise fraction       0.039      0.033    -0.006  ok       
spectral flatness    0.121      0.071    -0.050  drift    synth is too pure/tonal (missing noise character)
residual centroid    5039 Hz    4410 Hz  -12%    ok       
noise decay vs tone  3.906      n/a      n/a              NOT COMPARABLE -- measurable on the reference but NOT on the synth: both tracks still above -20 dB at clip end. That asymmetry is itself the finding: the synth's signal character differs from the reference's.

-- temporal envelope --
AXIS             REFERENCE  SYNTH        DELTA          VERDICT   NOTE
attack           1.140 s    0.020 s      57.0x short    MISMATCH  synth attack is far too fast
ring T60         0.390 s    1.540 s      3.9x long      MISMATCH  synth decay/ring is far too sustained (NOTE: synth clip ended before -60dB was reached; its value is a LOWER BOUND, not a measured T60)
release          0.210 s    0.090 s      2.3x short     MISMATCH  synth release is far too fast
decay T10        0.310 s    1.470 s      4.7x long      drift     synth is too high on this axis
decay T20        0.360 s    1.510 s      4.2x long      MISMATCH  synth is too high on this axis
decay T30        0.370 s    n/a          n/a                      NOT COMPARABLE -- measurable on the reference but NOT on the synth: clip ended before -30 dB was reached (still above it at 1.54s), so this is a lower bound rather than a measured decay time. That asymmetry is itself the finding: the synth's signal character differs from the reference's.
energy sections  0.203      0.273        mean|Δ| 0.043  ok        
decay            0.000 s    0.010 s      n/a            MISMATCH  synth decay/ring is far too sustained
decay rate       n/a        -100.4 dB/s  n/a                      NOT COMPARABLE -- measurable on the synth but NOT on the reference: no decay region (the peak is the last audible frame). That asymmetry is itself the finding: the reference's signal character differs from the synth's.
peak time        1.160 s    0.020 s      58.0x short    MISMATCH  synth is too low on this axis
swell            0.000 s    0.000 s      n/a            ok        
total audible    1.560 s    1.570 s      1.0x long      ok        

-- partial structure --
AXIS                      REFERENCE  SYNTH  DELTA   VERDICT   NOTE
inharmonicity             0.000      0.006  +0.006  MISMATCH  synth partials are too inharmonic
partial series deviation  0.095      0.110  +0.015  ok        
spectral irregularity     3.421      2.467  -0.954  ok        
  ref partials (x f0):   1.00, 2.00, 3.00, 4.00, 6.00, 7.00, 8.00, 10.00
  synth partials (x f0): 1.00, 2.00, 3.00, 3.01, 4.00, 5.00, 6.00, 7.00

-- brightness / formant --
AXIS      REFERENCE  SYNTH    DELTA  VERDICT  NOTE
centroid  3044 Hz    3403 Hz  +12%   ok       
rolloff   1322 Hz    1760 Hz  +33%   ok       

-- modulation --
AXIS                         REFERENCE  SYNTH     DELTA          VERDICT  NOTE
sustain modulation           0.044      0.031     -0.014         ok       
loudness modulation rate     0.6719 Hz  1.008 Hz  +0.3359        ok       
loudness modulation depth    0.029      0.006     -0.023         ok       
modulation bands             0.479      0.188     mean|Δ| 0.194  drift    
modulation flatness          0.687      0.763     +0.076         drift    synth is too high on this axis
brightness modulation depth  0.011      0.018     +0.007         ok       

-- resolved decay --
AXIS                      REFERENCE         SYNTH                 DELTA                                VERDICT   NOTE
decay per mode            -10.96 dB/s       -0.1326 dB/s          mean|Δ| 10.5 dB/s 10.480             ok        
decay slope vs frequency  1.2 dB/s per kHz  -0.4292 dB/s per kHz  -1.629                               ok        
band T60                  -1.000 s          -1.000 s              mean 13.78x (worst 2k 33.27x) 3.785  MISMATCH  synth is too high on this axis

-- partial decorrelation --
AXIS                  REFERENCE  SYNTH  DELTA   VERDICT  NOTE
partial independence  0.340      0.532  +0.192  ok       

-- struck string --
AXIS                   REFERENCE     SYNTH         DELTA            VERDICT   NOTE
string stiffness B     0.000         0.000         +0.000           ok        
double-decay ratio     0.008         137.891       16424.58x        MISMATCH  synth is too high on this axis
prompt decay rate      3.362         4.110         +0.747           ok        
aftersound decay rate  400.500       0.030         -400.470         MISMATCH  synth is too low on this axis
partial stretch        -9.471 cents  0.0388 cents  mean|Δ| 128.442  drift     

-- wind attack --
AXIS                        REFERENCE  SYNTH     DELTA            VERDICT   NOTE
attack rise time            0.337 s    0.006 s   58.0x short      MISMATCH  synth is too low on this axis
attack brightening          531 Hz     -2580 Hz  -586%            MISMATCH  synth is too low on this axis
temporal centroid           0.751 s    0.745 s   1.0x short       ok        
attack centroid trajectory  2256 Hz    3529 Hz   mean|Δ| 888.887  drift     

-- modal structure --
AXIS              REFERENCE  SYNTH     DELTA                       VERDICT  NOTE
mode anchor       440 Hz     440 Hz    +0%                         ok       
mode ratios       0 cents    0 cents   mean gap 148 cents 147.662  ok       
mode density      0.791      0.949     1.20x                       ok       
mode harmonicity  1.000      1.000     +0.000                      ok       
beating rate      2.307 Hz   4.084 Hz  +1.777                      drift    synth is too high on this axis
beating depth     0.319      0.092     -0.227                      drift    synth is too low on this axis

-- onset character --
AXIS               REFERENCE     SYNTH       DELTA   VERDICT  NOTE
onset bite         0.253         2.171       8.58x   ok       
onset flatness     0.081         0.126       +0.046  ok       
onset pitch glide  -17.42 cents  4.42 cents  +21.84  ok       

-- perceptual character --
AXIS               REFERENCE      SYNTH          DELTA          VERDICT  NOTE
brightness ratio   6.916          7.733          1.12x          ok       
harmonic slope     -10.41 dB/oct  -13.27 dB/oct  -2.858         ok       
tristimulus        0.022          0.243          mean|Δ| 0.205  ok       
noise persistence  0.338          0.860          2.54x          ok       
air-band flatness  0.302          0.098          -0.203         drift    synth is too low on this axis

-- vibrato --
AXIS              REFERENCE  SYNTH        DELTA  VERDICT  NOTE
vibrato rate      n/a        5.65 Hz      n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no vibrato detected (band power below the noise floor, or extent under 2 cents). That asymmetry is itself the finding: the reference's signal character differs from the synth's.
vibrato extent    n/a        2.107 cents  n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no vibrato detected (band power below the noise floor, or extent under 2 cents). That asymmetry is itself the finding: the reference's signal character differs from the synth's.
vibrato jitter    n/a        0.389        n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no vibrato detected (band power below the noise floor, or extent under 2 cents). That asymmetry is itself the finding: the reference's signal character differs from the synth's.
vibrato AM depth  n/a        0.007        n/a             NOT COMPARABLE -- measurable on the synth but NOT on the reference: no vibrato detected (band power below the noise floor, or extent under 2 cents). That asymmetry is itself the finding: the reference's signal character differs from the synth's.

-- formants / body resonance --
AXIS            REFERENCE  SYNTH    DELTA  VERDICT  NOTE
first formant   1472 Hz    2449 Hz  +66%   ok       
second formant  2589 Hz    1583 Hz  -39%   drift    synth is too low on this axis
bridge hill     2589 Hz    2449 Hz  -5%    ok       

-- cepstral timbre --
AXIS          REFERENCE  SYNTH    DELTA           VERDICT  NOTE
MFCC profile  209.462    218.161  mean|Δ| 11.539  ok       

-- whole-signal statistics --
AXIS                  REFERENCE      SYNTH          DELTA      VERDICT   NOTE
centroid slope        -142.5 Hz/s    -223.5 Hz/s    -81.09     ok        
centroid curvature    -749 Hz/s²     493.6 Hz/s²    +1243      drift     synth is too high on this axis
centroid spread       89.55 Hz       61.28 Hz       -32%       ok        
loudness slope        -0.01482 1/s   -0.02554 1/s   -0.01071   ok        
loudness curvature    -0.2285 1/s²   -0.06548 1/s²  +0.1631    MISMATCH  synth is too high on this axis
loudness gesture      0.554          0.523          -0.031     ok        
loudness spread       0.029          0.003          -0.026     drift     synth is too low on this axis
flatness slope        -0.003416 1/s  -0.01271 1/s   -0.009295  ok        
flatness median       0.070          0.047          -0.023     ok        
noise-fraction slope  -0.02922 1/s   -0.004654 1/s  +0.02456   ok        
peak noise fraction   0.017          0.023          +0.006     ok        
crest median          1.977          2.587          1.31x      drift     synth is too high on this axis
harmonic bloom        0.4803 dB/s    0.1272 dB/s    -0.3531    ok        


-- stage sensitivity (which stage moves each MISMATCH axis; not a decomposition, does not sum) --
AXIS                   TOP STAGES (Δnorm above the no-effect floor when the stage is at identity)
attack                 gen1 0.00  gen2 0.00  gen3 0.00
ring T60               gen1 0.01  filter 0.00  gen2 0.00
release                gen1 0.00  gen2 0.00  gen3 0.00
decay T20              gen1 0.00  gen2 0.00  gen3 0.00
decay                  gen1 0.94  gen4 0.06  gen2 0.00
peak time              gen1 0.00  gen2 0.00  gen3 0.00
inharmonicity          gen1 5.75  gen3 0.63  gen5 0.61
band T60               gen6 0.51  gen2 0.07  gen1 0.06
double-decay ratio     gen1 0.90  gen4 0.13  gen2 0.11
aftersound decay rate  gen1 0.00  gen2 0.00  gen4 0.00
attack rise time       gen1 0.00  gen2 0.00  gen3 0.00
attack brightening     gen1 0.00  gen2 0.00  gen3 0.00
loudness curvature     gen1 0.02  filter 0.01  filtenv 0.01
```

<details><summary>Current version — <code>refs organic</code> placement</summary>

```
$ refs organic -instrument oboe -pitch 0

=== oboe — organic axes against the real-recording range (p10–p90 over 59 references) ===

AXIS                         VALUE     REAL p10  p50      p90       WHERE   POSITION
loudness modulation rate     0.672     0.672     0.672    1.34      inside  +0.00
loudness modulation depth    0.00942   0.037     0.24     1.34      below   -0.18
modulation bands             0.241     0.448     0.839    0.928     below   -1.25
modulation flatness          0.728     0.183     0.488    0.604     above   +0.57
brightness modulation depth  0.0242    0.0162    0.0823   0.41      inside  -0.15
partial bandwidth            0         0         0        1.51      inside  +0.00
partial independence         0.168     0.0917    0.503    1         inside  -0.37
onset bite                   2.21      0.0708    1        1.6e+03   inside  +0.00
onset flatness               0.27      0.114     0.26     0.407     inside  +0.03
onset pitch glide            4.98      -82.9     -2.46    26.9      inside  +0.07
residual centroid            4.47e+03  1.92e+03  3.1e+03  5.58e+03  inside  +0.37
noise decay vs tone          1         0.248     0.875    3.41      inside  +0.04
noise persistence            0.878     0.101     0.598    2.27      inside  +0.13
vibrato jitter               n/a                                            no vibrato detected (band power below the noise floor, or extent under 2 cents)
crest median                 2.57      1.65      2.38     3.24      inside  +0.12

POSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; 'low-n (n=k)' means fewer than 10 references qualified for that axis, too few to trust a band.
'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.
```

</details>

### Candidate A — reed chiff instead of a breath bed

Overrides:

```json
{
  "gen1_env_tail_mix": 0.0,
  "gen1_env_fast_mix": 1.5,
  "gen1_env_fast_rate": 4,
  "amp_release": 0.13,
  "filter_cutoff": 9000
}
```

Hypothesis (verbatim from `audition_candidates.json`):

> The listener's words are 'way too airy/breathy, needs warmness', and the cause is structural: the reed-buzz slot gen1 runs env_tail_mix 0.6 at env_tail_rate 0.0 -- a CONSTANT noise bed that never decays -- which is exactly what noise persistence measures (late-half noise / early-half noise) and why it reads 0.860 against the reference's 0.338. Zeroing gen1_env_tail_mix and turning the slot into a decaying chiff (env_fast_mix 0.6 -> 1.5, env_fast_rate 10 -> 4) gives a breath that BLOWS and then goes, like a tongued reed: noise persistence 0.860 -> 0.422, closing 84% of the gap. That edit alone overshoots the warmth -- brightness ratio falls 7.73 -> 5.93, past the reference's 6.92 and further from it than the current version -- so filter_cutoff 7000 -> 9000 lets the upper reed formant back through and lands 6.62, closing 64% of the brightness gap from the correct side. amp_release 0.10 -> 0.13 settles modulation flatness at 0.689 vs the reference's 0.687 (drift -> ok), which is the 'steadiness' question's axis. amp_attack stays at its shipped 0.012 s: a tongued oboe speaks in tens of milliseconds, and the reference's 1.14 s attack is that PERFORMANCE's swell, not the instrument (env.attack is a recorded gate exemption for this family). Every bound axis lands ok, and the PREDICTIVE axis improves: inharmonicity 0.00594 -> 0.00326 against the reference's 0.00038.

<details><summary>Full <code>refs diagnose</code> output for candidate A</summary>

```
$ refs diagnose -instrument oboe -ref oboe-A4 -override gen1_env_tail_mix=0,gen1_env_fast_mix=1.5,gen1_env_fast_rate=4,amp_release=0.13,filter_cutoff=9000

=== oboe (reference: oboe-A4) ===

-- noise / harmonic balance --
AXIS                 REFERENCE  SYNTH    DELTA   VERDICT  NOTE
noise fraction       0.039      0.022    -0.017  drift    synth is too pure/tonal (missing noise character)
spectral flatness    0.121      0.062    -0.058  drift    synth is too pure/tonal (missing noise character)
residual centroid    5039 Hz    4551 Hz  -10%    ok       
noise decay vs tone  3.906      0.977    0.25x   drift    synth is too low on this axis

-- temporal envelope --
AXIS             REFERENCE  SYNTH        DELTA          VERDICT   NOTE
attack           1.140 s    0.020 s      57.0x short    MISMATCH  synth attack is far too fast
ring T60         0.390 s    1.540 s      3.9x long      MISMATCH  synth decay/ring is far too sustained (NOTE: synth clip ended before -60dB was reached; its value is a LOWER BOUND, not a measured T60)
release          0.210 s    0.120 s      1.8x short     drift     synth release is far too fast
decay T10        0.310 s    1.450 s      4.7x long      drift     synth is too high on this axis
decay T20        0.360 s    1.500 s      4.2x long      MISMATCH  synth is too high on this axis
decay T30        0.370 s    n/a          n/a                      NOT COMPARABLE -- measurable on the reference but NOT on the synth: clip ended before -30 dB was reached (still above it at 1.54s), so this is a lower bound rather than a measured decay time. That asymmetry is itself the finding: the synth's signal character differs from the reference's.
energy sections  0.203      0.282        mean|Δ| 0.040  ok        
decay            0.000 s    0.010 s      n/a            MISMATCH  synth decay/ring is far too sustained
decay rate       n/a        -104.9 dB/s  n/a                      NOT COMPARABLE -- measurable on the synth but NOT on the reference: no decay region (the peak is the last audible frame). That asymmetry is itself the finding: the reference's signal character differs from the synth's.
peak time        1.160 s    0.020 s      58.0x short    MISMATCH  synth is too low on this axis
swell            0.000 s    0.000 s      n/a            ok        
total audible    1.560 s    1.570 s      1.0x long      ok        

-- partial structure --
AXIS                      REFERENCE  SYNTH  DELTA   VERDICT   NOTE
inharmonicity             0.000      0.003  +0.003  MISMATCH  synth partials are too inharmonic
partial series deviation  0.095      0.110  +0.015  ok        
spectral irregularity     3.421      2.779  -0.642  ok        
  ref partials (x f0):   1.00, 2.00, 3.00, 4.00, 6.00, 7.00, 8.00, 10.00
  synth partials (x f0): 1.00, 2.00, 3.00, 3.01, 4.00, 5.00, 6.00, 7.00

-- brightness / formant --
AXIS      REFERENCE  SYNTH    DELTA  VERDICT  NOTE
centroid  3044 Hz    2913 Hz  -4%    ok       
rolloff   1322 Hz    1760 Hz  +33%   ok       

-- modulation --
AXIS                         REFERENCE  SYNTH     DELTA          VERDICT  NOTE
sustain modulation           0.044      0.030     -0.014         ok       
loudness modulation rate     0.6719 Hz  1.008 Hz  +0.3359        ok       
loudness modulation depth    0.029      0.011     -0.018         ok       
modulation bands             0.479      0.264     mean|Δ| 0.143  drift    
modulation flatness          0.687      0.689     +0.002         ok       
brightness modulation depth  0.011      0.075     +0.064         ok       

-- resolved decay --
AXIS                      REFERENCE         SYNTH                 DELTA                               VERDICT   NOTE
decay per mode            -10.96 dB/s       -0.3644 dB/s          mean|Δ| 11.6 dB/s 11.618            ok        
decay slope vs frequency  1.2 dB/s per kHz  -0.9313 dB/s per kHz  -2.131                              ok        
band T60                  -1.000 s          -1.000 s              mean 8.00x (worst 2k 21.32x) 3.000  MISMATCH  synth is too high on this axis

-- partial decorrelation --
AXIS                  REFERENCE  SYNTH  DELTA   VERDICT  NOTE
partial independence  0.340      0.091  -0.249  drift    synth is too low on this axis

-- struck string --
AXIS                   REFERENCE     SYNTH          DELTA            VERDICT   NOTE
string stiffness B     0.000         0.000          +0.000           ok        
double-decay ratio     0.008         139.483        16614.30x        MISMATCH  synth is too high on this axis
prompt decay rate      3.362         5.011          +1.648           ok        
aftersound decay rate  400.500       0.036          -400.464         MISMATCH  synth is too low on this axis
partial stretch        -9.471 cents  0.08726 cents  mean|Δ| 128.458  drift     

-- wind attack --
AXIS                        REFERENCE  SYNTH     DELTA             VERDICT   NOTE
attack rise time            0.337 s    0.006 s   58.0x short       MISMATCH  synth is too low on this axis
attack brightening          531 Hz     -5155 Hz  -1070%            MISMATCH  synth is too low on this axis
temporal centroid           0.751 s    0.733 s   1.0x short        ok        
attack centroid trajectory  2256 Hz    4246 Hz   mean|Δ| 1391.029  drift     

-- modal structure --
AXIS              REFERENCE  SYNTH     DELTA                       VERDICT   NOTE
mode anchor       440 Hz     440 Hz    +0%                         ok        
mode ratios       0 cents    0 cents   mean gap 148 cents 147.660  ok        
mode density      0.791      0.949     1.20x                       ok        
mode harmonicity  1.000      1.000     +0.000                      ok        
beating rate      2.307 Hz   7.054 Hz  +4.747                      MISMATCH  synth is too high on this axis
beating depth     0.319      0.141     -0.179                      drift     synth is too low on this axis

-- onset character --
AXIS               REFERENCE     SYNTH        DELTA   VERDICT  NOTE
onset bite         0.253         4.407        17.42x  ok       
onset flatness     0.081         0.196        +0.115  drift    synth is too high on this axis
onset pitch glide  -17.42 cents  4.467 cents  +21.89  ok       

-- perceptual character --
AXIS               REFERENCE      SYNTH          DELTA          VERDICT  NOTE
brightness ratio   6.916          6.618          0.96x          ok       
harmonic slope     -10.41 dB/oct  -16.25 dB/oct  -5.842         ok       
tristimulus        0.022          0.246          mean|Δ| 0.205  ok       
noise persistence  0.338          0.422          1.25x          ok       
air-band flatness  0.302          0.111          -0.191         drift    synth is too low on this axis

-- vibrato --
AXIS              REFERENCE  SYNTH  DELTA  VERDICT  NOTE
vibrato rate      n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato extent    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato jitter    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato AM depth  n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)

-- formants / body resonance --
AXIS            REFERENCE  SYNTH    DELTA  VERDICT  NOTE
first formant   1472 Hz    2457 Hz  +67%   ok       
second formant  2589 Hz    1558 Hz  -40%   drift    synth is too low on this axis
bridge hill     2589 Hz    2457 Hz  -5%    ok       

-- cepstral timbre --
AXIS          REFERENCE  SYNTH    DELTA           VERDICT  NOTE
MFCC profile  209.462    193.534  mean|Δ| 11.449  ok       

-- whole-signal statistics --
AXIS                  REFERENCE      SYNTH          DELTA     VERDICT   NOTE
centroid slope        -142.5 Hz/s    -1494 Hz/s     -1352     MISMATCH  synth is too low on this axis
centroid curvature    -749 Hz/s²     1933 Hz/s²     +2682     drift     synth is too high on this axis
centroid spread       89.55 Hz       934.4 Hz       +943%     MISMATCH  synth is too high on this axis
loudness slope        -0.01458 1/s   -0.03774 1/s   -0.02316  ok        
loudness curvature    -0.2247 1/s²   -0.09091 1/s²  +0.1338   drift     synth is too high on this axis
loudness gesture      0.554          0.569          +0.015    ok        
loudness spread       0.029          0.003          -0.025    drift     synth is too low on this axis
flatness slope        -0.003416 1/s  -0.07201 1/s   -0.06859  drift     synth is too low on this axis
flatness median       0.070          0.013          -0.058    MISMATCH  synth is too low on this axis
noise-fraction slope  -0.02922 1/s   -0.0164 1/s    +0.01282  ok        
peak noise fraction   0.017          0.025          +0.008    ok        
crest median          1.977          2.390          1.21x     drift     synth is too high on this axis
harmonic bloom        0.4803 dB/s    0.03348 dB/s   -0.4468   ok        
```

</details>

<details><summary><code>refs organic</code> placement for candidate A</summary>

```
$ refs organic -instrument oboe -pitch 0 -override gen1_env_tail_mix=0,gen1_env_fast_mix=1.5,gen1_env_fast_rate=4,amp_release=0.13,filter_cutoff=9000

=== oboe — organic axes against the real-recording range (p10–p90 over 59 references) ===

AXIS                         VALUE     REAL p10  p50      p90       WHERE   POSITION
loudness modulation rate     0.672     0.672     0.672    1.34      inside  +0.00
loudness modulation depth    0.0154    0.037     0.24     1.34      below   -0.17
modulation bands             0.325     0.448     0.839    0.928     below   -1.07
modulation flatness          0.657     0.183     0.488    0.604     above   +0.40
brightness modulation depth  0.268     0.0162    0.0823   0.41      inside  +0.47
partial bandwidth            0         0         0        1.51      inside  +0.00
partial independence         0.0104    0.0917    0.503    1         below   -0.54
onset bite                   4.49      0.0708    1        1.6e+03   inside  +0.00
onset flatness               0.286     0.114     0.26     0.407     inside  +0.09
onset pitch glide            5.01      -82.9     -2.46    26.9      inside  +0.07
residual centroid            4.72e+03  1.92e+03  3.1e+03  5.58e+03  inside  +0.44
noise decay vs tone          0.982     0.248     0.875    3.41      inside  +0.03
noise persistence            0.394     0.101     0.598    2.27      inside  -0.09
vibrato jitter               n/a                                            no vibrato detected (band power below the noise floor, or extent under 2 cents)
crest median                 2.37      1.65      2.38     3.24      inside  -0.01

POSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; 'low-n (n=k)' means fewer than 10 references qualified for that axis, too few to trust a band.
'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.
```

</details>

### Candidate B — formant bank on the nasal peak

Overrides:

```json
{
  "gen1_env_tail_mix": 0.0,
  "gen1_env_fast_mix": 1.5,
  "gen1_env_fast_rate": 4,
  "amp_release": 0.13,
  "formant_enabled": 1,
  "formant_vowel": 0,
  "formant_voice_type": 1,
  "formant_shift": 1.2,
  "formant_mix": 0.35,
  "gen2_gain": 0.24,
  "gen3_gain": 0.08
}
```

Hypothesis (verbatim from `audition_candidates.json`):

> A's noise fix and amp_release, then a structurally different answer to the warmth: no filter change at all, and instead the FORMANT stage plus a reed-shaped harmonic bank. first formant reads 2449 Hz against the reference's 1472 Hz (+66%) -- the additive bank has no resonance there -- so formant_enabled 1 with vowel 'a' on voice_type 1 and formant_shift 1.2 places the bank's first formant at 1521 Hz, +3% of the reference. formant_mix is held at 0.35, under the 0.5 the guide warns about: above it a fixed formant bank vocodes the source and the oboe starts reading as a sung vowel rather than a reed. The brightness comes back not by opening a filter but by THINNING the fundamental -- gen2_gain (H1) 0.42 -> 0.24 and gen3_gain (H2) 0.12 -> 0.08 -- which is the real oboe's own spectrum (a weak H1 under an H3 peak, the measured reference reads H1 1.0 / H2 0.26 / H3 1.39): brightness ratio 7.73 -> 6.16 vs the reference's 6.92, a 7% closer gap. It also sharpens the breath fix: noise persistence 0.860 -> 0.374, closing 93% of the gap, because there is less fundamental for the residual to hide under. modulation flatness 0.689 (ok), inharmonicity 0.00594 -> 0.00326 (PREDICTIVE, improved).

<details><summary>Full <code>refs diagnose</code> output for candidate B</summary>

```
$ refs diagnose -instrument oboe -ref oboe-A4 -override gen1_env_tail_mix=0,gen1_env_fast_mix=1.5,gen1_env_fast_rate=4,amp_release=0.13,formant_enabled=1,formant_vowel=0,formant_voice_type=1,formant_shift=1.2,formant_mix=0.35,gen2_gain=0.24,gen3_gain=0.08

=== oboe (reference: oboe-A4) ===

-- noise / harmonic balance --
AXIS                 REFERENCE  SYNTH    DELTA   VERDICT  NOTE
noise fraction       0.039      0.022    -0.017  drift    synth is too pure/tonal (missing noise character)
spectral flatness    0.121      0.045    -0.076  drift    synth is too pure/tonal (missing noise character)
residual centroid    5039 Hz    4024 Hz  -20%    ok       
noise decay vs tone  3.906      0.977    0.25x   drift    synth is too low on this axis

-- temporal envelope --
AXIS             REFERENCE  SYNTH        DELTA          VERDICT   NOTE
attack           1.140 s    0.020 s      57.0x short    MISMATCH  synth attack is far too fast
ring T60         0.390 s    1.540 s      3.9x long      MISMATCH  synth decay/ring is far too sustained (NOTE: synth clip ended before -60dB was reached; its value is a LOWER BOUND, not a measured T60)
release          0.210 s    0.120 s      1.8x short     drift     synth release is far too fast
decay T10        0.310 s    1.450 s      4.7x long      drift     synth is too high on this axis
decay T20        0.360 s    1.500 s      4.2x long      MISMATCH  synth is too high on this axis
decay T30        0.370 s    n/a          n/a                      NOT COMPARABLE -- measurable on the reference but NOT on the synth: clip ended before -30 dB was reached (still above it at 1.54s), so this is a lower bound rather than a measured decay time. That asymmetry is itself the finding: the synth's signal character differs from the reference's.
energy sections  0.203      0.282        mean|Δ| 0.040  ok        
decay            0.000 s    0.010 s      n/a            MISMATCH  synth decay/ring is far too sustained
decay rate       n/a        -116.5 dB/s  n/a                      NOT COMPARABLE -- measurable on the synth but NOT on the reference: no decay region (the peak is the last audible frame). That asymmetry is itself the finding: the reference's signal character differs from the synth's.
peak time        1.160 s    0.020 s      58.0x short    MISMATCH  synth is too low on this axis
swell            0.000 s    0.000 s      n/a            ok        
total audible    1.560 s    1.570 s      1.0x long      ok        

-- partial structure --
AXIS                      REFERENCE  SYNTH  DELTA   VERDICT   NOTE
inharmonicity             0.000      0.003  +0.003  MISMATCH  synth partials are too inharmonic
partial series deviation  0.095      0.110  +0.015  ok        
spectral irregularity     3.421      2.921  -0.500  ok        
  ref partials (x f0):   1.00, 2.00, 3.00, 4.00, 6.00, 7.00, 8.00, 10.00
  synth partials (x f0): 1.00, 2.00, 3.00, 3.01, 4.00, 5.00, 6.00, 7.00

-- brightness / formant --
AXIS      REFERENCE  SYNTH    DELTA  VERDICT  NOTE
centroid  3044 Hz    2709 Hz  -11%   ok       
rolloff   1322 Hz    1760 Hz  +33%   ok       

-- modulation --
AXIS                         REFERENCE  SYNTH     DELTA          VERDICT  NOTE
sustain modulation           0.044      0.030     -0.014         ok       
loudness modulation rate     0.6719 Hz  1.008 Hz  +0.3359        ok       
loudness modulation depth    0.029      0.011     -0.018         ok       
modulation bands             0.479      0.265     mean|Δ| 0.143  drift    
modulation flatness          0.687      0.689     +0.002         ok       
brightness modulation depth  0.011      0.063     +0.052         ok       

-- resolved decay --
AXIS                      REFERENCE         SYNTH                 DELTA                               VERDICT   NOTE
decay per mode            -10.96 dB/s       -0.3552 dB/s          mean|Δ| 11.6 dB/s 11.628            ok        
decay slope vs frequency  1.2 dB/s per kHz  -0.9393 dB/s per kHz  -2.139                              ok        
band T60                  -1.000 s          -1.000 s              mean 7.99x (worst 2k 21.38x) 2.999  MISMATCH  synth is too high on this axis

-- partial decorrelation --
AXIS                  REFERENCE  SYNTH  DELTA   VERDICT  NOTE
partial independence  0.340      0.095  -0.245  drift    synth is too low on this axis

-- struck string --
AXIS                   REFERENCE     SYNTH          DELTA            VERDICT   NOTE
string stiffness B     0.000         0.000          +0.000           ok        
double-decay ratio     0.008         117.522        13998.55x        MISMATCH  synth is too high on this axis
prompt decay rate      3.362         5.272          +1.910           ok        
aftersound decay rate  400.505       0.045          -400.460         MISMATCH  synth is too low on this axis
partial stretch        -9.471 cents  0.09011 cents  mean|Δ| 128.458  drift     

-- wind attack --
AXIS                        REFERENCE  SYNTH     DELTA             VERDICT   NOTE
attack rise time            0.337 s    0.006 s   58.0x short       MISMATCH  synth is too low on this axis
attack brightening          531 Hz     -4404 Hz  -929%             MISMATCH  synth is too low on this axis
temporal centroid           0.751 s    0.733 s   1.0x short        ok        
attack centroid trajectory  2256 Hz    3861 Hz   mean|Δ| 1088.723  drift     

-- modal structure --
AXIS              REFERENCE  SYNTH     DELTA                       VERDICT   NOTE
mode anchor       440 Hz     440 Hz    +0%                         ok        
mode ratios       0 cents    0 cents   mean gap 148 cents 147.662  ok        
mode density      0.791      0.949     1.20x                       ok        
mode harmonicity  1.000      1.000     +0.000                      ok        
beating rate      2.307 Hz   6.311 Hz  +4.004                      MISMATCH  synth is too high on this axis
beating depth     0.319      0.141     -0.179                      drift     synth is too low on this axis

-- onset character --
AXIS               REFERENCE     SYNTH         DELTA   VERDICT   NOTE
onset bite         0.253         4.460         17.63x  ok        
onset flatness     0.081         0.280         +0.199  MISMATCH  synth is too high on this axis
onset pitch glide  -17.42 cents  -6.077 cents  +11.35  ok        

-- perceptual character --
AXIS               REFERENCE      SYNTH          DELTA          VERDICT  NOTE
brightness ratio   6.916          6.155          0.89x          ok       
harmonic slope     -10.41 dB/oct  -15.53 dB/oct  -5.125         ok       
tristimulus        0.022          0.081          mean|Δ| 0.099  ok       
noise persistence  0.338          0.374          1.11x          ok       
air-band flatness  0.302          0.075          -0.226         drift    synth is too low on this axis

-- vibrato --
AXIS              REFERENCE  SYNTH  DELTA  VERDICT  NOTE
vibrato rate      n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato extent    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato jitter    n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)
vibrato AM depth  n/a        n/a    n/a             NOT MEASURABLE on either side: no vibrato detected (band power below the noise floor, or extent under 2 cents)

-- formants / body resonance --
AXIS            REFERENCE  SYNTH    DELTA  VERDICT  NOTE
first formant   1472 Hz    1521 Hz  +3%    ok       
second formant  2589 Hz    2455 Hz  -5%    ok       
bridge hill     2589 Hz    2455 Hz  -5%    ok       

-- cepstral timbre --
AXIS          REFERENCE  SYNTH    DELTA           VERDICT  NOTE
MFCC profile  209.462    180.504  mean|Δ| 12.464  ok       

-- whole-signal statistics --
AXIS                  REFERENCE      SYNTH          DELTA     VERDICT   NOTE
centroid slope        -142.5 Hz/s    -1205 Hz/s     -1062     drift     synth is too low on this axis
centroid curvature    -749 Hz/s²     1591 Hz/s²     +2340     drift     synth is too high on this axis
centroid spread       89.55 Hz       736.1 Hz       +722%     MISMATCH  synth is too high on this axis
loudness slope        -0.01071 1/s   -0.02772 1/s   -0.01701  ok        
loudness curvature    -0.1652 1/s²   -0.06691 1/s²  +0.09826  drift     synth is too high on this axis
loudness gesture      0.554          0.562          +0.008    ok        
loudness spread       0.021          0.002          -0.019    ok        
flatness slope        -0.003415 1/s  -0.0519 1/s    -0.04848  drift     synth is too low on this axis
flatness median       0.070          0.009          -0.061    MISMATCH  synth is too low on this axis
noise-fraction slope  -0.02922 1/s   -0.01626 1/s   +0.01296  ok        
peak noise fraction   0.017          0.023          +0.006    ok        
crest median          1.977          2.489          1.26x     drift     synth is too high on this axis
harmonic bloom        0.4803 dB/s    0.03642 dB/s   -0.4439   ok        
```

</details>

<details><summary><code>refs organic</code> placement for candidate B</summary>

```
$ refs organic -instrument oboe -pitch 0 -override gen1_env_tail_mix=0,gen1_env_fast_mix=1.5,gen1_env_fast_rate=4,amp_release=0.13,formant_enabled=1,formant_vowel=0,formant_voice_type=1,formant_shift=1.2,formant_mix=0.35,gen2_gain=0.24,gen3_gain=0.08

=== oboe — organic axes against the real-recording range (p10–p90 over 59 references) ===

AXIS                         VALUE     REAL p10  p50      p90       WHERE   POSITION
loudness modulation rate     0.672     0.672     0.672    1.34      inside  +0.00
loudness modulation depth    0.0155    0.037     0.24     1.34      below   -0.17
modulation bands             0.325     0.448     0.839    0.928     below   -1.07
modulation flatness          0.657     0.183     0.488    0.604     above   +0.40
brightness modulation depth  0.233     0.0162    0.0823   0.41      inside  +0.38
partial bandwidth            0         0         0        1.51      inside  +0.00
partial independence         0.0115    0.0917    0.503    1         below   -0.54
onset bite                   4.54      0.0708    1        1.6e+03   inside  +0.00
onset flatness               0.3       0.114     0.26     0.407     inside  +0.14
onset pitch glide            4.98      -82.9     -2.46    26.9      inside  +0.07
residual centroid            4.15e+03  1.92e+03  3.1e+03  5.58e+03  inside  +0.29
noise decay vs tone          0.982     0.248     0.875    3.41      inside  +0.03
noise persistence            0.378     0.101     0.598    2.27      inside  -0.10
vibrato jitter               n/a                                            no vibrato detected (band power below the noise floor, or extent under 2 cents)
crest median                 2.55      1.65      2.38     3.24      inside  +0.10

POSITION = (value - p50) / (p90 - p10). WHERE flags values outside the band real recordings occupy; 'low-n (n=k)' means fewer than 10 references qualified for that axis, too few to trust a band.
'modulation bands' places the gesture-band (0.5-2Hz) share of the loudness-modulation spectrum — the vector's Primary() component.
```

</details>
