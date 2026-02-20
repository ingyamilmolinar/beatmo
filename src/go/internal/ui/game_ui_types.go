package ui

import (
	"time"

	"github.com/ingyamilmolinar/beatmo/core/model"
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

type soundReq struct {
	id      string
	vol     float64
	pitch   float64
	dur     float64
	when    float64
	hasWhen bool
	enqAt   time.Time
	gen     uint64
	// Parity metadata (row/abs) is only set for sequencer-driven playback
	// events. Preview sounds use (-1,-1) to avoid polluting parity buffers.
	row int
	abs int
}
