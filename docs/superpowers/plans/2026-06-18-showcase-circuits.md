# Genre Showcase Circuits Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: superpowers:subagent-driven-development. Steps use checkbox (`- [ ]`) syntax.

**Goal:** Add genre showcase circuits to the **Load Template** menu, featuring the 20 new synthesized instruments, so they can be auditioned in real musical context.

**Architecture:** There is NO Go template generator in this repo — the committed `internal/assets/templates/*.json` are the hand-authored source of truth, registered via `templateOrder` in `internal/assets/templates.go`. Hand-placing square-loop node coordinates for melodic circuits is impractical, so this plan builds a small **circuit-builder tool** (`cmd/gen-showcase`) that converts compact pattern-card data → the v1 export JSON, writes the template files, and is pinned by a drift test. New templates auto-appear in the menu (display name auto-derived, BPM read from JSON).

**Tech Stack:** Go (bundled `.tools/go/bin/go`), the v1 circuit JSON schema (`internal/ui/export.go`).

**Depends on:** the 20 instruments (instrument-seed-library plan, DONE — `violin`/`cello`/`piano-grand`/`flute`/`trumpet`/`guitar-electric` etc. are registered builtins).

## Global Constraints

- **NO Go generator exists.** Templates are committed JSON in `internal/assets/templates/`, listed in `templateOrder` (`templates.go:28`). The builder this plan adds writes ONLY the new showcase JSON files (the existing 7 genre templates stay hand-authored, untouched).
- **Menu wiring:** append each new stem to `templateOrder` (`templates.go:28`) AND to `wantGenres` in `templates_test.go:15` (it asserts exact-equality with the ordered list — it WILL fail otherwise). Display name auto-derives from the stem via `displayName()` (e.g. `classical-canon` → "Classical-Canon"); BPM read from the JSON's `bpm`.
- **Instrument resolution:** every instrument id used MUST be a registered builtin (`TestTemplates_AllInstrumentsRegistered` enforces it). The 20 new ids qualify. Each instrument entry needs `kind:"builtin"`, an `id`, `name`, `volume`, `color` (#RRGGBBAA), and `origin` (the node id where its loop starts).
- **Monophonic rows:** each instrument is ONE square loop = ONE voice. Express harmony as ARPEGGIATED / broken-chord single-note lines (like the existing salsa `fm-epiano` montuno), NOT block chords. A circuit = drum rows + a bass line + an arpeggiated comp + a lead line.
- **Pitch convention:** seed/modular instruments have base **A3 (220 Hz)** at `pitch:0`; node `pitch` = semitone offset (resampled). Offsets used below are from A3: A3=0, C4=+3, D4=+5, E4=+7, A4=+12, A5=+24, A6=+36; below: G3=−2, E3=−5, D3=−7, A2=−12, D2=−19. Drums use `pitch:0`.
- **Subdiv 16, square-loop layout** (verified against rock.json): perimeter = `bars*64` units, `side = perimeter/4`, `unitsPerStep = 4`, row j-offset `baseJ = rowIndex*(side+8)`. Corners at steps `0, totalSteps/4, totalSteps/2, 3*totalSteps/4`.
- **Bundled Go;** tests run normally (`cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/ ./internal/ui/ -run Template`). The circuit-builder is a `cmd` tool run with the real build.
- **Do NOT `git add`/`git commit`** — leave uncommitted (standing rule). "Checkpoint" = run tests + pause.

---

## File Structure

| File | Responsibility | Change |
|---|---|---|
| `src/go/cmd/gen-showcase/main.go` (NEW) | circuit-builder: pattern-card structs + perimeter layout + JSON emit + write files | Create |
| `src/go/cmd/gen-showcase/specs.go` (NEW) | the showcase pattern-card data (musical content) | Create |
| `src/go/internal/assets/templates/<stem>.json` (NEW ×N) | generated showcase circuits | Create (via the tool) |
| `src/go/internal/assets/templates.go` | menu order | Append stems to `templateOrder` |
| `src/go/internal/assets/templates_test.go` | order assertion | Append stems to `wantGenres` |
| `src/go/cmd/gen-showcase/drift_test.go` (NEW) | regenerate == committed | Create |

---

### Task 1: Circuit-builder tool + the two public-domain classical circuits

Builds the tool and validates the whole pipeline with `classical-canon` + `classical-ode` (violin + cello + piano-grand; no drums → simplest melodic case), confirming they appear in the Load Template menu and import cleanly.

**Files:** create `cmd/gen-showcase/{main.go,specs.go,drift_test.go}`; modify `templates.go`, `templates_test.go`; generate `templates/classical-canon.json`, `templates/classical-ode.json`.

- [ ] **Step 1: Write the circuit-builder** (`cmd/gen-showcase/main.go`)

Input structs + the deterministic layout. Use exactly this layout (matches rock.json):

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

type Hit struct {
	Step  int
	Pitch float64
	Vol   float64 // 0 => omit (use instrument default)
	Dur   float64 // 0 => omit
}
type RowSpec struct {
	Inst string // instrument id
	Hits []Hit
}
type InstSpec struct {
	ID, Name, Color string
	Volume, Pan     float64
	ReverbSend      float64
	SynthParams     map[string]float64
}
type Showcase struct {
	Stem        string
	BPM, Subdiv int
	Bars        int
	Insts       []InstSpec
	Rows        []RowSpec // row i uses Insts[i] (1:1, same order)
}

type node struct {
	ID       int       `json:"id"`
	I        int       `json:"i"`
	J        int       `json:"j"`
	Type     string    `json:"type"`
	Outputs  []int     `json:"outputs,omitempty"`
	Pitch    float64   `json:"pitch,omitempty"`
	Volume   float64   `json:"volume,omitempty"`
	Duration float64   `json:"duration,omitempty"`
}
type inst struct {
	Name        string             `json:"name"`
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Volume      float64            `json:"volume"`
	Origin      int                `json:"origin"`
	Color       string             `json:"color"`
	Pan         float64            `json:"pan,omitempty"`
	ReverbSend  float64            `json:"reverb_send,omitempty"`
	SynthParams map[string]float64 `json:"synth_params,omitempty"`
}
type doc struct {
	Version     int     `json:"version"`
	Subdiv      int     `json:"subdiv"`
	BPM         int     `json:"bpm"`
	Instruments []inst  `json:"instruments"`
	Nodes       []node  `json:"nodes"`
}

func perimeterPoint(p, side int) (int, int) {
	switch {
	case p < side:
		return p, 0
	case p < 2*side:
		return side, p - side
	case p < 3*side:
		return 3*side - p, side
	default:
		return 0, 4*side - p
	}
}

// build returns the JSON bytes for one showcase circuit.
func build(s Showcase) ([]byte, error) {
	totalSteps := s.Bars * 16
	unitsPerStep := s.Subdiv / 4
	perimeter := totalSteps * unitsPerStep
	side := perimeter / 4
	var d doc
	d.Version, d.Subdiv, d.BPM = 1, s.Subdiv, s.BPM
	idBase := 0
	for ri, row := range s.Rows {
		baseJ := ri * (side + 8)
		hitByStep := map[int]Hit{}
		for _, h := range row.Hits {
			hitByStep[h.Step] = h
		}
		stepSet := map[int]bool{0: true, totalSteps / 4: true, totalSteps / 2: true, 3 * totalSteps / 4: true}
		for st := range hitByStep {
			stepSet[st] = true
		}
		steps := make([]int, 0, len(stepSet))
		for st := range stepSet {
			steps = append(steps, st)
		}
		sort.Ints(steps)
		rowNodes := make([]node, 0, len(steps))
		for k, st := range steps {
			i, j := perimeterPoint(st*unitsPerStep, side)
			n := node{ID: idBase + k, I: i, J: baseJ + j, Type: "silent"}
			if h, ok := hitByStep[st]; ok {
				n.Type = "regular"
				n.Pitch = h.Pitch
				n.Volume = h.Vol
				n.Duration = h.Dur
			}
			rowNodes = append(rowNodes, n)
		}
		nn := len(rowNodes)
		for k := range rowNodes {
			rowNodes[k].Outputs = []int{rowNodes[(k+1)%nn].ID}
		}
		ins := s.Insts[ri]
		d.Instruments = append(d.Instruments, inst{
			Name: ins.Name, ID: ins.ID, Kind: "builtin", Volume: ins.Volume,
			Origin: rowNodes[0].ID, Color: ins.Color, Pan: ins.Pan,
			ReverbSend: ins.ReverbSend, SynthParams: ins.SynthParams,
		})
		d.Nodes = append(d.Nodes, rowNodes...)
		idBase += nn
	}
	return json.MarshalIndent(d, "", "  ")
}

func main() {
	outDir := filepath.Join("internal", "assets", "templates")
	for _, s := range showcases() {
		b, err := build(s)
		if err != nil {
			panic(err)
		}
		if err := os.WriteFile(filepath.Join(outDir, s.Stem+".json"), append(b, '\n'), 0o644); err != nil {
			panic(err)
		}
	}
}
```

- [ ] **Step 2: Write the two classical specs** (`cmd/gen-showcase/specs.go`)

```go
package main

// Pitch offsets from A3 (220Hz). Classical pieces transposed to fit the A-based
// modular tuning; intervals/contour preserved for recognizability.
func showcases() []Showcase {
	violinC := "#FFB30AFF"
	celloC := "#C87850FF"
	pianoC := "#00C8FFFF"
	return []Showcase{
		{
			Stem: "classical-canon", BPM: 96, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "violin", Name: "Violin", Color: violinC, Volume: 0.85, ReverbSend: 0.25},
				{ID: "piano-grand", Name: "Piano", Color: pianoC, Volume: 0.6, ReverbSend: 0.15},
				{ID: "cello", Name: "Cello", Color: celloC, Volume: 0.8, ReverbSend: 0.2},
			},
			Rows: []RowSpec{
				// Violin: Canon first phrase, half notes (steps 0 & 8 of each bar), long.
				{Inst: "violin", Hits: []Hit{
					{Step: 0, Pitch: 21, Dur: 1.8}, {Step: 8, Pitch: 19, Dur: 1.8},
					{Step: 16, Pitch: 17, Dur: 1.8}, {Step: 24, Pitch: 16, Dur: 1.8},
				}},
				// Piano: broken-chord arpeggio (8ths) over the Romanesca (D A Bm F#m).
				{Inst: "piano-grand", Hits: []Hit{
					{Step: 0, Pitch: 5}, {Step: 2, Pitch: 9}, {Step: 4, Pitch: 12}, {Step: 6, Pitch: 9},
					{Step: 8, Pitch: 0}, {Step: 10, Pitch: 4}, {Step: 12, Pitch: 7}, {Step: 14, Pitch: 4},
					{Step: 16, Pitch: 2}, {Step: 18, Pitch: 5}, {Step: 20, Pitch: 9}, {Step: 22, Pitch: 5},
					{Step: 24, Pitch: 9}, {Step: 26, Pitch: 12}, {Step: 28, Pitch: 16}, {Step: 30, Pitch: 12},
				}},
				// Cello: descending ground bass, one per half note, low register.
				{Inst: "cello", Hits: []Hit{
					{Step: 0, Pitch: -7, Dur: 1.8}, {Step: 8, Pitch: -12, Dur: 1.8},
					{Step: 16, Pitch: -10, Dur: 1.8}, {Step: 24, Pitch: -15, Dur: 1.8},
				}},
			},
		},
		{
			Stem: "classical-ode", BPM: 114, Subdiv: 16, Bars: 2,
			Insts: []InstSpec{
				{ID: "violin", Name: "Violin", Color: violinC, Volume: 0.85, ReverbSend: 0.2},
				{ID: "piano-grand", Name: "Piano", Color: pianoC, Volume: 0.55, ReverbSend: 0.12},
				{ID: "cello", Name: "Cello", Color: celloC, Volume: 0.75, ReverbSend: 0.18},
			},
			Rows: []RowSpec{
				// Ode to Joy, quarter notes (steps 0,4,8,12 per bar). Phrase: F#-F#-G-A-A-G-F#-E.
				{Inst: "violin", Hits: []Hit{
					{Step: 0, Pitch: 9, Dur: 0.9}, {Step: 4, Pitch: 9, Dur: 0.9}, {Step: 8, Pitch: 10, Dur: 0.9}, {Step: 12, Pitch: 12, Dur: 0.9},
					{Step: 16, Pitch: 12, Dur: 0.9}, {Step: 20, Pitch: 10, Dur: 0.9}, {Step: 24, Pitch: 9, Dur: 0.9}, {Step: 28, Pitch: 7, Dur: 0.9},
				}},
				// Piano: simple chord-tone arpeggio under the melody (I and V).
				{Inst: "piano-grand", Hits: []Hit{
					{Step: 0, Pitch: 5}, {Step: 4, Pitch: 9}, {Step: 8, Pitch: 12}, {Step: 12, Pitch: 9},
					{Step: 16, Pitch: 0}, {Step: 20, Pitch: 4}, {Step: 24, Pitch: 7}, {Step: 28, Pitch: 4},
				}},
				// Cello: roots D then A, low, half notes.
				{Inst: "cello", Hits: []Hit{
					{Step: 0, Pitch: -7, Dur: 1.8}, {Step: 8, Pitch: -7, Dur: 1.8},
					{Step: 16, Pitch: -12, Dur: 1.8}, {Step: 24, Pitch: -12, Dur: 1.8},
				}},
			},
		},
	}
}
```

- [ ] **Step 3: Generate the JSON** — `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-showcase`. Confirm `internal/assets/templates/classical-canon.json` + `classical-ode.json` exist and are valid JSON (`version:1`, instruments[], nodes[]).

- [ ] **Step 4: Register in the menu** — append `"classical-canon", "classical-ode"` to `templateOrder` (`templates.go:28`) AND to `wantGenres` (`templates_test.go:15`).

- [ ] **Step 5: Drift test** (`cmd/gen-showcase/drift_test.go`) — for each showcase, `build(s)` in-memory and assert it equals the committed `internal/assets/templates/<stem>.json` bytes. Fails if someone edits the JSON without the spec (keeps generator↔committed in sync). Run it.

- [ ] **Step 6: Validate menu + import**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/ ./internal/ui/ -run 'Template' -v`
Expected: PASS — `TestTemplates_OrderedAndNonEmpty` (9 templates now), `TestTemplates_ImportRoundTrip`, `TestTemplates_AllInstrumentsRegistered` (violin/cello/piano-grand resolve), drift test.

- [ ] **Step 7: Checkpoint (no commit).** The two classical circuits now appear in Load Template ("Classical-Canon", "Classical-Ode"). User auditions — first time hearing the strings + piano in a real piece.

---

### Task 2: Genre showcase circuits (drums + bass + comp + lead)

Add one strong circuit per remaining genre, each featuring new instruments, as `Showcase` specs in `specs.go` (the builder already handles everything). Append each stem to `templateOrder` + `wantGenres`, regenerate, validate.

Drum rows use existing drum instruments (`kick`/`snare`/`hihat`/`clap`/`cowbell`/`tom` — `pitch:0`); melodic rows use the new instruments. Pattern content from the design spec Appendix B (`docs/superpowers/specs/2026-06-18-synth-orchestral-instruments-and-showcases-design.md`). Translate each card's per-row step placements into `[]Hit` with A3-relative pitch offsets.

Circuits (stem → featured new instruments):
- `rock-anthem` → `guitar-electric` riff + `sub-bass` + kick/snare/hihat
- `pop-ballad` → `piano-grand` arpeggio + `flute` melody + `cello` pad + kick/clap/hihat
- `jazz-swing` → `piano-felt` comp + `fm-bass` walking line + `trumpet` lead + ride(hihat)/snare
- `salsa-mambo` → `piano-grand` montuno + `trumpet` stabs + `fm-bass` tumbao + cowbell/tom(congas)/sidestick(clave)
- `funk-groove` → `guitar-electric` chank + `fm-bass` slap line + `trumpet` stabs + kick/snare/hihat
- `house-piano` → `piano-grand` stabs + `sub-bass` + four-on-floor kick/clap/openhat
- `techno-stab` → `trumpet` (or `oboe`) detuned stab + `fm-bass` + kick/clap/hihat
- `hiphop-boombap` → `piano-felt` loop + `sub-bass` + boom-bap kick/snare/hihat

For EACH: add the `Showcase` to `showcases()`, append stem to `templateOrder` + `wantGenres`, `go run ./cmd/gen-showcase`, then run the Template test suite (Step 6 command) — must stay green (import round-trip + all-instruments-registered + drift).

- [ ] **Step 1:** author `rock-anthem` + `pop-ballad` specs (guitar/flute/piano/cello) → regenerate → register → Template suite green.
- [ ] **Step 2:** author `jazz-swing` + `salsa-mambo` (piano/trumpet/bass) → regenerate → register → suite green.
- [ ] **Step 3:** author `funk-groove` + `house-piano` → regenerate → register → suite green.
- [ ] **Step 4:** author `techno-stab` + `hiphop-boombap` → regenerate → register → suite green.
- [ ] **Step 5: Checkpoint (no commit).** 10 showcase circuits in the menu. User auditions the full set; we tune the instruments by ear based on how they sit in context.

---

### Task 3: Browser parity + docs

- [ ] **Step 1:** `make wasm` then load each showcase in a browser smoke (the templates import through the same path; `wasm_bridge_smoke` already covers the bridge). Confirm no console errors importing a melodic template with new instruments.
- [ ] **Step 2:** Note in `AGENTS.synth.instruments.md` that showcase circuits live in `cmd/gen-showcase` (regenerate via `go run ./cmd/gen-showcase`) and are drift-tested — so the authoring path is discoverable.
- [ ] **Step 3: Checkpoint (no commit).** Hand off the full set.

---

## Self-Review

**Spec coverage:** showcase circuits in Load Template (design §5) — Tasks 1–2; classical = exact public-domain transposed (Canon, Ode) — Task 1; genre circuits featuring all new instrument families — Task 2; browser + docs — Task 3. ✓
**No generator assumption corrected:** templates are hand-authored JSON; this plan adds a builder that WRITES the showcase JSON + a drift test, leaving the existing 7 untouched. ✓
**Menu test:** `wantGenres` updated alongside `templateOrder` every time (Task 1 Step 4, Task 2 each step) — the exact-equality assertion is the easy-to-miss gate. ✓
**Monophonic-row reality:** harmony is arpeggiated single-note lines (matches the engine's one-loop-per-instrument model + the existing salsa montuno). ✓
**Placeholders:** builder code is complete; classical specs are concrete; genre specs (Task 2) are authored during execution from Appendix B with the stated A3-offset convention. ✓
