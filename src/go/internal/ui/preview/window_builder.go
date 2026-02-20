package preview

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

type BeatInfoAtFunc func(abs int) model.BeatInfo
type WantAtFunc func(abs int, bi model.BeatInfo) bool
type CommitAtFunc func(abs int) (val bool, typ model.NodeType, kind timeline.CommitKind, ok bool)

type Config struct {
	Offset int
	Length int

	// NextIdx is the exclusive past boundary: abs < NextIdx is considered past.
	NextIdx int
	// ElapsedBeats is the global playhead floor used by parity checks. Past-step
	// preservation (freeze masks) only applies for abs < ElapsedBeats so future
	// windows keep mirroring predictor truth.
	ElapsedBeats int
	// FreezeLimit is the inclusive frozen ceiling for the row. When NextIdx <= 0,
	// it provides the only past boundary used for preserving prior window state.
	FreezeLimit int

	// PreservePastSteps enables step immutability for already-rendered past
	// subdivisions while the window remains stationary (PrevStepsOffset == Offset).
	PreservePastSteps bool
	PrevStepsOffset   int
	PrevSteps         []bool

	// PrevTypes are the previously rendered cell types for this row. When the
	// window remains stationary (PrevTypesOffset == Offset), past types are
	// preserved to avoid rendering drift.
	PrevTypesOffset int
	PrevTypes       []model.NodeType

	// FastPath disables heavier preservation work for perf harnesses.
	FastPath bool
}

// BuildRowWindow returns the row's rendered Steps and CellTypes for the given
// window. It enforces a single precedence table:
//
//  1. Immutable timeline commits (Playback/Import) for true past.
//  2. Prior window snapshot (freeze mask) for past cells when stationary.
//  3. Predictor/live preview for everything else.
//
// This function is intentionally pure: it must not mutate timeline state.
func BuildRowWindow(cfg Config, beatInfoAt BeatInfoAtFunc, wantAt WantAtFunc, commitAt CommitAtFunc, reuseSteps []bool, reuseTypes []model.NodeType) ([]bool, []model.NodeType) {
	if cfg.Length <= 0 {
		return reuseSteps[:0], reuseTypes[:0]
	}
	if beatInfoAt == nil || wantAt == nil {
		return ensureSteps(reuseSteps, cfg.Length), ensureTypes(reuseTypes, cfg.Length)
	}

	steps := ensureSteps(reuseSteps, cfg.Length)
	types := ensureTypes(reuseTypes, cfg.Length)

	pastBoundExclusive := 0
	if cfg.NextIdx > 0 {
		pastBoundExclusive = cfg.NextIdx
	} else if cfg.FreezeLimit >= 0 {
		pastBoundExclusive = cfg.FreezeLimit + 1
	}
	// Preserve past steps only within a bounded range to avoid a future freeze
	// masking re-added nodes. Mirrors the legacy guard in Game.buildRowWindow.
	preserveLimit := -1
	if pastBoundExclusive > 0 {
		preserveLimit = pastBoundExclusive - 1
	}
	if cfg.FreezeLimit >= 0 && (preserveLimit < 0 || cfg.FreezeLimit < preserveLimit) {
		preserveLimit = cfg.FreezeLimit
	}

	canPreserveSteps := cfg.PreservePastSteps && !cfg.FastPath && cfg.PrevStepsOffset == cfg.Offset && len(cfg.PrevSteps) > 0 && preserveLimit >= cfg.Offset
	canPreserveTypes := cfg.PrevTypesOffset == cfg.Offset && len(cfg.PrevTypes) == cfg.Length

	for i := 0; i < cfg.Length; i++ {
		abs := cfg.Offset + i
		bi := beatInfoAt(abs)
		want := wantAt(abs, bi)

		inPast := pastBoundExclusive > 0 && abs < pastBoundExclusive

		val, typ, kind, ok := false, model.NodeTypeInvisible, timeline.CommitKindPlayback, false
		if inPast && commitAt != nil {
			val, typ, kind, ok = commitAt(abs)
		}

		finalType := bi.NodeType
		finalStep := want

		if inPast {
			if ok && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport) {
				finalType = typ
				finalStep = val
			} else {
				if canPreserveTypes {
					finalType = cfg.PrevTypes[i]
				}
				if canPreserveSteps && abs <= preserveLimit && i < len(cfg.PrevSteps) {
					// Released commits are bookkeeping and should follow predictor to avoid
					// stale masks after edits.
					if abs < cfg.ElapsedBeats && (!ok || kind != timeline.CommitKindReleased) {
						finalStep = cfg.PrevSteps[i]
					}
				}
			}
		}

		steps[i] = finalStep
		types[i] = finalType
	}

	return steps, types
}

func ensureSteps(buf []bool, n int) []bool {
	if n <= 0 {
		return buf[:0]
	}
	if cap(buf) < n {
		return make([]bool, n)
	}
	return buf[:n]
}

func ensureTypes(buf []model.NodeType, n int) []model.NodeType {
	if n <= 0 {
		return buf[:0]
	}
	if cap(buf) < n {
		return make([]model.NodeType, n)
	}
	return buf[:n]
}
