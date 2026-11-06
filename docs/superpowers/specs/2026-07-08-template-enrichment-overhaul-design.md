# Template Enrichment Overhaul — Design

**Date:** 2026-07-08
**Status:** Approved by user (conversation, 2026-07-08)
**Predecessors:** `2026-06-20-genre-masterpiece-templates-design.md`, `2026-06-24-musicxml-to-circuit-asturias-design.md`, the 2026-06-27/28 note-accuracy passes.

## Goal

All 33 built-in templates get significantly overhauled so they sound **delightful, produced, and recognizable** — not "crappy and basic" (user's words). Same songs/pieces, same instrument *registry* (75 built-ins, no new instruments), but:

- **Full sections, more bars** — a recognizable section arc (intro → hook feel), typically 8–16 bars; the ~7 tiny 2-bar etudes become 8-bar mini-arrangements.
- **More layers** — rows added from the existing registry to match the real song's instrumentation (percussion, harmony, pads-as-rolls, counter-lines). Human voice is substituted by the most fitting lead instrument per song.
- **Dynamics & groove** — per-hit volumes (accents + ghost notes via low `Vol`), and per-node `delay`/`rush` groove (swing hats, laid-back backbeats, pushed montunos). Groove is a NEW generator capability: `Hit.Groove{Kind,Pct}` → node JSON `groove_kind`/`groove_pct` (model + export already support it).
- **Mix polish** — per-row volume/pan/reverb-send + insert low/high-pass filters tuned per song so the master sounds produced.

## Non-goals

- No new instruments (registry is frozen for this project).
- No engine/model changes beyond emitting the already-supported groove fields from the generator.
- No changes to the template menu/import UI.

## Source-of-truth policy (user mandate from prior passes)

Notes come from **real online sources, never from memory**:
- Pop/funk/etc.: BitMidi JSON API (`/api/midi/search` → `downloadUrl`), raw ABC notation, tab transcriptions readable via WebFetch.
- Classical: **real PD scores** — music21 corpus `.mxl` / Mutopia MusicXML, parsed via the existing `internal/musicxml` + `cmd/gen-showcase/score_import.go` (extended for multi-voice→multi-row where needed). The classical set (Bach ×4, Mozart K545, Pachelbel, Marcello, Handel, Vivaldi, Asturias) is re-derived from actual scores.
- Arrangement dossiers (instrumentation, groove feel, production character, section structure) researched per song from prose sources.
- All agent-returned note lists are re-verified against the raw source; pitches re-derived with the A3=0 convention (pitch = MIDI − 57).

## Architecture (unchanged, extended)

`cmd/gen-showcase/specs.go` stays the single authoring surface; `make`-time generation into `internal/assets/templates/*.json` (embedded, drift-guarded). Extensions:
1. `Hit.Groove` field + generator emission of groove node fields.
2. `score_import.go` gains multi-part/multi-voice mapping to multiple rows + per-part instrument assignment (classical batch).
3. Per-song `musicTruths` (key/BPM) and `musical_fidelity_test.go` checks extended/updated for every changed template.

## Hard constraints (from prior gotchas — must hold)

- One node per step per row; chords = consecutive-16th rolls (`chordAt` style).
- Naive resampler: melodic percussion pitches ≤ 0 or shortened decay; overlapping bass lines use consonant intervals or sparse spacing.
- Every instrument id ∈ `audio.BuiltinInstrumentIDs`; seeded modular instruments carry `synth_params:{"pitch":0}` minimum.
- `synth_params` ranges: brightness/drive/body 0..1, tone −1..1, decay ~0.1..4; open-hat lane uses distinct id from closed hat.
- Noise percussion (shaker/open-hat/clap/crash) gets ~9–12 kHz lowpass inserts.
- Audio-quality guards: master no-clip, crest > 2.5, per-solo flatness < 0.50, jumpRate < 1%, pitched rows harmonic-fraction > 50%, all-hits-downbeat peak < 0.9.

## Batches & order

1. Funk/Disco/Pop/Rock/Hip-Hop/House (11): wonder-superstition, chic-good-times, gaynor-survive, eagles-hotel-california, toto-africa, marley-exodus, dre-g-thang, blackbox-ride-on-time, bbking-thrill-is-gone, miles-so-what, jobim-ipanema.
2. Latin (4): puente-oye-como-va, salsa-vivir, bachata-obsesion, cielito-trumpet.
3. Classical from real scores (9): bach-toccata, bach-prelude-c, bach-flute-allemande, bach-cello-prelude, vivaldi-spring, mozart-k545, pachelbel-violin, marcello-oboe, handel-water-horn (+ asturias verify/enrich).
4. Etudes → mini-arrangements (7): felt-prelude, sax-blues, steel-folk, electric-riff, clav-funk, bass-groove, conga-tumbao.

## Verification

Per batch: regenerate; drift + source-of-truth + fidelity + `TestTemplateAudioQuality` + `TestTemplateMasterMix` + assets/UI template tests green.
Final: `make song-render` for every template → `tmp/template_audition/<stem>/` (master + stems) for the user's single listening pass. Reminder surfaced to user: `make wasm` needed to hear new templates in-browser.

## Working-tree policy

No staging, no commits (standing user rule). All changes left in the working tree on branch `add-core-node-types`.
