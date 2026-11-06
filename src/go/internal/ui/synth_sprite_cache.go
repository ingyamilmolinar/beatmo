package ui

// synth_sprite_cache.go — Task 2 of the render-caching plan (see
// docs/superpowers/specs/2026-07-13-trace-sprite-cache-design.md): the Synth
// tab's graph and its cards blit a cached GPU texture on a steady frame
// instead of re-walking their pixel loops (TraceSprite adopters).

// synthSpriteCache holds the Synth tab's component render caches (TraceSprite
// adopters). Owned by synthPanel; Release()d on tab-hide via the EQ panel's
// releaseHiddenTabSprites chokepoint. Not goroutine-safe (UI goroutine only).
type synthSpriteCache struct {
	graph TraceSprite // the rendered-audio graph (synth_graph_pane.go)
	cards map[string]*TraceSprite
	seen  map[string]bool
}

// card lazily creates (and marks seen this frame) the per-paramID card
// sprite. Task 3 wires the ParamCard draw path through this; kept here now so
// the holder's shape matches the plan up front.
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

// beginCardFrame / endCardFrame mark-and-sweep evict card sprites whose param
// left the visible set (section switch / scroll), keeping the map bounded.
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

// releaseAll releases every sprite's GPU texture immediately (tab-hide
// eviction) and clears the card map.
func (c *synthSpriteCache) releaseAll() {
	c.graph.Release()
	for id, s := range c.cards {
		s.Release()
		delete(c.cards, id)
	}
}
