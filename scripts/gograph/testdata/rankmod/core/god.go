package core

// God is a god type spread over three files. god_fx.go alone touches the
// fx* fields, which makes it an extract-struct candidate.
type God struct {
	shared                                      int
	fxA, fxB, fxC, fxD, fxE, fxF, fxG, fxH, fxI int
	menuA, menuB                                int
}

// Draw uses only the shared field.
func (g *God) Draw() { g.shared++ }
