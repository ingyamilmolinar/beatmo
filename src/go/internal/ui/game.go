package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/ingyamilmolinar/tunkul/core/engine"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

const (
	topOffset = 40 // transport-bar height in px
)

const ebitenTPS = 60 // Ticks per second for Ebiten (stubbed for tests)

// playSound plays a synthesized sound with volume. Overridden in tests.
var playSound = audio.PlayVol

var enableDefaultStart = true

func SetDefaultStartForTest(enable bool) { enableDefaultStart = enable }

func makeBeatKey(row, idx int) int { return (row << 16) | (idx & 0xFFFF) }

func splitBeatKey(key int) (row, idx int) { return key >> 16, key & 0xFFFF }

func loopSegmentLen(path []model.BeatInfo, start int) int {
	if start < 0 || start >= len(path) {
		return 0
	}
	origin := path[start].NodeID
	for i := start + 1; i < len(path); i++ {
		if path[i].NodeID == origin {
			return i - start
		}
	}
	return len(path) - start
}

// rawBeatLen returns the length of the non-expanded beat path. When the graph
// requests a beat row with a large beat length, the loop segment is repeated to
// fill that length. This helper strips the repeated portion so callers obtain
// the actual traversal length regardless of the current beatLength setting.
func rawBeatLen(path []model.BeatInfo, isLoop bool, loopStart int) int {
	if !isLoop {
		for i, b := range path {
			if b.NodeID == model.InvalidNodeID {
				return i
			}
		}
		return len(path)
	}
	if loopStart < 0 || loopStart >= len(path) {
		return len(path)
	}
	origin := path[loopStart].NodeID
	for i := loopStart + 1; i < len(path); i++ {
		if path[i].NodeID == origin {
			return i
		}
	}
	return len(path)
}

// sendLatest writes v to ch, dropping older values if the buffer is full.
// It loops until the send succeeds without ever blocking.
func sendLatest[T any](ch chan T, v T) {
	for {
		select {
		case ch <- v:
			return
		default:
			select {
			case <-ch:
			default:
			}
		}
	}
}

/* ───────────────────────── data types ───────────────────────── */

type uiNode struct {
	ID       model.NodeID
	I, J     int     // grid indices
	X, Y     float64 // cached world coords (grid.Unit()*I, grid.Unit()*J)
	Selected bool
	Start    bool
	path     []model.NodeID // Path taken by the pulse to reach this node
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
}

type soundReq struct {
	id   string
	vol  float64
	when []float64
}

type Game struct {
	/* subsystems */
	cam            *Camera
	split          *Splitter
	drum           *DrumView
	graph          *model.Graph
	engine         *engine.Engine
	engineProgress func() float64
	logger         *game_log.Logger
	grid           *Grid
	audioCh        chan soundReq
	bpmCh          chan int
	bpmAck         chan int
	playFn         func(string, float64, ...float64)

	/* graph data */
	nodes           []*uiNode
	edges           []uiEdge
	pendingStartRow int

	/* visuals */
	activePulses        []*pulse
	activePulse         *pulse // primary row pulse for legacy tests
	frame               int64
	renderedPulsesCount int
	highlightedBeats    map[int]int64 // Encoded row/index keys
	selNeighbors        map[*uiNode]bool
	hover               *uiNode
	cursorLabel         string

	/* editor state */
	sel            *uiNode
	linkDrag       dragLink
	camDragging    bool
	camDragged     bool
	leftPrev       bool
	pendingClick   bool
	clickI, clickJ int

	/* game state */
	playing            bool
	paused             bool
	bpm                int
	appliedBPM         int
	currentStep        int // Current step in the sequence
	lastBeatFrame      int64
	beatInfos          []model.BeatInfo   // Full traversal path for primary start
	drumBeatInfos      []model.BeatInfo   // BeatInfos sized to drum view
	beatInfosByRow     [][]model.BeatInfo // Per-row traversal paths
	isLoop             bool               // Indicates if the current graph forms a loop
	loopStartIndex     int                // The index in beatInfos where the loop segment begins
	isLoopByRow        []bool
	loopStartByRow     []int
	loopLenByRow       []int
	originIdxsByRow    [][]int
	nextOriginIdxByRow []int
	nextBeatIdxs       []int                // Absolute beat index per row
	nodeRows           map[model.NodeID]int // nodeID -> row index
	elapsedBeats       int

	/* misc */
	winW, winH int
	start      *uiNode // explicit “root/start” node (⇧S to set)
}

/* ───────────────── helper: node’s screen rect ───────────────── */

// Rectangle in *screen* pixels (y already includes the transport offset).
func (g *Game) nodeScreenRect(n *uiNode) (x1, y1, x2, y2 float64) {
	unitPx := g.grid.UnitPixels(g.cam.Scale) // px per smallest subdivision
	offX := math.Round(g.cam.OffsetX)        // camera panning
	offY := math.Round(g.cam.OffsetY)

	sx := offX + unitPx*float64(n.I)
	sy := offY + unitPx*float64(n.J) + topOffset
	r := g.grid.NodeRadius(g.cam.Scale) * g.cam.Scale
	return sx - r, sy - r, sx + r, sy + r
}

func (g *Game) nodeRadius(n *uiNode) float64 {
	r := g.grid.NodeRadius(g.cam.Scale)
	if g.hover == n {
		r *= 2
	}
	return r
}

// computeSelNeighbors refreshes the neighbor set for the currently selected node.
func (g *Game) computeSelNeighbors() {
	g.selNeighbors = map[*uiNode]bool{}
	if g.sel == nil {
		return
	}
	for i := range g.edges {
		if g.edges[i].A == g.sel {
			g.selNeighbors[g.edges[i].B] = true
		} else if g.edges[i].B == g.sel {
			g.selNeighbors[g.edges[i].A] = true
		}
	}
}

/* ───────────────────── constructor & layout ─────────────────── */

func New(logger *game_log.Logger) *Game {
	eng := engine.New(logger)
	g := &Game{
		cam:                NewCamera(),
		logger:             logger,
		graph:              eng.Graph,
		engine:             eng,
		engineProgress:     eng.Progress,
		split:              NewSplitter(720), // real height set in Layout below
		highlightedBeats:   make(map[int]int64),
		bpm:                120, // Default BPM
		beatInfos:          []model.BeatInfo{},
		drumBeatInfos:      []model.BeatInfo{},
		beatInfosByRow:     [][]model.BeatInfo{},
		isLoopByRow:        []bool{},
		loopStartByRow:     []int{},
		loopLenByRow:       []int{},
		originIdxsByRow:    [][]int{},
		nextOriginIdxByRow: []int{},
		nextBeatIdxs:       []int{},
		nodeRows:           make(map[model.NodeID]int),
		activePulses:       []*pulse{},
		pendingStartRow:    -1,
		grid:               NewGrid(DefaultGridStep),
		audioCh:            make(chan soundReq, 32),
		bpmCh:              make(chan int, 1),
		bpmAck:             make(chan int, 1),
		playFn:             playSound,
	}

	// bottom drum-machine view
	g.drum = NewDrumView(image.Rect(0, 600, 1280, 720), g.graph, logger)
	g.appliedBPM = g.drum.BPM()
	go g.audioLoop()
	go g.bpmLoop()
	g.initJS()
	return g
}

func (g *Game) audioLoop() {
	for req := range g.audioCh {
		g.playFn(req.id, req.vol, req.when...)
	}
}

// SetPlayFunc overrides the audio playback function used by this game instance.
func (g *Game) SetPlayFunc(fn func(string, float64, ...float64)) {
	g.playFn = fn
}

func (g *Game) bpmLoop() {
	for b := range g.bpmCh {
		// Drain any pending updates so only the latest BPM is applied.
		for {
			select {
			case b = <-g.bpmCh:
				// keep draining
			default:
				start := time.Now()
				g.logger.Debugf("[GAME] applying BPM=%d", b)
				g.engine.SetBPM(b)
				audio.SetBPM(b)
				g.logger.Debugf("[GAME] applied BPM=%d in %s", b, time.Since(start))
				sendLatest(g.bpmAck, b)
				goto next
			}
		}
	next:
	}
}

func (g *Game) Layout(w, h int) (int, int) {
	g.winW, g.winH = w, h

	/* update splitter and drum bounds */
	if g.split == nil {
		g.split = NewSplitter(h)
	}
	if g.split.ratio == 0 { // first time → store ratio
		g.split.ratio = float64(g.split.Y) / float64(h)
	}
	g.split.Y = int(float64(h) * g.split.ratio)
	g.drum.SetBounds(image.Rect(0, g.split.Y, g.winW, g.winH))
	if enableDefaultStart && len(g.nodes) == 0 {
		g.cam.OffsetX = float64(w) / 2
		g.cam.OffsetY = float64(g.split.Y-topOffset) / 2
		g.cam.Snap()
		g.pendingStartRow = 0
		g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	g.logger.Infof("[GAME] Layout: winW: %d, winH: %d, split.Y: %d, drum.Bounds: %v", g.winW, g.winH, g.split.Y, g.drum.Bounds)
	return w, h
}

/* ─────────────────────── graph helpers ─────────────────────── */

func (g *Game) nodeAt(i, j int) *uiNode {
	for _, n := range g.nodes {
		if n.I == i && n.J == j {
			return n
		}
	}
	return nil
}

func (g *Game) nodeByID(id model.NodeID) *uiNode {
	for _, n := range g.nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (g *Game) tryAddNode(i, j int, nodeType model.NodeType) *uiNode {
	if n := g.nodeAt(i, j); n != nil {
		// If there's an invisible node here and we want a regular node,
		// upgrade the existing node rather than blocking the placement.
		if nodeType == model.NodeTypeRegular {
			if g.pendingStartRow >= 0 {
				row := g.pendingStartRow
				if other, ok := g.nodeRows[n.ID]; ok && other != row {
					g.pendingStartRow = -1
					return n
				}
				if row >= 0 && row < len(g.drum.Rows) {
					if old := g.drum.Rows[row].Node; old != nil {
						old.Start = false
					}
					g.drum.Rows[row].Origin = n.ID
					g.drum.Rows[row].Node = n
					n.Start = true
					if row == 0 {
						g.start = n
						g.graph.StartNodeID = n.ID
					}
					g.updateBeatInfos()
				}
				g.pendingStartRow = -1
			} else if node, ok := g.graph.GetNodeByID(n.ID); ok && node.Type == model.NodeTypeInvisible {
				node.Type = model.NodeTypeRegular
				g.graph.Nodes[n.ID] = node
				g.logger.Debugf("[GAME] Upgraded invisible node to regular at grid=(%d,%d)", i, j)
				if g.start == nil {
					g.start = n
					n.Start = true
					g.graph.StartNodeID = n.ID
				}
				g.updateBeatInfos()
			}
		}
		return n
	}
	id := g.graph.AddNode(i, j, nodeType)
	unit := g.grid.Unit()
	n := &uiNode{ID: id, I: i, J: j, X: float64(i) * unit, Y: float64(j) * unit}

	if nodeType == model.NodeTypeRegular {
		if g.pendingStartRow >= 0 {
			row := g.pendingStartRow
			if row >= 0 && row < len(g.drum.Rows) {
				g.drum.Rows[row].Origin = n.ID
				g.drum.Rows[row].Node = n
				n.Start = true
				if row == 0 {
					g.start = n
					g.graph.StartNodeID = n.ID
				}
			}
			g.pendingStartRow = -1
		} else if g.start == nil {
			g.start = n
			n.Start = true
			g.graph.StartNodeID = n.ID
		}
	}
	g.nodes = append(g.nodes, n)
	g.updateBeatInfos()
	return n
}

func (g *Game) deleteNode(n *uiNode) {
	/* remove from slice */
	for idx, v := range g.nodes {
		if v.ID == n.ID {
			g.nodes = append(g.nodes[:idx], g.nodes[idx+1:]...)
			break
		}
	}
	/* drop touching edges */
	out := g.edges[:0]
	for _, e := range g.edges {
		if e.A.ID != n.ID && e.B.ID != n.ID {
			out = append(out, e)
		}
	}
	g.edges = out

	g.graph.RemoveNode(n.ID)

	if g.sel == n {
		g.sel = nil
	}
	if g.start == n {
		g.start = nil
	}
	g.updateBeatInfos()
}

func (g *Game) updateBeatInfos() {
	// Use a generously large beat length so CalculateBeatRow returns the
	// complete path even when disconnected nodes exist elsewhere in the
	// graph. We'll shrink the beat length back to the actual traversal size
	// after computing the raw path length.
	g.graph.SetBeatLength(int(g.graph.Next))

	fullBeatRow, isLoop, loopStart := g.graph.CalculateBeatRow()
	baseLen := rawBeatLen(fullBeatRow, isLoop, loopStart)

	g.beatInfos = fullBeatRow[:baseLen]
	g.isLoop = isLoop
	g.loopStartIndex = loopStart

	maxLen := baseLen
	g.nodeRows = map[model.NodeID]int{}
	nRows := len(g.drum.Rows)
	prevNext := g.nextBeatIdxs
	g.beatInfosByRow = make([][]model.BeatInfo, nRows)
	g.isLoopByRow = make([]bool, nRows)
	g.loopStartByRow = make([]int, nRows)
	g.loopLenByRow = make([]int, nRows)
	g.originIdxsByRow = make([][]int, nRows)
	g.nextOriginIdxByRow = make([]int, nRows)
	g.nextBeatIdxs = make([]int, nRows)
	copy(g.nextBeatIdxs, prevNext)
	if nRows > 0 {
		g.beatInfosByRow[0] = g.beatInfos
		g.isLoopByRow[0] = isLoop
		g.loopStartByRow[0] = loopStart
		if isLoop {
			g.loopLenByRow[0] = loopSegmentLen(g.beatInfos, loopStart)
		}
		origin := g.drum.Rows[0].Origin
		for idx, b := range g.beatInfos {
			if b.NodeID != model.InvalidNodeID {
				g.nodeRows[b.NodeID] = 0
			}
			if b.NodeID == origin {
				g.originIdxsByRow[0] = append(g.originIdxsByRow[0], idx)
			}
		}
	}

	// Compute beat paths for additional drum rows using their origin nodes.
	for i, r := range g.drum.Rows {
		if i == 0 {
			// row 0 handled above; ensure its origin tracks the start node
			if r.Origin == model.InvalidNodeID && g.start != nil {
				g.drum.Rows[0].Origin = g.start.ID
				g.drum.Rows[0].Node = g.start
			}
			continue
		}
		if r.Origin == model.InvalidNodeID {
			continue
		}
		rowPath, rowLoop, rowStart := g.graph.CalculateBeatRowFrom(r.Origin)
		rowLen := rawBeatLen(rowPath, rowLoop, rowStart)
		g.beatInfosByRow[i] = rowPath[:rowLen]
		g.isLoopByRow[i] = rowLoop
		g.loopStartByRow[i] = rowStart
		if rowLoop {
			g.loopLenByRow[i] = loopSegmentLen(g.beatInfosByRow[i], rowStart)
		}
		origin := r.Origin
		for idx, b := range rowPath[:rowLen] {
			if b.NodeID != model.InvalidNodeID {
				if _, exists := g.nodeRows[b.NodeID]; !exists {
					g.nodeRows[b.NodeID] = i
				}
			}
			if b.NodeID == origin {
				g.originIdxsByRow[i] = append(g.originIdxsByRow[i], idx)
			}
		}
		if rowLen > maxLen {
			maxLen = rowLen
		}
	}

	// Reduce the graph's beat length to the actual maximum traversal size so
	// subsequent path calculations are not padded with extra loop cycles.
	g.graph.SetBeatLength(maxLen)

	g.resetOriginSequences()

	if maxLen > g.drum.Length {
		g.drum.SetLength(maxLen)
	} else {
		g.drum.SetBeatLength(maxLen)
	}

	// Rebind active pulses to the updated beat paths while preserving
	// their absolute progression indices.
	for _, p := range g.activePulses {
		if p.row >= len(g.beatInfosByRow) {
			continue
		}
		path := g.beatInfosByRow[p.row]
		if len(path) == 0 {
			continue
		}
		p.path = path
		absLast := g.nextBeatIdxs[p.row] - 1
		wrappedLast := g.wrapBeatIndexRow(p.row, absLast)
		wrappedNext := g.wrapBeatIndexRow(p.row, g.nextBeatIdxs[p.row])
		p.lastIdx = absLast
		p.fromBeatInfo = path[wrappedLast]
		p.toBeatInfo = path[wrappedNext]
		p.pathIdx = wrappedNext
		p.from = g.nodeByID(p.fromBeatInfo.NodeID)
		p.to = g.nodeByID(p.toBeatInfo.NodeID)
		if p.from != nil {
			p.x1, p.y1 = p.from.X, p.from.Y
		}
		if p.to != nil {
			p.x2, p.y2 = p.to.X, p.to.Y
		}
	}

	g.logger.Debugf("[GAME] updateBeatInfos: drum.Length=%d, beatPath=%d", g.drum.Length, len(g.beatInfos))

	// Preserve current drum offset when the beat path changes. Clamp to the
	// new valid range instead of resetting to zero so resizing the drum view
	// doesn't jump back to the origin.
	maxOffset := len(g.beatInfos) - g.drum.Length
	if maxOffset < 0 {
		maxOffset = 0
	}
	if g.drum.Offset > maxOffset {
		g.drum.Offset = maxOffset
	}

	g.refreshDrumRow()
}

func (g *Game) beatInfoAt(idx int) model.BeatInfo {
	if len(g.beatInfos) == 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	if idx < len(g.beatInfos) {
		return g.beatInfos[idx]
	}
	if !g.isLoop {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	loopLen := len(g.beatInfos) - g.loopStartIndex
	if loopLen <= 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	idx = g.loopStartIndex + (idx-g.loopStartIndex)%loopLen
	return g.beatInfos[idx]
}

func (g *Game) beatInfoAtRow(row, idx int) model.BeatInfo {
	if row < 0 || row >= len(g.beatInfosByRow) {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	infos := g.beatInfosByRow[row]
	if len(infos) == 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	if idx < len(infos) {
		return infos[idx]
	}
	if !g.isLoopByRow[row] {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	loopLen := len(infos) - g.loopStartByRow[row]
	if loopLen <= 0 {
		return model.BeatInfo{NodeID: model.InvalidNodeID, NodeType: model.NodeTypeInvisible, I: -1, J: -1}
	}
	idx = g.loopStartByRow[row] + (idx-g.loopStartByRow[row])%loopLen
	return infos[idx]
}

func (g *Game) wrapBeatIndex(idx int) int {
	if len(g.beatInfos) == 0 {
		return 0
	}
	if idx < len(g.beatInfos) {
		return idx
	}
	if !g.isLoop {
		return len(g.beatInfos) - 1
	}
	loopLen := len(g.beatInfos) - g.loopStartIndex
	if loopLen <= 0 {
		return len(g.beatInfos) - 1
	}
	return g.loopStartIndex + (idx-g.loopStartIndex)%loopLen
}

func (g *Game) wrapBeatIndexRow(row, idx int) int {
	if row < 0 || row >= len(g.beatInfosByRow) {
		return 0
	}
	infos := g.beatInfosByRow[row]
	if len(infos) == 0 {
		return 0
	}
	if idx < len(infos) {
		return idx
	}
	if !g.isLoopByRow[row] {
		return len(infos) - 1
	}
	loopLen := len(infos) - g.loopStartByRow[row]
	if loopLen <= 0 {
		return len(infos) - 1
	}
	return g.loopStartByRow[row] + (idx-g.loopStartByRow[row])%loopLen
}

func (g *Game) resetOriginSequences() {
	for row := range g.originIdxsByRow {
		positions := g.originIdxsByRow[row]
		seq := 0
		if len(positions) > 1 {
			beat := 0
			if row < len(g.nextBeatIdxs) {
				beat = g.nextBeatIdxs[row]
			}
			for i, idx := range positions {
				if beat <= idx {
					seq = i
					break
				}
			}
			if beat > positions[len(positions)-1] {
				seq = 0
			}
		}
		if row < len(g.nextOriginIdxByRow) {
			g.nextOriginIdxByRow[row] = seq
		}
	}
}

func (g *Game) refreshDrumRow() {
	if len(g.drum.Rows) == 0 {
		g.drumBeatInfos = nil
		return
	}

	g.drumBeatInfos = make([]model.BeatInfo, g.drum.Length)
	for rowIdx, r := range g.drum.Rows {
		r.Steps = make([]bool, g.drum.Length)
		for i := 0; i < g.drum.Length; i++ {
			info := g.beatInfoAtRow(rowIdx, g.drum.Offset+i)
			if rowIdx == 0 {
				g.drumBeatInfos[i] = info
			}
			r.Steps[i] = info.NodeType == model.NodeTypeRegular
		}
	}
	g.logger.Debugf("[GAME] refreshDrumRow: offset=%d", g.drum.Offset)
}

func (g *Game) addEdge(a, b *uiNode) {
	if !(a.I == b.I || a.J == b.J) { // only orthogonal
		return
	}
	for _, e := range g.edges { // no duplicates
		if (e.A == a && e.B == b) || (e.A == b && e.B == a) {
			return
		}
	}

	// Store original a and b for graph edge creation
	originalA, originalB := a, b

	// Add intermediate nodes
	if a.I == b.I { // Vertical edge
		if a.J > b.J {
			a, b = b, a // ensure a.J < b.J
		}
		for j := a.J + 1; j < b.J; j++ {
			g.tryAddNode(a.I, j, model.NodeTypeInvisible)
		}
	} else { // Horizontal edge
		if a.I > b.I {
			a, b = b, a // ensure a.I < b.I
		}
		for i := a.I + 1; i < b.I; i++ {
			g.tryAddNode(i, a.J, model.NodeTypeInvisible)
		}
	}

	g.edges = append(g.edges, uiEdge{A: originalA, B: originalB, t: 0, pulse: -1})
	g.graph.Edges[[2]model.NodeID{originalA.ID, originalB.ID}] = struct{}{}
	g.logger.Debugf("[GAME] Added edge: %d,%d -> %d,%d", originalA.I, originalA.J, originalB.I, originalB.J)
	if g.graph.StartNodeID != model.InvalidNodeID {
		g.updateBeatInfos()
	}
	g.computeSelNeighbors()
}

func (g *Game) deleteEdge(a, b *uiNode) {
	// Collect intermediate nodes to delete
	nodesToDelete := []*uiNode{}
	if a.I == b.I { // Vertical edge
		if a.J > b.J {
			a, b = b, a // ensure a.J < b.J
		}
		for j := a.J + 1; j < b.J; j++ {
			if n := g.nodeAt(a.I, j); n != nil && g.graph.Nodes[n.ID].Type == model.NodeTypeInvisible {
				nodesToDelete = append(nodesToDelete, n)
			}
		}
	} else { // Horizontal edge
		if a.I > b.I {
			a, b = b, a // ensure a.I < b.I
		}
		for i := a.I + 1; i < b.I; i++ {
			if n := g.nodeAt(i, a.J); n != nil && g.graph.Nodes[n.ID].Type == model.NodeTypeInvisible {
				nodesToDelete = append(nodesToDelete, n)
			}
		}
	}

	// Delete collected intermediate nodes
	for _, n := range nodesToDelete {
		g.deleteNode(n)
	}

	for i := 0; i < len(g.edges); {
		e := g.edges[i]
		if (e.A == a && e.B == b) || (e.A == b && e.B == a) {
			g.edges[i] = g.edges[len(g.edges)-1]
			g.edges = g.edges[:len(g.edges)-1]
		} else {
			i++
		}
	}
	delete(g.graph.Edges, [2]model.NodeID{a.ID, b.ID})
	g.logger.Debugf("[GAME] Deleted edge: %d,%d -> %d,%d", a.I, a.J, b.I, b.J)
	g.updateBeatInfos()
	g.computeSelNeighbors()
}

/* ─────────────── input handling ───────────────────────────────────────── */

func (g *Game) handleEditor() {
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	right := isMouseButtonPressed(ebiten.MouseButtonRight)
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)

	if g.split.dragging {
		g.pendingClick = false
		g.leftPrev = left
		return
	}

	// coords -> world
	x, y := cursorPosition()
	if y < topOffset || y >= g.split.Y {
		g.pendingClick = false
		g.leftPrev = left
		return
	}
	wx := (float64(x) - g.cam.OffsetX) / g.cam.Scale
	wy := (float64(y-topOffset) - g.cam.OffsetY) / g.cam.Scale
	gx, gy, i, j := g.grid.Snap(wx, wy)

	// ---------------- delete node (right-click) ----------------
	if right && !shift && !left {
		if n := g.nodeAt(i, j); n != nil {
			g.logger.Debugf("[GAME] Deleting node: %d at grid=(%d,%d)", n.ID, i, j)
			g.deleteNode(n)
		}
		return
	}

	// ---------------- link drag (shift held OR drag in progress) ----
	if g.linkDrag.active || shift {
		g.handleLinkDrag(left, right, gx, gy, i, j)
		return
	}

	// click handling based on press+release without drag
	if left && !g.leftPrev {
		g.clickI, g.clickJ = i, j
		g.pendingClick = true
		g.camDragged = false
		g.logger.Debugf("[GAME] Mouse down at screen=(%d, %d), grid=(%d, %d)", x, y, i, j)
	}
	if !left && g.leftPrev {
		if g.pendingClick && !g.camDragged {
			g.logger.Debugf("[GAME] Mouse up at screen=(%d, %d), grid=(%d, %d)", x, y, i, j)
			g.logger.Debugf("[GAME] Add/select node: %d,%d", g.clickI, g.clickJ)
			n := g.tryAddNode(g.clickI, g.clickJ, model.NodeTypeRegular)
			if g.sel != n {
				if g.sel != nil {
					g.logger.Debugf("[GAME] Deselecting node: %d,%d", g.sel.I, g.sel.J)
					g.sel.Selected = false
				}
				g.logger.Debugf("[GAME] Selecting node: %d,%d", n.I, n.J)
				g.sel = n
				n.Selected = true
				g.computeSelNeighbors()
			}
		}
		g.pendingClick = false
		g.camDragged = false
	}
	if isKeyPressed(ebiten.KeyS) && g.sel != nil {
		if g.start != nil {
			g.start.Start = false
			g.logger.Debugf("[GAME] Unsetting start node: %d,%d", g.start.I, g.start.J)
		}
		g.start = g.sel
		g.start.Start = true
		g.graph.StartNodeID = g.sel.ID
		g.logger.Infof("[GAME] Setting start node: %d,%d", g.start.I, g.start.J)
		g.updateBeatInfos()
	}
	g.leftPrev = left
}

// blocksAt reports whether any UI overlay blocks interaction at (x,y).
func (g *Game) blocksAt(x, y int) bool {
	if g.drum != nil && g.drum.BlocksAt(x, y) {
		return true
	}
	return false
}

func (g *Game) handleLinkDrag(left, right bool, gx, gy float64, i, j int) {
	shift := isKeyPressed(ebiten.KeyShiftLeft) ||
		isKeyPressed(ebiten.KeyShiftRight)

	// start drag
	if left && !g.linkDrag.active && shift {
		if n := g.nodeAt(i, j); n != nil {
			g.logger.Debugf("[GAME] Start link drag: node=%d at grid=(%d,%d)", n.ID, n.I, n.J)
			g.linkDrag = dragLink{from: n, active: true}
		}
	}
	// update preview
	if g.linkDrag.active && left {
		g.linkDrag.toX, g.linkDrag.toY = gx, gy
		return
	}
	// release → commit or delete
	if g.linkDrag.active && !left {
		if n2 := g.nodeAt(i, j); n2 != nil && n2 != g.linkDrag.from {
			if right {
				g.logger.Debugf("[GAME] Deleting edge: node=%d grid=(%d,%d) and node=%d grid=(%d,%d)", g.linkDrag.from.ID, g.linkDrag.from.I, g.linkDrag.from.J, n2.ID, n2.I, n2.J)
				g.deleteEdge(g.linkDrag.from, n2)
			} else {
				g.logger.Debugf("[GAME] Adding edge: node=%d grid=(%d,%d) and node=%d grid=(%d,%d)", g.linkDrag.from.ID, g.linkDrag.from.I, g.linkDrag.from.J, n2.ID, n2.I, n2.J)
				g.addEdge(g.linkDrag.from, n2)
			}
		}
		g.logger.Debugf("[GAME] End link drag at grid=(%d,%d)", i, j)
		g.linkDrag = dragLink{}
	}
}

func (g *Game) spawnPulseFromRow(row, start int) {
	g.logger.Debugf("[GAME] Spawn pulse for row %d from %d", row, start)
	if row < 0 || row >= len(g.beatInfosByRow) {
		return
	}
	path := g.beatInfosByRow[row]
	if len(path) == 0 {
		g.logger.Infof("[GAME] Spawn pulse: No beat information available for row %d", row)
		return
	}
	curIdxWrapped := g.wrapBeatIndexRow(row, start)
	beatDuration := int64(60.0 / float64(g.drum.bpm) * ebitenTPS)
	fromBeatInfo := path[curIdxWrapped]
	g.nextBeatIdxs[row] = start
	idx := g.nextBeatIdxs[row]
	g.highlightBeat(row, idx, fromBeatInfo, beatDuration)
	if row == 0 {
		g.elapsedBeats = idx
	}
	g.nextBeatIdxs[row] = idx + 1
	nextInfo := g.beatInfoAtRow(row, start+1)
	if nextInfo.NodeID != model.InvalidNodeID {
		nextIdxWrapped := g.wrapBeatIndexRow(row, start+1)
		unit := g.grid.Unit()
		x1 := float64(fromBeatInfo.I) * unit
		y1 := float64(fromBeatInfo.J) * unit
		x2 := float64(nextInfo.I) * unit
		y2 := float64(nextInfo.J) * unit
		dist := hypot(x2-x1, y2-y1)
		beats := dist / g.grid.Step
		if beats <= 0 {
			beats = 1
		}
		p := &pulse{
			x1:           x1,
			y1:           y1,
			x2:           x2,
			y2:           y2,
			speed:        1.0 / (float64(beatDuration) * beats),
			fromBeatInfo: fromBeatInfo,
			toBeatInfo:   nextInfo,
			pathIdx:      nextIdxWrapped,
			lastIdx:      start,
			from:         g.nodeByID(fromBeatInfo.NodeID),
			to:           g.nodeByID(nextInfo.NodeID),
			path:         path,
			row:          row,
		}
		g.activePulses = append(g.activePulses, p)
		if row == 0 {
			g.activePulse = p
		}
	} else if row == 0 {
		g.activePulse = nil
	}
}

// spawnPulseFrom is kept for compatibility with tests; it spawns a pulse for the
// primary drum row.
func (g *Game) spawnPulseFrom(start int) { g.spawnPulseFromRow(0, start) }

/* ─────────────── Update & tick ────────────────────────────────────────── */

func (g *Game) Update() error {
	// Process engine events without blocking. If playback is stopped,
	// drain any pending ticks without advancing the timeline so beat and
	// time counters freeze immediately when the user hits Stop.
	for {
		select {
		case evt := <-g.engine.Events:
			if g.playing {
				g.onTick(evt.Step)
			}
		default:
			goto eventsDone
		}
	}
eventsDone:
	// splitter
	g.split.Update(g.winH)
	g.drum.SetBounds(image.Rect(0, g.split.Y, g.winW, g.winH))

	// camera pan only when not dragging link or splitter
	mx, my := cursorPosition()
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	panOK := !g.linkDrag.active && !g.split.dragging && !shift && !pt(mx, my, g.drum.Bounds) && !g.drum.Capturing()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	drag := g.cam.HandleMouse(panOK)
	g.camDragging = drag
	if left && drag {
		g.camDragged = true
	}

	// editor interactions (skip when an overlay consumes the cursor)
	if !g.blocksAt(mx, my) {
		g.handleEditor()
	} else {
		g.leftPrev = left
	}

	if my >= topOffset && !g.blocksAt(mx, my) {
		wx := (float64(mx) - g.cam.OffsetX) / g.cam.Scale
		wy := (float64(my-topOffset) - g.cam.OffsetY) / g.cam.Scale
		_, _, i, j := g.grid.Snap(wx, wy)
		g.hover = g.nodeAt(i, j)
	} else {
		g.hover = nil
	}

	// edge animation progress
	for i := range g.edges {
		if g.edges[i].t < 1 {
			g.edges[i].t += 0.05
			if g.edges[i].t >= 1 {
				g.edges[i].t = 1
				g.edges[i].pulse = 0
			}
		} else if g.edges[i].pulse >= 0 {
			g.edges[i].pulse += 0.05
			if g.edges[i].pulse > 1 {
				g.edges[i].pulse = -1
			}
		}
	}
	if g.playing {
		g.frame++
	}

	if g.playing {
		for i := 0; i < len(g.activePulses); {
			p := g.activePulses[i]
			p.t += p.speed
			if p.t >= 1 {
				prevIdx := p.lastIdx
				delete(g.highlightedBeats, makeBeatKey(p.row, prevIdx))
				if !g.advancePulse(p) {
					g.logger.Infof("[GAME] Update: pulse for row %d removed", p.row)
					if p.row == 0 {
						g.activePulse = nil
					}
					g.activePulses = append(g.activePulses[:i], g.activePulses[i+1:]...)
					for key := range g.highlightedBeats {
						if r, _ := splitBeatKey(key); r == p.row {
							delete(g.highlightedBeats, key)
						}
					}
					continue
				}
			}
			i++
		}
	}

	// Clear expired highlights
	g.clearExpiredHighlights()

	// drum view logic
	prevPlaying := g.playing
	prevLen := g.drum.Length
	g.drum.Update()
	for _, idx := range g.drum.ConsumeAddedRows() {
		g.pendingStartRow = idx
	}
	for _, idx := range g.drum.ConsumeOriginRequests() {
		g.pendingStartRow = idx
	}
	deleted := g.drum.ConsumeDeletedRows()
	for _, dr := range deleted {
		if dr.origin != model.InvalidNodeID {
			if n := g.nodeByID(dr.origin); n != nil {
				g.deleteNode(n)
			} else {
				g.graph.RemoveNode(dr.origin)
			}
		}
		// drop playback indices for deleted row and shift later ones
		if dr.index < len(g.nextBeatIdxs) {
			g.nextBeatIdxs = append(g.nextBeatIdxs[:dr.index], g.nextBeatIdxs[dr.index+1:]...)
		}
		if g.pendingStartRow == dr.index {
			g.pendingStartRow = -1
		} else if g.pendingStartRow > dr.index {
			g.pendingStartRow--
		}
		// purge pulses belonging to this row and shift remaining indices
		out := g.activePulses[:0]
		for _, p := range g.activePulses {
			if p.row == dr.index {
				continue
			}
			if p.row > dr.index {
				p.row--
			}
			out = append(out, p)
		}
		g.activePulses = out
		if g.activePulse != nil {
			if g.activePulse.row == dr.index {
				g.activePulse = nil
			} else if g.activePulse.row > dr.index {
				g.activePulse.row--
			}
		}
	}
	if len(deleted) > 0 {
		g.updateBeatInfos()
	}

	if g.drum.PlayPressed() {
		if g.playing {
			g.playing = false
			g.paused = true
		} else if g.start != nil {
			audio.Resume()
			g.playing = true
		} else {
			g.logger.Warnf("[GAME] Play pressed but no start node; ignoring")
			g.playing = false
		}
	}
	if g.drum.StopPressed() {
		g.playing = false
		g.paused = false
	}
	g.bpm = g.drum.BPM()
	if g.bpm != g.appliedBPM {
		g.logger.Debugf("[GAME] queue BPM %d", g.bpm)
		sendLatest(g.bpmCh, g.bpm)
	}

	select {
	case applied := <-g.bpmAck:
		g.logger.Debugf("[GAME] ack BPM %d", applied)
		ratio := float64(applied) / float64(g.appliedBPM)
		beatDuration := int64(60.0 / float64(applied) * ebitenTPS)
		for _, p := range g.activePulses {
			p.t *= ratio
			if p.t >= 1 {
				p.t = 0.999999
			}
			p.speed = 1.0 / float64(beatDuration)
		}
		g.appliedBPM = applied
	default:
	}

	if g.playing != prevPlaying {
		g.logger.Infof("[GAME] Playing state changed: %t -> %t", prevPlaying, g.playing)
		if g.playing {
			if !g.paused {
				for i := range g.nextBeatIdxs {
					g.nextBeatIdxs[i] = 0
				}
				g.resetOriginSequences()
				g.elapsedBeats = 0
				g.activePulses = nil
				g.activePulse = nil
				g.highlightedBeats = map[int]int64{}
			}
			g.engine.Start()
			g.paused = false
			g.logger.Infof("[GAME] Engine started.")
		} else {
			g.engine.Stop()
			g.logger.Infof("[GAME] Engine stopped.")
			for len(g.audioCh) > 0 {
				<-g.audioCh
			}
			if !g.paused {
				g.elapsedBeats = 0
				g.activePulses = nil
				g.activePulse = nil
				g.highlightedBeats = map[int]int64{}
			}
		}
		g.drum.SetPlaying(g.playing)
	}

	if !g.playing && !g.paused {
		g.logger.Infof("[GAME] Update: stopping playback, removing active pulses.")
		g.activePulses = nil
		g.activePulse = nil
	}

	if g.playing {
		g.drum.TrackBeat(g.elapsedBeats)
	}
	if g.drum.OffsetChanged() {
		g.refreshDrumRow()
	}
	if prevLen != g.drum.Length {
		maxOffset := len(g.beatInfos) - g.drum.Length
		if maxOffset < 0 {
			maxOffset = 0
		}
		if g.drum.Offset > maxOffset {
			g.drum.Offset = maxOffset
		}
		g.refreshDrumRow()
	}
	g.reportStateJS()
	return nil
}

/* ─────────────── Draw ─────────────────────────────────────────────────── */

func (g *Game) Draw(screen *ebiten.Image) {
	g.drawGridPane(screen) // top
	g.drawDrumPane(screen) // bottom (includes buttons)
}

func (g *Game) drawGridPane(screen *ebiten.Image) {
	top := screen.SubImage(image.Rect(0, 0, g.winW, g.split.Y)).(*ebiten.Image)
	top.Fill(colBGTop)

	// camera matrix for world drawings (shift down by bar height)
	unitPx := g.grid.UnitPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	camScale := unitPx / g.grid.Unit()
	var cam ebiten.GeoM
	cam.Scale(camScale, camScale)
	cam.Translate(offX, offY+float64(topOffset))

	// grid lattice computed in world coordinates then transformed
	minX, maxX, minY, maxY := visibleWorldRect(g.cam, g.winW, g.split.Y)
	groups := g.grid.Lines(g.cam, g.winW, g.split.Y)
	for _, lg := range groups {
		// convert desired pixel width to world units so screen thickness stays constant
		w := lg.Subdiv.Style.Width / camScale
		for _, x := range lg.Xs {
			DrawLineCam(screen, x, minY, x, maxY, &cam, lg.Subdiv.Style.Color, w)
		}
		for _, y := range lg.Ys {
			DrawLineCam(screen, minX, y, maxX, y, &cam, lg.Subdiv.Style.Color, w)
		}
	}
	var id ebiten.GeoM

	// edges with connection animation
	sigStyle := SignalUI
	sigStyle.Radius = float32(g.grid.SignalRadius(g.cam.Scale))
	edgeThick := g.grid.EdgeThickness(g.cam.Scale)
	arrow := g.grid.EdgeArrowSize()
	for i := range g.edges {
		e := &g.edges[i]
		edgeStyle := EdgeUI
		edgeStyle.Thickness = edgeThick
		edgeStyle.ArrowSize = arrow
		if row, ok := g.nodeRows[e.A.ID]; ok && row >= 0 && row < len(g.drum.Rows) {
			base := g.drum.Rows[row].Color
			edgeStyle.Color = adjustColor(base, 80)
		}
		edgeStyle.DrawProgress(screen, e.A.X, e.A.Y, e.B.X, e.B.Y, &cam, e.t)
		if e.pulse >= 0 {
			px := e.A.X + (e.B.X-e.A.X)*e.pulse
			py := e.A.Y + (e.B.Y-e.A.Y)*e.pulse
			sigStyle.Color = edgeStyle.Color
			sigStyle.Draw(screen, px, py, &cam)
		}
	}

	// link preview
	if g.linkDrag.active {
		edgeStyle := EdgeUI
		edgeStyle.Thickness = edgeThick
		edgeStyle.ArrowSize = arrow
		if row, ok := g.nodeRows[g.linkDrag.from.ID]; ok && row >= 0 && row < len(g.drum.Rows) {
			base := g.drum.Rows[row].Color
			edgeStyle.Color = adjustColor(base, 80)
		}
		edgeStyle.Draw(screen, g.linkDrag.from.X, g.linkDrag.from.Y,
			g.linkDrag.toX, g.linkDrag.toY, &cam)
	}

	// nodes
	nodeStyle := NodeUI
	for _, n := range g.nodes {
		nodeInfo, ok := g.graph.Nodes[n.ID]
		if !ok || nodeInfo.Type != model.NodeTypeRegular {
			continue
		}
		style := nodeStyle
		style.Radius = float32(g.nodeRadius(n))
		if row, ok := g.nodeRows[n.ID]; ok {
			if row >= 0 && row < len(g.drum.Rows) {
				base := g.drum.Rows[row].Color
				if n.Start {
					style.Fill = adjustColor(base, 40)
				} else {
					style.Fill = base
				}
				style.Border = adjustColor(base, 80)
			}
		}
		style.Draw(screen, n.X, n.Y, &cam)
		x1, y1, x2, y2 := g.nodeScreenRect(n)
		var id ebiten.GeoM
		if g.sel == n {
			DrawLineCam(screen, x1, y1, x2, y1, &id, colHighlight, 2)
			DrawLineCam(screen, x2, y1, x2, y2, &id, colHighlight, 2)
			DrawLineCam(screen, x2, y2, x1, y2, &id, colHighlight, 2)
			DrawLineCam(screen, x1, y2, x1, y1, &id, colHighlight, 2)
		} else if g.selNeighbors != nil && g.selNeighbors[n] {
			hl := fadeColor(colHighlight, 0.5)
			DrawLineCam(screen, x1, y1, x2, y1, &id, hl, 2)
			DrawLineCam(screen, x2, y1, x2, y2, &id, hl, 2)
			DrawLineCam(screen, x2, y2, x1, y2, &id, hl, 2)
			DrawLineCam(screen, x1, y2, x1, y1, &id, hl, 2)
		}
	}

	// pulses
	g.renderedPulsesCount = 0
	for _, p := range g.activePulses {
		px := p.x1 + (p.x2-p.x1)*p.t
		py := p.y1 + (p.y2-p.y1)*p.t
		col := SignalUI.Color
		if p.row >= 0 && p.row < len(g.drum.Rows) {
			base := g.drum.Rows[p.row].Color
			col = adjustColor(base, 80)
		}
		DrawLineCam(screen, p.x1, p.y1, px, py, &cam, fadeColor(col, 0.6), edgeThick)
		sigStyle.Color = col
		sigStyle.Draw(screen, px, py, &cam)
		g.renderedPulsesCount++
	}

	// cursor coordinate label
	mx, my := cursorPosition()
	if my < g.split.Y {
		camScale := unitPx / g.grid.Unit()
		wx := (float64(mx) - offX) / camScale
		wy := (float64(my) - offY - float64(topOffset)) / camScale
		_, _, ix, iy := g.grid.Snap(wx, wy)
		bx, nx, dx := g.grid.BeatSubdivision(ix)
		by, ny, dy := g.grid.BeatSubdivision(iy)
		xs := "0"
		ys := "0"
		if nx != 0 {
			xs = fmt.Sprintf("%d/%d", nx, dx)
		}
		if ny != 0 {
			ys = fmt.Sprintf("%d/%d", ny, dy)
		}
		g.cursorLabel = fmt.Sprintf("(%d:%s, %d:%s)", bx, xs, by, ys)
		ebitenutil.DebugPrintAt(screen, g.cursorLabel, mx+8, my+16)
	} else {
		g.cursorLabel = ""
	}

	// splitter line
	DrawLineCam(screen,
		0, float64(g.split.Y),
		float64(g.winW), float64(g.split.Y),
		&id, color.RGBA{90, 90, 90, 255}, 2)
}

func (g *Game) drawDrumPane(dst *ebiten.Image) {
	g.drum.Draw(dst, g.highlightedBeats, g.frame, g.drumBeatInfos, g.currentBeat())
}

func (g *Game) currentBeat() float64 {
	frac := float64(g.elapsedBeats)
	if g.playing && g.engineProgress != nil {
		frac += g.engineProgress()
	}
	return frac
}

func (g *Game) rootNode() *uiNode {
	if g.start != nil {
		return g.start
	}
	var root *uiNode
	for _, n := range g.nodes {
		if n.J != 0 {
			continue
		}
		inbound := false
		for _, e := range g.edges {
			if e.B == n {
				inbound = true
				break
			}
		}
		if !inbound {
			if root == nil || n.I < root.I {
				root = n
			}
		}
	}
	return root
}

func (g *Game) pulseForRow(row int) *pulse {
	for _, p := range g.activePulses {
		if p.row == row {
			return p
		}
	}
	return nil
}

func (g *Game) onTick(step int) {
	g.currentStep = step

	if step == 0 {
		for row := range g.drum.Rows {
			if g.pulseForRow(row) == nil {
				if row == 0 && g.start == nil {
					continue
				}
				if row > 0 && g.drum.Rows[row].Origin == model.InvalidNodeID {
					continue
				}
				g.spawnPulseFromRow(row, g.nextBeatIdxs[row])
			}
		}
	}
}

func (g *Game) highlightBeat(row, idx int, info model.BeatInfo, duration int64) {
	key := makeBeatKey(row, idx)
	g.highlightedBeats[key] = g.frame + duration
	if info.NodeType == model.NodeTypeRegular {
		inst := "snare"
		vol := 1.0
		if row < len(g.drum.Rows) {
			inst = g.drum.Rows[row].Instrument
			vol = g.drum.Rows[row].Volume
			anySolo := false
			for _, r := range g.drum.Rows {
				if r.Solo {
					anySolo = true
					break
				}
			}
			if g.drum.Rows[row].Muted || (anySolo && !g.drum.Rows[row].Solo) {
				g.logger.Debugf("[GAME] highlightBeat: muted row %d", row)
				return
			}
		}
		g.queueSound(inst, vol)
		g.logger.Debugf("[GAME] highlightBeat: Played %s at vol %.2f for node %d at beat %d row %d", inst, vol, info.NodeID, idx, row)
	}
}

func (g *Game) queueSound(id string, vol float64) {
	req := soundReq{id: id, vol: vol, when: []float64{audio.Now()}}
	select {
	case g.audioCh <- req:
	default:
	}
}

func (g *Game) clearExpiredHighlights() {
	for key, until := range g.highlightedBeats {
		if g.frame > until {
			delete(g.highlightedBeats, key)
			row, idx := splitBeatKey(key)
			g.logger.Debugf("[GAME] Cleared expired highlight for beat %d row %d. highlightedBeats: %v", idx, row, g.highlightedBeats)
		}
	}
}

func (g *Game) Seek(beats int) {
	if beats < 0 {
		beats = 0
	}
	g.highlightedBeats = map[int]int64{}
	g.activePulses = nil
	g.activePulse = nil
	if g.playing {
		for row := range g.drum.Rows {
			g.spawnPulseFromRow(row, beats)
		}
	} else {
		for i := range g.nextBeatIdxs {
			g.nextBeatIdxs[i] = beats
		}
		g.elapsedBeats = beats
	}
	g.resetOriginSequences()
}

func (g *Game) advancePulse(p *pulse) bool {
	beatDuration := int64(60.0 / float64(g.bpm) * ebitenTPS)

	// The pulse has arrived at p.toBeatInfo. Highlight it.
	arrivalBeatInfo := p.toBeatInfo
	arrivalPathIdx := p.pathIdx

	g.logger.Debugf("[GAME] advancePulse: Pulse arrived at beat index %d: %+v", arrivalPathIdx, arrivalBeatInfo)
	if p.row < len(g.drum.Rows) {
		origin := g.drum.Rows[p.row].Origin
		if origin != model.InvalidNodeID && arrivalBeatInfo.NodeID == origin &&
			p.row < len(g.nextOriginIdxByRow) && p.row < len(g.originIdxsByRow) {

			seq := g.nextOriginIdxByRow[p.row]
			positions := g.originIdxsByRow[p.row]
			expectedIdx := 0
			if seq < len(positions) {
				expectedIdx = positions[seq]
			}
			if arrivalPathIdx != expectedIdx {
				g.logger.Errorf("pulse jumped to origin out of order: row=%d idx=%d expected=%d", p.row, arrivalPathIdx, expectedIdx)
				panic(fmt.Sprintf("pulse jumped to origin out of order: row=%d idx=%d expected=%d", p.row, arrivalPathIdx, expectedIdx))
			}
			seq++
			if seq >= len(positions) {
				seq = 0
			}
			g.nextOriginIdxByRow[p.row] = seq
		}
	}

	idx := g.nextBeatIdxs[p.row]
	g.highlightBeat(p.row, idx, arrivalBeatInfo, beatDuration)
	p.lastIdx = idx
	if p.row == 0 {
		g.elapsedBeats = idx
	}
	g.nextBeatIdxs[p.row] = idx + 1

	// Advance pathIdx for the *next* pulse segment
	p.pathIdx++

	// If the end of the path is reached, check for a loop.
	path := p.path
	if p.pathIdx >= len(path) {
		if g.isLoopByRow[p.row] {
			p.pathIdx = g.loopStartByRow[p.row]
		} else {
			return false
		}
	}

	// Set up the next segment of the pulse's journey.
	prevIdx := p.pathIdx - 1
	if prevIdx < 0 {
		if g.isLoopByRow[p.row] {
			prevIdx = len(path) - 1
		} else {
			return false
		}
	}
	if g.isLoopByRow[p.row] && p.pathIdx == g.loopStartByRow[p.row] {
		prevIdx = len(path) - 1
	}
	p.fromBeatInfo = path[prevIdx]
	p.toBeatInfo = path[p.pathIdx]
	p.from = g.nodeByID(p.fromBeatInfo.NodeID)
	p.to = g.nodeByID(p.toBeatInfo.NodeID)

	if p.from == nil || p.to == nil {
		return false
	}

	// Set pulse start and end coordinates for animation
	p.x1 = p.from.X
	p.y1 = p.from.Y
	p.x2 = p.to.X
	p.y2 = p.to.Y
	dist := hypot(p.x2-p.x1, p.y2-p.y1)
	beats := dist / g.grid.Step
	if beats <= 0 {
		beats = 1
	}
	p.speed = 1.0 / (float64(beatDuration) * beats)
	p.t = 0 // Reset animation progress

	return true
}

func visibleWorldRect(cam *Camera, screenW, screenH int) (minX, maxX, minY, maxY float64) {
	minX = (-cam.OffsetX) / cam.Scale
	maxX = (float64(screenW) - cam.OffsetX) / cam.Scale
	minY = (-cam.OffsetY - float64(topOffset)) / cam.Scale
	maxY = (float64(screenH) - cam.OffsetY - float64(topOffset)) / cam.Scale
	return
}

/* ─────────────── math helpers ─────────────────────────────────────────── */

func atan2(y, x float64) float64 { return math.Atan2(y, x) }
func hypot(a, b float64) float64 { return math.Hypot(a, b) }
func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}
