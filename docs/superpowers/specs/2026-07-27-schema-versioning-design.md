# Schema versioning + userprefs marshal collapse

> **Amended 2026-10-02 (config-only synth engine, Phase B Task B7):** the migrate arm
> is gone. Readers now apply a strict rule — the current `schema_version` is accepted,
> anything else (an absent version included) is rejected. `IsLegacy` and the read-only
> `LegacyVersion` fields were deleted; the project reader still doubles as the
> foreign-JSON sniffer (a document without `schema_version` is "not a beatmo project
> file"), and boot-loaded stores (prefs, instruments) still start empty with a WARN.
> The sections below record the original three-way design.

Date: 2026-07-27

## Problem

Two related defects in how Beatmo serializes state.

**1. `marshalPrefsV3..V7` is a changelog frozen into function names.**
`internal/userprefs/store.go` grew one wrapper per prefs section. Each calls its
predecessor, then unmarshals the JSON it just received, sets one field, and
re-marshals:

```
marshalPrefs → V3 (audio_panel) → V4 (sample_edits) → V5 (notifications)
             → V6 (knob_steps)  → V7 (language)
```

A fully-populated save does **6 marshals + 5 unmarshals** of the whole document.
The read path has the same shape: `ensureLoadedLocked` calls six parse functions
(`parsePrefsV3`, `parseSampleEdits`, `parseNotifications`,
`parseKnobStepsFromPrefs`, `parseLanguageFromPrefs`), each independently
unmarshalling the entire document — duplicated verbatim across both backends
(`store_notjs.go:125-129`, `store_js.go:72-76`).

The chain's stated rationale — preserving deterministic ordering — does not hold.
`encoding/json` already sorts map keys, and struct field order is fixed by the
declaration. The only real sorting is `sort.Strings(ids)` for the favorites
slice. Determinism comes from the struct, not the chain.

Error handling also drifted: V3–V6 swallow a failed round-trip
(`return data, nil`, silently dropping the section) while V7 returns `nil, err`.

**2. Versioning is inconsistent across formats.**
Of six persisted formats, two carry `"version": 1` (int) and four carry nothing.
There is no shared notion of a schema version.

## Decisions

| Decision | Choice |
|---|---|
| Version representation | `"schema_version": "0.0.1"` — semver string; the old int `version` key is removed |
| Scope | JSON formats only; binary `.bsmp` and diagnostic JSONL untouched |
| Granularity | One shared constant in a new `internal/schema` package |
| Mismatch policy | Reject unknown versions; migrate version-less legacy files |

Version stays pinned at `0.0.1` until first release.

## Design

### `internal/schema`

A dependency-free package importable by `ui`, `audio`, `userprefs`, and
`uistate` without cycles.

```go
const Version = "0.0.1"

type Envelope struct {
    SchemaVersion string `json:"schema_version"`
}

func Current() Envelope
func (e Envelope) Validate(format string) error
func (e Envelope) IsLegacy() bool   // no schema_version present
```

`Envelope` is embedded anonymously. `encoding/json` flattens anonymous structs,
so it contributes a top-level `schema_version` key rather than nesting.

### Three-way version rule

Applied identically by every format:

| On-disk state | Behavior |
|---|---|
| `schema_version == "0.0.1"` | Accept |
| `schema_version` present, any other value | **Reject** with an error naming format, want, got |
| `schema_version` absent | **Migrate**: accept, log, stamp on next write |

The migrate arm exists so pre-versioning files already on disk keep working. It
is a pre-release affordance and should be removed at 1.0.0.

**Legacy detection is per-format, and on the project format it carries a second
job.** The project importer's old `f.Version < 1` check was doing double duty:
rejecting unsupported versions AND sniffing out non-Beatmo JSON. Import is
replace-not-merge, so a document that parses but isn't ours (a bare `{}`, some
other app's file picked in the dialog) used to import as an all-zero project —
a silent data wipe that returned success. A naive "absent version ⇒ migrate"
rule would reinstate exactly that bug.

So the retired integer marker survives as a **read-only** `LegacyVersion int`
(`json:"version,omitempty"`) on the project and bundle structs. Exporters leave
it zero and `omitempty` drops it, so new files carry `schema_version` alone. On
the project format, the legacy arm requires the marker to be *present*
(`LegacyVersion >= 1`); a document with neither marker is rejected as "not a
beatmo project file". The instrument bundle needs no such care — its `format`
magic string already rejects foreign JSON — but keeps the field so old bundles
still import. Both fields, and the legacy arms that read them, are deleted once
no unversioned files remain.

### Recovery differs by format class

Validation is equally strict everywhere; only recovery differs, because a fatal
error is the wrong response for a file the app wrote itself.

- **Documents opened explicitly** — project JSON, instrument bundle. A rejected
  version is a hard error surfaced in the UI. The user chose a file; they should
  be told it is wrong.
- **Stores loaded automatically at boot** — prefs, instruments, uistate. A
  rejected version logs a WARN and starts from empty. This is already the
  existing behavior for a corrupt file (`ensureLoadedLocked:133-141`). A hard
  error here would fail app startup on the app's own output.

### userprefs collapse

`prefsSnapshot` (already defined at `store_notjs.go:372`, with exactly the eight
fields the chain threads positionally) becomes the single currency. It moves to
`store.go` so both backends share it.

```go
func marshalPrefs(snap prefsSnapshot) ([]byte, error)          // one marshal
func parsePrefs(data []byte, logf logFn) (prefsSnapshot, error) // one unmarshal
```

Deleted: `marshalPrefs` (old 3-arg form), `marshalPrefsV3`, `marshalPrefsV4`,
`marshalPrefsV5`, `marshalPrefsV6`, `marshalPrefsV7`, `parsePrefs` (old form),
`parsePrefsV3`, `parseSampleEdits`, `parseNotifications`,
`parseKnobStepsFromPrefs`, `parseLanguageFromPrefs`.

Composability is preserved through small pure section helpers shared by both
directions — `sanitizeFavorites`, `sanitizeOverrides`, `sanitizeUserRecipes`,
`sanitizeSampleEdits`, `sanitizeKnobSteps`, and the existing
`boundNotifications`. Composition by function, not by chained re-marshalling.
Adding a section later is one struct field plus one helper — never a `V8`.

This also collapses the error-handling divergence into a single path.

### Formats touched

| Format | Change |
|---|---|
| Project JSON | Remove `Version int`; embed `Envelope`; `import.go` validates |
| Instrument bundle | Remove `Version int` + `InstrumentBundleVersion`; keep the `format: "beatmo.instrument"` discriminator (identifies *which* format — orthogonal to version) |
| `prefs.json` / `beatmo.prefs` | Embed `Envelope` (new) |
| `instruments.json` / `beatmo.instruments` | Embed `Envelope` (new) |
| uistate | Embed `Envelope` (new) |

`exportFile` and `importFile` are field-identical except `Subdiv`'s `omitempty`
and must already be edited in lockstep. They are unified into one `projectFile`
struct as a targeted cleanup in code this change already touches.

### Explicitly out of scope

- `.bsmp` sample blobs — binary, no magic marker. Left alone per scope decision.
- Diagnostic JSONL — scopeexport, eventstream, recording-session metadata.
  Throwaway debug output.
- `src/js/desktop_audio.json` — a generated cross-platform audio-comparison
  artifact written by `cmd/export_audio`, not a format the game parses at
  runtime. Keeps its own independent `version`.
- `scripts/gen_design_tokens/schema.json` — a JSON Schema document, unrelated.

## Migration of existing files

- **32 templates** (`internal/assets/templates/*.json`) are generated —
  regenerate via `cmd/gen-showcase` and update its writer + drift test.
- **Asset fixtures** (`internal/assets/*.json`), **testdata**
  (`internal/ui/testdata/*.json`), **repo project files** (`beatmo.json`,
  `beatmo-export.json`, `clean_test_demo.json`, `src/go/beatmo-export.json`),
  and **JS fixtures** (`src/js/llm_test/tests/fixtures/*.json`) are rewritten
  in place.
- **The user's live prefs** at `~/.config/beatmo/prefs.json` carries real data
  (favorites, a recipe override, two sample edits, two knob rungs, language) and
  no version. The migrate arm handles it on first read; it is stamped on next
  write. No data loss.
- `~/.config/beatmo/favorites.json` is a stale pre-`prefs.json` artifact
  referenced only by tests. Left untouched.

## Testing

- Round-trip tests per format: write → read → identical snapshot.
- Version rule table test: accept / reject / migrate, per format class.
- Rejection test asserting the error names format, want, and got.
- Legacy-migration test: a version-less document loads with data intact and
  gains `schema_version` on the next write.
- Determinism test (existing, in `recipes_test.go`) retargeted at the new
  single-shot `marshalPrefs`.
- `gen-showcase` drift test must pass against regenerated templates.
- Fast suite: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./...`
