# Genre-Defining Masterpiece Templates — Design

**Date:** 2026-06-20
**Status:** Draft for review
**Branch:** add-core-node-types

## 1. Goal

Replace **all** existing built-in template circuits with a curated set of **15 templates, one foundational/genre-defining masterpiece per genre**, recreated as faithfully as the loop engine allows. Selection criterion (user directive): the most **representative, creative, and considered masterpieces** — *not* simple riff-songs (explicitly excluded: Seven Nation Army, Smells Like Teen Spirit, and similar three-chord/four-note structures).

This is the long-deferred "Plan 3 (circuits)" from the orchestral-instruments work. The aim is to push the engine to recreate real songs as closely as possible so the user can A/B each template against the original recording.

**Non-goals / out of scope:**
- The startup demo circuit (`internal/assets/startup_demo.json`) is **untouched** — it is separate from `templates/` and remains the rock demo shown on launch.
- Vocals/lyrics are not reproduced. We recreate the **instrumental signature**: riffs, grooves, basslines, chord stabs, montunos, melodies.
- Full improvised solos are not transcribed; we capture heads, riffs, and signature phrases.

## 2. How the engine constrains the music (authoring model)

Templates are **generated**, not hand-edited. The source of truth is `showcases()` in `src/go/cmd/gen-showcase/specs.go`; `gen-showcase` lays each row out as a directed node-loop around a grid perimeter and writes `internal/assets/templates/<stem>.json`. A drift test (`cmd/gen-showcase/drift_test.go`) pins JSON↔spec, and `internal/assets/templates.go` registers the menu order.

Authoring therefore happens at the spec level: per row, a list of `Hit{Step, Pitch, Vol, Dur}`.

**Hard constraints (verified in `gen-showcase/main.go`):**
- **16th-note grid, 4/4-locked.** `totalSteps = Bars*16` regardless of `Subdiv`; quarter anchors sit at every `totalSteps/4`. One bar = 16 steps (beat 1=0, 2=4, 3=8, 4=12; offbeats 2,6,10,14; 16ths fill the rest). **No true triplets, no 5/4, no 3/4.** Swing/shuffle is faked by placing the "and" late (step 6/14 instead of an even 8th) and softening velocity.
- **Loop length is free** via `Bars` (8/12/16/32…). All rows in a template share the same loop length.
- **Pitch = semitones from A3** (A3=0). Keys are transposed to fit; **interval/contour is preserved** for recognizability. Pitch reference table lives at the top of `specs.go`.
- **One pitch per node.** Chords are voiced as **arpeggios** within a row, or as **stacked rows** (e.g. two organ rows = two chord tones). Drum "pitch" shifts tone (e.g. conga slap = +9).
- **Per-row dynamics** via `Vol`; **note length** via `Dur` (in beats). Reverb send per instrument via `ReverbSend`.

**Implication for fidelity:** anything triplet-based or in an odd meter must be approximated or avoided. This drove two classical selections (see §4) away from triplet/odd-meter masterpieces toward 16th-grid-native ones.

## 3. Instrument palette & three new instruments

Existing palette (66 built-ins): full drum kit + percussion (kick/snare/hihat/tom/clap/cowbell/ride/crash/shaker/rimshot/sidestick/conga + variants), basses (synth-bass, sub-bass, bass-guitar, fm-bass), guitars (nylon/steel/electric ×2), strings (violin, violin-ensemble, cello ×2), piano-grand, piano-felt, **fm-epiano (Rhodes-like)**, winds (flute, oboe, trumpet, french-horn + variants), FM (bell/lead/pluck).

### New instruments (all reuse the existing modular synth — no C/DSP code)

The orchestral seeds (violin, trumpet, guitar, piano…) are the precedent: each is a `RecipeParams` seed + a `CVariantInstrument{Render: renderModular, Beats}` entry + a `synth_recipe.go` descriptor. The three new instruments follow that exact pattern. Registration touchpoints per instrument:

1. `internal/audio/instrument_seeds.go` — add `organSeed` / `clavinetSeed` / `saxSeed` (`RecipeParams`).
2. `internal/audio/engine_instruments.go` — add `CVariantInstrument` entry (`Render: renderModular`, `Beats` per family).
3. `internal/audio/synth_recipe.go` — add `{ID: "synth-modular-organ", Display: "Organ", Category: modularRecipeCategory, Seed: organSeed}` (+ clavinet, sax).
4. Regenerate `internal/audio/instrument_ids_gen.go` (generated from `engine_instruments.go`).
5. Verify the recipe-discipline guards (`synth_recipe_discipline_test.go`, `synth_recipe_invariants_test.go`, seed-distinctness) pass.

**Synth-design approach (starting seeds — will need collaborative ear-tuning, like the orchestral seeds):**

| New inst | Synthesis method (model on) | Character notes |
|---|---|---|
| **organ** | Additive sine drawbars (model on `pianoGrandSeed` additive gens) | Sum of sine partials at Hammond drawbar ratios (16′,8′,5⅓′,4′,2⅔′,2′,1⅗′,1⅓′,1′ → freq multiples 0.5,1,1.5,2,3,4,5,6,8). Sustained envelope (fast attack ~0.01s, sustain 1.0, short release ~0.08s, no decay). Leslie-style LFO→pitch+amp chorus (`lfo` shallow ~6Hz). Optional key-click noise tap on attack. Two voicings used: bright Hammond (house/reggae/techno/salsa) vs full pipe organ (Bach — more partials, slower attack, big reverb). |
| **clavinet** | Karplus-Strong pluck (model on `guitarElectricSeed`) | Bright, percussive, very short sustain, hard attack transient, narrow band-pass for the "funky wah" bite. The Superstition showcase. |
| **saxophone** | Subtractive reed: saw → band-pass formant → filter-envelope (model on `oboeSeed`/`trumpetSeed`) | Tenor-sax formants (breathier, lower BP center than oboe, slight growl in the noise tap), vibrato LFO on sustain, moderate attack. Used as the So What 2nd horn and the Girl from Ipanema melody (Stan Getz tenor). |

Where a new instrument needs a second timbre (e.g. pipe vs Hammond organ), we ship one seed and adjust per-template via `SynthParams` deltas on the `InstSpec` (the spec already supports `SynthParams map[string]float64`).

## 4. The 15-template slate

Recommendation per genre, with the masterpiece rationale and one strong alternative. "★" marks a new instrument.

| # | Genre | **Recommended** | Why this one (creative/considered) | Alternative |
|---|---|---|---|---|
| 1 | Classical · Baroque | **Bach — Toccata & Fugue in D minor, BWV 565** | The definitive dramatic organ masterpiece; showcases ★organ | — |
| 2 | Classical · Baroque | **Vivaldi — "Spring" (La Primavera), Mvt I** | Programmatic masterpiece: ritornello + birdsong episodes; string ensemble | Mozart — Eine kleine Nachtmusik, Mvt I |
| 3 | Classical · Classical era | **Mozart — Piano Sonata No.16 in C, K545, Mvt I** | Alberti-bass + melody — a considered masterpiece that is a *perfect* 16th-grid fit | Beethoven — Moonlight Sonata Mvt I (triplet feel approximated) |
| 4 | Rock | **Eagles — Hotel California** | Harmonic journey (vi–III–VII–IV…) + the iconic harmonized twin-lead outro | Led Zeppelin — Kashmir (rhythmic/polymeter masterpiece) |
| 5 | Pop | **Toto — Africa** | Layered, sophisticated, harmonically developed; iconic synth/marimba ostinato | Michael Jackson — Billie Jean; Beach Boys — God Only Knows |
| 6 | Jazz | **Miles Davis — So What** | Modal-jazz landmark (Kind of Blue): bass riff + the quartal "So What chord" + trumpet head (+★sax) | Coltrane — Giant Steps (max harmonic ambition) |
| 7 | Salsa | **Tito Puente — Ran Kan Kan** | The Mambo King's instrumental masterpiece: montuno + horn mambo + timbale/conga + bass tumbao | Rubén Blades & Willie Colón — Pedro Navaja |
| 8 | Funk | **Stevie Wonder — Superstition** | The definitive ★clavinet masterpiece; sophisticated, multi-tracked, horn-driven | Herbie Hancock — Chameleon |
| 9 | House | **Mr. Fingers — Can You Feel It** | Larry Heard's deep, soulful, *considered* house cornerstone; ★organ pads | Marshall Jefferson — Move Your Body |
| 10 | Hip-Hop | **Dr. Dre — Nuthin' but a 'G' Thang** | G-funk production masterpiece: melodic synth lead + bassline + laid-back groove (recreatable on instruments, not samples) | Wu-Tang — C.R.E.A.M. |
| 11 | Techno | **Derrick May — Strings of Life** | The most musical/considered Detroit techno: syncopated piano + string stabs | Jeff Mills — The Bells |
| 12 | Reggae | **Bob Marley — Exodus** | Iconic bassline + one-drop + horn stabs + ★organ bubble + offbeat skank | Stir It Up; Could You Be Loved |
| 13 | Disco | **Chic — Good Times** | The most influential bassline in modern music (Bernard Edwards) + Nile Rodgers chops + string line | Donna Summer — I Feel Love (electronic/sequencer masterpiece) |
| 14 | Bossa Nova | **Jobim — The Girl from Ipanema** | The bossa masterpiece; sophisticated chromatic bridge; nylon guitar + ★sax melody | Jobim — Wave / Desafinado |
| 15 | Blues | **B.B. King — The Thrill Is Gone** | Minor-blues masterpiece with string arrangement + the iconic Lucille lead (not a generic shuffle) | Muddy Waters — Hoochie Coochie Man |

**Proposed menu order** (Classical first, then by era/energy): 1 Bach · 2 Vivaldi · 3 Mozart · 14 Bossa · 6 Jazz · 15 Blues · 8 Funk · 13 Disco · 4 Rock · 12 Reggae · 5 Pop · 10 Hip-Hop · 9 House · 11 Techno · 7 Salsa. (Adjustable.)

## 5. Per-template arrangement blueprints

Each blueprint gives key (transposed), tempo, bars, the instruments and their roles, and the concrete musical material to author. Precise note transcription is finalized at implementation against public-domain tabs/sheets; contour and harmony below are the target.

### 1. Bach — Toccata & Fugue in D minor (★organ) · Dm · ~70 feel · 8 bars
- **Organ (manual, lead):** the opening flourish — mordent on A (A·G·A) then the descending gesture `A–G–F–E–D–C#–D`, repeated an octave lower; then the famous **D-diminished arpeggio stabs** (D–F–Ab–B) cascading down; resolve on D. Rolling toccata 16th figurations fill the middle bars.
- **Organ (pedal, bass):** sustained low **D pedal point** (D2) under the flourish; moves D→C#→D at the cadence.
- **Strings (violin-ensemble):** sustained Dm/dim swells doubling the big chords for weight.
- *Fidelity note:* free/rubato rhythm is metered to 16ths; we keep the melodic contour, the diminished harmony, and the pedal D. This is the ★organ showpiece (full pipe-organ voicing: many partials, slow attack, heavy reverb).

### 2. Vivaldi — "Spring" Mvt I · E (transposed) · ~110 · 8–12 bars
- **Violin-ensemble (ritornello):** the "Spring has arrived" theme — bright E-major arpeggio bounce: `E–E–E · (G#)–B–B–B · B–A#–B–C#–B · A#–B` answered piano/echo (soft repeat) then forte.
- **Solo violin (birdsong episode):** rapid trilled 16th alternations high (B–C#–B–C# … and E–F#–E–F#) over held strings — approximates the bird trills.
- **Cello + continuo (organ or piano-felt):** walking bass roots E–B–E + simple chordal continuo.
- *Fidelity note:* 32nd-note runs/trills approximated as fast 16ths.

### 3. Mozart — Sonata K545 Mvt I · C · ~120 · 8 bars
- **Piano-grand (RH melody):** the famous opening — `C5 · E5–G5 · (turn) F5–E5–D5–C5 …` the clear singing theme.
- **Piano-grand or piano-felt (LH Alberti bass):** continuous broken-chord 16ths `C–G–E–G` (C), `B–G–D–G` (G7), etc. — *this maps perfectly to the 16th grid* and is the reason for this pick.
- Optionally split RH/LH into two rows so the Alberti bass and melody each loop cleanly.
- *Fidelity note:* near note-accurate; the cleanest classical recreation in the set.

### 4. Eagles — Hotel California · Bm · ~75 · 8 bars
- **Guitar-nylon/steel (arpeggios):** 12-string-style arpeggiated chords through the progression **Bm – F#7 – A – E – G – D – Em – F#7** (one chord per bar), broken into 8th/16th arpeggios.
- **Bass:** roots of the progression, with the descending walk B–F#–A–E–G–D–E–F#.
- **Guitar-electric ×2 (twin-lead, outro):** the harmonized lead in 3rds over the last 2 bars (one row a 3rd above the other) — the signature.
- **Drums:** relaxed rock — kick 1 & 3, snare 2 & 4, 8th hats; light tom fill into the turnaround.

### 5. Toto — Africa · B → chorus · ~93 · 16 bars
- **fm-epiano / fm-bell (ostinato):** the iconic marimba/synth riff (repeated syncopated 16th pattern) — the hook.
- **Synth-bass:** the rooted bassline tracking the chord changes.
- **Piano-grand / strings (chords):** the verse/chorus pads (the "I bless the rains" chorus: B–F#–...).
- **Drums:** the famous half-time-feel groove — kick + the layered toms/percussion accents, shaker 16ths.

### 6. Miles Davis — So What · D dorian / Eb dorian · ~136 swing · 16 bars (AABA, 8 bars each section, looped as A·A·B·A condensed)
- **Synth-bass:** the "So What" bass riff (the 2-bar call) in D dorian, transposing up a semitone to Eb for the B section.
- **Piano-felt (the "So What chord"):** the two quartal voicings answering the bass (stacked-4ths: D–G–C–F–A, then a step lower) — voiced as a 2-row stab or fast arpeggio on the "and".
- **Trumpet (head):** the melody statement; **★sax** doubles/answers an octave or unison for the Davis–Coltrane–Adderley front line.
- **Ride + hihat:** spang-a-lang swing (ride on beats + swung "and" of 2 & 4 at step 6/14; hihat foot on 2 & 4).

### 7. Tito Puente — Ran Kan Kan · Cm-ish · ~180 (fast mambo) · 8 bars
- **Piano-grand (montuno):** the driving octave montuno (syncopated offbeat two-hand pattern over the vamp).
- **Trumpet (+ ★sax) (horn mambo):** the punchy mambo section lines — stacked stabs/answer phrases.
- **Bass:** anticipated tumbao (lands on the "and of 4" into the downbeat).
- **Percussion:** sidestick **clave** (2-3 son), **cowbell** cáscara pulse, **conga** open/slap (two rows: low open tone, high slap), timbale approximated via snare/rimshot accents + a cascara fill.

### 8. Stevie Wonder — Superstition (★clavinet) · Eb minor · ~100 · 8 bars
- **★Clavinet (lead riff):** the famous multi-tracked syncopated Eb-minor-pentatonic riff — the hook. Possibly two clav rows (the doubled-octave layering).
- **Bass:** the clav-doubled bassline (Eb root with the pentatonic motion).
- **Drums:** the iconic intro groove — hats then the backbeat (kick syncopation, snare 2 & 4, busy 16th hats).
- **Trumpet + ★sax (horn stabs):** the brass punches on the answer phrases.

### 9. Mr. Fingers — Can You Feel It (★organ) · deep · ~121 · 16 bars
- **Kick:** four-on-floor. **Clap:** 2 & 4. **Hihat:** offbeat 16ths building bar→bar.
- **Synth-bass:** the warm rolling bassline (rooted, legato).
- **★Organ (pads):** the deep sustained chord changes — the soulful Larry-Heard pad voicings (two-row chord stacks), the emotional core.

### 10. Dr. Dre — Nuthin' but a 'G' Thang · ~93 · 8 bars
- **fm-lead / modular (G-funk lead):** the high portamento "whistle" synth lead — the signature melodic hook.
- **Synth-bass:** the laid-back bassline (the Leon Haywood-style groove).
- **Piano-grand / fm-epiano:** the chord comp.
- **Drums:** the relaxed boom-bap-meets-G-funk groove (kick syncopation, crisp snare 2 & 4, swung hats).

### 11. Derrick May — Strings of Life · ~126 · 16 bars
- **Piano-grand (stab riff):** the syncopated chord-stab progression — the main hook (the "string" stabs are really piano).
- **Violin-ensemble (string stabs):** the orchestral hits doubling the piano on accents; **★organ** thickens the held chords.
- **Drums:** driving four-on-floor kick + clap/snare on 2 & 4 + open-hat offbeats; the build.
- **Bass:** the rolling sub line under the stabs.

### 12. Bob Marley — Exodus (★organ) · Am · ~98 · 8 bars
- **Bass:** the relentless iconic Am bassline riff — the engine of the track.
- **Guitar-electric (skank):** crisp offbeat chops on every "and" (steps 2,6,10,14).
- **★Organ (bubble):** the reggae organ "bubble" — staccato offbeat double-hits between the skanks (the classic shuffle bubble).
- **Drums:** one-drop feel (kick + snare together on beat 3), hi-hat 8ths/16ths.
- **Trumpet + ★sax (horn stabs):** the "Exodus!" horn punches on the turnaround.

### 13. Chic — Good Times · ~112 · 16 bars
- **Bass-guitar/synth-bass:** the **Bernard Edwards** bassline — the most-sampled bassline ever (the descending/walking signature). The centerpiece.
- **Guitar-electric (Nile Rodgers chucking):** tight muted 16th chord chucks (the "chucking" funk-disco rhythm).
- **Violin-ensemble (strings):** the soaring "Good Times" string line.
- **Drums:** four-on-floor kick, clap/snare 2 & 4, open-hat offbeats; **fm-epiano/clavinet** comp.

### 14. Jobim — The Girl from Ipanema (★sax) · F (bridge Gb) · ~130 (felt in 2) · 16 bars
- **Guitar-nylon (bossa comp):** the syncopated bossa chord pattern (the João Gilberto "violão" stroke) through Fmaj7–G7–Gm7–C7… and the chromatic bridge.
- **★Sax (melody):** the Stan Getz tenor melody — "tall and tan and young and lovely…" contour. **Flute** as the alternate/answer voice.
- **Bass:** soft rooted bossa bass (root–fifth on 1 & 3).
- **Percussion:** shaker 16ths + rimshot/sidestick soft bossa clave; **piano-felt** light comp.

### 15. B.B. King — The Thrill Is Gone · Bm · ~98 · 12 bars
- **Guitar-electric (lead, "Lucille"):** the iconic minor-blues lead licks (B blues scale bends approximated as fast grace-note steps) over the 12-bar minor form.
- **Bass:** the smooth minor-blues walking/rooted line through **Bm–Bm–Bm–Bm · Em–Em–Bm–Bm · Gmaj7–F#7–Bm–F#7**.
- **Violin-ensemble (strings):** the lush sustained string arrangement (the studio signature).
- **Piano-grand / ★organ:** comping chords.
- **Drums:** slow 12/8-feel blues groove approximated on the 16th grid (kick 1 & 3, snare 2 & 4, swung hats); **12-bar = long loop**.

## 6. Implementation mechanics

1. **New instruments first** (so circuits can reference them): add `organSeed`, `clavinetSeed`, `saxSeed` + engine + recipe-descriptor entries; regenerate `instrument_ids_gen.go`; pass recipe-discipline guards. Ear-tune to taste (collaborative pass, like the orchestral seeds).
2. **Rewrite `showcases()`** in `cmd/gen-showcase/specs.go` with the 15 blueprints above (replacing the current 9 specs). Keep the pitch-reference header; extend it for any new low/high notes.
3. **Regenerate JSON:** `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-showcase`.
4. **Delete the old hand-authored templates** (`rock/hip-hop/pop/funk/salsa/house/techno.json`) and the superseded generated ones; the directory ends with exactly the 15 new `<stem>.json` files.
5. **Update `templateOrder`** in `internal/assets/templates.go` to the 15 new stems in menu order. Update the doc comment (no more "original 7 hand-authored" split — all 15 are generated).
6. **`displayName`** title-cases stems; pick stems that render well (e.g. `bach-toccata`, `vivaldi-spring`, `mozart-k545`, `eagles-hotel-california`, `toto-africa`, `miles-so-what`, `puente-ran-kan-kan`, `wonder-superstition`, `fingers-can-you-feel-it`, `dre-g-thang`, `may-strings-of-life`, `marley-exodus`, `chic-good-times`, `jobim-ipanema`, `bbking-thrill-is-gone`). Display names may instead come from a small stem→title map if title-casing is insufficient (decision at implementation).

## 7. Testing & verification

- **Drift test** (`cmd/gen-showcase/drift_test.go`) — committed JSON must equal `build()` output. Run after every spec change.
- **Template guards** (`internal/assets/templates_test.go`) — every embedded template imports cleanly, colors derive from `InstrumentSequence`, BPM parses. Update any count assertions (16→15) and the order list.
- **Fast Go suite:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/... ./internal/audio/...`.
- **Instrument seeds:** `instrument_seeds_test.go` (seed-distinctness), `synth_recipe_discipline_test.go`, `synth_recipe_invariants_test.go`, `all_instruments_pitch_table_test.go`, `melodic_pitch_range_test.go`.
- **Cross-platform parity** (predictor/audio identical WASM vs native): `make test-parity`, `xplat_audio_compare.browser.test.js`.
- **Ear test (the real acceptance gate):** load each template, play, A/B against the original recording; iterate on notes/velocities/instrument seeds collaboratively. This is where "as close as possible" is actually judged.

## 8. Risks & open questions

- **Triplet/odd-meter loss.** Moonlight Sonata, Take Five, Gymnopédie, Money (7/4) cannot be rendered faithfully on the 16th-grid. Mitigated by selection (chose 16th-native masterpieces); Moonlight offered only as an alt with the caveat.
- **New-instrument fidelity.** Organ/clav/sax seeds are starting points needing ear-tuning; first render will be approximate. Same workflow as the orchestral seeds.
- **Hip-hop/disco are sample-based originals.** We recreate the *played* loop (bass/keys/lead/drums), not a chopped sample — close in spirit, not a 1:1 sample match.
- **Grid size for long loops.** 16-bar templates create large node grids (perimeter ≈ 256 units/row). Functionally fine (import handles arbitrary i,j); visually large. Verify import + predictor horizon are happy with the biggest (16-bar, 7-row) templates.
- **Drum "pitch" semantics** vary by instrument; conga/tom tone-shift via pitch is established, but verify each percussion row's pitch offsets render as intended.
- **Selection still open for review** — every genre has a documented alternative; swaps are cheap (spec-level).

## 9. Decision log

- Replace all 16 existing templates with 15 song templates (user: "Replace all with the song set").
- Add Reggae, Disco, Bossa Nova, Blues beyond the original 9 genres (user: all four).
- Extended multi-section arrangements, not tight hooks (user).
- Create Organ + Clavinet + Sax (user).
- Apply "creative/considered masterpiece" lens, excluding simple riff-songs (user) → upgraded Rock (Hotel California), Pop (Africa), Salsa (Ran Kan Kan), Funk (Superstition), Hip-Hop (G Thang), Reggae (Exodus), Blues (Thrill Is Gone), and the classical trio (Bach Toccata / Vivaldi Spring / Mozart K545) over the earlier Canon/Ode/Smoke-on-the-Water/Billie-Jean choices.

## 10. As-built notes (post-implementation)

Three findings emerged during implementation that the spec didn't anticipate:

1. **One node per step per row.** The generator maps hits by step, so two hits on the same step in a row silently collapse. Chords are now voiced via a `chordAt` helper (consecutive-step spread, long Dur → rings as a chord; tight strum for stabs), and `build()` panics on a duplicate step so this can never regress. Close syncopated stabs (Strings of Life) became single-note lines to preserve rhythm.

2. **Seeded modular instruments need a `synth_params` trigger.** `renderModular` renders the generic identity tone; the seed only applies via the recipe path, which requires non-empty `synth_params`. The old templates lacked this, so their orchestral instruments sounded generic. The generator now emits `synth_params:{"pitch":0}` for every seeded-modular instrument (drums/FM excluded), which engages the seed (`MergeRecipeDefaults` supplies it) while the per-node pitch overrides the 0 at play time. Each new instrument also needs a `config.go` `InstrumentConfigs` entry (the 6th touchpoint).

3. **Global send-reverb has no reset.** The process-global `sendFX` reverb/delay tails survive `ResetInstruments`, so the full-suite audio tests polluted a reverb-heavy sustained organ (false clips in the full run, clean in isolation). Added `resetSendEffectsState()` and call it in the template render helpers.

Verified: full real-build `./internal/audio`, fast-path assets+audio, gen-showcase drift, UI template menu + import round-trip, and full native `go build ./...` all green. Remaining: WASM/browser cross-platform parity and the collaborative ear-test (the real acceptance gate).
