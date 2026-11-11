package preview

import (
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/timeline"
)

type BeatInfoAtFunc func(abs int) model.BeatInfo
type WantAtFunc func(abs int, bi model.BeatInfo) bool
type CommitAtFunc func(abs int) (val bool, typ model.NodeType, kind timeline.CommitKind, ok bool)

// PlayedAtFunc returns the frozen played snapshot for abs: what the grid showed
// for the cell as the playhead passed it (timeline.Service.Played).
type PlayedAtFunc func(abs int) (val bool, typ model.NodeType, ok bool)

// CrossingCell is what a cell shows at the moment the playhead passes it, and
// therefore what gets frozen into the played snapshot: an immutable
// Playback/Import commit when one exists, otherwise the live prediction on the
// live path. It is evaluated ONCE per cell, at the crossing — afterwards the
// cell renders from the snapshot and this is never consulted again for it.
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

	// PlayedAt, when set, is the immutable played snapshot. A past cell that
	// has one renders it verbatim — it outranks commits, the live predictor,
	// the live path and the previous window. The remaining precedence rules
	// below only ever apply to past cells the snapshot has not reached.
	PlayedAt PlayedAtFunc
}

// BuildRowWindow returns the row's rendered Steps and CellTypes for the given
// window. It enforces a single precedence table:
//
//  0. The played snapshot (what the cell showed as the playhead passed it).
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
	// Anchor the past boundary to the VISUAL playhead. Everything strictly left
	// of the playhead (abs <= ElapsedBeats) is immutable past and must resolve
	// from committed history / preserved prior state — never the live predictor.
	// The per-row highlight index (NextIdx) can lag the playhead in production
	// because highlights are delivered asynchronously via hlCh and the stale-info
	// guard in applySequencerHighlight drops in-flight highlights the moment a
	// circuit edit changes a row's beat path. When that happens NextIdx trails
	// ElapsedBeats, and without this extension the cells in [NextIdx, ElapsedBeats]
	// were treated as mutable future and re-rendered from the edited predictor —
	// i.e. the already-played past visibly changed under the user. Gated on
	// PreservePastSteps (== playing) so a stopped/paused window carrying a stale
	// ElapsedBeats does not over-freeze the whole grid.
	if cfg.PreservePastSteps && cfg.ElapsedBeats+1 > pastBoundExclusive {
		pastBoundExclusive = cfg.ElapsedBeats + 1
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
