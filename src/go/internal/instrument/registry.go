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
	Table    *TableDecl // stages only: a bounded table of model parameters
	Steps    []Step     // len == Version-1
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
			switch t.Kind {
			case KindLayer:
				for s := 0; s < slots; s++ {
					f := LayerField(s, v.Name)
					if _, ok := r.index[f]; ok {
						if _, set := r.init[f]; !set {
							r.init[f] = v.Init
						}
					}
				}
			case KindStage, KindSource:
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
	var all []string
	if t.Table != nil {
		if t.Kind != KindStage {
			return fmt.Errorf("only stages may carry a table")
		}
		if err := t.Table.check(); err != nil {
			return err
		}
		all = append(all, t.Table.fields()...)
	}
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
		if _, ok := r.index[f]; !ok {
			return fmt.Errorf("field %q is not in the ABI", f)
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
