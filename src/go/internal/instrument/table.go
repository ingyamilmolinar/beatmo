package instrument

import (
	"fmt"
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
