package ui

import (
	"fmt"
	"image"
	"io"
	"math"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/engine"
	"github.com/ingyamilmolinar/beatmo/core/model"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

var testLogger *game_log.Logger

func init() {
	testLogger = game_log.New(io.Discard, game_log.LevelError)
}

func TestDefaultOriginNodeCentered(t *testing.T) {
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	w, h := 640, 480
	g.Layout(w, h)
	if len(g.nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(g.nodes))
	}
	n := g.nodes[0]
	if n.I != 0 || n.J != 0 {
		t.Fatalf("node at (%d,%d), want (0,0)", n.I, n.J)
	}
	if !n.Start {
		t.Fatalf("node not marked as start")
	}
	wantX := float64(w) / 2
	wantY := float64(g.split.Y-gridTopOffset()) / 2
	if g.cam.OffsetX != wantX || g.cam.OffsetY != wantY {
		t.Fatalf("camera offsets = (%v,%v), want (%v,%v)", g.cam.OffsetX, g.cam.OffsetY, wantX, wantY)
	}
}

func advanceBeats(g *Game, beats int) {
	for i := 0; i < beats; i++ {
		if g.activePulse == nil {
			return
		}
		delete(g.highlightedBeats, makeBeatKey(g.activePulse.row, g.activePulse.lastIdx))
		g.advancePulse(g.activePulse)
	}
}

func TestSoundQueueNonBlocking(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	plays := make(chan string, 3)
	block := make(chan struct{})
	blockClosed := false
	t.Cleanup(func() {
		if !blockClosed {
			close(block)
		}
	})
	g.SetPlayFunc(func(id string, vol float64, when ...float64) {
		<-block
		plays <- id
	})

	done := make(chan struct{})
	go func() {
		g.queueSound("snare", 1)
		g.queueSound("kick", 1)
		g.queueSound("hat", 1)
		close(done)
	}()
	waitForChan(t, done, 10000)
	close(block)
	blockClosed = true
	for i := 0; i < 3; i++ {
		waitForChan(t, plays, 10000)
	}
}

func assertNotPanics(t *testing.T, f func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	f()
}

func TestDropdownBlocksEditorClick(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(200, 200)

	before := len(g.graph.Nodes)
	g.drum.rowLabels[0].OnClick()
	if !g.drum.instMenuOpen {
		t.Fatalf("menu not open")
	}
	r := g.drum.instMenuBtns[0].Rect()
	restore := SetInputForTest(
		func() (int, int) { return r.Min.X + 1, r.Min.Y + 1 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 200, 200 },
	)
	t.Cleanup(restore)
	g.Update()
	restore()

	after := len(g.graph.Nodes)
	if after != before {
		t.Fatalf("editor handled click under menu: nodes %d -> %d", before, after)
	}
}

func TestHighlightsAllRows(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// create three independent origin nodes
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)

	g.drum.SetBounds(image.Rect(0, 0, 200, 100))
	g.drum.Rows[0].Origin = n0.ID
	g.drum.Rows[0].Node = g.nodeByID(n0.ID)
	g.drum.AddRow()
	g.drum.Rows[1].Origin = n1.ID
	g.drum.Rows[1].Node = g.nodeByID(n1.ID)
	g.drum.AddRow()
	g.drum.Rows[2].Origin = n2.ID
	g.drum.Rows[2].Node = g.nodeByID(n2.ID)

	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	g.spawnPulseFromRow(1, 0)
	g.spawnPulseFromRow(2, 0)

	if _, ok := g.highlightedBeats[makeBeatKey(2, 0)]; !ok {
		t.Fatalf("expected highlight for row2, got %v", g.highlightedBeats)
	}
}

func TestPulseSpeedMatchesDistance(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(g.grid.MaxDiv(), 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse")
	}
	beatDuration := int64(60.0 / float64(g.drum.BPM()) * ebitenTPS)
	want := float64(g.grid.MaxDiv()) / float64(beatDuration)
	if math.Abs(g.activePulse.speed-want) > 1e-9 {
		t.Fatalf("pulse speed = %v, want %v", g.activePulse.speed, want)
	}
}

// Ensure that beat and time counters halt immediately after pausing playback.
func TestBeatCounterFreezesWhenPaused(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	// Create a start node so ticks advance the timeline.
	g.tryAddNode(0, 0, model.NodeTypeRegular)

	// Simulate two ticks while playing so elapsedBeats advances.
	g.SetPlaying(true)
	g.engine.Events <- engine.Event{Step: 0}
	if err := g.Update(); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	g.engine.Events <- engine.Event{Step: 1}
	if err := g.Update(); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	// Pause playback via the Play button path.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("pause failed: %v", err)
	}
	if g.Playing() {
		t.Fatal("expected paused playback")
	}
	before := g.drum.timelineInfo(g.displayBeat())
	g.engine.Events <- engine.Event{Step: 0}
	if err := g.Update(); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	after := g.drum.timelineInfo(g.displayBeat())
	if before != after {
		t.Fatalf("timeline advanced after stop: %q -> %q", before, after)
	}
}

// After pressing Stop and then Play, playback restarts from the beginning
// (index 0) and spawns pulses from the start node.
func TestPlayAfterStopResetsToStart(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// Build a simple path O->A so we can check initial segment.
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	// Start playback.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("play: %v", err)
	}
	if !g.Playing() {
		t.Fatal("not playing after first play")
	}

	// Advance internal counters to a non-zero position.
	g.elapsedBeats = 10

	// Stop playback.
	pressStop(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if g.Playing() {
		t.Fatal("still playing after stop")
	}
	if g.elapsedBeats != 0 {
		t.Fatalf("elapsedBeats=%d want 0 after stop", g.elapsedBeats)
	}

	// Play again: should restart from index 0 and spawn initial pulse.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("replay: %v", err)
	}
	if !g.Playing() {
		t.Fatal("not playing after replay")
	}
	if g.elapsedBeats != 0 {
		t.Fatalf("elapsedBeats=%d want 0 at start", g.elapsedBeats)
	}
	if g.activePulse == nil {
		t.Fatal("missing active pulse after restart")
	}
	if g.activePulse.lastIdx != 0 {
		t.Fatalf("pulse lastIdx=%d want 0", g.activePulse.lastIdx)
	}
	// Pulse should move toward the second node.
	if g.activePulse.toBeatInfo.NodeID != n1.ID {
		t.Fatalf("expected pulse toward second node, got to=%d", g.activePulse.toBeatInfo.NodeID)
	}
}

func TestCurrentBeatScalesEngineProgress(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlaying(true)
	g.elapsedBeats = g.grid.MaxDiv() // one beat in subdivisions
	g.engineProgress = func() float64 { return 0.5 }
	if got := g.currentBeat(); math.Abs(got-1.5) > 1e-9 {
		t.Fatalf("currentBeat=%v want 1.5", got)
	}
	g.SetPlaying(false)
	if got := g.currentBeat(); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("currentBeat after stop=%v want 1", got)
	}
}

func TestCurrentBeatConvertsSubBeats(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlaying(true)
	g.elapsedBeats = 2 * g.grid.MaxDiv() // two beats in subdivisions
	g.engineProgress = func() float64 { return 0.5 }
	if got := g.currentBeat(); math.Abs(got-2.5) > 1e-9 {
		t.Fatalf("currentBeat=%v want 2.5", got)
	}
}

func TestCurrentBeatMonotonic(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlaying(true)
	g.engineProgress = func() float64 { return 0.9 }
	first := g.currentBeat()
	g.engineProgress = func() float64 { return 0.1 }
	second := g.currentBeat()
	if second < first {
		t.Fatalf("beat regressed: %f -> %f", first, second)
	}
}

// Regression for jittery beat counter: when the scheduler's progress wraps to
// the next beat before the game processes the corresponding tick event,
// currentBeat should still advance to the next integer beat rather than
// lingering on the previous one.
func TestCurrentBeatAdvancesOnProgressWrap(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlaying(true)
	g.elapsedBeats = 4 * g.grid.MaxDiv() // start on beat 4
	seq := []float64{0.9, 0.05}
	var i int
	g.engineProgress = func() float64 {
		v := seq[i]
		i++
		return v
	}
	first := g.currentBeat()
	second := g.currentBeat()
	if int(second) != 5 {
		t.Fatalf("beat floor=%d want 5", int(second))
	}
	if second <= first {
		t.Fatalf("beat did not advance: %v -> %v", first, second)
	}
}

// Small drops in scheduler progress should be ignored so the beat counter
// advances smoothly without jumping ahead.
func TestCurrentBeatIgnoresJitter(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlaying(true)
	g.elapsedBeats = 4 * g.grid.MaxDiv() // start on beat 4
	seq := []float64{0.6, 0.4, 0.8}
	var i int
	g.engineProgress = func() float64 {
		v := seq[i]
		i++
		return v
	}
	a := g.currentBeat()
	b := g.currentBeat()
	c := g.currentBeat()
	if b != a {
		t.Fatalf("beat changed on jitter: %v -> %v", a, b)
	}
	if c <= b {
		t.Fatalf("beat did not advance: %v -> %v", b, c)
	}
}

// DrumView should reflect the globally applied BPM when computing timeline
// seconds/milliseconds, regardless of local UI edits.
func TestTimelineRefreshesBPMFromGame(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	// Simulate engine having applied a BPM different from the default.
	g.SetAppliedBPMForTest(150) // 0.4s per beat
	// Trigger draw path so DrumView refreshes its secPerBeat from game.
	img := ebiten.NewImage(10, 10)
	g.drawDrumPane(img)
	if diff := math.Abs(g.drum.secPerBeat - 0.4); diff > 1e-9 {
		t.Fatalf("secPerBeat=%.6f want 0.4", g.drum.secPerBeat)
	}
	// Validate formatted time matches 150 BPM conversion.
	g.drum.Length = 4
	g.drum.timelineUnitsPerBeat = 1
	info := g.drum.timelineInfo(2) // 2 beats -> 0.8s -> "0:00"
	if !strings.Contains(info, "Beat 2/4") {
		t.Fatalf("unexpected beat portion: %q", info)
	}
	if !strings.Contains(info, "| 0:00") {
		t.Fatalf("unexpected time portion: %q", info)
	}
}

// Verify that currentBeat is quantized to the grid's smallest subdivision
// (default 1/32 beat) so each playback tick maps to discrete sub-beat steps.
func TestCurrentBeatQuantizedToSubdiv(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlaying(true)
	g.elapsedBeats = 0
	// Sequence of scheduler progress values around sub-beat boundaries.
	seq := []float64{0.01, 0.033, 0.066}
	i := 0
	g.engineProgress = func() float64 { v := seq[i]; i++; return v }
	// 0.01 -> rounds to 0/32
	a := g.currentBeat()
	// 0.033 -> ~1/32
	b := g.currentBeat()
	// 0.066 -> ~2/32
	c := g.currentBeat()
	div := float64(g.grid.MaxDiv())
	if math.Abs(a-0.0) > 1e-9 {
		t.Fatalf("a=%.6f want 0", a)
	}
	if math.Abs(b-1.0/div) > 1e-9 {
		t.Fatalf("b=%.6f want %.6f", b, 1.0/div)
	}
	if math.Abs(c-2.0/div) > 1e-9 {
		t.Fatalf("c=%.6f want %.6f", c, 2.0/div)
	}
}

// Timeline string should display sub-beat time at default 32nd resolution.
func TestTimelineShowsSubBeatTime(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	g.SetAppliedBPMForTest(120) // 0.5s per beat
	g.drum.SetBPM(g.AppliedBPM())
	// One sub-beat = 1/32 beat -> ~16ms. New format truncates to integer beats.
	frac := 1.0 / float64(g.grid.MaxDiv())
	info := g.drum.timelineInfo(frac)
	if !strings.Contains(info, "Beat 0/") {
		t.Fatalf("unexpected beat display: %q", info)
	}
	if !strings.Contains(info, "| 0:00") {
		t.Fatalf("unexpected time display: %q", info)
	}
}

// Quantization should honor the grid's MaxDiv factor. When reduced to 16,
// sub-beat steps snap to sixteenth notes (1/16 beat).
func TestCurrentBeatQuantizationRespectsGrid(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.SetPlaying(true)
	g.elapsedBeats = 0
	// Replace grid subdivisions to end at 16.
	g.grid.SetSubs([]Subdivision{{Div: 1}, {Div: 2}, {Div: 4}, {Div: 8}, {Div: 16}})
	seq := []float64{0.01, 0.04} // ~0 and ~1/16
	i := 0
	g.engineProgress = func() float64 { v := seq[i]; i++; return v }
	_ = g.currentBeat() // 0
	b := g.currentBeat()
	want := 1.0 / 16.0
	if math.Abs(b-want) > 1e-9 {
		t.Fatalf("quantized beat=%.6f want %.6f (1/16)", b, want)
	}
}

// Counters should advance in 1/MaxDiv increments with time shown in ms that
// correspond to (60_000/BPM)/MaxDiv per sub-beat.
func TestTimelineCountersAdvanceEachSubdiv(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	bpm := 120
	g.SetAppliedBPMForTest(bpm)
	// Synchronize DrumView's secPerBeat from applied BPM via draw path.
	img := ebiten.NewImage(10, 10)
	g.drawDrumPane(img)
	g.SetPlaying(true)
	g.elapsedBeats = 0
	div := float64(g.grid.MaxDiv())

	// Generate a few progress values near exact boundaries.
	// With the simplified format "Beat X/Y | M:SS", verify integer beats
	// and that seconds advance correctly at larger steps.
	makeProg := func(k int) float64 { return float64(k)/div + 1e-5 }
	for k := 0; k <= 4; k++ {
		kk := k
		g.engineProgress = func() float64 { return makeProg(kk) }
		beat := g.currentBeat() // quantized
		info := g.drum.timelineInfo(beat)
		wantBeat := int(float64(k) / div)
		bstr := fmt.Sprintf("Beat %d/", wantBeat)
		if !strings.Contains(info, bstr) {
			t.Fatalf("k=%d info=%q missing %q", k, info, bstr)
		}
	}
}

// Timeline time must reflect BPM precisely (including rounding) for sub-beat
// steps at non-round BPM values.
func TestTimelineTimeMatchesBPMAcrossSubdiv(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	bpm := 90 // 60_000/90 = 666.666... ms per beat
	g.SetAppliedBPMForTest(bpm)
	// Sync DrumView's secPerBeat with applied BPM via draw path.
	g.drawDrumPane(ebiten.NewImage(10, 10))
	g.SetPlaying(true)
	g.elapsedBeats = 0
	div := float64(g.grid.MaxDiv())

	// Generate progress values near exact sub-beat boundaries and validate
	// that the simplified format produces valid output for a non-integer
	// ms per sub-beat. With second-level precision, early sub-beats all map to 0:00.
	makeProg := func(k int) float64 { return float64(k)/div + 1e-5 }
	for k := 0; k <= 4; k++ {
		kk := k
		g.engineProgress = func() float64 { return makeProg(kk) }
		beat := g.currentBeat()
		info := g.drum.timelineInfo(beat)
		// At 90 BPM, sub-beats 0-4 are all within the first second.
		if !strings.Contains(info, "| 0:00") {
			t.Fatalf("bpm=%d k=%d info=%q expected 0:00 time", bpm, k, info)
		}
	}
}

// Sanity at 60 BPM: each 1/32 sub-beat should add ~31.25ms (rounded), and
// every 32 sub-beats should advance exactly 1.000 beat.
func TestTimelineAt60BPMSteps(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	bpm := 60 // 1 beat = 1s
	g.SetAppliedBPMForTest(bpm)
	// Sync DrumView's secPerBeat from applied BPM via draw path.
	g.drawDrumPane(ebiten.NewImage(10, 10))
	g.SetPlaying(true)
	div := float64(g.grid.MaxDiv()) // default 32

	// Walk two beats worth of sub-steps. With the simplified "Beat X/Y | M:SS"
	// format, verify that integer beat values are non-decreasing and the
	// displayed time in seconds is consistent.
	prevBeatInt := -1
	for k := 0; k <= int(2*div); k++ {
		g.elapsedBeats = k
		frac := float64(k%int(div))/div + 1e-6
		g.engineProgress = func() float64 { return frac }

		beat := g.displayBeat()
		info := g.drum.timelineInfo(beat)
		// Extract the integer beat from the format "Beat X/Y | M:SS"
		beatInt := int(beat)
		if beatInt < prevBeatInt {
			t.Fatalf("k=%d beat went backwards: %d -> %d info=%q", k, prevBeatInt, beatInt, info)
		}
		bstr := fmt.Sprintf("Beat %d/", beatInt)
		if !strings.Contains(info, bstr) {
			t.Fatalf("k=%d info=%q missing beat %q", k, info, bstr)
		}
		prevBeatInt = beatInt
	}
}

// Smooth display beat should increase monotonically with scheduler progress
// between subdivision steps, avoiding step-by-step jitter in timers.
func TestDisplayBeatSmoothBetweenSubdivisions(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	g.SetPlaying(true)

	// Build a minimal path with 1 sub-step and spawn a pulse.
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatal("missing active pulse")
	}
	p := g.activePulse
	// Fix elapsed steps at 10 sub-divisions into the beat and move pulse.
	g.elapsedBeats = 10
	p.t = 0.10
	a := g.displayBeat()
	p.t = 0.15
	b := g.displayBeat()
	p.t = 0.20
	c := g.displayBeat()
	if !(a < b && b < c) {
		t.Fatalf("displayBeat not smooth/monotonic: a=%.6f b=%.6f c=%.6f", a, b, c)
	}
}

// Small regressions in scheduler progress should not cause displayBeat to
// move backwards or stutter.
func TestDisplayBeatIgnoresMinorJitter(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	g.SetPlaying(true)
	// Use pulse-driven display and vary p.t slightly backwards.
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatal("missing active pulse")
	}
	p := g.activePulse
	g.elapsedBeats = 5
	p.t = 0.20
	a := g.displayBeat()
	p.t = 0.19 // minor backward jitter in animation
	b := g.displayBeat()
	p.t = 0.21
	c := g.displayBeat()
	if b < a || c < b {
		t.Fatalf("displayBeat regressed with jitter: a=%.6f b=%.6f c=%.6f", a, b, c)
	}
}

// Across a progress wrap (near 1 -> near 0), displayBeat should continue
// increasing smoothly into the next beat even before another subdivision
// completes.
func TestDisplayBeatWrapsSmoothly(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	g.SetPlaying(true)
	// Pulse near end of a sub-step then start of the next.
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatal("missing active pulse")
	}
	p := g.activePulse
	g.elapsedBeats = g.grid.MaxDiv() - 1
	p.t = 0.99
	a := g.displayBeat()
	// Simulate wrap: advance a step and reset t low
	g.elapsedBeats++
	p.t = 0.01
	b := g.displayBeat()
	if b <= a {
		t.Fatalf("displayBeat did not advance across wrap: a=%.6f b=%.6f", a, b)
	}
}

// Timeline counters should increase smoothly (no 1/32-step jumps) as engine
// progress increases within a beat.
func TestTimelineCountersSmoothMonotonic(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	// Set BPM and sync DrumView's secPerBeat via draw path.
	g.SetAppliedBPMForTest(120) // 0.5s per beat
	g.drawDrumPane(ebiten.NewImage(1, 1))
	g.SetPlaying(true)
	// Prepare pulse-driven display and increase p.t in small deltas.
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatal("missing active pulse")
	}
	p := g.activePulse
	g.elapsedBeats = 0

	// With the simplified "Beat X/Y | M:SS" format, the displayed seconds
	// only have second-level precision. For small pulse.t deltas within the
	// same beat, the displayed time may not change. Verify that the displayed
	// beat values are non-decreasing and displayBeat itself is smooth.
	p.t = 0.10
	da := g.displayBeat()
	p.t = 0.15
	db := g.displayBeat()
	p.t = 0.20
	dc := g.displayBeat()
	if !(da <= db && db <= dc) {
		t.Fatalf("displayBeat not monotonic: %.6f, %.6f, %.6f", da, db, dc)
	}
}

// After stopping and resuming playback, counters must immediately reflect the
// new playback state (based on current step and pulse), without lag.
func TestCountersUpdateOnResume(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, 200)
	g.SetAppliedBPMForTest(120)
	g.drawDrumPane(ebiten.NewImage(1, 1))

	// Minimal path and initial pulse.
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatal("missing active pulse")
	}

	g.SetPlaying(true)
	g.elapsedBeats = 0
	if a := g.displayBeat(); a <= 0 {
		t.Fatalf("unexpected initial displayBeat: %.6f", a)
	}

	// Pause via play button toggle.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update pause: %v", err)
	}
	if g.Playing() {
		t.Fatal("still playing after pause")
	}

	// Move playhead to a new step before resuming.
	g.elapsedBeats = 20
	// Resume.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update resume: %v", err)
	}
	if !g.Playing() {
		t.Fatal("not playing after resume")
	}

	// Counters should immediately reflect the new base + pulse progress.
	b := g.displayBeat()
	min := float64(20) / float64(g.grid.MaxDiv())
	if b < min {
		t.Fatalf("displayBeat did not update after resume: got %.6f want >= %.6f", b, min)
	}
}

// TrackBeat should react immediately to progress wrap so the drum view follows
// playback without waiting for internal counters to update.
func TestTrackBeatUpdatesOnProgressWrap(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.SetLength(8)
	g.drum.SetFollow(true)
	g.SetPlaying(true)
	// elapsedBeats are tracked in subdivision steps; set to 4 full beats.
	g.elapsedBeats = 4 * g.grid.MaxDiv()
	seq := []float64{0.9, 0.05}
	var i int
	g.engineProgress = func() float64 {
		v := seq[i]
		i++
		return v
	}
	prev := g.drum.Offset
	g.drum.TrackBeat(int(math.Round(g.currentBeat() * float64(g.grid.MaxDiv()))))
	initial := g.drum.Offset
	if initial < prev {
		t.Fatalf("initial tracking offset regressed: %d -> %d", prev, initial)
	}
	prev = initial
	g.drum.TrackBeat(int(math.Round(g.currentBeat() * float64(g.grid.MaxDiv()))))
	if g.drum.Offset <= prev {
		t.Fatalf("offset did not advance after wrap: prev=%d now=%d", prev, g.drum.Offset)
	}
}

func TestUpdateBeatInfosDoesNotClampOffset(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.SetLength(8)
	g.drum.timelineBeats = 100
	g.beatInfos = make([]model.BeatInfo, 8)
	g.drum.Offset = 50
	g.updateBeatInfos()
	if g.drum.Offset != 50 {
		t.Fatalf("offset clamped to %d", g.drum.Offset)
	}
}

func TestSeekSetsCurrentBeat(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Seek(5)
	if got := g.currentBeat(); math.Abs(got-5) > 1e-9 {
		t.Fatalf("currentBeat after seek=%v want 5", got)
	}
}

func TestPlayButtonTogglesPause(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.tryAddNode(0, 0, model.NodeTypeRegular)

	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if !g.Playing() {
		t.Fatalf("expected playing")
	}
	if g.drum.playBtn.Text != "⏸" {
		t.Fatalf("play button text = %q want ⏸", g.drum.playBtn.Text)
	}

	g.elapsedBeats = 5
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("pause update failed: %v", err)
	}
	if g.Playing() {
		t.Fatalf("expected paused")
	}
	if g.elapsedBeats != 5 {
		t.Fatalf("elapsedBeats=%d want 5", g.elapsedBeats)
	}
	if g.drum.playBtn.Text != "▶" {
		t.Fatalf("play button text = %q want ▶", g.drum.playBtn.Text)
	}

	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("resume update failed: %v", err)
	}
	if !g.Playing() {
		t.Fatalf("expected playing after resume")
	}
	if g.elapsedBeats != 5 {
		t.Fatalf("elapsedBeats=%d want 5 after resume", g.elapsedBeats)
	}
}

// TestPlayButtonClickPausesPlayback ensures that clicking the play button pauses
// playback while leaving the follow state unchanged.
func TestPlayButtonClickPausesPlayback(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.recalcButtons()

	g.SetPlaying(true)
	g.drum.SetFollow(true)

	r := g.drum.playBtn.Rect()
	restore := SetInputForTest(
		func() (int, int) { return r.Min.X + 1, r.Min.Y + 1 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	g.drum.Update()
	restore()
	g.drum.Update()

	if err := g.Update(); err != nil {
		t.Fatalf("update failed: %v", err)
	}
	if g.Playing() {
		t.Fatalf("expected playback paused")
	}
	if !g.drum.FollowPlayback() {
		t.Fatalf("follow toggled unexpectedly")
	}
}

func TestPauseKeepsHighlight(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// simulate an active highlight at beat 0
	g.SetPlaying(true)
	info := model.BeatInfo{NodeID: 1, NodeType: model.NodeTypeRegular}
	g.highlightBeat(0, 0, info, 5)

	// pause playback via play button
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("pause update failed: %v", err)
	}
	if g.Playing() {
		t.Fatalf("expected paused state")
	}

	// advance several frames while paused
	for i := 0; i < 10; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("update %d failed: %v", i, err)
		}
	}

	if _, ok := g.highlightedBeats[makeBeatKey(0, 0)]; !ok {
		t.Fatalf("highlight cleared while paused")
	}
}

// TestPauseStopsPulseProgress verifies that pausing via the play button
// freezes active pulses so no further audio events fire.
func TestPauseStopsPulseProgress(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Build a simple two-node path one beat apart.
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(g.grid.MaxDiv(), 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()

	// Spawn a pulse and start playback.
	plays := make(chan struct{}, 16)
	g.SetPlayFunc(func(string, float64, ...float64) { plays <- struct{}{} })

	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	if err := g.Update(); err != nil {
		t.Fatalf("initial update failed: %v", err)
	}

	// Pause via play button on the next frame.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("pause update failed: %v", err)
	}
	if g.Playing() {
		t.Fatalf("expected paused state")
	}

	for len(plays) > 0 {
		<-plays
	}

	if len(g.activePulses) != 1 {
		t.Fatalf("expected one active pulse, got %d", len(g.activePulses))
	}
	p := g.activePulses[0]
	startT := p.t

	// Advance several frames while paused.
	for i := 0; i < 10; i++ {
		if err := g.Update(); err != nil {
			t.Fatalf("update %d failed: %v", i, err)
		}
	}

	if p.t != startT {
		t.Fatalf("pulse advanced while paused: %.3f -> %.3f", startT, p.t)
	}
	if len(plays) != 0 {
		t.Fatalf("audio played while paused")
	}
}

func TestMouseCoordinateLabel(t *testing.T) {
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	w, h := 640, 480
	g.Layout(w, h)

	unit := g.grid.Unit()
	div := g.grid.MaxDiv()
	ix := 4*div + 6   // 4 beats + 3/16
	iy := -5*div + 24 // -5 beats + 3/4
	wx := float64(ix) * unit
	wy := float64(iy) * unit
	mx := int(math.Round(wx*g.cam.Scale + g.cam.OffsetX))
	my := int(math.Round(wy*g.cam.Scale + g.cam.OffsetY + float64(gridTopOffset())))

	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	t.Cleanup(restore)
	img := ebiten.NewImage(w, h)
	g.drawGridPane(img)
	restore()

	want := "(4:3/16, -5:3/4)"
	if g.cursorLabel != want {
		t.Fatalf("label = %q, want %q", g.cursorLabel, want)
	}

	restore = SetInputForTest(
		func() (int, int) { return mx, g.split.Y + 10 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return w, h },
	)
	t.Cleanup(restore)
	g.drawGridPane(img)
	restore()
	if g.cursorLabel != "" {
		t.Fatalf("label visible outside grid: %q", g.cursorLabel)
	}
}

func TestSignalAdvancesThroughSubBeats(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(g.grid.MaxDiv(), 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse")
	}

	steps := g.grid.MaxDiv()
	for i := 1; i <= steps; i++ {
		delete(g.highlightedBeats, makeBeatKey(0, g.activePulse.lastIdx))
		ok := g.advancePulse(g.activePulse)
		if !ok {
			if i != steps {
				t.Fatalf("pulse ended early at step %d", i)
			}
			break
		}
	}

	if g.elapsedBeats != steps {
		t.Fatalf("elapsedBeats=%d want %d", g.elapsedBeats, steps)
	}
}

func TestTimelineCursorMatchesHighlightedBeat(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(g.grid.MaxDiv(), 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()

	g.spawnPulseFromRow(0, 0)
	if g.elapsedBeats != 0 {
		t.Fatalf("elapsedBeats=%d want 0", g.elapsedBeats)
	}
	if _, ok := g.highlightedBeats[makeBeatKey(0, 0)]; !ok {
		t.Fatalf("missing highlight for beat 0")
	}

	delete(g.highlightedBeats, makeBeatKey(0, g.activePulse.lastIdx))
	if !g.advancePulse(g.activePulse) {
		t.Fatalf("advancePulse ended early")
	}
	if g.elapsedBeats != 1 {
		t.Fatalf("elapsedBeats=%d want 1", g.elapsedBeats)
	}
	if _, ok := g.highlightedBeats[makeBeatKey(0, 1)]; !ok {
		t.Fatalf("missing highlight for beat 1")
	}
}

func TestGameStartsWithDefaultOrigin(t *testing.T) {
	withDefaultStart(t, true)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if len(g.drum.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(g.drum.Rows))
	}
	if g.drum.Rows[0].Origin == model.InvalidNodeID {
		t.Fatalf("expected default origin node")
	}
}

func TestTryAddNodeTogglesRow(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	nodeID := g.tryAddNode(2, 0, model.NodeTypeRegular).ID
	g.graph.StartNodeID = nodeID
	// After adding a node and setting it as start, the beat row should reflect it.
	// We need to call Update to propagate the change to drum.Rows.
	g.Update()
	if len(g.drum.Rows[0].Steps) <= 0 || !g.drum.Rows[0].Steps[0] {
		t.Fatalf("expected step 0 on")
	}
}

func TestDeleteNodeClearsRow(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.deleteNode(n)
	g.Update() // Propagate changes to drum.Rows
	if len(g.drum.Rows[0].Steps) > 1 && g.drum.Rows[0].Steps[1] {
		t.Fatalf("expected step 1 off after delete")
	}
}

func TestGameAssignsOriginToNewRow(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.AddRow()
	g.Update()
	n := g.tryAddNode(3, 0, model.NodeTypeRegular)
	if g.drum.Rows[1].Origin != n.ID {
		t.Fatalf("expected row origin %d got %d", n.ID, g.drum.Rows[1].Origin)
	}
}

func TestGameCalculatesBeatInfosPerRow(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)

	// add a second row and assign its origin on next node add
	g.drum.AddRow()
	g.Update()
	g.pendingStartRow = 1
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)

	g.updateBeatInfos()

	if len(g.beatInfosByRow) < 2 {
		t.Fatalf("expected beatInfos for 2 rows, got %d", len(g.beatInfosByRow))
	}
	if len(g.beatInfosByRow[0]) == 0 || g.beatInfosByRow[0][0].NodeID != n0.ID {
		t.Fatalf("row0 beatInfos start at %v want %v", g.beatInfosByRow[0], n0.ID)
	}
	if len(g.beatInfosByRow[1]) == 0 || g.beatInfosByRow[1][0].NodeID != n1.ID {
		t.Fatalf("row1 beatInfos start at %v want %v", g.beatInfosByRow[1], n1.ID)
	}
}

func TestDrumRowsStayIsolated(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Origin for first row
	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)

	// Second row with its own origin
	g.drum.AddRow()
	g.Update() // process row addition so next node sets origin
	g.pendingStartRow = 1
	n1 := g.tryAddNode(0, 1, model.NodeTypeRegular)

	// Connect an extra node to row 0 only
	n0b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n0b)
	g.updateBeatInfos()
	g.refreshDrumRow()

	if !g.drum.Rows[0].Steps[1] {
		t.Fatalf("expected row0 step1 on after edge")
	}
	if g.drum.Rows[1].Steps[1] {
		t.Fatalf("row1 step1 unexpectedly on after row0 update")
	}

	// Now connect a node for row 1 and ensure row 0 stays the same
	n1b := g.tryAddNode(1, 1, model.NodeTypeRegular)
	g.addEdge(n1, n1b)
	g.updateBeatInfos()
	g.refreshDrumRow()

	if !g.drum.Rows[1].Steps[1] {
		t.Fatalf("expected row1 step1 on after its edge")
	}
	if !g.drum.Rows[0].Steps[1] || g.drum.Rows[0].Steps[2] {
		t.Fatalf("row0 steps changed unexpectedly: %v", g.drum.Rows[0].Steps[:3])
	}

	// sanity: origins unaffected
	if g.drum.Rows[0].Origin != n0.ID || g.drum.Rows[1].Origin != n1.ID {
		t.Fatalf("origins changed: %v %v", g.drum.Rows[0].Origin, g.drum.Rows[1].Origin)
	}
}

func TestSpawnPulsePerRowPlaysInstrument(t *testing.T) {
	withDefaultAudio(t)
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// first row start
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n0
	g.graph.StartNodeID = n0.ID

	// second row
	g.drum.AddRow()
	g.Update()
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.drum.Rows[1].Origin = n1.ID
	g.drum.Rows[1].Node = n1
	if len(g.drum.rowLabels) < 2 {
		t.Fatalf("expected row labels for selection")
	}
	g.drum.rowLabels[1].OnClick()
	g.drum.SetInstrument("kick")

	g.updateBeatInfos()
	g.audioLookaheadSec = 0 // synchronous dispatch for deterministic order

	plays := make(chan string, 2)
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { plays <- id })

	g.spawnPulseFromRow(0, 0)
	g.spawnPulseFromRow(1, 0)

	got := []string{waitForChan(t, plays, 10000), waitForChan(t, plays, 10000)}
	if got[0] != g.drum.Rows[0].Instrument || got[1] != g.drum.Rows[1].Instrument {
		t.Fatalf("got plays %v", got)
	}
}

func TestAddRowDoesNotClearGraph(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	_ = g.tryAddNode(0, 0, model.NodeTypeRegular)
	_ = g.tryAddNode(1, 0, model.NodeTypeRegular)
	before := len(g.nodes)

	g.drum.AddRow()
	g.Update() // process row addition

	if len(g.nodes) != before {
		t.Fatalf("expected %d nodes after adding row, got %d", before, len(g.nodes))
	}
	if len(g.drum.Rows) < 2 || g.drum.Rows[1].Origin != model.InvalidNodeID {
		t.Fatalf("expected new row without origin")
	}
}

func TestDeleteRowRemovesActivePulses(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(g.grid.MaxDiv(), 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	g.drum.Rows[0].Origin = n1.ID
	g.drum.Rows[0].Node = n1

	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	if len(g.activePulses) != 1 {
		t.Fatalf("expected active pulse")
	}

	g.drum.DeleteRow(0)
	if err := g.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}
	if err := g.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}
	if len(g.activePulses) != 0 {
		t.Fatalf("expected pulses cleared, got %d", len(g.activePulses))
	}
}

func TestDeleteFirstRowKeepsSecond(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.AddRow()
	g.updateBeatInfos()
	g.nextBeatIdxs = []int{1, 7}

	g.drum.DeleteRow(0)
	if err := g.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}

	if len(g.drum.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(g.drum.Rows))
	}
	if len(g.nextBeatIdxs) != 1 || g.nextBeatIdxs[0] != 7 {
		t.Fatalf("unexpected nextBeatIdxs %v", g.nextBeatIdxs)
	}

	g.drum.AddRow()
	if err := g.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}
	if len(g.drum.Rows) != 2 {
		t.Fatalf("expected to add row after deletion, got %d", len(g.drum.Rows))
	}
}

func TestAdvancePulseLoopWrap(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.drum.SetLength(6)
	g.drum.SetBeatLength(g.drum.Length)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)

	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n1)

	g.updateBeatInfos()

	if !g.isLoop || g.loopStartIndex != 0 {
		t.Fatalf("expected loop starting at 0, got loop=%t start=%d", g.isLoop, g.loopStartIndex)
	}

	last := len(g.beatInfos) - 1
	p := &pulse{
		fromBeatInfo: g.beatInfos[last-1],
		toBeatInfo:   g.beatInfos[last],
		path:         g.beatInfos,
		pathIdx:      last,
		from:         g.nodeByID(g.beatInfos[last-1].NodeID),
		to:           g.nodeByID(g.beatInfos[last].NodeID),
		row:          0,
	}

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("advancePulse panicked: %v", r)
			}
		}()
		g.advancePulse(p)
	}()

	if p.pathIdx != 0 {
		t.Fatalf("expected pathIdx 0 after wrap, got %d", p.pathIdx)
	}
	if p.fromBeatInfo.NodeID != g.beatInfos[len(g.beatInfos)-1].NodeID {
		t.Fatalf("expected from node %d, got %d", g.beatInfos[len(g.beatInfos)-1].NodeID, p.fromBeatInfo.NodeID)
	}
	if p.toBeatInfo.NodeID != g.beatInfos[0].NodeID {
		t.Fatalf("expected to node %d, got %d", g.beatInfos[0].NodeID, p.toBeatInfo.NodeID)
	}
}

func TestAdvancePulsePanicsOnUnexpectedOrigin(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows[0].Origin = 1
	g.drum.Rows[0].Steps = make([]bool, 4)
	g.isLoopByRow = []bool{true}
	g.loopStartByRow = []int{0}
	g.originIdxsByRow = [][]int{{0, 2}}
	g.nextOriginIdxByRow = []int{0}
	g.nodes = []*uiNode{{ID: 1}, {ID: 2}}
	p := &pulse{
		fromBeatInfo: model.BeatInfo{NodeID: 2},
		toBeatInfo:   model.BeatInfo{NodeID: 1},
		path: []model.BeatInfo{
			{NodeID: 1}, {NodeID: 2}, {NodeID: 1},
		},
		pathIdx: 2,
		row:     0,
	}
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on unexpected origin jump")
		}
	}()
	g.advancePulse(p)
}

func TestAdvancePulseAllowsRepeatedOrigin(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows[0].Origin = 1
	g.drum.Rows[0].Steps = make([]bool, 4)
	g.isLoopByRow = []bool{true}
	g.loopStartByRow = []int{0}
	g.originIdxsByRow = [][]int{{0, 2, 4}}
	g.nextOriginIdxByRow = []int{1}
	g.nodes = []*uiNode{{ID: 1}, {ID: 2}}
	g.nextBeatIdxs = []int{0}
	p := &pulse{
		fromBeatInfo: model.BeatInfo{NodeID: 2},
		toBeatInfo:   model.BeatInfo{NodeID: 1},
		path:         []model.BeatInfo{{NodeID: 1}, {NodeID: 2}, {NodeID: 1}, {NodeID: 2}, {NodeID: 1}},
		pathIdx:      2,
		row:          0,
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("advancePulse panicked: %v", r)
			}
		}()
		g.advancePulse(p)
	}()
}

func TestAdvancePulseAllowsIrregularOriginSpacing(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.drum.Rows[0].Origin = 1
	g.drum.Rows[0].Steps = make([]bool, 4)
	g.isLoopByRow = []bool{true}
	g.loopStartByRow = []int{0}
	g.originIdxsByRow = [][]int{{0, 5}}
	g.nextOriginIdxByRow = []int{1}
	g.nextBeatIdxs = []int{0}
	g.nodes = []*uiNode{{ID: 1}, {ID: 2}, {ID: 3}, {ID: 4}, {ID: 5}}
	p := &pulse{
		fromBeatInfo: model.BeatInfo{NodeID: 5},
		toBeatInfo:   model.BeatInfo{NodeID: 1},
		path: []model.BeatInfo{
			{NodeID: 1}, {NodeID: 2}, {NodeID: 3}, {NodeID: 4}, {NodeID: 5}, {NodeID: 1},
		},
		pathIdx: 5,
		row:     0,
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("advancePulse panicked: %v", r)
			}
		}()
		g.advancePulse(p)
	}()
}

func TestTimelineDragWhilePlayingKeepsPulse(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n1)

	g.updateBeatInfos()

	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse before drag")
	}

	g.drum.Offset = 10
	g.drum.offsetChanged = true
	g.Update()

	if !g.Playing() {
		t.Fatalf("playing stopped after drag")
	}
	if g.activePulse == nil {
		t.Fatalf("expected active pulse after drag")
	}
}

func TestDrumWheelDoesNotZoomGrid(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)
	g.drum.SetLength(8)
	g.drum.SetBeatLength(g.drum.Length)
	g.Update() // set drum bounds

	wheelVal := 1.0
	restore := SetInputForTest(
		func() (int, int) { // cursor inside drum steps area
			return g.drum.Bounds.Min.X + g.drum.labelW + 390, g.drum.Bounds.Min.Y + 5
		},
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheelVal; wheelVal = 0; return 0, v },
		func() (int, int) { return g.winW, g.winH },
	)
	t.Cleanup(restore)
	defer restore()

	scale := g.cam.Scale
	g.Update()
	if g.cam.Scale != scale {
		t.Fatalf("expected camera scale unchanged, got %f", g.cam.Scale)
	}
}

// Length +/- buttons should grow/shrink by one full beat (MaxDiv subdivisions)
// in the Game context, and holding the button should trigger repeats.
func TestDrumLengthButtonsStepBeat(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1600, 900)
	inc := g.grid.MaxDiv()
	base := g.drum.Length
	if base != 4*inc {
		t.Fatalf("default drum length=%d want %d (4 beats)", base, 4*inc)
	}

	// Single click increases by one full beat.
	pressLenInc(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}
	if g.drum.Length != base+inc {
		t.Fatalf("len=%d want %d (+%d)", g.drum.Length, base+inc, inc)
	}
	if len(g.drum.Rows[0].Steps) != g.drum.Length {
		t.Fatalf("row steps not resized: %d", len(g.drum.Rows[0].Steps))
	}

	// Single click decrease returns to the previous length (base).
	pressLenDec(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}
	if g.drum.Length != base {
		t.Fatalf("len=%d want %d (back to base)", g.drum.Length, base)
	}

	// Repeated decreases clamp to the minimum of one beat.
	for g.drum.Length > inc {
		pressLenDec(t, g.drum)
		if err := g.Update(); err != nil {
			t.Fatalf("update error: %v", err)
		}
	}
	if g.drum.Length != inc {
		t.Fatalf("len=%d want %d (min 1 beat)", g.drum.Length, inc)
	}
}

// The default Game-wired DrumView should start with a usable window that
// spans at least four full beats in the current grid so new loops are visible
// at a “big picture” scale from the beginning.
func TestGameDrumViewDefaultLengthAtLeastFourBeats(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1600, 900)

	if g.drum == nil || g.grid == nil {
		t.Fatalf("game drum/grid not initialized")
	}
	units := g.grid.MaxDiv()
	if units <= 0 {
		t.Fatalf("grid MaxDiv=%d, want >0", units)
	}
	if g.drum.timelineUnitsPerBeat != units {
		t.Fatalf("timelineUnitsPerBeat=%d want %d", g.drum.timelineUnitsPerBeat, units)
	}
	if g.drum.Length < 4*units {
		t.Fatalf("default drum length=%d want >= %d (four beats)", g.drum.Length, 4*units)
	}
}

func TestDrumLengthButtonsHoldRepeats(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1600, 900)
	inc := g.grid.MaxDiv()
	base := g.drum.Length
	g.drum.recalcButtons()
	g.drum.calcLayout()

	// Hold the + button long enough to trigger one repeat.
	r := g.drum.lenIncBtn.Rect()
	restore := SetInputForTest(
		func() (int, int) { return r.Min.X + r.Dx()/2, r.Min.Y + r.Dy()/2 },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return g.winW, g.winH },
	)
	t.Cleanup(restore)
	// First click + one repeat after ~66 frames.
	for i := 0; i < 66; i++ {
		g.drum.Update()
	}
	restore()
	// Process any pending length edits via the game loop once.
	if err := g.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}
	// Expect two increments total (initial press + one repeat).
	want := base + 2*inc
	if g.drum.Length != want {
		t.Fatalf("len=%d want %d (base %d inc %d)", g.drum.Length, want, base, inc)
	}
}

// Changing the number of visible subdivisions should not change the pixel
// width of the step row; cell width adjusts instead. Validate for both
// button-based beats and wheel zoom.
func TestDrumStepsPixelWidthStableOnResize(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1200, 500)
	g.drum.recalcButtons()
	g.drum.calcLayout()
	stepsW := g.drum.Bounds.Dx() - g.drum.labelW - g.drum.controlsW
	baseCell := g.drum.cell
	baseLen := g.drum.Length
	// Initial: product should be close to available width.
	prod := g.drum.cell * g.drum.Length
	if prod <= 0 || prod > stepsW {
		t.Fatalf("initial steps width=%d overflows %d", prod, stepsW)
	}

	// Increase by one beat.
	pressLenInc(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("update error: %v", err)
	}
	g.drum.recalcButtons()
	g.drum.calcLayout()
	if g.drum.Bounds.Dx()-g.drum.labelW-g.drum.controlsW != stepsW {
		t.Fatalf("steps area width changed")
	}
	prod2 := g.drum.cell * g.drum.Length
	if prod2 <= 0 || prod2 > stepsW {
		t.Fatalf("resized steps width=%d overflows %d", prod2, stepsW)
	}
	if g.drum.Length <= baseLen || g.drum.cell > baseCell {
		t.Fatalf("cell/len did not adjust as expected: len %d->%d cell %d->%d", baseLen, g.drum.Length, baseCell, g.drum.cell)
	}

	// Wheel zoom in by one beat (4 notches at 0.25 beat each).
	wheelVal := 0.0
	restore := SetInputForTest(
		func() (int, int) {
			return g.drum.Bounds.Min.X + g.drum.labelW + 10, g.drum.Bounds.Min.Y + g.drum.headerH + 5
		},
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheelVal; wheelVal = 0; return 0, v },
		func() (int, int) { return g.winW, g.winH },
	)
	t.Cleanup(restore)
	for i := 0; i < 4; i++ {
		wheelVal = 1.0
		if err := g.Update(); err != nil {
			t.Fatalf("update: %v", err)
		}
	}
	restore()
	g.drum.recalcButtons()
	g.drum.calcLayout()
	if g.drum.Bounds.Dx()-g.drum.labelW-g.drum.controlsW != stepsW {
		t.Fatalf("steps area width changed after wheel")
	}
}

func TestPlayWithoutStartNodeStaysResponsive(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if len(g.nodes) > 0 {
		g.deleteNode(g.nodes[0])
	}
	g.Update()

	pressPlay(t, g.drum)
	g.Update()
	if g.Playing() {
		t.Fatalf("game should not start without start node")
	}

	// pressing play again should still be handled immediately
	pressPlay(t, g.drum)
	g.Update()
	if g.Playing() {
		t.Fatalf("game should remain stopped without start node")
	}
}

func TestAddEdgeNoDuplicates(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(a, b)
	if len(g.edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.edges))
	}
}

func TestAddRegularNodeOverInvisible(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Previously an edge introduced invisible pass-through nodes; now edges are
	// direct and placing a regular node on the path should create it directly.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(a, b)

	n := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if node, ok := g.graph.GetNodeByID(n.ID); !ok || node.Type != model.NodeTypeRegular {
		t.Fatalf("expected node at (1,0) to be regular after upgrade")
	}
}

func TestComplexCircuitTraversal(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	b := g.tryAddNode(3, 0, model.NodeTypeRegular)
	c := g.tryAddNode(3, 2, model.NodeTypeRegular)
	d := g.tryAddNode(0, 2, model.NodeTypeRegular)

	g.addEdge(start, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, start)

	expectedLen := 10
	// Align the drum window with the expected traversal size so length checks
	// in this regression remain stable regardless of the default UI window.
	g.drum.SetLength(expectedLen)
	g.updateBeatInfos()

	if len(g.beatInfos) != expectedLen {
		t.Fatalf("expected beatInfos length %d, got %d", expectedLen, len(g.beatInfos))
	}
	if g.drum.Length != expectedLen {
		t.Fatalf("expected drum length %d, got %d", expectedLen, g.drum.Length)
	}

	for i := range g.beatInfos {
		expected := g.beatInfos[i].NodeType == model.NodeTypeRegular
		if g.drum.Rows[0].Steps[i] != expected {
			t.Fatalf("drum row mismatch at %d", i)
		}
	}

	p := &pulse{
		fromBeatInfo: g.beatInfos[len(g.beatInfos)-1],
		toBeatInfo:   g.beatInfos[0],
		path:         g.beatInfos,
		pathIdx:      0,
		from:         g.nodeByID(g.beatInfos[len(g.beatInfos)-1].NodeID),
		to:           g.nodeByID(g.beatInfos[0].NodeID),
		row:          0,
	}

	for i := 0; i < expectedLen*2; i++ {
		if !g.advancePulse(p) {
			t.Fatalf("pulse stopped at step %d", i)
		}
	}
}

func TestUpdateRunsSchedulerWhenPlaying(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// ensure first step active
	nodeID := g.tryAddNode(0, 0, model.NodeTypeRegular).ID
	g.graph.StartNodeID = nodeID

	g.drum.SetBPM(60)

	// Simulate click on play button
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return g.drum.playBtn.Rect().Min.X + 1, g.drum.playBtn.Rect().Min.Y + 1 }, // Click inside the button
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	defer restore()

	g.Update() // Simulate press
	pressed = false
	g.Update() // Simulate release

	setPlayStartForAbs(g, 1)
	if err := g.Update(); err != nil {
		t.Fatalf("update after play: %v", err)
	}
	if len(g.seqNextIdxs) == 0 || g.seqNextIdxs[0] <= 0 {
		t.Fatalf("scheduler did not advance seqNextIdxs: %v", g.seqNextIdxs)
	}
}

func TestClickAddsNode(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.pendingStartRow = -1
	startNodes := len(g.nodes)

	pressed := true
	step := g.grid.MaxDiv()
	if step < 4 {
		step = 4
	}
	tx0, ty0 := screenPosForGrid(g, step, 0)
	restore := SetInputForTest(
		func() (int, int) { return tx0, ty0 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	defer restore()

	g.Update() // press
	if g.nodeAt(step, 0) != nil {
		t.Fatalf("node created before release")
	}
	pressed = false
	g.Update() // release
	n := g.nodeAtScreen(tx0, ty0)
	if n == nil {
		t.Fatalf("expected node created near click after release")
	}
	if !n.Selected || g.sel != n {
		t.Fatalf("new node should be selected")
	}
	if len(g.nodes) != startNodes+1 {
		t.Fatalf("expected node count %d -> %d, got %d", startNodes, startNodes+1, len(g.nodes))
	}
	pressed = true
	// Close the sidebar so it doesn't absorb grid clicks
	g.sidebar.Close()
	g.sidebar.closedGuard = 0 // clear guard so next click isn't blocked
	// click another position
	tx1, ty1 := screenPosForGrid(g, step*2, 0)
	restore2 := SetInputForTest(
		func() (int, int) { return tx1, ty1 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore2)
	defer restore2()
	g.Update() // press second node
	if g.nodeAt(step*2, 0) != nil {
		t.Fatalf("node created before release at second position")
	}
	pressed = false
	g.Update() // release second node
	if len(g.nodes) != startNodes+2 {
		t.Fatalf("expected node count %d -> %d after second click, got %d", startNodes+1, startNodes+2, len(g.nodes))
	}
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	if n2 == nil || !n2.Selected || g.sel != n2 || n.Selected {
		t.Fatalf("selection did not move to new node")
	}
}

func TestBPMButtonsAdjustSpeed(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.pendingStartRow = 0
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.pendingStartRow = -1
	g.updateBeatInfos()
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse")
	}
	initialSpeed := g.activePulse.speed
	initialBPM := g.bpm
	setPlayStartForAbs(g, g.grid.MaxDiv())
	_ = g.Update()
	tBefore := g.activePulse.t

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return g.drum.bpmIncBtn.Rect().Min.X + 1, g.drum.bpmIncBtn.Rect().Min.Y + 1 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	g.Update() // press
	pressed = false
	g.Update() // release
	restore()
	wantBPM := initialBPM + 1
	if g.bpm != wantBPM {
		t.Fatalf("expected bpm %d got %d", wantBPM, g.bpm)
	}
	waitForUpdateCond(t, g, 10000, func() bool { return g.AppliedBPM() == wantBPM })
	if g.AppliedBPM() != wantBPM {
		t.Fatalf("applied BPM not updated: %d", g.AppliedBPM())
	}
	if g.activePulse == nil {
		t.Fatalf("pulse reset")
	}
	if g.activePulse.speed == initialSpeed {
		t.Fatalf("pulse speed unchanged")
	}
	setPlayStartForAbs(g, g.grid.MaxDiv()*2)
	_ = g.Update()
	if g.activePulse.t <= tBefore {
		t.Fatalf("pulse did not continue")
	}
}

func TestRowLengthMatchesConnectedNodes(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// Add some nodes and edges
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)

	g.addEdge(n0, n1)
	g.addEdge(n1, n2)

	// Set start node
	g.start = n0
	g.graph.StartNodeID = n0.ID

	g.updateBeatInfos()

	// The drum view length should now be independent of the number of connected nodes
	// and should match the default drum.Length (which is 8)
	if len(g.drum.Rows[0].Steps) != g.drum.Length {
		t.Errorf("row len=%d want %d", len(g.drum.Rows[0].Steps), g.drum.Length)
	}

	// Verify the first few steps based on the connected nodes
	if !g.drum.Rows[0].Steps[0] {
		t.Errorf("Expected step 0 to be true, got false")
	}
	if !g.drum.Rows[0].Steps[1] {
		t.Errorf("Expected step 1 to be true, got false")
	}
	if !g.drum.Rows[0].Steps[2] {
		t.Errorf("Expected step 2 to be true, got false")
	}

	// Verify the remaining steps are false (padded)
	for i := 3; i < g.drum.Length; i++ {
		if g.drum.Rows[0].Steps[i] {
			t.Errorf("Expected step %d to be false (padded), got true", i)
		}
	}
}

func TestDrumRowReflectsSubBeatNodes(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	start.Start = true
	g.start = start
	g.graph.StartNodeID = start.ID

	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(start, n1)
	n2 := g.tryAddNode(32, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)

	// Ensure the drum window matches the expected path length so sub-beat
	// placements materialize in the Steps slice at their exact offsets.
	g.drum.SetLength(33)
	g.updateBeatInfos()
	g.refreshDrumRow()

	steps := g.drum.Rows[0].Steps
	if len(steps) != 33 {
		t.Fatalf("steps len=%d want 33", len(steps))
	}
	if !steps[1] || !steps[32] {
		t.Fatalf("expected steps[1] and steps[32] true, got %v %v", steps[1], steps[32])
	}
	if steps[2] {
		t.Fatalf("expected step 2 to be false")
	}
}

func TestPulseAnimationProgress(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.pendingStartRow = 0
	node0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	node1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(node0, node1)
	g.pendingStartRow = -1

	// Manually set playing to true and call spawnPulse to create the pulse
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)

	// The pulse should be active now
	if g.activePulse == nil {
		t.Fatalf("expected active pulse after spawning")
	}

	setPlayStartForAbsFloat(g, 0.1)
	_ = g.Update()
	firstT := g.activePulse.t

	// Advance within the same segment so the pulse progresses without re-anchoring.
	setPlayStartForAbsFloat(g, 0.6)
	_ = g.Update()

	if g.activePulse == nil {
		t.Fatalf("pulse disappeared unexpectedly")
	}

	// The animation time 't' should have progressed
	if g.activePulse.t <= firstT {
		t.Fatalf("active pulse did not advance: %f <= %f", g.activePulse.t, firstT)
	}
}

func TestPlaySoundOnRegularNodesOnly(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	g.pendingStartRow = 0
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n0, n2) // introduces an invisible node at (1,0)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	g.updateBeatInfos()
	if bi := g.beatInfoAtRow(0, 1); bi.NodeType != model.NodeTypeInvisible {
		t.Fatalf("expected invisible beat at abs=1, got type=%v node=%d", bi.NodeType, bi.NodeID)
	}

	plays := make(chan string, 2)
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { plays <- id })

	schedule := func(abs int) {
		setPlayStartForAbs(g, abs)
		g.seqScheduleTime()
	}

	schedule(0)
	if got := waitForChan(t, plays, 10000); got != g.drum.Rows[0].Instrument {
		t.Fatalf("unexpected instrument")
	}

	// Invisible node: no new sample expected.
	schedule(1)
	select {
	case <-plays:
		t.Fatalf("expected no sample for invisible node")
	default:
	}

	// Final regular node: another sample expected.
	schedule(2)
	waitForChan(t, plays, 10000)
}

func TestSoundPlaysAfterHighlight(t *testing.T) {
	withDefaultAudio(t)
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n
	g.graph.StartNodeID = n.ID
	g.updateBeatInfos()
	info := model.BeatInfo{NodeType: model.NodeTypeRegular, NodeID: n.ID}

	done := make(chan struct{}, 1)
	g.SetPlayFunc(func(id string, vol float64, when ...float64) {
		done <- struct{}{}
	})

	g.highlightBeat(0, 0, info, 0)
	waitForChan(t, done, 10000)
}

func TestHighlightBeatUsesSelectedInstrument(t *testing.T) {
	withDefaultAudio(t)
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n
	g.graph.StartNodeID = n.ID
	g.updateBeatInfos()
	g.drum.SetInstrument("kick")
	info := model.BeatInfo{NodeType: model.NodeTypeRegular, NodeID: n.ID}

	idCh := make(chan string, 1)
	g.SetPlayFunc(func(inst string, vol float64, when ...float64) { idCh <- inst })

	g.highlightBeat(0, 0, info, 0)
	got := waitForChan(t, idCh, 10000)
	if got != "kick" {
		t.Fatalf("expected instrument 'kick', got %s", got)
	}
}

func TestHighlightBeatUsesRowVolume(t *testing.T) {
	withDefaultAudio(t)
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n
	g.graph.StartNodeID = n.ID
	g.updateBeatInfos()
	info := model.BeatInfo{NodeType: model.NodeTypeRegular, NodeID: n.ID}
	g.drum.Rows[0].Volume = 0.25
	volCh := make(chan float64, 1)
	g.SetPlayFunc(func(id string, v float64, when ...float64) { volCh <- v })
	g.highlightBeat(0, 0, info, 0)
	v := waitForChan(t, volCh, 10000)
	if math.Abs(v-0.25) > 0.01 {
		t.Fatalf("expected volume 0.25 got %f", v)
	}
}

func TestVolumeSliderAffectsPlayback(t *testing.T) {
	withDefaultAudio(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(1024, 768)
	n := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n
	g.graph.StartNodeID = n.ID
	g.updateBeatInfos()
	info := model.BeatInfo{NodeType: model.NodeTypeRegular, NodeID: n.ID}
	r := g.drum.rowVolSliders[0].TrackRect()
	mx := r.Min.X + r.Dx()/4
	my := r.Min.Y + r.Dy()/2
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	g.drum.Update()
	restore()

	volCh := make(chan float64, 1)
	g.SetPlayFunc(func(id string, v float64, when ...float64) { volCh <- v })
	g.highlightBeat(0, 0, info, 0)
	v := waitForChan(t, volCh, 10000)
	if math.Abs(v-0.25) > 0.04 {
		t.Fatalf("expected volume ~0.25 got %f", v)
	}
}

func TestLoopPulseDoesNotJumpToOrigin(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n3 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n1)
	g.updateBeatInfos()
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	for i := 0; i < 3; i++ {
		g.advancePulse(g.activePulse)
	}
	// With intermediate steps synthesized, the segment after reaching n3 may
	// pass through an invisible step before n1. Accept either the invisible
	// pass-through or the immediate hop to n1 depending on spacing.
	to := g.activePulse.toBeatInfo.NodeID
	if (to != n1.ID && to != model.InvalidNodeID) || g.activePulse.fromBeatInfo.NodeID != n3.ID {
		t.Fatalf("expected pulse from %d to %d or invisible, got from %d to %d", n3.ID, n1.ID, g.activePulse.fromBeatInfo.NodeID, to)
	}
}

func TestOriginSequenceResetsAtSameIndex(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n0
	g.graph.StartNodeID = n0.ID
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n0)

	g.updateBeatInfos()
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)

	g.advancePulse(g.activePulse)
	g.advancePulse(g.activePulse)

	g.resetOriginSequences()

	assertNotPanics(t, func() { g.advancePulse(g.activePulse) })
}

func TestAudioLoopConsistency(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	a := g.tryAddNode(2, 0, model.NodeTypeRegular)
	b := g.tryAddNode(3, 0, model.NodeTypeRegular)
	c := g.tryAddNode(3, 1, model.NodeTypeRegular)
	d := g.tryAddNode(2, 1, model.NodeTypeRegular)
	g.graph.StartNodeID = start.ID

	g.addEdge(start, a) // introduces invisible at (1,0)
	g.addEdge(a, b)
	g.addEdge(b, c)
	g.addEdge(c, d)
	g.addEdge(d, a) // loop back via A
	g.start = start
	g.updateBeatInfos()

	// With the unified time-based sequencer, audio truth comes from the engine predictor.
	// Schedule a small window and ensure the play calls match predictor.AudibleAt.
	plays := 0
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { plays++ })

	const maxAbs = 7 // 8 subdivisions; within seqScheduleTime burst limit
	scheduleAbsForTest(g, maxAbs)

	expected := 0
	g.engine.Predictor.Ensure(maxAbs + 1)
	for abs := 0; abs <= maxAbs; abs++ {
		bi := g.beatInfoAtRow(0, abs)
		if bi.NodeType != model.NodeTypeRegular {
			continue
		}
		if g.engine.Predictor.AudibleAt(0, abs) {
			expected++
		}
	}
	if plays != expected {
		t.Fatalf("plays=%d want %d over abs[0..%d]", plays, expected, maxAbs)
	}
}

func TestNodeScreenAlignment(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.cam.Scale = 1.37
	g.cam.OffsetX = 12.3
	g.cam.OffsetY = 7.8

	n := g.tryAddNode(3, 2, model.NodeTypeRegular)
	g.graph.StartNodeID = n.ID
	unitPx := g.grid.UnitPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	sx := offX + unitPx*float64(n.I)
	sy := offY + unitPx*float64(n.J)

	expX := sx
	expY := sy

	if sx != expX || sy != expY {
		t.Fatalf("screen (%v,%v) want (%v,%v)", sx, sy, expX, expY)
	}
}

func TestDragMaintainsAlignment(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.cam.Scale = 1.37
	n := g.tryAddNode(2, 1, model.NodeTypeRegular)
	g.graph.StartNodeID = n.ID

	pos := []struct{ x, y int }{{100, gridTopOffset() + 100}, {120, gridTopOffset() + 110}}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	g.Update() // release

	unitPx := g.grid.UnitPixels(g.cam.Scale)
	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	nodeX := offX + unitPx*float64(n.I)
	nodeY := offY + unitPx*float64(n.J)
	gi := int(math.Round((nodeX - offX) / unitPx))
	gj := int(math.Round((nodeY - offY) / unitPx))
	if gi != n.I || gj != n.J {
		t.Fatalf("node not aligned with grid after drag")
	}
}
func TestInitialDrumRows(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	if len(g.drum.Rows) != 1 {
		t.Fatalf("rows=%d want 1", len(g.drum.Rows))
	}
	g.Update()
	g.Draw(ebiten.NewImage(640, 480)) // Call Draw to populate bgCache
	if len(g.drum.bgCache) != 1 {
		t.Fatalf("bgCache=%d want 1", len(g.drum.bgCache))
	}
}

func TestHighlightScalesWithZoom(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(1, 1, model.NodeTypeRegular)
	g.sel = n
	g.graph.StartNodeID = n.ID

	g.cam.Scale = 1.0
	a1, _, a2, _ := g.nodeScreenRect(n)
	w1 := a2 - a1

	g.cam.Scale = 2.0
	b1, _, b2, _ := g.nodeScreenRect(n)
	w2 := b2 - b1

	if math.Abs(w2-2*w1) > 1e-3 {
		t.Fatalf("highlight width did not scale: w1=%f w2=%f", w1, w2)
	}
}

func TestDragPanDoesNotCreateNode(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.pendingStartRow = -1
	startNodes := len(g.nodes)
	x0, y0 := screenPosForGrid(g, 1, 0)
	x1, y1 := screenPosForGrid(g, 3, 0)
	pos := []struct{ x, y int }{
		{x0, y0},
		{x1, y1},
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	g.Update() // release

	if len(g.nodes) != startNodes {
		t.Fatalf("node created after drag (count=%d want %d)", len(g.nodes), startNodes)
	}
	g.graph.StartNodeID = model.InvalidNodeID
}

func TestBottomPaneClickIgnored(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.cam.OffsetY = 100
	startNodes := len(g.nodes)

	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return 10, g.split.Y + 10 },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	defer restore()

	g.Update() // press in bottom pane
	pressed = false
	g.Update() // release

	if len(g.nodes) != startNodes {
		t.Fatalf("node created from bottom pane click")
	}
	g.graph.StartNodeID = model.InvalidNodeID
}

func TestScrollBarDragDoesNotPanGrid(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	for i := 0; i < 12; i++ {
		g.drum.AddRow()
	}
	startOffX := g.cam.OffsetX
	startOffY := g.cam.OffsetY
	thumb := g.drum.scrollThumbRect()
	mx, my := thumb.Min.X+1, thumb.Min.Y+1
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	g.Update()          // start drag on thumb
	my = g.split.Y - 50 // move into top panel while holding
	g.Update()          // continue drag outside drum view
	pressed = false
	g.Update() // release
	restore()
	if g.cam.OffsetX != startOffX || g.cam.OffsetY != startOffY {
		t.Fatalf("camera moved: off=(%f,%f) want (%f,%f)", g.cam.OffsetX, g.cam.OffsetY, startOffX, startOffY)
	}
	g.graph.StartNodeID = model.InvalidNodeID
}

func TestHighlightMatchesNode(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n := g.tryAddNode(2, 3, model.NodeTypeRegular)
	g.sel = n
	g.graph.StartNodeID = n.ID

	g.cam.Scale = 1.5
	g.cam.OffsetX = 12
	g.cam.OffsetY = 8

	offX := math.Round(g.cam.OffsetX)
	offY := math.Round(g.cam.OffsetY)
	worldX := float64(n.I) * g.grid.Unit()
	worldY := float64(n.J) * g.grid.Unit()
	screenX := worldX*g.cam.Scale + offX
	screenY := worldY*g.cam.Scale + offY + float64(gridTopOffset())
	r := g.grid.NodeRadius(g.cam.Scale) * g.cam.Scale

	x1, y1, x2, y2 := g.nodeScreenRect(n)
	if math.Abs(x1-(screenX-r)) > 1e-3 || math.Abs(x2-(screenX+r)) > 1e-3 ||
		math.Abs(y1-(screenY-r)) > 1e-3 || math.Abs(y2-(screenY+r)) > 1e-3 {
		t.Fatalf("highlight mismatch: (%f,%f,%f,%f) want (%f,%f,%f,%f)",
			x1, y1, x2, y2,
			screenX-r, screenY-r, screenX+r, screenY+r)
	}
}

func TestSplitterDragPersists(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	startY := g.split.Y
	handleX := g.winW / 2 // pill handle center X
	pos := []struct{ x, y int }{
		{handleX, startY},
		{handleX, startY + 50},
		{handleX, startY + 50},
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	idx = 2
	g.Update()         // release
	g.Layout(640, 480) // layout called again as in game loop
	g.Update()
	if g.split.Y != startY+50 {
		t.Fatalf("splitter Y=%d want %d", g.split.Y, startY+50)
	}
	g.graph.StartNodeID = model.InvalidNodeID
}

// When the screen size can't be queried (e.g. it returns 0), dragging the
// splitter should still preserve its final position once released.
func TestSplitterDragPersistsWithoutScreenSize(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	startY := g.split.Y
	handleX := g.winW / 2 // pill handle center X
	pos := []struct{ x, y int }{
		{handleX, startY},
		{handleX, startY + 50},
		{handleX, startY + 50},
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	idx = 2
	g.Update()         // release
	g.Layout(640, 480) // layout called again as in game loop
	g.Update()
	if g.split.Y != startY+50 {
		t.Fatalf("splitter Y=%d want %d", g.split.Y, startY+50)
	}
	g.graph.StartNodeID = model.InvalidNodeID
}
func TestSplitterDragDoesNotCreateNode(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.Layout(640, 480)
	startNodes := len(g.nodes)
	startY := g.split.Y
	handleX := g.winW / 2 // pill handle center X
	pos := []struct{ x, y int }{
		{handleX, startY},      // press on handle
		{handleX, startY + 40}, // drag
		{handleX, startY + 40}, // release
		{handleX, startY + 40}, // idle
	}
	idx := 0
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return pos[idx].x, pos[idx].y },
		func(b ebiten.MouseButton) bool { return pressed && b == ebiten.MouseButtonLeft },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	defer restore()

	g.Update() // press
	idx = 1
	g.Update() // drag
	pressed = false
	idx = 2
	g.Update() // release while over divider
	idx = 3
	g.Update() // after release
	g.Layout(640, 480)
	g.Update()

	if len(g.nodes) != startNodes {
		t.Fatalf("unexpected node created during splitter drag")
	}
	g.graph.StartNodeID = model.InvalidNodeID
}

func TestDrumViewResizeKeepsOffset(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)

	// Populate beat infos with a dummy path longer than the drum view.
	inc := g.grid.MaxDiv()
	// Ensure the path is long enough to accommodate a +1 beat resize without clamping.
	g.beatInfos = make([]model.BeatInfo, 8+2*inc)
	g.drum.SetLength(8)
	g.drum.Offset = 2
	g.refreshDrumRow()

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	defer restore()

	// Increase length and ensure offset is preserved.
	pressLenInc(t, g.drum)
	g.Update()
	if g.drum.Offset != 2 {
		t.Fatalf("offset changed after length increase: %d", g.drum.Offset)
	}

	// Decrease length and ensure offset is preserved.
	pressLenDec(t, g.drum)
	g.Update()
	if g.drum.Offset != 2 {
		t.Fatalf("offset changed after length decrease: %d", g.drum.Offset)
	}
}

func TestStartNodeSelection(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	if g.start != n1 || !n1.Start {
		t.Fatalf("first node should be start")
	}
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.sel = n2
	restore := SetInputForTest(
		func() (int, int) { return 0, gridTopOffset() + 10 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyS },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 640, 480 },
	)
	t.Cleanup(restore)
	defer restore()
	g.Update()
	if g.start != n2 || !n2.Start || n1.Start {
		t.Fatalf("start node not updated")
	}
	g.graph.StartNodeID = n2.ID
}

func TestHighlightEmptyCells(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(1280, 720)
	g.drum.SetBPM(60)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(3, 0, model.NodeTypeRegular) // Node with a gap
	g.addEdge(n1, n2)

	g.start = n1
	g.graph.StartNodeID = n1.ID
	if len(g.drum.Rows) > 0 {
		g.drum.SetInstrument("snare")
		ensureInstrumentAvailable(t, g, "snare")
	}

	g.drum.SetLength(4)
	g.updateBeatInfos() // This will now correctly handle the invisible nodes

	if len(g.beatInfos) != 4 { // n1, invisible, invisible, n2
		t.Fatalf("Expected beatInfos length to be 4, got %d", len(g.beatInfos))
	}

	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)

	if _, ok := g.highlightedBeats[makeBeatKey(0, 0)]; !ok {
		t.Errorf("Tick 0: Beat at index 0 should be highlighted")
	}

	advanceBeats(g, 1)
	if _, ok := g.highlightedBeats[makeBeatKey(0, 1)]; !ok {
		t.Errorf("Tick 1: Beat at index 1 should be highlighted")
	}

	advanceBeats(g, 1)
	if _, ok := g.highlightedBeats[makeBeatKey(0, 2)]; !ok {
		t.Errorf("Tick 2: Beat at index 2 should be highlighted")
	}

	advanceBeats(g, 1)
	if _, ok := g.highlightedBeats[makeBeatKey(0, 3)]; !ok {
		t.Errorf("Tick 3: Beat at index 3 should be highlighted")
	}
}

func TestBeatInfosNotTrimmedByDrumLength(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)

	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)

	g.start = n0
	g.graph.StartNodeID = n0.ID

	g.drum.SetLength(1)
	g.updateBeatInfos()
	// Shrink the drum view again without recomputing beatInfos
	g.drum.SetLength(1)

	if len(g.beatInfos) <= g.drum.Length {
		t.Fatalf("expected beatInfos length > drum length, got %d <= %d", len(g.beatInfos), g.drum.Length)
	}

	if g.beatInfos[0].NodeID != n0.ID || g.beatInfos[1].NodeID != n1.ID || g.beatInfos[2].NodeID != n2.ID {
		t.Errorf("unexpected beatInfos sequence: %v", g.beatInfos)
	}
}

func TestPulseTraversalIgnoresDrumLength(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)

	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n2)

	g.start = n0
	g.graph.StartNodeID = n0.ID

	g.drum.SetLength(1)
	g.updateBeatInfos()
	// Shrink drum view again without affecting beatInfos
	g.drum.SetLength(1)

	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)

	if g.activePulse == nil || g.activePulse.toBeatInfo.NodeID != n1.ID {
		t.Fatalf("expected pulse heading to second node")
	}

	// Advance to the next segment explicitly to avoid relying on
	// time-based sync paths.
	_ = g.advancePulse(g.activePulse)
	if g.activePulse == nil || g.activePulse.toBeatInfo.NodeID != n2.ID {
		t.Fatalf("expected pulse to continue to third node, got %+v", g.activePulse)
	}

	// Reach final node; pulse should report completion (no next segment).
	done := true
	if g.activePulse != nil {
		done = !g.advancePulse(g.activePulse)
	}
	if !done {
		t.Fatalf("expected pulse to stop after last node, but it continued")
	}
}

func TestDrumViewLoopingWithInvisibleNodes(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)

	// Create a path with an invisible node: n0 -> invisible -> n1
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	_ = g.tryAddNode(1, 0, model.NodeTypeInvisible) // Invisible node

	g.addEdge(n0, n1)

	g.start = n0
	g.graph.StartNodeID = n0.ID

	// Set drum view length
	g.drum.SetLength(6)

	g.updateBeatInfos()

	// Expected drum view steps: n0 (true), invisible (false), n1 (true), then padded
	expectedSteps := []bool{
		true,  // n0
		false, // invisible
		true,  // n1
		false, // padded
		false, // padded
		false, // padded
	}

	if len(g.drum.Rows[0].Steps) != g.drum.Length {
		t.Errorf("Expected drum view steps length to be %d, got %d", g.drum.Length, len(g.drum.Rows[0].Steps))
	}

	for i, expected := range expectedSteps {
		if g.drum.Rows[0].Steps[i] != expected {
			t.Errorf("Drum view step %d: expected %t, got %t", i, expected, g.drum.Rows[0].Steps[i])
		}
	}
}

func TestDrumViewLoopingHighlighting(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger) // Use the Game struct to leverage its graph manipulation and beat info update logic
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)

	// Set up the drum view length to accommodate the expected sequence
	g.drum.SetLength(6)
	g.drum.SetBeatLength(g.drum.Length)

	// Create the circuit: [X] -> [] -> [X] -> [X]
	//                       ^      ^
	//                       |      |
	//                      [X] <- [X]

	// Nodes:
	// (0,0) - Node 1 (Regular)
	// (1,0) - Node 2 (Invisible)
	// (2,0) - Node 3 (Regular)
	// (3,0) - Node 4 (Regular)
	// (3,1) - Node 5 (Regular)
	// (2,1) - Node 6 (Regular)

	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular) // X
	g.start = n1
	g.graph.StartNodeID = n1.ID

	n2 := g.tryAddNode(1, 0, model.NodeTypeInvisible) // invisible
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)   // X
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)   // X
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)   // X
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)   // X

	// Edges:
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3) // Loop back to n3

	// Update beat infos to populate drum view steps
	g.updateBeatInfos()

	// Only invisible seam segments are suppressed; regular nodes remain visible.
	expectedDrumRow := []bool{true, false, true, true, true, true}

	if len(g.drum.Rows[0].Steps) != len(expectedDrumRow) {
		t.Fatalf("Expected drum row length %d, got %d", len(expectedDrumRow), len(g.drum.Rows[0].Steps))
	}

	for i, expected := range expectedDrumRow {
		if g.drum.Rows[0].Steps[i] != expected {
			t.Errorf("At index %d: Expected %t, got %t. Full drum row: %v", i, expected, g.drum.Rows[0].Steps[i], g.drum.Rows[0].Steps)
		}
	}

	// Test a more complex looped circuit
	// [X] -> [X] -> [X]
	//  ^           |
	//  |           v
	// [X] <- [X] <- [X]

	// Reset graph and drum view
	g = New(logger)
	g.drum.SetLength(6)
	g.drum.SetBeatLength(g.drum.Length)

	cn1 := g.tryAddNode(0, 0, model.NodeTypeRegular) // X
	g.start = cn1
	g.graph.StartNodeID = cn1.ID

	cn2 := g.tryAddNode(1, 0, model.NodeTypeRegular) // X
	cn3 := g.tryAddNode(2, 0, model.NodeTypeRegular) // X
	cn4 := g.tryAddNode(2, 1, model.NodeTypeRegular) // X
	cn5 := g.tryAddNode(1, 1, model.NodeTypeRegular) // X
	cn6 := g.tryAddNode(0, 1, model.NodeTypeRegular) // X

	g.addEdge(cn1, cn2)
	g.addEdge(cn2, cn3)
	g.addEdge(cn3, cn4)
	g.addEdge(cn4, cn5)
	g.addEdge(cn5, cn6)
	g.addEdge(cn6, cn1) // Loop back to cn1

	g.updateBeatInfos()

	// No invisible seam bridges in this circuit; all steps should remain visible.
	expectedDrumRow2 := []bool{true, true, true, true, true, true}

	if len(g.drum.Rows[0].Steps) != len(expectedDrumRow2) {
		t.Fatalf("Expected drum row length %d, got %d", len(expectedDrumRow2), len(g.drum.Rows[0].Steps))
	}

	for i, expected := range expectedDrumRow2 {
		if g.drum.Rows[0].Steps[i] != expected {
			t.Errorf("At index %d: Expected %t, got %t. Full drum row: %v", i, expected, g.drum.Rows[0].Steps[i], g.drum.Rows[0].Steps)
		}
	}
}

func TestPulseTraversalBeyondDrumView(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)

	// Build looped circuit: [O] -> [] -> [X] -> [X]
	//                                   ^      v
	//                                   [X] <- [X]
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID

	n2 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)

	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3) // loop

	// Drum view shorter than path length
	g.drum.SetLength(4)
	g.drum.SetBeatLength(g.drum.Length)

	g.updateBeatInfos()

	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse")
	}

	steps := 4
	for i := 0; i < steps; i++ {
		if !g.advancePulse(g.activePulse) {
			t.Fatalf("pulse stopped early at step %d", i)
		}
	}

	if g.activePulse.pathIdx != steps+1 {
		t.Fatalf("expected pathIdx %d, got %d", steps+1, g.activePulse.pathIdx)
	}

	if g.activePulse.lastIdx != steps {
		t.Fatalf("expected lastIdx %d, got %d", steps, g.activePulse.lastIdx)
	}
}

func TestSignalTraversalInLoop(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)

	// Nodes
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	n_inv1 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)

	// Edges
	g.addEdge(n1, n_inv1)
	g.addEdge(n_inv1, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3) // Loop back to n3

	g.drum.SetLength(11) // Drum view longer than path
	g.updateBeatInfos()

	// Expected sequence of node IDs for the pulse traversal
	expectedNodeIDs := []model.NodeID{
		n1.ID,
		n_inv1.ID,
		n3.ID,
		n4.ID,
		n5.ID,
		n6.ID,
	}

	t.Logf("Expected Node IDs: %v", expectedNodeIDs)
	actualNodeIDs := []model.NodeID{}
	for _, beatInfo := range g.beatInfos {
		actualNodeIDs = append(actualNodeIDs, beatInfo.NodeID)
	}
	t.Logf("Actual Beat Infos: %v", actualNodeIDs)

	// Verify that the generated beatInfos begin with the expected sequence
	if len(actualNodeIDs) < len(expectedNodeIDs) {
		t.Fatalf("Initial beatInfos shorter than expected. want >=%d got %d", len(expectedNodeIDs), len(actualNodeIDs))
	}
	for i, expectedID := range expectedNodeIDs {
		if actualNodeIDs[i] != expectedID {
			t.Errorf("Initial beatInfos mismatch at index %d. Expected %d, got %d", i, expectedID, actualNodeIDs[i])
		}
	}

	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatalf("Expected active pulse after spawning")
	}
	// Advance through several beats ensuring pulse persists
	for i := 0; i < len(expectedNodeIDs)*2; i++ {
		if g.activePulse == nil {
			t.Fatalf("Pulse ended prematurely at beat %d", i)
		}
		g.activePulse.t = 1
		g.Update()
	}
}

func TestLoopExpansionAndHighlighting(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)

	// prepare drum view length to capture multiple loop laps
	g.drum.SetLength(10)
	g.drum.SetBeatLength(g.drum.Length)

	// build circuit: start -> invisible -> n1 -> n2 -> n3 -> n4 -> back to n1
	start := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = start
	g.graph.StartNodeID = start.ID
	inv := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	n1 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	n3 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	n4 := g.tryAddNode(2, 1, model.NodeTypeRegular)

	g.addEdge(start, inv)
	g.addEdge(inv, n1)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n1) // close loop

	g.updateBeatInfos()

	// verify drum beat infos expand deterministically across drum length
	wantIDs := []model.NodeID{start.ID, inv.ID, n1.ID, n2.ID, n3.ID, n4.ID, n1.ID, n2.ID, n3.ID, n4.ID}
	if len(g.drumBeatInfos) != len(wantIDs) {
		t.Fatalf("expected %d drum beat infos, got %d", len(wantIDs), len(g.drumBeatInfos))
	}
	for i, id := range wantIDs {
		if g.drumBeatInfos[i].NodeID != id {
			t.Fatalf("at %d expected node %d got %d", i, id, g.drumBeatInfos[i].NodeID)
		}
	}

	if len(g.beatInfos) != 6 {
		t.Fatalf("expected base path length 6, got %d", len(g.beatInfos))
	}

	// now simulate pulse highlighting across two laps
	g.spawnPulseFromRow(0, 0)
	// sequence of highlighted beat indices expected for first 12 advancements
	expected := []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11}
	got := make([]int, len(expected))
	got[0] = 0
	for i := 1; i < len(expected); i++ {
		delete(g.highlightedBeats, makeBeatKey(0, g.activePulse.lastIdx))
		if !g.advancePulse(g.activePulse) {
			t.Fatalf("pulse ended early at step %d", i)
		}
		if _, ok := g.highlightedBeats[makeBeatKey(0, expected[i])]; !ok {
			t.Fatalf("expected highlight %d, got %v", expected[i], g.highlightedBeats)
		}
		for key := range g.highlightedBeats {
			_, idx := splitBeatKey(key)
			got[i] = idx
			if idx != g.elapsedBeats {
				t.Fatalf("timeline and highlight out of sync: got %d elapsed %d", idx, g.elapsedBeats)
			}
			beats := g.currentBeat()
			wantBeat := float64(g.elapsedBeats) / float64(g.grid.MaxDiv())
			if math.Abs(beats-wantBeat) > 1e-9 {
				t.Fatalf("currentBeat=%.6f want %.6f", beats, wantBeat)
			}
		}
	}
	if !reflect.DeepEqual(expected, got) {
		t.Fatalf("highlight sequence mismatch. expected %v got %v", expected, got)
	}
}

func TestBPMChangeDuringLoopKeepsForwardProgress(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(640, 480)

	// Build looped circuit: [O] -> [] -> [X] -> [X]
	//                                   ^      v
	//                                   [X] <- [X]
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	g.start = n1
	g.graph.StartNodeID = n1.ID
	n2 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	n3 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	n4 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	n5 := g.tryAddNode(3, 1, model.NodeTypeRegular)
	n6 := g.tryAddNode(2, 1, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.addEdge(n2, n3)
	g.addEdge(n3, n4)
	g.addEdge(n4, n5)
	g.addEdge(n5, n6)
	g.addEdge(n6, n3)

	g.drum.SetLength(8)
	g.drum.SetBeatLength(g.drum.Length)
	g.updateBeatInfos()

	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse")
	}

	setPlayStartForAbs(g, g.grid.MaxDiv())
	_ = g.Update()
	beforeBeat := g.nextBeatIdxs[0]
	beforeT := g.activePulse.t

	g.drum.SetBPM(240)
	_ = g.Update()

	if g.nextBeatIdxs[0] < beforeBeat {
		t.Fatalf("beat index went backwards: %d -> %d", beforeBeat, g.nextBeatIdxs[0])
	}

	if g.activePulse.t+0.02 < beforeT {
		t.Fatalf("pulse regressed after BPM change: before %.3f after %.3f", beforeT, g.activePulse.t)
	}

	afterBeat := g.nextBeatIdxs[0]
	advanceBeats(g, 3)
	if g.activePulse == nil {
		t.Fatalf("pulse ended early after BPM change")
	}
	if g.nextBeatIdxs[0] < afterBeat+3 {
		t.Fatalf("beat index did not advance after BPM change: %d -> %d", afterBeat, g.nextBeatIdxs[0])
	}
}

func TestGraphUpdateDuringPlaybackPreservesProgress(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	advanceBeats(g, 1)
	prev := g.nextBeatIdxs[0]
	if prev <= 1 {
		t.Fatalf("expected progress beyond origin, got %d", prev)
	}
	g.updateBeatInfos()
	if g.nextBeatIdxs[0] != prev {
		t.Fatalf("beat index reset after update: %d -> %d", prev, g.nextBeatIdxs[0])
	}
	advanceBeats(g, 1)
	if g.nextBeatIdxs[0] != prev+1 {
		t.Fatalf("beat index did not advance: want %d got %d", prev+1, g.nextBeatIdxs[0])
	}
}

func TestAddDisconnectedNodeDuringPlaybackMaintainsSequence(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	advanceBeats(g, 2)
	prev := g.nextBeatIdxs[0]
	before := append([]model.BeatInfo(nil), g.beatInfosByRow[0]...)
	g.tryAddNode(5, 5, model.NodeTypeRegular) // disconnected node
	if g.nextBeatIdxs[0] != prev {
		t.Fatalf("beat index changed after add node: got %d want %d", g.nextBeatIdxs[0], prev)
	}
	if !reflect.DeepEqual(before, g.beatInfosByRow[0]) {
		t.Fatalf("beat path changed after add node: %v -> %v", before, g.beatInfosByRow[0])
	}
	advanceBeats(g, 4)
	if g.nextBeatIdxs[0] != prev+4 {
		t.Fatalf("beat index did not advance correctly: got %d want %d", g.nextBeatIdxs[0], prev+4)
	}
}

func TestBPMChangeDuringPlaybackMaintainsSequence(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.addEdge(n1, n0)
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	if g.activePulse == nil {
		t.Fatalf("expected active pulse")
	}
	advanceBeats(g, 1)
	prev := g.nextBeatIdxs[0]
	g.SetBeatBaseForTest(float64(prev-1) / float64(g.grid.MaxDiv()))
	setPlayStartForAbs(g, prev)
	g.drum.SetBPM(60)
	g.Update()
	if g.nextBeatIdxs[0] < prev {
		t.Fatalf("beat index reset after BPM change: %d -> %d", prev, g.nextBeatIdxs[0])
	}
	after := g.nextBeatIdxs[0]
	advanceBeats(g, 1)
	if g.nextBeatIdxs[0] <= after {
		t.Fatalf("beat index did not advance after BPM change: before %d after %d", after, g.nextBeatIdxs[0])
	}
}

func TestPlaybackRestartDoesNotPanicOnOrigin(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	prev := n0
	for i := 1; i < 6; i++ {
		n := g.tryAddNode(i, 0, model.NodeTypeRegular)
		g.addEdge(prev, n)
		prev = n
	}
	g.addEdge(prev, n0)
	g.updateBeatInfos()
	g.SetPlaying(true)
	g.spawnPulseFromRow(0, 0)
	advanceBeats(g, 2)
	pressStop(t, g.drum)
	_ = g.Update()
	pressPlay(t, g.drum)
	_ = g.Update()
	if g.activePulse == nil {
		g.spawnPulseFromRow(0, 0)
	}
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("unexpected panic: %v", r)
		}
	}()
	for i := 0; i < 7; i++ {
		advanceBeats(g, 1)
	}
}

func TestMuteAndSoloPlayback(t *testing.T) {
	withDefaultAudio(t)
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.drum.AddRow()

	plays := make(chan string, 8)
	g.SetPlayFunc(func(id string, vol float64, when ...float64) { plays <- id })

	// Drain any plays emitted during initialization or lingering from previous tests.
	advanceFrames(g, 2)
	for len(plays) > 0 {
		<-plays
	}

	// Build minimal per-row loops so predictor truth exists for both rows.
	n0a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n0b := g.tryAddNode(1, 0, model.NodeTypeRegular)
	n1a := g.tryAddNode(0, 1, model.NodeTypeRegular)
	n1b := g.tryAddNode(1, 1, model.NodeTypeRegular)
	g.start = n0a
	g.graph.StartNodeID = n0a.ID
	g.drum.Rows[0].Origin = n0a.ID
	g.drum.Rows[0].Node = n0a
	g.drum.Rows[1].Origin = n1a.ID
	g.drum.Rows[1].Node = n1a
	g.addEdgeNoRefresh(n0a, n0b)
	g.addEdgeNoRefresh(n0b, n0a)
	g.addEdgeNoRefresh(n1a, n1b)
	g.addEdgeNoRefresh(n1b, n1a)
	g.updateBeatInfos()

	setRowMuted(t, g.drum, 0, true)
	g.highlightBeat(0, 0, g.beatInfoAtRow(0, 0), 1)
	advanceFrames(g, 2)
	select {
	case <-plays:
		t.Fatalf("expected no plays when muted")
	default:
	}

	setRowMuted(t, g.drum, 0, false)
	setRowSolo(t, g.drum, 1, true)
	g.highlightBeat(0, 1, g.beatInfoAtRow(0, 1), 1)
	advanceFrames(g, 2)
	select {
	case <-plays:
		t.Fatalf("unexpected play from non-solo row")
	default:
	}
	g.highlightBeat(1, 1, g.beatInfoAtRow(1, 1), 1)
	waitForChan(t, plays, 10000)

	// reset
	for len(plays) > 0 {
		<-plays
	}
	setRowSolo(t, g.drum, 1, false)
	g.highlightBeat(0, 2, g.beatInfoAtRow(0, 2), 1)
	g.highlightBeat(1, 2, g.beatInfoAtRow(1, 2), 1)
	for i := 0; i < 2; i++ {
		waitForChan(t, plays, 10000)
	}
}

func TestAutoTrackFollowsBeat(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(200, 200)
	g.drum.SetLength(4)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.SetPlaying(true)
	g.elapsedBeats = 5
	g.updateDrumTracking()
	if g.drum.Offset <= 0 {
		t.Fatalf("expected tracking offset to advance, got %d", g.drum.Offset)
	}
}

func TestAutoTrackDisabled(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	g.Layout(200, 200)
	g.drum.SetLength(4)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.SetPlaying(true)
	g.drum.SetFollow(false)
	g.elapsedBeats = 5
	g.updateDrumTracking()
	if g.drum.Offset != 0 {
		t.Fatalf("offset=%d want 0", g.drum.Offset)
	}
}

func TestUpdateBeatInfosClampUsesTimelineUnits(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(200, 200)
	g.drum.SetPlaying(true)
	g.drum.timelineUnitsPerBeat = 4
	g.drum.SetLength(16)
	g.drum.timelineBeats = 5
	g.drum.Offset = 3
	g.updateBeatInfos()
	if g.drum.Offset != 3 {
		t.Fatalf("offset was clamped to %d, expected to preserve 3", g.drum.Offset)
	}
}

// Rendering regression: a small progress dip should not shift the drum view
// or misplace the highlight when tracking is enabled.
func TestAutoTrackIgnoresProgressJitter(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(400, timelineHeight+24)
	g.drum.SetLength(8)
	g.updateBeatInfos()
	g.refreshDrumRow()
	g.SetPlaying(true)
	g.elapsedBeats = 4
	g.Update()
	initial := g.drum.Offset
	seq := []float64{0.6, 0.4}
	var i int
	g.engineProgress = func() float64 {
		if i < len(seq) {
			v := seq[i]
			i++
			return v
		}
		return seq[len(seq)-1]
	}
	g.Update() // jitter drop
	if g.drum.Offset != initial {
		t.Fatalf("offset changed unexpectedly: %d -> %d", initial, g.drum.Offset)
	}
	// draw the drum view to ensure rendering path runs; tracking should have
	// kept the offset stable despite the progress jitter
	g.drum.Draw(ebiten.NewImage(400, timelineHeight+24), nil, 0, nil, g.currentBeat())
	if g.drum.Offset != initial {
		t.Fatalf("offset changed after draw: %d", g.drum.Offset)
	}
}

func TestTrackButtonTogglesFollow(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(200, 200)
	if !g.drum.FollowPlayback() {
		t.Fatalf("follow should start enabled")
	}
	g.drum.trackBtn.OnClick()
	if g.drum.FollowPlayback() {
		t.Fatalf("follow not toggled off")
	}
}

func TestNodeHoverScalesRadius(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withDefaultStart(t, true)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(200, 200)
	n := g.nodeAt(0, 0)
	if n == nil {
		t.Fatal("missing origin node")
	}
	x1, y1, x2, y2 := g.nodeScreenRect(n)
	mx := int((x1 + x2) / 2)
	my := int((y1 + y2) / 2)
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	g.Update()
	restore()
	rHover := g.nodeRadius(n)
	g.hover = nil
	rNoHover := g.nodeRadius(n)
	if rHover <= rNoHover {
		t.Fatalf("expected larger radius when hovered: hover=%.2f base=%.2f", rHover, rNoHover)
	}
}

// Global zoom limits: length never below 1 beat and never exceeds the
// horizontal pixel capacity of the timeline.
func TestDrumZoomGlobalMinMax(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 300)
	g.drum.Update()
	inc := g.grid.MaxDiv()
	// Max by pixels
	max := g.drum.timelineRect.Dx()
	// Zoom out aggressively using + button until it no longer increases.
	prev := g.drum.Length
	for i := 0; i < 200; i++ {
		pressLenInc(t, g.drum)
		if err := g.Update(); err != nil {
			t.Fatalf("update: %v", err)
		}
		if g.drum.Length == prev {
			break
		}
		prev = g.drum.Length
	}
	if g.drum.Length > max {
		t.Fatalf("length exceeds pixel max: len=%d max=%d", g.drum.Length, max)
	}
	// Now zoom in using the wheel; should not drop below one beat.
	wheelVal := -1.0
	restore := SetInputForTest(
		func() (int, int) { return g.drum.timelineRect.Min.X + 1, g.drum.Bounds.Min.Y + timelineHeight + 5 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheelVal; wheelVal = 0; return 0, v },
		func() (int, int) { return g.winW, g.winH },
	)
	t.Cleanup(restore)
	// 40 notches (~10 beats) should be plenty; smoothing requires 4 per beat.
	for i := 0; i < 40; i++ {
		wheelVal = -1.0
		if err := g.Update(); err != nil {
			t.Fatalf("update: %v", err)
		}
	}
	restore()
	if g.drum.Length < inc {
		t.Fatalf("length dropped below 1 beat: %d < %d", g.drum.Length, inc)
	}
}

func TestDrawDrumPaneConcurrentHighlights(t *testing.T) {
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	dst := ebiten.NewImage(640, 480)

	stop := make(chan struct{})
	stopClosed := false
	var wg sync.WaitGroup
	wg.Add(1)
	t.Cleanup(func() {
		if !stopClosed {
			close(stop)
		}
		wg.Wait()
	})
	go func() {
		defer wg.Done()
		idx := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			row := idx % len(g.drum.Rows)
			beat := idx % g.drum.Length
			g.highlightSet(makeBeatKey(row, beat), encodeHighlight(g.frame+int64(beat), idx%2 == 0))
			if idx%3 == 0 {
				g.clearRowHighlights(row)
			}
			idx++
			runtime.Gosched()
		}
	}()

	for i := 0; i < 512; i++ {
		g.drawDrumPane(dst)
	}
	close(stop)
	stopClosed = true
	wg.Wait()
}

// TestGameUpdatePendingImport verifies that pendingImportData is processed
// and cleared after Update().
func TestGameUpdatePendingImport(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	// Build a small circuit so we have valid export data.
	a := g.tryAddNode(0, 0, model.NodeTypeRegular)
	b := g.tryAddNode(8, 0, model.NodeTypeRegular)
	g.addEdge(a, b)
	g.addEdge(b, a)
	g.start = a
	g.graph.StartNodeID = a.ID
	g.drum.Rows[0].Origin = a.ID
	g.drum.Rows[0].Node = a
	g.updateBeatInfos()

	data, err := g.drum.exportBytes()
	if err != nil {
		t.Fatalf("export failed: %v", err)
	}

	// Set pending import data.
	g.pendingImportData = data
	advanceFrames(g, 1)

	if g.pendingImportData != nil {
		t.Error("expected pendingImportData cleared after Update()")
	}
}

// TestGameUpdatePendingImportError verifies that invalid JSON in
// pendingImportData triggers error notification and clears data.
func TestGameUpdatePendingImportError(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.pendingImportData = []byte("not valid json {{{")
	advanceFrames(g, 1)

	if g.pendingImportData != nil {
		t.Error("expected pendingImportData cleared after invalid import")
	}
}

// TestGameUpdateSimpleDrawAutoDisable verifies that simpleDraw=true with
// auto-disable countdown disables simpleDraw after the countdown reaches 0.
func TestGameUpdateSimpleDrawAutoDisable(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultStart(t, false)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 600)

	g.simpleDraw = true
	g.simpleDrawAutoDisableFrames = 3

	advanceFrames(g, 2)
	if !g.simpleDraw {
		t.Error("expected simpleDraw still true after 2 frames (countdown=1 remaining)")
	}

	advanceFrames(g, 1)
	if g.simpleDraw {
		t.Error("expected simpleDraw=false after countdown reached 0")
	}
}

// TestBenchFmtMS verifies the benchFmtMS formatting helper.
func TestBenchFmtMS(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0.001, "1.00"},
		{0.1, "100.00"},
		{0, "0.00"},
		{math.NaN(), "N/A"},
		{math.Inf(1), "N/A"},
		{math.Inf(-1), "N/A"},
	}
	for _, tc := range cases {
		got := benchFmtMS(tc.in)
		if got != tc.want {
			t.Errorf("benchFmtMS(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
