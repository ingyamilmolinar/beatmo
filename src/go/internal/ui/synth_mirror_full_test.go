//go:build test

package ui

import "testing"

// Cache-logic-only tests: use &synthMirror{} directly (NO pool — a shared
// "synth.preview" Release would break sibling mirror tests).
func TestSynthMirror_RenderLiveFullHashGated(t *testing.T) {
	m := &synthMirror{}
	calls := 0
	render := func(string) []float64 { calls++; return []float64{float64(calls)} }

	m.renderLiveFull("inst", "h1", render)
	m.renderLiveFull("inst", "h1", render)
	if calls != 1 {
		t.Fatalf("same hash must not re-render: %d calls", calls)
	}
	m.renderLiveFull("inst", "h2", render)
	if calls != 2 {
		t.Fatalf("new hash must re-render: %d calls", calls)
	}
	pcm, ghost := m.snapshotFull()
	if len(pcm) != 1 || pcm[0] != 2 {
		t.Fatalf("live full pcm wrong: %v", pcm)
	}
	if len(ghost) != 1 || ghost[0] != 1 {
		t.Fatalf("prior render must demote to full ghost: %v", ghost)
	}
}

func TestSynthMirror_FullSlotIndependentOfSteadySlot(t *testing.T) {
	m := &synthMirror{}
	m.renderLive("inst", "hs", func(string) []float64 { return []float64{9} })
	m.renderLiveFull("inst", "hf", func(string) []float64 { return []float64{7} })
	steady, _ := m.snapshot()
	full, _ := m.snapshotFull()
	if steady[0] != 9 || full[0] != 7 {
		t.Fatalf("slots must be independent: steady=%v full=%v", steady, full)
	}
}

// TestSynthMirror_RequestFullStaleWhileRevalidate — on a NEW hash requestFull
// demotes the current pcmFull to ghostFull but keeps it drawing (does NOT nil
// it) until the async render lands. snapshotFull returns the OLD value before
// the drain, the NEW value after, with the old demoted to ghostFull.
func TestSynthMirror_RequestFullStaleWhileRevalidate(t *testing.T) {
	m := &synthMirror{} // nil pool: job stays pending for drainFullForTest

	m.requestFull("inst", "h1", func(string) []float64 { return []float64{1} })
	m.drainFullForTest()
	if pcm, _ := m.snapshotFull(); len(pcm) != 1 || pcm[0] != 1 {
		t.Fatalf("first render must land: %v", pcm)
	}

	// New hash: old pcmFull must still draw (stale-while-revalidate) BEFORE drain.
	m.requestFull("inst", "h2", func(string) []float64 { return []float64{2} })
	pcmBefore, ghostBefore := m.snapshotFull()
	if len(pcmBefore) != 1 || pcmBefore[0] != 1 {
		t.Fatalf("stale-while-revalidate: old pcmFull must still draw before drain: %v", pcmBefore)
	}
	if len(ghostBefore) != 1 || ghostBefore[0] != 1 {
		t.Fatalf("old value must demote to ghostFull immediately: %v", ghostBefore)
	}

	// After drain the new value lands; the old stays as the ghost.
	m.drainFullForTest()
	pcmAfter, ghostAfter := m.snapshotFull()
	if len(pcmAfter) != 1 || pcmAfter[0] != 2 {
		t.Fatalf("new value must land after drain: %v", pcmAfter)
	}
	if len(ghostAfter) != 1 || ghostAfter[0] != 1 {
		t.Fatalf("ghostFull must retain old value: %v", ghostAfter)
	}
}

// TestSynthMirror_RequestFullSupersededJobDiscards — a stale job (captured from
// an older hash) that runs AFTER a newer hash committed must NOT clobber the
// newer render. The commit guard (m.lastHashFull == hash) protects this.
func TestSynthMirror_RequestFullSupersededJobDiscards(t *testing.T) {
	m := &synthMirror{}
	renderFor := func(tag float64) func(string) []float64 {
		return func(string) []float64 { return []float64{tag} }
	}

	m.requestFull("inst", "h1", renderFor(1))
	// Capture h1's job before h2 replaces it (mimics an in-flight pool worker).
	m.mu.Lock()
	staleJob := m.pendingFull
	m.mu.Unlock()

	m.requestFull("inst", "h2", renderFor(2))
	m.drainFullForTest() // runs h2's job -> pcmFull = {2}

	// The superseded h1 job runs late: the guard must keep h2's state.
	if staleJob != nil {
		staleJob()
	}
	pcm, _ := m.snapshotFull()
	if len(pcm) != 1 || pcm[0] != 2 {
		t.Fatalf("stale job clobbered newer render (guard failed): %v", pcm)
	}
}

// TestSynthMirror_RequestFullHashGated — identical hashes are coalesced (render
// runs once); a new hash re-renders.
func TestSynthMirror_RequestFullHashGated(t *testing.T) {
	m := &synthMirror{}
	calls := 0
	render := func(string) []float64 { calls++; return []float64{float64(calls)} }

	m.requestFull("inst", "h1", render)
	m.drainFullForTest()
	m.requestFull("inst", "h1", render) // same hash: coalesced, no new job
	m.drainFullForTest()
	if calls != 1 {
		t.Fatalf("same hash must render once: %d calls", calls)
	}

	m.requestFull("inst", "h2", render)
	m.drainFullForTest()
	if calls != 2 {
		t.Fatalf("new hash must re-render: %d calls", calls)
	}
}
