# SP1 — Reference Audio Corpus and Provenance

**Date:** 2026-08-19
**Program:** `2026-08-19-reference-grade-templates-program-design.md`
**Status:** APPROVED — implement this session.

## Problem

Reference audio is the input to every other sub-project, and today it is
scattered, partly broken, and unprovenanced:

- **Three different locations.** Five Iowa MIS clips are committed under
  `internal/audio/testdata/refs/`; eighteen freesound files sit loose in
  the **repository root**; `internal/wavref` reads a third place entirely,
  `$BEATMO_WAV_REFS`, which is unset by default.
- **One fetcher is dead.** `scripts/fetch_song_refs.sh` points at
  `musopen.org/static/...`, which now returns **HTTP 403** behind
  Cloudflare. `songrefs/` contains only a MANIFEST and no audio.
- **Provenance is prose.** Licence and source URL live in hand-maintained
  Markdown tables that nothing validates. Nothing checks that a local file
  is the file the manifest describes.
- **Pitch metadata is repeatedly wrong.** Reference filenames and upstream
  metadata have misreported the note in at least four cases — a violin
  labelled "MIDI 62" is actually D5, a trumpet variously labelled F#4,
  MIDI 67 and G3 is actually G4. Tools that trust the label render the
  comparison at the wrong pitch and measure nonsense.
- **Coverage is unknown.** Nothing answers "which instruments have no
  reference, and why".

## Goal

One committed, machine-readable catalogue; one resolver; one CLI. Every
reference is reproducible from its catalogue entry, verified by hash, and
carries metadata **measured from the audio** rather than copied from a
filename.

## Non-goals

- Acquiring references for copyrighted songs. Those do not exist under a
  usable licence and the catalogue records that fact instead.
- Changing how analysis works. SP1 only supplies inputs; SP2 consumes them.

## Design

### Licence rule decides storage, and only storage

The repository already encodes the correct rule in `.gitignore`: *"no WAV
files are committed … force-add only if a file is verified license-clean
and intentionally versioned."* SP1 applies it consistently, with size as
the second axis:

| Tier | Content | Licence | Size | Storage |
|---|---|---|---|---|
| **II — instrument** | one isolated note/phrase per acoustic instrument, clipped to ≤3 s | Iowa MIS *unrestricted*, VCSL *CC0 1.0*, freesound *CC0* | ~176 KB each, ~30 files, ≈5 MB total | **committed**, force-added |
| **I — song** | full public-domain piece for a classical template | PD / CC0 | tens of MB each | **never committed**; fetched to `$BEATMO_WAV_REFS` |

Tier II being committed is the significant improvement: the instrument
references are the ones that drive per-instrument tuning and the ones
tests want, so committing them makes SP2 and SP4 reproducible in CI with
no environment setup. Tier I stays local, matching the existing
`fetch-song-refs` note that full songs are "too heavy for CI".

### The catalogue

`src/go/internal/wavref/catalogue.json` — committed, embedded, carrying
`schema.Envelope` so it follows the project's one versioning rule.

```json
{
  "schema_version": "0.0.1",
  "refs": [
    {
      "id": "violin-A4",
      "tier": "instrument",
      "instruments": ["violin", "violin-ensemble"],
      "file": "violin_A4.wav",
      "committed": true,
      "source": {
        "provider": "uiowa-mis",
        "url": "https://theremin.music.uiowa.edu/sound%20files/...A4.stereo.aif",
        "license": "unrestricted",
        "credit": "University of Iowa Musical Instrument Samples"
      },
      "convert": { "channels": 1, "rate": 44100, "bits": 16, "max_sec": 2.0 },
      "sha256": "fb768f6d...",
      "measured": { "f0_hz": 440.1, "semis_from_a3": 12, "peak": 0.83, "rms": 0.11 }
    }
  ]
}
```

Field notes that matter:

- **`instruments` is a list.** One reference legitimately serves an
  instrument and its variants — the violin sample is the reference for
  both `violin` and `violin-ensemble`, which differ by a documented
  character offset rather than by having separate ground truth.
- **`sha256` is over the converted file, not the download.** Verification
  only means something if it covers the artifact tools actually read, and
  conversion is deterministic (fixed flags, no dither, no normalisation).
- **`measured` is detected by running the analyzer**, never parsed from a
  filename. `semis_from_a3` is what `synth-match` needs to render the
  candidate at the reference's actual pitch, so getting it from audio
  removes a whole recurring class of wrong comparison.
- **No loudness normalisation on disk.** `peak` and `rms` are recorded so
  consumers can normalise at read time without the corpus losing level
  information.

Entries whose audio does not exist carry a status instead of a hash:

- `"status": "synthetic"` — the instrument has no acoustic counterpart
  (`fm-*`, `bass-808`, `kick-electro`, `scifi-lead`, `modular-pad`,
  `ensemble-lead`, `viola-pad`, `ghost-bass`). Counted as covered.
- `"status": "needs-source"` — acoustic, but no usable licence-clean
  isolated recording was found (`guitar-steel`, `guitar-electric`,
  electric `bass-guitar`, `clavinet`, Hammond-style `organ`). Counted as
  a gap, with the search already tried recorded in a `note`.

### Resolver — `internal/wavref`

Additive; existing call sites keep working unchanged.

```go
// Existing, unchanged — bare-filename lookup in $BEATMO_WAV_REFS.
func Dir() (string, bool)
func Path(name string) (string, bool)

// New.
func Catalogue() (Cat, error)              // embedded, parsed once
func (Cat) Ref(id string) (Ref, bool)
func (Cat) ForInstrument(inst string) []Ref
func (Cat) ForTemplate(stem string) (Ref, bool)
func Resolve(id string) (path string, ok bool)  // committed dir, then $BEATMO_WAV_REFS
func Verify(id string) error                    // sha256 of the on-disk file
```

`Resolve` searches the committed `internal/audio/testdata/refs/` first,
then `$BEATMO_WAV_REFS`. Consumers keep the established skip-clean
contract: a missing Tier-I file skips the test, it does not fail it.

### CLI — `cmd/refs`

Replaces both bash scripts. Go rather than bash because `doctor` must read
`modularInstrumentDefs` and the template JSONs, and because a fetcher that
converts audio and detects pitch deserves tests.

| Command | Behaviour |
|---|---|
| `refs fetch [-id X] [-instrument Y] [-tier T] [-only-missing]` | download → convert per the entry's `convert` spec → detect f0 → write file → record `sha256` and `measured` back into the catalogue |
| `refs verify [-id X]` | hash every resolvable local file against the catalogue; non-zero exit on mismatch |
| `refs doctor` | coverage matrix: every instrument in the builtin roster and every template stem, classified `covered` / `synthetic` / `needs-source`, with totals |
| `refs list [-tier T]` | table of id, instrument(s), licence, presence |
| `refs candidates -instrument X [-n 3]` | fetch several competing references for one instrument into a review directory, side by side, for ear selection |
| `refs choose -instrument X -id Y` | promote the chosen candidate into the catalogue as that instrument's reference; the rejected ones are recorded in `alternatives` with the reason |

### Candidate references and ear selection

Which recording is the *right* reference is a judgement call the metrics
cannot make. Two clean CC0 violin samples differ in mic distance, room,
dynamic, bow position and vibrato depth; picking the wrong one silently
sets the target for every subsequent tuning pass. Iowa MIS alone offers
several articulations and dynamics per instrument, and VCSL offers several
round-robins and kits.

So the catalogue does not assume a single obvious choice. For any
instrument where more than one plausible source exists, `refs candidates`
fetches them all — converted identically, so they are directly comparable
— into a review directory named by instrument, and prints a short
comparison table (measured f0, duration, peak/RMS, brightness) alongside.
The chosen reference is promoted with `refs choose`; the losers stay in
the catalogue under `alternatives`, each with a one-line reason, so the
decision is recorded rather than forgotten and can be revisited when an
instrument later proves hard to tune.

This makes reference selection an explicit, reversible, ear-in-the-loop
step, and it feeds SP2 directly: a rejected candidate plus its reason is
itself a labelled data point for the ear-verdict corpus.

Conversion uses `ffmpeg` when present, falling back to the existing pure
-Python `scripts/aiff_to_wav.py` path for AIFF so a machine without
ffmpeg can still refresh the Iowa MIS entries. `fetch` is idempotent: an
entry whose local file already matches its recorded hash is skipped unless
`-force`.

### Corpus contents

**Tier II sources**, both verified reachable during design:

- **University of Iowa MIS** — `theremin.music.uiowa.edu`, "MIS Pitches -
  2014", licence *"may be downloaded and used for any projects, without
  restrictions"*. Isolated sustained single notes, the ideal monophonic
  reference. Covers violin, viola, cello, double bass, flute, oboe,
  clarinet, bassoon, alto and soprano sax, trumpet, horn, trombone, tuba,
  mallets, piano.
- **Versilian Community Sample Library (VCSL)** — `github.com/sgossner/VCSL`,
  **CC0 1.0**, 4231 WAVs under a Hornbostel-Sachs layout. Covers the
  percussion and keyboard side that MIS lacks: conga, bongos, snare (five
  kits), toms, bass drum, hi-hat, cowbell, shaker, claps, claves, cymbals,
  timpani, four pianos, harpsichord, harp, pipe organ, tenor sax.
- **The eighteen freesound CC0 files currently loose in the repository
  root** are absorbed into the corpus: clipped, hashed, given catalogue
  entries recording their freesound id and CC0 licence, and moved out of
  the root. They cover several instruments neither MIS nor VCSL does —
  notably nylon guitar and baritone sax.

**Tier I sources**: archive.org, including the mirrored
`collection:musopen`, plus IMSLP. Both verified reachable; the direct
musopen static URLs the old script used are not, and are retired.

### Tests

- Catalogue schema validates; ids unique; every `instruments` entry exists
  in the builtin roster; every `template` stem exists under
  `internal/assets/templates/`.
- Every `committed: true` entry resolves and matches its hash — this is a
  real assertion in CI, not a skip, because those files are in the tree.
- Every `committed: false` entry skips cleanly when absent.
- `doctor` output is a golden, so a new instrument added without a
  reference decision fails the build rather than silently becoming a gap.
- `measured.semis_from_a3` round-trips through `DetectF0` for every
  committed entry — the direct regression guard against the mislabelled
  pitch bug.

### Migration

1. `scripts/fetch_synth_refs.sh` and `scripts/fetch_song_refs.sh` are
   deleted; `make fetch-synth-refs` / `make fetch-song-refs` become
   `make fetch-refs` delegating to `cmd/refs`.
2. The two `MANIFEST.md` files are **generated** from the catalogue by
   `refs list` rather than hand-maintained, so provenance has one source.
3. Root-level WAVs move into the corpus; the repository root stops
   accumulating reference audio.

## Risks

- **Upstream drift.** A source URL can rot or its content change. The
  recorded hash detects the change; `doctor` reports it; the catalogue
  entry keeps the last-known-good hash so the failure is loud rather than
  silent.
- **VCSL is a large repository.** Fetch pulls individual files over
  `raw.githubusercontent.com` rather than cloning, so cost is per entry.
- **Iowa MIS serves AIFF.** Conversion is required, hence the ffmpeg /
  Python fallback pair.
- **Committing ~5 MB of audio.** Bounded by the ≤3 s clip rule and
  reviewed per entry; the alternative — every contributor and CI job
  needing a manual download step — is worse for SP2 and SP4.
