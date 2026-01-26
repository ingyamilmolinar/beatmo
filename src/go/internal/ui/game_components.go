package ui

func (g *Game) ensureComponentRegistry() *ComponentRegistry {
	if g == nil {
		return nil
	}
	if g.components == nil {
		reg := NewComponentRegistry()
		reg.Register(toolbarComponent{})
		reg.Register(notificationsComponent{})
		reg.Register(rowControlsComponent{})
		g.components = reg
	}
	if g.drum != nil {
		g.drum.SetComponentRegistry(g.components)
		g.components.Init(ComponentContext{Game: g, Drum: g.drum})
	}
	return g.components
}

func (g *Game) notifyComponentsGraphChange(change GraphChange) {
	if g == nil || g.components == nil {
		return
	}
	g.components.NotifyGraphChange(change)
}
