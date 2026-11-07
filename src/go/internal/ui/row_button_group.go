package ui

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

func (g *RowButtonGroup) HandleInput(mx, my int, left bool) bool {
	btns := []*Button{g.Mute, g.Solo, g.FX, g.Origin, g.Delete, g.Edit, g.Save, g.Menu, g.Label}
	for _, btn := range btns {
		if btn != nil && btn.HandleInputResult(mx, my, left) != InputIgnored {
			return true
		}
	}
	return false
}
