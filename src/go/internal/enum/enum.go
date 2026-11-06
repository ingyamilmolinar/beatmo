// Package enum is the one spelling table behind every closed enum: a Go int
// type whose values are indexes into a Names table. The type's own methods
// (String, MarshalText, UnmarshalText, Parse) are one-liners over the table,
// so the spellings live in exactly one place and an unknown spelling read
// from data is an error, never a new value.
//
//	type Role int
//	const (RoleNone Role = iota; RoleKick)
//	var roleNames = enum.Names[Role]{Kind: "kit role", Names: []string{"", "kick"}}
//	func (r Role) String() string               { return enum.String(roleNames, r) }
//	func (r Role) MarshalText() ([]byte, error) { return enum.Marshal(roleNames, r) }
//	func (r *Role) UnmarshalText(b []byte) error { return enum.Unmarshal(roleNames, b, r) }
//
// An empty name at index 0 makes the zero value the enum's "none": it is what
// an absent or empty field decodes to. Without it the empty spelling is
// rejected like any other unknown one.
//
// The operations are generic FUNCTIONS, not methods on Names, on purpose:
// Ebiten links text/template, whose reflect.Value.MethodByName puts the Go
// linker in a mode that keeps every exported method of every type that ever
// reaches an interface. As methods, each enum's Names[E] carried six method
// wrappers (and six more for *Names[E]) into main.wasm that nothing calls --
// ~58 KB of code over 15 enums. A function instantiates once per GC shape
// (one body for every ~int enum) with the same per-type dictionary a method
// call used, so type safety and the call cost are unchanged.
// TestNamesHasNoMethods keeps it that way.
package enum

import "fmt"

// Names is a closed enum's spelling table: Names[i] spells the value E(i).
// It deliberately has no methods; see the package comment.
type Names[E ~int] struct {
	Kind  string // what the enum names, for errors ("kit role")
	Names []string
}

// Valid reports whether e is one of the enum's values.
func Valid[E ~int](n Names[E], e E) bool { return int(e) >= 0 && int(e) < len(n.Names) }

// String spells e (fmt.Stringer); an out-of-range value spells itself so a
// bug stays visible in a log line.
func String[E ~int](n Names[E], e E) string {
	if !Valid(n, e) {
		return fmt.Sprintf("%s(%d)", n.Kind, int(e))
	}
	return n.Names[e]
}

// Parse reads a spelling; anything not in the table is an error.
func Parse[E ~int](n Names[E], s string) (E, error) {
	for i, name := range n.Names {
		if name == s {
			return E(i), nil
		}
	}
	return 0, fmt.Errorf("unknown %s %q", n.Kind, s)
}

// Marshal is MarshalText: an out-of-range value is an error, never written.
func Marshal[E ~int](n Names[E], e E) ([]byte, error) {
	if !Valid(n, e) {
		return nil, fmt.Errorf("invalid %s %d", n.Kind, int(e))
	}
	return []byte(n.Names[e]), nil
}

// Unmarshal is UnmarshalText.
func Unmarshal[E ~int](n Names[E], b []byte, e *E) error {
	v, err := Parse(n, string(b))
	if err != nil {
		return err
	}
	*e = v
	return nil
}

// All lists every value in order (the "none" value included when present).
func All[E ~int](n Names[E]) []E {
	out := make([]E, len(n.Names))
	for i := range out {
		out[i] = E(i)
	}
	return out
}
