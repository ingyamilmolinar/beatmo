package core

// FX touches nine fields no other file touches.
func (g *God) FX() {
	g.fxA, g.fxB, g.fxC = 1, 2, 3
	g.fxD, g.fxE, g.fxF = 4, 5, 6
	g.fxG, g.fxH, g.fxI = 7, 8, 9
	g.fxReset()
	g.shared++
}

func (g *God) fxReset() { g.fxA = 0 }
