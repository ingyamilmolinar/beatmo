# Reference Score Sources — Survey and Admission Registry

**Surveyed:** 2026-09-02 · **Status:** living document

Beatmo's built-in templates are generated from real notated scores
(`cmd/gen-showcase`). This document records **every** score repository
considered as a source for those scores, what it actually ships, its
licence, and whether it is admissible — so the survey is not repeated
and so a future addition can be judged against a fixed rule rather than
re-argued.

The companion machine-readable registry is
`src/go/internal/scoreref/catalogue.json` (per-score entries). This file
is the *source-level* survey that sits above it.

---

## The admission rule

A score is admissible only if it passes **all three** gates. Each gate is
recorded per score in the catalogue and enforced by a test.

### Gate 1 — the *work* is public domain

First published at least 96 years ago **and** every author — composer,
arranger, lyricist — dead at least 71 years (`scoreref.PublicationCutoff` /
`DeathCutoff`; in 2026 that is `<= 1930` and `<= 1955`). The cut-offs move
with the calendar since 2026-10-02 — the hardcoded 1929 was already a year
stale — and only ever widen, so an admitted work can never fall back out.

Deliberately conservative: it clears US publication-based PD and EU
life+70 simultaneously, so no jurisdiction analysis is needed per piece.
A co-written song stays protected until its LAST author's term ends, so a
score that names a lyricist must declare the lyricist's dates
(`TestLyricistDeclaredWhereTheScoreNamesOne`).

A **traditional** work (a folk tune, a rhythmic idiom such as son clave) has
no author to outlive; it is admitted on a cited `reference` establishing that
it is traditional. A named arranger of a traditional tune still has to clear
the death gate.
This gate is what excludes Toto's *Africa* and Stevie Wonder's
*Superstition* — and, less obviously, CC0 *encodings* of works still
under copyright. The CC0 corpora below contain Bartók, Berg, Bridge,
Gurney, Ives, Poldowski, Schoenberg, Warlock and Webern; a permissive
encoding licence says nothing about the underlying work, and several of
those fail Gate 1.

### Gate 2 — the *encoding* is permissively licensed

Allowlist: **`CC0-1.0` | `PD` | `unrestricted` | `CC-BY-4.0`**. The first
three are the bar `internal/wavref` already sets for reference audio;
CC BY is admitted subject to a mandatory `attribution` catalogue field
(see *Resolved* below).

Explicitly **not** admissible, and why:

| Licence | Why rejected |
|---|---|
| `CC BY-NC-SA`, `CC BY-NC` | NonCommercial. Beatmo ships at beatmo.io. |
| `CC BY-SA` | ShareAlike copyleft on a derivative-producing build step. |
| `CPDL` | Copyleft (ShareAlike); no NC restriction, but same problem. |

| No licence stated | Absence of a grant is not a grant. |

### Gate 3 — the provenance is verifiable

A named editorial project or an explicit in-file rights statement, an
exact upstream URL, and a SHA256 over the committed bytes. Anonymous
uploads fail this gate even when the licence is fine, because nothing
attests that the notes are right — which is the whole point of sourcing
from a score.

**Corroboration is required, not assumed.** Where a file carries an
embedded `<rights>` element, a test parses it and matches it against the
catalogue's recorded licence. Where it does not, the upstream repo's
SPDX id is recorded instead.

---

## Admissible sources

All four were verified directly on 2026-09-02 (GitHub API for SPDX ids,
raw file fetch for embedded `<rights>`).

| Source | Licence | Format | Size | Repertoire |
|---|---|---|---|---|
| [OpenScore Lieder](https://github.com/OpenScore/Lieder) | `CC0-1.0` (repo SPDX) | `.mxl` | 1462 songs | Art song, ~1700–1920. Schubert (incl. *Erlkönig*, *Ave Maria*, *Ständchen*, *Der Lindenbaum*, *Die Forelle*, *Gretchen am Spinnrade*), R. & C. Schumann, Brahms, Wolf, Fanny Hensel, Fauré, Debussy, Satie, Chopin Op.74, Beach, Burleigh, Coleridge-Taylor, Chiquinha Gonzaga, Liliʻuokalani, Thomas Arne, Rossini, Verdi, Joplin |
| [OpenScore String Quartets](https://github.com/OpenScore/StringQuartets) | `CC0-1.0` (repo SPDX) | `.mxl` | 122 quartets | Haydn (Op.1/9/20/64/74/76), Mozart, Beethoven (Op.18/59/127/130/133), Schubert, Brahms, Dvořák (*American*), Borodin, Boccherini, Arriaga, Andrée, Beach |
| [OpenScore Orchestra / Hauptstimme](https://github.com/MarkGotham/Hauptstimme) | Scores `CC0-1.0` (README: "Scores: CC0 1.0 Universal"; repo `LICENSE` is MIT and covers the code only) | `.mxl` | 192 movements | **Bach** Brandenburg Concertos 3 & 4, B Minor Mass; **all nine Beethoven symphonies**; Brahms 1–4 + *Ein deutsches Requiem*; Haydn 104; Schubert 5; Bruckner 5; Beach *Gaelic*; Lili Boulanger |
| [music21 corpus](https://github.com/cuthbertLab/music21) — *individual files only* | per-file `<rights>` | `.mxl` | 3 qualifying files | `liliuokalani/aloha_oe.mxl` ("Now, Public Domain; Encoding by MSAC, CC0"), `johnson_j_r/lift_every_voice.mxl` ("Public Domain (Encoding by MSAC: CC0)"), `leadSheet/fosterBrownHair.mxl` ("Public Domain") |

### Mutopia Project — admissible per file (added 2026-10-02)

[Mutopia](https://www.mutopiaproject.org) publishes typeset LilyPond editions,
each under a per-piece licence stated in its `.ly` header (`license = "…"`, or
`copyright = "…"` in older files): **Public Domain**, CC0, CC BY 4.0 — or CC
BY-SA, which Gate 2 rejects. It ships LilyPond-generated **MIDI** beside every
source, and `internal/smf` parses it into the same score model as MusicXML, so
no LilyPond parser is needed.

- **Both files are committed**: the `.mid` (what the generator reads) and the
  `.ly` (the companion). The companion is where Gate 2 is corroborated
  (`corroborateFromLilyPond`) and where the **pickup** is: MIDI cannot state
  an anacrusis, LilyPond starts tick 0 at the pickup bar, and the catalogue's
  `pickup` must equal the source's first `\partial`
  (`TestMIDIPickupMatchesLilyPondPartial`).
- Mutopia builds its MIDI with `\unfoldRepeats`, so measure numbers in a
  MIDI edition are **performed** bars, not printed ones.
- LilyPond writes one constant velocity when a score marks no dynamics; that
  reads as "no dynamics", never as a uniform forte.
- `scripts/verify_scores.sh` re-fetches both the `.mid` and the `.ly`.

Admitted 2026-10-02 (Public Domain) for the first set of groove templates —
Joplin rags, Sousa marches, *Soldier's Joy* — and removed again the same day
when those templates were replaced (the tunes were not well known enough).
Mutopia remains an admissible source; no template uses it today.

Rejected: Sousa *The Washington Post* and Foster *Old Folks at Home* (CC BY-SA
3.0 editions); *Red Apple Rag* (no citable source establishing it as
traditional); the Japanese folk-song arrangements of the 1895 Nagai/Obata
collection (arrangers' death years unknown); *Solace* (a multi-file source with
no single companion to corroborate).

### Beatmo originals, famous breaks, Korobeiniki (added 2026-10-02)

The groove templates showcase 70s-and-later music, which is copyrighted —
its MELODIES cannot ship. They pair three things instead:

- **Famous drum breaks as rhythm patterns** (`groove-funky-drummer`,
  `groove-amen-break`, `groove-we-will-rock-you`, …, catalogued `rhythm`).
  A short rhythm is generally not a protectable work of its own; each is
  Beatmo's own CC0 encoding of a published step transcription
  (drum-patterns.com), cited as the `reference`, and fingerprinted against it
  (`TestGrooveScoresMatchTheirIdiom`). No recording is sampled.
- **Original tunes** composed for Beatmo and dedicated CC0
  (`beatmo-*.musicxml`, catalogued `original`, composer `Beatmo`).
- **Korobeiniki** (the Tetris theme) — a public-domain Russian folk song,
  transcribed from the public-domain sheet on Wikimedia Commons
  (`File:Korobeiniki.jpg`) and checked pitch by pitch against it
  (`TestKorobeinikiMatchesTheSheet`). The best-known public-domain melodies
  (Jingle Bells, Happy Birthday, Hava Nagila, …) have no admissible
  MACHINE-READABLE edition: IMSLP holds scans and NC/SA typesettings only.

### Beatmo idiom scores — the groove layer (added 2026-10-02)

A template's groove is a **traditional rhythmic idiom** (son clave, the
tresillo, the 12/8 bell pattern, four on the floor…), which no corpus
publishes as a score. Beatmo encodes them itself as MusicXML percussion parts
(`internal/scoreref/scores/groove-*.musicxml`, provider `beatmo`,
`<rights>Beatmo (CC0)</rights>`), each catalogued as `traditional` with a
`reference` describing the idiom. Because the project is its own source here,
each groove carries a second, independent entry: the defining instrument's
onsets must equal the textbook pattern (`TestGrooveScoresMatchTheirIdiom`).

### ⚠ music21 is admissible per-file, never wholesale

`music21/corpus/license.txt` states the encodings are "distributed with
the permission of the encoders" and that "some encodings included in the
corpus may not be used for commercial uses or have other restrictions."
The repo's BSD-3-Clause `LICENSE` covers the **software**, not the
corpus. Spot-checking found:

- `leadSheet/berlinAlexandersRagtime.mxl` — `<rights>` is **"All Rights
  Reserved"**.
- `bach/bwv846.mxl` — no rights field; `<source>` is an anonymous
  `musescore.com/score/117279` upload. *This is the file that currently
  backs `bach-prelude-c` and `felt-prelude`.* Fails Gate 3.
- `mozart/k545/movement1_exposition.mxl`, `joplin/maple_leaf_rag.mxl`,
  `handel/rinaldo/Lascia_chio_pianga.mxl`,
  `beethoven/opus18no1/movement1.mxl`, `haydn/opus74no1/movement1.mxl` —
  no rights field at all.
- `beach/prayer_of_a_tired_child.musicxml` — "Engraving Copyright 2017
  Michael Scott Cuthbert, most uses allowed for free (CC-SA-BY)".
  ShareAlike → Gate 2 fails. (Beach is reachable via OpenScore instead.)

Any future music21 file must have its own `<rights>` read before use.

### In-file corroboration strings (verified 2026-09-02)

Every admissible corpus stamps its licence into the file, so Gate 2 is
checkable without trusting this document:

| Corpus | `<rights>` value | Extra |
|---|---|---|
| OpenScore Lieder | `OpenScore (CC0)` | a `<creator>` records the source edition, e.g. `Transcribed from the IMSLP file(s) at https://imslp.org/wiki/Special:ReverseLookup/15041` — Gate 3 is satisfied per file, not merely per repo |
| OpenScore String Quartets | `OpenScore (CC0)` | same transcription-provenance creator field |
| Hauptstimme / OpenScore Orchestra | `Score: CC0 1.0 Universal; Annotations: CC-By-SA` | the CC-BY-SA half covers the *analysis annotations*, which we do not use — only the notes |

Structural facts confirmed on real files (`Der Erlkönig` D.328;
Beethoven 5 mvt 1; Brandenburg 3 mvt 1): all are zipped `.mxl`
containing `score-partwise` XML with `META-INF/container.xml`, and all
carry `<measure number>`, `<dynamics>`, `<tie>` and `<part-name>`.
Beethoven 5 additionally carries `<transpose>` (Bb clarinets) and
`<repeat>`. `internal/musicxml` currently reads none of those five —
see the architecture section of the design.

---

## Rejected sources

Recorded so the analysis is not redone.

### Rejected on licence (Gate 2)

| Source | Format | Licence | Note |
|---|---|---|---|
| [KernScores / Craig Sapp corpora](https://kernscores.stanford.edu) — `bach-370-chorales`, `mozart-piano-sonatas`, `joplin`, organ/cello/Pachelbel sets | `**kern`, MusicXML on demand | `CC BY-NC-SA 4.0` (verified in each repo's `LICENSE.txt`) | **Backs 4 of today's templates.** Excellent scholarly editions; NonCommercial is fatal. Serves MusicXML directly via `ksdata?…&format=musicxml` if the policy ever changes. |
| [CCARH / MuseData](http://www.musedata.org) | MusicXML | "Copyright 1996, CCARH — rights to all derivative electronic formats reserved" | Backs today's `vivaldi-spring`. |
| [Josquin Research Project](https://github.com/josquin-research-project/jrp-scores) | `**kern` | `CC BY-NC 4.0` | ~700 works, 1420–1520. |
| [ASAP dataset](https://github.com/fosfrancesco/asap-dataset) | MusicXML + MIDI | `CC BY-NC-SA 4.0` | 222 scores aligned to 1068 performances. |
| [SEILS dataset](https://github.com/SEILSdataset/SEILSdataset) | MusicXML/MEI | `CC BY-NC-SA 4.0` | 30 16th-c. Italian madrigals. |
| [SymbTr](https://github.com/MTG/SymbTr) | symbolic | `CC BY-NC-SA 4.0` | 2200 Turkish makam pieces. |
| [DCMLab corpora](https://github.com/DCMLab) | MuseScore/MusicXML | `CC BY-NC-SA` | Beethoven sonatas/quartets, Mozart sonatas, Chopin. |
| [eduardomourar/music-scores-musicxml](https://github.com/eduardomourar/music-scores-musicxml) | MusicXML | `CC BY-SA-4.0` | PD works, ShareAlike engraving. |
| [MarkGotham/When-in-Rome](https://github.com/MarkGotham/When-in-Rome) | MusicXML | `CC BY-SA 4.0` | Its own README points to OpenScore Lieder for permissive scores. |
| [CPDL / ChoralWiki](https://cpdl.org) | PDF + MusicXML per edition | CPDL licence is **copyleft/ShareAlike**, no NC restriction; individual editions may instead be PD or CC | 51k+ choral works — the largest near-miss. PD-marked editions *would* pass all three gates, but the site is behind Cloudflare (both `curl` and WebFetch got 403/challenge), so no automated survey or fetch is possible. Re-open only with manual, per-edition download. |

### Rejected on format — nothing here can be parsed

| Source | Ships | Note |
|---|---|---|
| ~~[Mutopia Project](https://www.mutopiaproject.org/)~~ | LilyPond `.ly`, PDF, MIDI | **Re-opened 2026-10-02** through its LilyPond-generated MIDI (`internal/smf`) — see *Mutopia Project* above. |
| [Project Gutenberg sheet music](https://www.gutenberg.org/browse/categories/4) | **Finale `.MUS`** (proprietary binary) | Verified on ebooks 4949 (Bach Brandenburg 2) and 4749 (Beethoven Op.18 No.1) — every file is `.MUS`. Repertoire is superb (Brandenburgs, all 16 Beethoven quartets); format is a dead end. |
| [Open Well-Tempered Clavier](https://welltemperedclavier.org) / [Open Goldberg](https://opengoldbergvariations.org/) | CC0, but score distribution is via musescore.com only | archive.org mirrors (`OpenWell-TemperedClavier_Book_I`, `J.S.-Bach-Open-Goldberg-Variations-BWV-988-Piano`) are CC0-marked but carry **audio only** — no score files. Genuinely CC0 Bach; simply not fetchable. Superseded by Hauptstimme for Bach coverage. |
| ABC-format collections — `ryansMammoth`, `oneills1850`, `essenFolksong`, `airdsAirs`, `miscFolk`, `josquin` (all in music21); [abcnotation.com](https://abcnotation.com) (~800k tunes); The Session; Nottingham | ABC | `internal/musicxml` reads MusicXML only. A large traditional/folk seam that opens the moment an ABC parser exists — worth revisiting if folk repertoire is wanted. |
| [Werner Icking Music Archive](https://icking-music-archive.org/) | merged into IMSLP 2012–14 | No longer an independent source. |
| Scan libraries — IMSLP PDFs, Library of Congress, Levy Collection, Duke Historic American Sheet Music, Musopen | page images / PDF | No symbolic data. OMR output is inadmissible on accuracy grounds — see `bach-flute-allemande` below. |

### Rejected on provenance (Gate 3)

| Source | Note |
|---|---|
| [PDMX](https://zenodo.org/records/14648209) | 250k MusicXML scores from MuseScore, dataset licensed `CC-BY-4.0`, 1.9 GB `mxl.tar.gz`. Contains Pachelbel's Canon and much else. Every score is an anonymous user upload with no named editor and no cited source edition — exactly the provenance failure that disqualifies today's `bach-prelude-c`. Admissible only per-score, and only with independent verification against a cited urtext. |
| [musescore.com](https://musescore.com) PD-tagged scores | Same provenance problem, plus download requires an account. The OpenScore corpora are the curated, attributable subset of this pool — prefer them. |
| [IMSLP](https://imslp.org) MusicXML attachments | Uploader-supplied, licence declared per file. The MediaWiki API works but has no MusicXML-format facet (`Category:Scores with MusicXML` is empty), so there is no way to enumerate candidates. Per-file manual sourcing only. |
| [Hymnary.org](https://hymnary.org), [NEUMA](https://neuma.huma-num.fr/), Folkopedia, Folkoteca Galega, Lusthof der Muziek, Hausmusik, Guitar Loot, Ponchielli Band Music | Small or licence-silent. NEUMA ships MusicXML/MEI with a REST API but states no licence; the rest publish no machine-readable terms. Re-check only if a specific piece is wanted. |
| Musicalion, Visaudio, YourScore, La mà de guido, free-scores.com, Pianocoda | Subscription or purchase. Not applicable. |

---

## Scores currently in the tree — disposition

`cmd/gen-showcase/scores/` holds 10 committed `.musicxml` files. None
survives the rule:

| File | Fails | Detail |
|---|---|---|
| `bach-toccata-12`, `bach-cello-prelude-8`, `pachelbel-canon-16`, `mozart-k545-12` | Gate 2 | KernScores/Sapp `CC BY-NC-SA 4.0` |
| `vivaldi-spring-13` | Gate 2 | CCARH "rights to all derivative electronic formats reserved" |
| `bach-prelude-c-16` | Gate 3 | anonymous `musescore.com/score/117279` upload |
| `marcello-adagio-16` | Gate 3 | anonymous MuseScore export; no title, creator or source |
| `bach-flute-allemande-8` | Gate 3 | `<software>Neuratron PhotoScore</software>`, `<source>Scanned sheet music</source>` — unverified OMR output |
| `handel-hornpipe-16` | Gates 1 & 2 | "Arr. Michel Rondeau", `<rights>© 2018</rights>` — a copyrighted *arrangement*, not the PD original |
| `asturias-opening` | Gate 3 | 4 measures, 36 notes, zero metadata — hand-authored, transcribes nothing |

---

## Resolved: `CC BY 4.0` is admissible with attribution

Decided 2026-09-02. `CC-BY-4.0` joins the allowlist, which becomes
**`CC0-1.0` | `PD` | `unrestricted` | `CC-BY-4.0`**.

CC BY's only condition is attribution — retain the notice, credit the
creator, indicate changes, link the licence — which a committed
`NOTICE-scores.md`, generated from the catalogue's mandatory
`attribution` field, satisfies for a repo that redistributes the files.

Two things this does **not** do, stated so the rule is not misread:

- It does not rescue any NonCommercial or ShareAlike source. NC forbids
  the use; SA would impose its terms on the generated templates.
  Attribution is irrelevant to both.
- It does not change a single current selection. All four admissible
  corpora are CC0. The only CC-BY prize was PDMX, which fails Gate 3
  on provenance regardless of its licence.

It is admitted because the rule should be right, not because we need it
yet.

## What to do when adding a source

1. Establish the licence from the repo's SPDX id **or** the file's
   `<rights>` — not from a project's marketing copy.
2. Check Gate 1 for the specific work, not the corpus. CC0 corpora
   contain in-copyright works.
3. Add a row here with the verdict and the evidence, then a per-score
   entry in `internal/scoreref/catalogue.json`.
4. If a source is rejected, record it here anyway. The point of this
   file is that no one repeats the search.
