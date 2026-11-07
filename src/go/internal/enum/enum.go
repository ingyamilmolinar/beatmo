// Package enum is the one spelling table behind every closed enum (an int type indexing a Names table), so an unknown
// spelling from data is an error, never a new value; index 0 named "" is the enum's "none". The operations are generic
// FUNCTIONS: Ebiten links text/template, so the linker keeps every exported method of a type reaching an interface.
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
