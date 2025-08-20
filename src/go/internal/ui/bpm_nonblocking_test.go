//go:build test

package ui

import (
	"testing"
	"time"

	"github.com/ingyamilmolinar/tunkul/core/model"
	"github.com/ingyamilmolinar/tunkul/internal/audio"
)

func TestBPMChangeNonBlocking(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.playing = true

	block := make(chan struct{})
	audio.SetBPMFunc = func(int) { <-block }
	defer func() { audio.SetBPMFunc = func(int) {} }()

	g.drum.SetBPM(g.drum.BPM() + 1)

	done := make(chan struct{})
	go func() {
		g.Update()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(50 * time.Millisecond):
		t.Fatalf("update blocked on BPM change")
	}

	close(block)
	<-done
}

func TestBPMButtonHoldNonBlocking(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.playing = true

	block := make(chan struct{})
	audio.SetBPMFunc = func(int) { <-block }
	defer func() { audio.SetBPMFunc = func(int) {} }()

	// warm up engine so scheduler progress is available
	for i := 0; i < 30; i++ {
		g.drum.bpmIncBtn.OnClick()
		done := make(chan struct{})
		go func() {
			g.Update()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(50 * time.Millisecond):
			t.Fatalf("update blocked on BPM hold")
		}
	}

	close(block)
	g.playing = false
	g.engine.Stop()
	if g.bpm <= 120 {
		t.Fatalf("expected BPM to increase, got %d", g.bpm)
	}
}

func TestBPMHoldDoesNotStallPulseProgress(t *testing.T) {
	g := New(testLogger)
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
	g.playing = true

	block := make(chan struct{})
	audio.SetBPMFunc = func(int) { <-block }
	defer func() { audio.SetBPMFunc = func(int) {} }()

	for i := 0; i < 20; i++ {
		g.drum.bpmIncBtn.OnClick()
		if err := g.Update(); err != nil {
			t.Fatalf("update failed: %v", err)
		}
	}
	if p.t <= 0 {
		t.Fatalf("pulse progress stalled")
	}
	close(block)
	g.playing = false
	g.engine.Stop()
}

func TestBPMChangeUpdatesEngineAsync(t *testing.T) {
        g := New(testLogger)
        g.Layout(640, 480)
        g.playing = true

        target := g.drum.BPM() + 10
        g.drum.SetBPM(target)
        g.Update()

       deadline := time.Now().Add(50 * time.Millisecond)
       for time.Now().Before(deadline) {
               if g.engine.BPM() == target && g.appliedBPM == target {
                       return
               }
               time.Sleep(5 * time.Millisecond)
               g.Update()
       }
       t.Fatalf("engine BPM not updated: %d", g.engine.BPM())
}
