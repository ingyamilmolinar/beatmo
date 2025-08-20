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

        g.drum.bpmIncPressed = true

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

