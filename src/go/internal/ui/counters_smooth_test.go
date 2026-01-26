package ui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/ingyamilmolinar/tunkul/core/model"
)

// parseMS extracts the current time in milliseconds from a timelineInfo string.
func parseMS(info string) int {
	p := strings.Index(info, "Time ")
	if p < 0 {
		return -1
	}
	part := info[p+5:]
	slash := strings.Index(part, "/")
	if slash < 0 {
		return -1
	}
	left := part[:slash]
	var s, ms int
	fmt.Sscanf(left, "%ds %dms", &s, &ms)
	return s*1000 + ms
}

// Verify that the displayed beat counter advances smoothly (strictly
// increasing) across frames and that the formatted time in milliseconds tracks
// it monotonically without large jumps.
func TestCountersAdvanceSmoothly(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)

	// 1-beat segment from (0,0) -> (div,0)
	div := g.grid.MaxDiv()
	n0 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n1 := g.tryAddNode(div, 0, model.NodeTypeRegular)
	g.addEdge(n0, n1)
	g.updateBeatInfos()

	// Fix applied BPM and secPerBeat immediately to avoid async waiting.
	g.drum.SetBPM(120)
	g.SetAppliedBPMForTest(g.drum.BPM())
	// Drive secPerBeat via the draw path once.
	g.drawDrumPane(ebiten.NewImage(1, 1))

	// Start playback via the UI path, then clear time anchors so displayBeat
	// uses deterministic engineProgress values.
	pressPlay(t, g.drum)
	if err := g.Update(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if !g.Playing() {
		t.Fatal("not playing")
	}
	g.SetPlayStartForTest(time.Time{})
	g.SetAudioStartForTest(0)
	g.SetLastDisplayBeatForTest(0)
	g.state.SetLastProg(0)
	g.engineProgress = func() float64 { return 0 }

	// Collect several samples across frames.
	const samples = 10
	beats := make([]float64, 0, samples)
	mss := make([]int, 0, samples)
	progress := 0.0
	step := 0.05
	for i := 0; i < samples; i++ {
		progress += step
		g.engineProgress = func() float64 { return progress }
		b := g.displayBeat()
		beats = append(beats, b)
		info := g.drum.timelineInfo(b)
		mss = append(mss, parseMS(info))
	}

	// Ensure strictly increasing and reasonable deltas.
	for i := 1; i < len(beats); i++ {
		if !(beats[i] > beats[i-1]) {
			t.Fatalf("beat not increasing: %.6f -> %.6f", beats[i-1], beats[i])
		}
		// At 120 BPM and step-by-step subdivision increments, delta is small.
		if d := beats[i] - beats[i-1]; d <= 0 || d > 0.2 {
			t.Fatalf("beat delta out of range: %.6f", d)
		}
	}
	for i := 1; i < len(mss); i++ {
		if !(mss[i] > mss[i-1]) {
			t.Fatalf("ms not increasing: %d -> %d", mss[i-1], mss[i])
		}
		if d := mss[i] - mss[i-1]; d < 1 || d > 100 {
			t.Fatalf("ms delta out of range: %d", d)
		}
	}
}
