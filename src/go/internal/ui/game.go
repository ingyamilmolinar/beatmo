package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/beat"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

const (
	topOffset = 40 // transport-bar height in px
)

const ebitenTPS = 60 // Ticks per second for Ebiten (stubbed for tests)

// playSound plays a synthesized sound. Overridden in tests.
var playSound = audio.Play
var resetAudio = audio.Reset

/* ───────────────────────── data types ───────────────────────── */

type uiNode struct {
	ID       model.NodeID
	I, J     int     // grid indices
	X, Y     float64 // cached world coords (GridStep*I, GridStep*J)
	Selected bool
	Start    bool
	path     []model.NodeID // Path taken by the pulse to reach this node
}

func (n *uiNode) Bounds(scale float64) (x1, y1, x2, y2 float64) {
	halfSize := float64(NodeSpriteSize) / 2.0 * scale
	return n.X - halfSize, n.Y - halfSize, n.X + halfSize, n.Y + halfSize
}

type uiEdge struct{ A, B *uiNode }

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
	pathIdx                  int
	lastIdx                  int
}

type Game struct {
	/* subsystems */
	cam    *Camera
	split  *Splitter
	drum   *DrumView
	graph  *model.Graph
	sched  *beat.Scheduler
	logger *game_log.Logger

	/* graph data */
	nodes []*uiNode
	edges []uiEdge

	/* visuals */
	activePulse         *pulse
	frame               int64
	renderedPulsesCount int
	highlightedBeats    map[int]int64 // Map of beat index to frame number until which it's highlighted

	/* editor state */
	sel            *uiNode
	linkDrag       dragLink
	camDragging    bool
	camDragged     bool
	leftPrev       bool
	pendingClick   bool
	clickI, clickJ int

	/* game state */
	playing        bool
	bpm            int
	currentStep    int // Current step in the sequence
	lastBeatFrame  int64
	beatInfos      []model.BeatInfo // Full traversal path
	drumBeatInfos  []model.BeatInfo // BeatInfos sized to drum view
	isLoop         bool             // Indicates if the current graph forms a loop
	loopStartIndex int              // The index in beatInfos where the loop segment begins
	nextBeatIdx    int              // Absolute beat index for highlighting across loops
	audioStart     float64          // audioCtx.currentTime at playback start

	/* misc */
	winW, winH int
	start      *uiNode // explicit “root/start” node (⇧S to set)
}

/* ───────────────── helper: node’s screen rect ───────────────── */

// Rectangle in *screen* pixels (y already includes the transport offset).
func (g *Game) nodeScreenRect(n *uiNode) (x1, y1, x2, y2 float64) {
	stepPx := StepPixels(g.cam.Scale)               // grid step in screen px
	camScale := float64(stepPx) / float64(GridStep) // world→screen factor
	offX := math.Round(g.cam.OffsetX)               // camera panning
	offY := math.Round(g.cam.OffsetY)

	sx := offX + float64(stepPx*n.I)             // sprite centre X
	sy := offY + float64(stepPx*n.J) + topOffset // sprite centre Y
	size := float64(NodeSpriteSize) * camScale
	half := size / 2

	return sx - half, sy - half, sx + half, sy + half
}

/* ───────────────────── constructor & layout ─────────────────── */

func New(logger *game_log.Logger) *Game {
	g := &Game{
		cam:              NewCamera(),
		logger:           logger,
		graph:            model.NewGraph(logger),
		sched:            beat.NewScheduler(),
		split:            NewSplitter(720), // real height set in Layout below
		highlightedBeats: make(map[int]int64),
		bpm:              120, // Default BPM
		beatInfos:        []model.BeatInfo{},
		drumBeatInfos:    []model.BeatInfo{},
	}

	// bottom drum-machine view
	g.drum = NewDrumView(image.Rect(0, 600, 1280, 720), g.graph, logger)

	g.sched.OnTick = g.onTick
	return g
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

func (g *Game) tryAddNode(i, j int, nodeType model.NodeType) *uiNode {
	if n := g.nodeAt(i, j); n != nil {
		// If there's an invisible node here and we want a regular node,
		// upgrade the existing node rather than blocking the placement.
		if nodeType == model.NodeTypeRegular {
			if node, ok := g.graph.GetNodeByID(n.ID); ok && node.Type == model.NodeTypeInvisible {
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
	n := &uiNode{ID: id, I: i, J: j, X: float64(i * GridStep), Y: float64(j * GridStep)}

	if nodeType == model.NodeTypeRegular {
		if g.start == nil { // first ever node becomes the start
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
	// Traverse the graph once to capture the full path regardless of the
	// current drum view length.
	// NodeID has underlying type int; cast is safe for beat-length updates.
	g.graph.SetBeatLength(int(g.graph.Next))
	fullBeatRow, isLoop, loopStart := g.graph.CalculateBeatRow()

	baseLen := 0
	for _, b := range fullBeatRow {
		if b.NodeID == model.InvalidNodeID {
			break
		}
		baseLen++
	}

	g.beatInfos = fullBeatRow[:baseLen]
	g.isLoop = isLoop
	g.loopStartIndex = loopStart

	if baseLen > g.drum.Length {
		g.drum.Length = baseLen
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

func (g *Game) refreshDrumRow() {
	g.drumBeatInfos = make([]model.BeatInfo, g.drum.Length)
	for i := 0; i < g.drum.Length; i++ {
		g.drumBeatInfos[i] = g.beatInfoAt(g.drum.Offset + i)
	}
	drumRow := make([]bool, g.drum.Length)
	for i := range drumRow {
		drumRow[i] = g.drumBeatInfos[i].NodeType == model.NodeTypeRegular
	}
	g.drum.Rows[0].Steps = drumRow
	g.logger.Debugf("[GAME] refreshDrumRow: offset=%d row=%v", g.drum.Offset, drumRow)
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

	g.edges = append(g.edges, uiEdge{originalA, originalB})
	g.graph.Edges[[2]model.NodeID{originalA.ID, originalB.ID}] = struct{}{}
	g.logger.Debugf("[GAME] Added edge: %d,%d -> %d,%d", originalA.I, originalA.J, originalB.I, originalB.J)
	if g.graph.StartNodeID != model.InvalidNodeID {
		g.updateBeatInfos()
	}
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
	gx, gy, i, j := Snap(wx, wy)

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

func (g *Game) spawnPulse() {
	g.logger.Debugf("[GAME] Spawn pulse: attempting to start initial pulse")

	if len(g.beatInfos) == 0 {
		g.logger.Infof("[GAME] Spawn pulse: No beat information available, no initial pulse spawned.")
		return
	}

	beatDuration := int64(60.0 / float64(g.drum.bpm) * ebitenTPS)

	// The first beat in the sequence is where the pulse starts.
	fromBeatInfo := g.beatInfos[0]

	// Reset global beat index and highlight the first beat.
	g.nextBeatIdx = 0
	g.highlightBeat(g.nextBeatIdx, fromBeatInfo, beatDuration)
	g.nextBeatIdx++

	// If there's a next beat, create a pulse moving towards it.
	if len(g.beatInfos) > 1 {
		toBeatInfo := g.beatInfos[1]
		g.activePulse = &pulse{
			x1:           float64(fromBeatInfo.I * GridStep),
			y1:           float64(fromBeatInfo.J * GridStep),
			x2:           float64(toBeatInfo.I * GridStep),
			y2:           float64(toBeatInfo.J * GridStep),
			speed:        1.0 / float64(beatDuration),
			fromBeatInfo: fromBeatInfo,
			toBeatInfo:   toBeatInfo,
			pathIdx:      1, // Start at index 1, as we've already processed index 0.
			lastIdx:      0,
			from:         g.nodeByID(fromBeatInfo.NodeID),
			to:           g.nodeByID(toBeatInfo.NodeID),
		}
		g.logger.Infof("[GAME] Spawn pulse: initial pulse spawned from grid (%d,%d) to (%d,%d) (beat types: %v -> %v)", fromBeatInfo.I, fromBeatInfo.J, toBeatInfo.I, toBeatInfo.J, fromBeatInfo.NodeType, toBeatInfo.NodeType)
	} else {
		// If there's only one beat, there's no pulse to animate.
		g.logger.Infof("[GAME] Spawn pulse: Only one beat, no pulse to animate.")
	}
}

/* ─────────────── Update & tick ────────────────────────────────────────── */

func (g *Game) Update() error {
	g.logger.Debugf("[GAME] Update start: frame=%d playing=%t bpm=%d currentStep=%d", g.frame, g.playing, g.bpm, g.currentStep)
	// splitter
	g.split.Update()
	g.drum.SetBounds(image.Rect(0, g.split.Y, g.winW, g.winH))

	// camera pan only when not dragging link or splitter
	mx, my := cursorPosition()
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	panOK := !g.linkDrag.active && !g.split.dragging && !shift && !pt(mx, my, g.drum.Bounds)
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	drag := g.cam.HandleMouse(panOK)
	g.camDragging = drag
	if left && drag {
		g.camDragged = true
	}

	// editor interactions
	g.handleEditor()
	g.frame++

	if g.activePulse != nil {
		g.logger.Debugf("[GAME] Update: processing active pulse: t=%.2f, fromBeatInfo=%+v, toBeatInfo=%+v", g.activePulse.t, g.activePulse.fromBeatInfo, g.activePulse.toBeatInfo)
		g.activePulse.t += g.activePulse.speed
		if g.activePulse.t >= 1 {
			// Clear highlight for the beat we just left
			prevIdx := g.activePulse.lastIdx
			delete(g.highlightedBeats, prevIdx)
			g.logger.Debugf("[GAME] Cleared highlight for beat index %d. highlightedBeats: %v", prevIdx, g.highlightedBeats)

			if !g.advancePulse(g.activePulse) {
				g.logger.Infof("[GAME] Update: active pulse removed.")
				g.activePulse = nil
				// Clear all highlights when the active pulse is removed
				for idx := range g.highlightedBeats {
					delete(g.highlightedBeats, idx)
				}
			}
		}
	}

	// Clear expired highlights
	g.clearExpiredHighlights()
	g.logger.Debugf("[GAME] Current highlightedBeats: %v", g.highlightedBeats)

	// drum view logic
	prevPlaying := g.playing
	prevLen := g.drum.Length
	prevBPM := g.bpm
	g.drum.Update()
	if g.drum.OffsetChanged() {
		g.refreshDrumRow()
	}

	if g.drum.PlayPressed() {
		g.playing = true
	}
	if g.drum.StopPressed() {
		g.playing = false
	}
	g.bpm = g.drum.BPM()

	if g.bpm != prevBPM {
		g.logger.Debugf("[GAME] BPM changed from %d to %d", prevBPM, g.bpm)
		resetAudio()
		if g.activePulse != nil {
			beatDuration := int64(60.0 / float64(g.bpm) * ebitenTPS)
			g.activePulse.speed = 1.0 / float64(beatDuration)
			g.logger.Debugf("[GAME] BPM changed to %d, updated active pulse speed to %f", g.bpm, g.activePulse.speed)
		}
	}

	if g.playing != prevPlaying {
		g.logger.Infof("[GAME] Playing state changed: %t -> %t", prevPlaying, g.playing)
		if g.playing {
			g.audioStart = audio.Now()
			g.nextBeatIdx = 0
			g.sched.Start()
			g.logger.Infof("[GAME] Scheduler started.")
		} else {
			g.sched.Stop()
			g.logger.Infof("[GAME] Scheduler stopped.")
		}
	}

	if g.playing {
		g.sched.BPM = g.bpm
		g.sched.Tick()
		g.logger.Debugf("[GAME] Scheduler ticked. BPM: %d", g.bpm)
	} else {
		// Stop playback: remove all pulses
		g.logger.Infof("[GAME] Update: stopping playback, removing active pulse.")
		g.activePulse = nil
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
	g.logger.Debugf("[GAME] Update end. Frame: %d", g.frame)
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
	stepPx := StepPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	camScale := float64(stepPx) / float64(GridStep)
	var cam ebiten.GeoM
	cam.Scale(camScale, camScale)
	cam.Translate(offX, offY+float64(topOffset))

	frame := (g.frame / int64(6)) % int64(len(NodeFrames))

	// grid lattice computed in world coordinates then transformed
	minX, maxX, minY, maxY := visibleWorldRect(g.cam, g.winW, g.split.Y)
	startI := int(math.Floor(minX / GridStep))
	endI := int(math.Ceil(maxX / GridStep))
	startJ := int(math.Floor(minY / GridStep))
	endJ := int(math.Ceil(maxY / GridStep))

	var id ebiten.GeoM
	for i := startI; i <= endI; i++ {
		x := float64(i * GridStep)
		DrawLineCam(screen, x, minY, x, maxY, &cam, colGridLine, 1)
	}
	for j := startJ; j <= endJ; j++ {
		y := float64(j * GridStep)
		DrawLineCam(screen, minX, y, maxX, y, &cam, colGridLine, 1)
	}

	// edges
	for _, e := range g.edges {
		DrawLineCam(screen, e.A.X, e.A.Y, e.B.X, e.B.Y,
			&cam, color.White, 2)
	}

	// link preview
	if g.linkDrag.active {
		DrawLineCam(screen, g.linkDrag.from.X, g.linkDrag.from.Y,
			g.linkDrag.toX, g.linkDrag.toY,
			&cam, color.RGBA{200, 200, 200, 255}, 2)
	}

	// nodes
	for _, n := range g.nodes {
		nodeInfo, ok := g.graph.Nodes[n.ID]
		if !ok || nodeInfo.Type != model.NodeTypeRegular {
			continue
		}
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(-float64(NodeSpriteSize)/2, -float64(NodeSpriteSize)/2)
		op.GeoM.Translate(n.X, n.Y)
		op.GeoM.Concat(cam)
		screen.DrawImage(NodeFrames[frame], op)
		if n.Start {
			x1, y1, x2, y2 := g.nodeScreenRect(n)
			var id ebiten.GeoM
			DrawLineCam(screen, x1, y1, x2, y1, &id, color.RGBA{0, 255, 0, 255}, 2)
			DrawLineCam(screen, x2, y1, x2, y2, &id, color.RGBA{0, 255, 0, 255}, 2)
			DrawLineCam(screen, x2, y2, x1, y2, &id, color.RGBA{0, 255, 0, 255}, 2)
			DrawLineCam(screen, x1, y2, x1, y1, &id, color.RGBA{0, 255, 0, 255}, 2)
		}
	}

	// pulses
	g.renderedPulsesCount = 0
	if g.activePulse != nil {
		p := g.activePulse
		px := p.x1 + (p.x2-p.x1)*p.t
		py := p.y1 + (p.y2-p.y1)*p.t
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(px-8, py-8)
		op.GeoM.Concat(cam)
		screen.DrawImage(SignalDot, op)
		g.renderedPulsesCount = 1
	}

	// splitter line
	DrawLineCam(screen,
		0, float64(g.split.Y),
		float64(g.winW), float64(g.split.Y),
		&id, color.RGBA{90, 90, 90, 255}, 2)
}

func (g *Game) drawDrumPane(dst *ebiten.Image) {
	vis := map[int]int64{}
	for idx, fr := range g.highlightedBeats {
		if idx >= g.drum.Offset && idx < g.drum.Offset+g.drum.Length {
			vis[idx-g.drum.Offset] = fr
		}
	}
	g.drum.Draw(dst, vis, g.frame, g.drumBeatInfos)
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

func (g *Game) onTick(step int) {
	g.logger.Debugf("[GAME] On tick: step %d", step)
	g.currentStep = step

	// The scheduler ticks every beat. We only care about the first beat (step 0)
	// to potentially start a pulse if one isn't already active.
	if step == 0 && g.activePulse == nil {
		if g.start != nil {
			g.spawnPulse()
		}
	}
}

func (g *Game) nodeByID(id model.NodeID) *uiNode {
	for _, n := range g.nodes {
		if n.ID == id {
			return n
		}
	}
	if node, ok := g.graph.Nodes[id]; ok {
		return &uiNode{ID: id, I: node.I, J: node.J, X: float64(node.I * GridStep), Y: float64(node.J * GridStep)}
	}
	return nil
}

func (g *Game) highlightBeat(idx int, info model.BeatInfo, duration int64) {
	g.highlightedBeats[idx] = g.frame + duration
	if info.NodeType == model.NodeTypeRegular {
		spb := 60.0 / float64(g.bpm)
		if g.audioStart == 0 {
			g.audioStart = audio.Now()
		}
		when := g.audioStart + float64(idx)*spb
		playSound("snare", when)
		g.logger.Debugf("[GAME] highlightBeat: Played sample for node %d at beat %d (when=%f)", info.NodeID, idx, when)
	}
}

func (g *Game) clearExpiredHighlights() {
	for idx, until := range g.highlightedBeats {
		if g.frame > until {
			delete(g.highlightedBeats, idx)
			g.logger.Debugf("[GAME] Cleared expired highlight for beat %d. highlightedBeats: %v", idx, g.highlightedBeats)
		}
	}
}

func (g *Game) advancePulse(p *pulse) bool {
	beatDuration := int64(60.0 / float64(g.bpm) * ebitenTPS)

	// The pulse has arrived at p.toBeatInfo. Highlight it.
	arrivalBeatInfo := p.toBeatInfo
	arrivalPathIdx := p.pathIdx

	g.logger.Debugf("[GAME] advancePulse: Pulse arrived at beat index %d: %+v", arrivalPathIdx, arrivalBeatInfo)

	g.logger.Debugf("[GAME] advancePulse: Highlighting beat index %d. Type: %v, ID: %v", g.nextBeatIdx, arrivalBeatInfo.NodeType, arrivalBeatInfo.NodeID)
	g.highlightBeat(g.nextBeatIdx, arrivalBeatInfo, beatDuration)
	p.lastIdx = g.nextBeatIdx
	g.nextBeatIdx++

	// Advance pathIdx for the *next* pulse segment
	p.pathIdx++

	// If the end of the path is reached, check for a loop.
	if p.pathIdx >= len(g.beatInfos) {
		if g.isLoop {
			// If it's a loop, reset pathIdx to the beginning of the loop segment.
			p.pathIdx = g.loopStartIndex
			g.logger.Debugf("[GAME] advancePulse: Loop detected, resetting pathIdx to %d for continuous playback.", p.pathIdx)
		} else {
			g.logger.Debugf("[GAME] advancePulse: End of path, no loop. Returning false.")
			return false // End of path
		}
	}

	g.logger.Debugf("[GAME] advancePulse: Incremented pathIdx to %d. beatPath length: %d", p.pathIdx, len(g.beatInfos))

	// Set up the next segment of the pulse's journey.
	prevIdx := p.pathIdx - 1
	if prevIdx < 0 {
		if g.isLoop {
			prevIdx = len(g.beatInfos) - 1
		} else {
			g.logger.Warnf("[GAME] advancePulse: prevIdx < 0 in non-looping mode. Stopping pulse.")
			return false
		}
	}
	g.logger.Debugf("[GAME] advancePulse: Selecting next segment prevIdx=%d, pathIdx=%d", prevIdx, p.pathIdx)
	p.fromBeatInfo = g.beatInfos[prevIdx]
	p.toBeatInfo = g.beatInfos[p.pathIdx]
	p.from = g.nodeByID(p.fromBeatInfo.NodeID)
	p.to = g.nodeByID(p.toBeatInfo.NodeID)

	if p.from == nil || p.to == nil {
		g.logger.Warnf("[GAME] advancePulse: Could not find nodes for pulse segment from %d to %d. Stopping pulse.", p.fromBeatInfo.NodeID, p.toBeatInfo.NodeID)
		return false
	}

	// Set pulse start and end coordinates for animation
	p.x1 = p.from.X
	p.y1 = p.from.Y
	p.x2 = p.to.X
	p.y2 = p.to.Y
	p.t = 0 // Reset animation progress

	g.logger.Debugf("[GAME] advancePulse: Pulse updated. From (%d,%d) to (%d,%d). Current pathIdx: %d", p.fromBeatInfo.I, p.fromBeatInfo.J, p.toBeatInfo.I, p.toBeatInfo.J, p.pathIdx)

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
