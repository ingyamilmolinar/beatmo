package ui

// Released on tab-hide; UI goroutine only.
type synthSpriteCache struct {
	graph TraceSprite // the rendered-audio graph (synth_graph_pane.go)
	cards map[string]*TraceSprite
	seen  map[string]bool
}

func (c *synthSpriteCache) card(id string) *TraceSprite {
	if c.cards == nil {
		c.cards = map[string]*TraceSprite{}
	}
	s := c.cards[id]
	if s == nil {
		s = &TraceSprite{tag: "synthCard", minInterval: traceSpriteDragInterval}
		c.cards[id] = s
	}
	if c.seen == nil {
		c.seen = map[string]bool{}
	}
	c.seen[id] = true
	return s
}

// Mark-and-sweep so the map stays bounded as cards leave the visible set.
func (c *synthSpriteCache) beginCardFrame() {
	for k := range c.seen {
		delete(c.seen, k)
	}
}

func (c *synthSpriteCache) endCardFrame() {
	for id, s := range c.cards {
		if !c.seen[id] {
			s.Release()
			delete(c.cards, id)
		}
	}
}

func (c *synthSpriteCache) releaseAll() {
	c.graph.Release()
	for id, s := range c.cards {
		s.Release()
		delete(c.cards, id)
	}
}
