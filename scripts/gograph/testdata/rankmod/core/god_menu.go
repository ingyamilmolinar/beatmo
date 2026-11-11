package core

// Menu touches two exclusive fields: below the extraction threshold.
func (g *God) Menu() { g.menuA, g.menuB = g.shared, g.shared }
