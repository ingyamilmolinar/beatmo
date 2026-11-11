# MusicXML → Circuit Translator + Albéniz "Asturias" Template

**Date:** 2026-06-24
**Status:** Approved (design); implementation authorized
**Scope:** `src/go/internal/musicxml` (new), `src/go/cmd/gen-showcase` (score import + Asturias spec), `src/go/internal/assets/templates` (swap irish-air → asturias)

## Goal
Build reusable, offline (build-time) infrastructure that translates digital sheet
music (**MusicXML**) into Beatmo circuit templates, and use it to replace the
`irish-air-guitar` template with the public-domain Albéniz **Asturias (Leyenda)**
opening — faithful in notes, rhythm, octave, key, and mode.

## Why MusicXML
MusicXML is the open standard for digital sheet music and natively encodes
notes/rhythm/octave/**key**/**mode** (MIDI does not). It's plain XML → parse with
Go's `encoding/xml`, no new dependencies. OMR tools (Audiveris/MuseScore) and
notation editors all export MusicXML, so this parser is the reusable entry point
for "translate a sheet" without us writing an image recognizer.

## Components (each small, single-purpose)
1. **`internal/musicxml`** (new, importable, pure `encoding/xml`): parses a
   MusicXML subset → a neutral `Score`:
   - `Score{ Divisions int; KeyFifths int; Mode string; TempoBPM float64; TimeBeats, TimeBeatType int; Parts []Part }`
   - `Part{ ID string; Notes []Note }`
   - `Note{ Semitone int /* from A3=0 */; OnsetDiv int; DurationDiv int; Voice int; Rest bool; Chord bool }`
   - Handles: `<attributes><divisions>`, `<key><fifths>/<mode>`, `<time>`, tempo
     (`<sound tempo>` or `<metronome>`), `<note>` with `<pitch>(step,octave,alter)`,
     `<rest>`, `<chord/>`, `<duration>`, ties (extend duration). Pitch→semitone:
     `midi = (octave+1)*12 + {C0,D2,E4,F5,G7,A9,B11}[step] + alter`; `Semitone = midi - 57` (A3=57).
2. **`cmd/gen-showcase/score_import.go`** (package `main`): `scoreToShowcase(Score, opts) Showcase`.
   - Quantize onsets to grid steps: `step = round(onsetDiv / divisions * (subdiv/4))` (subdiv=16 → 4 steps/beat).
   - Map parts/voices → rows (one `RowSpec` per voice). `Note.Semitone → Hit.Pitch`,
     `DurationDiv → Hit.Dur` (in beats). Derive `BPM` from tempo, `Bars` from total length.
   - One regular node per step per row: if two notes collide on a step in one voice,
     spread via `chordAt` (adjacent steps) or drop to the next free step (logged).
   - Produces the existing `Showcase` struct → existing `build()` emits JSON (reuses
     perimeter layout, edge wiring, colors, drift test). No change to `build()`.
3. **`cmd/gen-showcase/scores/asturias-opening.musicxml`** (committed): hand-transcribed
   public-domain Asturias opening (~8–16 bars). **Two voices** → two rows:
   - Voice 1: the repeated upper pedal (the rapid `i`-finger note).
   - Voice 2: the bass melodic line (thumb), carrying the Phrygian descent.
   Both rows use `guitar-nylon` (the tuned instrument). E-minor/Phrygian tonality.
   License-clean: a PD composition's pitches/rhythms are not copyrightable; the file
   is our own encoding (not an imported third-party arrangement).
4. **Template swap**: remove the `irish-air-guitar` spec entry + `irish-air-guitar.json`;
   add the `asturias` spec (loads+converts the MusicXML at gen time) + generated `asturias.json`.

## Data flow
`asturias-opening.musicxml` → `musicxml.Parse` → `Score` → `scoreToShowcase` →
`build()` → `asturias.json` → `internal/assets/templates/`.

## Fidelity validation (the explicit requirement)
- **Notes + octave**: round-trip test — converted `Hit.Pitch` values equal the
  MusicXML notes' semitone-from-A3 (octave preserved by construction).
- **Rhythm**: a test asserts the ostinato onsets land on the expected sub-beat steps
  and durations match the score.
- **Key + mode**: the MusicXML declares `fifths`+`mode`; a diatonic test (mirroring
  `musical_fidelity_test.go`'s Ipanema test) asserts every Asturias note is in
  E-minor/Phrygian pitch-classes. Optionally: render `asturias.json` and confirm
  `fingerprint`/song key-detect reports E.
- **Regeneration**: the existing `drift_test.go` asserts `asturias.json` matches `build()`.

## Error handling / scope boundary
- Quantization rounding: warn (log) if a note is >½ step off grid.
- Polyphony: >1 note/step/voice → `chordAt` spread or next free step (logged); cross-voice
  overlap is fine (separate rows).
- Unsupported MusicXML (ornaments, tremolo expansion, repeats/voltas, multiple
  `<part>` beyond what we map) → ignored with a logged warning, never a panic.
- **Out of scope:** image OMR, Guitar-Pro/ASCII tabs (MusicXML is the interchange
  format those produce upstream); full-piece transcription (iconic opening only).

## Testing
- `internal/musicxml`: parser unit tests (small inline MusicXML → expected `Score`:
  pitch math, divisions, key/mode, chord, rest, tie).
- `score_import`: converter tests (notes → expected `Hit`s/steps; collision handling).
- Fidelity tests (notes/rhythm/key-mode) + drift regeneration.
- `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/musicxml/ ./cmd/gen-showcase/`.

## Notes
- The exact Asturias opening pitch sequence will be verified against a public-domain
  reference during implementation (encoding the PD composition's notes, not importing
  a third-party file), and confirmed via the diatonic + key-detect checks. The user
  is in an audition loop and can correct specific notes.
- `gen-showcase` Showcase/Hit/RowSpec types are `package main` (not importable); the
  reusable `Score` lives in `internal/musicxml`, the `Score→Showcase` glue stays in
  `package main`.
