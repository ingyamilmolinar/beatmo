# Groove templates: genre beats under public-domain melodies

**Date:** 2026-10-02 · **Status:** implemented (uncommitted)

## Goal

Every built-in template today is strings, voice or piano — a drum machine that
ships no beats. This adds a second family of templates: a **public-domain
melody** (ragtime, marches, fiddle tunes, folk song) laid over a **genre groove**
(boom-bap, house, reggaeton, merengue, cumbia, vallenato, salsa, techno, trap,
disco, rock, jazz), with drums as real rows. It also closes every gap the
2026-10-02 review found in the score → template pipeline.

Copyrighted songs stay out (Gate 1 of `internal/scoreref`, unchanged in
spirit). Grooves are traditional rhythmic idioms, encoded by Beatmo as CC0
percussion scores.

## Decisions

### 1. Groove window = one step (layout change)

The engine's groove window is ONE grid unit (`nodeNudgeSecondsAt`:
`pct * secPerBeat/MaxDiv`). Templates placed 4 units per step, so 100% delay
was a quarter of a 16th — swing (an offbeat 8th sits 2/3 of a step late at
2:1) was inexpressible. Templates now place **one unit per step** and export
`subdiv = steps per quarter` (4 for 16 steps/bar, 8 for 32). Node spacing on
screen is unchanged (a unit is `Step/MaxDiv`); the groove slider now means a
fraction of a 16th, which is what a swing control means everywhere else.

### 2. Groove belongs to its own node

`nodeNudgeSecondsAt` applied the most recent grooved node's offset to every
following ungrooved node for two beats, so one swung offbeat dragged the next
downbeat with it. A node's groove now applies to that node only (the model's
own doc says "per-node"). The legacy back-search is kept only for beats that
carry no node id.

### 3. All notated timing becomes delay/rush on the WRITTEN step

The generator computes each note's performed time in steps and keeps the node
on its written (rounded) step; the remainder becomes `delay`/`rush` (clamped to
one step, clamps counted and budgeted). Sources, combined additively:

| Source | Effect |
|---|---|
| tuplets / off-grid onsets | quantisation remainder (a triplet's 2nd/3rd notes) |
| swing (`<sound><swing>`, "Swing"/"Shuffle"/"Straight" words) | piecewise-linear warp of each swing pair |
| grace note | rushed ahead of its principal |
| fermata, tenuto | lean behind |
| rit. / accel. spans | ramped drag / push |
| `<arpeggiate>` | rolled chord: each higher lane later (or earlier for `down`) |
| `<breath-mark>`, `<caesura>` | the NEXT onset in the part is delayed |

Offsets are defined in BEATS (musical time), so the existing constants keep the
same audible size they had at 16 units/beat.

### 4. Percussion is parsed, not collapsed

`internal/musicxml` reads `<unpitched>`, `<instrument id>`, and the part-list's
`<score-instrument>`/`<midi-unpitched>` (1-based, = GM key + 1). A drum part is
split into one row per Beatmo instrument through a GM drum map (overridable per
PartMap); kit pieces mapping to one instrument share a row (pitch per node, e.g.
tom heights), spilling to a second row of the same instrument only on a
same-step collision. Pitched percussion (timpani) is a pitched part.

Every existing template whose excerpt contains a sounding percussion part now
maps it (`TestEveryPercussionPartInRangeIsMapped`).

### 5. Measures, meters and swing are timelines

`Score.Measures` records each measure's movement, number, onset and time
signature; `measureDiv` and the swing grid read it, so a meter change or pickup
no longer mis-sizes agogic spans or swing pairs.

### 6. MIDI scores (`internal/smf`)

Mutopia ships Public-Domain LilyPond editions with LilyPond-generated MIDI.
`internal/smf` parses SMF 0/1 into the same `musicxml.Score` model: tracks →
parts (channel 10 → unpitched, GM key), tempo + time-signature timelines →
measures, ties implicit. LilyPond MIDI unfolds repeats, so measure numbers are
PERFORMED measures. Constant velocities mean "no dynamics" (velocity 0).

### 7. Admission gates

- Gate 1 cut-offs derive from the current year (`now-96` publication, `now-71`
  death) instead of hardcoding 1929/1955, and a work with a lyricist must also
  clear the lyricist's death year.
- `Traditional` works (folk tunes, rhythmic idioms) pass Gate 1 with a cited
  reference describing the work.
- New providers: `mutopia` (licence corroborated from the committed `.ly`
  header) and `beatmo` (CC0 idiom scores, `<rights>Beatmo (CC0)</rights>`).
- Groove idioms carry a fingerprint test: the defining instrument's onsets must
  equal the textbook pattern (son clave 2-3, tresillo, cáscara…), a double
  entry that catches a typo in the authored score.

### 8. Template = melody layer + groove layer

`TemplateSpec.Groove` names a groove score that is LOOPED to the melody's
length (its length must divide it); groove swing applies to the whole template.
`BPM` overrides the score tempo (the genre sets the tempo). `PartMap.Bass`
takes the lowest note of each onset instead of the highest.

### 9. songrender

The offline template renderer took JSON `subdiv` as steps per BAR; it is units
per BEAT, so templates rendered at quarter speed. It now uses units per beat
and applies each node's groove offset, matching the engine.

## Non-goals

ABC/LilyPond parsing (no admissible source needs it yet), tabs, copyrighted
songs.

## Revision (same day): famous breaks + original tunes

The first melody set (Joplin rags, Sousa marches, *Soldier's Joy*) was not
well known enough, and the best-known public-domain tunes were too old and too
stereotypical for what the templates are meant to show. The target is
70s-and-later music, whose melodies are copyrighted, so a template is now:

- a **famous drum break** from a 70s+ record — Beatmo's CC0 encoding of a
  published step transcription (drum-patterns.com), catalogued as a `rhythm`
  and fingerprinted against that transcription; and
- an **original tune** composed for Beatmo (`original`, CC0) in the genre.

The one famous melody is **Korobeiniki** (the Tetris theme), a public-domain
folk song transcribed from a public-domain Commons sheet. The Mutopia editions
were removed; `internal/smf` and the companion corroboration stay as
infrastructure.
