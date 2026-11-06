# Public-Domain Template Curation — Design (as built)

**Date:** 2026-09-02
**Status:** IMPLEMENTED
**Supersedes:** `2026-08-22-public-domain-template-curation-design.md`

## Goal

Three things, in one pass:

1. Remove every template circuit not derived from a public-domain musical
   work backed by a real, citable notated score.
2. Make "accurate to the sheet music" a property tests can check, rather
   than a claim in a comment.
3. Carry the score's *expression* — dynamics and agogic drift — into the
   circuit, not just its pitches and onsets.

## Outcome

32 templates → **24**, none of them shared with the old set.

Every one is generated from a committed, CC0-licensed reference score.
There is no hand-authored musical content anywhere in the pipeline.

| Group | Templates |
|---|---|
| Baroque | Brandenburg 3, Brandenburg 4, B-minor Mass Kyrie |
| Classical | Beethoven 5, Ode to Joy, Haydn 104, Schubert 5, Haydn Op. 76, Mozart K. 387, Beethoven Op. 18 |
| Romantic | Brahms 1 finale, Dvořák *American*, Borodin 2, Beach *Gaelic*, Boulanger *D'un matin de printemps* |
| Lied · Song | Schubert *Erlkönig* / *Ave Maria* / *Ständchen*, Clara Schumann Op. 13, Fanny Hensel *Italien* |
| French · Americas | Fauré *Après un rêve*, Satie *Air du rat*, Liliʻuokalani *Aloha ʻOe*, Chiquinha Gonzaga *Espanha e Brasil* |

## The admission rule

Three gates, each a catalogue field and each enforced by a test in
`internal/scoreref`. `docs/score-sources.md` holds the full source survey.

1. **The work is public domain** — `first_publication_year ≤ 1929` and
   `composer_death_year ≤ 1955`. Conservative on purpose: it clears US
   publication-based PD and EU life+70 at once. This gate is not
   redundant with gate 2 — the CC0 corpora contain Bartók, Berg, Bridge
   and Webern, whose *encodings* are free while the *works* are not.
2. **The encoding is permissively licensed** —
   `CC0-1.0 | PD | unrestricted | CC-BY-4.0`. NonCommercial and
   ShareAlike are excluded and must not be added: NC forbids the use for
   a product that ships, SA would push its terms onto the generated
   templates. CC-BY entries require a non-empty `attribution`, rendered
   into a committed `NOTICE-scores.md`.
3. **The provenance is verifiable** — a named corpus, an exact upstream
   URL, and a SHA256 over the committed bytes.

Gate 2 is *corroborated, not asserted*: `TestGate2_EmbeddedRightsCorroborateLicense`
parses each score's own `<rights>` element and fails if it contradicts the
catalogue. The catalogue is hand-written; the score file is not.

### Why the scores are committed

The superseded draft made them fetch-only, mirroring `internal/wavref`.
That inheritance was wrong. `wavref` cannot commit its files because real
instrument recordings carry restrictive licences; every score admitted
here is CC0, which permits redistribution outright. Committing them keeps
`gen-showcase` and its drift test fully offline and makes the templates
reproducible from a clean checkout. `scripts/verify_scores.sh` re-fetches
upstream and compares hashes when a network check is wanted.

Files are committed **verbatim and untrimmed** — the previous tree shipped
hand-trimmed excerpts, whose hashes were therefore comparable to nothing.
Trimming moved into code, which is what made measure numbers necessary.

`internal/scoreref` embeds ~5 MB, so `TestNoProductionImports` forbids any
package outside `cmd/gen-showcase`, `cmd/refs` and tests from importing it.
The shipped binaries embed only the generated template JSON.

## Parser work (`internal/musicxml`)

Five gaps, each confirmed against real corpus files before being fixed:

| Gap | Consequence | Fix |
|---|---|---|
| No `.mxl` (ZIP) support | none of the four corpora readable | unwrap via `META-INF/container.xml` |
| No `<measure number>` | trimming impossible in code | `Note.Measure` + measure-range specs |
| No `<dynamics>` | every template dynamically flat | → `Note.Velocity` → `Hit.Vol` |
| No `<tie>` | a held note re-attacked — an event the score does not contain | tied groups merge into one note |
| No `<transpose>` | Bb clarinet parts a tone wrong | sounding pitch; deletes hand-coded corrections |

Also added: `<part-name>`, `<staff>`, `<grace>`, articulations, `<rights>`,
and the agogic `<words>` (rit./accel./a tempo) that drive groove. Repeats
are deliberately **not** expanded; measure ranges are pre-repeat numbers.

## Expression → circuit

**Dynamics → volume.** `<dynamics>` and `<sound dynamics>` set a running
velocity; `<accent>` and `<strong-accent>` scale it (×1.15 / ×1.25,
clamped to 1). A part with no marking anywhere keeps velocity 0, which the
importer passes through as "use the instrument default" — Bach marks no
dynamics in any of the openings used here, and inventing some would be
exactly the unsourced embellishment this change removes.
`TestDynamicsReachVolume` asserts **both** directions.

**Notation → groove drift.** `Hit.Groove`/`GroovePct` are derived, never
authored, in order of specificity:

| Notation | Groove |
|---|---|
| grace note | `rush` 0.55 — played before its principal note |
| fermata | `delay` 0.60 |
| inside a `rit.` span | `delay`, ramped 0.15 → 0.50 across the span |
| inside an `accel.` span | `rush`, same ramp |
| tenuto | `delay` 0.12 |

A span runs from its marking to the next *a tempo*. An unclosed marking
expires after two measures — left unbounded it grooved every remaining
note in the range, which is both wrong and unlistenable. The ramp matters
too: a ritardando arriving at full strength on its first note is a tempo
change, not a ritardando.

## Chords

Beatmo's model is one node per step per row, so a chord cannot live in one
row. Notes are grouped **by onset across voices** and ranked top-down by
pitch: lane 0 takes the highest sounding note at every onset, lane 1 the
next, capped at the part's `MaxVoices`.

Grouping across voices rather than within each one is what keeps a
keyboard part to `MaxVoices` rows — a piano staff routinely carries four
or more MusicXML voices, and laning each separately produced *thirteen*
rows for a single Lied.

This replaces the previous behaviour, which pushed a colliding chord tone
to the next free **step** — silently turning every chord in the corpus
into an arpeggio the score does not contain.

Splitting a keyboard part into three lanes produces a third row on one
instrument (`piano-grand-3`). `audio.instanceBaseID` only stripped
`-1`/`-2`, so such a row would have been unavailable and **silent in the
app**; it now strips `-1`..`-9`, while still leaving a real id like
`bass-808` alone.

## Testing — the score as oracle

The old guard was `musicTruths`: a hand-written table of each piece's
researched key and tempo. It could only restate what someone believed
about the music, it could not catch a wrong note, and it drifted from the
score it described. It is deleted.

Because every template is now a pure function of a committed score, the
tests re-read that score independently and check conservation:

- `TestTemplateIsDerivedFromScore` — every pitch a template plays must
  occur in its score's trimmed range. A hand-edited note, a wrong octave
  policy or a transposition slip all fail here.
- `TestTemplateDropBudgets` — the conservation identity
  `emitted + chord-tones-dropped + step-collisions == score notes in
  range`. Nothing may go missing uncounted.
- `TestPartMapNamesMatchScore` — part selection is by index (several
  scores carry duplicate part names) with the name as a checked
  assertion, so an upstream re-ordering cannot silently repoint a row.
- `TestTemplateTempoComesFromScore` — BPM is the score's metronome mark.
- `TestDynamicsReachVolume`, `TestGrooveIsDerivedFromNotation` — the
  expression layer, in both directions.
- `TestTemplateOrderMatchesAssets` — the spec table and the menu agree.
- `internal/musicxml/expression_test.go` — a unit test per parser gap.
- `internal/scoreref` — the three gates, hashes, orphans, import
  discipline, and that every committed score parses.

`internal/assets`'s per-song BPM band table is likewise gone: tempo is
derived and checked against the score, so all that remains there is a
plausibility range.

## Known gap

All seven song-tier WAV references were keyed to removed templates and
were pruned rather than left dangling; the catalogue now has none.
Re-seeding them needs a CC0/PD-Mark recording of each exact movement,
downloaded and hashed — real work, deliberately not bundled here. The
Hauptstimme corpus ships `_alignment.csv` files referencing public-domain
IMSLP recordings of the same movements, which is the obvious start.
`cmd/refs/song_test.go` now asserts integrity (a reference must name a
real template) instead of coverage, and `refs doctor` reports the gap.
