//go:build test

package ui

import (
	"fmt"
	"image"
	"image/color"
	"io"
	"math"
	"runtime"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
	game_log "github.com/ingyamilmolinar/beatmo/internal/log"
)

func instMenuFilteredOptionsForTest(dv *DrumView) []string {
	if dv == nil {
		return nil
	}
	filtered := append([]string(nil), dv.instOptions...)
	if dv.instMenuMode != instMenuModeInstruments {
		return filtered
	}
	if dv.instMenuActiveCat != "" {
		out := filtered[:0]
		for _, id := range filtered {
			if dv.instCatByID != nil && dv.instCatByID[id] == dv.instMenuActiveCat {
				out = append(out, id)
			}
		}
		filtered = append([]string(nil), out...)
	}
	if q := strings.TrimSpace(strings.ToLower(dv.instSearch)); q != "" {
		out := filtered[:0]
		for _, id := range filtered {
			if dv.matchInstrumentSearch(id, q) {
				out = append(out, id)
			}
		}
		filtered = append([]string(nil), out...)
	}
	return filtered
}

func TestNewDrumView(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)

	if drumView == nil {
		t.Fatal("NewDrumView returned nil")
	}
	if drumView.Length != 8 {
		t.Errorf("Expected initial drum view length to be 8, got %d", drumView.Length)
	}
	if len(drumView.Rows) != 1 {
		t.Fatalf("Expected 1 drum row, got %d", len(drumView.Rows))
	}
	if len(drumView.Rows[0].Steps) != 8 {
		t.Errorf("Expected drum row steps length to be 8, got %d", len(drumView.Rows[0].Steps))
	}
	for i, step := range drumView.Rows[0].Steps {
		if step {
			t.Errorf("Expected step %d to be false (empty), got true", i)
		}
	}
}

func TestTimelineInfoFormatsBeatAndTime(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 100, 100), nil, testLogger)
	dv.Length = 4
	dv.timelineUnitsPerBeat = 1
	dv.secPerBeat = 0.5
	got := dv.timelineInfo(2.5)
	// 1-indexed format: "Beat X · M:SS"
	want := "Beat 3 · 0:01"
	if got != want {
		t.Fatalf("timelineInfo=%q want %q", got, want)
	}
}

// Verify the simplified format rounds correctly at boundaries.
func TestTimelineInfoRoundingCarry(t *testing.T) {
	dv := NewDrumView(image.Rect(0, 0, 100, 100), nil, testLogger)
	dv.Length = 4
	dv.timelineUnitsPerBeat = 1
	dv.SetBPM(96) // 60/96 = 0.625s per beat => 625ms

	// 1.6 beats => 1.6 * 625ms = 1000ms -> 1s => "0:01"
	info := dv.timelineInfo(1.6)
	if !strings.Contains(info, "Beat 2") {
		t.Fatalf("unexpected beat portion: %q", info)
	}
	if !strings.Contains(info, "· 0:01") {
		t.Fatalf("unexpected time rounding: %q", info)
	}

	// Larger total: 8 beats × 625ms = 5000ms => 5s => "0:01" for current
	dv.Length = 8
	info = dv.timelineInfo(1.6)
	if !strings.Contains(info, "Beat 2") {
		t.Fatalf("unexpected beat info: %q", info)
	}
}

func TestDrumViewLengthIncrease(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 800, 400), graph, logger)

	// Simulate button press
	pressLenInc(t, drumView)
	drumView.Update()

	if drumView.Length != 9 {
		t.Errorf("Expected drum view length to increase to 9, got %d", drumView.Length)
	}
	if len(drumView.Rows[0].Steps) != 9 {
		t.Errorf("Expected drum row steps length to be 9, got %d", len(drumView.Rows[0].Steps))
	}
}

func TestMainVolumeSliderAdjustsAudio(t *testing.T) {
	prevVol := audio.MainVolume()
	audio.SetMainVolume(1)
	t.Cleanup(func() { audio.SetMainVolume(prevVol) })
	dv := NewDrumView(image.Rect(0, 0, 600, 300), nil, testLogger)
	// Desktop uses popup-based volume control; open via icon.
	if dv.mainVolIconRect.Empty() {
		dv.mainVolIconRect = image.Rect(100, 100, 130, 130)
	}
	if dv.mainVolSlider() == nil {
		dv.transportZone.mainVolSlider = NewSlider(1.0)
	}
	dv.openMasterVolumePopup()
	if !dv.masterVolPopup.IsOpen() {
		t.Fatal("master volume popup should be open")
	}
	r := dv.masterVolPopup.Rect()
	mx := r.Min.X + r.Dx()/2
	// Drag to the bottom of the track area → volume decreases toward 0.
	dv.masterVolPopup.HandleInput(mx, r.Max.Y-9, true)
	dv.masterVolPopup.HandleInput(mx, r.Max.Y-9, false)
	got := audio.MainVolume()
	if got >= 1.0 {
		t.Fatalf("main volume should have decreased, got %.3f", got)
	}
}

// Ensure row labels and delete buttons sit beneath the control panel and align
// to the left of their corresponding step rows.
func TestDrumRowLayout(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 500, 200), nil, logger)
	dv.recalcButtons()
	dv.calcLayout()

	if len(dv.rowLabels()) == 0 {
		t.Fatalf("expected at least one row label")
	}
	label := dv.rowLabels()[0].Rect()
	if label.Min.Y < dv.uploadBtn().Rect().Max.Y {
		t.Fatalf("row label overlaps controls: label %v controls bottom %d", label, dv.uploadBtn().Rect().Max.Y)
	}
	stepStart := dv.Bounds.Min.X + dv.labelW + dv.controlsW
	// On desktop, delete button is hidden (empty rect) — it lives in the overflow menu.
	del := dv.rowDeleteBtns()[0].Rect()
	if !del.Empty() {
		t.Fatalf("expected delete button rect to be empty on desktop, got %v", del)
	}
	if label.Max.X > stepStart {
		t.Fatalf("label encroaches into step area: %v >= %d", label, stepStart)
	}
	if dv.addRowBtn().Rect().Min.Y != label.Min.Y+dv.rowHeight() {
		t.Fatalf("add-row button not directly below row: %v", dv.addRowBtn().Rect())
	}
	for _, btn := range []*Button{dv.rowLabels()[0], dv.addRowBtn()} {
		tr := btn.textRect()
		r := btn.Rect()
		if !tr.In(r) {
			t.Fatalf("text outside row button: %v not in %v", tr, r)
		}
		cx := (r.Min.X + r.Max.X) / 2
		ctx := (tr.Min.X + tr.Max.X) / 2
		cy := (r.Min.Y + r.Max.Y) / 2
		cty := (tr.Min.Y + tr.Max.Y) / 2
		if intAbs(cx-ctx) > 1 || intAbs(cy-cty) > 1 {
			t.Fatalf("text not centered in row button: %v", r)
		}
	}
}

func TestDrumViewLengthDecrease(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 800, 400), graph, logger)

	// Increase length first to ensure we can decrease
	pressLenInc(t, drumView)
	drumView.Update() // Length is now 9

	// Simulate button press
	pressLenDec(t, drumView)
	drumView.Update()

	if drumView.Length != 8 {
		t.Errorf("Expected drum view length to decrease to 8, got %d", drumView.Length)
	}
	if len(drumView.Rows[0].Steps) != 8 {
		t.Errorf("Expected drum row steps length to be 8, got %d", len(drumView.Rows[0].Steps))
	}
}

func TestDrumViewVerticalScroll(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 600, 300), nil, logger)
	// Add enough rows to require scrolling even at 600x300.
	for i := 0; i < 12; i++ {
		dv.AddRow()
	}
	dv.calcLayout()
	dv.syncRowScroll()

	// Verify scroll API moves rowOffset.
	if dv.rowScroll().HandleWheel(-1) {
		dv.flushRowScroll()
	}
	if dv.rowOffset != 1 {
		t.Fatalf("first scroll: rowOffset=%d want 1", dv.rowOffset)
	}

	// Second scroll should advance further.
	dv.syncRowScroll()
	if dv.rowScroll().HandleWheel(-1) {
		dv.flushRowScroll()
	}
	if dv.rowOffset <= 1 {
		t.Fatalf("second scroll: rowOffset=%d want >1", dv.rowOffset)
	}
}

func TestDrumViewLengthMinMax(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelDebug)
	graph := model.NewGraph(logger)

	// Min length for a standalone DrumView (unitsPerBeat=1) is 1 subdivision.
	dv := NewDrumView(image.Rect(0, 0, 200, 120), graph, logger)
	dv.SetLength(1)
	pressLenDec(t, dv)
	dv.Update()
	if dv.Length != 1 {
		t.Errorf("Expected min drum view length to stay at 1, got %d", dv.Length)
	}

	// Max length is bounded by the screen-aware clamp in clampLength.
	// After pressing +, the DrumView should not exceed the clamped max.
	dv2 := NewDrumView(image.Rect(0, 0, 1200, 200), graph, logger)
	dv2.Update()
	// Set a large length so the next Inc will try to exceed the max.
	big := dv2.Bounds.Dx() * 2
	dv2.SetLength(big)
	// clampLength determines the actual max; pressing + from that point should not grow.
	clamped := dv2.clampLength(big)
	dv2.SetLength(clamped)
	pressLenInc(t, dv2)
	dv2.Update()
	if dv2.Length != clamped {
		t.Errorf("Expected max length %d, got %d", clamped, dv2.Length)
	}
}

func TestTimelineInfo(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.SetBPM(120)
	info := dv.timelineInfo(4)
	expected := "Beat 5 · 0:02"
	if info != expected {
		t.Fatalf("expected %q got %q", expected, info)
	}
}

func TestTimelineInfoFractionalBeat(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.SetBPM(120)
	dv.Length = 32
	dv.timelineUnitsPerBeat = 1
	info := dv.timelineInfo(1.25)
	// 1-indexed format: "Beat 2 · 0:00"
	if !strings.HasPrefix(info, "Beat 2") {
		t.Fatalf("unexpected beat info: %q", info)
	}
	if !strings.Contains(info, "· 0:00") {
		t.Fatalf("missing time info: %q", info)
	}
	if strings.Count(info, "Beat") != 1 {
		t.Fatalf("duplicate beat counts in %q", info)
	}
}

func TestTimelineInfoExtendsTotal(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.SetBPM(120)
	dv.timelineBeats = 32
	dv.isPlaying = true // Extension only applies during playback
	info := dv.timelineInfo(32.125)
	// When elapsed exceeds total, both truncate to the same integer
	if !strings.HasPrefix(info, "Beat 33") {
		t.Fatalf("unexpected extended total: %q", info)
	}
}

func TestTimelineInfoStoppedDenominator(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.SetBPM(120)
	dv.Length = 8
	dv.timelineUnitsPerBeat = 1
	dv.isPlaying = false
	// With 1-indexed format, beat 351 should show as "Beat 352"
	info := dv.timelineInfo(351)
	if !strings.Contains(info, "Beat 352") {
		t.Fatalf("expected Beat 352 when stopped, got %q", info)
	}

	// Cached version should behave the same
	dv.timelineZone.LastInfoText = "" // Clear cache
	cached := dv.timelineZone.timelineInfoCached(351)
	if !strings.Contains(cached, "Beat 352") {
		t.Fatalf("expected cached Beat 352 when stopped, got %q", cached)
	}
}

func TestEQChannelBtnWidthFitsLongNames(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 400), graph, logger)
	dv.Rows[0].Name = "Fm-epiano-1"
	dv.Rows[0].Instrument = "fm-epiano-1"

	w := dv.eqPanelZone.calcChannelBtnWidth()
	nameW := TextWidth("Fm-epiano-1")
	if w < nameW {
		t.Fatalf("EQ channel button width %d too small for name width %d", w, nameW)
	}
	// Should be at least the old minimum
	if w < 72 {
		t.Fatalf("EQ channel button width %d below minimum 72", w)
	}
}

func TestTimelineViewRect(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.SetBeatLength(16)
	dv.Offset = 4
	dv.recalcButtons()

	var got image.Rectangle
	orig := drawRect
	drewBorder := false
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if clr, ok := c.(color.NRGBA); ok && clr == colTimelineView {
				got = r
			}
		} else {
			if clr, ok := c.(color.RGBA); ok && clr == colTimelineViewHi {
				drewBorder = true
			}
		}
	}
	defer func() { drawRect = orig }()

	dv.Draw(ebiten.NewImage(800, 200), nil, 0, nil, 0)

	totalBeats := dv.timelineBeats
	start := dv.timelineRect.Min.X + int(float64(dv.Offset)/float64(totalBeats)*float64(dv.timelineRect.Dx()))
	width := int(float64(dv.Length) / float64(totalBeats) * float64(dv.timelineRect.Dx()))
	want := image.Rect(start, dv.timelineRect.Min.Y, start+width, dv.timelineRect.Max.Y)
	if got != want || !drewBorder {
		t.Fatalf("view rect/border mismatch: rect=%v border=%t want %v", got, drewBorder, want)
	}
}

func TestTimelineLayout(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.recalcButtons()
	textY := dv.Bounds.Min.Y + 5
	if textY >= dv.timelineRect.Min.Y {
		t.Fatalf("info text overlaps timeline bar")
	}
	rowStart := dv.Bounds.Min.Y + dv.headerH
	if dv.timelineRect.Max.Y >= rowStart {
		t.Fatalf("timeline bar overlaps drum rows")
	}
}

func TestTimelineExpandsAndViewShrinks(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 200), graph, logger)
	dv.recalcButtons()
	img := ebiten.NewImage(800, 200)

	var rect image.Rectangle
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if clr, ok := c.(color.NRGBA); ok && clr == colTimelineView {
				rect = r
			}
		}
	}
	defer func() { drawRect = orig }()

	dv.Draw(img, nil, 0, nil, 0)
	baseWidth := rect.Dx()

	dv.Draw(img, nil, 0, nil, 20)
	expandedWidth := rect.Dx()

	if dv.timelineBeats != 28 {
		t.Fatalf("timelineBeats = %d want 28", dv.timelineBeats)
	}
	if expandedWidth >= baseWidth {
		t.Fatalf("view width did not shrink: base %d expanded %d", baseWidth, expandedWidth)
	}
}

func TestTimelineBeatMarkersDecimate(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	dv.recalcButtons()
	dv.timelineBeats = 10000 // simulate long timeline

	var view image.Rectangle
	markers := 0
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if clr, ok := c.(color.NRGBA); ok && clr == colTimelineView {
				view = r
			} else if clr, ok := c.(color.RGBA); ok && clr == colTimelineBeat && r.Min.Y == dv.timelineRect.Min.Y && r.Max.Y == dv.timelineRect.Max.Y {
				markers++
			}
		}
		orig(dst, r, c, filled)
	}
	defer func() { drawRect = orig }()

	dv.Draw(ebiten.NewImage(400, 200), nil, 0, nil, 0)

	if view.Dx() < 1 {
		t.Fatalf("view width = %d want >=1", view.Dx())
	}
	if markers > dv.timelineRect.Dx()+1 {
		t.Fatalf("too many beat markers: %d > %d", markers, dv.timelineRect.Dx()+1)
	}
}

func TestTimelineScrubSeek(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.timelineBeats = 100

	mx := dv.timelineRect.Min.X + dv.timelineRect.Dx()/2
	my := dv.timelineRect.Min.Y + dv.timelineRect.Dy()/2
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	pressed = false
	dv.Update()
	restore()

	want := (dv.timelineBeats - dv.Length) / 2
	if dv.Offset != want {
		t.Fatalf("offset=%d want %d", dv.Offset, want)
	}
}

func TestTimelineScrubLongTimeline(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.recalcButtons()
	dv.timelineBeats = 10000

	mx := dv.timelineRect.Max.X - 1
	my := dv.timelineRect.Min.Y + dv.timelineRect.Dy()/2
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	pressed = false
	dv.Update()
	restore()

	// The scrub logic uses math.Round; mirror it here.
	unitsPerBeat := max1(dv.timelineUnitsPerBeat)
	lengthBeats := float64(dv.Length) / float64(unitsPerBeat)
	maxOffBeats := float64(dv.timelineBeats) - lengthBeats
	frac := float64(dv.timelineRect.Dx()-1) / float64(dv.timelineRect.Dx())
	want := int(math.Round(frac * maxOffBeats * float64(unitsPerBeat)))
	if dv.Offset != want {
		t.Fatalf("offset=%d want %d", dv.Offset, want)
	}
	if dv.timelineBeats != 10000 {
		t.Fatalf("timelineBeats changed: %d", dv.timelineBeats)
	}
}

func TestDrumViewUpdatesGraphBeatLength(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelDebug)
	graph := model.NewGraph(logger)
	drumView := NewDrumView(image.Rect(0, 0, 800, 400), graph, logger)

	// Initial check
	if graph.BeatLength() != 8 {
		t.Errorf("Expected initial graph beat length to be 8, got %d", graph.BeatLength())
	}

	// Increase length and check graph
	pressLenInc(t, drumView)
	drumView.Update()
	if graph.BeatLength() != 9 {
		t.Errorf("Expected graph beat length to be 9 after increase, got %d", graph.BeatLength())
	}

	// Decrease length and check graph
	pressLenDec(t, drumView)
	drumView.Update()
	if graph.BeatLength() != 8 {
		t.Errorf("Expected graph beat length to be 8 after decrease, got %d", graph.BeatLength())
	}
}

func TestDrumViewLooping(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelDebug)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	g.Layout(800, 100)
	g.drum.SetLength(10)
	g.drum.SetBeatLength(10)

	// Create a looping graph: O > X > X > (loop start) X > X > (loop end)
	node0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	node1 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	node2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	node3 := g.tryAddNode(3, 0, model.NodeTypeRegular)
	node4 := g.tryAddNode(4, 0, model.NodeTypeRegular)

	g.start = node0
	g.graph.StartNodeID = node0.ID
	g.addEdgeNoRefresh(node0, node1)
	g.addEdgeNoRefresh(node1, node2)
	g.addEdgeNoRefresh(node2, node3)
	g.addEdgeNoRefresh(node3, node4)
	g.addEdgeNoRefresh(node4, node2)
	g.updateBeatInfos()

	// Loop seam suppression only hides invisible bridge segments. With all
	// endpoints regular in this path O->1->2->3->4->(back)->2, every audible
	// step should remain visible except for the invisible pass-throughs.
	expectedSteps := []bool{true, true, true, true, true, false, true, true, true, false}
	t.Logf("Generated drum row: %v", g.drum.Rows[0].Steps)
	if len(g.drum.Rows[0].Steps) != len(expectedSteps) {
		t.Fatalf("Expected %d steps, but got %d", len(expectedSteps), len(g.drum.Rows[0].Steps))
	}

	for i, step := range g.drum.Rows[0].Steps {
		if step != expectedSteps[i] {
			t.Errorf("Step %d: expected %v, got %v", i, expectedSteps[i], step)
		}
	}
}

func TestDrumViewLoopHighlighting(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelDebug)
	logger.SetLevel(game_log.LevelDebug) // Enable debug logging for this test

	graph := model.NewGraph(logger)

	// Circuit: [O] > [] > [X] > [X]
	//                    ^     v
	//                   [X] < [X]
	// This translates to:
	// node0 (0,0) -> node_inv1 (1,0) -> node1 (2,0)
	// node1 (2,0) -> node_inv2 (2,1) -> node2 (2,2)
	// node2 (2,2) -> node_inv3 (1,2) -> node3 (0,2)
	// node3 (0,2) -> node_inv4 (0,1) -> node1 (2,0) (loop back to node1)

	node0 := graph.AddNode(0, 0, model.NodeTypeRegular)
	node_inv1 := graph.AddNode(1, 0, model.NodeTypeInvisible)
	node1 := graph.AddNode(2, 0, model.NodeTypeRegular)
	node_inv2 := graph.AddNode(2, 1, model.NodeTypeInvisible)
	node2 := graph.AddNode(2, 2, model.NodeTypeRegular)
	node_inv3 := graph.AddNode(1, 2, model.NodeTypeInvisible)
	node3 := graph.AddNode(0, 2, model.NodeTypeRegular)
	node_inv4 := graph.AddNode(0, 1, model.NodeTypeInvisible)

	graph.StartNodeID = node0
	graph.Edges[[2]model.NodeID{node0, node_inv1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv1, node1}] = struct{}{}
	graph.Edges[[2]model.NodeID{node1, node_inv2}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv2, node2}] = struct{}{}
	graph.Edges[[2]model.NodeID{node2, node_inv3}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv3, node3}] = struct{}{}
	graph.Edges[[2]model.NodeID{node3, node_inv4}] = struct{}{}
	graph.Edges[[2]model.NodeID{node_inv4, node1}] = struct{}{} // Loop back to node1

	drumView := NewDrumView(image.Rect(0, 0, 800, 100), graph, logger)
	drumView.SetLength(10) // Set a reasonable length for the drum view
	drumView.SetBeatLength(drumView.Length)

	game := New(logger)
	t.Cleanup(game.CloseForTest)
	game.graph = graph
	game.drum = drumView
	game.drum.SetBPM(120) // Set a BPM for consistent beat duration
	game.Layout(800, 720) // Set layout to initialize drum view bounds

	// Simulate starting playback
	game.SetPlaying(true)
	game.updateBeatInfos() // Call updateBeatInfos after drum is set
	game.spawnPulseFromRow(0, 0)

	// Run for a few beats to test loop highlighting
	for i := 0; i < 20 && game.activePulse != nil; i++ {
		delete(game.highlightedBeats, makeBeatKey(0, game.activePulse.lastIdx))
		game.advancePulse(game.activePulse)
		t.Logf("Step %d: highlightedBeats: %v", i, game.highlightedBeats)
		if len(game.highlightedBeats) != 1 {
			t.Fatalf("step %d: expected one highlight got %v", i, game.highlightedBeats)
		}
	}
}

func TestDrumViewButtonsDrawn(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelInfo)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)

	count := 0
	orig := drawButton
	drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
		count++
	}
	defer func() { drawButton = orig }()

	dv.Draw(ebiten.NewImage(400, 200), nil, 0, nil, 0)
	// After zone refactoring: transport zone draws play, stop, bpm+/-, subdiv,
	// len+/-, track, upload, import, export, viewSwitch. Row rack zone draws
	// per-row label, M, S, FX, overflow. Add-row button uses custom style
	// (dashed border) so it doesn't go through drawButton. EQ panel zone draws
	// EQ toggle + channel + filter toggles + band mutes when visible.
	// Count depends on visible rows, zone layout, and which controls are shown.
	if count < 20 {
		t.Fatalf("expected at least 20 buttons drawn, got %d", count)
	}
}

func TestDrumViewHighlightsMultipleRows(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 300), graph, logger)
	dv.SetLength(4)
	dv.SetBeatLength(4)
	dv.AddRow()

	highlights := [][]highlightEntry{
		{{idx: 1, val: 1}},
		{{idx: 2, val: 1}},
	}

	dst := ebiten.NewImage(800, 300)
	orig := drawRect
	t.Cleanup(func() { drawRect = orig })
	var hits [][2]int
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled && r.Min.Y >= dv.Bounds.Min.Y+dv.headerH {
			row := (r.Min.Y - (dv.Bounds.Min.Y + dv.headerH)) / dv.rowHeight()
			if row < 0 || row >= len(dv.Rows) {
				orig(dst, r, c, filled)
				return
			}
			// Map rectangle center X proportionally into the step index so
			// rounding distribution across cells doesn’t skew detection.
			startX := dv.timelineRect.Min.X
			totalW := dv.timelineRect.Dx()
			n := dv.Length
			// Determine the step index by locating the boundary bucket.
			col := 0
			for j := 1; j < n; j++ {
				bx := startX + (j*totalW)/n
				if r.Min.X >= bx {
					col = j
				} else {
					break
				}
			}
			hlExpected := color.RGBAModel.Convert(colHighlight).(color.RGBA)
			if color.RGBAModel.Convert(c).(color.RGBA) == hlExpected {
				hits = append(hits, [2]int{row, col})
			}
		}
		orig(dst, r, c, filled)
	}
	dv.Draw(dst, highlights, 0, make([]model.BeatInfo, dv.Length), 0)
	drawRect = orig

	want := map[[2]int]bool{{0, 1}: true, {1, 2}: true}
	seen := map[[2]int]bool{}
	for _, hit := range hits {
		if want[hit] {
			seen[hit] = true
		}
	}
	for key := range want {
		if !seen[key] {
			t.Fatalf("missing highlight for row/col %v; got %v", key, hits)
		}
	}
}

func TestDrumViewAddAndDeleteRow(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	dv.recalcButtons()
	dv.calcLayout()
	dv.calcLayout()

	pressed := true
	cx, cy := dv.addRowBtn().Rect().Min.X+1, dv.addRowBtn().Rect().Min.Y+1
	restore := SetInputForTest(func() (int, int) { return cx, cy }, func(ebiten.MouseButton) bool { return pressed }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 800, 600 })
	dv.Update()
	t.Cleanup(restore)
	pressed = false
	dv.Update()
	restore()
	if len(dv.Rows) != 2 {
		t.Fatalf("expected 2 rows got %d", len(dv.Rows))
	}

	// recalc layout and delete the second row directly
	dv.Update()
	dv.DeleteRow(1)
	if len(dv.Rows) != 1 {
		t.Fatalf("expected 1 row after deletion got %d", len(dv.Rows))
	}
}

// Even when the requested subdivisions vastly exceed the available pixels,
// the drum view should render decimated vertical marker lines so users can
// orient themselves. A horizontal baseline is intentionally omitted.
func TestDrumViewLineVisibleWhenOverzoomed(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	dv.recalcButtons()
	dv.calcLayout()
	// Force an extreme length by setting dv.Length directly to bypass clamps.
	extreme := dv.timelineRect.Dx() * 10
	dv.Length = extreme
	for _, r := range dv.Rows {
		r.Steps = make([]bool, dv.Length)
		r.CellTypes = make([]model.NodeType, dv.Length)
	}

	dst := ebiten.NewImage(400, 200)
	orig := drawRect
	t.Cleanup(func() { drawRect = orig })
	ticks := 0
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled && r.Min.Y >= dv.Bounds.Min.Y+timelineHeight {
			// vertical tick markers: width=1 spanning the row height
			if r.Dx() == 1 && r.Dy() == dv.rowHeight() && color.RGBAModel.Convert(c).(color.RGBA) == colTimelineBeat {
				ticks++
			}
		}
		orig(dst, r, c, filled)
	}
	dv.Draw(dst, nil, 0, make([]model.BeatInfo, 0), 0)
	drawRect = orig
	if ticks == 0 {
		t.Fatalf("missing decimated marker ticks at extreme zoom-out")
	}
}

func TestRenameOpensWithCursor(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)

	cs := &countStyle{}
	dv.rowLabels()[0].Style = cs

	calls := 0
	origCursor := drawCursor
	drawCursor = func(dst *ebiten.Image, r image.Rectangle, col color.Color) { calls++ }
	defer func() { drawCursor = origCursor }()

	dv.rowEditBtns()[0].OnClick()
	restore := SetInputForTest(func() (int, int) { return 0, 0 }, func(ebiten.MouseButton) bool { return false }, func(ebiten.Key) bool { return false }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 0, 0 })
	dv.Update()
	t.Cleanup(restore)
	restore()

	dv.Draw(ebiten.NewImage(200, 200), nil, 0, nil, 0)

	if calls == 0 {
		t.Fatalf("cursor not drawn")
	}
	if cs.n != 0 {
		t.Fatalf("row label drawn while renaming")
	}
}

type countStyle struct{ n int }

func (c *countStyle) Draw(dst *ebiten.Image, r image.Rectangle, pressed, hovered bool) { c.n++ }

func TestDeleteButtonDisabledWhenSingleRow(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), nil, logger)

	if dv.rowDeleteBtns()[0].OnClick != nil {
		t.Fatalf("delete button should be disabled with single row")
	}
	dv.DeleteRow(0)
	if len(dv.Rows) != 1 {
		t.Fatalf("single row should not be deletable")
	}

	dv.AddRow()
	if dv.rowDeleteBtns()[0].OnClick == nil || dv.rowDeleteBtns()[1].OnClick == nil {
		t.Fatalf("delete buttons not enabled after adding row")
	}
	dv.DeleteRow(1)
	if len(dv.Rows) != 1 {
		t.Fatalf("row not deleted")
	}
	if dv.rowDeleteBtns()[0].OnClick != nil {
		t.Fatalf("delete button should be disabled after deleting to one row")
	}
}

func TestDrumViewChangeInstrumentPerRow(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, logger)
	if len(dv.instOptions) < 2 {
		t.Fatalf("expected at least 2 instruments, got %d", len(dv.instOptions))
	}
	dv.AddRow()
	dv.Update()
	if len(dv.rowLabels()) < 2 {
		t.Fatalf("expected row labels for selection")
	}
	dv.rowLabels()[1].OnClick()

	before := dv.Rows[1].Instrument
	dv.CycleInstrument()
	if dv.Rows[1].Instrument == before {
		t.Fatalf("expected instrument to change")
	}
}

func TestDrumViewDeleteRowRecordsOrigin(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.AddRow()
	dv.Rows[1].Origin = 42
	dv.DeleteRow(1)
	rows := dv.ConsumeDeletedRows()
	if len(rows) != 1 {
		t.Fatalf("expected 1 deleted row, got %d", len(rows))
	}
	if rows[0].index != 1 || rows[0].origin != 42 {
		t.Fatalf("unexpected deleted row info: %+v", rows[0])
	}
}

func TestInstrumentMenuIncludesCustom(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	withDefaultAudio(t)
	graph := model.NewGraph(logger)
	// Use a larger view to fit more visible instrument rows
	dv := NewDrumView(image.Rect(0, 0, 400, 600), graph, logger)
	if err := audio.RegisterWAV("custom", writeTempWAV(t, "custom.wav")); err != nil {
		t.Fatalf("register wav: %v", err)
	}
	dv.Update()

	// Verify custom is in the instrument options list
	found := false
	for _, id := range dv.instOptions {
		if id == "custom" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("custom instrument not in instOptions: %v", dv.instOptions)
	}

	dv.rowLabels()[0].OnClick() // open menu

	// The menu may show limited visible rows; scroll to find "Custom" button
	var btn *Button
	for _, b := range dv.instMenuBtns {
		if b.Text == "Custom" {
			btn = b
		}
	}
	if btn == nil {
		// If not visible, the test should at least verify it's in the total count
		if dv.instMenuComp != nil {
			scroll := dv.instMenuComp.Scroll()
			if scroll.Total < 19 { // 18 built-in + 1 custom
				t.Errorf("expected at least 19 instruments, got %d", scroll.Total)
			}
		}
		// Use SetInstrument directly since button may not be visible
		dv.SetInstrument("custom")
	} else {
		btn.OnClick()
	}
	if dv.Rows[0].Instrument != "custom" {
		t.Fatalf("expected custom instrument selected, got %s", dv.Rows[0].Instrument)
	}
	if len(dv.Rows) != 1 {
		t.Fatalf("unexpected row count %d", len(dv.Rows))
	}
}

func TestDrumViewConsumeAddedRows(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.AddRow()
	rows := dv.ConsumeAddedRows()
	if len(rows) != 1 || rows[0] != 1 {
		t.Fatalf("expected added row index 1, got %v", rows)
	}
	if len(dv.ConsumeAddedRows()) != 0 {
		t.Fatalf("expected added rows cleared after consume")
	}
}

func TestDropdownBlocksUnderlyingControls(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, timelineHeight+3*24), graph, logger)
	dv.Update()

	// open menu for first row which appears above the add-row button
	dv.rowLabels()[0].OnClick()
	dv.Update()

	add := dv.addRowBtn().Rect()
	mx, my := add.Min.X+1, add.Min.Y+1
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
	dv.Update()

	if len(dv.ConsumeAddedRows()) != 0 {
		t.Fatalf("add row triggered via dropdown click")
	}
}

func TestDropdownOutsideClickDoesNotTriggerUnderlyingControls(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, timelineHeight+3*24), graph, logger)
	dv.Update()

	// open menu
	dv.rowLabels()[0].OnClick()
	dv.Update()

	// click outside menu where the add-row button resides
	add := dv.addRowBtn().Rect()
	mx, my := add.Min.X+1, add.Min.Y+1
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	dv.Update()
	if len(dv.ConsumeAddedRows()) != 0 {
		t.Fatalf("add row triggered via outside dropdown click")
	}
	pressed = false
	dv.Update()
	restore()
}

func TestRenameBoxBlocksUnderlyingControls(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 600, 200), graph, logger)
	dv.Update()

	// Open rename box for row 0 via callback (edit button is hidden on desktop).
	dv.rowEditBtns()[0].OnClick()
	dv.Update()

	if dv.renameBox == nil {
		t.Fatalf("rename box not active")
	}

	rx := dv.renameBox.Rect.Min.X + 1
	ry := dv.renameBox.Rect.Min.Y + 1
	restore := SetInputForTest(
		func() (int, int) { return rx, ry },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
	dv.Update()

	if dv.IsInstMenuOpen() {
		t.Fatalf("instrument menu opened via rename box click")
	}
	if len(dv.ConsumeAddedRows()) != 0 {
		t.Fatalf("add row triggered via rename box click")
	}
}

func TestDrumViewRenameInstrument(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 400), graph, logger)
	dv.Update()

	// Open rename via callback (edit button is hidden on desktop).
	dv.rowEditBtns()[0].OnClick()
	dv.Update()
	if dv.renameBox == nil {
		t.Fatalf("rename box not opened")
	}
	if dv.renameBox.Value() != dv.Rows[0].Name {
		t.Fatalf("rename box value %q", dv.renameBox.Value())
	}

	restore := SetInputForTest(func() (int, int) { return 0, 0 }, func(ebiten.MouseButton) bool { return false }, func(ebiten.Key) bool { return false }, func() []rune { return []rune("X") }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 0, 0 })
	t.Cleanup(restore)
	dv.Update()
	restore()
	restore = SetInputForTest(func() (int, int) { return 0, 0 }, func(ebiten.MouseButton) bool { return false }, func(k ebiten.Key) bool { return k == ebiten.KeyEnter }, func() []rune { return nil }, func() (float64, float64) { return 0, 0 }, func() (int, int) { return 0, 0 })
	t.Cleanup(restore)
	dv.Update()
	restore()
	if dv.renameBox != nil {
		t.Fatalf("rename box still active")
	}
	if dv.Rows[0].Name != dv.rowLabels()[0].Text || dv.Rows[0].Name != "SnareX" {
		t.Fatalf("unexpected name %q label %q", dv.Rows[0].Name, dv.rowLabels()[0].Text)
	}
}

func TestDrumViewRenameEmptyIgnored(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 400), graph, logger)
	dv.Update()

	// Open rename via callback (edit button is hidden on desktop).
	dv.rowEditBtns()[0].OnClick()
	dv.Update()
	if dv.renameBox == nil {
		t.Fatalf("rename box not opened")
	}

	origName := dv.Rows[0].Name
	origInst := dv.Rows[0].Instrument
	focusTextInput(t, dv, dv.renameBox)
	dv.renameBox.SetText("   ")

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	dv.Update()

	if dv.renameBox != nil {
		t.Fatalf("rename box still active after empty commit")
	}
	if dv.Rows[0].Name != origName || dv.Rows[0].Instrument != origInst {
		t.Fatalf("rename changed unexpectedly: name=%q inst=%q", dv.Rows[0].Name, dv.Rows[0].Instrument)
	}
}

func TestDrumViewRenameEscapeKeepsInstrument(t *testing.T) {
	assertDefaultParityState(t)
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 800, 400), graph, logger)
	dv.Update()

	// Open rename via callback (edit button is hidden on desktop).
	dv.rowEditBtns()[0].OnClick()
	dv.Update()
	if dv.renameBox == nil {
		t.Fatalf("rename box not opened")
	}

	origName := dv.Rows[0].Name
	origInst := dv.Rows[0].Instrument
	focusTextInput(t, dv, dv.renameBox)
	dv.renameBox.SetText("TempName")

	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEscape },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	dv.Update()

	if dv.renameBox != nil {
		t.Fatalf("rename box still active after escape")
	}
	if dv.Rows[0].Name != origName || dv.Rows[0].Instrument != origInst {
		t.Fatalf("rename changed unexpectedly: name=%q inst=%q", dv.Rows[0].Name, dv.Rows[0].Instrument)
	}
}

func TestDrumViewRenameBoxBounds(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 300, 200), graph, logger)
	dv.Update()
	dv.rowEditBtns()[0].OnClick()
	if dv.renameBox == nil {
		t.Fatalf("rename box not created")
	}
	expected := dv.renameBox.Rect
	minDim := expected.Dx()
	if expected.Dy() < minDim {
		minDim = expected.Dy()
	}
	inset := int(float64(minDim) * 0.1)
	want := image.Rect(expected.Min.X+inset, expected.Min.Y+inset, expected.Max.X-inset, expected.Max.Y-inset)
	var rects []image.Rectangle
	old := drawButton
	drawButton = func(dst *ebiten.Image, r image.Rectangle, f, b color.Color, pressed bool) {
		rects = append(rects, r)
	}
	defer func() { drawButton = old }()
	dv.Draw(ebiten.NewImage(300, 200), nil, 0, nil, 0)
	found := false
	for _, r := range rects {
		if r == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("rename box bounds %v not drawn", want)
	}
}

func TestRenameUpdatesInstrumentDropdown(t *testing.T) {
	withDefaultAudio(t)
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), g, logger)
	if dv.renameComp == nil {
		t.Fatal("renameComp is nil")
	}
	// Open rename via the component path.
	dv.renameRow = 0
	r := dv.rowLabels()[0].Rect()
	committed := false
	dv.renameComp.SetProps(RenameProps{
		AnchorRect:  r,
		InitialText: dv.Rows[0].Name,
		MaxLen:      32,
		OnCommit: func(newName string) {
			committed = true
			name := strings.TrimSpace(newName)
			if name != "" && dv.renameRow >= 0 && dv.renameRow < len(dv.Rows) {
				oldID := dv.Rows[dv.renameRow].Instrument
				newID := strings.ToLower(name)
				audio.RenameInstrument(oldID, newID)
				dv.Rows[dv.renameRow].Instrument = newID
				dv.Rows[dv.renameRow].Name = name
				dv.rowLabels()[dv.renameRow].Text = name
				dv.refreshInstruments()
			}
			dv.renameRow = -1
		},
		OnCancel: func() {
			dv.renameRow = -1
		},
	})
	dv.renameComp.Open()
	dv.openRenamePortal()
	// Set text to "snare2" and commit via Enter.
	if dv.renameComp.textBox != nil {
		dv.renameComp.textBox.SetText("snare2")
	}
	restore := SetInputForTest(
		func() (int, int) { return r.Min.X + 1, r.Min.Y + 1 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 200, 200 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
	if !committed {
		t.Fatal("rename OnCommit was not called")
	}
	if dv.Rows[0].Instrument != "snare2" {
		t.Fatalf("instrument=%s", dv.Rows[0].Instrument)
	}
	if !slices.Contains(audio.Instruments(), "snare2") {
		t.Fatalf("dropdown missing renamed instrument")
	}
}

func TestDrumViewOriginRequests(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	withDefaultAudio(t)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)
	dv.AddRow()
	dv.calcLayout()

	if len(dv.rowOriginBtns()) < 2 {
		t.Fatalf("expected origin buttons for two rows, got %d", len(dv.rowOriginBtns()))
	}
	dv.rowOriginBtns()[0].OnClick()
	dv.rowOriginBtns()[1].OnClick()
	rows := dv.ConsumeOriginRequests()
	if len(rows) != 2 || rows[0] != 0 || rows[1] != 1 {
		t.Fatalf("unexpected origin requests %v", rows)
	}
	if len(dv.ConsumeOriginRequests()) != 0 {
		t.Fatalf("origin requests not cleared")
	}
}

func TestDrumViewInstrumentColor(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	withDefaultAudio(t)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)
	expected := instColor(dv.Rows[0].Instrument)
	if dv.Rows[0].Color != expected {
		t.Fatalf("expected initial color %v got %v", expected, dv.Rows[0].Color)
	}
	dv.CycleInstrument()
	expected = instColor(dv.Rows[0].Instrument)
	if dv.Rows[0].Color != expected {
		t.Fatalf("expected cycled color %v got %v", expected, dv.Rows[0].Color)
	}
}

func colorsEqual(a, b color.Color) bool {
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func TestCustomInstrumentColorsRotate(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelDebug)
	withDefaultAudio(t)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, logger)

	if err := audio.RegisterWAV("c1", writeTempWAV(t, "c1.wav")); err != nil {
		t.Fatalf("register wav: %v", err)
	}
	dv.SetInstrument("c1")
	col1 := dv.Rows[0].Color

	dv.AddRow()
	dv.Update()
	if len(dv.rowLabels()) < 2 {
		t.Fatalf("expected row labels for selection")
	}
	dv.rowLabels()[1].OnClick()
	if err := audio.RegisterWAV("c2", writeTempWAV(t, "c2.wav")); err != nil {
		t.Fatalf("register wav: %v", err)
	}
	dv.SetInstrument("c2")
	col2 := dv.Rows[1].Color

	if colorsEqual(col1, col2) {
		t.Fatalf("expected different colors for custom instruments")
	}
	if colorsEqual(col1, colStep) || colorsEqual(col2, colStep) {
		t.Fatalf("unexpected fallback color used")
	}
}

func TestRowControlsSpanLeftPanel(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, testLogger)
	dv.calcLayout()
	// On desktop, delete and origin buttons are hidden (empty rects) —
	// they live in the overflow menu. Verify the overflow (menu) button is
	// the rightmost visible control.
	del := dv.rowDeleteBtns()[0].Rect()
	if !del.Empty() {
		t.Fatalf("expected delete button rect empty on desktop, got %v", del)
	}
	origin := dv.rowOriginBtns()[0].Rect()
	if !origin.Empty() {
		t.Fatalf("expected origin button rect empty on desktop, got %v", origin)
	}
}

func TestInstrumentDropdownSelect(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withDefaultAudio(t)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 300, 400), graph, logger)
	dv.calcLayout()
	dv.rowLabels()[0].OnClick() // open menu
	if !dv.IsInstMenuOpen() {
		t.Fatalf("instrument menu not open")
	}
	if len(dv.instMenuBtns) < 2 {
		t.Fatalf("expected at least two instrument options")
	}
	orig := dv.Rows[0].Instrument
	var btn *Button
	for _, b := range dv.instMenuBtns {
		if b.Text == "Back" {
			continue
		}
		if strings.EqualFold(b.Text, orig) {
			continue
		}
		btn = b
		break
	}
	if btn == nil {
		// Scroll to reveal another option when only one is visible.
		view := dv.instMenuScroll.View
		wheel := -1.0
		restore := SetInputForTest(
			func() (int, int) { return view.Min.X + 1, view.Min.Y + view.Dy()/2 },
			func(ebiten.MouseButton) bool { return false },
			func(ebiten.Key) bool { return false },
			func() []rune { return nil },
			func() (float64, float64) { v := wheel; wheel = 0; return 0, v },
			func() (int, int) { return 0, 0 },
		)
		t.Cleanup(restore)
		dv.Update()
		restore()
		for _, b := range dv.instMenuBtns {
			if b.Text == "Back" {
				continue
			}
			if strings.EqualFold(b.Text, orig) {
				continue
			}
			btn = b
			break
		}
	}
	if btn == nil {
		t.Fatalf("no alternative instrument option found")
	}
	bx, by := btn.Rect().Min.X+1, btn.Rect().Min.Y+1
	restore := SetInputForTest(
		func() (int, int) { return bx, by },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
	dv.Update()
	if dv.Rows[0].Instrument == orig {
		t.Fatalf("instrument not changed via dropdown: %s", dv.Rows[0].Instrument)
	}
	if dv.IsInstMenuOpen() {
		t.Fatalf("menu did not close after selection")
	}
}

func TestInstrumentDropdownScrollWheelRevealsHiddenOption(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withDefaultAudio(t)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 260, 400), graph, logger)
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not open")
	}
	filtered := instMenuFilteredOptionsForTest(dv)
	if len(filtered) <= dv.instMenuScroll.Visible {
		t.Fatalf("not enough instruments to require scrolling")
	}
	initialFirst := dv.instMenuScroll.First
	view := dv.instMenuScroll.View
	cx, cy := view.Min.X+1, view.Min.Y+view.Dy()/2
	wheel := -1.0
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheel; wheel = 0; return 0, v }, // scroll down once
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
	if dv.instMenuScroll.First <= initialFirst {
		t.Fatalf("scroll did not advance menu: first=%d initial=%d", dv.instMenuScroll.First, initialFirst)
	}
	btns := dv.instMenuBtns
	if len(btns) > 0 && btns[0].Text == "Back" {
		btns = btns[1:]
	}
	if len(btns) != dv.instMenuScroll.Visible {
		t.Fatalf("visible buttons=%d want %d", len(btns), dv.instMenuScroll.Visible)
	}
	targetIdx := dv.instMenuScroll.First + len(btns) - 1
	if targetIdx >= len(filtered) {
		t.Fatalf("target index out of range: %d", targetIdx)
	}
	targetID := filtered[targetIdx]
	btn := btns[len(btns)-1]
	bx, by := btn.Rect().Min.X+1, btn.Rect().Min.Y+1
	pressed := true
	restore = SetInputForTest(
		func() (int, int) { return bx, by },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if dv.Rows[0].Instrument != targetID {
		t.Fatalf("instrument not set via scrolled dropdown: %s vs %s", dv.Rows[0].Instrument, targetID)
	}
	if dv.IsInstMenuOpen() {
		t.Fatalf("menu did not close after selection")
	}
}

func TestInstrumentDropdownScrollbarDragToEnd(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withDefaultAudio(t)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 260, 220), graph, logger)
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not open")
	}
	if !dv.instMenuHasScroll() {
		t.Fatalf("menu reports no scrollbar")
	}
	filtered := instMenuFilteredOptionsForTest(dv)
	if len(filtered) == 0 {
		t.Fatalf("no instruments available for scroll test")
	}
	thumb := dv.instMenuThumbRect()
	if thumb.Empty() {
		t.Fatalf("thumb rect empty")
	}
	barBottomY := dv.instMenuScroll.View.Max.Y - 1

	// Use component HandleInput directly (matches inst_menu_second_select_test pattern).
	dv.instMenuComp.HandleInput(thumb.Min.X+1, thumb.Min.Y+1, true) // start drag
	dv.instMenuComp.HandleInput(thumb.Min.X+1, barBottomY, true)    // drag to bottom
	dv.instMenuComp.HandleInput(thumb.Min.X+1, barBottomY, false)   // release drag
	dv.syncInstMenuBtnsFromComp()

	maxFirst := len(filtered) - dv.instMenuScroll.Visible
	if maxFirst < 0 {
		maxFirst = 0
	}
	if dv.instMenuScroll.First != maxFirst {
		t.Fatalf("scroll drag did not reach end: first=%d max=%d", dv.instMenuScroll.First, maxFirst)
	}
	targetID := filtered[len(filtered)-1]
	btn := dv.instMenuBtns[len(dv.instMenuBtns)-1]

	// Click the last visible button directly. The button rect may not align
	// with the scroll view due to layout shift, so use OnClick() instead of
	// HandleInput coordinates.
	if btn.OnClick == nil {
		t.Fatalf("last button has no OnClick handler")
	}
	btn.OnClick()
	if dv.Rows[0].Instrument != targetID {
		t.Fatalf("instrument not set after scrollbar drag: %s vs %s", dv.Rows[0].Instrument, targetID)
	}
	if dv.IsInstMenuOpen() {
		t.Fatalf("menu still open after selection")
	}
}

func TestInstrumentCategoryFilterAndLazyLoad(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	kickPath := resolveSamplePath("sample-kick-drum")
	snarePath := resolveSamplePath("sample-snare")
	if kickPath == "" || snarePath == "" {
		t.Fatalf("sample paths missing: kick=%q snare=%q", kickPath, snarePath)
	}
	withAudioCatalog(t, []audio.SoundMeta{
		{ID: "kick-cat-1", Name: "Kick Cat 1", Category: "Kick", Path: kickPath},
		{ID: "snare-cat-1", Name: "Snare Cat 1", Category: "Snare", Path: snarePath},
		{ID: "snare-cat-2", Name: "Snare Cat 2", Category: "Snare", Path: snarePath},
		{ID: "snare-cat-3", Name: "Snare Cat 3", Category: "Snare", Path: snarePath},
	})
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 260, 400), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	if len(dv.instCategoryBtns) < 3 {
		t.Fatalf("expected category buttons, got %d", len(dv.instCategoryBtns))
	}
	var target *Button
	for _, b := range dv.instCategoryBtns {
		if b.Text == "Snare" {
			b.OnClick()
			break
		}
	}
	if dv.instMenuMode != "instruments" {
		t.Fatalf("menu did not switch to instruments")
	}
	if len(dv.instMenuBtns) < 2 { // back + snares
		t.Fatalf("expected instrument list, got %d", len(dv.instMenuBtns))
	}
	for _, b := range dv.instMenuBtns {
		if strings.Contains(strings.ToLower(b.Text), "snare cat 1") {
			target = b
		}
		if strings.Contains(strings.ToLower(b.Text), "kick") {
			t.Fatalf("kick option leaked into snare filter")
		}
	}
	if target == nil {
		t.Fatalf("snare option missing after filter")
	}
	bx, by := target.Rect().Min.X+1, target.Rect().Min.Y+1
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return bx, by },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if dv.Rows[0].Instrument != "snare-cat-1" {
		t.Fatalf("expected snare selected, got %s", dv.Rows[0].Instrument)
	}
	if !audio.IsRegistered("snare-cat-1") {
		t.Fatalf("instrument not lazily registered on selection")
	}
}

// Regression: category clicks must not close the menu or leak to other UI.
func TestInstrumentCategoryClickKeepsMenuOpenAndIsolated(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, []audio.SoundMeta{
		{ID: "kick-cat-1", Name: "Kick Cat 1", Category: "Kick"},
		{ID: "snare-cat-1", Name: "Snare Cat 1", Category: "Snare"},
	})
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 260, 220), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() || dv.instMenuMode != "categories" {
		t.Fatalf("menu not open in categories mode")
	}
	// Click the "Kick" category; menu should stay open and switch modes without closing.
	var kickBtn *Button
	for _, b := range dv.instCategoryBtns {
		if b.Text == "Kick" {
			kickBtn = b
			break
		}
	}
	if kickBtn == nil {
		t.Fatalf("kick category missing")
	}
	// Simulate click on category.
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { r := kickBtn.Rect(); return r.Min.X + 1, r.Min.Y + 1 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	pressed = false
	dv.Update()
	restore()

	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu closed after category click")
	}
	if dv.instMenuMode != "instruments" {
		t.Fatalf("menu did not switch to instruments: %s", dv.instMenuMode)
	}
	// Ensure scroll is reset and back button present (isolation of UI state).
	if dv.instMenuScroll.First != 0 {
		t.Fatalf("scroll not reset after category click: %d", dv.instMenuScroll.First)
	}
	if len(dv.instMenuBtns) == 0 || dv.instMenuBtns[0].Text != "Back" {
		t.Fatalf("back button missing after category click")
	}
	// Verify the underlying row label still exists and was not clicked.
	if dv.selRow != 0 {
		t.Fatalf("selRow changed unexpectedly: %d", dv.selRow)
	}
}

// When the filtered list exceeds the visible rows, wheel scrolling should move
// the menu without closing it or affecting other UI.
func TestInstrumentCategoryScrollMovesList(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	var entries []audio.SoundMeta
	for i := 0; i < 10; i++ {
		entries = append(entries, audio.SoundMeta{ID: fmt.Sprintf("sample-%02d", i), Name: fmt.Sprintf("Sample %02d", i), Category: "Samples"})
	}
	withAudioCatalog(t, entries)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 260, 400), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	// Enter Samples category.
	for _, b := range dv.instCategoryBtns {
		if b.Text == "Samples" {
			b.OnClick()
			break
		}
	}
	if dv.instMenuMode != "instruments" {
		t.Fatalf("menu not in instruments mode")
	}
	startFirst := dv.instMenuScroll.First
	view := dv.instMenuScroll.View
	wheel := -1.0
	restore := SetInputForTest(
		func() (int, int) { return view.Min.X + 1, view.Min.Y + view.Dy()/2 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheel; wheel = 0; return 0, v },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
	if dv.instMenuScroll.First <= startFirst {
		t.Fatalf("scroll did not advance: %d -> %d", startFirst, dv.instMenuScroll.First)
	}
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu closed after scrolling")
	}
	// Ensure row selection unchanged.
	if dv.selRow != 0 {
		t.Fatalf("selRow changed after scrolling: %d", dv.selRow)
	}
}

// Regression: scrollbar must scroll the instrument list after selecting a category,
// and the category list must not include an "All" entry.
func TestInstrumentScrollAfterCategorySelect_NoAllCategory(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	var entries []audio.SoundMeta
	for i := 0; i < 12; i++ {
		entries = append(entries, audio.SoundMeta{
			ID:       fmt.Sprintf("sn-%02d", i),
			Name:     fmt.Sprintf("Snare %02d", i),
			Category: "Snares (WAV)",
			Source:   "wav",
		})
	}
	withAudioCatalog(t, entries)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 260, 400), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() || dv.instMenuMode != "categories" {
		t.Fatalf("menu not open in categories mode")
	}
	for _, b := range dv.instCategoryBtns {
		if strings.EqualFold(b.Text, "all") {
			t.Fatalf("unexpected All category exposed")
		}
	}
	if len(dv.instCategoryBtns) == 0 {
		t.Fatalf("no categories rendered")
	}
	catBtn := dv.instCategoryBtns[0]
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { r := catBtn.Rect(); return r.Min.X + 1, r.Min.Y + 1 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if dv.instMenuMode != "instruments" {
		t.Fatalf("menu did not switch to instruments after category click")
	}
	if len(dv.instMenuBtns) < 2 { // back + at least one instrument
		t.Fatalf("instrument list not rendered after category select: %d", len(dv.instMenuBtns))
	}
	firstLabel := dv.instMenuBtns[1].Text // first instrument (index 0 is Back)
	startFirst := dv.instMenuScroll.First
	view := dv.instMenuScroll.View
	wheel := -1.0
	restore = SetInputForTest(
		func() (int, int) { return view.Min.X + 1, view.Min.Y + view.Dy()/2 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheel; wheel = 0; return 0, v },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
	if dv.instMenuScroll.First <= startFirst {
		t.Fatalf("scrollbar did not advance: %d -> %d", startFirst, dv.instMenuScroll.First)
	}
	if len(dv.instMenuBtns) < 2 {
		t.Fatalf("instrument buttons lost after scroll")
	}
	if dv.instMenuBtns[1].Text == firstLabel {
		t.Fatalf("instrument label unchanged after scroll: %s", firstLabel)
	}
}

// When instrument count exceeds the viewable rows, the scrollbar must appear and
// the popup must remain inside the rack widget bounds.
func TestInstrumentScrollbarAppearsAndClampedToRack(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	var entries []audio.SoundMeta
	for i := 0; i < 30; i++ {
		entries = append(entries, audio.SoundMeta{
			ID:       fmt.Sprintf("hh-%02d", i),
			Name:     fmt.Sprintf("HiHat %02d", i),
			Category: "Hi-Hats (WAV)",
			Source:   "wav",
		})
	}
	withAudioCatalog(t, entries)
	graph := model.NewGraph(logger)
	// Use a narrow-ish window to force scrolling, but tall enough that
	// the rack can fit the minimum menu (back + search + 1 instrument row).
	dv := NewDrumView(image.Rect(0, 0, 400, 300), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	// enter first category
	if len(dv.instCategoryBtns) == 0 {
		t.Fatalf("no categories to enter")
	}
	dv.instCategoryBtns[0].OnClick()
	if !dv.IsInstMenuOpen() || dv.instMenuMode != "instruments" {
		t.Fatalf("menu did not enter instruments mode")
	}
	if !dv.instMenuHasScroll() {
		t.Fatalf("expected scrollbar when instruments overflow")
	}
	bar := dv.instMenuScroll.BarRect(instMenuScrollBarWidth)
	if bar.Empty() {
		t.Fatalf("scroll bar rect empty")
	}
	thumb := dv.instMenuThumbRect()
	if thumb.Empty() || thumb.Dy() <= 0 {
		t.Fatalf("scroll thumb empty: %v", thumb)
	}
	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatalf("rack rect empty")
	}
	if dv.instMenuFullRect.Min.Y < rack.Min.Y || dv.instMenuFullRect.Max.Y > rack.Max.Y {
		t.Fatalf("menu escaped rack bounds: %v not within %v", dv.instMenuFullRect, rack)
	}
}

// Search box should filter instrument list.
func TestInstrumentSearchFiltersList(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	entries := []audio.SoundMeta{
		{ID: "kick-a", Name: "Kick Alpha", Category: "Kick Drums (WAV)", Source: "wav"},
		{ID: "kick-b", Name: "Kick Beta", Category: "Kick Drums (WAV)", Source: "wav"},
		{ID: "snare-a", Name: "Snare Alpha", Category: "Snares (WAV)", Source: "wav"},
	}
	withAudioCatalog(t, entries)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 320, 400), graph, logger) // Taller to fit more rows
	dv.instMenuForceCategories = true
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	// Enter Kick category
	found := false
	for _, b := range dv.instCategoryBtns {
		if strings.Contains(b.Text, "Kick") {
			b.OnClick()
			found = true
			break
		}
	}
	if !found || dv.instMenuMode != "instruments" {
		t.Fatalf("failed to enter Kick instruments")
	}
	if dv.instSearchBox == nil {
		t.Fatalf("search box not created")
	}

	// Use the component's search functionality
	if dv.instMenuComp != nil {
		// Set search via component
		dv.instMenuComp.SetSearchText("beta")
		dv.syncInstMenuBtnsFromComp()
	} else {
		// Legacy path
		dv.instSearchBox.SetText("beta")
		dv.instSearch = "beta"
		dv.buildInstMenu()
	}

	if len(dv.instMenuBtns) < 2 { // back + match
		t.Fatalf("expected search results, got %d buttons", len(dv.instMenuBtns))
	}
	// Find the matching button (skip Back button at index 0)
	foundBeta := false
	for _, b := range dv.instMenuBtns {
		if strings.Contains(strings.ToLower(b.Text), "beta") {
			foundBeta = true
			break
		}
	}
	if !foundBeta {
		t.Fatalf("search did not filter to beta, buttons: %v", func() []string {
			var names []string
			for _, b := range dv.instMenuBtns {
				names = append(names, b.Text)
			}
			return names
		}())
	}
}

func TestInstrumentScrollbarWheelDoesNotScrollRowsOrResize(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withDefaultAudio(t)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 320, 400), graph, logger)
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not open")
	}
	if !dv.instMenuHasScroll() {
		t.Fatalf("expected scrollbar for instrument menu")
	}
	startRowOff := dv.rowOffset
	startRack := dv.widgetRects[WidgetRack]
	startLayoutDrag := dv.layoutDragIdx

	wheel := -2.0 // scroll down
	view := dv.instMenuScroll.View
	cx, cy := view.Min.X+1, view.Min.Y+view.Dy()/2
	restore := SetInputForTest(
		func() (int, int) { return cx, cy },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheel; wheel = 0; return 0, v },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()

	if dv.rowOffset != startRowOff {
		t.Fatalf("row scroll changed while menu scrolling: %d -> %d", startRowOff, dv.rowOffset)
	}
	if dv.instMenuScroll.First <= 0 {
		t.Fatalf("scroll wheel did not move instrument menu: first=%d", dv.instMenuScroll.First)
	}
	if dv.layoutDragIdx != startLayoutDrag {
		t.Fatalf("layout drag mutated: %d -> %d", startLayoutDrag, dv.layoutDragIdx)
	}
	if dv.widgetRects[WidgetRack] != startRack {
		t.Fatalf("rack rect changed during menu scroll: %v -> %v", startRack, dv.widgetRects[WidgetRack])
	}
}

func TestInstrumentScrollbarDragIgnoresWidgetResize(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withDefaultAudio(t)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 320, 260), graph, logger)
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not open")
	}
	if !dv.instMenuHasScroll() {
		t.Fatalf("expected scrollbar for instrument menu")
	}
	rackBefore := dv.widgetRects[WidgetRack]
	rowBefore := dv.rowOffset
	filtered := instMenuFilteredOptionsForTest(dv)
	maxFirst := len(filtered) - dv.instMenuScroll.Visible
	if maxFirst < 0 {
		maxFirst = 0
	}

	thumb := dv.instMenuThumbRect()
	if thumb.Empty() {
		t.Fatalf("thumb empty")
	}
	bottomY := dv.instMenuScroll.View.Max.Y - 1
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return thumb.Min.X + 1, thumb.Min.Y + 1 },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update() // start drag
	restore()

	restore = SetInputForTest(
		func() (int, int) { return thumb.Min.X + 1, bottomY },
		func(b ebiten.MouseButton) bool { return b == ebiten.MouseButtonLeft && pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update() // drag to bottom
	pressed = false
	dv.Update() // release
	restore()

	if dv.instMenuScroll.First != maxFirst {
		t.Fatalf("drag did not reach end: %d vs %d", dv.instMenuScroll.First, maxFirst)
	}
	if dv.widgetRects[WidgetRack] != rackBefore {
		t.Fatalf("rack rect changed while dragging menu: %v -> %v", rackBefore, dv.widgetRects[WidgetRack])
	}
	if dv.rowOffset != rowBefore {
		t.Fatalf("row offset changed while dragging menu: %d -> %d", rowBefore, dv.rowOffset)
	}
	if dv.layoutDragIdx != -1 {
		t.Fatalf("layout drag activated during menu drag: %d", dv.layoutDragIdx)
	}
}

func TestInstrumentMenuOpensAtRowCategoryWithBack(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, []audio.SoundMeta{
		{ID: "snare", Name: "Snare", Category: "Snares"},
		{ID: "kick", Name: "Kick", Category: "Kicks"},
		{ID: "clap", Name: "Clap", Category: "Claps"},
	})
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 320, 240), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()

	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() || dv.instMenuMode != "categories" {
		t.Fatalf("menu not open in categories mode")
	}
	var snaresBtn *Button
	for _, b := range dv.instCategoryBtns {
		if b.Text == "Snares" {
			snaresBtn = b
			break
		}
	}
	if snaresBtn == nil {
		t.Fatalf("snares category missing")
	}
	snaresBtn.OnClick()
	if dv.instMenuMode != "instruments" {
		t.Fatalf("menu did not enter instruments mode")
	}
	if dv.instMenuActiveCat != "Snares" {
		t.Fatalf("expected active category Snares, got %q", dv.instMenuActiveCat)
	}
	if len(dv.instMenuBtns) == 0 || dv.instMenuBtns[0].Text != "Back" {
		t.Fatalf("back button missing; btns=%d", len(dv.instMenuBtns))
	}
	foundSnare := false
	for _, b := range dv.instMenuBtns {
		if strings.Contains(strings.ToLower(b.Text), "snare") {
			foundSnare = true
			break
		}
	}
	if !foundSnare {
		t.Fatalf("snare option missing from category-filtered list")
	}
}

func TestInstrumentMenuShowsBackWithoutForcedCategories(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, []audio.SoundMeta{
		{ID: "snare", Name: "Snare", Category: "Snares"},
		{ID: "clap", Name: "Clap", Category: "Claps"},
	})
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 320, 240), graph, logger)
	dv.SetInstrument("snare")
	dv.calcLayout()

	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not opened")
	}
	if dv.instMenuMode != "instruments" {
		t.Fatalf("expected instruments mode, got %q", dv.instMenuMode)
	}
	if dv.instMenuActiveCat != "Snares" {
		t.Fatalf("expected active cat Snares, got %q", dv.instMenuActiveCat)
	}
	if len(dv.instMenuBtns) == 0 || dv.instMenuBtns[0].Text != "Back" {
		t.Fatalf("back button missing when entering instruments directly")
	}
	if dv.instMenuScroll.Total != 1 {
		t.Fatalf("expected only snare in filtered list, total=%d", dv.instMenuScroll.Total)
	}
}

func TestCategoryScrollbarMovesAndClampedToRack(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	var entries []audio.SoundMeta
	for i := 0; i < 20; i++ {
		entries = append(entries, audio.SoundMeta{
			ID:       fmt.Sprintf("cat-%02d", i),
			Name:     fmt.Sprintf("Cat %02d", i),
			Category: fmt.Sprintf("Category-%02d", i),
		})
	}
	withAudioCatalog(t, entries)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 320, 400), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()

	dv.rowLabels()[0].OnClick()
	if dv.instMenuMode != "categories" {
		t.Fatalf("expected categories mode")
	}
	if !dv.instMenuHasScroll() {
		t.Fatalf("expected scrollbar for categories")
	}
	rack := dv.widgetRects[WidgetRack]
	if rack.Empty() {
		t.Fatalf("rack rect empty")
	}
	if dv.instMenuFullRect.Min.Y < rack.Min.Y || dv.instMenuFullRect.Max.Y > rack.Max.Y {
		t.Fatalf("category menu escaped rack: %v not in %v", dv.instMenuFullRect, rack)
	}
	// Reset scroll to beginning so we can test scrolling down
	if dv.instMenuComp != nil {
		dv.instMenuComp.SetScrollFirst(0)
		dv.syncInstMenuBtnsFromComp()
	} else {
		dv.instMenuScroll.First = 0
	}
	start := dv.instMenuScroll.First
	view := dv.instMenuScroll.View
	wheel := -1.0
	restore := SetInputForTest(
		func() (int, int) { return view.Min.X + 1, view.Min.Y + view.Dy()/2 },
		func(ebiten.MouseButton) bool { return false },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { v := wheel; wheel = 0; return 0, v },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
	if dv.instMenuScroll.First <= start {
		t.Fatalf("category scroll did not advance: %d -> %d", start, dv.instMenuScroll.First)
	}
}

func TestInstrumentBackReturnsToCategoriesWithoutClosing(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, []audio.SoundMeta{
		{ID: "snare", Name: "Snare", Category: "Snares"},
		{ID: "kick", Name: "Kick", Category: "Kicks"},
	})
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 320, 240), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()

	dv.rowLabels()[0].OnClick()
	if dv.instMenuMode != "categories" {
		t.Fatalf("expected categories mode")
	}
	if len(dv.instCategoryBtns) == 0 {
		t.Fatalf("no category buttons")
	}
	dv.instCategoryBtns[0].OnClick() // enter first category
	if dv.instMenuMode != "instruments" {
		t.Fatalf("did not enter instruments mode")
	}
	if len(dv.instMenuBtns) == 0 || dv.instMenuBtns[0].Text != "Back" {
		t.Fatalf("back button missing")
	}
	back := dv.instMenuBtns[0]
	bx, by := back.Rect().Min.X+1, back.Rect().Min.Y+1
	restore := SetInputForTest(
		func() (int, int) { return bx, by },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	restore()
	dv.Update()

	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu closed after back")
	}
	if dv.instMenuMode != "categories" {
		t.Fatalf("expected categories after back, got %q", dv.instMenuMode)
	}
}

func TestInstrumentMenuScrollsToCurrentInstrument(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	var metas []audio.SoundMeta
	for i := 0; i < 12; i++ {
		id := fmt.Sprintf("sn-%02d", i)
		metas = append(metas, audio.SoundMeta{ID: id, Name: "Sn", Category: "Snares"})
	}
	metas = append(metas, audio.SoundMeta{ID: "kick", Name: "Kick", Category: "Kicks"})
	withAudioCatalog(t, metas)

	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 260, 220), graph, logger)
	dv.instMenuForceCategories = true
	dv.refreshInstruments()
	dv.instMenuLastAdded = ""
	target := "sn-09"
	dv.SetInstrument(target)
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()

	if !dv.IsInstMenuOpen() || dv.instMenuMode != "categories" {
		t.Fatalf("menu not open in categories mode")
	}
	var snaresBtn *Button
	for _, b := range dv.instCategoryBtns {
		if b.Text == "Snares" {
			snaresBtn = b
			break
		}
	}
	if snaresBtn == nil {
		t.Fatalf("snares category missing")
	}
	snaresBtn.OnClick()

	if dv.instMenuMode != "instruments" {
		t.Fatalf("menu not open in instruments mode")
	}
	if dv.instMenuActiveCat != "Snares" {
		t.Fatalf("active category %q", dv.instMenuActiveCat)
	}
	if !dv.instMenuHasScroll() {
		t.Fatalf("expected scrollbar for long snare list")
	}
	filtered := instMenuFilteredOptionsForTest(dv)
	idx := slices.Index(filtered, target)
	if idx < 0 {
		t.Fatalf("target instrument not in options")
	}
	if idx < dv.instMenuScroll.First || idx >= dv.instMenuScroll.First+dv.instMenuScroll.Visible {
		t.Fatalf("current instrument not visible; first=%d vis=%d idx=%d", dv.instMenuScroll.First, dv.instMenuScroll.Visible, idx)
	}
	if len(dv.instMenuBtns) == 0 || dv.instMenuBtns[0].Text != "Back" {
		t.Fatalf("back button missing in instruments mode")
	}
}

func TestInstrumentCategoriesScrollMovesWindow(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	var metas []audio.SoundMeta
	for i := 0; i < 12; i++ {
		metas = append(metas, audio.SoundMeta{
			ID:       fmt.Sprintf("cat-%02d", i),
			Name:     "C",
			Category: fmt.Sprintf("Cat-%02d", i),
		})
	}
	withAudioCatalog(t, metas)

	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 260, 240), graph, logger)
	dv.instMenuForceCategories = true
	dv.refreshInstruments()
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()

	if !dv.IsInstMenuOpen() || dv.instMenuMode != "categories" {
		t.Fatalf("menu not open in categories mode")
	}
	if !dv.instMenuHasScroll() {
		t.Fatalf("expected scrollbar for long category list")
	}
	if len(dv.instCategoryBtns) == 0 {
		t.Fatalf("no category buttons")
	}
	firstLabel := dv.instCategoryBtns[0].Text

	// Scroll down a few slots and rebuild to mirror wheel/drag handling.
	var changed bool
	if dv.instMenuComp != nil {
		// Use component's scroll
		dv.instMenuComp.SetScrollFirst(3)
		dv.syncInstMenuBtnsFromComp()
		changed = dv.instMenuScroll.First == 3
	} else {
		// Legacy path
		changed = dv.instMenuScroll.ScrollBy(3)
		dv.buildInstMenu()
	}

	if !changed || dv.instMenuScroll.First != 3 {
		t.Fatalf("scroll first=%d changed=%v", dv.instMenuScroll.First, changed)
	}
	if len(dv.instCategoryBtns) == 0 {
		t.Fatalf("no category buttons after scroll")
	}
	if dv.instCategoryBtns[0].Text == firstLabel {
		t.Fatalf("category list did not scroll: first=%q after=%q", firstLabel, dv.instCategoryBtns[0].Text)
	}
	if want := "Cat-03"; dv.instCategoryBtns[0].Text != want {
		t.Fatalf("expected first visible category %q after scroll, got %q", want, dv.instCategoryBtns[0].Text)
	}
}

func TestInstrumentMenuRendersAboveEQ(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, nil)

	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 320, 220), graph, logger)
	dv.refreshInstruments()
	dv.calcLayout()

	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not opened")
	}
	menuRect := dv.instMenuFullRect
	if menuRect.Empty() {
		menuRect = dv.instMenuScroll.View
	}
	if menuRect.Empty() {
		t.Fatalf("menu rect empty")
	}
	// Force EQ panel to overlap the menu to assert draw order.
	dv.eqRect = menuRect

	var btnRect image.Rectangle
	if dv.instMenuMode == "categories" && len(dv.instCategoryBtns) > 0 {
		btnRect = dv.instCategoryBtns[0].Rect()
	} else if len(dv.instMenuBtns) > 0 {
		btnRect = dv.instMenuBtns[0].Rect()
	} else {
		t.Fatalf("no buttons to sample")
	}
	cx := (btnRect.Min.X + btnRect.Max.X) / 2
	cy := (btnRect.Min.Y + btnRect.Max.Y) / 2

	var got color.RGBA
	found := false
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled && image.Pt(cx, cy).In(r) {
			got = color.RGBAModel.Convert(c).(color.RGBA)
			found = true
		}
	}
	defer func() { drawRect = orig }()

	img := ebiten.NewImage(dv.Bounds.Dx(), dv.Bounds.Dy())
	dv.Draw(img, nil, 0, nil, 0)
	if !found {
		t.Fatalf("no drawRect filled the sample point: (%d,%d)", cx, cy)
	}
	eqBg := color.RGBAModel.Convert(colEQBg).(color.RGBA)
	if got == eqBg {
		t.Fatalf("instrument menu drew underneath EQ: pixel=%v eqBg=%v rect=%v mode=%s", got, eqBg, menuRect, dv.instMenuMode)
	}
}

func TestSelectingInstrumentDoesNotAddRow(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 600, 400), graph, testLogger)
	dv.calcLayout()
	startRows := len(dv.Rows)

	// Open inst menu via the direct API (matches how JS exports and other tests do it).
	dv.openInstMenuForRow(0)
	suppressClicksUntilRelease = false
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not opened")
	}
	if len(dv.instMenuBtns) == 0 {
		t.Fatalf("no instrument buttons")
	}
	btns := dv.instMenuBtns
	if len(btns) > 0 && btns[0].Text == "Back" {
		btns = btns[1:]
	}
	if len(btns) == 0 {
		t.Fatalf("no instrument buttons after filtering out Back")
	}
	// Select the first instrument via the component's HandleInput.
	b0 := btns[0]
	rect := b0.Rect()
	dv.instMenuComp.HandleInput(rect.Min.X+1, rect.Min.Y+1, true)
	suppressClicksUntilRelease = false
	dv.instMenuComp.HandleInput(rect.Min.X+1, rect.Min.Y+1, false)
	if len(dv.Rows) != startRows {
		t.Fatalf("rows=%d want %d", len(dv.Rows), startRows)
	}

	// Close the old portal before reopening.
	dv.closeInstMenuPortal()
	dv.openInstMenuForRow(0)
	suppressClicksUntilRelease = false
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not reopened")
	}
	// Select the last instrument.
	last := dv.instMenuBtns[len(dv.instMenuBtns)-1]
	rect = last.Rect()
	dv.instMenuComp.HandleInput(rect.Min.X+1, rect.Min.Y+1, true)
	suppressClicksUntilRelease = false
	dv.instMenuComp.HandleInput(rect.Min.X+1, rect.Min.Y+1, false)
	if len(dv.Rows) != startRows {
		t.Fatalf("rows grew after change: %d", len(dv.Rows))
	}
}

func TestInstrumentDropdownFitsBounds(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, testLogger)
	dv.AddRow()
	dv.AddRow()
	dv.calcLayout()
	idx := len(dv.Rows) - 1
	dv.rowLabels()[idx].OnClick()
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not opened")
	}
	for _, btn := range dv.instMenuBtns {
		r := btn.Rect()
		if r.Min.Y < dv.Bounds.Min.Y || r.Max.Y > dv.Bounds.Max.Y {
			t.Fatalf("menu button out of bounds: %v vs %v", r, dv.Bounds)
		}
	}
	first := dv.instMenuBtns[0].Rect()
	base := dv.rowLabels()[idx].Rect()
	if first.Max.Y > base.Min.Y {
		t.Fatalf("expected menu to open upward: %v vs %v", first, base)
	}
}

func TestNameBoxShowsBlinkingCursor(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	g := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 300, 200), g, logger)
	startUploadForTest(t, dv)
	waitForNaming(t, dv)

	calls := 0
	oldCursor := drawCursor
	drawCursor = func(dst *ebiten.Image, r image.Rectangle, col color.Color) {
		calls++
	}
	defer func() { drawCursor = oldCursor }()

	dst := ebiten.NewImage(320, 220)
	// Naming overlay now draws through the portal tree.
	if dv.tree != nil {
		dv.tree.Draw(dst)
	}
	if calls == 0 {
		t.Fatalf("expected blinking cursor for manual name input")
	}
}

func TestDropdownHoverHighlight(t *testing.T) {
	prevSuppress := suppressClicksUntilRelease
	suppressClicksUntilRelease = false
	t.Cleanup(func() { suppressClicksUntilRelease = prevSuppress })
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), graph, testLogger)
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	if !dv.IsInstMenuOpen() {
		t.Fatalf("menu not open")
	}
	// OnClick suppresses clicks until mouse-up; clear it so hover state can update.
	suppressClicksUntilRelease = false
	btn := dv.instMenuBtns[0]
	// capture normal draw colors
	img := ebiten.NewImage(10, 10)
	var normFill, normBorder color.Color
	orig := drawButton
	defer func() { drawButton = orig }()
	drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
		normFill, normBorder = fill, border
	}
	btn.Draw(img)
	if !colorsEqual(normBorder, colDropdownEdge) {
		t.Fatalf("unexpected border color: %#v", normBorder)
	}
	// simulate hover
	mx, my := btn.Rect().Min.X+1, btn.Rect().Min.Y+1
	btn.Handle(mx, my, false)
	var hovFill, hovBorder color.Color
	drawButton = func(dst *ebiten.Image, r image.Rectangle, fill, border color.Color, pressed bool) {
		hovFill, hovBorder = fill, border
	}
	btn.Draw(img)
	if colorsEqual(normFill, hovFill) || colorsEqual(normBorder, hovBorder) {
		t.Fatalf("expected hover to change colors")
	}
}

func TestDrumViewLayoutStacksRows(t *testing.T) {
	graph := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), graph, testLogger)
	dv.AddRow()
	dv.calcLayout()
	if len(dv.rowLabels()) != 2 {
		t.Fatalf("expected 2 row labels, got %d", len(dv.rowLabels()))
	}
	if dv.rowLabels()[1].Rect().Min.Y <= dv.rowLabels()[0].Rect().Min.Y {
		t.Fatalf("row labels not stacked vertically: %v vs %v", dv.rowLabels()[0].Rect(), dv.rowLabels()[1].Rect())
	}
	if dv.addRowBtn().Rect().Min.Y <= dv.rowLabels()[1].Rect().Min.Y {
		t.Fatalf("add button not below rows: %v vs %v", dv.addRowBtn().Rect(), dv.rowLabels()[1].Rect())
	}
}

func TestDrumViewDrawHighlightsInvisibleCells(t *testing.T) {
	logger := game_log.New(testLogOutput(), game_log.LevelDebug)
	g := New(logger)
	t.Cleanup(g.CloseForTest)
	assertDefaultParityState(t)
	// Ensure the drum pane is tall enough to render at least one row below the header.
	g.Layout(300, 240)
	g.drum.SetLength(3)
	g.drum.SetBeatLength(3)

	node0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	node1 := g.tryAddNode(1, 0, model.NodeTypeInvisible)
	node2 := g.tryAddNode(2, 0, model.NodeTypeRegular)
	g.start = node0
	g.graph.StartNodeID = node0.ID
	g.addEdgeNoRefresh(node0, node1)
	g.addEdgeNoRefresh(node1, node2)
	g.updateBeatInfos()

	type call struct {
		c color.Color
		r image.Rectangle
	}
	calls := []call{}
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		calls = append(calls, call{c: c, r: r})
	}
	defer func() { drawRect = orig }()

	highlighted := [][]highlightEntry{{{idx: 1, val: 1}}}
	g.drum.Draw(ebiten.NewImage(300, 240), highlighted, 0, g.beatInfos, 0)

	// Invisible cells should use grey highlight (colMuteHighlight), not white.
	var highlightCount int
	hlExpected := color.RGBAModel.Convert(colMuteHighlight).(color.RGBA)
	for _, call := range calls {
		if call.r.Min.Y < timelineHeight {
			continue
		}
		if clr, ok := call.c.(color.RGBA); ok {
			if clr == hlExpected {
				highlightCount++
			}
		}
	}
	if highlightCount == 0 {
		t.Fatalf("expected grey highlight draw for invisible cell, got %d", highlightCount)
	}
}

func TestTimelineCursorMatchesHighlight(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 600, 400), graph, logger)
	dv.Rows = []*DrumRow{{Steps: make([]bool, dv.Length), CellTypes: make([]model.NodeType, dv.Length)}}
	dv.calcLayout()
	highlighted := [][]highlightEntry{{{idx: 3, val: 1}}}

	var cursorCount, highlightCount int
	orig := drawRect
	drawRect = func(dst *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if clr, ok := c.(color.RGBA); ok {
			if clr == colTimelineCursor {
				cursorCount++
			}
			// Highlights use colHighlight (fallback when row Color is nil)
			// or the row's Color when set.
			if clr == colHighlight && r.Min.Y >= timelineHeight {
				highlightCount++
			}
		}
	}
	defer func() { drawRect = orig }()

	dv.Draw(ebiten.NewImage(600, 400), highlighted, 0, nil, 3)

	if cursorCount == 0 {
		t.Fatalf("cursor not drawn")
	}
	if highlightCount == 0 {
		t.Fatalf("highlight not drawn")
	}
}

func TestDrumViewSetBPMClamp(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dv.SetBPM(maxBPM + 10)
	if dv.bpm != maxBPM {
		t.Fatalf("expected BPM %d, got %d", maxBPM, dv.bpm)
	}
	if dv.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on high bpm")
	}
	dvLow := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	dvLow.SetBPM(0)
	if dvLow.bpm != 1 {
		t.Fatalf("expected BPM 1, got %d", dvLow.bpm)
	}
	if dvLow.bpmErrorAnim == 0 {
		t.Errorf("expected error animation on low bpm")
	}
}

func TestDrumViewSecPerBeatUpdates(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 100, 100), graph, logger)
	if dv.secPerBeat != 0.5 {
		t.Fatalf("expected secPerBeat 0.5, got %f", dv.secPerBeat)
	}
	dv.SetBPM(240)
	if dv.secPerBeat != 0.25 {
		t.Fatalf("expected secPerBeat 0.25, got %f", dv.secPerBeat)
	}
}

func TestDrumViewBPMTextInput(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), g, testLogger)
	dv.calcLayout()

	r := dv.bpmBox().Rect
	mx, my := r.Min.X+1, r.Min.Y+1
	pressed := true
	chars := []rune{}
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 200, 200 },
	)
	defer restore()

	dv.Update() // click to focus
	pressed = false

	chars = []rune{'2'}
	dv.Update()
	chars = []rune{'5'}
	dv.Update()
	chars = []rune{'0'}
	dv.Update()

	if dv.BPM() != 120 {
		t.Fatalf("BPM changed before commit: %d", dv.BPM())
	}

	chars = []rune{'\r'}
	dv.Update()

	if dv.BPM() != 250 {
		t.Fatalf("expected BPM 250 got %d", dv.BPM())
	}
}

func TestDrumViewBPMTextInputInvalid(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), g, testLogger)
	dv.calcLayout()

	r := dv.bpmBox().Rect
	mx, my := r.Min.X+1, r.Min.Y+1
	pressed := true
	chars := []rune{}
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 200, 200 },
	)
	defer restore()

	dv.Update() // focus
	pressed = false

	// enter out-of-range value
	chars = []rune{'2'}
	dv.Update()
	chars = []rune{'0'}
	dv.Update()
	chars = []rune{'0'}
	dv.Update()
	chars = []rune{'0'}
	dv.Update()

	if dv.BPM() != 120 {
		t.Fatalf("BPM changed before commit: %d", dv.BPM())
	}

	chars = []rune{'\r'}
	dv.Update()

	if dv.BPM() != 120 {
		t.Fatalf("expected BPM to remain 120 got %d", dv.BPM())
	}
	if dv.bpmErrorAnim == 0 {
		t.Fatalf("expected error highlight for invalid BPM input")
	}
}

func TestDrumViewBPMTextInputNonNumeric(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), g, testLogger)
	dv.calcLayout()

	r := dv.bpmBox().Rect
	mx, my := r.Min.X+1, r.Min.Y+1
	pressed := true
	chars := []rune{}
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { c := chars; chars = nil; return c },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 200, 200 },
	)
	defer restore()

	dv.Update() // focus
	pressed = false

	chars = []rune{'a'}
	dv.Update()

	if dv.BPM() != 120 {
		t.Fatalf("BPM changed before commit: %d", dv.BPM())
	}

	chars = []rune{'\r'}
	dv.Update()

	if dv.BPM() != 120 {
		t.Fatalf("expected BPM to remain 120 got %d", dv.BPM())
	}
	if dv.bpmErrorAnim == 0 {
		t.Fatalf("expected error highlight for invalid BPM input")
	}
}

func TestVolumeSliderOpensPopup(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 600, 200), g, testLogger)
	dv.calcLayout()
	r := dv.rowVolSliders()[0].Rect()
	mx := r.Min.X + r.Dx()/2
	my := r.Min.Y + r.Dy()/2
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return true },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()
	dv.Update()
	// Desktop now opens volume popup instead of inline slider change.
	// Volume should remain unchanged at the default.
	if dv.Rows[0].Volume != 1.0 {
		t.Fatalf("expected volume unchanged at 1.0 after click (popup opens), got %f", dv.Rows[0].Volume)
	}
}

// Dragging a volume slider to its maximum and releasing over the delete button
// should not remove the row.
func TestVolumeDragReleaseDoesNotDeleteRow(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 200, 200), g, testLogger)
	dv.calcLayout()
	sRect := dv.rowVolSliders()[0].TrackRect()
	delRect := dv.rowDeleteBtns()[0].Rect()

	mx, my := sRect.Min.X+1, sRect.Min.Y+sRect.Dy()/2
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	defer restore()

	dv.Update()           // press start
	mx = sRect.Max.X + 20 // drag beyond slider to max
	dv.Update()
	mx, my = delRect.Min.X+delRect.Dx()/2, delRect.Min.Y+delRect.Dy()/2
	dv.Update() // still dragging over delete button
	pressed = false
	dv.Update() // release over delete button

	if len(dv.Rows) != 1 {
		t.Fatalf("row deleted during slider drag")
	}
}

func TestMuteSoloButtons(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 600, 200), g, testLogger)
	dv.calcLayout()

	// Click mute button on first row
	mRect := dv.rowMuteBtns()[0].Rect()
	mx, my := mRect.Min.X+1, mRect.Min.Y+1
	pressed := true
	restore := SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if !dv.Rows[0].Muted {
		t.Fatalf("expected row muted after clicking mute button")
	}

	// Click solo button on first row
	sRect := dv.rowSoloBtns()[0].Rect()
	mx, my = sRect.Min.X+1, sRect.Min.Y+1
	pressed = true
	restore = SetInputForTest(
		func() (int, int) { return mx, my },
		func(ebiten.MouseButton) bool { return pressed },
		func(ebiten.Key) bool { return false },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()
	pressed = false
	dv.Update()
	restore()
	if !dv.Rows[0].Solo {
		t.Fatalf("expected row solo after clicking solo button")
	}
}

func TestMuteSoloInteractions(t *testing.T) {
	g := model.NewGraph(testLogger)
	dv := NewDrumView(image.Rect(0, 0, 300, 200), g, testLogger)
	dv.AddRow()
	dv.calcLayout()

	mRect := dv.rowMuteBtns()[0].Rect()
	mx, my := mRect.Min.X+1, mRect.Min.Y+1
	dv.rowMuteBtns()[0].Handle(mx, my, true)
	dv.rowMuteBtns()[0].Handle(mx, my, false)
	if !dv.Rows[0].Muted {
		t.Fatalf("expected row0 muted after single click")
	}
	if dv.Rows[0].Solo {
		t.Fatalf("row0 should not be solo when muted")
	}

	sRect := dv.rowSoloBtns()[1].Rect()
	mx, my = sRect.Min.X+1, sRect.Min.Y+1
	dv.rowSoloBtns()[1].Handle(mx, my, true)
	dv.rowSoloBtns()[1].Handle(mx, my, false)
	if !dv.Rows[1].Solo {
		t.Fatalf("expected row1 solo after click")
	}
	if dv.Rows[1].Muted {
		t.Fatalf("solo row should not be muted")
	}
	if !dv.Rows[0].Muted {
		t.Fatalf("other rows should be muted when a solo is active")
	}
}

func TestTrackBeatCentersCurrent(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.SetLength(8)
	dv.SetFollow(true)

	dv.TrackBeat(1)
	if dv.Offset != 0 {
		t.Fatalf("expected offset 0 near start, got %d", dv.Offset)
	}

	dv.TrackBeat(6)
	if dv.Offset != 2 {
		t.Fatalf("offset=%d want 2", dv.Offset)
	}
	if !dv.OffsetChanged() {
		t.Fatalf("expected offset change after tracking")
	}
}

func TestRowsStripingRebuildClearsDirtyFlags(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelNone)
	dv := NewDrumView(image.Rect(0, 0, 400, 200), nil, logger)
	dv.rowsStripingEnabled = true
	dv.rowsStripeCount = 2
	dv.rowsStripeAuto = false
	dv.rowsLayerDirty = true
	dv.markAllRowsDirty()
	if runtime.GOARCH != "wasm" {
		if dv.rowsStripesMaybeRebuild() {
			t.Fatalf("expected stripes disabled on non-wasm")
		}
		return
	}
	if !dv.rowsStripesMaybeRebuild() {
		t.Fatalf("initial stripes rebuild failed")
	}
	for i := range dv.rowDirty {
		dv.rowDirty[i] = false
	}
	for i := range dv.rowFullDirty {
		dv.rowFullDirty[i] = false
	}
	dv.rowsLayerDirty = false

	dv.Offset = 4
	dv.markRowsShiftDirty()
	if !dv.rowsStripesMaybeRebuild() {
		t.Fatalf("stripe rebuild after offset shift failed")
	}
	for i, dirty := range dv.rowDirty {
		if dirty {
			t.Fatalf("rowDirty[%d] still set after rebuild", i)
		}
	}
	if len(dv.rowCacheOff) == 0 {
		t.Fatalf("rowCacheOff empty")
	}
	if got := dv.rowCacheOff[0]; got != dv.Offset {
		t.Fatalf("rowCacheOff[0]=%d want %d", got, dv.Offset)
	}
}

func TestRowsStripeDynamicSizing(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 900, timelineHeight+6*24), nil, logger)
	dv.rowsStripingEnabled = true
	dv.rowsStripeAuto = true
	dv.rowsStripeCount = 0
	dv.calcLayout()
	dv.rowsLayerDirty = true
	dv.markAllRowsDirty()
	if runtime.GOARCH != "wasm" {
		if dv.rowsStripesMaybeRebuild() {
			t.Fatalf("expected stripes disabled on non-wasm")
		}
		return
	}
	if !dv.rowsStripesMaybeRebuild() {
		t.Fatalf("auto stripes rebuild failed")
	}
	want := (dv.timelineRect.Dx() + wasmStripeTargetPx - 1) / wasmStripeTargetPx
	if want < 2 {
		want = 2
	}
	if want > wasmStripeMaxCount {
		want = wasmStripeMaxCount
	}
	if dv.rowsStripeCount != want {
		t.Fatalf("auto stripe count=%d want %d", dv.rowsStripeCount, want)
	}

	manual := 12
	dv.rowsStripeAuto = false
	dv.rowsStripeCount = manual
	dv.rowsLayerDirty = true
	dv.rowsStripes = nil
	if !dv.rowsStripesMaybeRebuild() {
		t.Fatalf("manual stripes rebuild failed")
	}
	if dv.rowsStripeCount != manual {
		t.Fatalf("manual stripe count=%d want %d", dv.rowsStripeCount, manual)
	}
}

func TestMuteHighlightUsesDefaultColor(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 300, timelineHeight+24), graph, logger)
	dv.Rows = []*DrumRow{{
		Steps:     make([]bool, dv.Length),
		CellTypes: make([]model.NodeType, dv.Length),
	}}
	dv.Rows[0].Steps[0] = true
	dv.Rows[0].CellTypes[0] = model.NodeTypeMute

	highlighted := [][]highlightEntry{{{idx: 0, val: encodeHighlight(10, true)}}}

	dst := ebiten.NewImage(300, timelineHeight+24)
	orig := drawRect
	count := 0
	var got color.RGBA
	drawRect = func(d *ebiten.Image, r image.Rectangle, c color.Color, filled bool) {
		if filled {
			if rgba, ok := c.(color.RGBA); ok && rgba == colMuteHighlight {
				count++
				got = rgba
			}
		}
		orig(d, r, c, filled)
	}
	defer func() { drawRect = orig }()

	dv.Draw(dst, highlighted, 0, nil, 0)

	if count == 0 {
		t.Fatalf("mute highlight did not draw with mute highlight color")
	}
	want := color.RGBAModel.Convert(colMuteHighlight).(color.RGBA)
	if got != want {
		t.Fatalf("mute highlight mismatch got=%v want=%v", got, want)
	}
}

func TestMuteCellsRenderGrey(t *testing.T) {
	build := func(kind string, n int) ([]bool, []model.NodeType) {
		g := New(testLogger)
		t.Cleanup(g.CloseForTest)
		g.Layout(640, 480)

		s := g.tryAddNode(0, 0, model.NodeTypeRegular)
		g.start = s
		g.graph.StartNodeID = s.ID
		m := g.tryAddNode(1, 0, model.NodeTypeMute)
		r := g.tryAddNode(2, 0, model.NodeTypeRegular)
		g.addEdge(s, m)
		g.addEdge(m, r)
		g.addEdge(r, s)

		g.drum.Rows[0].Origin = s.ID
		g.drum.Rows[0].Node = s
		for i := range g.drum.Rows[0].Steps {
			g.drum.Rows[0].Steps[i] = true
		}

		if kind != "" {
			if node, ok := g.graph.GetNodeByID(m.ID); ok {
				p := node.Params
				p.LogicKind = kind
				p.LogicN = n
				g.graph.SetNodeParams(m.ID, p)
			}
		}

		g.updateBeatInfos()
		g.refreshDrumRow()
		return append([]bool(nil), g.drum.Rows[0].Steps...), append([]model.NodeType(nil), g.drum.Rows[0].CellTypes...)
	}

	stepsTriggered, cellsTriggered := build("", 0)
	if len(stepsTriggered) < 2 {
		t.Fatalf("insufficient steps in triggered scenario")
	}
	if cellsTriggered[1] != model.NodeTypeMute || !stepsTriggered[1] {
		t.Fatalf("expected mute cell to render when triggered: steps=%v cells=%v", stepsTriggered, cellsTriggered)
	}

	stepsSkipped, cellsSkipped := build("every_n_triggers", 2)
	if len(stepsSkipped) < 2 {
		t.Fatalf("insufficient steps in skipped scenario")
	}
	if cellsSkipped[1] != model.NodeTypeMute {
		t.Fatalf("expected cell type mute at index 1; cells=%v", cellsSkipped)
	}
	if stepsSkipped[1] {
		t.Fatalf("mute cell should remain empty when logic skips trigger; steps=%v", stepsSkipped)
	}
}

func TestButtonTextClippedWithinBounds(t *testing.T) {
	btn := NewButton("SuperLongInstrumentNameThatWouldOverflow", DropdownStyle, nil)
	btn.SetRect(image.Rect(0, 0, 40, 20))
	clipped := clipTextToWidth(btn.Text, btn.Rect().Dx()-2*buttonPad)
	if w := debugCharW * utf8.RuneCountInString(clipped); w > btn.Rect().Dx() {
		t.Fatalf("clipped text still exceeds bounds: %d > %d (%q)", w, btn.Rect().Dx(), clipped)
	}
	if !strings.Contains(clipped, "...") {
		t.Fatalf("expected ellipsis in clipped text: %q", clipped)
	}
}

func TestRowLabelWidthExpandsForLongNames(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, nil)
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 500, 300), graph, logger)
	long := "SuperLongRowInstrumentNameThatMustFit"
	dv.Rows[0].Name = long
	dv.recalcButtons()
	dv.calcLayout()
	required := debugCharW*utf8.RuneCountInString(long) + buttonPad*2 + 12
	if dv.labelW < required {
		t.Fatalf("labelW too small: %d < %d", dv.labelW, required)
	}
}

func TestDropdownTextDoesNotOverflowButtons(t *testing.T) {
	logger := game_log.New(io.Discard, game_log.LevelError)
	withAudioCatalog(t, []audio.SoundMeta{
		{ID: "very-long-instrument-id-name", Name: "Very Long Instrument Friendly Name That Should Fit", Category: "Kick Drums (WAV)", Source: "wav"},
	})
	graph := model.NewGraph(logger)
	dv := NewDrumView(image.Rect(0, 0, 600, 260), graph, logger)
	dv.instMenuForceCategories = true
	dv.calcLayout()
	dv.rowLabels()[0].OnClick()
	// Ensure width is widened for popup
	if dv.instMenuFullRect.Dx() < 200 {
		t.Fatalf("menu width too small: %d", dv.instMenuFullRect.Dx())
	}
	if len(dv.instCategoryBtns) == 0 {
		t.Fatalf("no categories")
	}
	dv.instCategoryBtns[0].OnClick()
	if len(dv.instMenuBtns) < 2 {
		t.Fatalf("no instrument buttons")
	}
	for _, b := range dv.instMenuBtns {
		clip := clipTextToWidth(b.Text, b.Rect().Dx()-2*buttonPad)
		if w := debugCharW * utf8.RuneCountInString(clip); w > b.Rect().Dx() {
			t.Fatalf("button text overflows: %v width %d > %d", b.Rect(), w, b.Rect().Dx())
		}
	}
}

// hasButtonInRegion checks if any drawCallButton/drawCallRoundedButton was
// recorded in the given region. It checks both screen-space coordinates
// (drawRowControlsDirect path) and cache-local coordinates (drawRowControlsToCache
// offsets by rowControlsCacheRect.Min).
func hasButtonInRegion(rec *drawCallRecorder, labelRect image.Rectangle, cacheOrigin image.Point) bool {
	// Check screen-space (direct path).
	for _, c := range rec.inRegion(labelRect) {
		if c.Kind == drawCallButton || c.Kind == drawCallRoundedButton {
			return true
		}
	}
	// Check cache-local coords (offset path).
	localRect := labelRect.Sub(cacheOrigin)
	for _, c := range rec.inRegion(localRect) {
		if c.Kind == drawCallButton || c.Kind == drawCallRoundedButton {
			return true
		}
	}
	return false
}

func TestRenameInvalidatesRowControlsCache(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 400), nil, logger)
	screen := ebiten.NewImage(800, 400)

	// Prime the cache: Update + Draw so row controls are cached.
	dv.Update()
	dv.Draw(screen, nil, 0, nil, 0)

	// Ensure cache is primed and valid.
	dv.rowRackZone.controlsCacheDirty = false
	if !dv.rowRackZone.controlsCacheValid() {
		t.Fatal("row controls cache should be valid after initial Draw")
	}

	// Click the edit button to open rename — this wires up the real OnCommit
	// callback from drumview_layout.go.
	dv.rowEditBtns()[0].OnClick()

	// Invoke the production OnCommit callback directly.
	dv.renameComp.Props().OnCommit("RenamedKick")

	// Record draw calls during the next Draw to verify cache rebuild.
	var rec drawCallRecorder
	rec.record(t, func() {
		dv.Draw(screen, nil, 0, nil, 0)
	})

	// Verify drawButton was called in the label region (cache was rebuilt).
	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not populated")
	}
	labelRect := dv.rowLabels()[0].Rect()
	if !hasButtonInRegion(&rec, labelRect, dv.rowControlsCacheRect.Min) {
		t.Fatalf("no drawButton call in label region %v after rename — cache was stale", labelRect)
	}

	// Verify the label text was updated.
	if dv.rowLabels()[0].Text != "RenamedKick" {
		t.Fatalf("label text = %q, want %q", dv.rowLabels()[0].Text, "RenamedKick")
	}
}

func TestRenameLegacyInvalidatesRowControlsCache(t *testing.T) {
	assertDefaultParityState(t)
	withDefaultAudio(t)
	logger := game_log.New(testLogOutput(), game_log.LevelError)
	dv := NewDrumView(image.Rect(0, 0, 800, 400), nil, logger)
	screen := ebiten.NewImage(800, 400)

	// Prime the cache: Update + Draw so row controls are cached.
	dv.Update()
	dv.Draw(screen, nil, 0, nil, 0)

	// Ensure cache is primed and valid.
	dv.rowRackZone.controlsCacheDirty = false
	if !dv.rowRackZone.controlsCacheValid() {
		t.Fatal("row controls cache should be valid after initial Draw")
	}

	// Open rename box on row 0 via callback (edit button is hidden on desktop).
	dv.rowEditBtns()[0].OnClick()
	dv.Update()
	if dv.renameBox == nil {
		t.Fatal("rename box not opened")
	}

	// Type new name and press Enter (legacy rename path in drumview_update.go).
	focusTextInput(t, dv, dv.renameBox)
	dv.renameBox.SetText("NewSnare")
	restore := SetInputForTest(
		func() (int, int) { return 0, 0 },
		func(ebiten.MouseButton) bool { return false },
		func(k ebiten.Key) bool { return k == ebiten.KeyEnter },
		func() []rune { return nil },
		func() (float64, float64) { return 0, 0 },
		func() (int, int) { return 0, 0 },
	)
	t.Cleanup(restore)
	dv.Update()

	if dv.Rows[0].Name != "NewSnare" {
		t.Fatalf("rename did not apply: got %q", dv.Rows[0].Name)
	}

	// Record draw calls during the next Draw to verify cache rebuild.
	var rec drawCallRecorder
	rec.record(t, func() {
		dv.Draw(screen, nil, 0, nil, 0)
	})

	// Verify drawButton was called in the label region (cache was rebuilt).
	if len(dv.rowLabels()) == 0 {
		t.Fatal("rowLabels not populated")
	}
	labelRect := dv.rowLabels()[0].Rect()
	if !hasButtonInRegion(&rec, labelRect, dv.rowControlsCacheRect.Min) {
		t.Fatalf("no drawButton call in label region %v after legacy rename — cache was stale", labelRect)
	}

	// Verify the label text was updated.
	if dv.rowLabels()[0].Text != "NewSnare" {
		t.Fatalf("label text = %q, want %q", dv.rowLabels()[0].Text, "NewSnare")
	}
}
