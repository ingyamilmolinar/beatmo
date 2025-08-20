//go:build test

package ui

import (
	"testing"
	"time"

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

func TestBPMChangeUpdatesEngineAsync(t *testing.T) {
	g := New(testLogger)
	g.Layout(640, 480)
	g.playing = true

	target := g.drum.BPM() + 10
	g.drum.SetBPM(target)
	g.Update()

	deadline := time.Now().Add(50 * time.Millisecond)
	for time.Now().Before(deadline) {
		if g.engine.BPM() == target {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("engine BPM not updated: %d", g.engine.BPM())
}
