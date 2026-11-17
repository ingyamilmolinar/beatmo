package core

// Solo is a type whose methods all live in its own file: its "exclusive"
// fields are simply its fields, so this file is the type, not a slice of it.
type Solo struct {
	a, b, c, d, e, f, g, h, i, j int
}

func (s *Solo) One() int   { return s.a + s.b + s.c }
func (s *Solo) Two() int   { return s.d + s.e + s.f + s.g }
func (s *Solo) Three() int { return s.h + s.i + s.j }

// UseSolo keeps Solo reachable.
func UseSolo() int { s := &Solo{}; return s.One() + s.Two() + s.Three() }
