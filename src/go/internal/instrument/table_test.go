package instrument

import (
	"errors"
	"math"
	"strings"
	"testing"
)

func posInf() float64 { return math.Inf(1) }

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
	return Doc{ID: "x", Name: "x", Render: Render{Seconds: 0.5, Amplitude: 1},
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
		"wrong columns":  func(tb *Table) { tb.Cols = []string{"amp", "freq"} },
		"over capacity":  func(tb *Table) { tb.Rows = append(tb.Rows, []float64{1, 0}, []float64{2, 0}, []float64{3, 0}) },
		"ragged row":     func(tb *Table) { tb.Rows[0] = []float64{440} },
		"out of bounds":  func(tb *Table) { tb.Rows[1][1] = 2 },
		"not finite":     func(tb *Table) { tb.Rows[0][0] = posInf() },
		"unknown column": func(tb *Table) { tb.Cols = []string{"freq", "phase"} },
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
	raw, err := MarshalFile(d)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"table"`) {
		t.Fatalf("no table in %s", raw)
	}
	back, err := ParseFile(raw)
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
