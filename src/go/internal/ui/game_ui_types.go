package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/instrument"
)

/* ───────────────────────── data types ───────────────────────── */

type uiNode struct {
	ID       model.NodeID
	I, J     int     // grid indices
	X, Y     float64 // cached world coords (grid.Unit()*I, grid.Unit()*J)
	Selected bool
	Start    bool
}

func (n *uiNode) Bounds(scale float64) (x1, y1, x2, y2 float64) {
	halfSize := float64(NodeSpriteSize) / 2.0 * scale
	return n.X - halfSize, n.Y - halfSize, n.X + halfSize, n.Y + halfSize
}

type uiEdge struct {
	A, B  *uiNode
	t     float64 // connection animation progress 0..1
	pulse float64 // direction pulse progress (-1 inactive)
}

type dragLink struct {
	from     *uiNode
	toX, toY float64
	active   bool
}

// Screen space: the marquee is an overlay drawn before any camera transform.
type marqueeDrag struct {
	active         bool
	startX, startY int // screen px at press
	curX, curY     int // screen px, updated each frame while held
}

type pulse struct {
	x1, y1, x2, y2           float64
	t, speed                 float64
	from, to                 *uiNode
	fromBeatInfo, toBeatInfo model.BeatInfo
	path                     []model.BeatInfo
	pathIdx                  int
	lastIdx                  int
	row                      int
	segBeats                 float64
}

// A fixed array, not a slice, so audioLoop can read one element while an edit writes another
// without a slice-header race; rows beyond it fall back to transport gating.
const maxAudioGenRows = 256

type soundReq struct {
	id      instrument.ID
	vol     float64
	pitch   float64
	dur     float64
	when    float64
	hasWhen bool
	// 0 for fire-and-forget sounds.
	grid  float64
	enqAt time.Time
	// audioLoop drops the req if it no longer matches g.audioGen.
	gen uint64
	// An edit bumps only changed rows, so unchanged rows' in-flight beats survive (no gap). Zero for
	// fire-and-forget sounds.
	rowGen uint64
	// Captured at SCHEDULING, in the seqMu section that records the decision: audioLoop may record
	// the event after a mutation advanced g.parityGen, and the current value would split the pair.
	parityGen uint64
	// (-1,-1) for previews, so parity buffers stay clean.
	row int
	abs int
}
