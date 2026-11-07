package preview

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

type BeatInfoAtFunc func(abs int) model.BeatInfo
type WantAtFunc func(abs int, bi model.BeatInfo) bool
type CommitAtFunc func(abs int) (val bool, typ model.NodeType, kind timeline.CommitKind, ok bool)

// What the cell showed as the playhead passed it.
type PlayedAtFunc func(abs int) (val bool, typ model.NodeType, ok bool)

// Evaluated ONCE per cell, at the crossing: afterwards the cell renders from the snapshot.
func CrossingCell(abs int, beatInfoAt BeatInfoAtFunc, wantAt WantAtFunc, commitAt CommitAtFunc) (bool, model.NodeType) {
	if commitAt != nil {
		if val, typ, kind, ok := commitAt(abs); ok && (kind == timeline.CommitKindPlayback || kind == timeline.CommitKindImport) {
			return val, typ
		}
	}
	bi := beatInfoAt(abs)
	return wantAt(abs, bi), bi.NodeType
}

type Config struct {
	Offset int
	Length int

	// NextIdx is the exclusive past boundary: abs < NextIdx is considered past.
	NextIdx int
	// Freeze masks apply only below it, so future windows keep mirroring the predictor.
	ElapsedBeats int
	// Inclusive; the only past boundary when NextIdx <= 0.
	FreezeLimit int

	// Only while the window is stationary (PrevStepsOffset == Offset).
	PreservePastSteps bool
	PrevStepsOffset   int
	PrevSteps         []bool

	// Kept while stationary to avoid rendering drift.
	PrevTypesOffset int
	PrevTypes       []model.NodeType

	// FastPath disables heavier preservation work for perf harnesses.
	FastPath bool

	// A past cell with a snapshot renders it verbatim, outranking everything; the rules below apply only to past
	// cells the snapshot has not reached.
	PlayedAt PlayedAtFunc
}

// The single precedence: played snapshot, then immutable commits for the true past, then the stationary prior
// window, then the predictor. Pure: it never mutates timeline state.
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
	// The past boundary is the VISUAL playhead: highlights arrive asynchronously and an edit drops in-flight ones,
	// so NextIdx can lag and the played past would re-render from the edited predictor. Only while playing.
	if cfg.PreservePastSteps && cfg.ElapsedBeats+1 > pastBoundExclusive {
		pastBoundExclusive = cfg.ElapsedBeats + 1
	}
	// Bounded so a future freeze never masks re-added nodes.
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
		inPast := pastBoundExclusive > 0 && abs < pastBoundExclusive

		// A played cell never touches live state — not even to look.
		if inPast && cfg.PlayedAt != nil {
			if val, typ, ok := cfg.PlayedAt(abs); ok {
				steps[i] = val
				types[i] = typ
				continue
			}
		}

		bi := beatInfoAt(abs)
		want := wantAt(abs, bi)

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
					// Released commits are bookkeeping and follow the predictor, or edits leave stale masks.
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
