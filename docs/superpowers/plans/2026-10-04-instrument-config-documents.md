# Instrument Config Documents (Sub-project 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
> **Repo rule:** never `git add` / `git commit` / `git stash` (the user commits). Every "Checkpoint" step is a VERIFY CHECKPOINT: run the named suites, read the output, and append one line to `.superpowers/sdd/2026-10-04-instrument-config-documents/progress.md` (create it on first use).

**Goal:** Every shipped instrument becomes ONE config document that holds every value of its synth voice, and the whole app — render, editing, library, project files, bundles and templates — runs on those documents with no defaults, identity fill, overlay or recipe layer. Sound stays bit-identical on each platform.

**Architecture:** A pure package `internal/instrument` holds the document (`Doc`), the module-type registry and pure functions over them (`Validate`, `Flatten`, `FieldValues`, `SetField`, `Diff`, `Migrate`, hashes). `internal/audio` declares the built-in module types, keeps the project's document copies in a store, renders voices from `Flatten(doc)`, and pushes the same block to the browser. The 150 documents are generated once from today's Go seeds and proven to render like them, then the seeds and the recipe layer are deleted.

**Tech Stack:** Go 1.26 (`.tools/go/bin/go`), cgo C engine (`build/libdrums.a`, unchanged), JavaScript/WebAudio (`src/js/audio.js`), Playwright browser tests.

**Spec:** `docs/superpowers/specs/2026-10-04-instrument-config-documents-design.md` (v2). Read it first; this plan argues from it. Background maps of today's code (file:line) are summarised inside each task.

## Global Constraints

- Bundled Go only: `.tools/go/bin/go`.
  - Fast path: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`.
  - Real path (cgo engine, real Ebiten): `cd src/go && xvfb-run -a ../../.tools/go/bin/go test <pkgs>`.
  - The full `internal/ui` suite takes ~33 min; give it `-timeout 40m`.
- **No sound changes.** At every checkpoint after Part A:
  - the 150 default one-shot renders are bit-identical to the baseline `~/.beatmo/render-parity/Ffix2` (render-parity tool, Task B10);
  - the export→import master-mix identity tests stay green.

  The only allowed deltas are the ones a task names. They are recorded with their reason in `progress.md`.
- **Per-platform identity, not cross-platform.** The browser applies neither voice `fx` nor `gate_tail` today and still won't (spec §8 "Kept divergences").
- **No C edits** in this sub-project, so there is no `make build/libdrums.a`. `src/js/main.wasm` must be rebuilt (`make wasm`) before any browser test.
- `schema.Version` stays `"0.0.1"`. Every module type is at version 1. `instrument.FileFormat = "instrument"`.
- Instrument documents live in `src/go/internal/assets/instruments/<id>.json` (pretty JSON, one per instrument) plus `index.json` (the ordered id list). The embedded form is ONE gzip pack, `src/go/internal/assets/instruments.pack.json.gz`, built by `make asset-pack`.
- **UI work follows CLAUDE.md's visual-verification rule.**
  - Build: `CGO_ENABLED=1 .tools/go/bin/go build -o /tmp/beatmo_screenshot ./cmd` (from `src/go`).
  - Capture: `XDG_CONFIG_HOME=$S/xdg xvfb-run -a -s "-screen 0 1280x720x24" /tmp/beatmo_screenshot -log ERROR -scene <name> -screenshot <out.png>`.
  - Mobile: `-mobile -scene-pass mobile -win-w 390 -win-h 844`.
  - Open and look at every PNG; a bug fix needs a before/after pair.
- **Concurrent sessions edit this tree.** Touch only the files a task names. If `git status` shows another session modifying one of them, stop and ask the user.
- **Never commit audio files.** Parity dumps live under `~/.beatmo/render-parity`.
- **Test lifetimes.** Tests that compare against the Go seeds or `newModularBlock` (Tasks A8–A10) are deleted with the seeds in Task C2. The parity tool and the liveness ratchet carry the guarantee from then on.
- `$S` below means this session's scratchpad directory.

## Review Focus

1. **A layer moved past its engine slot limit renders silence.** A kick or snare copied, switched or imported into layer 2, or a cymbal into layer 4. Expected: `Validate`, `SwitchType` and `CopyModules` refuse it with an error naming the slot limit. Pinned in Task A3 (`TestValidateRejectsLayerPastCapacity`) and Task A5 (`TestSwitchTypeRespectsCapacity`, `TestCopyModulesRespectsCapacity`).
2. **Editing a knob on a stage the instrument doesn't have yet** (the dimmed card in the Synth tab). Expected: the stage is created from `Init`, OFF, and the sound does not change until it is switched on. Pinned in Task A5 (`TestSetFieldMaterialisesAbsentStageOff`) and Task A9 (the absent-stage cases render identically).
3. **Melodic node playback and edited instruments use a different block today than the one-shot render.** Clamped ParamDef defaults on the recipe path; the browser default layer is the same clamped map. Expected: every instrument whose two blocks differ is listed for the user BEFORE the switch, and the documents follow the one-shot (baked) values. Pinned by Task A10's audit report and its checkpoint question.
4. **Importing a project or library entry written by a newer build, or naming an unknown module type.** Expected: a hard error naming `instrument_docs.<id>.<path>` and the version; the current circuit is untouched; boot-time stores WARN and skip. Pinned in Task B7 (`TestImportRejectsUnknownModuleTypeBeforeReset`) and Task B2 (`TestUserLibrarySkipsInvalidDocument`).
5. **A Save on one project copy leaks into other copies.** For example, Save on `voice-ahh` while `voice-ahh--2` is on another row. Expected: the library entry changes; every existing project copy keeps its values; only future picks see the Save. Pinned in Task B6 (`TestSaveChangesFuturePicksOnly`).

---

## File Structure

**New package `src/go/internal/instrument/`** (pure; imports only `internal/schema` and the standard library):

| File | Responsibility |
|---|---|
| `doc.go` | `Doc`, `Stage`, `Module`, `Render`, `Values`, `File`, `ParseFile`, `MarshalFile`, `Clone` |
| `registry.go` | `Kind`, `FieldSet`, `ValueDecl`, `Step`, `ModuleTypeDecl`, `Registry`, `NewRegistry`, `LayerField` |
| `validate.go` | `Validate`, `ValidationError` |
| `flatten.go` | `Flatten`, `OwnedFields`, `FieldValues` |
| `edit.go` | `Ref`, `SetValue`, `SetOn`, `NewModule`, `AddStage`, `RemoveModule`, `SwitchType`, `SetField`, `CopyModules`, error values |
| `diff.go` | `Diff`, `BlockHash`, `ContentHash` |
| `migrate.go` | `Migrate` |
| `*_test.go` | Table tests against a fake registry (`testreg_test.go`) |

**`src/go/internal/audio/`** (new files):

| File | Responsibility |
|---|---|
| `module_decls.go` | The built-in module-type declarations (data) |
| `module_registry.go` | `BuiltinModuleRegistry()`: declarations + `Init` from `ModularSynthParamDefs` + the ABI field list |
| `instrument_library.go` | Shipped pack loader, user library, the library view |
| `instrument_docs.go` | The project document store: `InstrumentDoc`, `SetInstrumentDoc`, `ResetInstrumentDocs`, platform push hook |
| `instrument_docs_wasm.go` | `js && wasm`: pushes blocks to `window.setInstrumentBlock` |
| `doc_voice_native.go` | `!test && !js`: the single native voice path |
| `instrument_param_defs.go` | `InstrumentParamDefs`, `InstrumentDeclaresField` (UI adapter) |

**Commands:**

| Path | Responsibility |
|---|---|
| `src/go/cmd/gen-instrument-docs/` | One-time converter, seeds → documents (deleted in Task C2) |
| `src/go/cmd/gen-asset-pack/` | Builds the embedded pack from the document sources (+ template pins in Part D) |

**Assets:** `src/go/internal/assets/instruments/*.json`, `index.json`, `instruments.pack.json.gz`, `instruments.go` (embed).

Modified files are named per task.

---

# Part A — Model and proof (no runtime change)

### Task A1: Document types and file I/O

**Files:**
- Create: `src/go/internal/instrument/doc.go`
- Test: `src/go/internal/instrument/doc_test.go`

**Interfaces:**
- Produces:
  - `type Values map[string]float64`
  - `type Stage struct{ V int; On bool; Values Values; Notes string }`
  - `type Module struct{ Type string; V int; On bool; Values Values; Notes string }`
  - `type Render struct{ Beats, Seconds, Amplitude, GateTail float64; Melodic bool }`
  - `type Doc struct{ ID, Origin, Name, Category, Notes string; Render Render; Stages map[string]Stage; Source *Module; Layers []*Module; FX []Module }`
  - `(Doc).Clone() Doc`
  - `type File struct{ schema.Envelope; Doc }`
  - `ParseFile([]byte) (Doc, error)`, `MarshalFile(Doc) ([]byte, error)`
  - `const FileFormat = "instrument"`

- [ ] **Step 1: Write the failing test**

```go
package instrument

import (
	"strings"
	"testing"
)

func sampleDoc() Doc {
	return Doc{
		ID: "kick-snap", Name: "Snap Kick", Category: "modular", Notes: "retuned 2026-10-02",
		Render: Render{Beats: 0.5, Seconds: 0.6, Amplitude: 0.8},
		Stages: map[string]Stage{"core": {V: 1, On: true, Values: Values{"core_g": 1}}},
		Layers: []*Module{nil, {Type: "tone", V: 1, On: true, Values: Values{"gain": 0.5}}},
		FX:     []Module{{Type: "fx.filter", V: 1, On: true, Values: Values{"cutoff": 4000}}},
	}
}

func TestFileRoundTrip(t *testing.T) {
	in := sampleDoc()
	b, err := MarshalFile(in)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"schema_version": "0.0.1"`) {
		t.Fatalf("file lacks the schema version:\n%s", b)
	}
	out, err := ParseFile(b)
	if err != nil {
		t.Fatal(err)
	}
	if d := Diff(in, out); len(d) != 0 || out.ID != in.ID || out.Layers[0] != nil {
		t.Fatalf("round trip changed the document: %v", d)
	}
}

func TestParseFileRejectsUnknownField(t *testing.T) {
	_, err := ParseFile([]byte(`{"schema_version":"0.0.1","id":"x","bogus":1}`))
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Fatalf("want an unknown-field error naming bogus, got %v", err)
	}
}

func TestParseFileRejectsOtherVersion(t *testing.T) {
	_, err := ParseFile([]byte(`{"schema_version":"9.9.9","id":"x"}`))
	if err == nil || !strings.Contains(err.Error(), "9.9.9") {
		t.Fatalf("want a version error naming 9.9.9, got %v", err)
	}
}

func TestCloneIsDeep(t *testing.T) {
	a := sampleDoc()
	b := a.Clone()
	b.Stages["core"].Values["core_g"] = 7
	b.Layers[1].Values["gain"] = 7
	b.FX[0].Values["cutoff"] = 7
	if a.Stages["core"].Values["core_g"] != 1 || a.Layers[1].Values["gain"] != 0.5 || a.FX[0].Values["cutoff"] != 4000 {
		t.Fatal("Clone shares value maps with the original")
	}
}
```

(`Diff` comes in Task A6. Until then, temporarily compare with `reflect.DeepEqual(in, out)`, and switch to `Diff` in A6.)

- [ ] **Step 2: Run it and watch it fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/instrument/`
Expected: FAIL to compile (`undefined: Doc`).

- [ ] **Step 3: Implement**

```go
// Package instrument holds the instrument config document — ONE document per
// instrument holding every value its synth voice depends on — and the pure
// functions over it. Nothing here touches audio or globals: a *Registry of
// module types is passed in. Spec:
// docs/superpowers/specs/2026-10-04-instrument-config-documents-design.md.
package instrument

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

// FileFormat names this document type in version errors.
const FileFormat = "instrument"

// Values maps a module's value names (ABI field names; slot-relative for
// layers) to numbers.
type Values map[string]float64

func (v Values) clone() Values {
	if v == nil {
		return nil
	}
	out := make(Values, len(v))
	for k, x := range v {
		out[k] = x
	}
	return out
}

// Stage is a pipeline stage. Its type is its key in Doc.Stages.
type Stage struct {
	V      int    `json:"v"`
	On     bool   `json:"on"`
	Values Values `json:"values"`
	Notes  string `json:"notes,omitempty"`
}

// Module is a source, layer or voice-FX module.
type Module struct {
	Type   string `json:"type"`
	V      int    `json:"v"`
	On     bool   `json:"on"`
	Values Values `json:"values"`
	Notes  string `json:"notes,omitempty"`
}

// Render is the voice's render metadata. Amplitude is the effective playback
// amplitude (loudness included); GateTail 0 means no gate.
type Render struct {
	Beats     float64 `json:"beats"`
	Seconds   float64 `json:"seconds"`
	Amplitude float64 `json:"amplitude"`
	GateTail  float64 `json:"gate_tail"`
	Melodic   bool    `json:"melodic"`
}

// Doc is one instrument. Layers[i] is engine generator slot i+1; nil = empty.
type Doc struct {
	ID       string           `json:"id"`
	Origin   string           `json:"origin,omitempty"`
	Name     string           `json:"name"`
	Category string           `json:"category"`
	Notes    string           `json:"notes,omitempty"`
	Render   Render           `json:"render"`
	Stages   map[string]Stage `json:"stages"`
	Source   *Module          `json:"source,omitempty"`
	Layers   []*Module        `json:"layers,omitempty"`
	FX       []Module         `json:"fx,omitempty"`
}

// Clone returns a deep copy: no map or module is shared with d.
func (d Doc) Clone() Doc {
	out := d
	out.Stages = make(map[string]Stage, len(d.Stages))
	for k, s := range d.Stages {
		s.Values = s.Values.clone()
		out.Stages[k] = s
	}
	if d.Source != nil {
		m := *d.Source
		m.Values = m.Values.clone()
		out.Source = &m
	}
	if d.Layers != nil {
		out.Layers = make([]*Module, len(d.Layers))
		for i, l := range d.Layers {
			if l != nil {
				m := *l
				m.Values = m.Values.clone()
				out.Layers[i] = &m
			}
		}
	}
	if d.FX != nil {
		out.FX = make([]Module, len(d.FX))
		for i, m := range d.FX {
			m.Values = m.Values.clone()
			out.FX[i] = m
		}
	}
	return out
}

// File is a standalone document on disk: the schema envelope plus the doc.
type File struct {
	schema.Envelope
	Doc
}

// ParseFile decodes a standalone document strictly (unknown fields are an
// error) and checks its schema version. It does NOT Validate against a
// registry — callers Migrate then Validate.
func ParseFile(data []byte) (Doc, error) {
	var f File
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return Doc{}, fmt.Errorf("instrument: %w", err)
	}
	if err := f.Envelope.Validate(FileFormat); err != nil {
		return Doc{}, err
	}
	return f.Doc, nil
}

// MarshalFile encodes d as a standalone, current-version document.
func MarshalFile(d Doc) ([]byte, error) {
	b, err := json.MarshalIndent(File{Envelope: schema.Current(), Doc: d}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}
```

- [ ] **Step 4: Run the tests and watch them pass**

Run: same command. Expected: PASS.

- [ ] **Step 5: Checkpoint** — log "A1 green" in progress.md.

---

### Task A2: The registry

**Files:**
- Create: `src/go/internal/instrument/registry.go`, `src/go/internal/instrument/testreg_test.go`
- Test: `src/go/internal/instrument/registry_test.go`

**Interfaces:**
- Consumes: `Values` (A1).
- Produces:
  - `type Kind int` with `KindStage`, `KindSource`, `KindLayer`, `KindFX`
  - `type FieldSet struct{ Field string; Value float64 }`
  - `type ValueDecl struct{ Name string; Init float64; Allowed []float64 }`
  - `type Step func(Values) Values`
  - `type ModuleTypeDecl struct{ Type string; Version int; Kind Kind; Required bool; Enable *FieldSet; Set, Global []FieldSet; Capacity int; FreeForm bool; Values []ValueDecl; Steps []Step }`
  - `NewRegistry(decls []ModuleTypeDecl, fields []string, slots int) (*Registry, error)`
  - `(*Registry).Type(string) (*ModuleTypeDecl, bool)`, `.Types() []*ModuleTypeDecl`, `.Fields() []string`, `.FieldIndex(string) (int, bool)`, `.Slots() int`, `.Init(field string) (float64, bool)`
  - `LayerField(slot int, name string) string`, which returns `gen<slot+1>_<name>`
  - test helper `testRegistry(t) *Registry` and `minimalDoc() Doc`

- [ ] **Step 1: Write the fake registry and the failing tests**

`testreg_test.go`:

```go
package instrument

import "testing"

// testFields is a miniature ABI: two required stages, one switchable stage,
// two source types, a 2-slot generator bank with a 1-slot kick family.
var testFields = []string{
	"core_g", "fin_y", "a_enabled", "a_x", "osc_enabled", "osc_type", "fm_r",
	"gen1_source", "gen1_gain", "gen1_kv", "gen1_kc", "kick_on",
	"gen2_source", "gen2_gain", "noise_seed",
}

func testDecls() []ModuleTypeDecl {
	return []ModuleTypeDecl{
		{Type: "core", Version: 1, Kind: KindStage, Required: true, Values: []ValueDecl{{Name: "core_g", Init: 1}}},
		{Type: "fin", Version: 1, Kind: KindStage, Required: true, Values: []ValueDecl{{Name: "fin_y", Init: 0.5}}},
		{Type: "a", Version: 1, Kind: KindStage, Enable: &FieldSet{"a_enabled", 1}, Values: []ValueDecl{{Name: "a_x", Init: 8}}},
		{Type: "osc.wave", Version: 1, Kind: KindSource, Enable: &FieldSet{"osc_enabled", 1},
			Values: []ValueDecl{{Name: "osc_type", Init: 0, Allowed: []float64{0, 1}}}},
		{Type: "osc.fm", Version: 1, Kind: KindSource, Enable: &FieldSet{"osc_enabled", 1},
			Set: []FieldSet{{"osc_type", 4}}, Values: []ValueDecl{{Name: "fm_r", Init: 1}}},
		{Type: "tone", Version: 1, Kind: KindLayer, Enable: &FieldSet{"source", 1}, Values: []ValueDecl{{Name: "gain", Init: 1}}},
		{Type: "kick.a", Version: 1, Kind: KindLayer, Enable: &FieldSet{"source", 5}, Set: []FieldSet{{"kv", 0}},
			Global: []FieldSet{{"kick_on", 1}}, Capacity: 1, Values: []ValueDecl{{Name: "kc", Init: 0.3}}},
		{Type: "kick.b", Version: 1, Kind: KindLayer, Enable: &FieldSet{"source", 5}, Set: []FieldSet{{"kv", 1}},
			Global: []FieldSet{{"kick_on", 1}}, Capacity: 1, Values: []ValueDecl{{Name: "kc", Init: 0.3}}},
		{Type: "fx.filter", Version: 1, Kind: KindFX, FreeForm: true},
	}
}

func testRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewRegistry(testDecls(), testFields, 2)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// minimalDoc is the smallest valid document of the test registry.
func minimalDoc() Doc {
	return Doc{
		ID: "t", Name: "T", Category: "test",
		Render: Render{Beats: 1, Seconds: 0.5, Amplitude: 0.8},
		Stages: map[string]Stage{
			"core": {V: 1, On: true, Values: Values{"core_g": 1}},
			"fin":  {V: 1, On: true, Values: Values{"fin_y": 0.5}},
		},
	}
}
```

`registry_test.go`:

```go
package instrument

import (
	"strings"
	"testing"
)

func TestNewRegistryAccepts(t *testing.T) {
	r := testRegistry(t)
	if kt, ok := r.Type("kick.a"); !ok || kt.Capacity != 1 {
		t.Fatalf("kick.a capacity = %+v", kt)
	}
	if tt, _ := r.Type("tone"); tt.Capacity != 2 {
		t.Fatalf("Capacity 0 must normalise to the slot count, got %d", tt.Capacity)
	}
	if v, ok := r.Init("gen2_gain"); !ok || v != 1 {
		t.Fatalf("Init(gen2_gain) = %v %v", v, ok)
	}
	if v, ok := r.Init("gen1_kc"); !ok || v != 0.3 {
		t.Fatalf("Init(gen1_kc) = %v %v", v, ok)
	}
	if _, ok := r.Init("noise_seed"); ok {
		t.Fatal("noise_seed is owned by no value, so it has no Init")
	}
	if LayerField(0, "gain") != "gen1_gain" {
		t.Fatal(LayerField(0, "gain"))
	}
}

func TestNewRegistryRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]ModuleTypeDecl) []ModuleTypeDecl
		want   string
	}{
		{"duplicate type", func(d []ModuleTypeDecl) []ModuleTypeDecl { return append(d, d[0]) }, "duplicate type"},
		{"unknown field", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[2].Values[0].Name = "nope"; return d }, `"nope"`},
		{"layer past ABI", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[6].Capacity = 2; return d }, "gen2_kc"},
		{"steps vs version", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[0].Version = 2; return d }, "steps"},
		{"required with switch", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[2].Required = true; return d }, "required"},
		{"init not allowed", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[3].Values[0].Init = 3; return d }, "Allowed"},
		{"set overlaps value", func(d []ModuleTypeDecl) []ModuleTypeDecl {
			d[4].Values = append(d[4].Values, ValueDecl{Name: "osc_type"})
			return d
		}, "both"},
		{"layer without switch", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[5].Enable = nil; return d }, "source field"},
		{"free-form non-fx", func(d []ModuleTypeDecl) []ModuleTypeDecl { d[2].FreeForm = true; return d }, "free-form"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := NewRegistry(c.mutate(testDecls()), testFields, 2)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}
```

- [ ] **Step 2: Run and watch it fail** (compile error). Command as in A1.

- [ ] **Step 3: Implement `registry.go`**

```go
package instrument

import (
	"fmt"
	"slices"
)

// Kind is where a module type lives in a document.
type Kind int

const (
	KindStage  Kind = iota + 1 // Doc.Stages, keyed by type
	KindSource                 // Doc.Source
	KindLayer                  // Doc.Layers (index = engine slot)
	KindFX                     // Doc.FX (voice render effects)
)

func (k Kind) String() string {
	switch k {
	case KindStage:
		return "stage"
	case KindSource:
		return "source"
	case KindLayer:
		return "layer"
	case KindFX:
		return "fx"
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

// FieldSet is one engine field and the value a module writes to it.
type FieldSet struct {
	Field string
	Value float64
}

// ValueDecl declares one value of a module type. Name is the ABI field name
// (slot-relative for layers). Init is the value a NEWLY CREATED module starts
// with — never read for an existing document. Allowed, when non-empty, lists
// the only legal values (a selector that decides the type).
type ValueDecl struct {
	Name    string
	Init    float64
	Allowed []float64
}

// Step migrates one module's values from version i to i+1.
type Step func(Values) Values

// ModuleTypeDecl declares one module type. See spec §4.
type ModuleTypeDecl struct {
	Type     string
	Version  int
	Kind     Kind
	Required bool      // stages only: must be present, always on
	Enable   *FieldSet // written when ON; nil = no on/off switch
	Set      []FieldSet
	Global   []FieldSet // layers only: absolute fields written when on
	Capacity int        // layers only: slots the engine serves this voice in; 0 = all
	FreeForm bool       // fx only (until the FX lift): values are not declared
	Values   []ValueDecl
	Steps    []Step // len == Version-1
}

func (t *ModuleTypeDecl) value(name string) (ValueDecl, bool) {
	for _, v := range t.Values {
		if v.Name == name {
			return v, true
		}
	}
	return ValueDecl{}, false
}

func (t *ModuleTypeDecl) setValue(field string) (float64, bool) {
	for _, s := range t.Set {
		if s.Field == field {
			return s.Value, true
		}
	}
	return 0, false
}

// Registry is an immutable set of module types bound to one ABI field list.
type Registry struct {
	decls  []*ModuleTypeDecl
	byType map[string]*ModuleTypeDecl
	fields []string
	index  map[string]int
	slots  int
	init   map[string]float64
}

// LayerField is the ABI field of a slot-relative layer value.
func LayerField(slot int, name string) string { return fmt.Sprintf("gen%d_%s", slot+1, name) }

// NewRegistry validates decls against the ABI field list and the generator
// slot count, and returns the registry. Declarations are copied.
func NewRegistry(decls []ModuleTypeDecl, fields []string, slots int) (*Registry, error) {
	r := &Registry{
		byType: map[string]*ModuleTypeDecl{},
		fields: slices.Clone(fields),
		index:  make(map[string]int, len(fields)),
		slots:  slots,
		init:   map[string]float64{},
	}
	for i, f := range fields {
		if _, dup := r.index[f]; dup {
			return nil, fmt.Errorf("registry: duplicate ABI field %q", f)
		}
		r.index[f] = i
	}
	for i := range decls {
		d := decls[i]
		t := &d
		if err := r.check(t); err != nil {
			return nil, fmt.Errorf("registry: %s: %w", t.Type, err)
		}
		r.byType[t.Type] = t
		r.decls = append(r.decls, t)
	}
	for _, t := range r.decls { // first declaration of a field wins
		for _, v := range t.Values {
			if t.Kind == KindLayer {
				for s := 0; s < slots; s++ {
					f := LayerField(s, v.Name)
					if _, ok := r.index[f]; ok {
						if _, set := r.init[f]; !set {
							r.init[f] = v.Init
						}
					}
				}
			} else if t.Kind != KindFX {
				if _, set := r.init[v.Name]; !set {
					r.init[v.Name] = v.Init
				}
			}
		}
	}
	return r, nil
}

func (r *Registry) check(t *ModuleTypeDecl) error {
	switch {
	case t.Type == "":
		return fmt.Errorf("empty type")
	case r.byType[t.Type] != nil:
		return fmt.Errorf("duplicate type")
	case t.Version < 1:
		return fmt.Errorf("version %d < 1", t.Version)
	case len(t.Steps) != t.Version-1:
		return fmt.Errorf("version %d needs %d migration steps, has %d", t.Version, t.Version-1, len(t.Steps))
	case t.FreeForm && t.Kind != KindFX:
		return fmt.Errorf("only fx types may be free-form")
	case t.Required && (t.Kind != KindStage || t.Enable != nil):
		return fmt.Errorf("a required type must be a stage with no on/off switch")
	}
	if t.Kind == KindLayer {
		if t.Enable == nil {
			return fmt.Errorf("layer types are switched by their source field (Enable)")
		}
		if t.Capacity < 0 || t.Capacity > r.slots {
			return fmt.Errorf("capacity %d outside 0..%d", t.Capacity, r.slots)
		}
		if t.Capacity == 0 {
			t.Capacity = r.slots
		}
	} else if t.Capacity != 0 || len(t.Global) > 0 {
		return fmt.Errorf("only layer types have Capacity or Global")
	}
	names := map[string]bool{}
	for _, v := range t.Values {
		if names[v.Name] {
			return fmt.Errorf("duplicate value %q", v.Name)
		}
		names[v.Name] = true
		if len(v.Allowed) > 0 && !slices.Contains(v.Allowed, v.Init) {
			return fmt.Errorf("value %q: Init %v not in Allowed %v", v.Name, v.Init, v.Allowed)
		}
	}
	for _, s := range t.Set {
		if names[s.Field] {
			return fmt.Errorf("%q is both a Set constant and a value", s.Field)
		}
	}
	if t.Enable != nil && names[t.Enable.Field] {
		return fmt.Errorf("%q is both the Enable field and a value", t.Enable.Field)
	}
	need := func(f string) error {
		if _, ok := r.index[f]; !ok {
			return fmt.Errorf("field %q is not in the ABI", f)
		}
		return nil
	}
	var all []string
	switch t.Kind {
	case KindLayer:
		for s := 0; s < t.Capacity; s++ {
			for _, v := range t.Values {
				all = append(all, LayerField(s, v.Name))
			}
			for _, x := range t.Set {
				all = append(all, LayerField(s, x.Field))
			}
			all = append(all, LayerField(s, t.Enable.Field))
		}
		for _, g := range t.Global {
			all = append(all, g.Field)
		}
	case KindStage, KindSource:
		for _, v := range t.Values {
			all = append(all, v.Name)
		}
		for _, x := range t.Set {
			all = append(all, x.Field)
		}
		if t.Enable != nil {
			all = append(all, t.Enable.Field)
		}
	}
	for _, f := range all {
		if err := need(f); err != nil {
			return err
		}
	}
	return nil
}

// Type returns the declaration of a module type.
func (r *Registry) Type(name string) (*ModuleTypeDecl, bool) {
	t, ok := r.byType[name]
	return t, ok
}

// Types returns every declaration in declaration order. Do not mutate.
func (r *Registry) Types() []*ModuleTypeDecl { return r.decls }

// Fields returns the ABI field list in block order.
func (r *Registry) Fields() []string { return slices.Clone(r.fields) }

// FieldIndex returns a field's index in the block.
func (r *Registry) FieldIndex(f string) (int, bool) {
	i, ok := r.index[f]
	return i, ok
}

// Slots is the generator slot count.
func (r *Registry) Slots() int { return r.slots }

// Init returns the creation value of an ABI field (false when no value owns it).
func (r *Registry) Init(field string) (float64, bool) {
	v, ok := r.init[field]
	return v, ok
}
```

- [ ] **Step 4: Run and pass.** If the `"layer past ABI"` case does not mention `gen2_kc`, fix the error text, not the test.

- [ ] **Step 5: Checkpoint.**

---

### Task A3: Validate

**Files:**
- Create: `src/go/internal/instrument/validate.go`
- Test: `src/go/internal/instrument/validate_test.go`

**Interfaces:**
- Consumes: A1, A2.
- Produces: `Validate(Doc, *Registry) error`, `type ValidationError struct{ Problems []string }`, `finite(float64) bool` (package-private helper reused later).

- [ ] **Step 1: Failing tests**

```go
package instrument

import (
	"math"
	"strings"
	"testing"
)

func kickLayer(typ string) *Module {
	return &Module{Type: typ, V: 1, On: true, Values: Values{"kc": 0.3}}
}

func TestValidateAcceptsMinimalAndFull(t *testing.T) {
	r := testRegistry(t)
	if err := Validate(minimalDoc(), r); err != nil {
		t.Fatal(err)
	}
	d := minimalDoc()
	d.Stages["a"] = Stage{V: 1, On: false, Values: Values{"a_x": 3}}
	d.Source = &Module{Type: "osc.fm", V: 1, On: true, Values: Values{"fm_r": 2}}
	d.Layers = []*Module{kickLayer("kick.b"), {Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}}
	d.FX = []Module{{Type: "fx.filter", V: 1, On: true, Values: Values{"anything": 1}}}
	if err := Validate(d, r); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejects(t *testing.T) {
	r := testRegistry(t)
	cases := []struct {
		name string
		edit func(*Doc)
		want string
	}{
		{"missing required", func(d *Doc) { delete(d.Stages, "fin") }, "stages.fin: required"},
		{"required off", func(d *Doc) { s := d.Stages["core"]; s.On = false; d.Stages["core"] = s }, "cannot be off"},
		{"unknown stage", func(d *Doc) { d.Stages["zz"] = Stage{V: 1, On: true} }, "stages.zz: unknown"},
		{"missing value", func(d *Doc) { d.Stages["a"] = Stage{V: 1, On: true, Values: Values{}} }, "stages.a.values.a_x: missing"},
		{"extra value", func(d *Doc) { d.Stages["core"].Values["zz"] = 1 }, "zz: not a value of core"},
		{"non-finite", func(d *Doc) { d.Stages["core"].Values["core_g"] = math.NaN() }, "core_g: not finite"},
		{"not allowed", func(d *Doc) {
			d.Source = &Module{Type: "osc.wave", V: 1, On: true, Values: Values{"osc_type": 3}}
		}, "osc_type: 3 not in"},
		{"wrong version", func(d *Doc) { s := d.Stages["core"]; s.V = 2; d.Stages["core"] = s }, "version 2"},
		{"layer as source", func(d *Doc) { d.Source = kickLayer("kick.a") }, "unknown source type"},
		{"trailing empty slot", func(d *Doc) { d.Layers = []*Module{kickLayer("kick.a"), nil} }, "trailing empty"},
		{"zero beats", func(d *Doc) { d.Render.Beats = 0 }, "render.beats"},
		{"gate out of range", func(d *Doc) { d.Render.GateTail = 1 }, "render.gate_tail"},
		{"empty id", func(d *Doc) { d.ID = "" }, "id: empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := minimalDoc()
			c.edit(&d)
			err := Validate(d, r)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("want error containing %q, got %v", c.want, err)
			}
		})
	}
}

// Review Focus 1: a kick in slot 2 renders silence in the C engine.
func TestValidateRejectsLayerPastCapacity(t *testing.T) {
	d := minimalDoc()
	d.Layers = []*Module{{Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}, kickLayer("kick.a")}
	err := Validate(d, testRegistry(t))
	if err == nil || !strings.Contains(err.Error(), "only in slots 1..1") {
		t.Fatalf("want a slot-limit error, got %v", err)
	}
}
```

- [ ] **Step 2: Run, fail.**

- [ ] **Step 3: Implement**

```go
package instrument

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// ValidationError lists every problem found, each prefixed by its path.
type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string { return "instrument: " + strings.Join(e.Problems, "; ") }

func finite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// Validate checks d against reg: structure, completeness, finiteness, Allowed
// sets, versions and layer slot limits. It never checks knob ranges (those are
// UI hints). Spec §5.
func Validate(d Doc, reg *Registry) error {
	var p []string
	bad := func(format string, a ...any) { p = append(p, fmt.Sprintf(format, a...)) }
	if d.ID == "" {
		bad("id: empty")
	}
	r := d.Render
	if !finite(r.Beats) || r.Beats <= 0 {
		bad("render.beats: must be a number > 0")
	}
	if !finite(r.Seconds) || r.Seconds < 0 {
		bad("render.seconds: must be a number >= 0")
	}
	if !finite(r.Amplitude) || r.Amplitude < 0 {
		bad("render.amplitude: must be a number >= 0")
	}
	if !finite(r.GateTail) || r.GateTail < 0 || r.GateTail >= 1 {
		bad("render.gate_tail: must be in [0, 1)")
	}
	for _, t := range reg.decls {
		if t.Required {
			if _, ok := d.Stages[t.Type]; !ok {
				bad("stages.%s: required", t.Type)
			}
		}
	}
	for _, key := range sortedKeys(d.Stages) {
		st := d.Stages[key]
		path := "stages." + key
		t, ok := reg.byType[key]
		if !ok || t.Kind != KindStage {
			bad("%s: unknown stage type", path)
			continue
		}
		checkModule(bad, path, t, st.V, st.On, st.Values)
	}
	if m := d.Source; m != nil {
		path := "source(" + m.Type + ")"
		if t, ok := reg.byType[m.Type]; !ok || t.Kind != KindSource {
			bad("%s: unknown source type", path)
		} else {
			checkModule(bad, path, t, m.V, m.On, m.Values)
		}
	}
	for i, m := range d.Layers {
		if m == nil {
			if i == len(d.Layers)-1 {
				bad("layers[%d]: trailing empty slot", i)
			}
			continue
		}
		path := fmt.Sprintf("layers[%d](%s)", i, m.Type)
		t, ok := reg.byType[m.Type]
		if !ok || t.Kind != KindLayer {
			bad("%s: unknown layer type", path)
			continue
		}
		if i >= t.Capacity {
			bad("%s: the engine plays this voice only in slots 1..%d", path, t.Capacity)
		}
		checkModule(bad, path, t, m.V, m.On, m.Values)
	}
	for i, m := range d.FX {
		path := fmt.Sprintf("fx[%d](%s)", i, m.Type)
		if t, ok := reg.byType[m.Type]; !ok || t.Kind != KindFX {
			bad("%s: unknown fx type", path)
		} else {
			checkModule(bad, path, t, m.V, m.On, m.Values)
		}
	}
	if len(p) == 0 {
		return nil
	}
	return &ValidationError{Problems: p}
}

func checkModule(bad func(string, ...any), path string, t *ModuleTypeDecl, v int, on bool, vals Values) {
	if v != t.Version {
		bad("%s: version %d, this build reads version %d", path, v, t.Version)
	}
	if !on && t.Enable == nil && t.Kind != KindFX {
		bad("%s: has no on/off switch, so it cannot be off (remove it instead)", path)
	}
	if t.FreeForm {
		for _, k := range sortedKeys(vals) {
			if !finite(vals[k]) {
				bad("%s.values.%s: not finite", path, k)
			}
		}
		return
	}
	for _, d := range t.Values {
		x, ok := vals[d.Name]
		switch {
		case !ok:
			bad("%s.values.%s: missing", path, d.Name)
		case !finite(x):
			bad("%s.values.%s: not finite", path, d.Name)
		case len(d.Allowed) > 0 && !slices.Contains(d.Allowed, x):
			bad("%s.values.%s: %v not in %v", path, d.Name, x, d.Allowed)
		}
	}
	for _, k := range sortedKeys(vals) {
		if _, ok := t.value(k); !ok {
			bad("%s.values.%s: not a value of %s", path, k, t.Type)
		}
	}
}
```

Note: the `layer as source` case reports `unknown source type`, because the kind is checked before the values.

- [ ] **Step 4: Run, pass.** Step 5: Checkpoint.

---

### Task A4: Flatten, OwnedFields, FieldValues

**Files:**
- Create: `src/go/internal/instrument/flatten.go`
- Test: `src/go/internal/instrument/flatten_test.go`

**Interfaces:**
- Produces:
  - `Flatten(Doc, *Registry) []float64`: a zero block; only present, ON modules write.
  - `OwnedFields(Doc, *Registry) map[string]float64`: every present module's values plus its `Set`/`Global`; its enable field is `Enable.Value` when on, 0 when off.
  - `FieldValues(Doc, *Registry) map[string]float64`: every ABI field's `Init` (0 when none), overlaid by `OwnedFields`.

- [ ] **Step 1: Failing tests**

```go
package instrument

import "testing"

func blockMap(r *Registry, b []float64) map[string]float64 {
	out := map[string]float64{}
	for i, f := range r.Fields() {
		if b[i] != 0 {
			out[f] = b[i]
		}
	}
	return out
}

func TestFlattenWritesOnlyPresentOnModules(t *testing.T) {
	r := testRegistry(t)
	d := minimalDoc()
	d.Stages["a"] = Stage{V: 1, On: false, Values: Values{"a_x": 3}} // off: writes nothing
	d.Source = &Module{Type: "osc.fm", V: 1, On: true, Values: Values{"fm_r": 2}}
	d.Layers = []*Module{kickLayer("kick.b")}
	got := blockMap(r, Flatten(d, r))
	want := map[string]float64{
		"core_g": 1, "fin_y": 0.5,
		"osc_enabled": 1, "osc_type": 4, "fm_r": 2,
		"gen1_source": 5, "gen1_kv": 1, "gen1_kc": 0.3, "kick_on": 1,
	}
	if len(got) != len(want) {
		t.Fatalf("Flatten = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("Flatten[%s] = %v, want %v (all: %v)", k, got[k], v, got)
		}
	}
}

func TestOwnedFieldsReportsOffModulesWithEnableZero(t *testing.T) {
	r := testRegistry(t)
	d := minimalDoc()
	d.Stages["a"] = Stage{V: 1, On: false, Values: Values{"a_x": 3}}
	d.Layers = []*Module{{Type: "kick.a", V: 1, On: false, Values: Values{"kc": 0.9}}}
	o := OwnedFields(d, r)
	if o["a_enabled"] != 0 || o["a_x"] != 3 {
		t.Fatalf("off stage: %v", o)
	}
	if v, ok := o["gen1_source"]; !ok || v != 0 {
		t.Fatalf("off layer must own its switch at 0: %v", o)
	}
	if o["gen1_kc"] != 0.9 || o["gen1_kv"] != 0 || o["kick_on"] != 1 {
		t.Fatalf("off layer keeps values + type constants: %v", o)
	}
}

func TestFieldValuesFillsInitThenOverlays(t *testing.T) {
	r := testRegistry(t)
	d := minimalDoc()
	d.Stages["core"].Values["core_g"] = 9
	fv := FieldValues(d, r)
	if fv["core_g"] != 9 || fv["a_x"] != 8 || fv["a_enabled"] != 0 || fv["gen2_gain"] != 1 || fv["noise_seed"] != 0 {
		t.Fatalf("FieldValues = %v", fv)
	}
	if len(fv) != len(testFields) {
		t.Fatalf("FieldValues must cover every ABI field: %d vs %d", len(fv), len(testFields))
	}
}
```

- [ ] **Step 2: Run, fail.**

- [ ] **Step 3: Implement**

```go
package instrument

// Flatten builds the engine param block: a zero block where each present, ON
// module writes its values, its type's Set/Global constants and its Enable
// field. Nothing else is written — no identity, no defaults. d must be valid.
func Flatten(d Doc, reg *Registry) []float64 {
	b := make([]float64, len(reg.fields))
	put := func(f string, v float64) {
		i, ok := reg.index[f]
		if !ok {
			panic("instrument: Flatten of an unvalidated document: field " + f)
		}
		b[i] = v
	}
	eachModule(d, reg, func(t *ModuleTypeDecl, slot int, on bool, vals Values) {
		if !on {
			return
		}
		name := slotName(slot)
		for _, v := range t.Values {
			put(name(v.Name), vals[v.Name])
		}
		for _, s := range t.Set {
			put(name(s.Field), s.Value)
		}
		for _, g := range t.Global {
			put(g.Field, g.Value)
		}
		if t.Enable != nil {
			put(name(t.Enable.Field), t.Enable.Value)
		}
	})
	return b
}

// OwnedFields reports, by ABI field, everything the document's modules say —
// ON or OFF. A module's enable field reads Enable.Value when on and 0 when off.
// It is the bridge to the flat-field world (UI adapter, tests).
func OwnedFields(d Doc, reg *Registry) map[string]float64 {
	out := map[string]float64{}
	eachModule(d, reg, func(t *ModuleTypeDecl, slot int, on bool, vals Values) {
		name := slotName(slot)
		for _, v := range t.Values {
			out[name(v.Name)] = vals[v.Name]
		}
		for _, s := range t.Set {
			out[name(s.Field)] = s.Value
		}
		for _, g := range t.Global {
			out[g.Field] = g.Value
		}
		if t.Enable != nil {
			if on {
				out[name(t.Enable.Field)] = t.Enable.Value
			} else {
				out[name(t.Enable.Field)] = 0
			}
		}
	})
	return out
}

// FieldValues is the UI's flat view (sub-project 1 adapter): every ABI field's
// creation value, overlaid by OwnedFields. The engine never reads it.
func FieldValues(d Doc, reg *Registry) map[string]float64 {
	out := make(map[string]float64, len(reg.fields))
	for _, f := range reg.fields {
		out[f] = reg.init[f]
	}
	for k, v := range OwnedFields(d, reg) {
		out[k] = v
	}
	return out
}

// eachModule visits every present non-FX module in a fixed order: stages
// (sorted by type), source, layers. slot is -1 except for layers.
func eachModule(d Doc, reg *Registry, fn func(t *ModuleTypeDecl, slot int, on bool, vals Values)) {
	for _, key := range sortedKeys(d.Stages) {
		st := d.Stages[key]
		fn(reg.byType[key], -1, st.On, st.Values)
	}
	if m := d.Source; m != nil {
		fn(reg.byType[m.Type], -1, m.On, m.Values)
	}
	for i, m := range d.Layers {
		if m != nil {
			fn(reg.byType[m.Type], i, m.On, m.Values)
		}
	}
}

func slotName(slot int) func(string) string {
	if slot < 0 {
		return func(f string) string { return f }
	}
	return func(f string) string { return LayerField(slot, f) }
}
```

- [ ] **Step 4: Run, pass.** Step 5: Checkpoint.

---

### Task A5: Edits — SetValue, SetOn, AddStage, SwitchType, SetField, CopyModules

**Files:**
- Create: `src/go/internal/instrument/edit.go`
- Test: `src/go/internal/instrument/edit_test.go`

**Interfaces:**
- Produces:
  - `type Ref struct{ Kind Kind; Stage string; Index int }` with constructors `StageRef(string)`, `SourceRef()`, `LayerRef(int)`, `FXRef(int)`, and `(Ref).String()`
  - errors `ErrUnknownField`, `ErrNotDeclared`, `ErrNotAllowed`, `ErrInactive`, `ErrNoSwitch`, `ErrNotSettable`, `ErrCapacity`, `ErrNoModule`
  - `SetValue(d, reg, ref, name, v) (Doc, error)`
  - `SetOn(d, reg, ref, on) (Doc, error)`
  - `NewModule(reg, typ, on) (Module, error)`
  - `AddStage(d, reg, typ, on) (Doc, error)`
  - `RemoveModule(d, reg, ref) (Doc, error)`
  - `SwitchType(d, reg, ref, typ) (Doc, error)`
  - `SetField(d, reg, field, v) (Doc, error)`
  - `CopyModules(dst, src, reg, refs) (Doc, error)`
- Every function returns a NEW document; the input is never modified.

- [ ] **Step 1: Failing tests**

```go
package instrument

import (
	"errors"
	"testing"
)

func TestSetValueIsImmutableAndChecked(t *testing.T) {
	r := testRegistry(t)
	a := minimalDoc()
	b, err := SetValue(a, r, StageRef("core"), "core_g", 3)
	if err != nil || b.Stages["core"].Values["core_g"] != 3 || a.Stages["core"].Values["core_g"] != 1 {
		t.Fatalf("SetValue: err=%v a=%v b=%v", err, a.Stages["core"], b.Stages["core"])
	}
	if _, err := SetValue(a, r, StageRef("core"), "nope", 1); !errors.Is(err, ErrNotDeclared) {
		t.Fatalf("want ErrNotDeclared, got %v", err)
	}
	if _, err := SetValue(a, r, StageRef("a"), "a_x", 1); !errors.Is(err, ErrNoModule) {
		t.Fatalf("want ErrNoModule for an absent stage, got %v", err)
	}
}

func TestSetOnNeedsASwitch(t *testing.T) {
	r := testRegistry(t)
	if _, err := SetOn(minimalDoc(), r, StageRef("core"), false); !errors.Is(err, ErrNoSwitch) {
		t.Fatalf("want ErrNoSwitch, got %v", err)
	}
}

// Review Focus 2.
func TestSetFieldMaterialisesAbsentStageOff(t *testing.T) {
	r := testRegistry(t)
	d, err := SetField(minimalDoc(), r, "a_x", 5)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := d.Stages["a"]
	if !ok || st.On || st.Values["a_x"] != 5 {
		t.Fatalf("stage a = %+v (present %v)", st, ok)
	}
	if blockMap(r, Flatten(d, r))["a_x"] != 0 {
		t.Fatal("an OFF materialised stage must not reach the engine")
	}
	d, _ = SetField(d, r, "a_enabled", 1)
	if !d.Stages["a"].On {
		t.Fatal("a_enabled=1 must switch the stage on")
	}
}

func TestSetFieldSwitchesSourceType(t *testing.T) {
	r := testRegistry(t)
	d, err := SetField(minimalDoc(), r, "osc_enabled", 1) // absent → first source type, on
	if err != nil || d.Source == nil || d.Source.Type != "osc.wave" || !d.Source.On {
		t.Fatalf("osc_enabled: %+v %v", d.Source, err)
	}
	d, _ = SetField(d, r, "osc_type", 1)
	if d.Source.Type != "osc.wave" || d.Source.Values["osc_type"] != 1 {
		t.Fatalf("osc_type 1 stays a wave value: %+v", d.Source)
	}
	d, err = SetField(d, r, "osc_type", 4)
	if err != nil || d.Source.Type != "osc.fm" || d.Source.Values["fm_r"] != 1 || !d.Source.On {
		t.Fatalf("osc_type 4 must switch to osc.fm from Init, keeping on: %+v %v", d.Source, err)
	}
	if _, err := SetField(d, r, "osc_type", 9); !errors.Is(err, ErrNotAllowed) {
		t.Fatalf("want ErrNotAllowed, got %v", err)
	}
}

func TestSetFieldOnLayers(t *testing.T) {
	r := testRegistry(t)
	d, err := SetField(minimalDoc(), r, "gen1_source", 5) // kick family: first declared variant
	if err != nil || d.Layers[0].Type != "kick.a" || !d.Layers[0].On {
		t.Fatalf("gen1_source=5: %+v %v", d.Layers, err)
	}
	d, _ = SetField(d, r, "gen1_kc", 0.7)
	d, err = SetField(d, r, "gen1_kv", 1) // a variant selector switches type, carrying kc
	if err != nil || d.Layers[0].Type != "kick.b" || d.Layers[0].Values["kc"] != 0.7 {
		t.Fatalf("gen1_kv=1: %+v %v", d.Layers[0], err)
	}
	d, _ = SetField(d, r, "gen1_source", 0)
	if d.Layers[0].On {
		t.Fatal("gen1_source=0 switches the layer off")
	}
	if _, err := SetField(d, r, "gen2_gain", 1); !errors.Is(err, ErrInactive) {
		t.Fatalf("a value of an empty slot is inactive, got %v", err)
	}
	if _, err := SetField(d, r, "kick_on", 0); !errors.Is(err, ErrNotSettable) {
		t.Fatalf("a Global constant is not settable, got %v", err)
	}
	if _, err := SetField(d, r, "zzz", 0); !errors.Is(err, ErrUnknownField) {
		t.Fatalf("want ErrUnknownField, got %v", err)
	}
}

// Review Focus 1.
func TestSwitchTypeRespectsCapacity(t *testing.T) {
	r := testRegistry(t)
	d := minimalDoc()
	d.Layers = []*Module{{Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}, {Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}}
	if _, err := SwitchType(d, r, LayerRef(1), "kick.a"); !errors.Is(err, ErrCapacity) {
		t.Fatalf("want ErrCapacity, got %v", err)
	}
}

// Review Focus 1.
func TestCopyModulesRespectsCapacity(t *testing.T) {
	r := testRegistry(t)
	src := minimalDoc()
	src.Layers = []*Module{kickLayer("kick.a")}
	src.Stages["a"] = Stage{V: 1, On: true, Values: Values{"a_x": 2}}
	dst, err := CopyModules(minimalDoc(), src, r, []Ref{StageRef("a"), LayerRef(0)})
	if err != nil || dst.Stages["a"].Values["a_x"] != 2 || dst.Layers[0].Type != "kick.a" {
		t.Fatalf("copy: %+v %v", dst, err)
	}
	if err := Validate(dst, r); err != nil {
		t.Fatal(err)
	}
	src.Layers = []*Module{{Type: "tone", V: 1, On: true, Values: Values{"gain": 1}}, kickLayer("kick.a")}
	if _, err := CopyModules(minimalDoc(), src, r, []Ref{LayerRef(1)}); !errors.Is(err, ErrCapacity) {
		t.Fatalf("want ErrCapacity, got %v", err)
	}
}

func TestRemoveRequiredStageFails(t *testing.T) {
	if _, err := RemoveModule(minimalDoc(), testRegistry(t), StageRef("core")); err == nil {
		t.Fatal("removing a required stage must fail")
	}
}
```

- [ ] **Step 2: Run, fail.**

- [ ] **Step 3: Implement `edit.go`**

```go
package instrument

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
)

var (
	ErrUnknownField = errors.New("not an engine field")
	ErrNotDeclared  = errors.New("not a value of this module type")
	ErrNotAllowed   = errors.New("value not allowed")
	ErrInactive     = errors.New("belongs to a module the instrument does not have")
	ErrNoSwitch     = errors.New("module type has no on/off switch")
	ErrNotSettable  = errors.New("is a constant of the module type")
	ErrCapacity     = errors.New("the engine does not play this voice in that slot")
	ErrNoModule     = errors.New("no module there")
)

// Ref addresses one module in a document.
type Ref struct {
	Kind  Kind
	Stage string // KindStage
	Index int    // KindLayer / KindFX
}

func StageRef(t string) Ref { return Ref{Kind: KindStage, Stage: t} }
func SourceRef() Ref        { return Ref{Kind: KindSource} }
func LayerRef(i int) Ref    { return Ref{Kind: KindLayer, Index: i} }
func FXRef(i int) Ref       { return Ref{Kind: KindFX, Index: i} }

func (r Ref) String() string {
	switch r.Kind {
	case KindStage:
		return "stages." + r.Stage
	case KindSource:
		return "source"
	case KindLayer:
		return fmt.Sprintf("layers[%d]", r.Index)
	case KindFX:
		return fmt.Sprintf("fx[%d]", r.Index)
	}
	return "?"
}

// moduleAt returns a copy of the module at ref (a stage reports Type = key).
func moduleAt(d Doc, ref Ref) (Module, bool) {
	switch ref.Kind {
	case KindStage:
		s, ok := d.Stages[ref.Stage]
		return Module{Type: ref.Stage, V: s.V, On: s.On, Values: s.Values, Notes: s.Notes}, ok
	case KindSource:
		if d.Source == nil {
			return Module{}, false
		}
		return *d.Source, true
	case KindLayer:
		if ref.Index < 0 || ref.Index >= len(d.Layers) || d.Layers[ref.Index] == nil {
			return Module{}, false
		}
		return *d.Layers[ref.Index], true
	case KindFX:
		if ref.Index < 0 || ref.Index >= len(d.FX) {
			return Module{}, false
		}
		return d.FX[ref.Index], true
	}
	return Module{}, false
}

// withModule returns a clone of d with ref replaced by m (nil removes it).
func withModule(d Doc, ref Ref, m *Module) Doc {
	out := d.Clone()
	switch ref.Kind {
	case KindStage:
		if m == nil {
			delete(out.Stages, ref.Stage)
		} else {
			out.Stages[ref.Stage] = Stage{V: m.V, On: m.On, Values: m.Values.clone(), Notes: m.Notes}
		}
	case KindSource:
		if m == nil {
			out.Source = nil
		} else {
			c := *m
			c.Values = c.Values.clone()
			out.Source = &c
		}
	case KindLayer:
		for len(out.Layers) <= ref.Index {
			out.Layers = append(out.Layers, nil)
		}
		if m == nil {
			out.Layers[ref.Index] = nil
		} else {
			c := *m
			c.Values = c.Values.clone()
			out.Layers[ref.Index] = &c
		}
		for len(out.Layers) > 0 && out.Layers[len(out.Layers)-1] == nil {
			out.Layers = out.Layers[:len(out.Layers)-1]
		}
	case KindFX:
		if m == nil {
			out.FX = slices.Delete(out.FX, ref.Index, ref.Index+1)
		} else {
			c := *m
			c.Values = c.Values.clone()
			out.FX[ref.Index] = c
		}
	}
	return out
}

func typeOf(reg *Registry, m Module) (*ModuleTypeDecl, error) {
	t, ok := reg.byType[m.Type]
	if !ok {
		return nil, fmt.Errorf("unknown module type %q", m.Type)
	}
	return t, nil
}

// SetValue sets one value of the module at ref.
func SetValue(d Doc, reg *Registry, ref Ref, name string, v float64) (Doc, error) {
	m, ok := moduleAt(d, ref)
	if !ok {
		return d, fmt.Errorf("%s: %w", ref, ErrNoModule)
	}
	t, err := typeOf(reg, m)
	if err != nil {
		return d, err
	}
	if !finite(v) {
		return d, fmt.Errorf("%s.values.%s: %w (not finite)", ref, name, ErrNotAllowed)
	}
	if !t.FreeForm {
		vd, ok := t.value(name)
		if !ok {
			return d, fmt.Errorf("%s.values.%s: %w", ref, name, ErrNotDeclared)
		}
		if len(vd.Allowed) > 0 && !slices.Contains(vd.Allowed, v) {
			return d, fmt.Errorf("%s.values.%s=%v: %w", ref, name, v, ErrNotAllowed)
		}
	}
	m.Values = m.Values.clone()
	if m.Values == nil {
		m.Values = Values{}
	}
	m.Values[name] = v
	return withModule(d, ref, &m), nil
}

// SetOn switches the module at ref on or off.
func SetOn(d Doc, reg *Registry, ref Ref, on bool) (Doc, error) {
	m, ok := moduleAt(d, ref)
	if !ok {
		return d, fmt.Errorf("%s: %w", ref, ErrNoModule)
	}
	t, err := typeOf(reg, m)
	if err != nil {
		return d, err
	}
	if t.Enable == nil && t.Kind != KindFX {
		return d, fmt.Errorf("%s: %w", ref, ErrNoSwitch)
	}
	m.On = on
	return withModule(d, ref, &m), nil
}

// NewModule creates a module of typ from its declared creation values.
func NewModule(reg *Registry, typ string, on bool) (Module, error) {
	t, ok := reg.byType[typ]
	if !ok {
		return Module{}, fmt.Errorf("unknown module type %q", typ)
	}
	vals := Values{}
	for _, v := range t.Values {
		vals[v.Name] = v.Init
	}
	if t.Enable == nil && t.Kind != KindFX {
		on = true // a module without a switch is on by being present
	}
	return Module{Type: typ, V: t.Version, On: on, Values: vals}, nil
}

// AddStage adds an absent stage from its creation values.
func AddStage(d Doc, reg *Registry, typ string, on bool) (Doc, error) {
	if _, ok := d.Stages[typ]; ok {
		return d, fmt.Errorf("stages.%s: already present", typ)
	}
	t, ok := reg.byType[typ]
	if !ok || t.Kind != KindStage {
		return d, fmt.Errorf("stages.%s: unknown stage type", typ)
	}
	m, _ := NewModule(reg, typ, on)
	return withModule(d, StageRef(typ), &m), nil
}

// RemoveModule removes the module at ref (required stages cannot be removed).
func RemoveModule(d Doc, reg *Registry, ref Ref) (Doc, error) {
	if ref.Kind == KindStage {
		if t, ok := reg.byType[ref.Stage]; ok && t.Required {
			return d, fmt.Errorf("%s: required", ref)
		}
	}
	if _, ok := moduleAt(d, ref); !ok {
		return d, fmt.Errorf("%s: %w", ref, ErrNoModule)
	}
	return withModule(d, ref, nil), nil
}

// SwitchType replaces the source or layer at ref with a module of typ. Values
// the new type shares by name are carried over; the rest start at Init. The
// on/off state is kept (an absent module becomes an OFF one).
func SwitchType(d Doc, reg *Registry, ref Ref, typ string) (Doc, error) {
	t, ok := reg.byType[typ]
	if !ok || t.Kind != ref.Kind || (ref.Kind != KindSource && ref.Kind != KindLayer) {
		return d, fmt.Errorf("%s: cannot hold type %q", ref, typ)
	}
	if ref.Kind == KindLayer && ref.Index >= t.Capacity {
		return d, fmt.Errorf("%s(%s): %w (slots 1..%d)", ref, typ, ErrCapacity, t.Capacity)
	}
	old, had := moduleAt(d, ref)
	m, _ := NewModule(reg, typ, had && old.On)
	if had {
		for k := range m.Values {
			if v, ok := old.Values[k]; ok {
				m.Values[k] = v
			}
		}
		m.Notes = old.Notes
	}
	return withModule(d, ref, &m), nil
}

var layerFieldRe = regexp.MustCompile(`^gen(\d+)_(.+)$`)

// SetField is the sub-project-1 adapter from a flat ABI field name to a
// document edit (spec §5). Deleted with the module-grouped Synth tab.
func SetField(d Doc, reg *Registry, field string, v float64) (Doc, error) {
	if !finite(v) {
		return d, fmt.Errorf("%s: %w (not finite)", field, ErrNotAllowed)
	}
	if m := layerFieldRe.FindStringSubmatch(field); m != nil {
		slot, _ := strconv.Atoi(m[1])
		if slot >= 1 && slot <= reg.slots {
			return setLayerField(d, reg, slot-1, m[2], v)
		}
	}
	for _, t := range reg.decls {
		if t.Kind != KindStage {
			continue
		}
		if t.Enable != nil && t.Enable.Field == field {
			if _, present := d.Stages[t.Type]; !present {
				if v < 0.5 {
					return d, nil
				}
				return AddStage(d, reg, t.Type, true)
			}
			return SetOn(d, reg, StageRef(t.Type), v >= 0.5)
		}
		if _, ok := t.value(field); ok {
			if _, present := d.Stages[t.Type]; !present {
				nd, err := AddStage(d, reg, t.Type, false)
				if err != nil {
					return d, err
				}
				d = nd
			}
			return SetValue(d, reg, StageRef(t.Type), field, v)
		}
	}
	if handled, nd, err := setSourceField(d, reg, field, v); handled {
		return nd, err
	}
	for _, t := range reg.decls {
		for _, g := range t.Global {
			if g.Field == field {
				return d, fmt.Errorf("%s: %w", field, ErrNotSettable)
			}
		}
	}
	return d, fmt.Errorf("%q: %w", field, ErrUnknownField)
}

func setSourceField(d Doc, reg *Registry, field string, v float64) (bool, Doc, error) {
	var sources []*ModuleTypeDecl
	for _, t := range reg.decls {
		if t.Kind == KindSource {
			sources = append(sources, t)
		}
	}
	for _, t := range sources {
		if t.Enable != nil && t.Enable.Field == field {
			if d.Source == nil {
				if v < 0.5 {
					return true, d, nil
				}
				nd, err := SwitchType(d, reg, SourceRef(), sources[0].Type)
				if err != nil {
					return true, d, err
				}
				nd, err = SetOn(nd, reg, SourceRef(), true)
				return true, nd, err
			}
			nd, err := SetOn(d, reg, SourceRef(), v >= 0.5)
			return true, nd, err
		}
	}
	selects := func(t *ModuleTypeDecl) bool {
		if sv, ok := t.setValue(field); ok {
			return sv == v
		}
		vd, ok := t.value(field)
		return ok && len(vd.Allowed) > 0 && slices.Contains(vd.Allowed, v)
	}
	isSelector := false
	for _, t := range sources {
		if _, ok := t.setValue(field); ok {
			isSelector = true
		}
		if vd, ok := t.value(field); ok && len(vd.Allowed) > 0 {
			isSelector = true
		}
	}
	if isSelector {
		if d.Source != nil {
			cur := reg.byType[d.Source.Type]
			if vd, ok := cur.value(field); ok && (len(vd.Allowed) == 0 || slices.Contains(vd.Allowed, v)) {
				nd, err := SetValue(d, reg, SourceRef(), field, v)
				return true, nd, err
			}
		}
		for _, t := range sources {
			if !selects(t) {
				continue
			}
			nd, err := SwitchType(d, reg, SourceRef(), t.Type)
			if err != nil {
				return true, d, err
			}
			if _, ok := t.value(field); ok {
				nd, err = SetValue(nd, reg, SourceRef(), field, v)
			}
			return true, nd, err
		}
		return true, d, fmt.Errorf("%s=%v: %w", field, v, ErrNotAllowed)
	}
	for _, t := range sources {
		if _, ok := t.value(field); ok {
			if d.Source != nil {
				if _, mine := reg.byType[d.Source.Type].value(field); mine {
					nd, err := SetValue(d, reg, SourceRef(), field, v)
					return true, nd, err
				}
			}
			return true, d, fmt.Errorf("%s: %w", field, ErrInactive)
		}
	}
	return false, d, nil
}

func setLayerField(d Doc, reg *Registry, slot int, name string, v float64) (Doc, error) {
	var cur *Module
	if slot < len(d.Layers) {
		cur = d.Layers[slot]
	}
	ref := LayerRef(slot)
	var layers []*ModuleTypeDecl
	for _, t := range reg.decls {
		if t.Kind == KindLayer {
			layers = append(layers, t)
		}
	}
	for _, t := range layers {
		if t.Enable.Field != name {
			continue
		}
		if v < 0.5 {
			if cur == nil {
				return d, nil
			}
			return SetOn(d, reg, ref, false)
		}
		if cur != nil && reg.byType[cur.Type].Enable.Value == v {
			return SetOn(d, reg, ref, true)
		}
		for _, u := range layers {
			if u.Enable.Value == v {
				nd, err := SwitchType(d, reg, ref, u.Type)
				if err != nil {
					return d, err
				}
				return SetOn(nd, reg, ref, true)
			}
		}
		return d, fmt.Errorf("%s=%v: %w", LayerField(slot, name), v, ErrNotAllowed)
	}
	if cur == nil {
		for _, t := range layers {
			if _, ok := t.value(name); ok {
				return d, fmt.Errorf("%s: %w", LayerField(slot, name), ErrInactive)
			}
			if _, ok := t.setValue(name); ok {
				return d, fmt.Errorf("%s: %w", LayerField(slot, name), ErrInactive)
			}
		}
		return d, fmt.Errorf("%q: %w", LayerField(slot, name), ErrUnknownField)
	}
	t := reg.byType[cur.Type]
	if _, ok := t.value(name); ok {
		return SetValue(d, reg, ref, name, v)
	}
	if sv, ok := t.setValue(name); ok {
		if sv == v {
			return d, nil
		}
		for _, u := range layers {
			if u.Enable.Value == t.Enable.Value {
				if uv, ok := u.setValue(name); ok && uv == v {
					return SwitchType(d, reg, ref, u.Type)
				}
			}
		}
		return d, fmt.Errorf("%s=%v: %w", LayerField(slot, name), v, ErrNotAllowed)
	}
	return d, fmt.Errorf("%s: %w", LayerField(slot, name), ErrInactive)
}

// CopyModules copies the modules at refs from src into dst at the same place
// (sub-project 4's backend). Layer copies respect the slot limit.
func CopyModules(dst, src Doc, reg *Registry, refs []Ref) (Doc, error) {
	out := dst
	for _, ref := range refs {
		m, ok := moduleAt(src, ref)
		if !ok {
			return dst, fmt.Errorf("source %s: %w", ref, ErrNoModule)
		}
		t, err := typeOf(reg, m)
		if err != nil {
			return dst, err
		}
		if ref.Kind == KindLayer && ref.Index >= t.Capacity {
			return dst, fmt.Errorf("%s(%s): %w", ref, m.Type, ErrCapacity)
		}
		if ref.Kind == KindFX {
			c := out.Clone()
			mm := m
			mm.Values = mm.Values.clone()
			c.FX = append(c.FX, mm)
			out = c
			continue
		}
		out = withModule(out, ref, &m)
	}
	return out, nil
}
```

- [ ] **Step 4: Run, pass.** Step 5: Checkpoint.

---

### Task A6: Diff, hashes, Migrate

**Files:**
- Create: `src/go/internal/instrument/diff.go`, `src/go/internal/instrument/migrate.go`
- Test: `src/go/internal/instrument/diff_test.go`, `src/go/internal/instrument/migrate_test.go`
- Modify: `src/go/internal/instrument/doc_test.go`, switching `TestFileRoundTrip` to `Diff`

**Interfaces:**
- Produces: `Diff(a, b Doc) []string`, `BlockHash([]float64) uint64`, `ContentHash(Doc) string` (`"sha256:"` + 16 hex; ignores `ID` and `Origin`), `Migrate(Doc, *Registry) (Doc, error)`.

- [ ] **Step 1: Failing tests**

```go
// diff_test.go
package instrument

import (
	"slices"
	"strings"
	"testing"
)

func TestDiffNamesChangedPaths(t *testing.T) {
	r := testRegistry(t)
	a := minimalDoc()
	b, _ := SetValue(a, r, StageRef("core"), "core_g", 2)
	b, _ = AddStage(b, r, "a", true)
	b.Render.Beats = 2
	got := Diff(a, b)
	want := []string{"render", "stages.a", "stages.core.values.core_g"}
	if !slices.Equal(got, want) {
		t.Fatalf("Diff = %v, want %v", got, want)
	}
	if len(Diff(a, a.Clone())) != 0 {
		t.Fatal("a clone differs from its original")
	}
}

func TestContentHashIgnoresIdentity(t *testing.T) {
	a := minimalDoc()
	b := a.Clone()
	b.ID, b.Origin = "other", "kick"
	if ContentHash(a) != ContentHash(b) {
		t.Fatal("ID/Origin must not change the content hash")
	}
	b.Stages["core"].Values["core_g"] = 2
	if ContentHash(a) == ContentHash(b) {
		t.Fatal("a value change must change the content hash")
	}
	if !strings.HasPrefix(ContentHash(a), "sha256:") || len(ContentHash(a)) != len("sha256:")+16 {
		t.Fatal(ContentHash(a))
	}
}

func TestBlockHashSeesEveryBit(t *testing.T) {
	a := []float64{0, 1, 2}
	b := []float64{0, 1, 2.0000000000000004}
	if BlockHash(a) == BlockHash(b) || BlockHash(a) != BlockHash([]float64{0, 1, 2}) {
		t.Fatal("BlockHash must be exact and deterministic")
	}
}
```

```go
// migrate_test.go
package instrument

import (
	"strings"
	"testing"
)

func TestMigrateAppliesStepsInOrder(t *testing.T) {
	decls := testDecls()
	decls[0].Version = 3
	decls[0].Steps = []Step{
		func(v Values) Values { v["core_g"] *= 10; return v },
		func(v Values) Values { v["core_g"]++; return v },
	}
	r, err := NewRegistry(decls, testFields, 2)
	if err != nil {
		t.Fatal(err)
	}
	d := minimalDoc() // core at v1, core_g = 1
	out, err := Migrate(d, r)
	if err != nil {
		t.Fatal(err)
	}
	if out.Stages["core"].V != 3 || out.Stages["core"].Values["core_g"] != 11 {
		t.Fatalf("migrated core = %+v", out.Stages["core"])
	}
	if d.Stages["core"].Values["core_g"] != 1 {
		t.Fatal("Migrate mutated its input")
	}
}

func TestMigrateRejectsNewerVersion(t *testing.T) {
	d := minimalDoc()
	s := d.Stages["core"]
	s.V = 5
	d.Stages["core"] = s
	_, err := Migrate(d, testRegistry(t))
	if err == nil || !strings.Contains(err.Error(), "newer than this build") {
		t.Fatalf("want a newer-version error, got %v", err)
	}
}
```

- [ ] **Step 2: Run, fail.**

- [ ] **Step 3: Implement**

```go
// diff.go
package instrument

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
)

// Diff returns the sorted paths where a and b differ. A module present on one
// side only is reported by its own path; otherwise its type/on/v and changed
// values are listed.
func Diff(a, b Doc) []string {
	var out []string
	add := func(p string) { out = append(out, p) }
	if a.Name != b.Name {
		add("name")
	}
	if a.Category != b.Category {
		add("category")
	}
	if a.Notes != b.Notes {
		add("notes")
	}
	if a.Render != b.Render {
		add("render")
	}
	keys := map[string]bool{}
	for k := range a.Stages {
		keys[k] = true
	}
	for k := range b.Stages {
		keys[k] = true
	}
	for _, k := range sortedKeys(keys) {
		sa, oka := a.Stages[k]
		sb, okb := b.Stages[k]
		if oka != okb {
			add("stages." + k)
			continue
		}
		out = append(out, diffModule("stages."+k,
			Module{Type: k, V: sa.V, On: sa.On, Values: sa.Values, Notes: sa.Notes},
			Module{Type: k, V: sb.V, On: sb.On, Values: sb.Values, Notes: sb.Notes})...)
	}
	switch {
	case (a.Source == nil) != (b.Source == nil):
		add("source")
	case a.Source != nil:
		out = append(out, diffModule("source", *a.Source, *b.Source)...)
	}
	for i := 0; i < max(len(a.Layers), len(b.Layers)); i++ {
		var la, lb *Module
		if i < len(a.Layers) {
			la = a.Layers[i]
		}
		if i < len(b.Layers) {
			lb = b.Layers[i]
		}
		p := fmt.Sprintf("layers[%d]", i)
		switch {
		case (la == nil) != (lb == nil):
			add(p)
		case la != nil:
			out = append(out, diffModule(p, *la, *lb)...)
		}
	}
	if len(a.FX) != len(b.FX) {
		add("fx")
	} else {
		for i := range a.FX {
			out = append(out, diffModule(fmt.Sprintf("fx[%d]", i), a.FX[i], b.FX[i])...)
		}
	}
	sort.Strings(out)
	return out
}

func diffModule(path string, a, b Module) []string {
	if a.Type != b.Type {
		return []string{path + ".type"}
	}
	var out []string
	if a.On != b.On {
		out = append(out, path+".on")
	}
	if a.V != b.V {
		out = append(out, path+".v")
	}
	if a.Notes != b.Notes {
		out = append(out, path+".notes")
	}
	names := map[string]bool{}
	for k := range a.Values {
		names[k] = true
	}
	for k := range b.Values {
		names[k] = true
	}
	for _, k := range sortedKeys(names) {
		va, oka := a.Values[k]
		vb, okb := b.Values[k]
		if oka != okb || va != vb {
			out = append(out, path+".values."+k)
		}
	}
	return out
}

// BlockHash is the voice-cache key of a flattened block: FNV-1a 64 over the
// exact float64 bits, little endian.
func BlockHash(block []float64) uint64 {
	h := fnv.New64a()
	var buf [8]byte
	for _, x := range block {
		binary.LittleEndian.PutUint64(buf[:], math.Float64bits(x))
		h.Write(buf[:])
	}
	return h.Sum64()
}

// ContentHash identifies a document's content (not its ID or Origin):
// "sha256:" + the first 16 hex digits over its canonical JSON. encoding/json
// writes struct fields in declaration order and map keys sorted, so the
// encoding is canonical for a given Doc value.
func ContentHash(d Doc) string {
	d.ID, d.Origin = "", ""
	b, err := json.Marshal(d)
	if err != nil {
		panic("instrument: ContentHash of an unmarshalable document: " + err.Error())
	}
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:8])
}
```

```go
// migrate.go
package instrument

import (
	"errors"
	"fmt"
)

// Migrate brings every module to its type's current version by applying the
// type's Steps in order (spec §9). Unknown types are left for Validate. A
// version newer than this build is an error naming the path. d is not changed.
func Migrate(d Doc, reg *Registry) (Doc, error) {
	out := d.Clone()
	var errs []error
	step := func(path, typ string, v *int, vals *Values) {
		t, ok := reg.byType[typ]
		if !ok {
			return
		}
		switch {
		case *v > t.Version:
			errs = append(errs, fmt.Errorf("%s: version %d is newer than this build (%d)", path, *v, t.Version))
			return
		case *v < 1:
			errs = append(errs, fmt.Errorf("%s: version %d < 1", path, *v))
			return
		}
		for *v < t.Version {
			*vals = t.Steps[*v-1](vals.clone())
			*v++
		}
	}
	for _, k := range sortedKeys(out.Stages) {
		st := out.Stages[k]
		step("stages."+k, k, &st.V, &st.Values)
		out.Stages[k] = st
	}
	if out.Source != nil {
		step("source", out.Source.Type, &out.Source.V, &out.Source.Values)
	}
	for i, m := range out.Layers {
		if m != nil {
			step(fmt.Sprintf("layers[%d]", i), m.Type, &m.V, &m.Values)
		}
	}
	for i := range out.FX {
		step(fmt.Sprintf("fx[%d]", i), out.FX[i].Type, &out.FX[i].V, &out.FX[i].Values)
	}
	return out, errors.Join(errs...)
}
```

In `doc_test.go` replace the `reflect.DeepEqual` comparison with `Diff` as written in A1.

- [ ] **Step 4: Run the whole package, pass.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/instrument/ && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./internal/instrument/`

- [ ] **Step 5: Checkpoint** — "A1–A6: internal/instrument green".

---

### Task A7: Built-in declarations and the registry

**Files:**
- Create: `src/go/internal/audio/module_decls.go`, `src/go/internal/audio/module_registry.go`
- Test: `src/go/internal/audio/module_registry_test.go` (fast path, no build tag)

**Interfaces:**
- Consumes: `instrument.*` (A2), `ModularSynthParamDefs()` (modular_recipe.go:16), `ModularParamSchema()` (synth_param_schema.go:70), `modularGenSlots` (synth_param_schema.go:27), `modularFields` (modular_fields.go:30).
- Produces: `BuiltinModuleRegistry() *instrument.Registry` (memoised, panics on an invalid table) and `builtinModuleDecls() []instrument.ModuleTypeDecl`.

The value lists below come from what each C function actually reads (`modular.c` / `modular_stages.c`, the per-function `p->` reads). Task A9's liveness test is the judge; this table is the starting point.

- [ ] **Step 1: Failing guard tests**

```go
package audio

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// Every ABI field is named by at least one module type (value, Set, Global
// or Enable) — except noise_seed, which no caller ever sets (spec §3).
func TestEveryABIFieldIsDeclared(t *testing.T) {
	reg := BuiltinModuleRegistry()
	named := map[string]bool{"noise_seed": true}
	for _, ty := range reg.Types() {
		slots := []int{-1}
		if ty.Kind == instrument.KindLayer {
			slots = slots[:0]
			for s := 0; s < ty.Capacity; s++ {
				slots = append(slots, s)
			}
		}
		for _, s := range slots {
			n := func(f string) string {
				if s < 0 {
					return f
				}
				return instrument.LayerField(s, f)
			}
			for _, v := range ty.Values {
				named[n(v.Name)] = true
			}
			for _, x := range ty.Set {
				named[n(x.Field)] = true
			}
			if ty.Enable != nil {
				named[n(ty.Enable.Field)] = true
			}
			for _, g := range ty.Global {
				named[g.Field] = true
			}
		}
	}
	for _, f := range ModularParamSchema() {
		if !named[f] {
			t.Errorf("ABI field %s belongs to no module type", f)
		}
	}
}

// A layer type's capacity is the ABI's slot count for its family block.
func TestLayerCapacityMatchesTheABI(t *testing.T) {
	reg := BuiltinModuleRegistry()
	for _, ty := range reg.Types() {
		if ty.Kind != instrument.KindLayer {
			continue
		}
		for s := 0; s < reg.Slots(); s++ {
			inABI := true
			for _, v := range ty.Values {
				if _, ok := reg.FieldIndex(instrument.LayerField(s, v.Name)); !ok {
					inABI = false
				}
			}
			if inABI != (s < ty.Capacity) {
				t.Errorf("%s: slot %d in ABI=%v but capacity=%d", ty.Type, s+1, inABI, ty.Capacity)
			}
		}
	}
}

// Every value has a ParamDef (its Init and, later, its UI metadata).
func TestEveryModuleValueHasAParamDef(t *testing.T) {
	defs := map[string]bool{}
	for _, d := range ModularSynthParamDefs() {
		defs[d.Name] = true
	}
	for _, ty := range BuiltinModuleRegistry().Types() {
		for _, v := range ty.Values {
			name := v.Name
			if ty.Kind == instrument.KindLayer {
				name = instrument.LayerField(0, v.Name)
			}
			if ty.Kind != instrument.KindFX && !defs[name] {
				t.Errorf("%s.%s has no ParamDef", ty.Type, v.Name)
			}
		}
	}
}

func TestFXTypesCoverEveryEffect(t *testing.T) {
	reg := BuiltinModuleRegistry()
	for _, et := range builtinEffectTypes {
		if _, ok := reg.Type("fx." + string(et)); !ok {
			t.Errorf("no module type for effect %s", et)
		}
	}
	if !slices.Contains(builtinEffectTypes, EffectFilter) {
		t.Fatal("effect list incomplete")
	}
}
```

- [ ] **Step 2: Run, fail.**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestEveryABIFieldIsDeclared|TestLayerCapacityMatchesTheABI|TestEveryModuleValueHasAParamDef|TestFXTypesCoverEveryEffect' ./internal/audio/`

- [ ] **Step 3: Implement `module_decls.go`**

```go
package audio

import "github.com/ingyamilmolinar/beatmo/internal/instrument"

// module_decls.go — THE built-in module types (spec §4). A type groups the ABI
// fields one engine stage / source / voice reads; its values ARE ABI field
// names (slot-relative for layers). A voice variant is its own type only when
// it runs a different C function (kick/snare/cymbal); a variant that only picks
// constants from a table stays a value (tom_variant, fm_variant).
//
// Adding a synth step or voice: append one declaration here, write the C that
// reads its fields, `make gen-synth-abi`. No existing document changes.

func vals(names ...string) []instrument.ValueDecl {
	out := make([]instrument.ValueDecl, len(names))
	for i, n := range names {
		out[i] = instrument.ValueDecl{Name: n}
	}
	return out
}

func sw(field string, v float64) *instrument.FieldSet { return &instrument.FieldSet{Field: field, Value: v} }

func set(field string, v float64) []instrument.FieldSet {
	return []instrument.FieldSet{{Field: field, Value: v}}
}

func pre(prefix string, names ...string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = prefix + n
	}
	return out
}

// slotsOf is the ABI slot count of a modularFields block — a family voice's
// capacity is whatever its arrays hold, never a hand-typed number.
func slotsOf(block string) int {
	for _, f := range modularFields {
		if f.Block == block {
			return f.Slots
		}
	}
	panic("slotsOf: no block " + block)
}

var builtinEffectTypes = []EffectType{
	EffectDistortion, EffectDelay, EffectReverb, EffectChorus, EffectBitcrusher, EffectFilter,
	EffectPhaser, EffectFlanger, EffectTremolo, EffectGate, EffectLimiter, EffectRingMod,
	EffectWaveshaper, EffectAutoWah, EffectCompressor, EffectTransient, EffectTape, EffectPitchShift,
}

func builtinModuleDecls() []instrument.ModuleTypeDecl {
	const (
		st  = instrument.KindStage
		src = instrument.KindSource
		ly  = instrument.KindLayer
	)
	d := []instrument.ModuleTypeDecl{
		// ── stages ──
		{Type: "core", Kind: st, Required: true, Values: vals("pitch", "osc_octave", "osc_detune", "voice_freq_hz", "gain", "noise_draws", "noise_prelude")},
		{Type: "amp_env", Kind: st, Enable: sw("env_enabled", 1), Values: vals("amp_attack", "amp_decay", "amp_sustain", "amp_release", "amp_curve")},
		{Type: "filter", Kind: st, Enable: sw("filter_enabled", 1), Values: vals("filter_type", "filter_cutoff", "filter_resonance")},
		{Type: "drive", Kind: st, Enable: sw("drive_enabled", 1), Values: vals("drive")},
		{Type: "pitch_env", Kind: st, Enable: sw("pitchenv_enabled", 1), Values: vals("pitchenv_amt", "pitchenv_decay")},
		{Type: "lfo", Kind: st, Enable: sw("lfo_enabled", 1), Values: vals("lfo_rate", "lfo_depth", "lfo_target", "lfo_delay")},
		{Type: "burst", Kind: st, Enable: sw("burst_enabled", 1), Values: vals("burst_sharp",
			"burst1_off", "burst1_amp", "burst2_off", "burst2_amp", "burst3_off", "burst3_amp", "burst4_off", "burst4_amp")},
		{Type: "filter_env", Kind: st, Enable: sw("filtenv_enabled", 1), Values: vals("filtenv_amt", "filtenv_decay", "filtenv_attack")},
		{Type: "unison", Kind: st, Values: vals("unison_voices", "unison_detune", "unison_mix", "unison_drift_rate", "unison_drift_depth",
			"ens_scatter", "ens_vib_rate", "ens_vib_depth", "ens_humanize", "ens_jitter")},
		{Type: "body", Kind: st, Values: vals("body_model", "body_mix", "bow_dynamics")},
		{Type: "formant", Kind: st, Enable: sw("formant_enabled", 1), Values: vals("formant_vowel", "formant_voice_type", "formant_mix",
			"formant_shift", "formant_breath", "formant_sing", "formant_morph_rate", "formant_morph_to", "formant_dry")},
		{Type: "finish", Kind: st, Required: true, Values: vals("post_pitch", "post_decay", "post_decay_rate", "post_body",
			"post_brightness", "post_darken", "post_drive", "out_clamp")},

		// ── sources (osc_type picks the DSP) ──
		{Type: "osc.wavetable", Kind: src, Enable: sw("osc_enabled", 1),
			Values: []instrument.ValueDecl{{Name: "osc_type", Allowed: []float64{0, 1, 2, 3, 12}}}},
		{Type: "osc.noise", Kind: src, Enable: sw("osc_enabled", 1),
			Values: []instrument.ValueDecl{{Name: "osc_type", Allowed: []float64{5, 6}}}},
		{Type: "osc.fm", Kind: src, Enable: sw("osc_enabled", 1), Set: set("osc_type", 4), Values: vals("fm_enabled", "fm_algorithm",
			"fm_op1_ratio", "fm_op2_ratio", "fm_op3_ratio", "fm_op4_ratio",
			"fm_op1_depth", "fm_op2_depth", "fm_op3_depth", "fm_op4_depth",
			"fm_op1_level", "fm_op2_level", "fm_op3_level", "fm_op4_level")},
		{Type: "osc.bowed", Kind: src, Enable: sw("osc_enabled", 1), Set: set("osc_type", 7), Values: vals("osc_bow_pos", "osc_bow_slope", "osc_bow_vel", "osc_bow_loss")},
		{Type: "osc.brass", Kind: src, Enable: sw("osc_enabled", 1), Set: set("osc_type", 8)},
		{Type: "osc.reed", Kind: src, Enable: sw("osc_enabled", 1), Set: set("osc_type", 9)},
		{Type: "osc.flute", Kind: src, Enable: sw("osc_enabled", 1), Set: set("osc_type", 10)},
		{Type: "osc.sax", Kind: src, Enable: sw("osc_enabled", 1), Set: set("osc_type", 11), Values: vals("osc_sax_blow", "osc_sax_reed_off",
			"osc_sax_reed_slope", "osc_sax_reflect", "osc_sax_breath", "osc_sax_loss")},

		// ── layers (gen_source picks the voice) ──
		{Type: "gen.wavetable", Kind: ly, Enable: sw("source", 1), Values: vals("wave", "freq_mode", "freq", "gain",
			"env_fast_rate", "env_tail_rate", "env_fast_mix", "env_tail_mix", "filt_type", "filt_alpha", "filt_freq", "filt_q", "phase_mode", "phase")},
		{Type: "gen.noise", Kind: ly, Enable: sw("source", 2), Values: vals("gain",
			"env_fast_rate", "env_tail_rate", "env_fast_mix", "env_tail_mix", "filt_type", "filt_alpha", "filt_freq", "filt_q", "noise_offset")},
		{Type: "ks", Kind: ly, Enable: sw("source", 3), Capacity: slotsOf("ks"), Values: vals("freq_mode", "freq", "gain", "env_fast_rate",
			"atk_amt", "atk_rate", "out_scale", "ks_sustain", "ks_pluck", "ks_blow")},
		{Type: "bass.analytic", Kind: ly, Enable: sw("source", 4), Capacity: slotsOf("bass"), Values: vals("freq_mode", "freq", "gain", "wave", "phase",
			"env_fast_rate", "pitch_env_amt", "pitch_env_rate", "harm_mix", "atk_amt", "atk_rate", "sat_k", "out_scale")},
		{Type: "tom", Kind: ly, Enable: sw("source", 6), Capacity: slotsOf("tom"), Values: vals("freq_mode", "freq", "wave", "gain",
			"tom_variant", "tom_sweep", "tom_ring", "tom_o1", "tom_o2", "tom_stick", "tom_room")},
		{Type: "clap", Kind: ly, Enable: sw("source", 8), Capacity: slotsOf("snare"), Values: vals("gain",
			"snare_tune", "snare_noise_d", "snare_tail_d", "snare_noise_m", "snare_attack")},
		{Type: "fm", Kind: ly, Enable: sw("source", 10), Capacity: slotsOf("fm"), Values: vals("wave", "fm_variant", "fm_base", "fm_pe_amt", "fm_pe_decay",
			"fm_r1", "fm_r2", "fm_r3", "fm_r4", "fm_d1", "fm_d2", "fm_d3", "fm_d4", "fm_dec1", "fm_dec2", "fm_dec3", "fm_dec4")},
	}

	// Kick: one type per C voice function (gen_kick_<variant>).
	kick := func(typ string, variant float64, own ...string) instrument.ModuleTypeDecl {
		return instrument.ModuleTypeDecl{Type: typ, Kind: ly, Enable: sw("source", 5), Capacity: slotsOf("kick"),
			Set: set("kick_variant", variant), Global: set("kick_enabled", 1),
			Values: vals(append([]string{"freq_mode", "freq", "wave"}, pre("kick_", own...)...)...)}
	}
	core := []string{"attack", "env0", "env1", "fade", "h2", "pe_amt", "pe_rate", "sat"}
	with := func(extra ...string) []string { return append(append([]string{}, core...), extra...) }
	d = append(d,
		kick("kick.base", 0, with("click", "h3", "h4", "noise")...),
		kick("kick.deep", 1, with("click", "h3", "noise")...),
		kick("kick.punchy", 2, with("click")...),
		kick("kick.lofi", 3, with("h3", "noise")...),
		kick("kick.tight", 4, with("click", "h3", "noise")...),
		kick("kick.hybrid", 5, with("click", "h3", "noise")...),
		kick("kick.modal", 6, with("click", "h3", "noise", "mode_detune", "mode_gain", "mode_decay")...),
		kick("kick.acoustic", 7, with("click", "h3", "h4", "noise", "mode_detune", "mode_gain", "mode_decay", "reverb")...),
		kick("kick.physical", 8, with("click", "h3", "h4", "noise", "mode_detune", "mode_gain", "mode_decay", "reverb", "hold", "hiss")...),
	)

	// Snare: one type per C voice function (gen_snare_<variant>).
	snare := func(typ string, variant float64, own ...string) instrument.ModuleTypeDecl {
		return instrument.ModuleTypeDecl{Type: typ, Kind: ly, Enable: sw("source", 7), Capacity: slotsOf("snare"),
			Set: set("snare_variant", variant), Global: set("snare_enabled", 1),
			Values: vals(append([]string{"freq_mode", "freq", "wave"}, pre("snare_", own...)...)...)}
	}
	short := []string{"attack", "noise_d", "noise_m", "tone2", "tone_d", "tone_m", "tune"}
	full := append(append([]string{}, short...), "tail_d", "wire_m")
	d = append(d,
		snare("snare.main", 0, full...),
		snare("snare.rimshot", 1, short...),
		snare("snare.sidestick", 2, short...),
		snare("snare.acoustic", 3, full...),
	)

	// Cymbal: hi-hat and ride always play the modal plate; open hi-hat and crash
	// play it when cym_modes >= 1 and their partial voice below that.
	cym := func(typ string, variant float64, own ...string) instrument.ModuleTypeDecl {
		return instrument.ModuleTypeDecl{Type: typ, Kind: ly, Enable: sw("source", 9), Capacity: slotsOf("cym"),
			Set: set("cym_variant", variant), Values: vals(own...)}
	}
	partial := pre("cym_", "tune", "env_fast", "env_tail", "tone_m", "noise_m", "noise_d")
	modal := append(append([]string{}, partial...), pre("cym_", "modes", "mode_lo", "mode_hi", "decay_tilt", "hf_corner", "beat",
		"tone_f", "tone_w", "body_f", "body_w", "body", "spark_f", "spark_w", "spark", "wash", "attack",
		"rattle", "rattle_rate", "rattle_decay", "drive_delay", "rattle_hold")...)
	d = append(d,
		cym("cym.hihat", 0, modal...),
		cym("cym.open", 1, append([]string{"wave"}, modal...)...),
		cym("cym.cowbell", 2, append([]string{"wave"}, partial...)...),
		cym("cym.shaker", 3, pre("cym_", "tune", "env_fast", "env_tail", "tone_m", "noise_m")...),
		cym("cym.ride", 4, modal...),
		cym("cym.crash", 5, append([]string{"wave"}, modal...)...),
	)

	// Voice render effects: free-form until the FX lift declares their values.
	for _, et := range builtinEffectTypes {
		d = append(d, instrument.ModuleTypeDecl{Type: "fx." + string(et), Kind: instrument.KindFX, FreeForm: true})
	}
	for i := range d {
		d[i].Version = 1
	}
	return d
}
```

- [ ] **Step 4: Implement `module_registry.go`**

```go
package audio

import (
	"slices"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

var (
	builtinRegOnce sync.Once
	builtinReg     *instrument.Registry
)

// BuiltinModuleRegistry is the registry of built-in module types bound to the
// modular ABI. Init (creation values) come from ModularSynthParamDefs until
// sub-project 3 moves UI metadata into the declarations. Panics on an invalid
// table — a build invariant pinned by module_registry_test.go.
func BuiltinModuleRegistry() *instrument.Registry {
	builtinRegOnce.Do(func() {
		defaults := map[string]float64{}
		for _, d := range ModularSynthParamDefs() {
			defaults[d.Name] = d.Default
		}
		decls := builtinModuleDecls()
		for i := range decls {
			for j := range decls[i].Values {
				v := &decls[i].Values[j]
				name := v.Name
				if decls[i].Kind == instrument.KindLayer {
					name = instrument.LayerField(0, name)
				}
				v.Init = defaults[name]
				if len(v.Allowed) > 0 && !slices.Contains(v.Allowed, v.Init) {
					v.Init = v.Allowed[0]
				}
			}
		}
		r, err := instrument.NewRegistry(decls, ModularParamSchema(), modularGenSlots)
		if err != nil {
			panic(err)
		}
		builtinReg = r
	})
	return builtinReg
}
```

- [ ] **Step 5: Run the four tests.** Expected: PASS. If `TestEveryABIFieldIsDeclared` lists a field, add it to the type whose C function reads it (grep `p->gen_<name>` / `p-><name>` in `src/c/modular*.c`). Never add it to a type that doesn't read it.

- [ ] **Step 6: Checkpoint.**

---

### Task A8: Generate the 150 documents (`cmd/gen-instrument-docs`)

**Files:**
- Create: `src/go/cmd/gen-instrument-docs/main.go` (`//go:build !test && !js`), `classify.go` (no tag), `notes.go` (no tag)
- Test: `src/go/cmd/gen-instrument-docs/classify_test.go`, `notes_test.go` (fast path)
- Create (generated): `src/go/internal/assets/instruments/<id>.json` ×150, `src/go/internal/assets/instruments/index.json`
- Modify: `Makefile` (target `instrument-docs`)

**Interfaces:**
- Consumes:
  - `audio.ModularInstrumentDefs()` (modular_instruments.go:369; fields ID, RecipeID, Display, Category, Seed, Beats, DurationSec, Amplitude, DefaultFX, GateTail)
  - `audio.ModularParamSchemaIdentity()`, `audio.BuiltinModuleRegistry()`, `audio.ConfigForInstrument(id)`, `audio.IsMelodicRecipe(recipeID)`, `audio.BuiltinInstrumentIDs`
  - `audio.RecipeRegistrations()["synth-modular"]` for the bare `modular` instrument (Beats 1.0 from engine_instruments.go:19-23, DurationSec 1.0 from config.go:17)
- Produces:
  - `classify(reg, in defInput) (instrument.Doc, []string, error)`, where the `[]string` lists dropped seed keys
  - `seedNotes(paths []string) (map[string]seedNote, error)`
  - `rowSeedVars(path string) (map[string]string, error)` (instrument id → seed var name)
  - The files above. `index.json` = `{"schema_version":"0.0.1","instruments":[ids in BuiltinInstrumentIDs order]}`.

**Classification rules** (spec §3; each rule has a test):

| Container | Rule |
|---|---|
| `block` | identity ⊕ seed (exactly what `bakedModularRender` renders). |
| Required stages (`core`, `finish`) | Always present, on. |
| Stages with Enable | `on = block[enable] >= 0.5`. Present iff on, OR the seed sets any of its values or its enable field. |
| Stages without Enable (`unison`, `body`) | Present (on) iff the seed sets any of their values. |
| Source | Type = the source type whose `Set osc_type` equals, or whose `Allowed` contains, `block["osc_type"]`. `on = block["osc_enabled"] >= 0.5`. Present iff on, OR the seed sets `osc_enabled`, `osc_type` or any value of ANY source type. |
| Layers, slot k | Present iff `block[gen<k+1>_source] > 0`. Type = the layer type with `Enable.Value == source` and every `Set` constant equal to the block's. `on` = every `Global` constant equals the block's (a kick with `kick_enabled` 0 is OFF). `k >= Capacity` → dropped and reported (it renders silence today). |
| Values | From `block`, for every value of a present module. |
| `fx` | `DefaultFX` → `{type: "fx."+Type, v: 1, on: Enabled, values: Params}`. |
| `render` | `{Beats, DurationSec, ConfigForInstrument(id).Amplitude as its shortest float32 decimal, GateTail, IsMelodicRecipe(RecipeID)}`. |
| Dropped keys | Seed keys no present module owns. Each is reported. |
| Invariant | A melodic document carries no `fx` and no `gate_tail` (the pitched path never applied them). Fail generation otherwise. |

- [ ] **Step 1: Failing tests**

```go
// classify_test.go
package main

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func classifyID(t *testing.T, id string) (instrument.Doc, []string) {
	t.Helper()
	for _, def := range audio.ModularInstrumentDefs() {
		if def.ID == id {
			doc, dropped, err := classify(audio.BuiltinModuleRegistry(), inputFromDef(def))
			if err != nil {
				t.Fatal(err)
			}
			if err := instrument.Validate(doc, audio.BuiltinModuleRegistry()); err != nil {
				t.Fatal(err)
			}
			return doc, dropped
		}
	}
	t.Fatalf("no def %s", id)
	return instrument.Doc{}, nil
}

func TestClassifyKickSnap(t *testing.T) {
	doc, _ := classifyID(t, "kick-snap")
	if len(doc.Layers) != 1 || doc.Layers[0].Type != "kick.punchy" || !doc.Layers[0].On {
		t.Fatalf("layers = %+v", doc.Layers)
	}
	if doc.Layers[0].Values["kick_click"] != 0.45 || doc.Stages["core"].Values["voice_freq_hz"] != 62 {
		t.Fatal("seed values must be copied verbatim")
	}
	if doc.Stages["finish"].Values["out_clamp"] != 0 {
		t.Fatal("seeded 0 is a real value")
	}
}

func TestClassifyViolinIsBowedWithBodyAndNotes(t *testing.T) {
	doc, _ := classifyID(t, "violin")
	if doc.Source == nil || doc.Source.Type != "osc.bowed" {
		t.Fatalf("source = %+v", doc.Source)
	}
	if _, ok := doc.Stages["body"]; !ok {
		t.Fatal("a seeded body stage must be present")
	}
	if doc.Layers[0].Type != "gen.noise" {
		t.Fatalf("bow-hair noise layer = %+v", doc.Layers[0])
	}
	if !doc.Render.Melodic || len(doc.FX) != 0 || doc.Render.GateTail != 0 {
		t.Fatal("violin is melodic with no voice fx")
	}
}

func TestClassifyClapBrightCarriesFXAndGate(t *testing.T) {
	doc, _ := classifyID(t, "clap-bright")
	if len(doc.FX) != 1 || doc.FX[0].Type != "fx.filter" || doc.FX[0].Values["cutoff"] != 4000 || doc.Render.GateTail != 0.5 {
		t.Fatalf("fx=%+v gate=%v", doc.FX, doc.Render.GateTail)
	}
}

func TestAmplitudeIsTheShortestFloat32Decimal(t *testing.T) {
	if got := shortestF32(float32(0.8)); got != 0.8 {
		t.Fatalf("shortestF32(0.8f) = %v", got)
	}
}
```

The `violin` expectations come from `violinSeed` (instrument_seeds.go:8-44): `osc_type` 7, `body_model` 1, and `gen1_source` 2 (bow-hair noise).

```go
// notes_test.go
package main

import (
	"strings"
	"testing"
)

func TestSeedNotesReadsDocAndKeyComments(t *testing.T) {
	notes, err := seedNotes([]string{"../../internal/audio/instrument_seeds.go"})
	if err != nil {
		t.Fatal(err)
	}
	n := notes["violinSeed"]
	if !strings.Contains(n.Doc, "additive synthesis") {
		t.Fatalf("doc comment = %q", n.Doc)
	}
	if !strings.Contains(n.Keys["osc_type"], "BOWED-STRING") {
		t.Fatalf("osc_type comment = %q", n.Keys["osc_type"])
	}
}

func TestRowSeedVars(t *testing.T) {
	rows, err := rowSeedVars("../../internal/audio/modular_instruments.go")
	if err != nil {
		t.Fatal(err)
	}
	if rows["kick-snap"] != "drumKickPunchySeed" || rows["clap-bright"] != "clapSeed" {
		t.Fatalf("rows = %v", rows)
	}
}
```

- [ ] **Step 2: Run, fail.** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./cmd/gen-instrument-docs/`

- [ ] **Step 3: Implement `classify.go`**

```go
package main

import (
	"fmt"
	"slices"
	"sort"
	"strconv"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

type defInput struct {
	ID, Name, Category string
	Seed               map[string]float64
	Beats, Seconds     float64
	Amplitude          float32
	GateTail           float64
	Melodic            bool
	FX                 []audio.EffectSlot
}

func inputFromDef(d audio.ModularInstrumentDef) defInput {
	return defInput{ID: d.ID, Name: d.Display, Category: d.Category, Seed: d.Seed,
		Beats: d.Beats, Seconds: d.DurationSec, Amplitude: audio.ConfigForInstrument(d.ID).Amplitude,
		GateTail: d.GateTail, Melodic: audio.IsMelodicRecipe(d.RecipeID), FX: d.DefaultFX}
}

func shortestF32(a float32) float64 {
	v, _ := strconv.ParseFloat(strconv.FormatFloat(float64(a), 'g', -1, 32), 64)
	return v
}

func classify(reg *instrument.Registry, in defInput) (instrument.Doc, []string, error) {
	block := audio.ModularParamSchemaIdentity()
	for k, v := range in.Seed {
		if _, ok := block[k]; !ok {
			return instrument.Doc{}, nil, fmt.Errorf("%s: seed key %q is not an ABI field", in.ID, k)
		}
		block[k] = v
	}
	used := map[string]bool{}
	seeded := func(names ...string) bool {
		for _, n := range names {
			if _, ok := in.Seed[n]; ok {
				return true
			}
		}
		return false
	}
	field := func(slot int, n string) string {
		if slot < 0 {
			return n
		}
		return instrument.LayerField(slot, n)
	}
	take := func(t *instrument.ModuleTypeDecl, slot int) instrument.Values {
		v := instrument.Values{}
		for _, vd := range t.Values {
			f := field(slot, vd.Name)
			v[vd.Name] = block[f]
			used[f] = true
		}
		for _, s := range t.Set {
			used[field(slot, s.Field)] = true
		}
		for _, g := range t.Global {
			used[g.Field] = true
		}
		if t.Enable != nil {
			used[field(slot, t.Enable.Field)] = true
		}
		return v
	}
	names := func(t *instrument.ModuleTypeDecl, slot int) []string {
		var out []string
		for _, vd := range t.Values {
			out = append(out, field(slot, vd.Name))
		}
		if t.Enable != nil {
			out = append(out, field(slot, t.Enable.Field))
		}
		return out
	}

	doc := instrument.Doc{ID: in.ID, Name: in.Name, Category: in.Category, Stages: map[string]instrument.Stage{},
		Render: instrument.Render{Beats: in.Beats, Seconds: in.Seconds, Amplitude: shortestF32(in.Amplitude),
			GateTail: in.GateTail, Melodic: in.Melodic}}

	var sources []*instrument.ModuleTypeDecl
	for _, t := range reg.Types() {
		switch t.Kind {
		case instrument.KindStage:
			on := t.Enable == nil || block[t.Enable.Field] >= 0.5
			if !(t.Required || (t.Enable != nil && on) || seeded(names(t, -1)...)) {
				continue
			}
			doc.Stages[t.Type] = instrument.Stage{V: t.Version, On: on, Values: take(t, -1)}
		case instrument.KindSource:
			sources = append(sources, t)
		}
	}

	var srcNames []string
	var srcType *instrument.ModuleTypeDecl
	for _, t := range sources {
		srcNames = append(srcNames, names(t, -1)...)
		for _, s := range t.Set {
			if s.Field == "osc_type" && s.Value == block["osc_type"] {
				srcType = t
			}
		}
		for _, vd := range t.Values {
			if vd.Name == "osc_type" && slices.Contains(vd.Allowed, block["osc_type"]) {
				srcType = t
			}
		}
	}
	if srcType == nil {
		return doc, nil, fmt.Errorf("%s: osc_type %v matches no source type", in.ID, block["osc_type"])
	}
	srcOn := block["osc_enabled"] >= 0.5
	if srcOn || seeded(append(srcNames, "osc_type")...) {
		used["osc_type"] = true
		doc.Source = &instrument.Module{Type: srcType.Type, V: srcType.Version, On: srcOn, Values: take(srcType, -1)}
	}

	var dropped []string
	for k := 0; k < reg.Slots(); k++ {
		src := block[instrument.LayerField(k, "source")]
		if src <= 0 {
			continue
		}
		var lt *instrument.ModuleTypeDecl
		for _, t := range reg.Types() {
			if t.Kind != instrument.KindLayer || t.Enable.Value != src {
				continue
			}
			match := true
			for _, s := range t.Set {
				if block[instrument.LayerField(k, s.Field)] != s.Value {
					match = false
				}
			}
			if match {
				lt = t
			}
		}
		if lt == nil {
			return doc, nil, fmt.Errorf("%s: slot %d (source %v) matches no layer type", in.ID, k+1, src)
		}
		if k >= lt.Capacity {
			dropped = append(dropped, fmt.Sprintf("layer %d (%s): past the engine's slot limit, renders silence today", k+1, lt.Type))
			continue
		}
		on := true
		for _, g := range lt.Global {
			if block[g.Field] != g.Value {
				on = false
			}
		}
		for len(doc.Layers) <= k {
			doc.Layers = append(doc.Layers, nil)
		}
		doc.Layers[k] = &instrument.Module{Type: lt.Type, V: lt.Version, On: on, Values: take(lt, k)}
	}
	for len(doc.Layers) > 0 && doc.Layers[len(doc.Layers)-1] == nil {
		doc.Layers = doc.Layers[:len(doc.Layers)-1]
	}

	for _, fx := range in.FX {
		doc.FX = append(doc.FX, instrument.Module{Type: "fx." + string(fx.Type), V: 1, On: fx.Enabled,
			Values: instrument.Values(fx.Params)})
	}
	if doc.Render.Melodic && (len(doc.FX) > 0 || doc.Render.GateTail != 0) {
		return doc, nil, fmt.Errorf("%s: melodic instruments never had voice fx/gate on their pitched path", in.ID)
	}

	var keys []string
	for k := range in.Seed {
		if !used[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		dropped = append(dropped, "seed key "+k+": no present module owns it")
	}
	return doc, dropped, nil
}
```

`fx.Params` may be nil; the fx type is free-form, so an empty map is valid. Copy the map (`maps.Clone`) so the document doesn't alias the def.

- [ ] **Step 4: Implement `notes.go`** (go/ast, no build tag)

```go
package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
)

type seedNote struct {
	Doc  string
	Keys map[string]string
}

// seedNotes returns, per package-level var, its doc comment and the comment
// attached to each string key of its composite literal(s) — the tuning
// provenance the seeds carry, moved into the documents' notes.
func seedNotes(paths []string) (map[string]seedNote, error) {
	out := map[string]seedNote{}
	fset := token.NewFileSet()
	for _, p := range paths {
		f, err := parser.ParseFile(fset, p, nil, parser.ParseComments)
		if err != nil {
			return nil, err
		}
		cmap := ast.NewCommentMap(fset, f, f.Comments)
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, spec := range gd.Specs {
				vs := spec.(*ast.ValueSpec)
				doc := vs.Doc
				if doc == nil {
					doc = gd.Doc
				}
				for i, name := range vs.Names {
					n := seedNote{Doc: strings.TrimSpace(doc.Text()), Keys: map[string]string{}}
					if i < len(vs.Values) {
						ast.Inspect(vs.Values[i], func(node ast.Node) bool {
							kv, ok := node.(*ast.KeyValueExpr)
							if !ok {
								return true
							}
							lit, ok := kv.Key.(*ast.BasicLit)
							if !ok || lit.Kind != token.STRING {
								return true
							}
							key, _ := strconv.Unquote(lit.Value)
							var parts []string
							for _, cg := range cmap[kv] {
								parts = append(parts, strings.TrimSpace(cg.Text()))
							}
							if len(parts) > 0 {
								n.Keys[key] = strings.Join(parts, " ")
							}
							return true
						})
					}
					out[name.Name] = n
				}
			}
		}
	}
	return out, nil
}

// rowSeedVars maps each modularInstrumentDefs row's ID to the identifier its
// Seed field names (rows whose Seed is an expression map to "").
func rowSeedVars(path string) (map[string]string, error) {
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		cl, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		var id, seed string
		for _, e := range cl.Elts {
			kv, ok := e.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			k, ok := kv.Key.(*ast.Ident)
			if !ok {
				continue
			}
			switch k.Name {
			case "ID":
				if lit, ok := kv.Value.(*ast.BasicLit); ok {
					id, _ = strconv.Unquote(lit.Value)
				}
			case "Seed":
				if ident, ok := kv.Value.(*ast.Ident); ok {
					seed = ident.Name
				}
			}
		}
		if id != "" {
			out[id] = seed
		}
		return true
	})
	return out, nil
}
```

**Notes placement** — add to `notes.go`:

```go
// applyNotes moves a seed's comments into its document: the var's doc comment
// → doc.Notes; each key's comment → the notes of the module owning that field
// (value, Set, Global or Enable), one "key: comment" line per key in key order;
// comments of keys no present module owns → doc.Notes as "(dropped) key: …".
func applyNotes(reg *instrument.Registry, doc *instrument.Doc, n seedNote) {
	doc.Notes = n.Doc
	add := func(dst *string, line string) {
		if *dst == "" {
			*dst = line
		} else {
			*dst += "\n" + line
		}
	}
	keys := make([]string, 0, len(n.Keys))
	for k := range n.Keys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		line := k + ": " + n.Keys[k]
		if placeNote(reg, doc, k, line, add) {
			continue
		}
		add(&doc.Notes, "(dropped) "+line)
	}
}

func placeNote(reg *instrument.Registry, doc *instrument.Doc, key, line string, add func(*string, string)) bool {
	for sk, st := range doc.Stages {
		if t, _ := reg.Type(sk); owns(t, -1, key) {
			add(&st.Notes, line)
			doc.Stages[sk] = st
			return true
		}
	}
	if m := doc.Source; m != nil {
		if t, _ := reg.Type(m.Type); owns(t, -1, key) {
			add(&m.Notes, line)
			return true
		}
	}
	for i, m := range doc.Layers {
		if m == nil {
			continue
		}
		if t, _ := reg.Type(m.Type); owns(t, i, key) {
			add(&m.Notes, line)
			return true
		}
	}
	return false
}

func owns(t *instrument.ModuleTypeDecl, slot int, key string) bool {
	name := func(f string) string {
		if slot < 0 {
			return f
		}
		return instrument.LayerField(slot, f)
	}
	for _, v := range t.Values {
		if name(v.Name) == key {
			return true
		}
	}
	for _, s := range t.Set {
		if name(s.Field) == key {
			return true
		}
	}
	for _, g := range t.Global {
		if g.Field == key {
			return true
		}
	}
	return t.Enable != nil && name(t.Enable.Field) == key
}
```

`notes.go` then also imports `sort` and `github.com/ingyamilmolinar/beatmo/internal/instrument`. Add this test to `notes_test.go` (it needs the registry, so it lives with `classify_test.go`'s helpers):

```go
func TestApplyNotesPutsKeyCommentsOnTheOwningModule(t *testing.T) {
	doc, _ := classifyID(t, "violin")
	notes, _ := seedNotes([]string{"../../internal/audio/instrument_seeds.go"})
	applyNotes(audio.BuiltinModuleRegistry(), &doc, notes["violinSeed"])
	if !strings.Contains(doc.Source.Notes, "osc_type: ") || !strings.Contains(doc.Notes, "additive synthesis") {
		t.Fatalf("source notes %q / doc notes %q", doc.Source.Notes, doc.Notes)
	}
	if !strings.Contains(doc.Stages["body"].Notes, "body_model: ") {
		t.Fatalf("body notes %q", doc.Stages["body"].Notes)
	}
}
```

- [ ] **Step 5: Implement `main.go`** (`//go:build !test && !js`)

```go
//go:build !test && !js

// gen-instrument-docs converts today's Go seeds into instrument documents
// (one-time, sub-project 1a). Run from src/go:
//
//	go run ./cmd/gen-instrument-docs -out internal/assets/instruments
//
// It prints every dropped seed key; review the list (spec §3). Deleted with
// the seeds in Task C2.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

func main() {
	out := flag.String("out", "internal/assets/instruments", "output directory")
	flag.Parse()
	reg := audio.BuiltinModuleRegistry()
	seedFiles, _ := filepath.Glob("internal/audio/instrument_seeds*.go")
	notes, err := seedNotes(seedFiles)
	if err != nil {
		log.Fatal(err)
	}
	rows, err := rowSeedVars("internal/audio/modular_instruments.go")
	if err != nil {
		log.Fatal(err)
	}
	inputs := map[string]defInput{}
	for _, d := range audio.ModularInstrumentDefs() {
		inputs[d.ID] = inputFromDef(d)
	}
	mod := audio.RecipeRegistrations()["synth-modular"]
	inputs["modular"] = defInput{ID: "modular", Name: mod.DisplayName, Category: mod.Category,
		Beats: 1.0, Seconds: 1.0, Amplitude: audio.ConfigForInstrument("modular").Amplitude}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		log.Fatal(err)
	}
	for _, id := range audio.BuiltinInstrumentIDs {
		in, ok := inputs[id]
		if !ok {
			log.Fatalf("%s: no definition", id)
		}
		doc, dropped, err := classify(reg, in)
		if err != nil {
			log.Fatal(err)
		}
		applyNotes(reg, &doc, notes[rows[id]])
		if err := instrument.Validate(doc, reg); err != nil {
			log.Fatalf("%s: %v", id, err)
		}
		for _, d := range dropped {
			fmt.Printf("DROPPED %s %s\n", id, d)
		}
		b, err := instrument.MarshalFile(doc)
		if err != nil {
			log.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(*out, id+".json"), b, 0o644); err != nil {
			log.Fatal(err)
		}
	}
	idx, _ := json.MarshalIndent(struct {
		schema.Envelope
		Instruments []string `json:"instruments"`
	}{schema.Current(), audio.BuiltinInstrumentIDs}, "", "  ")
	if err := os.WriteFile(filepath.Join(*out, "index.json"), append(idx, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
}

```

(`applyNotes` lives in `notes.go`; `main.go` imports `encoding/json`, `flag`, `fmt`, `log`, `os`, `path/filepath`, and the three internal packages.)

- [ ] **Step 6: Run the unit tests (fast path) — PASS**, including `TestApplyNotesPutsKeyCommentsOnTheOwningModule`.

- [ ] **Step 7: Generate and review**

```bash
cd src/go && ../../.tools/go/bin/go run ./cmd/gen-instrument-docs -out internal/assets/instruments | tee $S/dropped.txt
ls internal/assets/instruments | wc -l     # 151 (150 docs + index.json)
```

Read `$S/dropped.txt`. Every `seed key` line must be either:
- listed in `internal/audio/testdata/known_dead_seed_params.txt`, or
- a field of an inactive source/layer type (for example `fm_*` on a bowed source).

Copy the list into progress.md under "A8 dropped keys", with a one-line reason per key group. Any `layer N … past the engine's slot limit` line is a real engine bug in today's data: add it to progress.md and tell the user at the A10 checkpoint.

Add the Makefile target:

```make
.PHONY: instrument-docs
instrument-docs: ## (one-time) regenerate instrument documents from the Go seeds
	cd src/go && ../../.tools/go/bin/go run ./cmd/gen-instrument-docs -out internal/assets/instruments
```

- [ ] **Step 8: Checkpoint.** Open three generated files (`kick-snap.json`, `violin.json`, `clap-bright.json`) and check that they read sensibly. Log the counts.

---

### Task A9: The proof — documents render like today

**Files:**
- Create: `src/go/internal/audio/instrument_docs_proof_test.go` (`//go:build !test && !js`). Holds `defsByID`, `TestDocumentsRenderLikeTheBakedSeeds` and `TestFlattenMatchesIdentityFilledBlockEverywhere`; deleted in Task C2 with the seeds.
- Create: `src/go/internal/audio/module_liveness_test.go` (`//go:build !test && !js`). Holds `proofSR`, `renderBlockForTest`, `TestDeclaredModuleValuesAreLive` and `loadDeadModuleValues`; it outlives the seeds.

  Both are written as one listing in Step 2; split it along those lines, each file with only the imports it uses.
- Create: `src/go/internal/audio/testdata/dead_module_values.txt`
- Create: `src/go/internal/audio/shipped_docs_test.go` (fast path, no tag)

**Interfaces:**
- Consumes:
  - `renderModularBlock(buf []float32, sampleRate, samples int, b modularBlock)` (modular_c.go:79)
  - `newModularBlock(RecipeParams) modularBlock` (modular_c.go:28, identity fill)
  - `ModularInstrumentDefs()`
  - `perfFor`-style perturbation `perturbFor(d ParamDef, cur float64) (float64, bool)` (seed_param_liveness_test.go:74, same package)
- Produces:
  - `loadShippedDocsForTest(t) map[string]instrument.Doc` (reads `../assets/instruments/*.json`)
  - three tests

- [ ] **Step 1: Write `shipped_docs_test.go` (fast path)**

```go
package audio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

const shippedDocsDir = "../assets/instruments"

func loadShippedDocsForTest(t *testing.T) map[string]instrument.Doc {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(shippedDocsDir, "*.json"))
	out := map[string]instrument.Doc{}
	for _, p := range paths {
		if filepath.Base(p) == "index.json" {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		d, err := instrument.ParseFile(b)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out[d.ID] = d
	}
	return out
}

func TestShippedDocumentsValidate(t *testing.T) {
	docs := loadShippedDocsForTest(t)
	for id, d := range docs {
		if err := instrument.Validate(d, BuiltinModuleRegistry()); err != nil {
			t.Errorf("%s: %v", id, err)
		}
		if filepath.Base(id) != id {
			t.Errorf("bad id %q", id)
		}
	}
	b, err := os.ReadFile(filepath.Join(shippedDocsDir, "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var idx struct {
		Instruments []string `json:"instruments"`
	}
	if err := json.Unmarshal(b, &idx); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(idx.Instruments, BuiltinInstrumentIDs) || len(docs) != len(idx.Instruments) {
		t.Fatalf("index.json (%d) must list every document (%d) in BuiltinInstrumentIDs order", len(idx.Instruments), len(docs))
	}
}

// Shipped values lie inside their knob ranges, or the derived range (Task B4)
// widens to contain them — this lists which, so a widening is a decision.
func TestShippedValuesInsideParamDefRanges(t *testing.T) {
	defs := map[string]ParamDef{}
	for _, d := range ModularSynthParamDefs() {
		defs[d.Name] = d
	}
	reg := BuiltinModuleRegistry()
	for id, d := range loadShippedDocsForTest(t) {
		for f, v := range instrument.OwnedFields(d, reg) {
			pd, ok := defs[f]
			if ok && (v < pd.Min || v > pd.Max) {
				t.Logf("WIDEN %s %s=%v outside [%v,%v]", id, f, v, pd.Min, pd.Max)
			}
		}
	}
}
```

- [ ] **Step 2: Write `instrument_docs_proof_test.go` (real path)**

```go
//go:build !test && !js

package audio

import (
	"bufio"
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

const proofSR = 44100

func renderBlockForTest(b []float64, n int) []float32 {
	buf := make([]float32, n)
	renderModularBlock(buf, proofSR, n, modularBlock(b))
	return buf
}

func defsByID() map[string]ModularInstrumentDef {
	out := map[string]ModularInstrumentDef{"modular": {ID: "modular", Beats: 1.0, DurationSec: 1.0}}
	for _, d := range ModularInstrumentDefs() {
		out[d.ID] = d
	}
	return out
}

// TestDocumentsRenderLikeTheBakedSeeds: every document's Flatten renders
// bit-identically to today's default one-shot block (identity ⊕ seed), and
// carries the same render metadata, fx and gate (spec §10).
func TestDocumentsRenderLikeTheBakedSeeds(t *testing.T) {
	reg := BuiltinModuleRegistry()
	defs := defsByID()
	for id, doc := range loadShippedDocsForTest(t) {
		def, ok := defs[id]
		if !ok {
			t.Errorf("%s: no def", id)
			continue
		}
		n := int(float64(proofSR) * 60 / 120 * def.Beats)
		got := renderBlockForTest(instrument.Flatten(doc, reg), n)
		want := renderBlockForTest(newModularBlock(def.Seed), n)
		if !slices.Equal(got, want) {
			t.Errorf("%s: Flatten render differs from the baked seed render", id)
		}
		if doc.Render.Beats != def.Beats || doc.Render.Seconds != def.DurationSec || doc.Render.GateTail != def.GateTail {
			t.Errorf("%s: render metadata %+v vs def", id, doc.Render)
		}
		if len(doc.FX) != len(def.DefaultFX) {
			t.Errorf("%s: fx %d vs DefaultFX %d", id, len(doc.FX), len(def.DefaultFX))
		}
		for i := range doc.FX {
			if doc.FX[i].Type != "fx."+string(def.DefaultFX[i].Type) || doc.FX[i].On != def.DefaultFX[i].Enabled {
				t.Errorf("%s: fx[%d] %+v vs %+v", id, i, doc.FX[i], def.DefaultFX[i])
			}
		}
	}
}

// TestFlattenMatchesIdentityFilledBlockEverywhere: zeroing every field no
// present ON module owns is inaudible — for every document, every present value
// at min/max/zero, and every absent optional stage switched on from Init
// (spec §10). Renders 0.25 s per case.
func TestFlattenMatchesIdentityFilledBlockEverywhere(t *testing.T) {
	reg := BuiltinModuleRegistry()
	defs := map[string]ParamDef{}
	for _, d := range ModularSynthParamDefs() {
		defs[d.Name] = d
	}
	n := proofSR / 4
	check := func(id, label string, d instrument.Doc) {
		a := renderBlockForTest(instrument.Flatten(d, reg), n)
		b := renderBlockForTest(newModularBlock(instrument.OwnedFields(d, reg)), n)
		if !slices.Equal(a, b) {
			t.Errorf("%s %s: Flatten differs from the identity-filled block", id, label)
		}
	}
	docs := loadShippedDocsForTest(t)
	ids := make([]string, 0, len(docs))
	for id := range docs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		doc := docs[id]
		check(id, "default", doc)
		for f, cur := range instrument.OwnedFields(doc, reg) {
			pd, ok := defs[f]
			if !ok {
				continue
			}
			for _, v := range []float64{pd.Min, pd.Max, 0} {
				if v == cur || v < pd.Min || v > pd.Max {
					continue
				}
				d, err := instrument.SetField(doc, reg, f, v)
				if err != nil {
					continue // selectors outside Allowed, constants
				}
				check(id, fmt.Sprintf("%s@%v", f, v), d)
			}
		}
		for _, ty := range reg.Types() {
			if ty.Kind != instrument.KindStage || ty.Required {
				continue
			}
			st, present := doc.Stages[ty.Type]
			switch {
			case !present:
				d, _ := instrument.AddStage(doc, reg, ty.Type, true)
				check(id, "add:"+ty.Type, d)
			case !st.On:
				d, _ := instrument.SetOn(doc, reg, instrument.StageRef(ty.Type), true)
				check(id, "on:"+ty.Type, d)
			}
		}
	}
}

// TestDeclaredModuleValuesAreLive: every declared value changes the render of
// at least one shipped document of its type, or is listed (with a reason) in
// testdata/dead_module_values.txt. Ratchet: a listed entry that is live fails.
func TestDeclaredModuleValuesAreLive(t *testing.T) {
	reg := BuiltinModuleRegistry()
	defs := map[string]ParamDef{}
	for _, d := range ModularSynthParamDefs() {
		defs[d.Name] = d
	}
	known := loadDeadModuleValues(t)
	n := proofSR / 4
	live := map[string]bool{}
	seen := map[string]bool{}
	for _, doc := range loadShippedDocsForTest(t) {
		base := renderBlockForTest(instrument.Flatten(doc, reg), n)
		visit := func(ref instrument.Ref, m instrument.Module, slot int) {
			ty, _ := reg.Type(m.Type)
			seen[ty.Type] = true
			if !m.On || ty.FreeForm {
				return
			}
			for _, vd := range ty.Values {
				key := ty.Type + "." + vd.Name
				if live[key] || len(vd.Allowed) > 0 {
					continue
				}
				f := vd.Name
				if slot >= 0 {
					f = instrument.LayerField(slot, vd.Name)
				}
				v, ok := perturbFor(defs[f], m.Values[vd.Name])
				if !ok {
					continue
				}
				d, err := instrument.SetValue(doc, reg, ref, vd.Name, v)
				if err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(base, renderBlockForTest(instrument.Flatten(d, reg), n)) {
					live[key] = true
				}
			}
		}
		for k, st := range doc.Stages {
			visit(instrument.StageRef(k), instrument.Module{Type: k, V: st.V, On: st.On, Values: st.Values}, -1)
		}
		if doc.Source != nil {
			visit(instrument.SourceRef(), *doc.Source, -1)
		}
		for i, m := range doc.Layers {
			if m != nil {
				visit(instrument.LayerRef(i), *m, i)
			}
		}
	}
	for _, ty := range reg.Types() {
		if ty.Kind == instrument.KindFX {
			continue
		}
		if !seen[ty.Type] {
			if _, ok := known[ty.Type+".*"]; !ok {
				t.Errorf("UNUSED %s.* — no shipped document uses this type; list it with a reason", ty.Type)
			}
			continue
		}
		for _, vd := range ty.Values {
			key := ty.Type + "." + vd.Name
			_, listed := known[key]
			switch {
			case live[key] && listed:
				t.Errorf("%s is live now — remove it from dead_module_values.txt", key)
			case !live[key] && !listed && len(vd.Allowed) == 0:
				t.Errorf("DEAD %s — no shipped document's render moves with it; fix the declaration or list it with a reason", key)
			}
		}
	}
}

func loadDeadModuleValues(t *testing.T) map[string]string {
	f, err := os.Open("testdata/dead_module_values.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, reason, ok := strings.Cut(line, " ")
		if !ok || strings.TrimSpace(reason) == "" {
			t.Fatalf("dead_module_values.txt: %q needs a reason", line)
		}
		out[key] = strings.TrimSpace(reason)
	}
	return out
}
```

- [ ] **Step 3: Create the ratchet file** with a header and no entries:

```text
# Module values no shipped document's render moves with (TestDeclaredModuleValuesAreLive).
# One per line: "<type>.<value> <reason>" or "<type>.* <reason>" for an unused type.
```

- [ ] **Step 4: Run.**

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestShippedDocuments|TestShippedValues' -v ./internal/audio/ | grep -E 'WIDEN|FAIL|ok'
cd src/go && ../../.tools/go/bin/go test -run 'TestDocumentsRenderLikeTheBakedSeeds|TestFlattenMatchesIdentityFilledBlockEverywhere|TestDeclaredModuleValuesAreLive' -timeout 30m ./internal/audio/ 2>&1 | tee $S/proof.txt
```

Expected: the first two proofs PASS.

- **A `TestDocumentsRenderLikeTheBakedSeeds` failure** is a classification bug (A8). Fix the rule, regenerate, rerun. Never "fix" a document by hand.
- **A `TestFlattenMatchesIdentityFilledBlockEverywhere` failure** means some module reads a field it doesn't declare (identity ≠ 0 there). Add the field to that type's declaration (A7), regenerate, rerun.
- **`TestDeclaredModuleValuesAreLive`** prints DEAD/UNUSED lines.
  - Each DEAD line is either a declaration mistake (remove the value from the type), or a value that is live only in a context no shipped document uses (e.g. `cym.open.cym_tone_m` read only when `cym_modes < 1`).
  - List the second kind with that reason.
  - UNUSED types (`osc.brass`, …) are listed with "no shipped instrument plays this source yet".
  - Rerun until green.
  - Record every DEAD resolution in progress.md.

- [ ] **Step 5: Checkpoint** — the three proofs green; record the WIDEN list in progress.md.

---

### Task A10: Audit — where today's other paths disagree with the documents

**Files:**
- Create: `src/go/internal/audio/doc_path_audit_test.go` (`//go:build !test && !js`; runs only with `BEATMO_DOC_AUDIT=1`)

**Interfaces:**
- Consumes: `MergeRecipeDefaults(recipeID, nil)` (synth_registry.go:153; the clamped recipe layer = the browser default and native's edited/melodic path), `newModularBlock`, `renderBlockForTest` (A9).

- [ ] **Step 1: Write the audit**

```go
//go:build !test && !js

package audio

import (
	"fmt"
	"os"
	"slices"
	"sort"
	"strings"
	"testing"
)

// TestDocPathAudit lists every instrument whose RECIPE-PATH block (ParamDef
// defaults + seed clamped into knob ranges — what the browser plays by default,
// and what native plays for melodic node playback and after any knob edit)
// renders differently from its BAKED block (identity + raw seed — what the
// documents hold). Opt-in: BEATMO_DOC_AUDIT=1, report to BEATMO_DOC_AUDIT_OUT.
func TestDocPathAudit(t *testing.T) {
	if os.Getenv("BEATMO_DOC_AUDIT") != "1" {
		t.Skip("set BEATMO_DOC_AUDIT=1")
	}
	var lines []string
	for _, d := range ModularInstrumentDefs() {
		baked := newModularBlock(d.Seed)
		merged := newModularBlock(MergeRecipeDefaults(d.RecipeID, nil))
		n := proofSR
		if slices.Equal(renderBlockForTest(baked, n), renderBlockForTest(merged, n)) {
			continue
		}
		var fields []string
		for i, name := range ModularParamSchema() {
			if baked[i] != merged[i] {
				fields = append(fields, fmt.Sprintf("%s %v→%v", name, baked[i], merged[i]))
			}
		}
		sort.Strings(fields)
		melodic := IsMelodicRecipe(d.RecipeID)
		lines = append(lines, fmt.Sprintf("- **%s** (melodic=%v): %s", d.ID, melodic, strings.Join(fields, ", ")))
	}
	report := "# Doc-path audit\n\nBaked (documents) → recipe path (browser default; native melodic playback and edited voices).\n\n" + strings.Join(lines, "\n") + "\n"
	if out := os.Getenv("BEATMO_DOC_AUDIT_OUT"); out != "" {
		_ = os.WriteFile(out, []byte(report), 0o644)
	}
	t.Log(report)
}
```

- [ ] **Step 2: Run** `cd src/go && BEATMO_DOC_AUDIT=1 BEATMO_DOC_AUDIT_OUT=$S/doc_path_audit.md ../../.tools/go/bin/go test -run TestDocPathAudit ./internal/audio/`, then read `$S/doc_path_audit.md`.

- [ ] **Step 3: PART A CHECKPOINT — ask the user.**
  - Show the audit list: instruments, melodic flag and the fields that differ.
  - Show A8's `past the engine's slot limit` lines, if any.
  - Show the A9 WIDEN list.
  - Explain: after Part B, every listed instrument plays its baked values on BOTH platforms and in every context. That is a sound change for the browser default, for native melodic node playback, and for native edited voices of those instruments; the one-shot default is unchanged.
  - Ask whether the baked values are the intended sound, or whether specific instruments should take the recipe-path value instead. In that case the generator writes that value into the document, and the parity allowlist names those instruments.
  - Record the decision in progress.md.
  - Do not start Part B without the answer.

---

# Part B — The switch: runtime, library and files read documents

Within Part B, only the task's own tests must be green after each task. The full `internal/ui` suite, browser suites, parity and screenshots are required green at the B10 checkpoint.

### Task B1: The asset pack and the shipped library

**Files:**
- Create: `src/go/cmd/gen-asset-pack/main.go` (no tag), `src/go/internal/assets/instruments.go`, `src/go/internal/audio/instrument_library.go`
- Create (generated): `src/go/internal/assets/instruments.pack.json.gz`
- Test: `src/go/internal/assets/instruments_pack_test.go` (`TestAssetPackIsCurrent`), `src/go/internal/audio/instrument_library_test.go`
- Modify: `Makefile` (`asset-pack` target), `scripts/deploy-wasm-gcs.sh` (nothing; the pack is embedded)

**Interfaces:**
- Produces:
  - Pack JSON `{schema_version, order: [ids], shipped: {id: hash}, docs: {hash: doc}}`
  - `assets.InstrumentPack() []byte`
  - `func BuildPack(dir string) ([]byte, error)` (in `cmd/gen-asset-pack`, package main; the drift test shells out via `go run` — or move the builder to `internal/assets/packbuild` (no tag) so the test can call it. **Do the latter**: `internal/assets/packbuild.Build(dir string) ([]byte, error)`)
  - `audio.ShippedInstrumentIDs() []string`, `audio.ShippedDoc(id) (instrument.Doc, bool)`, `audio.PackDoc(hash) (instrument.Doc, bool)`

- [ ] **Step 1: Failing tests**

```go
// internal/assets/instruments_pack_test.go
package assets

import (
	"bytes"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets/packbuild"
)

func TestAssetPackIsCurrent(t *testing.T) {
	want, err := packbuild.Build("instruments")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, InstrumentPack()) {
		t.Fatal("instruments.pack.json.gz is stale — run `make asset-pack`")
	}
}
```

```go
// internal/audio/instrument_library_test.go
package audio

import "testing"

func TestShippedLibraryMatchesTheSources(t *testing.T) {
	src := loadShippedDocsForTest(t)
	ids := ShippedInstrumentIDs()
	if len(ids) != len(src) || ids[0] != BuiltinInstrumentIDs[0] {
		t.Fatalf("pack order %d ids, sources %d", len(ids), len(src))
	}
	for _, id := range ids {
		d, ok := ShippedDoc(id)
		if !ok || len(instrument.Diff(d, src[id])) != 0 {
			t.Fatalf("%s: pack doc differs from its source", id)
		}
	}
}
```

(Add `"github.com/ingyamilmolinar/beatmo/internal/instrument"` to the imports.)

- [ ] **Step 2: Implement `internal/assets/packbuild/packbuild.go`**

```go
// Package packbuild builds the embedded instrument pack from the readable
// sources in internal/assets/instruments (spec §3, §8 "Files"). Deterministic:
// encoding/json sorts map keys and gzip writes a zero header.
package packbuild

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
	"github.com/ingyamilmolinar/beatmo/internal/schema"
)

type Pack struct {
	schema.Envelope
	Order   []string                  `json:"order"`
	Shipped map[string]string         `json:"shipped"`
	Docs    map[string]instrument.Doc `json:"docs"`
}

func Build(dir string) ([]byte, error) {
	idxBytes, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		return nil, err
	}
	var idx struct {
		schema.Envelope
		Instruments []string `json:"instruments"`
	}
	if err := json.Unmarshal(idxBytes, &idx); err != nil {
		return nil, err
	}
	p := Pack{Envelope: schema.Current(), Order: idx.Instruments, Shipped: map[string]string{}, Docs: map[string]instrument.Doc{}}
	for _, id := range idx.Instruments {
		b, err := os.ReadFile(filepath.Join(dir, id+".json"))
		if err != nil {
			return nil, err
		}
		d, err := instrument.ParseFile(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", id, err)
		}
		h := instrument.ContentHash(d)
		p.Shipped[id] = h
		if _, dup := p.Docs[h]; !dup {
			p.Docs[h] = d
		}
	}
	raw, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&buf, gzip.BestCompression)
	if _, err := zw.Write(raw); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
```

`cmd/gen-asset-pack/main.go` calls `packbuild.Build("internal/assets/instruments")` and writes `internal/assets/instruments.pack.json.gz`. `internal/assets/instruments.go`:

```go
package assets

import _ "embed"

//go:embed instruments.pack.json.gz
var instrumentPack []byte

// InstrumentPack is the gzip'd instrument pack (packbuild.Pack).
func InstrumentPack() []byte { return instrumentPack }
```

Makefile:

```make
.PHONY: asset-pack
asset-pack: ## Rebuild the embedded instrument pack from internal/assets/instruments
	cd src/go && ../../.tools/go/bin/go run ./cmd/gen-asset-pack
```

- [ ] **Step 3: Implement `internal/audio/instrument_library.go`** (no tag). The shipped half is shown here; the user half comes in B2.

```go
package audio

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/assets/packbuild"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

var (
	packOnce sync.Once
	packData packbuild.Pack
)

func shippedPack() *packbuild.Pack {
	packOnce.Do(func() {
		zr, err := gzip.NewReader(bytes.NewReader(assets.InstrumentPack()))
		if err != nil {
			panic(fmt.Errorf("instrument pack: %w", err))
		}
		raw, err := io.ReadAll(zr)
		if err != nil {
			panic(fmt.Errorf("instrument pack: %w", err))
		}
		if err := json.Unmarshal(raw, &packData); err != nil {
			panic(fmt.Errorf("instrument pack: %w", err))
		}
		if err := packData.Envelope.Validate("instrument pack"); err != nil {
			panic(err)
		}
		reg := BuiltinModuleRegistry()
		for h, d := range packData.Docs {
			if err := instrument.Validate(d, reg); err != nil {
				panic(fmt.Errorf("instrument pack %s: %w", h, err))
			}
		}
	})
	return &packData
}

// ShippedInstrumentIDs is the shipped instrument order.
func ShippedInstrumentIDs() []string { return append([]string(nil), shippedPack().Order...) }

// ShippedDoc returns a copy of the shipped document for id.
func ShippedDoc(id string) (instrument.Doc, bool) {
	p := shippedPack()
	h, ok := p.Shipped[id]
	if !ok {
		return instrument.Doc{}, false
	}
	d := p.Docs[h].Clone()
	d.ID = id
	return d, true
}

// PackDoc returns a copy of the pack document with content hash h.
func PackDoc(h string) (instrument.Doc, bool) {
	d, ok := shippedPack().Docs[h]
	if !ok {
		return instrument.Doc{}, false
	}
	return d.Clone(), true
}
```

- [ ] **Step 4: Run** `make asset-pack`, then the two tests on the fast path. **WASM size:** `make wasm && cd src/go && ../../.tools/go/bin/go test -run TestWasmAssetSizeBudget ./internal/deploy/`. Record the size delta in progress.md.

- [ ] **Step 5: Checkpoint.**

---

### Task B2: The project document store and the user library

**Files:**
- Create: `src/go/internal/audio/instrument_docs.go` (no tag)
- Modify: `src/go/internal/audio/instrument_library.go` (user half), `src/go/internal/userprefs/instrument_store.go` (+ `_notjs.go`, `_js.go` unchanged backends)
- Test: `src/go/internal/audio/instrument_docs_test.go`, `src/go/internal/userprefs/instrument_store_test.go` (extend)

**Interfaces:**
- Produces:
  - Store: `InstrumentDoc(id) (instrument.Doc, bool)`, `SetInstrumentDoc(id string, d instrument.Doc) error` (Validate; `d.ID` forced to `id`; bumps `InstrumentDocRev`; invalidates the voice cache for id; calls `platformInstrumentDocChanged(id, d)`), `InstrumentDocRev(id) uint64`, `ResetInstrumentDocs()` (store = a copy of every library document, `Origin` = its id), `EnsureInstrumentDoc(id) (instrument.Doc, bool)`.
    - `EnsureInstrumentDoc`: present → return it. Otherwise the library doc for id. Otherwise, for an instance id `x--N`, the library doc for `InstanceBaseID(id)` with `ID = id` and `Origin = base`. Store and return it.
  - Hook: `var platformInstrumentDocChanged = func(id string, d instrument.Doc) {}`.
  - Library: `LibraryDoc(id) (instrument.Doc, bool)` (user over shipped), `LibraryIDs() []string` (shipped order, then user-only ids sorted), `UserLibraryDoc(id)`, `SaveLibraryDoc(d instrument.Doc) error` (Validate, store in memory, persist through the sink), `DeleteUserLibraryDoc(id) error`, `LoadUserInstrumentLibrary(src InstrumentLibrarySource) error`, `SetInstrumentLibrarySink(InstrumentLibrarySink)`.
  - `type InstrumentLibrarySource interface{ LoadInstrumentDocs() (map[string][]byte, error) }`, `type InstrumentLibrarySink interface{ SaveInstrumentDoc(id string, doc []byte) error; DeleteInstrumentDoc(id string) error }`.
  - userprefs store v2:
    - `instrumentsDoc{schema.Envelope; Instruments map[string]json.RawMessage `json:"instruments,omitempty"`; WavRegistry json.RawMessage `json:"wav_registry,omitempty"`}`
    - interface methods `LoadInstrumentDocs`, `SaveInstrumentDoc`, `DeleteInstrumentDoc`, `LoadWavRegistry`/`SaveWavRegistry` (kept), `WaitFlushed`, `Close`
    - `UserRecipes`/`RecipeOverrides`/`SampleEdits` and their methods are deleted
    - a store that still holds the legacy keys logs one WARN `[instruments] dropped N pre-document entries` and starts without them

- [ ] **Step 1: Failing tests** (fast path)

```go
package audio

import (
	"errors"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

type memLibrary struct{ docs map[string][]byte }

func (m *memLibrary) LoadInstrumentDocs() (map[string][]byte, error) { return m.docs, nil }
func (m *memLibrary) SaveInstrumentDoc(id string, b []byte) error {
	if m.docs == nil {
		m.docs = map[string][]byte{}
	}
	m.docs[id] = b
	return nil
}
func (m *memLibrary) DeleteInstrumentDoc(id string) error { delete(m.docs, id); return nil }

func resetLibraryForTest(t *testing.T) *memLibrary {
	t.Helper()
	lib := &memLibrary{}
	SetInstrumentLibrarySink(lib)
	if err := LoadUserInstrumentLibrary(lib); err != nil {
		t.Fatal(err)
	}
	ResetInstrumentDocs()
	t.Cleanup(func() {
		SetInstrumentLibrarySink(nil)
		_ = LoadUserInstrumentLibrary(&memLibrary{})
		ResetInstrumentDocs()
	})
	return lib
}

func TestStoreStartsWithLibraryCopies(t *testing.T) {
	resetLibraryForTest(t)
	d, ok := InstrumentDoc("kick-snap")
	if !ok || d.Origin != "kick-snap" {
		t.Fatalf("store copy: %+v %v", d.Origin, ok)
	}
	inst, ok := EnsureInstrumentDoc("voice-ahh--2")
	if !ok || inst.ID != "voice-ahh--2" || inst.Origin != "voice-ahh" {
		t.Fatalf("instance copy: id=%s origin=%s ok=%v", inst.ID, inst.Origin, ok)
	}
}

func TestSetInstrumentDocValidatesAndBumpsRev(t *testing.T) {
	resetLibraryForTest(t)
	d, _ := InstrumentDoc("kick-snap")
	rev := InstrumentDocRev("kick-snap")
	bad := d.Clone()
	delete(bad.Stages, "core")
	if err := SetInstrumentDoc("kick-snap", bad); err == nil {
		t.Fatal("an invalid document must be refused")
	}
	d2, _ := instrument.SetField(d, BuiltinModuleRegistry(), "gen1_kick_click", 0.9)
	if err := SetInstrumentDoc("kick-snap", d2); err != nil || InstrumentDocRev("kick-snap") == rev {
		t.Fatalf("set: %v", err)
	}
}

// Review Focus 4 (boot store half).
func TestUserLibrarySkipsInvalidDocument(t *testing.T) {
	good, _ := ShippedDoc("kick-snap")
	good.ID, good.Name = "user.snap.abc123", "My Snap"
	gb, _ := instrument.MarshalFile(good)
	lib := &memLibrary{docs: map[string][]byte{
		"user.snap.abc123": gb,
		"user.bad.000000":  []byte(`{"schema_version":"0.0.1","id":"user.bad.000000","stages":{}}`),
	}}
	var warned []string
	restore := setLibraryWarnfForTest(func(f string, a ...any) { warned = append(warned, f) })
	defer restore()
	if err := LoadUserInstrumentLibrary(lib); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = LoadUserInstrumentLibrary(&memLibrary{}) }()
	if _, ok := UserLibraryDoc("user.snap.abc123"); !ok {
		t.Fatal("the valid entry must load")
	}
	if _, ok := UserLibraryDoc("user.bad.000000"); ok || len(warned) != 1 || !strings.Contains(warned[0], "skipped") {
		t.Fatalf("the invalid entry must be skipped with one WARN: %v", warned)
	}
}

func TestSaveLibraryDocShadowsShipped(t *testing.T) {
	lib := resetLibraryForTest(t)
	d, _ := ShippedDoc("kick-snap")
	d, _ = instrument.SetField(d, BuiltinModuleRegistry(), "gen1_kick_click", 0.9)
	if err := SaveLibraryDoc(d); err != nil {
		t.Fatal(err)
	}
	got, _ := LibraryDoc("kick-snap")
	if got.Layers[0].Values["kick_click"] != 0.9 || lib.docs["kick-snap"] == nil {
		t.Fatal("a saved document shadows the shipped one and persists")
	}
	if err := DeleteUserLibraryDoc("kick-snap"); err != nil {
		t.Fatal(err)
	}
	if got, _ := LibraryDoc("kick-snap"); got.Layers[0].Values["kick_click"] == 0.9 {
		t.Fatal("Restore shipped must bring the shipped document back")
	}
	if err := DeleteUserLibraryDoc("kick-snap"); !errors.Is(err, ErrNoUserLibraryDoc) {
		t.Fatalf("want ErrNoUserLibraryDoc, got %v", err)
	}
}
```

- [ ] **Step 2: Implement.** Key code for the store:

```go
package audio

import (
	"fmt"
	"sync"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The project's instrument documents (spec §6): one copy per instrument id
// in use (rows are keyed by instrument id). The ONLY input of a voice render.
var docStore = struct {
	sync.RWMutex
	docs map[string]instrument.Doc
	rev  map[string]uint64
}{docs: map[string]instrument.Doc{}, rev: map[string]uint64{}}

// platformInstrumentDocChanged is replaced on js/wasm to push the flattened
// block to audio.js (instrument_docs_wasm.go).
var platformInstrumentDocChanged = func(id string, d instrument.Doc) {}

func InstrumentDoc(id string) (instrument.Doc, bool) {
	docStore.RLock()
	defer docStore.RUnlock()
	d, ok := docStore.docs[id]
	if !ok {
		return instrument.Doc{}, false
	}
	return d.Clone(), true
}

func InstrumentDocRev(id string) uint64 {
	docStore.RLock()
	defer docStore.RUnlock()
	return docStore.rev[id]
}

func SetInstrumentDoc(id string, d instrument.Doc) error {
	d = d.Clone()
	d.ID = id
	if err := instrument.Validate(d, BuiltinModuleRegistry()); err != nil {
		return fmt.Errorf("instrument %s: %w", id, err)
	}
	docStore.Lock()
	docStore.docs[id] = d
	docStore.rev[id]++
	docStore.Unlock()
	voiceCacheInvalidate(id)
	platformInstrumentDocChanged(id, d)
	return nil
}

func ResetInstrumentDocs() {
	docStore.Lock()
	docStore.docs = map[string]instrument.Doc{}
	for _, id := range LibraryIDs() {
		if d, ok := LibraryDoc(id); ok {
			d.Origin = id
			docStore.docs[id] = d
			docStore.rev[id]++
		}
	}
	snapshot := make(map[string]instrument.Doc, len(docStore.docs))
	for k, v := range docStore.docs {
		snapshot[k] = v
	}
	docStore.Unlock()
	for id, d := range snapshot {
		voiceCacheInvalidate(id)
		platformInstrumentDocChanged(id, d)
	}
}

func EnsureInstrumentDoc(id string) (instrument.Doc, bool) {
	if d, ok := InstrumentDoc(id); ok {
		return d, true
	}
	origin := id
	d, ok := LibraryDoc(id)
	if !ok {
		origin = InstanceBaseID(id)
		if d, ok = LibraryDoc(origin); !ok {
			return instrument.Doc{}, false
		}
	}
	d.Origin = origin
	if err := SetInstrumentDoc(id, d); err != nil {
		return instrument.Doc{}, false
	}
	return InstrumentDoc(id)
}
```

`voiceCacheInvalidate(id)` already exists (it is used by recipe_overrides.go:177). If its name differs, grep `func voiceCacheInvalidate`.

User library (in `instrument_library.go`):
- a `userLib` map behind a mutex;
- `ErrNoUserLibraryDoc`;
- `libraryWarnf` (a func var defaulting to `log.Warnf`, with a `setLibraryWarnfForTest` seam).

`LoadUserInstrumentLibrary` does, per entry:
1. `instrument.ParseFile`, then `Migrate`, then `Validate`;
2. checks the entry key == `doc.ID`;
3. on failure: one `libraryWarnf("[instruments] skipped %s: %v", id, err)` and skip.

It replaces the whole user map. `SaveLibraryDoc` marshals with `instrument.MarshalFile` and calls the sink.

- [ ] **Step 3: Rewrite the userprefs store** to the v2 shape above. Update `internal/userprefs/instrument_store_test.go`:
  - round trip `SaveInstrumentDoc` → new store → `LoadInstrumentDocs`;
  - a file holding `user_recipes` loads empty with one WARN.

  Keep the atomic-write desktop backend and the localStorage JS backend unchanged.

- [ ] **Step 4: Run** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'TestStore|TestSetInstrumentDoc|TestUserLibrary|TestSaveLibraryDoc' ./internal/audio/ && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/userprefs/`.

- [ ] **Step 5: Checkpoint.**

---

### Task B3: One native voice path

**Files:**
- Create: `src/go/internal/audio/doc_voice_native.go` (`//go:build !test && !js`)
- Modify:
  - `synth_recipe_dispatch.go`: `newRecipeAwareVoice(Origin)`, `newRecipeAwareVoicePitched(Origin)`, `tryRecipeVoice*`, `pitchedVoiceKey`, `pitchedVoiceKeyForParams`, `instrumentDurationSamples` all route to the doc path
  - `voice_prewarm_desktop.go`, `voice_warm_desktop.go` (`pitchAwareRecipe(RecipeForInstrument(id))` → `docMelodic(id)`)
  - `engine_instruments.go` (`ResetInstruments`: register `docInstrument{id}` for every `LibraryIDs()` id instead of `modularTableVoices` + the hand `modular` entry)
  - `config.go`, `config_stub.go`, `config_wasm.go`: `ConfigForInstrument` reads `render.seconds/amplitude` from the store when id has a document, with no loudness overlay for those ids
  - `sample_capture_notjs.go` (`renderInstrumentOneShotOpts` → doc path)
- Test: `src/go/internal/audio/doc_voice_test.go` (`!test && !js`)

**Interfaces:**
- Produces:
  - `docVoiceBuffer(id string, bpm, sampleRate int, pitch float64, pitched, ignoreEdit bool, origin renderOrigin) ([]float32, voiceCacheKey, bool)`
  - `type docInstrument struct{ id string }` (implements `Instrument`)
  - `docMelodic(id) bool`
  - `beatsDurationSamples(beats float64, bpm, sampleRate int) int`, which uses exactly the `time.Duration` arithmetic of `CVariantInstrument.renderAndCache` (variants.go:62-66)

The order of operations is the baked path's (variants.go:61-80): render, then `applyDefaultFX(fx)`, then `gateTail`, then `normalizeAndScale`. The Sampler edit and the pitch handling are the recipe path's (synth_recipe_dispatch.go:174-246): `block[pitch] = pitch` (exact), with the key's pitch = `roundPitchForCache(pitch)`.

- [ ] **Step 1: Failing tests**

```go
//go:build !test && !js

package audio

import (
	"slices"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// The default one-shot of every shipped instrument equals today's baked path
// (CVariantInstrument: render → DefaultFX → gateTail → normalize).
func TestDocVoiceEqualsBakedVoice(t *testing.T) {
	resetLibraryForTest(t)
	for _, def := range ModularInstrumentDefs() {
		baked := CVariantInstrument{Name: def.ID, Render: bakedModularRender(def.Seed), Beats: def.Beats, DefaultFX: def.DefaultFX}
		if def.GateTail > 0 {
			g := def.GateTail
			baked.Post = func(buf []float32, _ int) { gateTail(buf, g) }
		}
		want := baked.renderAndCache(voiceCacheKey{instrumentID: "baked:" + def.ID, bpm: 120, sampleRate: 44100}, 120, 44100)
		got, _, ok := docVoiceBuffer(def.ID, 120, 44100, 0, false, true, originPrewarm)
		if !ok || !slices.Equal(got, want) {
			t.Errorf("%s: doc voice differs from the baked voice", def.ID)
		}
	}
}

// The latent bug the switch fixes (spec §7.1): an edited clap-bright keeps its
// filter and gate.
func TestDocVoiceAppliesFXAndGateAfterAnEdit(t *testing.T) {
	resetLibraryForTest(t)
	d, _ := InstrumentDoc("clap-bright")
	d, _ = instrument.SetField(d, BuiltinModuleRegistry(), "gen1_snare_attack", 0.5)
	if err := SetInstrumentDoc("clap-bright", d); err != nil {
		t.Fatal(err)
	}
	buf, _, _ := docVoiceBuffer("clap-bright", 120, 44100, 0, false, true, originPrewarm)
	cut := int(float64(len(buf)) * 0.5)
	for i := cut; i < len(buf); i++ {
		if buf[i] != 0 {
			t.Fatalf("gate_tail 0.5 must zero the second half, sample %d = %v", i, buf[i])
		}
	}
}

func TestPitchedDocVoiceKeysOnRoundedPitch(t *testing.T) {
	resetLibraryForTest(t)
	_, k1, _ := docVoiceBuffer("violin", 120, 44100, 3.1, true, true, originPrewarm)
	_, k2, _ := docVoiceBuffer("violin", 120, 44100, 3.2, true, true, originPrewarm)
	if k1.pitch != 3 || k2.pitch != 3 || k1.paramsHash == k2.paramsHash {
		t.Fatalf("key pitch is rounded; the block carries the exact pitch: %+v %+v", k1, k2)
	}
}
```

- [ ] **Step 2: Implement `doc_voice_native.go`**

```go
//go:build !test && !js

package audio

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

// docInstrument is every synth instrument: its voice is rendered from the
// project's document for id (spec §8).
type docInstrument struct{ id string }

func (d docInstrument) NewVoice(bpm, sampleRate int) Voice {
	buf, _, ok := docVoiceBuffer(d.id, bpm, sampleRate, 0, false, false, originDispatch)
	if !ok {
		return nil
	}
	return &cVoice{buf: buf}
}

func beatsDurationSamples(beats float64, bpm, sampleRate int) int {
	spb := 60 / float64(bpm)
	dur := time.Duration(spb * beats * float64(time.Second))
	n := int(float64(sampleRate) * dur.Seconds())
	if n < 1 {
		n = 1
	}
	return n
}

func docMelodic(id string) bool {
	d, ok := InstrumentDoc(id)
	return ok && d.Render.Melodic
}

func effectSlotsOf(fx []instrument.Module) []EffectSlot {
	out := make([]EffectSlot, len(fx))
	for i, m := range fx {
		out[i] = EffectSlot{Type: EffectType(m.Type[len("fx."):]), Enabled: m.On, Params: map[string]float64(m.Values)}
	}
	return out
}

// docVoiceBuffer renders (or fetches from the voice cache) the voice of id's
// document. The returned buffer is shared read-only with the cache.
func docVoiceBuffer(id string, bpm, sampleRate int, pitch float64, pitched, ignoreEdit bool, origin renderOrigin) ([]float32, voiceCacheKey, bool) {
	doc, ok := EnsureInstrumentDoc(id)
	if !ok {
		return nil, voiceCacheKey{}, false
	}
	reg := BuiltinModuleRegistry()
	block := instrument.Flatten(doc, reg)
	key := voiceCacheKey{instrumentID: id, bpm: bpm, sampleRate: sampleRate}
	if pitched {
		i, _ := reg.FieldIndex("pitch")
		block[i] = pitch
		key.pitch = roundPitchForCache(pitch)
	}
	key.paramsHash = instrument.BlockHash(block)
	edit, hasEdit := SampleEditFor(id)
	if ignoreEdit {
		hasEdit = false
	}
	if hasEdit {
		key.paramsHash ^= hashSampleEdit(edit)
	}
	if buf, ok := globalVoiceCache.Get(key); ok {
		recordVoiceCacheHit()
		return buf, key, true
	}
	n := beatsDurationSamples(doc.Render.Beats, bpm, sampleRate)
	buf := make([]float32, n)
	start := time.Now()
	renderModularBlock(buf, sampleRate, n, modularBlock(block))
	if len(doc.FX) > 0 {
		applyDefaultFX(buf, sampleRate, effectSlotsOf(doc.FX))
	}
	if doc.Render.GateTail > 0 {
		gateTail(buf, doc.Render.GateTail)
	}
	normalizeAndScale(buf, id)
	recordVoiceRender(time.Since(start), origin)
	if hasEdit {
		buf = ApplySampleEditToBuffer(buf, sampleRate, edit)
	}
	globalVoiceCache.Put(key, buf)
	return buf, key, true
}
```

**Two behaviours to preserve and test:**
- **`bpm` in the key.** Today the baked key carries the bpm, and the recipe key carries `bpmKey` (0 for instruments without Beats). Every document has `beats > 0`, so carrying the bpm matches both paths.
- **The dispatch-must-not-render guard** (`dispatchMustNotRender` / `serveBestAvailable`, synth_recipe_dispatch.go:222-229). Keep it on the pitched path by moving those lines before the render inside `docVoiceBuffer` when `pitched && origin == originDispatch`.

- [ ] **Step 3: Re-point the dispatch.** In `synth_recipe_dispatch.go`, replace the bodies:

| Function | New body |
|---|---|
| `tryRecipeVoiceOptsOrigin` | `buf, _, ok := docVoiceBuffer(id, bpm, sr, 0, false, ignoreEdit, origin)` → `&cVoice{buf}` |
| `tryRecipeVoicePitchedOrigin` | same with `pitched=true` |
| `newRecipeAwareVoicePitchedOrigin` | `if docMelodic(id) { … pitched … }` |
| `pitchedVoiceKey` / `pitchedVoiceKeyForParams` | recompute the key exactly as `docVoiceBuffer` does, with no render (extract `docVoiceKey(id, bpm, sr, pitch, pitched, ignoreEdit) (voiceCacheKey, instrument.Doc, []float64, bool)` and call it from both) |

For instruments without a document (samples, catalog), keep `legacyNewVoice`. `TestVoiceCachedMatchesDispatch` must stay green unchanged.

- [ ] **Step 4: Registration and config.**
  - `ResetInstruments` (engine_instruments.go:30–74): call `ResetInstrumentDocs()` first, then `Register(id, docInstrument{id})` for every `LibraryIDs()` id.
  - Delete the `"modular"` `CVariantInstrument` literal and the `modularTableVoices(...)` call.
  - In the three `ConfigForInstrument` files, prepend:
    ```go
    if d, ok := InstrumentDoc(id); ok {
        return InstrumentConfig{ID: id, DurationSec: d.Render.Seconds, Amplitude: float32(d.Render.Amplitude)}
    }
    ```
  - `EnsureInstanceInstrument` (instance_instrument.go:77) calls `EnsureInstrumentDoc(id)` and registers `docInstrument{id}` when it returns ok. Its `BindInstrumentToRecipe` / loudness / defaults-push lines go.

- [ ] **Step 5: Run.**

```bash
cd src/go && ../../.tools/go/bin/go test -run 'TestDocVoice|TestPitchedDocVoice|TestVoiceCachedMatchesDispatch|TestPitchedVoiceHonoursASampleEdit|TestSampleEditDispatch' ./internal/audio/
```

Expected: PASS. Then run the parity dump label `B3`:

```bash
cd src/go && BEATMO_RENDER_PARITY_DIR=$HOME/.beatmo/render-parity BEATMO_RENDER_PARITY_LABEL=B3 ../../.tools/go/bin/go test -run TestRenderParityDump ./internal/audio/
BEATMO_RENDER_PARITY_DIR=$HOME/.beatmo/render-parity BEATMO_RENDER_PARITY_A=Ffix2 BEATMO_RENDER_PARITY_B=B3 ../../.tools/go/bin/go test -run TestRenderParityCompare ./internal/audio/
```

Every `.f32` default must be bit-exact. Engine cases are compared in B10.

- [ ] **Step 6: Checkpoint.**

---

### Task B4: Flat-field API over documents, and the Synth tab's adapter

**Files:**
- Create: `src/go/internal/audio/instrument_param_defs.go`
- Modify:
  - `synth_recipe.go`: `SetInstrumentParam` (:221), `SetInstrumentParams` (:299), `GetInstrumentParams` (:359), `ResetInstrumentParams` (:368)
  - `merged_params_cache.go` (`MergedInstrumentParamsRO` → `FieldValues`, cached by `InstrumentDocRev`)
  - `recipe_overrides.go` (`InstrumentParamsDiffer`)
  - `internal/ui/synth_panel_zone.go`: `:1074-1080` schema source, `:981` `instrumentHasSynthControls`, `:180` `schemaRecipeID`, `:149-160`, `:473-560` hide predicates, `:642-649` `synthParamHiddenInContext`
  - `synth_concept_viz.go:590-617`, `synth_preview_layer.go:240-252`
  - `modular_stage_params.go:146` (`IsAppendedStageName` drops its recipe argument)
  - `internal/ui/scene_catalog.go`: synth scenes using `SetInstrumentParam` keep working unchanged
- Test: `src/go/internal/audio/instrument_param_defs_test.go`, `src/go/internal/audio/flat_field_api_test.go`, and existing UI tests

**Interfaces:**
- Produces:
  - `InstrumentParamDefs(id) []ParamDef`: `ModularSynthParamDefs()` (cached base, copied) with `Default = FieldValues[name]`, the range widened to contain the value, the `voice_freq_hz` log range when > 0 (the BuildBuiltinRecipeDocs rule, synth_recipe.go:595-603), and `applyFamilyKnobRanges(defs, values)`.
  - `InstrumentDeclaresField(id, name) bool`:
    - stage values and stage enable fields are always declared (the UI shows absent stages dimmed);
    - `osc_enabled` and `osc_type` are always declared;
    - a source value is declared iff the current source type declares it;
    - a `gen1_*` field is declared iff layer 0's type declares it as a value, a `Set` field or its Enable field;
    - `Global` fields are never declared.
  - `SetInstrumentParam(id, name, v)`, same signature:
    1. `knownParamName`, then `sanitizeParamValue` (ParamDef clamp, unchanged);
    2. `doc, _ := EnsureInstrumentDoc(id)`;
    3. `instrument.SetField`;
    4. on `ErrInactive` / `ErrNotSettable`: `log.Debugf` and return;
    5. otherwise `SetInstrumentDoc`, then the existing hooks publish (keep the event).
  - `SetInstrumentParams(id, p)`: start from `LibraryDoc(origin)` and apply `SetField` for each key in sorted order.
  - `GetInstrumentParams(id)` = `OwnedFields(doc)`.
  - `ResetInstrumentParams(id)` = the library doc for `origin`, with ID = id and the same Origin.
  - `InstrumentDocModified(id) bool` = `len(Diff(normalised doc, normalised LibraryDoc(origin))) > 0`, where normalised = ID and Origin blanked. `InstrumentParamsDiffer` becomes a thin call to it.

- [ ] **Step 1: Failing tests (fast path)**

```go
package audio

import "testing"

func TestInstrumentParamDefsDefaultIsTheDocumentValue(t *testing.T) {
	resetLibraryForTest(t)
	defs := InstrumentParamDefs("kick-snap")
	for _, d := range defs {
		if d.Name == "gen1_kick_click" && d.Default != 0.45 {
			t.Fatalf("Default = %v, want the document's 0.45", d.Default)
		}
		if d.Default < d.Min || d.Default > d.Max {
			t.Fatalf("%s: Default %v outside [%v,%v] — the range must widen", d.Name, d.Default, d.Min, d.Max)
		}
	}
	if len(defs) != len(ModularSynthParamDefs()) {
		t.Fatal("one ParamDef per ABI field, as the Synth tab's bindings expect")
	}
}

func TestDeclaresFieldHidesDeadKnobs(t *testing.T) {
	resetLibraryForTest(t)
	cases := []struct {
		id, field string
		want      bool
	}{
		{"kick-snap", "gen1_kick_click", true},
		{"kick-snap", "gen1_kick_h3", false}, // gen_kick_punchy never reads h3
		{"kick-snap", "gen1_kick_variant", true},
		{"kick-snap", "kick_enabled", false},
		{"kick-snap", "filter_cutoff", true}, // stages are always offered
		{"violin", "osc_bow_pos", true},
		{"violin", "fm_op1_ratio", false}, // FM operators belong to osc.fm
		{"violin", "osc_type", true},
	}
	for _, c := range cases {
		if got := InstrumentDeclaresField(c.id, c.field); got != c.want {
			t.Errorf("%s %s = %v, want %v", c.id, c.field, got, c.want)
		}
	}
}

func TestSetInstrumentParamEditsTheDocument(t *testing.T) {
	resetLibraryForTest(t)
	SetInstrumentParam("kick-snap", "gen1_kick_click", 0.9)
	d, _ := InstrumentDoc("kick-snap")
	if d.Layers[0].Values["kick_click"] != 0.9 || !InstrumentDocModified("kick-snap") {
		t.Fatal("SetInstrumentParam must edit the project copy")
	}
	ResetInstrumentParams("kick-snap")
	if InstrumentDocModified("kick-snap") {
		t.Fatal("Reset returns the copy to its library entry")
	}
	SetInstrumentParam("violin", "fm_op1_ratio", 3) // inactive: ignored, no panic
	if v := MergedInstrumentParamsRO("violin")["fm_op1_ratio"]; v == 3 {
		t.Fatal("an inactive field must not be stored")
	}
}
```

- [ ] **Step 2: Implement** the functions above.
  - In `InstrumentParamDefs`, cache `ModularSynthParamDefs()` once (a package `sync.Once`) and copy the slice per call.
  - Keep `applyFamilyKnobRanges` as is; it reads `seed["gen1_source"]` and `params[i].Default`, so pass the `FieldValues` map.

- [ ] **Step 3: Switch the Synth tab to the adapter.**
  1. Every `audio.RecipeRegistrations()[audio.RecipeForInstrument(x)].Params` in the files listed (grep `RecipeRegistrations()` in `internal/ui/*.go`, non-test) becomes `audio.InstrumentParamDefs(x)`.
  2. `schemaRecipeID` is deleted. `IsAppendedStageName(recipeID, name)` becomes `IsAppendedStageName(name)` (every instrument is modular; read modular_stage_params.go:146 and drop the recipe lookup).
  3. In `synthParamHiddenInContext`, replace the four contextual predicates with one line: `!audio.InstrumentDeclaresField(instID, def.Name)`. The four are `synthFamilyParamHidden`, `synthOscModelParamHidden`, `synthCymbalModalParamHidden` and `synthKickPhysicalParamHidden`. Keep `def.Group == audio.SynthHiddenGroup` and `synthVoiceFreqParamHidden`.
  4. Use the same replacement in `synthSectionOrderForSchema` (:156). Delete the four predicate functions and their dedicated tests. Keep the scenes.

  Whoever needs the instrument id in those predicates already has `dv.selRow`'s instrument; follow the existing call chain.

- [ ] **Step 4: Run** the fast-path UI synth tests:

```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -run 'Synth|Macro|Focus|Concept|PlainEnglish|Knob' -timeout 20m ./internal/ui/
```

Each failure is one of three things:
- **A test asserting a knob that is now hidden because the module doesn't declare it.** Update the expectation to the declared set, with a comment naming the module type. For example, the FM section no longer appears on non-FM sources.
- **A test reading `RecipeRegistrations()[…].Params`.** Switch it to `InstrumentParamDefs`.
- **Anything else is a bug.** Fix the code.

Record every expectation change in progress.md.

- [ ] **Step 5: Screenshots (before/after).**
  - Build the binary from the pre-B4 tree (the user's last tree state, or ask) and from the current tree. Capture `synth_tab`, `crop_synth_snare_generator_voice`, a kick synth scene and a violin synth scene (list them with `-list-scenes | grep synth`), desktop and mobile, with `XDG_CONFIG_HOME=$S/xdg`.
  - Open every pair. Expected: identical, except that knobs of undeclared values are gone (kick h3 on punchy, FM operators on non-FM sources).
  - Record the before/after paths in progress.md.

- [ ] **Step 6: Checkpoint.**

---

### Task B5: The browser receives blocks

**Files:**
- Create: `src/go/internal/audio/instrument_docs_wasm.go` (`//go:build js && wasm`)
- Modify:
  - `src/go/internal/audio/synth_recipe_wasm.go`: delete the `platformInstrumentParamsChanged`, `platformInstrumentRecipeChanged` and `platformInstrumentDefaultsPush` JS bodies; keep `platformSampleEditChanged`
  - `src/go/internal/ui/js_exports_synth_recipe.go:272`: `SeedInstrumentDefaultsToPlatform()` → `audio.PushAllInstrumentDocsToPlatform()`
  - `src/go/internal/audio/sample_pcm_wasm.go`: after an unregister, re-push the id's block
  - `src/js/audio.js`
  - `src/go/internal/audio/synth_recipe_invariants_test.go:232-260`: scan for the new globals
  - browser tests that call `window.updateInstrumentParams` (`audio_lane_cancel` :163, `instrument_params` :117/:213-253, `synth_sample_param_change_keeps_audio` :122): use Go's `setInstrumentParam` or `setInstrumentBlock`
  - `src/js/wasm_bridge_smoke.browser.test.js`: catalogue

**Interfaces:**
- Produces:
  - JS `window.setInstrumentBlock(id, Float64Array, seconds, amp, melodic)` and `window.clearInstrumentBlock(id)`
  - Go `PushAllInstrumentDocsToPlatform()`
  - the js hook body of `platformInstrumentDocChanged`

- [ ] **Step 1: Go push (`instrument_docs_wasm.go`)**

```go
//go:build js && wasm

package audio

import (
	"encoding/binary"
	"math"
	"syscall/js"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func init() {
	platformInstrumentDocChanged = pushInstrumentDoc
}

func pushInstrumentDoc(id string, d instrument.Doc) {
	fn := js.Global().Get("setInstrumentBlock")
	if fn.Type() != js.TypeFunction {
		return // audio.js not loaded (Go-only harness)
	}
	block := instrument.Flatten(d, BuiltinModuleRegistry())
	raw := make([]byte, 8*len(block))
	for i, x := range block {
		binary.LittleEndian.PutUint64(raw[8*i:], math.Float64bits(x))
	}
	f64 := js.Global().Get("Float64Array").New(len(block))
	js.CopyBytesToJS(js.Global().Get("Uint8Array").New(f64.Get("buffer")), raw)
	fn.Invoke(id, f64, d.Render.Seconds, d.Render.Amplitude, d.Render.Melodic)
}

// PushAllInstrumentDocsToPlatform pushes every stored document (boot).
func PushAllInstrumentDocsToPlatform() {
	docStore.RLock()
	snap := make(map[string]instrument.Doc, len(docStore.docs))
	for k, v := range docStore.docs {
		snap[k] = v
	}
	docStore.RUnlock()
	for id, d := range snap {
		pushInstrumentDoc(id, d)
	}
}
```

Put an untagged no-op `PushAllInstrumentDocsToPlatform` in `instrument_docs_notjs.go` (`//go:build !js`).

- [ ] **Step 2: audio.js.**
  - **Delete:** the `MODULAR_PARAM_IDENTITY` import; the `modular_instruments.gen.js` import and the `Object.assign(RENDER…)` lines; `instrumentSynthParams`, `instrumentDefaultParams`, `seedInstrumentDefaults`, `_updateInstrumentParams`, `window.updateInstrumentParams`, `window.updateInstrumentRecipe`, `renderInfoFactory`, `instrumentParamsFor`, `identityParamBlock`, and the `INSTRUMENT_LOUDNESS_AMP` override loop (audio.js:1790-1794; amplitude now arrives with the block).
  - **Keep `RENDER_DEFAULTS` only if a sample-override path still needs it.** Otherwise delete it, and make the unregister site rely on Go's re-push.
  - **Add:**

```js
// One complete param block per instrument, flattened in Go from its instrument
// document (spec §8). The ONLY parameter source a render reads.
const instrumentBlocks = new Map();

window.setInstrumentBlock = (id, block, seconds, amp, melodic) => {
  if (!(block instanceof Float64Array) || block.length !== MODULAR_PARAM_COUNT) {
    throw new Error(`setInstrumentBlock(${id}): want a Float64Array(${MODULAR_PARAM_COUNT})`);
  }
  instrumentBlocks.set(id, block);
  RENDER[id] = 'render_modular_p';
  RENDER_INFO[id] = { seconds, amp, paramBlock: 'modular' };
  if (melodic) MELODIC_INSTRUMENTS.add(id); else MELODIC_INSTRUMENTS.delete(id);
  bumpInstrumentParamsGen(id);
  delete samples[id];
  rawRenderCache.delete(id);
};

window.clearInstrumentBlock = (id) => {
  instrumentBlocks.delete(id);
  delete RENDER[id];
  delete RENDER_INFO[id];
  MELODIC_INSTRUMENTS.delete(id);
  evictRenderCache(id);
};

// Engine warm-up block (compile/tier-up only — never an instrument).
function engineWarmBlock() {
  const b = new Float64Array(MODULAR_PARAM_COUNT);
  b[MODULAR_PARAM_INDEX.osc_enabled] = 1;
  b[MODULAR_PARAM_INDEX.env_enabled] = 1;
  b[MODULAR_PARAM_INDEX.amp_sustain] = 1;
  b[MODULAR_PARAM_INDEX.gain] = 1;
  return b;
}
```

  - `const MELODIC_INSTRUMENTS = new Set()` (no generated seed list).
  - `const RENDER = {}` and `const RENDER_INFO = {}`. The `modular` entry is pushed like every other.
  - In `ensureRenderedSample` (audio.js:4158-4191), replace the block assembly with:

```js
const pushed = instrumentBlocks.get(id);
if (!pushed) return null; // Go has not pushed this instrument
const paramArr = Float64Array.from(pushed);
if (atPitch) paramArr[MODULAR_PARAM_INDEX.pitch] = roundedPitch;
```

  - `warmupAudio()` uses `engineWarmBlock()` instead of `identityParamBlock()`.

- [ ] **Step 3: Delete the generator and its outputs.** Remove `src/js/modular_instruments.gen.js`, `src/go/cmd/gen-modular-instruments/`, `modular_instruments_gen_test.go`, the Makefile target, and the file from the `ENTRY_FILES` / `ASSET_FILES` arrays in `scripts/deploy-wasm-gcs.sh`. `manifest_drift_test.go` enforces the last one.

- [ ] **Step 4: Run.**

```bash
make wasm
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/deploy/ ./internal/audio/ -run 'Manifest|Invariant|Catalogue'
GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/modular_synth_audible.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/import_synth_render.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/instrument_params.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/audio_lane_cancel.browser.test.js
GO=$(pwd)/.tools/go/bin/go node src/js/xplat_audio_compare.browser.test.js
```

Expected: PASS. `xplat_audio_compare` is native == WASM for the same pushed block. **Audit instruments:** any instrument on the Task A10 list now plays its baked values in the browser too. Record which browser goldens moved; only the user-approved list may move.

- [ ] **Step 5: Checkpoint.**

---

### Task B6: Save, Save As, Reset, Restore shipped

**Files:**
- Modify:
  - `src/go/internal/ui/recipe_save_sink.go`: `saveRecipeForInstrument` (:73-100), `SaveActiveRecipeAs` (:184-233), `resetRecipeForInstrument` (:119-132), `RecipeSaveSink`, `SetRecipeSink`
  - `src/go/internal/ui/action_menu_open.go:47-64` (add Restore shipped)
  - `src/go/internal/ui/synth_panel_zone.go:2984` (dirty dot → `audio.InstrumentDocModified`)
  - `src/go/cmd/beatmo.go:206-212`, `src/go/internal/ui/js_bootstrap_wasm.go:38-43` (boot wiring)
  - `src/go/internal/ui/drumview_transport.go:357` (`SetInstrument`: picking a library id that no row uses refreshes the store copy from the library)
  - `scene_catalog.go` (new scene `synth_action_menu_restore_shipped`)
- Test: `src/go/internal/ui/instrument_library_flow_test.go` (real `Game.Update` input where a button is involved)

**Interfaces:**
- Produces:
  - `(*DrumView).SaveActiveRecipe()`, `SaveActiveRecipeAs(name)`, `ResetActiveRecipe()` (names kept, so the call sites and JS exports stay); new `RestoreShippedActiveInstrument()`
  - `audio.NewUserInstrumentID(name string, d instrument.Doc) string` = `"user." + slug(name) + "." + ContentHash(d)[7:13]`. `slug`: lowercase, runs of non-`[a-z0-9]` → `-`, trimmed, ≤ 24 chars.

| Action | Behaviour |
|---|---|
| **Save** | `lib := doc` with `ID = origin` (or the id when origin is empty) and `Origin = ""`; `audio.SaveLibraryDoc(lib)`; publish `EventRecipeSaved` (keep the kind). |
| **Save As** | `lib := doc` with `Name = name`, `ID = NewUserInstrumentID(name, doc)`, `Origin = ""`; `SaveLibraryDoc`; `audio.Register(lib.ID, docInstrument{lib.ID})` (native) / `EnsureInstrumentDoc(lib.ID)` so the menu shows it; project copy `Origin = lib.ID` via `SetInstrumentDoc`; `refreshInstruments()`; publish `EventRecipeCreated`. |
| **Reset** | `audio.ResetInstrumentParams(instID)` (B4); keep the emit + `recordUndo`. |
| **Restore shipped** | Enabled only when `audio.UserLibraryDoc(origin)` exists and `audio.ShippedDoc(origin)` exists. `audio.DeleteUserLibraryDoc(origin)`. Does not touch the project copy. Publish `EventRecipeSaved` with a `restored` detail; check `eventlogger` coverage. |

- [ ] **Step 1: Failing tests**

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

type memInstrumentLibrary struct{ docs map[string][]byte }

func (m *memInstrumentLibrary) LoadInstrumentDocs() (map[string][]byte, error) { return m.docs, nil }
func (m *memInstrumentLibrary) SaveInstrumentDoc(id string, b []byte) error {
	if m.docs == nil {
		m.docs = map[string][]byte{}
	}
	m.docs[id] = b
	return nil
}
func (m *memInstrumentLibrary) DeleteInstrumentDoc(id string) error { delete(m.docs, id); return nil }

// newDocsGame is a Game with a private, empty user instrument library and the
// given instruments on rows 0..n-1, picked through SetInstrument (the real
// pick path). Shared by the B6/B7/D1 tests — see testutil_test.go's header for
// the conventions it follows (assertDefaultParityState, withDefaultAudio).
func newDocsGame(t *testing.T, instruments ...string) *Game {
	t.Helper()
	assertDefaultParityState(t)
	withDefaultAudio(t)
	lib := &memInstrumentLibrary{}
	audio.SetInstrumentLibrarySink(lib)
	if err := audio.LoadUserInstrumentLibrary(lib); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		audio.SetInstrumentLibrarySink(nil)
		_ = audio.LoadUserInstrumentLibrary(&memInstrumentLibrary{})
		audio.ResetInstrumentDocs()
	})
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	for len(g.drum.Rows) < len(instruments) {
		g.drum.AddRow()
	}
	for i, inst := range instruments {
		g.drum.selRow = i
		g.drum.SetInstrument(inst)
	}
	return g
}

// Review Focus 5.
func TestSaveChangesFuturePicksOnly(t *testing.T) {
	g := newDocsGame(t, "voice-ahh")
	audio.SetInstrumentParam("voice-ahh", "amp_attack", 0.5)
	if _, ok := audio.EnsureInstrumentDoc("voice-ahh--2"); !ok {
		t.Fatal("instance copy")
	}
	g.drum.saveRecipeForInstrument("voice-ahh")
	lib, _ := audio.LibraryDoc("voice-ahh")
	if lib.Stages["amp_env"].Values["amp_attack"] != 0.5 {
		t.Fatal("Save writes the library entry")
	}
	inst, _ := audio.InstrumentDoc("voice-ahh--2")
	if inst.Stages["amp_env"].Values["amp_attack"] == 0.5 {
		t.Fatal("an existing project copy must keep its own values")
	}
}

func TestSaveAsRepointsOriginAndRegisters(t *testing.T) {
	g := newDocsGame(t, "kick-snap")
	audio.SetInstrumentParam("kick-snap", "gen1_kick_click", 0.9)
	g.drum.SaveActiveRecipeAsFor("kick-snap", "My Snap") // test seam over SaveActiveRecipeAs
	d, _ := audio.InstrumentDoc("kick-snap")
	lib, ok := audio.LibraryDoc(d.Origin)
	if !ok || lib.Name != "My Snap" || d.Origin[:5] != "user." || audio.InstrumentDocModified("kick-snap") {
		t.Fatalf("origin=%s lib=%+v", d.Origin, lib.Name)
	}
}
```

Put `newDocsGame` and `memInstrumentLibrary` in `instrument_library_flow_test.go`; B7 and D1 reuse them. `SaveActiveRecipeAs` reads the selected row today, so add the seam `SaveActiveRecipeAsFor(instID, name string)`, which holds the logic, and have `SaveActiveRecipeAs` call it with the selected row's instrument.

- [ ] **Step 2: Implement** as in the table. Boot wiring:
  - `cmd/beatmo.go` replaces the `SetUseUserRecipeOverrides` / `ApplyUserRecipeOverrides` / `SetRecipeSink` lines with:
    ```go
    audio.SetInstrumentLibrarySink(instStore)
    if err := audio.LoadUserInstrumentLibrary(instStore); err != nil {
        logger.Warnf("[instruments] %v", err)
    }
    ```
  - then `audio.ResetInstruments()` as the existing boot order requires; read `cmd/beatmo.go:200-235` and put the load BEFORE the first `ResetInstruments`.
  - `js_bootstrap_wasm.go` gets the same change.

- [ ] **Step 3: The ⋯ menu item.**
  1. Add "Restore shipped" to `openSynthActionMenu` through the shared action-menu builder, using the same pattern as Reset. Add its i18n key next to Reset's.
  2. Add the scene `synth_action_menu_restore_shipped`: Setup saves kick-snap and opens the ⋯ menu.
  3. Capture desktop + mobile and open the PNGs. The item must be visible, labelled, and not overlapping anything.
  4. Add a row to `TestEveryMenuClosesTheSameWay` only if this creates a new menu; it doesn't, since it is an item.
  5. Add a real-input test: tap the item through `Game.Update` and assert `audio.UserLibraryDoc("kick-snap")` is gone.

- [ ] **Step 4: Run** the new tests plus `-run 'Recipe|Save|Reset|ActionMenu'` on the fast path.

- [ ] **Step 5: Checkpoint.**

---

### Task B7: Project files carry documents

**Files:**
- Modify:
  - `src/go/internal/ui/project_file.go:18-119`: delete `Recipes`; add `InstrumentDocs`; `validateProjectRecipes` → `validateProjectDocs`
  - `src/go/internal/ui/export.go:20-54`: delete `Recipe`, `SynthParams`
  - `export.go:452-468`: delete the delta block
  - `export.go:494-512`: replace the recipes block with the docs map
  - `src/go/internal/ui/import.go`: delete the :182-193 recipe registration; replace the :709-740 binding block
  - `src/go/internal/ui/undo_change_detail.go:149-170`: diff documents
  - `src/go/cmd/gen-showcase/main.go:243-246`: stop emitting `synth_params`; delete `needsSeedTrigger` + `seededModularInstruments` (main.go:63-89); `make templates`
- Test: `src/go/internal/ui/project_docs_test.go`; adapt `import_validation_test.go`, `import_export_logic_test.go`, `export_import_master_audio_*` (real build)

**Interfaces:**
- Produces:
  - `projectFile.InstrumentDocs map[string]instrument.Doc `json:"instrument_docs,omitempty"``
  - export writes the stored document for every row instrument that has one
  - import:
    1. before any reset, for each entry: the key must equal `doc.ID` (or `doc.ID` is empty and gets set to the key), then `Migrate`, then `Validate`;
    2. any failure is a hard error naming `instrument_docs.<key>: <path…>`;
    3. a row carrying the legacy fields `recipe` / `synth_params` is a hard error: `instrument row %q predates instrument documents (recipe/synth_params); re-save it from an older build is not supported`;
    4. after `ResetInstrumentDocs()`, `SetInstrumentDoc` for each entry; rows without an entry use `EnsureInstrumentDoc`.

  Detect the legacy fields with a second, small unmarshal of the `instruments` array into `[]struct{ Recipe string `json:"recipe"`; SynthParams map[string]float64 `json:"synth_params"` }`.

- [ ] **Step 1: Failing tests**

```go
package ui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestExportCarriesEditedDocument(t *testing.T) {
	g := newDocsGame(t, "kick")
	audio.SetInstrumentParam("kick", "gen1_kick_click", 0.9)
	var f projectFile
	if err := json.Unmarshal(g.drum.exportBytes(), &f); err != nil {
		t.Fatal(err)
	}
	d, ok := f.InstrumentDocs["kick"]
	if !ok || d.Layers[0].Values["kick_click"] != 0.9 {
		t.Fatalf("instrument_docs[kick] = %+v", d)
	}
}

// Review Focus 4 (project half).
func TestImportRejectsUnknownModuleTypeBeforeReset(t *testing.T) {
	g := newDocsGame(t, "kick")
	before := g.drum.exportBytes()
	var f map[string]any
	_ = json.Unmarshal(before, &f)
	docs := f["instrument_docs"].(map[string]any)
	for _, raw := range docs {
		raw.(map[string]any)["fx"] = []any{map[string]any{"type": "fx.warp", "v": 1, "on": true, "values": map[string]any{}}}
		break
	}
	bad, _ := json.Marshal(f)
	err := g.Import(bad)
	if err == nil || !strings.Contains(err.Error(), "instrument_docs.") || !strings.Contains(err.Error(), "fx.warp") {
		t.Fatalf("want a path-named error, got %v", err)
	}
	if string(g.drum.exportBytes()) != string(before) {
		t.Fatal("a rejected file must leave the circuit untouched")
	}
}

func TestImportRejectsLegacySynthParams(t *testing.T) {
	g := newDocsGame(t, "kick")
	data := `{"schema_version":"0.0.1","bpm":120,"instruments":[{"name":"K","id":"kick","kind":"builtin","volume":1,"origin":0,"color":"#FFFFFFFF","synth_params":{"pitch":0}}],"nodes":[]}`
	if err := g.Import([]byte(data)); err == nil || !strings.Contains(err.Error(), "predates instrument documents") {
		t.Fatalf("want the legacy error, got %v", err)
	}
}
```

(`newDocsGame` comes from Task B6's `instrument_library_flow_test.go`.)

- [ ] **Step 2: Implement.** Undo diff: `diffInstrument` keeps its row diffs; the SynthParams part becomes `instrument.Diff(a.InstrumentDocs[id], b.InstrumentDocs[id])`, so `describeUndoChange` takes the whole files. Update its tests.

- [ ] **Step 3: Templates.** Run `make templates`. The diff must only remove `"synth_params":{"pitch":0}`; check it with `git diff --stat src/go/internal/assets/templates | tail -1` and a grep. Run `cmd/gen-showcase`'s drift test.

- [ ] **Step 4: Run.**
  - Fast: `-run 'Import|Export|Undo|Project' ./internal/ui/`.
  - Real: `cd src/go && xvfb-run -a ../../.tools/go/bin/go test -run 'TestExportImportMasterMix|TestEveryBuiltinTemplateRoundTrips|TestEveryBuiltinInstrumentMasterMix|TestSynthTabSaveDoesNotChangeTheMasterMix|TestHeadlineEditsAreAudible' -timeout 40m ./internal/ui/`.

  Adapt the headline harness (`export_import_master_audio_harness_test.go`): Save-As now writes library documents, and "clean machine" = `LoadUserInstrumentLibrary(empty)` + `ResetInstrumentDocs()`. **The renders must stay identical** — the documents travel in the file.

- [ ] **Step 5: Checkpoint.**

---

### Task B8: Instrument bundles v2

**Files:**
- Modify: `src/go/internal/audio/instrument_bundle.go:81-101`, `src/go/internal/ui/instrument_export.go:31-90`, `src/go/internal/ui/instrument_import.go:46-117`
- Test: `src/go/internal/ui/instrument_bundle_test.go` (adapt the existing ones)

**Interfaces:**
- `InstrumentDoc` (bundle entry) becomes `{Name, Kind, Color, Synth *instrument.Doc `json:"synth,omitempty"`, SampleEdit, WavRef, EffectChain, EQ}`. `Recipe` and the dead `SynthParams` are deleted.
- Build: `Synth = InstrumentDoc(instID)` for synth kinds.
- Apply:
  1. validate every `Synth` (Migrate + Validate) before anything registers;
  2. a shipped id or a collision resolves exactly as today (`resolveImportID`; a fresh id = `NewUserInstrumentID(name, doc)`);
  3. `SaveLibraryDoc(doc with the resolved id)`;
  4. `SetInstrumentDoc(newID, doc)`;
  5. register the voice;
  6. then FX / sample edit / WAV as today.
- The row assignment goes through `dv.SetInstrument`, fixing the direct `Rows[i].Instrument =` that skipped `onRowInstrumentChanged` (instrument_import.go). It is a one-line change; note it in progress.md.

- [ ] **Step 1: Adapt the existing bundle round-trip tests to the new field**, and add a test that a bundle naming `fx.warp` is rejected with nothing registered.
- [ ] **Step 2: Implement.** Step 3: run `-run 'Bundle|InstrumentImport|InstrumentExport'`. Step 4: Checkpoint.

---

### Task B9: Loudness lives in the documents

**Files:**
- Modify: `src/go/cmd/measure-loudness/` (`-write` updates `render.amplitude` in `internal/assets/instruments/<id>.json`, then rebuilds the pack); `src/go/internal/audio/loudness_amp.go`
- Delete: `instrument_loudness_gen.go` and `src/js/instrument_loudness.gen.js`, **if** their id set ⊆ the document ids (check first: `grep -o '"[a-z0-9.-]*":' src/go/internal/audio/instrument_loudness_gen.go | sort -u` vs `index.json`). Otherwise keep the table for the non-document ids only.
- Test: `loudness_drift_test.go` (now compares `measured` vs `render.amplitude`)

- [ ] **Step 1:** Make the drift test read documents. It fails only if a measured amplitude differs from the document's.
- [ ] **Step 2:** Change `-write`. Run `make measure-loudness`. The documents must be unchanged (`git diff --stat src/go/internal/assets/instruments` is empty), because A8 already wrote the effective amplitude.
- [ ] **Step 3:** Delete the tables, and the `audio.js` import if it is still present. Run the deploy manifest test.
- [ ] **Step 4: Checkpoint.**

---

### Task B10: PART B CHECKPOINT

- [ ] **Step 1: Rebuild the parity tool on documents.** In `render_parity_tool_test.go`, port `engineCaseRender` (:146) and `engineParityCases` (:167):
  - base = `EnsureInstrumentDoc(id)`;
  - a case = `instrument.SetField(doc, reg, field, v)` → `renderBlockForTest(Flatten, sr/4)`;
  - same case names `engine:<field>@min|max|zero`;
  - fields whose `SetField` returns an error produce no case.

  Port `renderParityCases`'s `<param>@min|max` cases to `RenderInstrumentPreviewWithOverrides` over `FieldValues` (preview_render.go:89; replace its `RecipeForInstrument` / merged lookup with the document's `FieldValues`).

- [ ] **Step 2: Dump and compare.**

```bash
cd src/go && BEATMO_RENDER_PARITY_DIR=$HOME/.beatmo/render-parity BEATMO_RENDER_PARITY_LABEL=docs ../../.tools/go/bin/go test -run TestRenderParityDump -timeout 30m ./internal/audio/
BEATMO_RENDER_PARITY_DIR=$HOME/.beatmo/render-parity BEATMO_RENDER_PARITY_A=Ffix2 BEATMO_RENDER_PARITY_B=docs \
  BEATMO_RENDER_PARITY_ALLOW='<ids from the A10 decision, as "<id> *">' \
  BEATMO_RENDER_PARITY_DROPPED='<fields made undeclared, e.g. gen1_kick_h3 for punchy, as globs>' \
  ../../.tools/go/bin/go test -run TestRenderParityCompare ./internal/audio/
```

Required:
- every default `.f32` bit-exact;
- engine cases bit-exact, except the A10-approved ids (their base block changed by decision) and lost cases of undeclared fields.

Record both lists in progress.md.

- [ ] **Step 3: Full suites.**
  - `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod -timeout 40m ./...`
  - `cd src/go && xvfb-run -a ../../.tools/go/bin/go test -timeout 40m ./internal/audio/... ./internal/ui/...`
  - `make test-parity`
  - the browser set from B5, plus `synth_save_export_import`, `xplat_browser_export_import`, `template_instruments_audible`, `audio_lane_e2e`

  Every failure gets root-caused (superpowers:systematic-debugging). No baselines are assumed pre-existing; prove causation.

- [ ] **Step 4: Screenshots.** Desktop + mobile: `synth_tab`, two family synth scenes, `theme_editor` (unrelated, as a smoke check), `synth_action_menu_restore_shipped`. Open each one.

- [ ] **Step 5:** Log "PART B GREEN" with the suite timings. Tell the user Part B is done and summarise the sound deltas (A10 list) and the knob-visibility deltas (B4).

---

# Part C — Delete the old layers; port the tools

### Task C1: Port every remaining consumer

**Files:** every non-test reference found by

```bash
cd src/go && grep -rln --include=*.go -E 'RecipeForInstrument|MergeRecipeDefaults|RecipeDefaultParams|BindInstrumentToRecipe|RegisterUserRecipeFromBase|RecipeShippedDefault|IsMelodicRecipe|pitchAwareRecipe|NewRecipe\(|RecipeRegistrations\(' . | grep -v _test.go
```

Known from the maps:

| Location | Notes |
|---|---|
| `cmd/refs/diagnose.go` | |
| `cmd/export_audio/main.go` | |
| `internal/synthmatch/{optimize,render_common,tunable}.go` | |
| `internal/synthdiag/ablate.go` | |
| `cmd/gen-fundamentals/measure.go` | |
| `cmd/gen-showcase` | `IsMelodicRecipe` → `audio.IsMelodicInstrument(id)` |
| `internal/audio/offline_song_render.go:9-40` | `SongRenderInstrument` gains `Doc *instrument.Doc`; apply via `SetInstrumentDoc` |
| `internal/audio/songrender/template.go:15-58` | Parse `instrument_docs` |
| `internal/audio/instrument_pitch.go:221` | |
| `internal/audio/modular_stage_essential.go:45` | Uses `MergedInstrumentParamsRO`; fine as is |
| `internal/audio/user_samples.go:191-192` | |
| `internal/ui/js_exports_synth_recipe.go` | See the next paragraph |

**`js_exports_synth_recipe.go`:**
- Delete the recipe-registry exports (`:196-206`, `:220`, `:297`, `:334`, `:344`, `:416-453`, `recipeParamDefToJS`).
- Keep `setInstrumentParam(s)`, `getInstrumentParams`, `resetInstrumentParams`, `saveActiveRecipe`, `resetActiveRecipe`.
- Add `getInstrumentDoc(id) → JSON`.
- Update `wasm_bridge_smoke.browser.test.js`'s catalogue and `js_exports_catalogue_drift_test.go`.

**Rules:**
- A tool that "renders an instrument with overrides" does `doc := EnsureInstrumentDoc(id)`, then `SetField` per override, then `Flatten` / `docVoiceBuffer`.
- `IsMelodicRecipe(RecipeForInstrument(id))` → `audio.IsMelodicInstrument(id)` (= `InstrumentDoc(id).Render.Melodic`, falling back to `ShippedDoc`).
- `synthmatch` / `synthdiag` optimise over flat param maps: give them `instrument.FieldValues` as input and `SetField` as the write. Do not redesign them.

- [ ] **Step 1:** Port file by file. After each package, run its tests on both paths. `cmd/gen-fundamentals`'s `TestInstrumentFundamentalsUpToDate` (real) must stay green, because the measurements read the same blocks.
- [ ] **Step 2:** Re-run the grep. Only definitions inside the files C2 deletes may remain.
- [ ] **Step 3: Checkpoint.**

### Task C2: Delete

- [ ] **Step 1: Delete** (spec §6 "Deleted").
  - **Go files and functions:**
    - `synth_registry.go`'s defaults / shipped maps and `MergeRecipeDefaults`
    - `recipe_overrides.go` whole (move `InstrumentDocModified` users already done)
    - `instrument_clone*.go`
    - `instrumentParamsMgr` + the overlay in `synth_recipe.go` (keep `ParamDef`, `RecipeParams`, `SetInstrumentParam*` / `GetInstrumentParams` / `ResetInstrumentParams` as the B4 adapters)
    - the binding (`BindInstrumentToRecipe`, `builtinInstrumentRecipeBindings`, `factoryRecipeForInstrument`, `RecipeForInstrument`, `synth_recipe_binding_hook.go`)
    - `BuildBuiltinRecipeDocs`, `RecipeDoc`, `RecipeRegistration` and the recipe registry (`RegisterRecipe`, `NewRecipe`, `RecipeRegistrations`, `synth_recipe_plugin.go`, `synth_registry_entries_*.go`)
    - `modular_defaults_seed.go`
    - `merged_params_cache.go`'s overlay half
    - `bakedModularRender`, `newModularBlock`, `modularIdentityBlock`, `CVariantInstrument` if unused (grep)
    - `instrument_seeds*.go`; `ModularInstrumentDef.Seed` and the whole `modularInstrumentDefs` table plus the `modularTable*` helpers (ids now come from `index.json`)
    - `melodic_recipes.go` (melodic is `render.melodic`)
    - `ModularParamSchemaIdentity` / `sharedModularParamSchemaIdentity`
    - the `Identity` column of `modularFields`
    - `cmd/gen-instrument-docs`
  - **Generators:**
    - `cmd/gen-synth-abi`: stop emitting `MODULAR_PARAM_IDENTITY`, then `make gen-synth-abi`
    - `gen_instrument_ids.go` (`//go:build ignore`): read `internal/assets/instruments/index.json` instead of scraping the table; `go generate ./internal/audio/`
  - **Tests that die with what they test:**
    - A8's classify/notes tests;
    - `instrument_docs_proof_test.go` (A9);
    - A10's audit;
    - B3's `TestDocVoiceEqualsBakedVoice`;
    - `seed_param_liveness_test.go`, superseded by `TestDeclaredModuleValuesAreLive`. **First move its `perturbFor` helper into `module_liveness_test.go`**, which uses it;
    - every `_test.go` that only exercised deleted API (`instrument_clone_test.go`, `recipe_overrides*_test.go`, …).

    `resetLibraryForTest` and `loadShippedDocsForTest` stay.
  - **Rule for tests:** for a test that exercised a deleted API to protect a *behaviour*, port the behaviour check to documents instead of deleting it. Each deletion vs port gets a line in progress.md.
- [ ] **Step 2:** `cd src/go && ../../.tools/go/bin/go build ./... && ../../.tools/go/bin/go vet -tags test -modfile=go.test.mod ./...` — fix to green.
- [ ] **Step 3: Checkpoint** — full fast suite.

### Task C3: Documentation

- [ ] **Step 1:** Update CLAUDE.md:
  - Quick Start: add `make asset-pack`; remove `gen-modular-instruments`.
  - "Critical Gotchas": rewrite "Every instrument is ONE config row" and "The param block is ONE generated table and the C engine has NO defaults" to describe documents, module types, `Flatten`, and the pack.
  - "Export → import audio identity": `projectFile.Recipes` → `instrument_docs`; delete the bullets about the defaults layer / cloned recipe base / `RegisterUserRecipeFromBase`.
  - "Portable single-instrument format": `Synth *instrument.Doc`.

  Update `AGENTS.synth.md` § Recipe Registry the same way. Keep each edit scoped to the lines that describe deleted mechanics.
- [ ] **Step 2:** Update the memory file `project_config_only_synth_engine.md` with what shipped, and add a pointer line for this sub-project in MEMORY.md (≤ 200 chars).
- [ ] **Step 3: Checkpoint.**

### Task C4: PART C CHECKPOINT

- [ ] Re-run B10 Steps 2–4 (parity vs Ffix2 with the same allow / dropped lists, full suites, browser set, screenshots). Log "PART C GREEN".

---

# Part D — Templates pin documents

### Task D1: Template rows reference pack documents by hash

**Files:**
- Modify: `src/go/cmd/gen-showcase/main.go` (the template `doc` gets `InstrumentDocs map[string]docRef`), `src/go/internal/assets/packbuild/packbuild.go` (pins), `src/go/internal/ui/project_file.go` + `import.go` (resolve refs)
- Create: `src/go/internal/assets/instrument_pins/` (normally empty; a template-pinned document that is no longer shipped lands here as `<hash>.json`)
- Test: `src/go/cmd/gen-showcase/drift_test.go` (extend), `src/go/internal/ui/template_docs_test.go`

**Interfaces:**
- A project `instrument_docs` value is either a full document or `{"ref": "sha256:…"}`. Only embedded templates use refs; export never writes one.
  - Go: `projectFile.InstrumentDocs map[string]projectDoc`, with `projectDoc` = `instrument.Doc` + `Ref string `json:"ref,omitempty"``. A custom `UnmarshalJSON` decodes `{"ref":…}` alone.
- Import resolves a ref with `audio.PackDoc(ref)`. An unknown ref is a hard error naming the template and the ref.
- `gen-showcase` writes, for every row id, `{"ref": ContentHash(EnsureInstrumentDoc(baseID))}` with `ID` = row id. Instance ids map to their base's hash.
- `packbuild.Build` also loads every `instrument_pins/*.json` into `docs`. Its drift test fails when a template ref resolves to neither a shipped nor a pinned doc.

- [ ] **Step 1: Failing tests**

```go
package ui

import (
	"encoding/json"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

func TestEveryTemplateRefResolves(t *testing.T) {
	for _, tp := range assets.Templates() {
		var f projectFile
		if err := json.Unmarshal(tp.Bytes, &f); err != nil {
			t.Fatalf("%s: %v", tp.Display, err)
		}
		if len(f.InstrumentDocs) == 0 {
			t.Fatalf("%s: a template must pin its instruments", tp.Display)
		}
		for id, pd := range f.InstrumentDocs {
			if pd.Ref == "" {
				t.Fatalf("%s/%s: templates reference the pack", tp.Display, id)
			}
			if _, ok := audio.PackDoc(pd.Ref); !ok {
				t.Fatalf("%s/%s: ref %s not in the pack", tp.Display, id, pd.Ref)
			}
		}
	}
}

func TestTemplateImportExpandsToFullCopies(t *testing.T) {
	g := newDocsGame(t)
	tp := assets.Templates()[0]
	if err := g.Import(tp.Bytes); err != nil {
		t.Fatal(err)
	}
	for _, r := range g.drum.Rows {
		d, ok := audio.InstrumentDoc(r.Instrument)
		if !ok {
			continue // sample rows
		}
		if err := instrument.Validate(d, audio.BuiltinModuleRegistry()); err != nil || d.ID != r.Instrument {
			t.Fatalf("%s: %v", r.Instrument, err)
		}
	}
}
```

- [ ] **Step 2: Implement**, then `make templates && make asset-pack`.
- [ ] **Step 3: Run** the two tests, the gen-showcase drift test, `TestAssetPackIsCurrent`, and (real) `TestEveryBuiltinTemplateRoundTripsByteIdentical`. The master mixes must be identical before and after D. Also `make wasm` + `TestWasmAssetSizeBudget`; record the raw/gzip delta.
- [ ] **Step 4: Checkpoint.**

### Task D2: FINAL CHECKPOINT

- [ ] Re-run B10 Steps 2–4.
- [ ] Re-run `make test-real` (`cd src/go && xvfb-run -a ../../.tools/go/bin/go test -timeout 60m ./...`).
- [ ] Log "SUB-PROJECT 1 GREEN" with the final sound-delta list (A10 decision) and knob-visibility list (B4).
- [ ] Tell the user it is ready for their commit, with that summary. Remind them that sub-project 2 starts by registering its first migration `Step`.

---

## Self-review notes (for the executor)

- **Spec coverage:** §3 → A1/A8; §4 → A2/A7; §5 → A3–A6; §6 → B2/B4/B6; §7 → A10/B3/B8; §8 → B3/B5; files → B1/B7/B8/D1; §9 → A6 (runner); §10 → A9/B10/C4/D2; §11 respected (no C edits, no UI regrouping).
- **Naming is fixed across tasks:** `InstrumentDoc`, `SetInstrumentDoc`, `EnsureInstrumentDoc`, `ResetInstrumentDocs`, `InstrumentDocRev`, `LibraryDoc`, `LibraryIDs`, `UserLibraryDoc`, `SaveLibraryDoc`, `DeleteUserLibraryDoc`, `ShippedDoc`, `PackDoc`, `ShippedInstrumentIDs`, `BuiltinModuleRegistry`, `InstrumentParamDefs`, `InstrumentDeclaresField`, `InstrumentDocModified`, `docVoiceBuffer`, `docInstrument`, `platformInstrumentDocChanged`, `PushAllInstrumentDocsToPlatform`, `NewUserInstrumentID`, `IsMelodicInstrument`.
