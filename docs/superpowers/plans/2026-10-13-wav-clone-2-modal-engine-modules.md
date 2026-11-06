# WAV clone — Plan 2: tables in documents + the `modal` / `strike` / `noiseband` engine stages

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the synth engine somewhere for a measured modal model to live: instrument documents may carry bounded TABLES of model parameters, and three new stages — `modal` (≤ 128 phasor resonators), `strike` (contact-force reweighting + click) and `noiseband` (≤ 32 enveloped noise bands) — render them on desktop and in the browser.

**Architecture:** `internal/instrument` learns `Table` (on `Stage`) + `TableDecl` (on `ModuleTypeDecl`): validated, cloned, diffed, hashed, flattened into dense ABI arrays (`modal_freq[0..127]` …) plus a row-count field. `modularFields` gains `Rows` (table column arrays, C `double modal_freq[128];`) and `Derived` (count fields Flatten writes from the table); neither gets a ParamDef — rows are never knobs. The C engine gains `src/c/modal_bank.c`, called right after the gen bank in `render_modular_p`. New scalar knobs are HIDDEN until plan 3 builds their Synth-tab cards.

**Tech Stack:** Go 1.26 (bundled), cgo C engine, emscripten (`/opt/emsdk`) for `src/js/drums.single.js`.

**Spec:** `docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md` (§3, §3a–§3e; §5's storage rules). Plan 1: `docs/superpowers/plans/2026-10-12-wav-clone-1-render-parity-and-analysis.md`.

## Global Constraints

- Repo rule: **never `git add` / `git commit`**; leave everything uncommitted. Touch only the files a task names; run focused `-run` tests (the tree is edited concurrently).
- New ABI rows are APPENDED at the tail of `modularFields` (the table order IS the ABI order; inserting renumbers every later field).
- "0 is a real value": every new field's `Init` is its identity, and a disabled stage is an exact bypass (`modal_enabled` / `strike_enabled` / `noiseband_enabled` Init 0).
- Tables carry model parameters only: Σ capacity × columns ≤ 1,024 values per document, every column has finite physical bounds (`TestTablesAreModelParametersOnly`). No PCM, wavetable or excitation table ever.
- C reads params at full double precision (`TestModularParamsAreNeverNarrowed`); no static mutable state (renders run concurrently); no new lazily built global table.
- No test reads reference audio; all audio in tests is generated.
- After any `modularFields` change: `make gen-synth-abi`, `make build/libdrums.a`, `make src/js/drums.single.js`, re-freeze `testdata/modular_schema_layout.json`.

## Design decisions (deviations from the spec, recorded in the spec in Task 8)

- **Stages, not layers.** The spec made `modal` / `noiseband` gen-SOURCES 11/12. Layer values are per gen slot (`gen<k>_*`) while a table is document-wide, and a capacity-1 layer may only sit in slot 1 — `modal` and `noiseband` would fight for it. As STAGES they are global fields with an enable switch, rendered after the gen bank, need no source id or slot capacity, and show up as ordinary stage cards.
- **Kernel not shared with the cymbal.** `gen_cymbal_modal`'s recursion is six lines that accumulate the SINE part into a float bank and stop at 140 dB; the clone bank accumulates the COSINE part (the `clone.Mode` convention) with per-mode attack and stops 100 dB under its analytic peak bound. Factoring a shared helper would touch the cymbal's float accumulation for no reuse gain; the cymbal goldens stay untouched.
- **Row order = priority.** "Modes: keep the N strongest" keeps the FIRST N rows; the writer (plan 3's Invert) orders rows by energy. Validate does not sort-check (energy depends on the law and the pitch).
- **No `modal` gain knob**: rows are in reference units and the voice is peak-normalised by `renderDocBlock`. `noiseband_level` (dB offset) exists as the one NOISE macro.

## Review Focus

1. **A knob edit on a tabled stage drops the table** — `edit.go`'s `withModule` rebuilds a `Stage` from a `Module`; the table must survive every `SetValue` / `SetOn`. Pinned in Task 1 (`TestSetValueKeepsTheStageTable`).
2. **A document with a table round-trips through JSON / the pack reader** (`ParseFile` uses `DisallowUnknownFields`) and its `ContentHash` changes when a cell changes. Pinned in Task 1 (`TestTableRoundTripsAndHashes`).
3. **A modal row at or above 0.45·sr** (a high mode transposed up) must be skipped, never alias. Pinned in Task 5 (`TestModalSkipsModesAboveTheLimit`).
4. **Concurrent renders of a modal + noiseband document** are byte-identical to serial ones (prewarm renders in parallel). Pinned in Task 5 (`TestModalRendersAreDeterministicAndConcurrent`).
5. **The browser engine refuses a stale `drums.single.js`** after the ABI grows — the rebuilt bundle must report the new count. Pinned in Task 7 (`node` check of `_modular_param_count`).

---

### Task 1: `internal/instrument` — tables

**Files:**
- Create: `src/go/internal/instrument/table.go`, `src/go/internal/instrument/table_test.go`
- Modify: `doc.go` (`Stage.Table`, `Clone`), `registry.go` (`ModuleTypeDecl.Table`, `check`), `flatten.go` (`Flatten`, `OwnedFields`, `eachModule`), `validate.go` (stages), `diff.go` (stages), `edit.go` (`withModule`)

**Interfaces — Produces:**
- `type Table struct{ Cols []string; Rows [][]float64 }` (JSON `cols`, `rows`)
- `type TableColumn struct{ Name, Field string; Min, Max float64 }`, `type TableDecl struct{ Columns []TableColumn; Capacity int; CountField string }`
- `func TableCell(field string, row int) string` → `"modal_freq[3]"`
- `func SetTable(d Doc, reg *Registry, stage string, t Table) (Doc, error)`
- `Stage.Table *Table`, `ModuleTypeDecl.Table *TableDecl`

- [ ] **Step 1: Write the failing tests** (`table_test.go`)

```go
package instrument

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// tableRegistry is a minimal registry with one tabled stage.
func tableRegistry(t *testing.T) *Registry {
	t.Helper()
	fields := []string{"bank_enabled", "bank_gain", "bank_count"}
	for _, col := range []string{"bank_freq", "bank_amp"} {
		for i := 0; i < 4; i++ {
			fields = append(fields, TableCell(col, i))
		}
	}
	reg, err := NewRegistry([]ModuleTypeDecl{{
		Type: "bank", Version: 1, Kind: KindStage,
		Enable: &FieldSet{Field: "bank_enabled", Value: 1},
		Values: []ValueDecl{{Name: "bank_gain", Init: 1}},
		Table: &TableDecl{Capacity: 4, CountField: "bank_count", Columns: []TableColumn{
			{Name: "freq", Field: "bank_freq", Min: 0, Max: 20000},
			{Name: "amp", Field: "bank_amp", Min: -1, Max: 1},
		}},
	}}, fields, 1)
	if err != nil {
		t.Fatal(err)
	}
	return reg
}

func tabledDoc() Doc {
	return Doc{ID: "x", Name: "x", Render: Render{Beats: 1, Amplitude: 1},
		Stages: map[string]Stage{"bank": {V: 1, On: true, Values: Values{"bank_gain": 0.5},
			Table: &Table{Cols: []string{"freq", "amp"}, Rows: [][]float64{{440, 0.5}, {1210, -0.25}}}}}}
}

func TestFlattenWritesTableCellsAndCount(t *testing.T) {
	reg := tableRegistry(t)
	d := tabledDoc()
	if err := Validate(d, reg); err != nil {
		t.Fatal(err)
	}
	b := Flatten(d, reg)
	at := func(f string) float64 { i, _ := reg.FieldIndex(f); return b[i] }
	if at("bank_count") != 2 || at(TableCell("bank_freq", 1)) != 1210 || at(TableCell("bank_amp", 0)) != 0.5 {
		t.Fatalf("block %v", b)
	}
	if at(TableCell("bank_freq", 2)) != 0 || at(TableCell("bank_freq", 3)) != 0 {
		t.Fatal("rows past the count must stay 0")
	}
	off := d.Clone()
	st := off.Stages["bank"]
	st.On = false
	off.Stages["bank"] = st
	for i, v := range Flatten(off, reg) {
		if v != 0 {
			t.Fatalf("an OFF stage wrote field %s = %v", reg.Fields()[i], v)
		}
	}
}

func TestValidateRejectsBadTables(t *testing.T) {
	reg := tableRegistry(t)
	cases := map[string]func(*Table){
		"wrong columns":   func(tb *Table) { tb.Cols = []string{"amp", "freq"} },
		"over capacity":   func(tb *Table) { tb.Rows = append(tb.Rows, []float64{1, 0}, []float64{2, 0}, []float64{3, 0}) },
		"ragged row":      func(tb *Table) { tb.Rows[0] = []float64{440} },
		"out of bounds":   func(tb *Table) { tb.Rows[1][1] = 2 },
		"not finite":      func(tb *Table) { tb.Rows[0][0] = posInf() },
		"unknown column":  func(tb *Table) { tb.Cols = []string{"freq", "phase"} },
	}
	for name, mut := range cases {
		d := tabledDoc()
		mut(d.Stages["bank"].Table)
		if err := Validate(d, reg); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	d := tabledDoc()
	d.Stages["bank"] = Stage{V: 1, On: true, Values: Values{"bank_gain": 1}} // no table = zero rows
	if err := Validate(d, reg); err != nil {
		t.Fatalf("a tabled stage without rows is valid: %v", err)
	}
}

func TestCloneDiffAndSetTable(t *testing.T) {
	reg := tableRegistry(t)
	a := tabledDoc()
	b := a.Clone()
	b.Stages["bank"].Table.Rows[0][0] = 441
	if a.Stages["bank"].Table.Rows[0][0] != 440 {
		t.Fatal("Clone shares the table")
	}
	if got := Diff(a, b); len(got) != 1 || got[0] != "stages.bank.table" {
		t.Fatalf("Diff = %v", got)
	}
	c, err := SetTable(a, reg, "bank", Table{Cols: []string{"freq", "amp"}, Rows: [][]float64{{100, 0.1}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Stages["bank"].Table.Rows) != 1 || len(a.Stages["bank"].Table.Rows) != 2 {
		t.Fatal("SetTable must return a modified copy")
	}
	if _, err := SetTable(a, reg, "bank", Table{Cols: []string{"freq", "amp"}, Rows: [][]float64{{100, 9}}}); err == nil {
		t.Fatal("SetTable accepted an out-of-bounds cell")
	}
	if _, err := SetTable(Doc{ID: "y", Render: a.Render}, reg, "bank", Table{}); !errors.Is(err, ErrNoModule) {
		t.Fatalf("SetTable on a missing stage: %v", err)
	}
}

func TestSetValueKeepsTheStageTable(t *testing.T) {
	reg := tableRegistry(t)
	d, err := SetValue(tabledDoc(), reg, StageRef("bank"), "bank_gain", 0.9)
	if err != nil {
		t.Fatal(err)
	}
	if tb := d.Stages["bank"].Table; tb == nil || len(tb.Rows) != 2 {
		t.Fatalf("SetValue dropped the table: %+v", d.Stages["bank"])
	}
	d, err = SetOn(d, reg, StageRef("bank"), false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Stages["bank"].Table == nil {
		t.Fatal("SetOn dropped the table")
	}
}

func TestTableRoundTripsAndHashes(t *testing.T) {
	d := tabledDoc()
	var buf bytes.Buffer
	if err := MarshalFile(&buf, d); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), `"table"`) {
		t.Fatalf("no table in %s", buf.String())
	}
	back, err := ParseFile(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if len(Diff(d, back)) != 0 {
		t.Fatalf("round trip differs: %v", Diff(d, back))
	}
	e := d.Clone()
	e.Stages["bank"].Table.Rows[1][1] = -0.5
	if ContentHash(d) == ContentHash(e) {
		t.Fatal("ContentHash ignores the table")
	}
}

func TestRegistryRejectsBadTableDecls(t *testing.T) {
	fields := []string{"bank_enabled", "bank_count", TableCell("bank_freq", 0)}
	bad := []TableDecl{
		{Capacity: 2, CountField: "bank_count", Columns: []TableColumn{{Name: "freq", Field: "bank_freq", Min: 0, Max: 1}}}, // cell 1 missing
		{Capacity: 1, CountField: "nope", Columns: []TableColumn{{Name: "freq", Field: "bank_freq", Min: 0, Max: 1}}},
		{Capacity: 1, CountField: "bank_count", Columns: []TableColumn{{Name: "freq", Field: "bank_freq", Min: 1, Max: 0}}},
		{Capacity: 1, CountField: "bank_count"},
	}
	for i, td := range bad {
		td := td
		if _, err := NewRegistry([]ModuleTypeDecl{{Type: "bank", Version: 1, Kind: KindStage,
			Enable: &FieldSet{Field: "bank_enabled", Value: 1}, Table: &td}}, fields, 1); err == nil {
			t.Errorf("decl %d accepted", i)
		}
	}
}
```

(`posInf()` is `math.Inf(1)` — add `func posInf() float64 { return math.Inf(1) }` in the test file with `"math"` imported. Check the real `MarshalFile` / `ParseFile` / `SetOn` signatures in `doc.go` / `edit.go` and adapt the calls, keeping the assertions.)

- [ ] **Step 2: Run to verify they fail**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/instrument/ -run 'Table|SetValueKeeps' -count=1`
Expected: FAIL — `undefined: TableCell` etc.

- [ ] **Step 3: Implement `table.go`**

```go
package instrument

import (
	"fmt"
	"math"
	"slices"
)

// Table is a stage's rows of MODEL PARAMETERS (one modal resonator or one
// noise band per row). Cols names the columns in the declared order; every
// row has one value per column. A table is never audio: the registry bounds
// every column physically and caps the total cells (spec §3a's data rule).
type Table struct {
	Cols []string    `json:"cols"`
	Rows [][]float64 `json:"rows"`
}

func (t *Table) clone() *Table {
	if t == nil {
		return nil
	}
	out := &Table{Cols: slices.Clone(t.Cols), Rows: make([][]float64, len(t.Rows))}
	for i, r := range t.Rows {
		out.Rows[i] = slices.Clone(r)
	}
	return out
}

func tablesEqual(a, b *Table) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if a == nil {
		return true
	}
	if !slices.Equal(a.Cols, b.Cols) || len(a.Rows) != len(b.Rows) {
		return false
	}
	for i := range a.Rows {
		if !slices.Equal(a.Rows[i], b.Rows[i]) {
			return false
		}
	}
	return true
}

// TableColumn declares one column: its document name, the ABI array it
// fills (cells TableCell(Field, 0..Capacity-1)) and its physical bounds.
type TableColumn struct {
	Name     string
	Field    string
	Min, Max float64
}

// TableDecl declares a stage's table. Flatten writes row i, column c into
// TableCell(c.Field, i) and the row count into CountField; cells of rows ≥
// the count stay 0.
type TableDecl struct {
	Columns    []TableColumn
	Capacity   int
	CountField string
}

// TableCell is the ABI field of row `row` of a table column array.
func TableCell(field string, row int) string { return fmt.Sprintf("%s[%d]", field, row) }

// fields lists every ABI field the table writes.
func (td *TableDecl) fields() []string {
	out := []string{td.CountField}
	for _, c := range td.Columns {
		for i := 0; i < td.Capacity; i++ {
			out = append(out, TableCell(c.Field, i))
		}
	}
	return out
}

func (td *TableDecl) check() error {
	if td.Capacity < 1 {
		return fmt.Errorf("table capacity %d < 1", td.Capacity)
	}
	if td.CountField == "" || len(td.Columns) == 0 {
		return fmt.Errorf("a table needs a count field and at least one column")
	}
	seen := map[string]bool{}
	for _, c := range td.Columns {
		if c.Name == "" || c.Field == "" || seen[c.Name] {
			return fmt.Errorf("table column %q: empty or duplicate", c.Name)
		}
		seen[c.Name] = true
		if !finite(c.Min) || !finite(c.Max) || !(c.Min < c.Max) {
			return fmt.Errorf("table column %q: bounds [%v, %v] must be finite with min < max", c.Name, c.Min, c.Max)
		}
	}
	return nil
}

// checkTable validates a stage's table against its declaration (nil = zero
// rows, valid).
func checkTable(bad func(string, ...any), path string, td *TableDecl, tb *Table) {
	if tb == nil {
		return
	}
	if len(tb.Cols) != len(td.Columns) {
		bad("%s.table.cols: %v, want %v", path, tb.Cols, columnNames(td))
		return
	}
	for i, c := range td.Columns {
		if tb.Cols[i] != c.Name {
			bad("%s.table.cols: %v, want %v", path, tb.Cols, columnNames(td))
			return
		}
	}
	if len(tb.Rows) > td.Capacity {
		bad("%s.table.rows: %d rows, capacity %d", path, len(tb.Rows), td.Capacity)
	}
	for r, row := range tb.Rows {
		if len(row) != len(td.Columns) {
			bad("%s.table.rows[%d]: %d values, want %d", path, r, len(row), len(td.Columns))
			continue
		}
		for c, x := range row {
			col := td.Columns[c]
			switch {
			case !finite(x):
				bad("%s.table.rows[%d].%s: not finite", path, r, col.Name)
			case x < col.Min || x > col.Max:
				bad("%s.table.rows[%d].%s: %v outside [%v, %v]", path, r, col.Name, x, col.Min, col.Max)
			}
		}
	}
}

func columnNames(td *TableDecl) []string {
	out := make([]string, len(td.Columns))
	for i, c := range td.Columns {
		out[i] = c.Name
	}
	return out
}

// SetTable replaces the table of the stage `stage` (which must be present)
// and returns the modified copy; the table is validated against the stage's
// declaration first. The only writer of tables.
func SetTable(d Doc, reg *Registry, stage string, t Table) (Doc, error) {
	st, ok := d.Stages[stage]
	if !ok {
		return d, fmt.Errorf("stages.%s: %w", stage, ErrNoModule)
	}
	decl, ok := reg.byType[stage]
	if !ok || decl.Table == nil {
		return d, fmt.Errorf("stages.%s: the type has no table", stage)
	}
	var p []string
	checkTable(func(f string, a ...any) { p = append(p, fmt.Sprintf(f, a...)) }, "stages."+stage, decl.Table, &t)
	if len(p) > 0 {
		return d, &ValidationError{Problems: p}
	}
	out := d.Clone()
	st = out.Stages[stage]
	st.Table = (&t).clone()
	out.Stages[stage] = st
	return out, nil
}

var _ = math.Inf // finite() lives in validate.go
```

- [ ] **Step 4: Wire the table through the existing code**

`doc.go`: add `Table *Table \`json:"table,omitempty"\`` to `Stage` (after `Values`); in `Clone`, `s.Table = s.Table.clone()` next to `s.Values = s.Values.clone()`.

`registry.go`: add `Table *TableDecl // stages only: a bounded table of model parameters` to `ModuleTypeDecl`; in `check`, before the field-existence loop:

```go
	if t.Table != nil {
		if t.Kind != KindStage {
			return fmt.Errorf("only stages may carry a table")
		}
		if err := t.Table.check(); err != nil {
			return err
		}
		all = append(all, t.Table.fields()...)
	}
```

(declare `var all []string` before this block so the existing switch appends to it.)

`flatten.go`: change `eachModule`'s callback to `func(t *ModuleTypeDecl, slot int, on bool, vals Values, tb *Table)` (stages pass `st.Table`, source/layers pass `nil`); in `Flatten` and `OwnedFields` add, after the Enable write:

```go
		if t.Table != nil {
			writeTable(t.Table, tb, put) // Flatten
		}
```

with

```go
// writeTable writes a table's cells and row count through put.
func writeTable(td *TableDecl, tb *Table, put func(string, float64)) {
	n := 0
	if tb != nil {
		n = len(tb.Rows)
		for r, row := range tb.Rows {
			for c, col := range td.Columns {
				put(TableCell(col.Field, r), row[c])
			}
		}
	}
	put(td.CountField, float64(n))
}
```

(`OwnedFields` uses `writeTable(t.Table, tb, func(f string, v float64) { out[f] = v })`.)

`validate.go` stage loop: after `checkModule(...)`:

```go
		switch {
		case t.Table != nil:
			checkTable(bad, path, t.Table, st.Table)
		case st.Table != nil:
			bad("%s.table: %s has no table", path, key)
		}
```

`diff.go` stage loop: after the `diffModule` append, `if !tablesEqual(sa.Table, sb.Table) { add("stages." + k + ".table") }`.

`edit.go` `withModule`, stage branch: keep the existing table —

```go
			prev := out.Stages[ref.Stage] // out is a clone: prev.Table is already a copy
			out.Stages[ref.Stage] = Stage{V: m.V, On: m.On, Values: m.Values.clone(), Notes: m.Notes, Table: prev.Table}
```

- [ ] **Step 5: Run the package**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/instrument/ -count=1`
Expected: PASS (new and existing tests). Leave uncommitted.

---

### Task 2: ABI — table column arrays and derived count fields

**Files:**
- Modify: `src/go/internal/audio/modular_fields.go` (`modularField.Rows`, `.Derived`; new blocks at the tail), `synth_param_schema.go` (schema names + inits for rows; `IsTableField`), `cmd/gen-synth-abi/main.go` (C `double name[R];`, `#define MODULAR_<NAME>_ROWS R`; JS index skips cells and emits `MODULAR_PARAM_TABLES`), `modular_abi_drift_test.go` (`cStructFields` handles a non-`gen_` array as table cells)
- Regenerate: `src/c/modular_params.gen.h`, `src/js/synth_param_abi.gen.js`, `internal/audio/testdata/modular_schema_layout.json`

**Interfaces — Produces:** schema names `modal_freq[0]..modal_freq[127]` etc.; `func IsTableField(name string) bool` (a table cell or a `Derived` count); C arrays `p->modal_freq[i]`, `MODULAR_MODAL_FREQ_ROWS`.

New rows (appended after the `kickphys` block, in this order):

```go
	// ── WAV-clone stages (spec 2026-10-12): MODAL resonator bank, STRIKE,
	// NOISEBAND. Scalars are the stages' knobs (hidden until the Synth-tab
	// cards land); Rows columns are TABLE cells written by Flatten from the
	// stage's table (never knobs); Derived rows are the table's row count. ──
	{Block: "modal", Name: "modal_enabled", Init: 0, Doc: "0=off (exact bypass) 1=on"},
	{Block: "modal", Name: "modal_freq_mode", Init: 0, Doc: "0=freq is a ratio to the voice frequency 1=freq is Hz"},
	{Block: "modal", Name: "modal_tune", Init: 0, Doc: "cents"},
	{Block: "modal", Name: "modal_stretch", Init: 0, Doc: "f' = f*(f/ref)^s; ref = voice freq (ratio mode) or 1 kHz"},
	{Block: "modal", Name: "modal_decay_x", Init: 1, Doc: "multiplies every mode's decay rate"},
	{Block: "modal", Name: "modal_bright", Init: 0, Doc: "dB/oct tilt re 1 kHz on the mode amplitudes"},
	{Block: "modal", Name: "modal_keep", Init: 128, Doc: "plays the first N rows"},
	{Block: "modal", Name: "modal_law_a", Init: 1, Doc: "decay law a*(f/1k)^p + b*(f/10k)^6, Np/s; a row's decay is a multiplier of it"},
	{Block: "modal", Name: "modal_law_p", Init: 0, Doc: "decay law exponent"},
	{Block: "modal", Name: "modal_law_b", Init: 0, Doc: "decay law high-frequency term, Np/s at 10 kHz"},
	{Block: "modal", Name: "modal_count", Init: 0, Derived: true, Doc: "rows in the table (written by Flatten)"},
	{Block: "modal", Name: "modal_freq", Rows: 128, Init: 0, Doc: "ratio or Hz"},
	{Block: "modal", Name: "modal_decay", Rows: 128, Init: 0, Doc: "multiplier of the decay law at the mode's frequency"},
	{Block: "modal", Name: "modal_amp", Rows: 128, Init: 0, Doc: "amplitude divided by the strike spectrum at the fitted contact"},
	{Block: "modal", Name: "modal_phase", Rows: 128, Init: 0, Doc: "radians, cosine phase at the onset"},
	{Block: "modal", Name: "modal_attack", Rows: 128, Init: 0, Doc: "seconds of raised-cosine onset"},
	{Block: "strike", Name: "strike_enabled", Init: 0, Doc: "0=off 1=on (reweights the modal amplitudes)"},
	{Block: "strike", Name: "strike_tc", Init: 0, Doc: "raised-cosine contact time, seconds (0: an ideal impulse)"},
	{Block: "strike", Name: "strike_hardness", Init: 0, Doc: "dB/oct tilt re 1 kHz"},
	{Block: "strike", Name: "strike_click", Init: 0, Doc: "contact click level relative to the bank's peak bound"},
	{Block: "noiseband", Name: "noiseband_enabled", Init: 0, Doc: "0=off (exact bypass) 1=on"},
	{Block: "noiseband", Name: "noiseband_level", Init: 0, Doc: "dB offset on every band"},
	{Block: "noiseband", Name: "noiseband_count", Init: 0, Derived: true, Doc: "rows in the table (written by Flatten)"},
	{Block: "noiseband", Name: "nb_center", Rows: 32, Init: 0, Doc: "Hz"},
	{Block: "noiseband", Name: "nb_q", Rows: 32, Init: 0, Doc: "band-pass Q"},
	{Block: "noiseband", Name: "nb_level", Rows: 32, Init: 0, Doc: "dB RMS at the end of the attack"},
	{Block: "noiseband", Name: "nb_attack", Rows: 32, Init: 0, Doc: "seconds of raised-cosine onset"},
	{Block: "noiseband", Name: "nb_fast_rate", Rows: 32, Init: 0, Doc: "Np/s"},
	{Block: "noiseband", Name: "nb_fast_mix", Rows: 32, Init: 0, Doc: "0..1 share of the fast decay"},
	{Block: "noiseband", Name: "nb_tail_rate", Rows: 32, Init: 0, Doc: "Np/s"},
```

- [ ] **Step 1: Failing test** — in `synth_param_schema_test.go` (test-tag file) add:

```go
func TestTableColumnsFlattenToCells(t *testing.T) {
	idx := ModularParamSchemaIndexMap()
	a, okA := idx["modal_freq[0]"]
	b, okB := idx["modal_freq[127]"]
	if !okA || !okB || b-a != 127 {
		t.Fatalf("modal_freq cells: [0]=%d,%v [127]=%d,%v", a, okA, b, okB)
	}
	if !IsTableField("modal_freq[5]") || !IsTableField("modal_count") || IsTableField("modal_tune") {
		t.Fatal("IsTableField misclassifies")
	}
	if _, ok := idx["modal_freq"]; ok {
		t.Fatal("a table column must not appear as a scalar")
	}
}
```

Run: `go test -tags test -modfile=go.test.mod ./internal/audio/ -run TestTableColumnsFlattenToCells` → FAIL (`undefined: IsTableField`).

- [ ] **Step 2: Implement.** `modularField` gets `Rows int` and `Derived bool` (doc comment: "Rows == R: a table column array `Name[R]`, flattened to `Name[0]..Name[R-1]` — a stage TABLE's cells, written by Flatten, never a knob. Derived: written by Flatten from a table (its row count), never a knob."). `modularParamSchema` and `modularFieldInits`: a `Rows > 0` row emits `instrument.TableCell(f.Name, i)` for i < Rows (import `internal/instrument`; check for an import cycle — `instrument` imports nothing from `audio`). Add:

```go
// IsTableField reports whether a schema field is written by Flatten from a
// stage table — a cell of a table column, or a table's row count. Such fields
// carry no ParamDef: rows are never knobs.
func IsTableField(name string) bool { return tableFields[name] }

var tableFields = func() map[string]bool {
	m := map[string]bool{}
	for _, f := range modularFields {
		switch {
		case f.Rows > 0:
			for i := 0; i < f.Rows; i++ {
				m[instrument.TableCell(f.Name, i)] = true
			}
		case f.Derived:
			m[f.Name] = true
		}
	}
	return m
}()
```

`ModularFieldsForCodegen` also exports `Rows`. `cmd/gen-synth-abi`: C decl for `Rows > 0` is `double <name>[<Rows>];` plus `#define MODULAR_<UPPER(name)>_ROWS <Rows>` emitted with the slot defines; JS: `MODULAR_PARAM_INDEX` lists only non-cell fields, and a new

```js
// MODULAR_PARAM_TABLES maps a table column to [first index, rows].
export const MODULAR_PARAM_TABLES = Object.freeze({
  modal_freq: [<idx>, 128],
  ...
});
```

`cStructFields` (`modular_abi_drift_test.go`): an array whose name does not start with `gen_` and whose bound is a number flattens to `name[0]..name[N-1]`.

- [ ] **Step 3: Regenerate + re-freeze**

```
make gen-synth-abi
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run 'TestModularFieldsReproduceTheShippedLayout' -count=1 -v
```

The layout test fails with the appended rows; regenerate its golden with the test's documented update path (read `modular_fields_test.go:15` — if it has no update flag, rewrite `testdata/modular_schema_layout.json` from `ModularParamSchema()` + `modularFieldInits()` with a one-off `go run` helper kept OUT of the tree). Then:

Run: `go test -tags test -modfile=go.test.mod ./internal/audio/ -run 'TestTableColumnsFlattenToCells|TestModularFieldsReproduceTheShippedLayout|TestSynthParamABIGenFileIsCurrent|TestModularParamsSchemaMatchesCStruct|TestFieldDocsDescribeNoSubstitutedDefaults|TestOnlyTheModularParamBlockCrossesTheABI' -count=1`
Expected: PASS. (`TestModularSynthParamDefs_ShapeAndDefaults` is fixed in Task 3.) Leave uncommitted.

---

### Task 3: Module declarations, ParamDefs, data-rule guard

**Files:**
- Modify: `src/go/internal/audio/module_decls.go` (three stage decls), `modular_recipe.go` (scalar ParamDefs for the three blocks, hidden; skip table fields), `modular_recipe_test.go` (shape test compares against the scalar schema), `testdata/dead_module_values.txt` (`modal.*`, `strike.*`, `noiseband.*` — no shipped instrument yet; liveness is pinned by Task 5's own test)
- Create: `src/go/internal/audio/module_tables_test.go`

**Interfaces — Produces:** registry stage types `modal`, `strike`, `noiseband`; column names `freq, decay, amp, phase, attack` and `center, q, level, attack, fast_rate, fast_mix, tail_rate`.

Declarations (append to `builtinModuleDecls()` beside the other stages):

```go
	{Type: "modal", Kind: st, Enable: declSwitch("modal_enabled", 1),
		Values: declVals("modal_freq_mode", "modal_tune", "modal_stretch", "modal_decay_x", "modal_bright",
			"modal_keep", "modal_law_a", "modal_law_p", "modal_law_b"),
		Table: &instrument.TableDecl{Capacity: 128, CountField: "modal_count", Columns: []instrument.TableColumn{
			{Name: "freq", Field: "modal_freq", Min: 0, Max: 24000},
			{Name: "decay", Field: "modal_decay", Min: 0, Max: 10000},
			{Name: "amp", Field: "modal_amp", Min: -1000, Max: 1000},
			{Name: "phase", Field: "modal_phase", Min: -7, Max: 7},
			{Name: "attack", Field: "modal_attack", Min: 0, Max: 1},
		}}},
	{Type: "strike", Kind: st, Enable: declSwitch("strike_enabled", 1),
		Values: declVals("strike_tc", "strike_hardness", "strike_click")},
	{Type: "noiseband", Kind: st, Enable: declSwitch("noiseband_enabled", 1),
		Values: declVals("noiseband_level"),
		Table: &instrument.TableDecl{Capacity: 32, CountField: "noiseband_count", Columns: []instrument.TableColumn{
			{Name: "center", Field: "nb_center", Min: 10, Max: 24000},
			{Name: "q", Field: "nb_q", Min: 0.1, Max: 100},
			{Name: "level", Field: "nb_level", Min: -200, Max: 24},
			{Name: "attack", Field: "nb_attack", Min: 0, Max: 1},
			{Name: "fast_rate", Field: "nb_fast_rate", Min: 0, Max: 5000},
			{Name: "fast_mix", Field: "nb_fast_mix", Min: 0, Max: 1},
			{Name: "tail_rate", Field: "nb_tail_rate", Min: 0, Max: 5000},
		}}},
```

(`modal_freq_mode` gets `Allowed: {0, 1}` — set it the way other selector values are declared in `module_decls.go`.)

ParamDefs (tail of `ModularSynthParamDefs`, mirroring the blocks; the three blocks' scalar rows only):

```go
	// ── WAV-clone stages (MODAL / STRIKE / NOISEBAND). Hidden until the
	// Synth-tab cards land (plan 3); table cells and row counts get no
	// ParamDef at all — rows are never knobs. Order mirrors modularFields. ──
	cloneBounds := map[string][2]float64{
		"modal_enabled": {0, 1}, "modal_freq_mode": {0, 1}, "modal_tune": {-1200, 1200},
		"modal_stretch": {-0.5, 0.5}, "modal_decay_x": {0.05, 20}, "modal_bright": {-24, 24},
		"modal_keep": {0, 128}, "modal_law_a": {0, 1000}, "modal_law_p": {-2, 4}, "modal_law_b": {0, 100000},
		"strike_enabled": {0, 1}, "strike_tc": {0, 0.05}, "strike_hardness": {-24, 24}, "strike_click": {0, 4},
		"noiseband_enabled": {0, 1}, "noiseband_level": {-60, 24},
	}
	for _, block := range []string{"modal", "strike", "noiseband"} {
		for _, f := range fieldsOfBlock(block) {
			if f.Rows > 0 || f.Derived {
				continue
			}
			b := cloneBounds[f.Name]
			defs = append(defs, ParamDef{Name: f.Name, Label: f.Name, Group: SynthHiddenGroup,
				Min: b[0], Max: b[1], Default: f.Init})
		}
	}
```

`TestModularSynthParamDefs_ShapeAndDefaults`: compare `defs` with the schema filtered by `!IsTableField(name)`.

- [ ] **Step 1: Failing guard** (`module_tables_test.go`, `-tags test` OK):

```go
package audio

import (
	"math"
	"testing"
)

// TestTablesAreModelParametersOnly: a document can never carry a waveform —
// the registry's tables are bounded in size and every column in value.
func TestTablesAreModelParametersOnly(t *testing.T) {
	reg := BuiltinModuleRegistry()
	cells := 0
	tabled := 0
	for _, decl := range reg.Types() {
		if decl.Table == nil {
			continue
		}
		tabled++
		cells += decl.Table.Capacity * len(decl.Table.Columns)
		for _, c := range decl.Table.Columns {
			if math.IsInf(c.Min, 0) || math.IsInf(c.Max, 0) || !(c.Min < c.Max) {
				t.Errorf("%s.%s: bounds [%v, %v] are not physical", decl.Type, c.Name, c.Min, c.Max)
			}
		}
	}
	if tabled != 2 {
		t.Fatalf("%d tabled stage types, want modal + noiseband", tabled)
	}
	if cells > 1024 {
		t.Fatalf("tables hold %d values per document; the data rule caps them at 1024", cells)
	}
}
```

Run → FAIL (`tabled stage types 0`).

- [ ] **Step 2: Implement** the decls, ParamDefs, shape-test filter and dead-value entries:

```
# ── WAV-clone stages: no shipped instrument yet (liveness: TestModalStagesAreLive) ──
modal.* no shipped instrument is a clone yet
strike.* no shipped instrument is a clone yet
noiseband.* no shipped instrument is a clone yet
```

- [ ] **Step 3: Run**

Run: `cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/audio/ -run 'TestTablesAreModelParametersOnly|TestModularSynthParamDefs|TestEveryModuleValueHasAParamDef|Registry|TestShippedPackValidates|TestFieldDocs' -count=1`
Expected: PASS. Leave uncommitted.

---

### Task 4: C engine — `modal_bank.c`

**Files:**
- Create: `src/c/modal_bank.h`, `src/c/modal_bank.c`
- Modify: `src/c/modular.c` (call after `modular_gen_bank_render`), `Makefile` (`C_SRC`, `C_OBJS`, `C_HDRS`, `clean`), `src/go/internal/audio/modular_abi_drift_test.go` (add `modal_bank.c` to the `-Wfloat-conversion` list)

- [ ] **Step 1: `modal_bank.h`**

```c
#ifndef MODAL_BANK_H
#define MODAL_BANK_H

#include "modular_params.gen.h"

/* WAV-clone stages (spec docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md).
 * Both ADD into out (render_modular_p zeroes it first) and are exact no-ops
 * while their stage is disabled. */

/* MODAL (+ STRIKE): up to MODULAR_MODAL_FREQ_ROWS phasor resonators,
 * x_i(t) = A_i e^{-alpha_i t} cos(2 pi f_i t + phi_i), with f_i, alpha_i and
 * A_i evaluated at the mode's ACTUAL frequency (ratio x voice freq, tune,
 * stretch): alpha_i = decay_i * decay_x * (a (f/1k)^p + b (f/10k)^6), and
 * A_i = amp_i * bright tilt * (strike on: S(f; tc) * hardness tilt). */
void modular_modal_render(float *out, int sampleRate, int samples,
                          const modular_params *p, double voice_freq, unsigned int seed);

/* NOISEBAND: up to MODULAR_NB_CENTER_ROWS band-passed noise rows sharing one
 * seeded source, each with a raised-cosine attack then a two-rate decay; a
 * row's level is its RMS in dB after the attack. */
void modular_noiseband_render(float *out, int sampleRate, int samples,
                              const modular_params *p, unsigned int seed);

/* |F(f)| of the force 0.5(1 - cos(2 pi t/tc)) on [0, tc], 1 at DC. */
double modal_strike_spectrum(double f, double tc);

#endif
```

- [ ] **Step 2: `modal_bank.c`**

```c
#include <math.h>
#include <stdlib.h>
#include "modal_bank.h"
#include "modular_stages.h" /* mod_biquad */
#include "noise.h"

#ifndef M_PI
#define M_PI 3.14159265358979323846
#endif

double modal_strike_spectrum(double f, double tc) {
    double x = f * tc;
    if (x == 0.0) return 1.0;
    double d = 1.0 - x * x;
    if (fabs(d) < 1e-9) return 0.5;
    return fabs(sin(M_PI * x) / (M_PI * x) / d);
}

static int clamp_count(double v, int cap) {
    long n = lrint(v);
    if (n < 0) return 0;
    if (n > cap) return cap;
    return (int)n;
}

void modular_modal_render(float *out, int sampleRate, int samples,
                          const modular_params *p, double voice_freq, unsigned int seed) {
    if (lrint(p->modal_enabled) == 0 || samples <= 0) return;
    int count = clamp_count(p->modal_count, MODULAR_MODAL_FREQ_ROWS);
    int keep = clamp_count(p->modal_keep, MODULAR_MODAL_FREQ_ROWS);
    if (keep < count) count = keep;
    const double sr = (double)sampleRate;
    const int ratio = lrint(p->modal_freq_mode) == 0;
    const double ref = ratio ? voice_freq : 1000.0;
    const double limit = fmin(20000.0, 0.45 * sr);
    const double tune = pow(2.0, p->modal_tune / 1200.0);
    const int strike = lrint(p->strike_enabled) != 0;

    double fr[MODULAR_MODAL_FREQ_ROWS], al[MODULAR_MODAL_FREQ_ROWS], am[MODULAR_MODAL_FREQ_ROWS];
    double bound = 0.0;
    for (int i = 0; i < count; ++i) {
        am[i] = 0.0;
        double f = p->modal_freq[i] * (ratio ? voice_freq : 1.0);
        if (!(f > 0.0)) continue;
        if (p->modal_stretch != 0.0) f *= pow(f / ref, p->modal_stretch);
        f *= tune;
        if (!(f < limit)) continue;
        double law = p->modal_law_a * pow(f / 1000.0, p->modal_law_p)
                   + p->modal_law_b * pow(f / 10000.0, 6.0);
        double alpha = p->modal_decay[i] * p->modal_decay_x * law;
        if (alpha < 0.0) alpha = 0.0;
        double oct = log2(f / 1000.0);
        double a = p->modal_amp[i];
        if (p->modal_bright != 0.0) a *= pow(10.0, p->modal_bright * oct / 20.0);
        if (strike)
            a *= modal_strike_spectrum(f, p->strike_tc) * pow(10.0, p->strike_hardness * oct / 20.0);
        fr[i] = f; al[i] = alpha; am[i] = a;
        bound += fabs(a);
    }
    if (bound > 0.0) {
        const double stop = bound * 1e-5; /* 100 dB under the bank's peak bound */
        for (int i = 0; i < count; ++i) {
            double a = am[i];
            if (fabs(a) <= stop) continue;
            int nLive = samples;
            if (al[i] > 0.0) {
                double t = log(fabs(a) / stop) / al[i] * sr + 1.0;
                if (t < (double)samples) nLive = (int)t;
            }
            double dec = exp(-al[i] / sr);
            double w = 2.0 * M_PI * fr[i] / sr;
            double cr = dec * cos(w), ci = dec * sin(w);
            double re = a * cos(p->modal_phase[i]), im = a * sin(p->modal_phase[i]);
            int att = (int)(p->modal_attack[i] * sr);
            if (att > nLive) att = nLive;
            for (int j = 0; j < nLive; ++j) {
                double v = re;
                if (j < att) v *= 0.5 * (1.0 - cos(M_PI * (double)j / (double)att));
                out[j] += (float)v;
                double nr = re * cr - im * ci;
                im = re * ci + im * cr;
                re = nr;
            }
        }
    }
    if (strike && p->strike_click > 0.0 && bound > 0.0) {
        int nc = (int)(fmax(p->strike_tc, 0.001) * sr);
        if (nc > samples) nc = samples;
        noise_ma_gen g;
        noise_ma_init(&g, (int)(seed ^ 0x5157u));
        double amp = p->strike_click * bound;
        for (int j = 0; j < nc; ++j) {
            double win = 0.5 * (1.0 - cos(2.0 * M_PI * (double)j / (double)nc));
            out[j] += (float)(amp * win * (double)noise_ma_white_tick(&g));
        }
    }
}

void modular_noiseband_render(float *out, int sampleRate, int samples,
                              const modular_params *p, unsigned int seed) {
    if (lrint(p->noiseband_enabled) == 0 || samples <= 0) return;
    int count = clamp_count(p->noiseband_count, MODULAR_NB_CENTER_ROWS);
    if (count == 0) return;
    const double sr = (double)sampleRate;
    float *src = (float *)malloc((size_t)samples * sizeof(float));
    if (!src) return;
    noise_ma_white_fill(src, samples, (int)(seed ^ 0x6e62u));
    /* white uniform [-1,1): sigma = 1/sqrt(3). An RBJ 0 dB-peak band-pass passes
     * a noise-equivalent bandwidth pi*fc/(2Q) of the sr/2 band: output RMS =
     * sigma*sqrt(pi*fc/(Q*sr)). Scale so a row's RMS is its level. */
    const double sigma = 1.0 / sqrt(3.0);
    const double offset = pow(10.0, p->noiseband_level / 20.0);
    for (int r = 0; r < count; ++r) {
        double fc = p->nb_center[r], q = p->nb_q[r];
        if (!(fc > 0.0) || !(q > 0.0) || !(fc < 0.49 * sr)) continue;
        mod_biquad bq;
        mod_biquad_set(&bq, 2, fc, q, sampleRate);
        double scale = offset * pow(10.0, p->nb_level[r] / 20.0) / (sigma * sqrt(M_PI * fc / (q * sr)));
        int att = (int)(p->nb_attack[r] * sr);
        double mix = fmin(fmax(p->nb_fast_mix[r], 0.0), 1.0);
        double dF = exp(-p->nb_fast_rate[r] / sr), dT = exp(-p->nb_tail_rate[r] / sr);
        double eF = mix, eT = 1.0 - mix;
        for (int j = 0; j < samples; ++j) {
            double s = (double)mod_biquad_tick_f(&bq, src[j]);
            double env;
            if (j < att) {
                env = 0.5 * (1.0 - cos(M_PI * (double)j / (double)att));
            } else {
                env = eF + eT;
                eF *= dF;
                eT *= dT;
            }
            out[j] += (float)(s * env * scale);
        }
    }
    free(src);
}
```

(Check `MODULAR_MODAL_FREQ_ROWS` / `MODULAR_NB_CENTER_ROWS` are the names Task 2's generator emits; `noise.h`'s `noise_ma_white_fill` signature; that `mod_biquad_set` takes a `double` cutoff and q.)

- [ ] **Step 3: Call it** — `modular.c`, right after `modular_gen_bank_render(...)`:

```c
    /* ── WAV-clone stages: MODAL (+ STRIKE) resonators and NOISEBAND rows add
     * into the mix after the gen bank, PRE-envelope/filter like every source.
     * Disabled (the default) ⇒ exact no-op. ── */
    modular_modal_render(out, sampleRate, samples, p, freq, noise_seed);
    modular_noiseband_render(out, sampleRate, samples, p, noise_seed);
```

and `#include "modal_bank.h"`. Makefile: add `modal_bank` to `C_SRC`, `build/modal_bank.o` to `C_OBJS`, `src/c/modal_bank.h` to `C_HDRS`, the object to `clean`.

- [ ] **Step 4: Build and run the ABI guards**

```
make build/libdrums.a
cd src/go && ../../.tools/go/bin/go test ./internal/audio -run 'TestLinkedEngineMatchesTheSchema|TestModularParamsAreNeverNarrowed|TestModularIdentityGolden|TestSynthGoldenByteIdentity|TestZeroIsARealValue' -count=1
```

Expected: PASS (no shipped instrument enables the new stages, so every golden holds). Leave uncommitted.

---

### Task 5: Real-C tests — the bank does what the model says

**Files:**
- Create: `src/go/internal/audio/modal_bank_test.go` (`//go:build !test && !js`)

**Interfaces — Consumes:** `instrument.SetTable`, `BuiltinModuleRegistry`, `renderModularBlock`, `instrument.Flatten`.

- [ ] **Step 1: Write the tests**

```go
//go:build !test && !js

package audio

import (
	"math"
	"sync"
	"testing"

	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

type testMode struct{ f, alpha, amp, phase float64 }

// modalDoc builds a minimal document whose sound is exactly modes (Hz mode,
// decay law a=1 p=0 b=0 so a row's decay IS its alpha, strike off).
func modalDoc(t *testing.T, modes []testMode) instrument.Doc {
	t.Helper()
	reg := BuiltinModuleRegistry()
	d := instrument.Doc{ID: "modal-test", Name: "modal-test", Render: instrument.Render{Beats: 2, Amplitude: 1},
		Stages: map[string]instrument.Stage{}}
	for _, typ := range []string{"finish"} {
		m, err := instrument.NewModule(reg, typ, true)
		if err != nil {
			t.Fatal(err)
		}
		d.Stages[typ] = instrument.Stage{V: m.V, On: true, Values: m.Values}
	}
	m, err := instrument.NewModule(reg, "modal", true)
	if err != nil {
		t.Fatal(err)
	}
	m.Values["modal_freq_mode"] = 1
	d.Stages["modal"] = instrument.Stage{V: m.V, On: true, Values: m.Values}
	tb := instrument.Table{Cols: []string{"freq", "decay", "amp", "phase", "attack"}}
	for _, md := range modes {
		tb.Rows = append(tb.Rows, []float64{md.f, md.alpha, md.amp, md.phase, 0})
	}
	d, err = instrument.SetTable(d, reg, "modal", tb)
	if err != nil {
		t.Fatal(err)
	}
	if err := instrument.Validate(d, reg); err != nil {
		t.Fatal(err)
	}
	return d
}

func renderDocRaw(d instrument.Doc, sr, n int) []float32 {
	buf := make([]float32, n)
	renderModularBlock(buf, sr, n, modularBlock(instrument.Flatten(d, BuiltinModuleRegistry())))
	return buf
}

// TestModalBankMatchesTheModeModel: the C bank renders Σ A e^{-αt} cos(2πft+φ)
// — the clone analysis' Mode convention — to float precision.
func TestModalBankMatchesTheModeModel(t *testing.T) {
	modes := []testMode{{440, 2, 0.4, 0.3}, {1210, 4.5, 0.2, -1.1}, {2380, 9, 0.12, 2}, {3950, 16, 0.06, 0.7}}
	const sr, n = 48000, 48000
	got := renderDocRaw(modalDoc(t, modes), sr, n)
	var worst float64
	for j := 0; j < n; j++ {
		tt := float64(j) / sr
		var want float64
		for _, m := range modes {
			want += m.amp * math.Exp(-m.alpha*tt) * math.Cos(2*math.Pi*m.f*tt+m.phase)
		}
		worst = math.Max(worst, math.Abs(float64(got[j])-want))
	}
	if worst > 1e-5 {
		t.Fatalf("bank deviates from the mode model by %.3g", worst)
	}
}

func TestModalStagesAreExactBypassesWhenOff(t *testing.T) {
	d := modalDoc(t, []testMode{{440, 2, 0.4, 0}})
	off := d.Clone()
	st := off.Stages["modal"]
	st.On = false
	off.Stages["modal"] = st
	for _, v := range renderDocRaw(off, 48000, 4800) {
		if v != 0 {
			t.Fatal("a disabled modal stage made sound")
		}
	}
}

// TestStrikeAtIdentityIsBitIdentical: strike ON with tc=0, hardness 0, click 0
// leaves the bank untouched (the identity rows are stored against).
func TestStrikeAtIdentityIsBitIdentical(t *testing.T) {
	reg := BuiltinModuleRegistry()
	d := modalDoc(t, []testMode{{440, 2, 0.4, 0.3}, {2000, 8, 0.1, 1}})
	m, err := instrument.NewModule(reg, "strike", true)
	if err != nil {
		t.Fatal(err)
	}
	with := d.Clone()
	with.Stages["strike"] = instrument.Stage{V: m.V, On: true, Values: m.Values}
	a, b := renderDocRaw(d, 48000, 9600), renderDocRaw(with, 48000, 9600)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("identity strike moved sample %d", i)
		}
	}
	with.Stages["strike"].Values["strike_tc"] = 0.002
	c := renderDocRaw(with, 48000, 9600)
	same := true
	for i := range a {
		if a[i] != c[i] {
			same = false
			break
		}
	}
	if same {
		t.Fatal("a 2 ms contact changed nothing")
	}
}

func TestModalSkipsModesAboveTheLimit(t *testing.T) {
	d := modalDoc(t, []testMode{{22000, 2, 0.4, 0}}) // above 0.45·48k
	for _, v := range renderDocRaw(d, 48000, 4800) {
		if v != 0 {
			t.Fatal("a mode above 0.45·sr was rendered (would alias)")
		}
	}
}

// TestModalTranspositionFollowsTheDecayLaw: in ratio mode a higher note puts
// every mode higher AND, through the law, makes it ring shorter.
func TestModalTranspositionFollowsTheDecayLaw(t *testing.T) {
	d := modalDoc(t, []testMode{{1, 1, 0.5, 0}})
	vals := d.Stages["modal"].Values
	vals["modal_freq_mode"] = 0
	vals["modal_law_a"] = 2
	vals["modal_law_p"] = 1
	energyAfter := func(pitch float64) float64 {
		b := instrument.Flatten(d, BuiltinModuleRegistry())
		b[pitchFieldIndex()] = pitch
		buf := make([]float32, 48000)
		renderModularBlock(buf, 48000, 48000, modularBlock(b))
		var e float64
		for _, v := range buf[24000:] {
			e += float64(v) * float64(v)
		}
		return e
	}
	if !(energyAfter(12) < energyAfter(0)) {
		t.Fatal("an octave up must decay faster under a rising law")
	}
}

// TestNoisebandRowLevelIsItsRMS: one row with no decay renders at its level.
func TestNoisebandRowLevelIsItsRMS(t *testing.T) {
	reg := BuiltinModuleRegistry()
	d := modalDoc(t, nil)
	m, err := instrument.NewModule(reg, "noiseband", true)
	if err != nil {
		t.Fatal(err)
	}
	d.Stages["noiseband"] = instrument.Stage{V: m.V, On: true, Values: m.Values}
	d, err = instrument.SetTable(d, reg, "noiseband", instrument.Table{
		Cols: []string{"center", "q", "level", "attack", "fast_rate", "fast_mix", "tail_rate"},
		Rows: [][]float64{{2000, 4.318, -20, 0, 0, 0, 0}},
	})
	if err != nil {
		t.Fatal(err)
	}
	buf := renderDocRaw(d, 48000, 96000)
	var s float64
	for _, v := range buf[4800:] {
		s += float64(v) * float64(v)
	}
	rmsDB := 10 * math.Log10(s/float64(len(buf)-4800))
	if math.Abs(rmsDB-(-20)) > 0.75 {
		t.Fatalf("row at −20 dB renders at %.2f dB RMS", rmsDB)
	}
}

func TestModalRendersAreDeterministicAndConcurrent(t *testing.T) {
	d := modalDoc(t, []testMode{{440, 2, 0.4, 0.3}, {1210, 4.5, 0.2, -1.1}})
	reg := BuiltinModuleRegistry()
	nb, _ := instrument.NewModule(reg, "noiseband", true)
	d.Stages["noiseband"] = instrument.Stage{V: nb.V, On: true, Values: nb.Values}
	d, err := instrument.SetTable(d, reg, "noiseband", instrument.Table{
		Cols: []string{"center", "q", "level", "attack", "fast_rate", "fast_mix", "tail_rate"},
		Rows: [][]float64{{3000, 4, -30, 0.002, 60, 0.7, 6}},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := renderDocRaw(d, 48000, 24000)
	var wg sync.WaitGroup
	errs := make(chan string, 8)
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got := renderDocRaw(d, 48000, 24000)
			for i := range got {
				if got[i] != want[i] {
					errs <- "concurrent render differs"
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Fatal(e)
	}
}

// TestModalStagesAreLive: every declared value and every table column of the
// three stages moves the render in a context where its stage plays (the
// shipped-instrument liveness test cannot see them yet — no clone ships).
func TestModalStagesAreLive(t *testing.T) {
	reg := BuiltinModuleRegistry()
	base := modalDoc(t, []testMode{{440, 2, 0.4, 0.3}, {1210, 4.5, 0.2, -1.1}, {2380, 9, 0.12, 2}})
	sk, _ := instrument.NewModule(reg, "strike", true)
	base.Stages["strike"] = instrument.Stage{V: sk.V, On: true, Values: sk.Values}
	base.Stages["strike"].Values["strike_tc"] = 0.001
	nb, _ := instrument.NewModule(reg, "noiseband", true)
	base.Stages["noiseband"] = instrument.Stage{V: nb.V, On: true, Values: nb.Values}
	base, err := instrument.SetTable(base, reg, "noiseband", instrument.Table{
		Cols: []string{"center", "q", "level", "attack", "fast_rate", "fast_mix", "tail_rate"},
		Rows: [][]float64{{3000, 4, -30, 0.002, 60, 0.7, 6}},
	})
	if err != nil {
		t.Fatal(err)
	}
	ref := renderDocRaw(base, 48000, 24000)
	moved := func(d instrument.Doc) bool {
		got := renderDocRaw(d, 48000, 24000)
		for i := range got {
			if got[i] != ref[i] {
				return true
			}
		}
		return false
	}
	alt := map[string]float64{
		"modal_tune": 50, "modal_stretch": 0.05, "modal_decay_x": 2, "modal_bright": 3, "modal_keep": 1,
		"modal_law_a": 2, "modal_law_p": 1, "modal_law_b": 50, "modal_freq_mode": 0,
		"strike_tc": 0.003, "strike_hardness": 3, "strike_click": 0.5, "noiseband_level": 6,
	}
	for _, typ := range []string{"modal", "strike", "noiseband"} {
		decl, _ := reg.Type(typ)
		for _, v := range decl.Values {
			x, ok := alt[v.Name]
			if !ok {
				t.Fatalf("no perturbation for %s.%s", typ, v.Name)
			}
			d := base.Clone()
			d.Stages[typ].Values[v.Name] = x
			if !moved(d) {
				t.Errorf("%s.%s = %v changes nothing", typ, v.Name, x)
			}
		}
		if decl.Table == nil {
			continue
		}
		for c, col := range decl.Table.Columns {
			d := base.Clone()
			row := d.Stages[typ].Table.Rows[0]
			nv := row[c]*1.1 + 0.001
			if nv > col.Max {
				nv = row[c] * 0.9
			}
			row[c] = nv
			if !moved(d) {
				t.Errorf("%s.table.%s changes nothing", typ, col.Name)
			}
		}
	}
}

func BenchmarkModalBank128x3s(b *testing.B) {
	var modes []testMode
	for i := 0; i < 128; i++ {
		modes = append(modes, testMode{f: 100 + 120*float64(i), alpha: 1 + 0.3*float64(i), amp: 0.01, phase: float64(i)})
	}
	t := &testing.T{}
	d := modalDoc(t, modes)
	block := modularBlock(instrument.Flatten(d, BuiltinModuleRegistry()))
	buf := make([]float32, 3*48000)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		renderModularBlock(buf, 48000, len(buf), block)
	}
}
```

(Check `NewModule`'s real signature and that it returns `Module` with `Values` filled from Inits; if `finish` is not `Required`-only, keep whatever minimal stage set `Validate` demands — read `TestShippedPackValidates`' docs for the required stages.)

- [ ] **Step 2: Run them**

Run: `make build/libdrums.a && cd src/go && ../../.tools/go/bin/go test ./internal/audio -run 'Modal|Strike|Noiseband' -count=1 -v && ../../.tools/go/bin/go test ./internal/audio -run XXX -bench BenchmarkModalBank -benchtime 10x`
Expected: PASS; the benchmark reports ns/op — spec target 20–40 ms for 128 modes × 3 s; record the number in the spec (Task 8). Leave uncommitted.

---

### Task 6: Dispatch hash cost

**Files:**
- Test: `src/go/internal/audio/voice_key_bench_test.go` (`!test && !js`)

- [ ] **Step 1:** benchmark `liveVoiceKey` for a melodic shipped instrument (pitched path → `BlockHashWith` over the whole block):

```go
//go:build !test && !js

package audio

import "testing"

func BenchmarkLiveVoiceKeyPitched(b *testing.B) {
	id := "marimba"
	if _, ok := EnsureInstrumentDoc(id); !ok {
		b.Skip("no marimba document")
	}
	for i := 0; i < b.N; i++ {
		liveVoiceKey(id, 120, 48000, float64(i%24), true, false)
	}
}
```

- [ ] **Step 2:** Run: `go test ./internal/audio -run XXX -bench LiveVoiceKeyPitched -benchtime 2000x`. If ns/op > 20 µs, cache the FNV state up to the pitch index in `docBlockForDispatch` and finish from there (equal hash by construction, guarded by the existing `TestPrewarmVoiceParamsKeyMatchesDispatch`); otherwise record the number in the spec and change nothing. Leave uncommitted.

---

### Task 7: Browser — regenerated ABI and the rebuilt engine

**Files:**
- Regenerate: `src/js/synth_param_abi.gen.js` (Task 2), `src/js/drums.single.js`
- Check: `src/js/audio.js` (any use of `MODULAR_PARAM_INDEX` that iterates fields)

- [ ] **Step 1:** `grep -n MODULAR_PARAM_INDEX src/js/*.js` — every use must index scalars only (table cells are not in the index any more); fix any loop that assumed every field is listed.
- [ ] **Step 2:** `make src/js/drums.single.js` (emscripten at `/opt/emsdk`; source `/opt/emsdk/emsdk_env.sh` first if `emcc` is not on PATH).
- [ ] **Step 3:** Verify the bundle's count (Review Focus 5):

```
node -e "const m=require('./src/js/drums.single.js'); (m.default||m)().then(x=>console.log(x._modular_param_count()))"
```

(adapt to the bundle's module format — read the top of `drums.single.js`; the printed count must equal `MODULAR_PARAM_COUNT` in the generated JS).
- [ ] **Step 4:** Run the bridge smoke: `GO=$(pwd)/.tools/go/bin/go node src/js/wasm_bridge_smoke.browser.test.js` — PASS. Leave uncommitted.

---

### Task 8: Docs and spec

**Files:**
- Modify: `AGENTS.synth.md` (a "WAV-clone stages" section: MODAL/STRIKE/NOISEBAND semantics, tables in documents, the data rule, `SetTable` as the only writer), `docs/superpowers/specs/2026-10-12-wav-clone-modal-design.md` (§3: stages not layers, no kernel share, row order = priority, measured bank cost and hash cost), `CLAUDE.md` (one gotcha line: "Instrument documents may carry TABLES (`Stage.Table`) …")

- [ ] **Step 1:** write the docs; **Step 2:** final verification:

```
cd src/go && ../../.tools/go/bin/go test -tags test -modfile=go.test.mod ./internal/instrument/ ./internal/audio/ -count=1
cd src/go && ../../.tools/go/bin/go test ./internal/audio/ -count=1     # real C, full package
```

Expected: PASS. Leave uncommitted.

## Self-review

- Spec §3a tables → Task 1 (+2, 3); §3b modal → 2–5; §3c strike → 2–5; §3d noiseband → 2–5; §3e ABI/cost/platforms → 2, 4, 5 (bench), 6, 7; §3f Synth tab → plan 3 (knobs hidden here, by decision).
- Placeholder scan: the "check the real signature" notes name the exact function and file to read; no TBDs.
- Type consistency: `Table`, `TableDecl`, `TableColumn`, `TableCell`, `SetTable`, `IsTableField`, field names (`modal_*`, `strike_*`, `noiseband_*`, `nb_*`) are used identically across tasks.

## Execution notes (2026-10-13, plan 2 executed inline)

All tasks done; deviations from the draft above, each forced by a test:

- Task 1: `MarshalFile(d) ([]byte, error)` (not a writer) — test adapted.
- Task 2: the generator-freshness tests panic until `build/libdrums.a` is
  rebuilt (the audio package checks the linked C count at init) — rebuild
  right after `make gen-synth-abi`. The layout pin was re-frozen by APPENDING
  the 882 new entries (the 439-entry prefix is byte-identical).
- Task 3: `TestEveryABIFieldIsDeclared` learned table cells + counts;
  `TestFamilyFieldsHaveTheirFamilyCapacity`'s block budget now counts
  non-table fields only (table cells are capped by
  `TestTablesAreModelParametersOnly`), raised 450 → 470 for the 16 new knobs
  (455 today). `modal_freq_mode` is a plain value (an `Allowed` set would make
  it a type selector).
- Task 4: the Makefile object list appears twice (`C_OBJS`, `clean`) — edit
  both by full line.
- Task 5: the test document also needs the required `core` stage;
  `modalDoc` takes `testing.TB` so the benchmark shares it. Measured: bank vs
  the closed-form mode model 7.7e-6 (≈ −92 dB), a −20 dB noise row renders at
  −20.14 dB RMS, 128 modes × 3 s = 8.6 ms.
- Task 6: `liveVoiceKey` (pitched) = 11.3 µs per trigger — under the 20 µs
  bar, unchanged.
- Task 7: audio.js's ABI self-check now also bounds `MODULAR_PARAM_TABLES`.
  `wasm_bridge_smoke` PASS (368 exports); `xplat_audio_compare` PASS with every
  `render_modular_p` case bit-exact; its one warning (built-in `modular`
  voice, last 512 samples) is Go's `applyFadeOut` window that the browser's
  raw render does not apply — pre-existing and untouched by this work.
- Real-C suite additions: `TestRenderModularP_EveryParamMutatesInContext`
  got a clone-stage context (3 modal rows, strike, 1 noise band) and targets
  for all 16 knobs; `stageEffectivelyActive` (`TestP5_…`) got the
  MODAL / STRIKE / NOISEBAND rules mirroring `modal_bank.c`.
