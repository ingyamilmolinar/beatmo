# Template Enrichment Overhaul Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Overhaul all 33 built-in templates into rich, produced-sounding, note-accurate arrangements (spec: `docs/superpowers/specs/2026-07-08-template-enrichment-overhaul-design.md`).

**Architecture:** `cmd/gen-showcase/specs.go` stays the single authoring surface → `go run ./cmd/gen-showcase` regenerates `internal/assets/templates/*.json` (drift-guarded). One infra extension (groove emission), one importer-side extension (multi-part score import for classical), then four content batches, each: online research → encode → per-song truth/fidelity tests → regen → audio-quality gates. Final: WAV audition renders.

**Tech Stack:** Go (bundled `.tools/go/bin/go`), `cmd/gen-showcase`, `internal/musicxml`, `cmd/song-match render`, WebSearch/WebFetch + BitMidi API for sources.

## Global Constraints

- **NEVER `git add` / `git commit` / `git revert` / `rm`** — standing user rule. All work stays uncommitted in the working tree.
- Go binary: `/home/ymolinar/Repos/beatmo/.tools/go/bin/go` (call it `$GO` below; run from `src/go/`).
- Pitch convention: **pitch 0 = A3 (220 Hz)**; template `Hit.Pitch` = semitones from A3 = MIDI − 57.
- One node per step per row (duplicate step panics in `build()`); chords = consecutive-16th rolls.
- Resampler: melodic percussion pitches ≤ 0 or shortened `decay`; dense bass lines consonant intervals or sparse spacing; downward pitch LENGTHENS notes (overlap mud), upward pitch aliases.
- Instrument ids must ∈ `audio.BuiltinInstrumentIDs`; seeded modular instruments need `synth_params` non-empty (`{"pitch":0}` min — generator does this via `seededModularInstruments`; new ids added to a template must be checked against that map).
- `synth_params` ranges: brightness/drive/body 0..1 (negatives clamp to 0), tone −1..1, decay ~0.1..4. Open-hat lane uses a distinct id (`hihat-1`) from closed `hihat`.
- Noise percussion (shaker/open-hat/clap/crash/cymbal) gets insert lowpass ~9 kHz (12 kHz crash) via `Effects: []EffectSpec{{Mode:0, Cutoff:9000, Q:0.707}}`.
- Groove JSON contract (importer `internal/ui/import.go:322-336`): node fields `groove_kind` ∈ {"delay","rush"}, `groove_pct` clamped 0..1.
- Never source notes from model memory — only from fetched MIDI/ABC/MusicXML/tabs; re-derive every pitch yourself.
- Quality gates (all must stay green per batch): `cmd/gen-showcase` drift + source-of-truth + fidelity tests; `internal/assets` template tests; `internal/ui` template import round-trip; `internal/audio` `TestTemplateAudioQuality` + `TestTemplateMasterMix` (real build, no `-tags test`, needs cgo).

---

### Task 1: Groove emission from gen-showcase

**Files:**
- Modify: `src/go/cmd/gen-showcase/main.go` (Hit struct, node struct, build())
- Test: `src/go/cmd/gen-showcase/groove_emit_test.go` (create)

**Interfaces:**
- Produces: `Hit{Step int, Pitch, Vol, Dur float64, Groove string, GroovePct float64}` — `Groove` ∈ {"", "delay", "rush"}; emitted as node JSON `groove_kind`/`groove_pct` only when non-empty. All later content tasks set these fields.

- [ ] **Step 1: Write the failing test** (`groove_emit_test.go`, package main): build a minimal Showcase with one row, hits at steps 0 (no groove) and 4 (`Groove:"delay", GroovePct:0.25`); `build()` it; unmarshal JSON; assert the step-4 regular node has `groove_kind=="delay"` && `groove_pct==0.25` and the step-0 node omits both keys (unmarshal into `map[string]any` and check key absence).
- [ ] **Step 2: Run** `cd src/go && $GO test ./cmd/gen-showcase/ -run TestGrooveEmit -v` → FAIL (unknown fields).
- [ ] **Step 3: Implement**: add `Groove string; GroovePct float64` to `Hit`; add `GrooveKind string \`json:"groove_kind,omitempty"\`; GroovePct float64 \`json:"groove_pct,omitempty"\`` to the generator `node` struct; in `build()` where regular nodes are created from hits, copy the fields. Validate in `build()`: unknown groove kind or pct outside 0..1 → error (fail fast at generation).
- [ ] **Step 4: Run** the test → PASS; run the whole package `cd src/go && $GO test ./cmd/gen-showcase/` → drift test still green (no spec uses groove yet, output unchanged).
- [ ] **Step 5: Round-trip check**: extend `internal/ui/template_import_test.go` only if it doesn't already assert groove; otherwise run `cd src/go && $GO test -tags test -modfile=go.test.mod -run TestImportGrooveParams ./internal/ui/` → PASS (importer contract already tested).

### Task 2: Multi-part score import (classical infra)

**Files:**
- Modify: `src/go/cmd/gen-showcase/score_import.go`
- Test: `src/go/cmd/gen-showcase/score_import_test.go` (extend)

**Interfaces:**
- Consumes: `internal/musicxml.Parse([]byte) (Score, error)` — `Score.Parts[].Notes[]{Semitone, OnsetDiv, DurationDiv, Voice, Rest, Chord}`.
- Produces: `scoreToShowcaseMulti(sc musicxml.Score, stem string, subdiv int, partInsts []InstSpec) Showcase` — maps part k → `partInsts[k]` (one row per (part,voice)); keeps existing `scoreToShowcase` untouched for asturias.

- [ ] **Step 1: Write failing test**: two-part synthetic Score (melody part + bass part) → expect 2 rows, correct instrument per row, onsets quantized to steps, chord notes rolled to consecutive free steps.
- [ ] **Step 2: Run** `cd src/go && $GO test ./cmd/gen-showcase/ -run TestScoreToShowcaseMulti -v` → FAIL (undefined function).
- [ ] **Step 3: Implement** `scoreToShowcaseMulti` (reuse the existing quantize/collision helpers; per-part volume/pan come from the `InstSpec`).
- [ ] **Step 4: Run** package tests → PASS.

### Task 3: Batch 1 — Funk/Disco/Pop/Rock/Hip-Hop/House (11 songs)

**Files:**
- Modify: `src/go/cmd/gen-showcase/specs.go` (each song's Showcase), `source_of_truth_test.go` (BPM/key truths if bars/BPM change), `musical_fidelity_test.go` (extend per-song checks)
- Regenerate: `src/go/internal/assets/templates/<stem>.json`

Songs + known-good source leads (BitMidi ids verified June 2026):
| Stem | Source lead |
|---|---|
| wonder-superstition | bitmidi 97097 (Eb min, 100bpm) |
| chic-good-times | bitmidi 23412 (A-centric transposition, kept) |
| gaynor-survive | Am cycle-of-4ths, 117bpm (prior transcription in specs) |
| eagles-hotel-california | real MIDI verified; Bm arpeggio cycle |
| toto-africa | kalimba riff verified; A4×5-G#4-C#5 |
| marley-exodus | bitmidi 18775 (Am, offbeat bass) |
| dre-g-thang | bitmidi 41193 (B minor) |
| blackbox-ride-on-time | bitmidi 88846 (Am-G-F-G, 120bpm) |
| bbking-thrill-is-gone | bitmidi 102720 (Bm, 88bpm, 12-bar) |
| miles-so-what | bitmidi 70824 (D-dorian→Eb-dorian) |
| jobim-ipanema | bitmidi 102483 + trillian.mit.edu ABC (F major) |

Per song, repeat this recipe (each sub-step is a checklist item per song):
- [ ] **Research**: dispatch a research agent to (a) re-fetch/expand the note source beyond the already-encoded signature part (verse/section continuation, secondary parts: keys, horn stabs, pads, percussion details) and (b) build an arrangement dossier: full instrumentation, groove feel (swung? behind the beat? pushed?), production character (reverb/space, brightness), section structure. BitMidi fetch: `curl -sS -L "https://bitmidi.com/uploads/<id>.mid" -H "User-Agent: Mozilla/5.0" -o <scratchpad>/<stem>.mid`, extract with the stdlib SMF approach (semitone = MIDI−57, quantize subdiv-16).
- [ ] **Encode**: expand the Showcase — target 8→16 bars (or a full 12-bar chorus ×1.33 for blues) covering a recognizable arc; add layer rows from the registry per the dossier (voice → best-fit lead instrument); per-hit `Vol` accents/ghosts (ghost ≈ 0.25–0.4 of accent vol); groove via `Hit.Groove` (e.g. swing = `delay` ~0.12–0.2 on offbeat 8ths/16ths of hats+ghost snare; reggae one-drop = slight `delay` on skank; disco = straight); mix: pan spread (keys L, guitar R, percussion off-center), reverb sends (leads/snare higher, bass/kick ~0), lowpass inserts on noise percussion.
- [ ] **Verify notes**: parse the encoded Hits back to note names and diff against the raw source extraction (script in scratchpad); update `musicTruths` (key pcSet, BPM band) and add/extend the song's fidelity test to pin the NEW signature material (not just the old riff).
- [ ] **Regen + gates**: `cd src/go && $GO run ./cmd/gen-showcase && $GO test ./cmd/gen-showcase/ && $GO test -tags test -modfile=go.test.mod ./internal/assets/ && $GO test -tags test -modfile=go.test.mod -run 'TestTemplates' ./internal/ui/` → all PASS.
- [ ] **Batch audio gate** (once per batch, real build ~2min): `cd src/go && CGO_ENABLED=1 $GO test -run 'TestTemplateAudioQuality|TestTemplateMasterMix' ./internal/audio/` → PASS. Fix any per-solo flatness/harmonic/headroom failures by adjusting volumes/decay/lowpass (NOT by weakening the guard; the flatness<0.50 and pitched-row harmonic>50% thresholds stand).

### Task 4: Batch 2 — Latin (4 songs)

Same recipe as Task 3 for: puente-oye-como-va (bitmidi 91454), salsa-vivir (hook already sourced; enrich montuno/bongó/campana layers per standard salsa instrumentation dossier), bachata-obsesion (requinto riff research; bongó+güira layers — güira substitute: shaker with tight lowpass), cielito-trumpet (ABC from abcnotation.com already sourced; add mariachi vihuela-style offbeat chords on guitar-nylon + bass root-fifth).
- [ ] Research ×4 (agents) → dossiers
- [ ] Encode ×4 (clave discipline: son clave 2-3/3-2 consistent across all percussion rows; montuno anticipations via `rush`)
- [ ] Verify + truths/fidelity ×4
- [ ] Regen + gates + batch audio gate

### Task 5: Batch 3 — Classical from real PD scores (9)

**Files:** as Task 3 plus `src/go/cmd/gen-showcase/scores/*.musicxml|.mxl` (new embedded scores), `score_import.go` embeds.

- [ ] **Fetch real scores** (agents + curl): music21 corpus raw GitHub (`bwv846.mxl` Prelude in C, cello suite BWV1007, flute partita BWV1013, K545, Canon in D…), Mutopia for the rest. `.mxl` = zip → unzip inner `.xml`.
- [ ] Per piece: parse via `scoreToShowcaseMulti` (or single-part `scoreToShowcase`), pick bar range (opening 8–16 bars, recognizable), assign instruments (violin-ensemble layering for Vivaldi tutti, organ manuals+pedal split rows for Toccata, LH/RH piano rows for K545/felt-prelude), light groove only (baroque = straight; romantic rubato out of scope), mix: reverb sends higher (hall), gentle pan by section seating.
- [ ] Embedded-score drift: each new score file referenced by `//go:embed`; drift test covers output.
- [ ] Regen + gates + batch audio gate.
- [ ] asturias: verify against its existing MusicXML; enrich (second guitar voice dynamics, pedal-note accents) by editing the MusicXML, not specs.

### Task 6: Batch 4 — Etudes → mini-arrangements (7)

felt-prelude, sax-blues, steel-folk, electric-riff, clav-funk, bass-groove, conga-tumbao: each 2 bars → 8-bar mini-arrangement with 3–5 rows (lead + bass + drums/percussion + optional pad), researched genre-idiomatic patterns (12-bar blues form for sax-blues; travis-picking figure for steel-folk; funk 16th scratch + ghost snare for clav-funk; full tumbao family — conga/bongó/campana/clave — for conga-tumbao). These have no external song truth; fidelity = key/BPM truths + idiom checks (e.g. sax-blues stays blues-scale, conga-tumbao keeps 2-3 clave).
- [ ] Encode ×7, regen + gates + batch audio gate.

### Task 7: Final verification + audition renders

- [ ] Full fast suite: `cd src/go && $GO test -tags test -modfile=go.test.mod ./...` → green (pre-existing branch failures per memory are exempt; list them in the final report).
- [ ] Full real audio suite: `cd src/go && CGO_ENABLED=1 $GO test ./internal/audio/` → green.
- [ ] Render every template: `for t in <all 33 stems>; do make song-render TEMPLATE=$t OUT=tmp/template_audition/$t; done` → master.wav + stems per template.
- [ ] Spot-check 5–6 renders with the analysis CLI (`make song-timeline FILE=...`) for obvious level/mud problems before handing to user.
- [ ] Final report: per-template one-line summary of what changed + reminder that `make wasm` is required to hear the new templates in-browser.
