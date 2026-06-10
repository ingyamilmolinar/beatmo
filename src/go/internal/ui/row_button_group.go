package ui

// RowButtonGroup holds all per-row buttons for a single drum row.
// It allows processing all buttons in a single pass instead of
// iterating separate arrays.
type RowButtonGroup struct {
	Mute   *Button
	Solo   *Button
	FX     *Button
	Origin *Button
	Delete *Button
	Edit   *Button
	Save   *Button
	Menu   *Button
	Label  *Button
}

// HandleInput processes all buttons in priority order (mute, solo, fx, origin,
// delete, edit, save, menu, label). Returns true if any button consumed input.
func (g *RowButtonGroup) HandleInput(mx, my int, left bool) bool {
	btns := []*Button{g.Mute, g.Solo, g.FX, g.Origin, g.Delete, g.Edit, g.Save, g.Menu, g.Label}
	for _, btn := range btns {
		if btn != nil && btn.HandleInputResult(mx, my, left) != InputIgnored {
			return true
		}
	}
	return false
}
