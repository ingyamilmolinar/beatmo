package ui

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/beat"
	"github.com/ingyamilmolinar/tunkul/core/model"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
)

const topOffset = 40 // transport-bar height in px

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
	x1, y1, x2, y2 float64
	t, speed       float64
	from, to       *uiNode
	path           []model.NodeID
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
	activePulse *pulse
	frame       int
	renderedPulsesCount int

	/* editor state */
	sel            *uiNode
	linkDrag       dragLink
	camDragging    bool
	camDragged     bool
	leftPrev       bool
	pendingClick   bool
	clickI, clickJ int

	/* misc */
	winW, winH  int
	start       *uiNode // explicit “root/start” node (⇧S to set)
}

/* ───────────────── helper: node’s screen rect ───────────────── */

// Rectangle in *screen* pixels (y already includes the transport offset).
func (g *Game) nodeScreenRect(n *uiNode) (x1, y1, x2, y2 float64) {
	stepPx   := StepPixels(g.cam.Scale)                  // grid step in screen px
	camScale := float64(stepPx) / float64(GridStep)      // world→screen factor
	offX     := math.Round(g.cam.OffsetX)                // camera panning
	offY     := math.Round(g.cam.OffsetY)

	sx   := offX + float64(stepPx*n.I)                   // sprite centre X
	sy   := offY + float64(stepPx*n.J) + topOffset       // sprite centre Y
	size := float64(NodeSpriteSize) * camScale
	half := size / 2

	return sx - half, sy - half, sx + half, sy + half
}

/* ───────────────────── constructor & layout ─────────────────── */

func New(logger *game_log.Logger) *Game {
	g := &Game{cam: NewCamera(), logger: logger}

	g.graph = model.NewGraph(logger)
	g.sched = beat.NewScheduler()
	g.split = NewSplitter(720) // real height set in Layout below

	// bottom drum-machine view
	g.drum = NewDrumView(image.Rect(0, 600, 1280, 720), g.graph, logger)
	g.drum.Rows = []*DrumRow{{Name: "H"}}
	g.drum.bpm = 120 // Adjusted BPM for faster debugging (3-4x original)

	g.sched.OnTick = g.onTick
	g.activePulse = nil
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
	g.drum.SetBounds(image.Rect(0, g.split.Y, w, h))
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

func (g *Game) tryAddNode(i, j int) *uiNode {
	if n := g.nodeAt(i, j); n != nil {
		return n
	}
	id := g.graph.AddNode(i, j)
	n  := &uiNode{ID: id, I: i, J: j, X: float64(i * GridStep), Y: float64(j * GridStep)}

	if g.start == nil { // first ever node becomes the start
		g.start  = n
		n.Start  = true
		g.graph.StartNodeID = n.ID
	}
	g.nodes = append(g.nodes, n)
	return n
}

func (g *Game) deleteNode(n *uiNode) {
	/* remove from slice */
	for idx, v := range g.nodes {
		if v == n {
			g.nodes = append(g.nodes[:idx], g.nodes[idx+1:]...)
			break
		}
	}
	/* drop touching edges */
	out := g.edges[:0]
	for _, e := range g.edges {
		if e.A != n && e.B != n {
			out = append(out, e)
		}
	}
	g.edges = out

	g.graph.RemoveNode(n.ID)
	g.drum.Rows[0].Steps, _ = g.graph.CalculateBeatRow()

	if g.sel == n {
		g.sel = nil
	}
	if g.start == n {
		g.start = nil
	}
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
	g.edges = append(g.edges, uiEdge{a, b})
	g.graph.Edges[[2]model.NodeID{a.ID, b.ID}] = struct{}{}
	g.logger.Debugf("[GAME] Added edge: %d,%d -> %d,%d", a.I, a.J, b.I, b.J)
	g.drum.Rows[0].Steps, _ = g.graph.CalculateBeatRow()
	g.drum.bgDirty = true // Invalidate background cache to redraw steps
}

func (g *Game) deleteEdge(a, b *uiNode) {
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
	delete(g.graph.Edges, [2]model.NodeID{b.ID, a.ID})
	g.logger.Debugf("[GAME] Deleted edge: %d,%d -> %d,%d", a.I, a.J, b.I, b.J)
	g.drum.Rows[0].Steps, _ = g.graph.CalculateBeatRow()
	g.drum.bgDirty = true // Invalidate background cache to redraw steps
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
			n := g.tryAddNode(g.clickI, g.clickJ)
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

func (g *Game) spawnPulse(from *uiNode) {
	g.logger.Debugf("[GAME] Spawn pulse: attempting to start initial pulse from node %d", from.ID)
	// Find the first successor of the start node
	var firstSuccessor *uiNode
	for _, edge := range g.edges {
		if edge.A.ID == from.ID {
			firstSuccessor = edge.B
			break
		}
	}

	if firstSuccessor != nil {
		g.activePulse = &pulse{
			x1: from.X, y1: from.Y, x2: firstSuccessor.X, y2: firstSuccessor.Y,
			speed: 0.1,
			from:  from,
			to:    firstSuccessor,
			path:  []model.NodeID{from.ID},
		}
		g.logger.Infof("[GAME] Spawn pulse: initial pulse spawned from %d to %d", from.ID, firstSuccessor.ID)
	} else {
		g.logger.Infof("[GAME] Spawn pulse: start node %d has no successors, no initial pulse spawned.", from.ID)
	}
}

/* ─────────────── Update & tick ────────────────────────────────────────── */

func (g *Game) Update() error {
	// splitter
	g.split.Update()
	g.drum.SetBounds(image.Rect(0, g.split.Y, g.winW, g.winH))
	g.drum.recalcButtons()

	// camera pan only when not dragging link or splitter
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	panOK := !g.linkDrag.active && !g.split.dragging && !shift
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
		g.logger.Debugf("[GAME] Update: processing active pulse: t=%.2f, from=%d, to=%d, path=%v", g.activePulse.t, g.activePulse.from.ID, g.activePulse.to.ID, g.activePulse.path)
		g.activePulse.t += g.activePulse.speed
		if g.activePulse.t >= 1 {
			if !g.advancePulse(g.activePulse) {
				g.logger.Infof("[GAME] Update: active pulse removed.")
				g.activePulse = nil
			}
		}
	}

	// drum view logic
	prevPlaying := g.drum.playing
	g.drum.Update()

	if g.drum.playing != prevPlaying {
		g.logger.Infof("[GAME] Update: drum playing state changed: %t -> %t", prevPlaying, g.drum.playing)
		if g.drum.playing {
			// Start playback: create initial pulse if none exists
			if g.activePulse == nil && g.start != nil {
				g.spawnPulse(g.start)
			}
			g.sched.Start()
		} else {
			g.sched.Stop()
			// Stop playback: remove all pulses
			g.logger.Infof("[GAME] Update: stopping playback, removing active pulse.")
			g.activePulse = nil
		}
	}

	if g.drum.playing {
		g.sched.BPM = g.drum.bpm
		g.sched.Tick()
	}

	// Update drum view length based on graph
	g.drum.Rows[0].Steps, _ = g.graph.CalculateBeatRow()
	g.drum.bgDirty = true // Invalidate background cache to redraw steps
	return nil
}

/* ─────────────── Draw ─────────────────────────────────────────────────── */

func (g *Game) Draw(screen *ebiten.Image) {
	g.drawGridPane(screen) // top
	g.drawDrumPane(screen) // bottom (includes buttons)
}

func (g *Game) drawGridPane(screen *ebiten.Image) {
	// camera matrix for world drawings (shift down by bar height)
	stepPx := StepPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	camScale := float64(stepPx) / float64(GridStep)
	var cam ebiten.GeoM
	cam.Scale(camScale, camScale)
	cam.Translate(offX, offY+float64(topOffset))

	frame := (g.frame / 6) % len(NodeFrames)

	// grid lattice computed in world coordinates then transformed
	minX, maxX, minY, maxY := visibleWorldRect(g.cam, g.winW, g.split.Y)
	startI := int(math.Floor(minX / GridStep))
	endI := int(math.Ceil(maxX / GridStep))
	startJ := int(math.Floor(minY / GridStep))
	endJ := int(math.Ceil(maxY / GridStep))

	var id ebiten.GeoM
	for i := startI; i <= endI; i++ {
		x := float64(i * GridStep)
		DrawLineCam(screen, x, minY, x, maxY, &cam, color.RGBA{40, 40, 40, 255}, 1)
	}
	for j := startJ; j <= endJ; j++ {
		y := float64(j * GridStep)
		DrawLineCam(screen, minX, y, maxX, y, &cam, color.RGBA{40, 40, 40, 255}, 1)
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
	g.drum.Draw(dst)
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

	// The scheduler ticks every beat. We only care about the first beat (step 0)
	// to potentially start a pulse if one isn't already active.
	if step == 0 && g.activePulse == nil {
		if g.start != nil {
			g.spawnPulse(g.start)
		}
	}
}

func (g *Game) nodeByID(id model.NodeID) *uiNode {
	for _, n := range g.nodes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (g *Game) advancePulse(p *pulse) bool {
	// Find the next node in the path
	var nextNode *uiNode

	// Check for successors from the current 'to' node
	foundSuccessor := false
	for _, edge := range g.edges {
		if edge.A.ID == p.to.ID {
			nextNode = edge.B
			foundSuccessor = true
			break // Only take the first successor for now
		}
	}

	if foundSuccessor {
		// Update the existing pulse to the next segment
		p.from = p.to
		p.to = nextNode
		p.x1 = p.from.X
		p.y1 = p.from.Y
		p.x2 = p.to.X
		p.y2 = p.to.Y
		p.t = 0 // Reset animation progress
		// For continuous loop, the path should reflect the current segment, not grow indefinitely.
		// This simplified logic assumes a single pulse in a closed circuit.
		newPath := []model.NodeID{p.from.ID, p.to.ID}
		p.path = newPath
		return true
	}

	return false // No successor, remove the pulse
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

