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
	// Every structural mutation (solo/mute, row delete, instrument, graph,
	// BPM, …) routes through here: freeze the played past before it can change.
	g.freezePlayedThroughPlayhead()
	g.parityGen.Add(1)
	// C1 fix (part b): clear any session-monotonic timing-gate peak
	// (seqFireMax/maxLag/bridgeMax/emitAbsMax) latched by an edit-induced
	// scheduling stall — pre-edit, or slipping through during the edit
	// before the grace-window observation guards apply. Without this the
	// gen bump + grace window below only suppress the CHECK, not the
	// latched statistic, and evalTimingStage's absMax dropout check (fires
	// regardless of MinSamples) can panic the first parityTimingCheck after
	// grace expires even though the edit itself was benign. Every
	// structural mutation routes through this method, so every edit clears
	// the peaks. See schedule_metrics.go's ResetPeaks doc and design §3.4.
	g.schedMetrics.ResetPeaks()
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
