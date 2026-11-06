# Reference Audio Corpus (SP1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the scattered, partly-broken reference-audio setup with one committed catalogue, one resolver, and one `cmd/refs` CLI that fetches, verifies, and reports coverage for every instrument and template reference.

**Architecture:** A committed `catalogue.json` embedded in `internal/wavref` is the single source of truth for reference provenance. `internal/wavref` gains catalogue accessors plus `Resolve`/`Verify` on top of its existing `Dir`/`Path`. A new `cmd/refs` CLI reads that catalogue to download, convert (ffmpeg), measure (pitch/peak/RMS from the audio), hash, and report. Tier-II instrument clips (≤3 s, licence-clean, ~5 MB) are committed; Tier-I full-piece song recordings are fetched into `$BEATMO_WAV_REFS` and never committed.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), `internal/schema` envelope versioning, `internal/wave` + `internal/audio/fingerprint` for measurement, `internal/audio.DecodeWAVToPCM` for reading, `ffmpeg` for conversion.

## Global Constraints

- **NEVER run `git add`, `git commit`, or any git write operation.** The user manages all staging and commits. Where this plan would normally say "commit", it says "verify" instead. Leave everything uncommitted.
- **Bundled Go only:** `.tools/go/bin/go`. Never the system `go`.
- **Fast test path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/... ./cmd/refs/` — use explicit package paths, NOT a bare `./...`. This branch carries two pre-existing artifacts that are not regressions: a phantom duplicate package `.../src/go/internal/ui` that only fails under `./...`, and `cmd/synth-match`/`cmd/synth-analyze` failing `-tags test` with "function main is undeclared" because their `main.go` is `//go:build !test && !js` (a shape `cmd/refs` deliberately mirrors).
- **Build-tag pattern for `cmd/`:** the CLI shell (`main.go`) carries `//go:build !test && !js`; every testable core file carries `//go:build !js` so it compiles under `-tags test`. This mirrors `cmd/synth-analyze` (`main.go` vs `analyze.go`) exactly. Deviating breaks `./...` under the test tag.
- **Schema version:** every Beatmo JSON document carries `schema.Envelope` (`"schema_version": "0.0.1"` from `schema.Version`). Embed it anonymously so `encoding/json` flattens it to a top-level key.
- **Licence rule for committing audio:** `.gitignore` ignores `*.wav`; force-add only files verified licence-clean (Iowa MIS "unrestricted", VCSL "CC0-1.0", freesound "CC0-1.0") **and** ≤3 s. Never commit Tier-I song recordings.
- **A3 = 220 Hz** is the pitch origin. `semis_from_a3 = round(12 * log2(f0 / 220))`.
- **Never trust filename or upstream pitch metadata.** All pitch comes from `fingerprint.DetectF0` on the converted audio. This has been wrong upstream at least four times.

---

## File Structure

| File | Responsibility |
|---|---|
| `src/go/internal/wavref/catalogue.go` (create) | Catalogue types, `//go:embed catalogue.json`, parse + validate, lookup accessors |
| `src/go/internal/wavref/catalogue.json` (create) | The catalogue data |
| `src/go/internal/wavref/catalogue_test.go` (create) | Schema, uniqueness, partition, cross-reference tests |
| `src/go/internal/wavref/resolve.go` (create) | Module-root discovery, `CommittedDir`, `Resolve`, `PCMHash`, `Verify` |
| `src/go/internal/wavref/resolve_test.go` (create) | Resolve precedence + hash round-trip |
| `src/go/internal/wavref/wavref.go` (untouched) | Existing `Dir`/`Path` — 5 call sites depend on it |
| `src/go/cmd/refs/main.go` (create) | CLI shell + subcommand dispatch (`!test && !js`) |
| `src/go/cmd/refs/list.go` (create) | `list` + MANIFEST generation (`!js`) |
| `src/go/cmd/refs/verify.go` (create) | `verify` (`!js`) |
| `src/go/cmd/refs/doctor.go` (create) | `doctor` coverage matrix (`!js`) |
| `src/go/cmd/refs/fetch.go` (create) | download → convert → measure → hash (`!js`) |
| `src/go/cmd/refs/candidates.go` (create) | `candidates` + `choose` (`!js`) |
| `src/go/cmd/refs/*_test.go` (create) | per-command tests (`!js`) |
| `src/go/cmd/refs/testdata/doctor_golden.txt` (create) | pinned coverage report |
| `Makefile` (modify) | `fetch-refs`, `refs-doctor`, `refs-verify`; retire `fetch-synth-refs`/`fetch-song-refs` |
| `scripts/fetch_synth_refs.sh`, `scripts/fetch_song_refs.sh` (delete) | Superseded; the song one is dead (musopen 403) |

---

### Task 1: Catalogue types, data, and validation

**Files:**
- Create: `src/go/internal/wavref/catalogue.go`
- Create: `src/go/internal/wavref/catalogue.json`
- Test: `src/go/internal/wavref/catalogue_test.go`

**Interfaces:**
- Consumes: `internal/schema` (`Envelope`, `Version`, `Validate`).
- Produces: `wavref.Cat`, `wavref.Ref`, `wavref.Source`, `wavref.Convert`, `wavref.Measured`, `wavref.Alternative`, `wavref.Catalogue() (Cat, error)`, `(Cat) Ref(id) (Ref, bool)`, `(Cat) ForInstrument(inst) []Ref`, `(Cat) ForTemplate(stem) (Ref, bool)`, `(Cat) Validate() error`. Tasks 2–8 all depend on these exact names.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/wavref/catalogue_test.go`:

```go
package wavref

import "testing"

func TestCatalogue_ParsesAndValidates(t *testing.T) {
	c, err := Catalogue()
	if err != nil {
		t.Fatalf("Catalogue(): %v", err)
	}
	if len(c.Refs) == 0 {
		t.Fatal("catalogue has no refs")
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
}

func TestCatalogue_IDsUnique(t *testing.T) {
	c, err := Catalogue()
	if err != nil {
		t.Fatalf("Catalogue(): %v", err)
	}
	seen := map[string]bool{}
	for _, r := range c.Refs {
		if r.ID == "" {
			t.Fatal("ref with empty id")
		}
		if seen[r.ID] {
			t.Errorf("duplicate ref id %q", r.ID)
		}
		seen[r.ID] = true
	}
}

func TestCatalogue_LookupByIDAndInstrument(t *testing.T) {
	c, err := Catalogue()
	if err != nil {
		t.Fatalf("Catalogue(): %v", err)
	}
	r, ok := c.Ref("violin-A4")
	if !ok {
		t.Fatal(`Ref("violin-A4") not found`)
	}
	if r.Tier != TierInstrument {
		t.Errorf("violin-A4 tier = %q, want %q", r.Tier, TierInstrument)
	}
	if got := c.ForInstrument("violin"); len(got) == 0 {
		t.Error(`ForInstrument("violin") returned nothing`)
	}
}

// A real entry must carry a file and a source; a status entry must carry
// neither. This is the invariant that keeps "covered" honest.
func TestCatalogue_StatusEntriesCarryNoFile(t *testing.T) {
	c, _ := Catalogue()
	for _, r := range c.Refs {
		switch r.Status {
		case "":
			if r.File == "" || r.Source == nil {
				t.Errorf("%s: real ref must have file and source", r.ID)
			}
		case StatusSynthetic, StatusNeedsSource:
			if r.File != "" || r.PCMSHA256 != "" {
				t.Errorf("%s: status ref must not carry file/hash", r.ID)
			}
			if r.Note == "" {
				t.Errorf("%s: status ref must explain itself in note", r.ID)
			}
		default:
			t.Errorf("%s: unknown status %q", r.ID, r.Status)
		}
	}
}

func TestCatalogue_CommittedIsInstrumentTierOnly(t *testing.T) {
	c, _ := Catalogue()
	for _, r := range c.Refs {
		if r.Committed && r.Tier != TierInstrument {
			t.Errorf("%s: only instrument-tier refs may be committed (song refs are too large)", r.ID)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wavref/ -run TestCatalogue -v`
Expected: FAIL — `undefined: Catalogue`, `undefined: TierInstrument`, etc.

- [ ] **Step 3: Write the catalogue types**

Create `src/go/internal/wavref/catalogue.go`:

```go
package wavref

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// catalogueFormat names this document in schema.Validate errors.
const catalogueFormat = "beatmo.wavref-catalogue"

// Tier values.
const (
	TierInstrument = "instrument" // one isolated note/phrase; committed when licence-clean and short
	TierSong       = "song"       // full public-domain piece; never committed
)

// Status values. An empty status means the ref has real audio.
const (
	StatusSynthetic   = "synthetic"    // no acoustic counterpart exists; counts as covered
	StatusNeedsSource = "needs-source" // acoustic, but no licence-clean isolated recording found
)

//go:embed catalogue.json
var catalogueJSON []byte

// Cat is the reference catalogue.
type Cat struct {
	schema.Envelope
	Refs []Ref `json:"refs"`
}

// Ref is one reference entry.
type Ref struct {
	ID   string `json:"id"`
	Tier string `json:"tier"`

	// Instruments lists every instrument id this reference is ground truth
	// for. One recording legitimately serves an instrument and its
	// character variants (violin and violin-ensemble share a sample).
	Instruments []string `json:"instruments,omitempty"`
	// Template is the template stem a song-tier reference belongs to.
	Template string `json:"template,omitempty"`

	// File is the bare filename inside the corpus directory. Empty when
	// Status is set.
	File string `json:"file,omitempty"`
	// Committed reports that File is force-added to git.
	Committed bool `json:"committed,omitempty"`

	Status string `json:"status,omitempty"`
	Note   string `json:"note,omitempty"`

	Source  *Source  `json:"source,omitempty"`
	Convert *Convert `json:"convert,omitempty"`

	// PCMSHA256 hashes the DECODED PCM (samples + rate), not the container
	// bytes. Container metadata varies between ffmpeg builds; the audio
	// does not, so this is what makes verification meaningful.
	PCMSHA256 string `json:"pcm_sha256,omitempty"`

	Measured     *Measured     `json:"measured,omitempty"`
	Alternatives []Alternative `json:"alternatives,omitempty"`
}

// Source records where the audio came from and under what licence.
type Source struct {
	Provider string `json:"provider"` // uiowa-mis | vcsl | freesound | archive-org | imslp | local
	URL      string `json:"url"`
	License  string `json:"license"` // unrestricted | CC0-1.0 | PD
	Credit   string `json:"credit,omitempty"`
}

// Convert is the deterministic conversion applied after download.
type Convert struct {
	Channels int     `json:"channels"`
	Rate     int     `json:"rate"`
	Bits     int     `json:"bits"`
	StartSec float64 `json:"start_sec,omitempty"`
	MaxSec   float64 `json:"max_sec"`
}

// Measured holds values detected FROM THE AUDIO, never parsed from a
// filename or upstream metadata (which has been wrong repeatedly).
type Measured struct {
	F0Hz        float64 `json:"f0_hz"`
	SemisFromA3 int     `json:"semis_from_a3"`
	Peak        float64 `json:"peak"`
	RMS         float64 `json:"rms"`
	DurationSec float64 `json:"duration_sec"`
}

// Alternative is a candidate reference that lost the ear comparison.
type Alternative struct {
	URL    string `json:"url"`
	Reason string `json:"reason"`
}

var (
	catOnce sync.Once
	catVal  Cat
	catErr  error
)

// Catalogue parses the embedded catalogue once and returns it.
func Catalogue() (Cat, error) {
	catOnce.Do(func() {
		if err := json.Unmarshal(catalogueJSON, &catVal); err != nil {
			catErr = fmt.Errorf("wavref: parse catalogue.json: %w", err)
			return
		}
		catErr = catVal.Envelope.Validate(catalogueFormat)
	})
	return catVal, catErr
}

// Ref returns the entry with the given id.
func (c Cat) Ref(id string) (Ref, bool) {
	for _, r := range c.Refs {
		if r.ID == id {
			return r, true
		}
	}
	return Ref{}, false
}

// ForInstrument returns every entry that is ground truth for inst.
func (c Cat) ForInstrument(inst string) []Ref {
	var out []Ref
	for _, r := range c.Refs {
		for _, i := range r.Instruments {
			if i == inst {
				out = append(out, r)
				break
			}
		}
	}
	return out
}

// ForTemplate returns the song reference for a template stem.
func (c Cat) ForTemplate(stem string) (Ref, bool) {
	for _, r := range c.Refs {
		if r.Tier == TierSong && r.Template == stem {
			return r, true
		}
	}
	return Ref{}, false
}

// Validate checks the structural invariants the tests and CLI rely on.
func (c Cat) Validate() error {
	seen := map[string]bool{}
	for _, r := range c.Refs {
		if r.ID == "" {
			return fmt.Errorf("wavref: ref with empty id")
		}
		if seen[r.ID] {
			return fmt.Errorf("wavref: duplicate ref id %q", r.ID)
		}
		seen[r.ID] = true

		switch r.Tier {
		case TierInstrument:
			if len(r.Instruments) == 0 {
				return fmt.Errorf("wavref: %s: instrument-tier ref lists no instruments", r.ID)
			}
		case TierSong:
			if r.Template == "" {
				return fmt.Errorf("wavref: %s: song-tier ref names no template", r.ID)
			}
		default:
			return fmt.Errorf("wavref: %s: unknown tier %q", r.ID, r.Tier)
		}

		switch r.Status {
		case "":
			if r.File == "" {
				return fmt.Errorf("wavref: %s: real ref has no file", r.ID)
			}
			if r.Source == nil {
				return fmt.Errorf("wavref: %s: real ref has no source", r.ID)
			}
		case StatusSynthetic, StatusNeedsSource:
			if r.File != "" || r.PCMSHA256 != "" {
				return fmt.Errorf("wavref: %s: status ref must not carry file or hash", r.ID)
			}
			if r.Note == "" {
				return fmt.Errorf("wavref: %s: status ref must explain itself", r.ID)
			}
		default:
			return fmt.Errorf("wavref: %s: unknown status %q", r.ID, r.Status)
		}

		if r.Committed && r.Tier != TierInstrument {
			return fmt.Errorf("wavref: %s: only instrument-tier refs may be committed", r.ID)
		}
	}
	return nil
}
```

- [ ] **Step 4: Seed the catalogue with the five already-committed Iowa MIS entries**

Create `src/go/internal/wavref/catalogue.json`. Source URLs and the instrument mapping come from `src/go/internal/audio/testdata/refs/MANIFEST.md`; `pcm_sha256` and `measured` are left empty here and are filled in by `refs fetch` in Task 5 (they are PCM hashes, not the file hashes the old manifest recorded, so they cannot be copied across).

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
        "url": "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Strings/Violin/Violin.arco.ff.sulA.A4.stereo.aif",
        "license": "unrestricted",
        "credit": "University of Iowa Musical Instrument Samples"
      },
      "convert": { "channels": 1, "rate": 44100, "bits": 16, "max_sec": 2.0 }
    },
    {
      "id": "cello-A3",
      "tier": "instrument",
      "instruments": ["cello", "cello-warm"],
      "file": "cello_A3.wav",
      "committed": true,
      "source": {
        "provider": "uiowa-mis",
        "url": "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Strings/Cello/Cello.arco.ff.sulA.A3.stereo.aif",
        "license": "unrestricted",
        "credit": "University of Iowa Musical Instrument Samples"
      },
      "convert": { "channels": 1, "rate": 44100, "bits": 16, "max_sec": 2.0 }
    },
    {
      "id": "flute-A4",
      "tier": "instrument",
      "instruments": ["flute"],
      "file": "flute_A4.wav",
      "committed": true,
      "source": {
        "provider": "uiowa-mis",
        "url": "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Woodwinds/Flute/Flute.nonvib.ff.A4.stereo.aif",
        "license": "unrestricted",
        "credit": "University of Iowa Musical Instrument Samples"
      },
      "convert": { "channels": 1, "rate": 44100, "bits": 16, "max_sec": 2.0 }
    },
    {
      "id": "oboe-A4",
      "tier": "instrument",
      "instruments": ["oboe-full"],
      "file": "oboe_A4.wav",
      "committed": true,
      "source": {
        "provider": "uiowa-mis",
        "url": "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Woodwinds/Oboe/Oboe.ff.A4.stereo.aif",
        "license": "unrestricted",
        "credit": "University of Iowa Musical Instrument Samples"
      },
      "convert": { "channels": 1, "rate": 44100, "bits": 16, "max_sec": 2.0 }
    },
    {
      "id": "trumpet-A4",
      "tier": "instrument",
      "instruments": ["trumpet", "trumpet-mellow"],
      "file": "trumpet_A4.wav",
      "committed": true,
      "source": {
        "provider": "uiowa-mis",
        "url": "https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/Brass/BbTrumpet/Trumpet.novib.ff.A4.stereo.aif",
        "license": "unrestricted",
        "credit": "University of Iowa Musical Instrument Samples"
      },
      "convert": { "channels": 1, "rate": 44100, "bits": 16, "max_sec": 2.0 }
    }
  ]
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wavref/ -v`
Expected: PASS — all five tests green.

- [ ] **Step 6: Verify (do not commit)**

Run: `cd src/go && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/wavref/`
Expected: no output. Leave the files uncommitted.

---

### Task 2: Resolve, PCM hashing, and Verify

**Files:**
- Create: `src/go/internal/wavref/resolve.go`
- Test: `src/go/internal/wavref/resolve_test.go`

**Interfaces:**
- Consumes: `wavref.Cat`/`Ref` (Task 1), existing `wavref.Dir()`/`Path()`. Stdlib only otherwise — this package must NOT import `internal/audio` (import cycle, see the correction note below).
- Produces: `wavref.ModuleRoot() (string, bool)`, `wavref.CommittedDir() (string, bool)`, `wavref.Resolve(id string) (string, bool)`, `wavref.PCMHash(samples []float32, rate int) string`, `wavref.VerifyHash(id, gotHash string) error`. Tasks 3–8 depend on these.

> **CORRECTION APPLIED 2026-08-19 (import cycle).** An earlier draft had
> `wavref` import `internal/audio` for `DecodeWAVToPCM`, with
> `PCMHash(path string)` and `Verify(id string)`. That closes an import
> cycle: `internal/audio/wavref_ref_test.go` is `package audio` and imports
> `wavref`, so `audio [test] → wavref → audio`. It is invisible under
> `-tags test` (that file is `//go:build !test && !js`) but breaks the real
> build. The dependency is therefore **inverted**: `wavref` imports only
> stdlib and `internal/schema`; **the caller decodes**. `PCMHash` is a pure
> function over decoded samples, and `VerifyHash` is a pure comparison.
> Consequently `resolve.go` and `resolve_test.go` carry **no build tag**.
> `cmd/refs` owns decoding via the `hashFile` helper introduced in Task 3.

**Why a module-root walk:** the committed corpus lives at `src/go/internal/audio/testdata/refs/`, but callers run with different working directories (`go test` uses the package dir; `go run ./cmd/refs` uses `src/go`). Walking up to `go.mod` finds the module root from any of them. Embedding the corpus instead is not an option — it would put ~5 MB of audio into every binary, including the WASM bundle.

- [ ] **Step 1: Write the failing test**

Create `src/go/internal/wavref/resolve_test.go` (no build tag, matching resolve.go):

```go
package wavref

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleRoot_FindsGoMod(t *testing.T) {
	root, ok := ModuleRoot()
	if !ok {
		t.Fatal("ModuleRoot() not found — the go.mod walk failed")
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Errorf("ModuleRoot() = %q but it holds no go.mod: %v", root, err)
	}
}

func TestCommittedDir_FoundFromPackageDir(t *testing.T) {
	d, ok := CommittedDir()
	if !ok {
		t.Fatal("CommittedDir() not found — module root walk failed")
	}
	if filepath.Base(d) != "refs" {
		t.Errorf("CommittedDir() = %q, want a path ending in /refs", d)
	}
	if _, err := os.Stat(d); err != nil {
		t.Errorf("CommittedDir() %q does not exist: %v", d, err)
	}
}

// $BEATMO_WAV_REFS must win over the committed directory, so a local
// re-fetch can override a committed clip without editing the tree.
func TestResolve_EnvDirTakesPrecedence(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "violin_A4.wav"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(EnvVar, tmp)

	got, ok := Resolve("violin-A4")
	if !ok {
		t.Fatal(`Resolve("violin-A4") not found`)
	}
	if want := filepath.Join(tmp, "violin_A4.wav"); got != want {
		t.Errorf("Resolve() = %q, want %q", got, want)
	}
}

func TestResolve_FallsBackToCommittedDir(t *testing.T) {
	t.Setenv(EnvVar, t.TempDir()) // empty override dir
	got, ok := Resolve("violin-A4")
	if !ok {
		t.Skip("committed violin_A4.wav absent from this checkout")
	}
	if filepath.Base(got) != "violin_A4.wav" {
		t.Errorf("Resolve() = %q, want .../violin_A4.wav", got)
	}
}

func TestResolve_UnknownIDNotOK(t *testing.T) {
	if _, ok := Resolve("no-such-ref"); ok {
		t.Error("Resolve() returned ok for an unknown id")
	}
}

// PCMHash is a pure function over decoded samples, so it is tested directly
// on synthetic input — no file, no decoder, no import of internal/audio.
func TestPCMHash_DeterministicAndSensitive(t *testing.T) {
	base := []float32{0, 0.5, -0.5, 0.25}

	h1 := PCMHash(base, 44100)
	if len(h1) != 64 {
		t.Errorf("PCMHash = %q, want 64 hex chars", h1)
	}
	if h2 := PCMHash(base, 44100); h1 != h2 {
		t.Errorf("PCMHash not deterministic: %q vs %q", h1, h2)
	}
	// Content sensitivity: one changed sample must change the hash.
	if got := PCMHash([]float32{0, 0.5, -0.5, 0.26}, 44100); got == h1 {
		t.Error("PCMHash ignored a changed sample")
	}
	// Rate sensitivity: same samples at another rate are different audio.
	if got := PCMHash(base, 48000); got == h1 {
		t.Error("PCMHash ignored the sample rate")
	}
	// Length sensitivity: a trailing zero is still a longer signal.
	if got := PCMHash(append(append([]float32(nil), base...), 0), 44100); got == h1 {
		t.Error("PCMHash ignored an appended sample")
	}
}

func TestVerifyHash_UnknownIDErrors(t *testing.T) {
	if err := VerifyHash("no-such-ref", "abc"); err == nil {
		t.Error("VerifyHash accepted an unknown id")
	}
}

// A ref that has never been fetched has no recorded hash; verifying it is
// vacuously fine, because absence is a fetch concern not a corruption one.
func TestVerifyHash_NoRecordedHashIsVacuouslyOK(t *testing.T) {
	c, err := Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	var id string
	for _, r := range c.Refs {
		if r.Status == "" && r.PCMSHA256 == "" {
			id = r.ID
			break
		}
	}
	if id == "" {
		t.Skip("every ref already carries a recorded hash")
	}
	if err := VerifyHash(id, "anything"); err != nil {
		t.Errorf("VerifyHash(%s) = %v, want nil for an unfetched ref", id, err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wavref/ -run 'TestModuleRoot|TestCommittedDir|TestResolve|TestPCMHash' -v`
Expected: FAIL — `undefined: ModuleRoot`, `undefined: CommittedDir`, `undefined: Resolve`, `undefined: PCMHash`.

- [ ] **Step 3: Write the implementation**

Create `src/go/internal/wavref/resolve.go`. **No build tag** — with the audio import removed there is no cgo dependency, so this compiles everywhere including js/wasm, exactly like `catalogue.go`.

```go
package wavref

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

// committedRel is the corpus directory relative to the Go module root.
const committedRel = "internal/audio/testdata/refs"

// ModuleRoot walks up from the working directory to the directory holding
// go.mod. Callers run from different cwds (go test uses the package dir,
// go run uses src/go), so an absolute anchor is required. Exported because
// cmd/refs needs it to locate the templates dir and catalogue.json without
// counting filepath.Dir calls.
func ModuleRoot() (string, bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// CommittedDir returns the in-tree corpus directory holding the committed
// instrument clips.
func CommittedDir() (string, bool) {
	root, ok := ModuleRoot()
	if !ok {
		return "", false
	}
	d := filepath.Join(root, committedRel)
	if _, err := os.Stat(d); err != nil {
		return "", false
	}
	return d, true
}

// Resolve locates the audio file for a catalogue id. $BEATMO_WAV_REFS wins
// over the committed directory so a local re-fetch can override in place.
// ok=false when the id is unknown, carries a status instead of audio, or
// the file is absent — callers skip rather than fail.
func Resolve(id string) (string, bool) {
	c, err := Catalogue()
	if err != nil {
		return "", false
	}
	r, ok := c.Ref(id)
	if !ok || r.File == "" {
		return "", false
	}
	if d, ok := Dir(); ok {
		p := filepath.Join(d, r.File)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	if d, ok := CommittedDir(); ok {
		p := filepath.Join(d, r.File)
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// PCMHash hashes DECODED audio — sample rate, sample count, then every sample
// as a big-endian int16 — rather than the container bytes. ffmpeg builds
// differ in the metadata chunks they emit; the audio does not, so hashing PCM
// is what makes verification mean "same sound".
//
// The caller decodes. This package deliberately does not import
// internal/audio: internal/audio's own test files are `package audio` and
// import wavref, so a wavref->audio edge would close an import cycle.
func PCMHash(samples []float32, rate int) string {
	h := sha256.New()
	var scratch [8]byte
	binary.BigEndian.PutUint32(scratch[:4], uint32(rate))
	binary.BigEndian.PutUint32(scratch[4:8], uint32(len(samples)))
	h.Write(scratch[:])
	var s2 [2]byte
	for _, f := range samples {
		v := int16(math.Round(math.Max(-32768, math.Min(32767, float64(f)*32767))))
		binary.BigEndian.PutUint16(s2[:], uint16(v))
		h.Write(s2[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// VerifyHash compares a caller-computed PCM hash against the catalogue's
// recorded value. A ref with no recorded hash verifies vacuously — an
// unfetched ref is a fetch concern, not a corruption concern.
func VerifyHash(id, gotHash string) error {
	c, err := Catalogue()
	if err != nil {
		return err
	}
	r, ok := c.Ref(id)
	if !ok {
		return fmt.Errorf("wavref: unknown ref %q", id)
	}
	if r.PCMSHA256 == "" {
		return nil
	}
	if gotHash != r.PCMSHA256 {
		return fmt.Errorf("wavref: %s: pcm hash mismatch\n  want %s\n  got  %s", id, r.PCMSHA256, gotHash)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wavref/ -v`
Expected: PASS. `TestResolve_FallsBackToCommittedDir` and `TestPCMHash_*` may SKIP if the committed clips are absent from the checkout; every other test must pass.

- [ ] **Step 5: Confirm the existing five call sites still build**

Run BOTH commands. The second is the one that matters:

```bash
cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/... 
cd src/go && ../../.tools/go/bin/go vet ./internal/wavref/ ./internal/audio/
```

Expected: no output from either.

**The second command has NO `-tags test`, deliberately.** `internal/audio/wavref_ref_test.go` carries `//go:build !test && !js`, so it is EXCLUDED from the test-tagged build — which means a `wavref -> internal/audio` import cycle is completely invisible to `-tags test` and only breaks the real build and `make test-real`. Vetting untagged is the only check that proves the cycle is absent. `wavref.Dir`/`Path` were not modified, so `cmd/synth-match`, `cmd/synth-analyze`, `internal/audio`, and `internal/audio/fingerprint` are unaffected.

- [ ] **Step 6: Verify (do not commit)**

---

### Task 3: `cmd/refs` skeleton with `list` and `verify`

**Files:**
- Create: `src/go/cmd/refs/main.go`
- Create: `src/go/cmd/refs/list.go`
- Create: `src/go/cmd/refs/verify.go`
- Test: `src/go/cmd/refs/list_test.go`

**Interfaces:**
- Consumes: `wavref.Catalogue`, `wavref.Resolve`, `wavref.Verify` (Tasks 1–2).
- Produces: `runList(w io.Writer, tier string) error`, `runVerify(w io.Writer, id string) error`, `manifestMarkdown(c wavref.Cat) string`, `hashFile(path string) (string, error)`. Tasks 4–7 add sibling `run*` functions and register them in `main.go`'s dispatch; Task 5 reuses `hashFile`.

**`hashFile` is where decoding lives.** `internal/wavref` deliberately does not import `internal/audio` (import cycle — see Task 2's correction note), so `cmd/refs` decodes and `wavref` only hashes and compares:

```go
// hashFile decodes a WAV and returns its PCM hash. Decoding lives here, not
// in internal/wavref, because wavref importing internal/audio would close an
// import cycle with that package's own tests.
func hashFile(path string) (string, error) {
	samples, rate, err := audio.DecodeWAVToPCM(path)
	if err != nil {
		return "", fmt.Errorf("decode %s: %w", path, err)
	}
	return wavref.PCMHash(samples, rate), nil
}
```

Put it in `verify.go` (add `"github.com/ingyamilmolinar/beatmo/internal/audio"` to that file's imports). Because it imports `internal/audio`, `verify.go` carries `//go:build !js` like the rest of `cmd/refs`.

- [ ] **Step 1: Write the failing test**

Create `src/go/cmd/refs/list_test.go`:

```go
//go:build !js

package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func TestRunList_IncludesEveryRef(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue(): %v", err)
	}
	var buf bytes.Buffer
	if err := runList(&buf, ""); err != nil {
		t.Fatalf("runList: %v", err)
	}
	for _, r := range c.Refs {
		if !strings.Contains(buf.String(), r.ID) {
			t.Errorf("list output missing ref %q", r.ID)
		}
	}
}

func TestRunList_TierFilter(t *testing.T) {
	var buf bytes.Buffer
	if err := runList(&buf, wavref.TierSong); err != nil {
		t.Fatalf("runList: %v", err)
	}
	c, _ := wavref.Catalogue()
	for _, r := range c.Refs {
		if r.Tier != wavref.TierInstrument {
			continue
		}
		if strings.Contains(buf.String(), "\t"+r.ID+"\t") {
			t.Errorf("song-tier listing leaked instrument ref %q", r.ID)
		}
	}
}

// Provenance must have exactly one source: the manifest is generated, not
// hand-maintained, so it cannot drift from the catalogue.
func TestManifestMarkdown_CarriesLicenceAndURL(t *testing.T) {
	c, _ := wavref.Catalogue()
	md := manifestMarkdown(c)
	for _, r := range c.Refs {
		if r.Source == nil {
			continue
		}
		if !strings.Contains(md, r.Source.URL) {
			t.Errorf("manifest missing source URL for %q", r.ID)
		}
		if !strings.Contains(md, r.Source.License) {
			t.Errorf("manifest missing licence for %q", r.ID)
		}
	}
}

func TestRunVerify_UnknownIDErrors(t *testing.T) {
	var buf bytes.Buffer
	if err := runVerify(&buf, "no-such-ref"); err == nil {
		t.Error("runVerify accepted an unknown id")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -v`
Expected: FAIL — `undefined: runList`, `undefined: manifestMarkdown`, `undefined: runVerify`.

- [ ] **Step 3: Write `list.go`**

Create `src/go/cmd/refs/list.go`:

```go
//go:build !js

package main

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// presence describes whether a ref's audio is resolvable right now.
func presence(r wavref.Ref) string {
	if r.Status != "" {
		return r.Status
	}
	if _, ok := wavref.Resolve(r.ID); ok {
		return "present"
	}
	return "missing"
}

func refTargets(r wavref.Ref) string {
	if r.Tier == wavref.TierSong {
		return r.Template
	}
	return strings.Join(r.Instruments, ",")
}

// runList writes a table of catalogue entries. tier == "" lists all.
func runList(w io.Writer, tier string) error {
	c, err := wavref.Catalogue()
	if err != nil {
		return err
	}
	refs := append([]wavref.Ref(nil), c.Refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tTIER\tTARGETS\tLICENCE\tSTATE")
	for _, r := range refs {
		if tier != "" && r.Tier != tier {
			continue
		}
		lic := "-"
		if r.Source != nil {
			lic = r.Source.License
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", r.ID, r.Tier, refTargets(r), lic, presence(r))
	}
	return tw.Flush()
}

// manifestMarkdown renders the provenance table that used to be
// hand-maintained in testdata/refs/MANIFEST.md. Generating it from the
// catalogue keeps provenance single-sourced.
func manifestMarkdown(c wavref.Cat) string {
	var b strings.Builder
	b.WriteString("# Reference WAVs — generated by `go run ./cmd/refs list -manifest`\n\n")
	b.WriteString("DO NOT EDIT. Provenance lives in `internal/wavref/catalogue.json`.\n\n")
	b.WriteString("| id | tier | targets | licence | credit | source | pcm sha256 |\n")
	b.WriteString("|---|---|---|---|---|---|---|\n")

	refs := append([]wavref.Ref(nil), c.Refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	for _, r := range refs {
		if r.Source == nil {
			fmt.Fprintf(&b, "| %s | %s | %s | — | — | _%s_ | — |\n",
				r.ID, r.Tier, refTargets(r), r.Status)
			continue
		}
		hash := r.PCMSHA256
		if hash == "" {
			hash = "—"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | `%s` |\n",
			r.ID, r.Tier, refTargets(r), r.Source.License, r.Source.Credit, r.Source.URL, hash)
	}
	return b.String()
}
```

- [ ] **Step 4: Write `verify.go`**

Create `src/go/cmd/refs/verify.go`:

```go
//go:build !js

package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// runVerify hashes local audio against the catalogue. id == "" verifies
// every ref. A missing file is reported, not an error: absence is a fetch
// concern. A hash mismatch IS an error — the audio changed under us.
func runVerify(w io.Writer, id string) error {
	c, err := wavref.Catalogue()
	if err != nil {
		return err
	}
	var refs []wavref.Ref
	if id != "" {
		r, ok := c.Ref(id)
		if !ok {
			return fmt.Errorf("refs verify: unknown ref %q", id)
		}
		refs = []wavref.Ref{r}
	} else {
		refs = append(refs, c.Refs...)
		sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	}

	var bad, missing, ok int
	for _, r := range refs {
		if r.Status != "" {
			continue
		}
		p, found := wavref.Resolve(r.ID)
		if !found {
			fmt.Fprintf(w, "missing  %s (%s)\n", r.ID, r.File)
			missing++
			continue
		}
		got, err := hashFile(p)
		if err != nil {
			fmt.Fprintf(w, "MISMATCH %s\n  %v\n", r.ID, err)
			bad++
			continue
		}
		if err := wavref.VerifyHash(r.ID, got); err != nil {
			fmt.Fprintf(w, "MISMATCH %s\n  %v\n  file %s\n", r.ID, err, p)
			bad++
			continue
		}
		fmt.Fprintf(w, "ok       %s\n", r.ID)
		ok++
	}
	fmt.Fprintf(w, "\n%d ok, %d missing, %d mismatched\n", ok, missing, bad)
	if bad > 0 {
		return fmt.Errorf("refs verify: %d reference(s) failed the hash check", bad)
	}
	return nil
}
```

- [ ] **Step 5: Write the CLI shell**

Create `src/go/cmd/refs/main.go`:

```go
//go:build !test && !js

// refs: manage the Beatmo reference-audio corpus.
//
// Provenance lives in internal/wavref/catalogue.json; audio lives either in
// the committed corpus (instrument clips) or in $BEATMO_WAV_REFS (song
// recordings, too large to commit).
//
// Usage:
//
//	refs list       [-tier instrument|song] [-manifest]
//	refs verify     [-id <ref-id>]
//	refs doctor
//	refs fetch      [-id <ref-id>] [-instrument <inst>] [-tier <t>] [-only-missing] [-force]
//	refs candidates -instrument <inst> [-n 3]
//	refs choose     -instrument <inst> -id <ref-id>
package main

import (
	"fmt"
	"os"
)

func usage() {
	fmt.Fprint(os.Stderr, `refs: manage the Beatmo reference-audio corpus

  refs list       [-tier instrument|song] [-manifest]
  refs verify     [-id <ref-id>]
  refs doctor
  refs fetch      [-id <ref-id>] [-instrument <inst>] [-tier <t>] [-only-missing] [-force]
  refs candidates -instrument <inst> [-n 3]
  refs choose     -instrument <inst> -id <ref-id>
`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "list":
		err = cmdList(os.Args[2:])
	case "verify":
		err = cmdVerify(os.Args[2:])
	case "doctor":
		err = cmdDoctor(os.Args[2:])
	case "fetch":
		err = cmdFetch(os.Args[2:])
	case "candidates":
		err = cmdCandidates(os.Args[2:])
	case "choose":
		err = cmdChoose(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "refs: %v\n", err)
		os.Exit(1)
	}
}
```

Add the `list`/`verify` flag parsers to `list.go` and `verify.go` respectively (they carry `//go:build !js` so they stay testable):

In `list.go`, append:

```go
func cmdList(args []string) error {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	tier := fs.String("tier", "", "filter by tier: instrument|song")
	manifest := fs.Bool("manifest", false, "write the generated MANIFEST.md into the committed corpus dir")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *manifest {
		c, err := wavref.Catalogue()
		if err != nil {
			return err
		}
		dir, ok := wavref.CommittedDir()
		if !ok {
			return fmt.Errorf("refs list -manifest: committed corpus dir not found")
		}
		path := filepath.Join(dir, "MANIFEST.md")
		if err := os.WriteFile(path, []byte(manifestMarkdown(c)), 0o644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stdout, "wrote %s\n", path)
		return nil
	}
	return runList(os.Stdout, *tier)
}
```

(add `"flag"`, `"os"`, `"path/filepath"` to `list.go`'s imports)

In `verify.go`, append:

```go
func cmdVerify(args []string) error {
	fs := flag.NewFlagSet("verify", flag.ExitOnError)
	id := fs.String("id", "", "verify a single ref id (default: all)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runVerify(os.Stdout, *id)
}
```

(add `"flag"`, `"os"` to `verify.go`'s imports)

- [ ] **Step 6: Add temporary stubs so the shell compiles**

`main.go` references `cmdDoctor`, `cmdFetch`, `cmdCandidates`, `cmdChoose`, which land in Tasks 4–6. Create `src/go/cmd/refs/stubs.go` so the package builds now; each stub is deleted by the task that implements it.

```go
//go:build !js

package main

import "errors"

// Deleted by Task 4.
func cmdDoctor([]string) error { return errors.New("doctor: not implemented yet (Task 4)") }

// Deleted by Task 5.
func cmdFetch([]string) error { return errors.New("fetch: not implemented yet (Task 5)") }

// Deleted by Task 6.
func cmdCandidates([]string) error { return errors.New("candidates: not implemented yet (Task 6)") }
func cmdChoose([]string) error     { return errors.New("choose: not implemented yet (Task 6)") }
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -v`
Expected: PASS — all four tests green.

- [ ] **Step 8: Verify the real binary builds and runs**

Run: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs list`
Expected: a five-row table (violin-A4, cello-A3, flute-A4, oboe-A4, trumpet-A4) with STATE `present` for each clip already in the tree.

- [ ] **Step 9: Verify (do not commit)**

---

### Task 4: `refs doctor` — coverage matrix

**Files:**
- Create: `src/go/cmd/refs/doctor.go`
- Delete: the `cmdDoctor` stub from `src/go/cmd/refs/stubs.go`
- Test: `src/go/cmd/refs/doctor_test.go`
- Create: `src/go/cmd/refs/testdata/doctor_golden.txt`

**Interfaces:**
- Consumes: `wavref.Catalogue` (Task 1), `audio.BuiltinInstrumentIDs` (existing, 133 ids, generated from `engine_instruments.go`).
- Produces: `baseInstrumentID(id string, known map[string]bool) string`, `coverage() ([]covRow, error)`, `runDoctor(w io.Writer) error`.

**Why base-id normalisation, and the trap in it.** `audio.BuiltinInstrumentIDs` contains per-row duplicate instances — `snare-1`, `snare-2`, `kick-1`, `kick-2`, `hihat-1`, `tom-1`, `clap-1`, `cowbell-1`, `sub-bass-1`, `fm-pluck-1`. These are the same instrument as their base, so requiring a catalogue entry for each would fill the report with noise.

The obvious rule — strip any trailing `-<digits>` when the base is a known instrument — is **wrong**. Auditing the roster, exactly two ids end in digits without being duplicates: `kick-808` and `bass-808`. `kick` *is* a known instrument, so that rule would silently merge `kick-808` into `kick` and lose a distinct instrument's coverage.

The correct rule is therefore: strip a trailing **single digit 1–9** only, and only when the base is itself a known instrument. Every real duplicate uses `-1` or `-2`; `-808` is three digits and survives. Both conditions are required — dropping either reintroduces the bug.

- [ ] **Step 1: Write the failing test**

Create `src/go/cmd/refs/doctor_test.go`:

```go
//go:build !js

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestBaseInstrumentID_StripsOnlyRealDuplicates(t *testing.T) {
	// "kick" IS a real instrument in the roster, which is exactly what makes
	// kick-808 dangerous: a naive "strip trailing digits when the base is
	// known" rule would merge kick-808 into kick and lose its coverage.
	known := map[string]bool{
		"snare": true, "snare-1": true, "tom": true, "tom-1": true,
		"kick": true, "kick-1": true, "kick-808": true,
		"bass-808": true, "sub-bass": true, "sub-bass-1": true,
	}
	cases := []struct{ in, want string }{
		{"snare", "snare"},
		{"snare-1", "snare"},
		{"tom-1", "tom"},
		{"kick-1", "kick"},
		{"sub-bass-1", "sub-bass"},
		// Multi-digit suffixes are part of the NAME, not a duplicate marker.
		{"kick-808", "kick-808"},
		{"bass-808", "bass-808"},
	}
	for _, c := range cases {
		if got := baseInstrumentID(c.in, known); got != c.want {
			t.Errorf("baseInstrumentID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Guard the rule against the real roster: no id ending in multiple digits
// may ever be collapsed, and every -1/-2 duplicate must be.
func TestBaseInstrumentID_RealRosterSanity(t *testing.T) {
	known := map[string]bool{}
	for _, id := range audio.BuiltinInstrumentIDs {
		known[id] = true
	}
	if got := baseInstrumentID("kick-808", known); got != "kick-808" {
		t.Errorf("kick-808 collapsed to %q — the 808 instruments must survive", got)
	}
	if got := baseInstrumentID("bass-808", known); got != "bass-808" {
		t.Errorf("bass-808 collapsed to %q", got)
	}
	if got := baseInstrumentID("snare-1", known); got != "snare" {
		t.Errorf("snare-1 = %q, want snare", got)
	}
}

// Every base instrument must be classified. An unclassified instrument is a
// gap in the catalogue, and the golden below is what makes adding a new
// instrument without a reference decision fail the build.
func TestCoverage_ClassifiesEveryBaseInstrument(t *testing.T) {
	rows, err := coverage()
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("coverage returned no rows")
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.Instrument] = true
		switch r.State {
		case "covered", "synthetic", "needs-source", "unclassified":
		default:
			t.Errorf("%s: unexpected state %q", r.Instrument, r.State)
		}
	}
	known := map[string]bool{}
	for _, id := range audio.BuiltinInstrumentIDs {
		known[id] = true
	}
	for _, id := range audio.BuiltinInstrumentIDs {
		b := baseInstrumentID(id, known)
		if !seen[b] {
			t.Errorf("instrument %q (base %q) missing from coverage report", id, b)
		}
	}
}

func TestRunDoctor_MatchesGolden(t *testing.T) {
	var buf bytes.Buffer
	if err := runDoctor(&buf); err != nil {
		t.Fatalf("runDoctor: %v", err)
	}
	want, err := os.ReadFile("testdata/doctor_golden.txt")
	if err != nil {
		t.Fatalf("read golden: %v (regenerate with: go run ./cmd/refs doctor > cmd/refs/testdata/doctor_golden.txt)", err)
	}
	if strings.TrimSpace(buf.String()) != strings.TrimSpace(string(want)) {
		t.Errorf("doctor output drifted from golden.\n--- got ---\n%s\n--- want ---\n%s", buf.String(), want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -run 'TestBase|TestCoverage|TestRunDoctor' -v`
Expected: FAIL — `undefined: baseInstrumentID`, `undefined: coverage`, `undefined: runDoctor`.

- [ ] **Step 3: Write the implementation**

Create `src/go/cmd/refs/doctor.go`:

```go
//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// dupSuffix matches the trailing "-N" that marks a per-row duplicate
// instance of an instrument (snare-1, tom-2). It is deliberately restricted
// to a SINGLE digit 1-9: every real duplicate uses -1 or -2, while
// kick-808 and bass-808 are distinct instruments whose names merely end in
// digits. A `\d+` here would silently merge kick-808 into kick.
var dupSuffix = regexp.MustCompile(`^(.*)-([1-9])$`)

// baseInstrumentID collapses a duplicate-row instance onto its base, but
// ONLY when the base is itself a known instrument. Both conditions matter:
// the single-digit bound protects the 808 instruments, and the known-base
// check protects any id that ends in "-1" without having a base.
func baseInstrumentID(id string, known map[string]bool) string {
	m := dupSuffix.FindStringSubmatch(id)
	if m == nil {
		return id
	}
	if known[m[1]] {
		return m[1]
	}
	return id
}

type covRow struct {
	Instrument string
	State      string // covered | synthetic | needs-source | unclassified
	RefID      string
	Note       string
}

// coverage classifies every base instrument in the builtin roster against
// the catalogue.
func coverage() ([]covRow, error) {
	c, err := wavref.Catalogue()
	if err != nil {
		return nil, err
	}

	known := make(map[string]bool, len(audio.BuiltinInstrumentIDs))
	for _, id := range audio.BuiltinInstrumentIDs {
		known[id] = true
	}
	bases := map[string]bool{}
	for _, id := range audio.BuiltinInstrumentIDs {
		bases[baseInstrumentID(id, known)] = true
	}

	rows := make([]covRow, 0, len(bases))
	for inst := range bases {
		row := covRow{Instrument: inst, State: "unclassified"}
		for _, r := range c.ForInstrument(inst) {
			switch r.Status {
			case "":
				row.State, row.RefID, row.Note = "covered", r.ID, ""
			case wavref.StatusSynthetic:
				row.State, row.RefID, row.Note = "synthetic", r.ID, r.Note
			case wavref.StatusNeedsSource:
				row.State, row.RefID, row.Note = "needs-source", r.ID, r.Note
			}
			if row.State == "covered" {
				break // a real reference outranks any status entry
			}
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Instrument < rows[j].Instrument })
	return rows, nil
}

// runDoctor writes the coverage report. Output is deterministic so it can
// be pinned as a golden.
func runDoctor(w io.Writer) error {
	rows, err := coverage()
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "INSTRUMENT\tSTATE\tREF\tNOTE")
	counts := map[string]int{}
	for _, r := range rows {
		counts[r.State]++
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Instrument, r.State, dashIfEmpty(r.RefID), dashIfEmpty(r.Note))
	}
	if err := tw.Flush(); err != nil {
		return err
	}

	c, err := wavref.Catalogue()
	if err != nil {
		return err
	}
	stems, err := templateStems()
	if err != nil {
		return err
	}
	var withRef, withoutRef []string
	for _, s := range stems {
		if _, ok := c.ForTemplate(s); ok {
			withRef = append(withRef, s)
		} else {
			withoutRef = append(withoutRef, s)
		}
	}

	fmt.Fprintf(w, "\ninstruments: %d covered, %d synthetic, %d needs-source, %d unclassified (of %d)\n",
		counts["covered"], counts["synthetic"], counts["needs-source"], counts["unclassified"], len(rows))
	fmt.Fprintf(w, "templates:   %d with a song reference, %d without (of %d)\n",
		len(withRef), len(withoutRef), len(stems))
	if len(withoutRef) > 0 {
		fmt.Fprintf(w, "  no song reference: %s\n", strings.Join(withoutRef, " "))
	}
	return nil
}

func dashIfEmpty(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// templateStems lists the committed template stems from the assets dir.
func templateStems() ([]string, error) {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return nil, fmt.Errorf("refs doctor: cannot locate the module root (no go.mod above cwd)")
	}
	dir := filepath.Join(root, "internal", "assets", "templates")
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("refs doctor: read templates: %w", err)
	}
	var out []string
	for _, e := range ents {
		if n := e.Name(); strings.HasSuffix(n, ".json") {
			out = append(out, strings.TrimSuffix(n, ".json"))
		}
	}
	sort.Strings(out)
	return out, nil
}

func cmdDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runDoctor(os.Stdout)
}
```

- [ ] **Step 4: Delete the `cmdDoctor` stub**

Remove the `cmdDoctor` line from `src/go/cmd/refs/stubs.go`.

- [ ] **Step 5: Generate the golden**

Run: `cd src/go && mkdir -p cmd/refs/testdata && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs doctor > cmd/refs/testdata/doctor_golden.txt && cat cmd/refs/testdata/doctor_golden.txt`
Expected: a table classifying every base instrument. At this point nearly every row reads `unclassified` — that is correct and expected; Task 7 fills the catalogue and regenerates this golden.

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -v`
Expected: PASS.

- [ ] **Step 7: Verify (do not commit)**

---

### Task 5: `refs fetch` — download, convert, measure, hash

**Files:**
- Create: `src/go/cmd/refs/fetch.go`
- Delete: the `cmdFetch` stub from `src/go/cmd/refs/stubs.go`
- Test: `src/go/cmd/refs/fetch_test.go`

**Interfaces:**
- Consumes: `wavref.Catalogue`, `wavref.ModuleRoot`, `wavref.CommittedDir`, `wavref.Dir` (Tasks 1–2), `hashFile` (Task 3 — decoding lives in `cmd/refs`, not `wavref`), `audio.DecodeWAVToPCM`, `fingerprint.DetectF0`, `wave.Wave`.
- Produces: `ffmpegArgs(in, out string, cv wavref.Convert) []string`, `semisFromA3(f0 float64) int`, `measureFile(path string) (wavref.Measured, error)`, `destDir(r wavref.Ref) (string, error)`, `runFetch(w io.Writer, opts fetchOpts) error`, `type fetchOpts struct{ ID, Instrument, Tier string; OnlyMissing, Force bool }`.

**Determinism:** ffmpeg gets `-map_metadata -1 -bitexact` so no encoder tag or timestamp lands in the file. Even so the recorded hash covers **decoded PCM**, not container bytes, so a different ffmpeg build changing chunk layout does not produce a false mismatch.

- [ ] **Step 1: Write the failing test**

Create `src/go/cmd/refs/fetch_test.go`:

```go
//go:build !js

package main

import (
	"math"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func TestFFmpegArgs_DeterministicAndCorrect(t *testing.T) {
	cv := wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, StartSec: 0.5, MaxSec: 2.0}
	got := strings.Join(ffmpegArgs("in.aif", "out.wav", cv), " ")
	for _, want := range []string{
		"-map_metadata -1", // no source tags
		"-bitexact",        // no encoder/timestamp tags
		"-ac 1",
		"-ar 44100",
		"-sample_fmt s16",
		"-ss 0.5",
		"-t 2",
		"-i in.aif",
		"out.wav",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("ffmpeg args missing %q\n  got: %s", want, got)
		}
	}
}

// -ss must precede -i so ffmpeg seeks the input rather than decoding and
// discarding, which is both faster and frame-accurate for our purposes.
func TestFFmpegArgs_SeekBeforeInput(t *testing.T) {
	cv := wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, StartSec: 1.0, MaxSec: 2.0}
	args := ffmpegArgs("in.aif", "out.wav", cv)
	ss, in := -1, -1
	for i, a := range args {
		switch a {
		case "-ss":
			ss = i
		case "-i":
			in = i
		}
	}
	if ss < 0 || in < 0 || ss > in {
		t.Errorf("-ss must come before -i; got %v", args)
	}
}

func TestFFmpegArgs_OmitsSeekWhenZero(t *testing.T) {
	cv := wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, MaxSec: 2.0}
	for _, a := range ffmpegArgs("in.aif", "out.wav", cv) {
		if a == "-ss" {
			t.Error("-ss emitted for StartSec 0")
		}
	}
}

func TestSemisFromA3(t *testing.T) {
	cases := []struct {
		f0   float64
		want int
	}{
		{220.0, 0},   // A3
		{440.0, 12},  // A4
		{110.0, -12}, // A2
		{261.63, 3},  // C4
		{587.33, 17}, // D5
		{0, 0},       // undetected pitch is reported as the origin
	}
	for _, c := range cases {
		if got := semisFromA3(c.f0); got != c.want {
			t.Errorf("semisFromA3(%.2f) = %d, want %d", c.f0, got, c.want)
		}
	}
}

// The whole point of measuring is to stop trusting filenames. Assert the
// measurement against a file whose true pitch we know independently.
func TestMeasureFile_DetectsPitchFromAudioNotName(t *testing.T) {
	p, ok := wavref.Resolve("violin-A4")
	if !ok {
		t.Skip("violin-A4 audio unavailable")
	}
	m, err := measureFile(p)
	if err != nil {
		t.Fatalf("measureFile: %v", err)
	}
	if math.Abs(m.F0Hz-440.0) > 12.0 {
		t.Errorf("F0Hz = %.2f, want ~440 (A4)", m.F0Hz)
	}
	if m.SemisFromA3 != 12 {
		t.Errorf("SemisFromA3 = %d, want 12", m.SemisFromA3)
	}
	if m.Peak <= 0 || m.Peak > 1.01 {
		t.Errorf("Peak = %f, want (0,1]", m.Peak)
	}
	if m.RMS <= 0 || m.RMS > m.Peak {
		t.Errorf("RMS = %f, want (0, Peak]", m.RMS)
	}
	if m.DurationSec <= 0 {
		t.Errorf("DurationSec = %f, want > 0", m.DurationSec)
	}
}

func TestDestDir_CommittedGoesInTree_SongGoesToEnvDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)

	inst := wavref.Ref{ID: "x", Tier: wavref.TierInstrument, Committed: true, File: "x.wav"}
	d, err := destDir(inst)
	if err != nil {
		t.Fatalf("destDir(committed): %v", err)
	}
	if !strings.HasSuffix(d, "testdata/refs") {
		t.Errorf("committed ref dest = %q, want the in-tree corpus", d)
	}

	song := wavref.Ref{ID: "y", Tier: wavref.TierSong, File: "y.wav"}
	d, err = destDir(song)
	if err != nil {
		t.Fatalf("destDir(song): %v", err)
	}
	if d != tmp {
		t.Errorf("song ref dest = %q, want %q ($%s)", d, tmp, wavref.EnvVar)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -run 'TestFFmpeg|TestSemis|TestMeasure|TestDestDir' -v`
Expected: FAIL — `undefined: ffmpegArgs`, `undefined: semisFromA3`, `undefined: measureFile`, `undefined: destDir`.

- [ ] **Step 3: Write the implementation**

Create `src/go/cmd/refs/fetch.go`:

```go
//go:build !js

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/audio/fingerprint"
	"github.com/ingyamilmolinar/beatmo/internal/wave"
	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// a3Hz is the pitch origin every Beatmo semitone offset is measured from.
const a3Hz = 220.0

// ffmpegArgs builds a deterministic conversion command. -map_metadata -1
// and -bitexact strip source tags and encoder/timestamp tags, so repeated
// conversions of the same input produce the same file.
func ffmpegArgs(in, out string, cv wavref.Convert) []string {
	args := []string{"-nostdin", "-y", "-loglevel", "error"}
	if cv.StartSec > 0 {
		// -ss before -i: seek the input rather than decode-and-discard.
		args = append(args, "-ss", strconv.FormatFloat(cv.StartSec, 'g', -1, 64))
	}
	args = append(args, "-i", in,
		"-map_metadata", "-1",
		"-bitexact",
		"-ac", strconv.Itoa(cv.Channels),
		"-ar", strconv.Itoa(cv.Rate),
		"-sample_fmt", sampleFmt(cv.Bits),
	)
	if cv.MaxSec > 0 {
		args = append(args, "-t", strconv.FormatFloat(cv.MaxSec, 'g', -1, 64))
	}
	return append(args, out)
}

func sampleFmt(bits int) string {
	if bits == 32 {
		return "s32"
	}
	return "s16"
}

// semisFromA3 converts a detected fundamental to Beatmo's semitone offset
// from A3=220Hz. An undetected pitch (0) reports the origin.
func semisFromA3(f0 float64) int {
	if f0 <= 0 {
		return 0
	}
	return int(math.Round(12 * math.Log2(f0/a3Hz)))
}

// measureFile derives every `measured` field FROM THE AUDIO. Upstream
// filename and metadata pitch labels have been wrong repeatedly, so they
// are never consulted.
func measureFile(path string) (wavref.Measured, error) {
	samples, rate, err := audio.DecodeWAVToPCM(path)
	if err != nil {
		return wavref.Measured{}, fmt.Errorf("decode %s: %w", path, err)
	}
	w := wave.Wave{Samples: make([]float64, len(samples)), SampleRate: rate, Label: filepath.Base(path)}
	var peak, sumsq float64
	for i, s := range samples {
		v := float64(s)
		w.Samples[i] = v
		if a := math.Abs(v); a > peak {
			peak = a
		}
		sumsq += v * v
	}
	rms := 0.0
	if len(samples) > 0 {
		rms = math.Sqrt(sumsq / float64(len(samples)))
	}
	f0 := fingerprint.DetectF0(w)
	return wavref.Measured{
		F0Hz:        f0,
		SemisFromA3: semisFromA3(f0),
		Peak:        peak,
		RMS:         rms,
		DurationSec: w.Duration(),
	}, nil
}

// destDir picks where a ref's audio lands: committed instrument clips go
// into the in-tree corpus; everything else goes to $BEATMO_WAV_REFS.
func destDir(r wavref.Ref) (string, error) {
	if r.Committed {
		d, ok := wavref.CommittedDir()
		if !ok {
			return "", fmt.Errorf("committed corpus dir not found (is the module tree intact?)")
		}
		return d, nil
	}
	d, ok := wavref.Dir()
	if !ok {
		return "", fmt.Errorf("set %s to a directory for uncommitted reference audio", wavref.EnvVar)
	}
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

func download(url, dest string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	// Some hosts (archive.org, github raw) reject an empty UA.
	req.Header.Set("User-Agent", "beatmo-refs/1.0")
	client := &http.Client{Timeout: 10 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

type fetchOpts struct {
	ID          string
	Instrument  string
	Tier        string
	OnlyMissing bool
	Force       bool
}

func (o fetchOpts) selects(r wavref.Ref) bool {
	if r.Status != "" {
		return false // nothing to fetch for synthetic / needs-source
	}
	if o.ID != "" && r.ID != o.ID {
		return false
	}
	if o.Tier != "" && r.Tier != o.Tier {
		return false
	}
	if o.Instrument != "" {
		found := false
		for _, i := range r.Instruments {
			if i == o.Instrument {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if o.OnlyMissing {
		if _, ok := wavref.Resolve(r.ID); ok {
			return false
		}
	}
	return true
}

// fetchOne downloads, converts, measures and hashes a single ref, updating
// it in place. It returns false when the ref was skipped as already good.
func fetchOne(w io.Writer, r *wavref.Ref, force bool) (bool, error) {
	if !force && r.PCMSHA256 != "" {
		if p, ok := wavref.Resolve(r.ID); ok {
			if got, err := hashFile(p); err == nil && got == r.PCMSHA256 {
				fmt.Fprintf(w, "skip     %s (already matches recorded hash)\n", r.ID)
				return false, nil
			}
		}
	}

	dir, err := destDir(*r)
	if err != nil {
		return false, fmt.Errorf("%s: %w", r.ID, err)
	}

	tmp, err := os.MkdirTemp("", "refs-fetch-")
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(tmp)

	raw := filepath.Join(tmp, "raw"+filepath.Ext(r.Source.URL))
	fmt.Fprintf(w, "get      %s\n", r.ID)
	if err := download(r.Source.URL, raw); err != nil {
		return false, fmt.Errorf("%s: %w", r.ID, err)
	}

	out := filepath.Join(dir, r.File)
	cv := wavref.Convert{Channels: 1, Rate: 44100, Bits: 16, MaxSec: 2.0}
	if r.Convert != nil {
		cv = *r.Convert
	}
	cmd := exec.Command("ffmpeg", ffmpegArgs(raw, out, cv)...)
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		return false, fmt.Errorf("%s: ffmpeg: %w\n%s", r.ID, err, outBytes)
	}

	m, err := measureFile(out)
	if err != nil {
		return false, fmt.Errorf("%s: %w", r.ID, err)
	}
	h, err := hashFile(out)
	if err != nil {
		return false, fmt.Errorf("%s: %w", r.ID, err)
	}
	r.Measured = &m
	r.PCMSHA256 = h
	fmt.Fprintf(w, "ok       %s  f0=%.1fHz (%+d semis from A3)  %.2fs  peak=%.3f\n",
		r.ID, m.F0Hz, m.SemisFromA3, m.DurationSec, m.Peak)
	return true, nil
}

// runFetch fetches every selected ref and writes the updated catalogue back.
//
// It goes through loadCatalogue (NOT wavref.Catalogue) deliberately: fetch is
// the only command that REWRITES the source-of-truth file, so it is the one
// that must refuse to operate on a structurally broken catalogue. Reading with
// only the schema_version check would let a duplicate id be silently mutated
// and written back, laundering the corruption.
func runFetch(w io.Writer, opts fetchOpts) error {
	c, err := loadCatalogue()
	if err != nil {
		return err
	}
	refs := append([]wavref.Ref(nil), c.Refs...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })

	changed := 0
	var failures []error
	for i := range refs {
		if !opts.selects(refs[i]) {
			continue
		}
		did, err := fetchOne(w, &refs[i], opts.Force)
		if err != nil {
			fmt.Fprintf(w, "FAIL     %v\n", err)
			failures = append(failures, err)
			continue
		}
		if did {
			changed++
		}
	}

	if changed > 0 {
		c.Refs = refs
		if err := writeCatalogue(c); err != nil {
			return err
		}
		fmt.Fprintf(w, "\nupdated catalogue.json (%d ref(s))\n", changed)
	}
	if len(failures) > 0 {
		return fmt.Errorf("refs fetch: %d ref(s) failed", len(failures))
	}
	return nil
}

// writeCatalogue rewrites catalogue.json in the source tree. The file is
// embedded, so a rebuild picks up the change.
//
// The write is atomic — temp file in the SAME directory, then rename — because
// this is the single source of truth for every reference. A crash mid-write
// would otherwise truncate it. This matches the atomic-rename convention the
// internal/userprefs stores already use.
func writeCatalogue(c wavref.Cat) error {
	root, ok := wavref.ModuleRoot()
	if !ok {
		return fmt.Errorf("cannot locate the module root to write catalogue.json")
	}
	path := filepath.Join(root, "internal", "wavref", "catalogue.json")
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "catalogue-*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func cmdFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	var o fetchOpts
	fs.StringVar(&o.ID, "id", "", "fetch a single ref id")
	fs.StringVar(&o.Instrument, "instrument", "", "fetch refs for one instrument")
	fs.StringVar(&o.Tier, "tier", "", "fetch one tier: instrument|song")
	fs.BoolVar(&o.OnlyMissing, "only-missing", false, "skip refs whose audio is already present")
	fs.BoolVar(&o.Force, "force", false, "re-fetch even when the local hash already matches")
	if err := fs.Parse(args); err != nil {
		return err
	}
	return runFetch(os.Stdout, o)
}
```

- [ ] **Step 4: Delete the `cmdFetch` stub**

Remove the `cmdFetch` line from `src/go/cmd/refs/stubs.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -v`
Expected: PASS.

- [ ] **Step 6: Verify against the real network and the real five clips**

Run: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs fetch -tier instrument -force`
Expected: five `ok` lines. `violin-A4` must report `f0≈440Hz (+12 semis from A3)`, `cello-A3` `f0≈220Hz (+0)`, `flute-A4` and `oboe-A4` `+12`, `trumpet-A4` `+12`. `catalogue.json` gains `pcm_sha256` and `measured` for all five.

Then: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs verify`
Expected: all 5 refs report as **verified** (not `no-hash`), 0 missing, 0 errors, 0 mismatched.

> **Output-format note.** Task 3's review found that `verify` printed `ok` identically whether a hash matched or the ref had no recorded hash at all, so it now distinguishes them — `ok` (hash confirmed) vs `ok (no hash yet)`, with separate counters, plus a distinct `ERROR` bucket for decode failures. Before this fetch runs, all 5 refs are `no hash yet`; **afterwards every one must read as verified.** That transition is the real assertion here: it proves `fetch` actually recorded hashes rather than silently doing nothing.

- [ ] **Step 7: Confirm the hashes are reproducible**

Run: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs fetch -tier instrument -force && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs verify`
Expected: all 5 refs verified again, 0 missing / 0 errors / 0 mismatched, and `git diff --stat src/go/internal/wavref/catalogue.json` shows **no change** — proving the conversion is deterministic.

- [ ] **Step 8: Verify (do not commit)**

---

### Task 6: `refs candidates` and `refs choose` — ear-selectable references

**Files:**
- Create: `src/go/cmd/refs/candidates.go`
- Delete: `src/go/cmd/refs/stubs.go` (now empty)
- Test: `src/go/cmd/refs/candidates_test.go`

**Interfaces:**
- Consumes: `fetchOne`, `measureFile`, `destDir` (Task 5), `wavref.Catalogue`, `wavref.Alternative`.
- Produces: `candidateDir(inst string) (string, error)`, `runCandidates(w io.Writer, inst string, n int) error`, `runChoose(w io.Writer, inst, id string) error`.

**Why:** which recording is the *right* reference is an ear judgement — two clean CC0 violin samples differ in mic distance, room, dynamic and vibrato depth, and picking wrong silently sets a bad target for every later tuning pass. `candidates` fetches the competitors side by side, converted identically so they are directly comparable, and prints a comparison table. `choose` promotes the winner and records the losers with their reason.

- [ ] **Step 1: Write the failing test**

Create `src/go/cmd/refs/candidates_test.go`:

```go
//go:build !js

package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

func TestCandidateDir_UnderEnvDirAndNamedByInstrument(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv(wavref.EnvVar, tmp)
	d, err := candidateDir("violin")
	if err != nil {
		t.Fatalf("candidateDir: %v", err)
	}
	if !strings.HasPrefix(d, tmp) {
		t.Errorf("candidateDir = %q, want it under %q", d, tmp)
	}
	if filepath.Base(d) != "violin" {
		t.Errorf("candidateDir base = %q, want %q", filepath.Base(d), "violin")
	}
	if _, err := os.Stat(d); err != nil {
		t.Errorf("candidateDir not created: %v", err)
	}
}

func TestRunCandidates_UnknownInstrumentErrors(t *testing.T) {
	t.Setenv(wavref.EnvVar, t.TempDir())
	var buf bytes.Buffer
	if err := runCandidates(&buf, "no-such-instrument", 3); err == nil {
		t.Error("runCandidates accepted an unknown instrument")
	}
}

// Choosing must be reversible: the losers are kept with a reason so the
// decision can be revisited when an instrument proves hard to tune.
func TestRunChoose_RecordsLosersAsAlternatives(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	refs := c.ForInstrument("violin")
	if len(refs) < 2 {
		t.Skip("violin has only one candidate; nothing to choose between")
	}
	var buf bytes.Buffer
	if err := runChoose(&buf, "violin", refs[0].ID); err != nil {
		t.Fatalf("runChoose: %v", err)
	}
	if !strings.Contains(buf.String(), refs[0].ID) {
		t.Errorf("choose output does not name the winner %q", refs[0].ID)
	}
}

func TestRunChoose_RejectsIDNotServingInstrument(t *testing.T) {
	var buf bytes.Buffer
	if err := runChoose(&buf, "violin", "trumpet-A4"); err == nil {
		t.Error("runChoose accepted a ref that is not a violin reference")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -run 'TestCandidate|TestRunChoose' -v`
Expected: FAIL — `undefined: candidateDir`, `undefined: runCandidates`, `undefined: runChoose`.

- [ ] **Step 3: Write the implementation**

Create `src/go/cmd/refs/candidates.go`:

```go
//go:build !js

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"text/tabwriter"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// candidateDir is the review directory for one instrument's competing
// references. It lives under $BEATMO_WAV_REFS so review audio never lands
// in the tree.
func candidateDir(inst string) (string, error) {
	base, ok := wavref.Dir()
	if !ok {
		return "", fmt.Errorf("set %s to a directory for candidate review audio", wavref.EnvVar)
	}
	d := filepath.Join(base, "candidates", inst)
	if err := os.MkdirAll(d, 0o755); err != nil {
		return "", err
	}
	return d, nil
}

// runCandidates fetches up to n competing references for inst into the
// review directory and prints them side by side for ear comparison.
func runCandidates(w io.Writer, inst string, n int) error {
	c, err := wavref.Catalogue()
	if err != nil {
		return err
	}
	refs := c.ForInstrument(inst)
	var real []wavref.Ref
	for _, r := range refs {
		if r.Status == "" {
			real = append(real, r)
		}
	}
	if len(real) == 0 {
		return fmt.Errorf("refs candidates: no candidate references for instrument %q", inst)
	}
	sort.Slice(real, func(i, j int) bool { return real[i].ID < real[j].ID })
	if n > 0 && len(real) > n {
		real = real[:n]
	}

	dir, err := candidateDir(inst)
	if err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tF0 (Hz)\tSEMIS\tDUR (s)\tPEAK\tRMS\tFILE")
	for i := range real {
		r := real[i]
		p, ok := wavref.Resolve(r.ID)
		if !ok {
			if _, err := fetchOne(w, &r, false); err != nil {
				fmt.Fprintf(w, "FAIL %v\n", err)
				continue
			}
			p, ok = wavref.Resolve(r.ID)
			if !ok {
				continue
			}
		}
		review := filepath.Join(dir, r.ID+".wav")
		if err := copyFile(p, review); err != nil {
			return err
		}
		m, err := measureFile(review)
		if err != nil {
			return err
		}
		fmt.Fprintf(tw, "%s\t%.1f\t%+d\t%.2f\t%.3f\t%.3f\t%s\n",
			r.ID, m.F0Hz, m.SemisFromA3, m.DurationSec, m.Peak, m.RMS, review)
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(w, "\nListen to the files above, then run:\n  refs choose -instrument %s -id <winning-id>\n", inst)
	return nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// runChoose promotes one candidate to be inst's reference. The losers stay
// in the catalogue as Alternatives so the decision is recorded, reversible,
// and available to SP2 as labelled data.
func runChoose(w io.Writer, inst, id string) error {
	c, err := wavref.Catalogue()
	if err != nil {
		return err
	}
	winner, ok := c.Ref(id)
	if !ok {
		return fmt.Errorf("refs choose: unknown ref %q", id)
	}
	serves := false
	for _, i := range winner.Instruments {
		if i == inst {
			serves = true
			break
		}
	}
	if !serves {
		return fmt.Errorf("refs choose: ref %q is not a reference for instrument %q", id, inst)
	}

	refs := append([]wavref.Ref(nil), c.Refs...)
	var alts []wavref.Alternative
	for i := range refs {
		if refs[i].ID == id || refs[i].Status != "" {
			continue
		}
		if !servesInstrument(refs[i], inst) {
			continue
		}
		if refs[i].Source != nil {
			alts = append(alts, wavref.Alternative{
				URL:    refs[i].Source.URL,
				Reason: fmt.Sprintf("not chosen for %s by ear; %s won", inst, id),
			})
		}
		refs[i].Instruments = withoutString(refs[i].Instruments, inst)
	}

	// Drop refs that no longer serve anything.
	kept := refs[:0]
	for _, r := range refs {
		if r.Tier == wavref.TierInstrument && r.Status == "" && len(r.Instruments) == 0 {
			continue
		}
		kept = append(kept, r)
	}
	refs = kept

	for i := range refs {
		if refs[i].ID == id {
			refs[i].Alternatives = append(refs[i].Alternatives, alts...)
		}
	}

	c.Refs = refs
	if err := writeCatalogue(c); err != nil {
		return err
	}
	fmt.Fprintf(w, "chose %s as the reference for %s (%d alternative(s) recorded)\n", id, inst, len(alts))
	return nil
}

func servesInstrument(r wavref.Ref, inst string) bool {
	for _, i := range r.Instruments {
		if i == inst {
			return true
		}
	}
	return false
}

// withoutString returns a NEW slice with drop removed.
//
// It deliberately allocates rather than filtering in place (`out := ss[:0]`).
// An in-place filter here silently corrupts the process-wide catalogue: the
// `refs := append([]wavref.Ref(nil), c.Refs...)` copy in runChoose is SHALLOW,
// so each Ref.Instruments still shares its backing array with wavref's memoized
// global. Filtering in place writes through that shared array — dropping
// "trumpet" from ["trumpet","trumpet-mellow"] leaves the global reading
// ["trumpet-mellow","trumpet-mellow"]. Invisible in a one-shot CLI run; in a
// test binary it poisons every later catalogue load in the same process.
// These lists are a few strings — the allocation costs nothing.
func withoutString(ss []string, drop string) []string {
	out := make([]string, 0, len(ss))
	for _, s := range ss {
		if s != drop {
			out = append(out, s)
		}
	}
	return out
}

func cmdCandidates(args []string) error {
	fs := flag.NewFlagSet("candidates", flag.ExitOnError)
	inst := fs.String("instrument", "", "instrument id to gather candidates for")
	n := fs.Int("n", 3, "maximum candidates to prepare")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inst == "" {
		return fmt.Errorf("refs candidates: -instrument is required")
	}
	return runCandidates(os.Stdout, *inst, *n)
}

func cmdChoose(args []string) error {
	fs := flag.NewFlagSet("choose", flag.ExitOnError)
	inst := fs.String("instrument", "", "instrument id")
	id := fs.String("id", "", "winning ref id")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *inst == "" || *id == "" {
		return fmt.Errorf("refs choose: -instrument and -id are both required")
	}
	return runChoose(os.Stdout, *inst, *id)
}
```

- [ ] **Step 4: Delete `stubs.go`**

Run: `rm src/go/cmd/refs/stubs.go`
All four stubbed commands are now implemented.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -v`
Expected: PASS.

- [ ] **Step 6: Verify the full CLI builds and every subcommand responds**

Run:
```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/refs ./cmd/refs && \
  /tmp/refs list && /tmp/refs verify && /tmp/refs doctor | tail -5
```
Expected: all three produce output and exit 0.

- [ ] **Step 7: Verify (do not commit)**

---

### Task 7: Populate the Tier-II instrument corpus

**Files:**
- Modify: `src/go/internal/wavref/catalogue.json` (add ~55 entries)
- Modify: `src/go/cmd/refs/testdata/doctor_golden.txt` (regenerate)
- Modify: `src/go/internal/audio/testdata/refs/MANIFEST.md` (regenerate)
- Delete: the 18 loose `*.wav` files at the repository root (after absorbing them)

**Interfaces:**
- Consumes: everything from Tasks 1–6.
- Produces: a catalogue where every base instrument is classified `covered`, `synthetic`, or `needs-source` — zero `unclassified`.

**Instrument classification.** Every base instrument in `audio.BuiltinInstrumentIDs` must land in exactly one bucket:

- **Iowa MIS** (`https://theremin.music.uiowa.edu/sound%20files/MIS%20Pitches%20-%202014/...`, licence `unrestricted`): violin, viola, cello, double bass, flute, oboe, clarinet, bassoon, alto sax, trumpet, horn, trombone, tuba, marimba, vibraphone, xylophone, bells, piano.
- **VCSL** (`https://raw.githubusercontent.com/sgossner/VCSL/master/<path>`, licence `CC0-1.0`, credit `Versilian Community Sample Library`): conga, bongos, snare, toms, bass drum (kick), hi-hat, cowbell, shaker, claps, claves, cymbals (ride/crash), timpani, grand piano, upright piano, harpsichord, concert harp, pipe organ, tenor sax. Browse the tree with:
  `curl -s "https://api.github.com/repos/sgossner/VCSL/git/trees/master?recursive=1" | python3 -c "import json,sys; [print(t['path']) for t in json.load(sys.stdin)['tree'] if t['path'].lower().endswith('.wav')]"`
- **Absorbed root WAVs** (licence `CC0-1.0`, provider `freesound`, URL `https://freesound.org/s/<id>/`): the 18 files currently at the repository root cover nylon guitar (`117852__kyster__2-oct-c.wav`), baritone sax (`360251__mtg__sax-baritone-c3.wav`), harp (`127160__daphne_in_wonderland__celtic_harp_g2.wav`), congas (`455509__mrrentapercussionist__...`), several kicks, a snare, piano, violin (`356181__mtg__violin-d5.wav`), flute, trumpet, french horn, oboe. Copy each into the corpus, add a catalogue entry, then delete the root copy.
- **`status: synthetic`** (no acoustic counterpart; counts as covered): every `fm-*` id, `bass-808`, `kick-808`, `kick-electro`, `dnb-kick`, `scifi-lead`, `modular-pad`, `ensemble-lead`, `viola-pad`, `ghost-bass`, `sub-bass`, `bass-fm`. Note field states why.
- **`status: needs-source`** (acoustic, no licence-clean isolated recording found): `guitar-steel`, `guitar-electric`, `guitar-electric-neck`, `bass-guitar`, `clavinet`, `organ` (Hammond/drawbar — VCSL's pipe and Renaissance organs are a different instrument), `voice-soprano`, `voice-whisper` (excluded by the brief). Note field records the search already tried.

- [ ] **Step 1: Write the failing test**

Add to `src/go/cmd/refs/doctor_test.go`:

```go
// The catalogue must make a decision about every instrument. An
// unclassified instrument means someone added an instrument without
// deciding what its ground truth is.
func TestCoverage_NoUnclassifiedInstruments(t *testing.T) {
	rows, err := coverage()
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	var bad []string
	for _, r := range rows {
		if r.State == "unclassified" {
			bad = append(bad, r.Instrument)
		}
	}
	if len(bad) > 0 {
		t.Errorf("%d instrument(s) unclassified in catalogue.json: %v", len(bad), bad)
	}
}

// A needs-source or synthetic entry without a reason is an unexplained
// gap; the note is what makes the classification reviewable.
func TestCoverage_StatusRowsExplainThemselves(t *testing.T) {
	rows, err := coverage()
	if err != nil {
		t.Fatalf("coverage: %v", err)
	}
	for _, r := range rows {
		if (r.State == "synthetic" || r.State == "needs-source") && r.Note == "" {
			t.Errorf("%s is %s but carries no note explaining why", r.Instrument, r.State)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -run TestCoverage -v`
Expected: FAIL — `TestCoverage_NoUnclassifiedInstruments` lists most instruments, because the catalogue currently holds only five entries.

- [ ] **Step 3: Enumerate what must be classified**

Run:
```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs doctor | awk '$2=="unclassified"{print $1}'
```
Expected: the working list of instruments needing a catalogue entry. Work through it top to bottom.

- [ ] **Step 4: Add the Iowa MIS and VCSL entries**

For each instrument, append an entry to `src/go/internal/wavref/catalogue.json` following the Task 1 shape. `pcm_sha256` and `measured` are left out — `refs fetch` fills them. Set `"committed": true` for every instrument-tier entry (they are ≤3 s and licence-clean). Example VCSL entry:

```json
{
  "id": "conga-open-vcsl",
  "tier": "instrument",
  "instruments": ["conga", "conga-open", "conga-tumba"],
  "file": "conga_open.wav",
  "committed": true,
  "source": {
    "provider": "vcsl",
    "url": "https://raw.githubusercontent.com/sgossner/VCSL/master/Membranophones/Struck%20Membranophones/Conga/<exact-file>.wav",
    "license": "CC0-1.0",
    "credit": "Versilian Community Sample Library (CC0)"
  },
  "convert": { "channels": 1, "rate": 44100, "bits": 16, "max_sec": 2.0 }
}
```

- [ ] **Step 5: Add the status entries**

Example pair:

```json
{
  "id": "fm-family-synthetic",
  "tier": "instrument",
  "instruments": ["fm-bell", "fm-epiano", "fm-pluck", "fm-bass", "fm-lead", "bass-fm", "bass-808", "kick-808", "kick-electro", "dnb-kick", "scifi-lead", "modular-pad", "ensemble-lead", "viola-pad", "ghost-bass", "sub-bass"],
  "status": "synthetic",
  "note": "Synthesizer voices with no acoustic counterpart — there is no real-world recording to match against, so these are authored by ear and excluded from reference-driven tuning."
},
{
  "id": "electric-guitars-needs-source",
  "tier": "instrument",
  "instruments": ["guitar-steel", "guitar-electric", "guitar-electric-neck", "bass-guitar", "clavinet", "organ"],
  "status": "needs-source",
  "note": "No licence-clean isolated recording found. Iowa MIS has no guitar, clavinet or Hammond organ; VCSL covers pipe and Renaissance organ but not a drawbar Hammond; archive.org keyword search returns mixed full tracks. Revisit if a CC0 isolated sample surfaces."
}
```

- [ ] **Step 6: Fetch the whole Tier-II corpus**

Run: `cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs fetch -tier instrument`
Expected: one `ok` line per entry with a plausible `f0`/`semis` reading. Investigate any entry whose detected pitch contradicts its source's stated note — that is exactly the mislabelling this design exists to catch, and the *audio* is the authority.

- [ ] **Step 7: Absorb the root WAVs and delete the originals**

For each of the 18 root `*.wav` files, add a catalogue entry with `provider: freesound`, `license: CC0-1.0`, and the `https://freesound.org/s/<id>/` URL derived from its filename prefix. Because these are already local, copy rather than download:

```bash
cd /home/ymolinar/Repos/beatmo
for f in *.wav; do cp "$f" src/go/internal/audio/testdata/refs/; done
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs fetch -tier instrument -only-missing
```

Then, once `refs verify` is clean, remove the root copies:

```bash
cd /home/ymolinar/Repos/beatmo && rm -f ./*.wav && ls *.wav 2>&1
```
Expected: `ls: cannot access '*.wav': No such file or directory`.

- [ ] **Step 8: Regenerate the golden and the manifest**

Run:
```bash
cd src/go && \
  CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs doctor > cmd/refs/testdata/doctor_golden.txt && \
  CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs list -manifest && \
  tail -4 cmd/refs/testdata/doctor_golden.txt
```
Expected: the summary line reports `0 unclassified`.

- [ ] **Step 9: Run the full test suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/wavref/ ./cmd/refs/ ./internal/audio/... -v 2>&1 | tail -30`
Expected: PASS, with `TestCoverage_NoUnclassifiedInstruments` green.

- [ ] **Step 10: Verify (do not commit)**

Note for the reviewer: this task force-adds audio. Because the plan forbids git writes, list the files that *would* need force-adding so the user can do it:
`cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs list -tier instrument | awk '$5=="present"{print $1}'`

---

### Task 8: Tier-I song references and migration

**Files:**
- Modify: `src/go/internal/wavref/catalogue.json` (add ~11 song entries)
- Modify: `Makefile` (`fetch-refs`, `refs-doctor`, `refs-verify`; retire the two old targets)
- Delete: `scripts/fetch_synth_refs.sh`, `scripts/fetch_song_refs.sh`
- Modify: `CLAUDE.md`, `AGENTS.synth.instruments.md` (point at the new tooling)
- Test: `src/go/cmd/refs/song_test.go`

**Interfaces:**
- Consumes: everything from Tasks 1–7.
- Produces: song-tier catalogue entries for the eleven public-domain templates; `make fetch-refs` as the single entry point.

**Which templates get a song reference.** Only the public-domain pieces: `bach-toccata`, `bach-prelude-c`, `bach-cello-prelude`, `bach-flute-allemande`, `mozart-k545`, `vivaldi-spring`, `pachelbel-violin`, `marcello-oboe`, `handel-water-horn`, `asturias`, `felt-prelude`. The other twenty-one recreate copyrighted songs and get **no** entry — `doctor` lists them under "no song reference", which is the correct and honest state, not a gap to close.

**Finding a recording.** `musopen.org/static/...` now returns HTTP 403 behind Cloudflare, which is why `fetch_song_refs.sh` is dead. Use archive.org instead:

```bash
curl -s "https://archive.org/advancedsearch.php?q=collection%3Amusopen&fl%5B%5D=identifier&fl%5B%5D=title&rows=50&output=json" | python3 -m json.tool
curl -s "https://archive.org/metadata/<identifier>" | python3 -c "import json,sys; [print(f['name']) for f in json.load(sys.stdin)['files']]"
```
The download URL is `https://archive.org/download/<identifier>/<file>`. Prefer a lossless source; ffmpeg converts whatever lands.

- [ ] **Step 1: Write the failing test**

Create `src/go/cmd/refs/song_test.go`:

```go
//go:build !js

package main

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/wavref"
)

// pdTemplates are the templates that recreate a public-domain piece, so a
// real reference recording legally exists for them.
var pdTemplates = []string{
	"bach-toccata", "bach-prelude-c", "bach-cello-prelude", "bach-flute-allemande",
	"mozart-k545", "vivaldi-spring", "pachelbel-violin", "marcello-oboe",
	"handel-water-horn", "asturias", "felt-prelude",
}

func TestEveryPublicDomainTemplateHasASongReference(t *testing.T) {
	c, err := wavref.Catalogue()
	if err != nil {
		t.Fatalf("Catalogue: %v", err)
	}
	for _, stem := range pdTemplates {
		if _, ok := c.ForTemplate(stem); !ok {
			t.Errorf("public-domain template %q has no song reference", stem)
		}
	}
}

// Song recordings are tens of MB. Committing them would bloat the repo, so
// the catalogue must never mark one committed.
func TestSongReferencesAreNeverCommitted(t *testing.T) {
	c, _ := wavref.Catalogue()
	for _, r := range c.Refs {
		if r.Tier == wavref.TierSong && r.Committed {
			t.Errorf("song ref %q is marked committed; full recordings must stay in $%s", r.ID, wavref.EnvVar)
		}
	}
}

// Copyrighted templates must NOT acquire a reference — that would mean
// something copyrighted got catalogued.
func TestCopyrightedTemplatesHaveNoSongReference(t *testing.T) {
	c, _ := wavref.Catalogue()
	pd := map[string]bool{}
	for _, s := range pdTemplates {
		pd[s] = true
	}
	stems, err := templateStems()
	if err != nil {
		t.Fatalf("templateStems: %v", err)
	}
	for _, s := range stems {
		if pd[s] {
			continue
		}
		if _, ok := c.ForTemplate(s); ok {
			t.Errorf("template %q is not public domain but has a song reference", s)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/refs/ -run 'TestEveryPublicDomain|TestSongReferences|TestCopyrighted' -v`
Expected: FAIL — `TestEveryPublicDomainTemplateHasASongReference` reports all eleven missing.

- [ ] **Step 3: Add the eleven song entries**

Append to `catalogue.json`, one per public-domain template. Note the absent `"committed"` (defaults false) and the longer `max_sec`:

```json
{
  "id": "song-bach-toccata",
  "tier": "song",
  "template": "bach-toccata",
  "file": "song_bach_toccata.wav",
  "source": {
    "provider": "archive-org",
    "url": "https://archive.org/download/<identifier>/<file>",
    "license": "PD",
    "credit": "<performer>, via archive.org"
  },
  "convert": { "channels": 1, "rate": 44100, "bits": 16, "max_sec": 120.0 }
}
```

- [ ] **Step 4: Fetch the song tier**

Run:
```bash
export BEATMO_WAV_REFS=$HOME/.beatmo/wav-refs
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go run ./cmd/refs fetch -tier song
```
Expected: one `ok` line per entry. These land in `$BEATMO_WAV_REFS`, not the tree.

- [ ] **Step 5: Confirm nothing large entered the tree**

Run: `cd /home/ymolinar/Repos/beatmo && find src/go/internal/audio/testdata/refs -name '*.wav' -size +2M | head`
Expected: no output — every committed clip is well under 2 MB.

- [ ] **Step 6: Replace the Makefile targets**

In `Makefile`, delete the `fetch-synth-refs` and `fetch-song-refs` targets and add:

```make
.PHONY: fetch-refs refs-doctor refs-verify

fetch-refs: ## Fetch reference audio into the corpus ([TIER=instrument|song], [ID=], [INSTRUMENT=], [ONLY_MISSING=1], [FORCE=1])
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs fetch \
		$(if $(TIER),-tier $(TIER),) $(if $(ID),-id $(ID),) \
		$(if $(INSTRUMENT),-instrument $(INSTRUMENT),) \
		$(if $(ONLY_MISSING),-only-missing,) $(if $(FORCE),-force,)

refs-doctor: ## Report reference coverage for every instrument and template
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs doctor

refs-verify: ## Hash every local reference against the catalogue
	cd src/go && ../../.tools/go/bin/go run ./cmd/refs verify
```

Update the `.PHONY` line that currently lists `fetch-synth-refs fetch-song-refs` to drop them.

- [ ] **Step 7: Delete the superseded scripts**

Run: `cd /home/ymolinar/Repos/beatmo && rm scripts/fetch_synth_refs.sh scripts/fetch_song_refs.sh && grep -rn "fetch_synth_refs\|fetch_song_refs\|fetch-synth-refs\|fetch-song-refs" --include='*.md' --include='Makefile' --include='*.go' --include='*.sh' . | grep -v docs/superpowers`
Expected: no output. If any reference remains, update it to the new target.

- [ ] **Step 8: Update the docs**

In `CLAUDE.md`, add to the Quick Start table:

```
| Fetch reference audio | `make fetch-refs [TIER=instrument\|song]` |
| Reference coverage report | `make refs-doctor` |
```

In `AGENTS.synth.instruments.md`, in the "Analysis-driven tuning against real recordings" section, replace the prose about `fetch_synth_refs.sh` and the hand-maintained MANIFEST with: provenance lives in `internal/wavref/catalogue.json`; `make refs-doctor` reports coverage; `measured.semis_from_a3` is detected from the audio, so `synth-match` should render at that pitch rather than trusting a filename.

- [ ] **Step 9: Run the full fast suite**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/... ./cmd/refs/ 2>&1 | grep -v "^ok\|no test files" | head -30`
Expected: no failures.

**Do not use a bare `./...` here.** This branch has two known, pre-existing artifacts that are NOT regressions and will drown the signal: (a) a phantom duplicate package `.../src/go/internal/ui` that fails to compile under `./...` while the real package is fine; (b) `cmd/synth-match` and `cmd/synth-analyze` report `function main is undeclared in the main package` under `-tags test`, because their `main.go` carries `//go:build !test && !js` — which `cmd/refs` deliberately mirrors. Verify explicit package paths instead. Nothing outside `internal/wavref` and `cmd/refs` was touched, and `wavref.Dir`/`Path` kept their signatures, so the five existing call sites are unaffected.

- [ ] **Step 10: Verify the whole CLI end to end**

Run:
```bash
cd src/go && CGO_ENABLED=1 ../../.tools/go/bin/go build -o /tmp/refs ./cmd/refs && \
  /tmp/refs doctor | tail -6 && /tmp/refs verify | tail -3
```
Expected: `0 unclassified`, eleven templates with a song reference, twenty-one without, and a clean verify.

- [ ] **Step 11: Verify (do not commit)**

Report to the user: the list of new `*.wav` files under `src/go/internal/audio/testdata/refs/` that need `git add -f`, and the list of deleted root `*.wav` files.

---

## Self-Review

**Spec coverage.** Every section of `2026-08-19-reference-audio-corpus-design.md` maps to a task: catalogue shape → Task 1; resolver and PCM hashing → Task 2; `list`/`verify` → Task 3; `doctor` → Task 4; `fetch` → Task 5; `candidates`/`choose` → Task 6; Tier-II corpus contents and root-WAV absorption → Task 7; Tier-I corpus, migration, generated MANIFEST and docs → Task 8. The spec's test list is covered: schema and uniqueness (Task 1), committed refs resolving and hashing (Tasks 2, 5), skip-clean behaviour (Task 2), doctor golden (Task 4), `semis_from_a3` round-trip (Task 5).

**Deviation from the spec, recorded here.** The spec called the field `sha256` over the converted file. The plan uses `pcm_sha256` over decoded PCM instead, because ffmpeg builds differ in the container chunks they emit and a container hash would produce false mismatches on a different machine while adding no real coverage. Verification of *audio* is what matters.

**Type consistency.** `wavref.Ref.PCMSHA256` is used consistently in Tasks 1, 2, 3, 5, 6. `runList`/`runVerify`/`runDoctor`/`runFetch`/`runCandidates`/`runChoose` all take `io.Writer` first. `cmdX(args []string) error` is uniform across all six subcommands and matches `main.go`'s dispatch. `wavref.Convert` is passed by value to `ffmpegArgs` and stored as `*Convert` on `Ref` — the nil case is defaulted in `fetchOne`. `covRow` is defined once in Task 4 and used by Task 7's added tests.

**Known follow-ups, deliberately out of scope:** the generated MANIFEST replaces the hand-written one but `songrefs/MANIFEST.md` is left in place until Task 8's grep confirms nothing references it; SP2 consumes `measured.semis_from_a3` but no existing caller is rewired to it in SP1 (that is SP2/SP4 work).
