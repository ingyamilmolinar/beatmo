package ui

// Prefers the cached row sprite (what was really rendered).
func (g *Game) parityViewState(row, abs int) (slate bool, inWindow bool, offset int, length int, source string) {
	if g == nil || g.drum == nil || row < 0 || row >= len(g.drum.Rows) {
		return false, false, 0, 0, ""
	}
	if off, ln, ok := g.drum.cachedRowWindow(row); ok {
		offset, length = off, ln
		inWindow = abs >= offset && abs < offset+length
		if inWindow {
			if v, ok := g.drum.cachedRowState(row, abs); ok {
				return v, true, offset, length, "cache"
			}
			rel := abs - offset
			if rel >= 0 && rel < len(g.drum.Rows[row].Steps) {
				return g.drum.Rows[row].Steps[rel], true, offset, length, "steps"
			}
			// Skip rather than guess from a possibly inconsistent slate.
			return false, false, offset, length, "cache"
		}
		return false, inWindow, offset, length, "cache"
	}
	offset = g.renderOffset
	length = g.renderLength
	if length <= 0 {
		return false, false, offset, length, "steps"
	}
	inWindow = abs >= offset && abs < offset+length
	if inWindow {
		rel := abs - offset
		if rel >= 0 && rel < len(g.drum.Rows[row].Steps) {
			slate = g.drum.Rows[row].Steps[rel]
		}
	}
	return slate, inWindow, offset, length, "steps"
}
