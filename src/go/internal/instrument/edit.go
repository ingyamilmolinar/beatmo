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
			prev := out.Stages[ref.Stage] // out is a clone: prev.Table is already a copy
			out.Stages[ref.Stage] = Stage{V: m.V, On: m.On, Values: m.Values.clone(), Notes: m.Notes, Table: prev.Table}
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
		if len(out.Layers) == 0 {
			out.Layers = nil
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
			if sv, ok := cur.setValue(field); ok && sv == v {
				return true, d, nil
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
	for _, u := range layers {
		if _, ok := u.value(name); ok {
			return d, fmt.Errorf("%s: %w", LayerField(slot, name), ErrInactive)
		}
		if _, ok := u.setValue(name); ok {
			return d, fmt.Errorf("%s: %w", LayerField(slot, name), ErrInactive)
		}
	}
	return d, fmt.Errorf("%q: %w", LayerField(slot, name), ErrUnknownField)
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
