package ui

import (
	"time"
)

// parityMutationGraceDefault is the default grace window granted to the parity
// comparator after a runtime structural mutation. 80 ms is one full frame at
// 60 Hz with margin, plus enough slack to drain a typical scheduler horizon.
const parityMutationGraceDefault = 80 * time.Millisecond

// structuralMutationOptions tunes mutateStructural behavior for specific call
// sites. Zero value selects sensible defaults: gen bump + grace + buffer
// hygiene + path-dirty flag.
type structuralMutationOptions struct {
	// HoldSeqMu makes mutateStructural acquire g.seqMu around fn. Set to false
	// when the caller already holds the lock (e.g. when invoked from a
	// sequencer-thread callback).
	HoldSeqMu bool
	// SkipPathsDirty leaves g.pathsDirty untouched. Default behavior marks it
	// true so the next refresh recomputes per-row path data; mutations that
	// don't affect the predictor (pure audio-mixer / EQ tweaks) can opt out.
	SkipPathsDirty bool
	// SkipPathChangeMark leaves pathChangeBeatByRow alone. Default marks every
	// row at the current elapsed beat so the freeze loop does not commit
	// pre-mutation past beats with the post-mutation predictor view.
	SkipPathChangeMark bool
	// SkipBufferClear keeps the parity audio/seq/highlight buffers around.
	// The gen filter alone is sufficient for correctness; clearing is a
	// memory-bounding hygiene step. Tests sometimes need to inspect the
	// buffers across a mutation so this lets them opt out.
	SkipBufferClear bool
	// Grace overrides the grace-window duration. Zero = parityMutationGraceDefault.
	// Tests use a small or zero value to make assertions deterministic.
	Grace time.Duration
}

// defaultStructuralMutationOptions returns the production defaults: hold
// seqMu, mark paths dirty, mark per-row path change, clear buffers,
// 80ms grace.
func defaultStructuralMutationOptions() structuralMutationOptions {
	return structuralMutationOptions{
		HoldSeqMu: true,
		Grace:     parityMutationGraceDefault,
	}
}

// mutateStructural is the single choke-point for any runtime mutation that
// could disturb predictor / scheduler / audio-channel state during playback.
// It bumps the parity generation, grants a grace window, optionally marks
// pathChangeBeatByRow for every row, optionally clears parity buffers, and
// then runs fn.
//
// Every UI-level mutation in the (G2) list (instrument change, EQ, BPM,
// length, graph edit, row add/del, etc.) routes through this. Audio-only
// mutations whose state cannot affect parity (pure DSP-graph edits with no
// scheduler observable) may still wrap themselves for symmetry — the cost
// of a gen bump is a single atomic store plus map allocation.
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

// bumpParityGen performs only the parity-coordination side-effects of a
// structural mutation: gen bump, grace window, buffer hygiene, path-dirty
// flag, cross-row pathChange marker. It does NOT acquire seqMu. Use when
// the caller is already holding the lock (e.g. invoked transitively from
// Game.Update) or when no lock coordination is needed (read-modify-write
// of audio-only state).
func (g *Game) bumpParityGen(reason string, opts structuralMutationOptions) {
	if g == nil {
		return
	}
	if opts.Grace == 0 {
		opts.Grace = parityMutationGraceDefault
	}
	g.parityGen.Add(1)
	if opts.Grace > 0 {
		deadline := time.Now().Add(opts.Grace).UnixNano()
		// Extend, never shrink — back-to-back mutations should leave the
		// later (longer) grace in effect.
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

// markAllRowsPathChangedAt sets pathChangeBeatByRow[row] = beat for every row.
// The freeze loop in refreshDrumRow respects this marker and skips committing
// pre-marker beats — exactly the protection needed when a runtime mutation
// has cross-row impact (e.g. instrument change on row 4 affecting row 2's
// predictor view).
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
		// Use max so a freshly-marked row that already carries a later marker
		// retains it.
		if beat > g.pathChangeBeatByRow[i] {
			g.pathChangeBeatByRow[i] = beat
		}
	}
}
