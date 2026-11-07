package instrument

// A zero block where each present ON module writes its values, its type's Set/Global constants and its Enable field.
// Nothing else: no identity, no defaults. d must be valid.
func Flatten(d Doc, reg *Registry) []float64 {
	b := make([]float64, len(reg.fields))
	put := func(f string, v float64) {
		i, ok := reg.index[f]
		if !ok {
			panic("instrument: Flatten of an unvalidated document: field " + f)
		}
		b[i] = v
	}
	eachModule(d, reg, func(t *ModuleTypeDecl, slot int, on bool, vals Values, tb *Table) {
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
		if t.Table != nil {
			writeTable(t.Table, tb, put)
		}
	})
	return b
}

// writeTable writes a table's cells and its row count through put.
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

// ON or OFF: an enable field reads Enable.Value when on, 0 when off. The bridge to the flat-field world.
func OwnedFields(d Doc, reg *Registry) map[string]float64 {
	out := map[string]float64{}
	eachModule(d, reg, func(t *ModuleTypeDecl, slot int, on bool, vals Values, tb *Table) {
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
		if t.Table != nil {
			writeTable(t.Table, tb, func(f string, v float64) { out[f] = v })
		}
	})
	return out
}

// The UI's flat view: every field's creation value overlaid by OwnedFields. The engine never reads it.
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
func eachModule(d Doc, reg *Registry, fn func(t *ModuleTypeDecl, slot int, on bool, vals Values, tb *Table)) {
	for _, key := range sortedKeys(d.Stages) {
		st := d.Stages[key]
		fn(reg.byType[key], -1, st.On, st.Values, st.Table)
	}
	if m := d.Source; m != nil {
		fn(reg.byType[m.Type], -1, m.On, m.Values, nil)
	}
	for i, m := range d.Layers {
		if m != nil {
			fn(reg.byType[m.Type], i, m.On, m.Values, nil)
		}
	}
}

func slotName(slot int) func(string) string {
	if slot < 0 {
		return func(f string) string { return f }
	}
	return func(f string) string { return LayerField(slot, f) }
}
