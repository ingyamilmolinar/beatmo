//go:build test

package ui

import (
	"testing"

	"github.com/ingyamilmolinar/beatmo/core/model"
	"github.com/ingyamilmolinar/beatmo/internal/audio"
)

func TestBPMChangeNonBlocking(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.SetPlaying(true)

	block := make(chan struct{})
	blockClosed := false
	t.Cleanup(func() {
		if !blockClosed {
			close(block)
		}
	})
	prevSetBPM := audio.BPMFuncForTest()
	audio.SetBPMFuncForTest(func(int) { <-block })
	defer func() { audio.SetBPMFuncForTest(prevSetBPM) }()

	g.drum.SetBPM(g.drum.BPM() + 1)

	done := make(chan struct{})
	go func() {
		g.Update()
		close(done)
	}()

	waitForChan(t, done, 10000)

	close(block)
	blockClosed = true
	waitForChan(t, done, 10000)
}

func TestBPMButtonHoldNonBlocking(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.SetPlaying(true)

	block := make(chan struct{})
	blockClosed := false
	t.Cleanup(func() {
		if !blockClosed {
			close(block)
		}
	})
	prevSetBPM := audio.BPMFuncForTest()
	audio.SetBPMFuncForTest(func(int) { <-block })
	defer func() { audio.SetBPMFuncForTest(prevSetBPM) }()

	// warm up engine so scheduler progress is available
	for i := 0; i < 30; i++ {
		g.drum.bpmIncBtn().OnClick()
		done := make(chan struct{})
		go func() {
			g.Update()
			close(done)
		}()
		waitForChan(t, done, 10000)
	}

	close(block)
	blockClosed = true
	stopPlaybackForTest(g)
	g.engine.Stop()
	if g.bpm <= 120 {
		t.Fatalf("expected BPM to increase, got %d", g.bpm)
	}
}

func TestBPMHoldDoesNotStallPulseProgress(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	// set up simple path with two nodes and an edge
	n1 := g.tryAddNode(0, 0, model.NodeTypeRegular)
	n2 := g.tryAddNode(1, 0, model.NodeTypeRegular)
	g.addEdge(n1, n2)
	g.updateBeatInfos()
	g.spawnPulseFromRow(0, 0)
	if len(g.activePulses) != 1 {
		t.Fatalf("expected one active pulse, got %d", len(g.activePulses))
	}
	p := g.activePulses[0]
	g.SetPlaying(true)

	block := make(chan struct{})
	blockClosed := false
	t.Cleanup(func() {
		if !blockClosed {
			close(block)
		}
	})
	prevSetBPM := audio.BPMFuncForTest()
	audio.SetBPMFuncForTest(func(int) { <-block })
	defer func() { audio.SetBPMFuncForTest(prevSetBPM) }()

	for i := 0; i < 5; i++ {
		g.drum.bpmIncBtn().OnClick()
		if err := g.Update(); err != nil {
			t.Fatalf("update failed: %v", err)
		}
	}
	if p.t <= 0 {
		t.Fatalf("pulse progress stalled")
	}
	close(block)
	blockClosed = true
	stopPlaybackForTest(g)
	g.engine.Stop()
}

func TestBPMChangeUpdatesEngineAsync(t *testing.T) {
	assertDefaultParityState(t)
	g := New(testLogger)
	t.Cleanup(g.CloseForTest)
	g.Layout(640, 480)
	g.SetPlaying(true)

	target := g.drum.BPM() + 10
	g.drum.SetBPM(target)
	g.Update()

	waitForUpdateCond(t, g, 2000, func() bool {
		return g.engine.BPM() == target && g.AppliedBPM() == target
	})
}
