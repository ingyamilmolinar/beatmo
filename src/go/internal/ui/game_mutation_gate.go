package ui

import (
	"time"
)

// One 60 Hz frame with margin, plus slack to drain a scheduler horizon.
const parityMutationGraceDefault = 80 * time.Millisecond

type structuralMutationOptions struct {
	// false when the caller already holds seqMu.
	HoldSeqMu bool
	// For mutations that cannot affect the predictor (pure mixer/EQ).
	SkipPathsDirty bool
	// The default marker keeps the freeze loop from committing pre-mutation beats with the new view.
	SkipPathChangeMark bool
	// The gen filter alone is correct; clearing only bounds memory, and tests may need the buffers.
	SkipBufferClear bool
	// 0 = parityMutationGraceDefault.
	Grace time.Duration
}

func defaultStructuralMutationOptions() structuralMutationOptions {
	return structuralMutationOptions{
		HoldSeqMu: true,
		Grace:     parityMutationGraceDefault,
	}
}

// The single chokepoint for runtime mutations that could disturb predictor, scheduler or channel
// state during playback.
func (g *Game) mutateStructural(reason string, fn func(), opts structuralMutationOptions) {
	if g == nil {
		if fn != nil {
			fn()
		}
		return
	}
	if opts.Grace == 0 {
		opts.Grace = parityMutationGraceDefault
	}
	if opts.HoldSeqMu {
		g.seqMu.Lock()
		defer g.seqMu.Unlock()
	}
	g.bumpParityGen(reason, opts)
	if fn != nil {
		fn()
	}
}

// Does NOT take seqMu: for callers that already hold it.
func (g *Game) bumpParityGen(reason string, opts structuralMutationOptions) {
	if g == nil {
		return
	}
	if opts.Grace == 0 {
		opts.Grace = parityMutationGraceDefault
	}
	// Freeze the played past before anything can change it.
	g.freezePlayedThroughPlayhead()
	g.parityGen.Add(1)
	// Clear the session-monotonic timing peaks too: the grace window only suppresses the check, and a
	// peak latched by an edit stall would fail the first check after grace.
	g.schedMetrics.ResetPeaks()
	if opts.Grace > 0 {
		deadline := time.Now().Add(opts.Grace).UnixNano()
		// Extend, never shrink, so back-to-back mutations keep the longer grace.
		for {
			cur := g.parityGraceUntilNS.Load()
			if cur >= deadline {
				break
			}
			if g.parityGraceUntilNS.CompareAndSwap(cur, deadline) {
				break
			}
		}
	}
	if !opts.SkipPathsDirty {
		g.pathsDirty = true
	}
	if !opts.SkipPathChangeMark {
		g.markAllRowsPathChangedAt(g.elapsedBeats)
	}
	if !opts.SkipBufferClear {
		g.clearParityStateForGenBump()
	}
	if g.logger != nil {
		g.logger.Tracef("[parity][gen-bump] reason=%s gen=%d grace=%v", reason, g.parityGen.Load(), opts.Grace)
	}
}

// The freeze loop skips committing pre-marker beats, which a cross-row mutation needs.
func (g *Game) markAllRowsPathChangedAt(beat int) {
	if g == nil || g.drum == nil {
		return
	}
	if len(g.pathChangeBeatByRow) < len(g.drum.Rows) {
		next := make([]int, len(g.drum.Rows))
		for i := range next {
			next[i] = -1
			if i < len(g.pathChangeBeatByRow) {
				next[i] = g.pathChangeBeatByRow[i]
			}
		}
		g.pathChangeBeatByRow = next
	}
	for i := range g.pathChangeBeatByRow {
		// max: keep a later marker.
		if beat > g.pathChangeBeatByRow[i] {
			g.pathChangeBeatByRow[i] = beat
		}
	}
}
