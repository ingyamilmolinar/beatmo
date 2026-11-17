# Circuit Templates Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a "Load template" entry to the overflow ("…") menu that lists seven first-party, genre-specific circuits (rock, hip-hop, pop, funk, salsa, house, techno), each a generated, bundled JSON file that imports immediately.

**Architecture:** A pure `internal/templates` package holds declarative genre specs + a square-loop graph layout function + a JSON emitter. A `cmd/gen-templates` writer emits seven committed JSON files into `internal/assets/templates/`, embedded via `//go:embed` and exposed through `assets.Templates()`. The overflow menu becomes page-aware to show the template list; selecting one feeds bytes to the existing import queue (`DrumView.onImport` → `Game.pendingImportData` → `Game.Import`). A drift test keeps the committed JSON in lock-step with the generator.

**Tech Stack:** Go 1.23 (bundled `.tools/go/bin/go`), `-tags test -modfile=go.test.mod` fast path, `encoding/json`, `//go:embed`, Ebiten-stub UI test harness.

**Key facts established during design (do not re-derive):**
- **Distance = time.** Between two explicit nodes joined by an orthogonal edge, the traversal (`core/model/graph_traversal.go:71-107`) synthesizes one silent step per grid unit. An instrument fires only at its explicit `regular` nodes. `subdiv` = grid units per beat; a loop's beats = perimeter_units ÷ subdiv.
- **Authoring primitive:** each instrument row is a square loop. At `subdiv:16`, 1 bar = 64 units, a 16×16 square; salsa uses 2 bars = 128 units, a 32×32 square. Hit-nodes are `regular`; unused corners are `silent` turns.
- **Import** (`internal/ui/import.go:69`): replace-not-merge; row 0's `origin` becomes `StartNodeID` (line 461-467); `synth_params` apply as a raw overlay even when `recipe` is omitted (line 561-564); coordinates scale proportionally by `MaxDiv/subdiv`.
- **Menu trigger:** `DrumView.onImport(data)` stashes `Game.pendingImportData`, drained in `Game.Update()` → `Game.Import`. Never call `g.Import` directly from input handling.
- **Bundled commands:** always `cd src/go && ../../.tools/go/bin/go ...`.

---

## File Structure

**New files:**
- `src/go/internal/templates/pattern.go` — glyph pattern parser (`parsePattern`).
- `src/go/internal/templates/pattern_test.go` — parser tests.
- `src/go/internal/templates/layout.go` — square-loop graph layout + JSON doc structs + `Generate`.
- `src/go/internal/templates/layout_test.go` — geometry + JSON tests.
- `src/go/internal/templates/genres.go` — the seven genre `Spec`s + `Specs()`.
- `src/go/internal/templates/genres_test.go` — spec-invariant tests.
- `src/go/cmd/gen-templates/main.go` — writer (emits the seven JSON files).
- `src/go/internal/assets/templates/{rock,hip-hop,pop,funk,salsa,house,techno}.json` — generated, committed.
- `src/go/internal/assets/templates.go` — `//go:embed` + `Templates()` registry.
- `src/go/internal/assets/templates_test.go` — registry + drift tests.
- `src/go/internal/ui/template_menu_test.go` — overflow page + import-wiring tests.

**Modified files:**
- `src/go/internal/ui/drumview_context_menu.go` — page-aware `overflowItems()` + `overflowPage` reset.
- `src/go/internal/ui/drumview.go` (or wherever `DrumView` struct is) — add `overflowPage int` field.
- `Makefile` — `gen-templates` target.
- `AGENTS.synth.md`, `AGENTS.synth.instruments.md` — genre instrument-configuration reference.
- `CLAUDE.md` — one-line pointer to the templates feature.

---

## Task 1: Pattern parser

**Files:**
- Create: `src/go/internal/templates/pattern.go`
- Test: `src/go/internal/templates/pattern_test.go`

The parser turns a glyph string into hits. Glyphs (spaces and `|` ignored, used only for readability): `.`=rest, `X`=normal hit, `g`=ghost (velocity 0.25), `o`=accent (velocity 1.2), `p`=probability hit (logic_kind `probability`, p 0.6), `x`=swung hit (groove `delay`, pct 0.55). Pitches align to hits in left-to-right order.

- [ ] **Step 1: Write the failing test**

```go
package templates

import (
	"reflect"
	"testing"
)

func TestParsePattern_BasicAndAttributes(t *testing.T) {
	hits, err := parsePattern("X.X. X.X.", []float64{1, 2, 3, 4}, 8)
	if err != nil {
		t.Fatalf("parsePattern err: %v", err)
	}
	wantSteps := []int{0, 2, 4, 6}
	gotSteps := make([]int, len(hits))
	for i, h := range hits {
		gotSteps[i] = h.Step
	}
	if !reflect.DeepEqual(gotSteps, wantSteps) {
		t.Fatalf("steps=%v want=%v", gotSteps, wantSteps)
	}
	if hits[0].Pitch != 1 || hits[3].Pitch != 4 {
		t.Fatalf("pitches not aligned: %+v", hits)
	}
}

func TestParsePattern_GlyphSemantics(t *testing.T) {
	hits, err := parsePattern("Xgpx", nil, 4)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(hits) != 4 {
		t.Fatalf("want 4 hits, got %d", len(hits))
	}
	if hits[1].Velocity != 0.25 {
		t.Fatalf("ghost velocity=%v want 0.25", hits[1].Velocity)
	}
	if hits[2].Logic != "probability" || hits[2].LogicP != 0.6 {
		t.Fatalf("probability glyph wrong: %+v", hits[2])
	}
	if hits[3].Groove != "delay" || hits[3].GroovePct != 0.55 {
		t.Fatalf("swing glyph wrong: %+v", hits[3])
	}
}

func TestParsePattern_WrongLength(t *testing.T) {
	if _, err := parsePattern("X.X", nil, 16); err == nil {
		t.Fatalf("expected error for wrong-length pattern")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/templates/ -run TestParsePattern -v`
Expected: FAIL (undefined: parsePattern / Hit).

- [ ] **Step 3: Write minimal implementation**

```go
package templates

import (
	"fmt"
	"strings"
)

// Hit is one explicit firing position in a row's pattern.
type Hit struct {
	Step      int
	Pitch     float64 // per-node semitone offset
	Velocity  float64 // per-node volume; 0 => use the row default
	Duration  float64 // per-node note length; 0 => default 1
	Logic     string  // "" or "probability"
	LogicP    float64
	Groove    string // "" or "delay"
	GroovePct float64
}

// parsePattern decodes a glyph pattern into hits. Spaces and '|' are ignored
// (readability only). The remaining glyph count must equal totalSteps. Pitches
// align to hits in left-to-right order; missing entries default to 0.
func parsePattern(pattern string, pitches []float64, totalSteps int) ([]Hit, error) {
	var glyphs []rune
	for _, r := range pattern {
		if r == ' ' || r == '|' {
			continue
		}
		glyphs = append(glyphs, r)
	}
	if len(glyphs) != totalSteps {
		return nil, fmt.Errorf("pattern %q has %d steps, want %d", strings.TrimSpace(pattern), len(glyphs), totalSteps)
	}
	var hits []Hit
	for step, g := range glyphs {
		if g == '.' {
			continue
		}
		h := Hit{Step: step}
		switch g {
		case 'X':
			// normal hit, no attributes
		case 'g':
			h.Velocity = 0.25
		case 'o':
			h.Velocity = 1.2
		case 'p':
			h.Logic = "probability"
			h.LogicP = 0.6
		case 'x':
			h.Groove = "delay"
			h.GroovePct = 0.55
		default:
			return nil, fmt.Errorf("unknown pattern glyph %q at step %d", string(g), step)
		}
		if n := len(hits); n < len(pitches) {
			h.Pitch = pitches[n]
		}
		hits = append(hits, h)
	}
	return hits, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/templates/ -run TestParsePattern -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/templates/pattern.go src/go/internal/templates/pattern_test.go
git commit -m "feat(templates): glyph pattern parser for genre circuits"
```

---

## Task 2: Square-loop graph layout

**Files:**
- Create: `src/go/internal/templates/layout.go`
- Test: `src/go/internal/templates/layout_test.go`

Lays a row's hits around a square perimeter: `regular` node per hit, `silent` node per unused corner, edges in perimeter order closing the loop. Per-step distance is `subdiv/4` units; perimeter is `totalSteps*unitsPerStep`; side is `perimeter/4`. Step 0 is always a corner, so it always has a node (the origin).

- [ ] **Step 1: Write the failing test**

```go
package templates

import "testing"

// fourOnFloor: 4 hits at steps 0,4,8,12 in a 1-bar (16-step) loop, subdiv 16.
func TestLayoutRow_FourOnFloorGeometry(t *testing.T) {
	row := Row{Instrument: "kick", Pattern: "X... X... X... X...", Volume: 1}
	nodes, origin, nextID, err := layoutRow(16, 1, row, 0, 0)
	if err != nil {
		t.Fatalf("layoutRow err: %v", err)
	}
	// 4 hits land exactly on the 4 corners => 4 regular nodes, no silent nodes.
	if len(nodes) != 4 {
		t.Fatalf("want 4 nodes, got %d: %+v", len(nodes), nodes)
	}
	for _, n := range nodes {
		if n.Type != "regular" {
			t.Fatalf("node %d type=%q want regular", n.ID, n.Type)
		}
	}
	if origin != 0 || nextID != 4 {
		t.Fatalf("origin=%d nextID=%d want 0,4", origin, nextID)
	}
	// Perimeter side = 16*4/4 = 16 units. Corners at (0,0),(16,0),(16,16),(0,16).
	want := [][2]int{{0, 0}, {16, 0}, {16, 16}, {0, 16}}
	for i, n := range nodes {
		if n.I != want[i][0] || n.J != want[i][1] {
			t.Fatalf("node %d at (%d,%d) want (%d,%d)", i, n.I, n.J, want[i][0], want[i][1])
		}
	}
	// Edges form a closed loop in perimeter order.
	for i, n := range nodes {
		wantOut := nodes[(i+1)%len(nodes)].ID
		if len(n.Outputs) != 1 || n.Outputs[0] != wantOut {
			t.Fatalf("node %d outputs=%v want [%d]", n.ID, n.Outputs, wantOut)
		}
	}
}

// A hit between corners forces a silent corner node so edges stay orthogonal.
func TestLayoutRow_SilentCornersAndAttributes(t *testing.T) {
	// Hits only at steps 0 and 2. Corners are steps 0,4,8,12.
	row := Row{Instrument: "snare", Pattern: "X.X. .... .... ....", Volume: 1, Pitches: []float64{0, 7}}
	nodes, _, _, err := layoutRow(16, 1, row, 10, 1)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	// Explicit steps = hits{0,2} ∪ corners{0,4,8,12} = {0,2,4,8,12} => 5 nodes.
	if len(nodes) != 5 {
		t.Fatalf("want 5 nodes, got %d: %+v", len(nodes), nodes)
	}
	regular, silent := 0, 0
	for _, n := range nodes {
		if n.Type == "regular" {
			regular++
		} else if n.Type == "silent" {
			silent++
		}
		if n.ID < 10 {
			t.Fatalf("idBase not honored: node id %d < 10", n.ID)
		}
	}
	if regular != 2 || silent != 3 {
		t.Fatalf("regular=%d silent=%d want 2,3", regular, silent)
	}
	// Row index 1 => baseJ offset > 0 so it does not overlap row 0's band.
	if nodes[0].J < 1 {
		t.Fatalf("expected row-1 vertical offset, got J=%d", nodes[0].J)
	}
	// The step-2 hit carries its pitch (second hit => pitches[1]=7).
	var foundPitch bool
	for _, n := range nodes {
		if n.Type == "regular" && n.Pitch == 7 {
			foundPitch = true
		}
	}
	if !foundPitch {
		t.Fatalf("step-2 hit pitch 7 not applied: %+v", nodes)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/templates/ -run TestLayoutRow -v`
Expected: FAIL (undefined: layoutRow / Row / node).

- [ ] **Step 3: Write minimal implementation**

```go
package templates

import (
	"fmt"
	"sort"
)

// Spec is one genre template definition.
type Spec struct {
	Genre   string
	Display string
	BPM     int
	Subdiv  int // grid units per beat (16); must be divisible by 4
	Bars    int // loop length in bars (1; salsa 2)
	Rows    []Row
}

// Row is one instrument lane in a template.
type Row struct {
	Instrument  string
	SynthParams map[string]float64
	Volume      float64
	Pan         float64
	DelaySend   float64
	ReverbSend  float64
	Pattern     string    // glyph pattern, length Bars*16 after stripping spaces/'|'
	Pitches     []float64 // aligned to hits, optional
}

// node mirrors the importable JSON node shape (export.go exportNode subset).
type node struct {
	ID         int     `json:"id"`
	I          int     `json:"i"`
	J          int     `json:"j"`
	Type       string  `json:"type"`
	Outputs    []int   `json:"outputs,omitempty"`
	Volume     float64 `json:"volume,omitempty"`
	Pitch      float64 `json:"pitch,omitempty"`
	Duration   float64 `json:"duration,omitempty"`
	LogicKind  string  `json:"logic_kind,omitempty"`
	LogicN     int     `json:"logic_n,omitempty"`
	LogicP     float64 `json:"logic_p,omitempty"`
	GrooveKind string  `json:"groove_kind,omitempty"`
	GroovePct  float64 `json:"groove_pct,omitempty"`
}

// perimeterPoint maps a clockwise perimeter distance p (0..4*side) to (i,j) on
// a square of the given side anchored at (0,0).
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

// layoutRow places a row's hits around a square loop. Returns the nodes, the
// origin node id (step 0), and the next free id. idBase is the first id to
// assign; rowIndex offsets the square vertically so rows do not overlap.
func layoutRow(subdiv, bars int, row Row, idBase, rowIndex int) ([]node, int, int, error) {
	if subdiv%4 != 0 {
		return nil, 0, 0, fmt.Errorf("subdiv %d not divisible by 4", subdiv)
	}
	totalSteps := bars * 16
	hits, err := parsePattern(row.Pattern, row.Pitches, totalSteps)
	if err != nil {
		return nil, 0, 0, fmt.Errorf("row %q: %w", row.Instrument, err)
	}
	unitsPerStep := subdiv / 4
	perimeter := totalSteps * unitsPerStep
	side := perimeter / 4
	baseJ := rowIndex * (side + 8)

	hitByStep := make(map[int]Hit, len(hits))
	for _, h := range hits {
		hitByStep[h.Step] = h
	}
	stepSet := map[int]bool{0: true, totalSteps / 4: true, totalSteps / 2: true, 3 * totalSteps / 4: true}
	for s := range hitByStep {
		stepSet[s] = true
	}
	steps := make([]int, 0, len(stepSet))
	for s := range stepSet {
		steps = append(steps, s)
	}
	sort.Ints(steps)

	nodes := make([]node, 0, len(steps))
	for k, s := range steps {
		i, j := perimeterPoint(s*unitsPerStep, side)
		nd := node{ID: idBase + k, I: i, J: baseJ + j, Type: "silent"}
		if h, ok := hitByStep[s]; ok {
			nd.Type = "regular"
			nd.Pitch = h.Pitch
			nd.Volume = h.Velocity
			nd.Duration = h.Duration
			nd.LogicKind = h.Logic
			nd.LogicP = h.LogicP
			nd.GrooveKind = h.Groove
			nd.GroovePct = h.GroovePct
		}
		nodes = append(nodes, nd)
	}
	n := len(nodes)
	for k := range nodes {
		nodes[k].Outputs = []int{nodes[(k+1)%n].ID}
	}
	return nodes, nodes[0].ID, idBase + n, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/templates/ -run TestLayoutRow -v`
Expected: PASS (2 tests).

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/templates/layout.go src/go/internal/templates/layout_test.go
git commit -m "feat(templates): square-loop graph layout from hit patterns"
```

---

## Task 3: JSON document assembly (`Generate`)

**Files:**
- Modify: `src/go/internal/templates/layout.go` (append doc structs + `Generate`)
- Test: `src/go/internal/templates/layout_test.go` (append)

`Generate(spec)` assembles the importable JSON: instruments (origin = each row's loop origin, deterministic colors, synth_params/pan/sends) + concatenated nodes + a default send-effects block. Row 0's origin id is 0 so import makes it `StartNodeID`.

- [ ] **Step 1: Write the failing test**

```go
func TestGenerate_ValidImportableJSON(t *testing.T) {
	spec := Spec{
		Genre: "test", Display: "Test", BPM: 120, Subdiv: 16, Bars: 1,
		Rows: []Row{
			{Instrument: "kick", Volume: 1, Pattern: "X... X... X... X..."},
			{Instrument: "snare", Volume: 0.8, Pattern: ".... X... .... X...", ReverbSend: 0.15},
		},
	}
	data, err := Generate(spec)
	if err != nil {
		t.Fatalf("Generate err: %v", err)
	}
	var doc struct {
		Version     int `json:"version"`
		Subdiv      int `json:"subdiv"`
		BPM         int `json:"bpm"`
		Instruments []struct {
			ID         string  `json:"id"`
			Origin     int     `json:"origin"`
			ReverbSend float64 `json:"reverb_send"`
		} `json:"instruments"`
		Nodes []struct {
			ID   int    `json:"id"`
			Type string `json:"type"`
		} `json:"nodes"`
		Send map[string]any `json:"send_effects"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if doc.Version != 1 || doc.Subdiv != 16 || doc.BPM != 120 {
		t.Fatalf("header wrong: %+v", doc)
	}
	if len(doc.Instruments) != 2 {
		t.Fatalf("want 2 instruments, got %d", len(doc.Instruments))
	}
	if doc.Instruments[0].Origin != 0 {
		t.Fatalf("row0 origin=%d want 0 (becomes StartNodeID on import)", doc.Instruments[0].Origin)
	}
	if doc.Instruments[1].ReverbSend != 0.15 {
		t.Fatalf("snare reverb_send=%v want 0.15", doc.Instruments[1].ReverbSend)
	}
	if doc.Send == nil {
		t.Fatalf("send_effects block missing")
	}
	if len(doc.Nodes) == 0 {
		t.Fatalf("no nodes emitted")
	}
}

func TestGenerate_Deterministic(t *testing.T) {
	spec := Spec{Genre: "t", Display: "T", BPM: 120, Subdiv: 16, Bars: 1,
		Rows: []Row{{Instrument: "kick", Volume: 1, Pattern: "X... X... X... X..."}}}
	a, _ := Generate(spec)
	b, _ := Generate(spec)
	if string(a) != string(b) {
		t.Fatalf("Generate not deterministic")
	}
}
```

Add `"encoding/json"` to the test file imports if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/templates/ -run TestGenerate -v`
Expected: FAIL (undefined: Generate).

- [ ] **Step 3: Write minimal implementation**

Append to `src/go/internal/templates/layout.go` (add `"encoding/json"` to its imports):

```go
type instrument struct {
	Name        string             `json:"name"`
	ID          string             `json:"id"`
	Kind        string             `json:"kind"`
	Volume      float64            `json:"volume"`
	Origin      int                `json:"origin"`
	Color       string             `json:"color"`
	Pan         float64            `json:"pan,omitempty"`
	DelaySend   float64            `json:"delay_send,omitempty"`
	ReverbSend  float64            `json:"reverb_send,omitempty"`
	SynthParams map[string]float64 `json:"synth_params,omitempty"`
}

type sendDelay struct {
	TimeMs    float64 `json:"time_ms"`
	Feedback  float64 `json:"feedback"`
	DampingHz float64 `json:"damping_hz"`
}
type sendReverb struct {
	Room    float64 `json:"room"`
	Damping float64 `json:"damping"`
	Wet     float64 `json:"wet"`
}
type sendEffects struct {
	Delay  *sendDelay  `json:"delay,omitempty"`
	Reverb *sendReverb `json:"reverb,omitempty"`
}

type doc struct {
	Version     int          `json:"version"`
	Subdiv      int          `json:"subdiv,omitempty"`
	BPM         int          `json:"bpm"`
	Instruments []instrument `json:"instruments"`
	Nodes       []node       `json:"nodes"`
	SendEffects *sendEffects `json:"send_effects,omitempty"`
}

// rowColors cycles distinct, legible hues for template rows (#RRGGBBAA).
var rowColors = []string{
	"#44A8A8FF", "#C87850FF", "#FFF610FF", "#D74BC8FF",
	"#DC8C64FF", "#C85078FF", "#80C850FF", "#4FB4FFFF",
}

// Generate assembles importable JSON bytes for a genre spec. Node ids are
// assigned sequentially per row starting at 0, so row 0's origin is id 0 and
// import promotes it to StartNodeID. Deterministic: same spec => same bytes.
func Generate(spec Spec) ([]byte, error) {
	if spec.Subdiv == 0 {
		spec.Subdiv = 16
	}
	if spec.Bars == 0 {
		spec.Bars = 1
	}
	d := doc{Version: 1, Subdiv: spec.Subdiv, BPM: spec.BPM}
	idBase := 0
	for ri, row := range spec.Rows {
		nodes, origin, next, err := layoutRow(spec.Subdiv, spec.Bars, row, idBase, ri)
		if err != nil {
			return nil, err
		}
		idBase = next
		d.Nodes = append(d.Nodes, nodes...)
		name := strings.ToUpper(row.Instrument[:1]) + row.Instrument[1:]
		d.Instruments = append(d.Instruments, instrument{
			Name:        name,
			ID:          row.Instrument,
			Kind:        "builtin",
			Volume:      row.Volume,
			Origin:      origin,
			Color:       rowColors[ri%len(rowColors)],
			Pan:         row.Pan,
			DelaySend:   row.DelaySend,
			ReverbSend:  row.ReverbSend,
			SynthParams: row.SynthParams,
		})
	}
	d.SendEffects = &sendEffects{
		Delay:  &sendDelay{TimeMs: 300, Feedback: 0.3, DampingHz: 3000},
		Reverb: &sendReverb{Room: 0.7, Damping: 0.4, Wet: 0.3},
	}
	out, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
```

Add `"strings"` to the `layout.go` import block (used by `Generate`).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/templates/ -v`
Expected: PASS (all templates tests).

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/templates/layout.go src/go/internal/templates/layout_test.go
git commit -m "feat(templates): assemble importable JSON document from spec"
```

---

## Task 4: The seven genre specs

**Files:**
- Create: `src/go/internal/templates/genres.go`
- Test: `src/go/internal/templates/genres_test.go`

Encodes all seven genres as data. Open-hat rows use `hihat-1` (a distinct instrument id from closed `hihat`) so their longer `decay` param does not collide with the closed-hat channel.

- [ ] **Step 1: Write the failing test**

```go
package templates

import "testing"

func TestSpecs_AllSevenGenres(t *testing.T) {
	specs := Specs()
	wantOrder := []string{"rock", "hip-hop", "pop", "funk", "salsa", "house", "techno"}
	if len(specs) != len(wantOrder) {
		t.Fatalf("got %d specs, want %d", len(specs), len(wantOrder))
	}
	for i, s := range specs {
		if s.Genre != wantOrder[i] {
			t.Fatalf("spec %d genre=%q want %q", i, s.Genre, wantOrder[i])
		}
	}
}

// Every row's pattern parses at its declared length, and every spec generates
// valid JSON. Salsa is the only 2-bar genre.
func TestSpecs_GenerateAndInvariants(t *testing.T) {
	for _, s := range Specs() {
		if s.BPM <= 0 || s.Subdiv == 0 || s.Bars == 0 || len(s.Rows) == 0 {
			t.Fatalf("%s: bad header %+v", s.Genre, s)
		}
		if (s.Genre == "salsa") != (s.Bars == 2) {
			t.Fatalf("%s: bars=%d (only salsa is 2-bar)", s.Genre, s.Bars)
		}
		// Row 0 must fire on step 0 (its origin becomes StartNodeID).
		hits0, err := parsePattern(s.Rows[0].Pattern, s.Rows[0].Pitches, s.Bars*16)
		if err != nil {
			t.Fatalf("%s row0 pattern: %v", s.Genre, err)
		}
		if len(hits0) == 0 || hits0[0].Step != 0 {
			t.Fatalf("%s: row0 must have a hit on step 0", s.Genre)
		}
		if _, err := Generate(s); err != nil {
			t.Fatalf("%s: Generate err: %v", s.Genre, err)
		}
	}
}

// Open-hat rows must not reuse the closed-hat instrument id (shared params).
func TestSpecs_DistinctHatChannels(t *testing.T) {
	for _, s := range Specs() {
		seen := map[string]bool{}
		for _, r := range s.Rows {
			if seen[r.Instrument] {
				t.Fatalf("%s: duplicate instrument id %q (would share params/channel)", s.Genre, r.Instrument)
			}
			seen[r.Instrument] = true
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/templates/ -run TestSpecs -v`
Expected: FAIL (undefined: Specs).

- [ ] **Step 3: Write minimal implementation**

```go
package templates

// Specs returns the seven built-in genre templates in menu order. Patterns use
// the glyph alphabet from parsePattern: X hit, g ghost, o accent, p probability,
// x swing, . rest. Open-hat rows use hihat-1 (distinct id) with a longer decay.
func Specs() []Spec {
	P := func(kv map[string]float64) map[string]float64 { return kv }
	return []Spec{
		// ── Rock — 120 BPM, straight, foursquare ──
		{
			Genre: "rock", Display: "Rock", BPM: 120, Subdiv: 16, Bars: 1,
			Rows: []Row{
				{Instrument: "kick-tight", Volume: 1.0, SynthParams: P(map[string]float64{"drive": 0.1}),
					Pattern: "X... .... X.X. ...."},
				{Instrument: "snare", Volume: 0.85, ReverbSend: 0.15,
					SynthParams: P(map[string]float64{"drive": 0.25, "tone": 0.2}),
					Pattern:     ".... X... .... X...", Pitches: nil},
				{Instrument: "hihat", Volume: 0.4, Pan: 0.25,
					SynthParams: P(map[string]float64{"brightness": 0.2}),
					Pattern:     "X.X. X.X. X.X. X.X."},
				{Instrument: "crash", Volume: 0.6, Pattern: "X... .... .... ...."},
				{Instrument: "bass-guitar", Volume: 0.9,
					Pattern: "X... .... X.X. ....", Pitches: []float64{0, 0, 7}},
			},
		},
		// ── Hip-Hop — 88 BPM, swung boom-bap ──
		{
			Genre: "hip-hop", Display: "Hip-Hop", BPM: 88, Subdiv: 16, Bars: 1,
			Rows: []Row{
				{Instrument: "kick-deep", Volume: 1.0, SynthParams: P(map[string]float64{"drive": 0.15}),
					Pattern: "X... ..X. ..X. ...."},
				{Instrument: "snare", Volume: 0.8, ReverbSend: 0.1,
					SynthParams: P(map[string]float64{"tone": -0.1}),
					Pattern:     ".... X... .... X..."},
				{Instrument: "clap", Volume: 0.5, Pattern: ".... X... .... X..."},
				{Instrument: "hihat", Volume: 0.35,
					SynthParams: P(map[string]float64{"brightness": -0.1}),
					Pattern:     "XxXx XxXx XxXx XxXx"},
				{Instrument: "sub-bass", Volume: 0.9,
					Pattern: "X... ..X. ..X. ....", Pitches: []float64{0, 0, -5}},
			},
		},
		// ── Pop — 120 BPM, bright & hooky ──
		{
			Genre: "pop", Display: "Pop", BPM: 120, Subdiv: 16, Bars: 1,
			Rows: []Row{
				{Instrument: "kick-punchy", Volume: 1.0, Pattern: "X... .... X... X..."},
				{Instrument: "clap", Volume: 0.7, ReverbSend: 0.2, Pattern: ".... X... .... X..."},
				{Instrument: "hihat", Volume: 0.4, SynthParams: P(map[string]float64{"brightness": 0.15}),
					Pattern: "X.X. X.X. X.X. X.X."},
				{Instrument: "fm-pluck", Volume: 0.7, Pan: 0.2, DelaySend: 0.15,
					Pattern: "X.X. X.X. X.X. X.X.", Pitches: []float64{0, 3, 5, 7, 7, 5, 3, 0}},
				{Instrument: "sub-bass", Volume: 0.85,
					Pattern: "X... X... X... X...", Pitches: []float64{0, 0, 0, 0}},
			},
		},
		// ── Funk — 105 BPM, syncopated, "on the one" ──
		{
			Genre: "funk", Display: "Funk", BPM: 105, Subdiv: 16, Bars: 1,
			Rows: []Row{
				{Instrument: "kick-tight", Volume: 1.0, Pattern: "X... ...X ..X. ...."},
				{Instrument: "snare", Volume: 0.85, SynthParams: P(map[string]float64{"drive": 0.2, "tone": 0.15}),
					Pattern: ".... X... .... X..."},
				{Instrument: "snare-ghost", Volume: 0.5, Pattern: "...g ..g. g... ...g"},
				{Instrument: "hihat", Volume: 0.4, Pan: 0.2, Pattern: "X.Xp X.X. X.Xp X.X."},
				{Instrument: "bass-guitar", Volume: 0.9, SynthParams: P(map[string]float64{"drive": 0.15}),
					Pattern: "X..X ..XX ..X. ..X.", Pitches: []float64{0, 7, 0, 3, 0, 5}},
			},
		},
		// ── Salsa — 190 BPM feel, 2-bar son clave (2-3) ──
		{
			Genre: "salsa", Display: "Salsa", BPM: 190, Subdiv: 16, Bars: 2,
			Rows: []Row{
				{Instrument: "cowbell", Volume: 0.7, Pan: 0.3,
					SynthParams: P(map[string]float64{"brightness": 0.2, "tone": 0.1}),
					Pattern:     "X... X... X... X... X... X... X... X..."},
				{Instrument: "sidestick", Volume: 0.7,
					Pattern: "..X. ..X. .... X... .... X.X. .... ...."},
				{Instrument: "tom", Volume: 0.65, ReverbSend: 0.15,
					Pattern: "..X. X..X ..X. X..X ..X. X..X ..X. X..X",
					Pitches: []float64{5, 0, -5, 5, 0, -5, 5, 0, -5, 5, 0, -5}},
				{Instrument: "fm-epiano", Volume: 0.6, Pan: -0.2, ReverbSend: 0.2,
					Pattern: ".... ..X. .... ..X. .... ..X. .... ..X.",
					Pitches: []float64{0, 4, 7, 4}},
				{Instrument: "bass-guitar", Volume: 0.9,
					Pattern: ".... ..X. .... X... .... ..X. .... X...",
					Pitches: []float64{0, -5, 0, -5}},
				{Instrument: "shaker", Volume: 0.4,
					Pattern: "X.X. X.X. X.X. X.X. X.X. X.X. X.X. X.X."},
			},
		},
		// ── House — 124 BPM, four-on-the-floor ──
		{
			Genre: "house", Display: "House", BPM: 124, Subdiv: 16, Bars: 1,
			Rows: []Row{
				{Instrument: "kick", Volume: 1.0, SynthParams: P(map[string]float64{"body": 0.2}),
					Pattern: "X... X... X... X..."},
				{Instrument: "clap", Volume: 0.7, ReverbSend: 0.2, Pattern: ".... X... .... X..."},
				{Instrument: "hihat", Volume: 0.35, Pattern: "X.X. X.X. X.X. X.X."},
				{Instrument: "hihat-1", Volume: 0.4, SynthParams: P(map[string]float64{"decay": 1.8}),
					Pattern: "..X. ..X. ..X. ..X."},
				{Instrument: "fm-bass", Volume: 0.85,
					Pattern: "..X. ..X. ..X. ..X.", Pitches: []float64{0, 0, 12, 0}},
				{Instrument: "fm-epiano", Volume: 0.55, Pan: -0.15,
					Pattern: "..X. .... ..X. ....", Pitches: []float64{0, 5}},
			},
		},
		// ── Techno — 132 BPM, driving & evolving ──
		{
			Genre: "techno", Display: "Techno", BPM: 132, Subdiv: 16, Bars: 1,
			Rows: []Row{
				{Instrument: "kick-punchy", Volume: 1.0, SynthParams: P(map[string]float64{"body": 0.25, "drive": 0.1}),
					Pattern: "X... X... X... X..."},
				{Instrument: "clap", Volume: 0.6, ReverbSend: 0.1, Pattern: ".... .... .... X..."},
				{Instrument: "hihat", Volume: 0.35, Pattern: "X.Xp X.X. X.Xp X.X."},
				{Instrument: "hihat-1", Volume: 0.4, SynthParams: P(map[string]float64{"decay": 1.8}),
					Pattern: "..X. ..X. ..X. ..X."},
				{Instrument: "fm-bass", Volume: 0.85, SynthParams: P(map[string]float64{"drive": 0.3, "tone": 0.2}),
					Pattern: "X.X. X.X. X.X. X.X.", Pitches: []float64{0, 0, 12, 0, 0, 7, 0, 0}},
				{Instrument: "modular", Volume: 0.5, DelaySend: 0.25, Pattern: ".... .... X... ...."},
			},
		},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/templates/ -v`
Expected: PASS (all templates tests, incl. the three TestSpecs_*).

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/templates/genres.go src/go/internal/templates/genres_test.go
git commit -m "feat(templates): seven genre circuit specs (rock/hip-hop/pop/funk/salsa/house/techno)"
```

---

## Task 5: Generator command + emit committed JSON

**Files:**
- Create: `src/go/cmd/gen-templates/main.go`
- Modify: `Makefile`
- Create (generated, committed): `src/go/internal/assets/templates/*.json`

The writer mirrors `cmd/gen-chain-spec`: pure, no Ebiten. It writes one file per genre into `internal/assets/templates/`.

- [ ] **Step 1: Write the generator**

```go
//go:build !test && !js

// gen-templates writes the seven built-in genre circuit templates as JSON into
// internal/assets/templates/. Source of truth: internal/templates/genres.go.
// A drift test (internal/assets/templates_test.go) regenerates and diffs so a
// commit cannot land with stale template JSON. Mirrors cmd/gen-chain-spec.
//
// Usage:
//
//	cd src/go && go run ./cmd/gen-templates
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ingyamilmolinar/beatmo/internal/templates"
)

func main() {
	outDir := filepath.Join("internal", "assets", "templates")
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "gen-templates: mkdir: %v\n", err)
		os.Exit(1)
	}
	for _, s := range templates.Specs() {
		data, err := templates.Generate(s)
		if err != nil {
			fmt.Fprintf(os.Stderr, "gen-templates: generate %s: %v\n", s.Genre, err)
			os.Exit(1)
		}
		path := filepath.Join(outDir, s.Genre+".json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "gen-templates: write %s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s (%d bytes)\n", path, len(data))
	}
}
```

- [ ] **Step 2: Add the Makefile target**

Find the `gen-design-tokens` target in `Makefile` and add this target nearby (match the file's existing recipe style; use `.tools/go/bin/go`):

```makefile
.PHONY: gen-templates
gen-templates: ## Generate built-in genre circuit templates (internal/templates -> internal/assets/templates/*.json)
	cd src/go && ../../.tools/go/bin/go run ./cmd/gen-templates
```

- [ ] **Step 3: Run the generator to emit the committed JSON**

Run: `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-templates`
Expected: prints `wrote internal/assets/templates/rock.json (...)` for all seven genres.

- [ ] **Step 4: Verify the generated files parse as JSON**

Run: `cd src/go && for f in internal/assets/templates/*.json; do python3 -c "import json; json.load(open('$f')); print('$f ok')"; done`
Expected: seven `... ok` lines (each file is valid JSON).

- [ ] **Step 5: Commit (generator + generated files)**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/cmd/gen-templates/main.go Makefile src/go/internal/assets/templates/
git commit -m "feat(templates): gen-templates command + committed genre JSON"
```

---

## Task 6: Asset registry (`assets.Templates()`)

**Files:**
- Create: `src/go/internal/assets/templates.go`
- Test: `src/go/internal/assets/templates_test.go` (registry portion)

Embeds the committed JSON and exposes ordered metadata for the menu.

- [ ] **Step 1: Write the failing test**

```go
package assets

import (
	"encoding/json"
	"testing"
)

func TestTemplates_OrderedAndNonEmpty(t *testing.T) {
	tpls := Templates()
	wantGenres := []string{"rock", "hip-hop", "pop", "funk", "salsa", "house", "techno"}
	if len(tpls) != len(wantGenres) {
		t.Fatalf("got %d templates, want %d", len(tpls), len(wantGenres))
	}
	for i, tp := range tpls {
		if tp.Genre != wantGenres[i] {
			t.Fatalf("template %d genre=%q want %q", i, tp.Genre, wantGenres[i])
		}
		if tp.Display == "" || tp.BPM <= 0 {
			t.Fatalf("template %q missing display/bpm: %+v", tp.Genre, tp)
		}
		var doc struct {
			Version int `json:"version"`
		}
		if err := json.Unmarshal(tp.Bytes, &doc); err != nil {
			t.Fatalf("template %q bytes not JSON: %v", tp.Genre, err)
		}
		if doc.Version != 1 {
			t.Fatalf("template %q version=%d want 1", tp.Genre, doc.Version)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/ -run TestTemplates_Ordered -v`
Expected: FAIL (undefined: Templates).

- [ ] **Step 3: Write minimal implementation**

```go
package assets

import "embed"

//go:embed templates/*.json
var templateFS embed.FS

// Template is a built-in genre circuit: display metadata + the importable JSON.
type Template struct {
	Genre   string
	Display string
	BPM     int
	Bytes   []byte
}

// templateOrder is the canonical menu order + display metadata. It mirrors
// internal/templates.Specs(); the drift test asserts the JSON matches.
var templateOrder = []struct {
	Genre, Display string
	BPM            int
}{
	{"rock", "Rock", 120},
	{"hip-hop", "Hip-Hop", 88},
	{"pop", "Pop", 120},
	{"funk", "Funk", 105},
	{"salsa", "Salsa", 190},
	{"house", "House", 124},
	{"techno", "Techno", 132},
}

// Templates returns the built-in genre circuits in menu order. Panics at init
// only if an embedded file is missing — a build-time guarantee, never runtime.
func Templates() []Template {
	out := make([]Template, 0, len(templateOrder))
	for _, m := range templateOrder {
		b, err := templateFS.ReadFile("templates/" + m.Genre + ".json")
		if err != nil {
			panic("assets: missing embedded template " + m.Genre + ".json: " + err.Error())
		}
		out = append(out, Template{Genre: m.Genre, Display: m.Display, BPM: m.BPM, Bytes: b})
	}
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/ -run TestTemplates_Ordered -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/assets/templates.go src/go/internal/assets/templates_test.go
git commit -m "feat(templates): embed + Templates() registry"
```

---

## Task 7: Drift test (committed == regenerated)

**Files:**
- Modify: `src/go/internal/assets/templates_test.go` (append)

Guarantees the committed JSON is exactly what the generator produces, so the generator stays the single source of truth.

- [ ] **Step 1: Write the failing test**

```go
import "github.com/ingyamilmolinar/beatmo/internal/templates"

func TestTemplates_NoDrift(t *testing.T) {
	byGenre := map[string][]byte{}
	for _, tp := range Templates() {
		byGenre[tp.Genre] = tp.Bytes
	}
	for _, s := range templates.Specs() {
		want, err := templates.Generate(s)
		if err != nil {
			t.Fatalf("%s: generate: %v", s.Genre, err)
		}
		got, ok := byGenre[s.Genre]
		if !ok {
			t.Fatalf("%s: no embedded template", s.Genre)
		}
		if string(got) != string(want) {
			t.Fatalf("%s: committed JSON is stale — run `make gen-templates` and commit", s.Genre)
		}
	}
}
```

Merge the new `import` into the existing import block at the top of `templates_test.go`.

- [ ] **Step 2: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/assets/ -run TestTemplates -v`
Expected: PASS (both registry + drift). If drift FAILS, run `make gen-templates` and re-commit the JSON.

- [ ] **Step 3: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/assets/templates_test.go
git commit -m "test(templates): drift guard — committed JSON matches generator"
```

---

## Task 8: Round-trip import test (all templates)

**Files:**
- Create: `src/go/internal/ui/template_import_test.go`

Imports every committed template into a real `Game` and asserts structural + musical invariants: row count, instrument ids, per-row hit counts, loop length in beats, melodic pitches survive, and synth_params apply. This is the proof the JSON is correct and importable.

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/assets"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	"github.com/ingyamilmolinar/beatmo/internal/templates"
)

// numHits counts firing glyphs in a spec row at its declared length.
func numHits(t *testing.T, genre string, r templates.Row, bars int) int {
	t.Helper()
	hits, err := templatesParseForTest(r, bars)
	if err != nil {
		t.Fatalf("%s: parse %q: %v", genre, r.Instrument, err)
	}
	return hits
}

func TestTemplates_ImportRoundTrip(t *testing.T) {
	assertDefaultParityState(t)
	specByGenre := map[string]templates.Spec{}
	for _, s := range templates.Specs() {
		specByGenre[s.Genre] = s
	}
	for _, tp := range assets.Templates() {
		tp := tp
		t.Run(tp.Genre, func(t *testing.T) {
			g := New(testLogger)
			t.Cleanup(g.CloseForTest)
			g.Layout(640, 480)
			if err := g.Import(tp.Bytes); err != nil {
				t.Fatalf("%s: import: %v", tp.Genre, err)
			}
			g.updateBeatInfos()

			spec := specByGenre[tp.Genre]
			if len(g.drum.Rows) != len(spec.Rows) {
				t.Fatalf("%s: rows=%d want %d", tp.Genre, len(g.drum.Rows), len(spec.Rows))
			}
			// Row 0 origin promoted to StartNodeID.
			if g.graph.StartNodeID == model.InvalidNodeID {
				t.Fatalf("%s: StartNodeID unset after import", tp.Genre)
			}
			maxDiv := g.grid.MaxDiv()
			for i, specRow := range spec.Rows {
				row := g.drum.Rows[i]
				if row.Instrument != specRow.Instrument {
					t.Fatalf("%s row %d: instrument=%q want %q", tp.Genre, i, row.Instrument, specRow.Instrument)
				}
				path := g.beatInfosByRow[i]
				if len(path) == 0 {
					t.Fatalf("%s row %d (%s): empty beat path", tp.Genre, i, row.Instrument)
				}
				// Loop length in beats == bars*4 (4/4). One loop = rawBeatLen.
				loopLen := rawBeatLenForTest(path)
				if beats := loopLen / maxDiv; beats != spec.Bars*4 {
					t.Fatalf("%s row %d (%s): loop=%d beats want %d", tp.Genre, i, row.Instrument, beats, spec.Bars*4)
				}
				// Distinct regular firing nodes in one loop == hit count.
				want := numHits(t, tp.Genre, specRow, spec.Bars)
				got := countRegularNodesForTest(path[:loopLen])
				if got != want {
					t.Fatalf("%s row %d (%s): %d firing nodes want %d", tp.Genre, i, row.Instrument, got, want)
				}
			}
			// Melodic rows: at least one node carries a non-zero pitch.
			for i, specRow := range spec.Rows {
				if len(specRow.Pitches) == 0 {
					continue
				}
				if !anyNonZeroPitchForTest(g, i) {
					t.Fatalf("%s row %d (%s): expected per-node pitch, found none", tp.Genre, i, specRow.Instrument)
				}
			}
			// synth_params applied as an overlay on the instrument.
			for _, specRow := range spec.Rows {
				if len(specRow.SynthParams) == 0 {
					continue
				}
				got := audio.GetInstrumentParams(specRow.Instrument)
				for k, v := range specRow.SynthParams {
					if gv, ok := got[k]; !ok || gv != v {
						t.Fatalf("%s %s: param %s=%v (ok=%v) want %v", tp.Genre, specRow.Instrument, k, gv, ok, v)
					}
				}
			}
		})
	}
}
```

- [ ] **Step 2: Write the small test helpers**

Create the same file's helpers (append to `template_import_test.go`):

```go
// templatesParseForTest returns the hit count for a spec row.
func templatesParseForTest(r templates.Row, bars int) (int, error) {
	hits, err := templates.HitsForTest(r.Pattern, r.Pitches, bars*16)
	if err != nil {
		return 0, err
	}
	return len(hits), nil
}

// rawBeatLenForTest returns the length of one loop in the beat path: the index
// at which the origin node id first repeats (mirrors rawBeatLen in game_beat_path.go).
func rawBeatLenForTest(path []model.BeatInfo) int {
	if len(path) == 0 {
		return 0
	}
	origin := path[0].NodeID
	for i := 1; i < len(path); i++ {
		if path[i].NodeID == origin {
			return i
		}
	}
	return len(path)
}

// countRegularNodesForTest counts distinct regular (firing) node ids in a loop.
func countRegularNodesForTest(loop []model.BeatInfo) int {
	seen := map[model.NodeID]bool{}
	for _, bi := range loop {
		if bi.NodeType == model.NodeTypeRegular && bi.NodeID != model.InvalidNodeID {
			seen[bi.NodeID] = true
		}
	}
	return len(seen)
}

// anyNonZeroPitchForTest reports whether any firing node in row's loop has pitch != 0.
func anyNonZeroPitchForTest(g *Game, row int) bool {
	for _, bi := range g.beatInfosByRow[row] {
		if bi.NodeType != model.NodeTypeRegular || bi.NodeID == model.InvalidNodeID {
			continue
		}
		if mn, ok := g.graph.GetNodeByID(bi.NodeID); ok && mn.Params.Pitch != 0 {
			return true
		}
	}
	return false
}
```

- [ ] **Step 3: Expose `HitsForTest` from the templates package**

Create `src/go/internal/templates/export_test_helpers.go`:

```go
package templates

// HitsForTest exposes parsePattern to external test packages (internal/ui).
// It is not used by production code.
func HitsForTest(pattern string, pitches []float64, totalSteps int) ([]Hit, error) {
	return parsePattern(pattern, pitches, totalSteps)
}
```

- [ ] **Step 4: Run test to verify it fails, then passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestTemplates_ImportRoundTrip -v`
Expected first run may FAIL if any pattern/instrument is off (e.g. wrong hit count, a silent origin on row 0, or an unknown instrument id). Fix the offending genre row in `genres.go`, re-run `make gen-templates`, re-commit JSON, and re-run until PASS for all seven subtests.

If `GetInstrumentParams` shows params NOT applied, the cause is that the import overlay needs a recipe binding: add the row's recipe id to the spec (extend `Row` with a `Recipe string` field, emit it in `Generate`'s `instrument.Recipe`, and set it per row). Re-generate and re-run. (Verify the correct recipe id with `audio.RecipeForInstrument(id)` printed in a scratch test.)

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/template_import_test.go src/go/internal/templates/export_test_helpers.go
git commit -m "test(templates): round-trip import invariants for all seven genres"
```

---

## Task 9: Overflow menu "Load template" entry (page-aware)

**Files:**
- Modify: `src/go/internal/ui/drumview_context_menu.go` (`overflowItems`, reset on close)
- Modify: the `DrumView` struct definition (add `overflowPage int`) — find it with `grep -n "overflowScroll" src/go/internal/ui/*.go` (the struct field block near other overflow fields)
- Test: `src/go/internal/ui/template_menu_test.go`

The overflow menu gains a second page. Page 0 shows File + "Load template"; page 1 shows a Back row + the seven genres. Selecting a genre stashes its bytes into the import queue.

- [ ] **Step 1: Write the failing test**

```go
package ui

import (
	"encoding/json"
	"testing"
)

func TestOverflowMenu_TemplatePageListsGenres(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum

	// Page 0 contains a "Load template" action.
	var loadTemplate *overflowItem
	for i := range dv.overflowItems() {
		if dv.overflowItems()[i].label == "Load template" {
			it := dv.overflowItems()[i]
			loadTemplate = &it
		}
	}
	if loadTemplate == nil {
		t.Fatalf("page 0 missing 'Load template' entry")
	}
	// Activating it switches to the template page.
	loadTemplate.onClick()
	if dv.overflowPage != 1 {
		t.Fatalf("overflowPage=%d want 1 after Load template", dv.overflowPage)
	}
	// Page 1 lists all seven genres by display name + a Back row.
	labels := map[string]bool{}
	var back bool
	for _, it := range dv.overflowItems() {
		labels[it.label] = true
		if it.label == "Back" {
			back = true
		}
	}
	if !back {
		t.Fatalf("template page missing Back row")
	}
	for _, want := range []string{"Rock", "Hip-Hop", "Pop", "Funk", "Salsa", "House", "Techno"} {
		if !labels[want] {
			t.Fatalf("template page missing %q (have %v)", want, labels)
		}
	}
}

func TestOverflowMenu_SelectTemplateQueuesImport(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dv := g.drum
	dv.overflowPage = 1

	// Click the "Rock" entry.
	var rock *overflowItem
	for i := range dv.overflowItems() {
		if dv.overflowItems()[i].label == "Rock" {
			it := dv.overflowItems()[i]
			rock = &it
		}
	}
	if rock == nil {
		t.Fatalf("no Rock entry on template page")
	}
	rock.onClick()

	if len(g.pendingImportData) == 0 {
		t.Fatalf("selecting a template did not queue import data")
	}
	var doc struct {
		Version     int `json:"version"`
		Instruments []struct {
			ID string `json:"id"`
		} `json:"instruments"`
	}
	if err := json.Unmarshal(g.pendingImportData, &doc); err != nil {
		t.Fatalf("queued data not JSON: %v", err)
	}
	if doc.Version != 1 || len(doc.Instruments) == 0 {
		t.Fatalf("queued data not a template: %+v", doc)
	}
	// Selecting resets the page so reopening starts at File.
	if dv.overflowPage != 0 {
		t.Fatalf("overflowPage=%d want 0 after selection", dv.overflowPage)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestOverflowMenu_Template -v`
Expected: FAIL (no `overflowPage` field / no "Load template" entry).

- [ ] **Step 3: Add the `overflowPage` field**

In the `DrumView` struct (near `overflowScroll`/`overflowDeferredTap`), add:

```go
	// overflowPage selects the overflow popup page: 0 = File actions, 1 = the
	// genre template list. Reset to 0 whenever the menu closes or a template is
	// chosen so reopening always starts at the File page.
	overflowPage int
```

- [ ] **Step 4: Make `overflowItems` page-aware**

Replace the body of `overflowItems()` in `drumview_context_menu.go` with:

```go
func (dv *DrumView) overflowItems() []overflowItem {
	if dv.overflowPage == 1 {
		items := []overflowItem{
			{label: "Templates", header: true},
			{label: "Back", iconID: IconChevronLeft, onClick: func() {
				dv.overflowPage = 0
			}},
		}
		for _, tp := range assets.Templates() {
			tp := tp
			items = append(items, overflowItem{
				label:  tp.Display,
				iconID: IconRows,
				onClick: func() {
					dv.overflowPage = 0
					dv.closeOverflowMenu()
					if dv.onImport != nil {
						_ = dv.onImport(tp.Bytes)
					}
				},
			})
		}
		return items
	}
	items := []overflowItem{
		{label: "File", header: true},
		{label: "Upload", iconID: IconUpload, onClick: func() {
			dv.closeOverflowMenu()
			if dv.uploadBtn().OnClick != nil {
				dv.uploadBtn().OnClick()
			}
		}},
		{label: "Import", iconID: IconImport, onClick: func() {
			dv.closeOverflowMenu()
			if dv.importBtn().OnClick != nil {
				dv.importBtn().OnClick()
			}
		}},
		{label: "Export", iconID: IconExport, onClick: func() {
			dv.closeOverflowMenu()
			if dv.exportBtn().OnClick != nil {
				dv.exportBtn().OnClick()
			}
		}},
		{label: "Load template", iconID: IconRows, onClick: func() {
			dv.overflowPage = 1
		}},
	}
	return items
}
```

Add `"github.com/ingyamilmolinar/beatmo/internal/assets"` to the file's imports if not already present.

- [ ] **Step 5: Reset the page when the menu closes**

Find `closeOverflowMenu()` (grep: `grep -n "func (dv \*DrumView) closeOverflowMenu" src/go/internal/ui/*.go`) and add `dv.overflowPage = 0` as the first line of its body so a fresh open always starts on the File page.

- [ ] **Step 6: Run test to verify it passes**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/ui/ -run TestOverflowMenu_Template -v`
Expected: PASS (both tests).

- [ ] **Step 7: Run the full ui package build to confirm no breakage**

Run: `cd src/go && ../../.tools/go/bin/go build -tags test -modfile=go.test.mod ./internal/ui/`
Expected: builds clean.

- [ ] **Step 8: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add src/go/internal/ui/drumview_context_menu.go src/go/internal/ui/template_menu_test.go
# plus the DrumView struct file you edited:
git add -u src/go/internal/ui/
git commit -m "feat(ui): page-aware overflow menu with Load template submenu"
```

---

## Task 10: Full fast-suite gate

**Files:** none (verification task)

- [ ] **Step 1: Run the templates + assets + ui fast tests together**

Run:
```bash
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod \
  ./internal/templates/ ./internal/assets/ ./internal/ui/ 2>&1 | tail -30
```
Expected: `ok` for `internal/templates` and `internal/assets`. For `internal/ui`, the templates tests pass; note that per memory `project_add_core_node_types_preexisting_failures` there may be ~14 pre-existing Synth-tab failures on this branch unrelated to this work — confirm any failures are NOT in `TestTemplates_*` / `TestOverflowMenu_Template`.

- [ ] **Step 2: Confirm the generator drift is clean**

Run: `cd src/go && ../../.tools/go/bin/go run ./cmd/gen-templates && git diff --stat src/go/internal/assets/templates/`
Expected: no diff (committed JSON already matches generator output).

- [ ] **Step 3: Commit any regenerated files (should be none)**

```bash
cd /home/ymolinar/Repos/beatmo
git status --short src/go/internal/assets/templates/
# If clean, nothing to commit.
```

---

## Task 11: Documentation — AGENTS.synth.md + instrument config

**Files:**
- Modify: `AGENTS.synth.instruments.md` (append a "Genre Templates — Instrument Configuration" section)
- Modify: `AGENTS.synth.md` (one-line pointer in the Satellite Files table or Key Source Files)
- Modify: `CLAUDE.md` (one-line pointer)

- [ ] **Step 1: Append the instrument-configuration reference to `AGENTS.synth.instruments.md`**

Add this section at the end of the file (before any trailing `---` if present, else append):

```markdown
---

## Genre Templates — Instrument Configuration

Built-in genre circuits live in `internal/templates` (specs) and ship as JSON in
`internal/assets/templates/`. Each template **selects + tunes existing builtins**
— it never adds a C recipe. Three levers, all set per row in `genres.go`:

1. **Base recipe choice** via the instrument id: `kick-tight` (acoustic),
   `kick-deep` (808), `kick-punchy` (909); `hihat` (closed) vs `hihat-1` (open,
   raised `decay`); `sub-bass`/`fm-bass` for low end; `fm-pluck`/`fm-epiano`/
   `modular` for melodic/texture.
2. **Per-instrument `synth_params`** — the 8 knobs (`pitch decay tone attack
   drive body color brightness`) as effective-deltas; applied on import as a raw
   overlay (works without an explicit `recipe`).
3. **Per-node `pitch` / `duration` / `volume`** for real basslines, montuno,
   hooks, ghosts (low volume), and accents.

Open-hat lanes MUST use a distinct instrument id (`hihat-1`) from the closed hat
(`hihat`) — `synth_params` are keyed by instrument id, so reusing `hihat` would
make the open and closed hats share one decay setting.

Per-genre instrument map:

| Genre | Kick | Snare/Clap | Hats | Low end | Melodic/Color |
|---|---|---|---|---|---|
| Rock | kick-tight (drive+.1) | snare (drive+.25,tone+.2); crash | hihat (brightness+.2) | bass-guitar (root pitch) | — |
| Hip-Hop | kick-deep (drive+.15) | snare (tone-.1); clap | hihat (brightness-.1, swung) | sub-bass (bassline) | — |
| Pop | kick-punchy | clap (+reverb) | hihat (brightness+.15) | sub-bass (root) | fm-pluck (hook) |
| Funk | kick-tight | snare (drive+.2); snare-ghost | hihat (8ths+prob) | bass-guitar (drive+.15, line) | — |
| Salsa | — | sidestick=clave; tom=conga (hi/lo pitch) | shaker (8ths) | bass-guitar (tumbao) | cowbell=campana; fm-epiano=montuno |
| House | kick (body+.2) | clap (+reverb) | hihat; hihat-1 (decay 1.8=open) | fm-bass (bassline) | fm-epiano (stab) |
| Techno | kick-punchy (body+.25,drive+.1) | clap; | hihat (8ths+prob); hihat-1 (open) | fm-bass (drive+.3,tone+.2=acid) | modular (texture) |

Pattern feel follows the rule above (§ Pattern & Feel): hats ride 8ths + accents
+ `probability` fills, never straight 16ths at tempo. The square-loop topology
(distance = time) is documented in `docs/superpowers/specs/2026-06-10-circuit-templates-design.md`.

To change a template's sound: edit the row in `internal/templates/genres.go`,
run `make gen-templates`, commit the regenerated JSON (the drift test enforces it).
```

- [ ] **Step 2: Add the pointer in `AGENTS.synth.md`**

In the "Key Source Files" table, add a row:

```markdown
| Genre templates | `src/go/internal/templates/{genres,layout,pattern}.go`, `cmd/gen-templates`, `internal/assets/templates/*.json` |
```

- [ ] **Step 3: Add the pointer in `CLAUDE.md`**

Under the "Quick Start" table (or near "Regenerate design tokens"), add a row:

```markdown
| Regenerate genre templates | `make gen-templates` (internal/templates → `internal/assets/templates/*.json`; drift-guarded) |
```

- [ ] **Step 4: Verify the docs render (no broken table)**

Run: `cd /home/ymolinar/Repos/beatmo && grep -n "Genre Templates" AGENTS.synth.instruments.md && grep -n "gen-templates" CLAUDE.md AGENTS.synth.md`
Expected: matches in each file.

- [ ] **Step 5: Commit**

```bash
cd /home/ymolinar/Repos/beatmo
git add AGENTS.synth.instruments.md AGENTS.synth.md CLAUDE.md
git commit -m "docs(templates): genre instrument-configuration reference"
```

---

## Self-Review Notes (verified against the spec)

- **Spec coverage:** menu entry (Task 9) ✓; cascading submenu via page-aware menu (Task 9) ✓; load-immediately via `onImport` queue (Task 9) ✓; seven genres as JSON files (Tasks 4–5) ✓; generator (Task 5) ✓; embed registry (Task 6) ✓; drift guard (Task 7) ✓; round-trip + per-genre invariants (Task 8) ✓; instrument selection+tuning, no new C synths (Tasks 4, 11) ✓; per-node pitch melodies (Tasks 4, 8) ✓; AGENTS docs update (Task 11) ✓; TDD throughout (every code task is test-first) ✓.
- **Type consistency:** `Spec`/`Row`/`Hit`/`node` defined in Task 1–3 and reused unchanged in Tasks 4–8; `Generate`, `Specs`, `Templates`, `HitsForTest`, `overflowItem{label,iconID,header,onClick}`, `overflowPage`, `DrumView.onImport`, `Game.pendingImportData`, `g.beatInfosByRow`, `model.BeatInfo{NodeID,NodeType,I,J}`, `g.graph.GetNodeByID` all match the names verified in the source during design.
- **Open question contingencies are inline, not placeholders:** Task 8 Step 4 names the exact remedy (add `Recipe` field) if `synth_params` don't apply without a recipe binding; design evidence (`import.go:561-564`) says they should, so the base plan omits `recipe`.
- **Known external noise:** `internal/ui` has ~14 pre-existing Synth-tab failures on `add-core-node-types` (memory: `project_add_core_node_types_preexisting_failures`) — Task 10 Step 1 says to confirm failures are outside the new `TestTemplates_*`/`TestOverflowMenu_*` tests.
```
