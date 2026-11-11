package ui

// postFromWorker hands fn to the UI goroutine, which runs it at the start of
// the next Update. Any goroutine may call it; fn may touch UI state.
func (g *Game) postFromWorker(fn func()) {
	if g == nil || fn == nil {
		return
	}
	g.workerPostMu.Lock()
	g.workerPosts = append(g.workerPosts, fn)
	g.workerPostMu.Unlock()
}

// drainWorkerPosts runs, on the UI goroutine, everything posted since the last
// frame, in posting order.
func (g *Game) drainWorkerPosts() {
	g.workerPostMu.Lock()
	q := g.workerPosts
	g.workerPosts = nil
	g.workerPostMu.Unlock()
	for _, fn := range q {
		fn()
	}
}
