# Offline Template Renderer Implementation Plan (Plan 2 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render a Beatmo template circuit OFFLINE and deterministically to a master WAV + per-instrument stems, by walking the template's node graph through the engine predictor to enumerate note events, rendering each note via the synth recipe path, and (production build) mixing through the real audio engine for template-faithful output (volume/pan/sends/insert-FX/master chain).

**Architecture:** A new `internal/audio/songrender` package owns the **headless arrangement** (template JSON → `model.Graph` → `engine.Predictor` → `[]NoteEvent`) — pure Go, no CGo, fully testable under `-tags test`. Rendering uses a **build-tag split** mirroring `internal/audio/synthmatch`: `render_stub.go` (`//go:build test`) is a deterministic pure-Go renderer (synth stub + per-instrument volume, master = exact Σ stems) so orchestration is fast-path testable; `render_prod.go` (`//go:build !test`) calls one **new exported facade** `audio.RenderSongOffline(...)` (in `package audio`, `!test && !js`) that drives the real offline mixer + send-reverb + master chain for full fidelity. This is Plan 2 of 3; Plan 1 (`fingerprint` song-level metrics) is done; Plan 3 (`songmatch` compare + tuning) consumes this.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), `core/model` (Graph), `core/engine` (Predictor), `internal/audio` (recipe render + offline mixer + send FX), `internal/assets` (embedded templates), `internal/audio/synthmatch` (the render_prod/stub pattern to mirror), `internal/wave`.

## Global Constraints

- **Use the bundled Go:** `../../.tools/go/bin/go` from `src/go`.
- **Fast test path:** `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod <pkgs>`.
- **Determinism:** no `time.Now`, no unseeded RNG anywhere. Identical (template, config) → byte-identical arrangement + (stub) audio.
- **Build-tag discipline:** the `songrender` library carries NO build tag on its pure files (arrangement, types) so they compile under `-tags test`. Only `render_prod.go` (`//go:build !test`) and `render_stub.go` (`//go:build test`) are split. The new `package audio` facade file is `//go:build !test && !js` (CGo).
- **Global-state hazard (CRITICAL):** `package audio` render functions mutate PROCESS-GLOBAL state — instrument params (`SetInstrumentParams`), channel configs (`SetChannelVolume/Pan`, `SetInsertEffects`, `SetReverbSend`), and the global `sendFX` reverb tail. Every render entrypoint MUST reset first: `audio.Reset()` + `audio.ResetInstruments()` before applying a template's instruments, and the prod facade MUST call `resetSendEffectsState()` before each mixer run (the send-reverb tail leaks between renders otherwise — see `template_audio_quality_test.go:331,354`). Tests that render must `t.Cleanup` a reset.
- **Pitch convention:** node `Pitch` is semitones from A3=220 Hz; `freq = 220·2^(pitch/12)`. Passed to the recipe as the `"pitch"` merged param.
- **Do NOT `git add`/`git commit`** — standing repo rule ([[feedback_never_stage_or_commit]]). Treat every "Commit" step as **"Checkpoint: stop, summarize, let the user review the diff. Do NOT stage."**
- **No stray build artifacts:** any `go build ./cmd/...` drops a binary in `src/go/`; `rm -f` it after building.

## Verified APIs (from the codebase spike — use these exact signatures)

- Templates (embedded): `assets.Templates() []assets.Template`; `Template{Genre, Display string; BPM int; Bytes []byte}` (`internal/assets/templates.go`). No by-stem accessor — iterate and match `.Genre`.
- Graph: `model.NewGraph(logger *game_log.Logger) *model.Graph`; `(*Graph).AddNode(i, j int, t model.NodeType) model.NodeID`; `(*Graph).SetNodeParams(id model.NodeID, p model.NodeParams)`; `Graph.Edges map[[2]model.NodeID]struct{}`; `Graph.StartNodeID model.NodeID`; `(*Graph).CalculateBeatRowFrom(start model.NodeID) ([]model.BeatInfo, bool, int)`. `model.NodeTypeRegular`/`Invisible`/`Silent`/`Mute`. `model.BeatInfo{NodeID model.NodeID; NodeType model.NodeType; I, J int}`. `model.NodeParams{Volume, Pitch, Duration float64; LogicKind string; LogicN int; LogicP float64; ...}`.
- Predictor: `engine.NewPredictor(g *model.Graph, logger *game_log.Logger) *engine.Predictor`; `(*Predictor).SetPaths(paths [][]model.BeatInfo, isLoop []bool, loopStart []int, nodes map[model.NodeID]model.Node)`; `(*Predictor).Ensure(horizon int)`; `(*Predictor).AudibleAt(row, idx int) bool`.
- Recipe render (mirror `synthmatch`): `synthmatch.RenderWithParams(instID string, pitch float64, sr int, durSec float64, overrides map[string]float64) (wave.Wave, error)` (build-tag split already exists). `audio.ConfigForInstrument(id).DurationSec`. `audio.BindInstrumentToRecipe(instID, recipeID string)`, `audio.SetInstrumentParams(instID string, p audio.RecipeParams)`.
- Offline mixer + facade building blocks (all `package audio`, `!test && !js`, UNEXPORTED — reachable only from inside `package audio`): `newTestMixer()`, `renderMixer(m, nSamples) []float64`, `resetSendEffectsState()`, `newRecipeAwareVoicePitched(id string, bpm, sr int, pitch float64)`, `scaledVoice{v, gain}`, `(*mixer).Schedule(instID string, v voice, delaySamples int)`, channel config setters `SetChannelVolume/SetChannelPan/SetInsertEffects/SetReverbSend`. WAV out: `audio.ExportCaptureToWAV(samples []float64, filename string) error`.
- Template JSON shape (mirror as a lean struct; from `internal/ui/export.go`): top-level `{version, subdiv, bpm, instruments[], nodes[]}`; instrument `{name, id, kind, volume, origin (start node id), color, pan, reverb_send, delay_send, recipe, synth_params, effects}`; node `{id, i, j, type, inputs[], outputs[], volume, pitch, duration, logic_kind, logic_n, logic_p}`.

---

### Task 1: Template parse model (headless JSON → structs)

**Files:**
- Create: `src/go/internal/audio/songrender/template.go`
- Test: `src/go/internal/audio/songrender/template_test.go`

**Interfaces:**
- Produces:
  - `type InstrumentSpec struct { Name, ID, Kind string; Volume, Pan, ReverbSend, DelaySend float64; Origin int; Recipe string; SynthParams map[string]float64; Effects []audio.EffectSlot }`
  - `type NodeSpec struct { ID, I, J int; Type string; Inputs, Outputs []int; Volume, Pitch, Duration float64; LogicKind string; LogicN int; LogicP float64 }`
  - `type ParsedTemplate struct { BPM, Subdiv int; Instruments []InstrumentSpec; Nodes []NodeSpec }`
  - `func ParseTemplate(jsonBytes []byte) (ParsedTemplate, error)`
  - `func LoadTemplate(stem string) (ParsedTemplate, error)` — finds the embedded template by `assets.Templates()` `.Genre == stem`, parses `.Bytes`.

- [ ] **Step 1: Write the failing test**

```go
package songrender

import "testing"

func TestLoadTemplate_BachToccata(t *testing.T) {
	pt, err := LoadTemplate("bach-toccata")
	if err != nil {
		t.Fatal(err)
	}
	if pt.BPM <= 0 || pt.Subdiv <= 0 {
		t.Errorf("bad header: bpm=%d subdiv=%d", pt.BPM, pt.Subdiv)
	}
	if len(pt.Instruments) == 0 || len(pt.Nodes) == 0 {
		t.Fatalf("empty: %d instruments, %d nodes", len(pt.Instruments), len(pt.Nodes))
	}
	// Organ is the Bach toccata instrument.
	found := false
	for _, in := range pt.Instruments {
		if in.ID == "organ" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an organ instrument in bach-toccata")
	}
}

func TestLoadTemplate_UnknownStem(t *testing.T) {
	if _, err := LoadTemplate("does-not-exist"); err == nil {
		t.Error("expected error for unknown stem")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/songrender/ -run LoadTemplate` → FAIL (package/undefined).

- [ ] **Step 3: Implement `template.go`**

```go
// Package songrender renders a Beatmo template circuit offline to a master
// waveform + per-instrument stems. The arrangement (template -> note events via
// the engine predictor) is pure Go and testable; rendering is build-tag split
// (render_stub.go for tests, render_prod.go for the faithful audio-engine path).
package songrender

import (
	"encoding/json"
	"fmt"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

type InstrumentSpec struct {
	Name        string             `json:"name"`
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Volume      float64            `json:"volume"`
	Pan         float64            `json:"pan,omitempty"`
	ReverbSend  float64            `json:"reverb_send,omitempty"`
	DelaySend   float64            `json:"delay_send,omitempty"`
	Origin      int                `json:"origin"`
	Recipe      string             `json:"recipe,omitempty"`
	SynthParams map[string]float64 `json:"synth_params,omitempty"`
	Effects     []audio.EffectSlot `json:"effects,omitempty"`
}

type NodeSpec struct {
	ID        int     `json:"id"`
	I         int     `json:"i"`
	J         int     `json:"j"`
	Type      string  `json:"type"`
	Inputs    []int   `json:"inputs,omitempty"`
	Outputs   []int   `json:"outputs,omitempty"`
	Volume    float64 `json:"volume,omitempty"`
	Pitch     float64 `json:"pitch,omitempty"`
	Duration  float64 `json:"duration,omitempty"`
	LogicKind string  `json:"logic_kind,omitempty"`
	LogicN    int     `json:"logic_n,omitempty"`
	LogicP    float64 `json:"logic_p,omitempty"`
}

type ParsedTemplate struct {
	BPM         int              `json:"bpm"`
	Subdiv      int              `json:"subdiv"`
	Instruments []InstrumentSpec `json:"instruments"`
	Nodes       []NodeSpec       `json:"nodes"`
}

// ParseTemplate unmarshals a v1 template JSON into a ParsedTemplate.
func ParseTemplate(b []byte) (ParsedTemplate, error) {
	var pt ParsedTemplate
	if err := json.Unmarshal(b, &pt); err != nil {
		return ParsedTemplate{}, fmt.Errorf("songrender: parse template: %w", err)
	}
	if pt.Subdiv <= 0 {
		pt.Subdiv = 16 // schema default
	}
	return pt, nil
}

// LoadTemplate finds the embedded built-in template whose stem == genre and parses it.
func LoadTemplate(stem string) (ParsedTemplate, error) {
	for _, t := range assets.Templates() {
		if t.Genre == stem {
			return ParseTemplate(t.Bytes)
		}
	}
	return ParsedTemplate{}, fmt.Errorf("songrender: no template with stem %q", stem)
}
```

- [ ] **Step 4: Run to verify pass** — `… -run LoadTemplate` → PASS. (If `audio.EffectSlot`'s JSON shape rejects the template's `effects`, log the error from the test and adjust the struct to match `audio.EffectSlot`'s actual fields — confirm against `internal/audio` `EffectSlot`.)

- [ ] **Step 5: Checkpoint**

---

### Task 2: Build `model.Graph` + per-row paths from a ParsedTemplate

**Files:**
- Create: `src/go/internal/audio/songrender/graph.go`
- Test: `src/go/internal/audio/songrender/graph_test.go`

**Interfaces:**
- Consumes: `ParsedTemplate` (Task 1); `model` Graph API.
- Produces:
  - `type RowPaths struct { Graph *model.Graph; Paths [][]model.BeatInfo; IsLoop []bool; LoopStart []int; Nodes map[model.NodeID]model.Node; RowInst []string }`
  - `func BuildGraph(pt ParsedTemplate) (RowPaths, error)` — adds every node (mapping JSON node id → `model.NodeID`), wires edges from `Outputs`, sets `NodeParams`, then for each instrument computes the path from its `Origin` node via `CalculateBeatRowFrom`. `RowInst[i]` is `Instruments[i].ID`.

- [ ] **Step 1: Write the failing test**

```go
package songrender

import "testing"

func TestBuildGraph_BachToccata(t *testing.T) {
	pt, err := LoadTemplate("bach-toccata")
	if err != nil {
		t.Fatal(err)
	}
	rp, err := BuildGraph(pt)
	if err != nil {
		t.Fatal(err)
	}
	if len(rp.Paths) != len(pt.Instruments) {
		t.Fatalf("got %d row paths, want %d (one per instrument)", len(rp.Paths), len(pt.Instruments))
	}
	if len(rp.Nodes) == 0 {
		t.Error("no nodes in graph snapshot")
	}
	// At least one instrument row must have a non-empty path.
	nonEmpty := 0
	for _, p := range rp.Paths {
		if len(p) > 0 {
			nonEmpty++
		}
	}
	if nonEmpty == 0 {
		t.Error("all row paths empty — graph wiring or origin lookup is broken")
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run BuildGraph` → FAIL.

- [ ] **Step 3: Implement `graph.go`**

```go
package songrender

import (
	"fmt"
	"io"

	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

type RowPaths struct {
	Graph     *model.Graph
	Paths     [][]model.BeatInfo
	IsLoop    []bool
	LoopStart []int
	Nodes     map[model.NodeID]model.Node
	RowInst   []string
}

func nodeType(s string) model.NodeType {
	switch s {
	case "invisible":
		return model.NodeTypeInvisible
	case "silent":
		return model.NodeTypeSilent
	case "mute":
		return model.NodeTypeMute
	default:
		return model.NodeTypeRegular
	}
}

// BuildGraph constructs a model.Graph from the parsed template and computes one
// traversal path per instrument (from that instrument's Origin node).
func BuildGraph(pt ParsedTemplate) (RowPaths, error) {
	g := model.NewGraph(game_log.New(io.Discard, game_log.LevelError))
	idMap := make(map[int]model.NodeID, len(pt.Nodes))
	for _, n := range pt.Nodes {
		nid := g.AddNode(n.I, n.J, nodeType(n.Type))
		g.SetNodeParams(nid, model.NodeParams{
			Volume: n.Volume, Pitch: n.Pitch, Duration: n.Duration,
			LogicKind: n.LogicKind, LogicN: n.LogicN, LogicP: n.LogicP,
		})
		idMap[n.ID] = nid
	}
	// Wire edges from each node's Outputs.
	for _, n := range pt.Nodes {
		from, ok := idMap[n.ID]
		if !ok {
			continue
		}
		for _, out := range n.Outputs {
			to, ok := idMap[out]
			if ok {
				g.Edges[[2]model.NodeID{from, to}] = struct{}{}
			}
		}
	}

	rp := RowPaths{Graph: g, Nodes: g.Nodes}
	for _, inst := range pt.Instruments {
		origin, ok := idMap[inst.Origin]
		if !ok {
			// No origin node → empty path for this row (still keep the slot).
			rp.Paths = append(rp.Paths, nil)
			rp.IsLoop = append(rp.IsLoop, false)
			rp.LoopStart = append(rp.LoopStart, 0)
			rp.RowInst = append(rp.RowInst, inst.ID)
			continue
		}
		path, isLoop, loopStart := g.CalculateBeatRowFrom(origin)
		rp.Paths = append(rp.Paths, path)
		rp.IsLoop = append(rp.IsLoop, isLoop)
		rp.LoopStart = append(rp.LoopStart, loopStart)
		rp.RowInst = append(rp.RowInst, inst.ID)
	}
	if len(rp.Paths) == 0 {
		return RowPaths{}, fmt.Errorf("songrender: no instrument rows")
	}
	return rp, nil
}
```

- [ ] **Step 4: Run to verify pass** — `… -run BuildGraph` → PASS. (Confirm `game_log.New` signature + `LevelError` const against `core/log`; if `New` needs a non-nil `io.Writer`, pass `io.Discard`.)

- [ ] **Step 5: Checkpoint**

---

### Task 3: Note-event enumeration via the predictor

**Files:**
- Create: `src/go/internal/audio/songrender/arrangement.go`
- Test: `src/go/internal/audio/songrender/arrangement_test.go`

**Interfaces:**
- Consumes: `RowPaths` (Task 2); `engine` Predictor.
- Produces:
  - `type NoteEvent struct { Row int; InstID string; AbsStep int; AtSample int; PitchSemis, Volume, DurSec float64 }`
  - `type Arrangement struct { BPM, Subdiv, Bars, SampleRate int; Instruments []InstrumentSpec; Notes []NoteEvent; TotalSamples int }`
  - `func BuildArrangement(pt ParsedTemplate, bars, sampleRate int) (Arrangement, error)` — builds the graph, runs the predictor over `bars*Subdiv` steps, and emits one `NoteEvent` per audible node. `AtSample = AbsStep * samplesPerStep`, where `samplesPerStep = round(sampleRate * 60 / BPM / Subdiv * beatsPerStep)`. (One step = one subdivision; a beat = `Subdiv/4` steps in 4/4, so seconds-per-step = `60/BPM * 4/Subdiv`.)

- [ ] **Step 1: Write the failing test**

```go
package songrender

import "testing"

func TestBuildArrangement_BachToccataHasNotes(t *testing.T) {
	pt, err := LoadTemplate("bach-toccata")
	if err != nil {
		t.Fatal(err)
	}
	arr, err := BuildArrangement(pt, 4, 44100)
	if err != nil {
		t.Fatal(err)
	}
	if len(arr.Notes) == 0 {
		t.Fatal("no notes emitted from bach-toccata over 4 bars")
	}
	// Every note references a real instrument id and lands within the timeline.
	valid := map[string]bool{}
	for _, in := range pt.Instruments {
		valid[in.ID] = true
	}
	for _, n := range arr.Notes {
		if !valid[n.InstID] {
			t.Errorf("note with unknown instrument %q", n.InstID)
		}
		if n.AtSample < 0 || n.AtSample >= arr.TotalSamples {
			t.Errorf("note at sample %d outside [0,%d)", n.AtSample, arr.TotalSamples)
		}
	}
}

func TestBuildArrangement_Deterministic(t *testing.T) {
	pt, _ := LoadTemplate("bach-toccata")
	a, _ := BuildArrangement(pt, 2, 44100)
	b, _ := BuildArrangement(pt, 2, 44100)
	if len(a.Notes) != len(b.Notes) {
		t.Fatalf("non-deterministic note count: %d vs %d", len(a.Notes), len(b.Notes))
	}
	for i := range a.Notes {
		if a.Notes[i] != b.Notes[i] {
			t.Fatalf("note %d differs between runs", i)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run BuildArrangement` → FAIL.

- [ ] **Step 3: Implement `arrangement.go`**

```go
package songrender

import (
	"fmt"
	"io"
	"math"

	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

type NoteEvent struct {
	Row        int
	InstID     string
	AbsStep    int
	AtSample   int
	PitchSemis float64
	Volume     float64
	DurSec     float64
}

type Arrangement struct {
	BPM, Subdiv, Bars, SampleRate int
	Instruments                   []InstrumentSpec
	Notes                         []NoteEvent
	TotalSamples                  int
}

// beatInfoAt returns the BeatInfo at absolute step abs for a row path, honoring
// the row's loop (mirrors the predictor's own index folding).
func beatInfoAt(path []model.BeatInfo, isLoop bool, loopStart, abs int) (model.BeatInfo, bool) {
	if len(path) == 0 {
		return model.BeatInfo{}, false
	}
	if abs < len(path) {
		return path[abs], true
	}
	if !isLoop {
		return model.BeatInfo{}, false
	}
	loopLen := len(path) - loopStart
	if loopLen <= 0 {
		return model.BeatInfo{}, false
	}
	idx := loopStart + ((abs - loopStart) % loopLen)
	return path[idx], true
}

func BuildArrangement(pt ParsedTemplate, bars, sampleRate int) (Arrangement, error) {
	rp, err := BuildGraph(pt)
	if err != nil {
		return Arrangement{}, err
	}
	totalSteps := bars * pt.Subdiv
	if totalSteps <= 0 {
		return Arrangement{}, fmt.Errorf("songrender: non-positive totalSteps")
	}

	pred := engine.NewPredictor(rp.Graph, game_log.New(io.Discard, game_log.LevelError))
	pred.SetPaths(rp.Paths, rp.IsLoop, rp.LoopStart, rp.Nodes)
	pred.Ensure(totalSteps)

	// seconds per step = 60/BPM * 4/Subdiv (a beat = Subdiv/4 steps in 4/4).
	secPerStep := 60.0 / float64(pt.BPM) * 4.0 / float64(pt.Subdiv)
	samplesPerStep := secPerStep * float64(sampleRate)

	arr := Arrangement{
		BPM: pt.BPM, Subdiv: pt.Subdiv, Bars: bars, SampleRate: sampleRate,
		Instruments:  pt.Instruments,
		TotalSamples: int(math.Round(float64(totalSteps) * samplesPerStep)),
	}
	for row := 0; row < len(rp.Paths); row++ {
		inst := pt.Instruments[row]
		for abs := 0; abs < totalSteps; abs++ {
			if !pred.AudibleAt(row, abs) {
				continue
			}
			bi, ok := beatInfoAt(rp.Paths[row], rp.IsLoop[row], rp.LoopStart[row], abs)
			if !ok {
				continue
			}
			node := rp.Nodes[bi.NodeID]
			dur := node.Params.Duration
			if dur <= 0 {
				dur = 1.0
			}
			dur *= audio.ConfigForInstrument(inst.ID).DurationSec
			vol := node.Params.Volume
			if vol <= 0 {
				vol = 1.0
			}
			vol *= inst.Volume
			arr.Notes = append(arr.Notes, NoteEvent{
				Row: row, InstID: inst.ID, AbsStep: abs,
				AtSample:   int(math.Round(float64(abs) * samplesPerStep)),
				PitchSemis: node.Params.Pitch,
				Volume:     vol,
				DurSec:     dur,
			})
		}
	}
	return arr, nil
}
```

- [ ] **Step 4: Run to verify pass** — `… -run BuildArrangement` → PASS. (If `AudibleAt`/`SetPaths`/`Ensure` signatures differ, fix to the real ones; if no notes emerge, verify the predictor needs the instrument recipe bound first — it does NOT for `AudibleAt`, which is purely structural.)

- [ ] **Step 5: Checkpoint**

---

### Task 4: Render types + stub renderer (`//go:build test`)

**Files:**
- Create: `src/go/internal/audio/songrender/render.go` (shared types, NO build tag)
- Create: `src/go/internal/audio/songrender/render_stub.go` (`//go:build test`)
- Test: `src/go/internal/audio/songrender/render_stub_test.go` (`//go:build test`)

**Interfaces:**
- Produces (in `render.go`, no tag): `type Rendered struct { Arrangement Arrangement; Master []float64; Stems map[string][]float64 }`. Doc that `Render(stem string, bars, sampleRate int) (Rendered, error)` is implemented per build tag.
- Produces (in `render_stub.go`): `func Render(stem string, bars, sampleRate int) (Rendered, error)` — deterministic: per note, `synthmatch.RenderWithParams(instID, pitch, sr, durSec, nil)` (the test stub synth), scaled by `note.Volume`, summed into that instrument's stem at `note.AtSample`; `Master` = exact element-wise Σ stems. No pan/sends/master chain in the stub (those are prod-only fidelity).

- [ ] **Step 1: Write the failing test** (`render_stub_test.go`, `//go:build test`)

```go
//go:build test

package songrender

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestRenderStub_MasterEqualsSumOfStems(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	r, err := Render("bach-toccata", 2, 44100)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Master) == 0 || len(r.Stems) == 0 {
		t.Fatalf("empty render: master=%d stems=%d", len(r.Master), len(r.Stems))
	}
	// Master must equal the exact element-wise sum of all stems (stub contract).
	sum := make([]float64, len(r.Master))
	for _, s := range r.Stems {
		for i := range s {
			if i < len(sum) {
				sum[i] += s[i]
			}
		}
	}
	for i := range r.Master {
		if d := r.Master[i] - sum[i]; d > 1e-9 || d < -1e-9 {
			t.Fatalf("master != sum of stems at %d: %v vs %v", i, r.Master[i], sum[i])
		}
	}
	// Non-trivial: at least one stem has signal.
	peak := 0.0
	for _, s := range r.Stems {
		for _, v := range s {
			if v > peak {
				peak = v
			} else if -v > peak {
				peak = -v
			}
		}
	}
	if peak < 1e-6 {
		t.Errorf("all stems silent (peak=%g) — stub synth not wired", peak)
	}
}

func TestRenderStub_Deterministic(t *testing.T) {
	audio.Reset()
	audio.ResetInstruments()
	a, _ := Render("bach-toccata", 1, 44100)
	b, _ := Render("bach-toccata", 1, 44100)
	if len(a.Master) != len(b.Master) {
		t.Fatalf("len differ %d vs %d", len(a.Master), len(b.Master))
	}
	for i := range a.Master {
		if a.Master[i] != b.Master[i] {
			t.Fatalf("non-deterministic master at %d", i)
		}
	}
}
```

- [ ] **Step 2: Run to verify it fails** — `… -run RenderStub` → FAIL.

- [ ] **Step 3: Implement `render.go` (types) + `render_stub.go`**

`render.go`:
```go
package songrender

// Rendered is the offline render of a template: the full arrangement, the master
// mix, and per-instrument stems (keyed by instrument id). Sample slices are mono
// float64 at Arrangement.SampleRate. Render is build-tag split:
//   - render_stub.go  (//go:build test):  deterministic pure-Go synth, master == Σ stems.
//   - render_prod.go  (//go:build !test): faithful audio-engine mix (vol/pan/sends/master).
type Rendered struct {
	Arrangement Arrangement
	Master      []float64
	Stems       map[string][]float64
}
```

`render_stub.go`:
```go
//go:build test

package songrender

import (
	"github.com/ingyamilmolinar/beatmo/internal/audio/synthmatch"
)

// Render (test build): deterministic. Each note is rendered by the synthmatch
// stub synth, scaled by note.Volume, summed into its instrument's stem at the
// note's sample offset. Master is the exact element-wise sum of all stems.
func Render(stem string, bars, sampleRate int) (Rendered, error) {
	pt, err := LoadTemplate(stem)
	if err != nil {
		return Rendered{}, err
	}
	arr, err := BuildArrangement(pt, bars, sampleRate)
	if err != nil {
		return Rendered{}, err
	}
	stems := make(map[string][]float64)
	for _, in := range arr.Instruments {
		if _, ok := stems[in.ID]; !ok {
			stems[in.ID] = make([]float64, arr.TotalSamples)
		}
	}
	for _, n := range arr.Notes {
		w, err := synthmatch.RenderWithParams(n.InstID, n.PitchSemis, sampleRate, n.DurSec, nil)
		if err != nil {
			continue // stub: skip un-renderable instruments rather than fail the whole song
		}
		dst := stems[n.InstID]
		for i, s := range w.Samples {
			j := n.AtSample + i
			if j >= len(dst) {
				break
			}
			dst[j] += s * n.Volume
		}
	}
	master := make([]float64, arr.TotalSamples)
	for _, s := range stems {
		for i, v := range s {
			master[i] += v
		}
	}
	return Rendered{Arrangement: arr, Master: master, Stems: stems}, nil
}
```

- [ ] **Step 4: Run to verify pass** — `… -run RenderStub` → PASS, then whole package `… ./internal/audio/songrender/` → PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 5: Exported faithful render facade in `package audio`

**Files:**
- Create: `src/go/internal/audio/offline_song_render.go` (`//go:build !test && !js`)
- Test: build smoke only (CGo; not fast-path testable — like the rest of the offline mixer).

**Interfaces:**
- Produces (exported from `package audio`):
  - `type SongRenderInstrument struct { ID, Recipe string; SynthParams map[string]float64; Volume, Pan, ReverbSend, DelaySend float64; Effects []EffectSlot }`
  - `type SongRenderNote struct { InstID string; AtSample int; Pitch, Gain float64; DurSec float64 }`
  - `func RenderSongOffline(insts []SongRenderInstrument, notes []SongRenderNote, bpm, sampleRate, totalSamples int) (master []float64, stems map[string][]float64, err error)` — applies each instrument's config to the global engine, then renders the master (all notes) and one stem per instrument (only that instrument's notes), each via the real offline mixer + master chain. Calls `resetSendEffectsState()` before every mixer run.

- [ ] **Step 1: Implement `offline_song_render.go`** (in `package audio`, reaching the unexported helpers)

```go
//go:build !test && !js

package audio

// SongRenderInstrument / SongRenderNote are the offline-render request types for
// RenderSongOffline (used by internal/audio/songrender's production path). They
// live in package audio because faithful rendering needs the unexported offline
// mixer + send-effects + recipe-aware voice helpers.
type SongRenderInstrument struct {
	ID, Recipe  string
	SynthParams map[string]float64
	Volume, Pan float64
	ReverbSend  float64
	DelaySend   float64
	Effects     []EffectSlot
}

type SongRenderNote struct {
	InstID   string
	AtSample int
	Pitch    float64
	Gain     float64
	DurSec   float64
}

// applySongInstruments configures the global engine for one render pass.
func applySongInstruments(insts []SongRenderInstrument) {
	for _, in := range insts {
		if in.Recipe != "" {
			BindInstrumentToRecipe(in.ID, in.Recipe)
		}
		if len(in.SynthParams) > 0 {
			SetInstrumentParams(in.ID, RecipeParams(in.SynthParams))
		}
		SetChannelVolume(in.ID, in.Volume)
		SetChannelPan(in.ID, in.Pan)
		SetReverbSend(in.ID, in.ReverbSend)
		SetDelaySend(in.ID, in.DelaySend)
		SetInsertEffects(in.ID, in.Effects)
	}
}

// renderOnePass schedules the given notes into a fresh offline mixer and renders
// totalSamples through the full master chain. keep filters which instruments are
// audible this pass (nil → all). resetSendEffectsState clears the reverb tail.
func renderOnePass(notes []SongRenderNote, bpm, sampleRate, totalSamples int, keep string) []float64 {
	resetSendEffectsState()
	m := newTestMixer()
	for _, n := range notes {
		if keep != "" && n.InstID != keep {
			continue
		}
		v := newRecipeAwareVoicePitched(n.InstID, bpm, sampleRate, n.Pitch)
		if v == nil {
			continue
		}
		// scaledVoice satisfies Voice via a POINTER receiver — must pass &scaledVoice.
		m.Schedule(n.InstID, &scaledVoice{v: v, gain: n.Gain}, n.AtSample)
	}
	return renderMixer(m, totalSamples)
}

// RenderSongOffline renders the master (all notes) and per-instrument stems.
func RenderSongOffline(insts []SongRenderInstrument, notes []SongRenderNote, bpm, sampleRate, totalSamples int) ([]float64, map[string][]float64, error) {
	applySongInstruments(insts)
	master := renderOnePass(notes, bpm, sampleRate, totalSamples, "")
	stems := make(map[string][]float64, len(insts))
	for _, in := range insts {
		stems[in.ID] = renderOnePass(notes, bpm, sampleRate, totalSamples, in.ID)
	}
	return master, stems, nil
}
```

- [ ] **Step 2: Build smoke (prod build, with CGo)** — `cd src/go && ../../.tools/go/bin/go build ./internal/audio/` → exit 0. (This compiles the CGo path. If any helper name differs from the spike — `newTestMixer`, `renderMixer`, `resetSendEffectsState`, `newRecipeAwareVoicePitched`, `scaledVoice{v,gain}`, `(*mixer).Schedule`, `SetDelaySend` — grep the real name in `internal/audio/*.go` and use it. The `voice` interface type of `newRecipeAwareVoicePitched`'s return and `scaledVoice`'s field must line up; adjust the wrapper to the real `scaledVoice` literal.)

- [ ] **Step 3: Vet** — `../../.tools/go/bin/go vet ./internal/audio/` → exit 0.

- [ ] **Step 4: Confirm test build still compiles** (the facade is excluded under `-tags test`, so package audio must still build): `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run NONE` → exit 0 (compiles, runs nothing).

- [ ] **Step 5: Checkpoint**

---

### Task 6: Production renderer (`//go:build !test`)

**Files:**
- Create: `src/go/internal/audio/songrender/render_prod.go` (`//go:build !test`)
- Test: build smoke (prod) + reuse the package's arrangement tests (which are tag-free).

**Interfaces:**
- Consumes: `audio.RenderSongOffline` (Task 5); `Arrangement` (Task 3).
- Produces: `func Render(stem string, bars, sampleRate int) (Rendered, error)` — the faithful path: load → `BuildArrangement` → map instruments/notes to `audio.SongRenderInstrument`/`audio.SongRenderNote` → `audio.RenderSongOffline` → `Rendered`.

- [ ] **Step 1: Implement `render_prod.go`**

```go
//go:build !test

package songrender

import "github.com/ingyamilmolinar/beatmo/internal/audio"

// Render (production build): faithful offline mix through the real audio engine.
func Render(stem string, bars, sampleRate int) (Rendered, error) {
	pt, err := LoadTemplate(stem)
	if err != nil {
		return Rendered{}, err
	}
	arr, err := BuildArrangement(pt, bars, sampleRate)
	if err != nil {
		return Rendered{}, err
	}
	insts := make([]audio.SongRenderInstrument, len(arr.Instruments))
	for i, in := range arr.Instruments {
		insts[i] = audio.SongRenderInstrument{
			ID: in.ID, Recipe: in.Recipe, SynthParams: in.SynthParams,
			Volume: in.Volume, Pan: in.Pan, ReverbSend: in.ReverbSend,
			DelaySend: in.DelaySend, Effects: in.Effects,
		}
	}
	notes := make([]audio.SongRenderNote, len(arr.Notes))
	for i, n := range arr.Notes {
		notes[i] = audio.SongRenderNote{
			InstID: n.InstID, AtSample: n.AtSample, Pitch: n.PitchSemis,
			Gain: n.Volume, DurSec: n.DurSec,
		}
	}
	master, stems, err := audio.RenderSongOffline(insts, notes, arr.BPM, sampleRate, arr.TotalSamples)
	if err != nil {
		return Rendered{}, err
	}
	return Rendered{Arrangement: arr, Master: master, Stems: stems}, nil
}
```

- [ ] **Step 2: Build smoke (prod)** — `cd src/go && ../../.tools/go/bin/go build ./internal/audio/songrender/` → exit 0.

- [ ] **Step 3: Vet (prod)** — `../../.tools/go/bin/go vet ./internal/audio/songrender/` → exit 0.

- [ ] **Step 4: Confirm fast-path tests still pass** (arrangement tests are tag-free; `Render` resolves to the stub under `-tags test`): `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/songrender/` → PASS.

- [ ] **Step 5: Checkpoint**

---

### Task 7: `song-match render` CLI subcommand + make target

**Files:**
- Modify: `src/go/cmd/song-match/main.go` (add the `render` subcommand to the existing `timeline` dispatcher from Plan 1)
- Modify: `Makefile` (add `song-render` target; extend the `.PHONY` line)

**Interfaces:**
- Consumes: `songrender.Render`; `audio.ExportCaptureToWAV`.

- [ ] **Step 1: Add the `render` subcommand** — extend `main()`'s switch in `cmd/song-match/main.go`:

```go
	case "render":
		cmdRender(os.Args[2:])
```

and add (same file):

```go
func cmdRender(args []string) {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	stem := fs.String("template", "", "template stem id, e.g. bach-toccata (required)")
	outDir := fs.String("out", ".", "output directory for master.wav + <inst>.wav stems")
	bars := fs.Int("bars", 8, "number of bars to render")
	sr := fs.Int("sr", 44100, "sample rate")
	_ = fs.Parse(args)
	if *stem == "" {
		fmt.Fprintln(os.Stderr, "song-match render: -template is required")
		os.Exit(1)
	}
	audio.Reset()
	audio.ResetInstruments()
	r, err := songrender.Render(*stem, *bars, *sr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "render: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(*outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "mkdir: %v\n", err)
		os.Exit(1)
	}
	masterPath := filepath.Join(*outDir, "master.wav")
	if err := audio.ExportCaptureToWAV(r.Master, masterPath); err != nil {
		fmt.Fprintf(os.Stderr, "write master: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d samples)\n", masterPath, len(r.Master))
	for id, s := range r.Stems {
		p := filepath.Join(*outDir, id+".wav")
		if err := audio.ExportCaptureToWAV(s, p); err != nil {
			fmt.Fprintf(os.Stderr, "write stem %s: %v\n", id, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", p)
	}
}
```

Add imports `path/filepath` and `github.com/ingyamilmolinar/beatmo/internal/audio/songrender` to the file (keep existing imports). Update the usage string in `main()` to mention `render`.

- [ ] **Step 2: Build smoke + artifact cleanup** — `cd src/go && ../../.tools/go/bin/go build ./cmd/song-match/` → exit 0; then `rm -f song-match`.

- [ ] **Step 3: Vet** — `../../.tools/go/bin/go vet ./cmd/song-match/` → exit 0.

- [ ] **Step 4: Add the make target** — in `Makefile`, add to the synth/song `.PHONY` line `song-render`, and add:

```make
song-render: ## Render a template to master.wav + per-instrument stems (TEMPLATE=, OUT=, [BARS=], [SR=])
	cd src/go && ../../.tools/go/bin/go run ./cmd/song-match render -template $(TEMPLATE) -out $(OUT) \
		$(if $(BARS),-bars $(BARS),) $(if $(SR),-sr $(SR),)
```

- [ ] **Step 5: End-to-end (prod)** — `make song-render TEMPLATE=bach-toccata OUT=/tmp/bach BARS=2` → writes `/tmp/bach/master.wav` + stem WAVs without error. Then clean `/tmp/bach`. Checkpoint.

---

### Task 8: Fast-path integration test (arrangement + stub render contract)

**Files:**
- Create: `src/go/internal/audio/songrender/integration_test.go` (`//go:build test`)

**Interfaces:** none (test only).

- [ ] **Step 1: Write the test** — render every built-in template under the stub and assert the arrangement + master/stems invariants hold across the whole catalogue (catches a template whose graph produces no notes, or whose instruments fail to map):

```go
//go:build test

package songrender

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestAllTemplates_RenderStubInvariants(t *testing.T) {
	for _, tpl := range assets.Templates() {
		stem := tpl.Genre
		t.Run(stem, func(t *testing.T) {
			audio.Reset()
			audio.ResetInstruments()
			r, err := Render(stem, 2, 44100)
			if err != nil {
				t.Fatalf("%s: render: %v", stem, err)
			}
			if r.Arrangement.TotalSamples <= 0 {
				t.Fatalf("%s: no samples", stem)
			}
			if len(r.Master) != r.Arrangement.TotalSamples {
				t.Errorf("%s: master len %d != totalSamples %d", stem, len(r.Master), r.Arrangement.TotalSamples)
			}
			// Master == Σ stems (stub contract).
			sum := make([]float64, len(r.Master))
			for _, s := range r.Stems {
				for i := range s {
					sum[i] += s[i]
				}
			}
			for i := range r.Master {
				if d := r.Master[i] - sum[i]; d > 1e-9 || d < -1e-9 {
					t.Fatalf("%s: master != Σ stems at %d", stem, i)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run** — `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/songrender/ -run TestAllTemplates` → PASS for every template. (If a template emits zero notes, investigate its graph wiring / origin mapping before weakening the assertion — a no-note template is a real bug.)

- [ ] **Step 3: Whole-package** — `../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/songrender/` → PASS. Checkpoint.

---

## Self-Review

**Spec coverage (Plan 2 slice of the song-wav-matching spec §5):**
- §5 offline renderer (template → master + stems, template-faithful) → Tasks 1–7. Arrangement (template→graph→predictor→events) = Tasks 1–3; testable stub = Task 4; faithful prod via new `audio.RenderSongOffline` facade = Tasks 5–6; CLI `render` + make = Task 7; catalogue integration test = Task 8.
- Build-tag split mirroring `synthmatch` (prod fidelity, testable stub) → Tasks 4/5/6 (Global Constraints).
- `resetSendEffectsState` between renders + global-state reset → Task 5 facade + Global Constraints.
- Deferred to Plan 3 (not here): `SongFingerprint`/`SongTimeline` of the rendered master (Plan 1, done), the real-vs-template `Compare`, instrument-activity, tuning. Plan 2 produces only the `Rendered{Arrangement, Master, Stems}` that Plan 3 consumes.

**Placeholder scan:** none — every step has concrete code or an exact build/vet/make command. The spots that say "confirm the real name if it differs" name the exact symbol from the spike (`file:line` provided) and are verification, not placeholders.

**Type consistency:** `ParsedTemplate`/`InstrumentSpec`/`NodeSpec` (Task 1) consumed by `BuildGraph` (Task 2) → `RowPaths` → `BuildArrangement` (Task 3) → `Arrangement`/`NoteEvent` → `Render`/`Rendered` (Tasks 4/6) → CLI (Task 7). The `audio.SongRenderInstrument`/`SongRenderNote` facade types (Task 5) are consumed only by `render_prod.go` (Task 6). `Render(stem, bars, sampleRate)` has one signature, two build-tagged implementations.

**Open verification during execution:** `core/log` `New`/`LevelError` signature (Tasks 2/3); `audio.EffectSlot` JSON field names vs the template `effects` shape (Task 1); the exact unexported mixer/voice helper names + `scaledVoice` literal + the `voice` interface returned by `newRecipeAwareVoicePitched` (Task 5 — all named with `file:line` from the spike); whether `AudibleAt` needs `Ensure` called with a larger horizon than `totalSteps` for loop folding (Task 3).
