# Public-Domain Template Curation & Score Reference Corpus — Design

**Date:** 2026-08-22
**Status:** SUPERSEDED by
`2026-09-02-public-domain-template-curation-design.md`.

Retained for the record. Two of its load-bearing premises turned out to be
wrong once the source survey was done properly: no CC0 MusicXML corpus was
thought to exist (three do, totalling ~1800 works), and music21's corpus was
believed to be BSD-licensed wholesale (its `corpus/license.txt` says otherwise
— see `docs/score-sources.md`). Because a committed, permissively licensed
corpus IS achievable, the fetch-and-never-commit contract below — and the
drift-test weakening it forced — were not needed.

## Goal

Two coupled changes:

1. **Curate.** Remove every template circuit not derived from a
   public-domain musical work backed by a real, citable notated score.
   32 templates → 7.
2. **Persist provenance.** Give every surviving template a verifiable
   reference score, stored the way reference WAVs are stored: a
   committed catalogue plus a fetched, never-committed corpus.

## Why the cut is this deep

Template provenance falls into three classes, established by reading
`cmd/gen-showcase/spec_*.go` and `source_of_truth_test.go`:

| Class | Count | Backing |
|---|---|---|
| Real score | 11 | Public-domain MusicXML, embedded in `cmd/gen-showcase/scores/` |
| MIDI transcription | 15 | bitmidi ids cited in comments — no score, and the works are under copyright |
| Hand-authored etude | 6 | Genre pedagogy prose; transcribes nothing |

The 15 transcriptions recreate copyrighted songs (Africa, Hotel
California, Superstition, …). Their sheet music cannot be obtained or
redistributed, so they can never satisfy the criterion. The 6 etudes
have no source at all.

Of the 11 score-backed templates, 4 fail on the score itself:

- `asturias` — 4 measures, 36 notes, **zero metadata**. Hand-authored,
  not sourced.
- `bach-flute-allemande` — `software: Neuratron PhotoScore`,
  `source: Scanned sheet music`. OMR output of unverified accuracy.
- `handel-water-horn` — `Arr. Michel Rondeau`, `rights: © 2018`. A
  copyrighted arrangement, not the public-domain original.
- `marcello-oboe` — anonymous MuseScore export; no title, creator, or
  source URL. Provenance unverifiable.

**Survivors (7):** `bach-toccata`, `bach-prelude-c`, `felt-prelude`,
`bach-cello-prelude`, `mozart-k545`, `pachelbel-violin`,
`vivaldi-spring`. Six distinct scores back them — `felt-prelude` and
`bach-prelude-c` share BWV 846.

## Why the scores must not be committed

Every available MusicXML encoding of these public-domain works carries a
restrictive licence. The works are PD; the *encodings* are not:

| Score | Encoding licence |
|---|---|
| mozart-k545 | `CC BY-NC-SA 4.0` (stated in-file) |
| vivaldi-spring | `Copyright 1996, CCARH` — "Rights to all derivative electronic formats reserved" |
| bach-toccata, bach-cello-prelude, pachelbel-canon | No rights field; KernScores/Sapp corpus is uniformly CC BY-NC-SA 4.0 per its `LICENSE.txt` |
| bach-prelude-c | Anonymous MuseScore upload; licence unknown |

No public-domain or CC0 MusicXML exists for these pieces on any
reachable source: OpenScore's CC0 repos cover Lieder and String Quartets
only; Mutopia ships LilyPond/PDF/MIDI, not MusicXML; music21's bundled
corpus lacks all but BWV 846.

`internal/wavref` already fixes this project's standard for reference
material at `unrestricted | CC0-1.0 | PD`. `CC BY-NC-SA` is not among
them, and a ShareAlike + NonCommercial artifact committed to a repo that
ships a product at beatmo.io is an exposure worth avoiding.

The resolution is the Tier-I song-WAV contract, applied verbatim: **the
catalogue is committed, the score files are not.** What ships in the repo
is the generated template JSON, which carries only pitch and onset —
facts about a public-domain composition, which an encoding's copyright
does not reach. The licensed artifact is the score file, and it stays out
of the tree.

## Architecture

### `internal/scoreref` — new package

Deliberately mirrors `internal/wavref`, including its skip-when-absent
contract.

```go
// Cat is the score reference catalogue.
type Cat struct {
    schema.Envelope                 // format "beatmo.scoreref-catalogue"
    Scores []Score `json:"scores"`
}

type Score struct {
    ID        string   `json:"id"`                  // "bwv565"
    Templates []string `json:"templates"`           // stems this score backs
    File      string   `json:"file"`                // bare name in the corpus dir
    Work      Work     `json:"work"`
    Source    Source   `json:"source"`
    Trim      Trim     `json:"trim"`                // measures actually used
    SHA256    string   `json:"sha256"`              // of the fetched file, verbatim
    Note      string   `json:"note,omitempty"`
}

type Work struct {
    Title    string `json:"title"`
    Composer string `json:"composer"`
    Catalog  string `json:"catalog,omitempty"`      // "BWV 565", "RV 269"
}

type Source struct {
    Provider string `json:"provider"`               // kernscores | github | imslp
    URL      string `json:"url"`                    // exact fetch URL
    License  string `json:"license"`                // CC-BY-NC-SA-4.0 | BSD-3-Clause | ccarh-restricted
    Credit   string `json:"credit,omitempty"`
}

type Trim struct{ FromMeasure, ToMeasure int }
```

API, matching `wavref` name-for-name: `Catalogue()`, `Ref(id)`,
`Dir()`, `Resolve(id) (path string, ok bool)`, `Load(id) ([]byte, error)`.

There is no `CommittedDir()` counterpart — unlike instrument-tier WAVs,
**no score tier is ever committed**, so `Resolve` consults only the
corpus directory.

### Corpus location

`$BEATMO_SCORE_REFS`, defaulting to `$HOME/.beatmo/score-refs`, declared
in `internal/env/vars.go` under `ScopeTest` alongside `WavRefs`:

```go
ScoreRefs = Str("BEATMO_SCORE_REFS", "", ScopeTest,
    "Directory of local, uncommitted reference scores; consumers skip cleanly when unset.")
```

This supersedes the earlier decision to move score files to
`internal/scoreref/scores/`. That directory is not created; the package
owns the catalogue, not the files.

### Fetch

KernScores serves MusicXML directly — `format=musicxml` on the `ksdata`
endpoint — so the fetcher is a plain HTTP GET with no Python or music21
dependency. Verified against the live host.

Sources, all confirmed reachable and content-matched against the
committed files' metadata:

| Score | Source URL | Licence |
|---|---|---|
| `bwv565` | `kernscores.stanford.edu/cgi-bin/ksdata?l=users/craig/classical/bach/organ&file=bwv0565.krn&format=musicxml` | CC BY-NC-SA 4.0 |
| `bwv1007-01` | `…?l=users/craig/classical/bach/cello&file=bwv1007-01.krn&format=musicxml` | CC BY-NC-SA 4.0 |
| `canon` | `…?l=users/craig/classical/pachelbel&file=canon.krn&format=musicxml` | CC BY-NC-SA 4.0 |
| `sonata15-1` | `raw.githubusercontent.com/craigsapp/mozart-piano-sonatas/main/kern/sonata15-1.krn` | CC BY-NC-SA 4.0 |
| `bwv846` | `raw.githubusercontent.com/cuthbertLab/music21/master/music21/corpus/bach/bwv846.mxl` | BSD-3-Clause |
| `rv269` | **unpinned — see Risks** | ccarh-restricted |

**`.mxl` is decompressed at fetch time.** `internal/musicxml.Parse`
takes raw XML and has no zip handling, but music21's `bwv846.mxl` is a
ZIP container. The fetcher unwraps it with `archive/zip` — reading the
root score path from `META-INF/container.xml` — and writes plain
`.musicxml` into the corpus, so `Load` stays a file read and the parser
is untouched. The catalogue's `sha256` therefore covers the
**decompressed** bytes, matching what a consumer actually reads.

`cmd/refs` gains a `scores` subcommand group reusing the existing
command scaffolding: `refs scores fetch`, `refs scores verify` (sha256
every local file against the catalogue), `refs scores doctor` (report
coverage per surviving template). Makefile targets `fetch-scores`,
`scores-verify`, `scores-doctor` mirror `fetch-refs` / `refs-verify` /
`refs-doctor`.

### gen-showcase

`//go:embed scores/*.musicxml` is removed from `spec_classical.go` and
`score_import.go`. Score bytes come from `scoreref.Load(id)`. When the
corpus is absent, `gen-showcase` exits with a message naming
`make fetch-scores`; it never silently emits an empty circuit.

**Templates are regenerated.** The fetched MusicXML is not byte-identical
to what is committed today — KernScores' own exporter produces a
different file from the music21 conversion currently in-tree (637 KB vs
153 KB for the Canon), and `bach-prelude-c` changes source entirely from
the anonymous MuseScore upload to music21's BWV 846. Regenerated output
is validated against `TestTemplatesComplyWithSourceOfTruth`, whose
key/tempo assertions are source-independent and therefore a genuine check
rather than a restatement of the new output.

## Deletions

- 25 template JSONs under `internal/assets/templates/`.
- 15 `spec_*.go` files (the copyrighted-song specs).
- The 6 etude builders in `spec_etudes.go`; `feltPreludeShowcase` moves
  to `spec_classical.go` and the file is deleted.
- The asturias path in `score_import.go`; `scoreToShowcase*` helpers stay.
- **All 10 `.musicxml` files leave the repo**: the 4 failing scores are
  deleted outright; the 6 surviving scores move to the fetched corpus.
  All 10 `//go:embed` directives (9 in `spec_classical.go`, 1 in
  `score_import.go`) go with them.
- `templateGroups` collapses to one `Classical` group of 7 stems.

**Instrument definitions are not touched.** Coverage drops from 75
distinct instrument ids to 18, but every instrument stays registered and
playable; they are simply no longer exercised by a template.

## Guards

Trimmed, never weakened:

- `musicTruths` → 7 entries; `TestAfricaChorusHasFlatSeven` and
  `asturias_fidelity_test.go` deleted with their subjects.
- `templates_test.go` orphan/embed guard updated to the new count.
- New `TestScoreCatalogueIntegrity` in `internal/scoreref`: every entry's
  `templates` resolve in `templateGroups`; no template lacks a score; ids
  unique.
- New `TestScoreCorpusMatchesCatalogue`: sha256 of each fetched file
  matches; **skips cleanly** when the corpus is absent, per the wavref
  contract.
- `refs/testdata/doctor_golden.txt`, `cmd/refs/song_test.go`, and the
  `marcello-oboe` song-ref in `wavref/catalogue.json` updated.
- `internal/ui` template tests and
  `src/js/template_instruments_audible.browser.test.js` updated to
  surviving stems.

## Risks

**`drift_test.go` becomes fetch-dependent.** It compares committed JSON
against `build()` output, which now needs the corpus. It must skip when
the corpus is absent — a real weakening: an offline run no longer catches
spec/JSON drift. Mitigated by the committed JSON remaining the shipped
artifact and by `make fetch-scores` being a documented prerequisite.

**`vivaldi-spring`'s source URL is not yet pinned.** The CCARH/MuseData
location on `kernscores.stanford.edu` was not found during design;
`kern.humdrum.org` is returning 503. If a fetchable URL cannot be
established during implementation, `vivaldi-spring` is cut for the same
reason as the other four — unverifiable provenance — leaving 6 templates.
It is not to be kept on an unverified URL.

**Regeneration changes shipped circuits.** `bach-prelude-c` and
`felt-prelude` change source encoding, so their circuits will differ
audibly from today's. This is a provenance improvement, not a regression,
but it is a user-visible change to two templates.

**Program-spec conflict.** `2026-08-19-reference-grade-templates-program-design.md`
(APPROVED) states the 21 hand-authored templates "must stay that way",
and its SP4 criterion — every instrument gets a circuit — is unreachable
after this cut. That document is amended in the same change: SP3's
retention clause marked superseded, SP4/SP5 rescoped to the surviving
instrument set.
