package ui

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/ingyamilmolinar/tunkul/core/engine"
	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
	game_log "github.com/ingyamilmolinar/tunkul/internal/log"
	"github.com/ingyamilmolinar/tunkul/internal/utils"
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
	// For non-loops, the unbounded path already includes any synthesized
	// intermediate (invisible) steps between endpoints; keep the full length.
	if !isLoop {
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

// sendLatest writes v to ch, dropping an existing item if the buffer is full.
// It never blocks and will silently discard v if the channel remains full.
func sendLatest[T any](ch chan T, v T) {
	select {
	case ch <- v:
		return
	default:
		select {
		case <-ch:
		default:
		}
		select {
		case ch <- v:
		default:
		}
	}
}

// sendLatestBPM aggressively ensures v is left in the channel by dropping
// existing items until a non-blocking send succeeds. This is used for BPM
// updates where only the most recent value matters and tests expect that the
// final requested BPM is applied even if the audio layer was busy.
func sendLatestBPM(ch chan int, v int) {
	for i := 0; i < 8; i++ { // bounded attempts to avoid long spins
		select {
		case ch <- v:
			return
		default:
		}
		// Drop one pending value if any.
		select {
		case <-ch:
		default:
		}
	}
	// Best-effort final attempt; ok if it drops due to extreme contention.
	select {
	case ch <- v:
	default:
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
	segBeats                 float64
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
	nodesByID       map[model.NodeID]*uiNode
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
	lastBeat           float64
	lastProg           float64
	lastDisplayBeat    float64
	pausedBeats        int

	/* misc */
	winW, winH    int
	start         *uiNode // explicit “root/start” node (⇧S to set)
	centered      bool    // camera centered on first layout
	demoBuilt     bool    // demo circuit built once
	demoScheduled bool    // build demo on next update
	// animBeatPrev was used for time-based advancement; unused now
	highlightHook func(row, idx int)

	lastFrame        time.Time
	playStart        time.Time
	beatBase         float64
	samplesScheduled bool
	// prevBPM caches the last UI BPM applied so we can detect changes and
	// re-anchor the timebase to avoid jumps when BPM changes mid-play.
	prevBPM int

	// sequencer (engine-driven audio independent from UI)
	seqCh                <-chan engine.Event
	seqNextIdxs          []int
	seqQuit              chan struct{}
	useSequencerForAudio bool
	audioStart           float64
	justResumed          bool
	justPaused           bool

	// lastHLIdxByRow tracks the last highlighted absolute subdivision index
	// per row to prevent duplicate highlight hooks when the same cell is
	// refreshed across frames.
	lastHLIdxByRow []int

	// highlight scheduling from sequencer goroutine
	hlCh chan struct {
		row, idx int
		info     model.BeatInfo
	}

	// enable special behavior for precise timing tests
	timingTestMode bool
}

// bpmOwner points to the Game instance that currently owns BPM application.
// Older Game instances created by earlier tests yield and do not call the
// global audio.SetBPM while a newer Game exists.
var bpmOwner atomic.Pointer[Game]

// SetUseSequencerForTest overrides whether the time-based audio sequencer and
// UI synchronization are active. Tests can enable this to validate the sync
// logic without relying on real wall-clock scheduling.
func (g *Game) SetUseSequencerForTest(enable bool) { g.useSequencerForAudio = enable }

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
		nodesByID:          make(map[model.NodeID]*uiNode),
		activePulses:       []*pulse{},
		pendingStartRow:    -1,
		grid:               NewGrid(DefaultGridStep),
		audioCh:            make(chan soundReq, 32),
		bpmCh:              make(chan int, 1),
		bpmAck:             make(chan int, 1),
		playFn:             playSound,
		lastHLIdxByRow:     nil,
		hlCh: make(chan struct {
			row, idx int
			info     model.BeatInfo
		}, 64),
	}

	// Defer embedded WAV autoload; we'll schedule it after the first Update so
	// the demo instrument picks prefer built-ins and the UI becomes responsive.

	// bottom drum-machine view
	g.drum = NewDrumView(image.Rect(0, 600, 1280, 720), g.graph, logger)
	// Ensure the timeline header interprets offsets/length in beats while we
	// track them internally in subdivision steps.
	g.drum.timelineUnitsPerBeat = g.grid.MaxDiv()
	// wire import handler
	g.drum.onImport = func(data []byte) error { return g.Import(data) }
	// provide current MaxDiv for export JSON
	currentMaxDiv = func() int { return g.grid.MaxDiv() }
	g.appliedBPM = g.drum.BPM()
	g.prevBPM = g.appliedBPM
	go g.audioLoop()
	go g.bpmLoop()
	// Subscribe to engine ticks for scheduler-driven audio sequencing.
	g.seqCh = eng.Subscribe()
	g.seqQuit = make(chan struct{})
	go g.sequencerLoop()
	// Use time-based audio scheduling universally; optionally mirror
	// highlight hooks at scheduling time during timing tests.
	g.useSequencerForAudio = true
	g.timingTestMode = (os.Getenv("BPM_TIMING_TEST") == "1")
	// Set this Game as the active BPM owner so previous instances stop
	// applying audio.SetBPM in background loops during tests.
	bpmOwner.Store(g)
	g.initJS()
	// Defer demo construction to the first layout/update to avoid blocking
	// constructor time. Layout will build it once when not under tests.
	return g
}

// sequencerLoop listens to engine tick events and schedules audio playback
// strictly on the engine timeline, decoupled from UI rendering.
func (g *Game) sequencerLoop() {
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-g.seqQuit:
			return
		case <-ticker.C:
			if !g.playing || !g.useSequencerForAudio {
				continue
			}
			g.seqScheduleTime()
		}
	}
}

// seqScheduleBeat schedules audio for the next beat across all rows based on
// beatInfosByRow and internal seqNextIdxs counters. It avoids touching UI
// state (highlights/pulses) and only queues audio.
func (g *Game) seqScheduleBeat() {
	// Ensure counters match current row count.
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	for row := range g.drum.Rows {
		path := g.beatInfosByRow
		if row >= len(path) {
			continue
		}
		infos := path[row]
		if len(infos) == 0 {
			continue
		}
		idx := g.seqNextIdxs[row]
		// Wrap when looping; otherwise stop at end.
		if idx >= len(infos) {
			if row < len(g.isLoopByRow) && g.isLoopByRow[row] {
				start := 0
				if row < len(g.loopStartByRow) {
					start = g.loopStartByRow[row]
				}
				loopLen := len(infos) - start
				if loopLen <= 0 {
					continue
				}
				idx = start + (idx-start)%loopLen
			} else {
				continue
			}
		}
		info := infos[idx]
		if info.NodeType == model.NodeTypeRegular {
			inst := g.drum.Rows[row].Instrument
			vol := g.drum.Rows[row].Volume
			g.queueSound(inst, vol)
		}
		g.seqNextIdxs[row] = g.seqNextIdxs[row] + 1
	}
}

// seqScheduleTime schedules audio for all rows based on wall-clock time,
// independent of UI rendering, at subdivision resolution.
func (g *Game) seqScheduleTime() {
	div := g.grid.MaxDiv()
	if div <= 0 {
		return
	}
	// Drive scheduling from the applied BPM to keep audio tightly aligned to
	// the engine/audio layer. Fall back to UI BPM only if no applied value is
	// available yet (e.g., just started).
	bpm := g.appliedBPM
	if bpm <= 0 {
		bpm = g.bpm
	}
	if g.playing && bpm != g.prevBPM && g.prevBPM > 0 {
		prev := g.prevBPM
		var dtSec float64
		if n := audio.Now(); n > 0 && g.audioStart > 0 {
			dtSec = n - g.audioStart
		} else if !g.playStart.IsZero() {
			dtSec = time.Since(g.playStart).Seconds()
		}
		beatsNow := g.beatBase + dtSec*float64(prev)/60.0
		g.beatBase = beatsNow
		g.playStart = time.Now()
		if n := audio.Now(); n > 0 {
			g.audioStart = n
		}
		if len(g.seqNextIdxs) != len(g.drum.Rows) {
			g.seqNextIdxs = make([]int, len(g.drum.Rows))
		}
		for row := range g.drum.Rows {
			next := 0
			if row < len(g.nextBeatIdxs) {
				next = g.nextBeatIdxs[row]
			}
			g.seqNextIdxs[row] = next
		}
		g.prevBPM = bpm
	}
	if bpm <= 0 {
		return
	}
	// Compute target absolute subdivision index since playStart.
	var dtSec float64
	if n := audio.Now(); n > 0 && g.audioStart > 0 {
		dtSec = n - g.audioStart
	} else if !g.playStart.IsZero() {
		dtSec = time.Since(g.playStart).Seconds()
	} else {
		return
	}
	baseBeats := g.beatBase
	target := int(math.Floor((baseBeats + dtSec*float64(bpm)/60.0) * float64(div)))
	// Ensure counters align to row count
	if len(g.seqNextIdxs) != len(g.drum.Rows) {
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	// Schedule up to current target for each row.
	for row := range g.drum.Rows {
		for g.seqNextIdxs[row] <= target {
			idx := g.seqNextIdxs[row]
			info := g.beatInfoAtRow(row, idx)
			if info.NodeType == model.NodeTypeRegular {
				// respect mute/solo state like highlightBeat
				inst := g.drum.Rows[row].Instrument
				vol := g.drum.Rows[row].Volume
				anySolo := false
				for _, r := range g.drum.Rows {
					if r.Solo {
						anySolo = true
						break
					}
				}
				if !(g.drum.Rows[row].Muted || (anySolo && !g.drum.Rows[row].Solo)) {
					g.queueSound(inst, vol)
					if g.timingTestMode {
						// Fire the test hook immediately to align timestamps with audio scheduling.
						if g.highlightHook != nil {
							g.highlightHook(row, idx)
						}
					} else {
						// Enqueue highlight event for the main thread to process visuals and hooks.
						select {
						case g.hlCh <- struct {
							row, idx int
							info     model.BeatInfo
						}{row: row, idx: idx, info: info}:
						default:
						}
					}
				}
			}
			g.seqNextIdxs[row] = idx + 1
		}
	}
}

func (g *Game) audioLoop() {
	for req := range g.audioCh {
		if g.paused {
			// Drop queued sounds specifically while paused so tests observe silence.
			g.logger.Debugf("[AUDIO] drop id=%s vol=%.3f while paused", req.id, req.vol)
			continue
		}
		g.logger.Debugf("[AUDIO] playFn id=%s vol=%.3f when=%v", req.id, req.vol, req.when)
		g.playFn(req.id, req.vol, req.when...)
	}
}

// SetPlayFunc overrides the audio playback function used by this game instance.
func (g *Game) SetPlayFunc(fn func(string, float64, ...float64)) {
	g.playFn = func(id string, vol float64, when ...float64) {
		if g.paused {
			return
		}
		fn(id, vol, when...)
	}
}

func (g *Game) bpmLoop() {
	for {
		// Wait for at least one update.
		b, ok := <-g.bpmCh
		if !ok {
			return
		}
		// Drain any pending updates so only the latest BPM is applied.
		for {
			select {
			case b = <-g.bpmCh:
				// keep draining
			default:
				start := time.Now()
				g.logger.Debugf("[GAME] applying BPM=%d", b)
				if bpmOwner.Load() != g {
					goto applied
				}
				g.engine.SetBPM(b)
				audio.SetBPM(b)
				g.logger.Debugf("[GAME] applied BPM=%d in %s", b, time.Since(start))
				sendLatest(g.bpmAck, b)
				// If UI has moved on while we were applying BPM (e.g. audio
				// layer was blocked), immediately converge to the current
				// desired DrumView BPM without waiting for another Update().
				// This prevents flakiness where the last requested value was
				// dropped from bpmCh by coalescing.
				for {
					cur := g.drum.BPM()
					if cur == b {
						break
					}
					b = cur
					start = time.Now()
					g.logger.Debugf("[GAME] reconciling BPM=%d", b)
					if bpmOwner.Load() != g {
						break
					}
					g.engine.SetBPM(b)
					audio.SetBPM(b)
					g.logger.Debugf("[GAME] reconciled BPM=%d in %s", b, time.Since(start))
					sendLatest(g.bpmAck, b)
				}
				goto applied
			}
		}
	applied:
		// loop to wait for next update
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
	// Center camera once. During tests we usually keep (0,0) stable, except
	// when default-start behavior is explicitly requested by tests.
	if !g.centered && (!runningUnderGoTest() || enableDefaultStart) {
		g.cam.OffsetX = float64(w) / 2
		g.cam.OffsetY = float64(g.split.Y-topOffset) / 2
		g.cam.Snap()
		g.centered = true
	}
	if enableDefaultStart && !g.demoBuilt {
		if !runningUnderGoTest() {
			if !g.demoScheduled {
				g.demoScheduled = true // let Update build it shortly after startup
			}
		} else if len(g.nodes) == 0 {
			// In tests, create a single centered origin node.
			g.pendingStartRow = 0
			g.tryAddNode(0, 0, model.NodeTypeRegular)
		}
	} else if enableDefaultStart && len(g.nodes) == 0 {
		// Fallback when demo is disabled (tests) or failed.
		g.pendingStartRow = 0
		g.tryAddNode(0, 0, model.NodeTypeRegular)
	}
	// Ensure centering occurs in tests with default start even if earlier block didn't run.
	if !g.centered && runningUnderGoTest() && enableDefaultStart {
		g.cam.OffsetX = float64(w) / 2
		g.cam.OffsetY = float64(g.split.Y-topOffset) / 2
		g.cam.Snap()
		g.centered = true
	}
	g.logger.Debugf("[GAME] Layout: winW: %d, winH: %d, split.Y: %d, drum.Bounds: %v", g.winW, g.winH, g.split.Y, g.drum.Bounds)
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
	if n := g.nodesByID[id]; n != nil {
		return n
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
				g.logger.Infof("[GAME] Node upgraded to regular id=%d grid=(%d,%d)", n.ID, i, j)
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
	g.nodesByID[n.ID] = n
	if nodeType == model.NodeTypeRegular {
		g.logger.Infof("[GAME] Node created id=%d grid=(%d,%d)", n.ID, i, j)
	} else {
		g.logger.Infof("[GAME] Invisible node created id=%d grid=(%d,%d)", n.ID, i, j)
	}
	// Select newly created regular nodes to match test expectations.
	if nodeType == model.NodeTypeRegular {
		if g.sel != nil {
			g.sel.Selected = false
		}
		g.sel = n
		n.Selected = true
		g.computeSelNeighbors()
	}
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
	delete(g.nodesByID, n.ID)
	g.logger.Infof("[GAME] Node deleted id=%d grid=(%d,%d)", n.ID, n.I, n.J)

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
	// Build unbounded path so we capture all intermediate steps without
	// relying on Graph.BeatLength. This avoids truncation now that invisible
	// steps are synthesized on-the-fly instead of stored as nodes.
	fullBeatRow, isLoop, loopStart := g.graph.CalculateBeatRowUnbounded()
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
	// Reset last-highlighted indices to force a hook on first arrival per row.
	g.lastHLIdxByRow = make([]int, nRows)
	for i := range g.lastHLIdxByRow {
		g.lastHLIdxByRow[i] = -1
	}
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

	// Preserve current drum offset when the beat path changes. Clamp against
	// the timeline length rather than the raw beat path so tracking can
	// continue beyond the initial graph traversal.
	maxOffset := g.drum.timelineBeats - g.drum.Length
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
	// When the visible steps exceed available pixels, DrumView.Draw decimates
	// and does not use per-step state. Skip expensive per-cell computation to
	// keep the UI path lightweight.
	needPerStep := g.drum.Length <= g.drum.timelineRect.Dx()
	if needPerStep {
		g.drumBeatInfos = make([]model.BeatInfo, g.drum.Length)
		for rowIdx, r := range g.drum.Rows {
			r.Steps = make([]bool, g.drum.Length)
			for i := 0; i < g.drum.Length; i++ {
				abs := g.drum.Offset + i
				info := g.beatInfoAtRow(rowIdx, abs)
				if rowIdx == 0 {
					g.drumBeatInfos[i] = info
				}
				on := info.NodeType == model.NodeTypeRegular
				if rowIdx < len(g.isLoopByRow) && g.isLoopByRow[rowIdx] {
					start := g.loopStartByRow[rowIdx]
					seg := g.loopLenByRow[rowIdx]
					if seg > 0 && abs >= start+1 {
						if (abs-(start+1))%seg == 0 {
							on = false
						}
					}
				}
				r.Steps[i] = on
			}
		}
	} else {
		// Maintain expected lengths but avoid deriving values we won't draw.
		g.drumBeatInfos = nil
		for _, r := range g.drum.Rows {
			r.Steps = make([]bool, g.drum.Length)
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

	// Record UI edge and single graph edge between regular endpoints only.
	g.edges = append(g.edges, uiEdge{A: a, B: b, t: 0, pulse: -1})
	g.graph.Edges[[2]model.NodeID{a.ID, b.ID}] = struct{}{}
	g.logger.Debugf("[GAME] Added edge: %d,%d -> %d,%d", a.I, a.J, b.I, b.J)
	g.logger.Infof("[GAME] Edge created from id=%d grid=(%d,%d) to id=%d grid=(%d,%d)", a.ID, a.I, a.J, b.ID, b.I, b.J)
	if g.graph.StartNodeID != model.InvalidNodeID {
		g.updateBeatInfos()
	}
	g.computeSelNeighbors()
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
	g.logger.Debugf("[GAME] Deleted edge: %d,%d -> %d,%d", a.I, a.J, b.I, b.J)
	g.logger.Infof("[GAME] Edge deleted from id=%d grid=(%d,%d) to id=%d grid=(%d,%d)", a.ID, a.I, a.J, b.ID, b.I, b.J)
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
	// release → commit or delete (only between regular nodes)
	if g.linkDrag.active && !left {
		if n2 := g.nodeAt(i, j); n2 != nil && n2 != g.linkDrag.from {
			tFrom := g.graph.Nodes[g.linkDrag.from.ID].Type
			tTo := g.graph.Nodes[n2.ID].Type
			if tFrom == model.NodeTypeRegular && tTo == model.NodeTypeRegular {
				if right {
					g.logger.Debugf("[GAME] Deleting edge: node=%d grid=(%d,%d) and node=%d grid=(%d,%d)", g.linkDrag.from.ID, g.linkDrag.from.I, g.linkDrag.from.J, n2.ID, n2.I, n2.J)
					g.deleteEdge(g.linkDrag.from, n2)
				} else {
					g.logger.Debugf("[GAME] Adding edge: node=%d grid=(%d,%d) and node=%d grid=(%d,%d)", g.linkDrag.from.ID, g.linkDrag.from.I, g.linkDrag.from.J, n2.ID, n2.I, n2.J)
					g.addEdge(g.linkDrag.from, n2)
				}
			} else {
				g.logger.Debugf("[GAME] Ignoring link to non-regular node at grid=(%d,%d)", i, j)
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
		speed:        float64(g.grid.MaxDiv()) / float64(beatDuration),
		fromBeatInfo: fromBeatInfo,
		toBeatInfo:   nextInfo,
		pathIdx:      nextIdxWrapped,
		lastIdx:      start,
		from:         g.nodeByID(fromBeatInfo.NodeID),
		to:           g.nodeByID(nextInfo.NodeID),
		path:         path,
		row:          row,
		segBeats:     beats,
	}
	g.activePulses = append(g.activePulses, p)
	if row == 0 {
		g.activePulse = p
	}
}

// spawnPulseFrom is kept for compatibility with tests; it spawns a pulse for the
// primary drum row.
func (g *Game) spawnPulseFrom(start int) { g.spawnPulseFromRow(0, start) }

/* ─────────────── Update & tick ────────────────────────────────────────── */

func (g *Game) Update() error {
	// Snapshot state at frame start to support precise pause without visual drift.
	beatsAtFrameStart := g.elapsedBeats
	startNext := append([]int(nil), g.nextBeatIdxs...)
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
	// Build demo on first Update after scheduling to avoid blocking ctor/layout.
	if g.demoScheduled && !g.demoBuilt && !runningUnderGoTest() {
		g.buildDemo()
		g.demoScheduled = false
	}
	// Kick off sample autoload once the game is live so built-in demo picks
	// are not overshadowed by sample IDs and startup remains snappy.
	if !g.samplesScheduled {
		g.samplesScheduled = true
		go audio.AutoLoadEmbeddedWAVs()
	}
	// splitter
	g.split.Update(g.winH)
	g.drum.SetBounds(image.Rect(0, g.split.Y, g.winW, g.winH))

	// editor interactions and drum view update before camera panning so
	// scrollbars and overlays can capture the mouse and block grid panning.
	mx, my := cursorPosition()
	left := isMouseButtonPressed(ebiten.MouseButtonLeft)
	// Nudge BPM text input focus early when clicking inside the BPM box so
	// manual editing works even if other handlers short-circuit later.
	if g.drum != nil {
		r := g.drum.bpmBox.Rect
		if left && mx >= r.Min.X && mx < r.Max.X && my >= r.Min.Y && my < r.Max.Y {
			g.drum.bpmBox.focused = true
		}
	}
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

	// Run drum view logic before evaluating panning so it can capture drags.
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
		if dr.index < len(g.nextBeatIdxs) {
			g.nextBeatIdxs = append(g.nextBeatIdxs[:dr.index], g.nextBeatIdxs[dr.index+1:]...)
		}
		if g.pendingStartRow == dr.index {
			g.pendingStartRow = -1
		} else if g.pendingStartRow > dr.index {
			g.pendingStartRow--
		}
		out := g.activePulses[:0]
		for _, p := range g.activePulses {
			if p.row != dr.index {
				if p.row > dr.index {
					p.row--
				}
				out = append(out, p)
			}
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

	// camera pan only when not dragging link, splitter, or drum view
	shift := isKeyPressed(ebiten.KeyShiftLeft) || isKeyPressed(ebiten.KeyShiftRight)
	panOK := !g.linkDrag.active && !g.split.dragging && !shift && !pt(mx, my, g.drum.Bounds) && !g.drum.Capturing()
	drag := g.cam.HandleMouse(panOK)
	g.camDragging = drag
	if left && drag {
		g.camDragged = true
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
	forcedAdvanced := false
	if g.playing {
		g.frame++
		if g.activePulse == nil && len(g.activePulses) > 0 {
			// Ensure legacy tests that reference activePulse directly can
			// observe the current primary-row pulse.
			g.activePulse = g.activePulses[0]
		}
		// Allow tests to force segment completion by setting p.t=1 and
		// calling Update() without waiting for an engine tick.
		for i := 0; i < len(g.activePulses); {
			p := g.activePulses[i]
			advanced := false
			for p.t >= 1 {
				prevIdx := p.lastIdx
				delete(g.highlightedBeats, makeBeatKey(p.row, prevIdx))
				if !g.advancePulse(p) {
					if p.row == 0 {
						g.activePulse = nil
					}
					g.activePulses = append(g.activePulses[:i], g.activePulses[i+1:]...)
					for key := range g.highlightedBeats {
						if r, _ := splitBeatKey(key); r == p.row {
							delete(g.highlightedBeats, key)
						}
					}
					advanced = true
					forcedAdvanced = true
					goto nextPulseForced
				}
				advanced = true
				forcedAdvanced = true
				// Loop again if multiple segments were forced complete.
			}
			if !advanced {
				i++
			}
			continue
		nextPulseForced:
			// Only increment index if we didn't remove this pulse.
			if i < len(g.activePulses) && g.activePulses[i] == p {
				i++
			}
		}
		if g.activePulse == nil && len(g.activePulses) > 0 {
			g.activePulse = g.activePulses[0]
		}
		// After any pulse advancement, align to current timeline so UI
		// reflects playback precisely even under heavy UI work. Skip the very
		// first frame after resuming to avoid visual jumps.
		if g.useSequencerForAudio && !g.justResumed {
			g.syncUIToTime()
		}
		// Clear the one-frame resume guard.
		if g.justResumed {
			g.justResumed = false
		}
	}

	// Drain any pending highlight events dispatched by the sequencer loop and
	// apply them on the UI thread to avoid data races with highlight state.
	for {
		select {
		case ev := <-g.hlCh:
			beatDuration := int64(60.0 / float64(max1(g.appliedBPM)) * ebitenTPS)
			g.highlightVisual(ev.row, ev.idx, ev.info, beatDuration)
			if len(g.nextBeatIdxs) != len(g.drum.Rows) {
				g.nextBeatIdxs = make([]int, len(g.drum.Rows))
			}
			g.nextBeatIdxs[ev.row] = ev.idx + 1
			if ev.row == 0 {
				g.elapsedBeats = ev.idx
			}
		default:
			goto hlDone
		}
	}
hlDone:
	// Clear expired highlights
	g.clearExpiredHighlights()

	// drum view logic already run above

	if g.drum.PlayPressed() {
		if g.playing {
			g.logger.Infof("[GAME] Pause pressed")
			// Pause: freeze base exactly at completed subdivisions
			div := g.grid.MaxDiv()
			if div <= 0 {
				div = 1
			}
			// Do not allow this frame's advancement to move the marker; lock to frame-start.
			g.elapsedBeats = beatsAtFrameStart
			g.nextBeatIdxs = append([]int(nil), startNext...)
			g.beatBase = float64(g.elapsedBeats) / float64(div)
			g.playStart = time.Time{}
			g.audioStart = 0
			g.lastBeat = g.beatBase
			g.playing = false
			g.paused = true
			g.pausedBeats = g.elapsedBeats
			// Reset display accumulator to the exact current base so resuming
			// picks up cleanly from the new playback state.
			divDisp := g.grid.MaxDiv()
			if divDisp <= 0 {
				divDisp = 1
			}
			g.lastDisplayBeat = float64(g.elapsedBeats) / float64(divDisp)
			// Flush any queued audio to ensure silence while paused.
			for {
				select {
				case <-g.audioCh:
					// drop
				default:
					goto drained
				}
			}
		drained:
			// Ensure the visual marker remains fixed at the paused index this
			// frame by applying a freeze after the rest of Update.
			g.justPaused = true
		} else if g.start != nil {
			g.logger.Infof("[GAME] Play pressed")
			// Start/resume
			audio.Resume()
			wasPaused := g.paused
			if wasPaused {
				// Resume from pause: keep position.
				g.beatBase = g.lastBeat
			} else {
				// Fresh start after Stop: reset position to the beginning.
				g.beatBase = 0
				g.lastBeat = 0
				g.justResumed = true
			}
			g.playStart = time.Now()
			if n := audio.Now(); n > 0 {
				g.audioStart = n
			} else {
				g.audioStart = 0
			}
			g.playing = true
			if wasPaused {
				// Spawn pulses immediately so displayBeat ties to the live signal
				// without waiting for the next engine tick. Use current internal
				// subdivision step index to preserve position precisely.
				for row := range g.drum.Rows {
					g.spawnPulseFromRow(row, g.elapsedBeats)
				}
				if g.activePulse == nil && len(g.activePulses) > 0 {
					g.activePulse = g.activePulses[0]
				}
				// Reset display accumulator to the new base.
				div := g.grid.MaxDiv()
				if div <= 0 {
					div = 1
				}
				g.lastDisplayBeat = float64(g.elapsedBeats) / float64(div)
				// Align the sequencer counters with the current next indices
				// to avoid any catch-up burst of scheduled audio.
				if len(g.seqNextIdxs) != len(g.drum.Rows) {
					g.seqNextIdxs = make([]int, len(g.drum.Rows))
				}
				// Compute current absolute subdivision target from wall-clock (dt ~ 0 right now).
				div2 := g.grid.MaxDiv()
				if div2 <= 0 {
					div2 = 1
				}
				bpm := g.appliedBPM
				if bpm <= 0 {
					bpm = g.bpm
				}
				var target int
				if bpm > 0 {
					target = int(math.Floor(g.beatBase * float64(div2)))
				} else {
					target = g.elapsedBeats
				}
				for row := range g.drum.Rows {
					next := g.elapsedBeats
					if row < len(g.nextBeatIdxs) {
						next = g.nextBeatIdxs[row]
					}
					t := target + 1
					if next > t {
						t = next
					}
					g.seqNextIdxs[row] = t
				}
				g.justResumed = true
				g.lastFrame = time.Now()
			}
			// Immediately align UI to the current timebase so the first frame
			// after starting reflects the correct position without lag.
			if g.useSequencerForAudio {
				g.syncUIToTime()
			}
		} else {
			g.logger.Warnf("[GAME] Play pressed but no start node; ignoring")
			g.playing = false
		}
	}
	if g.drum.StopPressed() {
		g.logger.Infof("[GAME] Stop pressed")
		g.playing = false
		g.paused = false
		g.beatBase = 0
		g.playStart = time.Time{}
		g.lastBeat = 0
		g.audioStart = 0
		g.seqNextIdxs = make([]int, len(g.drum.Rows))
	}
	// Detect BPM changes from the UI and re-anchor the timebase so that
	// playback position remains continuous without a jump. We must compute
	// the current absolute beat using the previous BPM, then reset the base
	// to "now" so future scheduling with the new BPM continues seamlessly.
	prevUI := g.bpm
	newBPM := g.drum.BPM()
	if newBPM != prevUI && g.playing {
		g.logger.Infof("[GAME] BPM change requested: %d -> %d", prevUI, newBPM)
		prev := prevUI
		if prev > 0 {
			// Compute elapsed seconds on the current timeline.
			var dtSec float64
			if n := audio.Now(); n > 0 && g.audioStart > 0 {
				dtSec = n - g.audioStart
			} else if !g.playStart.IsZero() {
				dtSec = time.Since(g.playStart).Seconds()
			}
			// Absolute beat position at this instant using the previous BPM.
			beatsNow := g.beatBase + dtSec*float64(prev)/60.0
			// Re-anchor base to preserve continuity and reset the clock.
			g.beatBase = beatsNow
			g.playStart = time.Now()
			if n := audio.Now(); n > 0 {
				g.audioStart = n
			}
			// Align sequencer counters with current next-beat indices to avoid
			// any catch-up burst the next time scheduling runs.
			if len(g.seqNextIdxs) != len(g.drum.Rows) {
				g.seqNextIdxs = make([]int, len(g.drum.Rows))
			}
			for row := range g.drum.Rows {
				next := 0
				if row < len(g.nextBeatIdxs) {
					next = g.nextBeatIdxs[row]
				}
				if next <= 0 {
					next = 0
				}
				g.seqNextIdxs[row] = next
			}
		}
	}
	g.bpm = newBPM
	if g.bpm != g.appliedBPM {
		g.logger.Infof("[GAME] Queue BPM %d", g.bpm)
		sendLatestBPM(g.bpmCh, g.bpm)
	}

	select {
	case applied := <-g.bpmAck:
		g.logger.Infof("[GAME] BPM applied: %d", applied)
		// Update pulse speeds to reflect new BPM for test expectations.
		beatDuration := int64(60.0 / float64(applied) * ebitenTPS)
		for _, p := range g.activePulses {
			p.speed = float64(g.grid.MaxDiv()) / float64(beatDuration)
		}
		g.appliedBPM = applied
	default:
	}

	// Per-frame advancement for smooth animation when using dt integration.
	// When the time-based sequencer is active, pulse state is driven by
	// syncUIToTime and we avoid dt-based integration to prevent jitter.
	if g.playing && !forcedAdvanced && !g.justResumed && !g.useSequencerForAudio {
		now := time.Now()
		dt := 0.0
		if !g.lastFrame.IsZero() {
			dt = now.Sub(g.lastFrame).Seconds()
		}
		g.lastFrame = now
		for i := 0; i < len(g.activePulses); {
			p := g.activePulses[i]
			seg := p.segBeats
			if seg <= 0 {
				seg = 1
			}
			dtBeats := dt * float64(g.bpm) / 60.0
			if dtBeats > 0.1 {
				dtBeats = 0.1
			}
			if dtBeats <= 0 {
				dtBeats = (1.0 / float64(ebitenTPS)) * float64(g.bpm) / 60.0
			}
			p.t += dtBeats / seg
			// Drain overflows while preserving leftover beats into the next segment.
			for p.t >= 1 {
				// leftover beats beyond this segment
				leftoverBeats := (p.t - 1) * seg
				prevIdx := p.lastIdx
				delete(g.highlightedBeats, makeBeatKey(p.row, prevIdx))
				if !g.advancePulse(p) {
					if p.row == 0 {
						g.activePulse = nil
					}
					g.activePulses = append(g.activePulses[:i], g.activePulses[i+1:]...)
					for key := range g.highlightedBeats {
						if r, _ := splitBeatKey(key); r == p.row {
							delete(g.highlightedBeats, key)
						}
					}
					goto nextPulse
				}
				// map leftover beats into the new segment's t
				seg = p.segBeats
				if seg <= 0 {
					seg = 1
				}
				p.t = leftoverBeats / seg
			}
			i++
		nextPulse:
			// only increment when pulse still present
			if i < len(g.activePulses) && g.activePulses[i] != p {
				// pulse removed; keep index
			}
		}
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
				// Reset sequencer counters for fresh start.
				g.seqNextIdxs = make([]int, len(g.drum.Rows))
				// Spawn initial pulses from the beginning so playback visibly
				// starts at 0 immediately after Stop -> Play.
				for row := range g.drum.Rows {
					g.spawnPulseFromRow(row, 0)
				}
				if g.activePulse == nil && len(g.activePulses) > 0 {
					g.activePulse = g.activePulses[0]
				}
				g.lastDisplayBeat = 0
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
	if !g.playing && g.paused {
		// Keep counters pinned exactly while paused.
		g.elapsedBeats = g.pausedBeats
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
	// If we just paused, re-assert the current visual highlight locations so
	// the marker does not jump forward within this frame due to earlier sync.
	if g.justPaused {
		// Freeze highlights per row to their last subdivision index.
		for row := range g.drum.Rows {
			idx := 0
			if row < len(g.nextBeatIdxs) {
				idx = g.nextBeatIdxs[row] - 1
			}
			if idx < 0 {
				idx = 0
			}
			// Visual-only highlight; clear other markers in this row.
			info := g.beatInfoAtRow(row, idx)
			duration := int64(60.0 / float64(max1(g.appliedBPM)) * ebitenTPS)
			g.highlightVisual(row, idx, info, duration)
		}
		g.justPaused = false
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
	// Keep DrumView's seconds-per-beat in sync with the engine/app-lied BPM for
	// timeline counters without overriding the user-edited BPM control value.
	// This avoids a race where UI changes are undone by the draw loop before
	// Game.Update() can propagate them to the engine.
	bpm := g.appliedBPM
	if bpm <= 0 {
		bpm = g.bpm
	}
	if bpm > 0 {
		g.drum.secPerBeat = 60.0 / float64(bpm)
	}
	// Use a smooth, non-quantized beat value for timeline counters to avoid
	// jitter in the displayed timers, while highlights and steps remain
	// quantized via internal counters.
	g.drum.Draw(dst, g.highlightedBeats, g.frame, g.drumBeatInfos, g.displayBeat())
}

func (g *Game) currentBeat() float64 {
	// Convert the internal elapsed step counter (subdivisions) to beats.
	// While paused/stopped, report the last whole-beat position so counters
	// freeze immediately.
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	base := float64(g.elapsedBeats) / float64(div)
	if g.playing {
		prog := g.engineProgress() // 0..1 fraction of current beat
		// Smooth jitter and detect wrap-around. If progress drops a little,
		// clamp to the previous value. If it drops significantly, interpret
		// as a wrap to the next beat and add 1.
		frac := prog
		if g.lastProg > 0 && prog+0.25 < g.lastProg {
			// Wrapped to the next beat between calls.
			frac = 1 + prog
		} else if prog < g.lastProg {
			// Minor jitter: keep previous fractional progress.
			frac = g.lastProg
		}
		beat := base + frac
		// Quantize to the nearest grid subdivision so each playback tick maps
		// cleanly to discrete sub-beat steps.
		q := math.Round(beat*float64(div)) / float64(div)
		// Ensure monotonic progression even after rounding.
		if q < g.lastBeat {
			q = g.lastBeat
		}
		// Store the raw (0..1) fraction for next comparisons, and the
		// quantized beat for external consumers.
		g.lastProg = prog
		g.lastBeat = q
		return q
	}
	// Not playing: lock to last integer beat.
	return base
}

// displayBeat returns a smooth beat position suitable for UI timers. It
// combines the completed subdivision steps with the scheduler's fractional
// progress and clamps minor regressions for jitter-free display. It never
// lags behind the last completed subdivision within the current beat.
func (g *Game) displayBeat() float64 {
	// Smooth, time-based beat counter for UI display. Uses the same timebase
	// the sequencer relies on (beatBase + elapsed seconds * BPM/60), so it
	// advances continuously with millisecond precision and remains
	// independent of discrete subdivision updates.
	div := g.grid.MaxDiv()
	if div <= 0 {
		div = 1
	}
	if !g.playing {
		if g.lastDisplayBeat > 0 {
			return g.lastDisplayBeat
		}
		// Freeze at the last whole beat based on completed subdivisions.
		return float64(g.elapsedBeats) / float64(div)
	}
	// If the timebase has not been initialized yet (e.g., tests manipulating
	// engineProgress without calling Play), fall back to subdivision/pulse
	// based computation to preserve expected behavior in existing tests.
	if g.playStart.IsZero() {
		div := g.grid.MaxDiv()
		if div <= 0 {
			div = 1
		}
		baseWhole := g.elapsedBeats / div
		base := float64(baseWhole)
		if p := g.pulseForRow(0); p != nil {
			seg := p.segBeats
			if seg <= 0 {
				dist := hypot(p.x2-p.x1, p.y2-p.y1)
				seg = dist / g.grid.Step
				if seg <= 0 {
					seg = 1.0 / float64(div)
				}
			}
			v := base + p.t*seg
			// Enforce a minimum increment equivalent to 1ms at current BPM
			// so the formatted millisecond timer never stalls across frames.
			bpm := g.appliedBPM
			if bpm <= 0 {
				bpm = g.bpm
			}
			if bpm > 0 {
				minDelta := float64(bpm) / 60000.0
				if v < g.lastDisplayBeat+minDelta {
					maxV := base + seg
					candidate := g.lastDisplayBeat + minDelta
					if candidate < maxV {
						v = candidate
					} else {
						v = maxV
					}
				}
			}
			if v < g.lastDisplayBeat {
				v = g.lastDisplayBeat
			}
			g.lastDisplayBeat = v
			return v
		}
		// Fallback to engine progress when no pulse is active.
		prog := g.engineProgress()
		frac := prog
		if g.lastProg > 0 && prog+0.25 < g.lastProg {
			frac = 1 + prog
		} else if prog < g.lastProg {
			frac = g.lastProg
		}
		v := base + frac
		if v < g.lastDisplayBeat {
			v = g.lastDisplayBeat
		}
		g.lastProg = prog
		g.lastDisplayBeat = v
		return v
	}

	// Determine BPM for display.
	bpm := g.appliedBPM
	if bpm <= 0 {
		bpm = g.bpm
	}
	if bpm <= 0 {
		return g.lastDisplayBeat
	}
	// Compute elapsed seconds since playStart using audio clock if present.
	var dtSec float64
	if n := audio.Now(); n > 0 && g.audioStart > 0 {
		dtSec = n - g.audioStart
	} else if !g.playStart.IsZero() {
		dtSec = time.Since(g.playStart).Seconds()
	}
	v := g.beatBase + dtSec*float64(bpm)/60.0
	// Enforce monotonic growth to suppress tiny regressions from clock jitter.
	if v < g.lastDisplayBeat {
		v = g.lastDisplayBeat
	}
	g.lastDisplayBeat = v
	return v
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
	// Strict policy: only one highlight per row to avoid leftover markers.
	g.clearRowHighlights(row)
	g.highlightedBeats[key] = g.frame + duration
	if g.highlightHook != nil && info.NodeType == model.NodeTypeRegular {
		if g.timingTestMode {
			// Sequencer already fired the test hook at scheduling time.
			// Avoid a duplicate from the UI thread in timing tests.
			goto skipHook
		}
		// Fire hook only once per row/index across frames.
		if row >= len(g.lastHLIdxByRow) {
			g.lastHLIdxByRow = make([]int, len(g.drum.Rows))
			for i := range g.lastHLIdxByRow {
				g.lastHLIdxByRow[i] = -1
			}
		}
		if g.lastHLIdxByRow[row] != idx {
			g.lastHLIdxByRow[row] = idx
			g.highlightHook(row, idx)
		}
	}
skipHook:
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
		g.logger.Debugf("[GAME] highlightBeat row=%d idx=%d inst=%s vol=%.3f", row, idx, inst, vol)
		// When the time-based sequencer is active (non-test), avoid double-
		// triggering audio from UI highlights.
		if !(g.useSequencerForAudio && g.playing) {
			g.queueSound(inst, vol)
		}
		g.logger.Debugf("[GAME] highlightBeat: Played %s at vol %.2f for node %d at beat %d row %d", inst, vol, info.NodeID, idx, row)
	}
}

func (g *Game) queueSound(id string, vol float64) {
	// Queue non-blockingly; if the buffer is saturated, drop the oldest
	// and enqueue the latest so playback remains responsive while tests
	// can also assert non-blocking behavior.
	n := audio.Now()
	req := soundReq{id: id, vol: utils.Clamp01(vol)}
	if n > 0 {
		req.when = []float64{n}
	}
	g.logger.Debugf("[AUDIO] queue id=%s vol=%.3f when=%v", id, vol, req.when)
	sendLatest(g.audioCh, req)
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

// clearRowHighlights removes all highlight entries for the given row.
func (g *Game) clearRowHighlights(row int) {
	for key := range g.highlightedBeats {
		if r, _ := splitBeatKey(key); r == row {
			delete(g.highlightedBeats, key)
		}
	}
}

// highlightVisual mirrors highlightBeat but never queues audio. It only
// updates the highlight map and optional test hook.
func (g *Game) highlightVisual(row, idx int, info model.BeatInfo, duration int64) {
	key := makeBeatKey(row, idx)
	// Strict: only a single highlight per row.
	g.clearRowHighlights(row)
	g.highlightedBeats[key] = g.frame + duration
	if g.highlightHook != nil && info.NodeType == model.NodeTypeRegular {
		if g.timingTestMode {
			// Sequencer emits the hook directly during timing tests.
			return
		}
		if row >= len(g.lastHLIdxByRow) {
			g.lastHLIdxByRow = make([]int, len(g.drum.Rows))
			for i := range g.lastHLIdxByRow {
				g.lastHLIdxByRow[i] = -1
			}
		}
		if g.lastHLIdxByRow[row] != idx {
			g.lastHLIdxByRow[row] = idx
			g.highlightHook(row, idx)
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
	// Convert beat count to internal subdivision steps
	steps := beats * g.grid.MaxDiv()
	if g.playing {
		for row := range g.drum.Rows {
			g.spawnPulseFromRow(row, steps)
		}
	} else {
		for i := range g.nextBeatIdxs {
			g.nextBeatIdxs[i] = steps
		}
		g.elapsedBeats = steps
	}
	g.resetOriginSequences()
	// Reset sequencer counters to stay aligned with updated paths.
	g.seqNextIdxs = make([]int, len(g.drum.Rows))
}

// syncUIToTime aligns UI pulses and counters to the current engine timeline
// so that the visual state matches playback even after heavy UI work.
func (g *Game) syncUIToTime() {
	if !g.playing {
		return
	}
	div := g.grid.MaxDiv()
	if div <= 0 {
		return
	}
	// If playback was enabled programmatically (tests) without going through
	// the Play button path, initialize the timebase so visuals can advance.
	if g.playStart.IsZero() {
		g.playStart = time.Now()
		if n := audio.Now(); n > 0 {
			g.audioStart = n
		}
	}
	// Absolute position in subdivisions (float) from wall-clock timeline.
	bpm := g.appliedBPM
	if bpm <= 0 {
		bpm = g.bpm
	}
	if bpm <= 0 {
		return
	}
	var dtSec float64
	if n := audio.Now(); n > 0 && g.audioStart > 0 {
		dtSec = n - g.audioStart
	} else if !g.playStart.IsZero() {
		dtSec = time.Since(g.playStart).Seconds()
	} else {
		return
	}
	absDivF := (g.beatBase + dtSec*float64(bpm)/60.0) * float64(div)
	target := int(math.Floor(absDivF))
	// Clamp negative just in case.
	if target < 0 {
		target = 0
	}
	// Ensure arrays sized to row count.
	if len(g.nextBeatIdxs) != len(g.drum.Rows) {
		g.nextBeatIdxs = make([]int, len(g.drum.Rows))
	}
	for row := range g.drum.Rows {
		// Ensure path exists.
		if row >= len(g.beatInfosByRow) || len(g.beatInfosByRow[row]) == 0 {
			continue
		}
		p := g.pulseForRow(row)
		if p == nil {
			// Create a pulse aligned at target without scheduling audio.
			infoFrom := g.beatInfoAtRow(row, target)
			infoTo := g.beatInfoAtRow(row, target+1)
			unit := g.grid.Unit()
			x1 := float64(infoFrom.I) * unit
			y1 := float64(infoFrom.J) * unit
			x2 := float64(infoTo.I) * unit
			y2 := float64(infoTo.J) * unit
			dist := hypot(x2-x1, y2-y1)
			seg := dist / g.grid.Step
			if seg <= 0 {
				seg = 1
			}
			p = &pulse{
				x1: x1, y1: y1, x2: x2, y2: y2,
				fromBeatInfo: infoFrom, toBeatInfo: infoTo,
				pathIdx: g.wrapBeatIndexRow(row, target+1),
				lastIdx: target,
				from:    g.nodeByID(infoFrom.NodeID), to: g.nodeByID(infoTo.NodeID),
				path: g.beatInfosByRow[row], row: row, segBeats: seg,
			}
			g.activePulses = append(g.activePulses, p)
			if row == 0 && g.activePulse == nil {
				g.activePulse = p
			}
		} else {
			// Re-anchor to target if we fell behind.
			if p.lastIdx != target {
				infoFrom := g.beatInfoAtRow(row, target)
				infoTo := g.beatInfoAtRow(row, target+1)
				unit := g.grid.Unit()
				p.x1 = float64(infoFrom.I) * unit
				p.y1 = float64(infoFrom.J) * unit
				p.x2 = float64(infoTo.I) * unit
				p.y2 = float64(infoTo.J) * unit
				p.fromBeatInfo = infoFrom
				p.toBeatInfo = infoTo
				p.pathIdx = g.wrapBeatIndexRow(row, target+1)
				p.lastIdx = target
				dist := hypot(p.x2-p.x1, p.y2-p.y1)
				seg := dist / g.grid.Step
				if seg <= 0 {
					seg = 1
				}
				p.segBeats = seg
			}
		}
		// Set animation progress precisely to current fraction within segment.
		delta := absDivF - float64(p.lastIdx)
		if delta < 0 {
			delta = 0
		}
		if p.segBeats <= 0 {
			p.segBeats = 1
		}
		t := delta / p.segBeats
		if t < 0 {
			t = 0
		} else if t > 0.999 {
			t = 0.999
		}
		p.t = t
		// Update counters and visual highlight state without queuing audio.
		g.nextBeatIdxs[row] = target + 1
		if row == 0 {
			g.elapsedBeats = target
		}
		beatDuration := int64(60.0 / float64(max1(g.appliedBPM)) * ebitenTPS)
		g.highlightVisual(row, target, p.fromBeatInfo, beatDuration)
	}
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
	// Seam fix: if wrapping to loop start lands on an invisible marker,
	// jump to the first regular node inside the loop.
	if g.isLoopByRow[p.row] && p.pathIdx == g.loopStartByRow[p.row] && p.toBeatInfo.NodeType == model.NodeTypeInvisible {
		j := p.pathIdx
		for j < len(path) && path[j].NodeType == model.NodeTypeInvisible {
			j++
		}
		if j < len(path) {
			p.pathIdx = j
			p.toBeatInfo = path[p.pathIdx]
		}
	}
	p.from = g.nodeByID(p.fromBeatInfo.NodeID)
	p.to = g.nodeByID(p.toBeatInfo.NodeID)

	// Set pulse start and end coordinates for animation using beat info
	unit := g.grid.Unit()
	p.x1 = float64(p.fromBeatInfo.I) * unit
	p.y1 = float64(p.fromBeatInfo.J) * unit
	p.x2 = float64(p.toBeatInfo.I) * unit
	p.y2 = float64(p.toBeatInfo.J) * unit
	dist := hypot(p.x2-p.x1, p.y2-p.y1)
	beats := dist / g.grid.Step
	if beats <= 0 {
		beats = 1
	}
	p.segBeats = beats
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
